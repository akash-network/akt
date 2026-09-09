package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type govQueryRecorder struct {
	govv1.QueryClient

	proposalRequests  []*govv1.QueryProposalRequest
	proposalResponse  *govv1.QueryProposalResponse
	proposalErr       error
	proposalsRequests []*govv1.QueryProposalsRequest
	proposalsResponse *govv1.QueryProposalsResponse
	proposalsErr      error
	voteRequests      []*govv1.QueryVoteRequest
	voteResponse      *govv1.QueryVoteResponse
	voteErr           error
	votesRequests     []*govv1.QueryVotesRequest
	votesResponse     *govv1.QueryVotesResponse
	votesErr          error
	depositRequests   []*govv1.QueryDepositRequest
	depositResponse   *govv1.QueryDepositResponse
	depositErr        error
	depositsRequests  []*govv1.QueryDepositsRequest
	depositsResponse  *govv1.QueryDepositsResponse
	depositsErr       error
	tallyRequests     []*govv1.QueryTallyResultRequest
	tallyResponse     *govv1.QueryTallyResultResponse
	tallyErr          error
	paramsRequests    []*govv1.QueryParamsRequest
	paramsResponse    *govv1.QueryParamsResponse
	paramsErr         error
	calls             []string
}

func (recorder *govQueryRecorder) Proposal(
	_ context.Context,
	request *govv1.QueryProposalRequest,
	_ ...grpc.CallOption,
) (*govv1.QueryProposalResponse, error) {
	recorder.calls = append(recorder.calls, "proposal")
	recorder.proposalRequests = append(recorder.proposalRequests, request)
	return recorder.proposalResponse, recorder.proposalErr
}

func (recorder *govQueryRecorder) Proposals(
	_ context.Context,
	request *govv1.QueryProposalsRequest,
	_ ...grpc.CallOption,
) (*govv1.QueryProposalsResponse, error) {
	recorder.calls = append(recorder.calls, "proposals")
	recorder.proposalsRequests = append(recorder.proposalsRequests, request)
	return recorder.proposalsResponse, recorder.proposalsErr
}

func (recorder *govQueryRecorder) Vote(
	_ context.Context,
	request *govv1.QueryVoteRequest,
	_ ...grpc.CallOption,
) (*govv1.QueryVoteResponse, error) {
	recorder.calls = append(recorder.calls, "vote")
	recorder.voteRequests = append(recorder.voteRequests, request)
	return recorder.voteResponse, recorder.voteErr
}

func (recorder *govQueryRecorder) Votes(
	_ context.Context,
	request *govv1.QueryVotesRequest,
	_ ...grpc.CallOption,
) (*govv1.QueryVotesResponse, error) {
	recorder.calls = append(recorder.calls, "votes")
	recorder.votesRequests = append(recorder.votesRequests, request)
	return recorder.votesResponse, recorder.votesErr
}

func (recorder *govQueryRecorder) Deposit(
	_ context.Context,
	request *govv1.QueryDepositRequest,
	_ ...grpc.CallOption,
) (*govv1.QueryDepositResponse, error) {
	recorder.calls = append(recorder.calls, "deposit")
	recorder.depositRequests = append(recorder.depositRequests, request)
	return recorder.depositResponse, recorder.depositErr
}

func (recorder *govQueryRecorder) Deposits(
	_ context.Context,
	request *govv1.QueryDepositsRequest,
	_ ...grpc.CallOption,
) (*govv1.QueryDepositsResponse, error) {
	recorder.calls = append(recorder.calls, "deposits")
	recorder.depositsRequests = append(recorder.depositsRequests, request)
	return recorder.depositsResponse, recorder.depositsErr
}

func (recorder *govQueryRecorder) TallyResult(
	_ context.Context,
	request *govv1.QueryTallyResultRequest,
	_ ...grpc.CallOption,
) (*govv1.QueryTallyResultResponse, error) {
	recorder.calls = append(recorder.calls, "tally")
	recorder.tallyRequests = append(recorder.tallyRequests, request)
	return recorder.tallyResponse, recorder.tallyErr
}

