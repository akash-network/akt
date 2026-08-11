package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	wf "pkg.akt.dev/akt/internal/workflow"
	"pkg.akt.dev/akt/internal/workflow/builtin"
)

func newState() *wf.RunState {
	return wf.NewRunState("wf-1", "deploy", "akash1owner", map[string]any{
		"dseq":    "12345",
		"deposit": "5usd",
	})
}

// TestExtractOutputsNamedTemplates covers the templated-output path used by tx
// and query steps to hand values to later steps: each output template is
// evaluated against .Result (the step's raw JSON) plus the run state.
func TestExtractOutputsNamedTemplates(t *testing.T) {
	raw := json.RawMessage(`{"deployment":{"id":{"dseq":"9911","owner":"akash1owner"}},"code":0}`)

	outputs, err := wf.ExtractOutputs(map[string]string{
		"dseq":    "{{ .Result.deployment.id.dseq }}",
		"owner":   "{{ .Result.deployment.id.owner }}",
		"account": "{{ .Account }}",
		"literal": "no-template-here",
	}, raw, newState())
	if err != nil {
		t.Fatalf("ExtractOutputs: %v", err)
	}

	want := map[string]string{
		"dseq":    "9911",
		"owner":   "akash1owner",
		"account": "akash1owner",
		"literal": "no-template-here",
	}

	for k, v := range want {
		if got := outputs[k]; got != v {
			t.Errorf("output %q = %v, want %q", k, got, v)
		}
	}
}

// TestExtractOutputsWithoutDefinitionsReturnsWholeResult covers the fallback
// branch: a step with no declared outputs exposes the entire decoded result,
// so a later step can reach fields nobody thought to name.
func TestExtractOutputsWithoutDefinitionsReturnsWholeResult(t *testing.T) {
	outputs, err := wf.ExtractOutputs(nil, json.RawMessage(`{"a":1,"b":"two"}`), newState())
	if err != nil {
		t.Fatalf("ExtractOutputs: %v", err)
	}

	if outputs["b"] != "two" {
		t.Errorf("outputs = %v, want the decoded result", outputs)
	}

	// A non-object result has nothing to expose; that must be nil, not an error
	// that aborts an otherwise successful step.
	outputs, err = wf.ExtractOutputs(nil, json.RawMessage(`["not","an","object"]`), newState())
	if err != nil {
		t.Fatalf("a non-object result must not error: %v", err)
	}
	if outputs != nil {
		t.Errorf("outputs = %v, want nil for a non-object result", outputs)
	}
}

// TestExtractOutputsNilsUnresolvableEntries pins the per-output error handling:
// one bad template must not lose the outputs that did resolve. The bad entry
// becomes nil so a downstream {{ }} reference renders empty rather than
// silently reusing a stale value.
func TestExtractOutputsNilsUnresolvableEntries(t *testing.T) {
	outputs, err := wf.ExtractOutputs(map[string]string{
		"good": "{{ .Result.ok }}",
		"bad":  "{{ .Result.ok ", // unterminated action: parse error
	}, json.RawMessage(`{"ok":"yes"}`), newState())
	if err != nil {
		t.Fatalf("ExtractOutputs: %v", err)
	}

	if outputs["good"] != "yes" {
		t.Errorf("good output = %v, want yes", outputs["good"])
	}
	if v, present := outputs["bad"]; !present || v != nil {
		t.Errorf("bad output = %v (present=%v), want a nil entry", v, present)
	}
}

// TestParseList covers the helper prompt steps use to turn a previous step's
// JSON output back into selectable rows (the bid list, most importantly).
func TestParseList(t *testing.T) {
	list, err := wf.ParseList(`  [{"provider":"akash1a","price":"100"},{"provider":"akash1b"}]  `)
	if err != nil {
		t.Fatalf("ParseList: %v", err)
	}
	if len(list) != 2 || list[0]["provider"] != "akash1a" {
		t.Errorf("list = %v", list)
	}

	for name, in := range map[string]string{
		"empty":       "",
		"whitespace":  "   ",
		"not json":    "bids: none",
		"json object": `{"provider":"akash1a"}`,
	} {
		if _, err := wf.ParseList(in); err == nil {
			t.Errorf("%s: expected an error for %q", name, in)
		}
	}
}

