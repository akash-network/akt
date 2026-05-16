package workflow_test

import (
	"os"
	"path/filepath"
	"testing"

	wf "pkg.akt.dev/akt/internal/workflow"
	"pkg.akt.dev/akt/internal/workflow/builtin"
)

func TestLoaderNotFound(t *testing.T) {
	loader := wf.NewLoader("", "", nil)

	_, err := loader.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent workflow")
	}
}

func TestLoaderGlobalWorkflow(t *testing.T) {
	root := t.TempDir()
	wfDir := filepath.Join(root, "workflows")
	_ = os.MkdirAll(wfDir, 0o755)

	custom := `
name: my-deploy
description: Custom deploy workflow
version: 1
steps:
  - name: step1
    type: output
    template: hello
`
	_ = os.WriteFile(filepath.Join(wfDir, "my-deploy.yaml"), []byte(custom), 0o644)

	loader := wf.NewLoader(root, "", builtin.Workflows())

	def, err := loader.Load("my-deploy")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if def.Name != "my-deploy" {
		t.Errorf("name = %q, want my-deploy", def.Name)
	}

	if def.Description != "Custom deploy workflow" {
		t.Errorf("description = %q, want Custom deploy workflow", def.Description)
	}
}

func TestLoaderContextOverride(t *testing.T) {
	root := t.TempDir()

	// Global version.
	globalDir := filepath.Join(root, "workflows")
	_ = os.MkdirAll(globalDir, 0o755)
	_ = os.WriteFile(filepath.Join(globalDir, "test.yaml"), []byte(`
name: test
description: global version
version: 1
steps:
  - name: s1
    type: output
    template: global
`), 0o644)

	// Per-context override.
	ctxDir := filepath.Join(root, "contexts", "prod", "workflows")
	_ = os.MkdirAll(ctxDir, 0o755)
	_ = os.WriteFile(filepath.Join(ctxDir, "test.yaml"), []byte(`
name: test
description: context override
version: 1
steps:
  - name: s1
    type: output
    template: context
`), 0o644)

	loader := wf.NewLoader(root, "prod", builtin.Workflows())

	def, err := loader.Load("test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Per-context should win.
	if def.Description != "context override" {
		t.Errorf("description = %q, want context override", def.Description)
	}
}

func TestLoaderList(t *testing.T) {
	root := t.TempDir()
	wfDir := filepath.Join(root, "workflows")
	_ = os.MkdirAll(wfDir, 0o755)

	_ = os.WriteFile(filepath.Join(wfDir, "wf-a.yaml"), []byte(`
name: wf-a
description: Workflow A
version: 1
steps:
  - name: s1
    type: output
    template: a
`), 0o644)

	_ = os.WriteFile(filepath.Join(wfDir, "wf-b.yaml"), []byte(`
name: wf-b
description: Workflow B
version: 1
steps:
  - name: s1
    type: output
    template: b
`), 0o644)

	loader := wf.NewLoader(root, "", builtin.Workflows())

	names := loader.List()

	// Expect the 2 user-defined workflows plus the 3 built-in ones
	// (deploy, update, close).
	builtinCount := len(builtin.Workflows())
	want := 2 + builtinCount
	if len(names) != want {
		t.Errorf("expected %d workflows, got %d: %v", want, len(names), names)
	}
}
