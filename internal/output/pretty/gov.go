package pretty

import (
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

func init() {
	Register((*govv1.QueryProposalsResponse)(nil), PrettyFormatterFunc(formatProposalsList))
	Register((*govv1.Proposal)(nil), PrettyFormatterFunc(formatProposalDetail))
}

// formatProposalStatus strips the "PROPOSAL_STATUS_" prefix and lowercases the result.
func formatProposalStatus(status govv1.ProposalStatus) string {
	s := status.String()
	s = strings.TrimPrefix(s, "PROPOSAL_STATUS_")
	return strings.ToLower(s)
}

// RenderProposalList renders a proposals list as a styled string.
func RenderProposalList(res *govv1.QueryProposalsResponse) string {
	var buf strings.Builder
	headers := []string{"ID", "TITLE", "MESSAGES", "STATUS", "YES", "NO", "ABSTAIN", "VETO", "VOTING END"}
	rows := make([][]string, 0, len(res.Proposals))
	for _, p := range res.Proposals {
		votingEnd := "-"
		if p.VotingEndTime != nil {
			votingEnd = p.VotingEndTime.Format("2006-01-02 15:04")
		}
		yes, no, abstain, veto := formatTallyPercentages(p.FinalTallyResult)
		rows = append(rows, []string{
			Bold(fmt.Sprintf("%d", p.Id)),
			truncateString(p.Title, 40),
			fmt.Sprintf("%d", len(p.Messages)),
			ColorState(formatProposalStatus(p.Status)),
			yes, no, abstain, veto,
			votingEnd,
		})
	}
	WriteTableOrEmpty(&buf, headers, rows, "(no proposals)")
	return buf.String()
}

func formatTallyPercentages(tally *govv1.TallyResult) (string, string, string, string) {
	if tally == nil {
		return "-", "-", "-", "-"
	}

	counts := []*big.Int{new(big.Int), new(big.Int), new(big.Int), new(big.Int)}
	values := []string{tally.YesCount, tally.NoCount, tally.AbstainCount, tally.NoWithVetoCount}
	total := new(big.Int)
	for i, value := range values {
		if _, ok := counts[i].SetString(value, 10); !ok || counts[i].Sign() < 0 {
			return "-", "-", "-", "-"
		}
		total.Add(total, counts[i])
	}
	if total.Sign() == 0 {
		return "-", "-", "-", "-"
	}

	percent := func(count *big.Int) string {
		ratio := new(big.Rat).SetFrac(count, total)
		ratio.Mul(ratio, big.NewRat(100, 1))
		return strings.TrimSuffix(ratio.FloatString(1), ".0") + "%"
	}

	return percent(counts[0]), percent(counts[1]), percent(counts[2]), percent(counts[3])
}

func formatProposalsList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderProposalList(msg.(*govv1.QueryProposalsResponse)))
	return err
}

// RenderProposalDetail renders a proposal detail as a styled string.
func RenderProposalDetail(cctx sdkclient.Context, p *govv1.Proposal) string {
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Proposal"))
	KV(&buf, "ID", Bold(fmt.Sprintf("%d", p.Id)))
	KV(&buf, "Title", Bold(p.Title))
	KV(&buf, "Status", ColorState(formatProposalStatus(p.Status)))
	KV(&buf, "Proposer", p.Proposer)
	if p.Summary != "" {
		KV(&buf, "Summary", p.Summary)
	}
	if p.Metadata != "" {
		KV(&buf, "Metadata", p.Metadata)
	}
	expedited := "no"
	if p.Expedited {
		expedited = "yes"
	}
	KV(&buf, "Expedited", expedited)

	Newline(&buf)
	fmt.Fprintln(&buf, Section("Messages"))
	if len(p.Messages) == 0 {
		fmt.Fprintln(&buf, Dim("(no executable messages)"))
	}
	for i, packed := range p.Messages {
		if i > 0 {
			Newline(&buf)
		}
		fmt.Fprintln(&buf, Section(fmt.Sprintf("Message %d", i+1)))
		if packed == nil {
			KV(&buf, "Status", Dim("message is absent"))
			continue
		}

		KV(&buf, "Type", packed.TypeUrl)
		message, ok := unpackTxMessage(cctx, packed)
		if ok && cctx.Codec != nil {
			payload, err := cctx.Codec.MarshalJSON(message)
			if err == nil {
				_ = WriteHighlightedJSON(&buf, payload)
				continue
			}
		}

		KV(&buf, "Raw Value (base64)", base64.StdEncoding.EncodeToString(packed.Value))
	}

	Newline(&buf)
	fmt.Fprintln(&buf, Section("Timeline"))
	if p.SubmitTime != nil {
		KV(&buf, "Submit Time", p.SubmitTime.Format("2006-01-02 15:04:05 UTC"))
	}
	if p.DepositEndTime != nil {
		KV(&buf, "Deposit End", p.DepositEndTime.Format("2006-01-02 15:04:05 UTC"))
	}
	if p.VotingStartTime != nil {
		KV(&buf, "Voting Start", p.VotingStartTime.Format("2006-01-02 15:04:05 UTC"))
	}
	if p.VotingEndTime != nil {
		KV(&buf, "Voting End", p.VotingEndTime.Format("2006-01-02 15:04:05 UTC"))
	}
	if len(p.TotalDeposit) > 0 {
		Newline(&buf)
		fmt.Fprintln(&buf, Section("Deposit"))
		for _, coin := range p.TotalDeposit {
			KV(&buf, "Total Deposit", Bold(FormatCoin(coin)))
		}
	}
	if p.FinalTallyResult != nil {
		Newline(&buf)
		fmt.Fprintln(&buf, Section("Tally"))
		yes, no, abstain, veto := formatTallyPercentages(p.FinalTallyResult)
		KV(&buf, "Yes", formatTallyCount(p.FinalTallyResult.YesCount, yes))
		KV(&buf, "No", formatTallyCount(p.FinalTallyResult.NoCount, no))
		KV(&buf, "Abstain", formatTallyCount(p.FinalTallyResult.AbstainCount, abstain))
		KV(&buf, "No With Veto", formatTallyCount(p.FinalTallyResult.NoWithVetoCount, veto))
	}
	if p.FailedReason != "" {
		Newline(&buf)
		KV(&buf, "Failed Reason", p.FailedReason)
	}
	return buf.String()
}

func formatProposalDetail(w io.Writer, _ *cobra.Command, cctx sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderProposalDetail(cctx, msg.(*govv1.Proposal)))
	return err
}

func formatTallyCount(count, percentage string) string {
	if percentage == "-" {
		return count
	}
	return fmt.Sprintf("%s (%s)", count, percentage)
}

// truncateString truncates a string to maxLen characters, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
