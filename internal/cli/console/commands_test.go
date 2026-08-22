package console

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
	consoleapi "pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
)

// newTestManager builds a Manager on a temp home with a "mainnet" network
// (from template), a "prod" context, and "prod" as the current context.
// DefaultConfig already seeds the "default" keyring.
func newTestManager(t *testing.T) *aktctx.Manager {
	t.Helper()

	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	if err := m.CreateContext(aktctx.Context{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	if err := m.UseContext("prod"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	return m
}

// execConsole runs `akt console <args...>` against the given manager,
// pointing the client at srvURL when non-empty, and returns stdout.
func execConsole(t *testing.T, m *aktctx.Manager, srvURL string, args ...string) (string, error) {
	return execConsoleContext(context.Background(), t, m, srvURL, args...)
}

func execConsoleContext(ctx context.Context, t *testing.T, m *aktctx.Manager, srvURL string, args ...string) (string, error) {
	stdout, stderr, err := execConsoleContextStreams(ctx, t, m, srvURL, args...)
	if err != nil {
		return stdout + stderr, err
	}

	return stdout, err
}

func execConsoleContextStreams(ctx context.Context, t *testing.T, m *aktctx.Manager, srvURL string, args ...string) (string, string, error) {
	t.Helper()

	// Neutralize any ambient key so tests only see what they configure.
	t.Setenv(aktctx.EnvConsoleAPIKey, "")

	cmd := Commands(func() *aktctx.Manager { return m })
	cmd.PersistentFlags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "Output format: pretty, json, yaml")
	cmd.PersistentFlags().String(flagdefs.FlagContext, "", "Active context name")
	cmd.SetContext(ctx)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	prefix := make([]string, 0, 4)
	if srvURL != "" {
		prefix = append(prefix, "--console-api-url", srvURL)
	}
	hasOutput := false
	for _, arg := range args {
		if arg == "--output" || arg == "-o" || strings.HasPrefix(arg, "--output=") {
			hasOutput = true
			break
		}
	}
	if !hasOutput {
		prefix = append(prefix, "--output", "json")
	}
	args = append(prefix, args...)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return stdout.String(), stderr.String(), err
}

func decodeStructuredOutput(t *testing.T, format, raw string) any {
	t.Helper()

	var value any
	switch format {
	case "json":
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatalf("decode JSON output %q: %v", raw, err)
		}
	case "yaml":
		if err := yaml.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatalf("decode YAML output %q: %v", raw, err)
		}
		// Normalize YAML's native scalar types through JSON so callers can
		// compare the semantic tree with the JSON output.
		normalized, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("normalize YAML output: %v", err)
		}
		if err := json.Unmarshal(normalized, &value); err != nil {
			t.Fatalf("decode normalized YAML output: %v", err)
		}
	default:
		t.Fatalf("unsupported structured output format %q", format)
	}

	return value
}

func decodeStructuredMap(t *testing.T, format, raw string) map[string]any {
	t.Helper()

	value := decodeStructuredOutput(t, format, raw)
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s output = %#v, want an object", format, value)
	}

	return object
}

func TestEveryConsoleActionDeclaresNextStep(t *testing.T) {
	const annotation = "akt.console.next"
	root := Commands(func() *aktctx.Manager { return nil })

	var missing []string
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		children := cmd.Commands()
		if len(children) == 0 {
			if (cmd.RunE != nil || cmd.Run != nil) && strings.TrimSpace(cmd.Annotations[annotation]) == "" {
				missing = append(missing, cmd.CommandPath())
			}
			return
		}
		for _, child := range children {
			walk(child)
		}
	}
	walk(root)

	if len(missing) != 0 {
		t.Fatalf("Console action leaves without next-step guidance: %s", strings.Join(missing, ", "))
	}
}

func TestConsoleNextSuggestionHasSafeFallback(t *testing.T) {
	if got := consoleNextSuggestion(&cobra.Command{Use: "future-action"}); got != "akt console --help" {
		t.Fatalf("fallback next step = %q", got)
	}
}

