package console

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/capability"
	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
)

// newAuthedManager returns a manager whose "prod" context already carries a
// Console API key, for the authenticated command surface.
func newAuthedManager(t *testing.T) *aktctx.Manager {
	t.Helper()

	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	return m
}

// TestWalletBalanceScalesMicroACT is a money-display test. /v1/balances is
// reported in µACT integer units; every field must be divided by 1e6 before
// it reaches the user. A missed division on any one of the three fields
// misreports the user's balance by six orders of magnitude — and
// DeploymentsUSD (escrowed funds) is the field with no other caller.
func TestWalletBalanceScalesMicroACT(t *testing.T) {
	m := newAuthedManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/balances" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		// 12.34 available, 7.50 escrowed, 19.84 total.
		writeJSON(t, w, `{"data":{"balance":12340000,"deployments":7500000,"total":19840000}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "wallet", "balance")
	if err != nil {
		t.Fatalf("wallet balance: %v", err)
	}

	var got struct {
		Available        string `json:"available"`
		InDeployments    string `json:"inDeployments"`
		Total            string `json:"total"`
		AllocationStatus string `json:"allocationStatus"`
		AllocationNote   string `json:"allocationNote"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not the documented JSON shape (%q): %v", out, err)
	}

	if got.Available != "$12.34" {
		t.Errorf("available = %q, want $12.34", got.Available)
	}
	if got.InDeployments != "$7.50" {
		t.Errorf("inDeployments = %q, want $7.50 (escrow must be scaled too)", got.InDeployments)
	}
	if got.Total != "$19.84" {
		t.Errorf("total = %q, want $19.84", got.Total)
	}
	if got.AllocationStatus != "provisional" {
		t.Errorf("allocationStatus = %q, want provisional", got.AllocationStatus)
	}
	for _, want := range []string{"may lag", "recent creates and closes", "total is authoritative"} {
		if !strings.Contains(got.AllocationNote, want) {
			t.Errorf("allocationNote = %q, want it to contain %q", got.AllocationNote, want)
		}
	}
}

func TestFormatUSDPreservesSubCentValues(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{0, "$0.00"},
		{12.34, "$12.34"},
		{0.01, "$0.01"},
		{0.005, "$0.005"},
		{0.000001, "$0.000001"},
		{0.0000004, "$<0.000001"},
		{-0.0000004, "-$<0.000001"},
	}

	for _, tt := range tests {
		if got := formatUSD(tt.value); got != tt.want {
			t.Errorf("formatUSD(%g) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

// TestWalletCostFormatsUSD covers the weekly-cost path: the API returns a bare
// number and the CLI must render it as a USD string, not a raw float.
func TestWalletCostFormatsUSD(t *testing.T) {
	m := newAuthedManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/weekly-cost" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		writeJSON(t, w, `{"data":{"weeklyCost":3.5}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "wallet", "cost")
	if err != nil {
		t.Fatalf("wallet cost: %v", err)
	}

	if !strings.Contains(out, `"weeklyCost": "$3.50"`) {
		t.Errorf("weekly cost should render as $3.50, got %q", out)
	}
}

// TestWalletSettingsWireShape covers both branches of `wallet settings`: the
// read (GET) and the write (PUT with the data-enveloped body). Auto-reload
// tops the managed wallet up with real money, so the enable/disable flag must
// reach the API exactly as typed.
func TestWalletSettingsWireShape(t *testing.T) {
	m := newAuthedManager(t)

	var gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)

		if r.URL.Path != "/v1/wallet-settings" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Method == http.MethodPut && strings.Contains(gotBody, `"autoReloadEnabled":false`) {
			writeJSON(t, w, `{"data":{"autoReloadEnabled":false}}`)
			return
		}
		writeJSON(t, w, `{"data":{"autoReloadEnabled":true}}`)
	}))
	defer srv.Close()

	// Read.
	if _, err := execConsole(t, m, srv.URL, "wallet", "settings"); err != nil {
		t.Fatalf("wallet settings (read): %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("read used %s, want GET", gotMethod)
	}

	// Write: true.
	if _, err := execConsole(t, m, srv.URL, "wallet", "settings", "true"); err != nil {
		t.Fatalf("wallet settings true: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("write used %s, want PUT", gotMethod)
	}
	if !strings.Contains(gotBody, `"autoReloadEnabled":true`) {
		t.Errorf("PUT body = %s, want autoReloadEnabled:true", gotBody)
	}

	// Write: false must send false, not omit the field.
	if _, err := execConsole(t, m, srv.URL, "wallet", "settings", "false"); err != nil {
		t.Fatalf("wallet settings false: %v", err)
	}
	if !strings.Contains(gotBody, `"autoReloadEnabled":false`) {
		t.Errorf("PUT body = %s, want autoReloadEnabled:false", gotBody)
	}
}

// TestSettingsRejectNonBooleanValue covers parseBoolValue on both settings
// commands. A permissive parse ("yes", "1", "TRUE") would silently disagree
// with the strict true|false the help text documents — and on the wallet
// command that decides whether real money is auto-charged.
func TestSettingsRejectNonBooleanValue(t *testing.T) {
	m := newAuthedManager(t)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeJSON(t, w, `{"data":{}}`)
	}))
	defer srv.Close()

	for _, bad := range []string{"yes", "1", "TRUE", "on"} {
		if _, err := execConsole(t, m, srv.URL, "wallet", "settings", bad); err == nil {
			t.Errorf("wallet settings %q must be rejected", bad)
		} else if !strings.Contains(err.Error(), "auto-reload must be true or false") {
			t.Errorf("wallet settings %q: unexpected error %v", bad, err)
		}

		if _, err := execConsole(t, m, srv.URL, "deployment", "settings", "42", bad); err == nil {
			t.Errorf("deployment settings %q must be rejected", bad)
		} else if !strings.Contains(err.Error(), "auto-top-up must be true or false") {
			t.Errorf("deployment settings %q: unexpected error %v", bad, err)
		}
	}

	if requests != 0 {
		t.Errorf("a rejected boolean must not reach the API, got %d requests", requests)
	}
}

