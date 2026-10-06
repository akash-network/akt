package console

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"gopkg.in/yaml.v3"
)

const maxSecretInputBytes = 2 << 20

// SecretValues holds plaintext only inside a request boundary. Its zero value
// means no secret source was supplied; an explicitly empty object is distinct.
type SecretValues struct {
	values map[string]string
}

// EmptySecretValues represents an explicitly supplied empty secret map.
func EmptySecretValues() SecretValues { return SecretValues{values: map[string]string{}} }

// Supplied reports whether a caller supplied a secret source, even an empty one.
func (s SecretValues) Supplied() bool { return s.values != nil }

// Len reports the number of supplied secrets without exposing their values.
func (s SecretValues) Len() int { return len(s.values) }

// Format prevents diagnostic formatting, including %#v, from exposing values.
func (s SecretValues) Format(w fmt.State, _ rune) { _, _ = io.WriteString(w, "[redacted secrets]") }

// ReadSecretValuesFile reads the optional secret source once. A dash selects
// the caller's input stream; files must be regular files, not pipes or devices.
func ReadSecretValuesFile(path string, input io.Reader) (SecretValues, error) {
	if path == "" {
		return SecretValues{}, nil
	}
	if path == "-" {
		if input == nil {
			return SecretValues{}, errors.New("console: secret input stream is unavailable")
		}
		return ReadSecretValues(input)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return SecretValues{}, errors.New("console: secrets file must be a readable regular file")
	}
	file, err := os.Open(path) //nolint:gosec // user-selected secret source
	if err != nil {
		return SecretValues{}, errors.New("console: cannot open secrets file")
	}
	defer func() { _ = file.Close() }()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return SecretValues{}, errors.New("console: secrets file must be a readable regular file")
	}
	return ReadSecretValues(file)
}

// ReadSecretValues accepts one bounded JSON or YAML object of string values.
// Parser errors deliberately omit source snippets, which may contain secrets.
func ReadSecretValues(input io.Reader) (SecretValues, error) {
	raw, err := io.ReadAll(io.LimitReader(input, maxSecretInputBytes+1))
	if err != nil {
		return SecretValues{}, errors.New("console: cannot read secret input")
	}
	if len(raw) > maxSecretInputBytes {
		return SecretValues{}, errors.New("console: secret input exceeds 2 MiB")
	}
	var doc yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&doc) != nil {
		return SecretValues{}, errors.New("console: secrets must be a JSON or YAML object of strings")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return SecretValues{}, errors.New("console: secret input must contain exactly one document")
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return SecretValues{}, errors.New("console: secrets must be a JSON or YAML object of strings")
	}
	node := doc.Content[0]
	if len(node.Content)/2 > 100 {
		return SecretValues{}, errors.New("console: a deployment accepts at most 100 secrets")
	}
	names := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	result := EmptySecretValues()
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !names.MatchString(key.Value) {
			return SecretValues{}, errors.New("console: invalid secret name; use 1-64 letters, digits, or underscores, starting with a letter or underscore")
		}
		if _, exists := result.values[key.Value]; exists {
			return SecretValues{}, errors.New("console: secret names must be unique")
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || value.Anchor != "" {
			return SecretValues{}, errors.New("console: each secret value must be a string without YAML aliases")
		}
		encoded, _ := json.Marshal(value.Value)
		if len(encoded) > 16<<10 {
			return SecretValues{}, errors.New("console: each JSON-encoded secret value must fit within 16 KiB")
		}
		result.values[key.Value] = value.Value
	}
	return result, nil
}

// SecretsContext is the authenticated public encryption material supplied by Console.
type SecretsContext struct {
	Subject        string          `json:"sub"`
	KeyID          string          `json:"kid"`
	JWK            jose.JSONWebKey `json:"jwk"`
	RequiredClaims []string        `json:"requiredClaims"`
}

// GetSecretsContext reads and validates the user's current public sealing key.
func (c *Client) GetSecretsContext(ctx context.Context) (*SecretsContext, error) {
	var result SecretsContext
	if err := c.sensitiveClient().doData(ctx, http.MethodGet, "/v1/sdl-secrets-context", nil, &result); err != nil {
		return nil, err
	}
	if err := validateSecretsContext(result); err != nil {
		return nil, err
	}
	return &result, nil
}

func validateSecretsContext(value SecretsContext) error {
	key, ok := value.JWK.Key.(*rsa.PublicKey)
	if !ok || key.N == nil || key.N.BitLen() < 2048 || key.N.BitLen() > 8192 || key.E < 3 || key.E%2 == 0 ||
		value.JWK.Algorithm != string(jose.RSA_OAEP_256) || value.JWK.Use != "enc" || value.Subject == "" || value.KeyID == "" {
		return errors.New("console: invalid secret encryption context")
	}
	if value.JWK.KeyID != "" && value.JWK.KeyID != value.KeyID {
		return errors.New("console: secret encryption key identifiers disagree")
	}
	claims := make(map[string]bool, len(value.RequiredClaims))
	for _, claim := range value.RequiredClaims {
		switch claim {
		case "sub", "kid", "exp":
			claims[claim] = true
		default:
			return errors.New("console: secret encryption context requires unsupported claims")
		}
	}
	if !claims["sub"] || !claims["kid"] || !claims["exp"] {
		return errors.New("console: secret encryption context omits required claims")
	}
	return nil
}

func sealSecrets(value SecretsContext, secrets SecretValues, rawSDL string, now time.Time) (string, error) {
	if err := validateSecretsContext(value); err != nil {
		return "", err
	}
	options := (&jose.EncrypterOptions{}).
		WithHeader("kid", value.KeyID).
		WithHeader("sub", value.Subject).
		WithHeader("exp", now.Add(5*time.Minute).Unix())
	if rawSDL != "" {
		hash := sha256.Sum256([]byte(rawSDL))
		options.WithHeader("sdlHash", base64.RawURLEncoding.EncodeToString(hash[:]))
	}
	encrypter, err := jose.NewEncrypter(jose.A256GCM, jose.Recipient{Algorithm: jose.RSA_OAEP_256, Key: value.JWK.Key}, options)
	if err != nil {
		return "", errors.New("console: cannot initialize secret encryption")
	}
	values := secrets.values
	if values == nil {
		values = map[string]string{}
	}
	plaintext, _ := json.Marshal(values)
	object, err := encrypter.Encrypt(plaintext)
	if err != nil {
		return "", errors.New("console: cannot encrypt secrets")
	}
	token, err := object.CompactSerialize()
	if err != nil {
		return "", errors.New("console: cannot encode encrypted secrets")
	}
	return token, nil
}

func (c *Client) sealSecrets(ctx context.Context, secrets SecretValues, rawSDL string) (string, error) {
	value, err := c.GetSecretsContext(ctx)
	if err != nil {
		return "", err
	}
	return sealSecrets(*value, secrets, rawSDL, time.Now())
}
