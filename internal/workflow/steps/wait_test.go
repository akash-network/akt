package steps

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

type waitTestChain struct {
	queries int
	result  json.RawMessage
}

func (*waitTestChain) BroadcastTx(context.Context, string, map[string]string) (*TxResult, error) {
	return nil, nil
}

func (c *waitTestChain) Query(context.Context, string, map[string]string) (json.RawMessage, error) {
	c.queries++
	if c.result != nil {
		return c.result, nil
	}
	return json.RawMessage(`{"ready":true}`), nil
}

func TestWaitRejectsInvalidTimeoutBeforeQuery(t *testing.T) {
	for _, timeout := range []string{"eventually", "0s", "-1s"} {
		t.Run(timeout, func(t *testing.T) {
			chain := &waitTestChain{}
			step := workflow.StepDef{
				Name:    "wait",
				Type:    workflow.StepWait,
				Query:   "status",
				Timeout: timeout,
				Until:   "true",
			}

			result, err := (&WaitExecutor{chain: chain}).Execute(
				context.Background(),
				step,
				workflow.NewRunState("id", "test", "", nil),
			)
			if err == nil || !strings.Contains(err.Error(), "timeout") {
				t.Fatalf("timeout %q error = %v", timeout, err)
			}
			if result == nil || result.Status != "failed" {
				t.Fatalf("timeout %q result = %+v", timeout, result)
			}
			if chain.queries != 0 {
				t.Errorf("timeout %q reached the query client %d times", timeout, chain.queries)
			}
		})
	}
}

func TestWaitReportsProgressAndUsesUserFacingTimeout(t *testing.T) {
	chain := &waitTestChain{result: json.RawMessage(`{"bids":[]}`)}
	step := workflow.StepDef{
		Name:         "wait-for-bids",
		Type:         workflow.StepWait,
		Query:        "market.bids",
		Timeout:      "5ms",
		Until:        `{{ ge (len .Result.bids) 1 }}`,
		TimeoutError: "no bids received (at least 1 required)",
	}

	var progress []WaitProgress
	result, err := NewWaitExecutor(chain, func(update WaitProgress) {
		progress = append(progress, update)
	}).Execute(context.Background(), step, workflow.NewRunState("id", "deploy", "", nil))

	if err == nil {
		t.Fatal("wait returned nil error after its timeout")
	}
	want := "no bids received (at least 1 required); timed out after 5ms"
	if err.Error() != want {
		t.Fatalf("wait error = %q, want %q", err, want)
	}
	if result == nil || result.Error != want {
		t.Fatalf("wait result = %+v, want user-facing timeout", result)
	}
	if strings.Contains(result.Error, ".Result") || strings.Contains(result.Error, "{{") {
		t.Fatalf("wait exposed its template condition: %q", result.Error)
	}
	if len(progress) == 0 {
		t.Fatal("wait emitted no progress before timing out")
	}
	if got := progress[0]; got.Query != "market.bids" || got.Remaining > 5*time.Millisecond {
		t.Fatalf("first progress = %+v", got)
	}
}