func TestConsoleNextStepUsesInformationalChannel(t *testing.T) {
	m := newTestManager(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/user/me" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, `{"data":{"username":"alice"}}`)
	}))
	defer srv.Close()

	run := func(quiet bool) (string, string, error) {
		cmd := Commands(func() *aktctx.Manager { return m })
		cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
			return PrintNextStep(executed)
		}
		cmd.PersistentFlags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "Output format: pretty, json, yaml")
		cmd.PersistentFlags().Bool(flagdefs.FlagQuiet, false, "Suppress informational output")
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		args := []string{"--console-api-url", srv.URL, "--output", "json"}
		if quiet {
			args = append(args, "--quiet")
		}
		args = append(args, "login", "sekrit")
		cmd.SetArgs(args)
		err := cmd.Execute()
		return stdout.String(), stderr.String(), err
	}

	stdout, stderr, err := run(false)
	if err != nil {
		t.Fatalf("console login: %v", err)
	}
	if !json.Valid([]byte(stdout)) {
		t.Fatalf("structured stdout was contaminated: %q", stdout)
	}
	if !strings.Contains(stderr, "Next:\n") || !strings.Contains(stderr, "akt console whoami") {
		t.Fatalf("next-step stderr = %q", stderr)
	}

	_, stderr, err = run(true)
	if err != nil {
		t.Fatalf("quiet console login: %v", err)
	}
	if stderr != "" {
		t.Fatalf("quiet console login stderr = %q, want empty", stderr)
	}
}

func TestConsoleTreeLeavesPostRunOwnershipToAncestor(t *testing.T) {
	previous := cobra.EnableTraverseRunHooks
	cobra.EnableTraverseRunHooks = true
	t.Cleanup(func() { cobra.EnableTraverseRunHooks = previous })

	m := newTestManager(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/user/me" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, `{"data":{"username":"alice"}}`)
	}))
	defer srv.Close()

	ancestorRuns := 0
	root := &cobra.Command{
		Use: "akt",
		PersistentPostRunE: func(executed *cobra.Command, _ []string) error {
			ancestorRuns++
			return PrintNextStep(executed)
		},
	}
	root.PersistentFlags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "Output format")
	root.PersistentFlags().Bool(flagdefs.FlagQuiet, false, "Suppress informational output")
	root.AddCommand(Commands(func() *aktctx.Manager { return m }))
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"console", "--console-api-url", srv.URL, "login", "sekrit"})

	if err := root.Execute(); err != nil {
		t.Fatalf("console login: %v", err)
	}
	if ancestorRuns != 1 {
		t.Fatalf("ancestor persistent post-run calls = %d, want 1", ancestorRuns)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

type consoleOutputErrorWriter struct {
	err error
}

func (w consoleOutputErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type consoleOutputShortWriter struct{}

func (consoleOutputShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	return len(p) - 1, nil
}

type consoleNthWriteError struct {
	calls  int
	failAt int
	err    error
	wrote  bytes.Buffer
}

func (w *consoleNthWriteError) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		return 0, w.err
	}

	return w.wrote.Write(p)
}

func TestPromptForKeyTerminalBoundary(t *testing.T) {
	t.Run("prompt write fails before secret read", func(t *testing.T) {
		writeErr := errors.New("prompt destination closed")
		cmd := &cobra.Command{}
		cmd.SetErr(consoleOutputErrorWriter{err: writeErr})
		readCalls := 0

		key, err := promptForKeyWithTerminal(
			cmd,
			77,
			func(fd int) bool { return fd == 77 },
			func(int) ([]byte, error) {
				readCalls++
				return []byte("must-not-be-read"), nil
			},
		)
		if !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "write API key prompt") {
			t.Fatalf("prompt error = %v, want wrapped destination failure", err)
		}
		if key != "" || readCalls != 0 {
			t.Fatalf("key/read calls = %q/%d, want no secret read after prompt failure", key, readCalls)
		}
	})

	t.Run("terminator failure does not return secret", func(t *testing.T) {
		writeErr := errors.New("prompt terminator failed")
		writer := &consoleNthWriteError{failAt: 2, err: writeErr}
		cmd := &cobra.Command{}
		cmd.SetErr(writer)
		readFD := 0

		key, err := promptForKeyWithTerminal(
			cmd,
			91,
			func(int) bool { return true },
			func(fd int) ([]byte, error) {
				readFD = fd
				return []byte("  one-time-secret  "), nil
			},
		)
		if !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "prompt terminator") {
			t.Fatalf("terminator error = %v, want wrapped destination failure", err)
		}
		if key != "" || readFD != 91 {
			t.Fatalf("key/read fd = %q/%d, want withheld secret read from fd 91", key, readFD)
		}
		if got := writer.wrote.String(); got != "Console API key: " {
			t.Fatalf("accepted prompt bytes = %q", got)
		}
	})

	t.Run("terminal secret is trimmed after prompt is completed", func(t *testing.T) {
		cmd := &cobra.Command{}
		var prompt bytes.Buffer
		cmd.SetErr(&prompt)

		key, err := promptForKeyWithTerminal(
			cmd,
			12,
			func(int) bool { return true },
			func(fd int) ([]byte, error) {
				if fd != 12 {
					t.Fatalf("read fd = %d, want 12", fd)
				}
				return []byte("  one-time-secret\r\n"), nil
			},
		)
		if err != nil || key != "one-time-secret" {
			t.Fatalf("prompt result = %q, %v", key, err)
		}
		if got := prompt.String(); got != "Console API key: \n" {
			t.Fatalf("prompt output = %q", got)
		}
	})

	t.Run("read failure still restores the prompt line", func(t *testing.T) {
		readErr := errors.New("terminal read failed")
		cmd := &cobra.Command{}
		var prompt bytes.Buffer
		cmd.SetErr(&prompt)

		key, err := promptForKeyWithTerminal(
			cmd,
			12,
			func(int) bool { return true },
			func(int) ([]byte, error) { return nil, readErr },
		)
		if key != "" || !errors.Is(err, readErr) || !strings.Contains(err.Error(), "read API key") {
			t.Fatalf("prompt result = %q, %v, want wrapped terminal read failure", key, err)
		}
		if got := prompt.String(); got != "Console API key: \n" {
			t.Fatalf("prompt output after read failure = %q", got)
		}
	})
}

