package keys

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	sdkkeys "github.com/cosmos/cosmos-sdk/client/keys"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
	"pkg.akt.dev/akt/internal/output"
)

func TestKeysShowNodeFlagParity(t *testing.T) {
	cmd := showCmd(nil, nil)
	sdkkeys.ShowKeysCmd().Flags().VisitAll(func(want *pflag.Flag) {
		got := cmd.Flags().Lookup(want.Name)
		if got == nil {
			t.Errorf("missing node flag --%s", want.Name)
			return
		}
		if got.Shorthand != want.Shorthand || got.DefValue != want.DefValue || got.Value.Type() != want.Value.Type() {
			t.Errorf("--%s: shorthand/default/type = %q/%q/%q, want %q/%q/%q",
				want.Name, got.Shorthand, got.DefValue, got.Value.Type(), want.Shorthand, want.DefValue, want.Value.Type())
		}
	})
}

func TestKeysShowBechPrefixes(t *testing.T) {
	kr := testKeyring(t)
	record, err := kr.Key("alice")
	if err != nil {
		t.Fatal(err)
	}
	address, err := record.GetAddress()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ flag, want string }{
		{"acc", address.String()},
		{"val", sdk.ValAddress(address).String()},
		{"cons", sdk.ConsAddress(address).String()},
	} {
		for _, ref := range []string{"alice", address.String()} {
			for _, format := range []string{"pretty", "json", "yaml"} {
				t.Run(tc.flag+"/"+ref+"/"+format, func(t *testing.T) {
					out, err := runKeysCommand(t, kr, "show", ref, "-a", "--bech", tc.flag, "-o", format)
					if err != nil {
						t.Fatal(err)
					}
					got := strings.TrimSpace(out)
					if format != "pretty" {
						if err := yaml.Unmarshal([]byte(out), &got); err != nil {
							t.Fatal(err)
						}
					}
					if got != tc.want {
						t.Fatalf("address = %q, want %q", got, tc.want)
					}
				})
			}
		}
		out, err := runKeysCommand(t, kr, "show", "alice", "--bech", tc.flag, "-o", "json")
		if err != nil {
			t.Fatal(err)
		}
		var details keyDetails
		if err := json.Unmarshal([]byte(out), &details); err != nil {
			t.Fatal(err)
		}
		if details.Address != tc.want {
			t.Fatalf("full output address = %q, want %q", details.Address, tc.want)
		}
	}
}

func TestKeysShowPublicKey(t *testing.T) {
	kr := testKeyring(t)
	record, err := kr.Key("alice")
	if err != nil {
		t.Fatal(err)
	}
	pk, err := record.GetPubKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"pretty", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			out, err := runKeysCommand(t, kr, "show", "alice", "-p", "-o", format)
			if err != nil {
				t.Fatal(err)
			}
			if format != "pretty" {
				var scalar string
				if err := yaml.Unmarshal([]byte(out), &scalar); err != nil {
					t.Fatal(err)
				}
				out = scalar
			}
			var got struct {
				Type string `json:"@type"`
				Key  string `json:"key"`
			}
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatal(err)
			}
			if got.Type != "/cosmos.crypto.secp256k1.PubKey" || got.Key != base64.StdEncoding.EncodeToString(pk.Bytes()) {
				t.Fatalf("public key = %+v", got)
			}
		})
	}
}

