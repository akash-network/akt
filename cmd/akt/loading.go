package main

import (
	"github.com/akash-network/akt/logo"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"os"
	"time"
)

func loadingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "show-loading",
		Run: func(_ *cobra.Command, _ []string) {

			logo.Write(os.Stdout)

			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Prefix = "    "
			s.Suffix = "  NEW CLI LOADING..."
			s.Start()
			time.Sleep(40 * time.Second)
			s.Stop()
		},
	}
	return cmd
}
