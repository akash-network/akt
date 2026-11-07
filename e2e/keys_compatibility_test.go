package e2e

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"

	aktcodec "pkg.akt.dev/akt/internal/codec"
)

func TestKeysPublicImportAndMultisig(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)
	enc := aktcodec.MakeEncodingConfig()
	pks := []cryptotypes.PubKey{secp256k1.GenPrivKey().PubKey(), secp256k1.GenPrivKey().PubKey()}
	if bytes.Compare(pks[0].Address(), pks[1].Address()) < 0 {
		pks[0], pks[1] = pks[1], pks[0]
	}
	pubJSON, err := enc.Codec.MarshalInterfaceJSON(pks[0])
	if err != nil {
		t.Fatal(err)
	}
	mustRunAkt(t, home, "context", "keys", "add", "json-signer", "--pubkey", string(pubJSON))
	mustRunAkt(t, home, "context", "keys", "add", "base64-signer", "--pubkey-base64", base64.StdEncoding.EncodeToString(pks[1].Bytes()))
	for _, noSort := range []bool{false, true} {
		name := "sorted"
		wantKeys := []cryptotypes.PubKey{pks[1], pks[0]}
		args := []string{"context", "keys", "add", name, "--multisig", "json-signer,base64-signer", "--multisig-threshold", "2"}
		if noSort {
			name = "ordered"
			args[3] = name
			args = append(args, "--nosort")
			wantKeys = pks
		}
		mustRunAkt(t, home, args...)
		got := strings.TrimSpace(mustRunAkt(t, home, "context", "keys", "show", name, "--address"))
		want, err := enc.TxConfig.SigningContext().AddressCodec().BytesToString(multisig.NewLegacyAminoPubKey(2, wantKeys).Address())
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("multisig address = %s, want %s (nosort=%t)", got, want, noSort)
		}
	}
	before := mustRunAkt(t, home, "context", "keys", "list", "-n", "-o", "json")
	mustRunAkt(t, home, "context", "keys", "add", "preview", "--multisig", "json-signer,base64-signer", "--dry-run")
	mustRunAkt(t, home, "context", "keys", "migrate")
	after := mustRunAkt(t, home, "context", "keys", "list", "--list-names", "-o", "json")
	if before != after {
		t.Fatal("dry-run or migration changed the saved keys")
	}
	var names []string
	if err := json.Unmarshal([]byte(after), &names); err != nil || len(names) != 4 {
		t.Fatal("names-only output did not contain the four saved keys")
	}
}

func TestKeysDryRunWithLockedContext(t *testing.T) {
	home := t.TempDir()
	initHomeWithLockedKeyring(t, home)
	enc := aktcodec.MakeEncodingConfig()
	pubJSON, err := enc.Codec.MarshalInterfaceJSON(secp256k1.GenPrivKey().PubKey())
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"--no-backup"}, {"--pubkey", string(pubJSON)}} {
		args := append([]string{"context", "keys", "add", "preview", "--dry-run", "-o", "json"}, extra...)
		out := mustRunAkt(t, home, args...)
		var result struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil || result.Name != "preview" || result.Address == "" {
			t.Fatal("dry-run did not produce a key result")
		}
	}
}
