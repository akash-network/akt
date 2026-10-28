package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
	aclient "pkg.akt.dev/go/node/client"
	types "pkg.akt.dev/go/node/sink/v1"
)

// GetTxSinkCmd returns the transaction commands for the sink module
func GetTxSinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Sink (AKT to Solana transfer) transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		GetTxSinkDepositCmd(),
		GetTxSinkGuardianPauseCmd(),
		GetTxSinkReportOutcomeCmd(),
		GetTxSinkVetoVoidCmd(),
		GetTxSinkFinalizeRefundCmd(),
		GetTxSinkProposeParamsCmd(),
		GetTxSinkProposeVoidCmd(),
		GetTxSinkProposeVetoCmd(),
		GetTxSinkProposeResolveCmd(),
	)

	return cmd
}

// GetTxSinkDepositCmd locks AKT for a Solana destination through MsgSink
func GetTxSinkDepositCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deposit [solana-address] [amount]",
		Short: "Lock AKT on Akash to mint the same amount on Solana",
		Long: `Lock AKT in the sink module for a Solana wallet address. The amount is
whole uakt (1500000uakt) or an exact AKT decimal (1.5akt).

The deposit only locks AKT. A successful broadcast means the chain recorded
the transfer; the AKT is minted on Solana once a relayer proves the record,
usually within a few minutes, and reporters then confirm it on Akash. If
Solana cannot accept the destination, the transfer is voided and the AKT is
refunded to the sender after the challenge window. Follow the transfer with
"akt q sink record <id>".

Use a Solana wallet address, not a token account.`,
		Example: `  akt tx sink deposit <solana-address> 1000000uakt --from mykey
  akt tx sink deposit <solana-address> 2.5akt --from mykey

  # Then follow it
  akt q sink record <id>`,
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			destType, err := cmd.Flags().GetUint32(flagdefs.FlagDestType)
			if err != nil {
				return err
			}

			amount, err := parseSinkAmount(args[1])
			if err != nil {
				return err
			}

			cctx, err := GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgSink{
				DestType: destType,
				Dest:     args[0],
				Amount:   amount,
				Sender:   cctx.GetFromAddress().String(),
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

	cflags.AddSinkDepositFlags(cmd.Flags())
	cflags.AddTxFlagsToCmd(cmd)

	return cmd
}

// GetTxSinkGuardianPauseCmd disables deposits, refunds or both. The signer
// must be params.pause_guardian; only governance can enable either again.
func GetTxSinkGuardianPauseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guardian-pause [deposits|refunds|all]",
		Short: "Disable sink deposits, refunds or both as the pause guardian",
		Long: `Disable sink deposits, refunds or both. The signer must be the pause
guardian named in the sink parameters. The guardian can only switch things
off; a governance proposal ("akt tx sink propose-params") turns them back on.`,
		Example: `  akt tx sink guardian-pause all --from guardian
  akt tx sink guardian-pause refunds --from guardian`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			deposits, refunds, err := parseSinkPauseTarget(args[0])
			if err != nil {
				return err
			}

			cctx, err := GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgGuardianPause{
				Guardian: cctx.GetFromAddress().String(),
				Deposits: deposits,
				Refunds:  refunds,
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

	return cmd
}

