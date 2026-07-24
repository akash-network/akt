package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/capability"
)

// applyCapabilityGating walks the command tree and adjusts the presentation
// of commands whose capability requirement (capability.AnnotationKey
// annotation) the active context cannot satisfy:
//
//   - ModeDim: the command stays listed but its short help is prefixed with
//     "[unavailable]" so users see it exists and why it is off.
//   - ModeHide: the command is removed from help listings.
//
// Execution is enforced separately by requirementError so both modes (and
// direct invocation of hidden commands) fail fast with the explanation.
// Mutations are idempotent per process: cobra commands are rebuilt each run.
func applyCapabilityGating(root *cobra.Command, set capability.Set, mode capability.Mode) {
	if mode == capability.ModeOff {
		return
	}

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if req := cmd.Annotations[capability.AnnotationKey]; req != "" && !set.Satisfies(req) {
			switch mode {
			case capability.ModeHide:
				cmd.Hidden = true
			default: // ModeDim
				if !strings.HasPrefix(cmd.Short, "[unavailable] ") {
					cmd.Short = "[unavailable] " + cmd.Short
				}
			}
		}

		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}

	walk(root)
}

// requirementError returns the fail-fast error for cmd when it (or an
// ancestor) carries a capability requirement the set cannot satisfy, or nil
// when the command is runnable with the current configuration.
func requirementError(cmd *cobra.Command, set capability.Set) error {
	for c := cmd; c != nil; c = c.Parent() {
		req := c.Annotations[capability.AnnotationKey]
		if req == "" || set.Satisfies(req) {
			continue
		}

		return fmt.Errorf("%s is unavailable with the current context configuration: %s",
			c.CommandPath(), set.Explain(req))
	}

	return nil
}
