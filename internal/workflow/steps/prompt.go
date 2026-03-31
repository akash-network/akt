package steps

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"pkg.akt.dev/akt/internal/workflow"
)

// PromptExecutor handles interactive user input steps (e.g. bid selection).
type PromptExecutor struct{}

func (e *PromptExecutor) Type() workflow.StepType { return workflow.StepPrompt }

func (e *PromptExecutor) Execute(ctx context.Context, step workflow.StepDef, state *workflow.RunState) (*workflow.StepResult, error) {
	start := time.Now()

	// Resolve the selection mode.
	mode, err := workflow.ResolveTemplate(step.Mode, state)
	if err != nil {
		mode = "interactive"
	}

	// Resolve the data reference to get the items to select from.
	dataRaw, err := workflow.ResolveTemplate(step.Data, state)
	if err != nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    fmt.Sprintf("resolve data: %s", err),
			Duration: time.Since(start),
		}, fmt.Errorf("resolve prompt data: %w", err)
	}

	// Parse the data as a list of items.
	items, err := workflow.ParseList(dataRaw)
	if err != nil || len(items) == 0 {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    "no items to select from",
			Duration: time.Since(start),
		}, fmt.Errorf("no items to select from")
	}

	var selected map[string]any

	switch {
	case mode == "interactive":
		selected, err = interactiveSelect(items, step.Display.Columns)
	case mode == "cheapest":
		selected = selectCheapest(items)
	case strings.HasPrefix(mode, "provider="):
		addr := strings.TrimPrefix(mode, "provider=")
		selected = selectByProvider(items, addr)
	default:
		selected, err = interactiveSelect(items, step.Display.Columns)
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

	if selected == nil {
		return &workflow.StepResult{
			Name:     step.Name,
			Type:     step.Type,
			Status:   "failed",
			Error:    "no item selected",
			Duration: time.Since(start),
		}, fmt.Errorf("no item selected")
	}

	outputs := map[string]any{"selected": selected}

	return &workflow.StepResult{
		Name:     step.Name,
		Type:     step.Type,
		Status:   "success",
		Output:   outputs,
		Duration: time.Since(start),
	}, nil
}

func interactiveSelect(items []map[string]any, columns []string) (map[string]any, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items")
	}

	// Display items.
	for i, item := range items {
		parts := make([]string, 0, len(columns))
		for _, col := range columns {
			if v, ok := item[col]; ok {
				parts = append(parts, fmt.Sprintf("%v", v))
			}
		}

		fmt.Printf("  %d  %s\n", i+1, strings.Join(parts, "  "))
	}

	fmt.Printf("\nSelect [1-%d]: ", len(items))

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(items) {
		return nil, fmt.Errorf("invalid selection: %q", input)
	}

	return items[n-1], nil
}

func selectCheapest(items []map[string]any) map[string]any {
	if len(items) == 0 {
		return nil
	}

	// Simple: return first item. Real implementation would compare price fields.
	// TODO: implement proper price comparison when price types are defined.
	return items[0]
}

func selectByProvider(items []map[string]any, addr string) map[string]any {
	for _, item := range items {
		if p, ok := item["provider"]; ok && fmt.Sprintf("%v", p) == addr {
			return item
		}
	}

	return nil
}
