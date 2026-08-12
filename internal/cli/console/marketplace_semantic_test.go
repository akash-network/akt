package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestMarketplaceCommandsPreserveCatalogSemantics(t *testing.T) {
	m := newTestManager(t)

	tests := []struct {
		name     string
		args     []string
		path     string
		response string
		want     string
	}{
		{
			name:     "provider regions",
			args:     []string{"provider", "regions"},
			path:     "/v1/provider-regions",
			response: `[{"key":"us-west","description":"US West","providers":["akash1provider"]}]`,
			want:     `[{"key":"us-west","description":"US West","providers":["akash1provider"]}]`,
		},
		{
			name:     "provider auditors",
			args:     []string{"provider", "auditors"},
			path:     "/v1/auditors",
			response: `[{"id":"auditor-1","name":"Akash Auditor","address":"akash1auditor","website":"https://auditor.example.test"}]`,
			want:     `[{"id":"auditor-1","name":"Akash Auditor","address":"akash1auditor","website":"https://auditor.example.test"}]`,
		},
		{
			name:     "template list retains catalog tree",
			args:     []string{"template", "list"},
			path:     "/v1/templates-list",
			response: `{"data":[{"title":"Compute","templates":[{"id":"tpl-1","name":"worker"}]}]}`,
			want:     `[{"title":"Compute","templates":[{"id":"tpl-1","name":"worker"}]}]`,
		},
		{
			name:     "template get",
			args:     []string{"template", "get", "tpl%2Fone"},
			path:     "/v1/templates/tpl%252Fone",
			response: `{"data":{"id":"tpl/one","name":"worker","summary":"one worker","deploy":"services: {}\n","readme":"run it"}}`,
			want:     `{"id":"tpl/one","name":"worker","summary":"one worker","deploy":"services: {}\n","readme":"run it"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodGet || r.URL.EscapedPath() != test.path {
					t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.EscapedPath(), test.path)
				}
				if _, present := r.Header["X-Api-Key"]; present {
					t.Error("public catalog request unexpectedly sent an API key")
				}
				writeJSON(t, w, test.response)
			}))
			defer srv.Close()

			args := append(append([]string(nil), test.args...), "--output", "json")
			out, err := execConsole(t, m, srv.URL, args...)
			if err != nil {
				t.Fatalf("execute command: %v", err)
			}
			if requests != 1 {
				t.Fatalf("requests = %d, want exactly one", requests)
			}

			var got, want any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("decode command output %q: %v", out, err)
			}
			if err := json.Unmarshal([]byte(test.want), &want); err != nil {
				t.Fatalf("decode expected contract: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("output = %#v, want %#v", got, want)
			}
		})
	}
}

func TestMarketplaceCommandsRetainOperationContextOnServiceFailure(t *testing.T) {
	m := newTestManager(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "provider regions", args: []string{"provider", "regions"}, want: "list provider regions"},
		{name: "provider auditors", args: []string{"provider", "auditors"}, want: "list auditors"},
		{name: "gpu prices", args: []string{"gpu"}, want: "get GPU prices"},
		{name: "template list", args: []string{"template", "list"}, want: "list templates"},
		{name: "template get", args: []string{"template", "get", "tpl-1"}, want: "get template tpl-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				http.Error(w, "catalog unavailable", http.StatusServiceUnavailable)
			}))
			defer srv.Close()

			out, err := execConsole(t, m, srv.URL, test.args...)
			if err == nil {
				t.Fatal("service failure returned success")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want operation context %q", err, test.want)
			}
			if !strings.Contains(out, "Error: "+test.want+":") {
				t.Fatalf("diagnostic = %q, want operation context %q", out, test.want)
			}
			if strings.Contains(out, "catalog unavailable") {
				t.Fatalf("diagnostic leaked the service response body: %q", out)
			}
			if requests != 3 {
				t.Fatalf("requests = %d, want the bounded three-attempt retry policy", requests)
			}
		})
	}
}

func TestTemplateSDLRejectsMissingSource(t *testing.T) {
	m := newTestManager(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v1/templates/tpl-empty" {
			t.Errorf("path = %s, want /v1/templates/tpl-empty", r.URL.EscapedPath())
		}
		writeJSON(t, w, `{"data":{"id":"tpl-empty","name":"empty"}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "template", "sdl", "tpl-empty")
	if err == nil || !strings.Contains(err.Error(), "template tpl-empty has no deploy SDL") {
		t.Fatalf("error = %v, want missing deploy SDL", err)
	}
	if !strings.Contains(out, "Error: template tpl-empty has no deploy SDL") {
		t.Fatalf("diagnostic = %q, want missing deploy SDL error", out)
	}
}
