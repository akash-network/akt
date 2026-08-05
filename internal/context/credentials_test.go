package context_test

import (
	"os"
	"path/filepath"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func newCredentialManager(t *testing.T) *aktctx.Manager {
	t.Helper()

	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	return m
}

func TestConsoleAPIKeyStoreRoundTrip(t *testing.T) {
	root := t.TempDir()

	if err := aktctx.SetConsoleAPIKey(root, "prod", "sk-test-123"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	key, err := aktctx.StoredConsoleAPIKey(root, "prod")
	if err != nil {
		t.Fatalf("StoredConsoleAPIKey: %v", err)
	}
	if key != "sk-test-123" {
		t.Errorf("key = %q, want sk-test-123", key)
	}

	info, err := os.Stat(aktctx.ConsoleAPIKeyPath(root, "prod"))
	if err != nil {
		t.Fatalf("stat credential: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credential file mode = %o, want 600", perm)
	}

	// Empty key removes the credential.
	if err := aktctx.SetConsoleAPIKey(root, "prod", ""); err != nil {
		t.Fatalf("SetConsoleAPIKey(remove): %v", err)
	}

	key, err = aktctx.StoredConsoleAPIKey(root, "prod")
	if err != nil {
		t.Fatalf("StoredConsoleAPIKey after remove: %v", err)
	}
	if key != "" {
		t.Errorf("key after removal = %q, want empty", key)
	}
}

func TestConsoleAPIKeysAreIndependentPerContext(t *testing.T) {
	root := t.TempDir()

	if err := aktctx.SetConsoleAPIKey(root, "prod", "key-prod"); err != nil {
		t.Fatalf("set prod: %v", err)
	}
	if err := aktctx.SetConsoleAPIKey(root, "staging", "key-staging"); err != nil {
		t.Fatalf("set staging: %v", err)
	}

	prodKey, _ := aktctx.StoredConsoleAPIKey(root, "prod")
	stagingKey, _ := aktctx.StoredConsoleAPIKey(root, "staging")

	if prodKey != "key-prod" || stagingKey != "key-staging" {
		t.Errorf("keys not independent: prod=%q staging=%q", prodKey, stagingKey)
	}
}

func TestResolvePopulatesConsoleCredential(t *testing.T) {
	m := newCredentialManager(t)

	if err := m.CreateContext(aktctx.Context{
		Name:       "console",
		Network:    aktctx.Network{Name: "mainnet"},
		AuthMethod: aktctx.AuthMethodConsoleAPI,
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	if err := aktctx.SetConsoleAPIKey(m.Root(), "console", "sk-stored"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	rc, err := m.Resolve("console")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if rc.AuthMethod != aktctx.AuthMethodConsoleAPI {
		t.Errorf("auth method = %q, want console-api", rc.AuthMethod)
	}
	if rc.ConsoleAPIURL != aktctx.DefaultConsoleAPIURL {
		t.Errorf("console url = %q, want default %q", rc.ConsoleAPIURL, aktctx.DefaultConsoleAPIURL)
	}
	if rc.ConsoleAPIKey != "sk-stored" {
		t.Errorf("console key = %q, want sk-stored", rc.ConsoleAPIKey)
	}
}

func TestResolveEnvOverridesStoredKey(t *testing.T) {
	m := newCredentialManager(t)

	if err := m.CreateContext(aktctx.Context{
		Name:       "console",
		Network:    aktctx.Network{Name: "mainnet"},
		AuthMethod: aktctx.AuthMethodConsoleAPI,
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	if err := aktctx.SetConsoleAPIKey(m.Root(), "console", "sk-stored"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	t.Setenv(aktctx.EnvConsoleAPIKey, "sk-env")

	rc, err := m.Resolve("console")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if rc.ConsoleAPIKey != "sk-env" {
		t.Errorf("console key = %q, want env override sk-env", rc.ConsoleAPIKey)
	}
}

func TestResolveDefaultsToKeyringAuth(t *testing.T) {
	m := newCredentialManager(t)

	if err := m.CreateContext(aktctx.Context{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	rc, err := m.Resolve("prod")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if rc.AuthMethod != aktctx.AuthMethodKeyring {
		t.Errorf("auth method = %q, want keyring default", rc.AuthMethod)
	}
}

func TestCreateContextRejectsInvalidAuthMethod(t *testing.T) {
	m := newCredentialManager(t)

	err := m.CreateContext(aktctx.Context{
		Name:       "bad",
		Network:    aktctx.Network{Name: "mainnet"},
		AuthMethod: "carrier-pigeon",
	})
	if err == nil {
		t.Fatal("expected invalid auth-method to be rejected")
	}
}

func TestAuthMethodRoundTripsThroughConfig(t *testing.T) {
	m := newCredentialManager(t)

	if err := m.CreateContext(aktctx.Context{
		Name:          "console",
		Network:       aktctx.Network{Name: "mainnet"},
		AuthMethod:    aktctx.AuthMethodConsoleAPI,
		ConsoleAPIURL: "https://console-api.example.com",
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	// Reload from disk and verify the fields persisted.
	m2, err := aktctx.NewManager(m.Root())
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}

	ctx := m2.GetContext("console")
	if ctx == nil {
		t.Fatal("context not found after reload")
		return
	}
	if ctx.AuthMethod != aktctx.AuthMethodConsoleAPI {
		t.Errorf("auth method after reload = %q, want console-api", ctx.AuthMethod)
	}
	if ctx.ConsoleAPIURL != "https://console-api.example.com" {
		t.Errorf("console url after reload = %q", ctx.ConsoleAPIURL)
	}
}

func TestActiveContextResolution(t *testing.T) {
	m := newCredentialManager(t)

	if got := m.ActiveContext(""); got != "" {
		t.Errorf("no contexts: ActiveContext = %q, want empty", got)
	}

	if err := m.CreateContext(aktctx.Context{Name: "only", Network: aktctx.Network{Name: "mainnet"}}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	// A single configured context is auto-selected even with no current-context.
	if got := m.ActiveContext(""); got != "only" {
		t.Errorf("single context: ActiveContext = %q, want only", got)
	}

	if err := m.CreateContext(aktctx.Context{Name: "second", Network: aktctx.Network{Name: "mainnet"}}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	if got := m.ActiveContext(""); got != "" {
		t.Errorf("two contexts, none current: ActiveContext = %q, want empty", got)
	}

	if err := m.UseContext("second"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}
	if got := m.ActiveContext(""); got != "second" {
		t.Errorf("current-context: ActiveContext = %q, want second", got)
	}

	// The explicit override always wins.
	if got := m.ActiveContext("only"); got != "only" {
		t.Errorf("override: ActiveContext = %q, want only", got)
	}
}

func TestUpdateNetworklessConsoleContext(t *testing.T) {
	m := newCredentialManager(t)

	if err := m.CreateContext(aktctx.Context{
		Name:       "console",
		AuthMethod: aktctx.AuthMethodConsoleAPI,
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	// A network-less console context must remain editable — the remedy
	// printed by capability errors is exactly `akt context edit ...`.
	if err := m.UpdateContext("console", func(c *aktctx.Context) error {
		c.ConsoleAPIURL = "https://console-api.example.com"
		return nil
	}); err != nil {
		t.Fatalf("UpdateContext on a network-less console context: %v", err)
	}

	if got := m.GetContext("console").ConsoleAPIURL; got != "https://console-api.example.com" {
		t.Errorf("edit not persisted: %q", got)
	}

	// A bogus network reference is still rejected.
	if err := m.UpdateContext("console", func(c *aktctx.Context) error {
		c.Network = aktctx.Network{Name: "nope"}
		return nil
	}); err == nil {
		t.Error("unknown network must still be rejected")
	}
}

func TestCredentialDirectoryIsNotWorldReadable(t *testing.T) {
	root := t.TempDir()

	if err := aktctx.SetConsoleAPIKey(root, "prod", "sk-secret"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	// The file is 0600, but a world-readable parent directory still leaks
	// its presence and lets a local user replace it.
	dir := filepath.Dir(aktctx.ConsoleAPIKeyPath(root, "prod"))
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat credential directory: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("credential directory mode = %o, want no group/other access", perm)
	}
}

func TestContextDirectoriesAreNotWorldReadable(t *testing.T) {
	m := newCredentialManager(t)

	// EnsureContextDirs runs on create; the context directory holds the
	// credential alongside the store and action log.
	if err := m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	info, err := os.Stat(aktctx.ContextDir(m.Root(), "prod"))
	if err != nil {
		t.Fatalf("stat context directory: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("context directory mode = %o, want no group/other access", perm)
	}
}
