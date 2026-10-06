// Package deploymentconfig applies the Console's service configuration edits
// to an SDL without changing its resource or endpoint declarations.
package deploymentconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const maxDocumentBytes = 1 << 20

// Patch matches the editable fields of a Console deployment. Ciphertext is
// added by the Console client, never accepted from a patch document.
type Patch struct {
	Services          map[string]ServicePatch `json:"services,omitempty"`
	Name              *string                 `json:"name,omitempty"`
	IfManifestVersion string                  `json:"ifManifestVersion,omitempty"`
}

// ServicePatch preserves absent, null, and empty arrays for clearable fields.
type ServicePatch struct {
	Image       *string                 `json:"image,omitempty"`
	Command     json.RawMessage         `json:"command,omitempty"`
	Args        json.RawMessage         `json:"args,omitempty"`
	Env         map[string]*string      `json:"env,omitempty"`
	Credentials json.RawMessage         `json:"credentials,omitempty"`
	Expose      map[string]ExposePatch  `json:"expose,omitempty"`
	Storage     map[string]StoragePatch `json:"storage,omitempty"`
}

// ExposePatch addresses an existing container port.
type ExposePatch struct {
	Port        *uint32           `json:"port,omitempty"`
	As          *uint32           `json:"as,omitempty"`
	Accept      *[]string         `json:"accept,omitempty"`
	HTTPOptions *HTTPOptionsPatch `json:"httpOptions,omitempty"`
}

// HTTPOptionsPatch uses Console JSON names; Apply maps them to SDL names.
type HTTPOptionsPatch struct {
	MaxBodySize *uint32   `json:"maxBodySize,omitempty"`
	ReadTimeout *uint32   `json:"readTimeout,omitempty"`
	SendTimeout *uint32   `json:"sendTimeout,omitempty"`
	NextTries   *uint32   `json:"nextTries,omitempty"`
	NextTimeout *uint32   `json:"nextTimeout,omitempty"`
	NextCases   *[]string `json:"nextCases,omitempty"`
}

// StoragePatch edits a declared volume's mount options, never its size.
type StoragePatch struct {
	Mount    *string `json:"mount,omitempty"`
	ReadOnly *bool   `json:"readOnly,omitempty"`
}

// ParsePatch accepts one strict YAML or JSON object. Errors omit user values.
func ParsePatch(data []byte) (Patch, error) {
	document, err := parseObject(data)
	if err != nil {
		return Patch{}, fmt.Errorf("invalid deployment patch: %w", err)
	}
	if err := validatePatchShape(document); err != nil {
		return Patch{}, err
	}
	// parseObject produces only JSON-compatible values.
	raw, _ := json.Marshal(document)
	var patch Patch
	if err := decodeStrict(raw, &patch); err != nil {
		return Patch{}, errors.New("invalid deployment patch: unknown field or incorrect field type")
	}
	if err := Validate(patch); err != nil {
		return Patch{}, err
	}
	return patch, nil
}

