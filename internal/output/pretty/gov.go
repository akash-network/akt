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

func formatProposalsList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*govv1.QueryProposalsResponse)

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

		status := formatProposalStatus(p.Status)

		rows = append(rows, []string{
			Bold(fmt.Sprintf("%d", p.Id)),
			truncateString(p.Title, 40),
			ColorState(status),
			submitTime,
			votingEnd,
		})
	}

	WriteTable(w, headers, rows)
	return nil
}

func formatProposalDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	p := msg.(*govv1.Proposal)

	fmt.Fprintln(w, Section("Proposal"))
	KV(w, "ID", Bold(fmt.Sprintf("%d", p.Id)))
	KV(w, "Title", Bold(p.Title))
	KV(w, "Status", ColorState(formatProposalStatus(p.Status)))
	KV(w, "Proposer", p.Proposer)

	if p.Summary != "" {
		KV(w, "Summary", p.Summary)
	}

	if p.Expedited {
		KV(w, "Expedited", "yes")
	}

	Newline(w)
	fmt.Fprintln(w, Section("Timeline"))
	if p.SubmitTime != nil {
		KV(w, "Submit Time", p.SubmitTime.Format("2006-01-02 15:04:05 UTC"))
	}
	if p.DepositEndTime != nil {
		KV(w, "Deposit End", p.DepositEndTime.Format("2006-01-02 15:04:05 UTC"))
	}
	if p.VotingStartTime != nil {
		KV(w, "Voting Start", p.VotingStartTime.Format("2006-01-02 15:04:05 UTC"))
	}
	if p.VotingEndTime != nil {
		KV(w, "Voting End", p.VotingEndTime.Format("2006-01-02 15:04:05 UTC"))
	}

	if len(p.TotalDeposit) > 0 {
		Newline(w)
		fmt.Fprintln(w, Section("Deposit"))
		for _, coin := range p.TotalDeposit {
			KV(w, "Total Deposit", Bold(FormatCoin(coin)))
		}
	}

	if p.FinalTallyResult != nil {
		Newline(w)
		fmt.Fprintln(w, Section("Tally"))
		KV(w, "Yes", p.FinalTallyResult.YesCount)
		KV(w, "No", p.FinalTallyResult.NoCount)
		KV(w, "Abstain", p.FinalTallyResult.AbstainCount)
		KV(w, "No With Veto", p.FinalTallyResult.NoWithVetoCount)
	}

	if p.FailedReason != "" {
		Newline(w)
		KV(w, "Failed Reason", p.FailedReason)
	}

	return nil
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
