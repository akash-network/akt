package workflow

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"pkg.akt.dev/akt/internal/capability"
	aktctx "pkg.akt.dev/akt/internal/context"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
	wf "pkg.akt.dev/akt/internal/workflow"
	"pkg.akt.dev/akt/internal/workflow/builtin"
	"pkg.akt.dev/go/sdl"
)

type workflowFailingWriter struct {
	err    error
	short  bool
	writes int
}

func (w *workflowFailingWriter) Write(data []byte) (int, error) {
	w.writes++

	if w.short {
		if len(data) == 0 {
			return 0, nil
		}

		return len(data) - 1, nil
	}

	return 0, w.err
}

// commandNames extracts the sorted names of a set of cobra commands.
func commandNames(cmds []*cobra.Command) []string {
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		names = append(names, c.Name())
	}
	sort.Strings(names)

	return names
}

// findCommand returns the command with the given name, or nil.
func findCommand(cmds []*cobra.Command, name string) *cobra.Command {
	for _, c := range cmds {
		if c.Name() == name {
			return c
		}
	}

	return nil
}

// staticFns returns homeFn/ctxNameFn closures over a fixed home and empty context.
func staticFns(home string) (func() string, func() string) {
	return func() string { return home }, func() string { return "" }
}

// TestCommandsBuiltinsOnly verifies that with no user workflows present,
// Commands surfaces exactly the built-in set: close, deploy, update.
func TestCommandsBuiltinsOnly(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmds := Commands(homeFn, ctxNameFn)

	got := commandNames(cmds)
	want := []string{"close", "deploy", "update"}
	if len(got) != len(want) {
		t.Fatalf("Commands() returned %v, want exactly %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Commands() returned %v, want exactly %v", got, want)
		}
	}
}

