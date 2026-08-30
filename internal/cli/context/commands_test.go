package context

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	stdcontext "context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"pkg.akt.dev/akt/internal/actionlog"
	aktctx "pkg.akt.dev/akt/internal/context"
)

// runOK executes a command and fails the test if it errors.
func runOK(t *testing.T, cmd *cobra.Command, args ...string) {
	t.Helper()

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
}

// runOutput executes a command with an isolated Cobra writer and returns its
// stdout. Command output must never depend on replacing the process stdout.
func runOutput(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}

	return out.String()
}

// runErr executes a command expecting a failure and returns the error.
func runErr(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("execute %v: expected an error", args)
	}

	return err
}

// TestCreateRequiresNetworkUnlessConsole covers the one place `context create`
// validates in RunE rather than with MarkFlagRequired: a keyring context with
// no network is unusable, but a console-api context legitimately has none.
func TestCreateRequiresNetworkUnlessConsole(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	err := runErr(t, createCmd(mgrFn), "broken")
	if !strings.Contains(err.Error(), "--network is required") {
		t.Errorf("unexpected error: %v", err)
	}
	if m.GetContext("broken") != nil {
		t.Error("a rejected create must not leave a context behind")
	}

	// console-api contexts may omit the network entirely.
	runOK(t, createCmd(mgrFn), "managed", "--auth-method", aktctx.AuthMethodConsoleAPI)

	c := m.GetContext("managed")
	if c == nil {
		t.Fatal("network-less console-api context was not created")
		return
	}
	if c.AuthMethod != aktctx.AuthMethodConsoleAPI {
		t.Errorf("auth method = %q, want console-api", c.AuthMethod)
	}
}

func TestCreatePrintsEffectiveContextConfirmation(t *testing.T) {
	m := newTestManager(t)
	cmd := createCmd(func() *aktctx.Manager { return m })
	out := runOutput(t, cmd, "prod", "--network", "mainnet", "--set-current")
	for _, want := range []string{`Context "prod" created`, "network: mainnet", "deploy-via: chain", "current: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("create confirmation %q missing %q", out, want)
		}
	}
}

func TestCreateDeployViaSelectsPreferredWorkflowRail(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn),
		"managed", "--network", "mainnet", "--deploy-via", "console",
	)
	if got := m.GetContext("managed").AuthMethod; got != aktctx.AuthMethodConsoleAPI {
		t.Fatalf("console deploy preference stored auth method %q", got)
	}

	runOK(t, createCmd(mgrFn),
		"local", "--network", "mainnet", "--deploy-via", "chain",
	)
	if got := m.GetContext("local").AuthMethod; got != aktctx.AuthMethodKeyring {
		t.Fatalf("chain deploy preference stored auth method %q", got)
	}
}

func TestCreateRejectsInvalidDeployRailBeforePersisting(t *testing.T) {
	m := newTestManager(t)
	err := runErr(t, createCmd(func() *aktctx.Manager { return m }),
		"invalid", "--network", "mainnet", "--deploy-via", "carrier-pigeon",
	)
	if !strings.Contains(err.Error(), "invalid deploy rail") {
		t.Fatalf("invalid deploy preference error = %v", err)
	}
	if m.GetContext("invalid") != nil {
		t.Fatal("invalid deploy preference persisted a context")
	}
}

func TestCreateRejectsConflictingDeployRailFlags(t *testing.T) {
	m := newTestManager(t)
	err := runErr(t, createCmd(func() *aktctx.Manager { return m }),
		"ambiguous", "--network", "mainnet",
		"--deploy-via", "console",
		"--auth-method", aktctx.AuthMethodKeyring,
	)

	if !strings.Contains(err.Error(), "none of the others") {
		t.Fatalf("conflicting rail error = %v", err)
	}
	if m.GetContext("ambiguous") != nil {
		t.Fatal("rejected create persisted an ambiguous context")
	}
}

func TestCreateRejectsInvocationKeyringBackendOverride(t *testing.T) {
	m := newTestManager(t)
	root := Commands(func() *aktctx.Manager { return m }, func() (sdkkeyring.Keyring, error) {
		return nil, errors.New("keyring must not open")
	})
	root.PersistentFlags().String(flagdefs.FlagKeyringBackend, "", "")

	err := runErr(t, root,
		"--"+flagdefs.FlagKeyringBackend, "file",
		"create", "prod", "--network", "mainnet",
	)
	for _, want := range []string{"--keyring-backend", "akt context keyring set"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("create override error = %q, want %q", err, want)
		}
	}
	if m.GetContext("prod") != nil {
		t.Fatal("rejected context create mutated configuration")
	}
}

