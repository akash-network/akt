package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	wf "pkg.akt.dev/akt/internal/workflow"
)

// TestResolveContextToleratesMissingManager covers every nil-safe arm of
// resolveContext. It runs before the rail is chosen, so a panic here would
// break `akt deploy` for anyone without a configured context — exactly the
// users the first-run wizard is meant to help.
func TestResolveContextToleratesMissingManager(t *testing.T) {
	if got := resolveContext(nil, func() string { return "" }); got != nil {
		t.Errorf("nil manager func = %+v, want nil", got)
	}

	if got := resolveContext(func() *aktctx.Manager { return nil }, func() string { return "" }); got != nil {
		t.Errorf("nil manager = %+v, want nil", got)
	}

	// A real manager with no current context: Resolve fails, and the helper
	// must translate that into "no context" rather than propagating an error.
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := resolveContext(func() *aktctx.Manager { return m }, func() string { return "" }); got != nil {
		t.Errorf("unresolvable context = %+v, want nil", got)
	}

	// With a context configured, the resolved record comes back.
	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	rc := resolveContext(func() *aktctx.Manager { return m }, func() string { return "prod" })
	if rc == nil || rc.Name != "prod" {
		t.Fatalf("resolved context = %+v, want prod", rc)
	}
}

// TestEmitJSONLShape pins the machine-readable workflow output (SPEC §2.3.8).
// Scripts consume it line by line, so the per-step result mapping and the
// never-null errors/txs arrays are part of the contract.
func TestEmitJSONLShape(t *testing.T) {
	state := wf.NewRunState("run-1", "deploy", "akash1owner", nil)
	state.SetStepResult("create", &wf.StepResult{
		Name:   "create",
		Status: "success",
		TxHash: "ABC123",
		Height: 42,
	})
	state.SetStepResult("skipme", &wf.StepResult{Name: "skipme", Status: "skipped"})
	state.SetStepResult("boom", &wf.StepResult{Name: "boom", Status: "failed", Error: "out of gas"})

	var buf bytes.Buffer
	emitJSONL(&buf, state, nil)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("emitted %d lines, want one per step:\n%s", len(lines), buf.String())
	}

	type jsonl struct {
		Workflow string `json:"workflow"`
		ID       string `json:"id"`
		Step     string `json:"step"`
		Result   string `json:"result"`
		Errors   []string
		Txs      []struct {
			Hash   string `json:"hash"`
			Height int64  `json:"height"`
		}
	}

	var got []jsonl
	for _, line := range lines {
		var l jsonl
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatalf("line %q is not valid JSON: %v", line, err)
		}
		got = append(got, l)
	}

	// Order follows StepOrder, so scripts can correlate with the run.
	if got[0].Step != "create" || got[1].Step != "skipme" || got[2].Step != "boom" {
		t.Errorf("steps out of order: %+v", got)
	}
	for _, l := range got {
		if l.Workflow != "deploy" || l.ID != "run-1" {
			t.Errorf("line missing run identity: %+v", l)
		}
		if l.Errors == nil || l.Txs == nil {
			t.Errorf("errors/txs must be arrays, never null: %+v", l)
		}
	}

	if got[0].Result != "completed" || len(got[0].Txs) != 1 || got[0].Txs[0].Hash != "ABC123" || got[0].Txs[0].Height != 42 {
		t.Errorf("success line = %+v, want completed with the tx", got[0])
	}
	if got[1].Result != "skipped" {
		t.Errorf("skipped line = %+v", got[1])
	}
	if got[2].Result != "error" || len(got[2].Errors) != 1 || got[2].Errors[0] != "out of gas" {
		t.Errorf("failure line = %+v, want error with the message", got[2])
	}
}

// TestEmitJSONLSkipsMissingResults covers the nil-result guard: StepOrder can
// name a step whose result was never recorded, and emitting a null line would
// break line-oriented consumers.
func TestEmitJSONLSkipsMissingResults(t *testing.T) {
	state := wf.NewRunState("run-1", "deploy", "akash1owner", nil)
	state.SetStepResult("ghost", nil)

	var buf bytes.Buffer
	emitJSONL(&buf, state, nil)

	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("a nil step result must emit nothing, got %q", buf.String())
	}
}

