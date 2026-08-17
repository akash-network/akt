package consensus

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseHeightRoundStep(t *testing.T) {
	height, round, step, err := ParseHeightRoundStep("123/4/5")
	require.NoError(t, err)
	require.Equal(t, int64(123), height)
	require.Equal(t, 4, round)
	require.Equal(t, 5, step)

	for _, tc := range []struct {
		name  string
		value string
		field string
	}{
		{name: "shape", value: "1/2", field: "format"},
		{name: "height", value: "bad/2/3", field: "height"},
		{name: "round", value: "1/bad/3", field: "round"},
		{name: "step", value: "1/2/bad", field: "step"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := ParseHeightRoundStep(tc.value)
			require.ErrorContains(t, err, tc.field)
		})
	}
}

func TestParseHeightRoundStepRejectsNegativeValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		field string
	}{
		{name: "height", value: "-1/0/1", field: "height"},
		{name: "round", value: "1/-1/1", field: "round"},
		{name: "step", value: "1/0/-1", field: "step"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := ParseHeightRoundStep(tc.value)
			require.ErrorContains(t, err, tc.field)
		})
	}
}

func TestParseConsensusStateRejectsNegativeRoundWithoutPanic(t *testing.T) {
	resp := &ConsensusResponse{}
	resp.Result.RoundState.HeightRoundStep = "1/-1/1"

	var err error
	require.NotPanics(t, func() {
		_, err = ParseConsensusState(resp, nil)
	})
	require.ErrorContains(t, err, "round")
}

func TestParseBitArray(t *testing.T) {
	pattern, voted, total, percent := ParseBitArray("BA{3:x_x} 40/60 = 0.67")
	require.Equal(t, "x_x", pattern)
	require.Equal(t, int64(40), voted)
	require.Equal(t, int64(60), total)
	require.InDelta(t, 0.67, percent, 0.0001)

	pattern, voted, total, percent = ParseBitArray("not a bit array")
	require.Empty(t, pattern)
	require.Zero(t, voted)
	require.Zero(t, total)
	require.Zero(t, percent)
}

func TestCountVotes(t *testing.T) {
	require.Zero(t, CountVotes(nil))
	require.Equal(t, 3, CountVotes([]string{"vote-a", "nil-Vote", "vote-b", ""}))
}

func TestParseConsensusStateUsesVoteSetForExactRound(t *testing.T) {
	start := time.Now().Add(-2 * time.Second)
	resp := &ConsensusResponse{}
	resp.Result.RoundState = RoundState{
		HeightRoundStep: "123/2/4",
		StartTime:       start,
		Proposer: ProposerInfo{
			Address: "validator-b",
			Index:   1,
		},
		HeightVoteSet: []HeightVote{
			{
				Round:              2,
				Prevotes:           []string{"vote-a", "nil-Vote", "vote-c"},
				PrevotesBitArray:   "BA{3:x_x} 40/60 = 0.67",
				Precommits:         []string{"nil-Vote", "vote-b", "nil-Vote"},
				PrecommitsBitArray: "BA{3:_x_} 20/60 = 0.33",
			},
		},
	}
	validators := []Validator{
		{Address: "validator-a", VotingPower: "10", PubKey: PubKey{Value: "pub-a"}},
		{Address: "validator-b", VotingPower: "20", PubKey: PubKey{Value: "pub-b"}},
		{Address: "validator-c", VotingPower: "30", PubKey: PubKey{Value: "pub-c"}},
	}

	state, err := ParseConsensusState(resp, validators)
	require.NoError(t, err)
	require.Equal(t, int64(123), state.Height)
	require.Equal(t, 2, state.Round)
	require.Equal(t, 4, state.Step)
	require.Equal(t, start, state.StartTime)
	require.GreaterOrEqual(t, state.Elapsed, 2*time.Second)
	require.Equal(t, "validator-b", state.ProposerAddress)
	require.Equal(t, 1, state.ProposerIndex)
	require.Equal(t, 3, state.TotalValidators)
	require.Equal(t, int64(60), state.TotalVotingPower)
	require.Equal(t, 2, state.PrevoteCount)
	require.Equal(t, int64(40), state.PrevotePower)
	require.InDelta(t, 0.67, state.PrevotePercent, 0.0001)
	require.Equal(t, "x_x", state.PrevoteBitArray)
	require.Equal(t, 1, state.PrecommitCount)
	require.Equal(t, int64(20), state.PrecommitPower)
	require.InDelta(t, 0.33, state.PrecommitPercent, 0.0001)
	require.Equal(t, "_x_", state.PrecommitBitArray)
	require.Equal(t, []ValidatorStatus{
		{
			Index: 0, Address: "validator-a", PubKey: "pub-a", VotingPower: 10,
			Prevoted: true,
		},
		{
			Index: 1, Address: "validator-b", PubKey: "pub-b", VotingPower: 20,
			Precommited: true, IsProposer: true,
		},
		{
			Index: 2, Address: "validator-c", PubKey: "pub-c", VotingPower: 30,
			Prevoted: true,
		},
	}, state.Validators)
}

func TestParseConsensusStateWithoutVotes(t *testing.T) {
	resp := &ConsensusResponse{}
	resp.Result.RoundState = RoundState{
		HeightRoundStep: "8/1/2",
		StartTime:       time.Now(),
		Proposer:        ProposerInfo{Index: 0},
	}
	validators := []Validator{
		{Address: "validator-a", VotingPower: "invalid", PubKey: PubKey{Value: "pub-a"}},
	}

	state, err := ParseConsensusState(resp, validators)
	require.NoError(t, err)
	require.Equal(t, 1, state.TotalValidators)
	require.Zero(t, state.TotalVotingPower)
	require.Equal(t, []ValidatorStatus{{
		Index: 0, Address: "validator-a", PubKey: "pub-a", IsProposer: true,
	}}, state.Validators)
}

func TestParseConsensusStateRejectsNilResponse(t *testing.T) {
	var err error
	require.NotPanics(t, func() {
		_, err = ParseConsensusState(nil, nil)
	})
	require.ErrorContains(t, err, "response")
}

func FuzzParseHeightRoundStepCanonicalRoundTrip(f *testing.F) {
	f.Add("123/4/5")
	f.Add("0/0/0")
	f.Add("1/-1/2")
	f.Add("malformed")

	f.Fuzz(func(t *testing.T, input string) {
		height, round, step, err := ParseHeightRoundStep(input)
		if err != nil {
			return
		}

		require.GreaterOrEqual(t, height, int64(0))
		require.GreaterOrEqual(t, round, 0)
		require.GreaterOrEqual(t, step, 0)

		canonical := fmt.Sprintf("%d/%d/%d", height, round, step)
		roundTripHeight, roundTripRound, roundTripStep, err := ParseHeightRoundStep(canonical)
		require.NoError(t, err)
		require.Equal(t, height, roundTripHeight)
		require.Equal(t, round, roundTripRound)
		require.Equal(t, step, roundTripStep)
	})
}
