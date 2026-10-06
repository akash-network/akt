package workflow

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jose "github.com/go-jose/go-jose/v4"
	aktctx "pkg.akt.dev/akt/internal/context"
)

func newWorkflowSecretsServer(t *testing.T, next http.Handler) *httptest.Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/sdl-secrets-context" {
			if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"sub": "workflow-user", "kid": "workflow-key",
				"jwk":            jose.JSONWebKey{Key: &key.PublicKey, Use: "enc", Algorithm: "RSA-OAEP-256"},
				"requiredClaims": []string{"kid", "sub", "exp"},
			}}); err != nil {
				t.Error(err)
			}
			return
		}
		next.ServeHTTP(w, r)
	}))
}

type unreadSecretInput struct{ read bool }

func (r *unreadSecretInput) Read([]byte) (int, error) {
	r.read = true
	return 0, errors.New("secret stdin must not be consumed")
}

func TestPatchDryRunDoesNotReadSecretValues(t *testing.T) {
	home := t.TempDir()
	patchPath := filepath.Join(t.TempDir(), "changes.yaml")
	if err := os.WriteFile(patchPath, []byte("services:\n  web:\n    image: nginx:1.27-alpine\n"), 0600); err != nil {
		t.Fatal(err)
	}
	homeFn, ctxFn := staticFns(home)
	cmd := findCommand(Commands(homeFn, ctxFn), "update")
	input := &unreadSecretInput{}
	cmd.SetIn(input)
	out, err := executeCommand(t, cmd, patchPath, "123", "--patch", "--secrets-file", "-", "--dry-run")
	if err != nil || input.read {
		t.Fatalf("dry run consumed secrets or failed: %v, %s", err, out)
	}
	if strings.Contains(out, "nginx:1.27-alpine") {
		t.Fatal("patch contents leaked into plan")
	}
}

func TestChainDryRunRejectsConsoleReferences(t *testing.T) {
	home := t.TempDir()
	m := newTestManager(t, home, "chain", aktctx.AuthMethodKeyring)
	path := writeValidWorkflowSDL(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Every fixture has one web service; insert its reference before image.
	data = []byte(strings.Replace(string(data), "    image:", "    env:\n      - TOKEN=ac-secret://TOKEN\n    image:", 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := findCommand(CommandsWithManager(func() string { return home }, func() string { return "chain" }, func() *aktctx.Manager { return m }), "deploy")
	_, err = executeCommand(t, cmd, path, "--deposit", "1000000uact", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "secret references require a Console context") {
		t.Fatalf("chain reference error = %v", err)
	}
}

func TestChainPatchDryRunValidatesBaseAndChanges(t *testing.T) {
	home := t.TempDir()
	m := newTestManager(t, home, "chain", aktctx.AuthMethodKeyring)
	base := writeValidWorkflowSDL(t)
	patchPath := filepath.Join(t.TempDir(), "changes.yaml")
	for _, tc := range []struct {
		name, patch, base, wantError string
	}{
		{"valid", "services:\n  web:\n    image: nginx:1.27-alpine\n", base, ""},
		{"missing base", "services:\n  web:\n    image: nginx:1.27-alpine\n", "", "--base-sdl"},
		{"secret reference", "services:\n  web:\n    env:\n      TOKEN: ac-secret://TOKEN\n", base, "require a Console context"},
		{"unknown service", "services:\n  missing:\n    image: nginx:1.27-alpine\n", base, "service"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(patchPath, []byte(tc.patch), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := findCommand(CommandsWithManager(func() string { return home }, func() string { return "chain" }, func() *aktctx.Manager { return m }), "update")
			args := []string{patchPath, "123", "--patch", "--dry-run"}
			if tc.base != "" {
				args = append(args, "--base-sdl", tc.base)
			}
			_, err := executeCommand(t, cmd, args...)
			if tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
		})
	}
}
