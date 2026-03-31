package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

// ShellExecutor runs a shell command. For custom workflows.
type ShellExecutor struct{}

func (e *ShellExecutor) Type() workflow.StepType { return workflow.StepShell }

func (e *ShellExecutor) Execute(ctx context.Context, step workflow.StepDef, state *workflow.RunState) (*workflow.StepResult, error) {
	start := time.Now()

	cmdStr, err := workflow.ResolveTemplate(step.Command, state)
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    err.Error(),
			Duration: time.Since(start),
		}, fmt.Errorf("resolve command: %w", err)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    fmt.Sprintf("%s: %s", err, stderr.String()),
			Duration: time.Since(start),
		}, err
	}

	// Capture stdout as raw result.
	raw := json.RawMessage(stdout.Bytes())

	outputs, _ := workflow.ExtractOutputs(step.Output, raw, state)

	return &workflow.StepResult{
		Name:      step.Name,
		Type:      step.Type,
		Status:    "success",
		Output:    outputs,
		RawResult: raw,
		Duration:  time.Since(start),
	}, nil
}
