package steps

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/workflow"
)

type recordingWorkflowProvider struct {
	sendProvider string
	sendDSeq     uint64
	sendSDL      []byte
	sendCalls    int
	activeDSeq   uint64
	activeSDL    []byte
	activeCalls  int
	providers    []string
	status       json.RawMessage
	statusCalls  int
	statusCtx    context.Context
	statusProv   string
	statusDSeq   uint64
	sendErr      error
	activeErr    error
	statusErr    error
}

func (provider *recordingWorkflowProvider) SendManifest(_ context.Context, address string, dseq uint64, sdl []byte) error {
	provider.sendCalls++
	provider.sendProvider = address
	provider.sendDSeq = dseq
	provider.sendSDL = append([]byte(nil), sdl...)
	return provider.sendErr
}

func (provider *recordingWorkflowProvider) SendManifestToActiveLeases(_ context.Context, dseq uint64, sdl []byte) ([]string, error) {
	provider.activeCalls++
	provider.activeDSeq = dseq
	provider.activeSDL = append([]byte(nil), sdl...)
	return provider.providers, provider.activeErr
}

func (provider *recordingWorkflowProvider) LeaseStatus(ctx context.Context, address string, dseq uint64) (json.RawMessage, error) {
	provider.statusCalls++
	provider.statusCtx = ctx
	provider.statusProv = address
	provider.statusDSeq = dseq
	return provider.status, provider.statusErr
}

func TestProviderExecutorDispatchesEveryAction(t *testing.T) {
	state := workflow.NewRunState("run-1", "provider", "akash1owner", map[string]any{
		"dseq": 42,
		"sdl":  "version: 2.0",
	})

	t.Run("send manifest", func(t *testing.T) {
		provider := &recordingWorkflowProvider{}
		step := workflow.StepDef{
			Name:   "send",
			Type:   workflow.StepProvider,
			Action: "send-manifest",
			Params: map[string]string{
				"provider": "akash1provider",
				"dseq":     "{{ .Params.dseq }}",
				"sdl":      "{{ .Params.sdl }}",
			},
		}
		result, err := (&ProviderExecutor{provider: provider}).Execute(context.Background(), step, state)
		if err != nil || result.Status != "success" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
		if provider.sendCalls != 1 || provider.sendProvider != "akash1provider" || provider.sendDSeq != 42 || string(provider.sendSDL) != "version: 2.0" {
			t.Fatalf("send call = provider=%q dseq=%d sdl=%q calls=%d", provider.sendProvider, provider.sendDSeq, provider.sendSDL, provider.sendCalls)
		}
	})

	t.Run("all active leases", func(t *testing.T) {
		provider := &recordingWorkflowProvider{providers: []string{"akash1a", "akash1b"}}
		step := workflow.StepDef{
			Name:   "fanout",
			Type:   workflow.StepProvider,
			Action: "send-manifest-to-active-leases",
			Params: map[string]string{"dseq": "42", "sdl": "manifest"},
		}
		result, err := (&ProviderExecutor{provider: provider}).Execute(context.Background(), step, state)
		if err != nil || result.Status != "success" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
		if provider.activeCalls != 1 || provider.activeDSeq != 42 || string(provider.activeSDL) != "manifest" {
			t.Fatalf("active call = dseq=%d sdl=%q calls=%d", provider.activeDSeq, provider.activeSDL, provider.activeCalls)
		}
		if result.Output["count"] != 2 || !reflect.DeepEqual(result.Output["providers"], provider.providers) || !json.Valid(result.RawResult) {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("lease status", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), contextKey("provider"), "status-context")
		provider := &recordingWorkflowProvider{status: json.RawMessage(`{"services":{"web":{"available":1}}}`)}
		step := workflow.StepDef{
			Name:   "status",
			Type:   workflow.StepProvider,
			Action: "lease-status",
			Params: map[string]string{"provider": "akash1provider", "dseq": "42"},
			Output: map[string]string{"available": "{{ .Result.services.web.available }}"},
		}
		result, err := (&ProviderExecutor{provider: provider}).Execute(ctx, step, state)
		if err != nil || result.Status != "success" || result.Output["available"] != "1" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
		if provider.statusCalls != 1 || provider.statusProv != "akash1provider" || provider.statusDSeq != 42 || provider.statusCtx.Value(contextKey("provider")) != "status-context" {
			t.Fatalf("status call = provider=%q dseq=%d calls=%d", provider.statusProv, provider.statusDSeq, provider.statusCalls)
		}
	})
}

func TestProviderExecutorBoundaryFailures(t *testing.T) {
	state := workflow.NewRunState("run-1", "provider", "akash1owner", nil)

	t.Run("missing client", func(t *testing.T) {
		result, err := (&ProviderExecutor{}).Execute(context.Background(), workflow.StepDef{
			Name: "send", Type: workflow.StepProvider, Action: "send-manifest",
		}, state)
		assertFailedStep(t, result, err, "provider client not available")
	})

	t.Run("invalid params stop before provider", func(t *testing.T) {
		provider := &recordingWorkflowProvider{}
		result, err := (&ProviderExecutor{provider: provider}).Execute(context.Background(), workflow.StepDef{
			Name:   "send",
			Type:   workflow.StepProvider,
			Action: "send-manifest",
			Params: map[string]string{"dseq": "{{"},
		}, state)
		assertFailedStep(t, result, err, "resolve params")
		if provider.sendCalls != 0 {
			t.Fatalf("invalid params reached provider %d times", provider.sendCalls)
		}
	})

	for _, value := range []string{"", "0", "-1", "not-a-number"} {
		t.Run("invalid dseq "+value, func(t *testing.T) {
			provider := &recordingWorkflowProvider{}
			result, err := (&ProviderExecutor{provider: provider}).Execute(context.Background(), workflow.StepDef{
				Name:   "send",
				Type:   workflow.StepProvider,
				Action: "send-manifest",
				Params: map[string]string{"dseq": value},
			}, state)
			assertFailedStep(t, result, err, "invalid dseq")
			if provider.sendCalls != 0 {
				t.Fatalf("invalid dseq reached provider %d times", provider.sendCalls)
			}
		})
	}

	tests := []struct {
		name   string
		action string
		setup  func(*recordingWorkflowProvider) error
	}{
		{
			name:   "send manifest error",
			action: "send-manifest",
			setup: func(provider *recordingWorkflowProvider) error {
				provider.sendErr = errors.New("manifest rejected")
				return provider.sendErr
			},
		},
		{
			name:   "lease status error",
			action: "lease-status",
			setup: func(provider *recordingWorkflowProvider) error {
				provider.statusErr = errors.New("gateway unavailable")
				return provider.statusErr
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &recordingWorkflowProvider{}
			wantErr := tc.setup(provider)
			result, err := (&ProviderExecutor{provider: provider}).Execute(context.Background(), workflow.StepDef{
				Name:   tc.name,
				Type:   workflow.StepProvider,
				Action: tc.action,
				Params: map[string]string{"provider": "akash1provider", "dseq": "42", "sdl": "manifest"},
			}, state)
			if !errors.Is(err, wantErr) || result.Status != "failed" || result.Error != wantErr.Error() {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
		})
	}

	t.Run("unknown action", func(t *testing.T) {
		result, err := (&ProviderExecutor{provider: &recordingWorkflowProvider{}}).Execute(context.Background(), workflow.StepDef{
			Name: "unknown", Type: workflow.StepProvider, Action: "delete-cluster",
		}, state)
		if err == nil || !strings.Contains(err.Error(), `unknown provider action "delete-cluster"`) || result.Status != "failed" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
	})
}