func TestKeysShowInvalidOptionsBeforeKeyring(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "requires at least 1 arg"},
		{[]string{"alice", "--bech", "bogus"}, "must be one of acc, val, cons"},
		{[]string{"alice", "-a", "-p"}, "cannot use both"},
		{[]string{"alice", "-d", "-p"}, "--device requires --bech acc"},
		{[]string{"alice", "-d", "--bech", "val"}, "--device requires --bech acc"},
		{[]string{"alice", "-d", "--bech", "cons"}, "--device requires --bech acc"},
		{[]string{"alice", "-a", "--qrcode", "-o", "json"}, "--qrcode requires --output pretty"},
		{[]string{"alice", "-a", "--qrcode", "-o", "yaml"}, "--qrcode requires --output pretty"},
		{[]string{"alice", "bob", "--multisig-threshold", "0"}, "must be between 1 and 2"},
		{[]string{"alice", "bob", "--multisig-threshold", "-1"}, "must be between 1 and 2"},
		{[]string{"alice", "bob", "--multisig-threshold", "3"}, "must be between 1 and 2"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cmd := showCmd(func() (sdkkeyring.Keyring, error) {
				t.Fatal("invalid options opened the keyring")
				return nil, nil
			}, nil)
			cmd.Flags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "Output format")
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestKeysShowMultisigPreview(t *testing.T) {
	kr := testKeyring(t)
	bob, _, err := kr.NewMnemonic("bob", sdkkeyring.English, "m/44'/118'/0'/0/1", "", aktkeyring.DefaultAlgo())
	if err != nil {
		t.Fatal(err)
	}
	alice, err := kr.Key("alice")
	if err != nil {
		t.Fatal(err)
	}
	alicePK, _ := alice.GetPubKey()
	bobPK, _ := bob.GetPubKey()
	bobAddr, _ := bob.GetAddress()
	for _, tc := range []struct {
		name      string
		refs      []string
		pubkeys   []cryptotypes.PubKey
		threshold int
	}{
		{"default threshold", []string{"alice", "bob"}, []cryptotypes.PubKey{alicePK, bobPK}, 1},
		{"all signers", []string{"alice", bobAddr.String()}, []cryptotypes.PubKey{alicePK, bobPK}, 2},
		{"positional order", []string{"bob", "alice"}, []cryptotypes.PubKey{bobPK, alicePK}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"show"}, tc.refs...)
			args = append(args, "-o", "json")
			if tc.threshold == 2 {
				args = append(args, "--multisig-threshold", "2")
			}
			out, recorded, err := runKeysCommandRecorded(t, kr, args...)
			if err != nil {
				t.Fatal(err)
			}
			var details keyDetails
			if err := json.Unmarshal([]byte(out), &details); err != nil {
				t.Fatal(err)
			}
			wantPK := multisig.NewLegacyAminoPubKey(tc.threshold, tc.pubkeys)
			if details.Name != "multi" || details.Type != "multi" || details.Address != sdk.AccAddress(wantPK.Address()).String() {
				t.Fatalf("preview = %+v", details)
			}
			if len(recorded) != 0 {
				t.Fatalf("read-only preview recorded actions: %+v", recorded)
			}
		})
	}

	// Duplicate references are retained and diagnosed on stderr, leaving JSON usable.
	out, stderr, err := runKeysCommandWithInput(t, kr, "", "show", "alice", "alice", "-p", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "WARNING: duplicate keys found: alice") {
		t.Fatalf("duplicate warning = %q", stderr)
	}
	var pubkeyJSON string
	if err := json.Unmarshal([]byte(out), &pubkeyJSON); err != nil {
		t.Fatal(err)
	}
	var preview struct {
		Type       string            `json:"@type"`
		Threshold  uint32            `json:"threshold"`
		PublicKeys []json.RawMessage `json:"public_keys"`
	}
	if err := json.Unmarshal([]byte(pubkeyJSON), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Type != "/cosmos.crypto.multisig.LegacyAminoPubKey" || preview.Threshold != 1 || len(preview.PublicKeys) != 2 || !bytes.Equal(preview.PublicKeys[0], preview.PublicKeys[1]) {
		t.Fatalf("duplicate preview = %s", pubkeyJSON)
	}
	records, err := kr.List()
	if err != nil || len(records) != 2 {
		t.Fatalf("keyring changed after previews: %d records, error %v", len(records), err)
	}
}

type showRecordKeyring struct {
	sdkkeyring.Keyring
	record *sdkkeyring.Record
}

func (kr showRecordKeyring) Key(name string) (*sdkkeyring.Record, error) {
	if name == kr.record.Name {
		return kr.record, nil
	}
	return kr.Keyring.Key(name)
}

func TestKeysShowLedger(t *testing.T) {
	kr := testKeyring(t)
	alice, _ := kr.Key("alice")
	pk, _ := alice.GetPubKey()
	path := hd.CreateHDPath(118, 7, 3)
	deviceErr := errors.New("device rejected address")
	for _, tc := range []struct {
		name      string
		args      []string
		path      *hd.BIP44Params
		deviceErr error
		wantCalls int
		wantErr   string
	}{
		{"cached key", []string{"ledger", "-a"}, path, nil, 0, ""},
		{"verify address", []string{"ledger", "-a", "-d"}, path, nil, 1, ""},
		{"verify full details", []string{"ledger", "--device"}, path, nil, 1, ""},
		{"device failure", []string{"ledger", "-a", "-d"}, path, deviceErr, 1, "show address on Ledger"},
		{"missing path", []string{"ledger", "-d"}, nil, nil, 0, "has no BIP44 path"},
		{"local key", []string{"alice", "-d"}, path, nil, 0, "requires a Ledger key"},
		{"multisig", []string{"alice", "ledger", "-d"}, path, nil, 0, "requires a Ledger key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record, err := sdkkeyring.NewLedgerRecord("ledger", pk, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			var stdout bytes.Buffer
			cmd := showCmd(func() (sdkkeyring.Keyring, error) {
				return showRecordKeyring{Keyring: kr, record: record}, nil
			}, func(gotPath hd.BIP44Params, gotPK cryptotypes.PubKey, prefix string) error {
				calls++
				if stdout.Len() != 0 {
					t.Fatal("printed key before device verification")
				}
				if gotPath.String() != path.String() || !gotPK.Equals(pk) || prefix != "akash" {
					t.Fatalf("device inputs: path=%s, key=%v, prefix=%s", gotPath.String(), gotPK, prefix)
				}
				return tc.deviceErr
			})
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(&stdout)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tc.args)
			err = cmd.Execute()
			if calls != tc.wantCalls {
				t.Fatalf("device calls = %d, want %d", calls, tc.wantCalls)
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || stdout.Len() != 0 {
					t.Fatalf("error = %v, stdout = %q", err, stdout.String())
				}
				if tc.deviceErr != nil && !errors.Is(err, tc.deviceErr) {
					t.Fatalf("device error not preserved: %v", err)
				}
			} else if err != nil || !strings.Contains(stdout.String(), sdk.AccAddress(pk.Address()).String()) {
				t.Fatalf("error = %v, stdout = %q", err, stdout.String())
			}
		})
	}
}

