package context_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func newTestManager(t *testing.T) *aktctx.Manager {
	t.Helper()

	root := t.TempDir()

	m, err := aktctx.NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	return m
}

// ---------------------------------------------------------------------------
// Network tests
// ---------------------------------------------------------------------------

func TestCreateNetworkFromTemplate(t *testing.T) {
	m := newTestManager(t)

	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	net := m.GetNetwork("mainnet")
	if net == nil {
		t.Fatal("expected network to exist")
	}

	if net.ChainID != "akashnet-2" {
		t.Errorf("chain-id = %q, want akashnet-2", net.ChainID)
	}

	if len(net.Endpoints.RPC) < 1 {
		t.Error("expected at least one RPC endpoint")
	}
}

func TestCreateNetworkFromTemplate_CustomName(t *testing.T) {
	m := newTestManager(t)

	if err := m.CreateNetworkFromTemplate("my-main", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	net := m.GetNetwork("my-main")
	if net == nil {
		t.Fatal("expected network with custom name")
	}

	if net.Name != "my-main" {
		t.Errorf("name = %q, want my-main", net.Name)
	}
}

func TestCreateNetworkDuplicate(t *testing.T) {
	m := newTestManager(t)

	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")

	err := m.CreateNetworkFromTemplate("mainnet", "mainnet")
	if err == nil {
		t.Fatal("expected error creating duplicate network")
	}
}

func TestCreateNetworkBadTemplate(t *testing.T) {
	m := newTestManager(t)

	err := m.CreateNetworkFromTemplate("foo", "nonexistent")
	if err == nil {
		t.Fatal("expected error with unknown template")
	}
}

func TestUpdateNetwork(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")

	err := m.UpdateNetwork("mainnet", func(n *aktctx.Network) error {
		n.GasPrices = "0.04uakt"
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateNetwork: %v", err)
	}

	net := m.GetNetwork("mainnet")
	if net.GasPrices != "0.04uakt" {
		t.Errorf("gas-prices = %q, want 0.04uakt", net.GasPrices)
	}
}

func TestForkNetwork(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")

	if err := m.ForkNetwork("mainnet", "mainnet-fork"); err != nil {
		t.Fatalf("ForkNetwork: %v", err)
	}

	fork := m.GetNetwork("mainnet-fork")
	if fork == nil {
		t.Fatal("expected fork to exist")
	}

	orig := m.GetNetwork("mainnet")
	if fork.ChainID != orig.ChainID {
		t.Error("fork should have same chain-id as original")
	}

	// Mutating fork should not affect original.
	_ = m.UpdateNetwork("mainnet-fork", func(n *aktctx.Network) error {
		n.GasPrices = "0.1uakt"
		return nil
	})

	orig = m.GetNetwork("mainnet")
	if orig.GasPrices == "0.1uakt" {
		t.Error("original should not be affected by fork mutation")
	}
}

func TestDeleteNetwork(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")

	if err := m.DeleteNetwork("mainnet"); err != nil {
		t.Fatalf("DeleteNetwork: %v", err)
	}

	if net := m.GetNetwork("mainnet"); net != nil {
		t.Error("expected network to be deleted")
	}
}

func TestDeleteNetworkInUse(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})

	err := m.DeleteNetwork("mainnet")
	if err == nil {
		t.Fatal("expected error deleting network in use")
	}
}

func TestNetworkUsers(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})
	_ = m.CreateContext(aktctx.Context{Name: "monitoring", Network: aktctx.Network{Name: "mainnet"}})

	users := m.NetworkUsers("mainnet")
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

// ---------------------------------------------------------------------------
// Context tests
// ---------------------------------------------------------------------------

func TestCreateAndUseContext(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")

	err := m.CreateContext(aktctx.Context{
		Name:           "prod",
		Network:        aktctx.Network{Name: "mainnet"},
		DefaultAccount: "alice",
	})
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	if err := m.UseContext("prod"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	if m.CurrentContext() != "prod" {
		t.Errorf("current = %q, want prod", m.CurrentContext())
	}
}

func TestCreateContextMissingNetwork(t *testing.T) {
	m := newTestManager(t)

	err := m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "nonexistent"}})
	if err == nil {
		t.Fatal("expected error with missing network")
	}
}

func TestCreateContextDefaults(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")

	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})

	ctx := m.GetContext("prod")
	if ctx.Keyring.Name != "default" {
		t.Errorf("keyring = %q, want default", ctx.Keyring.Name)
	}

	if ctx.Gas != "auto" {
		t.Errorf("gas = %q, want auto", ctx.Gas)
	}

	if ctx.ProviderDefaults.AuthType != "jwt" {
		t.Errorf("auth-type = %q, want jwt", ctx.ProviderDefaults.AuthType)
	}
}

