package console

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
)

const forbiddenBody = `{"error":"ForbiddenError","message":"Forbidden","code":"forbidden","type":"client_error"}`

func runTeamCommand(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()

	t.Setenv(aktctx.EnvConsoleAPIKey, "")
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	return cmd.Execute()
}

func TestTeamCommandsNeedAnAPIKey(t *testing.T) {
	m := newTeamManager(t, "", "")
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", ""); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}
	mgrFn := func() *aktctx.Manager { return m }

	tests := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{"organization list", organizationCmds(mgrFn), []string{"list"}},
		{"project list", projectCmds(mgrFn), []string{"list"}},
		{"project get", projectCmds(mgrFn), []string{"get", "checkout-api"}},
		{"project create", projectCmds(mgrFn), []string{"create", "payments"}},
		{"project rename", projectCmds(mgrFn), []string{"rename", "checkout-api", "checkout"}},
		{"project delete", projectCmds(mgrFn), []string{"delete", "checkout-api"}},
		{"project use", projectCmds(mgrFn), []string{"use", "checkout-api"}},
		{"deployment move", deploymentMoveCmd(mgrFn), []string{"42", "checkout-api"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runTeamCommand(t, tc.cmd, tc.args...)
			if err == nil || !strings.Contains(err.Error(), "no Console API key configured") {
				t.Fatalf("error = %v, want the missing key remedy", err)
			}
		})
	}
}

func TestProjectUseNeedsAContext(t *testing.T) {
	err := runTeamCommand(t, projectCmds(func() *aktctx.Manager { return nil }), "use", "checkout-api")
	if err == nil || !strings.Contains(err.Error(), "requires a context") {
		t.Fatalf("error without a manager = %v", err)
	}

	empty, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Setenv("AKT_CONTEXT", "")
	err = runTeamCommand(t, projectCmds(func() *aktctx.Manager { return empty }), "use", "checkout-api")
	if err == nil || !strings.Contains(err.Error(), "requires a context to store the selection") {
		t.Fatalf("error without a context = %v", err)
	}
}

func TestTeamCommandsReportConsoleFailures(t *testing.T) {
	m := newTeamManager(t, "", "")
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		switch {
		case r.URL.Path == "/v1/organizations",
			r.Method == http.MethodPatch,
			r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusForbidden)
			writeJSON(t, w, forbiddenBody)
			return true
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects":
			w.WriteHeader(http.StatusConflict)
			writeJSON(t, w, `{"error":"ConflictError","message":"Another project has this name","code":"project_name_taken","type":"client_error"}`)
			return true
		}
		return false
	})

	tests := []struct {
		args []string
		want string
	}{
		{[]string{"organization", "list"}, "list organizations"},
		{[]string{"project", "get", "missing"}, "get project missing"},
		{[]string{"project", "create", "Checkout API"}, "project_name_taken"},
		{[]string{"project", "rename", "missing", "other"}, "rename project missing"},
		{[]string{"project", "rename", "checkout-api", "checkout"}, "rename project checkout-api"},
		{[]string{"project", "delete", "missing"}, "delete project missing"},
		{[]string{"project", "delete", "checkout-api"}, "delete project checkout-api"},
		{[]string{"project", "use", "checkout-api"}, "use project checkout-api"},
		{[]string{"deployment", "move", "42", "missing"}, "move deployment 42"},
		{[]string{"deployment", "move", "42", "default"}, "move deployment 42 to project default"},
		{[]string{"apikey", "create", "ci", "--console-project", "missing"}, `create API key: console: no project named "missing"`},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			_, err := execConsole(t, m, srv.URL, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestProjectListReportsAFailedListing(t *testing.T) {
	m := newTeamManager(t, "", "")
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/v1/projects" {
			return false
		}
		w.WriteHeader(http.StatusForbidden)
		writeJSON(t, w, forbiddenBody)
		return true
	})

	_, err := execConsole(t, m, srv.URL, "project", "list")
	if err == nil || !strings.Contains(err.Error(), "list projects") {
		t.Fatalf("error = %v", err)
	}
}

