package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFprintJSONSemanticsYAMLHonorsJSONModel(t *testing.T) {
	value := struct {
		CreatedAt json.RawMessage `json:"created_at"`
		Dynamic   json.RawMessage `json:"dynamic"`
	}{
		CreatedAt: json.RawMessage(`"27957328"`),
		Dynamic:   json.RawMessage(`{"sequence":900719925474099312345,"ready":true}`),
	}

	var buf bytes.Buffer
	if err := FprintJSONSemantics(&buf, FormatYAML, value); err != nil {
		t.Fatalf("FprintJSONSemantics: %v", err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(buf.Bytes(), &document); err != nil {
		t.Fatalf("decode YAML: %v", err)
	}
	root := document.Content[0]
	createdAt := yamlMapValue(t, root, "created_at")
	if createdAt.Tag != "!!str" || createdAt.Value != "27957328" {
		t.Errorf("created_at = tag %q value %q, want the JSON string", createdAt.Tag, createdAt.Value)
	}
	if yamlMapValueOptional(root, "createdat") != nil {
		t.Error("YAML must honor the JSON field name created_at")
	}

	dynamic := yamlMapValue(t, root, "dynamic")
	if dynamic.Kind != yaml.MappingNode {
		t.Fatalf("dynamic kind = %d, want a mapping", dynamic.Kind)
	}
	sequence := yamlMapValue(t, dynamic, "sequence")
	if sequence.Tag != "!!int" || sequence.Value != "900719925474099312345" {
		t.Errorf("sequence = tag %q value %q, want the exact JSON integer", sequence.Tag, sequence.Value)
	}
	ready := yamlMapValue(t, dynamic, "ready")
	if ready.Tag != "!!bool" || ready.Value != "true" {
		t.Errorf("ready = tag %q value %q, want true", ready.Tag, ready.Value)
	}
}

func yamlMapValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()

	value := yamlMapValueOptional(node, key)
	if value == nil {
		t.Fatalf("YAML mapping is missing key %q", key)
	}

	return value
}

func yamlMapValueOptional(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}

	return nil
}
