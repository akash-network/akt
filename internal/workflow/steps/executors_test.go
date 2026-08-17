package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"pkg.akt.dev/akt/internal/workflow"
)

type recordingStepChain struct {
	queryCalls     int
	broadcastCalls int
	queryPath      string
	msgType        string
	queryParams    map[string]string
	txParams       map[string]string
	queryResult    json.RawMessage
	txResult       *TxResult
	queryErr       error
	txErr          error
	queryContext   context.Context
	txContext      context.Context
}

func (chain *recordingStepChain) BroadcastTx(ctx context.Context, msgType string, params map[string]string) (*TxResult, error) {
	chain.broadcastCalls++
	chain.msgType = msgType
	chain.txParams = params
	chain.txContext = ctx
	return chain.txResult, chain.txErr
}

func (chain *recordingStepChain) Query(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	chain.queryCalls++
	chain.queryPath = path
	chain.queryParams = params
	chain.queryContext = ctx
	return chain.queryResult, chain.queryErr
}

func TestCheckExecutorConditionSemantics(t *testing.T) {
	state := workflow.NewRunState("run-1", "verify", "akash1owner", nil)
	state.SetStepResult("query", &workflow.StepResult{
		Name:   "query",
		Status: "success",
		Output: map[string]any{"ready": "yes"},
	})
	executor := &CheckExecutor{}

	t.Run("condition met", func(t *testing.T) {
		step := workflow.StepDef{
			Name:      "ready",
			Type:      workflow.StepCheck,
			Condition: `{{ eq (index .Steps "query").ready "yes" }}`,
		}
		result, err := executor.Execute(context.Background(), step, state)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.Status != "success" || result.Error != "" {
			t.Fatalf("result = %+v, want successful check", result)
		}
	})

	t.Run("empty condition is true", func(t *testing.T) {
		result, err := executor.Execute(context.Background(), workflow.StepDef{
			Name: "unconditional",
			Type: workflow.StepCheck,
		}, state)
		if err != nil || result.Status != "success" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
	})

	t.Run("abort when false", func(t *testing.T) {
		step := workflow.StepDef{
			Name:      "must-be-ready",
			Type:      workflow.StepCheck,
			Condition: "false",
			OnFail:    workflow.OnErrorAbort,
		}
		result, err := executor.Execute(context.Background(), step, state)
		if err == nil || !strings.Contains(err.Error(), "not met") {
			t.Fatalf("error = %v, want unmet-condition failure", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Error, "condition not met") {
			t.Fatalf("result = %+v, want failed check with explanation", result)
		}
	})

	t.Run("skip when false", func(t *testing.T) {
		step := workflow.StepDef{
			Name:      "optional-check",
			Type:      workflow.StepCheck,
			Condition: "false",
			OnFail:    workflow.OnErrorSkip,
		}
		result, err := executor.Execute(context.Background(), step, state)
		if err != nil {
			t.Fatalf("on-fail skip must not abort the workflow: %v", err)
		}
		if result.Status != "skipped" || !strings.Contains(result.Error, "condition not met") {
			t.Fatalf("result = %+v, want an explained skipped check", result)
		}
	})

	t.Run("invalid condition", func(t *testing.T) {
		step := workflow.StepDef{Name: "invalid", Type: workflow.StepCheck, Condition: "{{"}
		result, err := executor.Execute(context.Background(), step, state)
		if err == nil || !strings.Contains(err.Error(), "eval condition") {
			t.Fatalf("error = %v, want condition evaluation context", err)
		}
		if result.Status != "failed" || !strings.Contains(result.Error, "eval condition") {
			t.Fatalf("result = %+v, want failed evaluation", result)
		}
	})
}

func TestCheckOnFailSkipContinuesWorkflow(t *testing.T) {
	registry := NewRegistry(nil, nil)
	var output strings.Builder
	registry.Register(&OutputExecutor{Out: &output})
	engine := workflow.NewEngine(registry, nil)

	state, err := engine.Run(context.Background(), &workflow.WorkflowDef{
		Name: "skip-check",
		Steps: []workflow.StepDef{
			{
				Name:      "optional",
				Type:      workflow.StepCheck,
				Condition: "false",
				OnFail:    workflow.OnErrorSkip,
			},
			{
				Name:     "continued",
				Type:     workflow.StepOutput,
				Template: "workflow continued",
			},
		},
	}, "akash1owner", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if state.Steps["optional"].Status != "skipped" || state.Steps["continued"].Status != "success" {
		t.Fatalf("state = %+v", state.Steps)
	}
	if got := output.String(); got != "workflow continued" {
		t.Fatalf("continued output = %q", got)
	}
}

func TestOutputExecutorRendersAndReportsWriteFailures(t *testing.T) {
	state := workflow.NewRunState("run-1", "render", "akash1owner", map[string]any{"dseq": 42})

	t.Run("renders to writer and result", func(t *testing.T) {
		var output strings.Builder
		result, err := (&OutputExecutor{Out: &output}).Execute(context.Background(), workflow.StepDef{
			Name:     "summary",
			Type:     workflow.StepOutput,
			Template: `deployment {{ .Params.dseq }}`,
		}, state)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if output.String() != "deployment 42" || result.Output["text"] != "deployment 42" || result.Status != "success" {
			t.Fatalf("output = %q, result = %+v", output.String(), result)
		}
	})

	t.Run("nil writer uses stdout", func(t *testing.T) {
		var result *workflow.StepResult
		var executeErr error
		output, captureErr := runInteractiveSelectionFunc(t, "", func() {
			result, executeErr = (&OutputExecutor{}).Execute(context.Background(), workflow.StepDef{
				Name:     "summary",
				Type:     workflow.StepOutput,
				Template: "stdout result",
			}, state)
		})
		if captureErr != nil || executeErr != nil {
			t.Fatalf("capture error = %v, execute error = %v", captureErr, executeErr)
		}
		if output != "stdout result" || result.Status != "success" || result.Output["text"] != output {
			t.Fatalf("stdout = %q, result = %+v", output, result)
		}
	})

	t.Run("template parse error writes nothing", func(t *testing.T) {
		var output strings.Builder
		result, err := (&OutputExecutor{Out: &output}).Execute(context.Background(), workflow.StepDef{
			Name:     "broken",
			Type:     workflow.StepOutput,
			Template: "{{",
		}, state)
		if err == nil || !strings.Contains(err.Error(), "render output template") {
			t.Fatalf("error = %v, want render context", err)
		}
		if result.Status != "failed" || output.Len() != 0 {
			t.Fatalf("result = %+v, output = %q", result, output.String())
		}
	})

	t.Run("writer error fails step", func(t *testing.T) {
		writeErr := errors.New("destination closed")
		result, err := (&OutputExecutor{Out: errorWriter{err: writeErr}}).Execute(context.Background(), workflow.StepDef{
			Name:     "summary",
			Type:     workflow.StepOutput,
			Template: "important result",
		}, state)
		if !errors.Is(err, writeErr) {
			t.Fatalf("error = %v, want writer failure", err)
		}
		if result == nil || result.Status != "failed" || !strings.Contains(result.Error, writeErr.Error()) {
			t.Fatalf("result = %+v, want failed write", result)
		}
	})

	t.Run("short write fails step", func(t *testing.T) {
		result, err := (&OutputExecutor{Out: shortWriter{}}).Execute(context.Background(), workflow.StepDef{
			Name:     "summary",
			Type:     workflow.StepOutput,
			Template: "important result",
		}, state)
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("error = %v, want io.ErrShortWrite", err)
		}
		if result == nil || result.Status != "failed" {
			t.Fatalf("result = %+v, want failed short write", result)
		}
	})
}

type errorWriter struct{ err error }

func (writer errorWriter) Write([]byte) (int, error) { return 0, writer.err }

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

func TestQueryExecutorUsesResolvedInputsAndObservableResult(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("request"), "query-ctx")
	chain := &recordingStepChain{queryResult: json.RawMessage(`{"deployment":{"id":{"dseq":"42"}},"state":"active"}`)}
	state := workflow.NewRunState("run-1", "inspect", "akash1owner", map[string]any{"dseq": 42})
	step := workflow.StepDef{
		Name:  "get-deployment",
		Type:  workflow.StepQuery,
		Query: "deployment.get",
		Params: map[string]string{
			"owner": "{{ .Account }}",
			"dseq":  "{{ .Params.dseq }}",
		},
		Output: map[string]string{
			"dseq":  "{{ .Result.deployment.id.dseq }}",
			"state": "{{ .Result.state }}",
		},
	}

	result, err := (&QueryExecutor{chain: chain}).Execute(ctx, step, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if chain.queryCalls != 1 || chain.queryPath != "deployment.get" {
		t.Fatalf("query call = count %d path %q", chain.queryCalls, chain.queryPath)
	}
	if want := map[string]string{"owner": "akash1owner", "dseq": "42"}; !reflect.DeepEqual(chain.queryParams, want) {
		t.Fatalf("query params = %#v, want %#v", chain.queryParams, want)
	}
	if chain.queryContext.Value(contextKey("request")) != "query-ctx" {
		t.Fatal("query did not receive the executor context")
	}
	if result.Status != "success" || result.Output["dseq"] != "42" || result.Output["state"] != "active" {
		t.Fatalf("result = %+v", result)
	}
	if string(result.RawResult) != string(chain.queryResult) {
		t.Fatalf("raw result = %s, want %s", result.RawResult, chain.queryResult)
	}
}

func TestQueryExecutorBoundaryFailures(t *testing.T) {
	state := workflow.NewRunState("run-1", "inspect", "akash1owner", nil)
	step := workflow.StepDef{Name: "query", Type: workflow.StepQuery, Query: "deployment.get"}

	t.Run("missing client", func(t *testing.T) {
		result, err := (&QueryExecutor{}).Execute(context.Background(), step, state)
		assertFailedStep(t, result, err, "chain client not available")
	})

	t.Run("invalid parameter template does not query", func(t *testing.T) {
		chain := &recordingStepChain{}
		invalid := step
		invalid.Params = map[string]string{"dseq": "{{"}
		result, err := (&QueryExecutor{chain: chain}).Execute(context.Background(), invalid, state)
		assertFailedStep(t, result, err, "resolve params")
		if chain.queryCalls != 0 {
			t.Fatalf("invalid params reached chain query %d times", chain.queryCalls)
		}
	})

	t.Run("query error is preserved", func(t *testing.T) {
		queryErr := errors.New("rpc unavailable")
		chain := &recordingStepChain{queryErr: queryErr}
		result, err := (&QueryExecutor{chain: chain}).Execute(context.Background(), step, state)
		if !errors.Is(err, queryErr) || result.Status != "failed" || result.Error != queryErr.Error() {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
	})

	t.Run("unstructured result remains successful", func(t *testing.T) {
		chain := &recordingStepChain{queryResult: json.RawMessage(`[]`)}
		result, err := (&QueryExecutor{chain: chain}).Execute(context.Background(), step, state)
		if err != nil || result.Status != "success" || result.Output != nil || string(result.RawResult) != "[]" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
	})
}

func TestTxExecutorUsesResolvedInputsAndReceipt(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("request"), "tx-ctx")
	chain := &recordingStepChain{txResult: &TxResult{
		TxHash: "ABC123",
		Height: 99,
		Data:   json.RawMessage(`{"deployment":{"id":{"dseq":"42"}}}`),
	}}
	state := workflow.NewRunState("run-1", "deploy", "akash1owner", map[string]any{"deposit": "0.5"})
	step := workflow.StepDef{
		Name: "create",
		Type: workflow.StepTx,
		Msg:  "deployment.create",
		Params: map[string]string{
			"owner":   "{{ .Account }}",
			"deposit": "{{ .Params.deposit }}",
		},
		Output: map[string]string{"dseq": "{{ .Result.deployment.id.dseq }}"},
	}

	result, err := (&TxExecutor{chain: chain}).Execute(ctx, step, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if chain.broadcastCalls != 1 || chain.msgType != "deployment.create" {
		t.Fatalf("broadcast = count %d type %q", chain.broadcastCalls, chain.msgType)
	}
	if want := map[string]string{"owner": "akash1owner", "deposit": "0.5"}; !reflect.DeepEqual(chain.txParams, want) {
		t.Fatalf("tx params = %#v, want %#v", chain.txParams, want)
	}
	if chain.txContext.Value(contextKey("request")) != "tx-ctx" {
		t.Fatal("broadcast did not receive the executor context")
	}
	if result.Status != "success" || result.TxHash != "ABC123" || result.Height != 99 || result.Output["dseq"] != "42" {
		t.Fatalf("result = %+v", result)
	}
}

func TestTxExecutorBoundaryFailures(t *testing.T) {
	state := workflow.NewRunState("run-1", "deploy", "akash1owner", nil)
	step := workflow.StepDef{Name: "tx", Type: workflow.StepTx, Msg: "deployment.close"}

	t.Run("missing client", func(t *testing.T) {
		result, err := (&TxExecutor{}).Execute(context.Background(), step, state)
		assertFailedStep(t, result, err, "chain client not available")
	})

	t.Run("invalid parameter template does not broadcast", func(t *testing.T) {
		chain := &recordingStepChain{}
		invalid := step
		invalid.Params = map[string]string{"dseq": "{{"}
		result, err := (&TxExecutor{chain: chain}).Execute(context.Background(), invalid, state)
		assertFailedStep(t, result, err, "resolve params")
		if chain.broadcastCalls != 0 {
			t.Fatalf("invalid params reached broadcaster %d times", chain.broadcastCalls)
		}
	})

	t.Run("broadcast error is preserved", func(t *testing.T) {
		broadcastErr := errors.New("checktx rejected")
		chain := &recordingStepChain{txErr: broadcastErr}
		result, err := (&TxExecutor{chain: chain}).Execute(context.Background(), step, state)
		if !errors.Is(err, broadcastErr) || result.Status != "failed" || result.Error != broadcastErr.Error() {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
	})
}

func TestShellExecutorCapturesStructuredOutputAndFailures(t *testing.T) {
	state := workflow.NewRunState("run-1", "custom", "akash1owner", map[string]any{"value": 42})

	t.Run("structured stdout", func(t *testing.T) {
		step := workflow.StepDef{
			Name:    "inspect",
			Type:    workflow.StepShell,
			Command: `printf '{"value":{{ .Params.value }}}'`,
			Output:  map[string]string{"value": "{{ .Result.value }}"},
		}
		result, err := (&ShellExecutor{}).Execute(context.Background(), step, state)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.Status != "success" || result.Output["value"] != "42" || string(result.RawResult) != `{"value":42}` {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("template error does not execute", func(t *testing.T) {
		step := workflow.StepDef{Name: "broken", Type: workflow.StepShell, Command: "{{"}
		result, err := (&ShellExecutor{}).Execute(context.Background(), step, state)
		assertFailedStep(t, result, err, "resolve command")
	})

	t.Run("nonzero exit captures stderr", func(t *testing.T) {
		step := workflow.StepDef{Name: "broken", Type: workflow.StepShell, Command: `printf 'specific failure' >&2; exit 7`}
		result, err := (&ShellExecutor{}).Execute(context.Background(), step, state)
		if err == nil || result.Status != "failed" || !strings.Contains(result.Error, "specific failure") {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		step := workflow.StepDef{Name: "cancelled", Type: workflow.StepShell, Command: "sleep 1"}
		result, err := (&ShellExecutor{}).Execute(ctx, step, state)
		if err == nil || result.Status != "failed" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
	})
}

type contextKey string

func assertFailedStep(t *testing.T, result *workflow.StepResult, err error, marker string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), marker) {
		t.Fatalf("error = %v, want containing %q", err, marker)
	}
	if result == nil || result.Status != "failed" || result.Error == "" {
		t.Fatalf("result = %+v, want failed step with a concrete error", result)
	}
}

type replacementExecutor struct {
	stepType workflow.StepType
	id       string
}

func (executor *replacementExecutor) Type() workflow.StepType { return executor.stepType }

func (executor *replacementExecutor) Execute(context.Context, workflow.StepDef, *workflow.RunState) (*workflow.StepResult, error) {
	return &workflow.StepResult{Status: executor.id}, nil
}

func TestRegistryProvidesEveryBuiltinAndSupportsSafeReplacement(t *testing.T) {
	registry := NewRegistry(&recordingStepChain{}, &fakeWorkflowProvider{})
	for _, stepType := range []workflow.StepType{
		workflow.StepTx,
		workflow.StepQuery,
		workflow.StepWait,
		workflow.StepPrompt,
		workflow.StepProvider,
		workflow.StepOutput,
		workflow.StepShell,
		workflow.StepCheck,
	} {
		executor, err := registry.Get(stepType)
		if err != nil {
			t.Errorf("Get(%q): %v", stepType, err)
			continue
		}
		if executor.Type() != stepType {
			t.Errorf("Get(%q).Type() = %q", stepType, executor.Type())
		}
	}

	if _, err := registry.Get(workflow.StepType("unknown")); err == nil || !strings.Contains(err.Error(), `no executor registered`) {
		t.Fatalf("unknown type error = %v", err)
	}

	replacement := &replacementExecutor{stepType: workflow.StepOutput, id: "replacement"}
	registry.Register(replacement)
	got, err := registry.Get(workflow.StepOutput)
	if err != nil || got != replacement {
		t.Fatalf("replacement = %T %p, error = %v; want %p", got, got, err, replacement)
	}
}

func TestRegistryConcurrentReadersAndWriters(t *testing.T) {
	registry := NewRegistry(nil, nil)
	const workers = 32

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			registry.Register(&replacementExecutor{stepType: workflow.StepOutput, id: fmt.Sprintf("writer-%d", id)})
		}(i)
		go func() {
			defer wg.Done()
			if executor, err := registry.Get(workflow.StepOutput); err != nil || executor == nil {
				t.Errorf("concurrent Get: executor=%v error=%v", executor, err)
			}
		}()
	}
	wg.Wait()

	if executor, err := registry.Get(workflow.StepOutput); err != nil || executor == nil {
		t.Fatalf("final Get: executor=%v error=%v", executor, err)
	}
}
