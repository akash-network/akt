package workflow_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/types/bech32"

	wf "pkg.akt.dev/akt/internal/workflow"
	"pkg.akt.dev/go/sdkutil"
)

func TestValidateBidSelectionAcceptsOnlySupportedModesAndFullProviderAddresses(t *testing.T) {
	provider, err := bech32.ConvertAndEncode(
		sdkutil.Bech32PrefixAccAddr,
		bytes.Repeat([]byte{0x11}, 20),
	)
	if err != nil {
		t.Fatalf("encode provider: %v", err)
	}

	for _, selection := range []string{"interactive", "cheapest", "provider=" + provider} {
		if err := wf.ValidateBidSelection(selection); err != nil {
			t.Errorf("ValidateBidSelection(%q): %v", selection, err)
		}
	}

	wrongPrefix, err := bech32.ConvertAndEncode("cosmos", bytes.Repeat([]byte{0x11}, 20))
	if err != nil {
		t.Fatalf("encode wrong-prefix provider: %v", err)
	}
	shortAddress, err := bech32.ConvertAndEncode(sdkutil.Bech32PrefixAccAddr, bytes.Repeat([]byte{0x11}, 19))
	if err != nil {
		t.Fatalf("encode short provider: %v", err)
	}

	tests := []struct {
		selection string
		want      string
	}{
		{"", "use interactive, cheapest, or provider=<full-address>"},
		{"provider=", "provider address cannot be empty"},
		{"provider=not-bech32", "invalid provider address"},
		{"provider=" + wrongPrefix, "expected a full akash account address"},
		{"provider=" + shortAddress, "expected a full akash account address"},
	}
	for _, test := range tests {
		t.Run(test.selection, func(t *testing.T) {
			err := wf.ValidateBidSelection(test.selection)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestLoaderRejectsInvalidWorkflowDefinitions(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{name: "malformed", yaml: "name: [", want: "parse workflow"},
		{
			name: "missing-name",
			yaml: "steps:\n  - name: display\n    type: output\n",
			want: "missing required field: name",
		},
		{name: "missing-steps", yaml: "name: empty\n", want: `workflow "empty" has no steps`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := wf.NewLoader(t.TempDir(), "", map[string][]byte{"invalid": []byte(test.yaml)})
			_, err := loader.Load("invalid")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestLoaderListHonorsPrecedenceAndIgnoresNonWorkflowEntries(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, "contexts", "prod", "workflows")
	globalDir := filepath.Join(root, "workflows")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatalf("make context workflows: %v", err)
	}
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("make global workflows: %v", err)
	}

	writeTestWorkflow(t, filepath.Join(contextDir, "shared.yaml"), "shared")
	writeTestWorkflow(t, filepath.Join(globalDir, "shared.yml"), "shared")
	writeTestWorkflow(t, filepath.Join(globalDir, "global.yml"), "global")
	writeTestFile(t, filepath.Join(globalDir, "README.txt"), "not a workflow")
	if err := os.MkdirAll(filepath.Join(globalDir, "directory.yaml"), 0o755); err != nil {
		t.Fatalf("make misleading directory: %v", err)
	}

	loader := wf.NewLoader(root, "prod", map[string][]byte{
		"shared":  []byte("unused duplicate"),
		"builtin": []byte("unused in List"),
	})
	names := loader.List()
	sort.Strings(names)
	want := []string{"builtin", "global", "shared"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("workflow names = %v, want %v", names, want)
	}
}

func writeTestWorkflow(t *testing.T, path, name string) {
	t.Helper()
	writeTestFile(t, path, "name: "+name+"\nsteps:\n  - name: display\n    type: output\n")
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type transientExecutor struct {
	stepType wf.StepType
	calls    int
	failures int
	cancel   context.CancelFunc
}

func (executor *transientExecutor) Type() wf.StepType { return executor.stepType }

func (executor *transientExecutor) Execute(
	_ context.Context,
	step wf.StepDef,
	_ *wf.RunState,
) (*wf.StepResult, error) {
	executor.calls++
	result := &wf.StepResult{Name: step.Name, Type: step.Type}
	if executor.calls <= executor.failures {
		result.Status = "failed"
		result.Error = "transient failure"
		if executor.cancel != nil {
			executor.cancel()
		}
		return result, errors.New("transient failure")
	}
	result.Status = "success"
	return result, nil
}

type singleExecutorRegistry struct {
	executor wf.StepExecutor
	err      error
}

func (registry singleExecutorRegistry) Get(wf.StepType) (wf.StepExecutor, error) {
	return registry.executor, registry.err
}

type recordingWorkflowLogger struct {
	workflowIDs []string
	indexes     []int
	results     []*wf.StepResult
}

func (logger *recordingWorkflowLogger) LogStep(workflowID string, index int, result *wf.StepResult) {
	logger.workflowIDs = append(logger.workflowIDs, workflowID)
	logger.indexes = append(logger.indexes, index)
	logger.results = append(logger.results, result)
}

func TestEngineRetriesTransientFailureButLogsOneFinalStep(t *testing.T) {
	executor := &transientExecutor{stepType: wf.StepQuery, failures: 2}
	logger := &recordingWorkflowLogger{}
	engine := wf.NewEngine(singleExecutorRegistry{executor: executor}, logger)
	definition := &wf.WorkflowDef{Name: "retry", Steps: []wf.StepDef{{
		Name: "query", Type: wf.StepQuery, Retry: &wf.RetryDef{Max: 3, Delay: "1ms"},
	}}}

	state, err := engine.Run(context.Background(), definition, "akash1owner", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if executor.calls != 3 {
		t.Errorf("executor calls = %d, want 3", executor.calls)
	}
	if len(logger.results) != 1 || logger.results[0].Status != "success" || logger.indexes[0] != 0 ||
		logger.workflowIDs[0] != state.WorkflowID {
		t.Fatalf("step log = indexes %v results %#v; want one final success", logger.indexes, logger.results)
	}
	if state.Steps["query"] == nil || state.Steps["query"].Status != "success" {
		t.Fatalf("run state = %#v, want successful retried step", state.Steps)
	}
}

func TestEngineCancellationInterruptsRetryBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	executor := &transientExecutor{stepType: wf.StepProvider, failures: 2, cancel: cancel}
	engine := wf.NewEngine(singleExecutorRegistry{executor: executor}, nil)
	definition := &wf.WorkflowDef{Name: "retry", Steps: []wf.StepDef{{
		Name: "manifest", Type: wf.StepProvider, Retry: &wf.RetryDef{Max: 3, Delay: "2s"},
	}}}

	started := time.Now()
	state, err := engine.Run(ctx, definition, "akash1owner", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context cancellation", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("cancellation waited for retry backoff")
	}
	if executor.calls != 1 {
		t.Errorf("executor calls = %d, want no retry after cancellation", executor.calls)
	}
	if state.Steps["manifest"] == nil || state.Steps["manifest"].Status != "failed" {
		t.Fatalf("failed attempt missing from state: %#v", state.Steps)
	}
}

func TestEngineRecordsRegistryLookupFailure(t *testing.T) {
	sentinel := errors.New("executor is not registered")
	logger := &recordingWorkflowLogger{}
	engine := wf.NewEngine(singleExecutorRegistry{err: sentinel}, logger)
	definition := &wf.WorkflowDef{Name: "bad", Steps: []wf.StepDef{{Name: "unknown", Type: wf.StepType("custom")}}}

	state, err := engine.Run(context.Background(), definition, "akash1owner", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want registry failure", err)
	}
	result := state.Steps["unknown"]
	if result == nil || result.Status != "failed" || result.Error != sentinel.Error() {
		t.Fatalf("registry failure result = %#v", result)
	}
	if len(logger.results) != 1 || logger.results[0] != result {
		t.Fatalf("logged results = %#v, want the recorded failed step", logger.results)
	}
}
