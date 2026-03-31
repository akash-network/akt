package steps

import (
	"context"
	"fmt"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

// QueryExecutor executes a chain query.
type QueryExecutor struct {
	chain ChainClient
}

func (e *QueryExecutor) Type() workflow.StepType { return workflow.StepQuery }

func (e *QueryExecutor) Execute(ctx context.Context, step workflow.StepDef, state *workflow.RunState) (*workflow.StepResult, error) {
	start := time.Now()

	if e.chain == nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    "chain client not available",
			Duration: time.Since(start),
		}, fmt.Errorf("chain client not available")
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

	data, err := e.chain.Query(ctx, step.Query, params)
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    err.Error(),
			Duration: time.Since(start),
		}, err
	}

	outputs, err := workflow.ExtractOutputs(step.Output, data, state)
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    err.Error(),
			Duration: time.Since(start),
		}, fmt.Errorf("extract outputs: %w", err)
	}

	return &workflow.StepResult{
		Name:      step.Name,
		Type:      step.Type,
		Status:    "success",
		Output:    outputs,
		RawResult: data,
		Duration:  time.Since(start),
	}, nil
}
