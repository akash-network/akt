package context

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// DefaultHome is the fallback home directory used when no override or env var
// is set. It matches the last-resort path in ConfigHome.
var DefaultHome = filepath.Join(os.Getenv("HOME"), ".config", "akt")

// ConfigHome resolves the akt home directory.
//
// Resolution order:
//  1. override parameter (from --home flag)
//  2. AKT_HOME env var
//  3. $XDG_CONFIG_HOME/akt
//  4. ~/.config/akt
func ConfigHome(override string) (string, error) {
	if override != "" {
		return override, nil
	}

	if v := os.Getenv("AKT_HOME"); v != "" {
		return v, nil
	}

	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "akt"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".config", "akt"), nil
}

// ConfigPath returns the path to config.yaml within the given config root.
func ConfigPath(root string) string {
	return filepath.Join(root, "config.yaml")
}

// ContextDir returns the data directory for a named context.
func ContextDir(root, name string) string {
	return filepath.Join(root, "contexts", name)
}

// StoreDir returns the store directory for a named context.
func StoreDir(root, name string) string {
	return filepath.Join(root, "contexts", name, "store")
}

// ActionLogPath returns the action log file path for a named context.
func ActionLogPath(root, name string) string {
	return filepath.Join(root, "contexts", name, "actions.log")
}

// WorkflowsDir returns the global workflows directory within the home.
func WorkflowsDir(root string) string {
	return filepath.Join(root, "workflows")
}

// ContextWorkflowsDir returns the per-context workflows directory.
func ContextWorkflowsDir(root, name string) string {
	return filepath.Join(root, "contexts", name, "workflows")
}

// KeyringDir returns the directory for a named keyring.
func KeyringDir(root string, kr Keyring) string {
	if kr.Dir != "" {
		return kr.Dir
	}

	return filepath.Join(root, "keyrings", kr.Name)
}

// NewViper creates a Viper instance configured for the given root directory.
// It reads config.yaml, binds environment variables with the AKT_ prefix,
// and sets defaults from DefaultConfig.
func NewViper(root string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(root)
	v.SetEnvPrefix("AKT")
	v.AutomaticEnv()

	// Set defaults from the default config.
	v.SetDefault("version", ConfigVersion)
	v.SetDefault("current-context", "")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// If the file doesn't exist that's fine (fresh setup).
			// Any other error (parse error, permission error) is real.
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read config: %w", err)
			}
		}
	}

	return v, nil
}

// LoadConfig reads and parses the config.yaml file from the given root
// using Viper. If the file does not exist, a default config is returned.
func LoadConfig(root string) (*Config, error) {
	v, err := NewViper(root)
	if err != nil {
		return nil, err
	}

	return unmarshalConfig(v)
}

// unmarshalConfig converts Viper state into a typed Config struct.
// Viper's built-in Unmarshal doesn't handle our custom YAML marshal/unmarshal
// for Context (where Network and Keyring serialize as name strings), so we
// read the raw config file and use our own YAML types.
func unmarshalConfig(v *viper.Viper) (*Config, error) {
	cfgFile := v.ConfigFileUsed()
	if cfgFile == "" {
		cfg := DefaultConfig()
		return &cfg, nil
	}

	data, err := os.ReadFile(cfgFile)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			return &cfg, nil
		}

		return nil, fmt.Errorf("read config %s: %w", cfgFile, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", cfgFile, err)
	}

	if cfg.Version == 0 {
		cfg.Version = ConfigVersion
	}

	return &cfg, nil
}

// SaveConfig writes the config to config.yaml in the given root.
// It creates the root directory and parent directories as needed.
// Uses YAML encoder with 2-space indentation.
func SaveConfig(root string, cfg *Config) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create config directory %s: %w", root, err)
	}

	path := ConfigPath(root)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config %s: %w", path, err)
	}
	defer f.Close()

	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := enc.Close(); err != nil {
		return err
	}

	payload := buf.Bytes()
	if len(payload) == 0 {
		return nil
	}

	if !bytes.HasPrefix(payload, []byte("---")) {
		if _, err := f.Write([]byte("---\n")); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	if _, err := f.Write(payload); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// EnsureContextDirs creates the store and action log directories for a context.
func EnsureContextDirs(root, name string) error {
	storeDir := StoreDir(root, name)
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return fmt.Errorf("create store directory %s: %w", storeDir, err)
	}

	return nil
}
