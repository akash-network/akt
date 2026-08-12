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
)

func TestSubscribeConsensusStateRestoresSubscriptionsAfterReconnect(t *testing.T) {
	const timeout = 8 * time.Second

	var connections atomic.Int32
	var validatorRequests atomic.Int32
	validatorHeights := make(chan string, 5)
	firstSubscriptions := make(chan []testWSRequest, 1)
	secondSubscriptions := make(chan []testWSRequest, 1)
	dropFirstConnection := make(chan struct{})
	handlerDone := make(chan struct{}, 2)
	peerErrors := make(chan error, 4)
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, r *http.Request) {
		validatorRequests.Add(1)
		validatorHeights <- r.URL.Query().Get("height")
		if page := r.URL.Query().Get("page"); page != "1" {
			reportWSPeerError(peerErrors, "validator page = %q, want 1", page)
			http.Error(w, "unexpected validator page", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"type":"tendermint/PubKeyEd25519","value":"key-a"},"voting_power":"10"},{"address":"BB","pub_key":{"type":"tendermint/PubKeyEd25519","value":"key-b"},"voting_power":"20"}],"count":"2","total":"2"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
			handlerDone <- struct{}{}
		}()

		connection := connections.Add(1)
		requests, err := readTestSubscriptions(conn, 2)
		if err != nil {
			reportWSPeerError(peerErrors, "connection %d: %v", connection, err)
			return
		}

		switch connection {
		case 1:
			firstSubscriptions <- requests
			if err := writeInitialConsensusFrames(conn, requests); err != nil {
				reportWSPeerError(peerErrors, "write initial consensus frames: %v", err)
				return
			}
			select {
			case <-dropFirstConnection:
			case <-time.After(timeout):
				reportWSPeerError(peerErrors, "timed out waiting to drop first connection")
				return
			}
			// Close the TCP connection without a WebSocket close frame so the
			// client treats it as a transport failure and reconnects.
			_ = conn.UnderlyingConn().Close()
		case 2:
			secondSubscriptions <- requests
			if err := writeForwardRoundVoteFrame(conn, requests); err != nil {
				reportWSPeerError(peerErrors, "write forward-round vote frame: %v", err)
				return
			}

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		default:
			reportWSPeerError(peerErrors, "unexpected connection %d", connection)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	subscription, err := NewClient(server.URL, server.URL).SubscribeConsensusState(ctx)
	require.NoError(t, err)
	snapshots := subscription.Snapshots

	wantQueries := []string{"tm.event='Vote'", "tm.event='NewRoundStep'"}
	require.Equal(t, wantQueries, testWSQueries(receiveTestSubscriptions(t, firstSubscriptions, timeout)))

	initialStep := receiveConsensusSnapshot(t, snapshots, timeout)
	require.Equal(t, int64(42), initialStep.State.Height)
	require.Zero(t, initialStep.State.Round)
	require.Equal(t, 3, initialStep.State.Step)
	require.Zero(t, initialStep.State.PrevoteCount)

	initialVote := receiveConsensusSnapshot(t, snapshots, timeout)
	require.Zero(t, initialVote.State.Round)
	require.Equal(t, 1, initialVote.State.PrevoteCount)
	require.Equal(t, int64(10), initialVote.State.PrevotePower)
	require.True(t, initialVote.State.Validators[0].Prevoted)

	close(dropFirstConnection)
	require.Equal(t, wantQueries, testWSQueries(receiveTestSubscriptions(t, secondSubscriptions, timeout)))

	vote := receiveConsensusSnapshot(t, snapshots, timeout)
	require.Equal(t, int64(42), vote.State.Height)
	require.Equal(t, 1, vote.State.Round)
	require.Zero(t, vote.State.Step)
	require.Equal(t, 1, vote.State.PrevoteCount)
	require.Equal(t, int64(20), vote.State.PrevotePower)
	require.InDelta(t, 2.0/3.0, vote.State.PrevotePercent, 0.0001)
	require.False(t, vote.State.Validators[0].Prevoted)
	require.True(t, vote.State.Validators[1].Prevoted)

	cancel()
	requireChannelClosed(t, snapshots, timeout)
	requireDoneClosed(t, subscription.Done(), timeout)
	for range 2 {
		select {
		case <-handlerDone:
		case <-time.After(timeout):
			t.Fatal("websocket peer did not shut down")
		}
	}
	require.Equal(t, int32(2), connections.Load())
	require.Equal(t, int32(4), validatorRequests.Load())
	require.Equal(t, "", <-validatorHeights, "connection setup must probe the latest set")
	require.Equal(t, "42", <-validatorHeights, "the first event must bind to its exact validator height")
	require.Equal(t, "", <-validatorHeights, "reconnect setup must refresh the latest set")
	require.Equal(t, "42", <-validatorHeights, "the reconnected event must bind to its exact validator height")
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

func TestSubscribeConsensusStateStopsAfterCanceledReconnect(t *testing.T) {
	const timeout = 5 * time.Second

	var connections atomic.Int32
	firstSubscribed := make(chan struct{})
	dropFirstConnection := make(chan struct{})
	secondConnected := make(chan struct{})
	handlerDone := make(chan struct{}, 2)
	peerErrors := make(chan error, 2)
	upgrader := websocket.Upgrader{}
	var cancel context.CancelFunc

	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"value":"key-a"},"voting_power":"10"}],"count":"1","total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
			handlerDone <- struct{}{}
		}()

		switch connections.Add(1) {
		case 1:
			requests, err := readTestSubscriptions(conn, 2)
			if err != nil {
				reportWSPeerError(peerErrors, "initial subscriptions: %v", err)
				return
			}
			if err := writeTestWSAcks(conn, requests); err != nil {
				reportWSPeerError(peerErrors, "initial acknowledgements: %v", err)
				return
			}
			close(firstSubscribed)
			select {
			case <-dropFirstConnection:
			case <-time.After(timeout):
				reportWSPeerError(peerErrors, "timed out waiting to drop first connection")
				return
			}
			_ = conn.UnderlyingConn().Close()
		case 2:
			close(secondConnected)
			cancel()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		default:
			reportWSPeerError(peerErrors, "unexpected reconnect")
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancelContext := context.WithCancel(context.Background())
	cancel = cancelContext
	subscription, err := NewClient(server.URL, server.URL).SubscribeConsensusState(ctx)
	require.NoError(t, err)
	snapshots := subscription.Snapshots

	select {
	case <-firstSubscribed:
	case <-time.After(timeout):
		t.Fatal("initial subscriptions were not established")
	}
	close(dropFirstConnection)
	select {
	case <-secondConnected:
	case <-time.After(timeout):
		t.Fatal("websocket did not reconnect")
	}
	requireChannelClosed(t, snapshots, timeout)
	requireDoneClosed(t, subscription.Done(), timeout)
	for range 2 {
		select {
		case <-handlerDone:
		case <-time.After(timeout):
			t.Fatal("websocket peer did not shut down")
		}
	}
	require.Equal(t, int32(2), connections.Load())
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

func TestSubscribeConsensusStateCancelsDuringReconnectBackoff(t *testing.T) {
	const timeout = 3 * time.Second

	var connections atomic.Int32
	firstClosed := make(chan struct{})
	peerDone := make(chan struct{})
	peerErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"value":"key-a"},"voting_power":"10"}],"count":"1","total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		if got := connections.Add(1); got != 1 {
			reportWSPeerError(peerErrors, "unexpected reconnect %d", got)
			return
		}
		requests, err := readTestSubscriptions(conn, 2)
		if err != nil {
			reportWSPeerError(peerErrors, "read subscriptions: %v", err)
			return
		}
		if err := writeTestWSAcks(conn, requests); err != nil {
			reportWSPeerError(peerErrors, "write acknowledgements: %v", err)
			return
		}
		_ = conn.UnderlyingConn().Close()
		close(firstClosed)
		close(peerDone)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	subscription, err := NewClient(server.URL, server.URL).SubscribeConsensusState(ctx)
	require.NoError(t, err)
	select {
	case <-firstClosed:
	case <-time.After(timeout):
		t.Fatal("peer did not close the initial transport")
	}
	select {
	case <-subscription.Done():
		t.Fatal("subscription closed instead of entering reconnect backoff")
	case <-time.After(consensusReconnectDelay / 2):
	}
	cancel()
	requireChannelClosed(t, subscription.Snapshots, timeout)
	requireDoneClosed(t, subscription.Done(), timeout)
	select {
	case <-peerDone:
	case <-time.After(timeout):
		t.Fatal("websocket peer did not shut down")
	}
	require.Equal(t, int32(1), connections.Load())
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