// TestToJsonTemplateFunc covers the toJson template function. Steps that pass
// structured data onward (the bid list handed to a prompt step) must serialize
// it as JSON; without toJson the receiving step would get Go's fmt rendering
// of a map, which ParseList cannot read back.
func TestToJsonTemplateFunc(t *testing.T) {
	state := newState()
	state.SetStepResult("bids", &wf.StepResult{
		Name:   "bids",
		Status: "success",
		Output: map[string]any{
			"bids": []map[string]any{{"provider": "akash1a"}},
		},
	})

	rendered, err := wf.ResolveTemplate("{{ toJson .Steps.bids.bids }}", state)
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}

	list, err := wf.ParseList(rendered)
	if err != nil {
		t.Fatalf("toJson output must round-trip through ParseList, got %q: %v", rendered, err)
	}
	if len(list) != 1 || list[0]["provider"] != "akash1a" {
		t.Errorf("round-tripped list = %v", list)
	}
}

// TestResolveTemplateReportsBadTemplates covers the parse and execute error
// branches. A workflow author's typo must abort the run with a message naming
// the template, not render "<no value>" into a transaction parameter.
func TestResolveTemplateReportsBadTemplates(t *testing.T) {
	if _, err := wf.ResolveTemplate("{{ .Params.dseq ", newState()); err == nil {
		t.Error("an unterminated action must be a parse error")
	} else if !strings.Contains(err.Error(), "parse template") {
		t.Errorf("unexpected error: %v", err)
	}

	if _, err := wf.ResolveTemplate(`{{ toJson . | printf "%s" }}{{ fail }}`, newState()); err == nil {
		t.Error("an unknown function must be an error")
	}
}

// TestResolveParamsPropagatesTemplateErrors pins that a failing parameter
// aborts resolution and names the offending key — resolved params become tx
// message fields, so a partially-resolved map must never be returned.
func TestResolveParamsPropagatesTemplateErrors(t *testing.T) {
	out, err := wf.ResolveParams(map[string]string{
		"ok":  "{{ .Params.dseq }}",
		"bad": "{{ .Params.dseq ",
	}, newState())
	if err == nil {
		t.Fatal("a failing param template must abort resolution")
	}
	if out != nil {
		t.Errorf("no partial map may be returned, got %v", out)
	}
	if !strings.Contains(err.Error(), `resolve param "bad"`) {
		t.Errorf("error should name the failing key, got %q", err)
	}

	// A nil/empty map is returned untouched (no allocation, no error).
	if out, err := wf.ResolveParams(nil, newState()); err != nil || out != nil {
		t.Errorf("empty params = (%v, %v), want (nil, nil)", out, err)
	}
}

// TestEvalConditionFalsyForms pins every value the engine treats as "condition
// not met". These gate whether a step runs at all, so an unrecognized falsy
// form would execute a step the workflow author meant to skip.
func TestEvalConditionFalsyForms(t *testing.T) {
	state := newState()
	state.SetStepResult("q", &wf.StepResult{
		Name:   "q",
		Status: "success",
		Output: map[string]any{"count": 0, "name": ""},
	})

	cases := []struct {
		cond string
		want bool
	}{
		{"", true},                        // no condition: always run
		{"literal-true", true},            // non-template, non-empty
		{"false", false},                  // literal false
		{"{{ .Steps.q.count }}", false},   // renders "0"
		{"{{ .Steps.q.name }}", false},    // renders ""
		{"{{ .Steps.q.missing }}", false}, // missingkey=zero -> "<no value>"
		{"{{ .Steps.q._status }}", true},  // "success"
		{"{{ eq .Account `akash1owner` }}", true},
		{"{{ eq .Account `nobody` }}", false},
	}

	for _, c := range cases {
		got, err := wf.EvalCondition(c.cond, nil, state)
		if err != nil {
			t.Errorf("EvalCondition(%q): %v", c.cond, err)
			continue
		}
		if got != c.want {
			t.Errorf("EvalCondition(%q) = %v, want %v", c.cond, got, c.want)
		}
	}
}

