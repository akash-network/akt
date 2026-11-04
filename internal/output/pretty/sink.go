package pretty

import (
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/gogoproto/proto"
	"github.com/mr-tron/base58"
	"github.com/spf13/cobra"

	types "pkg.akt.dev/go/node/sink/v1"
)

func init() {
	Register((*types.QueryTransferResponse)(nil), PrettyFormatterFunc(formatSinkTransfer))
	Register((*types.QueryTransfersResponse)(nil), PrettyFormatterFunc(formatSinkTransfers))
	Register((*types.QueryDisputesResponse)(nil), PrettyFormatterFunc(formatSinkDisputes))
	Register((*types.QueryReportersResponse)(nil), PrettyFormatterFunc(formatSinkReporters))
	Register((*types.QueryAccountResponse)(nil), PrettyFormatterFunc(formatSinkAccount))
	Register((*types.QueryStateResponse)(nil), PrettyFormatterFunc(formatSinkState))
	Register((*types.Params)(nil), PrettyFormatterFunc(formatSinkParams))
}

// sinkTime renders a unix time in seconds; zero means unset.
func sinkTime(unix int64) string {
	if unix == 0 {
		return "-"
	}
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

// sinkStatusLabel renders a settlement status as a word.
func sinkStatusLabel(s types.Status) string {
	switch s {
	case types.StatusTransferPending:
		return StyleYellow.Render("Transfer pending")
	case types.StatusMinted:
		return StyleGreen.Render("Minted")
	case types.StatusVoided:
		return StyleRed.Render("Voided")
	case types.StatusRefundPending:
		return StyleYellow.Render("Refund pending")
	case types.StatusRefunded:
		return Dim("Refunded")
	default:
		return Dim(s.String())
	}
}

// sinkHoldLabel renders a settlement hold; any hold waits for governance.
func sinkHoldLabel(h types.Hold) string {
	switch h {
	case types.HoldNone:
		return "None"
	case types.HoldVeto:
		return StyleRed.Render("Veto")
	case types.HoldConflict:
		return StyleRed.Render("Conflict")
	default:
		return Dim(h.String())
	}
}

// sinkOutcomeLabel renders a reported Solana outcome.
func sinkOutcomeLabel(o types.Outcome) string {
	switch o {
	case types.OutcomeMinted:
		return "Minted"
	case types.OutcomeVoided:
		return "Voided"
	default:
		return Dim("Unspecified")
	}
}

// sinkSignature renders a Solana transaction signature in full base58.
func sinkSignature(signature []byte) string {
	if len(signature) == 0 {
		return "-"
	}
	return base58.Encode(signature)
}

// sinkObservation renders Solana evidence on one line.
func sinkObservation(o types.Observation) string {
	return fmt.Sprintf("%s at slot %s, signature %s", sinkOutcomeLabel(o.Outcome), strconv.FormatUint(o.Slot, 10), sinkSignature(o.TransactionSignature))
}

// sinkThreshold displays both the governed decimal ratio and its rounded quorum.
func sinkThreshold(threshold math.LegacyDec, members int) string {
	if threshold.IsNil() || threshold.IsNegative() || threshold.GT(math.LegacyOneDec()) {
		return "Invalid threshold"
	}
	required := threshold.MulInt64(int64(members)).Ceil().TruncateInt64()
	ratio := strings.TrimRight(strings.TrimRight(threshold.String(), "0"), ".")
	return fmt.Sprintf("%s (%d of %d)", ratio, required, members)
}

func validateSinkTransfer(state types.TransferState) error {
	if err := state.ID.ValidateBasic(); err != nil {
		return fmt.Errorf("sink query returned malformed node response: %w", err)
	}
	if err := state.Data.ValidateBasic(); err != nil {
		return fmt.Errorf("sink query returned malformed node response: %w", err)
	}
	if !state.Status.IsValid() {
		return fmt.Errorf("sink query returned malformed node response: invalid financial status")
	}
	return nil
}

// RenderSinkTransfer renders a transfer's record and settlement.
func RenderSinkTransfer(state types.TransferState) (string, error) {
	if err := validateSinkTransfer(state); err != nil {
		return "", err
	}

	var buf strings.Builder
	s := state.Settlement

	fmt.Fprintln(&buf, Section("Transfer"))
	KV(&buf, "ID", Bold(state.ID.String()))
	KV(&buf, "Sender", state.ID.Owner)
	KV(&buf, "Recipient", state.Data.Dest)
	KV(&buf, "Amount", FormatCoin(state.Data.Amount))
	KV(&buf, "Height", FormatHeight(state.ID.Height))
	if s.VoidAuthorization != nil {
		KV(&buf, "Void Ordered Height", FormatHeight(s.VoidAuthorization.RequestedHeight))
		KV(&buf, "Void Ordered Round", strconv.FormatUint(s.VoidAuthorization.RequestedRound, 10))
	}

	Newline(&buf)
	fmt.Fprintln(&buf, Section("Settlement"))
	minted, voided := s.Tallies()
	KV(&buf, "Status", sinkStatusLabel(state.Status))
	KV(&buf, "Hold", sinkHoldLabel(s.Hold))
	KV(&buf, "Round", strconv.FormatUint(s.Round, 10))
	KV(&buf, "Policy Version", strconv.FormatUint(s.Policy.Version, 10))
	KV(&buf, "Threshold", sinkThreshold(s.Policy.Threshold, len(s.Policy.Members)))
	KV(&buf, "Votes", fmt.Sprintf("%d minted, %d voided", minted, voided))
	KV(&buf, "Challenge Ends", sinkTime(s.ChallengeEnd))
	if s.VetoBy != "" {
		KV(&buf, "Vetoed By", s.VetoBy)
		KV(&buf, "Veto Reason", s.VetoReason)
		KV(&buf, "Veto Policy Version", strconv.FormatUint(s.VetoPolicyVersion, 10))
	}
	if s.GovernanceObservation != nil {
		KV(&buf, "Governance Evidence", sinkObservation(*s.GovernanceObservation))
	}

	Newline(&buf)
	fmt.Fprintln(&buf, Section("Votes"))
	rows := make([][]string, 0, len(s.Votes))
	for _, vote := range s.Votes {
		rows = append(rows, []string{
			vote.Reporter,
			strconv.FormatUint(vote.PolicyVersion, 10),
			sinkOutcomeLabel(vote.Observation.Outcome),
			strconv.FormatUint(vote.Observation.Slot, 10),
			sinkSignature(vote.Observation.TransactionSignature),
		})
	}
	WriteTableColsOrEmpty(&buf, []ColDef{
		{Header: "REPORTER"},
		{Header: "POLICY VERSION"},
		{Header: "OUTCOME"},
		{
			Header: "SLOT",
			Align:  AlignRight,
		},
		{Header: "SIGNATURE"},
	}, rows, "(no votes)")

	return buf.String(), nil
}

func formatSinkTransfer(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	transfer, err := RenderSinkTransfer(msg.(*types.QueryTransferResponse).Transfer)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(w, transfer)
	return err
}

// RenderSinkTransfers renders one native page without interpreting its cursor.
func RenderSinkTransfers(transfers []types.TransferState, page *sdkquery.PageResponse) (string, error) {
	var buf strings.Builder

	rows := make([][]string, 0, len(transfers))
	for _, state := range transfers {
		if err := validateSinkTransfer(state); err != nil {
			return "", err
		}

		s := state.Settlement
		rows = append(rows, []string{
			state.ID.String(),
			sinkStatusLabel(state.Status),
			sinkHoldLabel(s.Hold),
			FormatCoin(state.Data.Amount),
			state.Data.Dest,
			strconv.FormatUint(s.Round, 10),
			sinkTime(s.ChallengeEnd),
		})
	}

	WriteTableColsOrEmpty(&buf, []ColDef{
		{
			Header: "ID",
			Align:  AlignRight,
		},
		{Header: "STATUS"},
		{Header: "HOLD"},
		{
			Header: "AMOUNT",
			Align:  AlignRight,
		},
		{Header: "RECIPIENT"},
		{
			Header: "ROUND",
			Align:  AlignRight,
		},
		{Header: "CHALLENGE ENDS"},
	}, rows, "(no transfers)")

	if page != nil && len(page.NextKey) > 0 {
		Newline(&buf)
		KV(&buf, "Next page key", base64.StdEncoding.EncodeToString(page.NextKey))
		fmt.Fprintln(&buf, Dim("  Continue with --page-key; retain --height and --reverse for consistent paging."))
	}

	return buf.String(), nil
}

func formatSinkTransfers(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryTransfersResponse)
	table, err := RenderSinkTransfers(res.Transfers, res.Pagination)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(w, table)
	return err
}

