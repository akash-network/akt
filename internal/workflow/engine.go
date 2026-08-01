package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// StepExecutor is the interface step implementations satisfy.
// Defined here to avoid a circular import with the steps package.
type StepExecutor interface {
	Type() StepType
	Execute(ctx context.Context, step StepDef, state *RunState) (*StepResult, error)
}

// StepRegistry looks up executors by step type.
type StepRegistry interface {
	Get(t StepType) (StepExecutor, error)
}

// Logger is called after each step completes. The workflow engine uses this
// to write entries to the action log. Nil logger means no logging.
type Logger interface {
	LogStep(workflowID string, stepIndex int, result *StepResult)
}

// Engine executes workflow definitions step by step.
type Engine struct {
	registry StepRegistry
	logger   Logger
}

// NewEngine creates a workflow engine with the given step registry and logger.
func NewEngine(registry StepRegistry, logger Logger) *Engine {
	return &Engine{
		registry: registry,
		logger:   logger,
	}
}

// Run executes a workflow definition with the given parameters.
// It returns the final run state containing all step results.
func (e *Engine) Run(ctx context.Context, wf *WorkflowDef, account string, params map[string]any) (*RunState, error) {
	workflowID := GenerateWorkflowID()
	state := NewRunState(workflowID, wf.Name, account, params)

	for i, step := range wf.Steps {
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		default:
		}

		result, err := e.executeStep(ctx, step, state)
		if result != nil {
			state.SetStepResult(step.Name, result)
		}

		// Log the step result.
		if e.logger != nil && result != nil {
			e.logger.LogStep(workflowID, i, result)
		}

		if err != nil {
			switch step.OnError {
			case OnErrorContinue:
				// Log and continue.
				continue
			case OnErrorSkip:
				// Skip silently.
				continue
			default:
				// OnErrorAbort or unset: abort the workflow.
				return state, fmt.Errorf("step %q failed: %w", step.Name, err)
			}
		}

		// For check steps, handle on-fail.
		if step.Type == StepCheck && result != nil && result.Status == "skipped" {
			continue
		}
	}

	return state, nil
}

// executeStep runs a single step with optional retry logic.
func (e *Engine) executeStep(ctx context.Context, step StepDef, state *RunState) (*StepResult, error) {
	executor, err := e.registry.Get(step.Type)
	if err != nil {
		return &StepResult{
			Name:   step.Name,
			Type:   step.Type,
			Status: "failed",
			Error:  err.Error(),
		}, err
	}

	maxAttempts := 1
	var retryDelay time.Duration

	if step.Retry != nil && step.Retry.Max > 0 {
		maxAttempts = step.Retry.Max
		if step.Retry.Delay != "" {
			retryDelay, _ = time.ParseDuration(step.Retry.Delay)
		}

		if retryDelay == 0 {
			retryDelay = 2 * time.Second
		}
	}

	var lastResult *StepResult
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return lastResult, ctx.Err()
			case <-time.After(retryDelay):
			}
		}

		lastResult, lastErr = executor.Execute(ctx, step, state)
		if lastErr == nil {
			return lastResult, nil
		}
	}

	return lastResult, lastErr
}

// GenerateWorkflowID creates a short random hex ID for correlating one run's
// output and action-log entries.
func GenerateWorkflowID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}
