package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/console"
	"pkg.akt.dev/akt/internal/workflow/steps"
)

const testConsoleCtx = "test-ctx"

// newConsoleClient wires an httptest server into a console client backed
// consoleChainClient with a temp manifest root.
func newConsoleClient(t *testing.T, handler http.HandlerFunc) (steps.ChainClient, string) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	root := t.TempDir()

	return NewConsoleChainClient(console.New(srv.URL, "test-key"), nil, root, testConsoleCtx), root
}

// decodeEnvelope decodes a {"data": ...} request body.
func decodeEnvelope(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}

	return body.Data
}

func writeTestSDL(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write SDL: %v", err)
	}

	return path
}

func TestConsoleCreateDeployment(t *testing.T) {
	const sdlText = "services:\n  web:\n    image: nginx\n"
	sdlPath := writeTestSDL(t, sdlText)

	var gotMethod, gotPath string
	var gotData map[string]any

	c, root := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotData = decodeEnvelope(t, r)

		_, _ = w.Write([]byte(`{"data":{"dseq":"4242","manifest":"[{\"name\":\"web\"}]","signTx":{"code":0,"transactionHash":"HASH1","rawLog":""}}}`))
	})

	res, err := c.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
		"sdl":     sdlPath,
		"deposit": "5",
	})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/v1/deployments" {
		t.Errorf("request = %s %s, want POST /v1/deployments", gotMethod, gotPath)
	}
	if gotData["sdl"] != sdlText {
		t.Errorf("sent sdl = %q, want file content %q", gotData["sdl"], sdlText)
	}
	if dep, ok := gotData["deposit"].(float64); !ok || dep != 5 {
		t.Errorf("sent deposit = %v, want 5 USD", gotData["deposit"])
	}

	if res.TxHash != "HASH1" {
		t.Errorf("TxHash = %q, want HASH1", res.TxHash)
	}

	var data map[string]string
	if err := json.Unmarshal(res.Data, &data); err != nil {
		t.Fatalf("unmarshal result data: %v", err)
	}
	if data["dseq"] != "4242" {
		t.Errorf("data dseq = %q, want string \"4242\"", data["dseq"])
	}

	// The manifest must be cached for the subsequent lease creation.
	manifest, err := console.LoadManifest(root, testConsoleCtx, "4242")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if manifest != `[{"name":"web"}]` {
		t.Errorf("cached manifest = %q", manifest)
	}
}

func TestConsoleCreateDeploymentRawSDL(t *testing.T) {
	const sdlText = "services:\n  raw:\n    image: nginx\n"

	var gotSDL any
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotSDL = decodeEnvelope(t, r)["sdl"]
		_, _ = w.Write([]byte(`{"data":{"dseq":"1","manifest":"[]"}}`))
	})

	// The sdl param is raw content, not a path: it must pass through as-is.
	if _, err := c.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
		"sdl":     sdlText,
		"deposit": "0.5",
	}); err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	if gotSDL != sdlText {
		t.Errorf("sent sdl = %q, want raw content %q", gotSDL, sdlText)
	}
}

func TestConsoleCreateDeploymentSignTxFailure(t *testing.T) {
	// A Console 200 whose managed-wallet broadcast failed on chain
	// (signTx.code != 0) is a failed step: treating it as success would
	// leave deploy.yaml hanging in wait-for-bids until timeout with the
	// rawLog dropped.
	sdlPath := writeTestSDL(t, "services:\n  web:\n    image: nginx\n")

	c, _ := newConsoleClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"dseq":"4243","manifest":"[]","signTx":{"code":5,"transactionHash":"DEADBEEF","rawLog":"insufficient fees"}}}`))
	})

	_, err := c.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
		"sdl":     sdlPath,
		"deposit": "5",
	})
	if err == nil {
		t.Fatal("expected a non-zero signTx code to fail the step")
	}
	for _, want := range []string{"code 5", "insufficient fees", "DEADBEEF"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not surface %q", err.Error(), want)
		}
	}
}

func TestConsoleSDLPathMissingIsError(t *testing.T) {
	// A value that looks like a file path but does not exist is a typo, not
	// raw SDL content: POSTing it as content would create a garbage
	// deployment on the managed wallet.
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request expected for a missing SDL file")
		w.WriteHeader(http.StatusInternalServerError)
	})

	for _, param := range []string{
		"deply.yaml", // typo'd filename
		"missing/deploy.yml",
		filepath.Join(t.TempDir(), "nope.yaml"),
		"some/dir/without/suffix",
	} {
		_, err := c.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
			"sdl":     param,
			"deposit": "5",
		})
		if err == nil {
			t.Errorf("sdl %q: expected missing-file error, got success", param)
			continue
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("sdl %q: error %q does not mention the missing file", param, err)
		}
	}
}

func TestConsoleCreateDeploymentEmptyManifestNotCached(t *testing.T) {
	// An empty manifest must not be cached (mirroring the CLI twin):
	// lease create should fail with the clear no-cached-manifest error
	// instead of sending an empty manifest to the provider.
	c, root := newConsoleClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"dseq":"4244","manifest":""}}`))
	})

	if _, err := c.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
		"sdl":     "services:\n  web:\n    image: nginx\n",
		"deposit": "5",
	}); err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	if _, err := console.LoadManifest(root, testConsoleCtx, "4244"); err == nil {
		t.Fatal("empty manifest must not be cached")
	}
}

