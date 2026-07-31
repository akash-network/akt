package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"pkg.akt.dev/akt/internal/capability"
	rest "pkg.akt.dev/go/provider/client"
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

func TestResolveProviderQueriesHostURI(t *testing.T) {
	cmd := newProviderCmd(t, "--provider", testProviderAddr)

	called := false
	addr, url, err := resolveProviderWithLookup(cmd, nil, func(_ context.Context, owner string) (string, error) {
		called = true
		if owner != testProviderAddr {
			t.Errorf("lookup owner = %q, want %q", owner, testProviderAddr)
		}
		return "https://on-chain.example.com:8443", nil
	})
	if err != nil {
		t.Fatalf("resolveProviderWithLookup: %v", err)
	}
	if !called {
		t.Fatal("provider address did not trigger an on-chain host URI lookup")
	}
	if addr.String() != testProviderAddr || url != "https://on-chain.example.com:8443" {
		t.Errorf("resolveProviderWithLookup = (%s, %q)", addr, url)
	}
}

func TestResolveProviderURLOverrideSkipsLookup(t *testing.T) {
	cmd := newProviderCmd(t,
		"--provider", testProviderAddr,
		"--provider-url", "https://override.example.com:8443",
	)

	_, url, err := resolveProviderWithLookup(cmd, nil, func(context.Context, string) (string, error) {
		t.Fatal("explicit --provider-url must skip on-chain lookup")
		return "", nil
	})
	if err != nil {
		t.Fatalf("resolveProviderWithLookup: %v", err)
	}
	if url != "https://override.example.com:8443" {
		t.Errorf("url = %q, want explicit override", url)
	}
}

func TestResolveProviderRejectsEmptyOnChainHostURI(t *testing.T) {
	cmd := newProviderCmd(t, "--provider", testProviderAddr)

	_, _, err := resolveProviderWithLookup(cmd, nil, func(context.Context, string) (string, error) {
		return "", nil
	})
	if err == nil {
		t.Fatal("an empty on-chain host URI must be rejected")
	}
	if !strings.Contains(err.Error(), "has no host URI on chain") {
		t.Errorf("error should identify the empty provider record, got %q", err)
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

func TestPrintJSONHonorsYAML(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "yaml", "")
	cmd.SetOut(&buf)

	value := struct {
		HostURI string `json:"host_uri"`
		Leases  int    `json:"leases"`
	}{HostURI: "https://provider.example.com:8443", Leases: 2}

	if err := printJSON(cmd, value); err != nil {
		t.Fatalf("printJSON: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Fatalf("YAML output is still JSON: %q", buf.String())
	}

	var got map[string]any
	if err := yaml.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode YAML: %v", err)
	}
	if got["host_uri"] != value.HostURI || got["leases"] != value.Leases {
		t.Errorf("YAML output = %#v, want JSON field names and scalar types", got)
	}
}

func TestLeaseShellDefaultsToBinSh(t *testing.T) {
	cmd := leaseShellCmd()
	if err := cmd.Args(cmd, nil); err != nil {
		t.Fatalf("lease-shell without a command must be accepted: %v", err)
	}

	got := leaseShellCommand(nil)
	if len(got) != 1 || got[0] != "/bin/sh" {
		t.Fatalf("default shell command = %#v, want [/bin/sh]", got)
	}
	if !strings.Contains(cmd.Long, "/bin/sh") || !strings.Contains(cmd.Example, "--service web\n") {
		t.Fatalf("lease-shell help does not explain the commandless default: long=%q example=%q", cmd.Long, cmd.Example)
	}
}

func TestLeaseLogsRejectsInvalidTailBeforeGateway(t *testing.T) {
	for _, args := range [][]string{
		{"lease-logs", "1", "--tail=-2"},
		{"lease-logs", "1", "--tail=5", "--follow"},
	} {
		root := Commands()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(args)

		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "--tail") {
			t.Fatalf("%v error = %v, want local --tail refusal", args, err)
		}
		if strings.Contains(err.Error(), "provider address") {
			t.Fatalf("%v reached provider resolution before validating --tail: %v", args, err)
		}
	}
}

func TestConsumeLeaseLogsHonorsServiceTailAndOneShotEOF(t *testing.T) {
	stream := make(chan rest.ServiceLogMessage, 3)
	stream <- rest.ServiceLogMessage{Name: "web-a", Message: "first"}
	stream <- rest.ServiceLogMessage{Name: "worker-a", Message: "ignore"}
	stream <- rest.ServiceLogMessage{Name: "web-b", Message: "last"}
	close(stream)

	onClose := make(chan string, 1)
	onClose <- "unexpected EOF"
	close(onClose)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	err := consumeLeaseLogs(context.Background(), cmd, &rest.ServiceLogs{
		Stream: stream, OnClose: onClose,
	}, "web", false, 1)
	if err != nil {
		t.Fatalf("consumeLeaseLogs: %v", err)
	}
	if got, want := buf.String(), "[web-b] last\n"; got != want {
		t.Fatalf("logs = %q, want %q", got, want)
	}
}

func TestConsumeLeaseEventsTreatsFollowEOFAsFailure(t *testing.T) {
	stream := make(chan rest.LeaseEvent)
	close(stream)
	onClose := make(chan string, 1)
	onClose <- "unexpected EOF"
	close(onClose)

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	err := consumeLeaseEvents(context.Background(), cmd, &rest.LeaseKubeEvents{
		Stream: stream, OnClose: onClose,
	}, true)
	if err == nil || !strings.Contains(err.Error(), "event stream closed") {
		t.Fatalf("follow EOF error = %v, want interrupted event stream", err)
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