func TestCreateContextRejectsInvalidProviderAuthType(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")

	err := m.CreateContext(aktctx.Context{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
		ProviderDefaults: aktctx.ProviderDefaults{
			AuthType: "password",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "provider auth type") {
		t.Fatalf("error = %v, want provider auth enum validation", err)
	}
}

func TestCreateContextDataDirs(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})

	storeDir := aktctx.StoreDir(m.Root(), "prod")
	if _, err := os.Stat(storeDir); os.IsNotExist(err) {
		t.Errorf("store directory %s should exist", storeDir)
	}
}

func TestDeleteContext(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})
	_ = m.CreateContext(aktctx.Context{Name: "staging", Network: aktctx.Network{Name: "mainnet"}})
	_ = m.UseContext("prod")

	if err := m.DeleteContext("staging", false); err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}

	if ctx := m.GetContext("staging"); ctx != nil {
		t.Error("expected context to be deleted")
	}

	// Data directory should be removed.
	dir := aktctx.ContextDir(m.Root(), "staging")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected context data directory to be removed")
	}
}

func TestDeleteCurrentContextFails(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})
	_ = m.UseContext("prod")

	err := m.DeleteContext("prod", false)
	if err == nil {
		t.Fatal("expected error deleting current context")
	}
}

func TestDeleteContextKeepData(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "staging", Network: aktctx.Network{Name: "mainnet"}})

	if err := m.DeleteContext("staging", true); err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}

	// Data directory should still exist.
	dir := aktctx.StoreDir(m.Root(), "staging")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected data directory to be preserved with keepData=true")
	}
}

func TestRenameContext(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})
	_ = m.UseContext("prod")

	if err := m.RenameContext("prod", "production"); err != nil {
		t.Fatalf("RenameContext: %v", err)
	}

	if m.GetContext("prod") != nil {
		t.Error("old name should not exist")
	}

	if m.GetContext("production") == nil {
		t.Error("new name should exist")
	}

	if m.CurrentContext() != "production" {
		t.Errorf("current context should be updated to production, got %q", m.CurrentContext())
	}
}

func TestUpdateContext(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})

	err := m.UpdateContext("prod", func(c *aktctx.Context) error {
		c.DefaultAccount = "bob"
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateContext: %v", err)
	}

	ctx := m.GetContext("prod")
	if ctx.DefaultAccount != "bob" {
		t.Errorf("default-account = %q, want bob", ctx.DefaultAccount)
	}
}

func TestUpdateContextRejectsInvalidReferencesWithoutMutation(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
		Keyring: aktctx.Keyring{Name: "default"},
	})

	err := m.UpdateContext("prod", func(ctx *aktctx.Context) error {
		ctx.Network = aktctx.Network{Name: "does-not-exist"}
		return nil
	})
	if err == nil {
		t.Fatal("invalid network update succeeded")
	}
	if got := m.GetContext("prod").Network.Name; got != "mainnet" {
		t.Errorf("rejected update left network %q, want mainnet", got)
	}
}

func TestUpdateContextAndNetworkForksInOneWrite(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
		Keyring: aktctx.Keyring{Name: "default"},
	})

	err := m.UpdateContextAndNetwork(
		"prod",
		"mainnet-prod",
		func(ctx *aktctx.Context) error {
			ctx.DefaultAccount = "alice"
			return nil
		},
		func(network *aktctx.Network) error {
			network.Endpoints.RPC = []string{"https://private.example:443"}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("UpdateContextAndNetwork: %v", err)
	}

	ctx := m.GetContext("prod")
	if ctx.Network.Name != "mainnet-prod" || ctx.DefaultAccount != "alice" {
		t.Errorf("context after fork = %+v", ctx)
	}
	fork := m.GetNetwork("mainnet-prod")
	if fork == nil || len(fork.Endpoints.RPC) != 1 || fork.Endpoints.RPC[0] != "https://private.example:443" {
		t.Errorf("fork = %+v", fork)
	}
	if parent := m.GetNetwork("mainnet"); parent == nil || parent.Endpoints.RPC[0] == "https://private.example:443" {
		t.Errorf("parent was modified: %+v", parent)
	}
}

// ---------------------------------------------------------------------------
// Resolve tests
// ---------------------------------------------------------------------------

func TestResolve(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{
		Name:           "prod",
		Network:        aktctx.Network{Name: "mainnet"},
		DefaultAccount: "alice",
		Fees:           "5000uakt",
	})
	_ = m.UseContext("prod")

	rc, err := m.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if rc.Name != "prod" {
		t.Errorf("name = %q, want prod", rc.Name)
	}

	if rc.Network.ChainID != "akashnet-2" {
		t.Errorf("chain-id = %q, want akashnet-2", rc.Network.ChainID)
	}

	if rc.DefaultAccount != "alice" {
		t.Errorf("default-account = %q, want alice", rc.DefaultAccount)
	}

	if rc.Fees != "5000uakt" {
		t.Errorf("fees = %q, want 5000uakt", rc.Fees)
	}

	if rc.GasPrices != "0.025uakt" {
		t.Errorf("gas-prices = %q, want 0.025uakt (from network)", rc.GasPrices)
	}
}

