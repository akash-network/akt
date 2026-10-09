package console

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

func TestReadSecretValuesBoundaries(t *testing.T) {
	tests := []struct {
		name, input string
		valid       bool
	}{
		{"yaml", "TOKEN: keep-me-private\nEMPTY: \"\"\n", true},
		{"json", `{"TOKEN":"keep-me-private","EMPTY":""}`, true},
		{"empty", `{}`, true},
		{"duplicate", `{"TOKEN":"keep-me-private","TOKEN":"second"}`, false},
		{"number", `{"TOKEN":123}`, false},
		{"null", `{"TOKEN":null}`, false},
		{"list", `["keep-me-private"]`, false},
		{"trailing", "TOKEN: keep-me-private\n---\nTOKEN: second", false},
		{"alias", "TOKEN: &secret keep-me-private\nSECOND: *secret", false},
		{"bad name", `{"9TOKEN":"keep-me-private"}`, false},
		{"long name", fmt.Sprintf(`{"%s":"keep-me-private"}`, strings.Repeat("A", 65)), false},
		{"long value", `{"TOKEN":"` + strings.Repeat("x", 16<<10) + `"}`, false},
		{"input bound", strings.Repeat(" ", maxSecretInputBytes+1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := ReadSecretValues(strings.NewReader(tt.input))
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v error=%v", tt.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "keep-me-private") {
				t.Fatal("error exposes secret")
			}
			if tt.valid && !values.Supplied() {
				t.Fatal("explicit input lost presence")
			}
			if strings.Contains(fmt.Sprintf("%+v %#v", values, values), "keep-me-private") {
				t.Fatal("diagnostic exposes secret")
			}
			encoded, _ := json.Marshal(values) //nolint:staticcheck // Accidental serialization must not expose private fields.
			if strings.Contains(string(encoded), "keep-me-private") {
				t.Fatal("JSON exposes secret")
			}
		})
	}
	entries := make([]string, 101)
	for i := range entries {
		entries[i] = fmt.Sprintf("K%d: value", i)
	}
	if _, err := ReadSecretValues(strings.NewReader(strings.Join(entries, "\n"))); err == nil {
		t.Fatal("accepted too many names")
	}
	zero, err := ReadSecretValuesFile("", nil)
	if err != nil || zero.Supplied() {
		t.Fatal("absent file is not an unsupplied value")
	}
	stdin, err := ReadSecretValuesFile("-", strings.NewReader("{}"))
	if err != nil || !stdin.Supplied() {
		t.Fatal("stdin empty object lost presence")
	}
}

func secretTestContext(t *testing.T) (*rsa.PrivateKey, SecretsContext) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key, SecretsContext{Subject: "test-user", KeyID: "test-key", JWK: jose.JSONWebKey{Key: &key.PublicKey, Algorithm: "RSA-OAEP-256", Use: "enc"}, RequiredClaims: []string{"kid", "sub", "exp"}}
}

// This decoder deliberately uses only crypto/rsa and crypto/cipher, not the
// JOSE encoder under test. It verifies the upstream compact-JWE wire contract.
func decryptSecretToken(t *testing.T, key *rsa.PrivateKey, token string) (map[string]any, map[string]string) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 5 {
		t.Fatalf("compact JWE has %d components", len(parts))
	}
	decoded := make([][]byte, 5)
	for i, p := range parts {
		var err error
		decoded[i], err = base64.RawURLEncoding.DecodeString(p)
		if err != nil {
			t.Fatal(err)
		}
	}
	var header map[string]any
	if err := json.Unmarshal(decoded[0], &header); err != nil {
		t.Fatal(err)
	}
	if header["alg"] != "RSA-OAEP-256" || header["enc"] != "A256GCM" {
		t.Fatalf("wrong algorithms: %v", header)
	}
	cek, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, decoded[1], nil)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := aead.Open(nil, decoded[2], append(decoded[3], decoded[4]...), []byte(parts[0]))
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := json.Unmarshal(plaintext, &values); err != nil {
		t.Fatal(err)
	}
	return header, values
}

func TestSealSecretsInteroperatesWithStandardCrypto(t *testing.T) {
	key, ctx := secretTestContext(t)
	values, err := ReadSecretValues(strings.NewReader(`{"TOKEN":"private-value"}`))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	token, err := sealSecrets(ctx, values, "exact SDL\n", now)
	if err != nil {
		t.Fatal(err)
	}
	header, plain := decryptSecretToken(t, key, token)
	hash := sha256.Sum256([]byte("exact SDL\n"))
	if header["sub"] != "test-user" || header["kid"] != "test-key" || header["exp"] != float64(now.Add(5*time.Minute).Unix()) || header["sdlHash"] != base64.RawURLEncoding.EncodeToString(hash[:]) {
		t.Fatalf("incorrect protected headers: %v", header)
	}
	if plain["TOKEN"] != "private-value" {
		t.Fatal("secret changed during encryption")
	}
	token, err = sealSecrets(ctx, SecretValues{}, "", now)
	if err != nil {
		t.Fatal(err)
	}
	header, plain = decryptSecretToken(t, key, token)
	if _, ok := header["sdlHash"]; ok {
		t.Fatal("PATCH seal must not bind an obsolete SDL")
	}
	if plain == nil || len(plain) != 0 {
		t.Fatal("omitted values must seal an empty object")
	}
}

func TestSecretsContextRejectsUnsupportedMaterial(t *testing.T) {
	_, valid := secretTestContext(t)
	cases := []SecretsContext{valid, valid, valid, valid}
	cases[0].JWK.Algorithm = "RSA1_5"
	cases[1].JWK.Use = "sig"
	cases[2].RequiredClaims = []string{"sub", "exp", "unknown"}
	cases[3].Subject = ""
	for i, value := range cases {
		if err := validateSecretsContext(value); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestSecretsContextReadDoesNotEchoResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"private-value"}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "").GetSecretsContext(context.Background())
	if err == nil || strings.Contains(err.Error(), "private-value") {
		t.Fatalf("unsafe response: %v", err)
	}
}
