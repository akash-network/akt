// Package client builds the Cosmos SDK client.Context from the akt context system.
//
// It bridges our context (network, keyring, defaults) to the SDK's client.Context
// that all tx/query commands expect. This allows chain-sdk CLI commands to work
// unmodified with our context-based configuration.
package client

import (
	"context"
	"fmt"
	"os"
	"strings"

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

// LocalIdentityMode controls when command startup may open the selected
// context's keyring (SPEC §1.7).
type LocalIdentityMode uint8

const (
	LocalIdentityNone LocalIdentityMode = iota
	LocalIdentityOnDemand
	LocalIdentityRequired
)

// BuildClientContext constructs a fully populated Cosmos SDK client.Context
// from the akt context. This bridges our context system to the SDK.
func BuildClientContext(
	rc *aktctx.Context,
	kr sdkkeyring.Keyring,
	enc sdkutil.EncodingConfig,
	fromOverride string,
) sdkclient.Context {
	cctx, _ := buildClientContext(rc, kr, enc, fromOverride, true)
	return cctx
}

func buildClientContext(
	rc *aktctx.Context,
	kr sdkkeyring.Keyring,
	enc sdkutil.EncodingConfig,
	fromOverride string,
	resolveNamedAccount bool,
) (sdkclient.Context, error) {
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
	// A name that is not resolved is left as the bare From value. Best-effort
	// callers can still build a context for the keys and context commands
	// someone would use to fix it, while eager initialization returns the
	// resolution error to commands that require an identity.
	//
	// kr is nil for commands that declare no local identity (SPEC §1.7). A
	// named account then stays unresolved -- resolving it would open the
	// keyring and prompt -- while an address-valued default still resolves,
	// because parsing bech32 costs nothing.
	from := fromOverride
	if from == "" {
		from = rc.DefaultAccount
	}
	var resolveErr error
	if from != "" {
		cctx = cctx.WithFrom(from)

		if addr, err := sdk.AccAddressFromBech32(from); err == nil {
			cctx = cctx.WithFromAddress(addr)
		} else if resolveNamedAccount {
			addr, err := resolveKeyringAddress(kr, from)
			if err != nil {
				resolveErr = err
			} else {
				cctx = cctx.WithFromName(from).WithFromAddress(addr)
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

	return cctx, resolveErr
}

// ResolveAccountAddress resolves the account carried by cctx. Address-valued
// accounts are parsed without consulting the keyring; named accounts trigger
// the first key operation on an on-demand keyring.
func ResolveAccountAddress(cctx sdkclient.Context) (sdk.AccAddress, error) {
	if addr := cctx.GetFromAddress(); !addr.Empty() {
		return addr, nil
	}

	from := strings.TrimSpace(cctx.From)
	if from == "" {
		return nil, nil
	}

	if addr, err := sdk.AccAddressFromBech32(from); err == nil {
		return addr, nil
	}

	return resolveKeyringAddress(cctx.Keyring, from)
}

func resolveKeyringAddress(kr sdkkeyring.Keyring, from string) (sdk.AccAddress, error) {
	if kr == nil {
		return nil, fmt.Errorf("resolve account %q: keyring is unavailable", from)
	}

	record, err := kr.Key(from)
	if err != nil {
		return nil, fmt.Errorf("resolve account %q: %w", from, err)
	}
	if record == nil {
		return nil, fmt.Errorf("resolve account %q: keyring returned no record", from)
	}
	// Cosmos SDK Record.GetAddress dereferences PubKey before it can return a
	// decode error. Treat persisted keyring data as boundary input so a corrupt
	// record becomes a useful error instead of a process panic.
	if record.PubKey == nil {
		return nil, fmt.Errorf("resolve account %q address: keyring record has no public key", from)
	}

	addr, err := record.GetAddress()
	if err != nil {
		return nil, fmt.Errorf("resolve account %q address: %w", from, err)
	}

	return addr, nil
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
	cctx, err := buildClientContext(rc, kr, enc, fromOverride, resolveNamedAccount)
	if err != nil {
		return err
	}

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
//
// identityMode is the caller's explicit decision about when this invocation
// may open the local signing identity (SPEC §1.7). The client layer does not
// infer that policy from command names.
func MustResolveAndInit(
	cmd *cobra.Command,
	mgr *aktctx.Manager,
	krMgr *aktkeyring.Manager,
	enc sdkutil.EncodingConfig,
	contextOverride string,
	fromOverride string,
	identityMode LocalIdentityMode,
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

	// A nil keyring is a deliberate, complete answer for commands that declare
	// no local identity. On-demand commands receive a proxy that opens the
	// backend only when a key operation is actually requested.
	var kr sdkkeyring.Keyring
	resolveNamedAccount := false
	if identityMode != LocalIdentityNone {
		krName := rc.Keyring.Name
		if krName == "" {
			krName = "default"
		}

		switch identityMode {
		case LocalIdentityOnDemand:
			kr = krMgr.Deferred(krName)
		case LocalIdentityRequired:
			kr, err = krMgr.Get(krName)
			if err != nil {
				return false, err
			}
			resolveNamedAccount = true
		}
	}

	return true, initClientContext(cmd, rc, kr, enc, fromOverride, resolveNamedAccount)
}