func formatSinkDisputes(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryDisputesResponse)
	table, err := RenderSinkTransfers(res.Transfers, res.Pagination)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(w, table)
	return err
}

// RenderSinkReporters shows the membership and quorum of each returned policy.
func RenderSinkReporters(res *types.QueryReportersResponse) string {
	var buf strings.Builder
	if len(res.Reporters) == 0 {
		fmt.Fprintln(&buf, "(no reporter policy configured)")
	}
	for i, policy := range res.Reporters {
		if i > 0 {
			Newline(&buf)
		}
		fmt.Fprintln(&buf, Section("Reporter Policy"))
		KV(&buf, "Version", strconv.FormatUint(policy.Version, 10))
		KV(&buf, "Threshold", sinkThreshold(policy.Threshold, len(policy.Members)))
		writeSinkList(&buf, sinkParamsKeyWidth, "Members", policy.Members)
	}
	return buf.String()
}

func formatSinkReporters(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderSinkReporters(msg.(*types.QueryReportersResponse)))
	return err
}

// RenderSinkAccount renders an account's sink usage counters.
func RenderSinkAccount(res *types.QueryAccountResponse) string {
	var buf strings.Builder
	u := res.Usage

	fmt.Fprintln(&buf, Section("Sink Account"))
	KV(&buf, "Epoch", strconv.FormatUint(res.Epoch, 10))
	KV(&buf, "Total Sent", formatCoinSafe(u.TotalSent))
	KV(&buf, "Sent This Epoch", formatCoinSafe(u.EpochSent))

	return buf.String()
}