// GetTxSinkReportOutcomeCmd reports a transfer's finalized Solana outcome as
// a reporter
func GetTxSinkReportOutcomeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report-outcome [id] [minted|voided] [slot] [signature]",
		Short: "Report a transfer's finalized Solana outcome as a reporter",
		Long: `Vote on a transfer's Solana outcome. The signer must be a current
reporter. The evidence is the finalized slot and the base58 signature of the
Solana transaction that minted or voided the record; read both from your own
Solana node.

The report names the chain's current reporter epoch and the transfer's
current round, which are read from the chain before signing. Use
--reporter-epoch and --round to name them explicitly.`,
		Example:           `  akt tx sink report-outcome 12 minted 412345678 <solana-signature> --from reporter`,
		Args:              cobra.ExactArgs(4),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			id, err := parseSinkTransferID(args[0])
			if err != nil {
				return err
			}

			outcome, err := parseSinkOutcome(args[1])
			if err != nil {
				return err
			}

			observation, err := parseSinkObservation(outcome, args[2], args[3])
			if err != nil {
				return err
			}

			epoch, err := cmd.Flags().GetUint64(flagdefs.FlagReporterEpoch)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed(flagdefs.FlagReporterEpoch) {
				res, err := cl.Query().Sink().State(ctx, &types.QueryStateRequest{})
				if err != nil {
					return fmt.Errorf("read the current reporter epoch: %w", err)
				}
				if err := requireQueryResponse("sink state", res); err != nil {
					return err
				}
				epoch = res.Totals.ReporterEpoch
			}

			round, err := cmd.Flags().GetUint64(flagdefs.FlagRound)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed(flagdefs.FlagRound) {
				res, err := cl.Query().Sink().Transfer(ctx, &types.QueryTransferRequest{Id: id})
				if err != nil {
					return fmt.Errorf("read the round of sink transfer %d: %w", id, err)
				}
				if err := requireQueryResponse("sink transfer", res); err != nil {
					return err
				}
				round = res.Transfer.Settlement.Round
			}

			cctx, err := GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgReportOutcome{
				Reporter:      cctx.GetFromAddress().String(),
				Id:            id,
				ReporterEpoch: epoch,
				Round:         round,
				Observation:   observation,
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddSinkReportFlags(cmd.Flags())
	cflags.AddTxFlagsToCmd(cmd)

	return cmd
}

// GetTxSinkVetoVoidCmd holds a voided transfer before its refund
func GetTxSinkVetoVoidCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "veto-void [id] [reason]",
		Short: "Veto a void confirmation during its challenge window as a reporter",
		Long: `Stop the refund of a voided transfer. A current reporter can veto during
the challenge window, for example when the void does not match Solana. The
transfer is then held until governance resolves it. Governance vetoes
through "akt tx sink propose-veto".`,
		Example:           `  akt tx sink veto-void 12 "status account shows a mint at slot 412345678" --from reporter`,
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			id, err := parseSinkTransferID(args[0])
			if err != nil {
				return err
			}

			reason, err := parseSinkReason(args[1])
			if err != nil {
				return err
			}

			cctx, err := GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgVetoVoid{
				Signer: cctx.GetFromAddress().String(),
				Id:     id,
				Reason: reason,
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)

	return cmd
}

// GetTxSinkFinalizeRefundCmd pays an eligible refund to the original sender
func GetTxSinkFinalizeRefundCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "finalize-refund [id]",
		Short: "Pay a voided transfer's refund once its challenge window has passed",
		Long: `Refund a voided transfer. Anyone can send this and pay its fee; the
refund always goes to the transfer's original sender, for exactly the
recorded amount. The chain refuses it while the challenge window is open,
while the transfer is held, when refunds are disabled or when the day's
refund cap is reached.`,
		Example:           `  akt tx sink finalize-refund 12 --from mykey`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			id, err := parseSinkTransferID(args[0])
			if err != nil {
				return err
			}

			cctx, err := GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgFinalizeRefund{
				Executor: cctx.GetFromAddress().String(),
				Id:       id,
			}

			resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
		},
	}

	cflags.AddTxFlagsToCmd(cmd)

	return cmd
}