// Validate checks the wire vocabulary and bounds. An empty patch is allowed
// here because a caller can supply a separate secret map for rotation.
func Validate(patch Patch) error {
	if patch.Name != nil && (strings.TrimSpace(*patch.Name) == "" || utf8.RuneCountInString(strings.TrimSpace(*patch.Name)) > 256) {
		return errors.New("deployment name must contain 1 to 256 characters")
	}
	if len(patch.IfManifestVersion) > 64 {
		return errors.New("manifest version is too long")
	}
	for _, service := range patch.Services {
		for _, raw := range []json.RawMessage{service.Command, service.Args} {
			if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}
			var values []any
			if err := decodeStrict(raw, &values); err != nil {
				return errors.New("command and args must be arrays of strings or null")
			}
			for _, value := range values {
				if _, ok := value.(string); !ok {
					return errors.New("command and args must contain only strings")
				}
			}
		}
		if len(service.Credentials) != 0 && !bytes.Equal(bytes.TrimSpace(service.Credentials), []byte("null")) {
			var credentials struct {
				Host     *string `json:"host"`
				Username *string `json:"username"`
				Password *string `json:"password"`
			}
			if err := decodeStrict(service.Credentials, &credentials); err != nil || credentials.Host == nil || credentials.Username == nil || credentials.Password == nil {
				return errors.New("credentials must contain host, username, and password strings, or be null")
			}
		}
		for key := range service.Env {
			if !validEnvName(key) {
				return errors.New("env contains an invalid variable name")
			}
		}
		for _, expose := range service.Expose {
			for _, port := range []*uint32{expose.Port, expose.As} {
				if port != nil && (*port == 0 || *port > 65535) {
					return errors.New("port numbers must be between 1 and 65535")
				}
			}
			if expose.HTTPOptions != nil {
				options := expose.HTTPOptions
				if options.MaxBodySize != nil && (*options.MaxBodySize == 0 || *options.MaxBodySize > 104857600) {
					return errors.New("maxBodySize must be between 1 and 104857600")
				}
				if options.ReadTimeout != nil && *options.ReadTimeout == 0 || options.SendTimeout != nil && *options.SendTimeout == 0 {
					return errors.New("readTimeout and sendTimeout must be positive")
				}
				if options.NextCases != nil {
					if len(*options.NextCases) == 0 {
						return errors.New("nextCases must not be empty")
					}
					for _, value := range *options.NextCases {
						switch value {
						case "error", "timeout", "500", "502", "503", "504", "403", "404", "429":
						case "off":
							if len(*options.NextCases) != 1 {
								return errors.New("nextCases off must stand alone")
							}
						default:
							return errors.New("nextCases contains an unsupported condition")
						}
					}
				}
			}
		}
	}
	return nil
}

func validEnvName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i, c := range name {
		if c == '-' || c == '.' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("trailing data")
	}
	return nil
}

// YAML is decoded through its syntax tree so duplicate keys, aliases, and
// additional documents cannot silently overwrite data or expand without bound.
func parseObject(data []byte) (map[string]any, error) {
	if len(data) > maxDocumentBytes {
		return nil, errors.New("document exceeds 1 MiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return nil, errors.New("document is not valid YAML or JSON")
	}
	if !errors.Is(decoder.Decode(new(yaml.Node)), io.EOF) {
		return nil, errors.New("only one document is allowed")
	}
	value, err := nodeValue(node.Content[0], 0, new(int))
	if err != nil {
		return nil, err
	}
	document, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("document must be an object")
	}
	return document, nil
}

func nodeValue(node *yaml.Node, depth int, count *int) (any, error) {
	*count = *count + 1
	if *count > 100000 {
		return nil, errors.New("document expansion exceeds 100000 nodes")
	}
	if depth > 64 {
		return nil, errors.New("document nesting exceeds 64 levels")
	}
	switch node.Kind {
	case yaml.AliasNode:
		return nodeValue(node.Alias, depth+1, count)
	case yaml.MappingNode:
		result := make(map[string]any, len(node.Content)/2)
		seen := make(map[string]bool, len(node.Content)/2)
		// YAML merge keys provide defaults; explicit keys always win.
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Tag != "!!merge" {
				continue
			}
			value, err := nodeValue(node.Content[i+1], depth+1, count)
			if err != nil {
				return nil, err
			}
			merged, ok := value.([]any)
			if !ok {
				merged = []any{value}
			}
			for _, raw := range merged {
				defaults, ok := raw.(map[string]any)
				if !ok {
					return nil, errors.New("YAML merge must name an object")
				}
				for key, value := range defaults {
					if _, exists := result[key]; !exists {
						result[key] = value
					}
				}
			}
		}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" && key.Tag != "!!merge" && key.Tag != "!!int" {
				return nil, errors.New("object keys must be strings or integers")
			}
			keyName := key.Value
			if key.Tag == "!!int" {
				number, err := nodeValue(key, depth+1, count)
				if err != nil {
					return nil, err
				}
				keyName = strconv.FormatFloat(number.(float64), 'f', -1, 64)
			}
			if seen[keyName] {
				return nil, errors.New("document contains a duplicate key")
			}
			seen[keyName] = true
			if key.Tag == "!!merge" {
				continue
			}
			value, err := nodeValue(node.Content[i+1], depth+1, count)
			if err != nil {
				return nil, err
			}
			result[keyName] = value
		}
		return result, nil
	case yaml.SequenceNode:
		values := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := nodeValue(child, depth+1, count)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	default: // The YAML decoder emits only mappings, sequences, aliases, and scalars here.
		if node.Tag != "!!str" && node.Tag != "!!null" && node.Tag != "!!int" && node.Tag != "!!float" && node.Tag != "!!bool" {
			return nil, errors.New("unsupported YAML scalar type")
		}
		var value any
		if err := node.Decode(&value); err != nil {
			return nil, errors.New("invalid scalar")
		}
		// Normalize JSON/YAML numeric types for semantic comparison and port edits.
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, errors.New("invalid scalar")
		}
		_ = json.Unmarshal(raw, &value) // Successful Marshal guarantees valid JSON.
		return value, nil
	}
}

