package context

import (
	"fmt"
	"os"
	"slices"
	"sync"
)

// Manager provides CRUD operations for networks, contexts, and keyrings.
// It holds the config in memory and persists changes to disk.
// Manager is safe for concurrent use.
type Manager struct {
	mu   sync.RWMutex
	root string
	cfg  *Config
}

// NewManager creates a manager by loading (or initialising) config from root.
func NewManager(root string) (*Manager, error) {
	cfg, err := LoadConfig(root)
	if err != nil {
		return nil, err
	}

	return &Manager{root: root, cfg: cfg}, nil
}

// Root returns the config root directory.
func (m *Manager) Root() string { return m.root }

// Config returns a shallow copy of the current config.
func (m *Manager) Config() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return *m.cfg
}

// save persists the in-memory config to disk. Caller must hold m.mu.
func (m *Manager) save() error {
	return SaveConfig(m.root, m.cfg)
}

// Reload re-reads the config file from disk, replacing the in-memory state.
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, err := LoadConfig(m.root)
	if err != nil {
		return err
	}

	m.cfg = cfg

	return nil
}

// ---------------------------------------------------------------------------
// Network operations
// ---------------------------------------------------------------------------

// GetNetwork returns a network by name, or nil if not found.
func (m *Manager) GetNetwork(name string) *Network {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getNetwork(name)
}

func (m *Manager) getNetwork(name string) *Network {
	for i := range m.cfg.Networks {
		if m.cfg.Networks[i].Name == name {
			return &m.cfg.Networks[i]
		}
	}

	return nil
}

// ListNetworks returns all configured networks.
func (m *Manager) ListNetworks() []Network {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Network, len(m.cfg.Networks))
	copy(out, m.cfg.Networks)

	return out
}

// CreateNetwork adds a new network definition. If template is non-empty,
// the network fields are populated from NetworkTemplates.
func (m *Manager) CreateNetwork(net Network) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if net.Name == "" {
		return fmt.Errorf("network name is required")
	}

	if m.getNetwork(net.Name) != nil {
		return fmt.Errorf("network %q already exists", net.Name)
	}

	m.cfg.Networks = append(m.cfg.Networks, net)

	return m.save()
}

// CreateNetworkFromTemplate creates a network from a built-in template.
// The name parameter overrides the template's default name.
func (m *Manager) CreateNetworkFromTemplate(name, template string) error {
	tmpl, ok := NetworkTemplates()[template]
	if !ok {
		return fmt.Errorf("unknown network template %q (available: mainnet, testnet, sandbox)", template)
	}

	net := tmpl
	net.Name = name

	return m.CreateNetwork(net)
}

// UpdateNetwork replaces a network definition in-place.
// All contexts referencing this network see the change.
func (m *Manager) UpdateNetwork(name string, apply func(*Network) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	net := m.getNetwork(name)
	if net == nil {
		return fmt.Errorf("network %q not found", name)
	}

	if err := apply(net); err != nil {
		return err
	}

	return m.save()
}

// ForkNetwork creates a copy of an existing network with a new name.
func (m *Manager) ForkNetwork(srcName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	src := m.getNetwork(srcName)
	if src == nil {
		return fmt.Errorf("network %q not found", srcName)
	}

	if m.getNetwork(newName) != nil {
		return fmt.Errorf("network %q already exists", newName)
	}

	fork := *src
	fork.Name = newName
	m.cfg.Networks = append(m.cfg.Networks, fork)

	return m.save()
}

// DeleteNetwork removes a network. It fails if any context references it.
func (m *Manager) DeleteNetwork(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	users := m.contextsUsingNetwork(name)
	if len(users) > 0 {
		return fmt.Errorf("cannot delete network %q: used by contexts %v", name, users)
	}

	m.cfg.Networks = slices.DeleteFunc(m.cfg.Networks, func(n Network) bool {
		return n.Name == name
	})

	return m.save()
}

// NetworkUsers returns the names of contexts referencing a network.
func (m *Manager) NetworkUsers(name string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.contextsUsingNetwork(name)
}

func (m *Manager) contextsUsingNetwork(name string) []string {
	var out []string
	for _, c := range m.cfg.Contexts {
		if c.Network.Name == name {
			out = append(out, c.Name)
		}
	}

	return out
}

