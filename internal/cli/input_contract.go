package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	"pkg.akt.dev/akt/internal/output"
)

// enforceGroupInputValidation makes every pure command group reject a token
// that does not name one of its children. Cobra otherwise treats a group with
// no Run/RunE as non-runnable and turns an unknown positional into successful
// help output.
func enforceGroupInputValidation(cmd *cobra.Command) {
	if cmd.HasSubCommands() && !cmd.Runnable() {
		cmd.RunE = chaincli.ValidateCmd
	}

	for _, child := range cmd.Commands() {
		enforceGroupInputValidation(child)
	}
}

// enforceOutputValidation constrains output flags copied from upstream
// command trees. Locally defined flags already carry their exact enum and are
// left untouched, including the workflow-only jsonl extension.
func enforceOutputValidation(cmd *cobra.Command) {
	seen := make(map[*pflag.Flag]struct{})

	var walk func(*cobra.Command)
	walk = func(current *cobra.Command) {
		for _, flags := range []*pflag.FlagSet{current.LocalFlags(), current.PersistentFlags()} {
			flag := flags.Lookup("output")
			if flag == nil {
				continue
			}
			if _, ok := seen[flag]; ok {
				continue
			}
			seen[flag] = struct{}{}
			output.ConstrainFlag(flag, "pretty", "pretty", "json", "yaml")
		}

		for _, child := range current.Commands() {
			walk(child)
		}
	}

	walk(cmd)
}
