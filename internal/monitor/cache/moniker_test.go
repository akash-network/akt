package cache

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// openTestMonikerCache creates a temporary bbolt database and returns an open MonikerCache.
func openTestMonikerCache(t *testing.T) *MonikerCache {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "moniker_test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mc, err := OpenMonikerCache(db)
	if err != nil {
		t.Fatalf("OpenMonikerCache: %v", err)
	}
	return mc
}

func TestOpenMonikerCacheCreatesValid(t *testing.T) {
	mc := openTestMonikerCache(t)
	if mc == nil {
		t.Fatal("OpenMonikerCache returned nil")
	}
}

func TestMonikerSetGetRoundTrip(t *testing.T) {
	mc := openTestMonikerCache(t)

	monikers := map[string]string{
		"pubkey1": "Validator Alpha",
		"pubkey2": "Validator Beta",
	}
	mc.Set(monikers)

	got := mc.Get()
	if len(got) != 2 {
		t.Fatalf("Get() returned %d entries, want 2", len(got))
	}
	if got["pubkey1"] != "Validator Alpha" {
		t.Errorf("Get()[\"pubkey1\"] = %q, want %q", got["pubkey1"], "Validator Alpha")
	}
	if got["pubkey2"] != "Validator Beta" {
		t.Errorf("Get()[\"pubkey2\"] = %q, want %q", got["pubkey2"], "Validator Beta")
	}
}

func TestHasMonikersAfterSet(t *testing.T) {
	mc := openTestMonikerCache(t)

	if mc.HasMonikers() {
		t.Error("HasMonikers() = true on empty cache, want false")
	}

	mc.Set(map[string]string{"pk1": "Val1"})

	if !mc.HasMonikers() {
		t.Error("HasMonikers() = false after Set, want true")
	}
}

func TestMonikerUnknownKeyReturnsEmpty(t *testing.T) {
	mc := openTestMonikerCache(t)

	mc.Set(map[string]string{"pk1": "Val1"})

	got := mc.Get()
	if v := got["unknown_key"]; v != "" {
		t.Errorf("Get()[\"unknown_key\"] = %q, want empty string", v)
	}
}

func TestMonikerSavePersists(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "moniker_persist.db")

	// Open, write, close.
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	mc, err := OpenMonikerCache(db)
	if err != nil {
		t.Fatalf("OpenMonikerCache: %v", err)
	}
	mc.Set(map[string]string{"pk_persist": "Persistent Validator"})
	if err := mc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	db.Close()

	// Reopen and verify.
	db2, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB (reopen): %v", err)
	}
	defer db2.Close()

	mc2, err := OpenMonikerCache(db2)
	if err != nil {
		t.Fatalf("OpenMonikerCache (reopen): %v", err)
	}
	if !mc2.HasMonikers() {
		t.Error("HasMonikers() = false after reopen, want true")
	}
	got := mc2.Get()
	if got["pk_persist"] != "Persistent Validator" {
		t.Errorf("Get()[\"pk_persist\"] = %q after reopen, want %q", got["pk_persist"], "Persistent Validator")
	}
}

func TestMonikerSetReplacesAll(t *testing.T) {
	mc := openTestMonikerCache(t)

	mc.Set(map[string]string{"pk1": "Val1", "pk2": "Val2"})
	mc.Set(map[string]string{"pk3": "Val3"})

	got := mc.Get()
	if len(got) != 1 {
		t.Errorf("Get() returned %d entries after replace, want 1", len(got))
	}
	if got["pk3"] != "Val3" {
		t.Errorf("Get()[\"pk3\"] = %q, want %q", got["pk3"], "Val3")
	}
	if _, exists := got["pk1"]; exists {
		t.Error("old key pk1 still present after Set replacement")
	}
}

func TestCacheOpenFailuresAreReported(t *testing.T) {
	db, err := bolt.Open(filepath.Join(t.TempDir(), "closed.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(db); err == nil {
		t.Fatal("Open accepted a closed database")
	}
	if _, err := OpenMonikerCache(db); err == nil {
		t.Fatal("OpenMonikerCache accepted a closed database")
	}

	directory := t.TempDir()
	if _, err := OpenDB(directory); err == nil {
		t.Fatal("OpenDB accepted a directory as a database file")
	} else if !strings.Contains(err.Error(), directory) {
		t.Errorf("OpenDB error = %q, want path %q", err, directory)
	}

	// Keep the os import tied to the contract: the directory remains intact
	// after the failed open.
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		t.Fatalf("database failure changed target directory: info=%v err=%v", info, err)
	}
}

func TestOpenStoresInitializesAllBucketsAndReportsClosedDB(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "stores.db"))
	if err != nil {
		t.Fatal(err)
	}
	providers, monikers, err := OpenStores(db)
	if err != nil {
		t.Fatal(err)
	}
	if providers == nil || monikers == nil {
		t.Fatalf("OpenStores() returned nil handles: providers=%v monikers=%v", providers, monikers)
	}
	if err := db.View(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bucketProviders, bucketMeta, bucketMonikers} {
			if tx.Bucket(name) == nil {
				return errors.New("OpenStores omitted bucket " + string(name))
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenStores(db); err == nil {
		t.Fatal("OpenStores accepted a closed database")
	}
}

func TestMonikerCacheWithoutBucketReadsEmpty(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "raw-moniker.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cache := &MonikerCache{db: db}
	if cache.HasMonikers() {
		t.Fatal("moniker cache without a bucket reported entries")
	}
	if got := cache.Get(); len(got) != 0 {
		t.Fatalf("moniker cache without a bucket = %v, want empty", got)
	}
}
