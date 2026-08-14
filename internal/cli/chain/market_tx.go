package cli

import (
	"errors"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
)

// GetTxMarketCmds returns the transaction commands for market module
func GetTxMarketCmds() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        mv1.ModuleName,
		Short:                      "Transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}
	cmd.AddCommand(
		GetTxMarketBidCmds(),
		GetTxMarketLeaseCmds(),
	)
	return cmd
}

func GetTxMarketBidCmds() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "bid",
		Short:                      "Bid subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}
	cmd.AddCommand(
		GetTxMarketBidCreateCmd(),
		GetTxMarketBidCloseCmd(),
	)
	return cmd
}

func GetTxMarketBidCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             "Create a market bid",
		Args:              cobra.ExactArgs(0),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			price, err := cmd.Flags().GetString(flagdefs.FlagPrice)
			if err != nil {
				return err
			}

			coin, err := sdk.ParseDecCoin(price)
			if err != nil {
				return err
			}

			id, err := cflags.OrderIDFromFlags(cmd.Flags(), cflags.WithProvider(cctx.FromAddress))
			if err != nil {
				return err
			}

			deposit, err := DetectDeposit(ctx, cmd.Flags(), cl.Query(), DetectBidDeposit)
			if err != nil {
				return err
			}

			msg := &mtypes.MsgCreateBid{
				ID:      mv1.MakeBidID(id, cctx.GetFromAddress()),
				Price:   coin,
				Deposit: deposit,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)
	cflags.AddOrderIDFlags(cmd.Flags())

	cmd.Flags().String(flagdefs.FlagPrice, "", "Bid Price")
	cflags.AddDepositFlags(cmd.Flags())

	return cmd
}

func GetTxMarketBidCloseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "close",
		Short:             "Close a market bid",
		Args:              cobra.ExactArgs(0),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			id, err := cflags.BidIDFromFlags(cmd.Flags(), cflags.WithProvider(cctx.FromAddress))
			if err != nil {
				return err
			}

			reason, err := cflags.BidClosedReasonFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &mtypes.MsgCloseBid{
				ID:     id,
				Reason: reason,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)
	cflags.AddBidIDFlags(cmd.Flags())
	cflags.AddBidClosedReasonFlag(cmd.Flags())

	return cmd
}

func GetTxMarketLeaseCmds() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "lease",
		Short:                      "Lease subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		GetTxMarketLeaseCreateCmd(),
		GetTxMarketLeaseWithdrawCmd(),
		GetTxMarketLeaseCloseCmd(),
	)

	return cmd
}

func GetTxMarketLeaseCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create [dseq] [provider]",
		Short:             "Create a market lease",
		Args:              cobra.MaximumNArgs(2),
		Example:           `akt tx market lease create 12345 akash1provider...`,
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			id, err := leaseIDFromFlagsAndArgs(cmd, args, cctx.FromAddress)
			if err != nil {
				return err
			}

			msg := &mtypes.MsgCreateLease{
				BidID: id.BidID(),
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)
	addLeaseOwnerTxFlags(cmd)

	return cmd
}

func GetTxMarketLeaseWithdrawCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "withdraw [dseq] [provider]",
		Short:             "Settle and withdraw available funds from market order escrow account",
		Args:              cobra.MaximumNArgs(2),
		Example:           `akt tx market lease withdraw 12345 akash1provider...`,
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			cctx := cl.ClientContext()

			id, err := leaseIDFromFlagsAndArgs(cmd, args, cctx.FromAddress)
			if err != nil {
				return err
			}

			msg := &mtypes.MsgWithdrawLease{
				ID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)
	addLeaseOwnerTxFlags(cmd)

	return cmd
}

func GetTxMarketLeaseCloseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "close [dseq] [provider]",
		Short:             "Close a market order",
		Args:              cobra.MaximumNArgs(2),
		Example:           `akt tx market lease close 12345 akash1provider...`,
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)
			cctx := cl.ClientContext()

			id, err := leaseIDFromFlagsAndArgs(cmd, args, cctx.FromAddress)
			if err != nil {
				return err
			}

			// for lease closed tx reason is always owner
			msg := &mtypes.MsgCloseLease{
				ID:     id,
				Reason: mv1.LeaseClosedReasonOwner,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)
	addLeaseOwnerTxFlags(cmd)

	return cmd
}

// addLeaseOwnerTxFlags registers the lease identity flags for the lease tx
// commands: --owner (defaults to the signer), --gseq, and --oseq stay — none
// of them has a positional twin.
// FEEDBACK(2026-07): --dseq/--provider disabled for the positional-only UX
// trial (use the positional [dseq] [provider] arguments instead). Restore by
// replacing this helper's body with the original registration:
// cflags.AddLeaseIDFlags(cmd.Flags())
func addLeaseOwnerTxFlags(cmd *cobra.Command) {
	cmd.Flags().String(cflags.FlagOwner, "", "Deployment Owner")
	cmd.Flags().Uint32(cflags.FlagGSeq, 1, "Group Sequence")
	cmd.Flags().Uint32(cflags.FlagOSeq, 1, "Order Sequence")
	// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().Uint64(cflags.FlagDSeq, 0, "Deployment Sequence")
	// FEEDBACK(2026-07): --provider disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().String(cflags.FlagProvider, "", "Provider")
}

// leaseIDFromFlagsAndArgs resolves a LeaseID from the optional positional
// [dseq] [provider] arguments; gseq/oseq keep their flag defaults and the
// owner defaults to the signer (overridable with --owner).
func leaseIDFromFlagsAndArgs(cmd *cobra.Command, args []string, owner sdk.AccAddress) (mv1.LeaseID, error) {
	oid, err := cflags.OrderIDFromFlags(cmd.Flags(), cflags.WithOwner(owner))
	if err != nil {
		return mv1.LeaseID{}, err
	}

	// FEEDBACK(2026-07): --provider disabled for the positional-only UX
	// trial (use the positional form instead). Restore by uncommenting if
	// users ask for the flag form back.
	// provider, err := cmd.Flags().GetString(cflags.FlagProvider)
	// if err != nil {
	// 	return mv1.LeaseID{}, err
	// }
	provider := ""

	// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial;
	// the positional [dseq] [provider] arguments are the only source (zero
	// fallbacks).
	if oid.DSeq, provider, err = cflags.LeaseSeqsFromArgs(args, 0, provider); err != nil {
		return mv1.LeaseID{}, err
	}

	if oid.DSeq == 0 {
		return mv1.LeaseID{}, errors.New("dseq is required: pass it positionally")
	}

	if provider == "" {
		return mv1.LeaseID{}, errors.New("provider is required: pass it positionally")
	}

	paddr, err := sdk.AccAddressFromBech32(provider)
	if err != nil {
		return mv1.LeaseID{}, err
	}

	return mv1.MakeBidID(oid, paddr).LeaseID(), nil
}