func TestProjectUseRecordsARejectedSelection(t *testing.T) {
	m := newTeamManager(t, "", "")
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/v1/organizations" {
			return false
		}
		writeJSON(t, w, `{"data":[{"id":"acme team","name":"Acme","slug":"acme","type":"team","role":"member","isActive":true,"hasPaymentMethod":null,"createdAt":"2026-10-01T00:00:00Z"}]}`)
		return true
	})

	logPath := filepath.Join(t.TempDir(), "actions.log")
	logger, err := actionlog.Open(logPath)
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	defer func() { _ = logger.Close() }()

	_, err = execConsoleContext(cliutil.WithActionLog(context.Background(), logger), t, m, srv.URL, "project", "use", "checkout-api")
	if err == nil || !strings.Contains(err.Error(), "without spaces") {
		t.Fatalf("error = %v, want the stored scope validation", err)
	}
	if got := m.GetContext("prod").ConsoleProject; got != "" {
		t.Errorf("a rejected selection must not be stored, got %q", got)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read action log: %v", err)
	}
	if !strings.Contains(string(raw), `"status":"failed"`) {
		t.Errorf("the rejected selection must be recorded as failed, got %s", raw)
	}
}

func TestProjectUseWithoutAnActionLog(t *testing.T) {
	m := newTeamManager(t, "", "")
	srv := newTeamServer(t, nil)

	if out, err := execConsole(t, m, srv.URL, "project", "use", "default"); err != nil {
		t.Fatalf("project use: %v\n%s", err, out)
	}
	if got := m.GetContext("prod").ConsoleProject; got != cliOtherProjectID {
		t.Errorf("stored project = %q", got)
	}
}

func TestWhoamiReportsAnUnreachableSelectedProject(t *testing.T) {
	m := newTeamManager(t, "acme", "11111111-1111-4111-8111-111111111111")
	srv := newTeamServer(t, nil)

	_, err := execConsole(t, m, srv.URL, "whoami")
	if err == nil || !strings.Contains(err.Error(), "resolve the selected project") {
		t.Fatalf("error = %v", err)
	}
}

func TestWhoamiReportsARefusedOrganization(t *testing.T) {
	m := newTeamManager(t, "other-team", "")
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/v1/organizations" {
			return false
		}
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(t, w, `{"error":"BadRequestError","message":"The organization is not the API key's","code":"organization_mismatch","type":"client_error"}`)
		return true
	})

	_, err := execConsole(t, m, srv.URL, "whoami")
	if err == nil || !strings.Contains(err.Error(), "get organization") || !strings.Contains(err.Error(), "organization_mismatch") {
		t.Fatalf("error = %v", err)
	}
}

func TestAPIKeyListShowsTheScopeOfEachKey(t *testing.T) {
	m := newTeamManager(t, "acme", cliProjectID)
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/v1/api-keys" {
			return false
		}
		writeJSON(t, w, `{"data":[
			{"id":"8a7b6c5d-4e3f-4a2b-9c1d-0e9f8a7b6c5d","name":"ci","keyFormat":"ac.sk.production.abc","createdAt":"2026-10-01T00:00:00Z","updatedAt":"2026-10-01T00:00:00Z","expiresAt":null,"lastUsedAt":null,"organizationId":"`+cliOrganizationID+`","projectId":"`+cliProjectID+`"},
			{"id":"9b8c7d6e-5f4a-4b3c-8d2e-1f0a9b8c7d6e","name":"team","keyFormat":"ac.sk.production.def","createdAt":"2026-10-01T00:00:00Z","updatedAt":"2026-10-01T00:00:00Z","expiresAt":null,"lastUsedAt":null,"organizationId":"`+cliOrganizationID+`","projectId":null}
		]}`)
		return true
	})

	out, err := execConsole(t, m, srv.URL, "apikey", "list")
	if err != nil {
		t.Fatalf("apikey list: %v", err)
	}
	if !strings.Contains(out, `"projectId": "`+cliProjectID+`"`) || strings.Count(out, `"organizationId": "`+cliOrganizationID+`"`) != 2 {
		t.Errorf("apikey list = %s", out)
	}
	if lists := srv.find(http.MethodGet, "/v1/api-keys"); len(lists) != 1 || lists[0].project != "" {
		t.Errorf("listing keys must not be narrowed to the selected project, got %+v", lists)
	}
}
