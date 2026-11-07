package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	cmtjson "github.com/cometbft/cometbft/libs/json"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/monitor/consensus"
)

func TestConsensusDecodesCometEncodedRoundEvents(t *testing.T) {
	tracker := newConsensusTracker([]consensus.Validator{
		{
			Address:     "AA",
			VotingPower: "60",
		},
		{
			Address:     "BB",
			VotingPower: "40",
		},
	})

	raw, err := cmtjson.Marshal(ctypes.ResultEvent{
		Query: "tm.event='NewRound'",
		Data: cmtypes.EventDataNewRound{
			Height: 42,
			Round:  2,
			Step:   "RoundStepNewRound",
			Proposer: cmtypes.ValidatorInfo{
				Address: []byte{0xBB},
				Index:   1,
			},
		},
	})
	require.NoError(t, err)
	snapshot, ok := tracker.handleMessage(raw)
	require.True(t, ok)
	require.Equal(t, 2, snapshot.State.Step)
	require.Equal(t, "BB", snapshot.State.ProposerAddress)
	require.True(t, snapshot.State.Validators[1].IsProposer)

	tracker.handleVoteEvent(json.RawMessage(`{"value":{"Vote":{"type":1,"height":"42","round":2,"validator_index":0}}}`))
	raw, err = cmtjson.Marshal(ctypes.ResultEvent{
		Query: "tm.event='NewRoundStep'",
		Data: cmtypes.EventDataRoundState{
			Height: 42,
			Round:  3,
			Step:   "RoundStepPrevote",
		},
	})
	require.NoError(t, err)
	snapshot, ok = tracker.handleMessage(raw)
	require.True(t, ok)
	require.Equal(t, 3, snapshot.State.Round)
	require.Equal(t, 4, snapshot.State.Step)
	require.Zero(t, snapshot.State.PrevotePower)
	require.Equal(t, -1, snapshot.State.ProposerIndex)

	// The proposer subscription can arrive after the step subscription.
	raw, err = cmtjson.Marshal(ctypes.ResultEvent{
		Query: "tm.event='NewRound'",
		Data: cmtypes.EventDataNewRound{
			Height: 42,
			Round:  3,
			Step:   "RoundStepNewRound",
			Proposer: cmtypes.ValidatorInfo{
				Address: []byte{0xAA},
				Index:   0,
			},
		},
	})
	require.NoError(t, err)
	snapshot, ok = tracker.handleMessage(raw)
	require.True(t, ok)
	require.Equal(t, 4, snapshot.State.Step)
	require.Equal(t, "AA", snapshot.State.ProposerAddress)
}

func TestLateObserverSeedsExistingVotesBeforeReplayingEvents(t *testing.T) {
	var snapshotReads atomic.Int32
	var subscribed atomic.Bool
	peerDone := make(chan struct{})
	peerErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","voting_power":"60"},{"address":"BB","voting_power":"25"},{"address":"CC","voting_power":"15"}],"total":"3"}}`)
	})
	mux.HandleFunc("/consensus_state", func(w http.ResponseWriter, _ *http.Request) {
		snapshotReads.Add(1)
		if !subscribed.Load() {
			http.Error(w, "snapshot requested before subscription", http.StatusConflict)
			return
		}
		// The rounded textual percentage deliberately disagrees. Integer powers
		// and actual per-validator votes must determine the monitor percentage.
		_, _ = fmt.Fprint(w, `{"result":{"round_state":{"height/round/step":"42/0/6","height_vote_set":[{"round":0,"prevotes":["vote-a","nil-Vote","nil-Vote"],"precommits":["nil-Vote","vote-b","nil-Vote"],"prevotes_bit_array":"BA{3:x__} 60/100 = 0.59"}],"proposer":{"address":"AA","index":0}}}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			peerErrors <- err
			return
		}
		defer close(peerDone)
		defer conn.Close()
		requests, err := readTestSubscriptions(conn, 4)
		if err != nil {
			peerErrors <- err
			return
		}
		subscribed.Store(true)
		if err := writeTestWSAcks(conn, requests); err != nil {
			peerErrors <- err
			return
		}
		for _, index := range []int{0, 1, 1} {
			frame := fmt.Sprintf(`{"query":"tm.event='Vote'","data":{"value":{"Vote":{"type":1,"height":"42","round":0,"validator_index":%d}}}}`, index)
			if err := writeTestWSResult(conn, requests[0].ID, frame); err != nil {
				peerErrors <- err
				return
			}
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	subscription, err := NewClient(server.URL, server.URL).SubscribeConsensusState(ctx)
	require.NoError(t, err)
	select {
	case initial := <-subscription.Snapshots:
		require.True(t, initial.Reset)
		require.Equal(t, int64(60), initial.State.PrevotePower)
		require.Equal(t, .60, initial.State.PrevotePercent)
		require.Equal(t, .25, initial.State.PrecommitPercent)
	case <-time.After(3 * time.Second):
		t.Fatal("late observer did not receive existing votes")
	}
	for _, want := range []int64{60, 85, 85} {
		snapshot := receiveConsensusSnapshot(t, subscription.Snapshots, 3*time.Second)
		require.Equal(t, want, snapshot.State.PrevotePower)
		require.Equal(t, int64(100), snapshot.State.TotalVotingPower)
		require.Equal(t, int64(25), snapshot.State.PrecommitPower)
	}
	require.Equal(t, int32(1), snapshotReads.Load())
	cancel()
	requireDoneClosed(t, subscription.Done(), 3*time.Second)
	<-peerDone
	select {
	case err := <-peerErrors:
		t.Fatal(err)
	default:
	}
}

func TestCommittedBlockPreservesChainTimestamp(t *testing.T) {
	stamp := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	raw, err := cmtjson.Marshal(ctypes.ResultEvent{
		Query: "tm.event='NewBlock'",
		Data: cmtypes.EventDataNewBlock{
			Block: &cmtypes.Block{
				Header: cmtypes.Header{
					Height: 42,
					Time:   stamp,
				},
			},
		},
	})
	require.NoError(t, err)
	snapshot, ok := newConsensusTracker(nil).handleMessage(raw)
	require.True(t, ok)
	require.Equal(t, int64(42), snapshot.Block.Height)
	require.Equal(t, stamp, snapshot.Block.Time)
}

func TestConnectionWaitsForHeightSpecificBootstrapReadiness(t *testing.T) {
	var heightReads atomic.Int32
	closed := make(chan struct{}, 2)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	registerEmptyConsensusSnapshot(mux)
	mux.HandleFunc("/validators", func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("height") == "42" && heightReads.Add(1) == 1 {
			http.Error(w, "height not ready during restart", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","voting_power":"100"}],"total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
			closed <- struct{}{}
		}()
		requests, err := readTestSubscriptions(conn, 4)
		if err != nil {
			return
		}
		if err := writeTestWSAcks(conn, requests); err != nil {
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client := NewClient(server.URL, server.URL)
	subscription, err := client.SubscribeConsensusState(t.Context())
	require.ErrorContains(t, err, "bootstrap consensus at height 42")
	require.Nil(t, subscription)
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("failed bootstrap retained its socket")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	subscription, err = client.SubscribeConsensusState(ctx)
	require.NoError(t, err)
	cancel()
	requireDoneClosed(t, subscription.Done(), 3*time.Second)
	require.Equal(t, int32(2), heightReads.Load())
}
