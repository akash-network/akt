package cache

import (
	"path/filepath"
	"testing"

	"pkg.akt.dev/akt/internal/monitor/rpc"
)

// openTestCache creates a temporary bbolt database and returns an open ProviderCache.
func openTestCache(t *testing.T) *ProviderCache {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	c, err := Open(db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return c
}

func TestHasProvidersEmptyCache(t *testing.T) {
	c := openTestCache(t)
	if c.HasProviders() {
		t.Error("HasProviders() = true on empty cache, want false")
	}
}

func TestSyncWithChainAddsProviders(t *testing.T) {
	c := openTestCache(t)

	providers := []rpc.OnChainProvider{
		{
			Owner:      "akash1owner1",
			HostURI:    "https://provider1.example.com:8443",
			Attributes: map[string]string{"organization": "Org1", "country": "US"},
			IsOnline:   true,
		},
		{
			Owner:      "akash1owner2",
			HostURI:    "https://provider2.example.com:8443",
			Attributes: map[string]string{"organization": "Org2", "country": "DE"},
			IsOnline:   false,
		},
	}

	newOwners := c.SyncWithChain(providers)
	if len(newOwners) != 2 {
		t.Errorf("SyncWithChain returned %d new owners, want 2", len(newOwners))
	}

	if !c.HasProviders() {
		t.Error("HasProviders() = false after SyncWithChain, want true")
	}

	all := c.GetAllProviders()
	if len(all) != 2 {
		t.Errorf("GetAllProviders() returned %d, want 2", len(all))
	}

	// Verify provider data.
	p1, ok := all["akash1owner1"]
	if !ok {
		t.Fatal("provider akash1owner1 not found")
	}
	if p1.HostURI != "https://provider1.example.com:8443" {
		t.Errorf("HostURI = %q, want %q", p1.HostURI, "https://provider1.example.com:8443")
	}
	if p1.Name != "Org1" {
		t.Errorf("Name = %q, want %q", p1.Name, "Org1")
	}
}

func TestMarkProviderOnline(t *testing.T) {
	c := openTestCache(t)

	// Seed a provider.
	c.SyncWithChain([]rpc.OnChainProvider{
		{
			Owner:      "akash1owner1",
			HostURI:    "https://provider1.example.com:8443",
			Attributes: map[string]string{"organization": "Org1"},
		},
	})

	c.MarkProviderOnline("akash1owner1", "0.6.0", 8000, 16000, 32000, 64000, 2, 4, []string{"A100"})

	p, ok := c.GetProvider("akash1owner1")
	if !ok {
		t.Fatal("GetProvider returned not found after MarkProviderOnline")
	}
	if !p.IsOnline {
		t.Error("IsOnline = false, want true")
	}
	if p.Version != "0.6.0" {
		t.Errorf("Version = %q, want %q", p.Version, "0.6.0")
	}
	if p.CPUAvailable != 8000 {
		t.Errorf("CPUAvailable = %d, want 8000", p.CPUAvailable)
	}
	if p.CPUTotal != 16000 {
		t.Errorf("CPUTotal = %d, want 16000", p.CPUTotal)
	}
	if p.MemAvailable != 32000 {
		t.Errorf("MemAvailable = %d, want 32000", p.MemAvailable)
	}
	if p.MemTotal != 64000 {
		t.Errorf("MemTotal = %d, want 64000", p.MemTotal)
	}
	if p.GPUAvailable != 2 {
		t.Errorf("GPUAvailable = %d, want 2", p.GPUAvailable)
	}
	if p.GPUTotal != 4 {
		t.Errorf("GPUTotal = %d, want 4", p.GPUTotal)
	}
	if len(p.GPUModels) != 1 || p.GPUModels[0] != "A100" {
		t.Errorf("GPUModels = %v, want [A100]", p.GPUModels)
	}
	if p.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", p.ConsecutiveFailures)
	}
}