// TestCreateStoresConsoleKeyOutsideConfig pins SPEC §7.1: the key passed to
// `context create --console-api-key` lands in the per-context credential file
// with 0600, and never in config.yaml or the action log.
func TestCreateStoresConsoleKeyOutsideConfig(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn),
		"prod", "--network", "mainnet",
		"--auth-method", aktctx.AuthMethodConsoleAPI,
		"--console-api-key", "sk-super-secret",
	)

	stored, err := aktctx.StoredConsoleAPIKey(m.Root(), "prod")
	if err != nil {
		t.Fatalf("StoredConsoleAPIKey: %v", err)
	}
	if stored != "sk-super-secret" {
		t.Errorf("stored key = %q", stored)
	}

	info, err := os.Stat(aktctx.ConsoleAPIKeyPath(m.Root(), "prod"))
	if err != nil {
		t.Fatalf("stat credential: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credential mode = %o, want 600", perm)
	}

	cfg, err := os.ReadFile(aktctx.ConfigPath(m.Root()))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(cfg), "sk-super-secret") {
		t.Error("the Console API key must never be written to config.yaml")
	}

	for _, e := range readLog(t, m.Root(), "prod") {
		if strings.Contains(string(e.Params), "sk-super-secret") {
			t.Errorf("the Console API key must never reach the action log: %+v", e)
		}
	}
}

// TestEditRecordsCredentialChangeWithoutTheKey covers the credential branch of
// `context edit`: setting and clearing the key must be recorded as
// updated/removed, never as the key itself.
func TestEditRecordsCredentialChangeWithoutTheKey(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")
	runOK(t, editCmd(mgrFn), "prod", "--console-api-key", "sk-rotated")

	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "prod"); got != "sk-rotated" {
		t.Errorf("stored key = %q, want sk-rotated", got)
	}

	entries := readLog(t, m.Root(), "prod")
	params := string(entries[0].Params)
	if !strings.Contains(params, `"console-api-key":"updated"`) {
		t.Errorf("edit params = %s, want console-api-key:updated", params)
	}
	if strings.Contains(params, "sk-rotated") {
		t.Error("the key value must never be logged")
	}

	// An empty value removes the credential.
	runOK(t, editCmd(mgrFn), "prod", "--console-api-key", "")

	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "prod"); got != "" {
		t.Errorf("key after removal = %q, want empty", got)
	}
	if params := string(readLog(t, m.Root(), "prod")[0].Params); !strings.Contains(params, `"console-api-key":"removed"`) {
		t.Errorf("removal params = %s, want console-api-key:removed", params)
	}
}

// TestEditRejectsInvalidAuthMethod covers the auth-method validation inside
// the UpdateContext callback. An unknown method would leave the context
// choosing neither rail at resolve time.
func TestEditRejectsInvalidAuthMethod(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")

	err := runErr(t, editCmd(mgrFn), "prod", "--auth-method", "carrier-pigeon")
	if !strings.Contains(err.Error(), "invalid auth-method") {
		t.Errorf("unexpected error: %v", err)
	}

	if got := m.GetContext("prod").AuthMethod; got == "carrier-pigeon" {
		t.Error("a rejected auth-method must not be persisted")
	}
}

func TestEditDeployViaChangesOnlyPreferredWorkflowRail(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }
	runOK(t, createCmd(mgrFn),
		"prod", "--network", "mainnet", "--default-account", "alice",
		"--console-api-key", "secret",
	)

	runOK(t, editCmd(mgrFn), "prod", "--deploy-via", "console")
	rc, resolveErr := m.Resolve("prod")
	if resolveErr != nil {
		t.Fatalf("resolve edited context: %v", resolveErr)
	}
	if got := rc.AuthMethod; got != aktctx.AuthMethodConsoleAPI {
		t.Fatalf("edited deploy preference = %q, want console-api", got)
	}
	if rc.Network.Name != "mainnet" || rc.Keyring.Name != "default" || rc.DefaultAccount != "alice" || rc.ConsoleAPIKey != "secret" {
		t.Fatal("editing deploy preference changed another credential or context field")
	}

	err := runErr(t, editCmd(mgrFn), "prod", "--deploy-via", "carrier-pigeon")
	if !strings.Contains(err.Error(), "invalid deploy rail") {
		t.Fatalf("invalid deploy preference error = %v", err)
	}
	if got := m.GetContext("prod").AuthMethod; got != aktctx.AuthMethodConsoleAPI {
		t.Fatalf("rejected edit persisted auth method %q", got)
	}
}

