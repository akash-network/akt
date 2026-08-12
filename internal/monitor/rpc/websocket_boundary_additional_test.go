package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/monitor/consensus"
)

func TestConsensusTransportErrorPreservesCause(t *testing.T) {
	want := errors.New("connection reset")
	err := consensusTransportError{err: want}

	require.EqualError(t, err, want.Error())
	require.ErrorIs(t, err, want)
	require.True(t, isConsensusTransportError(fmt.Errorf("read consensus: %w", err)))
	require.False(t, isConsensusTransportError(want))
}

func TestWaitForConsensusReconnectHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	require.False(t, waitForConsensusReconnect(ctx))
	require.Less(t, time.Since(started), consensusReconnectDelay)
}

func TestConsensusMessageHeightRejectsMalformedBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantHeight int64
		wantOK     bool
	}{
		{name: "malformed envelope", raw: "{"},
		{name: "missing data", raw: `{"query":"tm.event='Vote'"}`},
		{name: "malformed vote", raw: `{"query":"tm.event='Vote'","data":"bad"}`},
		{name: "malformed round step", raw: `{"query":"tm.event='NewRoundStep'","data":"bad"}`},
		{name: "unknown event", raw: `{"query":"tm.event='NewBlock'","data":{"value":{"height":"7"}}}`},
		{name: "malformed height", raw: `{"query":"tm.event='Vote'","data":{"value":{"Vote":{"height":"seven"}}}}`},
		{name: "negative height", raw: `{"query":"tm.event='NewRoundStep'","data":{"value":{"height":"-1"}}}`},
		{name: "vote", raw: `{"query":"tm.event='Vote'","data":{"value":{"Vote":{"height":"7"}}}}`, wantHeight: 7, wantOK: true},
		{name: "round step", raw: `{"query":"tm.event='NewRoundStep'","data":{"value":{"height":"8"}}}`, wantHeight: 8, wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			height, ok := consensusMessageHeight(json.RawMessage(test.raw))
			require.Equal(t, test.wantOK, ok)
			require.Equal(t, test.wantHeight, height)
		})
	}
}

func TestConsensusWebSocketURLValidatesTransportAndPreservesEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
		wantErr  string
	}{
		{endpoint: "http://rpc.example.test:26657", want: "ws://rpc.example.test:26657/websocket"},
		{endpoint: "https://rpc.example.test/rpc/", want: "wss://rpc.example.test/rpc/websocket"},
		{endpoint: "ws://rpc.example.test/custom", want: "ws://rpc.example.test/custom/websocket"},
		{endpoint: "wss://rpc.example.test/custom?token=public", want: "wss://rpc.example.test/custom/websocket?token=public"},
		{endpoint: "ftp://rpc.example.test", wantErr: `unsupported RPC scheme "ftp"`},
	}

	for _, test := range tests {
		t.Run(test.endpoint, func(t *testing.T) {
			got, err := consensusWebSocketURL(test.endpoint)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestConsensusResultClassifiersRejectMalformedJSON(t *testing.T) {
	require.False(t, isConsensusSubscriptionAck(json.RawMessage(`{`)))
	require.False(t, isConsensusSubscriptionAck(json.RawMessage(`null`)))
	require.True(t, isConsensusSubscriptionAck(json.RawMessage(`{}`)))

	require.False(t, isConsensusEventResult(json.RawMessage(`{`)))
	require.False(t, isConsensusEventResult(json.RawMessage(`{"query":"tm.event='Vote'","data":null}`)))
	require.True(t, isConsensusEventResult(json.RawMessage(`{"query":"tm.event='Vote'","data":{"value":{}}}`)))
}

func TestSubscribeConsensusEventsRejectsDeadTransport(t *testing.T) {
	ws, peerDone := openConsensusBoundarySocket(t, func(conn *websocket.Conn) error {
		return conn.Close()
	})
	<-peerDone
	require.NoError(t, ws.Close())

	_, err := subscribeConsensusEvents(context.Background(), ws)
	require.ErrorContains(t, err, "subscribe to Vote events")
}

func TestSubscribeConsensusEventsRejectsReadDeadlineFailure(t *testing.T) {
	ws, peerDone := openConsensusBoundarySocket(t, func(conn *websocket.Conn) error {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return err
			}
		}
	})
	require.NoError(t, ws.Close())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := subscribeConsensusEvents(ctx, ws)
	require.ErrorContains(t, err, "set subscription read deadline")
	<-peerDone
}

func TestSubscribeConsensusEventsPropagatesDeadlineSetupFailures(t *testing.T) {
	writeErr := errors.New("write deadline rejected")
	readErr := errors.New("read deadline rejected")

	tests := []struct {
		name   string
		socket *consensusDeadlineSocket
		want   error
	}{
		{
			name:   "write deadline",
			socket: &consensusDeadlineSocket{writeDeadlineErr: writeErr},
			want:   writeErr,
		},
		{
			name:   "read deadline",
			socket: &consensusDeadlineSocket{readDeadlineErr: readErr},
			want:   readErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := subscribeConsensusEvents(ctx, test.socket)
			require.ErrorIs(t, err, test.want)
			require.Empty(t, test.socket.writes, "subscription writes must not begin after deadline setup fails")
		})
	}
}

