package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
	consoleapi "pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
)

const (
	cliOrganizationID = "0b8052e2-6c1b-4b9a-9f3e-1d2c3b4a5f60"
	cliProjectID      = "7d4c3b2a-1e0f-4a9b-8c7d-6e5f4a3b2c1d"
	cliOtherProjectID = "3a2b1c0d-9e8f-4a7b-8c6d-5e4f3a2b1c0d"
)

const cliOrganizationsBody = `{"data":[{"id":"` + cliOrganizationID + `","name":"Acme","slug":"acme","type":"team","role":"member","isActive":true,"hasPaymentMethod":null,"createdAt":"2026-10-01T00:00:00Z"}]}`

const cliProjectsBody = `{"data":[
	{"id":"` + cliProjectID + `","name":"Checkout API","slug":"checkout-api","description":null,"isDefault":false,"createdAt":"2026-10-01T00:00:00Z","createdBy":null,"lastActivityAt":"2026-10-02T00:00:00Z","deployments":null,"access":{"canDeploy":true,"canManage":false}},
	{"id":"` + cliOtherProjectID + `","name":"default","slug":"default","description":null,"isDefault":true,"createdAt":"2026-09-01T00:00:00Z","createdBy":null,"lastActivityAt":"2026-09-02T00:00:00Z","deployments":null,"access":{"canDeploy":true,"canManage":true}}
]}`

// teamServer answers the team routes and records every request it saw.
type teamServer struct {
	*httptest.Server

	mu       sync.Mutex
	requests []teamRequest
	handle   func(w http.ResponseWriter, r *http.Request, body map[string]any) bool
}

type teamRequest struct {
	method       string
	path         string
	query        string
	organization string
	project      string
	body         map[string]any
}

func newTeamServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, body map[string]any) bool) *teamServer {
	t.Helper()

	s := &teamServer{handle: handle}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&envelope)

		s.mu.Lock()
		s.requests = append(s.requests, teamRequest{
			method:       r.Method,
			path:         r.URL.Path,
			query:        r.URL.RawQuery,
			organization: r.Header.Get(consoleapi.HeaderOrganizationID),
			project:      r.Header.Get(consoleapi.HeaderProjectID),
			body:         envelope.Data,
		})
		s.mu.Unlock()

		if s.handle != nil && s.handle(w, r, envelope.Data) {
			return
		}
		switch r.URL.Path {
		case "/v1/organizations":
			writeJSON(t, w, cliOrganizationsBody)
		case "/v1/projects":
			writeJSON(t, w, cliProjectsBody)
		case "/v1/user/me":
			writeJSON(t, w, userBody)
		case "/v1/deployments":
			writeJSON(t, w, `{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":20,"hasMore":false}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)

	return s
}

func (s *teamServer) find(method, path string) []teamRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []teamRequest
	for _, request := range s.requests {
		if request.method == method && request.path == path {
			out = append(out, request)
		}
	}

	return out
}

func newTeamManager(t *testing.T, organization, project string) *aktctx.Manager {
	t.Helper()

	t.Setenv(aktctx.EnvConsoleOrganization, "")
	t.Setenv(aktctx.EnvConsoleProject, "")
	m := newAuthedManager(t)
	if organization != "" || project != "" {
		if err := m.UpdateContext("prod", func(c *aktctx.Context) error {
			c.ConsoleOrganization = organization
			c.ConsoleProject = project
			return nil
		}); err != nil {
			t.Fatalf("UpdateContext: %v", err)
		}
	}

	return m
}

func TestDeploymentListActsInTheContextProject(t *testing.T) {
	m := newTeamManager(t, "acme", cliProjectID)
	srv := newTeamServer(t, nil)

	if _, err := execConsole(t, m, srv.URL, "deployment", "list"); err != nil {
		t.Fatalf("deployment list: %v", err)
	}

	lists := srv.find(http.MethodGet, "/v1/deployments")
	if len(lists) != 1 || lists[0].organization != "acme" || lists[0].project != cliProjectID || !strings.Contains(lists[0].query, "projectId="+cliProjectID) {
		t.Fatalf("deployment list requests = %+v", lists)
	}
}

func TestProjectFlagOverridesTheContextForOneCommand(t *testing.T) {
	m := newTeamManager(t, "acme", cliProjectID)
	srv := newTeamServer(t, nil)

	if _, err := execConsole(t, m, srv.URL, "deployment", "list", "--console-project", "default"); err != nil {
		t.Fatalf("deployment list: %v", err)
	}
	if _, err := execConsole(t, m, srv.URL, "deployment", "list", "--console-project="); err != nil {
		t.Fatalf("deployment list: %v", err)
	}

	lists := srv.find(http.MethodGet, "/v1/deployments")
	if len(lists) != 2 {
		t.Fatalf("deployment list requests = %+v", lists)
	}
	if lists[0].project != cliOtherProjectID {
		t.Errorf("--console-project default sent project %q, want %q", lists[0].project, cliOtherProjectID)
	}
	if lists[1].project != "" || strings.Contains(lists[1].query, "projectId") {
		t.Errorf("an empty --console-project must act across every project, got %+v", lists[1])
	}
	if got := m.GetContext("prod").ConsoleProject; got != cliProjectID {
		t.Errorf("a flag must not change the stored selection, got %q", got)
	}
}

func TestProjectEnvironmentOverridesTheContext(t *testing.T) {
	m := newTeamManager(t, "acme", cliProjectID)
	t.Setenv(aktctx.EnvConsoleProject, cliOtherProjectID)
	srv := newTeamServer(t, nil)

	if _, err := execConsole(t, m, srv.URL, "deployment", "list"); err != nil {
		t.Fatalf("deployment list: %v", err)
	}
	if lists := srv.find(http.MethodGet, "/v1/deployments"); len(lists) != 1 || lists[0].project != cliOtherProjectID {
		t.Fatalf("deployment list requests = %+v", lists)
	}
}

func TestPublicCatalogCommandsSendNoScope(t *testing.T) {
	m := newTeamManager(t, "acme", "")
	t.Setenv(aktctx.EnvConsoleProject, "checkout-api")
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/v1/providers" {
			return false
		}
		writeJSON(t, w, `[]`)
		return true
	})

	if out, err := execConsole(t, m, srv.URL, "provider", "list"); err != nil {
		t.Fatalf("provider list: %v\n%s", err, out)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.requests) != 1 || srv.requests[0].path != "/v1/providers" || srv.requests[0].organization != "" || srv.requests[0].project != "" {
		t.Fatalf("a public catalog command must not resolve or send a scope, got %+v", srv.requests)
	}
}

func TestContextWithoutTeamsSendsNoScope(t *testing.T) {
	m := newTeamManager(t, "", "")
	srv := newTeamServer(t, nil)

	if _, err := execConsole(t, m, srv.URL, "deployment", "list"); err != nil {
		t.Fatalf("deployment list: %v", err)
	}
	for _, request := range srv.find(http.MethodGet, "/v1/deployments") {
		if request.organization != "" || request.project != "" || strings.Contains(request.query, "projectId") {
			t.Errorf("a context without teams must send no scope, got %+v", request)
		}
	}
}

func TestProjectUseStoresTheProjectAndItsOrganization(t *testing.T) {
	m := newTeamManager(t, "", "")
	srv := newTeamServer(t, nil)

	logPath := filepath.Join(t.TempDir(), "actions.log")
	logger, err := actionlog.Open(logPath)
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	defer func() { _ = logger.Close() }()

	out, err := execConsoleContext(cliutil.WithActionLog(context.Background(), logger), t, m, srv.URL, "project", "use", "checkout-api")
	if err != nil {
		t.Fatalf("project use: %v\n%s", err, out)
	}

	stored := m.GetContext("prod")
	if stored.ConsoleProject != cliProjectID || stored.ConsoleOrganization != cliOrganizationID {
		t.Fatalf("stored scope = %q/%q", stored.ConsoleOrganization, stored.ConsoleProject)
	}
	ack := decodeStructuredMap(t, "json", out)
	if ack["projectName"] != "Checkout API" || ack["organizationName"] != "Acme" || ack["context"] != "prod" {
		t.Errorf("acknowledgement = %#v", ack)
	}

	organizations := srv.find(http.MethodGet, "/v1/organizations")
	if len(organizations) != 1 || organizations[0].project != cliProjectID {
		t.Errorf("the organization lookup must run in the chosen project, got %+v", organizations)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read action log: %v", err)
	}
	if !strings.Contains(string(raw), `"type":"context"`) || !strings.Contains(string(raw), cliProjectID) {
		t.Errorf("project use must record a context change, got %s", raw)
	}
}

func TestProjectUseRejectsAnUnknownProjectWithoutChangingTheContext(t *testing.T) {
	m := newTeamManager(t, "", "")
	srv := newTeamServer(t, nil)

	_, err := execConsole(t, m, srv.URL, "project", "use", "missing")
	if err == nil || !strings.Contains(err.Error(), `no project named "missing"`) {
		t.Fatalf("project use error = %v", err)
	}
	if got := m.GetContext("prod").ConsoleProject; got != "" {
		t.Errorf("stored project = %q, want unchanged", got)
	}
}

func TestProjectManagementIsNotNarrowedToTheSelectedProject(t *testing.T) {
	m := newTeamManager(t, "acme", cliProjectID)
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) bool {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects":
			w.WriteHeader(http.StatusCreated)
			writeJSON(t, w, `{"data":{"id":"`+cliOtherProjectID+`","name":"`+body["name"].(string)+`","slug":"payments","description":null,"isDefault":false,"createdAt":"2026-10-01T00:00:00Z","createdBy":null}}`)
			return true
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/projects/"+cliProjectID:
			writeJSON(t, w, `{"data":{"id":"`+cliProjectID+`","name":"`+body["name"].(string)+`","slug":"checkout-api","description":null,"isDefault":false,"createdAt":"2026-10-01T00:00:00Z","createdBy":null}}`)
			return true
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/projects/"+cliProjectID:
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		return false
	})

	for _, args := range [][]string{
		{"project", "list"},
		{"organization", "list"},
		{"team", "list"},
		{"project", "get", "checkout-api"},
		{"project", "create", "payments", "Card payments"},
		{"project", "rename", "Checkout API", "checkout"},
		{"project", "delete", "checkout-api"},
	} {
		if out, err := execConsole(t, m, srv.URL, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, request := range srv.requests {
		if request.project != "" {
			t.Errorf("%s %s sent x-project-id %q", request.method, request.path, request.project)
		}
		if request.organization != "acme" {
			t.Errorf("%s %s sent x-organization-id %q, want the context's", request.method, request.path, request.organization)
		}
	}
}

func TestProjectDeleteAcknowledgesTheProject(t *testing.T) {
	m := newTeamManager(t, "", "")
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		return false
	})

	out, err := execConsole(t, m, srv.URL, "project", "delete", "checkout-api")
	if err != nil {
		t.Fatalf("project delete: %v", err)
	}
	ack := decodeStructuredMap(t, "json", out)
	deleted, _ := ack["deleted"].(bool)
	if ack["id"] != cliProjectID || !deleted {
		t.Errorf("acknowledgement = %#v", ack)
	}
}

func TestDeploymentMoveResolvesTheTargetProject(t *testing.T) {
	m := newTeamManager(t, "acme", cliProjectID)
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) bool {
		if r.Method != http.MethodPatch {
			return false
		}
		writeJSON(t, w, `{"data":{"dseq":"42","projectId":"`+body["projectId"].(string)+`"}}`)
		return true
	})

	out, err := execConsole(t, m, srv.URL, "deployment", "move", "42", "default")
	if err != nil {
		t.Fatalf("deployment move: %v", err)
	}

	moves := srv.find(http.MethodPatch, "/v1/deployments/42/project")
	if len(moves) != 1 || moves[0].body["projectId"] != cliOtherProjectID || moves[0].project != "" {
		t.Fatalf("move requests = %+v", moves)
	}
	ack := decodeStructuredMap(t, "json", out)
	if ack["dseq"] != "42" || ack["projectId"] != cliOtherProjectID {
		t.Errorf("acknowledgement = %#v", ack)
	}
}

func TestWhoamiShowsTheOrganizationRoleAndProject(t *testing.T) {
	m := newTeamManager(t, "acme", cliProjectID)
	srv := newTeamServer(t, nil)

	out, err := execConsole(t, m, srv.URL, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	result := decodeStructuredMap(t, "json", out)
	organization, _ := result["organization"].(map[string]any)
	project, _ := result["project"].(map[string]any)
	if result["username"] != "max" || organization["role"] != "member" || organization["slug"] != "acme" || project["name"] != "Checkout API" {
		t.Fatalf("whoami = %#v", result)
	}

	m = newTeamManager(t, "", "")
	out, err = execConsole(t, m, srv.URL, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	result = decodeStructuredMap(t, "json", out)
	if _, ok := result["project"]; !ok || result["project"] != nil {
		t.Errorf("without a selected project whoami reports project null, got %#v", result)
	}

	pretty, err := execConsole(t, m, srv.URL, "whoami", "-o", "pretty")
	if err != nil {
		t.Fatalf("whoami pretty: %v", err)
	}
	if !strings.Contains(pretty, "Role: member") || !strings.Contains(pretty, "Project: none") {
		t.Errorf("pretty whoami = %q", pretty)
	}
}

func TestAPIKeyCreateBindsOnlyAnExplicitProject(t *testing.T) {
	m := newTeamManager(t, "acme", cliProjectID)
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) bool {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/api-keys" {
			return false
		}
		project := "null"
		if id, ok := body["projectId"].(string); ok {
			project = `"` + id + `"`
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, `{"data":{"id":"key-1","name":"`+body["name"].(string)+`","apiKey":"ak-one-time","organizationId":"`+cliOrganizationID+`","projectId":`+project+`}}`)
		return true
	})

	out, err := execConsole(t, m, srv.URL, "apikey", "create", "team-wide")
	if err != nil {
		t.Fatalf("apikey create: %v", err)
	}
	if strings.Contains(out, `"projectId"`) {
		t.Errorf("a key created without --console-project must reach every project, got %s", out)
	}

	out, err = execConsole(t, m, srv.URL, "apikey", "create", "checkout-ci", "--console-project", "default")
	if err != nil {
		t.Fatalf("apikey create: %v", err)
	}
	if !strings.Contains(out, `"projectId": "`+cliOtherProjectID+`"`) {
		t.Errorf("bound key output = %s", out)
	}

	creates := srv.find(http.MethodPost, "/v1/api-keys")
	if len(creates) != 2 {
		t.Fatalf("create requests = %+v", creates)
	}
	if _, bound := creates[0].body["projectId"]; bound || creates[0].project != "" {
		t.Errorf("the context's project must not bind or narrow a new key, got %+v", creates[0])
	}
	if creates[1].body["projectId"] != cliOtherProjectID || creates[1].project != "" {
		t.Errorf("an explicit --console-project binds the key in the body only, got %+v", creates[1])
	}
}

func TestTeamRefusalReachesTheUserWithItsRemedy(t *testing.T) {
	m := newTeamManager(t, "acme", cliProjectID)
	srv := newTeamServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/v1/deployments" {
			return false
		}
		w.WriteHeader(http.StatusForbidden)
		writeJSON(t, w, `{"error":"ForbiddenError","message":"The project is not reachable in this organization","code":"project_forbidden","type":"client_error"}`)
		return true
	})

	_, err := execConsole(t, m, srv.URL, "deployment", "list")
	if err == nil {
		t.Fatal("deployment list succeeded, want the refusal")
	}
	for _, want := range []string{"project_forbidden", "akt console project list", "--console-project"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}
