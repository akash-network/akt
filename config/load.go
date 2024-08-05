package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// DefaultLoadOptions returns the default load options
func DefaultLoadOptions() LoadOptions {
	return LoadOptions{
		Path:   "",
		Global: true,
	}
}

// Load loads the configuration based on the given options
func Load(opts LoadOptions) (Config, error) {
	var configPath string
	var err error

	// Use the provided path if specified
	if opts.Path != "" {
		configPath = filepath.Join(opts.Path, "config.yml")
	} else {
		// Check local path first if no path provided or if no-global is specified
		configPath, err = findConfigPathLocal()
		if err != nil && opts.Global {
			// If no local config found and global is allowed, check global path
			configPath, err = globalConfigPath()
			if err != nil {
				return Config{}, err
			}
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				return Config{}, fmt.Errorf("configuration file not found")
			}
		} else if err != nil {
			return Config{}, err
		}
	}

	configData, err := ioutil.ReadFile(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(configData, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal configuration data: %w", err)
	}

	return cfg, nil
}

func globalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".akt", "config.yml"), nil
}

func findConfigPathLocal() (string, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		configPath := filepath.Join(pwd, ".akt", "config.yml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parent := filepath.Dir(pwd)
		if parent == pwd {
			break
		}
		pwd = parent
	}

	return "", fmt.Errorf("local configuration file not found")
}
