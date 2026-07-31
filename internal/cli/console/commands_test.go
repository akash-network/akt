package console

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
// pointing the client at srvURL when non-empty, and returns combined output.
func execConsole(t *testing.T, m *aktctx.Manager, srvURL string, args ...string) (string, error) {
	t.Helper()

	// Neutralize any ambient key so tests only see what they configure.
	t.Setenv(aktctx.EnvConsoleAPIKey, "")

	cmd := Commands(func() *aktctx.Manager { return m })
	cmd.PersistentFlags().VarP(output.NewFormatFlag("pretty"), "output", "o", "Output format: pretty, json, yaml")
	cmd.PersistentFlags().String("context", "", "Active context name")

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if srvURL != "" {
		args = append(args, "--console-api-url", srvURL)
	}
	cmd.SetArgs(args)

	err := cmd.Execute()

	return buf.String(), err
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

func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
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
	if ack["context"] != "staging" || ack["authenticated"] != true || ack["username"] != "max" {
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
	if ack["context"] != "staging" || ack["authenticated"] != false {
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

func TestDeploymentCloseAlreadyClosedExitsClean(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	// The Console API answers 404 for an already-closed deployment.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/deployments/42" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "deployment", "close", "42")
	if err != nil {
		t.Fatalf("close of an already-closed deployment must succeed, got %v", err)
	}

	if !strings.Contains(out, "already closed") {
		t.Errorf("output should report the no-op, got %q", out)
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
	out, err := execConsole(t, m, srv.URL, "template", "sdl", "tpl-1")
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
		if len(body.Data.Leases.Scope) != len(defaultJWTScope) {
			t.Errorf("default scope = %v, want %v", body.Data.Leases.Scope, defaultJWTScope)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		writeJSON(t, w, `{"data":{}}`)
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