func TestKeysShowQRCode(t *testing.T) {
	kr := testKeyring(t)
	plain, err := runKeysCommand(t, kr, "show", "alice", "-a", "--bech", "val")
	if err != nil {
		t.Fatal(err)
	}
	qr, err := runKeysCommand(t, kr, "show", "alice", "-a", "--bech", "val", "--qrcode")
	if err != nil || !strings.HasSuffix(qr, plain) || !strings.ContainsAny(qr, "▀▄█") {
		t.Fatalf("QR output = %q, error = %v", qr, err)
	}
	for _, args := range [][]string{
		{"show", "alice"},
		{"show", "alice", "-p"},
		{"show", "alice", "-o", "json"},
		{"show", "alice", "-p", "-o", "yaml"},
	} {
		without, err := runKeysCommand(t, kr, args...)
		if err != nil {
			t.Fatal(err)
		}
		with, err := runKeysCommand(t, kr, append(args, "--qrcode")...)
		if err != nil || with != without {
			t.Fatalf("--qrcode changed output without -a: %q, error %v", with, err)
		}
	}
}

func TestKeysShowWriterFailures(t *testing.T) {
	kr := testKeyring(t)
	writeErr := errors.New("output unavailable")
	for _, args := range [][]string{
		{"show", "alice"},
		{"show", "alice", "-o", "json"},
		{"show", "alice", "-a"},
		{"show", "alice", "-a", "-o", "yaml"},
		{"show", "alice", "-p"},
		{"show", "alice", "-p", "-o", "json"},
		{"show", "alice", "-a", "--qrcode"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			writer := &keysSequencedWriter{err: writeErr, failAt: 1}
			if err := executeKeysBoundaryCommand(t, kr, "", writer, io.Discard, args...); !errors.Is(err, writeErr) {
				t.Fatalf("write error = %v", err)
			}
		})
	}
	// Exercise a failure after QR rendering has already started.
	writer := &keysSequencedWriter{err: writeErr, failAt: 2}
	if err := executeKeysBoundaryCommand(t, kr, "", writer, io.Discard, "show", "alice", "-a", "--qrcode"); !errors.Is(err, writeErr) {
		t.Fatalf("partial QR error = %v", err)
	}
	var stdout bytes.Buffer
	if err := executeKeysBoundaryCommand(t, kr, "", &stdout, keysFaultWriter{err: writeErr}, "show", "alice", "alice"); !errors.Is(err, writeErr) || stdout.Len() != 0 {
		t.Fatalf("duplicate warning error = %v, stdout = %q", err, stdout.String())
	}
}

func TestKeysShowReadErrors(t *testing.T) {
	keyringErr := errors.New("keyring unavailable")
	cmd := showCmd(func() (sdkkeyring.Keyring, error) { return nil, keyringErr }, nil)
	cmd.SetArgs([]string{"alice"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); !errors.Is(err, keyringErr) {
		t.Fatalf("keyring error = %v", err)
	}

	kr := testKeyring(t)
	alice, _ := kr.Key("alice")
	pk, _ := alice.GetPubKey()
	unregistered, err := sdkkeyring.NewOfflineRecord("unregistered", pk)
	if err != nil {
		t.Fatal(err)
	}
	unregistered.PubKey.TypeUrl = "/unknown.PublicKey"
	for _, tc := range []struct {
		record *sdkkeyring.Record
		args   []string
		want   string
	}{
		{alice, []string{"show", "alice", "missing"}, "key \"missing\" not found"},
		{&sdkkeyring.Record{Name: "corrupt", PubKey: &codectypes.Any{}}, []string{"show", "corrupt"}, "get pubkey:"},
		{&sdkkeyring.Record{Name: "corrupt", PubKey: &codectypes.Any{}}, []string{"show", "alice", "corrupt"}, "get pubkey for \"corrupt\":"},
		{unregistered, []string{"show", "unregistered", "-p"}, "encode pubkey:"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := runKeysCommand(t, showRecordKeyring{Keyring: kr, record: tc.record}, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) || out != "" {
				t.Fatalf("error = %v, output = %q", err, out)
			}
		})
	}
}