// ---------------------------------------------------------------------------
// Context operations
// ---------------------------------------------------------------------------

// GetContext returns a context by name, or nil if not found.
func (m *Manager) GetContext(name string) *Context {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getContext(name)
}

func (m *Manager) getContext(name string) *Context {
	for i := range m.cfg.Contexts {
		if m.cfg.Contexts[i].Name == name {
			return &m.cfg.Contexts[i]
		}
	}

	return nil
}

// ListContexts returns all configured contexts.
func (m *Manager) ListContexts() []Context {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Context, len(m.cfg.Contexts))
	copy(out, m.cfg.Contexts)

	return out
}

// CurrentContext returns the name of the active context.
func (m *Manager) CurrentContext() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.cfg.CurrentContext
}

// CreateContext adds a new context. The referenced network and keyring must exist.
func (m *Manager) CreateContext(ctx Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx.Name == "" {
		return fmt.Errorf("context name is required")
	}

	if m.getContext(ctx.Name) != nil {
		return fmt.Errorf("context %q already exists", ctx.Name)
	}

	// console-api contexts may omit the network entirely: they operate
	// through the Console API alone and gain chain access only when a
	// network is attached later.
	if ctx.Network.Name == "" {
		if ctx.AuthMethod != AuthMethodConsoleAPI {
			return fmt.Errorf("a network is required unless auth-method is %q", AuthMethodConsoleAPI)
		}
	} else if m.getNetwork(ctx.Network.Name) == nil {
		return fmt.Errorf("network %q not found", ctx.Network.Name)
	}

	if ctx.Keyring.Name == "" {
		ctx.Keyring = Keyring{Name: "default"}
	}

	if m.getKeyring(ctx.Keyring.Name) == nil {
		return fmt.Errorf("keyring %q not found", ctx.Keyring.Name)
	}

	if ctx.Gas == "" {
		ctx.Gas = "auto"
	}

	if ctx.ProviderDefaults.AuthType == "" {
		ctx.ProviderDefaults.AuthType = "jwt"
	}

	switch ctx.AuthMethod {
	case "", AuthMethodKeyring, AuthMethodConsoleAPI:
	default:
		return fmt.Errorf("invalid auth-method %q: must be %q or %q", ctx.AuthMethod, AuthMethodKeyring, AuthMethodConsoleAPI)
	}

	m.cfg.Contexts = append(m.cfg.Contexts, ctx)

	// Create data directories for the new context.
	if err := EnsureContextDirs(m.root, ctx.Name); err != nil {
		return err
	}

	return m.save()
}

// UseContext switches the active context.
func (m *Manager) UseContext(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getContext(name) == nil {
		return fmt.Errorf("context %q not found", name)
	}

	m.cfg.CurrentContext = name

	return m.save()
}

