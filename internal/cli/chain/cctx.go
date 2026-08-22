package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/cliutil"
	clioutput "pkg.akt.dev/akt/internal/output"
	arpcclient "pkg.akt.dev/go/node/client"
)

// SetCmdClientContextHandler is to be used in a command pre-hook execution to
// read flags that populate a Context and sets that to the command's Context.
func SetCmdClientContextHandler(cctx sdkclient.Context, cmd *cobra.Command) (err error) {
	cctx, err = ReadPersistentCommandFlags(cctx, cmd.Flags())
	if err != nil {
		return err
	}

	return SetCmdClientContext(cmd, cctx)
}

// GetClientContextFromCmd returns a Context from a command or an empty Context
// if it has not been set.
func GetClientContextFromCmd(cmd *cobra.Command) sdkclient.Context {
	if v := cmd.Context().Value(ClientContextKey); v != nil {
		cctxPtr := v.(*sdkclient.Context)

		cctx := *cctxPtr
		cctx = cctx.WithCmdContext(cmd.Context())

		return cctx
	}

	return sdkclient.Context{}
}

// SetCmdClientContext sets a command's Context value to the provided argument.
func SetCmdClientContext(cmd *cobra.Command, cctx sdkclient.Context) error {
	v := cmd.Context().Value(ClientContextKey)
	if v == nil {
		return errors.New("client context not set")
	}

	cctxPtr := v.(*sdkclient.Context)
	*cctxPtr = cctx

	return nil
}

// GetClientQueryContext returns a Context from a command with fields set based on flags
// defined in AddQueryFlagsToCmd. An error is returned if any flag query fails.
//
// - client.Context field not pre-populated & flag not set: uses default flag value
// - client.Context field not pre-populated & flag set: uses set flag value
// - client.Context field pre-populated & flag not set: uses pre-populated value
// - client.Context field pre-populated & flag set: uses set flag value
func GetClientQueryContext(cmd *cobra.Command) (sdkclient.Context, error) {
	ctx := GetClientContextFromCmd(cmd)
	return ReadQueryCommandFlags(ctx, cmd.Flags())
}

// GetClientTxContext returns a Context from a command with fields set based on flags
// defined in AddTxFlagsToCmd. An error is returned if any flag query fails.
//
// - client.Context field not pre-populated & flag not set: uses default flag value
// - client.Context field not pre-populated & flag set: uses set flag value
// - client.Context field pre-populated & flag not set: uses pre-populated value
// - client.Context field pre-populated & flag set: uses set flag value
func GetClientTxContext(cmd *cobra.Command) (sdkclient.Context, error) {
	ctx := GetClientContextFromCmd(cmd)
	diagnostics := cmd.ErrOrStderr()
	if cliutil.IsQuiet(cmd) {
		diagnostics = io.Discard
	}
	return ReadTxCommandFlags(ctx, cmd.Flags(), diagnostics)
}

// resolveDefaultAccountAddress resolves the configured query account only at
// the point where an omitted owner filter needs it. Query initialization keeps
// named accounts unresolved so unrelated reads never unlock the keyring.
func resolveDefaultAccountAddress(cctx sdkclient.Context) (string, error) {
	if addr := cctx.GetFromAddress(); !addr.Empty() {
		return addr.String(), nil
	}

	from := strings.TrimSpace(cctx.From)
	if from == "" {
		return resolveTransportDefaultOwner(cctx)
	}

	if addr, err := sdk.AccAddressFromBech32(from); err == nil {
		return addr.String(), nil
	}

	if cctx.Keyring == nil {
		return "", fmt.Errorf("resolve default account %q: keyring is unavailable", from)
	}

	addr, _, _, err := sdkclient.GetFromFields(cctx, cctx.Keyring, from)
	if err != nil {
		return "", fmt.Errorf("resolve default account %q: %w", from, err)
	}

	return addr.String(), nil
}

