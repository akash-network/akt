package context_test

import (
	"os"
	"path/filepath"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// TestSetConsoleAPIKeyTightensExistingPermissions covers the explicit Chmod
// after WriteFile. os.WriteFile does NOT change the mode of a file that
// already exists, so a credential file left world-readable by an earlier akt
// version (or by a user's editor) would silently stay that way across a key
// rotation. This is the branch that fixes it.
func TestSetConsoleAPIKeyTightensExistingPermissions(t *testing.T) {
	root := t.TempDir()
	path := aktctx.ConsoleAPIKeyPath(root, "prod")

	// Pre-create a world-readable credential file, as a pre-hardening akt or a
	// careless editor would leave behind.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("sk-old\n"), 0o644); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Skipf("filesystem does not honor 0644 (got %o); permission test not meaningful", info.Mode().Perm())
	}

	if err := aktctx.SetConsoleAPIKey(root, "prod", "sk-new"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("stat after write: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode after rewrite = %o, want 600 (an existing loose mode must be tightened)", perm)
	}

	if got, _ := aktctx.StoredConsoleAPIKey(root, "prod"); got != "sk-new" {
		t.Errorf("stored key = %q, want sk-new", got)
	}
}

// TestSetConsoleAPIKeyCreatesRestrictedDirectory covers the MkdirAll branch for
// a context whose directory does not exist yet: the credential's parent must
// be created 0700, not with the process umask default.
func TestSetConsoleAPIKeyCreatesRestrictedDirectory(t *testing.T) {
	root := t.TempDir()

	if err := aktctx.SetConsoleAPIKey(root, "fresh", "sk-1"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	info, err := os.Stat(filepath.Dir(aktctx.ConsoleAPIKeyPath(root, "fresh")))
	if err != nil {
		t.Fatalf("stat context dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("context directory mode = %o, want no group/other access", perm)
	}
}

// TestRemoveMissingConsoleAPIKeyIsNoError covers the os.IsNotExist arm of the
// removal branch. `akt console logout` on a context that never had a key must
// succeed, so logout is safe to script.
func TestRemoveMissingConsoleAPIKeyIsNoError(t *testing.T) {
	root := t.TempDir()

	if err := aktctx.SetConsoleAPIKey(root, "never-had-one", ""); err != nil {
		t.Errorf("removing an absent credential must be a no-op, got %v", err)
	}
}

// TestStoredConsoleAPIKeySurfacesRealReadErrors covers the non-IsNotExist arm
// of the read: a credential path that exists but cannot be read as a file
// (here: it is a directory) must be an error, not an empty key that silently
// downgrades the caller to "unauthenticated".
func TestStoredConsoleAPIKeySurfacesRealReadErrors(t *testing.T) {
	root := t.TempDir()
	path := aktctx.ConsoleAPIKeyPath(root, "weird")

	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir credential-as-directory: %v", err)
	}

	key, err := aktctx.StoredConsoleAPIKey(root, "weird")
	if err == nil {
		t.Fatal("an unreadable credential path must be an error, not an empty key")
	}
	if key != "" {
		t.Errorf("key = %q, want empty on error", key)
	}
}

// TestStoredConsoleAPIKeyTrimsWhitespace pins the trim: keys are written with a
// trailing newline, and an untrimmed value would be sent in the x-api-key
// header verbatim and rejected as invalid.
func TestStoredConsoleAPIKeyTrimsWhitespace(t *testing.T) {
	root := t.TempDir()
	path := aktctx.ConsoleAPIKeyPath(root, "prod")

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("  sk-padded \n\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := aktctx.StoredConsoleAPIKey(root, "prod")
	if err != nil {
		t.Fatalf("StoredConsoleAPIKey: %v", err)
	}
	if got != "sk-padded" {
		t.Errorf("key = %q, want the trimmed value", got)
	}
}

// TestResolveConsoleAPIKeyPrecedence covers the documented resolution order
// (SPEC §7.1) at the function that implements it: the environment variable
// wins, then the stored credential, then nothing.
func TestResolveConsoleAPIKeyPrecedence(t *testing.T) {
	root := t.TempDir()

	t.Setenv(aktctx.EnvConsoleAPIKey, "")

	if got := aktctx.ResolveConsoleAPIKey(root, "prod"); got != "" {
		t.Errorf("with nothing configured: %q, want empty", got)
	}

	if err := aktctx.SetConsoleAPIKey(root, "prod", "sk-stored"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}
	if got := aktctx.ResolveConsoleAPIKey(root, "prod"); got != "sk-stored" {
		t.Errorf("with only a stored key: %q, want sk-stored", got)
	}

	t.Setenv(aktctx.EnvConsoleAPIKey, "sk-env")
	if got := aktctx.ResolveConsoleAPIKey(root, "prod"); got != "sk-env" {
		t.Errorf("with the env var set: %q, want sk-env", got)
	}

	// Another context's stored key must never leak into this one.
	if got := aktctx.ResolveConsoleAPIKey(root, "other"); got != "sk-env" {
		t.Errorf("env var applies to any context: %q, want sk-env", got)
	}
	t.Setenv(aktctx.EnvConsoleAPIKey, "")
	if got := aktctx.ResolveConsoleAPIKey(root, "other"); got != "" {
		t.Errorf("a different context must not see prod's key, got %q", got)
	}
}

// TestConsoleAPIKeyPathStaysInsideContextDir pins the placement decision that
// makes rename and delete work: the credential lives inside the context's own
// directory, so moving or removing that directory takes the key with it.
func TestConsoleAPIKeyPathStaysInsideContextDir(t *testing.T) {
	root := t.TempDir()

	path := aktctx.ConsoleAPIKeyPath(root, "prod")
	dir := aktctx.ContextDir(root, "prod")

	rel, err := filepath.Rel(dir, path)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	if rel != "console-api-key" {
		t.Errorf("credential path %q is not directly inside the context dir %q (rel=%q)", path, dir, rel)
	}
}