// TestCommandsUserWorkflowSurfacing verifies that a command appears if and
// only if its backing workflow exists: "foo" is absent before the workflow
// file is written and present after.
func TestCommandsUserWorkflowSurfacing(t *testing.T) {
	home := t.TempDir()
	homeFn, ctxNameFn := staticFns(home)

	// Negative case: no foo workflow exists, so no foo command surfaces.
	if cmd := findCommand(Commands(homeFn, ctxNameFn), "foo"); cmd != nil {
		t.Fatalf("Commands() surfaced %q before its workflow existed", "foo")
	}

	// Write a minimal valid user workflow.
	dir := filepath.Join(home, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yamlDef := `name: foo
description: A user-defined workflow
version: 1

params:
  label:
    type: string
    default: "hello"
    description: A label to print

steps:
  - name: display
    type: output
    template: |
      {{ .Params.label }}
`
	if err := os.WriteFile(filepath.Join(dir, "foo.yaml"), []byte(yamlDef), 0o644); err != nil {
		t.Fatal(err)
	}

	// Positive case: the workflow now resolves, so the command surfaces.
	cmd := findCommand(Commands(homeFn, ctxNameFn), "foo")
	if cmd == nil {
		t.Fatalf("Commands() did not surface %q after its workflow was written", "foo")
		return
	}
	if cmd.Short != "A user-defined workflow" {
		t.Fatalf("foo command Short = %q, want %q", cmd.Short, "A user-defined workflow")
	}
	if cmd.Flags().Lookup(flagdefs.FlagLabel) == nil {
		t.Fatalf("foo command missing --label flag generated from params")
	}
}

// TestCommandsSkipsMalformedWorkflow verifies that a malformed workflow YAML
// is skipped silently and does not produce a command.
func TestCommandsSkipsMalformedWorkflow(t *testing.T) {
	home := t.TempDir()
	homeFn, ctxNameFn := staticFns(home)

	dir := filepath.Join(home, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{ not: [valid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmds := Commands(homeFn, ctxNameFn)

	if cmd := findCommand(cmds, "bad"); cmd != nil {
		t.Fatalf("Commands() surfaced a command for malformed workflow %q", "bad")
	}
	got := commandNames(cmds)
	want := []string{"close", "deploy", "update"}
	if len(got) != len(want) {
		t.Fatalf("Commands() returned %v, want exactly the built-ins %v", got, want)
	}
}

// loadBuiltin loads a built-in workflow definition through the loader,
// using an empty temp home so only embedded definitions resolve.
func loadBuiltin(t *testing.T, name string) *wf.WorkflowDef {
	t.Helper()

	loader := wf.NewLoader(t.TempDir(), "", builtin.Workflows())
	def, err := loader.Load(name)
	if err != nil {
		t.Fatalf("load built-in workflow %q: %v", name, err)
	}

	return def
}

// TestCommandFromDefClose verifies the generated close command: positional
// [dseq] in Use, a dseq int flag, and the common --yes/--dry-run flags.
func TestCommandFromDefClose(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmd := commandFromDef(loadBuiltin(t, "close"), homeFn, ctxNameFn, nil)

	if cmd.Use != "close [dseq]" {
		t.Fatalf("close Use = %q, want %q", cmd.Use, "close [dseq]")
	}
	dseq := cmd.Flags().Lookup(flagdefs.FlagDSeq)
	if dseq == nil {
		t.Fatal("close command missing --dseq flag")
		return
	}
	if dseq.Value.Type() != "int" {
		t.Fatalf("--dseq flag type = %q, want %q", dseq.Value.Type(), "int")
	}
	if cmd.Flags().Lookup(flagdefs.FlagSkipConfirmation) == nil {
		t.Fatal("close command missing common --yes flag")
	}
	if cmd.Flags().Lookup(flagdefs.FlagDryRun) == nil {
		t.Fatal("close command missing common --dry-run flag")
	}
}

// TestCommandFromDefDeploy verifies the generated deploy command: the
// required file and optional deposit params are positional in Use, the SDL
// remains positional-only, and the common --yes/--dry-run flags exist.
func TestCommandFromDefDeploy(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmd := commandFromDef(loadBuiltin(t, "deploy"), homeFn, ctxNameFn, nil)

	if cmd.Use != "deploy <sdl-file> [deposit]" {
		t.Fatalf("deploy Use = %q, want %q", cmd.Use, "deploy <sdl-file> [deposit]")
	}
	if cmd.Flags().Lookup("sdl-file") != nil {
		t.Fatal("deploy command has --sdl-file flag; file param must be positional only")
	}
	for _, flag := range []string{"deposit", "bid-timeout", "ready-timeout", "no-wait-active", "bid-select", "yes", "dry-run"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("deploy command missing --%s flag", flag)
		}
	}
	depositHelp := cmd.Flags().Lookup(flagdefs.FlagDeposit).Usage
	if !strings.Contains(depositHelp, "auto (recommended") {
		t.Errorf("--deposit help %q does not recommend the network-derived default", depositHelp)
	}
	if strings.Contains(depositHelp, "uakt") {
		t.Errorf("--deposit help %q advertises a network-specific denomination", depositHelp)
	}
}

// TestUserWorkflowOverridesBuiltin verifies that a user workflow with the
// same name as a built-in takes precedence when the command is generated.
func TestUserWorkflowOverridesBuiltin(t *testing.T) {
	home := t.TempDir()
	homeFn, ctxNameFn := staticFns(home)

	builtinDesc := loadBuiltin(t, "close").Description

	dir := filepath.Join(home, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := `name: close
description: Custom close override
version: 2

steps:
  - name: display
    type: output
    template: |
      overridden
`
	if err := os.WriteFile(filepath.Join(dir, "close.yaml"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := findCommand(Commands(homeFn, ctxNameFn), "close")
	if cmd == nil {
		t.Fatal("Commands() did not surface close")
		return
	}
	if cmd.Short != "Custom close override" {
		t.Fatalf("close Short = %q, want the user override %q", cmd.Short, "Custom close override")
	}
	if cmd.Short == builtinDesc {
		t.Fatalf("close Short = %q still matches the built-in description; override not applied", cmd.Short)
	}
}

// TestCommandFromDefTxFlags verifies that the standard chain tx flags are
// merged onto workflow commands (needed for keyring chain-client discovery)
// without clobbering the workflow-specific --yes/--dry-run flags.
func TestCommandFromDefTxFlags(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmd := commandFromDef(loadBuiltin(t, "close"), homeFn, ctxNameFn, nil)

	for _, flag := range []string{"from", "gas", "node", "chain-id"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("close command missing tx flag --%s", flag)
		}
	}

	dryRun := cmd.Flags().Lookup(flagdefs.FlagDryRun)
	if dryRun == nil {
		t.Fatal("close command missing --dry-run flag")
		return
	}
	if !strings.Contains(dryRun.Usage, "execution plan") {
		t.Errorf("--dry-run usage = %q, want the workflow meaning, not the tx simulate meaning", dryRun.Usage)
	}
}

// newTestManager builds a real context manager in home with a single
// context named ctxName using the given auth method, and makes it current.
func newTestManager(t *testing.T, home, ctxName, authMethod string) *aktctx.Manager {
	t.Helper()

	m, err := aktctx.NewManager(home)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}
	if m.GetKeyring("default") == nil {
		if err := m.CreateKeyring(aktctx.Keyring{Name: "default"}); err != nil {
			t.Fatalf("CreateKeyring: %v", err)
		}
	}
	if err := m.CreateContext(aktctx.Context{
		Name:       ctxName,
		Network:    aktctx.Network{Name: "mainnet"},
		AuthMethod: authMethod,
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := m.UseContext(ctxName); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	return m
}

// executeCommand runs a generated workflow command with args, capturing
// combined output.
func executeCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return buf.String(), err
}

// TestExecuteConsoleContextWithoutKey verifies the AKT-647 no-credential
// error: a console-api context without a Console API key fails with guidance
// pointing at AKT_CONSOLE_API_KEY, `akt context edit`, and keyring contexts.
func TestExecuteConsoleContextWithoutKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv(aktctx.EnvConsoleAPIKey, "")

	m := newTestManager(t, home, "console", aktctx.AuthMethodConsoleAPI)

	cmds := CommandsWithManager(
		func() string { return home },
		func() string { return "console" },
		func() *aktctx.Manager { return m },
	)

	cmd := findCommand(cmds, "close")
	if cmd == nil {
		t.Fatal("CommandsWithManager() did not surface close")
	}

	_, err := executeCommand(t, cmd, "123")
	if err == nil {
		t.Fatal("expected no-credential error, got nil")
	}
	for _, want := range []string{"console-api", "no Console credential", aktctx.EnvConsoleAPIKey, "--console-api-key", "keyring"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// TestExecuteDryRunNeedsNoClient verifies --dry-run prints the plan and
// succeeds without any manager, credentials, or chain client.
func TestExecuteDryRunNeedsNoClient(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmd := findCommand(Commands(homeFn, ctxNameFn), "close")
	if cmd == nil {
		t.Fatal("Commands() did not surface close")
	}

	out, err := executeCommand(t, cmd, "--dry-run", "456")
	if err != nil {
		t.Fatalf("dry-run execute: %v", err)
	}
	for _, want := range []string{"close-deployment", "Dry run — no transactions broadcast."} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output %q does not contain %q", out, want)
		}
	}
}

func TestExecuteDryRunJSONLEmitsPlannedSteps(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())
	sdl := writeValidWorkflowSDL(t)
	tests := []struct {
		name  string
		args  []string
		steps []string
	}{
		{name: "deploy", args: []string{sdl, "--dry-run", "--output", "jsonl"}, steps: []string{"create-deployment", "wait-for-bids", "select-bid", "create-lease", "send-manifest", "display-result"}},
		{name: "update", args: []string{sdl, "456", "--dry-run", "--output", "jsonl"}, steps: []string{"update-deployment", "send-manifest", "display-result"}},
		{name: "close", args: []string{"456", "--dry-run", "--output", "jsonl"}, steps: []string{"close-deployment", "display-result"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := findCommand(Commands(homeFn, ctxNameFn), tc.name)
			if cmd == nil {
				t.Fatalf("workflow command %q not found", tc.name)
			}

			out, err := executeCommand(t, cmd, tc.args...)
			if err != nil {
				t.Fatalf("JSONL dry-run execute: %v\noutput:\n%s", err, out)
			}
			if strings.Contains(out, "Workflow:") || strings.Contains(out, "Dry run") {
				t.Fatalf("JSONL dry-run mixed human text into stdout:\n%s", out)
			}

			lines := strings.Split(strings.TrimSpace(out), "\n")
			if len(lines) != len(tc.steps) {
				t.Fatalf("JSONL dry-run emitted %d lines, want %d:\n%s", len(lines), len(tc.steps), out)
			}

			var runID string
			for i, raw := range lines {
				var got struct {
					Workflow string            `json:"workflow"`
					ID       string            `json:"id"`
					Step     string            `json:"step"`
					Result   string            `json:"result"`
					Errors   []string          `json:"errors"`
					Txs      []json.RawMessage `json:"txs"`
				}
				if err := json.Unmarshal([]byte(raw), &got); err != nil {
					t.Fatalf("line %d is not JSON: %v\n%s", i+1, err, raw)
				}
				if got.Workflow != tc.name || got.Step != tc.steps[i] || got.Result != "planned" {
					t.Errorf("line %d = %+v, want workflow=%q step=%q result=planned", i+1, got, tc.name, tc.steps[i])
				}
				if got.ID == "" {
					t.Errorf("line %d has empty run ID", i+1)
				} else if runID == "" {
					runID = got.ID
				} else if got.ID != runID {
					t.Errorf("line %d run ID = %q, want %q", i+1, got.ID, runID)
				}
				if got.Errors == nil || len(got.Errors) != 0 || got.Txs == nil || len(got.Txs) != 0 {
					t.Errorf("line %d errors/txs = %#v/%#v, want empty arrays", i+1, got.Errors, got.Txs)
				}
			}
		})
	}
}

// A console-api context is the whole point of CON-733: `akt deploy` must run
// with no deposit argument at all, and must refuse one rather than send a
// number the API discards.
func TestConsoleDryRunPlansWithoutADeposit(t *testing.T) {
	home := t.TempDir()
	m := newTestManager(t, home, "console", aktctx.AuthMethodConsoleAPI)
	sdl := writeValidWorkflowSDL(t)
	newCommand := func() *cobra.Command {
		return findCommand(CommandsWithManager(
			func() string { return home },
			func() string { return "console" },
			func() *aktctx.Manager { return m },
		), "deploy")
	}

	out, err := executeCommand(t, newCommand(), sdl, "--dry-run")
	if err != nil {
		t.Fatalf("Console dry-run without a deposit: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Rail: console") || !strings.Contains(out, "Console API create deployment") || strings.Contains(out, "deployment.MsgCreateDeployment") {
		t.Fatalf("dry-run did not translate steps for the Console rail:\n%s", out)
	}

	for _, deposit := range []string{"$5", "5", "0.49usd", "5000000uakt"} {
		_, err := executeCommand(t, newCommand(), sdl, "--deposit", deposit, "--dry-run")
		if err == nil || !strings.Contains(err.Error(), "funded automatically from your account credits") {
			t.Errorf("deposit %q error = %v, want the automatic-funding explanation", deposit, err)
		}
	}
}

func TestDeployPreflightRejectsDepositSDLDenominationMismatch(t *testing.T) {
	for _, test := range []struct {
		name       string
		authMethod string
		deposit    string
		sdlDenom   string
	}{
		{name: "chain", authMethod: aktctx.AuthMethodKeyring, deposit: "5000000uact", sdlDenom: "uakt"},
		{name: "console", authMethod: aktctx.AuthMethodConsoleAPI, deposit: "", sdlDenom: "uakt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			m := newTestManager(t, home, test.name, test.authMethod)
			cmd := findCommand(CommandsWithManager(
				func() string { return home },
				func() string { return test.name },
				func() *aktctx.Manager { return m },
			), "deploy")

			args := []string{writeWorkflowSDLWithDenom(t, test.sdlDenom)}
			if test.deposit != "" {
				args = append(args, "--deposit", test.deposit)
			}
			args = append(args, "--dry-run")

			out, err := executeCommand(t, cmd, args...)
			if err == nil {
				t.Fatalf("mismatched deployment unexpectedly passed:\n%s", out)
			}
			for _, want := range []string{"SDL price denomination", "deposit denomination", "uakt", "uact"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("preflight error %q missing %q", err, want)
				}
			}
			if strings.Contains(out, "Workflow:") {
				t.Fatalf("mismatched deployment printed a plan:\n%s", out)
			}
		})
	}
}

func TestDeployPreflightAcceptsMatchingExplicitChainDenomination(t *testing.T) {
	home := t.TempDir()
	m := newTestManager(t, home, "chain", aktctx.AuthMethodKeyring)
	cmd := findCommand(CommandsWithManager(
		func() string { return home },
		func() string { return "chain" },
		func() *aktctx.Manager { return m },
	), "deploy")

	out, err := executeCommand(t, cmd,
		writeWorkflowSDLWithDenom(t, "uakt"),
		"--deposit", "5000000uakt",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("matching explicit denomination: %v\n%s", err, out)
	}
}

func TestDeployPreflightReportsMalformedDepositAndUnreadableSDL(t *testing.T) {
	rc := &aktctx.Context{AuthMethod: aktctx.AuthMethodKeyring}

	err := validateDeploymentDenominations(map[string]any{
		"sdl-file":           "unused.yaml",
		flagdefs.FlagDeposit: "not-a-coin",
	}, rc)
	if err == nil || !strings.Contains(err.Error(), "resolve deployment deposit denomination") {
		t.Fatalf("malformed deposit error = %v", err)
	}

	missing := filepath.Join(t.TempDir(), "missing.yaml")
	err = validateDeploymentDenominations(map[string]any{
		"sdl-file":           missing,
		flagdefs.FlagDeposit: "5000000uact",
	}, rc)
	if err == nil || !strings.Contains(err.Error(), "read SDL") {
		t.Fatalf("unreadable SDL error = %v", err)
	}

	// On the chain rail the denomination comes from the deposit, so with no
	// deposit resolved there is nothing to compare the SDL against and the
	// preflight declines rather than guessing uact. It must decline before
	// reading the SDL, which "unused.yaml" would fail at.
	for name, params := range map[string]map[string]any{
		"absent deposit": {"sdl-file": "unused.yaml"},
		"blank deposit":  {"sdl-file": "unused.yaml", flagdefs.FlagDeposit: "  "},
	} {
		if err := validateDeploymentDenominations(params, rc); err != nil {
			t.Errorf("%s: chain preflight should decline quietly, got %v", name, err)
		}
	}
}

func TestDeployExecutionChecksDenominationsBeforeTransport(t *testing.T) {
	home := t.TempDir()
	m := newTestManager(t, home, "chain", aktctx.AuthMethodKeyring)
	cmd := findCommand(CommandsWithManager(
		func() string { return home },
		func() string { return "chain" },
		func() *aktctx.Manager { return m },
	), "deploy")

	_, err := executeCommand(t, cmd,
		writeWorkflowSDLWithDenom(t, "uakt"),
		"--deposit", "5000000uact",
	)
	if err == nil || !strings.Contains(err.Error(), "SDL price denomination") {
		t.Fatalf("execution preflight error = %v", err)
	}
	if strings.Contains(err.Error(), "no wallet/chain client") {
		t.Fatalf("transport was selected before denomination preflight: %v", err)
	}
}

// TestExecuteKeyringContextWithoutChainClient verifies the clear
// wallet/chain-client error when a keyring context has no chain client in
// the command context (the "neither credential" case).
func TestExecuteKeyringContextWithoutChainClient(t *testing.T) {
	home := t.TempDir()

	m := newTestManager(t, home, "wallet", aktctx.AuthMethodKeyring)

	cmds := CommandsWithManager(
		func() string { return home },
		func() string { return "wallet" },
		func() *aktctx.Manager { return m },
	)

	cmd := findCommand(cmds, "close")
	if cmd == nil {
		t.Fatal("CommandsWithManager() did not surface close")
	}

	_, err := executeCommand(t, cmd, "123")
	if err == nil {
		t.Fatal("expected wallet/chain-client error, got nil")
	}
	if !strings.Contains(err.Error(), "no wallet/chain client available") {
		t.Errorf("error %q does not explain that no wallet/chain client is available", err.Error())
	}
}

// TestExecuteConsoleDeployEndToEnd runs the built-in deploy workflow against
// a fake Console API: create deployment (SDL + USD deposit), poll bids via
// the Console fallback, auto-select the cheapest bid, create the lease with
// the cached manifest, and skip the provider send-manifest step.
func TestExecuteConsoleDeployEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv(aktctx.EnvConsoleAPIKey, "secret-key")

	var leaseBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/4242":
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"o","dseq":"4242"},"state":"active"},"leases":[{"id":{"owner":"o","dseq":"4242","gseq":1,"oseq":1,"provider":"akash1cheap"},"state":"active","status":{"services":{"web":{"available":1,"total":1,"uris":["web.example.test"]}}}}]}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments":
			_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"hasMore":false}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/deployments":
			_, _ = w.Write([]byte(`{"data":{"dseq":"4242","manifest":"[{\"name\":\"web\"}]","signTx":{"code":0,"transactionHash":"CREATEHASH"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/bids":
			_, _ = w.Write([]byte(`{"data":[
				{"bid":{"id":{"owner":"o","dseq":"4242","gseq":1,"oseq":1,"provider":"akash1exp"},"state":"open","price":{"denom":"uakt","amount":"25"}}},
				{"bid":{"id":{"owner":"o","dseq":"4242","gseq":1,"oseq":1,"provider":"akash1cheap"},"state":"open","price":{"denom":"uakt","amount":"10"}}}
			]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/providers/akash1cheap":
			_, _ = w.Write([]byte(`{"owner":"akash1cheap","isAudited":true,"attributes":[{"key":"region","value":"us-west"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/providers/akash1exp":
			_, _ = w.Write([]byte(`{"owner":"akash1exp","isAudited":false,"attributes":[{"key":"region","value":"eu-west"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/leases":
			if err := json.NewDecoder(r.Body).Decode(&leaseBody); err != nil {
				t.Errorf("decode lease body: %v", err)
			}
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"o","dseq":"4242"},"state":"active"},"leases":[{"id":{"owner":"o","dseq":"4242","gseq":1,"oseq":1,"provider":"akash1cheap"},"state":"active"}]}}`))
		default:
			t.Errorf("unexpected console request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	m := newTestManager(t, home, "console", aktctx.AuthMethodConsoleAPI)
	if err := m.UpdateContext("console", func(c *aktctx.Context) error {
		c.ConsoleAPIURL = srv.URL
		return nil
	}); err != nil {
		t.Fatalf("UpdateContext: %v", err)
	}

	sdlPath := writeValidWorkflowSDL(t)

	cmds := CommandsWithManager(
		func() string { return home },
		func() string { return "console" },
		func() *aktctx.Manager { return m },
	)

	cmd := findCommand(cmds, "deploy")
	if cmd == nil {
		t.Fatal("CommandsWithManager() did not surface deploy")
	}

	out, err := executeCommand(t, cmd, sdlPath, "--bid-select", "cheapest")
	if err != nil {
		t.Fatalf("deploy execute: %v\noutput:\n%s", err, out)
	}

	// The cheapest bid's lease must be created with the cached manifest.
	if leaseBody["manifest"] != `[{"name":"web"}]` {
		t.Errorf("lease manifest = %v, want the cached create-deployment manifest", leaseBody["manifest"])
	}
	leases, _ := leaseBody["leases"].([]any)
	if len(leases) != 1 {
		t.Fatalf("lease requests = %v, want exactly one", leaseBody["leases"])
	}
	lease, _ := leases[0].(map[string]any)
	if lease["dseq"] != "4242" || lease["provider"] != "akash1cheap" {
		t.Errorf("lease request = %v, want dseq 4242 provider akash1cheap", lease)
	}

	for _, want := range []string{
		"create-deployment", "tx: CREATEHASH",
		"wait-for-bids", "select-bid", "create-lease",
		"wait-for-ready", "URI (web): web.example.test", "Next:",
		"skipping step \"send-manifest\" (manifest submission handled by Console)",
		"completed successfully",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "send-manifest  ") {
		t.Errorf("send-manifest must not run for console auth:\n%s", out)
	}

	// SPEC §6.6: the run records its own outcome. Before this, a fully
	// successful deploy left `akt store status` reporting an empty store.
	st, err := bbolt.OpenContext(context.Background(), home, "console")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	dep, err := st.GetDeployment(context.Background(), "o", 4242)
	if err != nil {
		t.Fatalf("GetDeployment: %v", err)
	}
	if dep == nil {
		t.Fatal("a completed deploy left the local store empty")
		return
	}
	if dep.State != "active" || dep.SDLPath != sdlPath {
		t.Errorf("stored deployment = %+v, want active with the deployed SDL path", dep)
	}

	storedLeases, err := st.ListLeases(context.Background(), sstore.LeaseFilter{Owner: "o", DSeq: 4242})
	if err != nil {
		t.Fatalf("ListLeases: %v", err)
	}
	if len(storedLeases) != 1 || storedLeases[0].ID.Provider != "akash1cheap" {
		t.Errorf("stored leases = %+v, want the won lease", storedLeases)
	}

	bids, err := st.ListBids(context.Background(), sstore.BidFilter{Owner: "o", DSeq: 4242})
	if err != nil {
		t.Fatalf("ListBids: %v", err)
	}
	if len(bids) != 2 {
		t.Fatalf("stored bids = %d, want every bid seen", len(bids))
	}
	for _, b := range bids {
		want := "lost"
		if b.ID.Provider == "akash1cheap" {
			want = "matched"
			if !b.ProviderAudited || b.ProviderAttributes["region"] != "us-west" {
				t.Errorf("cheap provider metadata = %#v audited=%v", b.ProviderAttributes, b.ProviderAudited)
			}
		} else if b.ProviderAudited || b.ProviderAttributes["region"] != "eu-west" {
			t.Errorf("expensive provider metadata = %#v audited=%v", b.ProviderAttributes, b.ProviderAudited)
		}
		if b.State != want {
			t.Errorf("bid from %s = %q, want %q", b.ID.Provider, b.State, want)
		}
	}
}

func TestWorkflowCommandFailsWhenFinalReportWriteFails(t *testing.T) {
	tests := []struct {
		name           string
		outputArgs     []string
		responseStatus int
		shortWrite     bool
		wantReport     string
		wantRunFailure bool
		wantRequests   int
	}{
		{
			name:         "pretty writer error",
			wantReport:   "write workflow plan report",
			wantRequests: -1,
		},
		{
			name:         "pretty short write",
			shortWrite:   true,
			wantReport:   "write workflow plan report",
			wantRequests: -1,
		},
		{
			name:       "JSONL writer error",
			outputArgs: []string{"--output", "jsonl"},
			wantReport: "write workflow JSONL report",
		},
		{
			name:       "JSONL short write",
			outputArgs: []string{"--output", "jsonl"},
			shortWrite: true,
			wantReport: "write workflow JSONL report",
		},
		{
			name:           "engine and JSONL writer errors",
			outputArgs:     []string{"--output", "jsonl"},
			responseStatus: http.StatusInternalServerError,
			wantReport:     "write workflow JSONL report",
			wantRunFailure: true,
			wantRequests:   4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv(aktctx.EnvConsoleAPIKey, "secret-key")
			writeFinalReportTestWorkflow(t, home)

			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/123" {
					_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"123"},"state":"active"}}}`))
					return
				}
				if r.Method != http.MethodDelete || r.URL.Path != "/v1/deployments/123" {
					t.Errorf("unexpected Console request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				status := tc.responseStatus
				if status == 0 {
					_, _ = w.Write([]byte(`{"data":{"success":true}}`))
					return
				}
				w.WriteHeader(status)
			}))
			t.Cleanup(srv.Close)

			m := newTestManager(t, home, "console", aktctx.AuthMethodConsoleAPI)
			if err := m.UpdateContext("console", func(c *aktctx.Context) error {
				c.ConsoleAPIURL = srv.URL
				return nil
			}); err != nil {
				t.Fatalf("UpdateContext: %v", err)
			}

			cmd := findCommand(CommandsWithManager(
				func() string { return home },
				func() string { return "console" },
				func() *aktctx.Manager { return m },
			), "close")
			if cmd == nil {
				t.Fatal("CommandsWithManager() did not surface close")
			}

			writeErr := errors.New("stdout unavailable")
			writer := &workflowFailingWriter{err: writeErr, short: tc.shortWrite}
			var stderr bytes.Buffer
			cmd.SetOut(writer)
			cmd.SetErr(&stderr)
			cmd.SetArgs(append([]string{"123"}, tc.outputArgs...))

			err := cmd.Execute()
			wantErr := writeErr
			if tc.shortWrite {
				wantErr = io.ErrShortWrite
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("execute error = %v, want final stdout failure", err)
			}
			if !strings.Contains(err.Error(), tc.wantReport) {
				t.Fatalf("execute error = %v, want final report context %q", err, tc.wantReport)
			}
			if tc.wantRunFailure && !strings.Contains(err.Error(), `workflow "close" failed`) {
				t.Fatalf("execute error = %v, want engine failure preserved with writer failure", err)
			}
			wantRequests := tc.wantRequests
			switch wantRequests {
			case -1:
				wantRequests = 0
			case 0:
				wantRequests = 2
			}
			if requests != wantRequests {
				t.Fatalf("Console close requests = %d, want %d engine attempts before rendering", requests, wantRequests)
			}
			if writer.writes == 0 {
				t.Fatal("stdout writer was never called")
			}
		})
	}
}

