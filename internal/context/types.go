package context

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const (
	ProviderAuthJWT  = "jwt"
	ProviderAuthMTLS = "mtls"
)

// ResolveProviderAuthType applies the context default and validates the
// provider gateway authentication enum at the configuration boundary.
func ResolveProviderAuthType(value string) (string, error) {
	if value == "" {
		return ProviderAuthJWT, nil
	}
	switch value {
	case ProviderAuthJWT, ProviderAuthMTLS:
		return value, nil
	default:
		return "", fmt.Errorf("invalid provider auth type %q: must be %q or %q", value, ProviderAuthJWT, ProviderAuthMTLS)
	}
}

// Endpoints defines the network transport endpoints.
type Endpoints struct {
	RPC  []string `yaml:"rpc"            json:"rpc"`
	API  []string `yaml:"api,omitempty"  json:"api,omitempty"`
	GRPC []string `yaml:"grpc,omitempty" json:"grpc,omitempty"`
}

// Network defines chain connectivity settings.
// Networks are shared resources -- multiple contexts can reference the same network.
type Network struct {
	Name          string    `yaml:"name"                      json:"name"`
	ChainID       string    `yaml:"chain-id"                  json:"chain_id"`
	Endpoints     Endpoints `yaml:"endpoints"                 json:"endpoints"`
	GasPrices     string    `yaml:"gas-prices,omitempty"      json:"gas_prices,omitempty"`
	GasAdjustment string    `yaml:"gas-adjustment,omitempty"  json:"gas_adjustment,omitempty"`
	Faucet        string    `yaml:"faucet,omitempty"          json:"faucet,omitempty"`
}

// Keyring defines wallet storage configuration.
// Keyrings are shared -- adding a key makes it available to all contexts using the keyring.
type Keyring struct {
	Name    string `yaml:"name"              json:"name"`
	Backend string `yaml:"backend,omitempty" json:"backend,omitempty"`
	Dir     string `yaml:"dir,omitempty"     json:"dir,omitempty"`
}

// ProviderDefaults holds provider-specific defaults for a context.
type ProviderDefaults struct {
	AuthType string `yaml:"auth-type,omitempty" json:"auth_type,omitempty"`
}

// Context defines a named environment that composes a network, keyring,
// and context-specific settings (store + action log are implicitly per-context).
//
// In config (YAML), Network and Keyring are serialized as name strings that
// reference top-level network and keyring definitions. After resolution via
// Manager.Resolve(), the full Network and Keyring objects are populated with
// all their fields.
//
// TrackedAccounts names the accounts store reconciliation covers (SPEC §6.7):
// empty means the default account alone, ["*"] every account in the context's
// keyring.
type Context struct {
	Name             string           `yaml:"-"                           json:"name"`
	Network          Network          `yaml:"-"                           json:"network"`
	Keyring          Keyring          `yaml:"-"                           json:"keyring"`
	AuthMethod       string           `yaml:"auth-method,omitempty"       json:"auth_method,omitempty"`
	ConsoleAPIURL    string           `yaml:"console-api-url,omitempty"   json:"console_api_url,omitempty"`
	DefaultAccount   string           `yaml:"default-account,omitempty"   json:"default_account,omitempty"`
	TrackedAccounts  []string         `yaml:"tracked-accounts,omitempty"  json:"tracked_accounts,omitempty"`
	Gas              string           `yaml:"gas,omitempty"               json:"gas,omitempty"`
	Fees             string           `yaml:"fees,omitempty"              json:"fees,omitempty"`
	ProviderDefaults ProviderDefaults `yaml:"provider-defaults,omitempty" json:"provider_defaults,omitempty"`

	// Resolved fields -- populated by Manager.Resolve(), not serialized.
	// These are convenience accessors that flatten nested values.
	GasPrices     string `yaml:"-" json:"-"`
	GasAdjustment string `yaml:"-" json:"-"`
	AuthType      string `yaml:"-" json:"-"`
	Root          string `yaml:"-" json:"-"` // config root directory
	// ConsoleAPIKey is the resolved Console API key (env var overrides the
	// per-context credential file, SPEC §7.1). Never serialized.
	ConsoleAPIKey string `yaml:"-" json:"-"`
}

