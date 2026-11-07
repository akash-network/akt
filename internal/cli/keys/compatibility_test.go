package keys

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256r1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	"pkg.akt.dev/akt/internal/output"
)

func publicKeyJSON(t *testing.T, pk cryptotypes.PubKey) string {
	t.Helper()
	data, err := keyCodec().MarshalInterfaceJSON(pk)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func keyAddress(t *testing.T, kr sdkkeyring.Keyring, name string) string {
	t.Helper()
	rec, err := kr.Key(name)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := rec.GetAddress()
	if err != nil {
		t.Fatal(err)
	}
	return addr.String()
}

func TestPublicKeyImports(t *testing.T) {
	secp := secp256k1.GenPrivKey().PubKey()
	r1, err := secp256r1.GenPrivKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, pk := range []cryptotypes.PubKey{
		secp,
		ed25519.GenPrivKey().PubKey(),
		r1.PubKey(),
		multisig.NewLegacyAminoPubKey(1, []cryptotypes.PubKey{secp}),
	} {
		t.Run(pk.Type(), func(t *testing.T) {
			kr := sdkkeyring.NewInMemory(keyCodec())
			out, actions, err := runKeysCommandRecorded(t, kr, "add", "signer", "--pubkey", publicKeyJSON(t, pk), "-o", "json")
			if err != nil {
				t.Fatal(err)
			}
			rec, err := kr.Key("signer")
			if err != nil || rec.GetType() != sdkkeyring.TypeOffline {
				t.Fatal("public import did not create an offline record")
			}
			actual, err := rec.GetPubKey()
			if err != nil || !actual.Equals(pk) {
				t.Fatal("import changed the public key")
			}
			var result keyAddResult
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			if result.Type != keyTypeOffline || result.Mnemonic != "" || result.Address != keyAddress(t, kr, "signer") {
				t.Fatal("unexpected public import output")
			}
			if len(actions) != 1 || actions[0].action != actionKeysAdd || actions[0].details["type"] != keyTypeOffline {
				t.Fatalf("unexpected import actions: %+v", actions)
			}
			if _, err := runKeysCommand(t, kr, "add", "signer", "--pubkey", publicKeyJSON(t, secp256k1.GenPrivKey().PubKey())); err == nil {
				t.Fatal("public import overwrote an existing key")
			}
		})
	}
	kr := sdkkeyring.NewInMemory(keyCodec())
	if _, err := runKeysCommand(t, kr, "add", "base64", "--pubkey-base64", base64.StdEncoding.EncodeToString(secp.Bytes())); err != nil {
		t.Fatal(err)
	}
	if _, err := runKeysCommand(t, kr, "add", "json", "--pubkey", publicKeyJSON(t, secp)); err != nil {
		t.Fatal(err)
	}
	if keyAddress(t, kr, "base64") != keyAddress(t, kr, "json") {
		t.Fatal("JSON and base64 imports derived different addresses")
	}
}

func TestPublicKeyImportRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		`{}`, `null`, `{`,
		`{"@type":"/cosmos.crypto.secp256k1.PubKey","key":"AA=="}`,
		`{"@type":"/cosmos.crypto.ed25519.PubKey","key":"AA=="}`,
		`{"@type":"/cosmos.crypto.secp256r1.PubKey"}`,
		`{"@type":"/cosmos.crypto.secp256r1.PubKey","key":0}`,
		`{"@type":"/cosmos.crypto.multisig.LegacyAminoPubKey","threshold":"0","public_keys":[]}`,
		`{"@type":"/cosmos.crypto.multisig.LegacyAminoPubKey","threshold":"2","public_keys":[]}`,
		`{"@type":"/cosmos.crypto.multisig.LegacyAminoPubKey","threshold":"1","public_keys":[{}]}`,
	} {
		kr := sdkkeyring.NewInMemory(keyCodec())
		if _, err := runKeysCommand(t, kr, "add", "invalid", "--pubkey", input); err == nil {
			t.Errorf("accepted invalid public key %s", input)
		}
		if _, err := kr.Key("invalid"); err == nil {
			t.Fatal("invalid key was saved")
		}
	}
	for _, args := range [][]string{
		{"--pubkey-base64", "not base64"},
		{"--pubkey-base64", ""},
		{"--pubkey-base64", "AA==", "--key-type", "unknown"},
		{"--pubkey", "{}", "--recover"},
		{"--pubkey", "{}", "--pubkey-base64", "AA=="},
		{"--key-type", "unknown"},
		{"--ledger", "--hd-path", "m/44'/118'/0'/0/0"},
		{"--key-type", "secp256k1", "--algo", "unknown"},
	} {
		if _, err := runKeysCommand(t, sdkkeyring.NewInMemory(keyCodec()), append([]string{"add", "invalid"}, args...)...); err == nil {
			t.Fatalf("accepted invalid flags %v", args)
		}
	}
}