func TestSubscribeConsensusStateRefreshesValidatorsBeforeNewHeight(t *testing.T) {
	const timeout = 5 * time.Second

	var validatorRequests atomic.Int32
	validatorHeights := make(chan string, 4)
	peerDone := make(chan struct{})
	peerErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, r *http.Request) {
		if page := r.URL.Query().Get("page"); page != "1" {
			reportWSPeerError(peerErrors, "validator page = %q, want 1", page)
			http.Error(w, "unexpected validator page", http.StatusBadRequest)
			return
		}
		validatorRequests.Add(1)
		height := r.URL.Query().Get("height")
		validatorHeights <- height
		switch height {
		case "", "42":
			_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"value":"key-a"},"voting_power":"10"},{"address":"BB","pub_key":{"value":"key-b"},"voting_power":"20"}],"count":"2","total":"2"}}`)
		case "43":
			_, _ = fmt.Fprint(w, `{"result":{"block_height":"43","validators":[{"address":"CC","pub_key":{"value":"key-c"},"voting_power":"10"},{"address":"DD","pub_key":{"value":"key-d"},"voting_power":"90"}],"count":"2","total":"2"}}`)
		default:
			http.Error(w, "unexpected validator height", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
			close(peerDone)
			return
		}
		defer close(peerDone)
		defer conn.Close()

		requests, err := readTestSubscriptions(conn, 2)
		if err != nil {
			reportWSPeerError(peerErrors, "read subscriptions: %v", err)
			return
		}
		if err := writeInitialConsensusFrames(conn, requests); err != nil {
			reportWSPeerError(peerErrors, "write height 42 frames: %v", err)
			return
		}
		if err := writeTestWSResult(conn, requests[1].ID, `{"query":"tm.event='NewRoundStep'","data":{"value":{"height":"43","round":0,"step":1}}}`); err != nil {
			reportWSPeerError(peerErrors, "write height 43 step: %v", err)
			return
		}
		if err := writeTestWSResult(conn, requests[0].ID, `{"query":"tm.event='Vote'","data":{"value":{"Vote":{"type":1,"height":"43","round":0,"validator_index":1}}}}`); err != nil {
			reportWSPeerError(peerErrors, "write height 43 vote: %v", err)
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

	ctx, cancel := context.WithCancel(context.Background())
	subscription, err := NewClient(server.URL, server.URL).SubscribeConsensusState(ctx)
	require.NoError(t, err)
	snapshots := subscription.Snapshots

	_ = receiveConsensusSnapshot(t, snapshots, timeout)
	initialVote := receiveConsensusSnapshot(t, snapshots, timeout)
	require.Equal(t, int64(30), initialVote.State.TotalVotingPower)
	require.Equal(t, "AA", initialVote.State.Validators[0].Address)

	newHeight := receiveConsensusSnapshot(t, snapshots, timeout)
	require.Equal(t, int64(43), newHeight.State.Height)
	require.Equal(t, int64(100), newHeight.State.TotalVotingPower)
	require.Equal(t, "CC", newHeight.State.Validators[0].Address)
	require.Equal(t, "DD", newHeight.State.Validators[1].Address)
	require.Zero(t, newHeight.State.PrevotePower)

	newVote := receiveConsensusSnapshot(t, snapshots, timeout)
	require.Equal(t, int64(90), newVote.State.PrevotePower)
	require.InDelta(t, 0.9, newVote.State.PrevotePercent, 0.0001)
	require.False(t, newVote.State.Validators[0].Prevoted)
	require.True(t, newVote.State.Validators[1].Prevoted)
	require.Equal(t, int32(3), validatorRequests.Load())
	require.Equal(t, "", <-validatorHeights)
	require.Equal(t, "42", <-validatorHeights)
	require.Equal(t, "43", <-validatorHeights)

	cancel()
	requireChannelClosed(t, snapshots, timeout)
	requireDoneClosed(t, subscription.Done(), timeout)
	select {
	case <-peerDone:
	case <-time.After(timeout):
		t.Fatal("websocket peer did not shut down")
	}
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

func TestSubscribeConsensusStateClosesBeforeUsingStaleValidators(t *testing.T) {
	const timeout = 5 * time.Second

	var validatorRequests atomic.Int32
	validatorHeights := make(chan string, 4)
	peerDone := make(chan struct{})
	peerErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, r *http.Request) {
		validatorRequests.Add(1)
		height := r.URL.Query().Get("height")
		validatorHeights <- height
		if height == "43" {
			http.Error(w, "validator set unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"value":"key-a"},"voting_power":"10"}],"count":"1","total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
			close(peerDone)
			return
		}
		defer close(peerDone)
		defer conn.Close()

		requests, err := readTestSubscriptions(conn, 2)
		if err != nil {
			reportWSPeerError(peerErrors, "read subscriptions: %v", err)
			return
		}
		if err := writeInitialConsensusFrames(conn, requests); err != nil {
			reportWSPeerError(peerErrors, "write initial frames: %v", err)
			return
		}
		if err := writeTestWSResult(conn, requests[1].ID, `{"query":"tm.event='NewRoundStep'","data":{"value":{"height":"43","round":0,"step":1}}}`); err != nil {
			reportWSPeerError(peerErrors, "write higher-height frame: %v", err)
			return
		}

		_, _, _ = conn.ReadMessage()
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	subscription, err := NewClient(server.URL, server.URL).SubscribeConsensusState(context.Background())
	require.NoError(t, err)
	snapshots := subscription.Snapshots
	_ = receiveConsensusSnapshot(t, snapshots, timeout)
	_ = receiveConsensusSnapshot(t, snapshots, timeout)
	requireChannelClosed(t, snapshots, timeout)
	requireDoneClosed(t, subscription.Done(), timeout)
	require.Equal(t, int32(3), validatorRequests.Load())
	require.Equal(t, "", <-validatorHeights)
	require.Equal(t, "42", <-validatorHeights)
	require.Equal(t, "43", <-validatorHeights)

	select {
	case <-peerDone:
	case <-time.After(timeout):
		t.Fatal("websocket peer did not shut down")
	}
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

func TestSubscribeConsensusStateReportsSetupFailuresAndStopsPeer(t *testing.T) {
	t.Run("invalid endpoint", func(t *testing.T) {
		_, err := NewClient("://bad endpoint", "http://rest.example").SubscribeConsensusState(context.Background())
		require.ErrorContains(t, err, "create websocket client")
	})

	t.Run("websocket upgrade", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(server.Close)

		_, err := NewClient(server.URL, server.URL).SubscribeConsensusState(context.Background())
		require.ErrorContains(t, err, "websocket connect")
	})

	t.Run("validator failure closes websocket", func(t *testing.T) {
		peerDone := make(chan struct{})
		peerErrors := make(chan error, 1)
		upgrader := websocket.Upgrader{}
		mux := http.NewServeMux()
		mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		})
		mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
				close(peerDone)
				return
			}
			defer close(peerDone)
			defer conn.Close()
			_, _, _ = conn.ReadMessage()
		})

		server := httptest.NewServer(mux)
		t.Cleanup(server.Close)

		_, err := NewClient(server.URL, server.URL).SubscribeConsensusState(context.Background())
		require.ErrorContains(t, err, "load validators")
		select {
		case <-peerDone:
		case <-time.After(3 * time.Second):
			t.Fatal("websocket peer remained open after validator failure")
		}
		select {
		case peerErr := <-peerErrors:
			t.Fatal(peerErr)
		default:
		}
	})

	t.Run("canceled initial subscription closes websocket", func(t *testing.T) {
		peerDone := make(chan struct{})
		peerErrors := make(chan error, 1)
		upgrader := websocket.Upgrader{}
		ctx, cancel := context.WithCancel(context.Background())
		mux := http.NewServeMux()
		mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
			cancel()
			_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"value":"key-a"},"voting_power":"10"}],"count":"1","total":"1"}}`)
		})
		mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
				close(peerDone)
				return
			}
			defer close(peerDone)
			defer conn.Close()
			_, _, _ = conn.ReadMessage()
		})

		server := httptest.NewServer(mux)
		t.Cleanup(server.Close)

		_, err := NewClient(server.URL, server.URL).SubscribeConsensusState(ctx)
		require.ErrorIs(t, err, context.Canceled)
		select {
		case <-peerDone:
		case <-time.After(3 * time.Second):
			t.Fatal("websocket peer remained open after subscription failure")
		}
		select {
		case peerErr := <-peerErrors:
			t.Fatal(peerErr)
		default:
		}
	})
}