// TestDeploymentSettingsWireShape covers the read and write branches of
// `deployment settings`, including the PATCH body the API expects. Auto-top-up
// draws from the managed wallet, so the dseq and flag must both be right.
func TestDeploymentSettingsWireShape(t *testing.T) {
	m := newAuthedManager(t)

	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)

		writeJSON(t, w, `{"data":{"dseq":"12345","autoTopUpEnabled":true}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "deployment", "settings", "12345"); err != nil {
		t.Fatalf("deployment settings (read): %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v2/deployment-settings/12345" {
		t.Errorf("read = %s %s, want GET /v2/deployment-settings/12345", gotMethod, gotPath)
	}

	if _, err := execConsole(t, m, srv.URL, "deployment", "settings", "12345", "true"); err != nil {
		t.Fatalf("deployment settings (write): %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/v2/deployment-settings/12345" {
		t.Errorf("write = %s %s, want PATCH /v2/deployment-settings/12345", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, `"autoTopUpEnabled":true`) {
		t.Errorf("PATCH body = %s", gotBody)
	}
}

// TestDeploymentSettingsCreatesWhenAbsent covers the PATCH->POST fallback: a
// deployment with no settings row yet answers 404 to the PATCH, and the client
// must create the row instead of surfacing "not found" to the user.
func TestDeploymentSettingsCreatesWhenAbsent(t *testing.T) {
	m := newAuthedManager(t)

	var methods []string
	var postBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The command resolves the deployment before touching settings, so a
		// dseq that does not exist cannot have auto-top-up enabled on it.
		if strings.HasPrefix(r.URL.Path, "/v1/deployments/") {
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"}}}}`)
			return
		}

		methods = append(methods, r.Method)

		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		b, _ := io.ReadAll(r.Body)
		postBody = string(b)
		writeJSON(t, w, `{"data":{"dseq":"12345","autoTopUpEnabled":true}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "deployment", "settings", "12345", "true"); err != nil {
		t.Fatalf("settings on a deployment with no settings row: %v", err)
	}

	if len(methods) != 2 || methods[0] != http.MethodPatch || methods[1] != http.MethodPost {
		t.Fatalf("methods = %v, want [PATCH POST]", methods)
	}
	// The POST body must carry the dseq; the PATCH carried it in the path.
	if !strings.Contains(postBody, `"dseq":"12345"`) || !strings.Contains(postBody, `"autoTopUpEnabled":true`) {
		t.Errorf("POST body = %s, want dseq and flag", postBody)
	}
}

// TestDepositRejectsNonPositiveAmount covers the guard between the deposit
// parser (which permits zero) and the API: a zero or omitted amount must fail
// locally with a message pointing at the positional argument, rather than
// sending a no-op charge.
func TestDepositRejectsNonPositiveAmount(t *testing.T) {
	m := newAuthedManager(t)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeJSON(t, w, `{"data":{}}`)
	}))
	defer srv.Close()

	for _, arg := range [][]string{
		{"deployment", "deposit", "12345"},      // no amount at all
		{"deployment", "deposit", "12345", "0"}, // explicit zero
	} {
		_, err := execConsole(t, m, srv.URL, arg...)
		if err == nil {
			t.Errorf("%v must be rejected", arg)
			continue
		}
		if !strings.Contains(err.Error(), "positive USD amount") {
			t.Errorf("%v: unexpected error %v", arg, err)
		}
	}

	if requests != 0 {
		t.Errorf("a rejected amount must not reach the API, got %d requests", requests)
	}
}

// TestCreateRejectsNegativeDeposit covers the negative branch of the shared
// deposit parser through the CLI. "-5" must never be sent as a charge.
func TestCreateRejectsNegativeDeposit(t *testing.T) {
	m := newAuthedManager(t)

	sdlPath := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(sdlPath, []byte(validConsoleDeploymentSDL), 0o600); err != nil {
		t.Fatalf("write SDL: %v", err)
	}

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeJSON(t, w, `{"data":{"dseq":"1"}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "deployment", "create", sdlPath, "$-5"); err == nil {
		t.Fatal("a negative deposit must be rejected")
	}
	if requests != 0 {
		t.Errorf("a negative deposit must not reach the API, got %d requests", requests)
	}
}

