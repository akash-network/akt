package console

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	aktconsole "pkg.akt.dev/akt/internal/console"
)

func TestPinnedServerRefusesAnotherProject(t *testing.T) {
	srv, calls := newProjectServer(t)
	client := aktconsole.New(srv.URL, "test-key").WithScope("acme", mcpProjectID)

	tests := map[string]struct {
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args    map[string]any
	}{
		"close deployment": {HandleCloseDeployment(client), map[string]any{"dseq": "42", "project": "default"}},
		"list deployments": {HandleListDeployments(client), map[string]any{"project": "default"}},
		"get deployment":   {HandleGetDeployment(client), map[string]any{"dseq": "42", "project": mcpOtherProjectID}},
		"list bids":        {HandleListBids(client), map[string]any{"dseq": "42", "project": "default"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := callTool(t, tc.handler, tc.args)
			if !result.IsError || !strings.Contains(consoleResultText(t, result), "this server acts only in project "+mcpProjectID) {
				t.Fatalf("result = %s, want the pinned project refusal", consoleResultText(t, result))
			}
		})
	}

	for _, call := range calls() {
		if call.path != "/v1/projects" {
			t.Errorf("a refused call must not reach Console beyond resolving the project, got %+v", call)
		}
	}
}

func TestPinnedServerAcceptsItsOwnProject(t *testing.T) {
	srv, calls := newProjectServer(t)
	client := aktconsole.New(srv.URL, "test-key").WithScope("acme", "checkout-api")

	result := callTool(t, HandleCloseDeployment(client), map[string]any{"dseq": "42", "project": strings.ToUpper(mcpProjectID)})
	if result.IsError {
		t.Fatalf("close deployment: %s", consoleResultText(t, result))
	}
	if closed := lastCall(t, calls(), http.MethodDelete, "/v1/deployments/42"); closed.project != mcpProjectID {
		t.Fatalf("close sent project %q, want the pinned one", closed.project)
	}
}

func TestPinnedServerReportsUnresolvableProjects(t *testing.T) {
	srv, _ := newProjectServer(t)

	unknownPin := aktconsole.New(srv.URL, "test-key").WithScope("acme", "missing")
	result := callTool(t, HandleCloseDeployment(unknownPin), map[string]any{"dseq": "42", "project": "default"})
	if !result.IsError || !strings.Contains(consoleResultText(t, result), "resolve this server's project") {
		t.Errorf("unresolvable server project = %s", consoleResultText(t, result))
	}

	pinned := aktconsole.New(srv.URL, "test-key").WithScope("acme", mcpProjectID)
	result = callTool(t, HandleCloseDeployment(pinned), map[string]any{"dseq": "42", "project": "nope"})
	if !result.IsError || !strings.Contains(consoleResultText(t, result), `resolve project "nope"`) {
		t.Errorf("unresolvable requested project = %s", consoleResultText(t, result))
	}
}

func TestListProjectsDescribesWhetherTheServerIsPinned(t *testing.T) {
	if pinned := ToolListProjects(true).Description; !strings.Contains(pinned, "acts only in the project it was started with") || strings.Contains(pinned, "Pass a project") {
		t.Errorf("pinned description = %q", pinned)
	}
	if unpinned := ToolListProjects(false).Description; !strings.Contains(unpinned, "Pass a project") {
		t.Errorf("unpinned description = %q", unpinned)
	}
}
