package pretty

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
)

func TestRenderProposalListShowsTallyPercentages(t *testing.T) {
	res := &govv1.QueryProposalsResponse{Proposals: []*govv1.Proposal{
		{
			Id:       1,
			Title:    "Active proposal",
			Status:   govv1.StatusVotingPeriod,
			Messages: []*codectypes.Any{{}, {}},
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
	for _, want := range []string{"MESSAGES", "YES", "NO", "ABSTAIN", "VETO", "70%", "20%", "5%"} {
		if !strings.Contains(got, want) {
			t.Errorf("proposal list does not contain %q:\n%s", want, got)
		}
	}
	var activeLine string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "Active proposal") {
			activeLine = line
			break
		}
	}
	if activeLine == "" || !strings.Contains(activeLine, "2") {
		t.Errorf("proposal list does not show two executable messages:\n%s", got)
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

func TestFormatProposalDetailShowsCompleteExecutableMessages(t *testing.T) {
	encoding := aktcodec.MakeEncodingConfig()
	fromAddress := sdk.AccAddress(bytes.Repeat([]byte{1}, 20))
	toAddress := sdk.AccAddress(bytes.Repeat([]byte{2}, 20))
	from := fromAddress.String()
	to := toAddress.String()
	authority := sdk.AccAddress(bytes.Repeat([]byte{3}, 20)).String()

	send, err := codectypes.NewAnyWithValue(banktypes.NewMsgSend(
		fromAddress,
		toAddress,
		sdk.NewCoins(sdk.NewInt64Coin("uakt", 1234567)),
	))
	require.NoError(t, err)
	send = &codectypes.Any{TypeUrl: send.TypeUrl, Value: append([]byte(nil), send.Value...)}

	legacyContent, err := codectypes.NewAnyWithValue(
		&govv1beta1.TextProposal{Title: "Legacy policy", Description: "exact legacy description"},
	)
	require.NoError(t, err)
	legacy, err := codectypes.NewAnyWithValue(
		govv1.NewMsgExecLegacyContent(legacyContent, authority),
	)
	require.NoError(t, err)
	legacy = &codectypes.Any{TypeUrl: legacy.TypeUrl, Value: append([]byte(nil), legacy.Value...)}

	unknownValue := []byte{0xde, 0xad, 0xbe, 0xef}
	unknown := &codectypes.Any{
		TypeUrl: "/future.gov.v1.MsgChangePolicy",
		Value:   unknownValue,
	}

	proposal := &govv1.Proposal{
		Id:        42,
		Title:     "Inspect every action",
		Summary:   "The prose must match the executable messages.",
		Metadata:  "ipfs://bafybeigovernance",
		Proposer:  from,
		Messages:  []*codectypes.Any{send, legacy, unknown, nil},
		Expedited: false,
	}

	var output strings.Builder
	err = formatProposalDetail(
		&output,
		&cobra.Command{},
		sdkclient.Context{Codec: encoding.Codec, InterfaceRegistry: encoding.InterfaceRegistry},
		proposal,
	)
	require.NoError(t, err)
	got := ansi.Strip(output.String())

	for _, want := range []string{
		"ipfs://bafybeigovernance",
		"Expedited",
		"no",
		"Message 1",
		"/cosmos.bank.v1beta1.MsgSend",
		from,
		to,
		`"amount": "1234567"`,
		"Message 2",
		"/cosmos.gov.v1.MsgExecLegacyContent",
		"/cosmos.gov.v1beta1.TextProposal",
		"exact legacy description",
		"Message 3",
		unknown.TypeUrl,
		base64.StdEncoding.EncodeToString(unknownValue),
		"Message 4",
		"message is absent",
	} {
		require.Contains(t, got, want)
	}

	previous := -1
	for _, typeURL := range []string{send.TypeUrl, legacy.TypeUrl, unknown.TypeUrl} {
		position := strings.Index(got, typeURL)
		require.Greater(t, position, previous, "message %s rendered out of order", typeURL)
		previous = position
	}
}

func TestFormatProposalDetailShowsTallyCountsAndPercentages(t *testing.T) {
	proposal := &govv1.Proposal{FinalTallyResult: &govv1.TallyResult{
		YesCount:        "70",
		NoCount:         "20",
		AbstainCount:    "5",
		NoWithVetoCount: "5",
	}}

	var output strings.Builder
	require.NoError(t, formatProposalDetail(&output, &cobra.Command{}, sdkclient.Context{}, proposal))
	got := ansi.Strip(output.String())
	for _, want := range []string{"70 (70%)", "20 (20%)", "5 (5%)"} {
		require.Contains(t, got, want)
	}
}

func TestFormatProposalDetailPreservesCountsWithoutValidPercentage(t *testing.T) {
	proposal := &govv1.Proposal{FinalTallyResult: &govv1.TallyResult{
		YesCount:        "not-a-number",
		NoCount:         "-1",
		AbstainCount:    "2",
		NoWithVetoCount: "3",
	}}

	var output strings.Builder
	require.NoError(t, formatProposalDetail(&output, &cobra.Command{}, sdkclient.Context{}, proposal))
	got := ansi.Strip(output.String())
	for _, want := range []string{"not-a-number", "-1", "2", "3"} {
		require.Contains(t, got, want)
	}
	require.NotContains(t, got, "%")
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
	encoding := aktcodec.MakeEncodingConfig()
	cctx := sdkclient.Context{Codec: encoding.Codec, InterfaceRegistry: encoding.InterfaceRegistry}
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
			golden.RequireEqual(t, RenderProposalDetail(cctx, tc.p))
		})
	}
}
