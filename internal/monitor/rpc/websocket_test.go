package rpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/monitor/consensus"
)

func TestConsensusTrackerResetsVotesWhenRoundAdvancesAtSameHeight(t *testing.T) {
	tracker := newConsensusTracker([]consensus.Validator{
		{Address: "validator-0", VotingPower: "10"},
		{Address: "validator-1", VotingPower: "20"},
	})

	_, changed := tracker.handleNewRoundStepEvent(json.RawMessage(
		`{"type":"tendermint/event/NewRoundStep","value":{"height":"10","round":0,"step":3}}`,
	))
	require.True(t, changed)

	withVote, changed := tracker.handleVoteEvent(json.RawMessage(
		`{"type":"tendermint/event/Vote","value":{"Vote":{"type":1,"height":"10","round":0,"validator_index":0}}}`,
	))
	require.True(t, changed)
	require.Equal(t, 1, withVote.State.PrevoteCount)
	require.Equal(t, int64(10), withVote.State.PrevotePower)

	nextRound, changed := tracker.handleNewRoundStepEvent(json.RawMessage(
		`{"type":"tendermint/event/NewRoundStep","value":{"height":"10","round":1,"step":1}}`,
	))
	require.True(t, changed)
	require.Equal(t, int64(10), nextRound.State.Height)
	require.Equal(t, 1, nextRound.State.Round)
	require.Zero(t, nextRound.State.PrevoteCount)
	require.Zero(t, nextRound.State.PrevotePower)
	require.Zero(t, nextRound.State.PrecommitCount)
	require.Zero(t, nextRound.State.PrecommitPower)
	require.False(t, nextRound.State.Validators[0].Prevoted)
	require.False(t, nextRound.State.Validators[0].Precommited)
}

func TestConsensusTrackerKeepsVotesAcrossStepsInSameRound(t *testing.T) {
	tracker := newConsensusTracker([]consensus.Validator{
		{Address: "validator-0", VotingPower: "10"},
	})

	_, changed := tracker.handleNewRoundStepEvent(json.RawMessage(
		`{"type":"tendermint/event/NewRoundStep","value":{"height":"10","round":2,"step":3}}`,
	))
	require.True(t, changed)

	_, changed = tracker.handleVoteEvent(json.RawMessage(
		`{"type":"tendermint/event/Vote","value":{"Vote":{"type":1,"height":"10","round":2,"validator_index":0}}}`,
	))
	require.True(t, changed)

	nextStep, changed := tracker.handleNewRoundStepEvent(json.RawMessage(
		`{"type":"tendermint/event/NewRoundStep","value":{"height":"10","round":2,"step":4}}`,
	))
	require.True(t, changed)
	require.Equal(t, 2, nextStep.State.Round)
	require.Equal(t, 4, nextStep.State.Step)
	require.Equal(t, 1, nextStep.State.PrevoteCount)
	require.Equal(t, int64(10), nextStep.State.PrevotePower)
	require.True(t, nextStep.State.Validators[0].Prevoted)
}

func TestConsensusTrackerAcceptsInitialVoteAtNonzeroRound(t *testing.T) {
	tracker := newConsensusTracker([]consensus.Validator{{VotingPower: "10"}})

	snapshot, changed := tracker.handleVoteEvent(json.RawMessage(
		`{"value":{"Vote":{"type":1,"height":"10","round":2,"validator_index":0}}}`,
	))

	require.True(t, changed)
	require.Equal(t, int64(10), snapshot.State.Height)
	require.Equal(t, 2, snapshot.State.Round)
	require.Equal(t, 1, snapshot.State.PrevoteCount)
	require.Equal(t, int64(10), snapshot.State.PrevotePower)
}

