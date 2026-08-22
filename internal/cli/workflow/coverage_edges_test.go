package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	aclient "pkg.akt.dev/go/node/client"
	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"

	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	wf "pkg.akt.dev/akt/internal/workflow"
)

type coverageLightClient struct {
	aclient.LightClient
	query cv1beta3.QueryClient
}

func (c coverageLightClient) Query() cv1beta3.QueryClient { return c.query }

type coverageQueryClient struct {
	cv1beta3.QueryClient
	deployment dv1beta.QueryClient
}

func (c coverageQueryClient) Deployment() dv1beta.QueryClient { return c.deployment }

type coverageDeploymentQuery struct {
	dv1beta.QueryClient
	response *dv1beta.QueryParamsResponse
	err      error
}

func (q coverageDeploymentQuery) Params(
	context.Context,
	*dv1beta.QueryParamsRequest,
	...grpc.CallOption,
) (*dv1beta.QueryParamsResponse, error) {
	return q.response, q.err
}

type coverageReadinessProvider struct {
	raw json.RawMessage
	err error
}

func (coverageReadinessProvider) SendManifest(context.Context, string, uint64, []byte) error {
	return nil
}

func (coverageReadinessProvider) SendManifestToActiveLeases(context.Context, uint64, []byte) ([]string, error) {
	return nil, nil
}

func (p coverageReadinessProvider) LeaseStatus(context.Context, string, uint64) (json.RawMessage, error) {
	return p.raw, p.err
}

func TestChainDryRunDepositDiscoveryDecision(t *testing.T) {
	if !chainDryRunNeedsDepositQuery("auto") || !chainDryRunNeedsDepositQuery("") {
		t.Fatal("auto deposits must request chain discovery")
	}
	if chainDryRunNeedsDepositQuery("5uact") || chainDryRunNeedsDepositQuery("not-a-deposit") {
		t.Fatal("explicit or invalid deposits must defer to RunE without discovery")
	}
}

