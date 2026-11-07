package cli

import (
	"fmt"
	"math"
	"strconv"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	types "pkg.akt.dev/go/node/sink/v1"
)

// GetQuerySinkCmd returns the query commands for the sink module
func GetQuerySinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Sink (AKT to Solana transfer) query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		GetSinkTransferCmd(),
		GetSinkTransfersCmd(),
		GetSinkDisputesCmd(),
		GetSinkReportersCmd(),
		GetSinkAccountCmd(),
		GetSinkStateCmd(),
		GetSinkParamsCmd(),
	)

	return cmd
}

// GetSinkTransferCmd returns a transfer's record, void order and settlement
func GetSinkTransferCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "transfer [owner/height/sequence]",
		Short:             "Query a transfer's record, void order and settlement",
		Example:           `  akt query sink transfer <owner>/29188334/1`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSinkTransferID(args[0])
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			res, err := cl.Query().Sink().Transfer(ctx, &types.QueryTransferRequest{ID: id})
			if err != nil {
				return err
			}
			if err := requireQueryResponse("sink transfer", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetSinkTransfersCmd lists transfers through native owner/status pagination.
func GetSinkTransfersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfers [owner] [status]",
		Short: "List transfers across all financial states or filter by owner and status",
		Long: `List all transfers, or supply an owner, a status, or both.
Statuses: transfer-pending, minted, voided, refund-pending, refunded, all.

Use --page-key with the returned key to continue. Transfer cursors retain
their original owner/status selection. Keep --height and --reverse unchanged
across pages for a consistent snapshot. --offset and --page require a status.
Held transfers of every status can also be found with "akt q sink disputes".`,
		Example: `  akt q sink transfers
  akt q sink transfers refund-pending
  akt q sink transfers <owner> minted --limit 50 --height 29188334`,
		Args:              cobra.MaximumNArgs(2),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			req := &types.QueryTransfersRequest{}
			if len(args) > 0 {
				if status, err := parseSinkStatus(args[0]); err == nil {
					if len(args) != 1 {
						return fmt.Errorf("supply owner before status, or status alone")
					}
					req.Status = status
				} else {
					req.Owner = args[0]
				}
			}
			if len(args) == 2 {
				status, err := parseSinkStatus(args[1])
				if err != nil {
					return err
				}
				req.Status = status
			}
			if err := req.ValidateBasic(); err != nil {
				return err
			}

			page, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}
			if page.Offset > 0 && req.Status == types.StatusInvalid {
				return fmt.Errorf("sink transfers --offset or --page requires a financial status")
			}
			if page.Offset > 0 && len(page.Key) > 0 {
				return fmt.Errorf("--page-key cannot be combined with --offset or --page")
			}
			req.Pagination = page

			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)
			res, err := cl.Query().Sink().Transfers(ctx, req)
			if err != nil {
				return err
			}
			if err := requireQueryResponse("sink transfers", res); err != nil {
				return err
			}
			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}
	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "transfers")
	return cmd
}

// GetSinkDisputesCmd lists held transfers, including minted and refunded ones.
func GetSinkDisputesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disputes",
		Short: "List transfers held for governance, including terminal disputes",
		Long: `List one page of held transfers, including already minted or refunded
transfers. Continue with --page-key; keep --height and --reverse unchanged
across pages for a consistent snapshot.`,
		Example: `  akt q sink disputes
  akt q sink disputes --limit 50 --height 29188334`,
		Args:              cobra.NoArgs,
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			page, err := ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}
			if page.Offset > 0 && len(page.Key) > 0 {
				return fmt.Errorf("--page-key cannot be combined with --offset or --page")
			}
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)
			res, err := cl.Query().Sink().Disputes(ctx, &types.QueryDisputesRequest{Pagination: page})
			if err != nil {
				return err
			}
			if err := requireQueryResponse("sink disputes", res); err != nil {
				return err
			}
			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}
	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddPaginationFlagsToCmd(cmd, "disputes")
	return cmd
}

// GetSinkReportersCmd queries the current or retained reporter policies.
func GetSinkReportersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reporters [version|all]",
		Short: "Query the current reporter policy, a retained version, or all policies",
		Example: `  akt q sink reporters
  akt q sink reporters 7
  akt q sink reporters all`,
		Args:              cobra.MaximumNArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			var version uint64
			if len(args) == 1 {
				if args[0] == "all" {
					version = math.MaxUint64
				} else {
					var err error
					version, err = strconv.ParseUint(args[0], 10, 64)
					if err != nil {
						return fmt.Errorf("invalid reporter policy version %q: use a version number or all", args[0])
					}
				}
			}
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)
			res, err := cl.Query().Sink().Reporters(ctx, &types.QueryReportersRequest{Version: version})
			if err != nil {
				return err
			}
			if err := requireQueryResponse("sink reporters", res); err != nil {
				return err
			}
			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}
	cflags.AddQueryFlagsToCmd(cmd)
	return cmd
}

// GetSinkAccountCmd returns lifetime and native epoch deposit usage.
func GetSinkAccountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account [address]",
		Short: "Query an account's lifetime and current epoch sink usage",
		Long: `Query lifetime sent AKT and usage normalized to the current native "day"
epoch. Without an address the default account is used.`,
		Example: `  akt query sink account

  # Another account
  akt query sink account <address>`,
		Args:              cobra.MaximumNArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			var sender string
			if len(args) == 1 {
				sender = args[0]
			} else {
				account, err := resolveDefaultAccountAddress(cl.ClientContext())
				if err != nil {
					return err
				}
				if account == "" {
					return fmt.Errorf("sink account: no default account set; provide an owner address or configure default-account")
				}
				sender = account
			}

			if _, err := sdk.AccAddressFromBech32(sender); err != nil {
				return fmt.Errorf("invalid sink account address %q: %w", sender, err)
			}

			res, err := cl.Query().Sink().Account(ctx, &types.QueryAccountRequest{Owner: sender})
			if err != nil {
				return err
			}
			if err := requireQueryResponse("sink account", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetSinkStateCmd returns financial totals, native epoch usage and refund head.
func GetSinkStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "state",
		Short:             "Query sink totals, epoch usage and the refund candidate",
		Long:              `Query all five financial populations, native epoch send/refund usage and the oldest refund candidate. A candidate still needs challenge maturity, enabled refunds and capacity before payment.`,
		Example:           `  akt query sink state`,
		Args:              cobra.NoArgs,
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			res, err := cl.Query().Sink().State(ctx, &types.QueryStateRequest{})
			if err != nil {
				return err
			}
			if err := requireQueryResponse("sink state", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetSinkParamsCmd returns the module parameters
func GetSinkParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "params",
		Short: "Query the current sink parameters",
		Long: `Query the current sink parameters. The JSON output is the parameter set
"akt tx sink propose-params" reads, so it can be saved, edited and proposed.`,
		Example: `  akt query sink params

  # Start a parameter change from the current values
  akt query sink params -o json > sink-params.json`,
		Args:              cobra.NoArgs,
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			res, err := cl.Query().Sink().Params(ctx, &types.QueryParamsRequest{})
			if err != nil {
				return err
			}
			if err := requireQueryResponse("sink params", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), &res.Params)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}
