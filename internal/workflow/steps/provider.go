package steps

import (
	"context"
	"encoding/json"
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
		dseq, parseErr := parseProviderDSeq(params["dseq"])
		if parseErr != nil {
			return failedProviderResult(step, start, parseErr), parseErr
		}
		sdl := []byte(params["sdl"])
		err = e.provider.SendManifest(ctx, params["provider"], dseq, sdl)
	case "send-manifest-to-active-leases":
		dseq, parseErr := parseProviderDSeq(params["dseq"])
		if parseErr != nil {
			return failedProviderResult(step, start, parseErr), parseErr
		}

		providers, sendErr := e.provider.SendManifestToActiveLeases(ctx, dseq, []byte(params["sdl"]))
		outputs := map[string]any{
			"providers": providers,
			"count":     len(providers),
		}
		raw, marshalErr := json.Marshal(outputs)
		if marshalErr != nil {
			return failedProviderResult(step, start, marshalErr), fmt.Errorf("marshal provider result: %w", marshalErr)
		}
		if sendErr != nil {
			result := failedProviderResult(step, start, sendErr)
			result.Output = outputs
			result.RawResult = raw

			return result, sendErr
		}

		return &workflow.StepResult{
			Name:      step.Name,
			Type:      step.Type,
			Status:    "success",
			Output:    outputs,
			RawResult: raw,
			Duration:  time.Since(start),
		}, nil
	case "lease-status":
		dseq, parseErr := parseProviderDSeq(params["dseq"])
		if parseErr != nil {
			return failedProviderResult(step, start, parseErr), parseErr
		}
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

func parseProviderDSeq(value string) (uint64, error) {
	dseq, err := strconv.ParseUint(value, 10, 64)
	if err != nil || dseq == 0 {
		if err != nil {
			return 0, fmt.Errorf("invalid dseq %q: %w", value, err)
		}

		return 0, fmt.Errorf("invalid dseq %q: must be greater than zero", value)
	}

	return dseq, nil
}

func failedProviderResult(step workflow.StepDef, start time.Time, err error) *workflow.StepResult {
	return &workflow.StepResult{
		Name:     step.Name,
		Type:     step.Type,
		Status:   "failed",
		Error:    err.Error(),
		Duration: time.Since(start),
	}
}