func TestSubscribeConsensusStateRejectsServerSubscriptionError(t *testing.T) {
	peerDone := make(chan struct{})
	peerErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"value":"key-a"},"voting_power":"10"}],"count":"1","total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
			close(peerDone)
			return
		}
		defer close(peerDone)
		defer conn.Close()

		requests, err := readTestSubscriptions(conn, 1)
		if err != nil {
			reportWSPeerError(peerErrors, "read rejected subscription: %v", err)
			return
		}
		if err := conn.WriteJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      requests[0].ID,
			"error": map[string]any{
				"code":    -32000,
				"message": "subscription limit reached",
			},
		}); err != nil {
			reportWSPeerError(peerErrors, "write rejected subscription: %v", err)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	_, err := NewClient(server.URL, server.URL).SubscribeConsensusState(context.Background())
	require.ErrorContains(t, err, "subscribe to Vote events")
	require.ErrorContains(t, err, "subscription limit reached")

	select {
	case <-peerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket peer remained open after second subscription failure")
	}
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

func TestSubscribeConsensusStateRejectsMalformedSubscriptionAcknowledgement(t *testing.T) {
	peerDone := make(chan struct{})
	peerErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"value":"key-a"},"voting_power":"10"}],"count":"1","total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
			close(peerDone)
			return
		}
		defer close(peerDone)
		defer conn.Close()

		requests, err := readTestSubscriptions(conn, 2)
		if err != nil {
			reportWSPeerError(peerErrors, "read subscriptions: %v", err)
			return
		}
		if err := writeTestWSResult(conn, requests[0].ID, `{"unexpected":true}`); err != nil {
			reportWSPeerError(peerErrors, "write malformed acknowledgement: %v", err)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	_, err := NewClient(server.URL, server.URL).SubscribeConsensusState(context.Background())
	require.ErrorContains(t, err, "malformed acknowledgement")

	select {
	case <-peerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket peer remained open after malformed acknowledgement")
	}
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

func TestSubscribeConsensusStatePreservesEventBeforeAcknowledgements(t *testing.T) {
	const timeout = 3 * time.Second

	peerDone := make(chan struct{})
	peerErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","pub_key":{"value":"key-a"},"voting_power":"10"}],"count":"1","total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportWSPeerError(peerErrors, "upgrade websocket: %v", err)
			close(peerDone)
			return
		}
		defer close(peerDone)
		defer conn.Close()

		requests, err := readTestSubscriptions(conn, 2)
		if err != nil {
			reportWSPeerError(peerErrors, "read subscriptions: %v", err)
			return
		}
		if err := writeTestWSResult(conn, requests[0].ID, `{"query":"tm.event='Vote'","data":{"value":{"Vote":{"type":1,"height":"42","round":0,"validator_index":0}}}}`); err != nil {
			reportWSPeerError(peerErrors, "write early event: %v", err)
			return
		}
		if err := writeTestWSAcks(conn, requests); err != nil {
			reportWSPeerError(peerErrors, "write acknowledgements: %v", err)
			return
		}

		_, _, _ = conn.ReadMessage()
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	subscription, err := NewClient(server.URL, server.URL).SubscribeConsensusState(ctx)
	require.NoError(t, err)
	snapshot := receiveConsensusSnapshot(t, subscription.Snapshots, timeout)
	require.Equal(t, int64(42), snapshot.State.Height)
	require.Equal(t, 1, snapshot.State.PrevoteCount)

	cancel()
	requireChannelClosed(t, subscription.Snapshots, timeout)
	requireDoneClosed(t, subscription.Done(), timeout)
	select {
	case <-peerDone:
	case <-time.After(timeout):
		t.Fatal("websocket peer did not shut down")
	}
	select {
	case peerErr := <-peerErrors:
		t.Fatal(peerErr)
	default:
	}
}

