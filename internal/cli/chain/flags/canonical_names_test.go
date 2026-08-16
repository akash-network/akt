package flags

import (
	"testing"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCanonicalFlagSetBuilders(t *testing.T) {
	t.Run("keyring", func(t *testing.T) {
		flags := pflag.NewFlagSet("keyring", pflag.ContinueOnError)
		AddKeyringFlags(flags)
		for _, name := range []string{flagdefs.FlagKeyringDir, flagdefs.FlagKeyringBackend} {
			if flags.Lookup(name) == nil {
				t.Fatalf("missing --%s", name)
			}
		}
	})

	tests := []struct {
		name  string
		build func() *pflag.FlagSet
		flags []string
	}{
		{
			name:  "commission create",
			build: FlagSetCommissionCreate,
			flags: []string{flagdefs.FlagCommissionRate, flagdefs.FlagCommissionMaxRate, flagdefs.FlagCommissionMaxChangeRate},
		},
		{name: "amount", build: FlagSetAmount, flags: []string{flagdefs.FlagAmount}},
		{name: "public key", build: FlagSetPublicKey, flags: []string{flagdefs.FlagPubKey}},
		{
			name:  "description create",
			build: FlagSetDescriptionCreate,
			flags: []string{flagdefs.FlagMoniker, flagdefs.FlagIdentity, flagdefs.FlagWebsite, flagdefs.FlagSecurityContact, flagdefs.FlagDetails},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flags := test.build()
			for _, name := range test.flags {
				if flags.Lookup(name) == nil {
					t.Fatalf("missing --%s", name)
				}
			}
		})
	}
}

func TestCanonicalDeploymentAndMarketFlags(t *testing.T) {
	owner, err := sdk.AccAddressFromBech32(testOwner)
	if err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "deployment"}
	AddGroupIDFlags(cmd.Flags())
	MarkReqDeploymentIDFlags(cmd)
	if err := cmd.Flags().Set(flagdefs.FlagOwner, testOwner); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set(flagdefs.FlagDSeq, "42"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set(flagdefs.FlagGSeq, "7"); err != nil {
		t.Fatal(err)
	}

	deployment, err := DeploymentIDFromFlags(cmd.Flags())
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Owner != testOwner || deployment.DSeq != 42 {
		t.Fatalf("deployment ID = %+v", deployment)
	}
	owned, err := DeploymentIDFromFlagsForOwner(cmd.Flags(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if owned.Owner != testOwner || owned.DSeq != 42 {
		t.Fatalf("owned deployment ID = %+v", owned)
	}
	group, err := GroupIDFromFlags(cmd.Flags())
	if err != nil {
		t.Fatal(err)
	}
	if group.GSeq != 7 {
		t.Fatalf("group ID = %+v", group)
	}

	bidCmd := &cobra.Command{Use: "bid"}
	AddBidIDFlags(bidCmd.Flags())
	MarkReqProviderFlag(bidCmd)
	for name, value := range map[string]string{
		flagdefs.FlagOwner:    testOwner,
		flagdefs.FlagDSeq:     "42",
		flagdefs.FlagGSeq:     "7",
		flagdefs.FlagOSeq:     "3",
		flagdefs.FlagProvider: testProvider,
	} {
		if err := bidCmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	if _, err := BidIDFromFlags(bidCmd.Flags()); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalDeploymentFlagReadersPreserveTypeErrors(t *testing.T) {
	owner, err := sdk.AccAddressFromBech32(testOwner)
	if err != nil {
		t.Fatal(err)
	}

	badDeployment := pflag.NewFlagSet("deployment", pflag.ContinueOnError)
	badDeployment.String(flagdefs.FlagOwner, testOwner, "")
	badDeployment.String(flagdefs.FlagDSeq, "not-a-uint64", "")
	if _, err := DeploymentIDFromFlags(badDeployment); err == nil {
		t.Fatal("deployment dseq type mismatch succeeded")
	}
	if _, err := DeploymentIDFromFlagsForOwner(badDeployment, owner); err == nil {
		t.Fatal("owned deployment dseq type mismatch succeeded")
	}

	badGroup := pflag.NewFlagSet("group", pflag.ContinueOnError)
	AddDeploymentIDFlags(badGroup)
	badGroup.String(flagdefs.FlagGSeq, "not-a-uint32", "")
	if err := badGroup.Set(flagdefs.FlagOwner, testOwner); err != nil {
		t.Fatal(err)
	}
	if err := badGroup.Set(flagdefs.FlagDSeq, "42"); err != nil {
		t.Fatal(err)
	}
	if _, err := GroupIDFromFlags(badGroup); err == nil {
		t.Fatal("group gseq type mismatch succeeded")
	}
}

func TestBMELedgerFiltersReportMissingCanonicalFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
	}{
		{name: "denom", flags: []string{flagdefs.FlagOwner}},
		{name: "to denom", flags: []string{flagdefs.FlagOwner, flagdefs.FlagDenom}},
		{name: "status", flags: []string{flagdefs.FlagOwner, flagdefs.FlagDenom, flagdefs.FlagToDenom}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flags := pflag.NewFlagSet(test.name, pflag.ContinueOnError)
			for _, name := range test.flags {
				flags.String(name, "", "")
			}
			if _, err := BMELedgerFiltersFromFlags(flags); err == nil {
				t.Fatal("expected missing flag error")
			}
		})
	}
}
