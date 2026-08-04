package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	mtypes "pkg.akt.dev/go/node/market/v1"
)

// Addresses used by the lease-resolution fixtures. testProviderAddr is shared
// with commands_test.go.
const (
	testOwnerAddr         = "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh"
	otherTestProviderAddr = "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f"
)

// newLeaseCmd builds a provider command whose client context carries an owner,
// so lease resolution has the same default account production has.
func newLeaseCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()

	cmd := newProviderCmd(t, args...)

	addr := mustAddrFrom(t, testOwnerAddr)
	cctx := sdkclient.Context{}.WithFromAddress(addr)
	cmd.SetContext(context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx))

	return cmd
}

func mustAddrFrom(t *testing.T, bech32 string) sdk.AccAddress {
	t.Helper()

	addr, err := sdk.AccAddressFromBech32(bech32)
	if err != nil {
		t.Fatalf("test fixture %q is not a valid address: %v", bech32, err)
	}

	return addr
}

func testLease(gseq, oseq uint32, provider string, state mtypes.Lease_State) mtypes.Lease {
	return mtypes.Lease{
		ID: mtypes.LeaseID{
			Owner:    testOwnerAddr,
			DSeq:     12345,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: provider,
		},
		State: state,
	}
}

// leasesFixture returns a leaseQuery serving a fixed set of leases and
// asserting the owner/dseq the resolver filtered on.
func leasesFixture(t *testing.T, wantDSeq uint64, leases ...mtypes.Lease) leaseQuery {
	t.Helper()

	return func(_ context.Context, owner string, dseq uint64) ([]mtypes.Lease, error) {
		if owner != testOwnerAddr {
			t.Errorf("lease query owner = %q, want the context default account %q", owner, testOwnerAddr)
		}
		if dseq != wantDSeq {
			t.Errorf("lease query dseq = %d, want %d", dseq, wantDSeq)
		}

		return leases, nil
	}
}

func hostURIFixture(t *testing.T, wantProvider, url string) hostURIQuery {
	t.Helper()

	return func(_ context.Context, provider string) (string, error) {
		if provider != wantProvider {
			t.Errorf("host URI lookup provider = %q, want %q", provider, wantProvider)
		}

		return url, nil
	}
}

// TestResolveLeaseAdoptsTheActiveLeaseProvider is the fix for the reported bug:
// a deployment akt set up itself is already identified by its dseq, so no
// --provider should be needed. The resolved lease is also authoritative for
// gseq/oseq, which the caller never named.
func TestResolveLeaseAdoptsTheActiveLeaseProvider(t *testing.T) {
	cmd := newLeaseCmd(t, "--dseq", "12345")

	lid, url, err := resolveLeaseWith(cmd, nil,
		leasesFixture(t, 12345,
			testLease(1, 1, otherTestProviderAddr, mtypes.LeaseClosed),
			testLease(2, 3, testProviderAddr, mtypes.LeaseActive),
		),
		hostURIFixture(t, testProviderAddr, "https://gw.example.com:8443"),
		nil)
	if err != nil {
		t.Fatalf("resolveLeaseWith: %v", err)
	}

	if lid.Provider != testProviderAddr {
		t.Errorf("provider = %q, want the active lease's %q", lid.Provider, testProviderAddr)
	}
	if lid.Owner != testOwnerAddr {
		t.Errorf("owner = %q, want the context default account %q", lid.Owner, testOwnerAddr)
	}
	if lid.DSeq != 12345 {
		t.Errorf("dseq = %d, want 12345", lid.DSeq)
	}
	if lid.GSeq != 2 || lid.OSeq != 3 {
		t.Errorf("gseq/oseq = %d/%d, want the active lease's 2/3", lid.GSeq, lid.OSeq)
	}
	if url != "https://gw.example.com:8443" {
		t.Errorf("gateway url = %q, want the resolved provider's host URI", url)
	}
}