type testWSRequest struct {
	ID     int               `json:"id"`
	Method string            `json:"method"`
	Params map[string]string `json:"params"`
}

func readTestSubscriptions(conn *websocket.Conn, count int) ([]testWSRequest, error) {
	requests := make([]testWSRequest, 0, count)
	for range count {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("read subscription: %w", err)
		}
		var request testWSRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, fmt.Errorf("parse subscription: %w", err)
		}
		if request.Method != "subscribe" {
			return nil, fmt.Errorf("method = %q, want subscribe", request.Method)
		}
		if request.Params["query"] == "" {
			return nil, fmt.Errorf("subscription query is empty")
		}
		requests = append(requests, request)
	}

	return requests, nil
}

func writeInitialConsensusFrames(conn *websocket.Conn, requests []testWSRequest) error {
	if err := writeTestWSAcks(conn, requests); err != nil {
		return err
	}
	if err := writeTestWSResult(conn, requests[1].ID, `{"query":"tm.event='NewRoundStep'","data":{"value":{"height":"42","round":0,"step":3}}}`); err != nil {
		return err
	}
	return writeTestWSResult(conn, requests[0].ID, `{"query":"tm.event='Vote'","data":{"value":{"Vote":{"type":1,"height":"42","round":0,"validator_index":0}}}}`)
}