func TestMultisigRejectsKeysWithoutAminoEncoding(t *testing.T) {
	private, err := secp256r1.GenPrivKey()
	if err != nil {
		t.Fatal(err)
	}
	kr := sdkkeyring.NewInMemory(keyCodec())
	if _, err := runKeysCommand(t, kr, "add", "r1", "--pubkey", publicKeyJSON(t, private.PubKey())); err != nil {
		t.Fatal(err)
	}
	if _, err := runKeysCommand(t, kr, "add", "multi", "--multisig", "r1"); err == nil {
		t.Fatal("accepted a multisig member without Amino encoding")
	}
	pk := multisig.NewLegacyAminoPubKey(1, []cryptotypes.PubKey{private.PubKey()})
	if _, err := runKeysCommand(t, kr, "add", "multi-json", "--pubkey", publicKeyJSON(t, pk)); err == nil {
		t.Fatal("accepted a multisig JSON member without Amino encoding")
	}
}

func TestPublicKeyMultisigOrdering(t *testing.T) {
	pks := []cryptotypes.PubKey{secp256k1.GenPrivKey().PubKey(), secp256k1.GenPrivKey().PubKey()}
	if bytes.Compare(pks[0].Address(), pks[1].Address()) < 0 {
		pks[0], pks[1] = pks[1], pks[0]
	}
	kr := sdkkeyring.NewInMemory(keyCodec())
	for i, pk := range pks {
		if _, err := runKeysCommand(t, kr, "add", fmt.Sprintf("signer%d", i), "--pubkey", publicKeyJSON(t, pk)); err != nil {
			t.Fatal(err)
		}
	}
	for _, noSort := range []bool{false, true} {
		args := []string{"add", fmt.Sprintf("multi%t", noSort), "--multisig", "signer0", "--multisig", "signer1", "--multisig-threshold", "2"}
		want := []cryptotypes.PubKey{pks[1], pks[0]}
		if noSort {
			args = append(args, "--nosort")
			want = pks
		}
		if _, err := runKeysCommand(t, kr, args...); err != nil {
			t.Fatal(err)
		}
		rec, _ := kr.Key(args[1])
		pk, _ := rec.GetPubKey()
		if !pk.Equals(multisig.NewLegacyAminoPubKey(2, want)) {
			t.Fatalf("wrong multisig order with nosort=%t", noSort)
		}
	}
	for _, args := range [][]string{
		{"--multisig", "signer0,signer1", "--multisig-threshold", "0"},
		{"--multisig", "signer0,signer1", "--multisig-threshold", "3"},
		{"--multisig", "signer0,signer0"},
		{"--multisig", ""},
	} {
		if _, err := runKeysCommand(t, kr, append([]string{"add", "invalid"}, args...)...); err == nil {
			t.Fatal("invalid multisig was accepted")
		}
	}
	if _, actions, err := runKeysCommandRecorded(t, kr, "add", "preview", "--multisig", "signer0,signer1", "--dry-run"); err != nil || len(actions) != 0 {
		t.Fatalf("multisig dry-run: %v", err)
	}
	if _, err := kr.Key("preview"); err == nil {
		t.Fatal("multisig dry-run persisted a key")
	}
}

