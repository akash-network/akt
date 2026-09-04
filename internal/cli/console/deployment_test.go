package console

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	consoleapi "pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
)

func TestConfirmDeploymentCreateDefaultsToNo(t *testing.T) {
	cmd := &cobra.Command{}
	var diagnostics bytes.Buffer
	cmd.SetErr(&diagnostics)

	cmd.SetIn(strings.NewReader("\n"))
	require.ErrorContains(t, confirmDeploymentCreate(cmd, true, "deploy.yaml"), "cancelled")
	assert.Contains(t, diagnostics.String(), "Create deployment")

	cmd.SetIn(strings.NewReader("yes\n"))
	require.NoError(t, confirmDeploymentCreate(cmd, true, "deploy.yaml"))
	require.NoError(t, confirmDeploymentCreate(cmd, false, "deploy.yaml"))
}

func TestConfirmDeploymentCreateReportsPromptAndReadFailures(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetErr(consoleOutputErrorWriter{err: assert.AnError})
	cmd.SetIn(strings.NewReader("yes\n"))
	require.ErrorIs(t, confirmDeploymentCreate(cmd, true, "deploy.yaml"), assert.AnError)

	cmd.SetErr(io.Discard)
	cmd.SetIn(strings.NewReader(""))
	require.ErrorContains(t, confirmDeploymentCreate(cmd, true, "deploy.yaml"), "read deployment confirmation")
}

func TestDeploymentCreateReturnsInteractiveCancellation(t *testing.T) {
	cmd := deploymentCreateCmdWithTerminal(func() *aktctx.Manager { return nil }, func(int) bool { return true })
	cmd.Flags().String(flagdefs.FlagConsoleAPIKey, "sekrit", "")
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"deploy.yaml"})

	require.ErrorContains(t, cmd.Execute(), "deployment creation cancelled")
}

func TestDeploymentListStateCommandValidationAndFiltering(t *testing.T) {
	m := newAuthedManager(t)
	if _, err := execConsole(t, m, "", "deployment", "list", "pending"); err == nil {
		t.Fatal("invalid deployment state did not fail")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, `{"data":{"deployments":[{"deployment":{"id":{"dseq":"42"},"state":"active"}}],"pagination":{"hasMore":false}}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "deployment", "list", "active", "--limit", "1")
	require.NoError(t, err)
	assert.Contains(t, out, "42")
}

func TestDeploymentSettingsReadPreflightsDeployment(t *testing.T) {
	m := newAuthedManager(t)
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/deployments/42":
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"}}}`)
		case "/v2/deployment-settings/42":
			writeJSON(t, w, `{"data":{"dseq":"42","autoTopUpEnabled":true}}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	_, err := execConsole(t, m, srv.URL, "deployment", "settings", "42")
	require.NoError(t, err)
	assert.Equal(t, []string{"/v1/deployments/42", "/v2/deployment-settings/42"}, paths)
}

func TestDeploymentSettingsReadReturnsDeploymentPreflightFailure(t *testing.T) {
	m := newAuthedManager(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/deployments/404", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := execConsole(t, m, srv.URL, "deployment", "settings", "404")
	require.ErrorContains(t, err, "deployment 404")
}

const validConsoleDeploymentSDL = `version: "2.0"
services:
  web:
    image: nginx:1.27-alpine
    expose:
      - port: 80
        as: 80
        to:
          - global: true
profiles:
  compute:
    web:
      resources:
        cpu:
          units: 0.5
        memory:
          size: 512Mi
        storage:
          size: 512Mi
  placement:
    dcloud:
      pricing:
        web:
          denom: uact
          amount: 10000
deployment:
  web:
    dcloud:
      profile: web
      count: 1
`

// TestDeploymentCreateSendsNoDeposit pins the console rail's funding contract
// on `deployment create`: the request carries only the SDL, and the command
// takes no deposit argument at all.
func TestDeploymentCreateSendsNoDeposit(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	sdlPath := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(sdlPath, []byte(validConsoleDeploymentSDL), 0o600); err != nil {
		t.Fatalf("write SDL file: %v", err)
	}

	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments" {
			writeJSON(t, w, `{"data":{"deployments":[],"pagination":{"hasMore":false}}}`)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/deployments" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		writeJSON(t, w, `{"data":{"dseq":"321","manifest":"","signTx":{"code":0,"transactionHash":"tx-create-321","rawLog":""}}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "deployment", "create", sdlPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if strings.Contains(body, "deposit") {
		t.Errorf("request body = %s, want no deposit field", body)
	}
	if strings.Contains(out, "autoTopUp") {
		t.Errorf("create result = %s, want no auto-top-up notice: it named a command the API now refuses", out)
	}

	// A trailing deposit is no longer a positional this command accepts.
	if _, err := execConsole(t, m, srv.URL, "deployment", "create", sdlPath, "5"); err == nil {
		t.Error("a deposit argument must be rejected, not silently discarded")
	}
}

// TestDeploymentSettingsSetsRuntimeLimit pins the replacement for the retired
// auto-top-up toggle: the positional is a runtime limit in hours, and `none`
// clears it.
func TestDeploymentSettingsSetsRuntimeLimit(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"},"state":"active"}}}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		if strings.Contains(body, `"runtimeLimitHours":null`) {
			writeJSON(t, w, `{"data":{"dseq":"12345","autoTopUpEnabled":true,"runtimeLimitHours":null,"runtimeEndsAt":null}}`)
			return
		}
		writeJSON(t, w, `{"data":{"dseq":"12345","autoTopUpEnabled":true,"runtimeLimitHours":12,"runtimeEndsAt":null}}`)
	}))
	defer srv.Close()

	for _, tc := range []struct{ arg, want string }{
		{"12", `"runtimeLimitHours":12`},
		{"none", `"runtimeLimitHours":null`},
	} {
		body = ""
		if _, err := execConsole(t, m, srv.URL, "deployment", "settings", "12345", tc.arg); err != nil {
			t.Fatalf("settings %q: %v", tc.arg, err)
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("settings %q: request body = %s, want %s", tc.arg, body, tc.want)
		}
	}

	for _, arg := range []string{"0", "-3", "twelve", "true"} {
		if _, err := execConsole(t, m, srv.URL, "deployment", "settings", "12345", arg); err == nil {
			t.Errorf("settings %q: expected a rejection", arg)
		}
	}
}