func TestPromptForKeyUsesCommandInputOutsideTerminal(t *testing.T) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("process stdin is a terminal; the injectable terminal path is covered separately")
	}
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("  piped-secret  \n"))

	key, err := promptForKey(cmd)
	if err != nil || key != "piped-secret" {
		t.Fatalf("piped prompt result = %q, %v", key, err)
	}
}

func executeConsoleWithOutput(
	t *testing.T,
	m *aktctx.Manager,
	srvURL string,
	out io.Writer,
	args ...string,
) error {
	t.Helper()

	t.Setenv(aktctx.EnvConsoleAPIKey, "")
	cmd := Commands(func() *aktctx.Manager { return m })
	cmd.PersistentFlags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "Output format: pretty, json, yaml")
	cmd.PersistentFlags().String(flagdefs.FlagContext, "", "Active context name")
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	if srvURL != "" {
		args = append([]string{"--console-api-url", srvURL}, args...)
	}
	cmd.SetArgs(args)

	return cmd.Execute()
}

const userBody = `{"data":{"id":"uuid-1","userId":"user-1","username":"max","email":"max@example.com","emailVerified":true}}`

func TestLoginValidatesAndStoresKey(t *testing.T) {
	m := newTestManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/me" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "sekrit" {
			t.Errorf("x-api-key = %q, want sekrit", got)
		}
		writeJSON(t, w, userBody)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "login", "sekrit")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if !strings.Contains(out, "max") {
		t.Errorf("login output should print the username, got %q", out)
	}
	if strings.Contains(out, "sekrit") {
		t.Errorf("login output must never contain the API key, got %q", out)
	}

	data, err := os.ReadFile(aktctx.ConsoleAPIKeyPath(m.Root(), "prod"))
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	if string(data) != "sekrit\n" {
		t.Errorf("credential file content = %q, want %q", string(data), "sekrit\n")
	}
}

func TestLoginRejectsInvalidKey(t *testing.T) {
	m := newTestManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := execConsole(t, m, srv.URL, "login", "bad-key")
	if err == nil {
		t.Fatal("login with a rejected key should fail")
	}

	if _, statErr := os.Stat(aktctx.ConsoleAPIKeyPath(m.Root(), "prod")); !os.IsNotExist(statErr) {
		t.Error("a rejected key must not be stored")
	}
}

