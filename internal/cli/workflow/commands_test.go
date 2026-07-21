package workflow

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/spf13/cobra"

	wf "pkg.akt.dev/akt/internal/workflow"
	"pkg.akt.dev/akt/internal/workflow/builtin"
)

// commandNames extracts the sorted names of a set of cobra commands.
func commandNames(cmds []*cobra.Command) []string {
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		names = append(names, c.Name())
	}
	sort.Strings(names)

	return names
}

// findCommand returns the command with the given name, or nil.
func findCommand(cmds []*cobra.Command, name string) *cobra.Command {
	for _, c := range cmds {
		if c.Name() == name {
			return c
		}
	}

	return nil
}

// staticFns returns homeFn/ctxNameFn closures over a fixed home and empty context.
func staticFns(home string) (func() string, func() string) {
	return func() string { return home }, func() string { return "" }
}

// TestCommandsBuiltinsOnly verifies that with no user workflows present,
// Commands surfaces exactly the built-in set: close, deploy, update.
func TestCommandsBuiltinsOnly(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmds := Commands(homeFn, ctxNameFn)

	got := commandNames(cmds)
	want := []string{"close", "deploy", "update"}
	if len(got) != len(want) {
		t.Fatalf("Commands() returned %v, want exactly %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Commands() returned %v, want exactly %v", got, want)
		}
	}
}

// TestCommandsUserWorkflowSurfacing verifies that a command appears if and
// only if its backing workflow exists: "foo" is absent before the workflow
// file is written and present after.
func TestCommandsUserWorkflowSurfacing(t *testing.T) {
	home := t.TempDir()
	homeFn, ctxNameFn := staticFns(home)

	// Negative case: no foo workflow exists, so no foo command surfaces.
	if cmd := findCommand(Commands(homeFn, ctxNameFn), "foo"); cmd != nil {
		t.Fatalf("Commands() surfaced %q before its workflow existed", "foo")
	}

	// Write a minimal valid user workflow.
	dir := filepath.Join(home, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yamlDef := `name: foo
description: A user-defined workflow
version: 1

params:
  label:
    type: string
    default: "hello"
    description: A label to print

steps:
  - name: display
    type: output
    template: |
      {{ .Params.label }}
`
	if err := os.WriteFile(filepath.Join(dir, "foo.yaml"), []byte(yamlDef), 0o644); err != nil {
		t.Fatal(err)
	}

	// Positive case: the workflow now resolves, so the command surfaces.
	cmd := findCommand(Commands(homeFn, ctxNameFn), "foo")
	if cmd == nil {
		t.Fatalf("Commands() did not surface %q after its workflow was written", "foo")
	}
	if cmd.Short != "A user-defined workflow" {
		t.Fatalf("foo command Short = %q, want %q", cmd.Short, "A user-defined workflow")
	}
	if cmd.Flags().Lookup("label") == nil {
		t.Fatalf("foo command missing --label flag generated from params")
	}
}

// TestCommandsSkipsMalformedWorkflow verifies that a malformed workflow YAML
// is skipped silently and does not produce a command.
func TestCommandsSkipsMalformedWorkflow(t *testing.T) {
	home := t.TempDir()
	homeFn, ctxNameFn := staticFns(home)

	dir := filepath.Join(home, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{ not: [valid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmds := Commands(homeFn, ctxNameFn)

	if cmd := findCommand(cmds, "bad"); cmd != nil {
		t.Fatalf("Commands() surfaced a command for malformed workflow %q", "bad")
	}
	got := commandNames(cmds)
	want := []string{"close", "deploy", "update"}
	if len(got) != len(want) {
		t.Fatalf("Commands() returned %v, want exactly the built-ins %v", got, want)
	}
}

// loadBuiltin loads a built-in workflow definition through the loader,
// using an empty temp home so only embedded definitions resolve.
func loadBuiltin(t *testing.T, name string) *wf.WorkflowDef {
	t.Helper()

	loader := wf.NewLoader(t.TempDir(), "", builtin.Workflows())
	def, err := loader.Load(name)
	if err != nil {
		t.Fatalf("load built-in workflow %q: %v", name, err)
	}

	return def
}

// TestCommandFromDefClose verifies the generated close command: positional
// [dseq] in Use, a dseq int flag, and the common --yes/--dry-run flags.
func TestCommandFromDefClose(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmd := commandFromDef(loadBuiltin(t, "close"), homeFn, ctxNameFn)

	if cmd.Use != "close [dseq]" {
		t.Fatalf("close Use = %q, want %q", cmd.Use, "close [dseq]")
	}
	dseq := cmd.Flags().Lookup("dseq")
	if dseq == nil {
		t.Fatal("close command missing --dseq flag")
	}
	if dseq.Value.Type() != "int" {
		t.Fatalf("--dseq flag type = %q, want %q", dseq.Value.Type(), "int")
	}
	if cmd.Flags().Lookup("yes") == nil {
		t.Fatal("close command missing common --yes flag")
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("close command missing common --dry-run flag")
	}
}

// TestCommandFromDefDeploy verifies the generated deploy command: the
// required file param is positional in Use (not a flag), the remaining
// params become flags, and the common --yes/--dry-run flags exist.
func TestCommandFromDefDeploy(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmd := commandFromDef(loadBuiltin(t, "deploy"), homeFn, ctxNameFn)

	if cmd.Use != "deploy <sdl-file>" {
		t.Fatalf("deploy Use = %q, want %q", cmd.Use, "deploy <sdl-file>")
	}
	if cmd.Flags().Lookup("sdl-file") != nil {
		t.Fatal("deploy command has --sdl-file flag; file param must be positional only")
	}
	for _, flag := range []string{"deposit", "bid-timeout", "bid-select", "yes", "dry-run"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("deploy command missing --%s flag", flag)
		}
	}
}

// TestUserWorkflowOverridesBuiltin verifies that a user workflow with the
// same name as a built-in takes precedence when the command is generated.
func TestUserWorkflowOverridesBuiltin(t *testing.T) {
	home := t.TempDir()
	homeFn, ctxNameFn := staticFns(home)

	builtinDesc := loadBuiltin(t, "close").Description

	dir := filepath.Join(home, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := `name: close
description: Custom close override
version: 2

steps:
  - name: display
    type: output
    template: |
      overridden
`
	if err := os.WriteFile(filepath.Join(dir, "close.yaml"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := findCommand(Commands(homeFn, ctxNameFn), "close")
	if cmd == nil {
		t.Fatal("Commands() did not surface close")
	}
	if cmd.Short != "Custom close override" {
		t.Fatalf("close Short = %q, want the user override %q", cmd.Short, "Custom close override")
	}
	if cmd.Short == builtinDesc {
		t.Fatalf("close Short = %q still matches the built-in description; override not applied", cmd.Short)
	}
}
