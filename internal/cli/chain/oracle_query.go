package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	types "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/sdkutil"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/cliutil"
	"pkg.akt.dev/akt/internal/output/pretty"
)

// normalizeOracleDenom maps the denom spellings the rest of the CLI uses onto
// the key the oracle module stores prices under.
//
// The oracle keys prices by the base denom ("akt") — akt's own production
// caller passes sdkutil.DenomAkt — while every other surface (gas, balances,
// fees, escrow) takes the micro denom ("uakt"). Without this, the one command
// that reads the oracle would be the only one in the tool that rejects "uakt".
// Denoms outside the AKT/ACT families pass through untouched: they may be
// case-sensitive (IBC hashes) and we have no mapping for them.
func normalizeOracleDenom(denom string) string {
	switch strings.ToLower(strings.TrimSpace(denom)) {
	case sdkutil.DenomAkt, sdkutil.DenomMakt, sdkutil.DenomUakt:
		return sdkutil.DenomAkt
	case sdkutil.DenomAct, sdkutil.DenomMact, sdkutil.DenomUact:
		return sdkutil.DenomAct
	}

	return denom
}

// oracleQueryError wraps a raw gRPC/ABCI status in the three-part error
// contract (SPEC §11.1); without it the transport error reaches the user
// verbatim.
func oracleQueryError(what string, cause error) error {
	return &cliutil.CLIError{
		Code:       cliutil.ExitGeneral,
		Message:    fmt.Sprintf("oracle query failed: %s", what),
		Cause:      cause,
		Context:    "querying the oracle module",
		Suggestion: `Run "akt q oracle prices" to list the denoms the oracle carries.`,
	}
}

func GetQueryOracleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Oracle query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		GetOraclePricesCmd(),
		GetOracleAggregatedPriceCmd(),
		GetQueryOracleParamsCmd(),
	)

	return cmd
}

func GetOraclePricesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "prices",
		Args:              cobra.NoArgs,
		Aliases:           []string{"p"},
		Short:             "Query price history for denoms",
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			pageReq, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			// Get filter flags
			assetDenom, _ := cmd.Flags().GetString(cflags.FlagAssetDenom)
			baseDenom, _ := cmd.Flags().GetString(cflags.FlagBaseDenom)
			startTimeStr, _ := cmd.Flags().GetString("start-time")
			endTimeStr, _ := cmd.Flags().GetString("end-time")

			filters := types.PricesFilter{
				AssetDenom: assetDenom,
				BaseDenom:  baseDenom,
			}

			if startTimeStr != "" {
				ts, err := time.Parse(time.RFC3339, startTimeStr)
				if err != nil {
					return err
				}
				filters.StartTime = ts
			}

			if endTimeStr != "" {
				ts, err := time.Parse(time.RFC3339, endTimeStr)
				if err != nil {
					return err
				}
				filters.EndTime = ts
			}

			if !filters.StartTime.IsZero() && !filters.EndTime.IsZero() && filters.StartTime.After(filters.EndTime) {
				return fmt.Errorf("start-time %q must be before end-time %q", startTimeStr, endTimeStr)
			}

			req := &types.QueryPricesRequest{
				Filters:    filters,
				Pagination: pageReq,
			}

			res, err := cl.Query().Oracle().Prices(ctx, req)
			if err != nil {
				return oracleQueryError("cannot read the price history", err)
			}
			if err := requireQueryResponse("oracle prices", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "prices")
	cmd.Flags().String(cflags.FlagAssetDenom, "", "Filter by asset denomination as the oracle keys it, i.e. the base denom (e.g., akt)")
	cmd.Flags().String(cflags.FlagBaseDenom, "", "Filter by base denomination (e.g., usd)")
	cmd.Flags().String("start-time", "", "Filter by start time (RFC3339 format, e.g., 2024-01-01T00:00:00Z)")
	cmd.Flags().String("end-time", "", "Filter by end time (RFC3339 format, e.g., 2024-01-01T00:00:00Z)")

	return cmd
}

func GetOracleAggregatedPriceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "aggregated-price [denom]",
		Aliases: []string{"ap"},
		Short:   "Query aggregated price for a denom",
		Long: `Query the aggregated oracle price for a denom.

The oracle keys prices by base denom, so "akt", "AKT" and "uakt" all resolve to
"akt" (and the ACT family to "act"). Any other denom is sent as written.

Run "akt q oracle prices" to see which denoms the oracle carries.`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			denom := normalizeOracleDenom(args[0])
			if denom != args[0] {
				cliutil.Verbosef(cmd, "resolved denom %q to the oracle key %q", args[0], denom)
			}

			req := &types.QueryAggregatedPriceRequest{
				Denom: denom,
			}

			res, err := cl.Query().Oracle().AggregatedPrice(ctx, req)
			if err != nil {
				return oracleQueryError(fmt.Sprintf("no aggregated price for denom %q", denom), err)
			}
			if err := requireQueryResponse("oracle aggregated price", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

func GetQueryOracleParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "params",
		Args:              cobra.NoArgs,
		Short:             "Query the current oracle parameters",
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			req := &types.QueryParamsRequest{}

			res, err := cl.Query().Oracle().Params(ctx, req)
			if err != nil {
				return oracleQueryError("cannot read the module parameters", err)
			}
			if err := requireQueryResponse("oracle params", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}
