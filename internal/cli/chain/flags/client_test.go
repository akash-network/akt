package flags

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	cltypes "pkg.akt.dev/go/node/client/types"
)

func applyClientOptions(t *testing.T, opts []cltypes.ClientOption) *cltypes.ClientOptions {
	t.Helper()

	applied := &cltypes.ClientOptions{}
	for _, opt := range opts {
		if err := opt(applied); err != nil {
			t.Fatalf("apply option: %v", err)
		}
	}

	return applied
}

func TestClientOptionsDefaultGasSimulates(t *testing.T) {
	cmd := &cobra.Command{}
	AddTxFlagsToCmd(cmd)

	opts, err := ClientOptionsFromFlags(cmd.Flags())
	if err != nil {
		t.Fatalf("ClientOptionsFromFlags: %v", err)
	}

	applied := applyClientOptions(t, opts)
	if !applied.Gas.Simulate {
		t.Errorf("default --gas=auto must produce a simulating gas setting, got %+v", applied.Gas)
	}
	if applied.GasPrices != "0.025uakt" {
		t.Errorf("default gas prices = %q, want 0.025uakt", applied.GasPrices)
	}
}

func TestClientOptionsFixedFeesOverrideGasPrices(t *testing.T) {
	cmd := &cobra.Command{}
	AddTxFlagsToCmd(cmd)

	if err := cmd.Flags().Set(FlagFees, "123uakt"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set(FlagGasPrices, "not-a-price"); err != nil {
		t.Fatal(err)
	}

	opts, err := ClientOptionsFromFlags(cmd.Flags())
	if err != nil {
		t.Fatalf("ClientOptionsFromFlags: %v", err)
	}
	applied := applyClientOptions(t, opts)
	if applied.Fees != "123uakt" {
		t.Errorf("fees = %q, want 123uakt", applied.Fees)
	}
	if applied.GasPrices != "" {
		t.Errorf("fixed fees must clear gas prices, got %q", applied.GasPrices)
	}
}

func TestClientOptionsRejectMalformedCoinFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag string
		want string
	}{
		{name: "fees", flag: FlagFees, want: "--fees"},
		{name: "gas prices", flag: FlagGasPrices, want: "--gas-prices"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			AddTxFlagsToCmd(cmd)
			if err := cmd.Flags().Set(tc.flag, "not-a-coin"); err != nil {
				t.Fatal(err)
			}

			_, err := ClientOptionsFromFlags(cmd.Flags())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ClientOptionsFromFlags error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestClientOptionsExplicitGas(t *testing.T) {
	cmd := &cobra.Command{}
	AddTxFlagsToCmd(cmd)

	if err := cmd.Flags().Set(FlagGas, "250000"); err != nil {
		t.Fatalf("set gas: %v", err)
	}

	opts, err := ClientOptionsFromFlags(cmd.Flags())
	if err != nil {
		t.Fatalf("ClientOptionsFromFlags: %v", err)
	}

	applied := applyClientOptions(t, opts)
	if applied.Gas.Simulate || applied.Gas.Gas != 250000 {
		t.Errorf("explicit gas not applied: %+v", applied.Gas)
	}
}

func TestClientOptionsInvalidGasErrors(t *testing.T) {
	cmd := &cobra.Command{}
	AddTxFlagsToCmd(cmd)

	if err := cmd.Flags().Set(FlagGas, "not-a-number"); err != nil {
		t.Fatalf("set gas: %v", err)
	}

	if _, err := ClientOptionsFromFlags(cmd.Flags()); err == nil {
		t.Error("invalid gas value must be rejected")
	}
}
