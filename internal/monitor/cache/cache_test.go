package cache

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"pkg.akt.dev/akt/internal/monitor/rpc"

	bolt "go.etcd.io/bbolt"
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

func TestProviderSchedulingIntervals(t *testing.T) {
	c := openTestCache(t)
	now := time.Now()

	fixtures := map[string]*CachedProvider{
		"unchecked":      {},
		"online-due":     {IsOnline: true, LastChecked: now.Add(-OnlineCheckInterval - time.Second)},
		"online-recent":  {IsOnline: true, LastChecked: now},
		"offline-due":    {LastChecked: now.Add(-RecentOfflineInterval - time.Second), ConsecutiveFailures: 1},
		"offline-recent": {LastChecked: now, ConsecutiveFailures: 1},
		"long-due":       {LastChecked: now.Add(-LongTermOfflineInterval - time.Second), ConsecutiveFailures: LongTermOfflineThreshold},
		"long-recent":    {LastChecked: now, ConsecutiveFailures: LongTermOfflineThreshold},
	}
	for owner, provider := range fixtures {
		putCachedProvider(t, c, owner, provider)
	}

	got := c.GetProvidersDueForCheck()
	sort.Strings(got)
	want := []string{"long-due", "offline-due", "online-due", "unchecked"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("providers due = %v, want %v", got, want)
	}
}

func TestProviderPriorityOrdersStateThenOldestCheck(t *testing.T) {
	c := openTestCache(t)
	now := time.Now()

	fixtures := map[string]*CachedProvider{
		"unchecked":     {},
		"online-oldest": {IsOnline: true, LastChecked: now.Add(-2 * time.Minute)},
		"online-newest": {IsOnline: true, LastChecked: now.Add(-time.Minute)},
		"offline":       {LastChecked: now.Add(-time.Hour), ConsecutiveFailures: 1},
		"long-offline":  {LastChecked: now.Add(-24 * time.Hour), ConsecutiveFailures: LongTermOfflineThreshold},
	}
	for owner, provider := range fixtures {
		putCachedProvider(t, c, owner, provider)
	}

	want := []string{"unchecked", "online-oldest", "online-newest", "offline", "long-offline"}
	if got := c.GetProvidersByPriority(); !reflect.DeepEqual(got, want) {
		t.Errorf("provider priority = %v, want %v", got, want)
	}
}

func TestSyncWithChainRefreshesMetadataWithoutLosingHealth(t *testing.T) {
	c := openTestCache(t)
	c.SyncWithChain([]rpc.OnChainProvider{{
		Owner:      "akash1existing",
		HostURI:    "https://old.example.com",
		Attributes: map[string]string{"organization": "Old Org", "country": "US"},
	}})
	c.MarkProviderOnline("akash1existing", "v1.2.3", 1, 2, 3, 4, 5, 6, []string{"A100"})

	newOwners := c.SyncWithChain([]rpc.OnChainProvider{
		{
			Owner:      "akash1existing",
			HostURI:    "https://new.example.com",
			Attributes: map[string]string{"region": "us-west"},
		},
		{
			Owner:      "akash1new",
			HostURI:    "https://provider.example.com:8443/status",
			Attributes: map[string]string{},
			IsOnline:   true,
		},
	})

	if !reflect.DeepEqual(newOwners, []string{"akash1new"}) {
		t.Fatalf("new owners = %v, want [akash1new]", newOwners)
	}
	existing, ok := c.GetProvider("akash1existing")
	if !ok {
		t.Fatal("existing provider disappeared")
	}
	if existing.HostURI != "https://new.example.com" || existing.Name != "Old Org" || existing.Country != "US" {
		t.Errorf("existing metadata = %+v", existing)
	}
	if !existing.IsOnline || existing.Version != "v1.2.3" || existing.GPUTotal != 6 {
		t.Errorf("existing health was lost during chain refresh: %+v", existing)
	}
	if got := existing.Attributes["region"]; got != "us-west" {
		t.Errorf("refreshed region = %q, want us-west", got)
	}

	added, ok := c.GetProvider("akash1new")
	if !ok {
		t.Fatal("new provider missing")
	}
	if added.Name != "provider.example.com" {
		t.Errorf("hostname fallback = %q, want provider.example.com", added.Name)
	}
	if !added.IsOnline || added.LastSeenOnline.IsZero() {
		t.Errorf("new online provider did not record availability: %+v", added)
	}
}

