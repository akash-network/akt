package main

import (
	"github.com/spf13/cobra"
)

func rootCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use: "akt",
	}

	cmd.AddCommand(initCmd())
	cmd.AddCommand(configCmd())
	cmd.AddCommand(accountCmd())

	return cmd
}
