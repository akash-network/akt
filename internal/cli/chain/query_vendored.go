package cli

import (
	"context"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

// adoptVendoredQueryCmds prepares a query subtree that is imported wholesale
// from a dependency, rather than declared here.
//
// Such a subtree resolves its endpoint through the SDK's own
// GetClientQueryContext, which reads the --node flag and falls back to its
// default of tcp://localhost:26657. akt keeps the endpoint in the active
// context instead, so every one of these commands dialled localhost and failed
// on a perfectly good remote-RPC setup -- 38 commands across ibc and
// ibc-transfer. Two of them did worse than fail: upstream discards the query
// error and dereferences the nil result, so `query ibc client params` and
// `query ibc connection params` segfaulted with a Go stack trace and exit 2.
//
// Resolving the context here and storing it back on the command means the SDK
// finds a client already attached and leaves it alone, which fixes the wrong
// endpoint and both crashes with it.
//
// The hook is set on every command in the subtree, not just its root: cobra
// runs only the closest PersistentPreRunE, so a descendant that declares its
// own would otherwise skip this.
func adoptVendoredQueryCmd(cmd *cobra.Command) *cobra.Command {
	applyVendoredQueryPreRun(cmd)

	return cmd
}

func applyVendoredQueryPreRun(cmd *cobra.Command) {
	if inner := cmd.PersistentPreRunE; inner != nil {
		cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
			if err := vendoredQueryPreRunE(c, args); err != nil {
				return err
			}

			return inner(c, args)
		}
	} else {
		cmd.PersistentPreRunE = vendoredQueryPreRunE
	}

	for _, sub := range cmd.Commands() {
		applyVendoredQueryPreRun(sub)
	}
}

func vendoredQueryPreRunE(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	if cmd.Flags().Changed(cflags.FlagNode) {
		rpcURI, _ := cmd.Flags().GetString(cflags.FlagNode)
		ctx = context.WithValue(ctx, ContextTypeRPCURI, rpcURI)
		cmd.SetContext(ctx)
	}

	cctx, err := GetClientQueryContext(cmd)
	if err != nil {
		return err
	}

	return sdkclient.SetCmdClientContext(cmd, cctx)
}
