package main

import (
	"encoding/json"
	"fmt"

	"github.com/akash-network/akt/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Display current configuration",
	RunE:  runConfigCmd,
}

var (
	confDir  string
	noGlobal bool
	output   string
)

func init() {
	configCmd.Flags().StringVar(&confDir, "confdir", "", "Use an explicit directory instead of auto-discovered path")
	configCmd.Flags().BoolVar(&noGlobal, "no-global", false, "Do not include global ~/.akt configuration")
	configCmd.Flags().StringVarP(&output, "output", "o", "json", "Output format (json or yaml)")
}

func runConfigCmd(cmd *cobra.Command, args []string) error {
	opts := config.DefaultLoadOptions()
	if confDir != "" {
		opts.Path = confDir
	}
	if noGlobal {
		opts.Global = false
	}

	cfg, err := config.Load(opts)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	var outData []byte
	switch output {
	case "yaml":
		outData, err = yaml.Marshal(&cfg)
	case "json":
		outData, err = json.MarshalIndent(&cfg, "", "  ")
	default:
		return fmt.Errorf("invalid output format: %s", output)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	fmt.Printf("%s\n", outData)
	return nil
}