func resolveTransportDefaultOwner(cctx sdkclient.Context) (string, error) {
	if cctx.CmdContext == nil {
		return "", nil
	}
	resolver, ok := cctx.CmdContext.Value(ContextTypeDefaultOwnerResolver).(DefaultOwnerResolver)
	if !ok || resolver == nil {
		return "", nil
	}

	address, err := resolver(cctx.CmdContext)
	if err != nil {
		return "", err
	}
	parsed, err := sdk.AccAddressFromBech32(strings.TrimSpace(address))
	if err != nil {
		return "", fmt.Errorf("resolve transport default owner: invalid wallet address %q: %w", address, err)
	}

	return parsed.String(), nil
}

// defaultOwnerForQueryArg resolves the default account only for numeric
// shorthand such as 12345[/1]. Explicit addresses, state keywords, and invalid
// input can all be parsed without touching the keyring.
func defaultOwnerForQueryArg(cctx sdkclient.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}

	first := strings.SplitN(args[0], "/", 2)[0]
	if _, err := strconv.ParseUint(first, 10, 64); err == nil {
		return resolveDefaultAccountAddress(cctx)
	}

	return "", nil
}

// ReadQueryCommandFlags returns an updated Context with fields set based on flags
// defined in AddQueryFlagsToCmd. An error is returned if any flag query fails.
//
// Note, the provided cctx may have field pre-populated. The following order
// of precedence occurs:
//
// - client.Context field not pre-populated & flag not set: uses default flag value
// - client.Context field not pre-populated & flag set: uses set flag value
// - client.Context field pre-populated & flag not set: uses pre-populated value
// - client.Context field pre-populated & flag set: uses set flag value
func ReadQueryCommandFlags(cctx sdkclient.Context, flagSet *pflag.FlagSet) (sdkclient.Context, error) {
	if cctx.Height == 0 || flagSet.Changed(flagdefs.FlagHeight) {
		height, _ := flagSet.GetInt64(flagdefs.FlagHeight)
		cctx = cctx.WithHeight(height)
	}

	if !cctx.UseLedger || flagSet.Changed(flagdefs.FlagUseLedger) {
		useLedger, _ := flagSet.GetBool(flagdefs.FlagUseLedger)
		cctx = cctx.WithUseLedger(useLedger)
	}

	if err := validateQueryChainID(cctx, flagSet); err != nil {
		return cctx, err
	}

	return ReadPersistentCommandFlags(cctx, flagSet)
}

// validateQueryChainID applies the query-only chain identity contract without
// constructing a transport. Local derivations use it even though they do not
// need the rest of a client query context.
func validateQueryChainID(cctx sdkclient.Context, flagSet *pflag.FlagSet) error {
	// --chain-id names a chain, but a query's endpoint comes from the active
	// context. The flag cannot retarget the endpoint, so the honest outcomes are
	// to agree with the context or to say so.
	//
	// Queries only. A tx may legitimately name another chain while building an
	// unsigned or offline payload, and that path does not come through here.
	if flagSet.Lookup(flagdefs.FlagChainID) == nil || !flagSet.Changed(flagdefs.FlagChainID) {
		return nil
	}

	chainID, _ := flagSet.GetString(flagdefs.FlagChainID)
	if chainID != "" && cctx.ChainID != "" && chainID != cctx.ChainID {
		return fmt.Errorf(
			"--chain-id %q does not match the active context's chain %q; switch context with --context, or point at another node with --node",
			chainID, cctx.ChainID)
	}

	return nil
}

func GetRPCURIFromContext(ctx context.Context) string {
	val := ctx.Value(ContextTypeRPCURI)
	res, _ := val.(string)

	return res
}

