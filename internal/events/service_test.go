package events

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/boz/go-lifecycle"
	abci "github.com/cometbft/cometbft/abci/types"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmtypes "github.com/cometbft/cometbft/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	deployment "pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/util/pubsub"
)

type eventsClientStub struct {
	unsubscribed string
}

func (*eventsClientStub) Subscribe(context.Context, string, string, ...int) (<-chan ctypes.ResultEvent, error) {
	return nil, nil
}

func (*eventsClientStub) Unsubscribe(context.Context, string, string) error {
	return nil
}

func (stub *eventsClientStub) UnsubscribeAll(_ context.Context, subscriber string) error {
	stub.unsubscribed = subscriber
	return nil
}

func TestRunStopsWhenSubscriptionCloses(t *testing.T) {
	const subscriptionName = "test-subscription"

	client := &eventsClientStub{}

	service := &events{
		ctx:  context.Background(),
		ebus: client,
		lc:   lifecycle.New(),
	}

	subscription := make(chan ctypes.ResultEvent)
	close(subscription)

	result := make(chan error, 1)
	go func() {
		result <- service.run(subscriptionName, subscription, make(chan struct{}, 1))
	}()

	select {
	case err := <-result:
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), "subscription") && strings.Contains(err.Error(), "closed"),
			"error %q should identify the closed subscription", err)
		require.Equal(t, subscriptionName, client.unsubscribed)
	case <-time.After(500 * time.Millisecond):
		shutdownDone := make(chan struct{})
		go func() {
			service.lc.Shutdown(nil)
			close(shutdownDone)
		}()

		select {
		case <-result:
		case <-time.After(time.Second):
			t.Fatal("event service did not stop after forced test cleanup")
		}
		<-shutdownDone
		t.Fatal("event service did not stop when its subscription channel closed")
	}
}

type cometEventsStub struct {
	sdkclient.CometRPC

	subscription    chan ctypes.ResultEvent
	subscribeErr    error
	blockResults    *ctypes.ResultBlockResults
	blockResultsErr error
	blockCalls      chan int64
	unsubscribed    chan string
}

type flushAwareCometEventsStub struct {
	*cometEventsStub
	flushStarted chan struct{}
	releaseFlush chan struct{}
}