// TestCreateRejectsMissingSDLBeforeCharging pins the ordering: the SDL file is
// read before the deployment request goes out, so a typo'd path cannot leave
// the user with a funded deployment carrying no workload.
func TestCreateRejectsMissingSDLBeforeCharging(t *testing.T) {
	m := newAuthedManager(t)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeJSON(t, w, `{"data":{"dseq":"1"}}`)
	}))
	defer srv.Close()

	_, err := execConsole(t, m, srv.URL, "deployment", "create", filepath.Join(t.TempDir(), "nope.yaml"), "5")
	if err == nil {
		t.Fatal("a missing SDL file must fail")
	}
	if !strings.Contains(err.Error(), "read SDL file") {
		t.Errorf("unexpected error: %v", err)
	}
	if requests != 0 {
		t.Errorf("no deployment must be created for an unreadable SDL, got %d requests", requests)
	}
}

// TestCreateCachesManifestForLeaseCreate covers the manifest hand-off that
// makes `lease create` work without re-passing the manifest: the manifest
// returned by `deployment create` is written to the per-context cache with
// owner-only permissions (it can carry private registry credentials).
func TestCreateCachesManifestForLeaseCreate(t *testing.T) {
	m := newAuthedManager(t)

	sdlPath := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(sdlPath, []byte(validConsoleDeploymentSDL), 0o600); err != nil {
		t.Fatalf("write SDL: %v", err)
	}

	const manifest = `[{"name":"dcloud","services":[]}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/deployments" {
			writeJSON(t, w, `{"data":{"deployments":[],"pagination":{"hasMore":false}}}`)
			return
		}
		body, _ := json.Marshal(map[string]any{
			"data": map[string]any{
				"dseq":     "9911",
				"manifest": manifest,
				"signTx": map[string]any{
					"code":            0,
					"transactionHash": "ABC123",
					"rawLog":          "",
				},
			},
		})
		writeJSON(t, w, string(body))
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "deployment", "create", sdlPath, "5")
	if err != nil {
		t.Fatalf("deployment create: %v", err)
	}

	if !strings.Contains(out, `"dseq": "9911"`) || !strings.Contains(out, `"txHash": "ABC123"`) {
		t.Errorf("create output should report dseq and tx hash, got %q", out)
	}
	if strings.Contains(out, `"note"`) {
		t.Errorf("a successful cache write must not emit a note, got %q", out)
	}
	if strings.Contains(out, `"state"`) || strings.Contains(out, `"open"`) {
		t.Errorf("create acknowledgement must not invent deployment state, got %q", out)
	}
	for _, want := range []string{
		`"autoTopUp"`,
		`"enabled": true`,
		`"frequency": "daily"`,
		`"disableCommand": "akt console deployment settings 9911 false"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("create output does not contain %q: %s", want, out)
		}
	}

	cached, err := console.LoadManifest(m.Root(), "prod", "9911")
	if err != nil {
		t.Fatalf("manifest was not cached: %v", err)
	}
	if cached != manifest {
		t.Errorf("cached manifest = %q, want %q", cached, manifest)
	}

	path, err := console.ManifestPath(m.Root(), "prod", "9911")
	if err != nil {
		t.Fatalf("ManifestPath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cached manifest: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("cached manifest mode = %o, want no group/other access", perm)
	}
}

