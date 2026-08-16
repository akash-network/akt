package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestConsoleClientForCarriesMCPActionLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments":
			_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"skip":0,"limit":10,"hasMore":false}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/42":
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"1000000"}],"transferred":[]}}}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/deployments/41":
			_, _ = w.Write([]byte(`{"data":{"success":true}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/deployments/44":
			_, _ = w.Write([]byte(`{"data":{"success":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/42":
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"1000000"}],"transferred":[]}}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/deposit-deployment":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"deployment is settling"}`))
		default:
			t.Errorf("unexpected Console request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	mgr, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.CreateContext(aktctx.Context{
		Name:          "console",
		AuthMethod:    aktctx.AuthMethodConsoleAPI,
		ConsoleAPIURL: srv.URL,
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := mgr.UseContext("console"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	logger, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	cmd := newMCPConsoleClientCommand(cliutil.WithActionLog(context.Background(), logger), t)
	client := consoleClientFor(cmd, func() *aktctx.Manager { return mgr })
	if client == nil {
		t.Fatal("consoleClientFor returned nil")
	}

	if _, err := client.ListDeployments(context.Background(), 0, 10); err != nil {
		t.Fatalf("read-only ListDeployments: %v", err)
	}
	if err := client.CloseDeployment(context.Background(), "41"); err != nil {
		t.Fatalf("CloseDeployment: %v", err)
	}
	if err := client.Deposit(context.Background(), "42", 5); err == nil {
		t.Fatal("Deposit should surface the Console conflict")
	}

	entries, err := logger.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read action log: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("MCP Console mutations logged %d entries, want exactly 2: %+v", len(entries), entries)
	}
	if got := entries[0]; got.Type != actionlog.TypeConsole || got.Action != "deposit" || got.DSeq != 42 || got.Status != "failed" || got.Error == "" {
		t.Errorf("failed deposit entry = %+v", got)
	}
	if got := entries[1]; got.Type != actionlog.TypeConsole || got.Action != "close-deployment" || got.DSeq != 41 || got.Status != "success" {
		t.Errorf("successful close entry = %+v", got)
	}

	// A command context without a logger must preserve mutation behavior and
	// must not accidentally reuse the logger attached to another MCP server.
	unlogged := consoleClientFor(newMCPConsoleClientCommand(context.Background(), t), func() *aktctx.Manager { return mgr })
	if err := unlogged.CloseDeployment(context.Background(), "44"); err != nil {
		t.Fatalf("CloseDeployment without logger: %v", err)
	}
	entries, err = logger.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read action log after nil-logger mutation: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("nil-logger MCP client changed another action log: %+v", entries)
	}
}

func newMCPConsoleClientCommand(ctx context.Context, t *testing.T) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "mcp"}
	cmd.SetContext(ctx)
	cmd.Flags().String(flagdefs.FlagConsoleAPIKey, "", "")
	cmd.Flags().String(flagdefs.FlagContext, "", "")
	if err := cmd.Flags().Set(flagdefs.FlagConsoleAPIKey, "test-key"); err != nil {
		t.Fatalf("set Console API key: %v", err)
	}

	return cmd
}
