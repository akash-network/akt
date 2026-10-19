package cliutil

import (
	"os"
	"strings"

	flagdefs "pkg.akt.dev/akt/internal/flags"

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

	if flag := cmd.Flags().Lookup(flagdefs.FlagContext); flag != nil && flag.Changed {
		return manager.ActiveContext(flag.Value.String())
	}

	return manager.ActiveContext(os.Getenv("AKT_CONTEXT"))
}

// ConsoleScope returns the Console organization and project this invocation
// acts in (SPEC §7.1.1): a --console-organization or --console-project flag
// given on the command line, then the resolved context, where
// AKT_CONSOLE_ORGANIZATION and AKT_CONSOLE_PROJECT already override the
// stored selection, then those variables alone when no context resolved. An
// explicitly empty flag clears the selection for this invocation.
func ConsoleScope(cmd *cobra.Command, rc *aktctx.Context) (organization, project string) {
	if rc != nil {
		organization, project = rc.ConsoleOrganization, rc.ConsoleProject
	} else {
		organization = strings.TrimSpace(os.Getenv(aktctx.EnvConsoleOrganization))
		project = strings.TrimSpace(os.Getenv(aktctx.EnvConsoleProject))
	}

	if flag := cmd.Flags().Lookup(flagdefs.FlagConsoleOrganization); flag != nil && flag.Changed {
		organization = strings.TrimSpace(flag.Value.String())
	}
	if flag := cmd.Flags().Lookup(flagdefs.FlagConsoleProject); flag != nil && flag.Changed {
		project = strings.TrimSpace(flag.Value.String())
	}

	return organization, project
}