func TestCreateAndEditProviderAuthType(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn),
		"prod", "--network", "mainnet", "--provider-auth-type", "mtls")
	if got := m.GetContext("prod").ProviderDefaults.AuthType; got != "mtls" {
		t.Fatalf("created provider auth type = %q, want mtls", got)
	}

	runOK(t, editCmd(mgrFn), "prod", "--provider-auth-type", "jwt")
	if got := m.GetContext("prod").ProviderDefaults.AuthType; got != "jwt" {
		t.Fatalf("edited provider auth type = %q, want jwt", got)
	}

	err := runErr(t, editCmd(mgrFn), "prod", "--provider-auth-type", "password")
	if !strings.Contains(err.Error(), "provider auth type") {
		t.Fatalf("error = %v, want provider auth enum validation", err)
	}
	if got := m.GetContext("prod").ProviderDefaults.AuthType; got != "jwt" {
		t.Fatalf("rejected edit persisted provider auth type %q", got)
	}
}

// TestEditRejectsForkNetworkWithNetwork covers the mutually-exclusive flag
// guard: forking and switching networks in one command is ambiguous, and
// silently picking one would move the context to the wrong network.
func TestEditRejectsForkNetworkWithNetwork(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")

	err := runErr(t, editCmd(mgrFn), "prod", "--fork-network", "--network", "mainnet")
	if !strings.Contains(err.Error(), "cannot use --fork-network with --network") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestEditOnlyTouchesChangedFields pins the Changed()-gated update logic: an
// edit that names one flag must not reset the others to their zero defaults.
// Without the guard, `--default-account bob` would wipe the gas setting.
func TestEditOnlyTouchesChangedFields(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod",
		"--network", "mainnet", "--gas", "300000", "--fees", "5000uakt",
		"--default-account", "alice")

	runOK(t, editCmd(mgrFn), "prod", "--default-account", "bob")

	c := m.GetContext("prod")
	if c.DefaultAccount != "bob" {
		t.Errorf("default account = %q, want bob", c.DefaultAccount)
	}
	if c.Gas != "300000" {
		t.Errorf("gas = %q, want the untouched 300000", c.Gas)
	}
	if c.Fees != "5000uakt" {
		t.Errorf("fees = %q, want the untouched 5000uakt", c.Fees)
	}
	if c.Network.Name != "mainnet" {
		t.Errorf("network = %q, want the untouched mainnet", c.Network.Name)
	}
}

// TestListMarksCurrentContext covers listCmd, including the current-context
// marker and the resolved chain-id column (which comes from the network, not
// the context).
func TestListMarksCurrentContext(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--default-account", "alice", "--set-current")
	runOK(t, createCmd(mgrFn), "staging", "--network", "mainnet")

	out := runOutput(t, listCmd(mgrFn))

	var prodLine, stagingLine string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "prod"):
			prodLine = line
		case strings.Contains(line, "staging"):
			stagingLine = line
		}
	}

	if !strings.HasPrefix(strings.TrimLeft(prodLine, " "), "*") {
		t.Errorf("current context must be marked with *, got %q", prodLine)
	}
	if strings.Contains(stagingLine, "*") {
		t.Errorf("non-current context must not be marked, got %q", stagingLine)
	}
	if !strings.Contains(prodLine, "alice") {
		t.Errorf("default account column missing: %q", prodLine)
	}
	if !strings.Contains(out, "CHAIN-ID") {
		t.Errorf("chain-id column header missing from %q", out)
	}
}