func TestResolveDefaultsLegacyProviderAuthType(t *testing.T) {
	root := t.TempDir()
	cfg := aktctx.Config{
		Version:        aktctx.ConfigVersion,
		CurrentContext: "prod",
		Networks: []aktctx.Network{{
			Name:      "mainnet",
			ChainID:   "akashnet-2",
			Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc.example"}},
		}},
		Keyrings: []aktctx.Keyring{{Name: "default", Backend: "test"}},
		Contexts: []aktctx.Context{{
			Name:       "prod",
			Network:    aktctx.Network{Name: "mainnet"},
			Keyring:    aktctx.Keyring{Name: "default"},
			AuthMethod: aktctx.AuthMethodKeyring,
		}},
	}
	if err := aktctx.SaveConfig(root, &cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	m, err := aktctx.NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	rc, err := m.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if rc.AuthType != aktctx.ProviderAuthJWT {
		t.Errorf("AuthType = %q, want %q", rc.AuthType, aktctx.ProviderAuthJWT)
	}
	if rc.ProviderDefaults.AuthType != aktctx.ProviderAuthJWT {
		t.Errorf("ProviderDefaults.AuthType = %q, want %q", rc.ProviderDefaults.AuthType, aktctx.ProviderAuthJWT)
	}
}

func TestResolveNoCurrentContext(t *testing.T) {
	m := newTestManager(t)

	_, err := m.Resolve("")
	if err == nil {
		t.Fatal("expected error when no current context")
	}
}

// ---------------------------------------------------------------------------
// Persistence tests
// ---------------------------------------------------------------------------

func TestConfigPersistence(t *testing.T) {
	root := t.TempDir()

	// Create and populate.
	m1, _ := aktctx.NewManager(root)
	_ = m1.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m1.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}, DefaultAccount: "alice"})
	_ = m1.UseContext("prod")

	// Reload from disk.
	m2, err := aktctx.NewManager(root)
	if err != nil {
		t.Fatalf("NewManager reload: %v", err)
	}

	if m2.CurrentContext() != "prod" {
		t.Errorf("current = %q, want prod after reload", m2.CurrentContext())
	}

	net := m2.GetNetwork("mainnet")
	if net == nil || net.ChainID != "akashnet-2" {
		t.Error("network not persisted correctly")
	}

	ctx := m2.GetContext("prod")
	if ctx == nil || ctx.DefaultAccount != "alice" {
		t.Error("context not persisted correctly")
	}
}

func TestConfigFileContent(t *testing.T) {
	root := t.TempDir()

	m, _ := aktctx.NewManager(root)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})

	data, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("config.yaml should not be empty")
	}

	// Spot-check key fields are present.
	for _, want := range []string{"current-context", "networks", "contexts", "mainnet", "prod", "akashnet-2"} {
		if !contains(content, want) {
			t.Errorf("config.yaml missing %q", want)
		}
	}
}

func TestListNetworks(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateNetworkFromTemplate("testnet", "testnet")

	nets := m.ListNetworks()
	if len(nets) != 2 {
		t.Errorf("expected 2 networks, got %d", len(nets))
	}
}

func TestListContexts(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "a", Network: aktctx.Network{Name: "mainnet"}})
	_ = m.CreateContext(aktctx.Context{Name: "b", Network: aktctx.Network{Name: "mainnet"}})

	ctxs := m.ListContexts()
	if len(ctxs) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(ctxs))
	}
}

// NetworkUpdateAffectsAllContexts verifies that editing a shared network
// is visible from all contexts referencing it.
func TestNetworkUpdateAffectsAllContexts(t *testing.T) {
	m := newTestManager(t)
	_ = m.CreateNetworkFromTemplate("mainnet", "mainnet")
	_ = m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}})
	_ = m.CreateContext(aktctx.Context{Name: "monitoring", Network: aktctx.Network{Name: "mainnet"}})

	_ = m.UpdateNetwork("mainnet", func(n *aktctx.Network) error {
		n.Endpoints.RPC = []string{"https://custom-rpc:443"}
		return nil
	})

	// Both contexts should see the updated endpoint through Resolve.
	_ = m.UseContext("prod")
	rc1, _ := m.Resolve("prod")
	if rc1.Network.Endpoints.RPC[0] != "https://custom-rpc:443" {
		t.Error("prod context should see updated RPC")
	}

	rc2, _ := m.Resolve("monitoring")
	if rc2.Network.Endpoints.RPC[0] != "https://custom-rpc:443" {
		t.Error("monitoring context should see updated RPC")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
