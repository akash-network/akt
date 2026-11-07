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

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/monitor/consensus"
)

func TestConsensusBootstrapRejectsInvalidSnapshotAndClosesSocket(t *testing.T) {
	cases := []struct {
		name   string
		status int
		state  string
		want   string
	}{
		{name: "unavailable", status: http.StatusServiceUnavailable, want: "unexpected status code: 503"},
		{name: "height", state: "bad/0/1", want: "invalid syntax"},
		{name: "negative height", state: "-1/0/1", want: "validator height must be positive"},
		{name: "format", state: "42/0", want: "invalid height/round/step format"},
		{name: "round", state: "42/-1/1", want: "invalid round"},
		{name: "step", state: "42/0/bad", want: "invalid step"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			peerDone := make(chan error, 1)
			upgrader := websocket.Upgrader{}
			mux := http.NewServeMux()
			mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","voting_power":"100"}],"total":"1"}}`)
			})
			mux.HandleFunc("/consensus_state", func(w http.ResponseWriter, _ *http.Request) {
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = fmt.Fprintf(w, `{"result":{"round_state":{"height/round/step":%q}}}`, tc.state)
			})
			mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
				conn, err := upgrader.Upgrade(w, request, nil)
				if err != nil {
					peerDone <- err
					return
				}
				defer conn.Close()
				requests, err := readTestSubscriptions(conn, 4)
				if err == nil {
					err = writeTestWSAcks(conn, requests)
				}
				if err != nil {
					peerDone <- err
					return
				}
				_, _, err = conn.ReadMessage()
				if !websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
					peerDone <- fmt.Errorf("expected closed bootstrap socket, got %w", err)
					return
				}
				peerDone <- nil
			})
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)
			sub, err := NewClient(server.URL, server.URL).SubscribeConsensusState(t.Context())
			require.ErrorContains(t, err, "bootstrap consensus")
			require.ErrorContains(t, err, tc.want)
			require.Nil(t, sub)
			select {
			case err := <-peerDone:
				require.NoError(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("invalid bootstrap retained its WebSocket")
			}
		})
	}
}

func TestConsensusTrackerRejectsInvalidBlockAndProposerEvents(t *testing.T) {
	for _, raw := range []string{
		`{"query":"tm.event='NewBlock'","data":{"value":{"block":{"header":{"time":"bad"}}}}}`,
		`{"query":"tm.event='NewBlock'","data":{"value":{"block":{"header":{"height":"bad"}}}}}`,
		`{"query":"tm.event='NewBlock'","data":{"value":{"block":{"header":{"height":"0"}}}}}`,
		`{"query":"tm.event='NewBlock'","data":{"value":{"block":{"header":{"height":"42"}}}}}`,
		`{"query":"tm.event='NewRound'","data":{"value":{"height":"42","round":0,"step":"RoundStepNewRound","proposer":{"address":"AA","index":-1}}}}`,
		`{"query":"tm.event='NewRound'","data":{"value":{"height":"42","round":0,"step":"RoundStepNewRound","proposer":{"address":"AA","index":1}}}}`,
		`{"query":"tm.event='NewRound'","data":{"value":{"height":"42","round":0,"step":"RoundStepNewRound","proposer":{"address":"BB","index":0}}}}`,
	} {
		tracker := newConsensusTracker([]consensus.Validator{
			{Address: "AA", VotingPower: "100"},
		})
		snapshot, changed := tracker.handleMessage(json.RawMessage(raw))
		require.False(t, changed, raw)
		require.Empty(t, snapshot)
		require.False(t, tracker.initialized)
	}
}

// Observe a cancellation check without changing its result, so cancellation
// can be ordered after a peer failure even when the snapshot queue is full.
type observedConsensusContext struct {
	context.Context
	observe atomic.Bool
	checked chan struct{}
}

func (ctx *observedConsensusContext) Err() error {
	err := ctx.Context.Err()
	if ctx.observe.Load() {
		select {
		case ctx.checked <- struct{}{}:
		default:
		}
	}
	return err
}

func TestConsensusCancellationUnblocksErrorDeliveryToFullQueue(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &observedConsensusContext{
		Context: parent,
		checked: make(chan struct{}, 1),
	}
	failPeer := make(chan struct{})
	peerDone := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	registerEmptyConsensusSnapshot(mux)
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","voting_power":"100"}],"total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			peerDone <- err
			return
		}
		defer conn.Close()
		requests, err := readTestSubscriptions(conn, 4)
		if err == nil {
			err = writeTestWSAcks(conn, requests)
		}
		for i := 0; i < 15 && err == nil; i++ {
			err = writeTestWSResult(conn, requests[0].ID, `{"query":"tm.event='NewRoundStep'","data":{"value":{"height":"42","round":0,"step":"RoundStepNewRound"}}}`)
		}
		if err == nil {
			select {
			case <-failPeer:
				err = conn.WriteJSON(map[string]any{
					"jsonrpc": "2.0",
					"error":   map[string]any{"code": -32001, "message": "subscription revoked"},
				})
			case <-parent.Done():
			}
		}
		peerDone <- err
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	sub, err := NewClient(server.URL, server.URL).SubscribeConsensusState(ctx)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(sub.Snapshots) == cap(sub.Snapshots) }, 3*time.Second, time.Millisecond)
	ctx.observe.Store(true)
	close(failPeer)
	select {
	case <-ctx.checked:
	case <-time.After(3 * time.Second):
		t.Fatal("producer did not reach error delivery")
	}
	cancel()
	requireDoneClosed(t, sub.Done(), 3*time.Second)
	require.NoError(t, <-peerDone)
	for snapshot := range sub.Snapshots {
		require.NoError(t, snapshot.Err)
	}
}
