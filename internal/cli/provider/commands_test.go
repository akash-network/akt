package provider

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/capability"
)

// newProviderCmd builds a throwaway command carrying the persistent flags the
// `akt provider` group installs, so the helpers under test see the same flag
// set they see in production.
func newProviderCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().String("auth-type", "", "")
	cmd.Flags().String("provider", "", "")
	cmd.Flags().String("provider-url", "", "")
	addLeaseShellFlags(cmd)

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("parse flags %v: %v", args, err)
	}

	return cmd
}

// A valid bech32 akash account address, used wherever the code path runs
// through sdk.AccAddressFromBech32.
const testProviderAddr = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"

// mustAddr decodes testProviderAddr, failing the test if the fixture is not a
// valid bech32 address.
func mustAddr(t *testing.T) sdk.AccAddress {
	t.Helper()

	addr, err := sdk.AccAddressFromBech32(testProviderAddr)
	if err != nil {
		t.Fatalf("test fixture is not a valid address: %v", err)
	}

	return addr
}

// TestResolveProviderRequiresAnAddress covers the guard that keeps a gateway
// call from being attempted with no provider at all — without it the bech32
// decode below would produce a confusing "empty address" error instead.
func TestResolveProviderRequiresAnAddress(t *testing.T) {
	cmd := newProviderCmd(t)

	_, _, err := resolveProvider(cmd, nil)
	if err == nil {
		t.Fatal("a missing provider address must be rejected")
	}
	if !strings.Contains(err.Error(), "--provider") {
		t.Errorf("error should name the flag, got %q", err)
	}
}

// TestResolveProviderRejectsInvalidBech32 covers the address-validation
// branch: a typo'd provider must fail locally rather than being sent to a
// gateway as an opaque string.
func TestResolveProviderRejectsInvalidBech32(t *testing.T) {
	cmd := newProviderCmd(t, "--provider", "not-a-bech32-address")

	if _, _, err := resolveProvider(cmd, nil); err == nil {
		t.Fatal("an invalid bech32 provider address must be rejected")
	} else if !strings.Contains(err.Error(), "invalid provider address") {
		t.Errorf("error should name the bad address, got %q", err)
	}
}

// TestResolveProviderRequiresURL pins the current limitation: on-chain HostURI
// lookup is not implemented, so --provider-url is mandatory. If that lookup is
// ever wired in, this test is the reminder to revisit the contract.
func TestResolveProviderRequiresURL(t *testing.T) {
	cmd := newProviderCmd(t, "--provider", testProviderAddr)

	if _, _, err := resolveProvider(cmd, nil); err == nil {
		t.Fatal("a missing --provider-url must be rejected")
	} else if !strings.Contains(err.Error(), "--provider-url") {
		t.Errorf("error should name --provider-url, got %q", err)
	}
}

// TestResolveProviderPositionalWinsOverFlag pins the documented precedence:
// `akt provider status <addr>` addresses <addr>, not whatever --provider
// happens to carry (from a shell alias, say). Getting this backwards would
// silently query the wrong provider.
func TestResolveProviderPositionalWinsOverFlag(t *testing.T) {
	const other = "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh"

	cmd := newProviderCmd(t, "--provider", other, "--provider-url", "https://gw.example.com:8443")

	addr, url, err := resolveProvider(cmd, []string{testProviderAddr})
	if err != nil {
		t.Fatalf("resolveProvider: %v", err)
	}
	if addr.String() != testProviderAddr {
		t.Errorf("address = %s, want the positional %s", addr, testProviderAddr)
	}
	if url != "https://gw.example.com:8443" {
		t.Errorf("url = %q, want the --provider-url value", url)
	}
}

// TestResolveProviderFlagFallback covers the no-positional branch, which every
// lease-* command uses (they pass nil args and rely on --provider).
func TestResolveProviderFlagFallback(t *testing.T) {
	cmd := newProviderCmd(t, "--provider", testProviderAddr, "--provider-url", "https://gw")

	addr, url, err := resolveProvider(cmd, nil)
	if err != nil {
		t.Fatalf("resolveProvider: %v", err)
	}
	if addr.String() != testProviderAddr || url != "https://gw" {
		t.Errorf("resolveProvider = (%s, %q)", addr, url)
	}
}