func TestWorkflowCommandStopsBeforeMutationWhenPlanWriteFails(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		shortWrite bool
		wantReport string
	}{
		{name: "pretty plan writer error", args: []string{"123"}, wantReport: "write workflow plan report"},
		{name: "pretty plan short write", args: []string{"123"}, shortWrite: true, wantReport: "write workflow plan report"},
		{name: "pretty dry-run writer error", args: []string{"123", "--dry-run"}, wantReport: "write workflow plan report"},
		{name: "pretty dry-run short write", args: []string{"123", "--dry-run"}, shortWrite: true, wantReport: "write workflow plan report"},
		{name: "JSONL dry-run short write", args: []string{"123", "--dry-run", "--output", "jsonl"}, shortWrite: true, wantReport: "render workflow dry-run JSONL"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv(aktctx.EnvConsoleAPIKey, "secret-key")
			writeFinalReportTestWorkflow(t, home)

			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(srv.Close)

			m := newTestManager(t, home, "console", aktctx.AuthMethodConsoleAPI)
			if err := m.UpdateContext("console", func(c *aktctx.Context) error {
				c.ConsoleAPIURL = srv.URL
				return nil
			}); err != nil {
				t.Fatalf("UpdateContext: %v", err)
			}

			cmd := findCommand(CommandsWithManager(
				func() string { return home },
				func() string { return "console" },
				func() *aktctx.Manager { return m },
			), "close")
			if cmd == nil {
				t.Fatal("CommandsWithManager() did not surface close")
			}

			writeErr := errors.New("stdout unavailable")
			writer := &workflowFailingWriter{err: writeErr, short: tc.shortWrite}
			cmd.SetOut(writer)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			wantErr := writeErr
			if tc.shortWrite {
				wantErr = io.ErrShortWrite
			}
			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), tc.wantReport) {
				t.Fatalf("execute error = %v, want %q wrapping %v", err, tc.wantReport, wantErr)
			}
			if requests != 0 {
				t.Fatalf("Console requests = %d, want 0 after pre-execution output failure", requests)
			}
		})
	}
}

