package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/akash-network/akt/config" // Replace with the actual import path
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Initialize configuration",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runInitCmd,
}

func init() {
	initCmd.Flags().Bool("global", false, "Initialize the global configuration at ~/.akt")
	initCmd.Flags().String("confdir", "", "Directory to create the .akt directory and copy the config file")
	initCmd.Flags().StringP("output", "o", "json", "Output format (json or yaml)")
}

func runInitCmd(cmd *cobra.Command, args []string) error {
	fmt.Println("Starting init command")

	globalFlag, err := cmd.Flags().GetBool("global")
	if err != nil {
		return err
	}
	initConfDir, err := cmd.Flags().GetString("confdir")
	if err != nil {
		return err
	}
	initOutput, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	// Determine the source configuration file path
	var sourceConfigFile string
	pwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	if len(args) > 0 {
		sourceConfigFile = filepath.Join(args[0], "config.yml")
	} else {
		sourceConfigFile = filepath.Join(pwd, "config.yml")
	}

	if _, err := os.Stat(sourceConfigFile); os.IsNotExist(err) {
		return fmt.Errorf("source configuration file not found: %s", sourceConfigFile)
	}

	// Read the source configuration file
	configData, err := ioutil.ReadFile(sourceConfigFile)
	if err != nil {
		return fmt.Errorf("failed to read source configuration file: %w", err)
	}

	// Unmarshal the configuration data into Go structs
	var configStruct config.Config
	if err := yaml.Unmarshal(configData, &configStruct); err != nil {
		return fmt.Errorf("failed to unmarshal configuration data: %w", err)
	}

	// Determine target configuration path
	var configPath string
	if globalFlag {
		configPath, err = os.UserHomeDir()
		if err != nil {
			return err
		}
		configPath = filepath.Join(configPath, ".akt")
		fmt.Printf("Global configuration path: %s\n", configPath)
	} else if initConfDir != "" {
		configPath = filepath.Join(initConfDir, ".akt")
		fmt.Printf("Configuration path from confdir: %s\n", configPath)
	} else {
		configPath = filepath.Join(pwd, ".akt")
		fmt.Printf("Local configuration path: %s\n", configPath)
	}

	// Create target configuration directory
	if err := os.MkdirAll(configPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create configuration directory: %w", err)
	}
	fmt.Printf("Configuration directory created at %s\n", configPath)

	// Marshal the Go structs back to YAML
	marshaledConfigData, err := yaml.Marshal(&configStruct)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration data: %w", err)
	}

	// Write the marshaled configuration data to the target file
	destConfigFile := filepath.Join(configPath, "config.yml")
	if err := ioutil.WriteFile(destConfigFile, marshaledConfigData, 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}
	fmt.Printf("Configuration file copied to %s\n", destConfigFile)

	// Output the configuration in the specified format
	var outData []byte
	switch initOutput {
	case "yaml":
		outData, err = yaml.Marshal(&configStruct)
	case "json":
		outData, err = json.MarshalIndent(&configStruct, "", "  ")
	default:
		return fmt.Errorf("invalid output format: %s", initOutput)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal configuration for output: %w", err)
	}

	fmt.Printf("Configuration:\n%s\n", outData)
	fmt.Println("Init command completed successfully")
	return nil
}