// TestResolveLeaseKeepsExplicitSequences proves an explicitly named group or
// order is a filter, not a suggestion: it narrows the candidate leases and is
// never overwritten by the one that matched.
func TestResolveLeaseKeepsExplicitSequences(t *testing.T) {
	cmd := newLeaseCmd(t, "--dseq", "12345", "--gseq", "2", "--oseq", "1")

	lid, _, err := resolveLeaseWith(cmd, nil,
		leasesFixture(t, 12345,
			testLease(1, 1, otherTestProviderAddr, mtypes.LeaseActive),
			testLease(2, 1, testProviderAddr, mtypes.LeaseActive),
		),
		hostURIFixture(t, testProviderAddr, "https://gw.example.com:8443"),
		nil)
	if err != nil {
		t.Fatalf("resolveLeaseWith: %v", err)
	}

	if lid.Provider != testProviderAddr {
		t.Errorf("provider = %q, want the gseq-2 lease's %q", lid.Provider, testProviderAddr)
	}
	if lid.GSeq != 2 || lid.OSeq != 1 {
		t.Errorf("gseq/oseq = %d/%d, want the explicit 2/1", lid.GSeq, lid.OSeq)
	}
}

// TestResolveLeaseExplicitProviderSkipsTheChain keeps --provider a genuine
// override: naming it must not cost a lease query.
func TestResolveLeaseExplicitProviderSkipsTheChain(t *testing.T) {
	cmd := newLeaseCmd(t, "--dseq", "12345", "--provider", testProviderAddr,
		"--provider-url", "https://override.example.com:8443")

	lid, url, err := resolveLeaseWith(cmd, nil,
		func(context.Context, string, uint64) ([]mtypes.Lease, error) {
			t.Fatal("an explicit --provider must not trigger a lease lookup")
			return nil, nil
		},
		func(context.Context, string) (string, error) {
			t.Fatal("an explicit --provider-url must not trigger a host URI lookup")
			return "", nil
		},
		nil)
	if err != nil {
		t.Fatalf("resolveLeaseWith: %v", err)
	}
	if lid.Provider != testProviderAddr {
		t.Errorf("provider = %q, want the explicit %q", lid.Provider, testProviderAddr)
	}
	if lid.GSeq != 1 || lid.OSeq != 1 {
		t.Errorf("gseq/oseq = %d/%d, want the flag defaults 1/1", lid.GSeq, lid.OSeq)
	}
	if url != "https://override.example.com:8443" {
		t.Errorf("gateway url = %q, want the explicit override", url)
	}
}

// TestResolveLeaseReportsNoLeases covers a dseq that never reached a lease —
// the error must say so and point at a command that works.
func TestResolveLeaseReportsNoLeases(t *testing.T) {
	cmd := newLeaseCmd(t, "--dseq", "12345")

	_, _, err := resolveLeaseWith(cmd, nil, leasesFixture(t, 12345), nil, nil)
	if err == nil {
		t.Fatal("a deployment with no leases must fail")
	}
	if !strings.Contains(err.Error(), "deployment 12345 has no leases") {
		t.Errorf("error should name the empty deployment, got %q", err)
	}
	if !strings.Contains(err.Error(), "akt query market lease 12345") {
		t.Errorf("error should point at a real inspection command, got %q", err)
	}
}

// TestResolveLeaseReportsInactiveLeases mirrors the Console rail's wording
// (internal/cli/console/gateway.go): list the states that do exist so the user
// knows whether to wait or to redeploy.
func TestResolveLeaseReportsInactiveLeases(t *testing.T) {
	cmd := newLeaseCmd(t, "--dseq", "12345")

	_, _, err := resolveLeaseWith(cmd, nil,
		leasesFixture(t, 12345,
			testLease(1, 1, testProviderAddr, mtypes.LeaseClosed),
			testLease(2, 1, otherTestProviderAddr, mtypes.LeaseInsufficientFunds),
		),
		nil, nil)
	if err == nil {
		t.Fatal("a deployment with no active lease must fail")
	}
	if !strings.Contains(err.Error(), "no active lease") {
		t.Errorf("error should say no lease is active, got %q", err)
	}
	for _, state := range []string{"closed", "insufficient_funds"} {
		if !strings.Contains(err.Error(), state) {
			t.Errorf("error should list the %s state, got %q", state, err)
		}
	}
}

// TestResolveLeaseListsAmbiguousProviders covers the only case where
// --provider is genuinely required. The addresses must be listed in full: a
// truncated address cannot be pasted back into --provider.
func TestResolveLeaseListsAmbiguousProviders(t *testing.T) {
	cmd := newLeaseCmd(t, "--dseq", "12345")

	_, _, err := resolveLeaseWith(cmd, nil,
		leasesFixture(t, 12345,
			testLease(1, 1, testProviderAddr, mtypes.LeaseActive),
			testLease(2, 1, otherTestProviderAddr, mtypes.LeaseActive),
		),
		nil, nil)
	if err == nil {
		t.Fatal("two active leases must not be resolved silently")
	}
	if !strings.Contains(err.Error(), "--provider") {
		t.Errorf("error should ask for --provider, got %q", err)
	}
	for _, addr := range []string{testProviderAddr, otherTestProviderAddr} {
		if !strings.Contains(err.Error(), addr) {
			t.Errorf("error should list %s in full, got %q", addr, err)
		}
	}
}