func (recorder *govQueryRecorder) Params(
	_ context.Context,
	request *govv1.QueryParamsRequest,
	_ ...grpc.CallOption,
) (*govv1.QueryParamsResponse, error) {
	recorder.calls = append(recorder.calls, "params")
	recorder.paramsRequests = append(recorder.paramsRequests, request)
	return recorder.paramsResponse, recorder.paramsErr
}

type govAggregateQuery struct {
	clientv1beta3.QueryClient
	gov govv1.QueryClient
}

func (query *govAggregateQuery) Gov() govv1.QueryClient { return query.gov }

func decodeGovQueryJSON(t *testing.T, output string) map[string]interface{} {
	t.Helper()

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &result), "command output:\n%s", output)
	return result
}

func runGovQueryWithWriter(
	t *testing.T,
	cmd *cobra.Command,
	query clientv1beta3.QueryClient,
	writer io.Writer,
	args ...string,
) error {
	t.Helper()

	encoding := aktcodec.MakeEncodingConfig()
	lightClient := &stubLightClient{
		q: query,
		cctx: sdkclient.Context{
			Codec:             encoding.Codec,
			LegacyAmino:       encoding.Amino,
			InterfaceRegistry: encoding.InterfaceRegistry,
			TxConfig:          encoding.TxConfig,
		},
	}

	cmd.SetOut(writer)
	cmd.SetErr(writer)
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagOutput, "json"))
	ctx := context.WithValue(context.Background(), ContextTypeQueryClient, lightClient)
	cmd.SetContext(ctx)

	return cmd.RunE(cmd, args)
}

type govFailWriter struct{ err error }

func (writer govFailWriter) Write([]byte) (int, error) { return 0, writer.err }

