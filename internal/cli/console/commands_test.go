package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
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
