package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	synce "pkg.akt.dev/akt/internal/sync"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/go/util/pubsub"
)

// syncBridge connects the pubsub event bus to the sync engine and
// produces TUI refresh messages when the store is updated.
type syncBridge struct {
	subscriber pubsub.Subscriber
	engine     *synce.Engine
}

func newSyncBridge(bus pubsub.Bus, engine *synce.Engine) (*syncBridge, error) {
	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}
	return &syncBridge{subscriber: sub, engine: engine}, nil
}

// waitForEvent returns a tea.Cmd that blocks until the next bus event,
// feeds it to the sync engine, and returns a ViewDataRefreshMsg.
func (sb *syncBridge) waitForEvent() tea.Cmd {
	if sb == nil || sb.subscriber == nil {
		return nil
	}
	sub := sb.subscriber
	eng := sb.engine
	return func() tea.Msg {
		select {
		case ev, ok := <-sub.Events():
			if !ok {
				return nil
			}
			if eng != nil {
				_ = eng.HandleEvent(context.Background(), ev)
			}
			return messages.ViewDataRefreshMsg{}
		case <-sub.Done():
			return nil
		}
	}
}

func (sb *syncBridge) close() {
	if sb != nil && sb.subscriber != nil {
		sb.subscriber.Close()
	}
}