func TestGovQueryExactRequestsAndStructuredResults(t *testing.T) {
	depositor, voter := govTestAddresses()

	t.Run("proposal", func(t *testing.T) {
		message, err := codectypes.NewAnyWithValue(banktypes.NewMsgSend(
			depositor,
			voter,
			sdk.NewCoins(sdk.NewInt64Coin("uakt", 1234567)),
		))
		require.NoError(t, err)
		recorder := &govQueryRecorder{proposalResponse: &govv1.QueryProposalResponse{
			Proposal: &govv1.Proposal{
				Id:       101,
				Title:    "Deterministic proposal",
				Summary:  "A result that must survive structured output",
				Status:   govv1.StatusVotingPeriod,
				Proposer: depositor.String(),
				Messages: []*codectypes.Any{message},
			},
		}}

		var output bytes.Buffer
		err = runGovQueryWithWriter(
			t,
			GetQueryGovProposalCmd(),
			&govAggregateQuery{gov: recorder},
			&output,
			"101",
		)
		require.NoError(t, err)
		require.Equal(t, []*govv1.QueryProposalRequest{{ProposalId: 101}}, recorder.proposalRequests)

		result := decodeGovQueryJSON(t, output.String())
		require.Equal(t, "101", result["id"])
		require.Equal(t, "Deterministic proposal", result["title"])
		require.Equal(t, depositor.String(), result["proposer"])
		messages, ok := result["messages"].([]interface{})
		require.True(t, ok)
		require.Len(t, messages, 1)
		messageResult, ok := messages[0].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "/cosmos.bank.v1beta1.MsgSend", messageResult["@type"])
		require.Equal(t, depositor.String(), messageResult["from_address"])
		require.Equal(t, voter.String(), messageResult["to_address"])
		amounts, ok := messageResult["amount"].([]interface{})
		require.True(t, ok)
		require.Len(t, amounts, 1)
		amount, ok := amounts[0].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "uakt", amount["denom"])
		require.Equal(t, "1234567", amount["amount"])
	})

	t.Run("filtered proposals", func(t *testing.T) {
		recorder := &govQueryRecorder{proposalsResponse: &govv1.QueryProposalsResponse{
			Proposals: []*govv1.Proposal{{
				Id:      102,
				Title:   "Filtered proposal",
				Status:  govv1.StatusVotingPeriod,
				Summary: "Only the requested filter scope",
			}},
		}}

		output, err := execQueryCmd(
			t,
			GetQueryGovProposalsCmd(),
			&govAggregateQuery{gov: recorder},
			"--depositor", depositor.String(),
			"--voter", voter.String(),
			"--status", "voting_period",
			"--page", "3",
			"--limit", "2",
			"--count-total",
			"--reverse",
		)
		require.NoError(t, err)
		require.Equal(t, []*govv1.QueryProposalsRequest{{
			ProposalStatus: govv1.StatusVotingPeriod,
			Voter:          voter.String(),
			Depositor:      depositor.String(),
			Pagination: &query.PageRequest{
				Offset:     4,
				Limit:      2,
				CountTotal: true,
				Reverse:    true,
			},
		}}, recorder.proposalsRequests)

		result := decodeGovQueryJSON(t, output)
		proposals, ok := result["proposals"].([]interface{})
		require.True(t, ok)
		require.Len(t, proposals, 1)
		proposal, ok := proposals[0].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "102", proposal["id"])
		require.Equal(t, "Filtered proposal", proposal["title"])
	})

	t.Run("vote", func(t *testing.T) {
		recorder := &govQueryRecorder{
			proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{
				Id:     103,
				Status: govv1.StatusVotingPeriod,
			}},
			voteResponse: &govv1.QueryVoteResponse{Vote: &govv1.Vote{
				ProposalId: 103,
				Voter:      voter.String(),
				Options: []*govv1.WeightedVoteOption{{
					Option: govv1.OptionYes,
					Weight: "1.000000000000000000",
				}},
				Metadata: "ipfs://vote-proof",
			}},
		}

		output, err := execQueryCmd(
			t,
			GetQueryGovVoteCmd(),
			&govAggregateQuery{gov: recorder},
			"103",
			voter.String(),
		)
		require.NoError(t, err)
		require.Equal(t, []string{"proposal", "vote"}, recorder.calls)
		require.Equal(t, []*govv1.QueryProposalRequest{{ProposalId: 103}}, recorder.proposalRequests)
		require.Equal(t, []*govv1.QueryVoteRequest{{ProposalId: 103, Voter: voter.String()}}, recorder.voteRequests)

		result := decodeGovQueryJSON(t, output)
		require.Equal(t, "103", result["proposal_id"])
		require.Equal(t, voter.String(), result["voter"])
		require.Equal(t, "ipfs://vote-proof", result["metadata"])
	})

	t.Run("votes", func(t *testing.T) {
		recorder := &govQueryRecorder{
			proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{
				Id:     104,
				Status: govv1.StatusDepositPeriod,
			}},
			votesResponse: &govv1.QueryVotesResponse{Votes: []*govv1.Vote{{
				ProposalId: 104,
				Voter:      voter.String(),
				Options: []*govv1.WeightedVoteOption{{
					Option: govv1.OptionNo,
					Weight: "1.000000000000000000",
				}},
			}}},
		}

		output, err := execQueryCmd(
			t,
			GetQueryGovVotesCmd(),
			&govAggregateQuery{gov: recorder},
			"104",
			"--offset", "5",
			"--limit", "9",
		)
		require.NoError(t, err)
		require.Equal(t, []string{"proposal", "votes"}, recorder.calls)
		require.Equal(t, []*govv1.QueryVotesRequest{{
			ProposalId: 104,
			Pagination: &query.PageRequest{Offset: 5, Limit: 9},
		}}, recorder.votesRequests)

		result := decodeGovQueryJSON(t, output)
		votes, ok := result["votes"].([]interface{})
		require.True(t, ok)
		require.Len(t, votes, 1)
	})

	t.Run("deposit", func(t *testing.T) {
		recorder := &govQueryRecorder{
			proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 105}},
			depositResponse: &govv1.QueryDepositResponse{Deposit: &govv1.Deposit{
				ProposalId: 105,
				Depositor:  depositor.String(),
				Amount:     sdk.NewCoins(sdk.NewInt64Coin("uakt", 27)),
			}},
		}

		output, err := execQueryCmd(
			t,
			GetQueryGovDepositCmd(),
			&govAggregateQuery{gov: recorder},
			"105",
			depositor.String(),
		)
		require.NoError(t, err)
		require.Equal(t, []string{"proposal", "deposit"}, recorder.calls)
		require.Equal(t, []*govv1.QueryDepositRequest{{
			ProposalId: 105,
			Depositor:  depositor.String(),
		}}, recorder.depositRequests)

		result := decodeGovQueryJSON(t, output)
		require.Equal(t, "105", result["proposal_id"])
		require.Equal(t, depositor.String(), result["depositor"])
		amount, ok := result["amount"].([]interface{})
		require.True(t, ok)
		require.Len(t, amount, 1)
	})

	t.Run("deposits", func(t *testing.T) {
		recorder := &govQueryRecorder{
			proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 106}},
			depositsResponse: &govv1.QueryDepositsResponse{Deposits: []*govv1.Deposit{{
				ProposalId: 106,
				Depositor:  depositor.String(),
				Amount:     sdk.NewCoins(sdk.NewInt64Coin("uakt", 31)),
			}}},
		}

		output, err := execQueryCmd(
			t,
			GetQueryGovDepositsCmd(),
			&govAggregateQuery{gov: recorder},
			"106",
			"--page-key", "cGFnZS10d28=",
			"--limit", "4",
			"--reverse",
		)
		require.NoError(t, err)
		require.Equal(t, []string{"proposal", "deposits"}, recorder.calls)
		require.Equal(t, []*govv1.QueryDepositsRequest{{
			ProposalId: 106,
			Pagination: &query.PageRequest{Key: []byte("page-two"), Limit: 4, Reverse: true},
		}}, recorder.depositsRequests)

		result := decodeGovQueryJSON(t, output)
		deposits, ok := result["deposits"].([]interface{})
		require.True(t, ok)
		require.Len(t, deposits, 1)
	})

	t.Run("tally", func(t *testing.T) {
		recorder := &govQueryRecorder{
			proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 107}},
			tallyResponse: &govv1.QueryTallyResultResponse{Tally: &govv1.TallyResult{
				YesCount:        "70",
				NoCount:         "20",
				AbstainCount:    "7",
				NoWithVetoCount: "3",
			}},
		}

		output, err := execQueryCmd(
			t,
			GetQueryGovTallyCmd(),
			&govAggregateQuery{gov: recorder},
			"107",
		)
		require.NoError(t, err)
		require.Equal(t, []string{"proposal", "tally"}, recorder.calls)
		require.Equal(t, []*govv1.QueryTallyResultRequest{{ProposalId: 107}}, recorder.tallyRequests)

		result := decodeGovQueryJSON(t, output)
		require.Equal(t, "70", result["yes_count"])
		require.Equal(t, "3", result["no_with_veto_count"])
	})

	t.Run("combined params", func(t *testing.T) {
		params := govv1.DefaultParams()
		params.Quorum = "0.400000000000000000"
		params.Threshold = "0.550000000000000000"
		params.VetoThreshold = "0.340000000000000000"
		votingPeriod := 96 * time.Hour
		params.VotingPeriod = &votingPeriod
		recorder := &govQueryRecorder{paramsResponse: &govv1.QueryParamsResponse{Params: &params}}

		output, err := execQueryCmd(
			t,
			GetQueryGovQueryParamsCmd(),
			&govAggregateQuery{gov: recorder},
		)
		require.NoError(t, err)
		require.Equal(t, []*govv1.QueryParamsRequest{{ParamsType: "deposit"}}, recorder.paramsRequests)

		result := decodeGovQueryJSON(t, output)
		full, ok := result["params"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "0.400000000000000000", full["quorum"])
		voting, ok := result["voting_params"].(map[string]interface{})
		require.True(t, ok)
		require.NotEmpty(t, voting["voting_period"])
		tally, ok := result["tally_params"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "0.550000000000000000", tally["threshold"])
	})
}

