package client

import (
	"io"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

// promptCountingReader counts reads of the keyring manager's input. The file
// backend's passphrase prompt is the only thing that reads it, so a non-zero
// count is a prompt the user would have had to answer.
type promptCountingReader struct {
	io.Reader
	reads int
}

func (r *promptCountingReader) Read(p []byte) (int, error) {
	r.reads++
	return r.Reader.Read(p)
}

// lockedContext seeds a home whose active context has a file-backed keyring
// and a *named* default account -- the shape that made offline commands
// prompt, because resolving the name means unlocking the keyring.
func lockedContext(t *testing.T) (*aktctx.Manager, []aktctx.Keyring, string) {
	t.Helper()

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

	return mgr, keyringConfig, root
}

// TestOfflineCommandsDoNotOpenTheKeyring pins the promise that `akt sdl` and
// `akt monitor` run entirely locally (SPEC §2.11, §2.6). Both used to unlock
// the context's keyring at startup just to turn a named default-account into
// an address neither of them uses, so validating a local YAML file asked for a
// wallet passphrase.
func TestOfflineCommandsDoNotOpenTheKeyring(t *testing.T) {
	commands := map[string][]string{
		"sdl validate":     {"sdl", "validate"},
		"monitor":          {"monitor"},
		"monitor provider": {"monitor", "provider"},
		"version":          {"version"},
	}

	for name, path := range commands {
		t.Run(name, func(t *testing.T) {
			mgr, keyringConfig, root := lockedContext(t)
			enc := aktcodec.MakeEncodingConfig()

			input := &promptCountingReader{Reader: strings.NewReader("testpass123\n")}
			keyrings := aktkeyring.NewManager(root, keyringConfig, enc.Codec)
			keyrings.SetInput(input)

			leaf := commandTree(path...)

			resolved, err := MustResolveAndInit(leaf, mgr, keyrings, enc, "", "", LocalIdentityNone)
			require.NoError(t, err)
			require.True(t, resolved)
			require.Zero(t, input.reads, "an offline command must not read the keyring passphrase prompt")
		})
	}
}

// TestSignerCommandsStillResolveTheDefaultAccount is the other half of the
// contract: suppressing keyring access for offline commands must not leave
// signing commands without an identity.
func TestSignerCommandsStillResolveTheDefaultAccount(t *testing.T) {
	mgr, keyringConfig, root := lockedContext(t)
	enc := aktcodec.MakeEncodingConfig()

	keyrings := aktkeyring.NewManager(root, keyringConfig, enc.Codec)
	keyrings.SetInput(strings.NewReader("testpass123\ntestpass123\ntestpass123\n"))

	leaf := commandTree("tx", "bank", "send")

	resolved, err := MustResolveAndInit(leaf, mgr, keyrings, enc, "", "", LocalIdentityRequired)
	require.NoError(t, err)
	require.True(t, resolved)

	cctx := sdkclient.GetClientContextFromCmd(leaf)
	require.Equal(t, "alice", cctx.FromName)
	require.False(t, cctx.GetFromAddress().Empty())
}

func TestOnDemandIdentityOpensOnlyWhenAKeyIsUsed(t *testing.T) {
	mgr, keyringConfig, root := lockedContext(t)
	enc := aktcodec.MakeEncodingConfig()

	input := &promptCountingReader{Reader: strings.NewReader("testpass123\n")}
	keyrings := aktkeyring.NewManager(root, keyringConfig, enc.Codec)
	keyrings.SetInput(input)

	leaf := commandTree("query", "provider", "list")
	resolved, err := MustResolveAndInit(
		leaf,
		mgr,
		keyrings,
		enc,
		"",
		"",
		LocalIdentityOnDemand,
	)
	require.NoError(t, err)
	require.True(t, resolved)
	require.Zero(t, input.reads, "startup must not open the deferred keyring")

	cctx := sdkclient.GetClientContextFromCmd(leaf)
	require.NotNil(t, cctx.Keyring)
	_, err = cctx.Keyring.Key("alice")
	require.NoError(t, err)
	require.Positive(t, input.reads, "the first key operation must open the keyring")
}

func TestConsolePreferredContextHonorsLocalIdentityMode(t *testing.T) {
	mgr, keyringConfig, root := lockedContext(t)
	require.NoError(t, mgr.UpdateContext("locked", func(ctx *aktctx.Context) error {
		ctx.AuthMethod = aktctx.AuthMethodConsoleAPI
		return nil
	}))

	enc := aktcodec.MakeEncodingConfig()
	input := &promptCountingReader{Reader: strings.NewReader("testpass123\n")}
	keyrings := aktkeyring.NewManager(root, keyringConfig, enc.Codec)
	keyrings.SetInput(input)

	leaf := commandTree("query", "deployment", "list")
	resolved, err := MustResolveAndInit(
		leaf,
		mgr,
		keyrings,
		enc,
		"",
		"",
		LocalIdentityOnDemand,
	)
	require.NoError(t, err)
	require.True(t, resolved)
	require.Zero(t, input.reads)

	cctx := sdkclient.GetClientContextFromCmd(leaf)
	require.NotNil(t, cctx.Keyring)
	_, err = cctx.Keyring.Key("alice")
	require.NoError(t, err)
	require.Positive(t, input.reads)
}

func commandTree(path ...string) *cobra.Command {
	root := &cobra.Command{Use: "akt"}

	parent := root
	var leaf *cobra.Command
	for _, name := range path {
		leaf = &cobra.Command{Use: name}
		parent.AddCommand(leaf)
		parent = leaf
	}

	return leaf
}
