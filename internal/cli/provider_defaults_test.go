package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"testing"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestApplyProviderDefaultsUsesContextUnlessFlagChanged(t *testing.T) {
	provider := &cobra.Command{Use: "provider"}
	provider.PersistentFlags().String(flagdefs.FlagAuthType, "", "")
	lease := &cobra.Command{Use: "lease-status"}
	status := &cobra.Command{Use: "status"}
	provider.AddCommand(lease, status)

	rc := &aktctx.Context{AuthType: "mtls"}
	if err := applyProviderDefaults(lease, rc); err != nil {
		t.Fatalf("applyProviderDefaults: %v", err)
	}
	if got, _ := lease.InheritedFlags().GetString(flagdefs.FlagAuthType); got != "mtls" {
		t.Fatalf("context provider auth = %q, want mtls", got)
	}

	if err := lease.InheritedFlags().Set(flagdefs.FlagAuthType, "jwt"); err != nil {
		t.Fatalf("set explicit auth type: %v", err)
	}
	if err := applyProviderDefaults(lease, rc); err != nil {
		t.Fatalf("applyProviderDefaults explicit: %v", err)
	}
	if got, _ := lease.InheritedFlags().GetString(flagdefs.FlagAuthType); got != "jwt" {
		t.Fatalf("explicit provider auth = %q, want jwt", got)
	}

	if err := applyProviderDefaults(status, rc); err != nil {
		t.Fatalf("applyProviderDefaults status: %v", err)
	}
	if got, _ := status.InheritedFlags().GetString(flagdefs.FlagAuthType); got != "jwt" {
		t.Fatalf("public status changed the explicit provider auth %q", got)
	}
}

func TestProviderAuthTypeForMCPUsesSelectedContext(t *testing.T) {
	mgr, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}
	if err := mgr.CreateContext(aktctx.Context{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
		ProviderDefaults: aktctx.ProviderDefaults{
			AuthType: aktctx.ProviderAuthMTLS,
		},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := mgr.UseContext("prod"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	cmd := &cobra.Command{Use: "mcp"}
	cmd.Flags().String(flagdefs.FlagContext, "", "")
	got, err := providerAuthTypeFor(cmd, func() *aktctx.Manager { return mgr })
	if err != nil {
		t.Fatalf("providerAuthTypeFor: %v", err)
	}
	if got != aktctx.ProviderAuthMTLS {
		t.Fatalf("MCP provider auth = %q, want mtls", got)
	}
}

func TestProviderAuthTypeForMCPWithoutContextDefaultsToJWT(t *testing.T) {
	mgr, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cmd := &cobra.Command{Use: "mcp"}
	cmd.Flags().String(flagdefs.FlagContext, "", "")

	got, err := providerAuthTypeFor(cmd, func() *aktctx.Manager { return mgr })
	if err != nil {
		t.Fatalf("providerAuthTypeFor without context: %v", err)
	}
	if got != aktctx.ProviderAuthJWT {
		t.Fatalf("MCP provider auth = %q, want jwt", got)
	}
	if selected := selectedMCPContext(cmd, func() *aktctx.Manager { return nil }); selected != "" {
		t.Fatalf("nil manager selected context = %q, want none", selected)
	}

	if err := cmd.Flags().Set(flagdefs.FlagContext, "missing"); err != nil {
		t.Fatal(err)
	}
	if _, err := providerAuthTypeFor(cmd, func() *aktctx.Manager { return mgr }); err == nil {
		t.Fatal("providerAuthTypeFor accepted an explicitly missing context")
	}
}
