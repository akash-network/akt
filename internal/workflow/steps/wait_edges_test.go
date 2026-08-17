package steps

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

type adversarialWaitChain struct {
	calls  int
	path   string
	params map[string]string
	result json.RawMessage
	err    error
	delay  time.Duration
}

func (*adversarialWaitChain) BroadcastTx(context.Context, string, map[string]string) (*TxResult, error) {
	return nil, nil
}

func (chain *adversarialWaitChain) Query(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	chain.calls++
	chain.path = path
	chain.params = params
	if chain.delay > 0 {
		time.Sleep(chain.delay)
	}
	return chain.result, chain.err
}

func TestWaitExecutorImmediateSuccessExtractsOutputs(t *testing.T) {
	chain := &adversarialWaitChain{result: json.RawMessage(`{"bids":[{"provider":"akash1provider"}]}`)}
	state := workflow.NewRunState("run-1", "deploy", "akash1owner", map[string]any{"dseq": 42})
	step := workflow.StepDef{
		Name:    "wait-for-bids",
		Type:    workflow.StepWait,
		Query:   "market.bids",
		Timeout: "1s",
		Params: map[string]string{
			"owner": "{{ .Account }}",
			"dseq":  "{{ .Params.dseq }}",
		},
		Until:  `{{ ge (len .Result.bids) 1 }}`,
		Output: map[string]string{"provider": "{{ (index .Result.bids 0).provider }}"},
	}

	var progress []WaitProgress
	result, err := NewWaitExecutor(chain, func(update WaitProgress) {
		progress = append(progress, update)
	}).Execute(context.Background(), step, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "success" || result.Output["provider"] != "akash1provider" || string(result.RawResult) != string(chain.result) {
		t.Fatalf("result = %+v", result)
	}
	if chain.calls != 1 || chain.path != "market.bids" {
		t.Fatalf("query = calls %d path %q", chain.calls, chain.path)
	}
	if want := map[string]string{"owner": "akash1owner", "dseq": "42"}; !reflect.DeepEqual(chain.params, want) {
		t.Fatalf("params = %#v, want %#v", chain.params, want)
	}
	if len(progress) != 1 || progress[0].Step != "wait-for-bids" || progress[0].Query != "market.bids" || !json.Valid(progress[0].Result) {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestWaitExecutorBoundaryFailuresBeforePolling(t *testing.T) {
	state := workflow.NewRunState("run-1", "deploy", "akash1owner", nil)
	base := workflow.StepDef{
		Name:    "wait",
		Type:    workflow.StepWait,
		Query:   "status",
		Timeout: "10ms",
		Until:   "true",
	}

	t.Run("timeout template", func(t *testing.T) {
		chain := &adversarialWaitChain{}
		step := base
		step.Timeout = "{{"
		result, err := (&WaitExecutor{chain: chain}).Execute(context.Background(), step, state)
		assertFailedStep(t, result, err, "resolve wait timeout")
		if chain.calls != 0 {
			t.Fatalf("invalid timeout reached query %d times", chain.calls)
		}
	})

	t.Run("timeout error template", func(t *testing.T) {
		chain := &adversarialWaitChain{}
		step := base
		step.TimeoutError = "{{"
		result, err := (&WaitExecutor{chain: chain}).Execute(context.Background(), step, state)
		assertFailedStep(t, result, err, "resolve wait timeout error")
		if chain.calls != 0 {
			t.Fatalf("invalid timeout explanation reached query %d times", chain.calls)
		}
	})

	t.Run("empty timeout error", func(t *testing.T) {
		chain := &adversarialWaitChain{}
		step := base
		step.TimeoutError = "   "
		result, err := (&WaitExecutor{chain: chain}).Execute(context.Background(), step, state)
		assertFailedStep(t, result, err, "must not be empty")
		if chain.calls != 0 {
			t.Fatalf("empty timeout explanation reached query %d times", chain.calls)
		}
	})

	t.Run("missing client", func(t *testing.T) {
		result, err := (&WaitExecutor{}).Execute(context.Background(), base, state)
		assertFailedStep(t, result, err, "chain client not available")
	})

	t.Run("invalid params", func(t *testing.T) {
		chain := &adversarialWaitChain{}
		step := base
		step.Params = map[string]string{"dseq": "{{"}
		result, err := (&WaitExecutor{chain: chain}).Execute(context.Background(), step, state)
		assertFailedStep(t, result, err, "resolve params")
		if chain.calls != 0 {
			t.Fatalf("invalid params reached query %d times", chain.calls)
		}
	})
}

func TestWaitExecutorCancellationAndQueryErrorsAreBounded(t *testing.T) {
	state := workflow.NewRunState("run-1", "deploy", "akash1owner", nil)

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		chain := &adversarialWaitChain{result: json.RawMessage(`{"ready":false}`)}
		result, err := (&WaitExecutor{chain: chain}).Execute(ctx, workflow.StepDef{
			Name: "wait", Type: workflow.StepWait, Query: "status", Timeout: "1s", Until: `{{ .Result.ready }}`,
		}, state)
		if !errors.Is(err, context.Canceled) || result.Status != "failed" || result.Error != "context cancelled" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
		if chain.calls != 1 {
			t.Fatalf("cancelled wait queried %d times, want one bounded initial observation", chain.calls)
		}
	})

	t.Run("query errors time out without false progress", func(t *testing.T) {
		chain := &adversarialWaitChain{err: errors.New("rpc unavailable")}
		var progress []WaitProgress
		result, err := NewWaitExecutor(chain, func(update WaitProgress) {
			progress = append(progress, update)
		}).Execute(context.Background(), workflow.StepDef{
			Name: "wait", Type: workflow.StepWait, Query: "status", Timeout: "5ms", Until: "true",
		}, state)
		if err == nil || !strings.Contains(err.Error(), "timed out after 5ms") || result.Status != "failed" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
		if len(progress) != 0 {
			t.Fatalf("failed queries emitted progress: %+v", progress)
		}
	})

	t.Run("late false result clamps remaining time", func(t *testing.T) {
		chain := &adversarialWaitChain{
			result: json.RawMessage(`{"ready":false}`),
			delay:  5 * time.Millisecond,
		}
		var progress []WaitProgress
		result, err := NewWaitExecutor(chain, func(update WaitProgress) {
			progress = append(progress, update)
		}).Execute(context.Background(), workflow.StepDef{
			Name: "wait", Type: workflow.StepWait, Query: "status", Timeout: "1ms", Until: `{{ .Result.ready }}`,
		}, state)
		if err == nil || !strings.Contains(err.Error(), "timed out after 1ms") || result.Status != "failed" {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
		if len(progress) != 1 || progress[0].Remaining != 0 {
			t.Fatalf("progress = %+v", progress)
		}
	})
}
