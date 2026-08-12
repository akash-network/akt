package ui

import (
	"sync"

	tea "charm.land/bubbletea/v2"
)

// RuntimeTaskGroup tracks monitor commands that may outlive Bubble Tea's
// program loop. StopAndWait prevents new work and waits for every command that
// already crossed the runtime boundary.
type RuntimeTaskGroup struct {
	mu      sync.Mutex
	tasks   sync.WaitGroup
	stopped bool
}

// NewRuntimeTaskGroup creates an active task group for one monitor runtime.
func NewRuntimeTaskGroup() *RuntimeTaskGroup {
	return &RuntimeTaskGroup{}
}

func (g *RuntimeTaskGroup) run(task func() tea.Msg) tea.Msg {
	if g == nil {
		return task()
	}

	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		return nil
	}
	g.tasks.Add(1)
	g.mu.Unlock()

	defer g.tasks.Done()
	return task()
}

// adopt transfers asynchronous work started by an active runtime task into the
// same drain. The caller invokes it before that active task returns.
func (g *RuntimeTaskGroup) adopt(done <-chan struct{}) {
	if g == nil || done == nil {
		return
	}

	g.mu.Lock()
	g.tasks.Add(1)
	g.mu.Unlock()
	go func() {
		defer g.tasks.Done()
		<-done
	}()
}

// StopAndWait rejects commands that have not started and drains active ones.
func (g *RuntimeTaskGroup) StopAndWait() {
	if g == nil {
		return
	}

	g.mu.Lock()
	g.stopped = true
	g.mu.Unlock()
	g.tasks.Wait()
}
