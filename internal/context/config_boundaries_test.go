package context_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestStoreDBPathUsesTheContextStore(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "contexts", "prod", "store", "deployments.db")
	if got := aktctx.StoreDBPath(root, "prod"); got != want {
		t.Fatalf("StoreDBPath = %q, want %q", got, want)
	}
}

func TestLoadConfigDefaultsAnOmittedSchemaVersion(t *testing.T) {
	root := t.TempDir()
	data := []byte("current-context: ''\nnetworks: []\nkeyrings: []\ncontexts: []\n")
	if err := os.WriteFile(aktctx.ConfigPath(root), data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := aktctx.LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Version != aktctx.ConfigVersion {
		t.Fatalf("version = %d, want default %d", cfg.Version, aktctx.ConfigVersion)
	}
}

func TestSaveConfigReportsFilesystemCollisions(t *testing.T) {
	t.Run("root is a file", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "akt-home")
		if err := os.WriteFile(root, []byte("blocker"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}

		err := aktctx.SaveConfig(root, configPtr(aktctx.DefaultConfig()))
		if err == nil || !strings.Contains(err.Error(), "create config directory") {
			t.Fatalf("SaveConfig error = %v, want root-directory failure", err)
		}
	})

	t.Run("config path is a directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(aktctx.ConfigPath(root), 0o700); err != nil {
			t.Fatalf("mkdir config path: %v", err)
		}

		err := aktctx.SaveConfig(root, configPtr(aktctx.DefaultConfig()))
		if err == nil || !strings.Contains(err.Error(), "create config") {
			t.Fatalf("SaveConfig error = %v, want config-file failure", err)
		}
	})
}

func TestEnsureContextDirsReportsEachBlockedBoundary(t *testing.T) {
	t.Run("context directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "contexts"), []byte("blocker"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}

		err := aktctx.EnsureContextDirs(root, "prod")
		if err == nil || !strings.Contains(err.Error(), "create context directory") {
			t.Fatalf("EnsureContextDirs error = %v, want context-directory failure", err)
		}
	})

	t.Run("store directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(aktctx.ContextDir(root, "prod"), 0o700); err != nil {
			t.Fatalf("mkdir context: %v", err)
		}
		if err := os.WriteFile(aktctx.StoreDir(root, "prod"), []byte("blocker"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}

		err := aktctx.EnsureContextDirs(root, "prod")
		if err == nil || !strings.Contains(err.Error(), "create store directory") {
			t.Fatalf("EnsureContextDirs error = %v, want store-directory failure", err)
		}
	})
}

func TestSetConsoleAPIKeyReportsCredentialFilesystemFailures(t *testing.T) {
	t.Run("remove non-empty directory", func(t *testing.T) {
		root := t.TempDir()
		path := aktctx.ConsoleAPIKeyPath(root, "prod")
		if err := os.MkdirAll(filepath.Join(path, "child"), 0o700); err != nil {
			t.Fatalf("mkdir credential collision: %v", err)
		}

		err := aktctx.SetConsoleAPIKey(root, "prod", "")
		if err == nil || !strings.Contains(err.Error(), "remove console api key") {
			t.Fatalf("SetConsoleAPIKey error = %v, want removal failure", err)
		}
	})

	t.Run("context parent is a file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "contexts"), []byte("blocker"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}

		err := aktctx.SetConsoleAPIKey(root, "prod", "secret")
		if err == nil || !strings.Contains(err.Error(), "create context directory") {
			t.Fatalf("SetConsoleAPIKey error = %v, want context-directory failure", err)
		}
	})

	t.Run("credential path is a directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(aktctx.ConsoleAPIKeyPath(root, "prod"), 0o700); err != nil {
			t.Fatalf("mkdir credential collision: %v", err)
		}

		err := aktctx.SetConsoleAPIKey(root, "prod", "secret")
		if err == nil || !strings.Contains(err.Error(), "write console api key") {
			t.Fatalf("SetConsoleAPIKey error = %v, want write failure", err)
		}
	})
}

func configPtr(cfg aktctx.Config) *aktctx.Config { return &cfg }