func TestGovEndedVotesReadCanonicalPaginationFlags(t *testing.T) {
	recorder := &govQueryRecorder{proposalResponse: &govv1.QueryProposalResponse{
		Proposal: &govv1.Proposal{Id: 901, Status: govv1.StatusPassed},
	}}
	cmd := GetQueryGovVotesCmd()
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagPage, "3"))
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagLimit, "8"))

	_ = runGovQueryWithWriter(
		t,
		cmd,
		&govAggregateQuery{gov: recorder},
		io.Discard,
		"901",
	)
	require.Equal(t, []string{"proposal"}, recorder.calls)
}

//nolint:staticcheck // the command intentionally exposes Cosmos' deprecated split parameter views.
func TestGovQueryParameterSelectorsRequestAndPrintMatchingView(t *testing.T) {
	votingPeriod := 72 * time.Hour
	maxDepositPeriod := 48 * time.Hour
	tests := []struct {
		name          string
		selector      string
		response      *govv1.QueryParamsResponse
		structuredKey string
	}{
		{
			name:          "voting",
			selector:      "voting",
			response:      &govv1.QueryParamsResponse{VotingParams: &govv1.VotingParams{VotingPeriod: &votingPeriod}},
			structuredKey: "voting_period",
		},
		{
			name:     "tallying",
			selector: "tallying",
			response: &govv1.QueryParamsResponse{TallyParams: &govv1.TallyParams{
				Quorum: "0.34", Threshold: "0.51", VetoThreshold: "0.334",
			}},
			structuredKey: "threshold",
		},
		{
			name:     "deposit",
			selector: "deposit",
			response: &govv1.QueryParamsResponse{DepositParams: &govv1.DepositParams{
				MinDeposit:       sdk.NewCoins(sdk.NewInt64Coin("uakt", 1000)),
				MaxDepositPeriod: &maxDepositPeriod,
			}},
			structuredKey: "min_deposit",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &govQueryRecorder{paramsResponse: test.response}
			output, err := execQueryCmd(
				t,
				GetQueryGovParamCmd(),
				&govAggregateQuery{gov: recorder},
				test.selector,
			)
			require.NoError(t, err)
			require.Equal(t, []*govv1.QueryParamsRequest{{ParamsType: test.selector}}, recorder.paramsRequests)
			require.Contains(t, decodeGovQueryJSON(t, output), test.structuredKey)
		})
	}
}

