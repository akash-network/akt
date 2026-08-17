package steps

import (
	"context"
	"fmt"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

// CheckExecutor asserts a condition. If the condition is false, the step
// either skips or aborts based on on-fail.
type CheckExecutor struct{}

func (e *CheckExecutor) Type() workflow.StepType { return workflow.StepCheck }

func (e *CheckExecutor) Execute(ctx context.Context, step workflow.StepDef, state *workflow.RunState) (*workflow.StepResult, error) {
	start := time.Now()

	met, err := workflow.EvalCondition(step.Condition, nil, state)
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    fmt.Sprintf("eval condition: %s", err),
			Duration: time.Since(start),
		}, fmt.Errorf("eval condition %q: %w", step.Condition, err)
	}

	if met {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "success",
			Duration: time.Since(start),
		}, nil
	}

	// Condition not met.
	status := "failed"
	if step.OnFail == workflow.OnErrorSkip {
		status = "skipped"
	}

	result := &workflow.StepResult{
		Name:     step.Name,
		Type:     step.Type,
		Status:   status,
		Error:    fmt.Sprintf("condition not met: %s", step.Condition),
		Duration: time.Since(start),
	}
	if status == "skipped" {
		return result, nil
	}

	return result, fmt.Errorf("check failed: condition %q not met", step.Condition)
}
