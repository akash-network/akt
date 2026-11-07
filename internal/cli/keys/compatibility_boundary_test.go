package keys

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	flagdefs "pkg.akt.dev/akt/internal/flags"
)

type compatibilityFaultKeyring struct {
	sdkkeyring.Keyring
	record *sdkkeyring.Record
	err    error
}

func (kr compatibilityFaultKeyring) NewAccount(string, string, string, string, sdkkeyring.SignatureAlgo) (*sdkkeyring.Record, error) {
	return kr.record, kr.err
}

func (kr compatibilityFaultKeyring) SaveOfflineKey(string, cryptotypes.PubKey) (*sdkkeyring.Record, error) {
	return kr.record, kr.err
}

func (kr compatibilityFaultKeyring) SaveMultisig(string, cryptotypes.PubKey) (*sdkkeyring.Record, error) {
	return kr.record, kr.err
}

func (kr compatibilityFaultKeyring) ImportPrivKeyHex(string, string, string) error { return kr.err }

func (kr compatibilityFaultKeyring) MigrateAll() ([]*sdkkeyring.Record, error) { return nil, kr.err }

func (kr compatibilityFaultKeyring) ExportPrivateKeyObject(string) (cryptotypes.PrivKey, error) {
	return nil, kr.err
}

func TestCompatibilityCommandsPropagateKeyringOpenFailures(t *testing.T) {
	wantErr := errors.New("keyring unavailable")
	for _, args := range [][]string{
		{"add", "new"}, {"add", "multi", "--dry-run", "--multisig", "alice"},
		{"import-hex", "new"}, {"list-key-types"}, {"migrate"},
	} {
		cmd := Commands(func() (sdkkeyring.Keyring, error) { return nil, wantErr }, nil)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(args)
		require.ErrorIs(t, cmd.Execute(), wantErr)
	}
}

func TestKeyCreationPropagatesStorageAndRecordFailures(t *testing.T) {
	base := testKeyring(t)
	pk := secp256k1.GenPrivKey().PubKey()
	badRecord, err := sdkkeyring.NewOfflineRecord("new", pk)
	require.NoError(t, err)
	badRecord.PubKey = &codectypes.Any{}
	wantErr := errors.New("storage failed")
	for _, args := range [][]string{
		{"add", "new", "--source", mnemonicSource(t)},
		{"add", "new", "--pubkey", publicKeyJSON(t, pk)},
		{"add", "new", "--multisig", "alice"},
	} {
		for _, kr := range []compatibilityFaultKeyring{
			{Keyring: base, err: wantErr},
			{Keyring: base, record: badRecord},
		} {
			out, actions, err := runKeysCommandRecorded(t, kr, args...)
			require.Error(t, err)
			require.Empty(t, out)
			require.Len(t, actions, 1)
			if kr.err != nil {
				require.ErrorIs(t, err, wantErr)
				require.ErrorIs(t, actions[0].err, wantErr)
			} else {
				require.ErrorContains(t, err, "get address")
			}
		}
	}
	for _, args := range [][]string{
		{"add", "new", "--multisig", "alice,alice"},
		{"add", "new", "--multisig", " "},
		{"add", "new", "--multisig", "missing"},
	} {
		_, err := runKeysCommand(t, base, args...)
		require.Error(t, err)
	}
	_, err = runKeysCommand(t, showRecordKeyring{Keyring: base, record: badRecord}, "add", "multi", "--multisig", "new")
	require.ErrorContains(t, err, "get pubkey")
}

func TestKeyInputFailuresDoNotCreateRecords(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		input string
	}{
		{[]string{"add", "new", "--source", filepath.Join(t.TempDir(), "missing")}, ""},
		{[]string{"add", "new", "--interactive"}, "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about\npassphrase\n"},
		{[]string{"import-hex", " "}, ""},
		{[]string{"import-hex", "alice"}, ""},
		{[]string{"import-hex", "new", "--key-type", "unsupported"}, ""},
		{[]string{"import-hex", "new"}, ""},
	} {
		kr := testKeyring(t)
		_, _, err := runKeysCommandWithInput(t, kr, tc.input, tc.args...)
		require.Error(t, err)
		records, err := kr.List()
		require.NoError(t, err)
		require.Len(t, records, 1)
	}
}

