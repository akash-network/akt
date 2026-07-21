package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Authentication methods a context can use (SPEC §1.4, §7).
const (
	AuthMethodKeyring    = "keyring"
	AuthMethodConsoleAPI = "console-api"
)

// DefaultConsoleAPIURL is the default Console API base URL (SPEC §7.2).
const DefaultConsoleAPIURL = "https://console-api.akash.network"

// EnvConsoleAPIKey is the environment variable holding the Console API key.
// It overrides the per-context stored credential (SPEC §7.1).
const EnvConsoleAPIKey = "AKT_CONSOLE_API_KEY"

// consoleAPIKeyFile is the per-context credential file name.
const consoleAPIKeyFile = "console-api-key"

// ConsoleAPIKeyPath returns the path of a context's stored Console API key.
// The credential lives inside the context's data directory so renames move
// it and deletes remove it (SPEC §7.1).
func ConsoleAPIKeyPath(root, ctxName string) string {
	return filepath.Join(ContextDir(root, ctxName), consoleAPIKeyFile)
}

// SetConsoleAPIKey stores (or, with an empty key, removes) a context's
// Console API key. The file is written with 0600 permissions and is never
// part of config.yaml.
func SetConsoleAPIKey(root, ctxName, key string) error {
	path := ConsoleAPIKeyPath(root, ctxName)

	if key == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove console api key: %w", err)
		}

		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create context directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return fmt.Errorf("write console api key: %w", err)
	}

	// WriteFile does not change the mode of an existing file.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict console api key permissions: %w", err)
	}

	return nil
}

// StoredConsoleAPIKey reads a context's stored Console API key. A missing
// credential file returns an empty key with no error.
func StoredConsoleAPIKey(root, ctxName string) (string, error) {
	data, err := os.ReadFile(ConsoleAPIKeyPath(root, ctxName))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}

		return "", fmt.Errorf("read console api key: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// ResolveConsoleAPIKey resolves the effective Console API key for a context
// per SPEC §7.1: environment variable first, then the stored per-context
// credential. The --console-api-key flag override is applied by the CLI
// layer on top of this.
func ResolveConsoleAPIKey(root, ctxName string) string {
	if key := os.Getenv(EnvConsoleAPIKey); key != "" {
		return key
	}

	key, _ := StoredConsoleAPIKey(root, ctxName)

	return key
}
