package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultWaitTimeout  = 5 * time.Minute
)

// WaitExecutor polls a query until a condition is met or timeout expires.
type WaitExecutor struct {
	chain    ChainClient
	progress WaitProgressReporter
}

// WaitProgress describes one successful poll of a wait step.
type WaitProgress struct {
	Step      string
	Query     string
	Elapsed   time.Duration
	Remaining time.Duration
	Result    json.RawMessage
}

// WaitProgressReporter receives successful wait polls. It is optional; the
// workflow engine itself never writes progress to a terminal.
type WaitProgressReporter func(WaitProgress)

// NewWaitExecutor returns a wait executor with optional progress reporting.
func NewWaitExecutor(chain ChainClient, progress WaitProgressReporter) *WaitExecutor {
	return &WaitExecutor{chain: chain, progress: progress}
}

func (e *WaitExecutor) Type() workflow.StepType { return workflow.StepWait }

func (e *WaitExecutor) Execute(ctx context.Context, step workflow.StepDef, state *workflow.RunState) (*workflow.StepResult, error) {
	start := time.Now()

	// Resolve timeout from template.
	timeout := defaultWaitTimeout
	if step.Timeout != "" {
		resolved, err := workflow.ResolveTemplate(step.Timeout, state)
		if err != nil {
			return failedWaitResult(step, start, fmt.Errorf("resolve wait timeout: %w", err))
		}
		duration, err := time.ParseDuration(resolved)
		if err != nil {
			return failedWaitResult(step, start, fmt.Errorf("invalid wait timeout %q: %w", resolved, err))
		}
		if duration <= 0 {
			return failedWaitResult(step, start, fmt.Errorf("wait timeout must be greater than zero"))
		}
		timeout = duration
	}
	timeoutError := fmt.Sprintf("condition was not met; timed out after %s", timeout)
	if step.TimeoutError != "" {
		resolved, err := workflow.ResolveTemplate(step.TimeoutError, state)
		if err != nil {
			return failedWaitResult(step, start, fmt.Errorf("resolve wait timeout error: %w", err))
		}
		resolved = strings.TrimSpace(resolved)
		if resolved == "" {
			return failedWaitResult(step, start, fmt.Errorf("wait timeout error must not be empty"))
		}
		timeoutError = fmt.Sprintf("%s; timed out after %s", resolved, timeout)
	}

	if e.chain == nil {
		return failedWaitResult(step, start, fmt.Errorf("chain client not available"))
	}

	params, err := workflow.ResolveParams(step.Params, state)
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    err.Error(),
			Duration: time.Since(start),
		}, fmt.Errorf("resolve params: %w", err)
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		data, err := e.chain.Query(ctx, step.Query, params)
		if err == nil {
			if e.progress != nil {
				elapsed := time.Since(start)
				remaining := timeout - elapsed
				if remaining < 0 {
					remaining = 0
				}
				e.progress(WaitProgress{
					Step:      step.Name,
					Query:     step.Query,
					Elapsed:   elapsed,
					Remaining: remaining,
					Result:    data,
				})
			}

			// Check the until condition.
			met, evalErr := workflow.EvalCondition(step.Until, data, state)
			if evalErr == nil && met {
				outputs, _ := workflow.ExtractOutputs(step.Output, data, state)

				return &workflow.StepResult{
					Name:      step.Name,
					Type:      step.Type,
					Status:    "success",
					Output:    outputs,
					RawResult: data,
					Duration:  time.Since(start),
				}, nil
			}
		}

		select {
		case <-ctx.Done():
			return &workflow.StepResult{
				Name:     step.Name,
				Type:     step.Type,
				Status:   "failed",
				Error:    "context cancelled",
				Duration: time.Since(start),
			}, ctx.Err()
		case <-deadline:
			return &workflow.StepResult{
				Name:     step.Name,
				Type:     step.Type,
				Status:   "failed",
				Error:    timeoutError,
				Duration: time.Since(start),
			}, errors.New(timeoutError)
		case <-ticker.C:
			// Poll again.
		}
	}
}

func failedWaitResult(step workflow.StepDef, start time.Time, err error) (*workflow.StepResult, error) {
	return &workflow.StepResult{
		Name:     step.Name,
		Type:     step.Type,
		Status:   "failed",
		Error:    err.Error(),
		Duration: time.Since(start),
	}, err
}