func TestDeploymentCloseStructuredAcknowledgement(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/deployments/42" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Method == http.MethodGet {
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"}}}`)
			return
		}
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, `{"data":{"success":true}}`)
	}))
	defer srv.Close()

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			out, err := execConsole(t, m, srv.URL, "deployment", "close", "42", "-o", format)
			if err != nil {
				t.Fatalf("close -o %s: %v", format, err)
			}

			got := decodeStructuredMap(t, format, out)
			if len(got) != 2 || got["dseq"] != "42" || got["state"] != "closed" {
				t.Errorf("close acknowledgement = %#v, want dseq=42 state=closed", got)
			}
		})
	}
}

func TestDeploymentCloseRepeatedAttemptFailsWithoutSecondDelete(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/deployments/42" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Method == http.MethodGet {
			state := "active"
			if deletes.Load() > 0 {
				state = "closed"
			}
			fmt.Fprintf(w, `{"data":{"deployment":{"id":{"dseq":"42"},"state":%q}}}`, state)
			return
		}
		deletes.Add(1)
		writeJSON(t, w, `{"data":{"success":true}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "deployment", "close", "42", "-o", "json")
	if err != nil {
		t.Fatalf("first close: %v", err)
	}
	got := decodeStructuredMap(t, "json", out)
	if len(got) != 2 || got["dseq"] != "42" || got["state"] != "closed" {
		t.Errorf("first close acknowledgement = %#v", got)
	}
	if _, err := execConsole(t, m, srv.URL, "deployment", "close", "42", "-o", "json"); !errors.Is(err, consoleapi.ErrAlreadyClosed) {
		t.Fatalf("second close = %v, want ErrAlreadyClosed", err)
	}

	if got := deletes.Load(); got != 1 {
		t.Fatalf("DELETE requests = %d, want 1", got)
	}
}

func TestDeploymentCloseConvergesUniqueLocalDeploymentAndLeases(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	ctx := context.Background()
	s, err := bbolt.OpenContext(ctx, m.Root(), "prod")
	if err != nil {
		t.Fatalf("OpenContext: %v", err)
	}
	owner := "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu"
	if err := s.PutDeployment(ctx, &sstore.DeploymentRecord{Owner: owner, DSeq: 42, State: "active"}); err != nil {
		t.Fatalf("PutDeployment: %v", err)
	}
	leaseID := sstore.LeaseID{Owner: owner, DSeq: 42, GSeq: 1, OSeq: 1, Provider: "akash1provider"}
	if err := s.PutLease(ctx, &sstore.LeaseRecord{ID: leaseID, State: "active"}); err != nil {
		t.Fatalf("PutLease: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/42" {
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"}}}`)
			return
		}
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/deployments/42" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, `{"data":{"success":true}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "deployment", "close", "42"); err != nil {
		t.Fatalf("deployment close: %v", err)
	}

	s, err = bbolt.OpenContext(ctx, m.Root(), "prod")
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = s.Close() }()
	dep, err := s.GetDeployment(ctx, owner, 42)
	if err != nil || dep == nil || dep.State != "closed" || dep.ClosedAt == 0 {
		t.Fatalf("deployment after close = %+v, err %v", dep, err)
	}
	lease, err := s.GetLease(ctx, leaseID)
	if err != nil || lease == nil || lease.State != "closed" || lease.ClosedAt == 0 {
		t.Fatalf("lease after close = %+v, err %v", lease, err)
	}
}