// GetTxSinkProposeParamsCmd submits a governance proposal replacing the sink
// parameters
func GetTxSinkProposeParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "propose-params [params.json]",
		Short: "Submit a governance proposal that replaces the sink parameters",
		Long: `Submit a governance proposal with MsgUpdateParams. The file holds the
complete parameter set as JSON, in the shape "akt q sink params -o json"
prints; every field is replaced, so start from the current values. The
parameters are checked locally before broadcast.`,
		Example: `  akt query sink params -o json > sink-params.json
  # edit sink-params.json, then
  akt tx sink propose-params sink-params.json --deposit 50000000uakt --from mykey`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			authority, err := sinkProposalAuthority(cmd)
			if err != nil {
				return err
			}

			raw, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read sink parameters: %w", err)
			}

			var params types.Params
			if err := cl.ClientContext().Codec.UnmarshalJSON(raw, &params); err != nil {
				return fmt.Errorf("parse sink parameters %s: %w", args[0], err)
			}
			if err := params.Validate(); err != nil {
				return fmt.Errorf("invalid sink parameters: %w", err)
			}

			msg := &types.MsgUpdateParams{Authority: authority, Params: params}

			return submitSinkProposal(cmd, cl, msg,
				"Update sink parameters",
				"Replace the sink module parameters.")
		},
	}

	cflags.AddSinkProposalFlags(cmd, DefaultGovAuthority.String())

	return cmd
}

// GetTxSinkProposeVoidCmd submits a governance proposal that writes a void
// order for a transfer
func GetTxSinkProposeVoidCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "propose-void [id]",
		Short: "Submit a governance proposal that orders a transfer voided",
		Long: `Submit a governance proposal with MsgRequestVoid. Once it passes, the
void order is proven on Solana and the transfer can never be minted; after
reporters confirm the void and the challenge window passes, the AKT is
refunded. If the transfer was already minted, the void fails on Solana and
the transfer closes as minted. Use this for a transfer that cannot be minted.`,
		Example:           `  akt tx sink propose-void 12 --deposit 50000000uakt --from mykey`,
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			id, err := parseSinkTransferID(args[0])
			if err != nil {
				return err
			}

			authority, err := sinkProposalAuthority(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgRequestVoid{Authority: authority, Id: id}

			return submitSinkProposal(cmd, cl, msg,
				fmt.Sprintf("Void sink transfer %d", id),
				fmt.Sprintf("Write a void order for sink transfer %d so it is voided on Solana and refunded to its sender.", id))
		},
	}

	cflags.AddSinkProposalFlags(cmd, DefaultGovAuthority.String())

	return cmd
}

// GetTxSinkProposeVetoCmd submits a governance proposal that vetoes a void
func GetTxSinkProposeVetoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "propose-veto [id] [reason]",
		Short: "Submit a governance proposal that vetoes a void before its refund",
		Long: `Submit a governance proposal with MsgVetoVoid signed by governance. Unlike
a reporter, governance can veto a voided transfer at any time before the
refund is paid. The transfer is then held until a resolution passes.`,
		Example:           `  akt tx sink propose-veto 12 "void evidence does not match Solana" --from mykey`,
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			id, err := parseSinkTransferID(args[0])
			if err != nil {
				return err
			}

			reason, err := parseSinkReason(args[1])
			if err != nil {
				return err
			}

			authority, err := sinkProposalAuthority(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgVetoVoid{Signer: authority, Id: id, Reason: reason}

			return submitSinkProposal(cmd, cl, msg,
				fmt.Sprintf("Veto the void of sink transfer %d", id),
				fmt.Sprintf("Hold sink transfer %d before its refund: %s", id, reason))
		},
	}

	cflags.AddSinkProposalFlags(cmd, DefaultGovAuthority.String())

	return cmd
}

