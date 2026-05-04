package context_test

import (
	"os"
	"path/filepath"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// ---------------------------------------------------------------------------
// ConfigHome resolution tests
// ---------------------------------------------------------------------------

func TestConfigHome_Override(t *testing.T) {
	got, err := aktctx.ConfigHome("/custom/path")
	if err != nil {
		t.Fatalf("ConfigHome: %v", err)
	}
	if got != "/custom/path" {
		t.Errorf("got %q, want /custom/path", got)
	}
}

func TestConfigHome_AKT_HOME(t *testing.T) {
	t.Setenv("AKT_HOME", "/from/env")
	// Clear XDG so it doesn't interfere.
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := aktctx.ConfigHome("")
	if err != nil {
		t.Fatalf("ConfigHome: %v", err)
	}
	if got != "/from/env" {
		t.Errorf("got %q, want /from/env", got)
	}
}

func TestConfigHome_XDG(t *testing.T) {
	t.Setenv("AKT_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/home")

	got, err := aktctx.ConfigHome("")
	if err != nil {
		t.Fatalf("ConfigHome: %v", err)
	}
	if want := "/xdg/home/akt"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConfigHome_Default(t *testing.T) {
	t.Setenv("AKT_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := aktctx.ConfigHome("")
	if err != nil {
		t.Fatalf("ConfigHome: %v", err)
	}

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "akt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

func TestConfigPath(t *testing.T) {
	got := aktctx.ConfigPath("/root")
	if want := "/root/config.yaml"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestContextDir(t *testing.T) {
	got := aktctx.ContextDir("/root", "prod")
	if want := "/root/contexts/prod"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStoreDir(t *testing.T) {
	got := aktctx.StoreDir("/root", "prod")
	if want := "/root/contexts/prod/store"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestActionLogPath(t *testing.T) {
	got := aktctx.ActionLogPath("/root", "prod")
	if want := "/root/contexts/prod/actions.log"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// LoadConfig / SaveConfig round-trip
// ---------------------------------------------------------------------------

func TestSaveAndLoadConfig(t *testing.T) {
	root := t.TempDir()

	cfg := aktctx.DefaultConfig()
	cfg.CurrentContext = "test-ctx"

	if err := aktctx.SaveConfig(root, &cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Verify the file exists.
	path := aktctx.ConfigPath(root)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Read it back via LoadConfig.
	loaded, err := aktctx.LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.CurrentContext != "test-ctx" {
		t.Errorf("CurrentContext = %q, want %q", loaded.CurrentContext, "test-ctx")
	}

	if loaded.Version == 0 {
		t.Error("Version is 0 after load, want non-zero schema version")
	}
}

func TestLoadConfig_EmptyDir(t *testing.T) {
	root := t.TempDir()

	// No config file — should return defaults.
	cfg, err := aktctx.LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.CurrentContext != "" {
		t.Errorf("CurrentContext = %q, want empty default", cfg.CurrentContext)
	}
}

func TestSaveConfig_CreatesDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "dir")

	cfg := aktctx.DefaultConfig()
	if err := aktctx.SaveConfig(root, &cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if _, err := os.Stat(aktctx.ConfigPath(root)); err != nil {
		t.Fatalf("config file not created in nested dir: %v", err)
	}
}

func TestSaveConfig_YAMLDocumentStart(t *testing.T) {
	root := t.TempDir()

	cfg := aktctx.DefaultConfig()
	if err := aktctx.SaveConfig(root, &cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	data, err := os.ReadFile(aktctx.ConfigPath(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Per SPEC §1.2: all akt-generated YAML files include `---`.
	if len(data) < 3 || string(data[:3]) != "---" {
		t.Errorf("config file does not start with document start marker (---)")
	}
}

// ---------------------------------------------------------------------------
// EnsureContextDirs
// ---------------------------------------------------------------------------

func TestEnsureContextDirs(t *testing.T) {
	root := t.TempDir()

	if err := aktctx.EnsureContextDirs(root, "myctx"); err != nil {
		t.Fatalf("EnsureContextDirs: %v", err)
	}

	storeDir := aktctx.StoreDir(root, "myctx")
	if _, err := os.Stat(storeDir); err != nil {
		t.Errorf("store directory not created: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DefaultConfig sanity
// ---------------------------------------------------------------------------

func TestDefaultConfig_OutputIsPretty(t *testing.T) {
	cfg := aktctx.DefaultConfig()
	if cfg.Defaults.Output != "pretty" {
		t.Errorf("Defaults.Output = %q, want \"pretty\"", cfg.Defaults.Output)
	}
}

func TestDefaultConfig_VersionIsSet(t *testing.T) {
	cfg := aktctx.DefaultConfig()
	if cfg.Version == 0 {
		t.Error("DefaultConfig().Version is 0, want non-zero")
	}
}
