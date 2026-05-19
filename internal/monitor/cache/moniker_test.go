package cache

import (
	"path/filepath"
	"testing"
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
