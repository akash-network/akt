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
