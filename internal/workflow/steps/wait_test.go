package steps

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/workflow"
)

type waitTestChain struct {
	queries int
}

func (*waitTestChain) BroadcastTx(context.Context, string, map[string]string) (*TxResult, error) {
	return nil, nil
}

func (c *waitTestChain) Query(context.Context, string, map[string]string) (json.RawMessage, error) {
	c.queries++
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
