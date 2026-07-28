package steps

import (
	"context"
	"encoding/json"
	"testing"

	"pkg.akt.dev/akt/internal/workflow"
)

// bidsQueryJSON mimics the chain (and console fallback) market.bids query
// response shape: a top-level "bids" array of {"bid": {...}} wrappers.
const bidsQueryJSON = `{
	"bids": [
		{"bid": {"id": {"owner": "akash1x", "dseq": "4242", "gseq": 1, "oseq": 1, "provider": "akash1exp"}, "state": "open", "price": {"denom": "uakt", "amount": "25"}}},
		{"bid": {"id": {"owner": "akash1x", "dseq": "4242", "gseq": 1, "oseq": 2, "provider": "akash1cheap"}, "state": "open", "price": {"denom": "uakt", "amount": "10"}}}
	],
	"pagination": {"total": 2}
}`

// bidsState builds a run state as it looks after the deploy workflow's
// wait-for-bids step: the raw query result exposed as step outputs.
func bidsState(t *testing.T) *workflow.RunState {
	t.Helper()

	state := workflow.NewRunState("wf-1", "deploy", "akash1x", map[string]any{"bid-select": "cheapest"})

	var m map[string]any
	if err := json.Unmarshal([]byte(bidsQueryJSON), &m); err != nil {
		t.Fatalf("unmarshal bids fixture: %v", err)
	}

	state.SetStepResult("wait-for-bids", &workflow.StepResult{
		Name:      "wait-for-bids",
		Status:    "success",
		Output:    m,
		RawResult: json.RawMessage(bidsQueryJSON),
	})

	return state
}

// TestPromptCheapestSelectsLowestPriceAndExposesIdentity runs the prompt
// executor with the exact mode/data templates used by builtin/deploy.yaml
// and verifies the outputs feed the create-lease step's discrete params.
func TestPromptCheapestSelectsLowestPriceAndExposesIdentity(t *testing.T) {
	state := bidsState(t)

	step := workflow.StepDef{
		Name: "select-bid",
		Type: workflow.StepPrompt,
		Mode: `{{ index .Params "bid-select" }}`,
		Data: `{{ toJson (index .Steps "wait-for-bids").bids }}`,
	}

	res, err := (&PromptExecutor{}).Execute(context.Background(), step, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Status != "success" {
		t.Fatalf("status = %q, want success", res.Status)
	}

	state.SetStepResult("select-bid", res)

	// The create-lease step templates from builtin/deploy.yaml must resolve
	// to the cheapest bid's discrete id fields.
	for tmpl, want := range map[string]string{
		`{{ (index .Steps "select-bid").provider }}`: "akash1cheap",
		`{{ (index .Steps "select-bid").gseq }}`:     "1",
		`{{ (index .Steps "select-bid").oseq }}`:     "2",
		`{{ (index .Steps "select-bid").dseq }}`:     "4242",
		`{{ (index .Steps "select-bid").price }}`:    "10uakt",
	} {
		got, err := workflow.ResolveTemplate(tmpl, state)
		if err != nil {
			t.Errorf("ResolveTemplate(%q): %v", tmpl, err)
			continue
		}
		if got != want {
			t.Errorf("ResolveTemplate(%q) = %q, want %q", tmpl, got, want)
		}
	}

	if _, ok := res.Output["selected"].(map[string]any); !ok {
		t.Errorf("selected output missing or wrong type: %T", res.Output["selected"])
	}
}

// TestPromptProviderModeMatchesNestedShape verifies provider=<addr> mode
// matches against the nested bid id, not a top-level field.
func TestPromptProviderModeMatchesNestedShape(t *testing.T) {
	state := bidsState(t)

	step := workflow.StepDef{
		Name: "select-bid",
		Type: workflow.StepPrompt,
		Mode: "provider=akash1exp",
		Data: `{{ toJson (index .Steps "wait-for-bids").bids }}`,
	}

	res, err := (&PromptExecutor{}).Execute(context.Background(), step, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Output["provider"] != "akash1exp" {
		t.Errorf("provider output = %v, want akash1exp", res.Output["provider"])
	}
}

// TestPromptNoItems verifies the executor fails cleanly with no bids.
func TestPromptNoItems(t *testing.T) {
	state := workflow.NewRunState("wf-1", "deploy", "akash1x", nil)
	state.SetStepResult("wait-for-bids", &workflow.StepResult{
		Name:   "wait-for-bids",
		Status: "success",
		Output: map[string]any{"bids": []any{}},
	})

	step := workflow.StepDef{
		Name: "select-bid",
		Type: workflow.StepPrompt,
		Mode: "cheapest",
		Data: `{{ toJson (index .Steps "wait-for-bids").bids }}`,
	}

	if _, err := (&PromptExecutor{}).Execute(context.Background(), step, state); err == nil {
		t.Fatal("expected error for empty bid list")
	}
}

func TestSelectCheapest(t *testing.T) {
	items := []map[string]any{
		{"bid": map[string]any{"price": map[string]any{"amount": "30", "denom": "uakt"}, "id": map[string]any{"provider": "p1"}}},
		{"bid": map[string]any{"id": map[string]any{"provider": "no-price"}}},
		{"bid": map[string]any{"price": map[string]any{"amount": float64(7), "denom": "uakt"}, "id": map[string]any{"provider": "p3"}}},
	}

	got := selectCheapest(items)
	if bidIdentity(got)["provider"] != "p3" {
		t.Errorf("selectCheapest picked %v, want provider p3", bidIdentity(got))
	}

	// No prices at all: fall back to the first item.
	noPrices := []map[string]any{
		{"bid": map[string]any{"id": map[string]any{"provider": "a"}}},
		{"bid": map[string]any{"id": map[string]any{"provider": "b"}}},
	}
	if bidIdentity(selectCheapest(noPrices))["provider"] != "a" {
		t.Error("selectCheapest without prices must return the first item")
	}
}