func TestKeyDryRunDoesNotOpenPersistentKeyring(t *testing.T) {
	for _, extra := range [][]string{
		{"--no-backup", "--algo", "secp256k1"},
		{"--pubkey", publicKeyJSON(t, secp256k1.GenPrivKey().PubKey())},
	} {
		cmd := Commands(func() (sdkkeyring.Keyring, error) {
			t.Fatal("dry-run opened persistent keyring")
			return nil, nil
		}, func(*cobra.Command, string, error, map[string]string) { t.Fatal("dry-run logged mutation") })
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append([]string{"add", "preview", "--dry-run"}, extra...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInteractiveRecoveryUsesSingleInputStream(t *testing.T) {
	const mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	const passphrase = "synthetic test passphrase"
	kr := sdkkeyring.NewInMemory(keyCodec())
	out, _, err := runKeysCommandWithInput(t, kr, mnemonic+"\n"+passphrase+"\n"+passphrase+"\n", "add", "interactive", "--interactive", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, mnemonic) || strings.Contains(out, passphrase) {
		t.Fatal("interactive recovery printed supplied secrets")
	}
	want := sdkkeyring.NewInMemory(keyCodec())
	algos, _ := want.SupportedAlgorithms()
	if _, err := want.NewAccount("expected", mnemonic, passphrase, "m/44'/118'/0'/0/0", algos[0]); err != nil {
		t.Fatal(err)
	}
	if keyAddress(t, kr, "interactive") != keyAddress(t, want, "expected") {
		t.Fatal("interactive recovery lost mnemonic or passphrase input")
	}
	if _, _, err := runKeysCommandWithInput(t, kr, mnemonic+"\none\ntwo\n", "add", "mismatch", "--interactive"); err == nil {
		t.Fatal("accepted different passphrase confirmation")
	}
	if _, err := kr.Key("mismatch"); err == nil {
		t.Fatal("passphrase mismatch saved a key")
	}
}

func TestRawKeyExportImportAndRecording(t *testing.T) {
	kr := testKeyring(t)
	for _, flag := range []string{"--unsafe", "--unarmored-hex"} {
		if _, err := runKeysCommand(t, kr, "export", "alice", flag, "--yes"); err == nil {
			t.Fatal("accepted an unpaired raw export flag")
		}
	}
	out, actions, err := runKeysCommandRecorded(t, kr, "export", "alice", "--unsafe", "--unarmored-hex", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimSpace(out)
	if raw, err := hex.DecodeString(key); err != nil || len(raw) != 32 {
		t.Fatal("export did not return a 32-byte hex key")
	}
	if len(actions) != 1 || actions[0].action != actionKeysExport || len(actions[0].details) != 1 {
		t.Fatal("unexpected raw export action log")
	}
	target := sdkkeyring.NewInMemory(keyCodec())
	out, actions, err = runKeysCommandRecorded(t, target, "import-hex", "imported", "0x"+key, "--key-type", "secp256k1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, key) || len(actions) != 1 || actions[0].action != actionKeysImport {
		t.Fatal("unexpected hex import output or action log")
	}
	if keyAddress(t, kr, "alice") != keyAddress(t, target, "imported") {
		t.Fatal("raw export/import changed the address")
	}
	for _, action := range actions {
		for _, value := range action.details {
			if strings.Contains(value, key) {
				t.Fatal("private key leaked into action details")
			}
		}
	}
	for _, invalid := range []string{"", "0", "0x", "zz", strings.Repeat("00", 32), strings.Repeat("ff", 32), strings.Repeat("11", 33)} {
		if _, err := runKeysCommand(t, target, "import-hex", "invalid", invalid); err == nil {
			t.Fatal("invalid private key accepted")
		}
	}
	inputTarget := sdkkeyring.NewInMemory(keyCodec())
	if _, _, err := runKeysCommandWithInput(t, inputTarget, key+"\n", "import-hex", "prompted"); err != nil {
		t.Fatal(err)
	}
	if keyAddress(t, inputTarget, "prompted") != keyAddress(t, target, "imported") {
		t.Fatal("prompted import changed the key")
	}
}

func TestRawExportCancellationDoesNotLog(t *testing.T) {
	cmd := Commands(func() (sdkkeyring.Keyring, error) { return testKeyring(t), nil }, func(*cobra.Command, string, error, map[string]string) { t.Fatal("cancelled export was logged") })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetArgs([]string{"export", "alice", "--unsafe", "--unarmored-hex"})
	if err := cmd.Execute(); err != nil || out.Len() != 0 {
		t.Fatalf("cancelled export returned data or error: %v", err)
	}
}

func TestNamesAndAlgorithmsUseRequestedOutput(t *testing.T) {
	kr := testKeyring(t)
	for _, format := range []string{"pretty", "json", "yaml"} {
		for _, command := range [][]string{{"list", "--list-names"}, {"list-key-types"}} {
			out, actions, err := runKeysCommandRecorded(t, kr, append(command, "-o", format)...)
			if err != nil || len(actions) != 0 {
				t.Fatalf("read-only key command failed: %v", err)
			}
			want := "alice"
			if command[0] == "list-key-types" {
				want = "secp256k1"
			}
			if format == "pretty" {
				if out != want+"\n" {
					t.Fatalf("unexpected names output: %q", out)
				}
			} else {
				var names []string
				if err := yaml.Unmarshal([]byte(out), &names); err != nil || len(names) != 1 || names[0] != want {
					t.Fatalf("unexpected structured names: %q", out)
				}
			}
		}
	}
	if out, err := runKeysCommand(t, sdkkeyring.NewInMemory(keyCodec()), "list", "-n", "-o", "json"); err != nil || strings.TrimSpace(out) != "[]" {
		t.Fatal("empty names list must be an array")
	}
}

func TestEntropyAndConfirmationCompatibility(t *testing.T) {
	input := strings.Repeat("synthetic entropy ", 4)
	hash := sha256.Sum256([]byte(strings.TrimSpace(input)))
	want, err := bip39Mnemonic(hash[:])
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"mnemonic", "--unsafe-entropy", "--yes"}, {"mnemonic", "--unsafe-entropy"}} {
		out, _, err := runKeysCommandWithInput(t, nil, input+"\nyes\n", args...)
		if err != nil || strings.TrimSpace(out) != want {
			t.Fatalf("entropy generation failed: %v", err)
		}
	}
	out, _, err := runKeysCommandWithInput(t, nil, input+"\nno\n", "mnemonic", "--unsafe-entropy")
	if err != nil || out != "" {
		t.Fatal("cancelled entropy generation returned a mnemonic")
	}
	if _, _, err := runKeysCommandWithInput(t, nil, "short\n", "mnemonic", "--unsafe-entropy", "--yes"); err == nil {
		t.Fatal("accepted short entropy")
	}
	kr := testKeyring(t)
	if _, err := runKeysCommand(t, kr, "rename", "alice", "renamed", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runKeysCommandWithInput(t, kr, "no\n", "delete", "renamed", "--force"); err != nil {
		t.Fatal(err)
	}
	keyAddress(t, kr, "renamed")
	if _, err := runKeysCommand(t, kr, "delete", "renamed", "--force", "--yes"); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationPreservesKeysAndRecordsAction(t *testing.T) {
	kr := testKeyring(t)
	want := keyAddress(t, kr, "alice")
	for range 2 {
		_, actions, err := runKeysCommandRecorded(t, kr, "migrate")
		if err != nil || len(actions) != 1 || actions[0].action != actionKeysMigrate || actions[0].details["count"] != "1" {
			t.Fatalf("migration failed or was not recorded: %v", err)
		}
		if keyAddress(t, kr, "alice") != want {
			t.Fatal("migration changed the key")
		}
	}
}

func TestNodeKeyCommandFlagParity(t *testing.T) {
	// Command-local flags from Akash 2.1.0; shared runtime flags are supplied
	// by akt's root. Include the hidden --algo and --offline compatibility flags.
	want := map[string][]string{
		"add":    {"account", "algo", "coin-type", "dry-run", "hd-path", "index", "interactive", "key-type", "ledger", "multisig", "multisig-threshold", "no-backup", "nosort", "pubkey", "pubkey-base64", "recover", "source", "yes"},
		"delete": {"force", "yes"},
		"export": {"unarmored-hex", "unsafe", "yes"},
		"import": {}, "import-hex": {"key-type"},
		"list": {"list-names"}, "list-key-types": {}, "migrate": {},
		"mnemonic": {"unsafe-entropy", "yes"},
		"parse":    {}, "rename": {"yes"},
		"show": {"address", "bech", "device", "multisig-threshold", "pubkey", "qrcode"},
	}
	root := Commands(nil, nil)
	root.PersistentFlags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "Output format")
	for name, flags := range want {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd == root {
			t.Fatalf("missing key command %q", name)
		}
		for _, flag := range flags {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("%s missing --%s", name, flag)
			}
		}
	}
	if root.PersistentFlags().Lookup("offline") == nil {
		t.Fatal("missing offline compatibility flag")
	}
}