func TestCorruptProviderRowsAreIsolated(t *testing.T) {
	c := openTestCache(t)
	c.SyncWithChain([]rpc.OnChainProvider{{Owner: "healthy", HostURI: "https://healthy.example.com", IsOnline: true}})
	if err := c.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketProviders).Put([]byte("corrupt"), []byte("{"))
	}); err != nil {
		t.Fatalf("seed corrupt row: %v", err)
	}

	if _, ok := c.GetProvider("corrupt"); ok {
		t.Error("corrupt provider decoded successfully")
	}
	if got := c.GetAllProviders(); len(got) != 1 || got["healthy"] == nil {
		t.Errorf("all providers = %#v, want only healthy row", got)
	}
	if got := c.GetOnlineProviders(); len(got) != 1 || got[0].HostURI != "https://healthy.example.com" {
		t.Errorf("online providers = %#v, want healthy row", got)
	}
	if got := c.OnlineCount(); got != 1 {
		t.Errorf("online count = %d, want 1", got)
	}

	// Mutators and schedulers must skip only the bad row, not panic or hide
	// healthy state.
	c.MarkProviderOnline("corrupt", "v1", 0, 0, 0, 0, 0, 0, nil)
	c.MarkProviderOffline("corrupt")
	_ = c.GetProvidersDueForCheck()
	if got := c.GetProvidersByPriority(); !reflect.DeepEqual(got, []string{"healthy"}) {
		t.Errorf("provider priority with corrupt row = %v", got)
	}
}

func TestMarkProviderMissingIsNoop(t *testing.T) {
	c := openTestCache(t)
	c.MarkProviderOnline("missing", "v1", 1, 2, 3, 4, 5, 6, nil)
	c.MarkProviderOffline("missing")
	if c.HasProviders() {
		t.Fatal("marking a missing provider created cache state")
	}
}

func TestProviderCacheWithoutBucketsIsEmptyAndSafe(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "raw.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := &ProviderCache{db: db}

	if c.HasProviders() || c.ProviderCount() != 0 || c.OnlineCount() != 0 {
		t.Fatal("cache without buckets reported providers")
	}
	if _, ok := c.GetProvider("missing"); ok {
		t.Fatal("cache without buckets returned a provider")
	}
	if len(c.GetAllProviders()) != 0 || len(c.GetOnlineProviders()) != 0 {
		t.Fatal("cache without buckets returned provider rows")
	}
	c.MarkProviderOnline("missing", "v1", 0, 0, 0, 0, 0, 0, nil)
	c.MarkProviderOffline("missing")
	if got := c.SyncWithChain([]rpc.OnChainProvider{{Owner: "ignored"}}); len(got) != 0 {
		t.Errorf("sync without buckets returned new providers: %v", got)
	}
	if len(c.GetProvidersDueForCheck()) != 0 || len(c.GetProvidersByPriority()) != 0 {
		t.Fatal("cache without buckets scheduled providers")
	}
}

func TestCalculatePriorityAndHostnameBoundaries(t *testing.T) {
	now := time.Now()
	priorities := []struct {
		provider CachedProvider
		want     int
	}{
		{provider: CachedProvider{}, want: 0},
		{provider: CachedProvider{LastChecked: now, IsOnline: true}, want: 1},
		{provider: CachedProvider{LastChecked: now, ConsecutiveFailures: LongTermOfflineThreshold - 1}, want: 2},
		{provider: CachedProvider{LastChecked: now, ConsecutiveFailures: LongTermOfflineThreshold}, want: 3},
	}
	for _, tc := range priorities {
		if got := calculatePriority(&tc.provider); got != tc.want {
			t.Errorf("calculatePriority(%+v) = %d, want %d", tc.provider, got, tc.want)
		}
	}

	hosts := map[string]string{
		"https://provider.example.com:8443/path": "provider.example.com",
		"http://provider.example.com/path":       "provider.example.com",
		"provider.example.com":                   "provider.example.com",
	}
	for input, want := range hosts {
		if got := extractHostname(input); got != want {
			t.Errorf("extractHostname(%q) = %q, want %q", input, got, want)
		}
	}
}

func putCachedProvider(t *testing.T, cache *ProviderCache, owner string, provider *CachedProvider) {
	t.Helper()
	if err := cache.db.Update(func(tx *bolt.Tx) error {
		return putProvider(tx.Bucket(bucketProviders), owner, provider)
	}); err != nil {
		t.Fatalf("put cached provider %s: %v", owner, err)
	}
}
