package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	cmtypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	deployment "pkg.akt.dev/go/node/deployment/v1"
)

type eventStreamRPC struct {
	url          string
	headers      chan int64
	subscribed   chan string
	disconnected chan struct{}
}

func newEventStreamRPC(t *testing.T, event abci.Event) *eventStreamRPC {
	t.Helper()
	node := &eventStreamRPC{
		headers:      make(chan int64, 1),
		subscribed:   make(chan string, 1),
		disconnected: make(chan struct{}),
	}
	fixtureCtx, cancelFixture := context.WithCancel(context.Background())
	var handlers sync.WaitGroup
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.Add(1)
		defer handlers.Done()
		if r.URL.Path != "/websocket" {
			var request rpctypes.RPCRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			var result any
			switch request.Method {
			case "status":
				result = &coretypes.ResultStatus{SyncInfo: coretypes.SyncInfo{
					LatestBlockHeight:   41,
					EarliestBlockHeight: 1,
				}}
			case "block_results":
				result = &coretypes.ResultBlockResults{
					Height:     42,
					TxsResults: []*abci.ExecTxResult{{Events: []abci.Event{event}}},
				}
			default:
				t.Errorf("unexpected RPC method %q", request.Method)
				return
			}
			_ = json.NewEncoder(w).Encode(rpctypes.NewRPCSuccessResponse(request.ID, result))
			return
		}

		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		stopClosing := context.AfterFunc(fixtureCtx, func() { _ = conn.Close() })
		defer stopClosing()
		defer conn.Close()
		var request rpctypes.RPCRequest
		if err := conn.ReadJSON(&request); err != nil {
			// The generic RPC client also opens an idle websocket.
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) && !errors.Is(err, net.ErrClosed) {
				t.Error(err)
			}
			return
		}
		var params struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Error(err)
			return
		}
		if request.Method != "subscribe" {
			t.Errorf("unexpected websocket method %q", request.Method)
			return
		}
		defer close(node.disconnected)
		if err := conn.WriteJSON(rpctypes.NewRPCSuccessResponse(request.ID, coretypes.ResultSubscribe{})); err != nil {
			t.Error(err)
			return
		}
		node.subscribed <- params.Query
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		select {
		case height := <-node.headers:
			result := coretypes.ResultEvent{
				Query: params.Query,
				Data: cmtypes.EventDataNewBlockHeader{
					Header: cmtypes.Header{Height: height},
				},
			}
			if err := conn.WriteJSON(rpctypes.NewRPCSuccessResponse(request.ID, result)); err != nil {
				t.Error(err)
				return
			}
		case <-closed:
			return
		case <-fixtureCtx.Done():
			return
		}
		select {
		case <-closed:
		case <-fixtureCtx.Done():
		}
	}))
	t.Cleanup(func() {
		cancelFixture()
		server.Close()
		handlers.Wait()
	})
	node.url = server.URL
	return node
}

type eventStreamWriter struct {
	bytes.Buffer
	written chan struct{}
	err     error
}

func (w *eventStreamWriter) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Buffer.Write(data)
	select {
	case w.written <- struct{}{}:
	default:
	}
	return n, err
}

func TestEventsStreamOutputAndCancellation(t *testing.T) {
	for _, mode := range []string{"pretty", "json", "yaml", "output failure", "idle cancellation"} {
		t.Run(mode, func(t *testing.T) {
			event, err := sdk.TypedEventToEvent(&deployment.EventDeploymentCreated{
				ID: deployment.DeploymentID{
					Owner: stateTestOwner,
					DSeq:  42,
				},
				Hash: []byte("deployment-hash"),
			})
			require.NoError(t, err)
			node := newEventStreamRPC(t, abci.Event(event))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &eventStreamWriter{written: make(chan struct{}, 1)}
			wantErr := errors.New("event destination closed")
			if mode == "output failure" {
				writer.err = wantErr
			}
			cmd := EventsCmd()
			cmd.Flags().String(flagdefs.FlagOutput, mode, "")
			done := make(chan blockCommandResult, 1)
			go func() { done <- executeBlockCommandOnContext(t, ctx, cmd, nil, writer, "--node", node.url) }()
			select {
			case subscription := <-node.subscribed:
				require.Equal(t, "tm.event = 'NewBlockHeader'", subscription)
			case result := <-done:
				t.Fatalf("events command ended before subscribing: %v", result.err)
			case <-time.After(5 * time.Second):
				t.Fatal("events command did not subscribe")
			}
			if mode == "idle cancellation" {
				cancel()
			} else {
				node.headers <- 42
				if mode != "output failure" {
					select {
					case <-writer.written:
						cancel()
					case <-time.After(5 * time.Second):
						t.Fatal("events command did not write the event")
					}
				}
			}
			select {
			case result := <-done:
				if mode == "output failure" {
					require.ErrorIs(t, result.err, wantErr)
				} else if mode == "idle cancellation" && result.err != nil {
					require.ErrorIs(t, result.err, context.Canceled)
				} else {
					require.NoError(t, result.err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("events command did not shut down")
			}
			select {
			case <-node.disconnected:
			case <-time.After(5 * time.Second):
				t.Fatal("events command did not close its websocket")
			}
			if mode == "output failure" || mode == "idle cancellation" {
				require.Empty(t, writer.String())
				return
			}
			var got deployment.EventDeploymentCreated
			if mode == "yaml" {
				var document map[string]any
				require.NoError(t, yaml.Unmarshal(writer.Bytes(), &document))
				data, err := json.Marshal(document)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(data, &got))
			} else {
				require.NoError(t, json.Unmarshal(writer.Bytes(), &got))
			}
			require.Equal(t, stateTestOwner, got.ID.Owner)
			require.EqualValues(t, 42, got.ID.DSeq)
			require.Equal(t, []byte("deployment-hash"), got.Hash)
		})
	}
}