func TestDeployRecoveryAdviceSurfacesPaidPartialState(t *testing.T) {
	state := wf.NewRunState("run-1", "deploy", "akash1owner", map[string]any{
		"sdl-file": "/tmp/my deployment.yaml",
	})
	state.SetStepResult("create-deployment", &wf.StepResult{
		Name:   "create-deployment",
		Status: "success",
		Output: map[string]any{"dseq": "4242"},
	})
	state.SetStepResult("select-bid", &wf.StepResult{
		Name:   "select-bid",
		Status: "success",
		Output: map[string]any{"provider": "akash1provider"},
	})
	state.SetStepResult("create-lease", &wf.StepResult{
		Name:   "create-lease",
		Status: "success",
		Output: map[string]any{"dseq": "4242", "provider": "akash1provider"},
	})
	state.SetStepResult("send-manifest", &wf.StepResult{
		Name:   "send-manifest",
		Status: "failed",
		Error:  "gateway timeout",
	})

	advice := deployRecoveryAdvice(state, errors.New("send manifest failed"))
	if advice == nil {
		t.Fatal("deploy failure after create-deployment returned no recovery advice")
	}
	if advice.DSeq != 4242 || advice.Provider != "akash1provider" {
		t.Fatalf("partial state = %+v, want dseq 4242 and provider", advice)
	}
	if advice.Recovery != "akt provider send-manifest '/tmp/my deployment.yaml' --dseq 4242 --provider akash1provider" {
		t.Errorf("recovery command = %q", advice.Recovery)
	}
	if advice.Cleanup != "akt close 4242" {
		t.Errorf("cleanup command = %q", advice.Cleanup)
	}

	var human bytes.Buffer
	printResults(&human, state, errors.New("send manifest failed"), advice)
	for _, want := range []string{
		"Partial deployment state",
		"DSEQ: 4242",
		"Provider: akash1provider",
		"escrow may continue to be consumed",
		advice.Recovery,
		advice.Cleanup,
	} {
		if !strings.Contains(human.String(), want) {
			t.Errorf("human failure output missing %q:\n%s", want, human.String())
		}
	}

	err := workflowFailureError("deploy", errors.New("send manifest failed"), advice)
	for _, want := range []string{"DSEQ 4242", "akash1provider", "escrow may continue", advice.Recovery, advice.Cleanup} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("returned error missing %q: %v", want, err)
		}
	}
}

