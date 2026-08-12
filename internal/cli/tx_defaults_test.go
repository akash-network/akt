package cli

import (
	"os"
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

func TestApplyTransactionDefaultsFeeSourcePrecedence(t *testing.T) {
	tests := []struct {
		name       string
		envFees    *string
		envPrices  *string
		context    *aktctx.Context
		wantFees   string
		wantPrices string
	}{
		{
			name:       "environment fixed fees override every context fee source",
			envFees:    stringPointer("77uakt"),
			context:    &aktctx.Context{Fees: "42uakt", GasPrices: "0.04uakt"},
			wantFees:   "77uakt",
			wantPrices: "",
		},
		{
			name:       "environment gas prices override context fixed fees",
			envPrices:  stringPointer("0.09uakt"),
			context:    &aktctx.Context{Fees: "42uakt", GasPrices: "0.04uakt"},
			wantFees:   "",
			wantPrices: "0.09uakt",
		},
		{
			name:       "empty environment fees deliberately select context gas prices",
			envFees:    stringPointer(""),
			context:    &aktctx.Context{Fees: "42uakt", GasPrices: "0.04uakt"},
			wantFees:   "",
			wantPrices: "0.04uakt",
		},
		{
			name:       "context fixed fees suppress context gas prices",
			context:    &aktctx.Context{Fees: "42uakt", GasPrices: "0.04uakt"},
			wantFees:   "42uakt",
			wantPrices: "",
		},
		{
			name:       "context gas prices apply without fixed fees",
			context:    &aktctx.Context{GasPrices: "0.04uakt"},
			wantFees:   "",
			wantPrices: "0.04uakt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			unsetTestEnvironment(t, "AKT_FEES", "AKT_GAS_PRICES")
			if tc.envFees != nil {
				t.Setenv("AKT_FEES", *tc.envFees)
			}
			if tc.envPrices != nil {
				t.Setenv("AKT_GAS_PRICES", *tc.envPrices)
			}

			cmd := &cobra.Command{}
			cflags.AddTxFlagsToCmd(cmd)
			applyTransactionDefaults(cmd, tc.context)

			if got, _ := cmd.Flags().GetString(cflags.FlagFees); got != tc.wantFees {
				t.Errorf("fees = %q, want %q", got, tc.wantFees)
			}
			if got, _ := cmd.Flags().GetString(cflags.FlagGasPrices); got != tc.wantPrices {
				t.Errorf("gas prices = %q, want %q", got, tc.wantPrices)
			}
		})
	}
}

func TestApplyTransactionDefaultsIgnoresCommandsWithoutTransactionFlags(t *testing.T) {
	cmd := &cobra.Command{}
	applyTransactionDefaults(cmd, &aktctx.Context{Fees: "42uakt"})

	if cmd.Flags().Lookup(cflags.FlagFees) != nil {
		t.Fatal("non-transaction command gained transaction flags")
	}
}

func stringPointer(value string) *string {
	return &value
}

func unsetTestEnvironment(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		value, exists := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}
