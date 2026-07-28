package flags

import (
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
