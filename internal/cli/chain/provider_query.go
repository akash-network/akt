package cli

import (
	"github.com/spf13/cobra"

	sdk "github.com/cosmos/cosmos-sdk/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	types "pkg.akt.dev/go/node/provider/v1beta4"
)

// GetQueryProviderCmds returns the query commands for the provider module.
// Per the positional-primary convention (SPEC §3.8) the group itself is the
// query: a positional address returns that provider, no argument lists them
// all. `list` and `get` remain registered so existing scripts keep working.
func GetQueryProviderCmds() *cobra.Command {
	cmd := &cobra.Command{
		Use:   types.ModuleName + " [address]",
		Short: "Query providers",
		Long: `Query providers.

Without arguments, lists every provider on the network.

[address] is a provider's bech32 account address and returns that provider's
record alone.`,
		Example: `  # Every provider on the network
  akt query provider

  # One provider
  akt query provider akash1prov...`,
		Args:                       cobra.MaximumNArgs(1),
		SuggestionsMinimumDistance: 2,
		PersistentPreRunE:          QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return queryProviderList(cmd)
			}

			return queryProvider(cmd, args[0])
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "providers")

	cmd.AddCommand(
		GetQueryProvidersCmd(),
		GetQueryProviderCmd(),
	)

	return cmd
}

func GetQueryProvidersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Args:              cobra.NoArgs,
		Short:             "Query for all providers",
		Long:              "Query for all providers. Equivalent to `akt query provider` with no argument.",
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return queryProviderList(cmd)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "providers")

	return cmd
}

func GetQueryProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get [address]",
		Short:             "Query provider",
		Long:              "Query one provider by address. Equivalent to `akt query provider <address>`.",
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			return queryProvider(cmd, args[0])
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// queryProviderList prints every provider on the network.
func queryProviderList(cmd *cobra.Command) error {
	ctx := cmd.Context()
	cl := MustLightClientFromContext(ctx)

	pageReq, err := ReadPageRequest(cmd.Flags())
	if err != nil {
		return err
	}

	res, err := cl.Query().Provider().Providers(ctx, &types.QueryProvidersRequest{
		Pagination: pageReq,
	})
	if err != nil {
		return err
	}

	return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
}

// queryProvider prints one provider's record.
func queryProvider(cmd *cobra.Command, address string) error {
	ctx := cmd.Context()
	cl := MustLightClientFromContext(ctx)

	owner, err := sdk.AccAddressFromBech32(address)
	if err != nil {
		return err
	}

	res, err := cl.Query().Provider().Provider(ctx, &types.QueryProviderRequest{Owner: owner.String()})
	if err != nil {
		return err
	}

	return pretty.PrintQueryResult(cmd, cl.ClientContext(), &res.Provider)
}
