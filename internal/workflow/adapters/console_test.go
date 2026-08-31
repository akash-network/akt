package adapters

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"pkg.akt.dev/akt/internal/console"
	"pkg.akt.dev/akt/internal/workflow/steps"
	"pkg.akt.dev/go/sdl"
)

const testConsoleCtx = "test-ctx"

const validConsoleSDL = `version: "2.0"
services:
  web:
    image: nginx:1.27-alpine
    expose:
      - port: 80
        as: 80
        to:
          - global: true
profiles:
  compute:
    web:
      resources:
        cpu:
          units: 0.5
        memory:
          size: 512Mi
        storage:
          size: 512Mi
  placement:
    dcloud:
      pricing:
        web:
          denom: uact
          amount: 10000
deployment:
  web:
    dcloud:
      profile: web
      count: 1
`

// newConsoleClient wires an httptest server into a console client backed
// consoleChainClient with a temp manifest root.
func newConsoleClient(t *testing.T, handler http.HandlerFunc) (steps.ChainClient, string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments" {
			_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"hasMore":false}}}`))
			return
		}
		handler(w, r)
	}))
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

func consoleSDLHash(t *testing.T, content string) string {
	t.Helper()
	doc, err := sdl.Read([]byte(content))
	if err != nil {
		t.Fatalf("read SDL: %v", err)
	}
	version, err := doc.Version()
	if err != nil {
		t.Fatalf("derive SDL version: %v", err)
	}
	return base64.StdEncoding.EncodeToString(version)
}

func TestConsoleCreateDeployment(t *testing.T) {
	const sdlText = validConsoleSDL
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
	if data["rail"] != "console" || data["auto_top_up"] != "daily" {
		t.Errorf("data = %v, want Console rail and daily auto-top-up metadata", data)
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
	const sdlText = validConsoleSDL

	var gotSDL any
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotSDL = decodeEnvelope(t, r)["sdl"]
		_, _ = w.Write([]byte(`{"data":{"dseq":"1","manifest":"[]","signTx":{"code":0,"transactionHash":"HASH-RAW","rawLog":""}}}`))
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
	sdlPath := writeTestSDL(t, validConsoleSDL)

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

func TestConsoleCreateDeploymentDerivesMissingManifest(t *testing.T) {
	// Console occasionally omits the manifest from a successful create
	// response. The client derives the same manifest while hashing the SDL, so
	// the workflow can still create its lease without replaying the create.
	c, root := newConsoleClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"dseq":"4244","manifest":"","signTx":{"code":0,"transactionHash":"HASH-DERIVED","rawLog":""}}}`))
	})

	if _, err := c.BroadcastTx(context.Background(), msgCreateDeployment, map[string]string{
		"sdl":     validConsoleSDL,
		"deposit": "5",
	}); err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}

	manifest, err := console.LoadManifest(root, testConsoleCtx, "4244")
	if err != nil {
		t.Fatalf("derived manifest was not cached: %v", err)
	}
	if !json.Valid([]byte(manifest)) || !strings.Contains(manifest, `"name":"web"`) {
		t.Errorf("derived manifest = %q, want valid manifest JSON for the web service", manifest)
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
	const sdlText = validConsoleSDL
	expectedHash := consoleSDLHash(t, sdlText)
	sdlPath := writeTestSDL(t, sdlText)

	var gotMethod, gotPath string
	var gotData map[string]any

	c, _ := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"777"},"state":"active"},"leases":[]}}`))
			return
		}
		gotData = decodeEnvelope(t, r)
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"777"},"state":"active","hash":"` + expectedHash + `"},"leases":[]}}`))
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
		_, _ = w.Write([]byte(`{"data":{"success":true}}`))
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
	if !errors.Is(err, console.ErrAlreadyClosed) {
		t.Fatalf("already-closed error = %v, want ErrAlreadyClosed", err)
	}
	if res != nil {
		t.Fatalf("already-closed result = %+v, want no completed transaction", res)
	}
}

func TestConsoleCloseDeploymentAcceptsReconciledPostState(t *testing.T) {
	var submitted atomic.Bool
	var deletes atomic.Int32
	c, _ := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			state := "active"
			if submitted.Load() {
				state = "closed"
			}
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"888"},"state":"` + state + `"}}}`))
		case http.MethodDelete:
			deletes.Add(1)
			submitted.Store(true)
			_, _ = w.Write([]byte(`{"data":{}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	res, err := c.BroadcastTx(context.Background(), msgCloseDeployment, map[string]string{"dseq": "888"})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}
	if res == nil || deletes.Load() != 1 {
		t.Fatalf("reconciled result = %+v, DELETE requests = %d", res, deletes.Load())
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
	for _, want := range []string{`"gov.MsgVote"`, "is not supported on the Console workflow rail", "--deploy-via chain"} {
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
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"900"},"state":"active"},"leases":[{"id":{"owner":"akash1x","dseq":"900","gseq":2,"oseq":3,"provider":"akash1provider"},"state":"active"}]}}`))
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

func TestConsoleCreateLeaseAcceptsExactReadBackWithoutReplayingPost(t *testing.T) {
	var posts atomic.Int32
	c, root := newConsoleClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			posts.Add(1)
			w.WriteHeader(http.StatusBadGateway)
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"900"},"state":"active"},"leases":[{"id":{"dseq":"900","gseq":2,"oseq":3,"provider":"akash1provider"},"state":"active"}]}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	if err := console.SaveManifest(root, testConsoleCtx, "900", "[MANIFEST]"); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	res, err := c.BroadcastTx(context.Background(), msgCreateLease, map[string]string{
		"dseq": "900", "gseq": "2", "oseq": "3", "provider": "akash1provider",
	})
	if err != nil {
		t.Fatalf("BroadcastTx: %v", err)
	}
	if res == nil || posts.Load() != 1 {
		t.Fatalf("reconciled result = %+v, POST requests = %d", res, posts.Load())
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
	if !strings.Contains(err.Error(), "not supported on the Console workflow rail without chain access") {
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