// TestLeaseCreateUsesCachedManifest closes the loop: `lease create` must find
// the manifest cached by `deployment create` and send it with the lease.
func TestLeaseCreateUsesCachedManifest(t *testing.T) {
	m := newAuthedManager(t)

	const manifest = `[{"name":"dcloud"}]`
	if err := console.SaveManifest(m.Root(), "prod", "9911", manifest); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		writeJSON(t, w, `{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"9911"}},"leases":[{"id":{"owner":"akash1x","dseq":"9911","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active"}]}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "lease", "create", "9911", "akash1provider"); err != nil {
		t.Fatalf("lease create: %v", err)
	}

	if !strings.Contains(gotBody, `dcloud`) {
		t.Errorf("cached manifest not sent with the lease: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"provider":"akash1provider"`) {
		t.Errorf("provider not sent: %s", gotBody)
	}
	// gseq/oseq default to 1; a zero would address a nonexistent order.
	if !strings.Contains(gotBody, `"gseq":1`) || !strings.Contains(gotBody, `"oseq":1`) {
		t.Errorf("gseq/oseq defaults not sent: %s", gotBody)
	}
}

// TestLeaseCreateRequiresProviderAndManifest covers the two guards that stop a
// lease from being created against the wrong provider or with no manifest
// (which would leave the lease paid-for but unserviced).
func TestLeaseCreateRequiresProviderAndManifest(t *testing.T) {
	m := newAuthedManager(t)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeJSON(t, w, `{"data":{}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "lease", "create", "9911"); err == nil {
		t.Error("lease create without a provider must fail")
	} else if !strings.Contains(err.Error(), "provider is required") {
		t.Errorf("unexpected error: %v", err)
	}

	// Provider given, but nothing cached for this dseq.
	if _, err := execConsole(t, m, srv.URL, "lease", "create", "4242", "akash1provider"); err == nil {
		t.Error("lease create without a manifest must fail")
	} else if !strings.Contains(err.Error(), "no cached manifest") {
		t.Errorf("unexpected error: %v", err)
	}

	if requests != 0 {
		t.Errorf("neither failure may reach the API, got %d requests", requests)
	}
}

// TestBidListReportsEmptyResultPlainly covers the empty-bids branch. Providers
// bid asynchronously, so an empty list right after create is normal; printing
// "[]" would read as a failure.
func TestBidListReportsEmptyResultPlainly(t *testing.T) {
	m := newAuthedManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "no bids yet" is only honest for a deployment that exists, so the
		// empty case resolves it first.
		if strings.HasPrefix(r.URL.Path, "/v1/deployments/") {
			writeJSON(t, w, `{"data":{"deployment":{"id":{"dseq":"12345"}}}}`)
			return
		}

		if r.URL.Query().Get("dseq") != "12345" {
			t.Errorf("dseq not forwarded: %v", r.URL.Query())
		}
		writeJSON(t, w, `{"data":[]}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "bid", "list", "12345")
	if err != nil {
		t.Fatalf("bid list: %v", err)
	}
	if !strings.Contains(out, "No bids yet") {
		t.Errorf("empty bid list should be explained, got %q", out)
	}
}

