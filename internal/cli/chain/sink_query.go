package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	flagdefs "pkg.akt.dev/akt/internal/flags"
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
		GetSinkRecordCmd(),
		GetSinkTransferCmd(),
		GetSinkOpenTransfersCmd(),
		GetSinkAccountCmd(),
		GetSinkStateCmd(),
		GetSinkParamsCmd(),
	)

	return cmd
}

// GetSinkRecordCmd returns a transfer's public status and settlement
func GetSinkRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record [id]",
		Short: "Query a transfer's status and settlement by record ID",
		Long: `Query a sink transfer by the record ID its deposit returned.

STATUS is the module's public status:

  Transfer pending   AKT is locked; the Solana mint is not confirmed yet
  Completed          reporters confirmed the mint on Solana
  Void               Solana voided the transfer; the settlement shows whether
                     the refund is waiting for its challenge window or paid

The settlement below it shows reporter votes, holds and the challenge window.`,
		Example:           `  akt query sink record 12`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSinkTransferID(args[0])
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			res, err := cl.Query().Sink().Sink(ctx, &types.QuerySink{Id: strconv.FormatUint(id, 10)})
			if err != nil {
				return err
			}
			if err := requireQueryResponse("sink record", res); err != nil {
				return err
			}

			return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)

	return cmd
}

// GetSinkTransferCmd returns a transfer's record, void order and settlement
func GetSinkTransferCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "transfer [id]",
		Short:             "Query a transfer's record, void order and settlement",
		Example:           `  akt query sink transfer 12`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSinkTransferID(args[0])
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			res, err := cl.Query().Sink().Transfer(ctx, &types.QueryTransferRequest{Id: id})
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

// GetSinkOpenTransfersCmd pages through transfers that are neither minted nor
// refunded, in ascending ID order
func GetSinkOpenTransfersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open-transfers",
		Short: "List pending and voided transfers in ascending ID order",
		Long: `List transfers that are neither minted nor refunded, with IDs above
--start-after. When more remain, the output names the --start-after value
for the next page.

--held-only reads every page and lists only transfers held for governance
(a veto or conflicting reports), so one proposal can resolve all of them.`,
		Example: `  akt query sink open-transfers --limit 50

  # Every transfer waiting for governance
  akt query sink open-transfers --held-only`,
		Args:              cobra.NoArgs,
		PersistentPreRunE: QueryPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			startAfter, err := cmd.Flags().GetUint64(flagdefs.FlagStartAfter)
			if err != nil {
				return err
			}

			limit, err := cmd.Flags().GetUint32(flagdefs.FlagLimit)
			if err != nil {
				return err
			}

			heldOnly, err := cmd.Flags().GetBool(flagdefs.FlagHeldOnly)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			cl := MustLightClientFromContext(ctx)

			page := func(cursor uint64) (*types.QueryOpenTransfersResponse, error) {
				res, err := cl.Query().Sink().OpenTransfers(ctx, &types.QueryOpenTransfersRequest{
					StartAfter: cursor,
					Limit:      limit,
				})
				if err != nil {
					return nil, err
				}
				if err := requireQueryResponse("sink open transfers", res); err != nil {
					return nil, err
				}

				return res, nil
			}

			res, err := page(startAfter)
			if err != nil {
				return err
			}

			if !heldOnly {
				return pretty.PrintQueryResult(cmd, cl.ClientContext(), res)
			}

			held := &types.QueryOpenTransfersResponse{}
			cursor := startAfter
			for {
				for _, transfer := range res.Transfers {
					if transfer.Settlement.Hold != types.Hold_HOLD_NONE {
						held.Transfers = append(held.Transfers, transfer)
					}
				}

				if res.NextStartAfter == 0 {
					return pretty.PrintQueryResult(cmd, cl.ClientContext(), held)
				}

				// A cursor that does not advance would page forever.
				if res.NextStartAfter <= cursor {
					return fmt.Errorf("sink open transfers query returned malformed node response: next page starts after %d, not after %d", res.NextStartAfter, cursor)
				}
				cursor = res.NextStartAfter

				if res, err = page(cursor); err != nil {
					return err
				}
			}
		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddSinkOpenTransfersFlags(cmd.Flags())

	return cmd
}

// GetSinkAccountCmd returns an account's daily deposit usage and pending count
func GetSinkAccountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account [address]",
		Short: "Query an account's sink usage for the current UTC day",
		Long: `Query how many deposits and how much AKT an account sent to the sink on
its last deposit day, and how many of its transfers are still open. These
are the counters the per-account limits check. Without an address the
default account is used.`,
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
					return requireOwnerScope("sink account")
				}
				sender = account
			}

			if _, err := sdk.AccAddressFromBech32(sender); err != nil {
				return fmt.Errorf("invalid sink account address %q: %w", sender, err)
			}

			res, err := cl.Query().Sink().Account(ctx, &types.QueryAccountRequest{Sender: sender})
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

// GetSinkStateCmd returns the module totals and parameters
func GetSinkStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "state",
		Short:             "Query sink totals and parameters",
		Long:              `Query the locked AKT by bucket (pending, minted, voided), refund counters, the reporter epoch and the current parameters.`,
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

			res, err := cl.Query().Sink().State(ctx, &types.QueryStateRequest{})
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
