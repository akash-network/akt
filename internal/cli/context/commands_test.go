package context

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/actionlog"
	aktctx "pkg.akt.dev/akt/internal/context"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written. The context list/show/log commands print through
// output.PrintData and fmt.Print, both of which target the process stdout
// rather than cmd.OutOrStdout().
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	prev := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	os.Stdout = prev
	_ = w.Close()
	out := <-done
	_ = r.Close()

	return out
}

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
	}
	if c.AuthMethod != aktctx.AuthMethodConsoleAPI {
		t.Errorf("auth method = %q, want console-api", c.AuthMethod)
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

	out := captureStdout(t, func() { runOK(t, listCmd(mgrFn)) })

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

	out := captureStdout(t, func() { runOK(t, listCmd(func() *aktctx.Manager { return m })) })

	if !strings.Contains(out, "akt context create") {
		t.Errorf("empty list should point at the create command, got %q", out)
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

	out := captureStdout(t, func() { runOK(t, currentCmd(mgrFn)) })
	if !strings.Contains(out, "prod") || !strings.Contains(out, "mainnet") {
		t.Errorf("show output should name the context and its network, got %q", out)
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

	out := captureStdout(t, func() { runOK(t, logCmd(mgrFn)) })
	if !strings.Contains(out, "create-deployment") || !strings.Contains(out, "create") {
		t.Errorf("log should list both entry kinds, got %q", out)
	}

	// --type narrows to one kind.
	out = captureStdout(t, func() { runOK(t, logCmd(mgrFn), "--type", "tx") })
	if !strings.Contains(out, "create-deployment") {
		t.Errorf("--type tx should keep the tx entry, got %q", out)
	}
	if strings.Contains(out, "switch") {
		t.Errorf("--type tx must drop context entries, got %q", out)
	}

	// A type with no entries prints the empty message, not a bare header.
	out = captureStdout(t, func() { runOK(t, logCmd(mgrFn), "--type", "workflow") })
	if !strings.Contains(out, "No action log entries") {
		t.Errorf("an empty filter result should say so, got %q", out)
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
		out := captureStdout(t, func() { runOK(t, logCmd(mgrFn), "--since", since) })
		if !strings.Contains(out, "create") {
			t.Errorf("--since %q should keep recent entries, got %q", since, out)
		}
	}

	// A future cutoff filters everything out.
	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	out := captureStdout(t, func() { runOK(t, logCmd(mgrFn), "--since", future) })
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

	withStdin(t, "n\n", func() {
		out := captureStdout(t, func() { runOK(t, deleteCmd(mgrFn), "prod") })
		if !strings.Contains(out, "Cancelled") {
			t.Errorf("a declined delete should say so, got %q", out)
		}
	})

	if m.GetContext("prod") == nil {
		t.Error("a declined delete must not remove the context")
	}

	// A "y" answer goes through.
	withStdin(t, "y\n", func() {
		captureStdout(t, func() { runOK(t, deleteCmd(mgrFn), "prod") })
	})

	if m.GetContext("prod") != nil {
		t.Error("a confirmed delete must remove the context")
	}
}

// withStdin replaces os.Stdin with a pipe carrying input for the duration of
// fn (deleteCmd reads the confirmation with fmt.Scanln).
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	prev := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = prev
		_ = r.Close()
	}()

	fn()
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
