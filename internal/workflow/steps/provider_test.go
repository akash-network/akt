package steps

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/workflow"
)

type fakeWorkflowProvider struct {
	providers []string
	err       error
}

func (p *fakeWorkflowProvider) SendManifest(context.Context, string, uint64, []byte) error {
	return p.err
}

func (p *fakeWorkflowProvider) SendManifestToActiveLeases(context.Context, uint64, []byte) ([]string, error) {
	return p.providers, p.err
}

func (p *fakeWorkflowProvider) LeaseStatus(context.Context, string, uint64) (json.RawMessage, error) {
	return nil, p.err
}

func TestProviderExecutorSendsManifestToAllActiveLeases(t *testing.T) {
	providerErr := errors.New("one provider refused")
	executor := &ProviderExecutor{provider: &fakeWorkflowProvider{
		providers: []string{"akash1accepted"},
		err:       providerErr,
	}}
	state := workflow.NewRunState("run-1", "update", "akash1owner", map[string]any{
		"dseq":     4242,
		"sdl-file": "deploy.yaml",
	})
	step := workflow.StepDef{
		Name:   "send-manifest",
		Type:   workflow.StepProvider,
		Action: "send-manifest-to-active-leases",
		Params: map[string]string{
			"dseq": "{{ .Params.dseq }}",
			"sdl":  "{{ index .Params \"sdl-file\" }}",
		},
	}

	result, err := executor.Execute(context.Background(), step, state)
	if !errors.Is(err, providerErr) {
		t.Fatalf("error = %v, want provider failure", err)
	}
	if result.Status != "failed" || result.Output["count"] != 1 {
		t.Fatalf("result = %+v, want failed with one successful delivery", result)
	}
	providers, ok := result.Output["providers"].([]string)
	if !ok || len(providers) != 1 || providers[0] != "akash1accepted" {
		t.Errorf("successful providers = %#v", result.Output["providers"])
	}
	var raw struct {
		Providers []string `json:"providers"`
		Count     int      `json:"count"`
	}
	if err := json.Unmarshal(result.RawResult, &raw); err != nil {
		t.Fatalf("raw result is not JSON: %v", err)
	}
	if raw.Count != 1 || len(raw.Providers) != 1 {
		t.Errorf("raw result = %+v", raw)
	}
}

func TestProviderExecutorTreatsNoActiveLeasesAsSuccess(t *testing.T) {
	executor := &ProviderExecutor{provider: &fakeWorkflowProvider{}}
	state := workflow.NewRunState("run-1", "update", "akash1owner", nil)
	step := workflow.StepDef{
		Name:   "send-manifest",
		Type:   workflow.StepProvider,
		Action: "send-manifest-to-active-leases",
		Params: map[string]string{"dseq": "4242", "sdl": "deploy.yaml"},
	}

	result, err := executor.Execute(context.Background(), step, state)
	if err != nil {
		t.Fatalf("no active leases: %v", err)
	}
	if result.Status != "success" || result.Output["count"] != 0 {
		t.Errorf("result = %+v, want successful no-op", result)
	}
}

func TestProviderExecutorRejectsInvalidDSeq(t *testing.T) {
	executor := &ProviderExecutor{provider: &fakeWorkflowProvider{}}
	for _, action := range []string{"send-manifest", "send-manifest-to-active-leases", "lease-status"} {
		t.Run(action, func(t *testing.T) {
			step := workflow.StepDef{
				Name:   action,
				Type:   workflow.StepProvider,
				Action: action,
				Params: map[string]string{"dseq": "not-a-number", "provider": "akash1provider"},
			}
			result, err := executor.Execute(context.Background(), step, workflow.NewRunState("run", "test", "owner", nil))
			if err == nil || !strings.Contains(err.Error(), "invalid dseq") {
				t.Fatalf("error = %v, want invalid dseq", err)
			}
			if result == nil || result.Status != "failed" {
				t.Fatalf("result = %+v, want failed result", result)
			}
		})
	}
}
