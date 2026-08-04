package keyring

import (
	"fmt"
	"runtime"
	"strings"

	krbackend "github.com/99designs/keyring"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// Backends returns every keyring backend akt accepts, in the order they are
// offered to users (SPEC §1.5).
func Backends() []string {
	return []string{
		sdkkeyring.BackendOS,
		sdkkeyring.BackendFile,
		sdkkeyring.BackendTest,
		sdkkeyring.BackendKWallet,
		sdkkeyring.BackendPass,
		sdkkeyring.BackendMemory,
	}
}

// ValidateBackend reports whether backend names a keyring akt can open. An
// empty value means "unset" and is accepted: callers treat it as "inherit the
// configured backend".
func ValidateBackend(backend string) error {
	if backend == "" {
		return nil
	}

	for _, known := range Backends() {
		if backend == known {
			return nil
		}
	}

	return fmt.Errorf("unknown keyring backend %q; valid backends: %s", backend, strings.Join(Backends(), ", "))
}

// UnavailableBackendError reports a configured keyring backend that this host
// cannot provide. It is deliberately fatal rather than a fallback: substituting
// a different credential store would move the user's keys without telling them
// (SPEC §1.5).
type UnavailableBackendError struct {
	// Keyring is the configured keyring name, e.g. "default".
	Keyring string
	// Backend is the configured backend, e.g. "os".
	Backend string
	// Expected lists the platform stores that could have served Backend.
	Expected []string
}

func (e *UnavailableBackendError) Error() string {
	expected := "none on this platform"
	if len(e.Expected) > 0 {
		expected = strings.Join(e.Expected, " or ")
	}

	return fmt.Sprintf(
		"keyring %q is configured with backend %q, but this host provides no system credential store (looked for %s)\n"+
			"akt will not silently store keys somewhere else; select a backend this host has:\n"+
			"  akt --keyring-backend file ...            (this invocation only)\n"+
			"  akt context keyring set %s file           (persist the change)",
		e.Keyring, e.Backend, expected, e.Keyring)
}

// EffectiveBackend reports the concrete credential store that serves kr's
// configured backend on this host, and whether the host can provide it at all.
//
// Only "os" is an alias: every other backend is pinned by the SDK to exactly
// one store, so it is its own effective backend. Resolution is inspection only
// — it never reads a key, unlocks a store, or prompts.
func EffectiveBackend(root string, kr aktctx.Keyring) (string, bool) {
	backend := kr.Backend
	if backend == "" {
		backend = sdkkeyring.BackendOS
	}

	if backend != sdkkeyring.BackendOS {
		return backend, true
	}

	effective := resolveSystemBackend(appName, aktctx.KeyringDir(root, kr))
	if effective == "" {
		return "", false
	}

	return effective, true
}

// SystemKeyringAvailable reports whether this host provides a credential store
// that can serve the "os" backend (SPEC §1.5). Offering "os" where this is
// false hands the user a backend that silently becomes something else.
func SystemKeyringAvailable() bool {
	// The directory only matters to the file backend, which this probe never
	// allows, so an empty one is complete.
	return resolveSystemBackend(appName, "") != ""
}

// SystemBackendNames names the credential stores that can serve the "os"
// backend on this platform, whether or not they are present.
func SystemBackendNames() []string {
	expected := systemBackends()

	names := make([]string, 0, len(expected))
	for _, backend := range expected {
		names = append(names, string(backend))
	}

	return names
}

// systemBackends returns the credential stores that may serve "os" on this
// platform, in the order github.com/99designs/keyring itself considers them.
func systemBackends() []krbackend.BackendType {
	switch runtime.GOOS {
	case "darwin":
		return []krbackend.BackendType{krbackend.KeychainBackend}
	case "windows":
		return []krbackend.BackendType{krbackend.WinCredBackend}
	case "linux":
		return []krbackend.BackendType{krbackend.SecretServiceBackend, krbackend.KWalletBackend}
	}

	return nil
}

// resolveSystemBackend returns the system credential store that the "os"
// backend resolves to, or "" when this host has none.
//
// The check is a pinned open of each candidate rather than a registration
// lookup: github.com/99designs/keyring registers the kernel keyring
// unconditionally on Linux and only fails when its opener runs, and a
// registered Secret Service can still fail to connect. Because the library
// orders every OS-specific backend ahead of "pass" and "file", a pinned open
// that succeeds is also proof that the SDK's unpinned open would land on the
// same store — which is what lets akt promise that "os" never becomes a file
// keyring behind the user's back (DESIGN §3.1.2.1).
func resolveSystemBackend(serviceName, dir string) string {
	registered := make(map[krbackend.BackendType]bool)
	for _, backend := range krbackend.AvailableBackends() {
		registered[backend] = true
	}

	for _, backend := range systemBackends() {
		if !registered[backend] {
			continue
		}

		cfg := systemKeyringConfig(serviceName, dir)
		cfg.AllowedBackends = []krbackend.BackendType{backend}

		if _, err := krbackend.Open(cfg); err != nil {
			continue
		}

		return string(backend)
	}

	return ""
}

// systemKeyringConfig mirrors the Cosmos SDK's own "os" backend config so the
// probe and the real open agree. FilePasswordFunc is deliberately absent: the
// file backend is never an allowed backend here, and leaving the prompt
// unwired makes that structural rather than incidental.
func systemKeyringConfig(serviceName, dir string) krbackend.Config {
	return krbackend.Config{
		ServiceName:              serviceName,
		FileDir:                  dir,
		KeychainTrustApplication: true,
	}
}

// ApplyOverrides returns keyring configurations with the per-invocation
// --keyring-backend / --keyring-dir overrides (SPEC §3.1) applied. Empty
// overrides leave the persisted configuration untouched, so an unset flag can
// never shadow a context's stored backend.
func ApplyOverrides(keyrings []aktctx.Keyring, backend, dir string) []aktctx.Keyring {
	if backend == "" && dir == "" {
		return keyrings
	}

	out := make([]aktctx.Keyring, len(keyrings))
	copy(out, keyrings)

	for i := range out {
		if backend != "" {
			out[i].Backend = backend
		}
		if dir != "" {
			out[i].Dir = dir
		}
	}

	return out
}
