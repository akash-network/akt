package console

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/go/sdl"
)

func TestDeploymentUpdateUsesExactSDLAndReturnsUpdatedIdentity(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	doc, err := sdl.Read([]byte(validConsoleDeploymentSDL))
	if err != nil {
		t.Fatalf("parse test SDL: %v", err)
	}
	version, err := doc.Version()
	if err != nil {
		t.Fatalf("derive test SDL version: %v", err)
	}
	versionHash := base64.StdEncoding.EncodeToString(version)

	sdlPath := filepath.Join(t.TempDir(), "updated.yaml")
	if err := os.WriteFile(sdlPath, []byte(validConsoleDeploymentSDL), 0o600); err != nil {
		t.Fatalf("write test SDL: %v", err)
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.EscapedPath() != "/v1/deployments/555" {
			t.Errorf("request path = %s, want /v1/deployments/555", r.URL.EscapedPath())
		}
		if r.Method == http.MethodGet {
			writeJSON(t, w, `{"data":{"deployment":{"id":{"owner":"akash1owner","dseq":"555"},"state":"active"},"leases":[]}}`)
			return
		}
		if r.Method != http.MethodPut {
			t.Errorf("request method = %s, want GET or PUT", r.Method)
		}
		if got := r.Header.Get("x-api-key"); got != "sekrit" {
			t.Errorf("x-api-key = %q, want configured credential", got)
		}

		var body struct {
			Data struct {
				SDL string `json:"sdl"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.Data.SDL != validConsoleDeploymentSDL {
			t.Errorf("request SDL changed in transit")
		}
		writeJSON(t, w, `{"data":{"deployment":{"id":{"owner":"akash1owner","dseq":"555"},"state":"active","hash":"`+versionHash+`"},"leases":[]}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "deployment", "update", "555", sdlPath, "--output", "json")
	if err != nil {
		t.Fatalf("deployment update: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want one preflight and one update", requests)
	}

	got := decodeStructuredMap(t, "json", out)
	deployment, ok := got["deployment"].(map[string]any)
	if !ok {
		t.Fatalf("deployment output = %#v, want object", got["deployment"])
	}
	id, ok := deployment["id"].(map[string]any)
	if !ok || id["owner"] != "akash1owner" || id["dseq"] != "555" {
		t.Fatalf("deployment identity = %#v, want akash1owner/555", deployment["id"])
	}
	if deployment["state"] != "active" || deployment["hash"] != versionHash {
		t.Fatalf("deployment state/hash = %#v/%#v, want active/%s", deployment["state"], deployment["hash"], versionHash)
	}
}

func TestDeploymentUpdateRejectsLocalSDLFailuresBeforeNetwork(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer srv.Close()

	invalidPath := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte("not: [valid"), 0o600); err != nil {
		t.Fatalf("write invalid SDL: %v", err)
	}

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "missing file", path: filepath.Join(t.TempDir(), "missing.yaml"), want: "read SDL file"},
		{name: "invalid SDL", path: invalidPath, want: "update deployment 555: prepare deployment SDL"},
	} {
		t.Run(test.name, func(t *testing.T) {
			out, err := execConsole(t, m, srv.URL, "deployment", "update", "555", test.path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if !strings.Contains(out, "Error: "+test.want) {
				t.Fatalf("diagnostic = %q, want %q", out, test.want)
			}
		})
	}

	if requests != 0 {
		t.Fatalf("invalid local SDL inputs made %d network requests", requests)
	}
}
