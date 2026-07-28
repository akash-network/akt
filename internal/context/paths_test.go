package context_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// TestConfigHomeResolutionOrder pins the documented precedence for the akt
// home. Getting it wrong sends every context (and every stored credential) to
// a different directory than the user's existing configuration.
func TestConfigHomeResolutionOrder(t *testing.T) {
	t.Setenv("AKT_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	// 1. The --home override wins over everything.
	t.Setenv("AKT_HOME", "/env/akt")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	got, err := aktctx.ConfigHome("/override")
	if err != nil {
		t.Fatalf("ConfigHome: %v", err)
	}
	if got != "/override" {
		t.Errorf("override home = %q, want /override", got)
	}

	// 2. AKT_HOME beats XDG_CONFIG_HOME.
	if got, err = aktctx.ConfigHome(""); err != nil || got != "/env/akt" {
		t.Errorf("AKT_HOME home = (%q, %v), want /env/akt", got, err)
	}

	// 3. XDG_CONFIG_HOME/akt when AKT_HOME is unset.
	t.Setenv("AKT_HOME", "")
	if got, err = aktctx.ConfigHome(""); err != nil || got != filepath.Join("/xdg", "akt") {
		t.Errorf("XDG home = (%q, %v), want /xdg/akt", got, err)
	}

	// 4. ~/.config/akt as the last resort.
	t.Setenv("XDG_CONFIG_HOME", "")
	got, err = aktctx.ConfigHome("")
	if err != nil {
		t.Fatalf("ConfigHome: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(".config", "akt")) {
		t.Errorf("fallback home = %q, want a ~/.config/akt path", got)
	}
}

// TestPathHelpersAreRootedAndDistinct pins the on-disk layout the rename and
// delete paths depend on: everything belonging to a context lives under that
// context's directory, so moving the directory moves the store, the action
// log, the workflows, and the credential together.
func TestPathHelpersAreRootedAndDistinct(t *testing.T) {
	const root = "/home/u/.config/akt"

	ctxDir := aktctx.ContextDir(root, "prod")

	inContext := map[string]string{
		"store":      aktctx.StoreDir(root, "prod"),
		"action log": aktctx.ActionLogPath(root, "prod"),
		"workflows":  aktctx.ContextWorkflowsDir(root, "prod"),
		"credential": aktctx.ConsoleAPIKeyPath(root, "prod"),
	}

	for name, path := range inContext {
		if !strings.HasPrefix(path, ctxDir+string(filepath.Separator)) {
			t.Errorf("%s path %q is not inside the context dir %q", name, path, ctxDir)
		}
	}

	// Global workflows are deliberately NOT per-context: they are the
	// user-wide overrides of the built-ins.
	global := aktctx.WorkflowsDir(root)
	if global != filepath.Join(root, "workflows") {
		t.Errorf("global workflows dir = %q", global)
	}
	if strings.HasPrefix(global, ctxDir) {
		t.Error("global workflows must not live inside a context directory")
	}

	// Two contexts must never share a directory.
	if aktctx.ContextDir(root, "prod") == aktctx.ContextDir(root, "staging") {
		t.Error("context directories must be distinct")
	}
}

// TestKeyringDirHonorsExplicitDirectory covers both arms of KeyringDir. An
// explicit dir is how users point akt at an existing cosmos keyring; ignoring
// it would create a second, empty keyring and report "key not found".
func TestKeyringDirHonorsExplicitDirectory(t *testing.T) {
	const root = "/home/u/.config/akt"

	if got := aktctx.KeyringDir(root, aktctx.Keyring{Name: "default"}); got != filepath.Join(root, "keyrings", "default") {
		t.Errorf("default keyring dir = %q", got)
	}

	explicit := aktctx.Keyring{Name: "default", Dir: "/home/u/.akash"}
	if got := aktctx.KeyringDir(root, explicit); got != "/home/u/.akash" {
		t.Errorf("explicit keyring dir = %q, want /home/u/.akash", got)
	}
}

// TestLoadConfigOnEmptyRootReturnsDefaults covers the fresh-install path: with
// no config.yaml, LoadConfig must hand back usable defaults rather than an
// error, because that is what triggers the first-run wizard instead of a crash.
func TestLoadConfigOnEmptyRootReturnsDefaults(t *testing.T) {
	cfg, err := aktctx.LoadConfig(t.TempDir())
	if err != nil {
		t.Fatalf("LoadConfig on an empty root: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig returned a nil config")
	}
	if cfg.Version != aktctx.ConfigVersion {
		t.Errorf("version = %d, want %d", cfg.Version, aktctx.ConfigVersion)
	}
}

// TestLoadConfigRejectsMalformedYAML covers the parse-error branch. A corrupt
// config must be reported, not silently replaced with defaults — that would
// look like every context vanished.
func TestLoadConfigRejectsMalformedYAML(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(aktctx.ConfigPath(root), []byte("current-context: [unclosed\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := aktctx.LoadConfig(root); err == nil {
		t.Fatal("a malformed config.yaml must be an error")
	}
}

// TestSaveConfigRoundTrip covers SaveConfig plus the custom Context YAML
// marshalling (network and keyring serialize as name strings). A round-trip
// failure here loses the user's whole configuration on the next write.
func TestSaveConfigRoundTrip(t *testing.T) {
	root := t.TempDir()

	cfg := &aktctx.Config{
		Version:        aktctx.ConfigVersion,
		CurrentContext: "prod",
		Networks: []aktctx.Network{{
			Name:      "mainnet",
			ChainID:   "akashnet-2",
			Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc.example"}},
			GasPrices: "0.0025uakt",
		}},
		Keyrings: []aktctx.Keyring{{Name: "default", Backend: "test"}},
		Contexts: []aktctx.Context{{
			Name:           "prod",
			Network:        aktctx.Network{Name: "mainnet"},
			Keyring:        aktctx.Keyring{Name: "default"},
			DefaultAccount: "alice",
			Gas:            "auto",
		}},
	}

	if err := aktctx.SaveConfig(root, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := aktctx.LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.CurrentContext != "prod" {
		t.Errorf("current-context = %q", loaded.CurrentContext)
	}
	if len(loaded.Contexts) != 1 || loaded.Contexts[0].Network.Name != "mainnet" {
		t.Fatalf("contexts did not round-trip: %+v", loaded.Contexts)
	}
	if loaded.Contexts[0].Keyring.Name != "default" {
		t.Errorf("keyring reference did not round-trip: %+v", loaded.Contexts[0].Keyring)
	}
	if len(loaded.Networks) != 1 || loaded.Networks[0].ChainID != "akashnet-2" {
		t.Errorf("networks did not round-trip: %+v", loaded.Networks)
	}

	// The config carries no secrets and is not a credential file, but it does
	// describe the user's accounts; it must not be world-writable.
	info, err := os.Stat(aktctx.ConfigPath(root))
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o022 != 0 {
		t.Errorf("config mode = %o, want no group/other write", perm)
	}
}

// TestKeyringRegistryOperations covers GetKeyring, ListKeyrings, and
// CreateKeyring, including the duplicate and missing-name guards. Contexts
// reference keyrings by name, so a duplicate would make resolution ambiguous.
func TestKeyringRegistryOperations(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// DefaultConfig seeds "default".
	if kr := m.GetKeyring("default"); kr == nil {
		t.Fatal("the default keyring should exist")
	}
	if kr := m.GetKeyring("nope"); kr != nil {
		t.Errorf("unknown keyring = %+v, want nil", kr)
	}

	if err := m.CreateKeyring(aktctx.Keyring{Name: "ledger-kr"}); err != nil {
		t.Fatalf("CreateKeyring: %v", err)
	}

	kr := m.GetKeyring("ledger-kr")
	if kr == nil {
		t.Fatal("created keyring not found")
	}
	if kr.Backend != "os" {
		t.Errorf("backend = %q, want the os default", kr.Backend)
	}

	if err := m.CreateKeyring(aktctx.Keyring{Name: "ledger-kr"}); err == nil {
		t.Error("a duplicate keyring name must be rejected")
	}
	if err := m.CreateKeyring(aktctx.Keyring{}); err == nil {
		t.Error("an unnamed keyring must be rejected")
	}

	list := m.ListKeyrings()
	if len(list) != 2 {
		t.Fatalf("ListKeyrings = %+v, want 2", list)
	}

	// The returned slice is a copy: mutating it must not corrupt the config.
	list[0].Name = "mutated"
	if m.GetKeyring("mutated") != nil {
		t.Error("ListKeyrings must return a copy, not the live slice")
	}

	// The new keyring survives a reload from disk.
	m2, err := aktctx.NewManager(m.Root())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if m2.GetKeyring("ledger-kr") == nil {
		t.Error("created keyring was not persisted")
	}
}
