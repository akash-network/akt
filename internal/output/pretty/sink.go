package pretty

import (
	"fmt"
	"io"
	stdmath "math"
	"strconv"
	"strings"
	"time"

	"cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/mr-tron/base58"
	"github.com/spf13/cobra"

	types "pkg.akt.dev/go/node/sink/v1"
)

func init() {
	Register((*types.QuerySinkResponse)(nil), PrettyFormatterFunc(formatSinkRecord))
	Register((*types.QueryTransferResponse)(nil), PrettyFormatterFunc(formatSinkTransfer))
	Register((*types.QueryOpenTransfersResponse)(nil), PrettyFormatterFunc(formatSinkOpenTransfers))
	Register((*types.QueryAccountResponse)(nil), PrettyFormatterFunc(formatSinkAccount))
	Register((*types.QueryStateResponse)(nil), PrettyFormatterFunc(formatSinkState))
	Register((*types.Params)(nil), PrettyFormatterFunc(formatSinkParams))
}

// sinkCoin renders a sink uakt amount, which the module keeps as a uint64.
func sinkCoin(amount uint64) string {
	return FormatCoin(sdk.NewCoin(types.Denom, math.NewIntFromUint64(amount)))
}

// sinkTime renders a unix time in seconds; zero means unset.
func sinkTime(unix int64) string {
	if unix == 0 {
		return "-"
	}
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

// sinkDay renders a sink usage day: days since the unix epoch in UTC.
func sinkDay(day uint64) string {
	if day == 0 || day > uint64(stdmath.MaxInt64/86400) {
		return "-"
	}
	return time.Unix(int64(day)*86400, 0).UTC().Format(time.DateOnly)
}

// sinkPublicStatusLabel renders the module's public transfer status.
func sinkPublicStatusLabel(s types.QuerySinkResponse_SinkStatus) string {
	switch s {
	case types.QuerySinkResponse_transfer_pending:
		return StyleYellow.Render("Transfer pending")
	case types.QuerySinkResponse_completed:
		return StyleGreen.Render("Completed")
	case types.QuerySinkResponse_void:
		return StyleRed.Render("Void")
	case types.QuerySinkResponse_validation_pending:
		return Dim("Validation pending")
	case types.QuerySinkResponse_validated:
		return Dim("Validated")
	default:
		return Dim("Invalid")
	}
}

// sinkStatusLabel renders a settlement status as a word.
func sinkStatusLabel(s types.Status) string {
	switch s {
	case types.Status_STATUS_PENDING:
		return StyleYellow.Render("Pending")
	case types.Status_STATUS_MINTED:
		return StyleGreen.Render("Minted")
	case types.Status_STATUS_VOIDED:
		return StyleRed.Render("Voided")
	case types.Status_STATUS_REFUNDED:
		return Dim("Refunded")
	default:
		return Dim(s.String())
	}
}

// sinkHoldLabel renders a settlement hold; any hold waits for governance.
func sinkHoldLabel(h types.Hold) string {
	switch h {
	case types.Hold_HOLD_NONE:
		return "None"
	case types.Hold_HOLD_VETO:
		return StyleRed.Render("Veto")
	case types.Hold_HOLD_CONFLICT:
		return StyleRed.Render("Conflict")
	default:
		return Dim(h.String())
	}
}

// sinkOutcomeLabel renders a reported Solana outcome.
func sinkOutcomeLabel(o types.Outcome) string {
	switch o {
	case types.Outcome_OUTCOME_MINTED:
		return "Minted"
	case types.Outcome_OUTCOME_VOIDED:
		return "Voided"
	default:
		return Dim("Unspecified")
	}
}

// sinkDecisionLabel renders a governance resolution decision.
func sinkDecisionLabel(d types.Decision) string {
	switch d {
	case types.Decision_DECISION_RESTART:
		return "Restart"
	case types.Decision_DECISION_CONFIRM_MINT:
		return "Confirm mint"
	case types.Decision_DECISION_CONFIRM_VOID:
		return "Confirm void"
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

// sinkThreshold renders a reporter threshold against its member count.
func sinkThreshold(threshold uint32, members int) string {
	return fmt.Sprintf("%d of %d", threshold, members)
}

// decodeSinkRecord decodes a transfer's 77-byte record. The record is node
// input, so a value that does not decode is a malformed response.
func decodeSinkRecord(state types.TransferState) (types.Transfer, error) {
	transfer, err := types.DecodeTransfer(state.Record)
	if err != nil {
		return types.Transfer{}, fmt.Errorf("sink query returned malformed node response: transfer record does not decode: %w", err)
	}
	return transfer, nil
}

// RenderSinkTransfer renders a transfer's record and settlement.
func RenderSinkTransfer(state types.TransferState) (string, error) {
	transfer, err := decodeSinkRecord(state)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	s := state.Settlement

	fmt.Fprintln(&buf, Section("Transfer"))
	KV(&buf, "ID", Bold(strconv.FormatUint(transfer.ID, 10)))
	KV(&buf, "Sender", sdk.AccAddress(transfer.Sender[:]).String())
	KV(&buf, "Recipient", base58.Encode(transfer.Recipient[:]))
	KV(&buf, "Amount", sinkCoin(transfer.Amount))
	KV(&buf, "Height", FormatHeight(int64(transfer.Height)))
	KV(&buf, "Void Ordered", FormatBool(state.VoidOrdered))

	Newline(&buf)
	fmt.Fprintln(&buf, Section("Settlement"))
	minted, voided := s.Tallies()
	KV(&buf, "Status", sinkStatusLabel(s.Status))
	KV(&buf, "Hold", sinkHoldLabel(s.Hold))
	KV(&buf, "Round", strconv.FormatUint(s.Round, 10))
	KV(&buf, "Reporter Epoch", strconv.FormatUint(s.Policy.Epoch, 10))
	KV(&buf, "Threshold", sinkThreshold(s.Policy.Threshold, len(s.Policy.Members)))
	KV(&buf, "Votes", fmt.Sprintf("%d minted, %d voided", minted, voided))
	KV(&buf, "Challenge Ends", sinkTime(s.ChallengeEnd))
	if s.VetoBy != "" {
		KV(&buf, "Vetoed By", s.VetoBy)
		KV(&buf, "Veto Reason", s.VetoReason)
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
			sinkOutcomeLabel(vote.Observation.Outcome),
			strconv.FormatUint(vote.Observation.Slot, 10),
			sinkSignature(vote.Observation.TransactionSignature),
		})
	}
	WriteTableColsOrEmpty(&buf, []ColDef{
		{Header: "REPORTER"},
		{Header: "OUTCOME"},
		{Header: "SLOT", Align: AlignRight},
		{Header: "SIGNATURE"},
	}, rows, "(no votes)")

	return buf.String(), nil
}

func formatSinkRecord(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QuerySinkResponse)

	transfer, err := RenderSinkTransfer(res.Transfer)
	if err != nil {
		return err
	}

	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Sink Transfer"))
	KV(&buf, "Status", sinkPublicStatusLabel(res.Status))
	Newline(&buf)
	buf.WriteString(transfer)

	_, err = fmt.Fprint(w, buf.String())
	return err
}

