package keyring_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func TestGetTestKeyring(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	keyrings := []aktctx.Keyring{
		{Name: "default", Backend: sdkkeyring.BackendTest},
	}

	mgr := aktkeyring.NewManager(root, keyrings, cdc)

	kr, err := mgr.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if kr == nil {
		t.Fatal("expected non-nil keyring")
	}

	// Verify it's a test backend.
	if kr.Backend() != sdkkeyring.BackendTest {
		t.Errorf("backend = %q, want %q", kr.Backend(), sdkkeyring.BackendTest)
	}

	// kr.Backend() only echoes what was asked for -- it would report "os" on a
	// host where the SDK had silently opened a file keyring instead. Assert
	// the store this host actually provides as well (SPEC §1.5).
	effective, available := mgr.EffectiveBackend("default")
	if !available {
		t.Fatal("the test backend must be available on every host")
	}
	if effective != sdkkeyring.BackendTest {
		t.Errorf("effective backend = %q, want %q", effective, sdkkeyring.BackendTest)
	}
}

func TestGetCached(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	keyrings := []aktctx.Keyring{
		{Name: "default", Backend: sdkkeyring.BackendTest},
	}

	mgr := aktkeyring.NewManager(root, keyrings, cdc)
	mgr.SetInput(strings.NewReader(""))

	kr1, err := mgr.Get("default")
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}

	// Add a key via the first reference.
	algo := aktkeyring.DefaultAlgo()
	_, _, err = kr1.NewMnemonic("cachetest", sdkkeyring.English, "m/44'/118'/0'/0/0", "", algo)
	if err != nil {
		t.Fatalf("NewMnemonic: %v", err)
	}

	// Second Get should return the cached instance (key visible without re-open).
	kr2, err := mgr.Get("default")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}

	rec, err := kr2.Key("cachetest")
	if err != nil {
		t.Fatal("expected key to be visible from cached keyring")
	}

	if rec.Name != "cachetest" {
		t.Errorf("name = %q, want cachetest", rec.Name)
	}
}

func TestGetNotFound(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	mgr := aktkeyring.NewManager(root, nil, cdc)

	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent keyring")
	}
	if backend, ok := mgr.EffectiveBackend("nonexistent"); ok || backend != "" {
		t.Errorf("missing effective backend = (%q, %v), want empty/false", backend, ok)
	}
}

func TestGetByName(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	keyrings := []aktctx.Keyring{
		{Name: "mykr", Backend: sdkkeyring.BackendTest},
	}

	mgr := aktkeyring.NewManager(root, keyrings, cdc)

	kr, err := mgr.Get("mykr")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if kr.Backend() != sdkkeyring.BackendTest {
		t.Errorf("backend = %q, want %q", kr.Backend(), sdkkeyring.BackendTest)
	}

	if effective, available := mgr.EffectiveBackend("mykr"); !available || effective != sdkkeyring.BackendTest {
		t.Errorf("effective backend = (%q, %v), want (%q, true)", effective, available, sdkkeyring.BackendTest)
	}
}

func TestGetDefault(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	keyrings := []aktctx.Keyring{
		{Name: "default", Backend: sdkkeyring.BackendTest},
	}

	mgr := aktkeyring.NewManager(root, keyrings, cdc)

	kr, err := mgr.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if kr == nil {
		t.Fatal("expected non-nil keyring for default")
	}
}

func TestMultipleKeyrings(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	keyrings := []aktctx.Keyring{
		{Name: "kr1", Backend: sdkkeyring.BackendTest},
		{Name: "kr2", Backend: sdkkeyring.BackendTest},
	}

	mgr := aktkeyring.NewManager(root, keyrings, cdc)
	mgr.SetInput(strings.NewReader(""))

	kr1, err := mgr.Get("kr1")
	if err != nil {
		t.Fatalf("Get kr1: %v", err)
	}

	kr2, err := mgr.Get("kr2")
	if err != nil {
		t.Fatalf("Get kr2: %v", err)
	}

	// Add key to kr1, should NOT be visible from kr2 (separate instances).
	algo := aktkeyring.DefaultAlgo()
	_, _, _ = kr1.NewMnemonic("onlykr1", sdkkeyring.English, "m/44'/118'/0'/0/0", "", algo)

	_, err = kr2.Key("onlykr1")
	if err == nil {
		t.Error("key from kr1 should not be visible in kr2")
	}
}