func TestDeployRecoveryAdviceJSONLFields(t *testing.T) {
	state := wf.NewRunState("run-1", "deploy", "akash1owner", map[string]any{"sdl-file": "deploy.yaml"})
	state.SetStepResult("create-deployment", &wf.StepResult{
		Name:   "create-deployment",
		Status: "success",
		Output: map[string]any{"dseq": float64(77)},
	})
	state.SetStepResult("send-manifest", &wf.StepResult{Name: "send-manifest", Status: "failed", Error: "refused"})

	advice := deployRecoveryAdvice(state, errors.New("refused"))
	var buf bytes.Buffer
	emitJSONL(&buf, state, advice)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSONL lines = %d, want 2:\n%s", len(lines), buf.String())
	}

	var failed struct {
		Result   string `json:"result"`
		DSeq     uint64 `json:"dseq"`
		Provider string `json:"provider"`
		Recovery string `json:"recovery"`
		Cleanup  string `json:"cleanup"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &failed); err != nil {
		t.Fatalf("decode failed JSONL line: %v", err)
	}
	if failed.Result != "error" || failed.DSeq != 77 || failed.Provider != "" {
		t.Errorf("failed JSONL identity = %+v", failed)
	}
	if failed.Recovery != "" || failed.Cleanup != "akt close 77" {
		t.Errorf("failed JSONL recovery = %+v", failed)
	}
}

func TestDeployRecoveryAdviceRequiresCompletedCreate(t *testing.T) {
	state := wf.NewRunState("run-1", "deploy", "akash1owner", map[string]any{"sdl-file": "deploy.yaml"})
	state.SetStepResult("create-deployment", &wf.StepResult{Name: "create-deployment", Status: "failed"})

	if got := deployRecoveryAdvice(state, errors.New("broadcast refused")); got != nil {
		t.Fatalf("failed create has no paid partial state, got %+v", got)
	}

	state.Workflow = "update"
	state.Steps["create-deployment"] = &wf.StepResult{
		Name:   "create-deployment",
		Status: "success",
		Output: map[string]any{"dseq": "42"},
	}
	if got := deployRecoveryAdvice(state, errors.New("update failed")); got != nil {
		t.Fatalf("non-deploy workflow got deploy recovery advice: %+v", got)
	}
}

func TestShellQuoteKeepsRecoveryCommandsCopyPasteable(t *testing.T) {
	if got, want := shellQuote("/tmp/operator's deployment.yaml"), `'/tmp/operator'"'"'s deployment.yaml'`; got != want {
		t.Errorf("shellQuote = %q, want %q", got, want)
	}
}

// TestOutputFormatDefaultsToPretty covers both arms of outputFormat. Workflow
// commands are also built standalone (tests, generated subcommands) where
// --output is not registered; defaulting to "" would select an unknown format.
func TestOutputFormatDefaultsToPretty(t *testing.T) {
	bare := &cobra.Command{}
	if got := outputFormat(bare); got != "pretty" {
		t.Errorf("without --output = %q, want pretty", got)
	}

	withFlag := &cobra.Command{}
	withFlag.Flags().String("output", "", "")
	if got := outputFormat(withFlag); got != "pretty" {
		t.Errorf("empty --output = %q, want pretty", got)
	}

	if err := withFlag.Flags().Set("output", "jsonl"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if got := outputFormat(withFlag); got != "jsonl" {
		t.Errorf("explicit --output = %q, want jsonl", got)
	}
}

// TestFilterProviderStepsDropsOnlyProviderSteps covers the console-rail
// rewrite (SPEC §7.4): the Console API submits manifests itself, so provider
// steps must be removed — but nothing else may be, and the caller's definition
// must not be mutated (it is the shared, loaded workflow).
func TestFilterProviderStepsDropsOnlyProviderSteps(t *testing.T) {
	def := &wf.WorkflowDef{
		Name: "deploy",
		Steps: []wf.StepDef{
			{Name: "create", Type: wf.StepTx},
			{Name: "send-manifest", Type: wf.StepProvider},
			{Name: "lease", Type: wf.StepTx},
		},
	}

	var notes bytes.Buffer
	filtered := filterProviderSteps(def, &notes)

	if len(def.Steps) != 3 {
		t.Errorf("the input definition must not be mutated, got %d steps", len(def.Steps))
	}
	if len(filtered.Steps) != 2 {
		t.Fatalf("filtered steps = %+v, want the two tx steps", filtered.Steps)
	}
	if filtered.Steps[0].Name != "create" || filtered.Steps[1].Name != "lease" {
		t.Errorf("wrong steps kept: %+v", filtered.Steps)
	}
	if !strings.Contains(notes.String(), "send-manifest") {
		t.Errorf("the skipped step should be reported to the user, got %q", notes.String())
	}

	// A definition with no provider steps is returned unchanged and silent.
	notes.Reset()
	plain := &wf.WorkflowDef{Name: "close", Steps: []wf.StepDef{{Name: "close", Type: wf.StepTx}}}
	if got := filterProviderSteps(plain, &notes); got != plain {
		t.Error("a definition without provider steps should be returned as-is")
	}
	if notes.String() != "" {
		t.Errorf("nothing was skipped, so nothing should be reported: %q", notes.String())
	}
}

// TestAddMissingTxFlagsDoesNotClobberWorkflowParams pins the flag-merge rule:
// workflow parameters generate flags first, and the standard tx flags must not
// overwrite one that already exists (nor steal a taken shorthand). A clobbered
// param flag would silently drop the user's value.
func TestAddMissingTxFlagsDoesNotClobberWorkflowParams(t *testing.T) {
	cmd := &cobra.Command{Use: "deploy"}
	cmd.Flags().String("gas", "workflow-owned", "the workflow's own gas param")
	cmd.Flags().StringP("note", "y", "", "a param that squats on -y")

	addMissingTxFlags(cmd)

	if got := cmd.Flags().Lookup("gas").Usage; got != "the workflow's own gas param" {
		t.Errorf("an existing flag was replaced: %q", got)
	}
	if f := cmd.Flags().ShorthandLookup("y"); f == nil || f.Name != "note" {
		t.Errorf("a taken shorthand was reassigned: %+v", f)
	}

	// Standard tx flags that did not collide are still added.
	for _, name := range []string{"from", "fees"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("standard tx flag --%s was not added", name)
		}
	}
}
