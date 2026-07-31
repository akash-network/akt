package keys

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"gopkg.in/yaml.v3"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
	"pkg.akt.dev/akt/internal/output"
)

const keysTestAddress = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"

func testKeyring(t *testing.T) sdkkeyring.Keyring {
	t.Helper()

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	if _, _, err := kr.NewMnemonic(
		"alice",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	); err != nil {
		t.Fatalf("NewMnemonic: %v", err)
	}

	return kr
}

func runKeysCommand(t *testing.T, kr sdkkeyring.Keyring, args ...string) (string, error) {
	t.Helper()

	cmd := Commands(func() (sdkkeyring.Keyring, error) { return kr, nil })
	cmd.PersistentFlags().VarP(output.NewFormatFlag("pretty"), "output", "o", "Output format: pretty, json, yaml")
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), err
}

func TestKeysShowStructuredOutput(t *testing.T) {
	kr := testKeyring(t)

	jsonOut, err := runKeysCommand(t, kr, "show", "alice", "--output", "json")
	if err != nil {
		t.Fatalf("show JSON: %v", err)
	}
	var jsonValue map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &jsonValue); err != nil {
		t.Fatalf("parse show JSON %q: %v", jsonOut, err)
	}
	for _, field := range []string{"name", "type", "address", "pubkey"} {
		if value, ok := jsonValue[field].(string); !ok || value == "" {
			t.Errorf("show JSON field %q = %#v", field, jsonValue[field])
		}
	}
	if len(jsonValue) != 4 {
		t.Errorf("show JSON keys = %v, want exactly four canonical fields", jsonValue)
	}

	yamlOut, err := runKeysCommand(t, kr, "show", "alice", "--output", "yaml")
	if err != nil {
		t.Fatalf("show YAML: %v", err)
	}
	var yamlValue map[string]any
	if err := yaml.Unmarshal([]byte(yamlOut), &yamlValue); err != nil {
		t.Fatalf("parse show YAML %q: %v", yamlOut, err)
	}
	for _, field := range []string{"name", "type", "address", "pubkey"} {
		if value, ok := yamlValue[field].(string); !ok || value == "" {
			t.Errorf("show YAML field %q = %#v", field, yamlValue[field])
		}
	}
	if len(yamlValue) != 4 {
		t.Errorf("show YAML keys = %v, want exactly four canonical fields", yamlValue)
	}
}

func TestKeysShowAddressOnlyMachineScalar(t *testing.T) {
	kr := testKeyring(t)
	prettyOut, err := runKeysCommand(t, kr, "show", "alice", "--address")
	if err != nil {
		t.Fatalf("show address pretty: %v", err)
	}
	address := strings.TrimSpace(prettyOut)
	if !strings.HasPrefix(address, "akash1") || strings.ContainsAny(address, "\"': ") {
		t.Fatalf("pretty address = %q, want one raw bech32 scalar", prettyOut)
	}

	jsonOut, err := runKeysCommand(t, kr, "show", "alice", "--address", "--output", "json")
	if err != nil {
		t.Fatalf("show address JSON: %v", err)
	}
	var jsonAddress string
	if err := json.Unmarshal([]byte(jsonOut), &jsonAddress); err != nil {
		t.Fatalf("parse address JSON %q: %v", jsonOut, err)
	}
	if jsonAddress != address {
		t.Errorf("JSON address = %q, want %q", jsonAddress, address)
	}

	yamlOut, err := runKeysCommand(t, kr, "show", "alice", "--address", "--output", "yaml")
	if err != nil {
		t.Fatalf("show address YAML: %v", err)
	}
	yamlScalar := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(yamlOut), "---"))
	if !strings.HasPrefix(yamlScalar, `"`) {
		t.Errorf("YAML scalar is not quoted: %q", yamlOut)
	}
	var yamlAddress string
	if err := yaml.Unmarshal([]byte(yamlOut), &yamlAddress); err != nil {
		t.Fatalf("parse address YAML %q: %v", yamlOut, err)
	}
	if yamlAddress != address {
		t.Errorf("YAML address = %q, want %q", yamlAddress, address)
	}
}

func TestKeysParseStructuredOutput(t *testing.T) {
	jsonOut, err := runKeysCommand(t, nil, "parse", keysTestAddress, "--output", "json")
	if err != nil {
		t.Fatalf("parse bech32 JSON: %v", err)
	}
	var bech32Value struct {
		Format    string            `json:"format"`
		HRP       string            `json:"hrp"`
		Hex       string            `json:"hex"`
		Addresses map[string]string `json:"addresses"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &bech32Value); err != nil {
		t.Fatalf("parse bech32 JSON %q: %v", jsonOut, err)
	}
	if bech32Value.Format != "bech32" || bech32Value.HRP != "akash" {
		t.Errorf("bech32 metadata = %+v", bech32Value)
	}
	if bech32Value.Hex == "" || bech32Value.Hex != strings.ToUpper(bech32Value.Hex) {
		t.Errorf("canonical hex = %q", bech32Value.Hex)
	}
	for _, prefix := range []string{"akash", "cosmos", "osmo"} {
		if value := bech32Value.Addresses[prefix]; !strings.HasPrefix(value, prefix+"1") {
			t.Errorf("address %q = %q", prefix, value)
		}
	}

	yamlOut, err := runKeysCommand(t, nil, "parse", "0x"+bech32Value.Hex, "--output", "yaml")
	if err != nil {
		t.Fatalf("parse hex YAML: %v", err)
	}
	var hexValue map[string]any
	if err := yaml.Unmarshal([]byte(yamlOut), &hexValue); err != nil {
		t.Fatalf("parse hex YAML %q: %v", yamlOut, err)
	}
	if hexValue["format"] != "hex" || hexValue["hex"] != bech32Value.Hex {
		t.Errorf("hex YAML = %#v", hexValue)
	}
	if _, ok := hexValue["hrp"]; ok {
		t.Errorf("hex YAML unexpectedly includes hrp: %#v", hexValue)
	}
	addresses, ok := hexValue["addresses"].(map[string]any)
	if !ok || len(addresses) != 3 {
		t.Errorf("hex YAML addresses = %#v", hexValue["addresses"])
	}
}
