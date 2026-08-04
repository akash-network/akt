// Package keyring provides multi-keyring management for akt.
//
// It wraps the Cosmos SDK's crypto/keyring package, adding support for
// multiple named keyrings that can be shared across contexts.
//
// Each named keyring maps to a Cosmos SDK keyring.Keyring instance backed
// by a specific backend (os, file, test, kwallet, pass) and directory.
package keyring

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/cosmos/cosmos-sdk/codec"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// appName is the service name handed to the SDK keyring. It is defined in
// internal/context so the first-run wizard can name it when explaining why
// legacy akash keyring entries are invisible to akt (SPEC §1.12).
const appName = aktctx.KeyringServiceName

// Manager manages multiple named keyrings. It lazily opens keyrings on
// first access and caches them for the lifetime of the process.
// Manager is safe for concurrent use.
type Manager struct {
	mu      sync.Mutex
	root    string
	cdc     codec.Codec
	input   io.Reader
	cache   map[string]sdkkeyring.Keyring
	configs map[string]aktctx.Keyring
}

// NewManager creates a keyring manager from the config root and keyring
// definitions. The codec is required by the Cosmos SDK keyring for
// protobuf key serialization.
func NewManager(root string, keyrings []aktctx.Keyring, cdc codec.Codec) *Manager {
	configs := make(map[string]aktctx.Keyring, len(keyrings))
	for _, kr := range keyrings {
		configs[kr.Name] = kr
	}

	return &Manager{
		root:    root,
		cdc:     cdc,
		input:   os.Stdin,
		cache:   make(map[string]sdkkeyring.Keyring),
		configs: configs,
	}
}

// SetInput overrides the reader used for password prompts. Primarily for testing.
func (m *Manager) SetInput(r io.Reader) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.input = r
}

// Get returns the Cosmos SDK keyring for the given named keyring.
// The keyring is lazily opened on first access and cached.
func (m *Manager) Get(name string) (sdkkeyring.Keyring, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if kr, ok := m.cache[name]; ok {
		return kr, nil
	}

	cfg, ok := m.configs[name]
	if !ok {
		return nil, fmt.Errorf("keyring %q not found in config", name)
	}

	kr, err := m.open(cfg)
	if err != nil {
		// An unavailable backend already names the keyring and its remedy;
		// prefixing it again buries the remedy behind a second "open keyring".
		var unavailable *UnavailableBackendError
		if errors.As(err, &unavailable) {
			return nil, err
		}

		return nil, fmt.Errorf("open keyring %q: %w", name, err)
	}

	m.cache[name] = kr

	return kr, nil
}

// Reload updates the keyring configurations (e.g., after config live-reload).
// Cached keyrings whose config changed are evicted.
func (m *Manager) Reload(keyrings []aktctx.Keyring) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newConfigs := make(map[string]aktctx.Keyring, len(keyrings))
	for _, kr := range keyrings {
		newConfigs[kr.Name] = kr
	}

	// Evict cached keyrings whose config has changed.
	for name, cached := range m.configs {
		newCfg, exists := newConfigs[name]
		if !exists || newCfg.Backend != cached.Backend || newCfg.Dir != cached.Dir {
			delete(m.cache, name)
		}
	}

	m.configs = newConfigs
}

// Names returns the names of all configured keyrings.
func (m *Manager) Names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}

	return names
}

// open creates a Cosmos SDK keyring from our keyring config.
// Caller must hold m.mu.
func (m *Manager) open(cfg aktctx.Keyring) (sdkkeyring.Keyring, error) {
	backend := cfg.Backend
	if backend == "" {
		backend = sdkkeyring.BackendOS
	}

	if err := ValidateBackend(backend); err != nil {
		return nil, err
	}

	dir := aktctx.KeyringDir(m.root, cfg)

	// "os" is an alias for the platform credential store, and the SDK opens it
	// without pinning the backend list -- so on a host without one it silently
	// lands on pass or file while the config keeps claiming "os". Refuse
	// instead, and name the remedy (SPEC §1.5).
	if backend == sdkkeyring.BackendOS {
		if _, ok := EffectiveBackend(m.root, cfg); !ok {
			name := cfg.Name
			if name == "" {
				name = "default"
			}

			return nil, &UnavailableBackendError{
				Keyring:  name,
				Backend:  backend,
				Expected: SystemBackendNames(),
			}
		}
	}

	// Ensure keyring directory exists for file-based backends.
	if backend == sdkkeyring.BackendFile || backend == sdkkeyring.BackendTest {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create keyring dir %s: %w", dir, err)
		}
	}

	return sdkkeyring.New(appName, backend, dir, m.input, m.cdc)
}

// EffectiveBackend reports the concrete credential store that serves the named
// keyring on this host (SPEC §1.5), and whether the host can provide it.
func (m *Manager) EffectiveBackend(name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.configs[name]
	if !ok {
		return "", false
	}

	return EffectiveBackend(m.root, cfg)
}

// NewInMemory creates an in-memory keyring for dry-run / testing use.
// It is not associated with any named keyring in the config.
func NewInMemory(cdc codec.Codec) sdkkeyring.Keyring {
	return sdkkeyring.NewInMemory(cdc)
}
