package console

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/go/sdl"
)

const actionLogTestSDL = `version: "2.0"
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

func openTestLog(t *testing.T) *actionlog.Logger {
	t.Helper()

	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	return l
}

func TestConsoleMutationsRecordedInActionLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments":
			_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"hasMore":false}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/deployments":
			_, _ = w.Write([]byte(`{"data":{"dseq":"12345","manifest":"m","signTx":{"code":0,"transactionHash":"tx-12345","rawLog":""}}}`))
		case r.Method == http.MethodDelete:
			// 409 is not mapped to already-closed, so the close genuinely fails.
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"conflict"}`))
		default:
			_, _ = w.Write([]byte(`{"data":{}}`))
		}
	}))
	defer srv.Close()

	l := openTestLog(t)
	c := New(srv.URL, "test-key").WithActionLog(l)

	if _, err := c.CreateDeployment(context.Background(), actionLogTestSDL); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	if err := c.CloseDeployment(context.Background(), "999"); err == nil {
		t.Fatal("expected close to fail on conflict")
	}

	entries, err := l.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 console entries, got %d: %+v", len(entries), entries)
	}

	// Newest first: failed close, then successful create.
	if entries[0].Action != "close-deployment" || entries[0].Status != "failed" || entries[0].DSeq != 999 {
		t.Errorf("close entry wrong: %+v", entries[0])
	}
	if entries[1].Action != "create-deployment" || entries[1].Status != "success" || entries[1].DSeq != 12345 {
		t.Errorf("create entry wrong: %+v", entries[1])
	}
}

func TestCredentialEchoIsRedactedFromFailedActionLog(t *testing.T) {
	const apiKey = "action-log-secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"credential ` + apiKey + ` rejected"}`))
	}))
	defer srv.Close()

	l := openTestLog(t)
	c := New(srv.URL, apiKey).WithActionLog(l)
	err := c.CloseDeployment(context.Background(), "999")
	if err == nil {
		t.Fatal("expected close to fail")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatal("returned error exposed the API key")
	}

	entries, readErr := l.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if readErr != nil {
		t.Fatalf("read action log: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one action entry, got %d", len(entries))
	}
	if strings.Contains(entries[0].Error, apiKey) {
		t.Fatal("action log exposed the API key")
	}
	if !strings.Contains(entries[0].Error, "[REDACTED]") {
		t.Fatalf("action log did not retain a redaction marker: %q", entries[0].Error)
	}
}

func TestRedirectTargetIsOmittedFromReturnedErrorAndActionLog(t *testing.T) {
	const apiKey = "redirect-action-log-secret"
	const hostileOrigin = "https://attacker.invalid/collect"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", hostileOrigin+"?api_key="+apiKey)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	l := openTestLog(t)
	err := New(srv.URL, apiKey).WithActionLog(l).CloseDeployment(context.Background(), "999")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "redirect") {
		t.Fatalf("CloseDeployment() error = %v, want redirect rejection", err)
	}
	if strings.Contains(err.Error(), apiKey) || strings.Contains(err.Error(), hostileOrigin) {
		t.Fatalf("returned redirect error exposed untrusted target data: %q", err)
	}

	entries, readErr := l.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if readErr != nil {
		t.Fatalf("read action log: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one action entry, got %d", len(entries))
	}
	if strings.Contains(entries[0].Error, apiKey) || strings.Contains(entries[0].Error, hostileOrigin) {
		t.Fatalf("action log exposed untrusted redirect target data: %q", entries[0].Error)
	}
}

func TestReadResponseBodyEnforcesLimit(t *testing.T) {
	body, err := readResponseBody(strings.NewReader("1234"), 4)
	if err != nil || string(body) != "1234" {
		t.Fatalf("read exact limit = %q, %v", body, err)
	}

	body, err = readResponseBody(strings.NewReader("12345"), 4)
	if err == nil {
		t.Fatal("expected an oversized response to fail")
	}
	if body != nil || !strings.Contains(err.Error(), "4-byte limit") {
		t.Fatalf("oversized response = %q, %v", body, err)
	}

	if _, err := readResponseBody(strings.NewReader(""), -1); err == nil {
		t.Fatal("expected a negative limit to fail")
	}
}

