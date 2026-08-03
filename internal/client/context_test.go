package client

import (
	"io"
	"strings"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

type countingReader struct {
	io.Reader
	reads int
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.reads++
	return r.Reader.Read(p)
}

func TestProviderListDoesNotReadNamedDefaultAccount(t *testing.T) {
	root := t.TempDir()
	enc := aktcodec.MakeEncodingConfig()

	mgr, err := aktctx.NewManager(root)
	require.NoError(t, err)
	require.NoError(t, mgr.CreateNetwork(aktctx.Network{
		Name:    "mainnet",
		ChainID: "akashnet-2",
		Endpoints: aktctx.Endpoints{
			RPC: []string{"https://rpc.example.test"},
		},
	}))
	require.NoError(t, mgr.CreateKeyring(aktctx.Keyring{
		Name:    "locked",
		Backend: sdkkeyring.BackendFile,
	}))
	require.NoError(t, mgr.CreateContext(aktctx.Context{
		Name:           "locked",
		Network:        aktctx.Network{Name: "mainnet"},
		Keyring:        aktctx.Keyring{Name: "locked"},
		DefaultAccount: "alice",
	}))
	require.NoError(t, mgr.UseContext("locked"))

	keyringConfig := []aktctx.Keyring{{Name: "locked", Backend: sdkkeyring.BackendFile}}
	setupManager := aktkeyring.NewManager(root, keyringConfig, enc.Codec)
	setupManager.SetInput(strings.NewReader("testpass123\ntestpass123\n"))
	setupKeyring, err := setupManager.Get("locked")
	require.NoError(t, err)
	_, _, err = setupKeyring.NewMnemonic(
		"alice",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	require.NoError(t, err)

	promptInput := &countingReader{Reader: strings.NewReader("testpass123\n")}
	keyrings := aktkeyring.NewManager(root, keyringConfig, enc.Codec)
	keyrings.SetInput(promptInput)

	rootCmd := &cobra.Command{Use: "akt"}
	queryCmd := &cobra.Command{Use: "query"}
	providerCmd := &cobra.Command{Use: "provider"}
	listCmd := &cobra.Command{Use: "list"}
	rootCmd.AddCommand(queryCmd)
	queryCmd.AddCommand(providerCmd)
	providerCmd.AddCommand(listCmd)

	resolved, err := MustResolveAndInit(listCmd, mgr, keyrings, enc, "", "")
	require.NoError(t, err)
	require.True(t, resolved)
	require.Zero(t, promptInput.reads, "provider list must not access the keyring")
}
