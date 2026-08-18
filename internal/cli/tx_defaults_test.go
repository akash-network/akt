package cli

import (
	"os"
	"strings"
	"testing"

	sdkmath "cosmossdk.io/math"
	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestApplyGasPriceFloor(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		floor     string
		want      string
		wantError string
	}{
		{
			name:      "raises a lower matching price",
			candidate: "0.0025uakt",
			floor:     "0.025uakt",
			want:      "0.025000000000000000uakt",
		},
		{
			name:      "preserves a higher price verbatim",
			candidate: "0.04uakt",
			floor:     "0.025uakt",
			want:      "0.04uakt",
		},
		{
			name:      "rejects a different denomination",
			candidate: "0.04uatom",
			floor:     "0.025uakt",
			wantError: "no denomination",
		},
		{
			name:      "rejects malformed invocation prices",
			candidate: "bad-price",
			floor:     "0.025uakt",
			wantError: "--gas-prices",
		},
		{
			name:      "rejects malformed network prices",
			candidate: "0.04uakt",
			floor:     "bad-price",
			wantError: "configured network gas prices",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := applyGasPriceFloor(tc.candidate, tc.floor)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("gas prices = %q, want %q", got, tc.want)
			}
		})
	}
}

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

	if err := applyTransactionDefaults(cmd, resolved); err != nil {
		t.Fatal(err)
	}

	if got, _ := cmd.Flags().GetString(flagdefs.FlagGas); got != "300000" {
		t.Errorf("gas = %q, want context value", got)
	}
	if got, _ := cmd.Flags().GetFloat64(flagdefs.FlagGasAdjustment); got != 1.8 {
		t.Errorf("gas adjustment = %v, want context value", got)
	}
	if got, _ := cmd.Flags().GetString(flagdefs.FlagGasPrices); got != "0.07uakt" {
		t.Errorf("gas prices = %q, want environment value", got)
	}
	if got, _ := cmd.Flags().GetString(flagdefs.FlagFees); got != "" {
		t.Errorf("environment gas prices must suppress context fees, got %q", got)
	}
}

func TestApplyTransactionDefaultsLeavesExplicitFlagsAlone(t *testing.T) {
	t.Setenv("AKT_FEES", "77uakt")

	cmd := &cobra.Command{}
	cflags.AddTxFlagsToCmd(cmd)
	if err := cmd.Flags().Set(flagdefs.FlagGasPrices, "0.09uakt"); err != nil {
		t.Fatal(err)
	}

	if err := applyTransactionDefaults(cmd, &aktctx.Context{Fees: "42uakt", GasPrices: "0.04uakt"}); err != nil {
		t.Fatal(err)
	}

	if got, _ := cmd.Flags().GetString(flagdefs.FlagGasPrices); got != "0.09uakt" {
		t.Errorf("explicit gas prices changed to %q", got)
	}
	if got, _ := cmd.Flags().GetString(flagdefs.FlagFees); got != "" {
		t.Errorf("explicit gas prices must suppress lower-precedence fees, got %q", got)
	}
}

func TestApplyTransactionDefaultsRaisesExplicitPriceToNetworkFloor(t *testing.T) {
	cmd := &cobra.Command{}
	cflags.AddTxFlagsToCmd(cmd)
	if err := cmd.Flags().Set(flagdefs.FlagGasPrices, "0.0025uakt"); err != nil {
		t.Fatal(err)
	}

	if err := applyTransactionDefaults(cmd, &aktctx.Context{GasPrices: "0.025uakt"}); err != nil {
		t.Fatal(err)
	}

	got, _ := cmd.Flags().GetString(flagdefs.FlagGasPrices)
	if got != "0.025000000000000000uakt" {
		t.Errorf("gas prices = %q, want network floor", got)
	}
	prices, err := sdk.ParseDecCoins(got)
	if err != nil {
		t.Fatal(err)
	}
	fee := prices[0].Amount.Mul(sdkmath.LegacyNewDec(206739)).Ceil().RoundInt()
	if fee.String() != "5169" {
		t.Errorf("fee = %s, want 5169uakt for the reported gas estimate", fee)
	}
}

func TestApplyTransactionDefaultsDryRunUsesSimulationOnly(t *testing.T) {
	cmd := &cobra.Command{}
	cflags.AddTxFlagsToCmd(cmd)
	if err := cmd.Flags().Set(flagdefs.FlagDryRun, "true"); err != nil {
		t.Fatal(err)
	}

	if err := applyTransactionDefaults(cmd, &aktctx.Context{Gas: "auto"}); err != nil {
		t.Fatal(err)
	}

	if got, _ := cmd.Flags().GetString(flagdefs.FlagGas); got != "0" {
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
			if err := applyTransactionDefaults(cmd, tc.context); err != nil {
				t.Fatal(err)
			}

			if got, _ := cmd.Flags().GetString(flagdefs.FlagFees); got != tc.wantFees {
				t.Errorf("fees = %q, want %q", got, tc.wantFees)
			}
			if got, _ := cmd.Flags().GetString(flagdefs.FlagGasPrices); got != tc.wantPrices {
				t.Errorf("gas prices = %q, want %q", got, tc.wantPrices)
			}
		})
	}
}

func TestApplyTransactionDefaultsIgnoresCommandsWithoutTransactionFlags(t *testing.T) {
	cmd := &cobra.Command{}
	if err := applyTransactionDefaults(cmd, &aktctx.Context{Fees: "42uakt"}); err != nil {
		t.Fatal(err)
	}

	if cmd.Flags().Lookup(flagdefs.FlagFees) != nil {
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
