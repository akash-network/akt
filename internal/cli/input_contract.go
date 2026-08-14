package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	flagdefs "pkg.akt.dev/akt/internal/flags"

	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
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
			flag := flags.Lookup(flagdefs.FlagOutput)
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

// enforceTransactionModeValidation constrains transaction flags adopted from
// dependency-owned command trees. Locally registered flags already use the
// same enums, but the assembled surface must reject bad values uniformly.
func enforceTransactionModeValidation(cmd *cobra.Command) {
	type constraint struct {
		def     string
		usage   string
		allowed []string
	}
	constraints := map[string]constraint{
		cflags.FlagSignMode: {
			def:   cflags.SignModeDirect,
			usage: "Choose sign mode (direct|amino-json|direct-aux|eip-191), this is an advanced feature",
			allowed: []string{
				cflags.SignModeDirect,
				cflags.SignModeLegacyAminoJSON,
				cflags.SignModeDirectAux,
				cflags.SignModeEIP191,
			},
		},
		cflags.FlagBroadcastMode: {
			def:   cflags.BroadcastSync,
			usage: "Transaction broadcasting mode (sync|async|block)",
			allowed: []string{
				cflags.BroadcastSync,
				cflags.BroadcastAsync,
				cflags.BroadcastBlock,
			},
		},
	}

	seen := make(map[*pflag.Flag]struct{})
	var walk func(*cobra.Command)
	walk = func(current *cobra.Command) {
		for _, flags := range []*pflag.FlagSet{current.LocalFlags(), current.PersistentFlags()} {
			for name, constraint := range constraints {
				flag := flags.Lookup(name)
				if flag == nil {
					continue
				}
				if _, ok := seen[flag]; ok {
					continue
				}
				seen[flag] = struct{}{}
				output.ConstrainFlag(flag, constraint.def, constraint.allowed...)
				flag.Usage = constraint.usage
			}
		}

		for _, child := range current.Commands() {
			walk(child)
		}
	}

	walk(cmd)
}