func TestReload(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	keyrings := []aktctx.Keyring{
		{Name: "default", Backend: sdkkeyring.BackendTest},
	}

	mgr := aktkeyring.NewManager(root, keyrings, cdc)
	mgr.SetInput(strings.NewReader(""))

	// Get to populate cache; add a key.
	kr1, _ := mgr.Get("default")
	algo := aktkeyring.DefaultAlgo()
	_, _, _ = kr1.NewMnemonic("beforereload", sdkkeyring.English, "m/44'/118'/0'/0/0", "", algo)

	// Reload with a changed dir evicts the cache.
	newDir := t.TempDir()
	mgr.Reload([]aktctx.Keyring{
		{Name: "default", Backend: sdkkeyring.BackendTest, Dir: newDir},
	})

	// Should create a new instance (different dir, so old keys gone).
	kr2, err := mgr.Get("default")
	if err != nil {
		t.Fatalf("Get after reload: %v", err)
	}

	if kr2 == nil {
		t.Fatal("expected non-nil keyring after reload")
	}

	// Key from before reload should not exist in the new keyring dir.
	_, err = kr2.Key("beforereload")
	if err == nil {
		t.Error("expected key from old keyring dir to not be present after reload with new dir")
	}
}

func TestReloadEvictsRemovedKeyring(t *testing.T) {
	root := t.TempDir()
	mgr := aktkeyring.NewManager(root, []aktctx.Keyring{{Name: "removed", Backend: sdkkeyring.BackendTest}}, aktcodec.MakeEncodingConfig().Codec)
	if _, err := mgr.Get("removed"); err != nil {
		t.Fatal(err)
	}
	mgr.Reload(nil)
	if _, err := mgr.Get("removed"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("removed keyring error = %v", err)
	}
}

func TestNames(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	keyrings := []aktctx.Keyring{
		{Name: "alpha", Backend: sdkkeyring.BackendTest},
		{Name: "beta", Backend: sdkkeyring.BackendTest},
	}

	mgr := aktkeyring.NewManager(root, keyrings, cdc)

	names := mgr.Names()
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}

func TestInMemoryKeyring(t *testing.T) {
	cdc := aktcodec.MakeEncodingConfig().Codec

	kr := aktkeyring.NewInMemory(cdc)
	if kr == nil {
		t.Fatal("expected non-nil in-memory keyring")
	}

	if kr.Backend() != sdkkeyring.BackendMemory {
		t.Errorf("backend = %q, want %q", kr.Backend(), sdkkeyring.BackendMemory)
	}
}

// TestReloadAppliesOverriddenBackend covers the per-invocation
// --keyring-backend override reaching the manager: a context configured for
// "os" must open the overridden store, which is what makes `akt context keys`
// usable on a host that cannot provide the configured one.
func TestReloadAppliesOverriddenBackend(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	configured := []aktctx.Keyring{{Name: "default", Backend: sdkkeyring.BackendOS}}

	mgr := aktkeyring.NewManager(root, aktkeyring.ApplyOverrides(configured, sdkkeyring.BackendTest, ""), cdc)
	mgr.SetInput(strings.NewReader(""))

	kr, err := mgr.Get("default")
	if err != nil {
		t.Fatalf("Get with an overridden backend: %v", err)
	}

	if kr.Backend() != sdkkeyring.BackendTest {
		t.Errorf("backend = %q, want the override %q", kr.Backend(), sdkkeyring.BackendTest)
	}

	if effective, available := mgr.EffectiveBackend("default"); !available || effective != sdkkeyring.BackendTest {
		t.Errorf("effective backend = (%q, %v), want (%q, true)", effective, available, sdkkeyring.BackendTest)
	}
}