func validatePatchShape(document map[string]any) error {
	if err := checkFields(document, "services name ifManifestVersion", ""); err != nil {
		return err
	}
	services, _ := document["services"].(map[string]any)
	for _, raw := range services {
		service, ok := raw.(map[string]any)
		if !ok {
			return errors.New("service patch must be an object")
		}
		if err := checkFields(service, "image command args env credentials expose storage", "command args credentials"); err != nil {
			return err
		}
		if credentials, ok := service["credentials"].(map[string]any); ok {
			if err := checkFields(credentials, "host username password", ""); err != nil {
				return err
			}
		}
		exposed, _ := service["expose"].(map[string]any)
		for _, raw := range exposed {
			expose, ok := raw.(map[string]any)
			if !ok {
				return errors.New("expose patch must be an object")
			}
			if err := checkFields(expose, "port as accept httpOptions", ""); err != nil {
				return err
			}
			if options, ok := expose["httpOptions"].(map[string]any); ok {
				if err := checkFields(options, "maxBodySize readTimeout sendTimeout nextTries nextTimeout nextCases", ""); err != nil {
					return err
				}
			}
		}
		storage, _ := service["storage"].(map[string]any)
		for _, raw := range storage {
			volume, ok := raw.(map[string]any)
			if !ok {
				return errors.New("storage patch must be an object")
			}
			if err := checkFields(volume, "mount readOnly", ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkFields(object map[string]any, fields, nullable string) error {
	for key, value := range object {
		if !slices.Contains(strings.Fields(fields), key) {
			return errors.New("deployment patch contains an unknown field")
		}
		if value == nil && !slices.Contains(strings.Fields(nullable), key) {
			return errors.New("deployment patch contains null in a nonnullable field")
		}
		if values, ok := value.([]any); ok {
			for _, value := range values {
				if _, ok := value.(string); !ok {
					return errors.New("deployment patch arrays must contain strings")
				}
			}
		}
	}
	return nil
}

// HasReferences detects Console references anywhere in the parsed SDL. It
// returns a parse error rather than allowing invalid data past a rail guard.
func HasReferences(sdl string) (bool, error) {
	document, err := parseObject([]byte(sdl))
	if err != nil {
		return false, err
	}
	return hasReference(document), nil
}

func hasReference(value any) bool {
	switch value := value.(type) {
	case string:
		return strings.Contains(value, "ac-secret://")
	case []any:
		for _, child := range value {
			if hasReference(child) {
				return true
			}
		}
	case map[string]any:
		for _, child := range value {
			if hasReference(child) {
				return true
			}
		}
	}
	return false
}