// TestEvalConditionReadsStepResult covers the .Result binding: conditions are
// evaluated against the step's raw JSON, which is how `wait` steps decide
// whether to keep polling.
func TestEvalConditionReadsStepResult(t *testing.T) {
	raw := json.RawMessage(`{"bids":[{"provider":"akash1a"}]}`)

	got, err := wf.EvalCondition("{{ gt (len .Result.bids) 0 }}", raw, newState())
	if err != nil {
		t.Fatalf("EvalCondition: %v", err)
	}
	if !got {
		t.Error("a non-empty bid list should satisfy the condition")
	}

	got, err = wf.EvalCondition("{{ gt (len .Result.bids) 0 }}", json.RawMessage(`{"bids":[]}`), newState())
	if err != nil {
		t.Fatalf("EvalCondition: %v", err)
	}
	if got {
		t.Error("an empty bid list must not satisfy the condition")
	}
}

// TestEvalConditionSurfacesTemplateErrors pins that a broken condition aborts
// rather than defaulting to "run the step".
func TestEvalConditionSurfacesTemplateErrors(t *testing.T) {
	got, err := wf.EvalCondition("{{ .Result.bids ", nil, newState())
	if err == nil {
		t.Fatal("a broken condition must be an error")
	}
	if got {
		t.Error("a failing condition must not evaluate to true")
	}
}

// TestStepOutputMissingKeys covers RunState.StepOutput's two nil branches:
// an unknown step and a step that produced no outputs.
func TestStepOutputMissingKeys(t *testing.T) {
	state := newState()
	state.SetStepResult("nooutput", &wf.StepResult{Name: "nooutput", Status: "success"})

	if got := state.StepOutput("ghost", "dseq"); got != nil {
		t.Errorf("unknown step = %v, want nil", got)
	}
	if got := state.StepOutput("nooutput", "dseq"); got != nil {
		t.Errorf("step without outputs = %v, want nil", got)
	}
}

// builtinStepTemplate loads a built-in workflow and returns the template of
// the named step, so the shipped definition is exercised rather than a copy.
func builtinStepTemplate(t *testing.T, workflow, step string) string {
	t.Helper()

	def, err := wf.NewLoader(t.TempDir(), "", builtin.Workflows()).Load(workflow)
	if err != nil {
		t.Fatalf("load builtin %q: %v", workflow, err)
	}

	for _, s := range def.Steps {
		if s.Name == step {
			return s.Template
		}
	}

	t.Fatalf("builtin %q has no step %q", workflow, step)

	return ""
}

func TestBuiltinDeployResultReportsConsoleAutoTopUp(t *testing.T) {
	tmpl := builtinStepTemplate(t, "deploy", "display-result")
	state := wf.NewRunState("wf-1", "deploy", "akash1owner", nil)
	state.SetStepResult("create-deployment", &wf.StepResult{
		Name:   "create-deployment",
		Type:   wf.StepTx,
		Status: "success",
		Output: map[string]any{
			"dseq":        "9911",
			"rail":        "console",
			"auto_top_up": "daily",
		},
	})

	rendered, err := wf.ResolveTemplate(tmpl, state)
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}
	for _, want := range []string{
		"Auto top-up: enabled (daily)",
		"akt console deployment settings 9911 false",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("Console result does not contain %q:\n%s", want, rendered)
		}
	}
}

func TestBuiltinDeployResultOmitsConsoleSettingOnChain(t *testing.T) {
	tmpl := builtinStepTemplate(t, "deploy", "display-result")
	state := wf.NewRunState("wf-1", "deploy", "akash1owner", nil)
	state.SetStepResult("create-deployment", &wf.StepResult{
		Name:   "create-deployment",
		Type:   wf.StepTx,
		Status: "success",
		Output: map[string]any{"dseq": "9911", "owner": "akash1owner"},
	})

	rendered, err := wf.ResolveTemplate(tmpl, state)
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}
	if strings.Contains(rendered, "Auto top-up") || strings.Contains(rendered, "console deployment settings") {
		t.Errorf("chain result contains a Console-only setting:\n%s", rendered)
	}
}