func TestConsensusTrackerAdvancesOnForwardRoundVote(t *testing.T) {
	tracker := newConsensusTracker([]consensus.Validator{
		{VotingPower: "10"},
		{VotingPower: "20"},
	})

	_, changed := tracker.handleNewRoundStepEvent(json.RawMessage(
		`{"value":{"height":"42","round":0,"step":4}}`,
	))
	require.True(t, changed)
	_, changed = tracker.handleVoteEvent(json.RawMessage(
		`{"value":{"Vote":{"type":1,"height":"42","round":0,"validator_index":0}}}`,
	))
	require.True(t, changed)

	forward, changed := tracker.handleVoteEvent(json.RawMessage(
		`{"value":{"Vote":{"type":1,"height":"42","round":1,"validator_index":1}}}`,
	))
	require.True(t, changed)
	require.Equal(t, int64(42), forward.State.Height)
	require.Equal(t, 1, forward.State.Round)
	require.Zero(t, forward.State.Step)
	require.Equal(t, 1, forward.State.PrevoteCount)
	require.Equal(t, int64(20), forward.State.PrevotePower)
	require.False(t, forward.State.Validators[0].Prevoted)
	require.True(t, forward.State.Validators[1].Prevoted)

	_, changed = tracker.handleVoteEvent(json.RawMessage(
		`{"value":{"Vote":{"type":2,"height":"42","round":0,"validator_index":0}}}`,
	))
	require.False(t, changed)
	require.Equal(t, int64(42), tracker.height)
	require.Equal(t, 1, tracker.round)
	require.Zero(t, tracker.pcPower)
}

func TestConsensusTrackerRejectsStaleProgress(t *testing.T) {
	tests := []struct {
		name   string
		isVote bool
		data   json.RawMessage
	}{
		{
			name: "lower-height step",
			data: json.RawMessage(`{"value":{"height":"9","round":2,"step":4}}`),
		},
		{
			name: "lower-round step",
			data: json.RawMessage(`{"value":{"height":"10","round":1,"step":4}}`),
		},
		{
			name: "lower step",
			data: json.RawMessage(`{"value":{"height":"10","round":2,"step":3}}`),
		},
		{
			name:   "lower-height vote",
			isVote: true,
			data:   json.RawMessage(`{"value":{"Vote":{"type":2,"height":"9","round":2,"validator_index":0}}}`),
		},
		{
			name:   "lower-round vote",
			isVote: true,
			data:   json.RawMessage(`{"value":{"Vote":{"type":2,"height":"10","round":1,"validator_index":0}}}`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracker := newConsensusTracker([]consensus.Validator{{VotingPower: "10"}})
			_, changed := tracker.handleNewRoundStepEvent(json.RawMessage(
				`{"value":{"height":"10","round":2,"step":4}}`,
			))
			require.True(t, changed)
			_, changed = tracker.handleVoteEvent(json.RawMessage(
				`{"value":{"Vote":{"type":1,"height":"10","round":2,"validator_index":0}}}`,
			))
			require.True(t, changed)

			if tc.isVote {
				_, changed = tracker.handleVoteEvent(tc.data)
			} else {
				_, changed = tracker.handleNewRoundStepEvent(tc.data)
			}

			require.False(t, changed)
			require.Equal(t, int64(10), tracker.height)
			require.Equal(t, 2, tracker.round)
			require.Equal(t, 4, tracker.step)
			require.Equal(t, int64(10), tracker.pvPower)
			require.Zero(t, tracker.pcPower)
			require.True(t, tracker.prevoted[0])
			require.False(t, tracker.precommited[0])
		})
	}
}

func TestConsensusTrackerRejectsNegativeRoundStateValues(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
	}{
		{
			name: "height",
			data: json.RawMessage(`{"value":{"height":"-1","round":0,"step":1}}`),
		},
		{
			name: "round",
			data: json.RawMessage(`{"value":{"height":"10","round":-1,"step":1}}`),
		},
		{
			name: "step",
			data: json.RawMessage(`{"value":{"height":"10","round":0,"step":-1}}`),
		},
		{
			name: "malformed height",
			data: json.RawMessage(`{"value":{"height":"not-a-height","round":0,"step":1}}`),
		},
		{
			name: "malformed event",
			data: json.RawMessage(`not-json`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracker := newConsensusTracker(nil)
			_, changed := tracker.handleNewRoundStepEvent(tc.data)
			require.False(t, changed)
			require.Zero(t, tracker.height)
			require.Zero(t, tracker.round)
			require.Zero(t, tracker.step)
		})
	}
}