func TestWorkflowCommandReturnsRunFailureAfterWritingFinalReport(t *testing.T) {
	home := t.TempDir()
	t.Setenv(aktctx.EnvConsoleAPIKey, "secret-key")
	writeFinalReportTestWorkflow(t, home)

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/123" {
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"123"},"state":"active"}}}`))
			return
		}
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/deployments/123" {
			t.Errorf("unexpected Console request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.Error(w, "console unavailable", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	m := newTestManager(t, home, "console", aktctx.AuthMethodConsoleAPI)
	if err := m.UpdateContext("console", func(c *aktctx.Context) error {
		c.ConsoleAPIURL = srv.URL
		return nil
	}); err != nil {
		t.Fatalf("UpdateContext: %v", err)
	}

	cmd := findCommand(CommandsWithManager(
		func() string { return home },
		func() string { return "console" },
		func() *aktctx.Manager { return m },
	), "close")
	if cmd == nil {
		t.Fatal("CommandsWithManager() did not surface close")
	}

	out, err := executeCommand(t, cmd, "123")
	if err == nil || !strings.Contains(err.Error(), `workflow "close" failed`) {
		t.Fatalf("execute error = %v, want workflow failure", err)
	}
	if requests != 4 {
		t.Fatalf("Console close requests = %d, want one preflight and three DELETE attempts", requests)
	}
	for _, want := range []string{"Results:", "close-deployment", "failed", "HTTP 500"} {
		if !strings.Contains(out, want) {
			t.Errorf("final report does not contain %q:\n%s", want, out)
		}
	}
}

func writeFinalReportTestWorkflow(t *testing.T, home string) {
	t.Helper()

	dir := filepath.Join(home, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create workflow directory: %v", err)
	}

	definition := `name: close