// updateResultState builds the run state the update workflow's display-result
// step sees: the send-manifest step reports which providers accepted the new
// manifest and how many there were.
func updateResultState(providers []string) *wf.RunState {
	state := wf.NewRunState("wf-1", "update", "akash1owner", map[string]any{"dseq": "12345"})
	state.SetStepResult("update-deployment", &wf.StepResult{
		Name:   "update-deployment",
		Type:   wf.StepTx,
		Status: "success",
		TxHash: "ABCD1234",
	})
	state.SetStepResult("send-manifest", &wf.StepResult{
		Name:   "send-manifest",
		Type:   wf.StepProvider,
		Status: "success",
		Output: map[string]any{"providers": providers, "count": len(providers)},
	})

	return state
}

// TestBuiltinUpdateResultReportsManifestDelivery covers the chain rail with at
// least one active lease: the result names every provider that took the new
// manifest, and still warns that an unchanged image reference redeploys
// nothing.
func TestBuiltinUpdateResultReportsManifestDelivery(t *testing.T) {
	tmpl := builtinStepTemplate(t, "update", "display-result")

	rendered, err := wf.ResolveTemplate(tmpl, updateResultState([]string{"akash1prov1", "akash1prov2"}))
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}

	for _, want := range []string{
		"Deployment updated.",
		"DSEQ: 12345",
		"Manifest sent to 2 provider(s):",
		"    akash1prov1",
		"    akash1prov2",
		"a provider restarts a service only when its image reference or",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("result does not contain %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "WARNING") {
		t.Errorf("delivered manifest must not warn:\n%s", rendered)
	}
}

// TestBuiltinUpdateResultWarnsWhenNoProviderReceivedTheManifest covers the
// zero-lease no-op: SendManifestToActiveLeases succeeds with an empty provider
// set, so the step is "success" while nothing was redeployed. The result must
// say so instead of printing an unconditional "Deployment updated!".
func TestBuiltinUpdateResultWarnsWhenNoProviderReceivedTheManifest(t *testing.T) {
	tmpl := builtinStepTemplate(t, "update", "display-result")

	rendered, err := wf.ResolveTemplate(tmpl, updateResultState(nil))
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}

	for _, want := range []string{
		"nothing was redeployed",
		"DSEQ: 12345",
		"WARNING: this deployment has no active leases",
		"akt query market lease 12345 active",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("result does not contain %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Manifest sent to") {
		t.Errorf("no provider received the manifest, result claims otherwise:\n%s", rendered)
	}
}

// TestBuiltinUpdateResultOnConsoleRail covers the Console context, where
// filterProviderSteps removes the provider step entirely: with no send-manifest
// result in state the template must not read that as "zero providers".
func TestBuiltinUpdateResultOnConsoleRail(t *testing.T) {
	tmpl := builtinStepTemplate(t, "update", "display-result")

	state := wf.NewRunState("wf-1", "update", "akash1owner", map[string]any{"dseq": "12345"})
	state.SetStepResult("update-deployment", &wf.StepResult{
		Name:   "update-deployment",
		Type:   wf.StepTx,
		Status: "success",
	})

	rendered, err := wf.ResolveTemplate(tmpl, state)
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}

	if !strings.Contains(rendered, "handled by the Console API") {
		t.Errorf("console result does not name the Console API:\n%s", rendered)
	}
	if strings.Contains(rendered, "WARNING") {
		t.Errorf("console manifest delivery must not warn:\n%s", rendered)
	}
}

// failingRegistry hands back an executor that always fails, so the engine's
// error handling can be exercised without a chain.
type failingRegistry struct {
	calls *int
}

func (r *failingRegistry) Get(t wf.StepType) (wf.StepExecutor, error) {
	return &failingExecutor{stepType: t, calls: r.calls}, nil
}