func formatSinkTransfer(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	transfer, err := RenderSinkTransfer(msg.(*types.QueryTransferResponse).Transfer)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(w, transfer)
	return err
}

// RenderSinkOpenTransfers renders a page of open transfers as a table.
func RenderSinkOpenTransfers(res *types.QueryOpenTransfersResponse) (string, error) {
	var buf strings.Builder

	rows := make([][]string, 0, len(res.Transfers))
	for _, state := range res.Transfers {
		transfer, err := decodeSinkRecord(state)
		if err != nil {
			return "", err
		}

		s := state.Settlement
		rows = append(rows, []string{
			strconv.FormatUint(transfer.ID, 10),
			sinkStatusLabel(s.Status),
			sinkHoldLabel(s.Hold),
			sinkCoin(transfer.Amount),
			sdk.AccAddress(transfer.Sender[:]).String(),
			base58.Encode(transfer.Recipient[:]),
			strconv.FormatUint(s.Round, 10),
			sinkTime(s.ChallengeEnd),
		})
	}

	WriteTableColsOrEmpty(&buf, []ColDef{
		{Header: "ID", Align: AlignRight},
		{Header: "STATUS"},
		{Header: "HOLD"},
		{Header: "AMOUNT", Align: AlignRight},
		{Header: "SENDER"},
		{Header: "RECIPIENT"},
		{Header: "ROUND", Align: AlignRight},
		{Header: "CHALLENGE ENDS"},
	}, rows, "(no open transfers)")

	if res.NextStartAfter != 0 {
		Newline(&buf)
		fmt.Fprintf(&buf, "%s %s\n", Dim("More transfers:"), Bold(fmt.Sprintf("akt q sink open-transfers --start-after %d", res.NextStartAfter)))
	}

	return buf.String(), nil
}

func formatSinkOpenTransfers(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	table, err := RenderSinkOpenTransfers(msg.(*types.QueryOpenTransfersResponse))
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(w, table)
	return err
}

