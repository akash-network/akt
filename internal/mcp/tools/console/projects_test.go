package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	aktconsole "pkg.akt.dev/akt/internal/console"
)

const (
	mcpProjectID      = "7d4c3b2a-1e0f-4a9b-8c7d-6e5f4a3b2c1d"
	mcpOtherProjectID = "3a2b1c0d-9e8f-4a7b-8c6d-5e4f3a2b1c0d"
)

type scopedCall struct {
	method  string
	path    string
	query   string
	project string
}

func newProjectServer(t *testing.T) (*httptest.Server, func() []scopedCall) {
	t.Helper()

	var (
		mu    sync.Mutex
		calls []scopedCall
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, scopedCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, project: r.Header.Get(aktconsole.HeaderProjectID)})
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/organizations":
			_, _ = w.Write([]byte(`{"data":[{"id":"0b8052e2-6c1b-4b9a-9f3e-1d2c3b4a5f60","name":"Acme","slug":"acme","type":"team","role":"admin","isActive":true,"hasPaymentMethod":true,"createdAt":"2026-10-01T00:00:00Z"}]}`))
		case r.URL.Path == "/v1/projects":
			_, _ = w.Write([]byte(`{"data":[
				{"id":"` + mcpProjectID + `","name":"Checkout API","slug":"checkout-api","description":null,"isDefault":false,"createdAt":"2026-10-01T00:00:00Z","createdBy":null,"lastActivityAt":null,"deployments":null,"access":{"canDeploy":true,"canManage":true}},
				{"id":"` + mcpOtherProjectID + `","name":"default","slug":"default","description":null,"isDefault":true,"createdAt":"2026-09-01T00:00:00Z","createdBy":null,"lastActivityAt":null,"deployments":null,"access":{"canDeploy":true,"canManage":true}}
			]}`))
		case r.URL.Path == "/v1/deployments" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":20,"hasMore":false}}}`))
		case r.URL.Path == "/v1/bids":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.URL.Path == "/v1/deployments/42" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"}}}`))
		case r.URL.Path == "/v1/deployments/42" && r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"data":{"success":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return srv, func() []scopedCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]scopedCall(nil), calls...)
	}
}

func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}

	return result
}

func lastCall(t *testing.T, calls []scopedCall, method, path string) scopedCall {
	t.Helper()

	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method == method && calls[i].path == path {
			return calls[i]
		}
	}
	t.Fatalf("no %s %s in %+v", method, path, calls)

	return scopedCall{}
}

func TestDeploymentToolsActInTheNamedProject(t *testing.T) {
	srv, calls := newProjectServer(t)
	client := aktconsole.New(srv.URL, "test-key").WithScope("acme", mcpOtherProjectID)

	if result := callTool(t, HandleListDeployments(client), map[string]any{"project": "Checkout API"}); result.IsError {
		t.Fatalf("list deployments: %s", consoleResultText(t, result))
	}
	list := lastCall(t, calls(), http.MethodGet, "/v1/deployments")
	if list.project != mcpProjectID || !strings.Contains(list.query, "projectId="+mcpProjectID) {
		t.Fatalf("a named project must replace the server's, got %+v", list)
	}

	if result := callTool(t, HandleCloseDeployment(client), map[string]any{"dseq": "42", "project": "checkout-api"}); result.IsError {
		t.Fatalf("close deployment: %s", consoleResultText(t, result))
	}
	if closed := lastCall(t, calls(), http.MethodDelete, "/v1/deployments/42"); closed.project != mcpProjectID {
		t.Fatalf("close sent project %q", closed.project)
	}
}

func TestDeploymentToolsDefaultToTheServerProject(t *testing.T) {
	srv, calls := newProjectServer(t)
	client := aktconsole.New(srv.URL, "test-key").WithScope("acme", mcpOtherProjectID)

	if result := callTool(t, HandleListDeployments(client), map[string]any{"project": "  "}); result.IsError {
		t.Fatalf("list deployments: %s", consoleResultText(t, result))
	}
	if list := lastCall(t, calls(), http.MethodGet, "/v1/deployments"); list.project != mcpOtherProjectID {
		t.Fatalf("an omitted project must use the server's, got %+v", list)
	}
}

