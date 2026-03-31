package keyring_test

import (
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
