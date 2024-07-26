package main

import (
	"github.com/spf13/cobra"
)

func rootCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use: "akt",
	}

	cmd.AddCommand(loadingCmd())

	return cmd
}
