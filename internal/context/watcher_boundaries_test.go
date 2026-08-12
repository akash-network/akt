package context_test

import (
	"os"
	"testing"
	"time"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestWatcherObservesConfigCreatedAfterStart(t *testing.T) {
	root := t.TempDir()
	mgr, err := aktctx.NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	watcher, err := aktctx.NewWatcher(mgr)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	notifications := make(chan aktctx.Config, 1)
	watcher.Subscribe(func(cfg *aktctx.Config) {
		select {
		case notifications <- *cfg:
		default:
		}
	})
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start before config exists: %v", err)
	}
	defer watcher.Stop()

	cfg := aktctx.DefaultConfig()
	cfg.Networks = []aktctx.Network{{Name: "created-later", ChainID: "local-1"}}
	if err := aktctx.SaveConfig(root, &cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	select {
	case got := <-notifications:
		if len(got.Networks) != 1 || got.Networks[0].Name != "created-later" {
			t.Fatalf("notification config = %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher missed config creation")
	}
}

func TestWatcherRecoversAfterMalformedWrite(t *testing.T) {
	mgr := newConfiguredManager(t, nil)
	valid, err := os.ReadFile(aktctx.ConfigPath(mgr.Root()))
	if err != nil {
		t.Fatalf("read valid config: %v", err)
	}

	watcher, err := aktctx.NewWatcher(mgr)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	notifications := make(chan aktctx.Config, 4)
	watcher.Subscribe(func(cfg *aktctx.Config) { notifications <- *cfg })
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer watcher.Stop()

	if err := os.WriteFile(aktctx.ConfigPath(mgr.Root()), []byte("contexts: [\n"), 0o600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	select {
	case cfg := <-notifications:
		t.Fatalf("malformed reload notified subscribers: %+v", cfg)
	default:
	}
	if mgr.GetContext("prod") == nil {
		t.Fatal("malformed reload replaced the last valid manager state")
	}

	if err := os.WriteFile(aktctx.ConfigPath(mgr.Root()), valid, 0o600); err != nil {
		t.Fatalf("restore config: %v", err)
	}
	select {
	case cfg := <-notifications:
		if len(cfg.Contexts) != 1 || cfg.Contexts[0].Name != "prod" {
			t.Fatalf("recovery notification = %+v", cfg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not recover after malformed config was repaired")
	}
}