// TestLogoutRemovesStoredCredential covers the credential-removal path. A
// logout that leaves the key on disk is a real leak: the file outlives the
// user's expectation that they signed out.
func TestLogoutRemovesStoredCredential(t *testing.T) {
	m := newAuthedManager(t)

	path := aktctx.ConsoleAPIKeyPath(m.Root(), "prod")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("precondition: credential should exist: %v", err)
	}

	out, err := execConsole(t, m, "", "logout")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if !strings.Contains(out, "prod") {
		t.Errorf("logout should name the context, got %q", out)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("credential file still present after logout (stat err = %v)", err)
	}

	// Logout must be idempotent: a second run has nothing to remove.
	if _, err := execConsole(t, m, "", "logout"); err != nil {
		t.Errorf("second logout should be a no-op, got %v", err)
	}
}

// TestAPIKeyCreateShowsSecretOnceWithWarning covers the one place the CLI
// prints a Console secret. The secret and the "shown once" warning must both
// be present — a silent drop leaves the user with an unusable key.
func TestAPIKeyCreateShowsSecretOnceWithWarning(t *testing.T) {
	m := newAuthedManager(t)

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/api-keys" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		writeJSON(t, w, `{"data":{"id":"key-1","name":"ci","apiKey":"sk-brand-new"}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "apikey", "create", "ci", "2027-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("apikey create: %v", err)
	}

	if !strings.Contains(gotBody, `"name":"ci"`) {
		t.Errorf("name not sent: %s", gotBody)
	}
	if !strings.Contains(out, "sk-brand-new") {
		t.Errorf("the created secret must be printed once, got %q", out)
	}
	if !strings.Contains(out, "not be shown again") {
		t.Errorf("output should warn the secret is shown once, got %q", out)
	}

	// A missing name must be caught locally.
	if _, err := execConsole(t, m, srv.URL, "apikey", "create"); err == nil {
		t.Error("apikey create without a name must fail")
	}
}

// TestAPIKeyDeleteIsIdempotent covers the delete no-op: the client maps 404 to
// success, so re-running a delete script must not fail.
func TestAPIKeyDeleteIsIdempotent(t *testing.T) {
	m := newAuthedManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "apikey", "delete", "key-1")
	if err != nil {
		t.Fatalf("deleting an absent key must be a no-op, got %v", err)
	}
	if !strings.Contains(out, "key-1 deleted") {
		t.Errorf("unexpected output %q", out)
	}
}

func TestAPIKeyDeleteStructuredAcknowledgement(t *testing.T) {
	m := newAuthedManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/api-keys/key-1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			out, err := execConsole(t, m, srv.URL, "--output", format, "apikey", "delete", "key-1")
			if err != nil {
				t.Fatalf("delete absent API key: %v", err)
			}
			got := decodeStructuredOutput(t, format, out)
			want := map[string]any{"id": "key-1", "deleted": true}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("structured delete output = %#v, want %#v", got, want)
			}
		})
	}
}

// TestJWTCreateRejectsInvalidTTLAndScope covers the two local guards on the
// JWT minting path. A non-positive TTL or an empty scope produces a token the
// provider rejects, so both must fail before the request.
func TestJWTCreateRejectsInvalidTTLAndScope(t *testing.T) {
	m := newAuthedManager(t)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeJSON(t, w, `{"data":{"token":"t"}}`)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "jwt", "create", "--ttl", "0"); err == nil {
		t.Error("--ttl 0 must be rejected")
	}
	if _, err := execConsole(t, m, srv.URL, "jwt", "create", "--scope", " , "); err == nil {
		t.Error("an all-blank --scope must be rejected")
	}
	if requests != 0 {
		t.Errorf("invalid JWT parameters must not reach the API, got %d requests", requests)
	}
}