func TestKeyImportExportAndMigrationFailuresAreReported(t *testing.T) {
	wantErr := errors.New("backend unavailable")
	kr := compatibilityFaultKeyring{Keyring: testKeyring(t), err: wantErr}
	for _, args := range [][]string{
		{"import-hex", "new", strings.Repeat("01", 32)},
		{"export", "alice", "--unarmored-hex", "--unsafe", "--yes"},
		{"migrate"},
	} {
		_, actions, err := runKeysCommandRecorded(t, kr, args...)
		require.ErrorIs(t, err, wantErr)
		require.Len(t, actions, 1)
		require.ErrorIs(t, actions[0].err, wantErr)
	}
	_, err := runKeysCommand(t, struct{ sdkkeyring.Keyring }{kr.Keyring}, "export", "alice", "--unarmored-hex", "--unsafe", "--yes")
	require.ErrorContains(t, err, "does not support raw")
	err = executeKeysBoundaryCommand(t, kr.Keyring, "", io.Discard, io.Discard,
		"export", "alice", "--unarmored-hex", "--unsafe")
	require.ErrorContains(t, err, "confirmation")
	err = executeKeysBoundaryCommand(t, kr.Keyring, "no\n", io.Discard,
		&keysSequencedWriter{err: wantErr, failAt: 2}, "export", "alice", "--unarmored-hex", "--unsafe")
	require.ErrorIs(t, err, wantErr)
	err = executeKeysBoundaryCommand(t, kr.Keyring, "", keysFaultWriter{err: wantErr}, io.Discard,
		"list-key-types", "--output", "pretty")
	require.ErrorIs(t, err, wantErr)
}

func TestMnemonicBoundariesPropagateGenerationAndEncodingFailures(t *testing.T) {
	wantErr := errors.New("mnemonic generation unavailable")
	failGenerate := func() (string, error) { return "", wantErr }
	for _, unsafe := range []bool{false, true} {
		cmd := mnemonicCmd(failGenerate, func([]byte) (string, error) { return "", wantErr })
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetIn(strings.NewReader(strings.Repeat("entropy", 10) + "\n"))
		if unsafe {
			cmd.SetArgs([]string{"--unsafe-entropy", "--yes"})
		}
		require.ErrorIs(t, cmd.Execute(), wantErr)
	}
	kr := testKeyring(t)
	cmd := addCmd(func() (sdkkeyring.Keyring, error) { return kr, nil }, nil)
	err := addLocalKey(cmd, kr, nil, "new", "", nil, failGenerate)
	require.ErrorIs(t, err, wantErr)
	_, _, err = runKeysCommandWithInput(t, kr, "", "mnemonic", "--unsafe-entropy")
	require.ErrorContains(t, err, "read entropy")
}

func TestPublicKeyDecoderRejectsPointsAndUnknownTypes(t *testing.T) {
	for _, data := range []string{
		fmt.Sprintf(`{"@type":"/cosmos.crypto.secp256k1.PubKey","key":%q}`, base64.StdEncoding.EncodeToString(make([]byte, 33))),
		`{"@type":"/cosmos.crypto.secp256r1.PubKey","key":"AA=="}`,
		`{"@type":"/unknown.PubKey","key":"AA=="}`,
	} {
		_, err := decodePublicKey([]byte(data))
		require.Error(t, err)
	}
}

type keyWriterFunc func([]byte) (int, error)

func (write keyWriterFunc) Write(data []byte) (int, error) { return write(data) }

func TestPrivateKeyTerminalInputAndOutputFailures(t *testing.T) {
	for _, mode := range []string{"success", "prompt", "read", "newline"} {
		t.Run(mode, func(t *testing.T) {
			master, terminal, err := pty.Open()
			require.NoError(t, err)
			t.Cleanup(func() { _ = master.Close(); _ = terminal.Close() })
			cmd := &cobra.Command{}
			cmd.SetIn(terminal)
			var output bytes.Buffer
			cmd.SetErr(&output)
			wantErr := errors.New("terminal output failed")
			switch mode {
			case "prompt":
				cmd.SetErr(keysFaultWriter{err: wantErr})
			case "read":
				cmd.SetErr(keyWriterFunc(func(data []byte) (int, error) {
					_ = terminal.Close()
					return len(data), nil
				}))
			case "newline":
				cmd.SetErr(&keysSequencedWriter{err: wantErr, failAt: 2})
			}
			if mode == "success" || mode == "newline" {
				_, err = io.WriteString(master, "  0123abcd  \n")
				require.NoError(t, err)
			}
			key, err := readPrivateKey(cmd)
			switch mode {
			case "success":
				require.NoError(t, err)
				require.Equal(t, "0123abcd", key)
				require.NotContains(t, output.String(), key)
			case "read":
				require.ErrorContains(t, err, "read private key")
			default:
				require.ErrorIs(t, err, wantErr)
			}
		})
	}
}

func TestKeyAlgorithmAliasSelectsSamePublicKeyType(t *testing.T) {
	kr := testKeyring(t)
	pk := secp256k1.GenPrivKey().PubKey()
	cmd := addCmd(func() (sdkkeyring.Keyring, error) { return kr, nil }, nil)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"new", "--" + flagdefs.FlagKeyAlgorithm, "secp256k1", "--pubkey-base64", base64.StdEncoding.EncodeToString(pk.Bytes())})
	require.NoError(t, cmd.Execute())
}
