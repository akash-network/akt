package workflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
)

const workflowScopeProjectID = "7d4c3b2a-1e0f-4a9b-8c7d-6e5f4a3b2c1d"

func scopedDeployCommand(t *testing.T, authMethod string) func() *cobra.Command {
	t.Helper()

	t.Setenv(aktctx.EnvConsoleOrganization, "")
	t.Setenv(aktctx.EnvConsoleProject, "")
	home := t.TempDir()
	m := newTestManager(t, home, "team", authMethod)
	if err := m.UpdateContext("team", func(c *aktctx.Context) error {
		c.ConsoleOrganization = "acme"
		c.ConsoleProject = workflowScopeProjectID
		return nil
	}); err != nil {
		t.Fatalf("UpdateContext: %v", err)
	}

	return func() *cobra.Command {
		return findCommand(CommandsWithManager(
			func() string { return home },
			func() string { return "team" },
			func() *aktctx.Manager { return m },
		), "deploy")
	}
}

func TestConsoleDryRunNamesTheProjectItDeploysInto(t *testing.T) {
	newCommand := scopedDeployCommand(t, aktctx.AuthMethodConsoleAPI)
	sdl := writeValidWorkflowSDL(t)

	out, err := executeCommand(t, newCommand(), sdl, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Console organization: acme") || !strings.Contains(out, "Console project: "+workflowScopeProjectID) {
		t.Fatalf("dry-run plan does not name the stored scope:\n%s", out)
	}

	out, err = executeCommand(t, newCommand(), sdl, "--dry-run", "--console-project", "checkout-api")
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Console project: checkout-api") {
		t.Fatalf("--console-project must override the context for one run:\n%s", out)
	}
}

func TestConsoleDryRunJSONLReportsTheProject(t *testing.T) {
	newCommand := scopedDeployCommand(t, aktctx.AuthMethodConsoleAPI)

	out, err := executeCommand(t, newCommand(), writeValidWorkflowSDL(t), "--dry-run", "--output", "jsonl", "--console-project", "checkout-api")
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}

	var create struct {
		Step    string         `json:"step"`
		Outputs map[string]any `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(strings.SplitN(out, "\n", 2)[0]), &create); err != nil {
		t.Fatalf("decode first line: %v\n%s", err, out)
	}
	if create.Step != "create-deployment" || create.Outputs["console-organization"] != "acme" || create.Outputs["console-project"] != "checkout-api" {
		t.Fatalf("create-deployment line = %+v", create)
	}
}

func TestChainDryRunIgnoresTheConsoleScope(t *testing.T) {
	newCommand := scopedDeployCommand(t, aktctx.AuthMethodKeyring)

	out, err := executeCommand(t, newCommand(), writeValidWorkflowSDL(t), "--deposit", "5000000uact", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if strings.Contains(out, "Console project") || strings.Contains(out, "Console organization") {
		t.Fatalf("the chain rail has no Console scope:\n%s", out)
	}
}

func TestPlannedCreateOutputsAreOmittedWhenEmpty(t *testing.T) {
	if outputs := plannedCreateOutputs(map[string]any{}, consoleScope{}); outputs != nil {
		t.Fatalf("plannedCreateOutputs() = %#v, want nil", outputs)
	}
}