func TestDeploymentToolsReportAnUnknownProject(t *testing.T) {
	srv, calls := newProjectServer(t)
	client := aktconsole.New(srv.URL, "test-key")

	result := callTool(t, HandleListDeployments(client), map[string]any{"project": "missing"})
	if !result.IsError || !strings.Contains(consoleResultText(t, result), `no project named "missing"`) {
		t.Fatalf("result = %s", consoleResultText(t, result))
	}
	for _, call := range calls() {
		if call.path == "/v1/deployments" {
			t.Fatalf("an unknown project must not list deployments, got %+v", call)
		}
	}
}

func TestTeamToolsListTheOrganizationAndItsProjects(t *testing.T) {
	srv, calls := newProjectServer(t)
	client := aktconsole.New(srv.URL, "test-key").WithScope("acme", mcpProjectID)

	organizations := callTool(t, HandleListOrganizations(client), nil)
	if organizations.IsError || !strings.Contains(consoleResultText(t, organizations), `"role": "admin"`) {
		t.Fatalf("organizations = %s", consoleResultText(t, organizations))
	}

	projects := callTool(t, HandleListProjects(client), nil)
	text := consoleResultText(t, projects)
	if projects.IsError || !strings.Contains(text, "checkout-api") || !strings.Contains(text, mcpOtherProjectID) {
		t.Fatalf("projects = %s", text)
	}
	if list := lastCall(t, calls(), http.MethodGet, "/v1/projects"); list.project != "" {
		t.Fatalf("listing projects must not be narrowed to the selected one, got %+v", list)
	}
}

func TestTeamToolsReportTeamsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	client := aktconsole.New(srv.URL, "test-key")

	for name, handler := range map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"organizations": HandleListOrganizations(client),
		"projects":      HandleListProjects(client),
	} {
		result := callTool(t, handler, nil)
		if !result.IsError || !strings.Contains(consoleResultText(t, result), "teams") {
			t.Errorf("%s result = %s", name, consoleResultText(t, result))
		}
	}
}

func TestDeploymentReadToolsActInTheNamedProject(t *testing.T) {
	srv, calls := newProjectServer(t)
	client := aktconsole.New(srv.URL, "test-key")

	if result := callTool(t, HandleGetDeployment(client), map[string]any{"dseq": "42", "project": "checkout-api"}); result.IsError {
		t.Fatalf("get deployment: %s", consoleResultText(t, result))
	}
	if result := callTool(t, HandleListBids(client), map[string]any{"dseq": "42", "project": "checkout-api"}); result.IsError {
		t.Fatalf("list bids: %s", consoleResultText(t, result))
	}
	for _, path := range []string{"/v1/deployments/42", "/v1/bids"} {
		if call := lastCall(t, calls(), http.MethodGet, path); call.project != mcpProjectID {
			t.Errorf("GET %s sent project %q", path, call.project)
		}
	}
}

func TestDeploymentToolsReportAnAbsentDeployment(t *testing.T) {
	srv, _ := newProjectServer(t)
	client := aktconsole.New(srv.URL, "test-key")

	bids := callTool(t, HandleListBids(client), map[string]any{"dseq": "43", "project": "checkout-api"})
	if !bids.IsError || !strings.Contains(consoleResultText(t, bids), "no bids for 43") {
		t.Errorf("list bids = %s", consoleResultText(t, bids))
	}

	closed := callTool(t, HandleCloseDeployment(client), map[string]any{"dseq": "43", "project": "checkout-api"})
	if !closed.IsError || !strings.Contains(consoleResultText(t, closed), "failed to close deployment 43") {
		t.Errorf("close deployment = %s", consoleResultText(t, closed))
	}
}
