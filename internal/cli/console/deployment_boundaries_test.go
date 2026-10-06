package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	api "pkg.akt.dev/akt/internal/console"
)

func TestDeploymentSecretCommandFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployment.yaml")
	require.NoError(t, os.WriteFile(path, []byte(validConsoleDeploymentSDL), 0600))
	patch := filepath.Join(t.TempDir(), "patch.yaml")
	require.NoError(t, os.WriteFile(patch, []byte("services: ["), 0600))
	for _, tc := range []struct {
		name   string
		args   []string
		want   string
		writes int
	}{
		{"create bad secrets", []string{"deployment", "create", path, "--secrets-file", "/missing-secret-file"}, "regular file", 0},
		{"create refused", []string{"deployment", "create", path}, "422", 1},
		{"update bad secrets", []string{"deployment", "update", "42", path, "--secrets-file", "/missing-secret-file"}, "regular file", 0},
		{"malformed patch", []string{"deployment", "update", "42", patch, "--patch"}, "invalid deployment patch", 0},
		{"missing saved SDL", []string{"deployment", "sdl", "42"}, "no saved definition", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			srv := newDeploymentSecretsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					w.WriteHeader(422)
					return
				}
				writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"}}}`)
			}))
			defer srv.Close()
			_, err := execConsole(t, newAuthedManager(t), srv.URL, tc.args...)
			require.ErrorContains(t, err, tc.want)
			require.Equal(t, tc.writes, writes)
		})
	}
	_, err := execConsole(t, newTestManager(t), "", "deployment", "sdl", "42")
	require.Error(t, err)
}

func TestDeploymentPatchCommandUsesSavedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changes.yaml")
	require.NoError(t, os.WriteFile(path, []byte("services:\n  web:\n    image: nginx:1.28-alpine\n    env: {MODE: production, OLD: null}\n"), 0600))
	writes := 0
	srv := newDeploymentSecretsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"deployment": map[string]any{"id": map[string]string{"dseq": "42"}, "state": "active"}, "consoleSettings": map[string]string{"sdl": validConsoleDeploymentSDL, "manifestVersion": "old"}}}))
			return
		}
		require.Equal(t, http.MethodPatch, r.Method)
		writes++
		var request struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "old", request.Data["ifManifestVersion"])
		require.NotEmpty(t, request.Data["sealedSecrets"])
		service := request.Data["services"].(map[string]any)["web"].(map[string]any)
		require.Equal(t, "nginx:1.28-alpine", service["image"])
		require.Equal(t, map[string]any{"MODE": "production", "OLD": nil}, service["env"])
		writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"42"},"hash":"new"},"manifestVersion":"new"}}`)
	}))
	defer srv.Close()
	out, err := execConsole(t, newAuthedManager(t), srv.URL, "deployment", "update", "42", path, "--patch")
	require.NoError(t, err)
	require.Equal(t, 1, writes)
	require.Contains(t, out, `"manifestVersion": "new"`)
}

func TestSavedShellServiceUsesServerDefinition(t *testing.T) {
	for _, tc := range []struct {
		name, sdl, want string
		status          int
		failure         bool
	}{
		{"fetch failure", "", "pass the service", 401, true},
		{"malformed", "[", "invalid saved SDL", 200, true},
		{"empty", "services: {}", "no services", 200, true},
		{"single", "services: {web: {image: app}}", "web", 200, false},
		{"multiple", "services: {z: {}, a: {}}", "a, z", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"consoleSettings": map[string]string{"sdl": tc.sdl}}})
			}))
			defer srv.Close()
			result, err := savedShellService(context.Background(), api.New(srv.URL, ""), nil, "42")
			if tc.failure {
				require.ErrorContains(t, err, tc.want)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, result)
			}
		})
	}
}
