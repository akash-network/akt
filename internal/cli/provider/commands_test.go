package provider

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	cmd.Flags().String(flagdefs.FlagAuthType, "", "")
	cmd.Flags().String(flagdefs.FlagProvider, "", "")
	cmd.Flags().String(flagdefs.FlagProviderURL, "", "")
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

// TestLeaseScopePositionalDSeq covers the positional-only UX trial path
// (FEEDBACK 2026-07): the positional [dseq] must land in the LeaseID, and the
// gseq/oseq defaults must be 1 — a zero gseq would address a nonexistent
// group and fail at the provider with an unhelpful error.
func TestLeaseScopePositionalDSeq(t *testing.T) {
	cmd := newProviderCmd(t)

	scope, err := leaseScopeFromCmd(cmd, []string{"12345"})
	if err != nil {
		t.Fatalf("leaseScopeFromCmd: %v", err)
	}

	lid := scope.leaseID(testProviderAddr)

	if lid.DSeq != 12345 {
		t.Errorf("dseq = %d, want 12345", lid.DSeq)
	}
	if lid.GSeq != 1 || lid.OSeq != 1 {
		t.Errorf("gseq/oseq = %d/%d, want 1/1", lid.GSeq, lid.OSeq)
	}
	if lid.Provider != testProviderAddr {
		t.Errorf("provider = %q, want %q", lid.Provider, testProviderAddr)
	}
	if scope.GSeqSet || scope.OSeqSet {
		t.Errorf("unset --gseq/--oseq recorded as explicit (%v/%v)", scope.GSeqSet, scope.OSeqSet)
	}
}

// TestLeaseScopePositionalWinsOverFlag pins the precedence for lease-shell,
// the one lease command that still registers --dseq: an explicit positional
// value must win, exactly as DSeqFromArgs documents.
func TestLeaseScopePositionalWinsOverFlag(t *testing.T) {
	cmd := newProviderCmd(t, "--dseq", "999", "--gseq", "2", "--oseq", "3")

	scope, err := leaseScopeFromCmd(cmd, []string{"12345"})
	if err != nil {
		t.Fatalf("leaseScopeFromCmd: %v", err)
	}

	lid := scope.leaseID(testProviderAddr)
	if lid.DSeq != 12345 {
		t.Errorf("dseq = %d, want the positional 12345", lid.DSeq)
	}
	if lid.GSeq != 2 || lid.OSeq != 3 {
		t.Errorf("gseq/oseq = %d/%d, want the flag values 2/3", lid.GSeq, lid.OSeq)
	}
	if !scope.GSeqSet || !scope.OSeqSet {
		t.Error("explicit --gseq/--oseq must be recorded so lease resolution does not override them")
	}
}

// TestLeaseScopeUsesDSeqFlagWithoutPositional covers lease-shell's own path:
// it consumes its positional args as the remote command, so --dseq is the
// only source there.
func TestLeaseScopeUsesDSeqFlagWithoutPositional(t *testing.T) {
	cmd := newProviderCmd(t, "--dseq", "777")

	scope, err := leaseScopeFromCmd(cmd, nil)
	if err != nil {
		t.Fatalf("leaseScopeFromCmd: %v", err)
	}
	if scope.DSeq != 777 {
		t.Errorf("dseq = %d, want 777 from --dseq", scope.DSeq)
	}
}

// TestLeaseScopeRejectsMissingDSeq covers the guard that stops a lease call
// from being made against dseq 0, which the provider would answer with a
// generic not-found. The remedy must name --dseq here, because the command
// under test registers it.
func TestLeaseScopeRejectsMissingDSeq(t *testing.T) {
	cmd := newProviderCmd(t)

	if _, err := leaseScopeFromCmd(cmd, nil); err == nil {
		t.Fatal("dseq 0 must be rejected")
	} else if !strings.Contains(err.Error(), "dseq is required") {
		t.Errorf("error should say dseq is required, got %q", err)
	} else if !strings.Contains(err.Error(), "--dseq") {
		t.Errorf("error should name the --dseq flag this command registers, got %q", err)
	}
}