// TestResolveLeaseRequiresADefaultAccount refuses to send an empty owner to
// the chain, which means network-wide scope (SPEC §3.8.4).
func TestResolveLeaseRequiresADefaultAccount(t *testing.T) {
	// newProviderCmd has no client context, so there is no default account.
	cmd := newProviderCmd(t, "--dseq", "12345")

	_, _, err := resolveLeaseWith(cmd, nil,
		func(context.Context, string, uint64) ([]mtypes.Lease, error) {
			t.Fatal("an empty owner must never reach the chain")
			return nil, nil
		},
		nil, nil)
	if err == nil {
		t.Fatal("resolution without a default account must fail")
	}
	if !strings.Contains(err.Error(), "default account") {
		t.Errorf("error should name the missing default account, got %q", err)
	}
}

// TestResolveLeaseOrdersDSeqThenPreflightThenProvider pins the ordering the
// bug report exposed. Resolving the provider first meant a command missing
// both values blamed the provider and never mentioned the dseq; the local
// signing identity must still be validated before any network work.
func TestResolveLeaseOrdersDSeqThenPreflightThenProvider(t *testing.T) {
	preflighted := false
	preflight := func() error {
		preflighted = true
		return fmt.Errorf("provider gateway authentication requires a configured default account")
	}

	// No dseq: the dseq guard fires before the preflight.
	cmd := newLeaseCmd(t)
	_, _, err := resolveLeaseWith(cmd, nil,
		func(context.Context, string, uint64) ([]mtypes.Lease, error) {
			t.Fatal("provider resolution ran without a dseq")
			return nil, nil
		}, nil, preflight)
	if err == nil || !strings.Contains(err.Error(), "dseq is required") {
		t.Fatalf("error = %v, want the missing dseq", err)
	}
	if preflighted {
		t.Error("preflight ran before the dseq was resolved")
	}

	// With a dseq: the preflight fires before any lease lookup.
	cmd = newLeaseCmd(t, "--dseq", "12345")
	_, _, err = resolveLeaseWith(cmd, nil,
		func(context.Context, string, uint64) ([]mtypes.Lease, error) {
			t.Fatal("lease lookup ran before the authentication preflight")
			return nil, nil
		}, nil, preflight)
	if err == nil || !strings.Contains(err.Error(), "configured default account") {
		t.Fatalf("error = %v, want the signing identity remedy", err)
	}
	if !preflighted {
		t.Error("preflight never ran")
	}
}

// TestActiveLeaseProvidersReturnsEveryProvider backs `send-manifest` without
// --provider, which SPEC §2.4 defines as delivery to all active leases.
func TestActiveLeaseProvidersReturnsEveryProvider(t *testing.T) {
	scope := leaseScope{Owner: testOwnerAddr, DSeq: 12345}

	providers, err := activeLeaseProviders(context.Background(), scope,
		leasesFixture(t, 12345,
			testLease(1, 1, testProviderAddr, mtypes.LeaseActive),
			testLease(2, 1, otherTestProviderAddr, mtypes.LeaseActive),
			// A second active lease with the same provider must not
			// produce a duplicate submission.
			testLease(3, 1, testProviderAddr, mtypes.LeaseActive),
			testLease(4, 1, otherTestProviderAddr, mtypes.LeaseClosed),
		))
	if err != nil {
		t.Fatalf("activeLeaseProviders: %v", err)
	}

	want := []string{otherTestProviderAddr, testProviderAddr}
	if len(providers) != len(want) {
		t.Fatalf("providers = %v, want %v", providers, want)
	}
	for i := range want {
		if providers[i] != want[i] {
			t.Errorf("providers = %v, want %v (sorted)", providers, want)
		}
	}
}