// ReadPersistentCommandFlags returns a Context with fields set for "persistent"
// or common flags that do not necessarily change with context.
//
// Note, the provided cctx may have field pre-populated. The following order
// of precedence occurs:
//
// - client.Context field not pre-populated & flag not set: uses default flag value
// - client.Context field not pre-populated & flag set: uses set flag value
// - client.Context field pre-populated & flag not set: uses pre-populated value
// - client.Context field pre-populated & flag set: uses set flag value
func ReadPersistentCommandFlags(cctx sdkclient.Context, flagSet *pflag.FlagSet) (sdkclient.Context, error) {
	if cctx.OutputFormat == "" || flagSet.Changed(flagdefs.FlagOutput) {
		output, _ := flagSet.GetString(flagdefs.FlagOutput)
		cctx = cctx.WithOutputFormat(output)
	}

	if cctx.HomeDir == "" || flagSet.Changed(flagdefs.FlagHome) {
		homeDir, _ := flagSet.GetString(flagdefs.FlagHome)
		cctx = cctx.WithHomeDir(homeDir)
	}

	if !cctx.Simulate || flagSet.Changed(flagdefs.FlagDryRun) {
		dryRun, _ := flagSet.GetBool(flagdefs.FlagDryRun)
		cctx = cctx.WithSimulation(dryRun)
	}

	if cctx.KeyringDir == "" || flagSet.Changed(flagdefs.FlagKeyringDir) {
		keyringDir, _ := flagSet.GetString(flagdefs.FlagKeyringDir)

		// The keyring directory is optional and falls back to the home directory
		// if omitted.
		if keyringDir == "" {
			keyringDir = cctx.HomeDir
		}

		cctx = cctx.WithKeyringDir(keyringDir)
	}

	if cctx.ChainID == "" || flagSet.Changed(flagdefs.FlagChainID) {
		chainID, _ := flagSet.GetString(flagdefs.FlagChainID)

		cctx = cctx.WithChainID(chainID)
	}

	// --keyring-backend is deliberately NOT turned into a keyring here.
	//
	// It is an akt global flag (SPEC §3.1), applied to the keyring
	// configuration the root command hands to internal/keyring before any
	// leaf runs -- which is also where an "os" backend is checked against the
	// platform's credential stores. Rebuilding a keyring from the raw flag
	// value would bypass that check and, because this runs from the root's
	// own pre-run on every command, would open a key store for commands that
	// declare no local identity at all (SPEC §1.7).

	nodeChanged := flagSet.Lookup(flagdefs.FlagNode) != nil && flagSet.Changed(flagdefs.FlagNode)
	if cctx.Client == nil || nodeChanged {
		var rpcURI string
		if nodeChanged {
			// An explicit invocation override always wins over the endpoint stored
			// in the active context, including when a client was already built.
			rpcURI, _ = flagSet.GetString(flagdefs.FlagNode)
			if strings.TrimSpace(rpcURI) == "" {
				return cctx, fmt.Errorf("--%s cannot be empty", flagdefs.FlagNode)
			}
		} else {
			rpcURI = GetRPCURIFromContext(cctx.CmdContext)

			// Fall back to the command default when the context does not provide
			// an RPC URI.
			if rpcURI == "" && flagSet.Lookup(flagdefs.FlagNode) != nil {
				rpcURI, _ = flagSet.GetString(flagdefs.FlagNode)
			}
		}

		if rpcURI != "" {
			cctx = cctx.WithNodeURI(rpcURI)

			client, err := arpcclient.NewClient(cctx.CmdContext, rpcURI)
			if err != nil {
				return cctx, err
			}

			cctx = cctx.WithClient(client)
		}
	}

	if cctx.GRPCClient == nil || flagSet.Changed(flagdefs.FlagGRPC) {
		if grpcURI, _ := flagSet.GetString(flagdefs.FlagGRPC); grpcURI != "" {
			var dialOpts []grpc.DialOption

			useInsecure, _ := flagSet.GetBool(flagdefs.FlagGRPCInsecure)
			if useInsecure {
				dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
			} else {
				dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
					MinVersion: tls.VersionTLS12,
				})))
			}

			grpcClient, err := grpc.NewClient(grpcURI, dialOpts...)
			if err != nil {
				return sdkclient.Context{}, err
			}
			cctx = cctx.WithGRPCClient(grpcClient)
		}
	}

	return cctx, nil
}