description: Close without an output step so the test isolates the final report
version: 1

params:
  dseq:
    type: int
    required: true
    description: Deployment sequence to close

steps:
  - name: close-deployment
    type: tx
    msg: deployment.MsgCloseDeployment
    params:
      dseq: "{{ .Params.dseq }}"
    on-error: abort
`
	if err := os.WriteFile(filepath.Join(dir, "close.yaml"), []byte(definition), 0o600); err != nil {
		t.Fatalf("write workflow definition: %v", err)
	}
}

// TestCommandsCarryCapabilityAnnotation verifies every generated workflow
// command declares the either-rail capability requirement used by command
// gating: chain-tx (keyring) or console (console-api).
func TestCommandsCarryCapabilityAnnotation(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmds := Commands(homeFn, ctxNameFn)
	if len(cmds) == 0 {
		t.Fatal("Commands() surfaced no workflow commands")
	}

	for _, cmd := range cmds {
		if got := cmd.Annotations[capability.AnnotationKey]; got != "chain-tx|console" {
			t.Errorf("%s: annotation %q = %q, want %q", cmd.Name(), capability.AnnotationKey, got, "chain-tx|console")
		}
	}
}

// argumentSurface flattens a command's user-visible argument surface — the
// Use line (positionals) plus every flag's name, shorthand, type, default,
// and usage — into a deterministic string for comparison.
func argumentSurface(cmd *cobra.Command) string {
	var flags []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		flags = append(flags, strings.Join([]string{f.Name, f.Shorthand, f.Value.Type(), f.DefValue, f.Usage}, "|"))
	})
	sort.Strings(flags)

	return cmd.Use + "\n" + strings.Join(flags, "\n")
}

// TestArgumentSurfaceAuthIndependent locks in the transport parity guarantee
// (SPEC §2.3): a workflow command's argument surface — positionals in Use and
// the full flag set — is generated from the workflow definition alone and is
// identical whether the active context uses keyring auth, console-api auth,
// or no context manager exists at all. Transports may branch on behavior at
// execution time, never on the argument surface.
func TestArgumentSurfaceAuthIndependent(t *testing.T) {
	keyringHome := t.TempDir()
	keyringMgr := newTestManager(t, keyringHome, "wallet", aktctx.AuthMethodKeyring)

	consoleHome := t.TempDir()
	consoleMgr := newTestManager(t, consoleHome, "console", aktctx.AuthMethodConsoleAPI)

	noneHome := t.TempDir()

	variants := []struct {
		name string
		cmds []*cobra.Command
	}{
		{"no-manager", Commands(staticFns(noneHome))},
		{"keyring", CommandsWithManager(
			func() string { return keyringHome },
			func() string { return "wallet" },
			func() *aktctx.Manager { return keyringMgr },
		)},
		{"console-api", CommandsWithManager(
			func() string { return consoleHome },
			func() string { return "console" },
			func() *aktctx.Manager { return consoleMgr },
		)},
	}

	for _, name := range []string{"deploy", "update", "close"} {
		base := findCommand(variants[0].cmds, name)
		if base == nil {
			t.Fatalf("variant %q did not surface %q", variants[0].name, name)
		}
		want := argumentSurface(base)

		for _, v := range variants[1:] {
			cmd := findCommand(v.cmds, name)
			if cmd == nil {
				t.Fatalf("variant %q did not surface %q", v.name, name)
			}
			if got := argumentSurface(cmd); got != want {
				t.Errorf("%s: argument surface differs between %q and %q:\n--- %s\n%s\n--- %s\n%s",
					name, variants[0].name, v.name, variants[0].name, want, v.name, got)
			}
		}
	}
}

func TestDeployAcceptsPositionalDepositOnTheChainRail(t *testing.T) {
	tests := []struct {
		name       string
		authMethod string
		deposit    string
	}{
		{name: "chain coin", authMethod: aktctx.AuthMethodKeyring, deposit: "5000000uact"},
		{name: "chain small coin", authMethod: aktctx.AuthMethodKeyring, deposit: "5uact"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			manager := newTestManager(t, home, "deploy", test.authMethod)
			cmd := findCommand(CommandsWithManager(
				func() string { return home },
				func() string { return "deploy" },
				func() *aktctx.Manager { return manager },
			), "deploy")
			if cmd == nil {
				t.Fatal("deploy command not found")
			}

			out, err := executeCommand(t, cmd, writeValidWorkflowSDL(t), test.deposit, "--dry-run")
			if err != nil {
				t.Fatalf("positional deposit dry run: %v\n%s", err, out)
			}
			if !strings.Contains(out, "deposit:") || !strings.Contains(out, test.deposit) {
				t.Fatalf("plan does not contain positional deposit %q:\n%s", test.deposit, out)
			}
		})
	}
}

func TestDeployRejectsPositionalAndFlagDepositsTogether(t *testing.T) {
	home := t.TempDir()
	manager := newTestManager(t, home, "chain", aktctx.AuthMethodKeyring)
	cmd := findCommand(CommandsWithManager(
		func() string { return home },
		func() string { return "chain" },
		func() *aktctx.Manager { return manager },
	), "deploy")
	if cmd == nil {
		t.Fatal("deploy command not found")
	}

	_, err := executeCommand(t, cmd, writeValidWorkflowSDL(t), "5", "--deposit", "6", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "deposit supplied both positionally and with --deposit") {
		t.Fatalf("deposit conflict error = %v", err)
	}
}

// TestExecuteWithoutManager verifies that commands built via the legacy
// Commands constructor (nil manager) fail execution with a clear
// no-configuration message instead of a panic or a silent no-op.
func TestExecuteWithoutManager(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())

	cmd := findCommand(Commands(homeFn, ctxNameFn), "close")
	if cmd == nil {
		t.Fatal("Commands() did not surface close")
	}

	_, err := executeCommand(t, cmd, "123")
	if err == nil {
		t.Fatal("expected no-configuration error, got nil")
	}
	if !strings.Contains(err.Error(), "no configuration loaded") {
		t.Errorf("error %q does not mention missing configuration", err.Error())
	}
}

func TestBuiltinWorkflowParamTypesMatchPreflightContracts(t *testing.T) {
	deploy := loadBuiltin(t, "deploy")
	for name, want := range map[string]string{
		"sdl-file":   "sdl",
		"deposit":    "deposit",
		"bid-select": "bid-selection",
	} {
		if got := string(deploy.Params[name].Type); got != want {
			t.Errorf("deploy param %q type = %q, want %q", name, got, want)
		}
	}

	update := loadBuiltin(t, "update")
	if got := string(update.Params["sdl-file"].Type); got != "sdl" {
		t.Errorf("update param %q type = %q, want %q", "sdl-file", got, "sdl")
	}
}

func TestBuiltinUpdateSendsManifestBeforeReportingSuccess(t *testing.T) {
	update := loadBuiltin(t, "update")
	if len(update.Steps) != 3 {
		t.Fatalf("update steps = %+v, want update, manifest delivery, display", update.Steps)
	}

	manifest := update.Steps[1]
	if manifest.Name != "send-manifest" || manifest.Type != wf.StepProvider || manifest.Action != "send-manifest-to-active-leases" {
		t.Fatalf("update manifest step = %+v", manifest)
	}
	if manifest.Retry == nil || manifest.Retry.Max != 3 || manifest.Retry.Delay != "5s" {
		t.Errorf("update manifest retry = %+v, want 3 attempts at 5s", manifest.Retry)
	}
	if update.Steps[2].Type != wf.StepOutput {
		t.Errorf("success output must follow manifest delivery: %+v", update.Steps)
	}
}

func TestExecuteConsoleUpdateUsesConsoleManifestHandling(t *testing.T) {
	home := t.TempDir()
	t.Setenv(aktctx.EnvConsoleAPIKey, "secret-key")
	sdlPath := writeValidWorkflowSDL(t)
	expectedHash := workflowSDLVersionHash(t, sdlPath)

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/4242" {
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"o","dseq":"4242"},"state":"active"},"leases":[]}}`))
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/deployments/4242" {
			t.Errorf("unexpected console request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"o","dseq":"4242"},"state":"active","hash":"` + expectedHash + `"},"leases":[]}}`))
	}))
	t.Cleanup(srv.Close)

	m := newTestManager(t, home, "console", aktctx.AuthMethodConsoleAPI)
	if err := m.UpdateContext("console", func(c *aktctx.Context) error {
		c.ConsoleAPIURL = srv.URL
		return nil
	}); err != nil {
		t.Fatalf("UpdateContext: %v", err)
	}

	cmd := findCommand(CommandsWithManager(
		func() string { return home },
		func() string { return "console" },
		func() *aktctx.Manager { return m },
	), "update")
	if cmd == nil {
		t.Fatal("CommandsWithManager() did not surface update")
	}

	out, err := executeCommand(t, cmd, sdlPath, "4242")
	if err != nil {
		t.Fatalf("update execute: %v\noutput:\n%s", err, out)
	}
	if requests != 2 {
		t.Errorf("Console requests = %d, want one preflight and one update", requests)
	}
	for _, want := range []string{
		"skipping step \"send-manifest\" (manifest submission handled by Console)",
		"update-deployment", "display-result", "completed successfully",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

func workflowSDLVersionHash(t *testing.T, path string) string {
	t.Helper()

	rawSDL, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow SDL: %v", err)
	}
	doc, err := sdl.Read(rawSDL)
	if err != nil {
		t.Fatalf("parse workflow SDL: %v", err)
	}
	version, err := doc.Version()
	if err != nil {
		t.Fatalf("derive workflow SDL version: %v", err)
	}

	return base64.StdEncoding.EncodeToString(version)
}

