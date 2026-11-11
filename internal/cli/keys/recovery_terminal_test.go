//go:build !windows

package keys

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/creack/pty"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func TestKeyRecoveryTerminalPrompts(t *testing.T) {
	const mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	for _, failPrompt := range []string{"", "Enter your bip39 mnemonic", "Enter BIP39 passphrase", "Repeat BIP39 passphrase"} {
		t.Run(failPrompt, func(t *testing.T) {
			master, terminal, err := pty.Open()
			require.NoError(t, err)
			defer master.Close()
			defer terminal.Close()
			_, err = master.WriteString(mnemonic + "\nsecret\nsecret\n")
			require.NoError(t, err)

			kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
			cmd := addCmd(func() (sdkkeyring.Keyring, error) { return kr, nil }, nil)
			cmd.SetIn(terminal)
			var stdout bytes.Buffer
			writeErr := errors.New("prompt output unavailable")
			stderr := &keysMatchingWriter{
				err:   writeErr,
				match: failPrompt,
			}
			if failPrompt == "" {
				stderr.match = "no matching prompt"
			}
			cmd.SetOut(&stdout)
			cmd.SetErr(stderr)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs([]string{"recovered", "--recover", "--interactive"})
			err = cmd.Execute()
			if failPrompt != "" {
				require.ErrorIs(t, err, writeErr)
				records, listErr := kr.List()
				require.NoError(t, listErr)
				require.Empty(t, records)
				return
			}
			require.NoError(t, err)
			prompts := strings.Join(stderr.writes, "")
			require.Contains(t, prompts, "Enter your bip39 mnemonic")
			require.NotContains(t, prompts, "generate")
			require.Contains(t, prompts, "Enter BIP39 passphrase")
			require.Contains(t, prompts, "Repeat BIP39 passphrase")
			require.NotContains(t, stdout.String()+prompts, mnemonic)
			require.NotContains(t, stdout.String()+prompts, "secret")
		})
	}
}