type failingExecutor struct {
	stepType wf.StepType
	calls    *int
}

func (e *failingExecutor) Type() wf.StepType { return e.stepType }

func (e *failingExecutor) Execute(_ context.Context, step wf.StepDef, _ *wf.RunState) (*wf.StepResult, error) {
	*e.calls++

	return &wf.StepResult{
		Name:   step.Name,
		Type:   step.Type,
		Status: "failed",
		Error:  "boom",
	}, errors.New("boom")
}

// TestEngineAbortsOnStepFailure pins the default on-error behavior. A tx
// workflow that kept going after a failed broadcast would create a lease for a
// deployment that does not exist.
func TestEngineAbortsOnStepFailure(t *testing.T) {
	calls := 0
	def := &wf.WorkflowDef{
		Name: "test",
		Steps: []wf.StepDef{
			{Name: "first", Type: wf.StepTx},
			{Name: "second", Type: wf.StepTx},
		},
	}

	engine := wf.NewEngine(&failingRegistry{calls: &calls}, nil)

	state, err := engine.Run(context.Background(), def, "akash1owner", nil)
	if err == nil {
		t.Fatal("a failed step must abort the workflow")
	}
	if !strings.Contains(err.Error(), `step "first" failed`) {
		t.Errorf("error should name the failing step, got %q", err)
	}
	if calls != 1 {
		t.Errorf("the second step must not run, executor calls = %d", calls)
	}
	if state.Steps["second"] != nil {
		t.Error("no result may be recorded for the un-run step")
	}
}

// TestEngineContinuesOnErrorWhenAsked covers the on-error: continue branch,
// which workflows use for best-effort steps (e.g. caching a manifest).
func TestEngineContinuesOnErrorWhenAsked(t *testing.T) {
	calls := 0
	def := &wf.WorkflowDef{
		Name: "test",
		Steps: []wf.StepDef{
			{Name: "first", Type: wf.StepTx, OnError: wf.OnErrorContinue},
			{Name: "second", Type: wf.StepTx, OnError: wf.OnErrorSkip},
		},
	}

	engine := wf.NewEngine(&failingRegistry{calls: &calls}, nil)

	state, err := engine.Run(context.Background(), def, "akash1owner", nil)
	if err != nil {
		t.Fatalf("on-error continue/skip must not abort: %v", err)
	}
	if calls != 2 {
		t.Errorf("both steps should have run, executor calls = %d", calls)
	}
	if state.Steps["first"].Status != "failed" {
		t.Errorf("the failure must still be recorded, got %+v", state.Steps["first"])
	}
}

// TestEngineRetriesStep covers the retry loop: a step with retry.max must be
// attempted that many times before the workflow gives up.
func TestEngineRetriesStep(t *testing.T) {
	calls := 0
	def := &wf.WorkflowDef{
		Name: "test",
		Steps: []wf.StepDef{
			{Name: "flaky", Type: wf.StepQuery, Retry: &wf.RetryDef{Max: 3, Delay: "1ms"}},
		},
	}

	engine := wf.NewEngine(&failingRegistry{calls: &calls}, nil)

	if _, err := engine.Run(context.Background(), def, "akash1owner", nil); err == nil {
		t.Fatal("an always-failing step must eventually abort")
	}
	if calls != 3 {
		t.Errorf("executor calls = %d, want 3 (retry.max)", calls)
	}
}

// TestEngineStopsOnCancelledContext pins that Ctrl-C between steps stops the
// workflow instead of running the remaining transactions.
func TestEngineStopsOnCancelledContext(t *testing.T) {
	calls := 0
	def := &wf.WorkflowDef{
		Name:  "test",
		Steps: []wf.StepDef{{Name: "first", Type: wf.StepTx}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	engine := wf.NewEngine(&failingRegistry{calls: &calls}, nil)

	if _, err := engine.Run(ctx, def, "akash1owner", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Errorf("no step may run after cancellation, calls = %d", calls)
	}
}
