package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

// TxExecutor broadcasts a transaction to the chain.
type TxExecutor struct {
	chain ChainClient
}

func (e *TxExecutor) Type() workflow.StepType { return workflow.StepTx }

func (e *TxExecutor) Execute(ctx context.Context, step workflow.StepDef, state *workflow.RunState) (*workflow.StepResult, error) {
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

	// Resolve template params.
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

	result, err := e.chain.BroadcastTx(ctx, step.Msg, params)
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    err.Error(),
			Duration: time.Since(start),
		}, err
	}

	// Extract outputs from the tx result.
	outputs, err := workflow.ExtractOutputs(step.Output, result.Data, state)
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    err.Error(),
			Duration: time.Since(start),
			TxHash:   result.TxHash,
			Height:   result.Height,
		}, fmt.Errorf("extract outputs: %w", err)
	}

	return &workflow.StepResult{
		Name:      step.Name,
		Type:      step.Type,
		Status:    "success",
		Output:    outputs,
		RawResult: result.Data,
		TxHash:    result.TxHash,
		Height:    result.Height,
		Duration:  time.Since(start),
	}, nil
}

// Ensure the raw data can be marshalled for output extraction.
func txResultToJSON(data json.RawMessage) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}

	return m
}
