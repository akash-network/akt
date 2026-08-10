package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/capability"
	aktctx "pkg.akt.dev/akt/internal/context"
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

// invocationCapabilities augments a context-derived feature set with the
// capabilities this invocation supplies explicitly. Gating describes what the
// *configuration* can do, so a per-invocation override — an explicit RPC
// endpoint or a Console key passed on the command line — must never be
// rejected by it (SPEC §2.10).
//
// argv is the raw command line because several clean-copied SDK groups
// disable flag parsing; posArgs are the command's parsed positional
// arguments, used for commands that take an endpoint positionally.
func invocationCapabilities(set capability.Set, authMethod string, cmd *cobra.Command, argv, posArgs []string) capability.Set {
	grantChain := func() {
		set.ChainQuery = true
		set.Provider = true
		// An endpoint override supplies a connection, not a signer. Empty is
		// the backwards-compatible default for resolved keyring contexts.
		if authMethod != aktctx.AuthMethodConsoleAPI {
			set.ChainTx = true
		}
	}

	for _, a := range argv {
		if a == "--" {
			break
		}

		switch {
		case a == "--node" || strings.HasPrefix(a, "--node="):
			grantChain()
		case a == "--console-api-key" || strings.HasPrefix(a, "--console-api-key="):
			set.Console = true
		}
	}

	// The environment carries the same Console credential the client
	// resolves at point of use.
	if os.Getenv(aktctx.EnvConsoleAPIKey) != "" {
		set.Console = true
	}

	// `akt monitor [rpc-endpoint]` connects to an endpoint given
	// positionally and works with no context at all.
	if cmd != nil && strings.HasPrefix(cmd.CommandPath(), "akt monitor") && len(posArgs) > 0 {
		grantChain()
	}

	return set
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