func TestLoginAndLogoutHonorContextOverride(t *testing.T) {
	m := newTestManager(t)
	if err := m.CreateContext(aktctx.Context{
		Name:    "staging",
		Network: aktctx.Network{Name: "mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "prod-key"); err != nil {
		t.Fatalf("set prod key: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "staging-key" {
			t.Errorf("x-api-key = %q, want staging-key", got)
		}
		writeJSON(t, w, userBody)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL,
		"--context", "staging", "login", "staging-key", "--output", "json")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	ack := decodeStructuredMap(t, "json", out)
	authenticated, ok := ack["authenticated"].(bool)
	if ack["context"] != "staging" || !ok || !authenticated || ack["username"] != "max" {
		t.Errorf("login acknowledgement = %#v", ack)
	}
	if strings.Contains(out, "staging-key") {
		t.Fatal("login acknowledgement leaked the API key")
	}
	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "staging"); got != "staging-key" {
		t.Errorf("staging key = %q", got)
	}
	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "prod"); got != "prod-key" {
		t.Errorf("prod key changed to %q", got)
	}

	out, err = execConsole(t, m, "",
		"--context", "staging", "logout", "--output", "yaml")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	ack = decodeStructuredMap(t, "yaml", out)
	authenticated, ok = ack["authenticated"].(bool)
	if ack["context"] != "staging" || !ok || authenticated {
		t.Errorf("logout acknowledgement = %#v", ack)
	}
	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "staging"); got != "" {
		t.Errorf("staging key after logout = %q", got)
	}
	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "prod"); got != "prod-key" {
		t.Errorf("prod key changed to %q", got)
	}
}

func TestLoginAndLogoutHonorAKTContext(t *testing.T) {
	m := newTestManager(t)
	if err := m.CreateContext(aktctx.Context{
		Name:    "staging",
		Network: aktctx.Network{Name: "mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "prod-key"); err != nil {
		t.Fatalf("set prod key: %v", err)
	}
	t.Setenv("AKT_CONTEXT", "staging")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "staging-key" {
			t.Errorf("x-api-key = %q, want staging-key", got)
		}
		writeJSON(t, w, userBody)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "login", "staging-key", "--output", "json")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	ack := decodeStructuredMap(t, "json", out)
	if ack["context"] != "staging" {
		t.Errorf("login acknowledgement = %#v", ack)
	}
	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "staging"); got != "staging-key" {
		t.Errorf("staging key = %q", got)
	}
	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "prod"); got != "prod-key" {
		t.Errorf("prod key changed to %q", got)
	}

	out, err = execConsole(t, m, "", "logout", "--output", "yaml")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	ack = decodeStructuredMap(t, "yaml", out)
	if ack["context"] != "staging" {
		t.Errorf("logout acknowledgement = %#v", ack)
	}
	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "staging"); got != "" {
		t.Errorf("staging key after logout = %q", got)
	}
	if got, _ := aktctx.StoredConsoleAPIKey(m.Root(), "prod"); got != "prod-key" {
		t.Errorf("prod key changed to %q", got)
	}
}

func TestWhoamiPrintsUsername(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, userBody)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}

	if !strings.Contains(out, `"username": "max"`) || !strings.Contains(out, "max@example.com") {
		t.Errorf("whoami output missing user fields, got %q", out)
	}
}