// TestListWithNoContextsExplainsHowToCreateOne covers the empty branch: an
// empty table would leave a first-run user with no next step.
func TestListWithNoContextsExplainsHowToCreateOne(t *testing.T) {
	m := newTestManager(t)

	out := runOutput(t, listCmd(func() *aktctx.Manager { return m }))

	if !strings.Contains(out, "akt context create") {
		t.Errorf("empty list should point at the create command, got %q", out)
	}
}

func TestEmptyContextListUsesStructuredCommandWriter(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			m := newTestManager(t)
			cmd := listCmd(func() *aktctx.Manager { return m })
			cmd.Flags().String(flagdefs.FlagOutput, "pretty", "test output format")
			out := runOutput(t, cmd, "--output", format)
			if !strings.HasSuffix(strings.TrimSpace(out), "[]") {
				t.Fatalf("empty %s context list = %q, want an empty sequence", format, out)
			}
		})
	}
}

// TestShowResolvesTheActiveContext covers currentCmd's success and failure
// branches. With no current context the command must error rather than print
// an empty record.
func TestShowResolvesTheActiveContext(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	if err := currentCmd(mgrFn).Execute(); err == nil {
		t.Error("show without a current context must fail")
	}

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	cmd := currentCmd(mgrFn)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("context show: %v", err)
	}
	mainnet := m.GetNetwork("mainnet")
	if mainnet == nil {
		t.Fatal("mainnet network missing from test config")
		return
	}
	out := stdout.String()
	if !strings.Contains(out, "prod") || !strings.Contains(out, mainnet.ChainID) {
		t.Errorf("show output should name the context and resolved chain, got %q", out)
	}
}

func TestShowUsesPlainCommandWriterOutsideTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }
	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	cmd := currentCmd(mgrFn)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("context show: %v", err)
	}
	if !strings.Contains(stdout.String(), "prod") {
		t.Fatalf("context show did not use command writer: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("context show emitted ANSI outside a TTY: %q", stdout.String())
	}
}