// ReadTxCommandFlags returns an updated Context with fields set based on flags
// defined in AddTxFlagsToCmd. An error is returned if any flag query fails.
//
// Note, the provided cctx may have field pre-populated. The following order
// of precedence occurs:
//
// - client.Context field not pre-populated & flag not set: uses default flag value
// - client.Context field not pre-populated & flag set: uses set flag value
// - client.Context field pre-populated & flag not set: uses pre-populated value
// - client.Context field pre-populated & flag set: uses set flag value
func ReadTxCommandFlags(cctx sdkclient.Context, flagSet *pflag.FlagSet, diagnostics io.Writer) (sdkclient.Context, error) {
	if err := validateTxInvocation(cctx, flagSet); err != nil {
		return cctx, err
	}

	cctx, err := ReadPersistentCommandFlags(cctx, flagSet)
	if err != nil {
		return cctx, err
	}

	if !cctx.GenerateOnly || flagSet.Changed(flagdefs.FlagGenerateOnly) {
		genOnly, _ := flagSet.GetBool(flagdefs.FlagGenerateOnly)
		cctx = cctx.WithGenerateOnly(genOnly)
	}

	if !cctx.Offline || flagSet.Changed(flagdefs.FlagOffline) {
		offline, _ := flagSet.GetBool(flagdefs.FlagOffline)
		cctx = cctx.WithOffline(offline)
	}

	if !cctx.UseLedger || flagSet.Changed(flagdefs.FlagUseLedger) {
		useLedger, _ := flagSet.GetBool(flagdefs.FlagUseLedger)
		cctx = cctx.WithUseLedger(useLedger)
	}

	if cctx.BroadcastMode == "" || flagSet.Changed(flagdefs.FlagBroadcastMode) {
		bMode, _ := flagSet.GetString(flagdefs.FlagBroadcastMode)
		cctx = cctx.WithBroadcastMode(bMode)
	}

	if !cctx.SkipConfirm || flagSet.Changed(flagdefs.FlagSkipConfirmation) {
		skipConfirm, _ := flagSet.GetBool(flagdefs.FlagSkipConfirmation)
		cctx = cctx.WithSkipConfirmation(skipConfirm)
	}

	if cctx.SignModeStr == "" || flagSet.Changed(flagdefs.FlagSignMode) {
		signModeStr, _ := flagSet.GetString(flagdefs.FlagSignMode)
		cctx = cctx.WithSignModeStr(signModeStr)
	}

	if cctx.FeePayer == nil || flagSet.Changed(flagdefs.FlagFeePayer) {
		payer, _ := flagSet.GetString(flagdefs.FlagFeePayer)

		if payer != "" {
			payerAcc, err := sdk.AccAddressFromBech32(payer)
			if err != nil {
				return cctx, err
			}

			cctx = cctx.WithFeePayerAddress(payerAcc)
		}
	}

	if cctx.FeeGranter == nil || flagSet.Changed(flagdefs.FlagFeeGranter) {
		granter, _ := flagSet.GetString(flagdefs.FlagFeeGranter)

		if granter != "" {
			granterAcc, err := sdk.AccAddressFromBech32(granter)
			if err != nil {
				return cctx, err
			}

			cctx = cctx.WithFeeGranterAddress(granterAcc)
		}
	}

	fromUnresolved := cctx.From != "" && cctx.FromName == "" && len(cctx.FromAddress) == 0
	if cctx.From == "" || fromUnresolved || flagSet.Changed(flagdefs.FlagFrom) {
		from := cctx.From
		if flagSet.Changed(flagdefs.FlagFrom) || from == "" {
			from, _ = flagSet.GetString(flagdefs.FlagFrom)
		}
		lookupContext := cctx
		if cctx.Simulate {
			// The Cosmos helper intentionally refuses key names once simulation
			// is set. Resolve the name at this boundary first, then retain the
			// original simulation context for transaction execution.
			lookupContext = cctx.WithSimulation(false)
		}
		fromAddr, fromName, keyType, err := sdkclient.GetFromFields(lookupContext, cctx.Keyring, from)
		if err != nil {
			return cctx, err
		}

		cctx = cctx.WithFrom(from).WithFromAddress(fromAddr).WithFromName(fromName)

		// If the `from` signer account is a ledger key, we need to use
		// SIGN_MODE_AMINO_JSON, because ledger doesn't support proto yet.
		// ref: https://github.com/cosmos/cosmos-sdk/issues/8109
		if keyType == keyring.TypeLedger && cctx.SignModeStr != cflags.SignModeLegacyAminoJSON && !cctx.LedgerHasProtobuf {
			checked := clioutput.NewCheckedWriter(diagnostics)
			_, writeErr := fmt.Fprintln(checked, "Default sign-mode 'direct' not supported by Ledger, using sign-mode 'amino-json'.")
			if err := checked.Complete(writeErr); err != nil {
				return cctx, err
			}
			cctx = cctx.WithSignModeStr(cflags.SignModeLegacyAminoJSON)
		}
	}

	if !cctx.IsAux || flagSet.Changed(flagdefs.FlagAux) {
		isAux, _ := flagSet.GetBool(flagdefs.FlagAux)
		cctx = cctx.WithAux(isAux)
		if isAux {
			// If the user didn't explicitly set an --output flag, use JSON by
			// default.
			if cctx.OutputFormat == "" || !flagSet.Changed(flagdefs.FlagOutput) {
				cctx = cctx.WithOutputFormat("json")
			}

			// If the user didn't explicitly set a --sign-mode flag, use
			// DIRECT_AUX by default.
			if cctx.SignModeStr == "" || !flagSet.Changed(flagdefs.FlagSignMode) {
				cctx = cctx.WithSignModeStr(cflags.SignModeDirectAux)
			}
		}
	}

	return cctx, nil
}