func TestConsoleCreateDeploymentDepositValidation(t *testing.T) {
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request expected for invalid deposits")
		w.WriteHeader(http.StatusInternalServerError)
	})

	tests := []struct {
		deposit string
		want    string
	}{
		{"auto", "USD"},
		{"", "USD"},
		{"auto", "--deposit"},
		{"5000000uakt", "USD"},
		{"0.1", "below the Console minimum"},
	}

	for _, tt := range tests {
		_, err := c.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
			"sdl":     "services: {}",
			"deposit": tt.deposit,
		})
		if err == nil {
			t.Errorf("deposit %q: expected error", tt.deposit)
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("deposit %q: error %q does not mention %q", tt.deposit, err, tt.want)
		}
	}
}

func TestConsoleUpdateDeployment(t *testing.T) {
	const sdlText = "services:\n  web:\n    image: nginx:1.27\n"
	sdlPath := writeTestSDL(t, sdlText)

	var gotMethod, gotPath string
	var gotData map[string]any

	c, _ := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotData = decodeEnvelope(t, r)
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"777"},"state":"active"}}}`))
	})

	res, err := c.BroadcastTx(context.Background(), msgUpdateDeployment, map[string]string{
		"sdl":  sdlPath,
		"dseq": "777",
	})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	if gotMethod != http.MethodPut || gotPath != "/v1/deployments/777" {
		t.Errorf("request = %s %s, want PUT /v1/deployments/777", gotMethod, gotPath)
	}
	if gotData["sdl"] != sdlText {
		t.Errorf("sent sdl = %q, want file content", gotData["sdl"])
	}

	var data map[string]string
	_ = json.Unmarshal(res.Data, &data)
	if data["dseq"] != "777" {
		t.Errorf("data dseq = %q, want \"777\"", data["dseq"])
	}
}

func TestConsoleCloseDeployment(t *testing.T) {
	var gotMethod, gotPath string

	c, _ := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	res, err := c.BroadcastTx(context.Background(), msgCloseDeployment, map[string]string{"dseq": "888"})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	if gotMethod != http.MethodDelete || gotPath != "/v1/deployments/888" {
		t.Errorf("request = %s %s, want DELETE /v1/deployments/888", gotMethod, gotPath)
	}

	var data map[string]string
	_ = json.Unmarshal(res.Data, &data)
	if data["dseq"] != "888" {
		t.Errorf("data dseq = %q, want \"888\"", data["dseq"])
	}
}

func TestConsoleCloseDeploymentAlreadyClosed(t *testing.T) {
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // maps to ErrAlreadyClosed
	})

	res, err := c.BroadcastTx(context.Background(), msgCloseDeployment, map[string]string{"dseq": "999"})
	if err != nil {
		t.Fatalf("already-closed must be treated as success, got: %v", err)
	}

	var data map[string]string
	_ = json.Unmarshal(res.Data, &data)
	if data["dseq"] != "999" {
		t.Errorf("data dseq = %q, want \"999\"", data["dseq"])
	}
}

func TestConsoleUnsupportedMsg(t *testing.T) {
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request expected for unsupported messages")
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.BroadcastTx(context.Background(), "gov.MsgVote", nil)
	if err == nil {
		t.Fatal("expected unsupported-command error")
	}
	// SPEC §7.5 wording.
	for _, want := range []string{`"gov.MsgVote"`, "is not supported with console-api auth", "auth-method: keyring"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err.Error(), want)
		}
	}
}

func TestConsoleCreateLeaseUsesCachedManifest(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	c, root := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode lease body: %v", err)
		}
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"900"},"state":"active"}}}`))
	})

	if err := console.SaveManifest(root, testConsoleCtx, "900", "[MANIFEST]"); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	res, err := c.BroadcastTx(context.Background(), msgCreateLease, map[string]string{
		"dseq":     "900",
		"gseq":     "2",
		"oseq":     "3",
		"provider": "akash1provider",
	})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	if gotPath != "/v1/leases" {
		t.Errorf("request path = %s, want /v1/leases", gotPath)
	}
	if gotBody["manifest"] != "[MANIFEST]" {
		t.Errorf("sent manifest = %v, want cached manifest", gotBody["manifest"])
	}

	leases, ok := gotBody["leases"].([]any)
	if !ok || len(leases) != 1 {
		t.Fatalf("sent leases = %v, want exactly one", gotBody["leases"])
	}
	lease := leases[0].(map[string]any)
	if lease["dseq"] != "900" || lease["gseq"] != float64(2) || lease["oseq"] != float64(3) || lease["provider"] != "akash1provider" {
		t.Errorf("lease request = %v", lease)
	}

	var data map[string]string
	_ = json.Unmarshal(res.Data, &data)
	if data["dseq"] != "900" || data["gseq"] != "2" || data["oseq"] != "3" || data["provider"] != "akash1provider" {
		t.Errorf("result data = %v", data)
	}
}