func TestConsensusTrackerRejectsInvalidVoteIdentity(t *testing.T) {
	tracker := newConsensusTracker([]consensus.Validator{{VotingPower: "10"}})

	for _, data := range []json.RawMessage{
		json.RawMessage(`{"value":{"Vote":{"type":1,"height":"-1","round":0,"validator_index":0}}}`),
		json.RawMessage(`{"value":{"Vote":{"type":1,"height":"bad","round":0,"validator_index":0}}}`),
		json.RawMessage(`{"value":{"Vote":{"type":1,"height":"10","round":-1,"validator_index":0}}}`),
		json.RawMessage(`{"value":{"Vote":{"type":1,"height":"10","round":0,"validator_index":-1}}}`),
		json.RawMessage(`{"value":{"Vote":{"type":1,"height":"10","round":0,"validator_index":1}}}`),
	} {
		_, changed := tracker.handleVoteEvent(data)
		require.False(t, changed)
	}
}

func TestConsensusTrackerRoutesWebSocketMessages(t *testing.T) {
	tracker := newConsensusTracker([]consensus.Validator{{VotingPower: "10"}})

	for _, raw := range []json.RawMessage{
		json.RawMessage(`not-json`),
		json.RawMessage(`{"query":"tm.event='Vote'","data":{}}`),
		json.RawMessage(`{"query":"tm.event='Unknown'","data":{"value":{}}}`),
	} {
		_, changed := tracker.handleMessage(raw)
		require.False(t, changed)
	}

	round, changed := tracker.handleMessage(json.RawMessage(`{
  "query": "tm.event='NewRoundStep'",
  "data": {"value":{"height":"10","round":1,"step":2}}
}`))
	require.True(t, changed)
	require.Equal(t, int64(10), round.State.Height)
	require.Equal(t, 1, round.State.Round)
	require.Equal(t, 2, round.State.Step)

	vote, changed := tracker.handleMessage(json.RawMessage(`{
  "query": "tm.event='Vote'",
  "data": {"value":{"Vote":{"type":1,"height":"10","round":1,"validator_index":0}}}
}`))
	require.True(t, changed)
	require.Equal(t, 1, vote.State.PrevoteCount)
	require.Equal(t, int64(10), vote.State.PrevotePower)
}

func TestConsensusTrackerHandlesVoteTransitions(t *testing.T) {
	tracker := newConsensusTracker([]consensus.Validator{
		{Address: "validator-0", VotingPower: "10"},
		{Address: "validator-1", VotingPower: "20"},
	})

	_, changed := tracker.handleVoteEvent(json.RawMessage(`not-json`))
	require.False(t, changed)

	_, changed = tracker.handleNewRoundStepEvent(json.RawMessage(
		`{"value":{"height":"10","round":0,"step":1}}`,
	))
	require.True(t, changed)

	precommit, changed := tracker.handleVoteEvent(json.RawMessage(
		`{"value":{"Vote":{"type":2,"height":"10","round":0,"validator_index":1}}}`,
	))
	require.True(t, changed)
	require.Equal(t, 1, precommit.State.PrecommitCount)
	require.Equal(t, int64(20), precommit.State.PrecommitPower)

	duplicate, changed := tracker.handleVoteEvent(json.RawMessage(
		`{"value":{"Vote":{"type":2,"height":"10","round":0,"validator_index":1}}}`,
	))
	require.True(t, changed)
	require.Equal(t, 1, duplicate.State.PrecommitCount)
	require.Equal(t, int64(20), duplicate.State.PrecommitPower)

	_, changed = tracker.handleVoteEvent(json.RawMessage(
		`{"value":{"Vote":{"type":3,"height":"10","round":0,"validator_index":0}}}`,
	))
	require.False(t, changed)
	nextHeight, changed := tracker.handleVoteEvent(json.RawMessage(
		`{"value":{"Vote":{"type":1,"height":"11","round":0,"validator_index":0}}}`,
	))
	require.True(t, changed)
	require.Equal(t, int64(11), nextHeight.State.Height)
	require.Equal(t, 1, nextHeight.State.PrevoteCount)
	require.Zero(t, nextHeight.State.PrecommitCount)
}