func formatSinkAccount(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderSinkAccount(msg.(*types.QueryAccountResponse)))
	return err
}

// sinkParamsKeyWidth fits the longest parameter label, so every value in the
// section shares one column.
const sinkParamsKeyWidth = 32

// RenderSinkParams renders the sink module parameters.
func RenderSinkParams(p types.Params) string {
	var buf strings.Builder

	kv := func(key, value string) { KVWidth(&buf, sinkParamsKeyWidth, key, value) }
	enabled := func(on bool) string {
		if on {
			return StyleGreen.Render("Enabled")
		}
		return "Disabled"
	}

	fmt.Fprintln(&buf, Section("Sink Parameters"))
	kv("Deposits", enabled(p.Enabled))
	kv("Refunds", enabled(p.RefundsEnabled))
	kv("Min Transfer", formatCoinSafe(p.MinAmountPerTransfer))
	kv("Max Transfer", formatCoinSafe(p.MaxAmountPerTransfer))
	kv("Send Cap Per Epoch", formatCoinSafe(p.MaxTransferAmountPerEpoch))
	if !p.MaxEpochAmountPerAccount.Amount.IsNil() && p.MaxEpochAmountPerAccount.IsZero() {
		kv("Account Cap Per Epoch", "No limit")
	} else {
		kv("Account Cap Per Epoch", formatCoinSafe(p.MaxEpochAmountPerAccount))
	}
	writeSinkList(&buf, sinkParamsKeyWidth, "Reporters", p.Reporters)
	kv("Reporter Threshold", sinkThreshold(p.ReportersThreshold, len(p.Reporters)))
	kv("Challenge Period", FormatDuration(p.ReporterChallengePeriod))
	kv("Refund Cap Per Epoch", formatCoinSafe(p.MaxEpochRefund))
	if p.PauseGuardian == "" {
		kv("Pause Guardian", "None")
	} else {
		kv("Pause Guardian", p.PauseGuardian)
	}
	writeSinkList(&buf, sinkParamsKeyWidth, "Vault Allowlist", p.AllowedDestinationVaults)

	return buf.String()
}

