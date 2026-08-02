package cli

import (
	"testing"

	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestApplyTransactionDefaultsPrecedence(t *testing.T) {
	t.Setenv("AKT_GAS_PRICES", "0.07uakt")

	cmd := &cobra.Command{}
	cflags.AddTxFlagsToCmd(cmd)
	resolved := &aktctx.Context{
		Gas:           "300000",
		Fees:          "42uakt",
		GasPrices:     "0.04uakt",
		GasAdjustment: "1.8",
	}

	applyTransactionDefaults(cmd, resolved)

	if got, _ := cmd.Flags().GetString(cflags.FlagGas); got != "300000" {
		t.Errorf("gas = %q, want context value", got)
	}
	if got, _ := cmd.Flags().GetFloat64(cflags.FlagGasAdjustment); got != 1.8 {
		t.Errorf("gas adjustment = %v, want context value", got)
	}
	if got, _ := cmd.Flags().GetString(cflags.FlagGasPrices); got != "0.07uakt" {
		t.Errorf("gas prices = %q, want environment value", got)
	}
	if got, _ := cmd.Flags().GetString(cflags.FlagFees); got != "" {
		t.Errorf("environment gas prices must suppress context fees, got %q", got)
	}
}

func TestApplyTransactionDefaultsLeavesExplicitFlagsAlone(t *testing.T) {
	t.Setenv("AKT_FEES", "77uakt")

	cmd := &cobra.Command{}
	cflags.AddTxFlagsToCmd(cmd)
	if err := cmd.Flags().Set(cflags.FlagGasPrices, "0.09uakt"); err != nil {
		t.Fatal(err)
	}

	applyTransactionDefaults(cmd, &aktctx.Context{Fees: "42uakt", GasPrices: "0.04uakt"})

	if got, _ := cmd.Flags().GetString(cflags.FlagGasPrices); got != "0.09uakt" {
		t.Errorf("explicit gas prices changed to %q", got)
	}
	if got, _ := cmd.Flags().GetString(cflags.FlagFees); got != "" {
		t.Errorf("explicit gas prices must suppress lower-precedence fees, got %q", got)
	}
}

func TestApplyTransactionDefaultsDryRunUsesSimulationOnly(t *testing.T) {
	cmd := &cobra.Command{}
	cflags.AddTxFlagsToCmd(cmd)
	if err := cmd.Flags().Set(cflags.FlagDryRun, "true"); err != nil {
		t.Fatal(err)
	}

	applyTransactionDefaults(cmd, &aktctx.Context{Gas: "auto"})

	if got, _ := cmd.Flags().GetString(cflags.FlagGas); got != "0" {
		t.Errorf("dry-run gas = %q, want 0 to disable simulate-and-execute", got)
	}
}