// TestPublicCatalogWorksWithoutAKey covers the keyless surface. The catalog
// commands pass requireKey=false, and breaking that would lock out users who
// have not logged in at all.
func TestPublicCatalogWorksWithoutAKey(t *testing.T) {
	m := newTestManager(t) // no credential stored

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.Header["X-Api-Key"]; present {
			t.Error("no key is configured, so none must be sent")
		}

		switch r.URL.Path {
		case "/v1/gpu-prices":
			writeJSON(t, w, `{"availability":{"total":10,"available":4},"models":[{"vendor":"nvidia","model":"h100","price":{"min":1,"max":3,"avg":2}}]}`)
		case "/v1/providers":
			writeJSON(t, w, `[{"owner":"akash1p","isOnline":true},{"owner":"akash1q"}]`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "gpu")
	if err != nil {
		t.Fatalf("gpu: %v", err)
	}
	if !strings.Contains(out, "h100") {
		t.Errorf("gpu output missing the model, got %q", out)
	}

	// --limit trims client-side (the endpoint has no server-side paging).
	out, err = execConsole(t, m, srv.URL, "provider", "list", "--limit", "1")
	if err != nil {
		t.Fatalf("provider list: %v", err)
	}
	if strings.Count(out, `"owner"`) != 1 {
		t.Errorf("--limit 1 should trim to one provider, got %q", out)
	}
}

// TestProviderGetPrintsRawDocument covers printRawJSON: `provider get` must
// surface fields beyond the typed summary (stats, uptime buckets), which only
// the raw document carries.
func TestProviderGetPrintsRawDocument(t *testing.T) {
	m := newTestManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/providers/akash1p" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		writeJSON(t, w, `{"owner":"akash1p","unmodeledStat":{"leaseCount":7}}`)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "provider", "get", "akash1p")
	if err != nil {
		t.Fatalf("provider get: %v", err)
	}

	if !strings.Contains(out, "unmodeledStat") || !strings.Contains(out, "leaseCount") {
		t.Errorf("raw document fields must survive to the output, got %q", out)
	}
}

// TestFlagKeyOverridesStoredCredential covers the documented resolution order
// (SPEC §7.1/§7.2): --console-api-key beats the stored per-context credential.
// Operators rely on it to run one-off commands as a different identity.
func TestFlagKeyOverridesStoredCredential(t *testing.T) {
	m := newAuthedManager(t) // stores "sekrit"

	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		writeJSON(t, w, userBody)
	}))
	defer srv.Close()

	if _, err := execConsole(t, m, srv.URL, "whoami", "--console-api-key", "flag-key"); err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if gotKey != "flag-key" {
		t.Errorf("x-api-key = %q, want the flag value", gotKey)
	}
}

// TestAuthenticatedCommandsCarryConsoleCapability pins the gating annotations.
// The capability layer uses them to dim or hide commands when no Console key
// is resolvable; a command that loses its annotation silently reappears and
// then fails deep inside an HTTP call instead of up front.
func TestAuthenticatedCommandsCarryConsoleCapability(t *testing.T) {
	root := Commands(func() *aktctx.Manager { return nil })

	gated := map[string]bool{
		"whoami": true, "deployment": true, "bid": true, "lease": true,
		"wallet": true, "usage": true, "apikey": true, "jwt": true,
		"logs": true, "events": true, "status": true, "shell": true,
	}
	keyless := map[string]bool{
		"login": true, "logout": true, "provider": true,
		"gpu": true, "template": true, "screen": true,
	}

	seen := map[string]bool{}
	for _, sub := range root.Commands() {
		name := sub.Name()
		seen[name] = true
		got := sub.Annotations[capability.AnnotationKey]

		switch {
		case gated[name]:
			if got != string(capability.Console) {
				t.Errorf("%q annotation = %q, want %q", name, got, capability.Console)
			}
		case keyless[name]:
			if got != "" {
				t.Errorf("%q must stay ungated (works without a key), got %q", name, got)
			}
		}
	}

	for name := range gated {
		if !seen[name] {
			t.Errorf("gated command %q is missing from the console group", name)
		}
	}
	for name := range keyless {
		if !seen[name] {
			t.Errorf("keyless command %q is missing from the console group", name)
		}
	}
}
