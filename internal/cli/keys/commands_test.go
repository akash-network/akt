package keys

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	kmultisig "github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
	"pkg.akt.dev/akt/internal/output"
)

const keysTestAddress = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"

type ledgerPrefixKeyring struct {
	sdkkeyring.Keyring
	prefix string
	stop   error
}

func (keyring *ledgerPrefixKeyring) Key(string) (*sdkkeyring.Record, error) {
	return nil, errors.New("key not found")
}

func (keyring *ledgerPrefixKeyring) SaveLedgerKey(
	_ string,
	_ sdkkeyring.SignatureAlgo,
	prefix string,
	_, _, _ uint32,
) (*sdkkeyring.Record, error) {
	keyring.prefix = prefix

	return nil, keyring.stop
}

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

// recordedAction is one action-log write captured from a keys command.
type recordedAction struct {
	action  string
	err     error
	details map[string]string
}

// runKeysCommand runs a keys command with no recorder attached, which is what
// happens when no context is selected: recording is skipped and the command
// behaves exactly the same.
func runKeysCommand(t *testing.T, kr sdkkeyring.Keyring, args ...string) (string, error) {
	t.Helper()

	return runKeysCommandWith(t, kr, nil, args...)
}

// runKeysCommandRecorded runs a keys command with a capturing recorder and
// returns what it recorded alongside the command result.
func runKeysCommandRecorded(t *testing.T, kr sdkkeyring.Keyring, args ...string) (string, []recordedAction, error) {
	t.Helper()

	var recorded []recordedAction
	recorder := Recorder(func(_ *cobra.Command, action string, actionErr error, details map[string]string) {
		recorded = append(recorded, recordedAction{action: action, err: actionErr, details: details})
	})

	out, err := runKeysCommandWith(t, kr, recorder, args...)

	return out, recorded, err
}

func runKeysCommandWith(t *testing.T, kr sdkkeyring.Keyring, recorder Recorder, args ...string) (string, error) {
	t.Helper()

	cmd := Commands(func() (sdkkeyring.Keyring, error) { return kr, nil }, recorder)
	cmd.PersistentFlags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "Output format: pretty, json, yaml")
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return out.String(), err
}

func runKeysCommandWithInput(t *testing.T, kr sdkkeyring.Keyring, input string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	cmd := Commands(func() (sdkkeyring.Keyring, error) { return kr, nil }, nil)
	cmd.PersistentFlags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "Output format: pretty, json, yaml")
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out, errOut bytes.Buffer
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)

	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestKeysAddLedgerUsesConfiguredAccountPrefix(t *testing.T) {
	configuredPrefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	if configuredPrefix == "cosmos" {
		t.Fatal("test configuration must distinguish the Akash account prefix from the SDK default")
	}

	stop := errors.New("ledger boundary reached")
	keyring := &ledgerPrefixKeyring{stop: stop}
	_, err := runKeysCommand(t, keyring, "add", "ledger", "--ledger")
	if !errors.Is(err, stop) {
		t.Fatalf("ledger add error = %v, want boundary error", err)
	}
	if keyring.prefix != configuredPrefix {
		t.Errorf("SaveLedgerKey prefix = %q, want configured account prefix %q", keyring.prefix, configuredPrefix)
	}
}

func TestKeysAddReadsCanonicalMultisigThreshold(t *testing.T) {
	kr := testKeyring(t)
	if _, _, err := kr.NewMnemonic(
		"bob",
		sdkkeyring.English,
		"m/44'/118'/0'/0/1",
		"",
		aktkeyring.DefaultAlgo(),
	); err != nil {
		t.Fatal(err)
	}

	if _, err := runKeysCommand(
		t,
		kr,
		"add", "team",
		"--"+flagdefs.FlagMultisig, "alice,bob",
		"--"+flagdefs.FlagMultisigThreshold, "2",
	); err != nil {
		t.Fatal(err)
	}
	record, err := kr.Key("team")
	if err != nil {
		t.Fatal(err)
	}
	pubkey, err := record.GetPubKey()
	if err != nil {
		t.Fatal(err)
	}
	multisig, ok := pubkey.(*kmultisig.LegacyAminoPubKey)
	if !ok || multisig.Threshold != 2 {
		t.Fatalf("team pubkey = %#v", pubkey)
	}
}