// contextYAML is the on-disk representation where Network and Keyring are name strings.
type contextYAML struct {
	Name             string           `yaml:"name"`
	Network          string           `yaml:"network"`
	Keyring          string           `yaml:"keyring,omitempty"`
	AuthMethod       string           `yaml:"auth-method,omitempty"`
	ConsoleAPIURL    string           `yaml:"console-api-url,omitempty"`
	DefaultAccount   string           `yaml:"default-account,omitempty"`
	TrackedAccounts  []string         `yaml:"tracked-accounts,omitempty"`
	Gas              string           `yaml:"gas,omitempty"`
	Fees             string           `yaml:"fees,omitempty"`
	ProviderDefaults ProviderDefaults `yaml:"provider-defaults,omitempty"`
}

// MarshalYAML serializes a Context to YAML, writing Network and Keyring as name strings.
func (c Context) MarshalYAML() (interface{}, error) {
	return contextYAML{
		Name:             c.Name,
		Network:          c.Network.Name,
		Keyring:          c.Keyring.Name,
		AuthMethod:       c.AuthMethod,
		ConsoleAPIURL:    c.ConsoleAPIURL,
		DefaultAccount:   c.DefaultAccount,
		TrackedAccounts:  c.TrackedAccounts,
		Gas:              c.Gas,
		Fees:             c.Fees,
		ProviderDefaults: c.ProviderDefaults,
	}, nil
}

// UnmarshalYAML deserializes a Context from YAML, reading Network and Keyring as name strings.
// The full Network and Keyring objects are populated later by Manager.Resolve() or
// Manager load logic.
func (c *Context) UnmarshalYAML(value *yaml.Node) error {
	var raw contextYAML
	if err := value.Decode(&raw); err != nil {
		return err
	}

	c.Name = raw.Name
	c.Network = Network{Name: raw.Network}
	c.Keyring = Keyring{Name: raw.Keyring}
	c.AuthMethod = raw.AuthMethod
	c.ConsoleAPIURL = raw.ConsoleAPIURL
	c.DefaultAccount = raw.DefaultAccount
	c.TrackedAccounts = raw.TrackedAccounts
	c.Gas = raw.Gas
	c.Fees = raw.Fees
	c.ProviderDefaults = raw.ProviderDefaults

	return nil
}

// TUISettings holds TUI-specific configuration.
type TUISettings struct {
	Theme             string            `yaml:"theme,omitempty"              json:"theme,omitempty"`
	Keybindings       string            `yaml:"keybindings,omitempty"        json:"keybindings,omitempty"`
	CustomKeybindings map[string]string `yaml:"custom-keybindings,omitempty" json:"custom_keybindings,omitempty"`
	RefreshInterval   string            `yaml:"refresh-interval,omitempty"   json:"refresh_interval,omitempty"`
}

// PluginSettings holds plugin-specific configuration.
type PluginSettings struct {
	Paths    []string `yaml:"paths,omitempty"    json:"paths,omitempty"`
	Disabled []string `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// Defaults holds global default values.
type Defaults struct {
	Output        string `yaml:"output,omitempty"         json:"output,omitempty"`
	BroadcastMode string `yaml:"broadcast-mode,omitempty" json:"broadcast_mode,omitempty"`
	// CommandGating selects how commands the context configuration cannot
	// execute are presented: dim (default), hide, or off. Both dim and
	// hide are supported while UX feedback is collected.
	CommandGating string `yaml:"command-gating,omitempty" json:"command_gating,omitempty"`
}

// Config is the top-level configuration structure persisted in config.yaml.
type Config struct {
	Version        int            `yaml:"version"                    json:"version"`
	CurrentContext string         `yaml:"current-context"            json:"current_context"`
	Networks       []Network      `yaml:"networks"                   json:"networks"`
	Keyrings       []Keyring      `yaml:"keyrings"                   json:"keyrings"`
	Contexts       []Context      `yaml:"contexts"                   json:"contexts"`
	TUI            TUISettings    `yaml:"tui,omitempty"              json:"tui,omitempty"`
	Plugins        PluginSettings `yaml:"plugins,omitempty"          json:"plugins,omitempty"`
	Defaults       Defaults       `yaml:"defaults,omitempty"         json:"defaults,omitempty"`
}

// ConfigVersion is the current schema version.
const ConfigVersion = 1
