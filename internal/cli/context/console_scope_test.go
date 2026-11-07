package context

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	"pkg.akt.dev/akt/internal/output/pretty"
)

const scopeProjectID = "7d4c3b2a-1e0f-4a9b-8c7d-6e5f4a3b2c1d"

func showContextJSON(t *testing.T, mgrFn func() *aktctx.Manager, name string) map[string]any {
	t.Helper()

	cmd := currentCmd(mgrFn)
	cmd.Flags().String(flagdefs.FlagContext, "", "test context override")
	cmd.Flags().String(flagdefs.FlagOutput, "pretty", "test output format")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--context", name, "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("context show: %v", err)
	}

	var details map[string]any
	if err := json.Unmarshal(out.Bytes(), &details); err != nil {
		t.Fatalf("decode context JSON %q: %v", out.String(), err)
	}

	return details
}

func TestCreateStoresTheConsoleScope(t *testing.T) {
	t.Setenv(aktctx.EnvConsoleOrganization, "")
	t.Setenv(aktctx.EnvConsoleProject, "")
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "team",
		"--auth-method", aktctx.AuthMethodConsoleAPI,
		"--console-organization", " acme ",
		"--console-project", scopeProjectID,
	)

	stored := m.GetContext("team")
	if stored.ConsoleOrganization != "acme" || stored.ConsoleProject != scopeProjectID {
		t.Fatalf("stored scope = %q/%q", stored.ConsoleOrganization, stored.ConsoleProject)
	}
	details := showContextJSON(t, mgrFn, "team")
	if details["console_organization"] != "acme" || details["console_project"] != scopeProjectID {
		t.Errorf("context show = %#v", details)
	}
	params := string(readLog(t, m.Root(), "team")[0].Params)
	if !strings.Contains(params, `"console-organization":"acme"`) || !strings.Contains(params, `"console-project":"`+scopeProjectID+`"`) {
		t.Errorf("create params = %s", params)
	}
}

func TestCreateRejectsAProjectName(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	err := runErr(t, createCmd(mgrFn), "team",
		"--auth-method", aktctx.AuthMethodConsoleAPI,
		"--console-project", "checkout-api",
	)
	if !strings.Contains(err.Error(), "akt console project use") {
		t.Errorf("create error = %v, want the project use remedy", err)
	}
	if m.GetContext("team") != nil {
		t.Error("a rejected scope must not create the context")
	}
}

func TestEditChangesAndClearsTheConsoleScope(t *testing.T) {
	t.Setenv(aktctx.EnvConsoleOrganization, "")
	t.Setenv(aktctx.EnvConsoleProject, "")
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "team", "--auth-method", aktctx.AuthMethodConsoleAPI)
	runOK(t, editCmd(mgrFn), "team", "--console-organization", "acme", "--console-project", scopeProjectID)

	if got := m.GetContext("team"); got.ConsoleOrganization != "acme" || got.ConsoleProject != scopeProjectID {
		t.Fatalf("edited scope = %q/%q", got.ConsoleOrganization, got.ConsoleProject)
	}
	params := string(readLog(t, m.Root(), "team")[0].Params)
	if !strings.Contains(params, `"console-project":"`+scopeProjectID+`"`) {
		t.Errorf("edit params = %s", params)
	}

	runOK(t, editCmd(mgrFn), "team", "--console-project", "")

	if got := m.GetContext("team"); got.ConsoleOrganization != "acme" || got.ConsoleProject != "" {
		t.Fatalf("cleared scope = %q/%q", got.ConsoleOrganization, got.ConsoleProject)
	}
	details := showContextJSON(t, mgrFn, "team")
	if _, shown := details["console_project"]; shown {
		t.Errorf("a cleared project must not be shown, got %#v", details)
	}
}

func TestShowWithoutTeamsKeepsItsShape(t *testing.T) {
	t.Setenv(aktctx.EnvConsoleOrganization, "")
	t.Setenv(aktctx.EnvConsoleProject, "")
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "solo", "--network", "mainnet")

	details := showContextJSON(t, mgrFn, "solo")
	for _, key := range []string{"console_organization", "console_project"} {
		if _, shown := details[key]; shown {
			t.Errorf("a context without teams must not report %s, got %#v", key, details)
		}
	}
	if strings.Contains(pretty.RenderContextShow(*m.GetContext("solo"), "os"), "Organization") {
		t.Error("a chain context without teams must not render a Console section")
	}
}

func TestShowRendersTheSelectedScope(t *testing.T) {
	rendered := pretty.RenderContextShow(aktctx.Context{
		Name:                "team",
		AuthMethod:          aktctx.AuthMethodConsoleAPI,
		ConsoleOrganization: "acme",
		ConsoleProject:      scopeProjectID,
	}, "os")

	for _, want := range []string{"Organization", "acme", "Project", scopeProjectID} {
		if !strings.Contains(rendered, want) {
			t.Errorf("context show missing %q:\n%s", want, rendered)
		}
	}
}
