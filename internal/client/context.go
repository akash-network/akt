// Package client builds the Cosmos SDK client.Context from the akt context system.
//
// It bridges our context (network, keyring, defaults) to the SDK's client.Context
// that all tx/query commands expect. This allows chain-sdk CLI commands to work
// unmodified with our context-based configuration.
package client

import (
	"context"
	"os"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/cobra"

	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"

	arpcclient "pkg.akt.dev/go/node/client"
	"pkg.akt.dev/go/sdkutil"
)

// BuildClientContext constructs a fully populated Cosmos SDK client.Context
// from the akt context. This bridges our context system to the SDK.
func BuildClientContext(
	rc *aktctx.Context,
	kr sdkkeyring.Keyring,
	enc sdkutil.EncodingConfig,
	fromOverride string,
) sdkclient.Context {
	return buildClientContext(rc, kr, enc, fromOverride, true)
}

func buildClientContext(
	rc *aktctx.Context,
	kr sdkkeyring.Keyring,
	enc sdkutil.EncodingConfig,
	fromOverride string,
	resolveNamedAccount bool,
) sdkclient.Context {
	cctx := sdkclient.Context{}.
		WithCodec(enc.Codec).
		WithInterfaceRegistry(enc.InterfaceRegistry).
		WithTxConfig(enc.TxConfig).
		WithLegacyAmino(enc.Amino).
		WithInput(os.Stdin).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithHomeDir(rc.Root).
		WithKeyring(kr).
		WithChainID(rc.Network.ChainID).
		WithKeyringDir(aktctx.KeyringDir(rc.Root, rc.Keyring))

	// Carry the invocation account before downstream tx/query hooks run.
	//
	// WithFrom records the name only; FromName and FromAddress stay empty
	// until something resolves the name against the keyring. Commands that
	// need a signer resolve it here. Query initialization deliberately leaves
	// a named account unresolved so network-wide and explicitly scoped reads
	// never unlock the keyring; owner-defaulting query handlers resolve it only
	// when they actually need its address.
	//
	// A name that is not resolved is left as the bare From value rather
	// than failing: the context is built for every command, including the
	// keys and context commands someone would use to fix it.
	from := fromOverride
	if from == "" {
		from = rc.DefaultAccount
	}
	if from != "" {
		cctx = cctx.WithFrom(from)

		if addr, err := sdk.AccAddressFromBech32(from); err == nil {
			cctx = cctx.WithFromAddress(addr)
		} else if resolveNamedAccount && kr != nil {
			if rec, err := kr.Key(from); err == nil {
				if addr, err := rec.GetAddress(); err == nil {
					cctx = cctx.WithFromName(from).WithFromAddress(addr)
				}
			}
		}
	}

	// Set broadcast mode.
	cctx = cctx.WithBroadcastMode("sync")

	// Set node URI from the first RPC endpoint.
	// NormalizeEndpoint ensures a port is present (inferred from scheme when
	// omitted) so that downstream CometBFT/cosmos-sdk clients can connect.
	if len(rc.Network.Endpoints.RPC) > 0 {
		cctx = cctx.WithNodeURI(arpcclient.NormalizeEndpoint(rc.Network.Endpoints.RPC[0]))
	}

	// Set output format.
	cctx = cctx.WithOutputFormat("json")

	return cctx
}

// InitClientContext initializes the Cosmos SDK client context on a cobra command.
// This must be called in the root PersistentPreRunE before any chain-sdk CLI
// commands execute. It stores the client.Context in the cobra context the same
// way the SDK's SetCmdClientContextHandler does.
func InitClientContext(
	cmd *cobra.Command,
	rc *aktctx.Context,
	kr sdkkeyring.Keyring,
	enc sdkutil.EncodingConfig,
	fromOverride string,
) error {
	return initClientContext(cmd, rc, kr, enc, fromOverride, true)
}

func initClientContext(
	cmd *cobra.Command,
	rc *aktctx.Context,
	kr sdkkeyring.Keyring,
	enc sdkutil.EncodingConfig,
	fromOverride string,
	resolveNamedAccount bool,
) error {
	cctx := buildClientContext(rc, kr, enc, fromOverride, resolveNamedAccount)

	// Store address codecs in the context for chain-sdk commands.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Set the RPC URI from the resolved network so that downstream
	// PersistentPreRunE hooks (QueryPersistentPreRunE, TxPersistentPreRunE)
	// can discover the endpoint via GetRPCURIFromContext.
	if len(rc.Network.Endpoints.RPC) > 0 {
		ctx = context.WithValue(ctx, chaincli.ContextTypeRPCURI, arpcclient.NormalizeEndpoint(rc.Network.Endpoints.RPC[0]))
	}

	// The SDK expects a *client.Context pointer so it can be mutated by
	// downstream PersistentPreRunE hooks (tx/query).
	ctx = context.WithValue(ctx, sdkclient.ClientContextKey, &cctx)
	cmd.SetContext(ctx)

	return nil
}

// MustResolveAndInit is a convenience that resolves the context, gets the
// keyring, and initializes the SDK client context on the command. Used in
// the root PersistentPreRunE. Returns true if a context was resolved.
func MustResolveAndInit(
	cmd *cobra.Command,
	mgr *aktctx.Manager,
	krMgr *aktkeyring.Manager,
	enc sdkutil.EncodingConfig,
	contextOverride string,
	fromOverride string,
) (bool, error) {
	ctxName := contextOverride
	if ctxName == "" {
		ctxName = mgr.CurrentContext()
	}

	// If no context is explicitly set, auto-select when exactly one exists.
	if ctxName == "" {
		contexts := mgr.ListContexts()
		if len(contexts) == 1 {
			ctxName = contexts[0].Name
		} else {
			return false, nil
		}
	}

	rc, err := mgr.Resolve(ctxName)
	if err != nil {
		return false, err
	}

	krName := rc.Keyring.Name
	if krName == "" {
		krName = "default"
	}

	kr, err := krMgr.Get(krName)
	if err != nil {
		return false, err
	}

	return true, initClientContext(cmd, rc, kr, enc, fromOverride, !isQueryCommand(cmd))
}

func isQueryCommand(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Name() == "query" {
			return true
		}
	}

	return false
}