func (stub *flushAwareCometEventsStub) FlushUnsubscribe(ctx context.Context) error {
	close(stub.flushStarted)
	select {
	case <-stub.releaseFlush:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (stub *cometEventsStub) Subscribe(context.Context, string, string, ...int) (<-chan ctypes.ResultEvent, error) {
	if stub.subscribeErr != nil {
		return nil, stub.subscribeErr
	}

	return stub.subscription, nil
}

func (*cometEventsStub) Unsubscribe(context.Context, string, string) error {
	return nil
}

func (stub *cometEventsStub) UnsubscribeAll(_ context.Context, subscriber string) error {
	stub.unsubscribed <- subscriber
	return nil
}

func (stub *cometEventsStub) BlockResults(_ context.Context, height *int64) (*ctypes.ResultBlockResults, error) {
	if height != nil {
		stub.blockCalls <- *height
	}

	return stub.blockResults, stub.blockResultsErr
}

type recordingBus struct {
	pubsub.Bus
	published  chan pubsub.Event
	publishErr error
}

func (bus *recordingBus) Publish(event pubsub.Event) error {
	bus.published <- event
	return bus.publishErr
}

type cometOnlyStub struct {
	sdkclient.CometRPC
}

func TestNewServiceRejectsClientWithoutEventSubscriptions(t *testing.T) {
	var service Service
	var err error
	require.NotPanics(t, func() {
		service, err = NewService(context.Background(), &cometOnlyStub{}, "test", &recordingBus{})
	})
	require.Nil(t, service)
	require.ErrorContains(t, err, "event subscriptions")
}

func TestNewServiceReturnsSubscribeError(t *testing.T) {
	wantErr := errors.New("subscribe failed")
	node := &cometEventsStub{subscribeErr: wantErr}

	service, err := NewService(context.Background(), node, "test", &recordingBus{})
	require.Nil(t, service)
	require.ErrorIs(t, err, wantErr)
}

func TestServiceProcessesNewBlockAndShutsDown(t *testing.T) {
	typed := &deployment.EventDeploymentCreated{
		ID:   deployment.DeploymentID{Owner: "akash1owner", DSeq: 7},
		Hash: []byte("deployment-hash"),
	}
	event, err := sdk.TypedEventToEvent(typed)
	require.NoError(t, err)

	node := &cometEventsStub{
		subscription: make(chan ctypes.ResultEvent, 1),
		blockResults: &ctypes.ResultBlockResults{
			TxsResults: []*abci.ExecTxResult{
				nil,
				{Events: []abci.Event{{Type: "unknown"}, abci.Event(event)}},
			},
		},
		blockCalls:   make(chan int64, 1),
		unsubscribed: make(chan string, 1),
	}
	bus := &recordingBus{published: make(chan pubsub.Event, 1)}

	service, err := NewService(context.Background(), node, "observer", bus)
	require.NoError(t, err)

	node.subscription <- ctypes.ResultEvent{
		Data: cmtypes.EventDataNewBlockHeader{Header: cmtypes.Header{Height: 7}},
	}

	select {
	case height := <-node.blockCalls:
		require.Equal(t, int64(7), height)
	case <-time.After(time.Second):
		t.Fatal("service did not fetch the announced block")
	}

	select {
	case published := <-bus.published:
		got, ok := published.(*deployment.EventDeploymentCreated)
		require.True(t, ok)
		require.Equal(t, typed.ID, got.ID)
		require.Equal(t, typed.Hash, got.Hash)
	case <-time.After(time.Second):
		t.Fatal("service did not publish the typed block event")
	}

	service.Shutdown()
	service.Shutdown()
	require.Equal(t, "observer-blk-hdr", <-node.unsubscribed)
}

func TestServiceShutdownWaitsForTransportUnsubscribeFlush(t *testing.T) {
	node := &flushAwareCometEventsStub{
		cometEventsStub: &cometEventsStub{
			subscription: make(chan ctypes.ResultEvent),
			unsubscribed: make(chan string, 1),
		},
		flushStarted: make(chan struct{}),
		releaseFlush: make(chan struct{}),
	}

	service, err := NewService(context.Background(), node, "observer", &recordingBus{})
	require.NoError(t, err)

	shutdownDone := make(chan struct{})
	go func() {
		service.Shutdown()
		close(shutdownDone)
	}()

	select {
	case <-node.flushStarted:
	case <-shutdownDone:
		t.Fatal("event service returned before flushing its unsubscribe request")
	case <-time.After(time.Second):
		t.Fatal("event service did not start the unsubscribe flush")
	}
	select {
	case <-shutdownDone:
		t.Fatal("event service returned while the unsubscribe flush was blocked")
	default:
	}

	close(node.releaseFlush)
	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("event service did not finish after the unsubscribe flush completed")
	}
	require.Equal(t, "observer-blk-hdr", <-node.unsubscribed)
}

func TestProcessBlockStopsOnRPCAndPublishErrors(t *testing.T) {
	t.Run("rpc", func(t *testing.T) {
		node := &cometEventsStub{
			blockResultsErr: errors.New("rpc failed"),
			blockCalls:      make(chan int64, 1),
		}
		bus := &recordingBus{published: make(chan pubsub.Event, 1)}
		service := &events{ctx: context.Background(), client: node, bus: bus}

		service.processBlock(9)
		require.Equal(t, int64(9), <-node.blockCalls)
		select {
		case <-bus.published:
			t.Fatal("RPC failure must not publish an event")
		default:
		}
	})

	t.Run("publish", func(t *testing.T) {
		typed := &deployment.EventDeploymentCreated{ID: deployment.DeploymentID{Owner: "akash1owner", DSeq: 1}}
		event, err := sdk.TypedEventToEvent(typed)
		require.NoError(t, err)
		node := &cometEventsStub{
			blockResults: &ctypes.ResultBlockResults{TxsResults: []*abci.ExecTxResult{{
				Events: []abci.Event{abci.Event(event), abci.Event(event)},
			}}},
			blockCalls: make(chan int64, 1),
		}
		bus := &recordingBus{
			published:  make(chan pubsub.Event, 2),
			publishErr: errors.New("publish failed"),
		}
		service := &events{ctx: context.Background(), client: node, bus: bus}

		service.processBlock(10)
		require.Len(t, bus.published, 1)
	})

	t.Run("nil result", func(t *testing.T) {
		node := &cometEventsStub{blockCalls: make(chan int64, 1)}
		bus := &recordingBus{published: make(chan pubsub.Event, 1)}
		service := &events{ctx: context.Background(), client: node, bus: bus}

		require.NotPanics(t, func() {
			service.processBlock(11)
		})
		require.Equal(t, int64(11), <-node.blockCalls)
		require.Empty(t, bus.published)
	})
}

func TestProcessBlockPublishesTransactionAndFinalizeBlockEvents(t *testing.T) {
	txTyped := &deployment.EventDeploymentCreated{
		ID: deployment.DeploymentID{Owner: "akash1owner", DSeq: 1},
	}
	finalizeTyped := &deployment.EventDeploymentCreated{
		ID: deployment.DeploymentID{Owner: "akash1owner", DSeq: 2},
	}
	txEvent, err := sdk.TypedEventToEvent(txTyped)
	require.NoError(t, err)
	finalizeEvent, err := sdk.TypedEventToEvent(finalizeTyped)
	require.NoError(t, err)

	node := &cometEventsStub{
		blockResults: &ctypes.ResultBlockResults{
			TxsResults: []*abci.ExecTxResult{{Events: []abci.Event{abci.Event(txEvent)}}},
			FinalizeBlockEvents: []abci.Event{
				{Type: "unknown"},
				abci.Event(finalizeEvent),
			},
		},
		blockCalls: make(chan int64, 1),
	}
	bus := &recordingBus{published: make(chan pubsub.Event, 2)}
	service := &events{ctx: context.Background(), client: node, bus: bus}

	service.processBlock(12)

	require.Equal(t, int64(12), <-node.blockCalls)
	require.Len(t, bus.published, 2)
	first, ok := (<-bus.published).(*deployment.EventDeploymentCreated)
	require.True(t, ok)
	require.Equal(t, uint64(1), first.ID.DSeq)
	second, ok := (<-bus.published).(*deployment.EventDeploymentCreated)
	require.True(t, ok)
	require.Equal(t, uint64(2), second.ID.DSeq)
}

func TestBlockHeaderQuery(t *testing.T) {
	require.Equal(t, "tm.event = 'NewBlockHeader'", blkHeaderQuery().String())
}
