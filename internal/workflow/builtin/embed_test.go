package builtin_test

import (
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/workflow"
	"pkg.akt.dev/akt/internal/workflow/builtin"
)

func TestEmbeddedWorkflowsParseAndExposeTheCompleteBuiltinActions(t *testing.T) {
	wantSteps := map[string][]workflow.StepType{
		"deploy": {
			workflow.StepTx,
			workflow.StepWait,
			workflow.StepPrompt,
			workflow.StepTx,
			workflow.StepProvider,
			workflow.StepOutput,
		},
		"update": {workflow.StepTx, workflow.StepProvider, workflow.StepOutput},
		"close":  {workflow.StepTx, workflow.StepOutput},
	}

	embedded := builtin.Workflows()
	if len(embedded) != len(wantSteps) {
		t.Fatalf("embedded workflows = %v, want deploy, update, and close", mapKeys(embedded))
	}

	for name, expectedTypes := range wantSteps {
		t.Run(name, func(t *testing.T) {
			data, ok := embedded[name]
			if !ok {
				t.Fatalf("workflow %q is not embedded", name)
			}

			definition, err := workflow.NewLoader(t.TempDir(), "", map[string][]byte{name: data}).Load(name)
			if err != nil {
				t.Fatalf("parse embedded workflow: %v", err)
			}
			if definition.Name != name || definition.Description == "" || definition.Version != 1 {
				t.Fatalf("definition header = name %q description %q version %d", definition.Name, definition.Description, definition.Version)
			}
			if len(definition.Steps) != len(expectedTypes) {
				t.Fatalf("step count = %d, want %d", len(definition.Steps), len(expectedTypes))
			}

			seenNames := make(map[string]struct{}, len(definition.Steps))
			for index, step := range definition.Steps {
				if step.Type != expectedTypes[index] {
					t.Errorf("step %d (%s) type = %q, want %q", index, step.Name, step.Type, expectedTypes[index])
				}
				if step.Name == "" {
					t.Errorf("step %d has no stable name", index)
				}
				if _, duplicate := seenNames[step.Name]; duplicate {
					t.Errorf("duplicate step name %q", step.Name)
				}
				seenNames[step.Name] = struct{}{}
			}
		})
	}
}

func TestWorkflowBytesAreFreshAcrossCalls(t *testing.T) {
	first := builtin.Workflows()
	firstDeploy := first["deploy"]
	if len(firstDeploy) == 0 {
		t.Fatal("deploy workflow is empty")
	}
	firstDeploy[0] = 'X'
	delete(first, "close")

	second := builtin.Workflows()
	if len(second["deploy"]) == 0 || !strings.HasPrefix(string(second["deploy"]), "name: deploy") {
		t.Fatalf("caller mutation leaked into embedded deploy: %q", second["deploy"])
	}
	if _, ok := second["close"]; !ok {
		t.Fatal("caller deletion leaked into the next workflow map")
	}
}

func mapKeys(values map[string][]byte) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
