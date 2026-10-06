package console

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jose "github.com/go-jose/go-jose/v4"
	api "pkg.akt.dev/akt/internal/console"
)

func newDeploymentSecretsServer(t *testing.T, next http.Handler) *httptest.Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return deploymentSecretsServer(t, key, next)
}

func deploymentSecretsServer(t *testing.T, key *rsa.PrivateKey, next http.Handler) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/sdl-secrets-context" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"sub": "cli-user", "kid": "cli-key",
				"jwk":            jose.JSONWebKey{Key: &key.PublicKey, KeyID: "cli-key", Use: "enc", Algorithm: "RSA-OAEP-256"},
				"requiredClaims": []string{"kid", "sub", "exp"},
			}}); err != nil {
				t.Error(err)
			}
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func TestDeploymentCreateSealsSecretFileWithoutCachingValues(t *testing.T) {
	m := newAuthedManager(t)
	dir := t.TempDir()
	sdlPath := filepath.Join(dir, "deploy.yaml")
	secretPath := filepath.Join(dir, "secrets.yaml")
	rawSDL := strings.Replace(validConsoleDeploymentSDL, "image: nginx:1.27-alpine", "image: nginx:1.27-alpine\n    env:\n      - TOKEN=ac-secret://TOKEN", 1)
	if rawSDL == validConsoleDeploymentSDL {
		t.Fatal("SDL fixture image changed")
	}
	if err := os.WriteFile(sdlPath, []byte(rawSDL), 0o600); err != nil {
		t.Fatal(err)
	}
	const value = "secret-value-not-in-output"
	if err := os.WriteFile(secretPath, []byte("TOKEN: "+value+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	posts := 0
	srv := deploymentSecretsServer(t, key, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments" {
			writeJSON(t, w, `{"data":{"deployments":[],"pagination":{"hasMore":false}}}`)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/deployments" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		posts++
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if strings.Contains(string(raw), value) {
			t.Error("plaintext secret reached request")
		}
		var body struct {
			Data struct {
				SDL  string `json:"sdl"`
				Seal string `json:"sealedSecrets"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Error(err)
			return
		}
		if !strings.Contains(body.Data.SDL, "ac-secret://TOKEN") {
			t.Error("secret reference missing")
		}
		sealed, err := jose.ParseEncrypted(body.Data.Seal, []jose.KeyAlgorithm{jose.RSA_OAEP_256}, []jose.ContentEncryption{jose.A256GCM})
		if err != nil {
			t.Error(err)
			return
		}
		plain, err := sealed.Decrypt(key)
		if err != nil {
			t.Error(err)
			return
		}
		var values map[string]string
		if err := json.Unmarshal(plain, &values); err != nil || values["TOKEN"] != value {
			t.Error("sealed map did not carry supplied value")
		}
		writeJSON(t, w, `{"data":{"dseq":"321","manifest":"manifest-not-for-output","signTx":{"code":0,"transactionHash":"tx321"}}}`)
	}))
	defer srv.Close()
	out, err := execConsole(t, m, srv.URL, "deployment", "create", sdlPath, "--secrets-file", secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 1 || !strings.Contains(out, `"dseq": "321"`) || strings.Contains(out, value) || strings.Contains(out, "manifest-not-for-output") {
		t.Fatalf("unsafe or incomplete acknowledgement: %s", out)
	}
	if _, err := api.LoadManifest(m.Root(), "prod", "321"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected manifest cache result: %v", err)
	}
}

func TestDeploymentSDLReadsSavedReferencesOnAnotherMachine(t *testing.T) {
	m := newAuthedManager(t)
	const saved = "---\nservices:\n  web:\n    image: nginx\n    env:\n      - TOKEN=ac-secret://TOKEN\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/deployments/123" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"deployment":      map[string]any{"id": map[string]any{"dseq": "123"}},
			"consoleSettings": map[string]any{"sdl": saved, "manifestVersion": "saved-version"},
		}}); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	out, err := execConsole(t, m, srv.URL, "deployment", "sdl", "123")
	if err != nil || out != saved {
		t.Fatalf("saved SDL = %q, %v", out, err)
	}
}