func TestReadResponseBodyPropagatesReaderFailure(t *testing.T) {
	wantErr := errors.New("response stream failed")
	body, err := readResponseBody(iotest.ErrReader(wantErr), 4)
	if body != nil || !errors.Is(err, wantErr) {
		t.Fatalf("readResponseBody() = %q, %v; want reader failure", body, err)
	}
}

func TestRedactResponseSecretWithoutCredentialPreservesDiagnostic(t *testing.T) {
	const diagnostic = "public catalog request failed"
	if got := redactResponseSecret(diagnostic, ""); got != diagnostic {
		t.Fatalf("redactResponseSecret() = %q, want %q", got, diagnostic)
	}
}

func TestCloseAlreadyClosedRecordedAsFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	l := openTestLog(t)
	c := New(srv.URL, "test-key").WithActionLog(l)

	err := c.CloseDeployment(context.Background(), "555")
	if !errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("expected ErrAlreadyClosed, got %v", err)
	}

	entries, readErr := l.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "close-deployment" || entries[0].Status != "failed" || entries[0].DSeq != 555 {
		t.Errorf("already-closed entry wrong: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Error, "already closed") {
		t.Errorf("already-closed entry error = %q", entries[0].Error)
	}
}

func TestRepeatedCloseRecordsTruthfulOutcomes(t *testing.T) {
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/deployments/555" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			state := "active"
			if deletes.Load() > 0 {
				state = "closed"
			}
			_, _ = fmt.Fprintf(w, `{"data":{"deployment":{"id":{"dseq":"555"},"state":%q}}}`, state)
		case http.MethodDelete:
			deletes.Add(1)
			_, _ = w.Write([]byte(`{"data":{"success":true}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	l := openTestLog(t)
	c := New(srv.URL, "test-key").WithActionLog(l)
	if err := c.CloseDeployment(context.Background(), "555"); err != nil {
		t.Fatalf("first CloseDeployment(): %v", err)
	}
	if err := c.CloseDeployment(context.Background(), "555"); !errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("second CloseDeployment() = %v, want ErrAlreadyClosed", err)
	}

	if got := deletes.Load(); got != 1 {
		t.Fatalf("DELETE requests = %d, want 1", got)
	}
	entries, err := l.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("close action entries = %d, want 2: %+v", len(entries), entries)
	}
	if entries[0].Action != "close-deployment" || entries[0].Status != "failed" || entries[0].DSeq != 555 || !strings.Contains(entries[0].Error, "already closed") {
		t.Errorf("second close action entry = %+v", entries[0])
	}
	if entries[1].Action != "close-deployment" || entries[1].Status != "success" || entries[1].DSeq != 555 || entries[1].Error != "" {
		t.Errorf("first close action entry = %+v", entries[1])
	}
}

func TestCloseValidation400RecordedAsFailed(t *testing.T) {
	// A 400 without already-closed semantics is a genuine failure: it must
	// not be logged as a successful close.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"777"},"state":"active"}}}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"deployment cannot be closed while active leases exist"}`))
	}))
	defer srv.Close()

	l := openTestLog(t)
	c := New(srv.URL, "test-key").WithActionLog(l)

	err := c.CloseDeployment(context.Background(), "777")
	if err == nil {
		t.Fatal("expected the validation 400 to fail the close")
	}
	if errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("validation 400 must not map to ErrAlreadyClosed, got %v", err)
	}

	entries, readErr := l.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "close-deployment" || entries[0].Status != "failed" || entries[0].DSeq != 777 {
		t.Errorf("validation-400 close entry wrong: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Error, "active leases") {
		t.Errorf("entry error %q must carry the server's message", entries[0].Error)
	}
}

func TestNewMutationsRecordedInActionLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/42":
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"}}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/wallet-settings":
			_, _ = w.Write([]byte(`{"data":{"autoReloadEnabled":true}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/v2/deployment-settings/42":
			_, _ = w.Write([]byte(`{"data":{"dseq":"42","autoTopUpEnabled":true,"runtimeLimitHours":12}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/api-keys":
			_, _ = w.Write([]byte(`{"data":{"id":"k1","name":"n","apiKey":"secret"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/api-keys":
			_, _ = w.Write([]byte(`{"data":[{"id":"11111111-1111-4111-8111-111111111111","name":"n"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/api-keys/11111111-1111-4111-8111-111111111111":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	l := openTestLog(t)
	c := New(srv.URL, "test-key").WithActionLog(l)

	ctx := context.Background()
	if _, err := c.UpdateWalletSettings(ctx, true); err != nil {
		t.Fatalf("UpdateWalletSettings: %v", err)
	}
	twelveHours := 12
	if _, err := c.SetDeploymentRuntimeLimit(ctx, "42", &twelveHours); err != nil {
		t.Fatalf("SetDeploymentRuntimeLimit: %v", err)
	}
	if _, err := c.CreateAPIKey(ctx, "n", ""); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := c.DeleteAPIKey(ctx, "11111111-1111-4111-8111-111111111111"); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}

	entries, err := l.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Newest first.
	want := []struct {
		action string
		dseq   uint64
	}{
		{"delete-api-key", 0},
		{"create-api-key", 0},
		{"update-deployment-settings", 42},
		{"update-wallet-settings", 0},
	}

	if len(entries) != len(want) {
		t.Fatalf("expected %d entries, got %d: %+v", len(want), len(entries), entries)
	}
	for i, w := range want {
		if entries[i].Action != w.action || entries[i].Status != "success" || entries[i].DSeq != w.dseq {
			t.Errorf("entry %d wrong: want action=%s dseq=%d, got %+v", i, w.action, w.dseq, entries[i])
		}
	}
}

func TestConsoleValidationFailuresOccurBeforeNetworkAndAreLogged(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer srv.Close()

	log := openTestLog(t)
	client := New(srv.URL, "test-key").WithActionLog(log)
	_, err := client.CreateAPIKey(context.Background(), " \t\n", "")
	if err == nil || !strings.Contains(err.Error(), "name must not be blank") {
		t.Fatalf("blank API-key name error = %v", err)
	}
	if _, err := client.UpdateDeployment(context.Background(), "42", "not-valid-sdl: ["); err == nil || !strings.Contains(err.Error(), "prepare deployment SDL") {
		t.Fatalf("invalid deployment SDL error = %v", err)
	}
	zeroHours := 0
	if _, err := client.SetDeploymentRuntimeLimit(context.Background(), "42", &zeroHours); err == nil || !strings.Contains(err.Error(), "at least 1 hour") {
		t.Fatalf("invalid runtime limit error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("local validation failures made %d requests, want zero", requests.Load())
	}

	entries, readErr := log.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if readErr != nil {
		t.Fatalf("read action log: %v", readErr)
	}
	wantActions := []string{"update-deployment-settings", "update-deployment", "create-api-key"}
	if len(entries) != len(wantActions) {
		t.Fatalf("validation action entries = %+v, want %d failures", entries, len(wantActions))
	}
	for i, action := range wantActions {
		if entries[i].Action != action || entries[i].Status != "failed" {
			t.Errorf("validation entry %d = %+v, want failed %s", i, entries[i], action)
		}
	}
}

func TestCreateAPIKeyDefinitiveFailureIsNotMarkedPending(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"name already exists"}`))
	}))
	defer srv.Close()

	log := openTestLog(t)
	client := New(srv.URL, "test-key").WithActionLog(log)
	_, err := client.CreateAPIKey(context.Background(), "ci", "")
	if err == nil || strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("definitive API-key failure = %v, want direct validation error", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("definitive API-key failure made %d requests, want one", requests.Load())
	}

	entries, readErr := log.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if readErr != nil {
		t.Fatalf("read action log: %v", readErr)
	}
	if len(entries) != 1 || entries[0].Action != "create-api-key" || entries[0].Status != "failed" {
		t.Fatalf("definitive API-key action entry = %+v, want one failed create", entries)
	}
}

func TestConsoleWithoutActionLogIsNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"hasMore":false}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"dseq":"1","signTx":{"code":0,"transactionHash":"tx-1","rawLog":""}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	if _, err := c.CreateDeployment(context.Background(), actionLogTestSDL); err != nil {
		t.Fatalf("CreateDeployment without logger: %v", err)
	}
}

func TestAmbiguousDeploymentCreateIsLoggedPending(t *testing.T) {
	doc, err := sdl.Read([]byte(actionLogTestSDL))
	if err != nil {
		t.Fatalf("read SDL: %v", err)
	}
	version, err := doc.Version()
	if err != nil {
		t.Fatalf("SDL version: %v", err)
	}
	expectedHash := base64.StdEncoding.EncodeToString(version)

	var submitted atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if !submitted.Load() {
				_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"hasMore":false}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"deployments":[` +
				`{"deployment":{"id":{"dseq":"11"},"hash":"` + expectedHash + `"},"leases":[]},` +
				`{"deployment":{"id":{"dseq":"12"},"hash":"` + expectedHash + `"},"leases":[]}` +
				`],"pagination":{"hasMore":false}}}`))
		case http.MethodPost:
			submitted.Store(true)
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()

	l := openTestLog(t)
	c := New(srv.URL, "test-key").WithActionLog(l)
	_, err = c.CreateDeployment(context.Background(), actionLogTestSDL)
	if err == nil || !strings.Contains(err.Error(), "outcome unknown") || !strings.Contains(err.Error(), "akt console deployment list") {
		t.Fatalf("ambiguous create error = %v", err)
	}

	entries, readErr := l.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if len(entries) != 1 || entries[0].Action != "create-deployment" || entries[0].Status != "pending" {
		t.Fatalf("pending create entry = %+v", entries)
	}
	var params map[string]string
	if err := json.Unmarshal(entries[0].Params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params["versionHash"] != expectedHash {
		t.Errorf("versionHash = %q, want %q", params["versionHash"], expectedHash)
	}
}

func TestAmbiguousLeaseAndAPIKeyAreLoggedPending(t *testing.T) {
	leaseCtx, cancelLease := context.WithCancel(context.Background())
	defer cancelLease()

	var leasePosts atomic.Int32
	var keyPosts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/leases":
			leasePosts.Add(1)
			_, _ = w.Write([]byte(`{"data":{}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/api-keys":
			keyPosts.Add(1)
			_, _ = w.Write([]byte(`{"data":{}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/deployments/42":
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"}},"leases":[]}}`))
			if leasePosts.Load() > 0 {
				cancelLease()
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	l := openTestLog(t)
	c := New(srv.URL, "test-key").WithActionLog(l)

	_, err := c.CreateLease(leaseCtx, "manifest", []LeaseRequest{{
		DSeq: "42", GSeq: 1, OSeq: 1, Provider: "akash1provider",
	}})
	if err == nil || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("ambiguous lease error = %v", err)
	}

	if _, err := c.CreateAPIKey(context.Background(), "ci", ""); err == nil || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("ambiguous API-key error = %v", err)
	}

	entries, readErr := l.Read(actionlog.Filter{Type: actionlog.TypeConsole})
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if len(entries) != 2 {
		t.Fatalf("pending entries = %+v", entries)
	}
	for i, action := range []string{"create-api-key", "create-lease"} {
		if entries[i].Action != action || entries[i].Status != "pending" {
			t.Errorf("entry %d = %+v, want pending %s", i, entries[i], action)
		}
	}
	if leasePosts.Load() != 1 || keyPosts.Load() != 1 {
		t.Fatalf("POST counts lease=%d key=%d, want one each", leasePosts.Load(), keyPosts.Load())
	}
}