func TestGovQueryRejectsBoundaryInputsBeforeDependencies(t *testing.T) {
	tests := []struct {
		name       string
		command    func() *cobra.Command
		args       []string
		wantError  string
		assertIdle func(*testing.T, *govQueryRecorder)
	}{
		{
			name:      "vote voter address",
			command:   GetQueryGovVoteCmd,
			args:      []string{"7", "not-an-address"},
			wantError: "decoding bech32",
			assertIdle: func(t *testing.T, recorder *govQueryRecorder) {
				t.Helper()
				require.Empty(t, recorder.proposalRequests)
			},
		},
		{
			name:      "deposit depositor address",
			command:   GetQueryGovDepositCmd,
			args:      []string{"8", "not-an-address"},
			wantError: "decoding bech32",
			assertIdle: func(t *testing.T, recorder *govQueryRecorder) {
				t.Helper()
				require.Empty(t, recorder.proposalRequests)
				require.Empty(t, recorder.depositRequests)
			},
		},
		{
			name:      "parameter type",
			command:   GetQueryGovParamCmd,
			args:      []string{"quorum"},
			wantError: "argument must be one of",
			assertIdle: func(t *testing.T, recorder *govQueryRecorder) {
				t.Helper()
				require.Empty(t, recorder.paramsRequests)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &govQueryRecorder{
				proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{}},
				depositResponse:  &govv1.QueryDepositResponse{Deposit: &govv1.Deposit{}},
				paramsResponse:   &govv1.QueryParamsResponse{},
			}

			_, err := execQueryCmd(t, test.command(), &govAggregateQuery{gov: recorder}, test.args...)
			require.ErrorContains(t, err, test.wantError)
			test.assertIdle(t, recorder)
		})
	}
}

