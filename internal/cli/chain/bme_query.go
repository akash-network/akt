package cli

import (
	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	types "pkg.akt.dev/go/node/bme/v1"
)

func GetQueryBMECmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "BME query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		GetBMEParamsCmd(),
		GetBMEVaultStateCmd(),
		GetBMEStatusCmd(),
		GetBMELedgerRecordsCmd(),
	)

	return cmd
}

// GetBMEParamsCmd returns the command to query BME module parameters
func GetBMEParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "params",
		Args:              cobra.NoArgs,
		Short:             "Query the current BME module parameters",
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			req := &types.QueryParamsRequest{}

			res, err := cl.Query().BME().Params(ctx, req)
			if err != nil {
				return err
			}
			if err := requireQueryResponse("BME params", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetBMEVaultStateCmd returns the command to query the BME vault state
func GetBMEVaultStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "vault-state",
		Args:              cobra.NoArgs,
		Short:             "Query the current BME vault state",
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			req := &types.QueryVaultStateRequest{}

			res, err := cl.Query().BME().VaultState(ctx, req)
			if err != nil {
				return err
			}
			if err := requireQueryResponse("BME vault state", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetBMEStatusCmd returns the command to query the BME circuit breaker status
func GetBMEStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "status",
		Args:              cobra.NoArgs,
		Short:             "Query status of mint operations",
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			req := &types.QueryStatusRequest{}

			res, err := cl.Query().BME().Status(ctx, req)
			if err != nil {
				return err
			}
			if err := requireQueryResponse("BME status", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetBMELedgerRecordsCmd returns the command to query BME ledger records
func GetBMELedgerRecordsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ledger",
		Short: "Query ledger records",
		Long: `Query the burn/mint ledger: one record per conversion requested through
"akt tx bme burn-mint", "mint-act" or "burn-act".

The STATUS column reports where a conversion is:

  Pending             recorded, not settled yet; the chain burns and mints in a
                      later block, so BURNED is known and MINTED is not
  Executed            settled; BURNED and MINTED show the amounts and the
                      oracle price each was converted at
  Canceled (reason)   the conversion errored and the funds were returned; the
                      reason names the cause (for example "insufficient funds")

Filter with --owner, --denom, --to-denom and --status.`,
		Args:              cobra.ExactArgs(0),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			filters, err := cflags.BMELedgerFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			pageReq, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := cl.Query().BME().LedgerRecords(ctx, &types.QueryLedgerRecordsRequest{
				Filters:    filters,
				Pagination: pageReq,
			})
			if err != nil {
				return err
			}
			if err := requireQueryResponse("BME ledger", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "ledger records")
	cflags.AddBMELedgerFilterFlags(cmd.Flags())

	return cmd
}