// TestLeaseIDFromFlagsPositionalDSeq covers the positional-only UX trial path
// (FEEDBACK 2026-07): the positional [dseq] must land in the LeaseID, and the
// gseq/oseq defaults must be 1 — a zero gseq would address a nonexistent
// group and fail at the provider with an unhelpful error.
func TestLeaseIDFromFlagsPositionalDSeq(t *testing.T) {
	cmd := newProviderCmd(t)

	addr := mustAddr(t)

	lid, err := leaseIDFromFlags(cmd, []string{"12345"}, addr)
	if err != nil {
		t.Fatalf("leaseIDFromFlags: %v", err)
	}

	if lid.DSeq != 12345 {
		t.Errorf("dseq = %d, want 12345", lid.DSeq)
	}
	if lid.GSeq != 1 || lid.OSeq != 1 {
		t.Errorf("gseq/oseq = %d/%d, want 1/1", lid.GSeq, lid.OSeq)
	}
	if lid.Provider != testProviderAddr {
		t.Errorf("provider = %q, want %q", lid.Provider, testProviderAddr)
	}
}

// TestLeaseIDFromFlagsPositionalWinsOverFlag pins the precedence for
// lease-shell, the one command that still registers --dseq: an explicit
// positional value must win, exactly as DSeqFromArgs documents.
func TestLeaseIDFromFlagsPositionalWinsOverFlag(t *testing.T) {
	cmd := newProviderCmd(t, "--dseq", "999", "--gseq", "2", "--oseq", "3")

	addr := mustAddr(t)

	lid, err := leaseIDFromFlags(cmd, []string{"12345"}, addr)
	if err != nil {
		t.Fatalf("leaseIDFromFlags: %v", err)
	}
	if lid.DSeq != 12345 {
		t.Errorf("dseq = %d, want the positional 12345", lid.DSeq)
	}
	if lid.GSeq != 2 || lid.OSeq != 3 {
		t.Errorf("gseq/oseq = %d/%d, want the flag values 2/3", lid.GSeq, lid.OSeq)
	}
}

// TestLeaseIDFromFlagsUsesDSeqFlagWithoutPositional covers lease-shell's own
// path: it consumes its positional args as the remote command, so --dseq is
// the only source there.
func TestLeaseIDFromFlagsUsesDSeqFlagWithoutPositional(t *testing.T) {
	cmd := newProviderCmd(t, "--dseq", "777")

	addr := mustAddr(t)

	lid, err := leaseIDFromFlags(cmd, nil, addr)
	if err != nil {
		t.Fatalf("leaseIDFromFlags: %v", err)
	}
	if lid.DSeq != 777 {
		t.Errorf("dseq = %d, want 777 from --dseq", lid.DSeq)
	}
}

// TestLeaseIDFromFlagsRejectsMissingDSeq covers the guard that stops a lease
// call from being made against dseq 0, which the provider would answer with a
// generic not-found.
func TestLeaseIDFromFlagsRejectsMissingDSeq(t *testing.T) {
	cmd := newProviderCmd(t)

	addr := mustAddr(t)

	if _, err := leaseIDFromFlags(cmd, nil, addr); err == nil {
		t.Fatal("dseq 0 must be rejected")
	} else if !strings.Contains(err.Error(), "dseq is required") {
		t.Errorf("error should say dseq is required, got %q", err)
	}
}

// TestLeaseIDFromFlagsRejectsNonNumericDSeq covers the positional parse error.
func TestLeaseIDFromFlagsRejectsNonNumericDSeq(t *testing.T) {
	cmd := newProviderCmd(t)

	addr := mustAddr(t)

	if _, err := leaseIDFromFlags(cmd, []string{"twelve"}, addr); err == nil {
		t.Fatal("a non-numeric positional dseq must be rejected")
	} else if !strings.Contains(err.Error(), "invalid dseq") {
		t.Errorf("error should name the bad dseq, got %q", err)
	}
}

// TestPrintJSONWritesToCommandOutput covers the happy path plus the marshal
// error branch. printJSON is the only output path for every provider query,
// so a silent write to os.Stdout (rather than cmd.OutOrStdout) would break
// every output-capturing caller.
func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	if err := printJSON(cmd, map[string]int{"dseq": 42}); err != nil {
		t.Fatalf("printJSON: %v", err)
	}

	var got map[string]int
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON (%q): %v", buf.String(), err)
	}
	if got["dseq"] != 42 {
		t.Errorf("output = %v, want dseq 42", got)
	}

	// Unmarshalable values must surface an error, not a partial write.
	if err := printJSON(cmd, make(chan int)); err == nil {
		t.Error("an unmarshalable value must produce an error")
	}
}

// TestCommandsGatedOnProviderCapability pins the capability annotation. The
// gating layer reads it to dim/hide the group when the context has no RPC
// endpoint; losing the annotation silently re-exposes commands that cannot
// work (gateway discovery and wallet auth both need chain access).
func TestCommandsGatedOnProviderCapability(t *testing.T) {
	cmd := Commands()

	if got := cmd.Annotations[capability.AnnotationKey]; got != string(capability.Provider) {
		t.Errorf("provider group annotation = %q, want %q", got, capability.Provider)
	}

	for _, name := range []string{"auth-type", "provider", "provider-url"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("persistent flag --%s missing", name)
		}
	}
}