func TestGovQueryRejectsPaginationBeforeProposalPreflight(t *testing.T) {
	tests := []struct {
		name    string
		command func() *cobra.Command
		args    []string
	}{
		{
			name:    "votes page and offset",
			command: GetQueryGovVotesCmd,
			args:    []string{"9", "--page", "2", "--offset", "1"},
		},
		{
			name:    "deposits malformed page key",
			command: GetQueryGovDepositsCmd,
			args:    []string{"10", "--page-key", "%%%"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &govQueryRecorder{proposalResponse: &govv1.QueryProposalResponse{
				Proposal: &govv1.Proposal{Status: govv1.StatusVotingPeriod},
			}}

			_, err := execQueryCmd(t, test.command(), &govAggregateQuery{gov: recorder}, test.args...)
			require.Error(t, err)
			require.Empty(t, recorder.proposalRequests)
		})
	}
}

func TestGovQueryProposalPreflightErrorsPreserveCause(t *testing.T) {
	dependencyErr := errors.New("governance proposal dependency unavailable")
	tests := []struct {
		name    string
		command func() *cobra.Command
		args    []string
	}{
		{name: "vote", command: GetQueryGovVoteCmd, args: []string{"7", govTestAddress(t)}},
		{name: "votes", command: GetQueryGovVotesCmd, args: []string{"8"}},
		{name: "deposit", command: GetQueryGovDepositCmd, args: []string{"9", govTestAddress(t)}},
		{name: "deposits", command: GetQueryGovDepositsCmd, args: []string{"10"}},
		{name: "tally", command: GetQueryGovTallyCmd, args: []string{"11"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &govQueryRecorder{proposalErr: dependencyErr}
			_, err := execQueryCmd(
				t,
				test.command(),
				&govAggregateQuery{gov: recorder},
				test.args...,
			)
			require.ErrorIs(t, err, dependencyErr)
			require.Equal(t, []string{"proposal"}, recorder.calls)
		})
	}
}

func TestGovQueryDependencyErrorsStopAtFailingRequest(t *testing.T) {
	dependencyErr := errors.New("governance query dependency unavailable")
	address := govTestAddress(t)
	tests := []struct {
		name      string
		command   func() *cobra.Command
		args      []string
		recorder  func() *govQueryRecorder
		wantCalls []string
	}{
		{
			name:    "proposal",
			command: GetQueryGovProposalCmd,
			args:    []string{"21"},
			recorder: func() *govQueryRecorder {
				return &govQueryRecorder{proposalErr: dependencyErr}
			},
			wantCalls: []string{"proposal"},
		},
		{
			name:    "proposals",
			command: GetQueryGovProposalsCmd,
			recorder: func() *govQueryRecorder {
				return &govQueryRecorder{proposalsErr: dependencyErr}
			},
			wantCalls: []string{"proposals"},
		},
		{
			name:    "vote",
			command: GetQueryGovVoteCmd,
			args:    []string{"22", address},
			recorder: func() *govQueryRecorder {
				return &govQueryRecorder{
					proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 22}},
					voteErr:          dependencyErr,
				}
			},
			wantCalls: []string{"proposal", "vote"},
		},
		{
			name:    "votes",
			command: GetQueryGovVotesCmd,
			args:    []string{"23"},
			recorder: func() *govQueryRecorder {
				return &govQueryRecorder{
					proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 23, Status: govv1.StatusVotingPeriod}},
					votesErr:         dependencyErr,
				}
			},
			wantCalls: []string{"proposal", "votes"},
		},
		{
			name:    "deposit",
			command: GetQueryGovDepositCmd,
			args:    []string{"24", address},
			recorder: func() *govQueryRecorder {
				return &govQueryRecorder{
					proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 24}},
					depositErr:       dependencyErr,
				}
			},
			wantCalls: []string{"proposal", "deposit"},
		},
		{
			name:    "deposits",
			command: GetQueryGovDepositsCmd,
			args:    []string{"25"},
			recorder: func() *govQueryRecorder {
				return &govQueryRecorder{
					proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 25}},
					depositsErr:      dependencyErr,
				}
			},
			wantCalls: []string{"proposal", "deposits"},
		},
		{
			name:    "tally",
			command: GetQueryGovTallyCmd,
			args:    []string{"26"},
			recorder: func() *govQueryRecorder {
				return &govQueryRecorder{
					proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 26}},
					tallyErr:         dependencyErr,
				}
			},
			wantCalls: []string{"proposal", "tally"},
		},
		{
			name:    "combined params",
			command: GetQueryGovQueryParamsCmd,
			recorder: func() *govQueryRecorder {
				return &govQueryRecorder{paramsErr: dependencyErr}
			},
			wantCalls: []string{"params"},
		},
		{
			name:    "parameter view",
			command: GetQueryGovParamCmd,
			args:    []string{"voting"},
			recorder: func() *govQueryRecorder {
				return &govQueryRecorder{paramsErr: dependencyErr}
			},
			wantCalls: []string{"params"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := test.recorder()
			output, err := execQueryCmd(
				t,
				test.command(),
				&govAggregateQuery{gov: recorder},
				test.args...,
			)
			require.ErrorIs(t, err, dependencyErr)
			require.Equal(t, test.wantCalls, recorder.calls)
			require.Empty(t, output)
		})
	}
}