func TestConsoleCreateLeaseMissingManifest(t *testing.T) {
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request expected when the manifest is missing")
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.BroadcastTx(context.Background(), msgCreateLease, map[string]string{
		"dseq":     "901",
		"provider": "akash1provider",
	})
	if err == nil {
		t.Fatal("expected missing-manifest error")
	}
	for _, want := range []string{"no cached manifest", "901", "akt deploy"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestConsoleQueryBidsFallback(t *testing.T) {
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/bids":
			if r.URL.Query().Get("dseq") != "4242" {
				t.Errorf("request = %s?%s, want /v1/bids?dseq=4242", r.URL.Path, r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[{"bid":{"id":{"owner":"akash1x","dseq":"4242","gseq":1,"oseq":1,"provider":"akash1p"},"state":"open","price":{"denom":"uakt","amount":"10"}}}]}`))
		case "/v1/providers/akash1p":
			_, _ = w.Write([]byte(`{"owner":"akash1p","isAudited":true,"attributes":[{"key":"region","value":"us-west"}]}`))
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	raw, err := c.Query(context.Background(), queryMarketBids, map[string]string{"dseq": "4242"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// The response must use the chain query shape: a top-level "bids"
	// array of {"bid": {...}} wrappers, so wait conditions and the prompt
	// step behave identically to keyring auth.
	var res struct {
		Bids []struct {
			Bid struct {
				ID struct {
					DSeq     string `json:"dseq"`
					Provider string `json:"provider"`
				} `json:"id"`
				State string `json:"state"`
			} `json:"bid"`
		} `json:"bids"`
		ProviderMetadata map[string]struct {
			Attributes map[string]string `json:"attributes"`
			Audited    bool              `json:"audited"`
		} `json:"provider_metadata"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal bids: %v", err)
	}
	if len(res.Bids) != 1 {
		t.Fatalf("bids = %d, want 1", len(res.Bids))
	}
	if res.Bids[0].Bid.ID.Provider != "akash1p" || res.Bids[0].Bid.ID.DSeq != "4242" {
		t.Errorf("bid id = %+v", res.Bids[0].Bid.ID)
	}
	if res.ProviderMetadata["akash1p"].Attributes["region"] != "us-west" || !res.ProviderMetadata["akash1p"].Audited {
		t.Errorf("provider metadata = %#v", res.ProviderMetadata)
	}
}

func TestConsoleQueryUnsupportedPathWithoutChain(t *testing.T) {
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	_, err := c.Query(context.Background(), queryMarketLeases, nil)
	if err == nil {
		t.Fatal("expected error for unsupported query path without chain access")
	}
	if !strings.Contains(err.Error(), "not supported with console-api auth") {
		t.Errorf("error %q lacks the unsupported-query wording", err.Error())
	}
}

// recordingChainClient records Query calls for delegation tests.
type recordingChainClient struct {
	path   string
	params map[string]string
}

func (r *recordingChainClient) BroadcastTx(context.Context, string, map[string]string) (*steps.TxResult, error) {
	return nil, nil
}

func (r *recordingChainClient) Query(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	r.path = path
	r.params = params

	return json.RawMessage(`{"bids":[]}`), nil
}

func TestConsoleQueryDelegatesToChain(t *testing.T) {
	rec := &recordingChainClient{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("console API must not be hit when a chain query client exists")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := NewConsoleChainClient(console.New(srv.URL, "k"), rec, t.TempDir(), testConsoleCtx)

	raw, err := c.Query(context.Background(), queryMarketBids, map[string]string{"dseq": "1", "owner": "akash1x"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if rec.path != queryMarketBids || rec.params["owner"] != "akash1x" {
		t.Errorf("delegated query = %q %v", rec.path, rec.params)
	}
	if string(raw) != `{"bids":[]}` {
		t.Errorf("raw = %s, want the chain client's response", raw)
	}
}