// TestLeaseScopeRejectsNonNumericDSeq covers the positional parse error.
func TestLeaseScopeRejectsNonNumericDSeq(t *testing.T) {
	cmd := newProviderCmd(t)

	if _, err := leaseScopeFromCmd(cmd, []string{"twelve"}); err == nil {
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
	cmd.Flags().String(flagdefs.FlagOutput, "yaml", "")
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

func TestLeaseShellRejectsStructuredInteractiveModeBeforeProviderResolution(t *testing.T) {
	cmd := leaseShellCmd()
	cmd.Flags().String(flagdefs.FlagOutput, "json", "")

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "explicit remote command") {
		t.Fatalf("lease-shell error = %v", err)
	}
	if strings.Contains(err.Error(), "provider address") {
		t.Fatalf("lease-shell reached provider resolution before refusal: %v", err)
	}
}

func TestLeaseShellReadsCanonicalExecutionFlags(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, "stop after lease-shell flags", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	root := Commands()
	root.PersistentFlags().StringP(flagdefs.FlagOutput, "o", "pretty", "output format")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"lease-shell",
		"--" + flagdefs.FlagDSeq, "12345",
		"--" + flagdefs.FlagProvider, testProviderAddr,
		"--" + flagdefs.FlagProviderURL, srv.URL,
		"--" + flagdefs.FlagService, "web",
		"--" + flagdefs.FlagTTY + "=false",
		"--" + flagdefs.FlagStdin,
		"--", "echo", "ready",
	})

	err := root.ExecuteContext(newAttestationCommandContext(t))
	if err == nil {
		t.Fatal("lease-shell unexpectedly succeeded")
	}
	if requests == 0 {
		t.Fatalf("lease-shell did not reach the gateway after reading its flags: %v", err)
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
		"status", "lease-status", "lease-attestation", "lease-logs", "lease-events",
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
		"lease-status": true, "lease-attestation": true, "lease-logs": true,
		"lease-events": true, "get-manifest": true,
	}

	for _, sub := range cmd.Commands() {
		switch {
		case positional[sub.Name()]:
			if sub.Flags().Lookup(flagdefs.FlagDSeq) != nil {
				t.Errorf("%s must take dseq positionally, not via --dseq", sub.Name())
			}
			if sub.Flags().Lookup(flagdefs.FlagGSeq) == nil || sub.Flags().Lookup(flagdefs.FlagOSeq) == nil {
				t.Errorf("%s should still expose --gseq/--oseq", sub.Name())
			}

		case sub.Name() == "lease-shell":
			// lease-shell consumes its positional args as the remote command,
			// so it keeps --dseq.
			if sub.Flags().Lookup(flagdefs.FlagDSeq) == nil {
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

func TestStatusUsesPublicGatewayWithoutWallet(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want no provider status credentials", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"status", testProviderAddr,
		"--provider-url", srv.URL,
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("walletless provider status: %v", err)
	}
	if requests != 1 {
		t.Fatalf("provider status requests = %d, want 1", requests)
	}
}

func TestStatusTimeoutConfiguresGatewayBoundary(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-r.Context().Done()
	}))
	defer srv.Close()

	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"status", testProviderAddr,
		"--provider-url", srv.URL,
		"--timeout", "40ms",
	})

	started := time.Now()
	err := root.Execute()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("provider status timeout error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("provider status timeout took %s, want the configured deadline", elapsed)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("provider status requests = %d, want 1", got)
	}
}

func TestStatusRejectsNonPositiveTimeoutBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer srv.Close()

	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"status", testProviderAddr,
		"--provider-url", srv.URL,
		"--timeout", "0s",
	})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--timeout must be greater than zero") {
		t.Fatalf("provider status timeout error = %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("invalid timeout made %d requests, want 0", got)
	}
}

func TestStatusRejectsAuthenticationFlag(t *testing.T) {
	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"status", testProviderAddr,
		"--provider-url", "https://gw.example",
		"--auth-type", "jwt",
	})

	if err := root.Execute(); err == nil {
		t.Fatal("--auth-type on public provider status must be rejected")
	} else if !strings.Contains(err.Error(), "does not apply") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStatusFailureIncludesExactChainFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "offline", http.StatusBadGateway)
	}))
	defer srv.Close()

	const provider = "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh"
	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"status", provider, "--provider-url", srv.URL})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "akt query provider "+provider) {
		t.Fatalf("provider status error = %v", err)
	}
}

