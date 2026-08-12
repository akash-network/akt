package steps

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

// OutputExecutor renders a Go template and writes it to Out (stdout by
// default). The rendered text is also recorded in the step result's Output
// under "text" so callers running in structured output modes (e.g. JSONL)
// can redirect Out and still access the rendered content.
type OutputExecutor struct {
	// Out is the destination for rendered templates. Nil means os.Stdout.
	Out io.Writer
}

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

	out := e.Out
	if out == nil {
		out = os.Stdout
	}

	written, err := io.WriteString(out, rendered)
	if err == nil && written != len(rendered) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    err.Error(),
			Duration: time.Since(start),
		}, fmt.Errorf("write output: %w", err)
	}

	return &workflow.StepResult{
		Name:     step.Name,
		Type:     step.Type,
		Status:   "success",
		Output:   map[string]any{"text": rendered},
		Duration: time.Since(start),
	}, nil
}
