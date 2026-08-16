package flags

import (
	"strings"
	"testing"

	flagdefs "pkg.akt.dev/akt/internal/flags"

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

	if err := cmd.Flags().Set(flagdefs.FlagFees, "123uakt"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set(flagdefs.FlagGasPrices, "not-a-price"); err != nil {
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
		{name: "fees", flag: flagdefs.FlagFees, want: "--fees"},
		{name: "gas prices", flag: flagdefs.FlagGasPrices, want: "--gas-prices"},
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

	if err := cmd.Flags().Set(flagdefs.FlagGas, "250000"); err != nil {
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

	if err := cmd.Flags().Set(flagdefs.FlagGas, "not-a-number"); err != nil {
		t.Fatalf("set gas: %v", err)
	}

	if _, err := ClientOptionsFromFlags(cmd.Flags()); err == nil {
		t.Error("invalid gas value must be rejected")
	}
}

func TestClientOptionsCanonicalOptionalFlags(t *testing.T) {
	cmd := &cobra.Command{}
	AddTxFlagsToCmd(cmd)

	for name, value := range map[string]string{
		flagdefs.FlagNote:          "canonical note",
		flagdefs.FlagTimeoutHeight: "77",
		flagdefs.FlagSignMode:      SignModeLegacyAminoJSON,
	} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}

	opts, err := ClientOptionsFromFlags(cmd.Flags())
	if err != nil {
		t.Fatal(err)
	}
	applied := applyClientOptions(t, opts)
	if applied.Note != "canonical note" || applied.TimeoutHeight != 77 || applied.SignMode != SignModeLegacyAminoJSON {
		t.Fatalf("optional client flags were not applied: %+v", applied)
	}
}