func TestChainDryRunPreRunOnlyDiscoversAutoDeposit(t *testing.T) {
	home := t.TempDir()
	manager := newTestManager(t, home, "chain", aktctx.AuthMethodKeyring)
	newCommand := func() *cobra.Command {
		cmd := findCommand(CommandsWithManager(
			func() string { return home },
			func() string { return "chain" },
			func() *aktctx.Manager { return manager },
		), "deploy")
		query := coverageQueryClient{deployment: coverageDeploymentQuery{response: &dv1beta.QueryParamsResponse{
			Params: dv1beta.Params{MinDeposits: sdk.NewCoins(sdk.NewInt64Coin("uact", 5_000_000))},
		}}}
		light := coverageLightClient{query: query}
		base := context.Background()
		clientContext := sdkclient.Context{}.WithCmdContext(base)
		ctx := context.WithValue(base, chaincli.ClientContextKey, &clientContext)
		ctx = context.WithValue(ctx, chaincli.ContextTypeQueryClient, light)
		cmd.SetContext(ctx)

		return cmd
	}
	sdl := writeValidWorkflowSDL(t)

	if out, err := executeCommand(t, newCommand(), sdl, "--deposit", "5uact", "--dry-run"); err != nil {
		t.Fatalf("explicit deposit dry run: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, newCommand(), sdl, "--deposit", "auto", "--dry-run"); err != nil {
		t.Fatalf("auto deposit dry run: %v\n%s", err, out)
	}
}

func TestResolveDryRunParamsAcrossRails(t *testing.T) {
	t.Run("Console validation", func(t *testing.T) {
		manager := newTestManager(t, t.TempDir(), "console", aktctx.AuthMethodConsoleAPI)
		managerFn := func() *aktctx.Manager { return manager }
		cmd := &cobra.Command{}

		for _, raw := range []string{"bad", "auto", "$0.10"} {
			_, err := resolveDryRunParams(cmd, map[string]any{flagdefs.FlagDeposit: raw}, managerFn, func() string { return "console" })
			if err == nil {
				t.Fatalf("Console deposit %q did not fail", raw)
			}
		}
		resolved, err := resolveDryRunParams(cmd, map[string]any{flagdefs.FlagDeposit: "$0.50"}, managerFn, func() string { return "console" })
		if err != nil || resolved[flagdefs.FlagDeposit] != "0.5" {
			t.Fatalf("resolved Console deposit = %#v, %v", resolved, err)
		}
	})

	t.Run("chain explicit and auto", func(t *testing.T) {
		manager := newTestManager(t, t.TempDir(), "chain", aktctx.AuthMethodKeyring)
		managerFn := func() *aktctx.Manager { return manager }
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		explicit, err := resolveDryRunParams(cmd, map[string]any{flagdefs.FlagDeposit: "5uact"}, managerFn, func() string { return "chain" })
		if err != nil || explicit[flagdefs.FlagDeposit] != "5uact" {
			t.Fatalf("explicit chain deposit = %#v, %v", explicit, err)
		}

		if _, err := resolveDryRunParams(cmd, map[string]any{flagdefs.FlagDeposit: "auto"}, managerFn, func() string { return "chain" }); err == nil {
			t.Fatal("chain auto deposit without a query client did not fail")
		}

		query := coverageQueryClient{deployment: coverageDeploymentQuery{response: &dv1beta.QueryParamsResponse{
			Params: dv1beta.Params{MinDeposits: sdk.NewCoins(sdk.NewInt64Coin("uact", 5_000_000))},
		}}}
		light := coverageLightClient{query: query}
		cmd.SetContext(context.WithValue(context.Background(), chaincli.ContextTypeQueryClient, light))
		resolved, err := resolveDryRunParams(cmd, map[string]any{flagdefs.FlagDeposit: "auto"}, managerFn, func() string { return "chain" })
		if err != nil || resolved[flagdefs.FlagDeposit] != "5000000uact" {
			t.Fatalf("resolved chain deposit = %#v, %v", resolved, err)
		}
	})
}

func coverageDeployState(params map[string]any) *wf.RunState {
	state := wf.NewRunState("run-coverage", "deploy", "akash1owner", params)
	state.SetStepResult("create-deployment", &wf.StepResult{
		Name: "create-deployment", Status: "success", Output: map[string]any{"dseq": "42"},
	})
	state.SetStepResult("create-lease", &wf.StepResult{
		Name: "create-lease", Status: "success", Output: map[string]any{"provider": "akash1provider"},
	})
	state.SetStepResult("display-result", &wf.StepResult{Name: "display-result", Status: "success"})
	return state
}

func TestEnrichDeployCompletionBoundaryBranches(t *testing.T) {
	if err := enrichDeployCompletion(context.Background(), nil, &aktctx.Context{}, nil, nil); err != nil {
		t.Fatal(err)
	}
	state := coverageDeployState(map[string]any{})
	if err := enrichDeployCompletion(context.Background(), state, &aktctx.Context{}, nil, nil); err != nil {
		t.Fatal(err)
	}

	missing := wf.NewRunState("run", "deploy", "", map[string]any{"ready-timeout": "1s"})
	if err := enrichDeployCompletion(context.Background(), missing, &aktctx.Context{}, nil, nil); err == nil {
		t.Fatal("missing create result did not fail")
	}
	badDSeq := coverageDeployState(map[string]any{"ready-timeout": "1s"})
	badDSeq.Steps["create-deployment"].Output["dseq"] = "bad"
	if err := enrichDeployCompletion(context.Background(), badDSeq, &aktctx.Context{}, nil, nil); err == nil {
		t.Fatal("invalid create dseq did not fail")
	}

	for _, timeout := range []any{5, "bad", "0s"} {
		state = coverageDeployState(map[string]any{"ready-timeout": timeout})
		if err := enrichDeployCompletion(context.Background(), state, &aktctx.Context{}, nil, nil); err == nil {
			t.Fatalf("invalid timeout %#v did not fail", timeout)
		}
	}

	state = coverageDeployState(map[string]any{"ready-timeout": "1s"})
	if err := enrichDeployCompletion(context.Background(), state, &aktctx.Context{AuthMethod: aktctx.AuthMethodConsoleAPI}, nil, nil); err == nil {
		t.Fatal("missing Console readiness client did not fail")
	}
	state = coverageDeployState(map[string]any{"ready-timeout": "1s"})
	if err := enrichDeployCompletion(context.Background(), state, &aktctx.Context{}, nil, nil); err == nil {
		t.Fatal("missing provider readiness client did not fail")
	}

	state = coverageDeployState(map[string]any{"ready-timeout": "20ms"})
	provider := coverageReadinessProvider{raw: json.RawMessage(`{"services":{"web":{"available":1,"total":1,"uris":["https://web.example"]}}}`)}
	if err := enrichDeployCompletion(context.Background(), state, &aktctx.Context{}, nil, provider); err != nil {
		t.Fatal(err)
	}
	ready, _ := state.Steps["display-result"].Output["ready"].(bool)
	if state.Steps["wait-for-ready"].Status != "success" || !ready {
		t.Fatalf("successful readiness = %#v", state.Steps)
	}

	state = coverageDeployState(map[string]any{"ready-timeout": "500ms"})
	provider = coverageReadinessProvider{err: errors.New("provider unavailable")}
	err := enrichDeployCompletion(context.Background(), state, &aktctx.Context{}, nil, provider)
	if err == nil || state.Steps["wait-for-ready"].Status != "failed" {
		t.Fatalf("failed readiness = %v, %#v", err, state.Steps["wait-for-ready"])
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	state = coverageDeployState(map[string]any{"ready-timeout": "500ms"})
	err = enrichDeployCompletion(
		context.Background(),
		state,
		&aktctx.Context{AuthMethod: aktctx.AuthMethodConsoleAPI},
		console.New(server.URL, "key"),
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("Console readiness error = %v", err)
	}
}

func TestEnrichSuccessfulDeployPreservesEngineFailures(t *testing.T) {
	sentinel := errors.New("engine failed")
	if err := enrichSuccessfulDeploy(context.Background(), nil, nil, nil, nil, sentinel); !errors.Is(err, sentinel) {
		t.Fatalf("engine failure = %v", err)
	}
	if err := enrichSuccessfulDeploy(context.Background(), nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("successful non-deploy run = %v", err)
	}
}

func TestConsoleRuntimeStatusBoundaries(t *testing.T) {
	if _, err := consoleRuntimeStatus(nil); err == nil {
		t.Fatal("nil deployment status did not fail")
	}
	bad := &console.DeploymentDetail{Leases: []console.Lease{{Status: &console.LeaseStatus{Services: json.RawMessage(`{`)}}}}
	if _, err := consoleRuntimeStatus(bad); err == nil {
		t.Fatal("malformed service status did not fail")
	}
	detail := &console.DeploymentDetail{Leases: []console.Lease{
		{},
		{Status: &console.LeaseStatus{}},
		{Status: &console.LeaseStatus{Services: json.RawMessage(`{"web":{"available":1,"total":1}}`)}},
	}}
	raw, err := consoleRuntimeStatus(detail)
	if err != nil || !strings.Contains(string(raw), "web") {
		t.Fatalf("Console runtime status = %s, %v", raw, err)
	}
}

func TestReadinessDecodeAndFetchFailures(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{`),
		json.RawMessage(`{"services":{}}`),
		json.RawMessage(`{"services":{"web":{"available":1,"total":0}}}`),
	} {
		ready, _, err := deploymentReady(raw)
		if string(raw) == "{" {
			if err == nil {
				t.Fatal("invalid readiness JSON did not fail")
			}
		} else if err != nil || ready {
			t.Fatalf("non-ready status %s = %t, %v", raw, ready, err)
		}
	}

	_, _, err := waitForDeploymentReadiness(context.Background(), time.Millisecond, func(context.Context) (json.RawMessage, error) {
		return nil, errors.New("fetch failed")
	})
	if err == nil || !strings.Contains(err.Error(), "fetch failed") {
		t.Fatalf("fetch timeout = %v", err)
	}

	requests := 0
	uris, _, err := waitForDeploymentReadiness(context.Background(), 3*time.Second, func(context.Context) (json.RawMessage, error) {
		requests++
		if requests == 1 {
			return json.RawMessage(`{"services":{"web":{"available":0,"total":1}}}`), nil
		}
		return json.RawMessage(`{"services":{"web":{"available":1,"total":1,"uris":["https://web.example"]}}}`), nil
	})
	if err != nil || requests != 2 || len(uris["web"]) != 1 {
		t.Fatalf("readiness retry = requests %d, uris %#v, err %v", requests, uris, err)
	}
}

func TestRenderDeployNextBoundaryAndChainActions(t *testing.T) {
	var rendered strings.Builder
	renderDeployNext(&rendered, nil)
	renderDeployNext(&rendered, wf.NewRunState("run", "update", "", nil))
	state := wf.NewRunState("run", "deploy", "", nil)
	renderDeployNext(&rendered, state)
	state.SetStepResult("display-result", &wf.StepResult{Name: "display-result", Output: map[string]any{}})
	renderDeployNext(&rendered, state)

	state.Steps["display-result"].Output = map[string]any{
		"dseq": "42", "provider": "akash1provider", "ready": true,
		"uris": map[string][]string{"web": {"https://web.example"}},
	}
	rendered.Reset()
	renderDeployNext(&rendered, state)
	for _, want := range []string{"Ready: yes", "URI (web)", "akt provider lease-status 42 --provider akash1provider", "akt close 42"} {
		if !strings.Contains(rendered.String(), want) {
			t.Errorf("chain next actions missing %q:\n%s", want, rendered.String())
		}
	}
	if got := consolePlanAction("unknown.Msg"); got != "unsupported Console action unknown.Msg" {
		t.Fatalf("unknown Console plan action = %q", got)
	}
}

func TestMergeCompletionOutputWithoutDisplayResult(t *testing.T) {
	state := wf.NewRunState("run", "deploy", "", nil)
	mergeCompletionOutput(state, map[string]any{"ready": true})
}