func TestKeysAddRecoveryHonorsStructuredOutput(t *testing.T) {
	source := filepath.Join(t.TempDir(), "mnemonic.txt")
	if err := os.WriteFile(source, []byte("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about\n"), 0o600); err != nil {
		t.Fatalf("write mnemonic fixture: %v", err)
	}

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
			out, err := runKeysCommand(t, kr,
				"add", "recovered", "--recover", "--source", source, "--output", format)
			if err != nil {
				t.Fatalf("add recovered key: %v", err)
			}

			var got map[string]any
			if format == "json" {
				err = json.Unmarshal([]byte(out), &got)
			} else {
				err = yaml.Unmarshal([]byte(out), &got)
			}
			if err != nil {
				t.Fatalf("parse %s key result %q: %v", format, out, err)
			}
			for _, field := range []string{"name", "address", "type"} {
				if value, ok := got[field].(string); !ok || value == "" {
					t.Errorf("%s field %q = %#v", format, field, got[field])
				}
			}
			if _, exists := got["mnemonic"]; exists {
				t.Errorf("recovered key repeated its mnemonic: %#v", got)
			}
		})
	}
}

func TestEmptyKeyListUsesStructuredCommandWriter(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			out, err := runKeysCommand(t, aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec),
				"list", "--output", format)
			if err != nil {
				t.Fatalf("list empty keys: %v", err)
			}
			if !strings.HasSuffix(strings.TrimSpace(out), "[]") {
				t.Fatalf("empty %s key list = %q, want an empty sequence", format, out)
			}
		})
	}
}

func TestKeyRecoveryReadsConfiguredInput(t *testing.T) {
	const mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)

	stdout, stderr, err := runKeysCommandWithInput(t, kr, mnemonic+"\n",
		"add", "recovered", "--recover", "--output", "json")
	if err != nil {
		t.Fatalf("recover from command stdin: %v", err)
	}
	if !strings.Contains(stderr, "Enter your mnemonic") {
		t.Fatalf("recovery prompt = %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode recovery output %q: %v", stdout, err)
	}
	if result["name"] != "recovered" || !strings.HasPrefix(result["address"].(string), "akash1") {
		t.Fatalf("recovery result = %#v", result)
	}
	if _, exists := result["mnemonic"]; exists {
		t.Fatal("recovery output repeated the supplied mnemonic")
	}
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

// only returns the single recorded action, failing when a command recorded a
// different number of them.
func only(t *testing.T, recorded []recordedAction) recordedAction {
	t.Helper()

	if len(recorded) != 1 {
		t.Fatalf("recorded %d actions, want exactly 1: %+v", len(recorded), recorded)
	}

	return recorded[0]
}

// TestKeyMutationsAreRecorded covers the action-log coverage rule for the
// keyring (SPEC §2.2.2): every mutation reports itself, with the key type and
// the full address, under its own dotted action name.
func TestKeyMutationsAreRecorded(t *testing.T) {
	source := filepath.Join(t.TempDir(), "mnemonic.txt")
	if err := os.WriteFile(source, []byte("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about\n"), 0o600); err != nil {
		t.Fatalf("write mnemonic fixture: %v", err)
	}

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)

	// add: a generated local key.
	_, recorded, err := runKeysCommandRecorded(t, kr, "add", "alice")
	if err != nil {
		t.Fatalf("add alice: %v", err)
	}
	entry := only(t, recorded)
	if entry.action != actionKeysAdd || entry.err != nil {
		t.Errorf("add recorded %q (err %v), want %q with no error", entry.action, entry.err, actionKeysAdd)
	}
	if entry.details["name"] != "alice" || entry.details["type"] != keyTypeLocal {
		t.Errorf("add details = %v", entry.details)
	}

	address, err := runKeysCommand(t, kr, "show", "alice", "--address")
	if err != nil {
		t.Fatalf("show alice: %v", err)
	}
	address = strings.TrimSpace(address)
	if entry.details["address"] != address {
		t.Errorf("add recorded address %q, want the full address %q", entry.details["address"], address)
	}

	// Reading a key is not a state change and records nothing.
	if _, recorded, _ = runKeysCommandRecorded(t, kr, "list"); len(recorded) != 0 {
		t.Errorf("read-only list recorded %+v", recorded)
	}
	if _, recorded, _ = runKeysCommandRecorded(t, kr, "show", "alice"); len(recorded) != 0 {
		t.Errorf("read-only show recorded %+v", recorded)
	}

	// add --recover: a distinct action, so an audit reader can tell an
	// imported identity from a freshly generated one.
	_, recorded, err = runKeysCommandRecorded(t, kr, "add", "recovered", "--recover", "--source", source)
	if err != nil {
		t.Fatalf("recover key: %v", err)
	}
	entry = only(t, recorded)
	if entry.action != actionKeysRecover || entry.details["name"] != "recovered" {
		t.Errorf("recover recorded %q with %v", entry.action, entry.details)
	}
	if !strings.HasPrefix(entry.details["address"], "akash1") {
		t.Errorf("recover recorded address %q", entry.details["address"])
	}

	// rename records both names.
	_, recorded, err = runKeysCommandRecorded(t, kr, "rename", "alice", "alice-main")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	entry = only(t, recorded)
	if entry.action != actionKeysRename || entry.details["from"] != "alice" || entry.details["to"] != "alice-main" {
		t.Errorf("rename recorded %q with %v", entry.action, entry.details)
	}

	// delete records the address of the key that is now gone.
	_, recorded, err = runKeysCommandRecorded(t, kr, "delete", "alice-main", "--yes")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	entry = only(t, recorded)
	if entry.action != actionKeysDelete || entry.details["address"] != address {
		t.Errorf("delete recorded %q with %v, want the deleted address %q", entry.action, entry.details, address)
	}
}