func TestDeploymentCloseDoesNotGuessBetweenLocalOwners(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	ctx := context.Background()
	s, err := bbolt.OpenContext(ctx, m.Root(), "prod")
	if err != nil {
		t.Fatalf("OpenContext: %v", err)
	}
	owners := []string{
		"akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu",
		"akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
	}
	for _, owner := range owners {
		if err := s.PutDeployment(ctx, &sstore.DeploymentRecord{Owner: owner, DSeq: 42, State: "active"}); err != nil {
			t.Fatalf("PutDeployment: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"}}}`)
			return
		}
		writeJSON(t, w, `{"data":{"success":true}}`)
	}))
	defer srv.Close()

	_, stderr, err := execConsoleContextStreams(context.Background(), t, m, srv.URL, "deployment", "close", "42")
	if err != nil {
		t.Fatalf("deployment close: %v", err)
	}
	if !strings.Contains(stderr, "multiple local owners") {
		t.Errorf("ambiguous local close did not warn:\n%s", stderr)
	}

	s, err = bbolt.OpenContext(ctx, m.Root(), "prod")
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = s.Close() }()
	for _, owner := range owners {
		dep, getErr := s.GetDeployment(ctx, owner, 42)
		if getErr != nil || dep == nil || dep.State != "active" {
			t.Fatalf("ambiguous close changed %s: %+v, err %v", owner, dep, getErr)
		}
	}
}

func TestDeploymentCloseAlreadyClosedWarnsWhenLocalStateIsAmbiguous(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	ctx := context.Background()
	s, err := bbolt.OpenContext(ctx, m.Root(), "prod")
	if err != nil {
		t.Fatalf("OpenContext: %v", err)
	}
	for _, owner := range []string{
		"akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu",
		"akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
	} {
		if err := s.PutDeployment(ctx, &sstore.DeploymentRecord{Owner: owner, DSeq: 42, State: "active"}); err != nil {
			t.Fatalf("PutDeployment: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/deployments/42" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, stderr, err := execConsoleContextStreams(ctx, t, m, srv.URL, "deployment", "close", "42")
	if !errors.Is(err, consoleapi.ErrAlreadyClosed) {
		t.Fatalf("deployment close error = %v, want ErrAlreadyClosed", err)
	}
	if !strings.Contains(stderr, "multiple local owners") {
		t.Fatalf("already-closed local convergence warning = %q", stderr)
	}
}

// TestDeploymentListZeroFlagValues pins that skip may be zero but the API's
// one-based page size may not. Both are rejected before a malformed request
// can reach Console.
func TestDeploymentListZeroFlagValues(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		writeJSON(t, w, `{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":20,"hasMore":false}}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "deployment", "list", "--skip", "0", "--limit", "0"); err == nil {
		t.Fatal("deployment list with a zero limit must fail")
	}

	if gotQuery != nil {
		t.Errorf("invalid pagination reached Console with query %v", gotQuery)
	}
}

func TestManifestFromFile(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}

		return p
	}

	t.Run("accepts a rendered manifest", func(t *testing.T) {
		p := write("manifest.json", `[{"name":"dcloud","services":[]}]`)

		got, err := manifestFromFile(p)
		if err != nil {
			t.Fatalf("expected the JSON manifest to be accepted, got %v", err)
		}
		if got == "" {
			t.Fatal("expected the file contents back")
		}
	})

	// The regression: passing the SDL got as far as Console, which replied
	// "invalid character '-' in numeric literal" -- its JSON parser hitting
	// the leading `---`. Nothing about that names the real mistake.
	t.Run("rejects an SDL with a message that names the cause", func(t *testing.T) {
		p := write("deploy.yaml", "---\nversion: \"2.0\"\nservices:\n  web:\n    image: nginx\n")

		_, err := manifestFromFile(p)
		if err == nil {
			t.Fatal("expected an SDL file to be rejected")
		}
		for _, want := range []string{"not JSON", "not the SDL"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should mention %q, got: %v", want, err)
			}
		}
	})

	t.Run("reports a missing file", func(t *testing.T) {
		if _, err := manifestFromFile(filepath.Join(dir, "absent.json")); err == nil {
			t.Fatal("expected a missing file to error")
		}
	})
}