// writeSinkList writes one value per line, labelled on the first line only
// and aligned under the value column, or "None" when the list is empty.
func writeSinkList(w io.Writer, width int, label string, values []string) {
	if len(values) == 0 {
		KVWidth(w, width, label, "None")
		return
	}
	for i, value := range values {
		if i == 0 {
			KVWidth(w, width, label, value)
			continue
		}
		fmt.Fprintf(w, "  %s %s\n", padRight("", width), value)
	}
}

func formatSinkParams(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderSinkParams(*msg.(*types.Params)))
	return err
}

// RenderSinkState keeps each financial population separate from epoch usage.
func RenderSinkState(res *types.QueryStateResponse) (string, error) {
	t := res.Totals

	locked, err := t.EscrowAmount()
	if err != nil {
		return "", fmt.Errorf("sink state query returned malformed node response: %w", err)
	}

	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Sink Totals"))
	population := func(label string, amount string, count uint64) {
		KV(&buf, label, fmt.Sprintf("%s in %d transfers", amount, count))
	}
	population("Transfer Pending", FormatCoin(t.PendingAmount), t.PendingCount)
	population("Minted", FormatCoin(t.MintedAmount), t.MintedCount)
	population("Voided", FormatCoin(t.VoidedAmount), t.VoidedCount)
	population("Refund Pending", FormatCoin(t.RefundPendingAmount), t.RefundPendingCount)
	population("Refunded", formatCoinSafe(t.RefundedAmount), t.RefundedCount)
	KV(&buf, "Locked", Bold(FormatCoin(locked)))
	KV(&buf, "Epoch", strconv.FormatUint(res.Epoch, 10))
	KV(&buf, "Sent This Epoch", formatCoinSafe(res.SendUsage.Amount))
	KV(&buf, "Refunded This Epoch", formatCoinSafe(res.RefundUsage.Amount))
	if res.RefundHead == nil {
		KV(&buf, "Refund Candidate", "None")
	} else {
		KV(&buf, "Refund Candidate", res.RefundHead.String())
		fmt.Fprintln(&buf, Dim("  Payment still requires challenge maturity, authorization, enabled refunds and capacity."))
	}

	return buf.String(), nil
}

func formatSinkState(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	state, err := RenderSinkState(msg.(*types.QueryStateResponse))
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(w, state)
	return err
}

// SinkTransferCommand is the follow-up command for a deposited transfer.
func SinkTransferCommand(id string) string {
	if id == "" {
		id = "<owner/height/sequence>"
	}
	return "akt q sink transfer " + id
}

// RenderSinkDeposit renders the message detail of a sink deposit. The
// transaction only locks AKT; the mint happens later on Solana, so the
// success status alone would read as a completed transfer.
func RenderSinkDeposit(m *types.MsgSink, recordID string) string {
	var buf strings.Builder

	KV(&buf, "Sender", m.Sender)
	KV(&buf, "Destination", m.Dest)
	KV(&buf, "Amount", formatCoinSafe(m.Amount))
	if recordID != "" {
		KV(&buf, "Transfer ID", Bold(recordID))
	}

	Newline(&buf)
	fmt.Fprintln(&buf, Dim("  The AKT is locked on Akash. It is minted on Solana once a relayer"))
	fmt.Fprintln(&buf, Dim("  proves the record. Follow it with:"))
	fmt.Fprintf(&buf, "    %s\n", Bold(SinkTransferCommand(recordID)))

	return buf.String()
}
