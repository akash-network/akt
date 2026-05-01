package pretty

import (
	"fmt"
	"io"
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
	headers := []string{"ID", "TITLE", "STATUS", "SUBMIT TIME", "VOTING END"}
	rows := make([][]string, 0, len(res.Proposals))
	for _, p := range res.Proposals {
		submitTime := "-"
		if p.SubmitTime != nil {
			submitTime = p.SubmitTime.Format("2006-01-02 15:04")
		}
		votingEnd := "-"
		if p.VotingEndTime != nil {
			votingEnd = p.VotingEndTime.Format("2006-01-02 15:04")
		}
		rows = append(rows, []string{
			Bold(fmt.Sprintf("%d", p.Id)),
			truncateString(p.Title, 40),
			ColorState(formatProposalStatus(p.Status)),
			submitTime, votingEnd,
		})
	}
	WriteTable(&buf, headers, rows)
	return buf.String()
}

func formatProposalsList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderProposalList(msg.(*govv1.QueryProposalsResponse)))
	return err
}

// RenderProposalDetail renders a proposal detail as a styled string.
func RenderProposalDetail(p *govv1.Proposal) string {
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Proposal"))
	KV(&buf, "ID", Bold(fmt.Sprintf("%d", p.Id)))
	KV(&buf, "Title", Bold(p.Title))
	KV(&buf, "Status", ColorState(formatProposalStatus(p.Status)))
	KV(&buf, "Proposer", p.Proposer)
	if p.Summary != "" {
		KV(&buf, "Summary", p.Summary)
	}
	if p.Expedited {
		KV(&buf, "Expedited", "yes")
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
		KV(&buf, "Yes", p.FinalTallyResult.YesCount)
		KV(&buf, "No", p.FinalTallyResult.NoCount)
		KV(&buf, "Abstain", p.FinalTallyResult.AbstainCount)
		KV(&buf, "No With Veto", p.FinalTallyResult.NoWithVetoCount)
	}
	if p.FailedReason != "" {
		Newline(&buf)
		KV(&buf, "Failed Reason", p.FailedReason)
	}
	return buf.String()
}

func formatProposalDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderProposalDetail(msg.(*govv1.Proposal)))
	return err
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