func TestMarkProviderOffline(t *testing.T) {
	c := openTestCache(t)

	c.SyncWithChain([]rpc.OnChainProvider{
		{
			Owner:      "akash1owner1",
			HostURI:    "https://provider1.example.com:8443",
			Attributes: map[string]string{"organization": "Org1"},
			IsOnline:   true,
		},
	})

	// Mark online first, then offline.
	c.MarkProviderOnline("akash1owner1", "0.6.0", 8000, 16000, 32000, 64000, 0, 0, nil)
	c.MarkProviderOffline("akash1owner1")

	p, ok := c.GetProvider("akash1owner1")
	if !ok {
		t.Fatal("GetProvider returned not found after MarkProviderOffline")
	}
	if p.IsOnline {
		t.Error("IsOnline = true after MarkProviderOffline, want false")
	}
	if p.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", p.ConsecutiveFailures)
	}
}

func TestGetOnlineProviders(t *testing.T) {
	c := openTestCache(t)

	c.SyncWithChain([]rpc.OnChainProvider{
		{Owner: "akash1a", HostURI: "https://a.example.com", Attributes: map[string]string{}},
		{Owner: "akash1b", HostURI: "https://b.example.com", Attributes: map[string]string{}},
		{Owner: "akash1c", HostURI: "https://c.example.com", Attributes: map[string]string{}},
	})

	c.MarkProviderOnline("akash1a", "v1", 100, 200, 100, 200, 0, 0, nil)
	c.MarkProviderOnline("akash1c", "v1", 100, 200, 100, 200, 0, 0, nil)
	// akash1b stays offline.

	online := c.GetOnlineProviders()
	if len(online) != 2 {
		t.Errorf("GetOnlineProviders() returned %d, want 2", len(online))
	}
}

func TestProviderCountAndOnlineCount(t *testing.T) {
	c := openTestCache(t)

	c.SyncWithChain([]rpc.OnChainProvider{
		{Owner: "akash1a", HostURI: "https://a.example.com", Attributes: map[string]string{}},
		{Owner: "akash1b", HostURI: "https://b.example.com", Attributes: map[string]string{}},
		{Owner: "akash1c", HostURI: "https://c.example.com", Attributes: map[string]string{}},
	})

	if got := c.ProviderCount(); got != 3 {
		t.Errorf("ProviderCount() = %d, want 3", got)
	}
	if got := c.OnlineCount(); got != 0 {
		t.Errorf("OnlineCount() = %d on fresh sync, want 0", got)
	}

	c.MarkProviderOnline("akash1a", "v1", 100, 200, 100, 200, 0, 0, nil)
	if got := c.OnlineCount(); got != 1 {
		t.Errorf("OnlineCount() = %d after marking one online, want 1", got)
	}
}

func TestSavePersists(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")

	// Open, write, close.
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	c, err := Open(db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	c.SyncWithChain([]rpc.OnChainProvider{
		{Owner: "akash1persist", HostURI: "https://persist.example.com", Attributes: map[string]string{"organization": "PersistOrg"}},
	})
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	db.Close()

	// Reopen and verify.
	db2, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB (reopen): %v", err)
	}
	defer db2.Close()

	c2, err := Open(db2)
	if err != nil {
		t.Fatalf("Open (reopen): %v", err)
	}
	if !c2.HasProviders() {
		t.Error("HasProviders() = false after reopen, want true")
	}
	all := c2.GetAllProviders()
	if len(all) != 1 {
		t.Errorf("GetAllProviders() returned %d after reopen, want 1", len(all))
	}
	p, ok := all["akash1persist"]
	if !ok {
		t.Fatal("provider akash1persist not found after reopen")
	}
	if p.Name != "PersistOrg" {
		t.Errorf("Name = %q after reopen, want %q", p.Name, "PersistOrg")
	}
}

func TestGetProviderNotFound(t *testing.T) {
	c := openTestCache(t)
	_, ok := c.GetProvider("nonexistent")
	if ok {
		t.Error("GetProvider(\"nonexistent\") returned ok=true, want false")
	}
}