func TestShowHonorsContextOverrideAndIncludesResolvedPaths(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }
	t.Setenv("AKT_CONTEXT", "prod")

	if err := m.CreateNetworkFromTemplate("sandbox", "sandbox"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}
	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")
	runOK(t, createCmd(mgrFn), "staging", "--network", "sandbox")
	if err := aktctx.SetConsoleAPIKey(m.Root(), "staging", "never-print-this-key"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	cmd := currentCmd(mgrFn)
	cmd.Flags().String(flagdefs.FlagContext, "", "test context override")
	cmd.Flags().String(flagdefs.FlagOutput, "pretty", "test output format")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--context", "staging", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("context show: %v", err)
	}

	var got struct {
		Name          string         `json:"name"`
		Network       aktctx.Network `json:"network"`
		StorePath     string         `json:"store_path"`
		ActionLogPath string         `json:"action_log_path"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode context JSON %q: %v", out.String(), err)
	}
	if got.Name != "staging" {
		t.Errorf("name = %q, want staging", got.Name)
	}
	if got.Network.Name != "sandbox" || got.Network.ChainID == "" {
		t.Errorf("network = %+v, want resolved sandbox network", got.Network)
	}
	if got.StorePath != aktctx.StoreDir(m.Root(), "staging") {
		t.Errorf("store_path = %q", got.StorePath)
	}
	if got.ActionLogPath != aktctx.ActionLogPath(m.Root(), "staging") {
		t.Errorf("action_log_path = %q", got.ActionLogPath)
	}
	if strings.Contains(out.String(), "never-print-this-key") {
		t.Fatal("structured context output leaked the Console API key")
	}
	if current := m.CurrentContext(); current != "prod" {
		t.Errorf("show changed current context to %q", current)
	}

	yamlCmd := currentCmd(mgrFn)
	yamlCmd.Flags().String(flagdefs.FlagContext, "", "test context override")
	yamlCmd.Flags().String(flagdefs.FlagOutput, "pretty", "test output format")
	var yamlOut bytes.Buffer
	yamlCmd.SetOut(&yamlOut)
	yamlCmd.SetErr(&bytes.Buffer{})
	yamlCmd.SetArgs([]string{"--context", "staging", "--output", "yaml"})
	if err := yamlCmd.Execute(); err != nil {
		t.Fatalf("context show YAML: %v", err)
	}
	var yamlGot struct {
		Name          string         `yaml:"name"`
		Network       aktctx.Network `yaml:"network"`
		StorePath     string         `yaml:"store_path"`
		ActionLogPath string         `yaml:"action_log_path"`
	}
	if err := yaml.Unmarshal(yamlOut.Bytes(), &yamlGot); err != nil {
		t.Fatalf("decode context YAML %q: %v", yamlOut.String(), err)
	}
	if yamlGot.Name != got.Name || yamlGot.Network.ChainID != got.Network.ChainID || yamlGot.StorePath != got.StorePath || yamlGot.ActionLogPath != got.ActionLogPath {
		t.Errorf("YAML details = %+v, want semantic parity with %+v", yamlGot, got)
	}
}

func TestShowHonorsAKTContext(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	if err := m.CreateNetworkFromTemplate("sandbox", "sandbox"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}
	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")
	runOK(t, createCmd(mgrFn), "staging", "--network", "sandbox")
	t.Setenv("AKT_CONTEXT", "staging")

	cmd := currentCmd(mgrFn)
	cmd.Flags().String(flagdefs.FlagContext, "", "test context override")
	cmd.Flags().String(flagdefs.FlagOutput, "pretty", "test output format")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("context show: %v", err)
	}

	var got struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode context JSON %q: %v", out.String(), err)
	}
	if got.Name != "staging" {
		t.Errorf("name = %q, want staging from AKT_CONTEXT", got.Name)
	}
}

// TestLogRequiresACurrentContext covers logCmd's precondition. The log is
// per-context, so with no current context there is nothing to read.
func TestLogRequiresACurrentContext(t *testing.T) {
	m := newTestManager(t)

	if err := logCmd(func() *aktctx.Manager { return m }).Execute(); err == nil {
		t.Fatal("log without a current context must fail")
	}
}

// TestLogRendersEntriesAndFilters covers the reading path: type filtering, the
// limit, and the empty-result message.
func TestLogRendersEntriesAndFilters(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	// Seed a tx entry alongside the context entries written by create.
	l, err := actionlog.Open(aktctx.ActionLogPath(m.Root(), "prod"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	if err := l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "create-deployment", Status: "success"}); err != nil {
		t.Fatalf("log: %v", err)
	}
	_ = l.Close()

	out := runOutput(t, logCmd(mgrFn))
	if !strings.Contains(out, "create-deployment") || !strings.Contains(out, "create") {
		t.Errorf("log should list both entry kinds, got %q", out)
	}

	// --type narrows to one kind.
	out = runOutput(t, logCmd(mgrFn), "--type", "tx")
	if !strings.Contains(out, "create-deployment") {
		t.Errorf("--type tx should keep the tx entry, got %q", out)
	}
	if strings.Contains(out, "switch") {
		t.Errorf("--type tx must drop context entries, got %q", out)
	}

	// A type with no entries prints the empty message, not a bare header.
	out = runOutput(t, logCmd(mgrFn), "--type", "workflow")
	if !strings.Contains(out, "No action log entries") {
		t.Errorf("an empty filter result should say so, got %q", out)
	}
}

func TestReconcilePendingTransactionsAppendsTerminalRevisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.log")
	logger, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	defer func() { _ = logger.Close() }()

	submitted := time.Date(2026, time.August, 6, 18, 29, 4, 0, time.UTC)
	for _, entry := range []actionlog.Entry{
		{Timestamp: submitted, Type: actionlog.TypeTx, Action: "success", TxHash: "AA", Status: "pending"},
		{Timestamp: submitted.Add(time.Second), Type: actionlog.TypeTx, Action: "failure", TxHash: "BB", Status: "pending"},
		{Timestamp: submitted.Add(2 * time.Second), Type: actionlog.TypeTx, Action: "already-done", TxHash: "CC", Status: "success"},
		{Timestamp: submitted.Add(3 * time.Second), Type: actionlog.TypeTx, Action: "invalid-hash", TxHash: "not-hex", Status: "pending"},
		{Timestamp: submitted.Add(4 * time.Second), Type: actionlog.TypeTx, Action: "not-found", TxHash: "DD", Status: "pending"},
	} {
		if err := logger.Log(entry); err != nil {
			t.Fatalf("seed action log: %v", err)
		}
	}

	entries, err := logger.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read pending entries: %v", err)
	}

	calls := make(map[byte]int)
	lookup := func(_ stdcontext.Context, _ sdkclient.Context, hash []byte) (*sdk.TxResponse, error) {
		calls[hash[0]]++
		switch hash[0] {
		case 0xAA:
			return &sdk.TxResponse{TxHash: "AA", Height: 4694579, GasUsed: 138868}, nil
		case 0xBB:
			return &sdk.TxResponse{TxHash: "BB", Height: 4694580, GasUsed: 90000, Code: 7, RawLog: "unauthorized"}, nil
		default:
			return nil, errors.New("transaction not found")
		}
	}

	if got := reconcilePendingTransactions(stdcontext.Background(), logger, sdkclient.Context{}, entries, lookup); got != 2 {
		t.Fatalf("appended %d terminal revisions, want 2", got)
	}

	got, err := logger.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read reconciled entries: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("read returned %d logical transactions, want 5: %+v", len(got), got)
	}

	byHash := make(map[string]actionlog.Entry, len(got))
	for _, entry := range got {
		byHash[entry.TxHash] = entry
	}
	if entry := byHash["AA"]; entry.Status != "success" || entry.Height != 4694579 || entry.GasUsed != 138868 || !entry.Timestamp.Equal(submitted) {
		t.Errorf("successful transaction was not reconciled: %+v", entry)
	}
	if entry := byHash["BB"]; entry.Status != "failed" || entry.ResultCode != 7 || entry.Error != "unauthorized" {
		t.Errorf("failed transaction was not reconciled: %+v", entry)
	}
	if entry := byHash["DD"]; entry.Status != "pending" {
		t.Errorf("lookup failure changed pending transaction: %+v", entry)
	}
	if calls[0xAA] != 1 || calls[0xBB] != 1 || calls[0xDD] != 1 || len(calls) != 3 {
		t.Errorf("lookup calls = %v, want one call for each valid pending hash only", calls)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read append-only log: %v", err)
	}
	if lines := strings.Count(strings.TrimSpace(string(raw)), "\n") + 1; lines != 7 {
		t.Errorf("raw log has %d lines, want 5 submissions plus 2 revisions", lines)
	}
}

func TestLogHonorsContextOverride(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")
	runOK(t, createCmd(mgrFn), "staging", "--network", "mainnet")

	logger, err := actionlog.Open(aktctx.ActionLogPath(m.Root(), "staging"))
	if err != nil {
		t.Fatalf("open staging log: %v", err)
	}
	if err := logger.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "staging-only-action", Status: "success"}); err != nil {
		t.Fatalf("write staging log: %v", err)
	}
	_ = logger.Close()

	cmd := logCmd(mgrFn)
	cmd.Flags().String(flagdefs.FlagContext, "", "test context override")
	out := runOutput(t, cmd, "--context", "staging")
	if !strings.Contains(out, "staging-only-action") {
		t.Errorf("override log output = %q", out)
	}
	if current := m.CurrentContext(); current != "prod" {
		t.Errorf("log changed current context to %q", current)
	}
}

func TestLogHonorsAKTContext(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")
	runOK(t, createCmd(mgrFn), "staging", "--network", "mainnet")

	logger, err := actionlog.Open(aktctx.ActionLogPath(m.Root(), "staging"))
	if err != nil {
		t.Fatalf("open staging log: %v", err)
	}
	if err := logger.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "staging-env-action", Status: "success"}); err != nil {
		t.Fatalf("write staging log: %v", err)
	}
	_ = logger.Close()
	t.Setenv("AKT_CONTEXT", "staging")

	cmd := logCmd(mgrFn)
	cmd.Flags().String(flagdefs.FlagContext, "", "test context override")
	out := runOutput(t, cmd)
	if !strings.Contains(out, "staging-env-action") {
		t.Errorf("AKT_CONTEXT log output = %q", out)
	}
}

func TestLogRejectsUnknownType(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }
	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	err := runErr(t, logCmd(mgrFn), "--type", "transaction-ish")
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEditForksAndEditsTheSelectedNetwork(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")
	runOK(t, createCmd(mgrFn), "monitoring", "--network", "mainnet")
	originalRPC := append([]string(nil), m.GetNetwork("mainnet").Endpoints.RPC...)

	runOK(t, editCmd(mgrFn), "prod",
		"--fork-network",
		"--rpc", "https://private.example:443",
		"--gas-prices", "0.1uakt",
	)

	prod := m.GetContext("prod")
	if prod.Network.Name != "mainnet-prod" {
		t.Fatalf("prod network = %q, want mainnet-prod", prod.Network.Name)
	}
	if got := m.GetContext("monitoring").Network.Name; got != "mainnet" {
		t.Errorf("monitoring network = %q, want mainnet", got)
	}
	fork := m.GetNetwork("mainnet-prod")
	if fork == nil {
		t.Fatal("forked network was not created")
		return
	}
	if len(fork.Endpoints.RPC) != 1 || fork.Endpoints.RPC[0] != "https://private.example:443" {
		t.Errorf("fork RPC = %v", fork.Endpoints.RPC)
	}
	if fork.GasPrices != "0.1uakt" {
		t.Errorf("fork gas prices = %q", fork.GasPrices)
	}
	if got := m.GetNetwork("mainnet").Endpoints.RPC; strings.Join(got, "\n") != strings.Join(originalRPC, "\n") {
		t.Errorf("parent RPC changed from %v to %v", originalRPC, got)
	}

	if _, err := os.Stat(filepath.Join(m.Root(), "config.yaml")); err != nil {
		t.Fatalf("fork was not persisted: %v", err)
	}
}

func TestEditForkNetworkRequiresANetworkField(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }
	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")

	err := runErr(t, editCmd(mgrFn), "prod", "--fork-network")
	if !strings.Contains(err.Error(), "requires at least one network field") {
		t.Errorf("unexpected error: %v", err)
	}
	if m.GetNetwork("mainnet-prod") != nil {
		t.Fatal("rejected fork left a network behind")
	}
}

func TestCanonicalContextFlagsApplyEveryEditableField(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	if err := m.CreateNetworkFromTemplate("sandbox", "sandbox"); err != nil {
		t.Fatal(err)
	}
	runOK(t, keyringCreateCmd(mgrFn), "alternate", sdkkeyring.BackendTest, "--dir", filepath.Join(t.TempDir(), "keys"))
	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")
	runOK(t, editCmd(mgrFn),
		"prod",
		"--network", "sandbox",
		"--keyring", "alternate",
		"--default-account", "bob",
		"--gas", "333333",
		"--fees", "4uakt",
		"--provider-auth-type", aktctx.ProviderAuthMTLS,
		"--auth-method", aktctx.AuthMethodConsoleAPI,
		"--console-api-url", "https://console.example.test",
		"--rpc", "https://rpc.example.test:443",
		"--api", "https://api.example.test:443",
		"--grpc", "grpc.example.test:443",
		"--gas-prices", "0.03uakt",
		"--gas-adjustment", "1.5",
		"--yes",
	)

	got := m.GetContext("prod")
	if got == nil {
		t.Fatal("edited context is missing")
	}
	if got.Network.Name != "sandbox" || got.Keyring.Name != "alternate" || got.DefaultAccount != "bob" || got.Gas != "333333" || got.Fees != "4uakt" {
		t.Fatalf("edited context = %+v", got)
	}
	if got.ProviderDefaults.AuthType != aktctx.ProviderAuthMTLS || got.AuthMethod != aktctx.AuthMethodConsoleAPI || got.ConsoleAPIURL != "https://console.example.test" {
		t.Fatalf("edited context transport settings = %+v", got)
	}
	network := m.GetNetwork("sandbox")
	if network == nil {
		t.Fatal("edited network is missing")
	}
	if strings.Join(network.Endpoints.RPC, ",") != "https://rpc.example.test:443" || strings.Join(network.Endpoints.API, ",") != "https://api.example.test:443" || strings.Join(network.Endpoints.GRPC, ",") != "grpc.example.test:443" {
		t.Fatalf("edited endpoints = %+v", network.Endpoints)
	}
	if network.GasPrices != "0.03uakt" || network.GasAdjustment != "1.5" {
		t.Fatalf("edited gas defaults = %+v", network)
	}

	for _, cmd := range []*cobra.Command{createCmd(mgrFn), editCmd(mgrFn)} {
		complete, ok := cmd.GetFlagCompletionFunc(flagdefs.FlagNetwork)
		if !ok {
			t.Fatal("network completion is not registered")
		}
		names, directive := complete(cmd, nil, "")
		if directive != cobra.ShellCompDirectiveNoFileComp || len(names) < 2 {
			t.Fatalf("network completions = %v, directive = %v", names, directive)
		}
	}
}

func TestNetworkIsSharedAfterContextSwitch(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	if err := m.CreateNetworkFromTemplate("sandbox", "sandbox"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}
	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")
	runOK(t, createCmd(mgrFn), "staging", "--network", "sandbox")

	if !networkSharedAfterContextUpdate(m, "sandbox", "prod") {
		t.Fatal("sandbox must be treated as shared when prod switches onto staging's network")
	}
}

// TestLogSinceAcceptsDurationsAndDates covers all three --since forms plus the
// rejection path. A silently-ignored bad value would show the user the wrong
// time window.
func TestLogSinceAcceptsDurationsAndDates(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	for _, since := range []string{
		"1h",
		time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		"2000-01-01",
	} {
		out := runOutput(t, logCmd(mgrFn), "--since", since)
		if !strings.Contains(out, "create") {
			t.Errorf("--since %q should keep recent entries, got %q", since, out)
		}
	}

	// A future cutoff filters everything out.
	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	out := runOutput(t, logCmd(mgrFn), "--since", future)
	if !strings.Contains(out, "No action log entries") {
		t.Errorf("a future --since should filter everything, got %q", out)
	}

	err := runErr(t, logCmd(mgrFn), "--since", "last tuesday")
	if !strings.Contains(err.Error(), "invalid --since") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestTruncate covers the log renderer's error-column shortener, including the
// boundary where truncation kicks in. An off-by-one here slices past the end
// of short strings.
func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"exactly-10", 10, "exactly-10"},
		{"eleven-char", 10, "eleven-..."},
		{"", 5, ""},
	}

	for _, c := range cases {
		if got := truncate(c.in, c.max); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
		if len(truncate(c.in, c.max)) > max(len(c.in), c.max) {
			t.Errorf("truncate(%q, %d) grew the string", c.in, c.max)
		}
	}
}

// TestDeleteCancelledByPrompt covers the confirmation branch: a `no` answer
// must leave the context (and its store and action log) intact.
func TestDeleteCancelledByPrompt(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")

	cmd := deleteCmd(mgrFn)
	cmd.SetIn(strings.NewReader("n\n"))
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"prod"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("decline delete: %v", err)
	}
	if !strings.Contains(stderr.String(), "Cancelled") {
		t.Errorf("a declined delete should say so on stderr, got %q", stderr.String())
	}

	if m.GetContext("prod") == nil {
		t.Error("a declined delete must not remove the context")
	}

	// A "y" answer goes through.
	cmd = deleteCmd(mgrFn)
	cmd.SetIn(strings.NewReader("y\n"))
	runOK(t, cmd, "prod")

	if m.GetContext("prod") != nil {
		t.Error("a confirmed delete must remove the context")
	}
}

// TestCompleteContextNames covers the shell-completion helper used by use,
// edit, delete, and rename — including the nil-manager guard that keeps
// completion from panicking before a config exists.
func TestCompleteContextNames(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }

	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")
	runOK(t, createCmd(mgrFn), "staging", "--network", "mainnet")

	names, directive := completeContextNames(mgrFn)(nil, nil, "")
	if len(names) != 2 {
		t.Errorf("completions = %v, want both contexts", names)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}

	names, directive = completeContextNames(func() *aktctx.Manager { return nil })(nil, nil, "")
	if names != nil {
		t.Errorf("a nil manager must yield no completions, got %v", names)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
}

// TestCommandsTreeIsComplete guards the command surface: `akt context` is the
// documented entry point for contexts, networks, and keys, and a dropped
// subcommand is invisible until a user reaches for it.
func TestCommandsTreeIsComplete(t *testing.T) {
	m := newTestManager(t)

	root := Commands(
		func() *aktctx.Manager { return m },
		func() (sdkkeyring.Keyring, error) { return nil, nil },
	)

	have := map[string]bool{}
	for _, sub := range root.Commands() {
		have[sub.Name()] = true
	}

	for _, want := range []string{"create", "use", "list", "show", "edit", "delete", "rename", "log", "network", "keys"} {
		if !have[want] {
			t.Errorf("subcommand %q missing from the context group", want)
		}
	}
}