func TestSubscribeConsensusEventsRejectsAcknowledgementFailures(t *testing.T) {
	tests := []struct {
		name string
		peer func(*websocket.Conn) error
		want string
	}{
		{
			name: "connection closes before acknowledgement",
			peer: func(conn *websocket.Conn) error {
				if _, err := readTestSubscriptions(conn, 2); err != nil {
					return err
				}
				return conn.Close()
			},
			want: "await subscription acknowledgement",
		},
		{
			name: "unexpected response identity",
			peer: func(conn *websocket.Conn) error {
				if _, err := readTestSubscriptions(conn, 2); err != nil {
					return err
				}
				return conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": 99, "result": map[string]any{}})
			},
			want: "unexpected response id 99",
		},
		{
			name: "server error includes detail",
			peer: func(conn *websocket.Conn) error {
				requests, err := readTestSubscriptions(conn, 2)
				if err != nil {
					return err
				}
				return conn.WriteJSON(map[string]any{
					"jsonrpc": "2.0",
					"id":      requests[0].ID,
					"error": map[string]any{
						"code": -32000, "message": "subscription rejected", "data": "limit=1",
					},
				})
			},
			want: "subscription rejected: limit=1",
		},
		{
			name: "duplicate acknowledgement",
			peer: func(conn *websocket.Conn) error {
				requests, err := readTestSubscriptions(conn, 2)
				if err != nil {
					return err
				}
				if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": requests[0].ID, "result": map[string]any{}}); err != nil {
					return err
				}
				return conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": requests[0].ID, "result": map[string]any{}})
			},
			want: "duplicate acknowledgement",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ws, peerDone := openConsensusBoundarySocket(t, test.peer)
			defer ws.Close()

			_, err := subscribeConsensusEvents(context.Background(), ws)
			require.ErrorContains(t, err, test.want)
			require.NoError(t, <-peerDone)
		})
	}
}

func TestSubscribeConsensusStateClosesOnPostAcknowledgementProtocolError(t *testing.T) {
	peerDone := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","voting_power":"10"}],"total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			peerDone <- err
			return
		}
		defer conn.Close()
		requests, err := readTestSubscriptions(conn, 2)
		if err == nil {
			err = writeTestWSAcks(conn, requests)
		}
		if err == nil {
			err = conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": requests[0].ID})
		}
		if err == nil {
			err = writeTestWSResult(conn, requests[0].ID, `{"query":"tm.event='Unknown'","data":{"value":{}}}`)
		}
		if err == nil {
			err = conn.WriteJSON(map[string]any{
				"jsonrpc": "2.0",
				"id":      requests[0].ID,
				"error":   map[string]any{"code": -32001, "message": "subscription revoked"},
			})
		}
		peerDone <- err
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	subscription, err := NewClient(server.URL, server.URL).SubscribeConsensusState(context.Background())
	require.NoError(t, err)
	requireChannelClosed(t, subscription.Snapshots, 3*time.Second)
	requireDoneClosed(t, subscription.Done(), 3*time.Second)
	require.NoError(t, <-peerDone)
}