// validateTxInvocation checks transaction identity before common flag parsing
// can overwrite the selected context's chain ID. Online construction,
// simulation, and broadcast stay on the context's chain; an explicitly
// offline invocation may construct a payload for another chain.
func validateTxInvocation(cctx sdkclient.Context, flagSet *pflag.FlagSet) error {
	chainFlag := flagSet.Lookup(flagdefs.FlagChainID)
	if chainFlag == nil || !chainFlag.Changed {
		return nil
	}

	chainID, _ := flagSet.GetString(flagdefs.FlagChainID)
	if strings.TrimSpace(chainID) == "" {
		return fmt.Errorf("--%s cannot be empty", flagdefs.FlagChainID)
	}

	offline := false
	if offlineFlag := flagSet.Lookup(flagdefs.FlagOffline); offlineFlag != nil && offlineFlag.Changed {
		offline, _ = flagSet.GetBool(flagdefs.FlagOffline)
	}
	if !offline && cctx.ChainID != "" && chainID != cctx.ChainID {
		return fmt.Errorf(
			"--chain-id %q does not match the active context's chain %q; switch context with --context, or use --offline to construct for another chain",
			chainID, cctx.ChainID)
	}

	return nil
}

// ValidateTxInvocation checks the assembled command against the selected SDK
// client context. The root calls it before leaf hooks so dependency-owned tx
// commands and workflow dry-runs receive the same identity guard.
func ValidateTxInvocation(cmd *cobra.Command) error {
	return validateTxInvocation(GetClientContextFromCmd(cmd), cmd.Flags())
}
