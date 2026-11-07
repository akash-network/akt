package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	cmtypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	deployment "pkg.akt.dev/go/node/deployment/v1"
)

type eventStreamRPC struct {
	blockRPCProbe
	events       chan coretypes.ResultEvent
	subscribed   chan string
	unsubscribed chan string
}

func (rpc *eventStreamRPC) Subscribe(_ context.Context, subscriber, query string, _ ...int) (<-chan coretypes.ResultEvent, error) {
	rpc.subscribed <- subscriber + ":" + query
	return rpc.events, nil
}

func (rpc *eventStreamRPC) Unsubscribe(context.Context, string, string) error { return nil }

func (rpc *eventStreamRPC) UnsubscribeAll(_ context.Context, subscriber string) error {
	rpc.unsubscribed <- subscriber
	return nil
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
			node := &eventStreamRPC{
				blockRPCProbe: blockRPCProbe{
					resultsResponse: &coretypes.ResultBlockResults{
						TxsResults: []*abci.ExecTxResult{{Events: []abci.Event{abci.Event(event)}}},
					},
				},
				events:       make(chan coretypes.ResultEvent, 1),
				subscribed:   make(chan string, 1),
				unsubscribed: make(chan string, 1),
			}
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
			go func() { done <- executeBlockCommandOnContext(t, ctx, cmd, node, writer) }()
			select {
			case subscription := <-node.subscribed:
				require.Equal(t, "akash-cli-blk-hdr:tm.event = 'NewBlockHeader'", subscription)
			case <-time.After(5 * time.Second):
				t.Fatal("events command did not subscribe")
			}
			if mode == "idle cancellation" {
				cancel()
			} else {
				node.events <- coretypes.ResultEvent{Data: cmtypes.EventDataNewBlockHeader{
					Header: cmtypes.Header{Height: 42},
				}}
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
			case subscriber := <-node.unsubscribed:
				require.Equal(t, "akash-cli-blk-hdr", subscriber)
			case <-time.After(5 * time.Second):
				t.Fatal("events command did not unsubscribe")
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