// RenderSinkAccount renders an account's sink usage counters.
func RenderSinkAccount(res *types.QueryAccountResponse) string {
	var buf strings.Builder
	u := res.Usage

	fmt.Fprintln(&buf, Section("Sink Account"))
	if len(u.Sender) == 0 {
		KV(&buf, "Sender", "-")
	} else {
		KV(&buf, "Sender", sdk.AccAddress(u.Sender).String())
	}
	KV(&buf, "Day", sinkDay(u.Day))
	KV(&buf, "Deposits That Day", strconv.FormatUint(uint64(u.DailyCount), 10))
	KV(&buf, "Amount That Day", sinkCoin(u.DailyAmount))
	KV(&buf, "Pending Transfers", strconv.FormatUint(uint64(u.PendingCount), 10))

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
	if p.MinTransfer == 0 {
		kv("Min Transfer", "None")
	} else {
		kv("Min Transfer", sinkCoin(p.MinTransfer))
	}
	kv("Max Transfer", sinkCoin(p.MaxTransfer))
	kv("Solana Daily Mint Cap", sinkCoin(p.SolanaDailyMintCap))
	if p.MaxDailyAmountPerAccount == 0 {
		kv("Max Amount Per Account Per Day", "No limit")
	} else {
		kv("Max Amount Per Account Per Day", sinkCoin(p.MaxDailyAmountPerAccount))
	}
	kv("Deposits Per Account Per Day", strconv.FormatUint(uint64(p.MaxDailyPerAccount), 10))
	kv("Pending Per Account", strconv.FormatUint(uint64(p.MaxPendingPerAccount), 10))
	writeSinkList(&buf, sinkParamsKeyWidth, "Reporters", p.Reporters)
	kv("Reporter Threshold", sinkThreshold(p.ReporterThreshold, len(p.Reporters)))
	if p.ChallengePeriodSeconds > uint64(stdmath.MaxInt64/int64(time.Second)) {
		kv("Challenge Period", strconv.FormatUint(p.ChallengePeriodSeconds, 10)+"s")
	} else {
		kv("Challenge Period", FormatDuration(time.Duration(p.ChallengePeriodSeconds)*time.Second))
	}
	kv("Daily Refund Cap", sinkCoin(p.DailyRefundCap))
	if p.PauseGuardian == "" {
		kv("Pause Guardian", "None")
	} else {
		kv("Pause Guardian", p.PauseGuardian)
	}
	vaults := make([]string, 0, len(p.VaultAllowlist))
	for _, vault := range p.VaultAllowlist {
		vaults = append(vaults, base58.Encode(vault))
	}
	writeSinkList(&buf, sinkParamsKeyWidth, "Vault Allowlist", vaults)

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

// RenderSinkState renders the module totals followed by its parameters.
func RenderSinkState(res *types.QueryStateResponse) (string, error) {
	t := res.Totals

	locked, err := t.EscrowAmount()
	if err != nil {
		return "", fmt.Errorf("sink state query returned malformed node response: %w", err)
	}

	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Sink Totals"))
	KV(&buf, "Last Transfer ID", strconv.FormatUint(t.LastTransferId, 10))
	KV(&buf, "Launch Reserve", sinkCoin(t.LaunchReserve))
	KV(&buf, "Pending", fmt.Sprintf("%s in %d transfers", sinkCoin(t.PendingAmount), t.PendingCount))
	KV(&buf, "Minted", sinkCoin(t.MintedAmount))
	KV(&buf, "Voided", sinkCoin(t.VoidedAmount))
	KV(&buf, "Refunded", sinkCoin(t.RefundedAmount))
	if t.RefundDay == 0 {
		KV(&buf, "Refunded Today", "-")
	} else {
		KV(&buf, "Refunded Today", fmt.Sprintf("%s on %s", sinkCoin(t.RefundedToday), sinkDay(t.RefundDay)))
	}
	KV(&buf, "Locked", Bold(sinkCoin(locked)))
	KV(&buf, "Reporter Epoch", strconv.FormatUint(t.ReporterEpoch, 10))

	Newline(&buf)
	buf.WriteString(RenderSinkParams(res.Params))

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

// SinkRecordCommand is the follow-up command for a deposited transfer.
func SinkRecordCommand(id string) string {
	if id == "" {
		id = "<id>"
	}
	return "akt q sink record " + id
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
		KV(&buf, "Record ID", Bold(recordID))
	}

	Newline(&buf)
	fmt.Fprintln(&buf, Dim("  The AKT is locked on Akash. It is minted on Solana once a relayer"))
	fmt.Fprintln(&buf, Dim("  proves the record, usually within a few minutes. Follow it with:"))
	fmt.Fprintf(&buf, "    %s\n", Bold(SinkRecordCommand(recordID)))

	return buf.String()
}