// UpdateContext modifies an existing context in-place.
func (m *Manager) UpdateContext(name string, apply func(*Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx := m.getContext(name)
	if ctx == nil {
		return fmt.Errorf("context %q not found", name)
	}

	if err := apply(ctx); err != nil {
		return err
	}

	// Validate references after mutation.
	if m.getNetwork(ctx.Network.Name) == nil {
		return fmt.Errorf("network %q not found", ctx.Network.Name)
	}

	if m.getKeyring(ctx.Keyring.Name) == nil {
		return fmt.Errorf("keyring %q not found", ctx.Keyring.Name)
	}

	return m.save()
}

// DeleteContext removes a context. Cannot delete the current context.
// If keepData is false, the context data directory is also removed.
func (m *Manager) DeleteContext(name string, keepData bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg.CurrentContext == name {
		return fmt.Errorf("cannot delete the current context %q; switch to another context first", name)
	}

	if m.getContext(name) == nil {
		return fmt.Errorf("context %q not found", name)
	}

	m.cfg.Contexts = slices.DeleteFunc(m.cfg.Contexts, func(c Context) bool {
		return c.Name == name
	})

	if !keepData {
		dir := ContextDir(m.root, name)
		_ = os.RemoveAll(dir) // best-effort
	}

	return m.save()
}

// RenameContext renames a context and its data directory.
func (m *Manager) RenameContext(oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getContext(newName) != nil {
		return fmt.Errorf("context %q already exists", newName)
	}

	ctx := m.getContext(oldName)
	if ctx == nil {
		return fmt.Errorf("context %q not found", oldName)
	}

	oldDir := ContextDir(m.root, oldName)
	newDir := ContextDir(m.root, newName)

	// Rename data directory if it exists.
	if _, err := os.Stat(oldDir); err == nil {
		if err := os.Rename(oldDir, newDir); err != nil {
			return fmt.Errorf("rename context directory: %w", err)
		}
	}

	ctx.Name = newName

	if m.cfg.CurrentContext == oldName {
		m.cfg.CurrentContext = newName
	}

	return m.save()
}

// ---------------------------------------------------------------------------
// Keyring operations
// ---------------------------------------------------------------------------

// GetKeyring returns a keyring by name, or nil if not found.
func (m *Manager) GetKeyring(name string) *Keyring {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getKeyring(name)
}

func (m *Manager) getKeyring(name string) *Keyring {
	for i := range m.cfg.Keyrings {
		if m.cfg.Keyrings[i].Name == name {
			return &m.cfg.Keyrings[i]
		}
	}

	return nil
}

// ListKeyrings returns all configured keyrings.
func (m *Manager) ListKeyrings() []Keyring {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Keyring, len(m.cfg.Keyrings))
	copy(out, m.cfg.Keyrings)

	return out
}

// CreateKeyring adds a new keyring definition.
func (m *Manager) CreateKeyring(kr Keyring) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if kr.Name == "" {
		return fmt.Errorf("keyring name is required")
	}

	if m.getKeyring(kr.Name) != nil {
		return fmt.Errorf("keyring %q already exists", kr.Name)
	}

	if kr.Backend == "" {
		kr.Backend = "os"
	}

	m.cfg.Keyrings = append(m.cfg.Keyrings, kr)

	return m.save()
}

// ---------------------------------------------------------------------------
// Resolution (compose a full effective context from config + env + flags)
// ---------------------------------------------------------------------------

// Resolve resolves the current (or named) context into a fully populated Context.
// This dereferences the network and keyring names into their full definitions
// and populates the convenience fields (GasPrices, GasAdjustment, AuthType, Root).
func (m *Manager) Resolve(name string) (*Context, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if name == "" {
		name = m.cfg.CurrentContext
	}

	if name == "" {
		return nil, fmt.Errorf("no context specified and no current-context set in config")
	}

	ctx := m.getContext(name)
	if ctx == nil {
		return nil, fmt.Errorf("context %q not found", name)
	}

	// Network-less console-api contexts resolve with an empty network;
	// capability gating then disables chain-backed commands.
	net := &Network{}
	if ctx.Network.Name != "" {
		net = m.getNetwork(ctx.Network.Name)
		if net == nil {
			return nil, fmt.Errorf("network %q (referenced by context %q) not found", ctx.Network.Name, name)
		}
	}

	kr := m.getKeyring(ctx.Keyring.Name)
	if kr == nil {
		return nil, fmt.Errorf("keyring %q (referenced by context %q) not found", ctx.Keyring.Name, name)
	}

	authMethod := ctx.AuthMethod
	if authMethod == "" {
		authMethod = AuthMethodKeyring
	}

	consoleURL := ctx.ConsoleAPIURL
	if consoleURL == "" {
		consoleURL = DefaultConsoleAPIURL
	}

	return &Context{
		Name:             ctx.Name,
		Network:          *net,
		Keyring:          *kr,
		AuthMethod:       authMethod,
		ConsoleAPIURL:    consoleURL,
		DefaultAccount:   ctx.DefaultAccount,
		Gas:              ctx.Gas,
		Fees:             ctx.Fees,
		ProviderDefaults: ctx.ProviderDefaults,
		GasPrices:        net.GasPrices,
		GasAdjustment:    net.GasAdjustment,
		AuthType:         ctx.ProviderDefaults.AuthType,
		Root:             m.root,
		ConsoleAPIKey:    ResolveConsoleAPIKey(m.root, ctx.Name),
	}, nil
}
