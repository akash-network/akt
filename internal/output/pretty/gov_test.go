package pretty

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

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
