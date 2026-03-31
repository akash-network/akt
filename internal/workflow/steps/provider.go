package steps

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

// ProviderExecutor handles provider gateway calls (send-manifest, lease-status, etc).
type ProviderExecutor struct {
	provider ProviderClient
}

func (e *ProviderExecutor) Type() workflow.StepType { return workflow.StepProvider }

func (e *ProviderExecutor) Execute(ctx context.Context, step workflow.StepDef, state *workflow.RunState) (*workflow.StepResult, error) {
	start := time.Now()

	if e.provider == nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    "provider client not available",
			Duration: time.Since(start),
		}, fmt.Errorf("provider client not available")
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

	switch step.Action {
	case "send-manifest":
		dseq, _ := strconv.ParseUint(params["dseq"], 10, 64)
		sdl := []byte(params["sdl"])
		err = e.provider.SendManifest(ctx, params["provider"], dseq, sdl)
	case "lease-status":
		dseq, _ := strconv.ParseUint(params["dseq"], 10, 64)
		var data []byte
		data, err = e.provider.LeaseStatus(ctx, params["provider"], dseq)
		if err == nil {
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
	default:
		err = fmt.Errorf("unknown provider action %q", step.Action)
	}

	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    err.Error(),
			Duration: time.Since(start),
		}, err
	}

	return &workflow.StepResult{
		Name:     step.Name,
		Type:     step.Type,
		Status:   "success",
		Duration: time.Since(start),
	}, nil
}