func TestDeploymentListSendsAPIKey(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var gotPath, gotQuery, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotKey = r.Header.Get("x-api-key")
		writeJSON(t, w, `{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":20,"hasMore":false}}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "deployment", "list")
	if err != nil {
		t.Fatalf("deployment list: %v", err)
	}

	if gotPath != "/v1/deployments" {
		t.Errorf("path = %q, want /v1/deployments", gotPath)
	}
	if gotQuery != "skip=0&limit=20" {
		t.Errorf("query = %q, want skip=0&limit=20", gotQuery)
	}
	if gotKey != "sekrit" {
		t.Errorf("x-api-key = %q, want sekrit", gotKey)
	}
	if !strings.Contains(out, `"deployments"`) {
		t.Errorf("output should include the deployment list, got %q", out)
	}
}

func TestDeploymentGetYAMLPreservesJSONSemantics(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/deployments/42" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		writeJSON(t, w, `{"data":{"deployment":{"id":{"owner":"akash1owner","dseq":"42"},"state":"active","created_at":"27957328"},"leases":[{"id":{"owner":"akash1owner","dseq":"42","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active","status":{"services":{"web":{"name":"web","available":1,"total":1}},"forwarded_ports":{"web":[{"host":"example.test","port":80}]},"ips":null}}],"escrow_account":{"balance":{"denom":"uakt","amount":"900719925474099312345"}}}}`)
	}))
	defer srv.Close()

	jsonOut, err := execConsole(t, m, srv.URL, "deployment", "get", "42", "-o", "json")
	if err != nil {
		t.Fatalf("deployment get JSON: %v", err)
	}
	yamlOut, err := execConsole(t, m, srv.URL, "deployment", "get", "42", "-o", "yaml")
	if err != nil {
		t.Fatalf("deployment get YAML: %v", err)
	}

	jsonValue := decodeStructuredOutput(t, "json", jsonOut)
	yamlValue := decodeStructuredOutput(t, "yaml", yamlOut)
	if !reflect.DeepEqual(yamlValue, jsonValue) {
		t.Fatalf("YAML changed the JSON data model\nJSON: %#v\nYAML: %#v\nraw YAML:\n%s", jsonValue, yamlValue, yamlOut)
	}

	root, ok := yamlValue.(map[string]any)
	if !ok {
		t.Fatalf("YAML output = %#v, want an object", yamlValue)
	}
	deployment := root["deployment"].(map[string]any)
	if got := deployment["created_at"]; got != "27957328" {
		t.Errorf("created_at = %#v, want the JSON string %q", got, "27957328")
	}
	if _, exists := deployment["createdat"]; exists {
		t.Error("YAML output must use the JSON field name created_at, not createdat")
	}
	services := root["leases"].([]any)[0].(map[string]any)["status"].(map[string]any)["services"]
	if _, ok := services.(map[string]any); !ok {
		t.Fatalf("services = %#v, want the JSON object", services)
	}
}

func TestDeploymentCloseAlreadyClosedExitsNonZero(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	// The Console API answers 404 for an already-closed deployment.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/deployments/42" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	stdout, _, err := execConsoleContextStreams(context.Background(), t, m, srv.URL, "--output", "pretty", "deployment", "close", "42")
	if !errors.Is(err, consoleapi.ErrAlreadyClosed) {
		t.Fatalf("close of an already-closed deployment error = %v, want ErrAlreadyClosed", err)
	}
	if strings.Contains(stdout, "Deployment 42 closed") || strings.Contains(stdout, `"state":"closed"`) {
		t.Errorf("already-closed close emitted a success acknowledgement %q", stdout)
	}
}

func TestMissingKeyErrorMessage(t *testing.T) {
	m := newTestManager(t)

	_, err := execConsole(t, m, "", "deployment", "list")
	if err == nil {
		t.Fatal("a key-requiring command without a key must fail")
	}

	msg := err.Error()
	for _, want := range []string{
		`no Console API key configured for context "prod"`,
		"akt console login",
		aktctx.EnvConsoleAPIKey,
		"akt context edit prod --console-api-key",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should mention %q", msg, want)
		}
	}
}

func TestTemplateSDLPrintsRawSDL(t *testing.T) {
	m := newTestManager(t)

	const rawSDL = "version: \"2.0\"\nservices:\n  web:\n    image: nginx:1.27\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/templates/tpl-1" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ := json.Marshal(map[string]any{
			"data": map[string]any{"id": "tpl-1", "name": "web", "deploy": rawSDL},
		})
		writeJSON(t, w, string(body))
	}))
	defer srv.Close()

	// Public endpoint: no key configured anywhere.
	out, err := execConsole(t, m, srv.URL, "--output", "pretty", "template", "sdl", "tpl-1")
	if err != nil {
		t.Fatalf("template sdl: %v", err)
	}

	if out != rawSDL {
		t.Errorf("template sdl output = %q, want the raw SDL %q", out, rawSDL)
	}

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			out, err := execConsole(t, m, srv.URL, "template", "sdl", "tpl-1", "-o", format)
			if err != nil {
				t.Fatalf("template sdl -o %s: %v", format, err)
			}

			value := decodeStructuredMap(t, format, out)
			if got := value["sdl"]; got != rawSDL {
				t.Errorf("structured SDL = %#v, want exact source %q", got, rawSDL)
			}
		})
	}
}

func TestPlainConsoleCommandsPropagateOutputFailures(t *testing.T) {
	m := newAuthedManager(t)
	sentinel := errors.New("console destination failed")
	requests := make(map[string]int)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.Method+" "+r.URL.Path]++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/api-keys":
			writeJSON(t, w, `{"data":[{"id":"11111111-1111-1111-1111-111111111111","name":"ci"}]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/api-keys/11111111-1111-1111-1111-111111111111":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/777":
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"777"},"state":"active"}}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/deployments/777":
			writeJSON(t, w, `{"data":{"success":true}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/templates/tpl-1":
			writeJSON(t, w, `{"data":{"id":"tpl-1","name":"web","deploy":"services: {}\\n"}}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/bids"):
			writeJSON(t, w, `{"data":[]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/12345":
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/bid-screening":
			writeJSON(t, w, `{"providers":[]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.RequestURI())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	sdlPath := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(sdlPath, []byte(screenSDL), 0o600); err != nil {
		t.Fatalf("write SDL: %v", err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{name: "API key delete result", args: []string{"apikey", "delete", "11111111-1111-1111-1111-111111111111"}},
		{name: "raw template SDL", args: []string{"template", "sdl", "tpl-1"}},
		{name: "empty bid result", args: []string{"bid", "list", "12345"}},
		{name: "empty bid screening result", args: []string{"screen", sdlPath}},
		{name: "deployment close result", args: []string{"deployment", "close", "777"}},
	}

	failures := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "hard error", writer: consoleOutputErrorWriter{err: sentinel}, want: sentinel},
		{name: "short write", writer: consoleOutputShortWriter{}, want: io.ErrShortWrite},
	}

	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					err := executeConsoleWithOutput(t, m, srv.URL, failure.writer, tc.args...)
					if !errors.Is(err, failure.want) {
						t.Fatalf("error = %v, want destination failure %v", err, failure.want)
					}
				})
			}
		})
	}

	for _, request := range []string{
		http.MethodDelete + " /v1/api-keys/11111111-1111-1111-1111-111111111111",
		http.MethodDelete + " /v1/deployments/777",
	} {
		if requests[request] != len(failures) {
			t.Fatalf("%s requests = %d, want %d completed mutations", request, requests[request], len(failures))
		}
	}
}