func writeForwardRoundVoteFrame(conn *websocket.Conn, requests []testWSRequest) error {
	if err := writeTestWSAcks(conn, requests); err != nil {
		return err
	}
	return writeTestWSResult(conn, requests[0].ID, `{"query":"tm.event='Vote'","data":{"value":{"Vote":{"type":1,"height":"42","round":1,"validator_index":1}}}}`)
}

func writeTestWSAcks(conn *websocket.Conn, requests []testWSRequest) error {
	for _, request := range requests {
		if err := writeTestWSResult(conn, request.ID, `{}`); err != nil {
			return err
		}
	}
	return nil
}

func writeTestWSResult(conn *websocket.Conn, id int, result string) error {
	return conn.WriteJSON(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Result:  json.RawMessage(result),
	})
}

func testWSQueries(requests []testWSRequest) []string {
	queries := make([]string, len(requests))
	for i, request := range requests {
		queries[i] = request.Params["query"]
	}
	return queries
}

func receiveTestSubscriptions(t *testing.T, subscriptions <-chan []testWSRequest, timeout time.Duration) []testWSRequest {
	t.Helper()
	select {
	case requests := <-subscriptions:
		return requests
	case <-time.After(timeout):
		t.Fatal("timed out waiting for subscriptions")
		return nil
	}
}

func receiveConsensusSnapshot(t *testing.T, snapshots <-chan ConsensusSnapshot, timeout time.Duration) ConsensusSnapshot {
	t.Helper()
	select {
	case snapshot, ok := <-snapshots:
		require.True(t, ok, "consensus snapshot channel closed early")
		return snapshot
	case <-time.After(timeout):
		t.Fatal("timed out waiting for consensus snapshot")
		return ConsensusSnapshot{}
	}
}

func requireChannelClosed(t *testing.T, snapshots <-chan ConsensusSnapshot, timeout time.Duration) {
	t.Helper()
	select {
	case _, ok := <-snapshots:
		require.False(t, ok, "consensus snapshot channel remained open")
	case <-time.After(timeout):
		t.Fatal("timed out waiting for consensus snapshot channel to close")
	}
}

func requireDoneClosed(t *testing.T, done <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("consensus producer remained active")
	}
}

func reportWSPeerError(errors chan<- error, format string, args ...any) {
	select {
	case errors <- fmt.Errorf(format, args...):
	default:
	}
}