func TestActiveLeaseProvidersReportsNoActiveLease(t *testing.T) {
	scope := leaseScope{Owner: testOwnerAddr, DSeq: 12345}

	_, err := activeLeaseProviders(context.Background(), scope,
		leasesFixture(t, 12345, testLease(1, 1, testProviderAddr, mtypes.LeaseClosed)))
	if err == nil {
		t.Fatal("a deployment with no active lease must fail")
	}
	if !strings.Contains(err.Error(), "no active lease") ||
		!strings.Contains(err.Error(), "akt query market lease 12345") {
		t.Errorf("error should name the problem and a real remedy, got %q", err)
	}
}

// TestSendManifestTargetsRefusesOneURLForManyProviders keeps --provider-url an
// honest single-gateway override: it cannot stand in for a fan-out.
func TestSendManifestTargetsRefusesOneURLForManyProviders(t *testing.T) {
	cmd := newLeaseCmd(t, "--dseq", "12345", "--provider-url", "https://gw.example.com:8443")

	scope, err := leaseScopeFromCmd(cmd, nil)
	if err != nil {
		t.Fatalf("leaseScopeFromCmd: %v", err)
	}

	leases := leasesFixture(t, 12345,
		testLease(1, 1, testProviderAddr, mtypes.LeaseActive),
		testLease(2, 1, otherTestProviderAddr, mtypes.LeaseActive),
	)

	if _, err := sendManifestTargets(cmd, scope, leases); err == nil {
		t.Fatal("--provider-url with several active leases must be refused")
	} else if !strings.Contains(err.Error(), "--provider") {
		t.Errorf("error should ask for --provider, got %q", err)
	}
}

// TestSendManifestTargetsFansOutToEveryActiveLease is SPEC §2.4's documented
// default for send-manifest: no --provider means every provider holding an
// active lease.
func TestSendManifestTargetsFansOutToEveryActiveLease(t *testing.T) {
	cmd := newLeaseCmd(t, "--dseq", "12345")

	scope, err := leaseScopeFromCmd(cmd, nil)
	if err != nil {
		t.Fatalf("leaseScopeFromCmd: %v", err)
	}

	providers, err := sendManifestTargets(cmd, scope, leasesFixture(t, 12345,
		testLease(1, 1, testProviderAddr, mtypes.LeaseActive),
		testLease(2, 1, otherTestProviderAddr, mtypes.LeaseActive),
		testLease(3, 1, testProviderAddr, mtypes.LeaseClosed),
	))
	if err != nil {
		t.Fatalf("sendManifestTargets: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("providers = %v, want both active-lease providers", providers)
	}
}

// TestSendManifestTargetsHonorsAnExplicitProvider keeps --provider a narrowing
// override, and must not spend a lease query to honor it.
func TestSendManifestTargetsHonorsAnExplicitProvider(t *testing.T) {
	cmd := newLeaseCmd(t, "--dseq", "12345", "--provider", testProviderAddr)

	scope, err := leaseScopeFromCmd(cmd, nil)
	if err != nil {
		t.Fatalf("leaseScopeFromCmd: %v", err)
	}

	providers, err := sendManifestTargets(cmd, scope,
		func(context.Context, string, uint64) ([]mtypes.Lease, error) {
			t.Fatal("an explicit --provider must not trigger a lease lookup")
			return nil, nil
		})
	if err != nil {
		t.Fatalf("sendManifestTargets: %v", err)
	}
	if len(providers) != 1 || providers[0] != testProviderAddr {
		t.Errorf("providers = %v, want only the explicit %s", providers, testProviderAddr)
	}
}

// TestMissingDSeqErrorNamesTheAcceptedForm pins that the remedy matches the
// command: a positional-[dseq] command must never be told to pass --dseq, and
// a flag-only command must never be told to pass a positional.
func TestMissingDSeqErrorNamesTheAcceptedForm(t *testing.T) {
	positional := &cobra.Command{Use: "lease-status"}
	addLeaseFlags(positional)

	if err := missingDSeqError(positional); !strings.Contains(err.Error(), "positional argument") {
		t.Errorf("positional-dseq command remedy = %q, want the positional form", err)
	} else if strings.Contains(err.Error(), "--dseq") {
		t.Errorf("positional-dseq command must not advertise the disabled --dseq flag: %q", err)
	}

	flagged := &cobra.Command{Use: "lease-shell"}
	addLeaseShellFlags(flagged)

	if err := missingDSeqError(flagged); !strings.Contains(err.Error(), "--dseq") {
		t.Errorf("flag-dseq command remedy = %q, want --dseq", err)
	}
}
