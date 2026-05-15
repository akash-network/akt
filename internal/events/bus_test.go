package events_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/util/pubsub"
)

func TestBusPublishSubscribe(t *testing.T) {
	bus := pubsub.NewBus()
	defer bus.Close()

	sub, err := bus.Subscribe()
	require.NoError(t, err)
	defer sub.Close()

	ev := "test-event"
	require.NoError(t, bus.Publish(ev))

	select {
	case got := <-sub.Events():
		assert.Equal(t, ev, got)
	case <-pubsub.AfterThreadStart(t):
		require.Fail(t, "timed out waiting for event")
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	bus := pubsub.NewBus()
	defer bus.Close()

	sub1, err := bus.Subscribe()
	require.NoError(t, err)
	defer sub1.Close()

	sub2, err := bus.Subscribe()
	require.NoError(t, err)
	defer sub2.Close()

	ev := "shared-event"
	require.NoError(t, bus.Publish(ev))

	select {
	case got := <-sub1.Events():
		assert.Equal(t, ev, got)
	case <-pubsub.AfterThreadStart(t):
		require.Fail(t, "sub1 timed out waiting for event")
	}

	select {
	case got := <-sub2.Events():
		assert.Equal(t, ev, got)
	case <-pubsub.AfterThreadStart(t):
		require.Fail(t, "sub2 timed out waiting for event")
	}
}

func TestBusClose(t *testing.T) {
	bus := pubsub.NewBus()

	bus.Close()

	select {
	case <-bus.Done():
		// expected
	case <-pubsub.AfterThreadStart(t):
		require.Fail(t, "timed out waiting for bus Done()")
	}

	// Publish after close should return ErrNotRunning.
	assert.Equal(t, pubsub.ErrNotRunning, bus.Publish("after-close"))
}
