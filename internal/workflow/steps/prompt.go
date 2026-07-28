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

	// Expose the full selected item plus its flattened identity fields
	// (dseq/gseq/oseq/provider/price) so downstream steps can reference
	// discrete values, e.g. {{ (index .Steps "select-bid").provider }}.
	outputs := map[string]any{"selected": selected}
	for k, v := range bidIdentity(selected) {
		outputs[k] = v
	}

	return &workflow.StepResult{
		Name:     step.Name,
		Type:     step.Type,
		Status:   "success",
		Output:   outputs,
		Duration: time.Since(start),
	}, nil
}

// bidIdentity extracts the identifying fields of a bid-like item as flat,
// stringly-typed values. It understands the chain query shape
// {"bid":{"id":{owner,dseq,gseq,oseq,provider},"price":{...}}}, the flatter
// {"id":{...}} shape, and items carrying the fields at the top level.
// Numbers are normalized to plain decimal strings so templates never render
// scientific notation.
func bidIdentity(item map[string]any) map[string]any {
	inner := item
	if b, ok := item["bid"].(map[string]any); ok {
		inner = b
	}

	id := inner
	if m, ok := inner["id"].(map[string]any); ok {
		id = m
	}

	out := make(map[string]any)
	for _, k := range []string{"owner", "dseq", "gseq", "oseq", "provider"} {
		if v, ok := id[k]; ok {
			out[k] = normalizeScalar(v)
		}
	}

	if p, ok := inner["price"].(map[string]any); ok {
		amount, _ := normalizeScalar(p["amount"]).(string)
		denom, _ := p["denom"].(string)
		out["price"] = strings.TrimSpace(amount + denom)
	}

	return out
}

// bidPrice returns the numeric price amount of a bid-like item, if present.
func bidPrice(item map[string]any) (float64, bool) {
	inner := item
	if b, ok := item["bid"].(map[string]any); ok {
		inner = b
	}

	p, ok := inner["price"].(map[string]any)
	if !ok {
		return 0, false
	}

	switch a := p["amount"].(type) {
	case float64:
		return a, true
	case string:
		if f, err := strconv.ParseFloat(a, 64); err == nil {
			return f, true
		}
	}

	return 0, false
}

// normalizeScalar renders JSON scalars as template-friendly values: float64
// becomes a plain decimal string (no exponent), everything else passes
// through unchanged.
func normalizeScalar(v any) any {
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}

	return v
}

func interactiveSelect(items []map[string]any, columns []string) (map[string]any, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items")
	}

	// Display items. Column values resolve from the item's top level first,
	// then from its flattened bid identity (provider, price, dseq, ...).
	for i, item := range items {
		identity := bidIdentity(item)
		parts := make([]string, 0, len(columns))
		for _, col := range columns {
			if v, ok := item[col]; ok {
				parts = append(parts, fmt.Sprintf("%v", v))
			} else if v, ok := identity[col]; ok {
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

// selectCheapest returns the item with the lowest price amount. Items
// without a parseable price rank last; if no item has a price, the first
// item is returned.
func selectCheapest(items []map[string]any) map[string]any {
	if len(items) == 0 {
		return nil
	}

	best := items[0]
	bestPrice, bestOK := bidPrice(items[0])

	for _, item := range items[1:] {
		price, ok := bidPrice(item)
		if !ok {
			continue
		}

		if !bestOK || price < bestPrice {
			best = item
			bestPrice = price
			bestOK = true
		}
	}

	return best
}

func selectByProvider(items []map[string]any, addr string) map[string]any {
	for _, item := range items {
		if p, ok := bidIdentity(item)["provider"]; ok && fmt.Sprintf("%v", p) == addr {
			return item
		}
	}

	return nil
}
