package cli

import (
	"errors"
	"fmt"
	"time"

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

			var reclamationWindow *time.Duration
			value, _ := cmd.Flags().GetString(flagdefs.FlagReclamationWindow)
			if value != "" {
				duration, err := time.ParseDuration(value)
				if err != nil {
					return fmt.Errorf("--reclamation-window: %w", err)
				}
				reclamationWindow = &duration
			}
			msg := &mtypes.MsgCreateBid{
				ID:                mv1.MakeBidID(id, cctx.GetFromAddress()),
				ReclamationWindow: reclamationWindow,
				Price:             coin,
				Deposit:           deposit,
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
	cmd.Flags().String(flagdefs.FlagReclamationWindow, "", "Optional reclamation window duration (e.g. 24h, 720h)")
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
		Args:              cflags.PositionalOrFlagArgs(flagdefs.FlagDSeq, flagdefs.FlagProvider),
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
		Args:              cflags.PositionalOrFlagArgs(flagdefs.FlagDSeq, flagdefs.FlagProvider),
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
		Args:              cflags.PositionalOrFlagArgs(flagdefs.FlagDSeq, flagdefs.FlagProvider),
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

func addLeaseOwnerTxFlags(cmd *cobra.Command) {
	cflags.AddLeaseIDFlags(cmd.Flags())
}

// leaseIDFromFlagsAndArgs resolves a LeaseID from the optional positional
// [dseq] [provider] arguments; gseq/oseq keep their flag defaults and the
// owner defaults to the signer (overridable with --owner).
func leaseIDFromFlagsAndArgs(cmd *cobra.Command, args []string, owner sdk.AccAddress) (mv1.LeaseID, error) {
	oid, err := cflags.OrderIDFromFlags(cmd.Flags(), cflags.WithOwner(owner))
	if err != nil {
		return mv1.LeaseID{}, err
	}

	if err := cflags.RejectDuplicateArgs(cmd.Flags(), args, flagdefs.FlagDSeq, flagdefs.FlagProvider); err != nil {
		return mv1.LeaseID{}, err
	}
	provider, err := cmd.Flags().GetString(flagdefs.FlagProvider)
	if err != nil {
		return mv1.LeaseID{}, err
	}
	if oid.DSeq, provider, err = cflags.LeaseSeqsFromArgs(args, oid.DSeq, provider); err != nil {
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
