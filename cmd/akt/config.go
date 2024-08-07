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

func init() {
	configCmd.Flags().String("confdir", "", "Use an explicit directory instead of auto-discovered path")
	configCmd.Flags().Bool("no-global", false, "Do not include global ~/.akt configuration")
	configCmd.Flags().StringP("output", "o", "json", "Output format (json or yaml)")
}

func runConfigCmd(cmd *cobra.Command, args []string) error {
	confDir, err := cmd.Flags().GetString("confdir")
	if err != nil {
		return err
	}
	noGlobal, err := cmd.Flags().GetBool("no-global")
	if err != nil {
		return err
	}
	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

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