func TestKeyringAddAndList(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	keyrings := []aktctx.Keyring{
		{Name: "default", Backend: sdkkeyring.BackendTest},
	}

	mgr := aktkeyring.NewManager(root, keyrings, cdc)
	mgr.SetInput(strings.NewReader(""))

	kr, err := mgr.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Add a key.
	algo := aktkeyring.DefaultAlgo()
	rec, _, err := kr.NewMnemonic("testkey", sdkkeyring.English, "m/44'/118'/0'/0/0", "", algo)
	if err != nil {
		t.Fatalf("NewMnemonic: %v", err)
	}

	if rec.Name != "testkey" {
		t.Errorf("name = %q, want testkey", rec.Name)
	}

	// List should show the key.
	records, err := kr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("expected 1 key, got %d", len(records))
	}

	// Key should have an address.
	addr, err := rec.GetAddress()
	if err != nil {
		t.Fatalf("GetAddress: %v", err)
	}

	if addr.Empty() {
		t.Error("expected non-empty address")
	}
}

func TestKeyringSharedAcrossContexts(t *testing.T) {
	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	keyrings := []aktctx.Keyring{
		{Name: "shared", Backend: sdkkeyring.BackendTest},
	}

	mgr := aktkeyring.NewManager(root, keyrings, cdc)
	mgr.SetInput(strings.NewReader(""))

	// Both contexts reference the same "shared" keyring by name.
	// Get it twice -- should return the same cached instance.
	kr1, _ := mgr.Get("shared")
	kr2, _ := mgr.Get("shared")

	// Add a key via the first reference.
	algo := aktkeyring.DefaultAlgo()
	_, _, err := kr1.NewMnemonic("sharedkey", sdkkeyring.English, "m/44'/118'/0'/0/0", "", algo)
	if err != nil {
		t.Fatalf("NewMnemonic: %v", err)
	}

	// Should be visible from the second reference (same cached instance).
	rec, err := kr2.Key("sharedkey")
	if err != nil {
		t.Fatalf("Key from kr2: %v", err)
	}

	if rec.Name != "sharedkey" {
		t.Errorf("expected sharedkey, got %q", rec.Name)
	}
}

func TestCodecNotNil(t *testing.T) {
	cdc := aktcodec.MakeEncodingConfig().Codec
	if cdc == nil {
		t.Fatal("expected non-nil codec")
	}
}

func TestManagerDeferredLoadsOnceOnDemand(t *testing.T) {
	root := t.TempDir()
	mgr := aktkeyring.NewManager(root, []aktctx.Keyring{{Name: "default", Backend: sdkkeyring.BackendTest}}, aktcodec.MakeEncodingConfig().Codec)
	deferred := mgr.Deferred("default")
	if got := deferred.Backend(); got != sdkkeyring.BackendTest {
		t.Fatalf("deferred backend = %q, want test", got)
	}
	if _, err := deferred.List(); err != nil {
		t.Fatalf("deferred list: %v", err)
	}
	if _, err := deferred.List(); err != nil {
		t.Fatalf("second deferred list: %v", err)
	}
	first, err := mgr.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	second, err := mgr.Get("default")
	if err != nil {
		t.Fatal(err)
	}
	if first.Backend() != sdkkeyring.BackendTest || second.Backend() != sdkkeyring.BackendTest {
		t.Fatalf("cached backends = %q/%q, want test", first.Backend(), second.Backend())
	}

	missing := mgr.Deferred("missing")
	if got := missing.Backend(); got != sdkkeyring.BackendOS {
		t.Errorf("missing deferred backend = %q, want os default", got)
	}
	if _, err := missing.List(); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing deferred list error = %v", err)
	}
}

func TestManagerRejectsUnknownBackendAndDirectoryFailure(t *testing.T) {
	cdc := aktcodec.MakeEncodingConfig().Codec
	t.Run("unknown backend", func(t *testing.T) {
		mgr := aktkeyring.NewManager(t.TempDir(), []aktctx.Keyring{{Name: "bad", Backend: "unknown"}}, cdc)
		if _, err := mgr.Get("bad"); err == nil || !strings.Contains(err.Error(), "unknown keyring backend") {
			t.Fatalf("unknown backend error = %v", err)
		}
	})

	t.Run("keyring directory cannot be created", func(t *testing.T) {
		root := t.TempDir()
		blockingFile := filepath.Join(root, "not-a-directory")
		if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		mgr := aktkeyring.NewManager(root, []aktctx.Keyring{{Name: "bad", Backend: sdkkeyring.BackendTest, Dir: filepath.Join(blockingFile, "child")}}, cdc)
		if _, err := mgr.Get("bad"); err == nil || !strings.Contains(err.Error(), "create keyring dir") {
			t.Fatalf("directory error = %v", err)
		}
	})
}