// TestFailedKeyMutationIsRecorded covers the failure path: a mutation that
// does not happen is still part of the audit trail, and carries its error.
func TestFailedKeyMutationIsRecorded(t *testing.T) {
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)

	_, recorded, err := runKeysCommandRecorded(t, kr, "rename", "ghost", "ghost-main")
	if err == nil {
		t.Fatal("renaming a missing key must fail")
	}
	entry := only(t, recorded)
	if entry.action != actionKeysRename || entry.err == nil {
		t.Errorf("failed rename recorded %q with err %v", entry.action, entry.err)
	}

	// A rejected confirmation changes nothing and must record nothing.
	if _, _, err := kr.NewMnemonic("bob", sdkkeyring.English, "m/44'/118'/0'/0/0", "", aktkeyring.DefaultAlgo()); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	if _, recorded, _ = runKeysCommandRecorded(t, kr, "delete", "nosuchkey"); len(recorded) != 0 {
		t.Errorf("a lookup failure before the mutation recorded %+v", recorded)
	}
}

// TestKeyRecordingNeverCarriesSecrets is the keyring counterpart of the
// context credential rule: the log records that a key changed, never the
// material that would let someone reproduce it.
func TestKeyRecordingNeverCarriesSecrets(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	source := filepath.Join(t.TempDir(), "mnemonic.txt")
	if err := os.WriteFile(source, []byte(mnemonic+"\n"), 0o600); err != nil {
		t.Fatalf("write mnemonic fixture: %v", err)
	}

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)

	out, recorded, err := runKeysCommandRecorded(t, kr, "add", "generated")
	if err != nil {
		t.Fatalf("add generated key: %v", err)
	}
	// The generated mnemonic is printed for backup, so the printed value is
	// the exact secret that must not have been recorded.
	generated := strings.TrimSpace(out[strings.LastIndex(out, "\n\n")+1:])
	if len(strings.Fields(generated)) < 12 {
		t.Fatalf("could not read the generated mnemonic back from %q", out)
	}

	_, recoveredEntries, err := runKeysCommandRecorded(t, kr, "add", "recovered", "--recover", "--source", source)
	if err != nil {
		t.Fatalf("recover key: %v", err)
	}

	tests := []struct {
		entry  recordedAction
		action string
		name   string
	}{
		{entry: only(t, recorded), action: actionKeysAdd, name: "generated"},
		{entry: only(t, recoveredEntries), action: actionKeysRecover, name: "recovered"},
	}

	for _, test := range tests {
		entry := test.entry
		if entry.action != test.action || entry.err != nil {
			t.Errorf("recorded action = %q with err %v, want %q with no error", entry.action, entry.err, test.action)
		}
		if len(entry.details) != 3 {
			t.Errorf("action %q recorded %d details, want 3: %v", entry.action, len(entry.details), entry.details)
		}
		if got := entry.details["name"]; got != test.name {
			t.Errorf("action %q recorded name %q, want %q", entry.action, got, test.name)
		}
		if got := entry.details["type"]; got != keyTypeLocal {
			t.Errorf("action %q recorded type %q, want %q", entry.action, got, keyTypeLocal)
		}
		if got := entry.details["address"]; !strings.HasPrefix(got, "akash1") {
			t.Errorf("action %q recorded invalid address %q", entry.action, got)
		}
		for key, value := range entry.details {
			if strings.Contains(value, mnemonic) || strings.Contains(value, generated) {
				t.Errorf("action %q recorded mnemonic material in %q", entry.action, key)
			}
		}
	}
}
