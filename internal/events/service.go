// Package events provides a shared blockchain event service for the akt CLI.
//
// It subscribes to CometBFT NewBlockHeader events via WebSocket, fetches
// block results for each new block, parses all ABCI events into typed
// protobuf messages using sdk.ParseTypedEvent, and publishes every
// successfully parsed event to a pubsub.Bus. No whitelist filtering is
// applied — subscribers are responsible for selecting the event types
// they care about via a type switch on the received pubsub.Event.
//
// Copied from pkg.akt.dev/go/util/events and modified to remove the
// event-type whitelist so that all chain events are forwarded.
package events

import (
	"context"

	"github.com/boz/go-lifecycle"

	abci "github.com/cometbft/cometbft/abci/types"
	cmclient "github.com/cometbft/cometbft/rpc/client"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmtypes "github.com/cometbft/cometbft/types"
	"golang.org/x/sync/errgroup"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/util/pubsub"
)

type events struct {
	ctx    context.Context
	group  *errgroup.Group
	ebus   cmclient.EventsClient
	client sdkclient.CometRPC
	bus    pubsub.Bus
	lc     lifecycle.Lifecycle
}

// Service represents an event monitoring service that subscribes to and
// processes blockchain events. It monitors block headers and publishes
// all typed ABCI events to a message bus.
type Service interface {
	// Shutdown gracefully stops the event monitoring service and cleans
	// up resources.
	Shutdown()
}

// NewService creates and initializes a new blockchain event monitoring
// service.
//
// Parameters:
//   - pctx: Parent context for controlling the service lifecycle
//   - node: CometBFT RPC client for interacting with the blockchain
//   - name: Service name used as a prefix for subscription identifiers
//   - bus: Message bus for publishing processed events
func NewService(pctx context.Context, node sdkclient.CometRPC, name string, bus pubsub.Bus) (Service, error) {
	group, ctx := errgroup.WithContext(pctx)

	ev := &events{
		ctx:    ctx,
		group:  group,
		ebus:   node.(cmclient.EventsClient),
		client: node,
		lc:     lifecycle.New(),
		bus:    bus,
	}

	const queuesz = 1000

	blkHeaderName := name + "-blk-hdr"

	blkch, err := ev.ebus.Subscribe(ctx, blkHeaderName, blkHeaderQuery().String(), queuesz)
	if err != nil {
		return nil, err
	}

	startch := make(chan struct{}, 1)

	group.Go(func() error {
		ev.lc.WatchContext(ctx)
		return ev.lc.Error()
	})

	group.Go(func() error {
		return ev.run(blkHeaderName, blkch, startch)
	})

	select {
	case <-pctx.Done():
		return nil, pctx.Err()
	case <-startch:
		return ev, nil
	}
}

func (e *events) Shutdown() {
	select {
	case <-e.lc.Done():
		return
	default:
		e.lc.Shutdown(nil)
	}

	_ = e.group.Wait()
}

func (e *events) run(subs string, ch <-chan ctypes.ResultEvent, startch chan<- struct{}) error {
	defer func() {
		_ = e.ebus.UnsubscribeAll(e.ctx, subs)
		e.lc.ShutdownCompleted()
	}()

	startch <- struct{}{}

loop:
	for {
		select {
		case err := <-e.lc.ShutdownRequest():
			e.lc.ShutdownInitiated(err)
			break loop
		case ev := <-ch:
			// nolint: gocritic
			switch evt := ev.Data.(type) {
			case cmtypes.EventDataNewBlockHeader:
				e.processBlock(evt.Header.Height)
			}
		}
	}

	return e.ctx.Err()
}

func (e *events) processBlock(height int64) {
	blkResults, err := e.client.BlockResults(e.ctx, &height)
	if err != nil {
		return
	}

	for _, tx := range blkResults.TxsResults {
		if tx == nil {
			continue
		}

		for _, ev := range tx.Events {
			if pev := parseEvent(ev); pev != nil {
				if err := e.bus.Publish(pev); err != nil {
					return
				}
			}
		}
	}
}

// parseEvent attempts to deserialize an ABCI event into a typed protobuf
// message. Returns nil when the event cannot be parsed (unknown type,
// malformed attributes, etc.). No filtering is applied — every
// successfully parsed event is returned.
func parseEvent(bev abci.Event) interface{} {
	pev, err := sdk.ParseTypedEvent(bev)
	if err != nil {
		return nil
	}

	return pev
}
