package steps

import (
	"context"
	"fmt"
	"os"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

// OutputExecutor renders a Go template to stdout.
type OutputExecutor struct{}

func (e *OutputExecutor) Type() workflow.StepType { return workflow.StepOutput }

func (e *OutputExecutor) Execute(ctx context.Context, step workflow.StepDef, state *workflow.RunState) (*workflow.StepResult, error) {
	start := time.Now()

	rendered, err := workflow.ResolveTemplate(step.Template, state)
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    err.Error(),
			Duration: time.Since(start),
		}, fmt.Errorf("render output template: %w", err)
	}

	fmt.Fprint(os.Stdout, rendered)

	return &workflow.StepResult{
		Name:     step.Name,
		Type:     step.Type,
		Status:   "success",
		Duration: time.Since(start),
	}, nil
}
