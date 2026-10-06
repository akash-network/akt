package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	jose "github.com/go-jose/go-jose/v4"

	"pkg.akt.dev/akt/internal/console"
)

func TestConsoleRedeployInheritsClosedSourceAndSealsStdinOverrides(t *testing.T) {
	key, encryption := testSecretsContext(t)
	saved := strings.Replace(validConsoleSDL, "image: nginx:1.27-alpine", "image: nginx:1.27-alpine\n    env:\n      - TOKEN=ac-secret://TOKEN", 1)
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/deployments/41":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"deployment":      map[string]any{"id": map[string]string{"dseq": "41"}, "state": "closed", "hash": "source-version"},
				"consoleSettings": map[string]string{"sdl": saved, "manifestVersion": "source-version"},
			}})
		case "GET /v1/sdl-secrets-context":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": encryption})
		case "POST /v1/deployments":
			data := decodeEnvelope(t, r)
			if data["inheritSecretsFrom"] != "41" || data["sdl"] != saved {
				t.Error("redeploy did not carry the saved source definition and inheritance")
			}
			sealed, ok := data["sealedSecrets"].(string)
			if !ok {
				t.Error("missing secret seal")
				return
			}
			object, err := jose.ParseEncrypted(sealed, []jose.KeyAlgorithm{jose.RSA_OAEP_256}, []jose.ContentEncryption{jose.A256GCM})
			if err != nil {
				t.Error(err)
				return
			}
			plaintext, err := object.Decrypt(key)
			if err != nil {
				t.Error(err)
				return
			}
			var values map[string]string
			if err := json.Unmarshal(plaintext, &values); err != nil || values["TOKEN"] != "private-override" {
				t.Error("secret stdin override was not sealed")
			}
			_, _ = w.Write([]byte(`{"data":{"dseq":"42","signTx":{"code":0,"transactionHash":"REDEPLOY"}}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	root := t.TempDir()
	inputs := NewDeploymentInputs(root, "test", strings.NewReader("TOKEN: private-override\n"))
	client := NewConsoleChainClient(console.New(srv.URL, "test-key"), nil, root, "test", inputs)
	result, err := client.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{"source-dseq": "41", "secrets-file": "-"})
	if err != nil {
		t.Fatal(err)
	}
	if result.TxHash != "REDEPLOY" || strings.Contains(string(result.Data), "private-override") {
		t.Fatalf("unsafe or incomplete result: %s", result.Data)
	}
	if len(methods) != 3 {
		t.Fatalf("unexpected redeploy requests: %v", methods)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("redeploy wrote local secret/manifest state: %v %v", entries, err)
	}
}

func TestConsoleWorkflowPatchUsesSavedVersionAndEmptySeal(t *testing.T) {
	key, encryption := testSecretsContext(t)
	desired := strings.Replace(validConsoleSDL, "nginx:1.27-alpine", "nginx:1.28-alpine", 1)
	version := consoleSDLHash(t, desired)
	patches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/deployments/42":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"deployment":      map[string]any{"id": map[string]string{"dseq": "42"}, "state": "active", "hash": "source-version"},
				"consoleSettings": map[string]string{"sdl": validConsoleSDL, "manifestVersion": "source-version"},
			}})
		case "GET /v1/sdl-secrets-context":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": encryption})
		case "PATCH /v1/deployments/42":
			patches++
			data := decodeEnvelope(t, r)
			if data["ifManifestVersion"] != "source-version" {
				t.Error("missing saved-version concurrency guard")
			}
			if _, exists := data["sdl"]; exists {
				t.Error("whole SDL sent instead of patch")
			}
			sealed, ok := data["sealedSecrets"].(string)
			if !ok {
				t.Error("missing empty seal")
				return
			}
			object, err := jose.ParseEncrypted(sealed, []jose.KeyAlgorithm{jose.RSA_OAEP_256}, []jose.ContentEncryption{jose.A256GCM})
			if err != nil {
				t.Error(err)
				return
			}
			plaintext, err := object.Decrypt(key)
			if err != nil || string(plaintext) != "{}" {
				t.Error("normal-variable patch should explicitly seal an empty map")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"deployment":      map[string]any{"id": map[string]string{"dseq": "42"}, "state": "active", "hash": version},
				"manifestVersion": version,
				"consoleSettings": map[string]string{"sdl": desired, "manifestVersion": version},
			}})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	patch := writeTestSDL(t, "services:\n  web:\n    image: nginx:1.28-alpine\n")
	client := NewConsoleChainClient(console.New(srv.URL, "test-key"), nil, t.TempDir(), "test")
	_, err := client.BroadcastTx(context.Background(), msgUpdateDeployment, map[string]string{"dseq": "42", "sdl": patch, "patch": "true"})
	if err != nil || patches != 1 {
		t.Fatalf("patches=%d err=%v", patches, err)
	}
}