func TestGovQueryNoProposalsIsExplicitFailure(t *testing.T) {
	recorder := &govQueryRecorder{proposalsResponse: &govv1.QueryProposalsResponse{}}
	output, err := execQueryCmd(
		t,
		GetQueryGovProposalsCmd(),
		&govAggregateQuery{gov: recorder},
	)
	require.EqualError(t, err, "no proposals found")
	require.Empty(t, output)
	require.Equal(t, []string{"proposals"}, recorder.calls)
}

func TestGovQueryWriterErrorsPreserveCause(t *testing.T) {
	writerErr := errors.New("governance output destination unavailable")
	address := govTestAddress(t)
	tests := []struct {
		name     string
		command  func() *cobra.Command
		args     []string
		recorder *govQueryRecorder
	}{
		{
			name:    "proposal",
			command: GetQueryGovProposalCmd,
			args:    []string{"31"},
			recorder: &govQueryRecorder{proposalResponse: &govv1.QueryProposalResponse{
				Proposal: &govv1.Proposal{Id: 31, Title: "Writer failure"},
			}},
		},
		{
			name:    "proposals formatter",
			command: GetQueryGovProposalsCmd,
			recorder: &govQueryRecorder{proposalsResponse: &govv1.QueryProposalsResponse{
				Proposals: []*govv1.Proposal{{Id: 32, Title: "Writer failure"}},
			}},
		},
		{
			name:    "vote",
			command: GetQueryGovVoteCmd,
			args:    []string{"33", address},
			recorder: &govQueryRecorder{
				proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 33}},
				voteResponse: &govv1.QueryVoteResponse{Vote: &govv1.Vote{
					ProposalId: 33,
					Voter:      address,
					Options:    govv1.NewNonSplitVoteOption(govv1.OptionAbstain),
				}},
			},
		},
		{
			name:    "deposits",
			command: GetQueryGovDepositsCmd,
			args:    []string{"34"},
			recorder: &govQueryRecorder{
				proposalResponse: &govv1.QueryProposalResponse{Proposal: &govv1.Proposal{Id: 34}},
				depositsResponse: &govv1.QueryDepositsResponse{Deposits: []*govv1.Deposit{{
					ProposalId: 34,
					Depositor:  address,
					Amount:     sdk.NewCoins(sdk.NewInt64Coin("uakt", 1)),
				}}},
			},
		},
		{
			name:    "combined params",
			command: GetQueryGovQueryParamsCmd,
			recorder: func() *govQueryRecorder {
				params := govv1.DefaultParams()
				return &govQueryRecorder{paramsResponse: &govv1.QueryParamsResponse{Params: &params}}
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runGovQueryWithWriter(
				t,
				test.command(),
				&govAggregateQuery{gov: test.recorder},
				govFailWriter{err: writerErr},
				test.args...,
			)
			require.ErrorIs(t, err, writerErr)
		})
	}
}

func govTestAddress(t *testing.T) string {
	t.Helper()
	address, _ := govTestAddresses()
	return address.String()
}
