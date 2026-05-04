package network

import (
	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// completeNetworkNames returns a ValidArgsFunction that suggests network
// names from the current manager.
func completeNetworkNames(mgr func() *aktctx.Manager) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		m := mgr()
		if m == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		nets := m.ListNetworks()
		names := make([]string, 0, len(nets))
		for _, n := range nets {
			names = append(names, n.Name)
		}

		return names, cobra.ShellCompDirectiveNoFileComp
	}
}
