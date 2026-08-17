package ui

import (
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestRuntimeTaskGroupDrainsAdoptedProducerAndRejectsLaterWork(t *testing.T) {
	tasks := NewRuntimeTaskGroup()
	producerDone := make(chan struct{})
	commandStarted := make(chan struct{})
	releaseCommand := make(chan struct{})
	commandDone := make(chan struct{})

	go func() {
		_ = tasks.run(func() tea.Msg {
			close(commandStarted)
			tasks.adopt(producerDone)
			<-releaseCommand
			return nil
		})
		close(commandDone)
	}()
	<-commandStarted

	drained := make(chan struct{})
	go func() {
		tasks.StopAndWait()
		close(drained)
	}()
	assertRuntimeDrainBlocked(t, drained, "active command")

	close(releaseCommand)
	<-commandDone
	assertRuntimeDrainBlocked(t, drained, "adopted consensus producer")

	close(producerDone)
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("runtime task drain did not finish after producer stopped")
	}

	var ran atomic.Bool
	_ = tasks.run(func() tea.Msg {
		ran.Store(true)
		return nil
	})
	if ran.Load() {
		t.Fatal("stopped runtime accepted new work")
	}
}

func TestNilRuntimeTaskGroupKeepsOptionalLifecycleSafe(t *testing.T) {
	var tasks *RuntimeTaskGroup
	ran := false
	msg := tasks.run(func() tea.Msg {
		ran = true
		return "completed"
	})
	if !ran || msg != "completed" {
		t.Fatalf("nil task group run = %v, ran %t", msg, ran)
	}

	// Models constructed outside the standalone runtime intentionally have no
	// task group. Cleanup must remain a no-op for that supported configuration.
	tasks.adopt(make(chan struct{}))
	tasks.StopAndWait()
}

func assertRuntimeDrainBlocked(t *testing.T, drained <-chan struct{}, reason string) {
	t.Helper()
	select {
	case <-drained:
		t.Fatalf("runtime drain returned with %s still active", reason)
	case <-time.After(25 * time.Millisecond):
	}
}