func TestAuthenticatedGatewayRequiresDefaultAccountBeforeRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"lease-status", "1",
		"--provider", testProviderAddr,
		"--provider-url", srv.URL,
	})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "configured default account") {
		t.Fatalf("error = %v, want a configured default account remedy", err)
	}
	if requests != 0 {
		t.Fatalf("protected gateway requests = %d, want failure before network access", requests)
	}
}

func TestAuthenticatedGatewayPreflightRunsBeforeProviderLookup(t *testing.T) {
	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"lease-status", "1",
		"--provider", testProviderAddr,
	})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "configured default account") {
		t.Fatalf("error = %v, want local signing identity before provider lookup", err)
	}
	if strings.Contains(err.Error(), "initialize provider query client") ||
		strings.Contains(err.Error(), "query provider") {
		t.Fatalf("provider discovery hid authentication preflight: %v", err)
	}
}

func TestAuthenticatedGatewayRejectsUnknownAuthTypeBeforeIdentityChecks(t *testing.T) {
	root := Commands()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"lease-status", "1",
		"--provider", testProviderAddr,
		"--provider-url", "https://gw.example",
		"--auth-type", "bogus",
	})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unsupported auth type") {
		t.Fatalf("error = %v, want unsupported auth type", err)
	}
}

// TestLeaseCommandsBlameTheMissingDSeqNotTheProvider walks every lease-scoped
// subcommand invoked with neither a deployment sequence nor a provider. The
// deployment sequence is what the provider is resolved from, so it must be the
// value the error names, and the remedy must name the form this particular
// command actually accepts. The previous behavior reported "provider address
// is required (positional argument or --provider flag)" on all eight, which
// was doubly wrong: none of them takes a positional provider, and on four of
// them the positional slot is the dseq, so following the hint produced a
// second, more confusing parse error.
func TestLeaseCommandsBlameTheMissingDSeqNotTheProvider(t *testing.T) {
	cases := []struct {
		args   []string
		remedy string
	}{
		{[]string{"lease-status"}, "positional argument"},
		{[]string{"lease-logs"}, "positional argument"},
		{[]string{"lease-events"}, "positional argument"},
		{[]string{"get-manifest"}, "positional argument"},
		{[]string{"lease-shell", "--", "/bin/sh"}, "--dseq"},
		{[]string{"send-manifest", "deploy.yaml"}, "--dseq"},
		{[]string{"migrate-hostnames", "--hostnames", "a.example.com"}, "--dseq"},
		{[]string{"migrate-endpoints", "--endpoints", "ep1"}, "--dseq"},
	}

	for _, tc := range cases {
		root := Commands()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(tc.args)

		err := root.Execute()
		if err == nil {
			t.Errorf("%v without a dseq must fail", tc.args)
			continue
		}
		if !strings.Contains(err.Error(), "dseq is required") {
			t.Errorf("%v: error should name the missing dseq, got %v", tc.args, err)
		}
		if !strings.Contains(err.Error(), tc.remedy) {
			t.Errorf("%v: error should offer %q, got %v", tc.args, tc.remedy, err)
		}
		if strings.Contains(err.Error(), "provider address is required") {
			t.Errorf("%v: error still blames the provider: %v", tc.args, err)
		}
	}
}

// TestLeaseCommandsNeverAdvertiseAPositionalProvider pins the honesty of the
// lease-command errors: not one of them accepts a provider positionally, so
// none may suggest it.
func TestLeaseCommandsNeverAdvertiseAPositionalProvider(t *testing.T) {
	cases := [][]string{
		{"lease-status", "1"},
		{"lease-logs", "1"},
		{"lease-events", "1"},
		{"get-manifest", "1"},
		{"lease-shell", "--dseq", "1", "--", "/bin/sh"},
		{"send-manifest", "deploy.yaml", "--dseq", "1"},
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
			t.Errorf("%v without a signing identity must fail", args)
			continue
		}
		// With a dseq present the next guard is the local signing identity,
		// which every one of these needs before any provider work.
		if !strings.Contains(err.Error(), "configured default account") {
			t.Errorf("%v: error = %v, want the signing-identity remedy", args, err)
		}
		if strings.Contains(err.Error(), "positional argument or --provider") {
			t.Errorf("%v: error suggests a positional provider that does not exist: %v", args, err)
		}
	}
}
