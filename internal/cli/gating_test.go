package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/capability"
)

func gatedTree() (*cobra.Command, *cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "akt"}

	tx := &cobra.Command{
		Use:         "tx",
		Short:       "Transactions subcommands",
		Annotations: map[string]string{capability.AnnotationKey: string(capability.ChainTx)},
	}
	txLeaf := &cobra.Command{Use: "send", Short: "send", RunE: func(*cobra.Command, []string) error { return nil }}
	tx.AddCommand(txLeaf)

	console := &cobra.Command{
		Use:         "console",
		Short:       "Console commands",
		Annotations: map[string]string{capability.AnnotationKey: string(capability.Console)},
	}

	root.AddCommand(tx, console)

	return root, tx, console
}

func TestGatingDimMode(t *testing.T) {
	root, tx, console := gatedTree()
	consoleOnly := capability.Set{Console: true}

	applyCapabilityGating(root, consoleOnly, capability.ModeDim)

	if !strings.HasPrefix(tx.Short, "[unavailable] ") {
		t.Errorf("tx should be dimmed, got Short=%q", tx.Short)
	}
	if tx.Hidden {
		t.Error("dim mode must not hide commands")
	}
	if strings.HasPrefix(console.Short, "[unavailable]") {
		t.Errorf("console is available and must not be dimmed, got %q", console.Short)
	}

	// Idempotent: applying twice must not double the prefix.
	applyCapabilityGating(root, consoleOnly, capability.ModeDim)
	if strings.Count(tx.Short, "[unavailable]") != 1 {
		t.Errorf("dim prefix applied twice: %q", tx.Short)
	}
}

func TestGatingHideMode(t *testing.T) {
	root, tx, console := gatedTree()

	applyCapabilityGating(root, capability.Set{Console: true}, capability.ModeHide)

	if !tx.Hidden {
		t.Error("tx should be hidden in hide mode")
	}
	if console.Hidden {
		t.Error("console is available and must not be hidden")
	}
}

func TestGatingOffMode(t *testing.T) {
	root, tx, _ := gatedTree()

	applyCapabilityGating(root, capability.Set{}, capability.ModeOff)

	if tx.Hidden || strings.HasPrefix(tx.Short, "[unavailable]") {
		t.Error("off mode must not touch the tree")
	}
}

func TestRequirementErrorWalksAncestors(t *testing.T) {
	_, tx, _ := gatedTree()
	leaf := tx.Commands()[0]

	err := requirementError(leaf, capability.Set{Console: true})
	if err == nil {
		t.Fatal("leaf under an unsatisfied group must be gated")
	}
	for _, want := range []string{"akt tx", "unavailable", "RPC endpoint"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}

	if err := requirementError(leaf, capability.Set{ChainQuery: true, ChainTx: true}); err != nil {
		t.Errorf("satisfied requirement must not error: %v", err)
	}
}

func TestHelpRequestedIgnoresPositionalsAndTerminator(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"help flag", []string{"tx", "bank", "--help"}, true},
		{"short help flag", []string{"tx", "-h"}, true},
		{"help with value form", []string{"tx", "--help=true"}, true},
		// A bare "help" as a positional VALUE must not disable enforcement:
		// `akt tx deployment close help` used to skip the no-context guard
		// and go straight to broadcast.
		{"help as positional value", []string{"tx", "deployment", "close", "help"}, false},
		{"flag after terminator", []string{"provider", "lease-shell", "--", "sh", "-h"}, false},
		{"none", []string{"query", "deployment"}, false},
	}

	for _, c := range cases {
		if got := helpRequested(c.args); got != c.want {
			t.Errorf("%s: helpRequested(%v) = %v, want %v", c.name, c.args, got, c.want)
		}
	}
}

func TestIsHelpInvocationMatchesHelpCommand(t *testing.T) {
	// cobra resolves `akt help foo` to the help command itself.
	helpCmd := &cobra.Command{Use: "help"}
	if !isHelpInvocation(helpCmd, []string{"help", "tx"}) {
		t.Error("the help command must count as a help invocation")
	}

	other := &cobra.Command{Use: "close"}
	if isHelpInvocation(other, []string{"tx", "deployment", "close", "help"}) {
		t.Error("a positional 'help' value must not count as a help invocation")
	}
}

func TestInvocationCapabilitiesGrantsExplicitOverrides(t *testing.T) {
	empty := capability.Set{}

	if got := invocationCapabilities(empty, nil, []string{"query", "deployment", "--node", "https://rpc"}, nil); !got.ChainQuery {
		t.Error("--node must grant chain capabilities")
	}
	if got := invocationCapabilities(empty, nil, []string{"query", "--node=https://rpc"}, nil); !got.ChainTx {
		t.Error("--node=value form must grant chain capabilities")
	}
	if got := invocationCapabilities(empty, nil, []string{"console", "whoami", "--console-api-key", "sk"}, nil); !got.Console {
		t.Error("--console-api-key must grant the console capability")
	}

	// Overrides after the terminator are command data, not akt flags.
	if got := invocationCapabilities(empty, nil, []string{"provider", "lease-shell", "--", "sh", "--node"}, nil); got.ChainQuery {
		t.Error("tokens after -- must not grant capabilities")
	}

	monitor := &cobra.Command{Use: "monitor"}
	root := &cobra.Command{Use: "akt"}
	root.AddCommand(monitor)
	if got := invocationCapabilities(empty, monitor, []string{"monitor", "https://rpc"}, []string{"https://rpc"}); !got.ChainQuery {
		t.Error("a positional monitor endpoint must grant chain capabilities")
	}
	if got := invocationCapabilities(empty, monitor, []string{"monitor"}, nil); got.ChainQuery {
		t.Error("monitor without an endpoint must not grant capabilities")
	}
}

func TestInvocationCapabilitiesGrantsFromEnv(t *testing.T) {
	t.Setenv("AKT_CONSOLE_API_KEY", "sk-env")

	if got := invocationCapabilities(capability.Set{}, nil, []string{"console", "whoami"}, nil); !got.Console {
		t.Error("AKT_CONSOLE_API_KEY must grant the console capability")
	}
}

func TestRequiresContextExemptsOfflineGroups(t *testing.T) {
	root := &cobra.Command{Use: "akt"}
	for _, group := range []string{"sdl", "console", "context", "version", "completion", "monitor"} {
		g := &cobra.Command{Use: group}
		root.AddCommand(g)
		if requiresContext(g) {
			t.Errorf("%s must not require a configured context", group)
		}
	}

	tx := &cobra.Command{Use: "tx"}
	root.AddCommand(tx)
	if !requiresContext(tx) {
		t.Error("tx must still require a context")
	}
}