func writeValidWorkflowSDL(t *testing.T) string {
	return writeWorkflowSDLWithDenom(t, "uact")
}

func writeWorkflowSDLWithDenom(t *testing.T, denom string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "deploy.yaml")
	data := `---
version: "2.0"
services:
  web:
    image: nginx:1.27
    expose:
      - port: 80
        to:
          - global: true
profiles:
  compute:
    web:
      resources:
        cpu:
          units: "100m"
        memory:
          size: "128Mi"
        storage:
          size: "1Gi"
  placement:
    westcoast:
      pricing:
        web:
          denom: ` + denom + `
          amount: 50
deployment:
  web:
    westcoast:
      profile: web
      count: 1
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestWorkflowDryRunValidatesInputsBeforePrintingPlan(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())
	validSDL := writeValidWorkflowSDL(t)
	invalidSDL := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(invalidSDL, []byte("services: [not-an-sdl"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		command string
		args    []string
		wantErr string
	}{
		{name: "missing required dseq", command: "close", args: []string{"--dry-run"}, wantErr: "dseq"},
		{name: "zero dseq", command: "close", args: []string{"--dry-run", "0"}, wantErr: "greater than zero"},
		{name: "negative dseq", command: "close", args: []string{"--dry-run", "--dseq=-1"}, wantErr: "greater than zero"},
		{name: "conflicting dseq forms", command: "close", args: []string{"--dry-run", "1", "--dseq", "2"}, wantErr: "both positionally"},
		{name: "missing SDL file", command: "deploy", args: []string{"--dry-run", filepath.Join(t.TempDir(), "missing.yaml")}, wantErr: "sdl-file"},
		{name: "invalid SDL", command: "deploy", args: []string{"--dry-run", invalidSDL}, wantErr: "invalid SDL"},
		{name: "invalid deposit", command: "deploy", args: []string{"--dry-run", validSDL, "--deposit", "five-ish"}, wantErr: "invalid deposit"},
		{name: "zero duration", command: "deploy", args: []string{"--dry-run", validSDL, "--bid-timeout", "0s"}, wantErr: "greater than zero"},
		{name: "invalid duration", command: "deploy", args: []string{"--dry-run", validSDL, "--bid-timeout", "eventually"}, wantErr: "invalid duration"},
		{name: "invalid bid mode", command: "deploy", args: []string{"--dry-run", validSDL, "--bid-select", "random"}, wantErr: "bid selection"},
		{name: "invalid provider address", command: "deploy", args: []string{"--dry-run", validSDL, "--bid-select", "provider=short"}, wantErr: "provider address"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := findCommand(Commands(homeFn, ctxNameFn), tc.command)
			if cmd == nil {
				t.Fatalf("workflow command %q not found", tc.command)
			}

			out, err := executeCommand(t, cmd, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want text %q\noutput:\n%s", err, tc.wantErr, out)
			}
			if strings.Contains(out, "Workflow:") || strings.Contains(out, "Dry run") {
				t.Fatalf("invalid input printed a plan:\n%s", out)
			}
		})
	}
}

func TestWorkflowDryRunAcceptsTypedInputs(t *testing.T) {
	homeFn, ctxNameFn := staticFns(t.TempDir())
	cmd := findCommand(Commands(homeFn, ctxNameFn), "deploy")
	if cmd == nil {
		t.Fatal("deploy command not found")
	}

	out, err := executeCommand(
		t,
		cmd,
		"--dry-run",
		writeValidWorkflowSDL(t),
		"--deposit", "$5",
		"--bid-timeout", "30s",
		"--bid-select", "provider=akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
	)
	if err != nil {
		t.Fatalf("valid dry run: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Workflow: deploy") || !strings.Contains(out, "Dry run") {
		t.Fatalf("valid dry run did not print its plan:\n%s", out)
	}
}

func TestUserWorkflowParamsUseTheSameTypedValidation(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.txt")
	tests := []struct {
		name   string
		def    *wf.WorkflowDef
		params map[string]any
		want   string
	}{
		{
			name: "required string",
			def: &wf.WorkflowDef{Params: map[string]wf.ParamDef{
				"label": {Type: wf.ParamString, Required: true},
			}},
			params: map[string]any{"label": ""},
			want:   "required",
		},
		{
			name: "readable file",
			def: &wf.WorkflowDef{Params: map[string]wf.ParamDef{
				"input": {Type: wf.ParamFile},
			}},
			params: map[string]any{"input": missing},
			want:   "read file",
		},
		{
			name: "declared type",
			def: &wf.WorkflowDef{Params: map[string]wf.ParamDef{
				"enabled": {Type: wf.ParamBool},
			}},
			params: map[string]any{"enabled": "yes"},
			want:   "boolean",
		},
		{
			name: "unsupported type",
			def: &wf.WorkflowDef{Params: map[string]wf.ParamDef{
				"mystery": {Type: wf.ParamType("guess")},
			}},
			params: map[string]any{"mystery": "value"},
			want:   "unsupported type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWorkflowParams(tc.def, tc.params)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want text %q", err, tc.want)
			}
		})
	}
}