func TestJWTCreatePrintsToken(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/create-jwt-token" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		var body struct {
			Data struct {
				TTL    int `json:"ttl"`
				Leases struct {
					Access string   `json:"access"`
					Scope  []string `json:"scope"`
				} `json:"leases"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Data.TTL != 300 {
			t.Errorf("default ttl = %d, want 300", body.Data.TTL)
		}
		if len(body.Data.Leases.Scope) != len(defaultJWTScope()) {
			t.Errorf("default scope = %v, want %v", body.Data.Leases.Scope, defaultJWTScope())
		}

		writeJSON(t, w, `{"data":{"token":"tok-abc123"}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "jwt", "create")
	if err != nil {
		t.Fatalf("jwt create: %v", err)
	}

	if !strings.Contains(out, "tok-abc123") {
		t.Errorf("jwt create output should contain the token, got %q", out)
	}
}

func TestUsagePositionalDates(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var usageQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user/me":
			writeJSON(t, w, `{"data":{"id":"u1","username":"joe"}}`)
		case "/v1/wallets":
			writeJSON(t, w, `{"data":[{"address":"akash1wallet","creditAmount":1000000}]}`)
		case "/v1/usage/history":
			usageQuery = r.URL.Query()
			writeJSON(t, w, `[]`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "usage", "2026-01-01", "2026-01-31"); err != nil {
		t.Fatalf("usage with positional dates: %v", err)
	}

	if usageQuery.Get("startDate") != "2026-01-01" || usageQuery.Get("endDate") != "2026-01-31" {
		t.Errorf("positional dates not forwarded: %v", usageQuery)
	}

	// No dates at all: params must be omitted so the API applies defaults.
	usageQuery = nil
	if _, err := execConsole(t, m, srv.URL, "usage"); err != nil {
		t.Fatalf("usage without dates: %v", err)
	}
	if _, present := usageQuery["startDate"]; present {
		t.Errorf("empty startDate must be omitted, got %v", usageQuery)
	}
}

func TestDepositPositionalAmount(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var body string
	var deposited bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/12345" {
			amount := "1000000"
			if deposited {
				amount = "11000000"
			}
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"`+amount+`"}],"transferred":[]}}}}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		deposited = true
		writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"11000000"}],"transferred":[]}}}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "deployment", "deposit", "12345", "10"); err != nil {
		t.Fatalf("deposit with positional amount: %v", err)
	}
	if !strings.Contains(body, `"deposit":10`) || !strings.Contains(body, `"dseq":"12345"`) {
		t.Errorf("positional amount not sent: %s", body)
	}

	// Invalid positional amount must fail before any request.
	if _, err := execConsole(t, m, srv.URL, "deployment", "deposit", "12345", "ten"); err == nil {
		t.Error("invalid positional amount must be rejected")
	}
}
