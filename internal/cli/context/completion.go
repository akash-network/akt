package context

import (
	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// completeContextNames returns a ValidArgsFunction that suggests context
// names from the current manager. Used by commands that take a context
// name as a positional argument (use, edit, delete, rename).
func completeContextNames(mgr func() *aktctx.Manager) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		m := mgr()
		if m == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		ctxs := m.ListContexts()
		names := make([]string, 0, len(ctxs))
		for _, c := range ctxs {
			names = append(names, c.Name)
		}

		return names, cobra.ShellCompDirectiveNoFileComp
	}
}


