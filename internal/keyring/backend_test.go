package keyring_test

import (
	"errors"
	"strings"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

// TestEffectiveBackendPinsNonAliasBackends: every backend except "os" is
// pinned by the SDK to exactly one store, so it is its own effective backend
// on every host.
func TestEffectiveBackendPinsNonAliasBackends(t *testing.T) {
	root := t.TempDir()

	for _, backend := range []string{
		sdkkeyring.BackendFile,
		sdkkeyring.BackendTest,
		sdkkeyring.BackendKWallet,
		sdkkeyring.BackendPass,
		sdkkeyring.BackendMemory,
	} {
		effective, ok := aktkeyring.EffectiveBackend(root, aktctx.Keyring{Name: "default", Backend: backend})
		if !ok {
			t.Errorf("backend %q reported unavailable", backend)
		}
		if effective != backend {
			t.Errorf("backend %q resolved to %q, want itself", backend, effective)
		}
	}
}

// TestEffectiveBackendResolvesOSToASystemStore: "os" must never report itself.
// It is an alias, and echoing it back is exactly how a headless host ended up
// claiming the system keyring while the SDK used an encrypted file.
func TestEffectiveBackendResolvesOSToASystemStore(t *testing.T) {
	root := t.TempDir()

	effective, ok := aktkeyring.EffectiveBackend(root, aktctx.Keyring{Name: "default", Backend: sdkkeyring.BackendOS})

	if !ok {
		if effective != "" {
			t.Errorf("an unavailable backend must resolve to no store, got %q", effective)
		}
		return
	}

	if effective == sdkkeyring.BackendOS {
		t.Error(`"os" resolved to itself; it must name the concrete platform store`)
	}

	system := aktkeyring.SystemBackendNames()
	for _, name := range system {
		if effective == name {
			return
		}
	}

	t.Errorf("os resolved to %q, which is not one of this platform's system stores %v", effective, system)
}

// TestEmptyBackendMeansOS pins the documented default (SPEC §1.5): an omitted
// backend is "os", not "whatever opens".
func TestEmptyBackendMeansOS(t *testing.T) {
	root := t.TempDir()

	empty, emptyOK := aktkeyring.EffectiveBackend(root, aktctx.Keyring{Name: "default"})
	explicit, explicitOK := aktkeyring.EffectiveBackend(root, aktctx.Keyring{Name: "default", Backend: sdkkeyring.BackendOS})

	if empty != explicit || emptyOK != explicitOK {
		t.Errorf("empty backend resolved to (%q, %v), want the same as os (%q, %v)",
			empty, emptyOK, explicit, explicitOK)
	}
}

// TestUnavailableSystemKeyringFailsWithARemedy: when the host has no system
// credential store, opening an "os" keyring must fail rather than silently
// land on the file backend, and the error must name the way out.
func TestUnavailableSystemKeyringFailsWithARemedy(t *testing.T) {
	if aktkeyring.SystemKeyringAvailable() {
		t.Skip("this host provides a system credential store")
	}

	root := t.TempDir()
	cdc := aktcodec.MakeEncodingConfig().Codec

	mgr := aktkeyring.NewManager(root, []aktctx.Keyring{{Name: "default", Backend: sdkkeyring.BackendOS}}, cdc)
	mgr.SetInput(strings.NewReader(""))

	_, err := mgr.Get("default")
	if err == nil {
		t.Fatal("expected an error opening os on a host with no system credential store")
	}

	var unavailable *aktkeyring.UnavailableBackendError
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected UnavailableBackendError, got %T: %v", err, err)
	}

	for _, want := range []string{"--keyring-backend file", "akt context keyring set"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name the remedy %q, got:\n%s", want, err)
		}
	}
}

// TestUnavailableBackendErrorNamesBothRemedies runs on every host: the
// message is the entire user-facing value of failing instead of substituting,
// so it must always name the per-invocation flag and the persistent command.
func TestUnavailableBackendErrorNamesBothRemedies(t *testing.T) {
	err := &aktkeyring.UnavailableBackendError{
		Keyring:  "default",
		Backend:  sdkkeyring.BackendOS,
		Expected: []string{"secret-service", "kwallet"},
	}

	for _, want := range []string{
		`keyring "default"`,
		`backend "os"`,
		"secret-service or kwallet",
		"--keyring-backend file",
		"akt context keyring set default file",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must contain %q, got:\n%s", want, err)
		}
	}
}

func TestValidateBackendRejectsUnknownValues(t *testing.T) {
	if err := aktkeyring.ValidateBackend(""); err != nil {
		t.Errorf("an unset backend must be accepted as inherit: %v", err)
	}

	for _, backend := range aktkeyring.Backends() {
		if err := aktkeyring.ValidateBackend(backend); err != nil {
			t.Errorf("advertised backend %q rejected: %v", backend, err)
		}
	}

	if err := aktkeyring.ValidateBackend("keychain"); err == nil {
		t.Error("expected an error for a store name that is not a configurable backend")
	}
}

func TestApplyOverridesLeavesUnsetValuesAlone(t *testing.T) {
	configured := []aktctx.Keyring{
		{Name: "default", Backend: sdkkeyring.BackendOS, Dir: "/configured"},
		{Name: "other", Backend: sdkkeyring.BackendTest},
	}

	unchanged := aktkeyring.ApplyOverrides(configured, "", "")
	if unchanged[0].Backend != sdkkeyring.BackendOS || unchanged[0].Dir != "/configured" {
		t.Errorf("unset overrides must not touch the stored config, got %+v", unchanged[0])
	}

	overridden := aktkeyring.ApplyOverrides(configured, sdkkeyring.BackendFile, "")
	for _, kr := range overridden {
		if kr.Backend != sdkkeyring.BackendFile {
			t.Errorf("keyring %q kept backend %q, want the override", kr.Name, kr.Backend)
		}
	}
	if overridden[0].Dir != "/configured" {
		t.Errorf("an unset --keyring-dir must not clear the configured dir, got %q", overridden[0].Dir)
	}

	// The input slice must survive: the manager is built from the config the
	// context manager still owns.
	if configured[0].Backend != sdkkeyring.BackendOS {
		t.Error("ApplyOverrides mutated its input")
	}
}
