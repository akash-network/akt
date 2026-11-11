package keys

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func TestKeyRecoveryPipedInput(t *testing.T) {
	const mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	tests := []struct {
		name        string
		input       string
		interactive bool
		passphrase  string
		wantError   string
	}{
		{
			name:  "newline",
			input: mnemonic + "\n",
		},
		{
			name:  "EOF after mnemonic",
			input: mnemonic,
		},
		{
			name:  "surrounding whitespace and CRLF",
			input: " \t" + mnemonic + " \r\n",
		},
		{
			name:        "empty BIP39 passphrase",
			input:       mnemonic + "\n\n",
			interactive: true,
		},
		{
			name:        "confirmed BIP39 passphrase",
			input:       mnemonic + "\nsecret\nsecret\n",
			interactive: true,
			passphrase:  "secret",
		},
		{
			name:        "confirmation at EOF",
			input:       mnemonic + "\nsecret\nsecret",
			interactive: true,
			passphrase:  "secret",
		},
		{
			name:      "missing input",
			wantError: "read mnemonic",
		},
		{
			name:      "blank mnemonic",
			input:     " \t\n",
			wantError: "invalid mnemonic",
		},
		{
			name:      "unknown words",
			input:     "not a mnemonic\n",
			wantError: "invalid mnemonic",
		},
		{
			name:      "bad checksum",
			input:     strings.Repeat("abandon ", 11) + "abandon\n",
			wantError: "create key:",
		},
		{
			name:        "missing passphrase",
			input:       mnemonic + "\n",
			interactive: true,
			wantError:   "read BIP39 passphrase",
		},
		{
			name:        "missing confirmation",
			input:       mnemonic + "\nsecret\n",
			interactive: true,
			wantError:   "read BIP39 passphrase confirmation",
		},
		{
			name:        "mismatched confirmation",
			input:       mnemonic + "\nsecret\ndifferent\n",
			interactive: true,
			wantError:   "BIP39 passphrases do not match",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
			args := []string{"add", "recovered", "--recover"}
			if test.interactive {
				args = append(args, "--interactive")
			}
			stdout, stderr, err := runKeysCommandWithInput(t, kr, test.input, args...)
			require.Empty(t, stderr, "piped input must not display prompts")
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Empty(t, stdout)
				records, listErr := kr.List()
				require.NoError(t, listErr)
				require.Empty(t, records, "failed recovery must not create a key")
				return
			}
			require.NoError(t, err)
			record, err := kr.Key("recovered")
			require.NoError(t, err)
			address, err := record.GetAddress()
			require.NoError(t, err)
			derived, err := aktkeyring.DefaultAlgo().Derive()(mnemonic, test.passphrase, "m/44'/118'/0'/0/0")
			require.NoError(t, err)
			require.Equal(t, aktkeyring.DefaultAlgo().Generate()(derived).PubKey().Address().Bytes(), address.Bytes())
			require.NotContains(t, stdout, mnemonic)
		})
	}
}

func TestKeyRecoveryPropagatesPartialReadErrors(t *testing.T) {
	const mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	readErr := errors.New("input unavailable")
	for _, prefix := range []string{"", mnemonic, mnemonic + "\nsecret", mnemonic + "\nsecret\nsecret"} {
		t.Run(prefix, func(t *testing.T) {
			kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
			cmd := addCmd(func() (sdkkeyring.Keyring, error) { return kr, nil }, nil)
			cmd.SetIn(io.MultiReader(strings.NewReader(prefix), iotest.ErrReader(readErr)))
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs([]string{"recovered", "--recover", "--interactive"})
			require.ErrorIs(t, cmd.Execute(), readErr)
			require.Empty(t, stdout.String())
			require.Empty(t, stderr.String())
			records, err := kr.List()
			require.NoError(t, err)
			require.Empty(t, records)
		})
	}
}
