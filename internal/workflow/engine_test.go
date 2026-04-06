package workflow_test

import (
	"context"
	"testing"

	wf "pkg.akt.dev/akt/internal/workflow"
)

// mockRegistry returns a registry with no-op executors for testing.
type mockRegistry struct{}

func (r *mockRegistry) Get(t wf.StepType) (wf.StepExecutor, error) {
	return &mockExecutor{stepType: t}, nil
}

type mockExecutor struct {
	stepType wf.StepType
}

func (e *mockExecutor) Type() wf.StepType { return e.stepType }

func (e *mockExecutor) Execute(_ context.Context, step wf.StepDef, state *wf.RunState) (*wf.StepResult, error) {
	return &wf.StepResult{
		Name:   step.Name,
		Type:   step.Type,
		Status: "success",
		Output: map[string]any{"mock": true},
	}, nil
}

func TestEngineRunSimple(t *testing.T) {
	def := &wf.WorkflowDef{
		Name: "test",
		Steps: []wf.StepDef{
			{Name: "step1", Type: wf.StepOutput, Template: "hello"},
			{Name: "step2", Type: wf.StepOutput, Template: "world"},
		},
	}

	engine := wf.NewEngine(&mockRegistry{}, nil)

	state, err := engine.Run(context.Background(), def, "testacct", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(state.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(state.Steps))
	}

	if state.Steps["step1"].Status != "success" {
		t.Errorf("step1 status = %q, want success", state.Steps["step1"].Status)
	}

	if state.Steps["step2"].Status != "success" {
		t.Errorf("step2 status = %q, want success", state.Steps["step2"].Status)
	}
}

func TestEngineStepOrder(t *testing.T) {
	def := &wf.WorkflowDef{
		Name: "order-test",
		Steps: []wf.StepDef{
			{Name: "first", Type: wf.StepOutput},
			{Name: "second", Type: wf.StepOutput},
			{Name: "third", Type: wf.StepOutput},
		},
	}

	engine := wf.NewEngine(&mockRegistry{}, nil)

	state, err := engine.Run(context.Background(), def, "", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(state.StepOrder) != 3 {
		t.Fatalf("expected 3 step order entries, got %d", len(state.StepOrder))
	}

	expected := []string{"first", "second", "third"}
	for i, name := range expected {
		if state.StepOrder[i] != name {
			t.Errorf("step order[%d] = %q, want %q", i, state.StepOrder[i], name)
		}
	}
}

func TestRunStateStepOutput(t *testing.T) {
	state := wf.NewRunState("wf-1", "test", "alice", nil)

	state.SetStepResult("create", &wf.StepResult{
		Name:   "create",
		Status: "success",
		Output: map[string]any{"dseq": "12345"},
	})

	val := state.StepOutput("create", "dseq")
	if val != "12345" {
		t.Errorf("StepOutput = %v, want 12345", val)
	}

	val = state.StepOutput("create", "nonexistent")
	if val != nil {
		t.Errorf("StepOutput for missing key = %v, want nil", val)
	}

	val = state.StepOutput("nonexistent", "dseq")
	if val != nil {
		t.Errorf("StepOutput for missing step = %v, want nil", val)
	}
}

func TestResolveTemplate(t *testing.T) {
	state := wf.NewRunState("wf-1", "test", "alice", map[string]any{
		"deposit": "5000000uakt",
		"timeout": "5m",
	})

	state.SetStepResult("create", &wf.StepResult{
		Name:   "create",
		Status: "success",
		Output: map[string]any{"dseq": "12345"},
	})

	tests := []struct {
		tmpl string
		want string
	}{
		{"{{ .Account }}", "alice"},
		{"{{ .Params.deposit }}", "5000000uakt"},
		{"{{ (index .Steps \"create\").dseq }}", "12345"},
		{"no-template", "no-template"},
		{"", ""},
	}

	for _, tt := range tests {
		got, err := wf.ResolveTemplate(tt.tmpl, state)
		if err != nil {
			t.Errorf("ResolveTemplate(%q): %v", tt.tmpl, err)
			continue
		}

		if got != tt.want {
			t.Errorf("ResolveTemplate(%q) = %q, want %q", tt.tmpl, got, tt.want)
		}
	}
}

func TestResolveParams(t *testing.T) {
	state := wf.NewRunState("wf-1", "test", "bob", map[string]any{
		"amount": "1000uakt",
	})

	params := map[string]string{
		"from":   "{{ .Account }}",
		"amount": "{{ .Params.amount }}",
		"static": "hello",
	}

	resolved, err := wf.ResolveParams(params, state)
	if err != nil {
		t.Fatalf("ResolveParams: %v", err)
	}

	if resolved["from"] != "bob" {
		t.Errorf("from = %q, want bob", resolved["from"])
	}

	if resolved["amount"] != "1000uakt" {
		t.Errorf("amount = %q, want 1000uakt", resolved["amount"])
	}

	if resolved["static"] != "hello" {
		t.Errorf("static = %q, want hello", resolved["static"])
	}
}

func TestEvalCondition(t *testing.T) {
	state := wf.NewRunState("wf-1", "test", "", nil)

	met, err := wf.EvalCondition("", nil, state)
	if err != nil || !met {
		t.Error("empty condition should return true")
	}

	met, err = wf.EvalCondition("{{ .Account }}", nil, state)
	if err != nil {
		t.Fatalf("EvalCondition: %v", err)
	}

	if met {
		t.Error("empty account should evaluate to false")
	}
}