func TestOpenConsensusConnectionCancellationInterruptsAcknowledgementWait(t *testing.T) {
	requestsRead := make(chan struct{})
	peerDone := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","voting_power":"10"}],"total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			peerDone <- err
			return
		}
		defer conn.Close()
		_, err = readTestSubscriptions(conn, 2)
		close(requestsRead)
		if err == nil {
			_, _, err = conn.ReadMessage()
		}
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) || strings.Contains(fmt.Sprint(err), "unexpected EOF") {
			err = nil
		}
		peerDone <- err
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client := NewClient(server.URL, server.URL)
	wsURL, err := consensusWebSocketURL(server.URL)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, _, err := client.openConsensusConnection(ctx, wsURL)
		result <- err
	}()
	<-requestsRead
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.NoError(t, <-peerDone)
}

func TestConsumeConsensusConnectionCancellationUnblocksFullSnapshotQueue(t *testing.T) {
	peerDone := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/validators", func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("height") != "42" {
			http.Error(w, "exact height required", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprint(w, `{"result":{"block_height":"42","validators":[{"address":"AA","voting_power":"10"}],"total":"1"}}`)
	})
	mux.HandleFunc("/websocket", func(w http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			peerDone <- err
			return
		}
		defer conn.Close()
		_, _, err = conn.ReadMessage()
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) || strings.Contains(fmt.Sprint(err), "unexpected EOF") {
			err = nil
		}
		peerDone <- err
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	wsURL, err := consensusWebSocketURL(server.URL)
	require.NoError(t, err)
	ws, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.NoError(t, err)

	buffered := []consensusWSResponse{
		{Result: json.RawMessage(`{"query":"tm.event='NewRoundStep'","data":{"value":{"height":"42","round":0,"step":1}}}`)},
		{Result: json.RawMessage(`{"query":"tm.event='NewRoundStep'","data":{"value":{"height":"42","round":0,"step":2}}}`)},
	}
	ctx, cancel := context.WithCancel(context.Background())
	snapshots := make(chan ConsensusSnapshot, 1)
	result := make(chan error, 1)
	go func() {
		result <- NewClient(server.URL, server.URL).consumeConsensusConnection(
			ctx,
			ws,
			[]consensus.Validator{{Address: "AA", VotingPower: "10"}},
			buffered,
			snapshots,
		)
	}()
	require.Eventually(t, func() bool { return len(snapshots) == 1 }, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.NoError(t, <-peerDone)
}

func openConsensusBoundarySocket(
	t *testing.T,
	peer func(*websocket.Conn) error,
) (*websocket.Conn, <-chan error) {
	t.Helper()
	peerDone := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			peerDone <- err
			return
		}
		defer conn.Close()
		peerDone <- peer(conn)
	}))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn, peerDone
}

type consensusDeadlineSocket struct {
	writeDeadlineErr error
	readDeadlineErr  error
	writes           []any
}

func (socket *consensusDeadlineSocket) SetWriteDeadline(time.Time) error {
	return socket.writeDeadlineErr
}

func (socket *consensusDeadlineSocket) SetReadDeadline(time.Time) error {
	return socket.readDeadlineErr
}

func (socket *consensusDeadlineSocket) WriteJSON(value any) error {
	socket.writes = append(socket.writes, value)
	return nil
}

func (*consensusDeadlineSocket) ReadJSON(any) error {
	return errors.New("unexpected acknowledgement read")
}
