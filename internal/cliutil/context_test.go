package cliutil

import (
	"testing"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	flagdefs "pkg.akt.dev/akt/internal/flags"
)

func scopeCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "probe", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().String(flagdefs.FlagConsoleOrganization, "", "")
	cmd.Flags().String(flagdefs.FlagConsoleProject, "", "")
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	return cmd
}

func TestConsoleScopePrecedence(t *testing.T) {
	rc := &aktctx.Context{ConsoleOrganization: "acme", ConsoleProject: "context-project"}

	tests := []struct {
		name        string
		args        []string
		rc          *aktctx.Context
		env         [2]string
		wantOrg     string
		wantProject string
	}{
		{name: "context", rc: rc, wantOrg: "acme", wantProject: "context-project"},
		{name: "flags win", args: []string{"--console-organization", "other", "--console-project", " web "}, rc: rc, wantOrg: "other", wantProject: "web"},
		{name: "empty flag clears for one run", args: []string{"--console-project="}, rc: rc, wantOrg: "acme", wantProject: ""},
		{name: "environment without a context", env: [2]string{" env-org ", "env-project"}, wantOrg: "env-org", wantProject: "env-project"},
		{name: "nothing selected", wantOrg: "", wantProject: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(aktctx.EnvConsoleOrganization, tc.env[0])
			t.Setenv(aktctx.EnvConsoleProject, tc.env[1])

			organization, project := ConsoleScope(scopeCommand(t, tc.args...), tc.rc)
			if organization != tc.wantOrg || project != tc.wantProject {
				t.Errorf("ConsoleScope() = %q/%q, want %q/%q", organization, project, tc.wantOrg, tc.wantProject)
			}
		})
	}
}

func TestConsoleScopeIgnoresCommandsWithoutTheFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "probe"}
	organization, project := ConsoleScope(cmd, &aktctx.Context{ConsoleOrganization: "acme", ConsoleProject: "web"})
	if organization != "acme" || project != "web" {
		t.Errorf("ConsoleScope() = %q/%q", organization, project)
	}
}
