package context

import (
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const configWriteSettleDelay = 25 * time.Millisecond

// Watcher monitors config.yaml for changes and reloads the Manager.
// Subscribers are notified via callback after each successful reload.
type Watcher struct {
	mgr       *Manager
	fsw       *fsnotify.Watcher
	subs      []func(*Config)
	mu        sync.Mutex
	stopCh    chan struct{}
	stoppedCh chan struct{}
}

// NewWatcher creates a filesystem watcher on the config file.
// It does NOT start watching; call Start() to begin.
func NewWatcher(mgr *Manager) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		mgr:       mgr,
		fsw:       fsw,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}, nil
}

// Subscribe registers a callback that fires after every successful config reload.
// The callback receives the new Config value. Must be called before Start.
func (w *Watcher) Subscribe(fn func(*Config)) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.subs = append(w.subs, fn)
}

// Start begins watching the config file for writes. It blocks internally
// in a goroutine; call Stop() to tear down.
func (w *Watcher) Start() error {
	path := ConfigPath(w.mgr.Root())

	if err := w.fsw.Add(path); err != nil {
		// If the file doesn't exist yet, watch the directory instead so we
		// catch the creation event.
		dir := w.mgr.Root()
		if err2 := w.fsw.Add(dir); err2 != nil {
			return err2
		}
	}

	go w.loop()

	return nil
}

// Stop terminates the watcher goroutine and releases resources.
func (w *Watcher) Stop() {
	close(w.stopCh)
	<-w.stoppedCh
	_ = w.fsw.Close()
}

func (w *Watcher) loop() {
	defer close(w.stoppedCh)

	for {
		select {
		case <-w.stopCh:
			return

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			// A Create event can arrive after the writer opened config.yaml but
			// before its payload is complete. Let the accompanying write burst
			// settle before parsing so subscribers never observe that transient
			// empty or partial file.
			time.Sleep(configWriteSettleDelay)

			if err := w.mgr.Reload(); err != nil {
				slog.Warn("config reload failed", "error", err)
				continue
			}

			cfg := w.mgr.Config()

			w.mu.Lock()
			subs := make([]func(*Config), len(w.subs))
			copy(subs, w.subs)
			w.mu.Unlock()

			for _, fn := range subs {
				fn(&cfg)
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}

			slog.Warn("config watcher error", "error", err)
		}
	}
}
