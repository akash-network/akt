package pretty

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

func TestRenderProposalListShowsTallyPercentages(t *testing.T) {
	res := &govv1.QueryProposalsResponse{Proposals: []*govv1.Proposal{
		{
			Id:     1,
			Title:  "Active proposal",
			Status: govv1.StatusVotingPeriod,
			FinalTallyResult: &govv1.TallyResult{
				YesCount:        "70",
				NoCount:         "20",
				AbstainCount:    "5",
				NoWithVetoCount: "5",
			},
		},
		{
			Id:     2,
			Title:  "Awaiting votes",
			Status: govv1.StatusDepositPeriod,
		},
	}}

	got := RenderProposalList(res)
	for _, want := range []string{"YES", "NO", "ABSTAIN", "VETO", "70%", "20%", "5%"} {
		if !strings.Contains(got, want) {
			t.Errorf("proposal list does not contain %q:\n%s", want, got)
		}
	}
	var awaitingLine string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "Awaiting votes") {
			awaitingLine = line
			break
		}
	}
	if awaitingLine == "" || strings.Count(awaitingLine, "-") < 5 {
		t.Errorf("proposal without a tally should render four tally placeholders and its missing deadline:\n%s", got)
	}
}

func TestRenderProposalList(t *testing.T) {
	submitTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	votingStart := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	votingEnd := time.Date(2026, 1, 9, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		res *govv1.QueryProposalsResponse
	}{
		"Empty": {
			res: &govv1.QueryProposalsResponse{
				Proposals: nil,
			},
		},
		"WithProposals": {
			res: &govv1.QueryProposalsResponse{
				Proposals: []*govv1.Proposal{
					{
						Id:              1,
						Title:           "Enable IBC Transfers",
						Status:          govv1.StatusVotingPeriod,
						SubmitTime:      &submitTime,
						VotingStartTime: &votingStart,
						VotingEndTime:   &votingEnd,
					},
					{
						Id:              2,
						Title:           "Community Pool Spend",
						Status:          govv1.StatusPassed,
						SubmitTime:      &submitTime,
						VotingStartTime: &votingStart,
						VotingEndTime:   &votingEnd,
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderProposalList(tc.res))
		})
	}
}

func TestRenderProposalDetail(t *testing.T) {
	submitTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	depositEnd := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	votingStart := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	votingEnd := time.Date(2026, 1, 9, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		p *govv1.Proposal
	}{
		"VotingPeriod": {
			p: &govv1.Proposal{
				Id:              1,
				Title:           "Enable IBC Transfers",
				Summary:         "Proposal to enable IBC transfers on the network.",
				Proposer:        "akash1proposer123",
				Status:          govv1.StatusVotingPeriod,
				SubmitTime:      &submitTime,
				DepositEndTime:  &depositEnd,
				VotingStartTime: &votingStart,
				VotingEndTime:   &votingEnd,
				TotalDeposit:    sdk.NewCoins(sdk.NewInt64Coin("uakt", 50000000000)),
				FinalTallyResult: &govv1.TallyResult{
					YesCount:        "1000000",
					NoCount:         "200000",
					AbstainCount:    "50000",
					NoWithVetoCount: "10000",
				},
			},
		},
		"Passed": {
			p: &govv1.Proposal{
				Id:              2,
				Title:           "Community Pool Spend",
				Proposer:        "akash1proposer456",
				Status:          govv1.StatusPassed,
				SubmitTime:      &submitTime,
				DepositEndTime:  &depositEnd,
				VotingStartTime: &votingStart,
				VotingEndTime:   &votingEnd,
				TotalDeposit:    sdk.NewCoins(sdk.NewInt64Coin("uakt", 50000000000)),
				FinalTallyResult: &govv1.TallyResult{
					YesCount:        "5000000",
					NoCount:         "100000",
					AbstainCount:    "20000",
					NoWithVetoCount: "5000",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderProposalDetail(tc.p))
		})
	}
}
