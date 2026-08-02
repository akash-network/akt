package cliutil

import (
	"os"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// SelectedContextName returns the one context targeted by this invocation.
// An explicitly supplied global flag wins over AKT_CONTEXT; the manager then
// applies its current-context and sole-context fallbacks.
func SelectedContextName(cmd *cobra.Command, manager *aktctx.Manager) string {
	if manager == nil {
		return ""
	}

	if flag := cmd.Flags().Lookup("context"); flag != nil && flag.Changed {
		return manager.ActiveContext(flag.Value.String())
	}

	return manager.ActiveContext(os.Getenv("AKT_CONTEXT"))
}