// TestCommandsRegistersEveryGatewayOperation guards against a subcommand being
// dropped from the tree during a refactor: each name here is a documented
// `akt provider` operation.
func TestCommandsRegistersEveryGatewayOperation(t *testing.T) {
	cmd := Commands()

	have := map[string]bool{}
	for _, sub := range cmd.Commands() {
		have[sub.Name()] = true
	}

	for _, want := range []string{
		"status", "lease-status", "lease-logs", "lease-events",
		"lease-shell", "send-manifest", "get-manifest",
		"migrate-hostnames", "migrate-endpoints",
	} {
		if !have[want] {
			t.Errorf("subcommand %q missing from the provider group", want)
		}
	}
}

// TestPositionalDSeqCommandsDoNotRegisterDSeqFlag pins the positional-only UX
// trial (FEEDBACK 2026-07). If --dseq comes back on these commands without
// leaseIDFromFlags being revisited, the flag would silently win over the
// positional argument for callers that pass both.
func TestPositionalDSeqCommandsDoNotRegisterDSeqFlag(t *testing.T) {
	cmd := Commands()

	positional := map[string]bool{
		"lease-status": true, "lease-logs": true,
		"lease-events": true, "get-manifest": true,
	}

	for _, sub := range cmd.Commands() {
		switch {
		case positional[sub.Name()]:
			if sub.Flags().Lookup("dseq") != nil {
				t.Errorf("%s must take dseq positionally, not via --dseq", sub.Name())
			}
			if sub.Flags().Lookup("gseq") == nil || sub.Flags().Lookup("oseq") == nil {
				t.Errorf("%s should still expose --gseq/--oseq", sub.Name())
			}

		case sub.Name() == "lease-shell":
			// lease-shell consumes its positional args as the remote command,
			// so it keeps --dseq.
			if sub.Flags().Lookup("dseq") == nil {
				t.Error("lease-shell must keep --dseq (its positionals are the remote command)")
			}
		}
	}
}

// TestStatusRejectsMissingProvider drives the real RunE far enough to prove
// the resolveProvider guard runs before any network call is attempted.
func TestStatusRejectsMissingProvider(t *testing.T) {
	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"status"})

	if err := root.Execute(); err == nil {
		t.Fatal("status without a provider must fail")
	} else if !strings.Contains(err.Error(), "provider address is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestStatusRejectsMissingProviderURL proves the second guard fires too, so a
// user who supplies only an address gets the actionable message instead of a
// dial error against an empty URL.
func TestStatusRejectsMissingProviderURL(t *testing.T) {
	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"status", testProviderAddr})

	if err := root.Execute(); err == nil {
		t.Fatal("status without --provider-url must fail")
	} else if !strings.Contains(err.Error(), "--provider-url is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestGatewayClientRejectsUnknownAuthType covers gatewayClientFromCmd's only
// local failure mode. --auth-type is a persistent flag on the whole group, so
// a typo would otherwise be discovered only when the provider rejected the
// unsigned request.
func TestGatewayClientRejectsUnknownAuthType(t *testing.T) {
	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"status", testProviderAddr,
		"--provider-url", "https://gw.example",
		"--auth-type", "bogus",
	})

	if err := root.Execute(); err == nil {
		t.Fatal("an unknown --auth-type must be rejected")
	} else if !strings.Contains(err.Error(), "unsupported auth type") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestLeaseCommandsRequireProviderBeforeGatewayWork walks every lease-scoped
// subcommand through its first guard. These commands take no positional
// provider, so a missing --provider must be reported by name rather than
// surfacing as a gateway construction failure.
func TestLeaseCommandsRequireProviderBeforeGatewayWork(t *testing.T) {
	cases := [][]string{
		{"lease-status", "1"},
		{"lease-logs", "1"},
		{"lease-events", "1"},
		{"get-manifest", "1"},
		{"lease-shell", "--dseq", "1", "--", "/bin/sh"},
		{"send-manifest", "deploy.yaml"},
		{"migrate-hostnames", "--dseq", "1", "--hostnames", "a.example.com"},
		{"migrate-endpoints", "--dseq", "1", "--endpoints", "ep1"},
	}

	for _, args := range cases {
		root := Commands()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(args)

		err := root.Execute()
		if err == nil {
			t.Errorf("%v without a provider must fail", args)
			continue
		}
		if !strings.Contains(err.Error(), "provider address is required") {
			t.Errorf("%v: unexpected error %v", args, err)
		}
	}
}
