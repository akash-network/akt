package context_test

import (
	"os"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

const scopeTestProjectID = "7d4c3b2a-1e0f-4a9b-8c7d-6e5f4a3b2c1d"

func newConsoleContext(t *testing.T, m *aktctx.Manager, ctx aktctx.Context) {
	t.Helper()

	ctx.AuthMethod = aktctx.AuthMethodConsoleAPI
	if err := m.CreateContext(ctx); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
}

func TestConsoleScopeRoundTripsThroughConfig(t *testing.T) {
	m := newTestManager(t)
	newConsoleContext(t, m, aktctx.Context{Name: "team", ConsoleOrganization: "acme", ConsoleProject: scopeTestProjectID})

	raw, err := os.ReadFile(aktctx.ConfigPath(m.Root()))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	for _, want := range []string{"console-organization: acme", "console-project: " + scopeTestProjectID} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("config.yaml missing %q:\n%s", want, raw)
		}
	}

	reloaded, err := aktctx.NewManager(m.Root())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	t.Setenv(aktctx.EnvConsoleOrganization, "")
	t.Setenv(aktctx.EnvConsoleProject, "")
	rc, err := reloaded.Resolve("team")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if rc.ConsoleOrganization != "acme" || rc.ConsoleProject != scopeTestProjectID {
		t.Errorf("resolved scope = %q/%q", rc.ConsoleOrganization, rc.ConsoleProject)
	}
}

func TestContextWithoutScopeWritesNoScopeKeys(t *testing.T) {
	m := newTestManager(t)
	newConsoleContext(t, m, aktctx.Context{Name: "solo"})

	raw, err := os.ReadFile(aktctx.ConfigPath(m.Root()))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(raw), "console-organization") || strings.Contains(string(raw), "console-project") {
		t.Errorf("a context without teams must keep its config shape:\n%s", raw)
	}
}

func TestEnvironmentOverridesTheStoredScope(t *testing.T) {
	m := newTestManager(t)
	newConsoleContext(t, m, aktctx.Context{Name: "team", ConsoleOrganization: "acme", ConsoleProject: scopeTestProjectID})

	t.Setenv(aktctx.EnvConsoleOrganization, " other-team ")
	t.Setenv(aktctx.EnvConsoleProject, "checkout-api")
	rc, err := m.Resolve("team")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if rc.ConsoleOrganization != "other-team" || rc.ConsoleProject != "checkout-api" {
		t.Errorf("resolved scope = %q/%q, want the environment values", rc.ConsoleOrganization, rc.ConsoleProject)
	}

	t.Setenv(aktctx.EnvConsoleOrganization, "  ")
	t.Setenv(aktctx.EnvConsoleProject, "")
	rc, err = m.Resolve("team")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if rc.ConsoleOrganization != "acme" || rc.ConsoleProject != scopeTestProjectID {
		t.Errorf("blank variables must leave the stored scope, got %q/%q", rc.ConsoleOrganization, rc.ConsoleProject)
	}

	stored := m.GetContext("team")
	if stored.ConsoleProject != scopeTestProjectID {
		t.Errorf("Resolve must not change the stored selection, got %q", stored.ConsoleProject)
	}
}

func TestStoredScopeIsValidated(t *testing.T) {
	tests := []struct {
		name string
		ctx  aktctx.Context
		want string
	}{
		{"project name", aktctx.Context{Name: "a", ConsoleProject: "checkout-api"}, "must be a project id"},
		{"compact uuid", aktctx.Context{Name: "b", ConsoleProject: strings.ReplaceAll(scopeTestProjectID, "-", "")}, "must be a project id"},
		{"organization with space", aktctx.Context{Name: "c", ConsoleOrganization: "acme team"}, "without spaces"},
		{"organization too long", aktctx.Context{Name: "d", ConsoleOrganization: strings.Repeat("a", 129)}, "at most 128"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestManager(t)
			tc.ctx.AuthMethod = aktctx.AuthMethodConsoleAPI
			err := m.CreateContext(tc.ctx)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CreateContext error = %v, want %q", err, tc.want)
			}
		})
	}

	m := newTestManager(t)
	newConsoleContext(t, m, aktctx.Context{Name: "team"})
	err := m.UpdateContext("team", func(c *aktctx.Context) error {
		c.ConsoleProject = "web"
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "akt console project use") {
		t.Fatalf("UpdateContext error = %v, want the project use remedy", err)
	}
	if got := m.GetContext("team").ConsoleProject; got != "" {
		t.Errorf("a rejected edit must not be applied, got %q", got)
	}
}
