package context_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestWatcherDetectsWrite(t *testing.T) {
	m := newTestManager(t)

	// Seed a minimal config so Reload succeeds.
	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	w, err := aktctx.NewWatcher(m)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	var called atomic.Int32
	w.Subscribe(func(cfg *aktctx.Config) {
		called.Add(1)
	})

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Trigger a write to the config file.
	cfgPath := filepath.Join(m.Root(), "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Wait for the callback to fire.
	deadline := time.After(3 * time.Second)
	for called.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for subscriber callback")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	if got := called.Load(); got < 1 {
		t.Errorf("subscriber called %d times, want >= 1", got)
	}
}

func TestWatcherMultipleSubscribers(t *testing.T) {
	m := newTestManager(t)

	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	w, err := aktctx.NewWatcher(m)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	var a, b atomic.Int32
	w.Subscribe(func(_ *aktctx.Config) { a.Add(1) })
	w.Subscribe(func(_ *aktctx.Config) { b.Add(1) })

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	cfgPath := filepath.Join(m.Root(), "config.yaml")
	data, _ := os.ReadFile(cfgPath)
	_ = os.WriteFile(cfgPath, data, 0o644)

	deadline := time.After(3 * time.Second)
	for a.Load() == 0 || b.Load() == 0 {
		select {
		case <-deadline:
			t.Fatalf("timed out: a=%d b=%d", a.Load(), b.Load())
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestWatcherStopIsClean(t *testing.T) {
	m := newTestManager(t)

	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	w, err := aktctx.NewWatcher(m)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Stop should return without hanging.
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() did not return within 3s")
	}
}

func TestWatcherIgnoresNonWriteEvents(t *testing.T) {
	m := newTestManager(t)

	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	w, err := aktctx.NewWatcher(m)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	var called atomic.Int32
	w.Subscribe(func(_ *aktctx.Config) { called.Add(1) })

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// chmod is not a write event — subscriber should NOT fire.
	cfgPath := filepath.Join(m.Root(), "config.yaml")
	_ = os.Chmod(cfgPath, 0o600)

	time.Sleep(500 * time.Millisecond)

	if got := called.Load(); got != 0 {
		t.Errorf("subscriber called %d times on chmod, want 0", got)
	}
}