// GetTxSinkProposeResolveCmd submits a governance proposal that resolves a
// held or stalled transfer
func GetTxSinkProposeResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "propose-resolve [id] [restart|confirm-mint|confirm-void] [reason] [slot] [signature]",
		Short: "Submit a governance proposal that resolves a held or stalled transfer",
		Long: `Submit a governance proposal with MsgResolveTransfer.

  restart        clear the votes and hold and let reporters vote again
  confirm-mint   close the transfer as minted; give the Solana slot and
                 signature of the mint
  confirm-void   confirm the Solana void; give the slot and signature of the
                 void. A new challenge window starts, and the refund still
                 obeys the refund switch and daily cap

A transfer that is already minted or refunded cannot change.`,
		Example: `  akt tx sink propose-resolve 12 restart "reporters disagreed after an RPC outage" --from mykey
  akt tx sink propose-resolve 12 confirm-void "recipient was a token account" 412345678 <solana-signature> --from mykey`,
		Args:              cobra.RangeArgs(3, 5),
		PersistentPreRunE: TxPersistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl := MustClientFromContext(ctx)

			id, err := parseSinkTransferID(args[0])
			if err != nil {
				return err
			}

			decision, err := parseSinkDecision(args[1])
			if err != nil {
				return err
			}

			reason, err := parseSinkReason(args[2])
			if err != nil {
				return err
			}

			authority, err := sinkProposalAuthority(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgResolveTransfer{Authority: authority, Id: id, Decision: decision, Reason: reason}

			switch decision {
			case types.Decision_DECISION_RESTART:
				if len(args) != 3 {
					return fmt.Errorf("restart takes no Solana evidence: pass only the ID, decision and reason")
				}
			default:
				if len(args) != 5 {
					return fmt.Errorf("%s needs the Solana slot and transaction signature after the reason", args[1])
				}

				outcome := types.Outcome_OUTCOME_VOIDED
				if decision == types.Decision_DECISION_CONFIRM_MINT {
					outcome = types.Outcome_OUTCOME_MINTED
				}

				observation, err := parseSinkObservation(outcome, args[3], args[4])
				if err != nil {
					return err
				}
				msg.Observation = &observation
			}

			return submitSinkProposal(cmd, cl, msg,
				fmt.Sprintf("Resolve sink transfer %d: %s", id, strings.ToLower(args[1])),
				fmt.Sprintf("Resolve sink transfer %d (%s): %s", id, strings.ToLower(args[1]), reason))
		},
	}

	cflags.AddSinkProposalFlags(cmd, DefaultGovAuthority.String())

	return cmd
}

// sinkProposalAuthority returns the authority a proposed sink message names.
func sinkProposalAuthority(cmd *cobra.Command) (string, error) {
	authority, err := cmd.Flags().GetString(flagdefs.FlagAuthority)
	if err != nil {
		return "", err
	}

	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		return "", fmt.Errorf("invalid authority %q: %w", authority, err)
	}

	return authority, nil
}

// submitSinkProposal wraps one sink message in a governance proposal signed
// by the selected account and broadcasts it through the logged chain client.
// Title and summary fall back to a description of the action.
func submitSinkProposal(cmd *cobra.Command, cl aclient.Client, msg sdk.Msg, defaultTitle, defaultSummary string) error {
	title, err := cmd.Flags().GetString(flagdefs.FlagTitle)
	if err != nil {
		return err
	}
	if strings.TrimSpace(title) == "" {
		title = defaultTitle
	}

	summary, err := cmd.Flags().GetString(flagdefs.FlagSummary)
	if err != nil {
		return err
	}
	if strings.TrimSpace(summary) == "" {
		summary = defaultSummary
	}

	depositArg, err := cmd.Flags().GetString(flagdefs.FlagDeposit)
	if err != nil {
		return err
	}

	deposit, err := sdk.ParseCoinsNormalized(depositArg)
	if err != nil {
		return fmt.Errorf("invalid proposal deposit %q: %w", depositArg, err)
	}

	expedite, err := cmd.Flags().GetBool(flagdefs.FlagExpedite)
	if err != nil {
		return err
	}

	cctx, err := GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	proposal, err := v1.NewMsgSubmitProposal([]sdk.Msg{msg}, deposit, cctx.GetFromAddress().String(), "", title, summary, expedite)
	if err != nil {
		return fmt.Errorf("build sink proposal: %w", err)
	}

	resp, err := cl.Tx().BroadcastMsgs(cmd.Context(), []sdk.Msg{proposal})
	if err != nil {
		return err
	}

	return pretty.PrintTxResult(cmd, cl.ClientContext(), resp)
}
