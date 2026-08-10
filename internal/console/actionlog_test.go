package console

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
		case r.Method == http.MethodPost && r.URL.Path == "/v1/deployments":
			_, _ = w.Write([]byte(`{"data":{"dseq":"12345","manifest":"m"}}`))
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

	if _, err := c.CreateDeployment(context.Background(), actionLogTestSDL, 5); err != nil {
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

func TestCloseAlreadyClosedRecordedAsSuccess(t *testing.T) {
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
	// Desired end state was reached, so the idempotent close logs as success.
	if entries[0].Action != "close-deployment" || entries[0].Status != "success" || entries[0].DSeq != 555 {
		t.Errorf("already-closed entry wrong: %+v", entries[0])
	}
}

func TestCloseValidation400RecordedAsFailed(t *testing.T) {
	// A 400 without already-closed semantics is a genuine failure: it must
	// not be logged as a successful close.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"cannot close: deployment has active leases pending settlement"}`))
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
		case r.Method == http.MethodPut && r.URL.Path == "/v1/wallet-settings":
			_, _ = w.Write([]byte(`{"data":{"autoReloadEnabled":true}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/v2/deployment-settings/42":
			_, _ = w.Write([]byte(`{"data":{"dseq":"42","autoTopUpEnabled":true}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/api-keys":
			_, _ = w.Write([]byte(`{"data":{"id":"k1","name":"n","apiKey":"secret"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/api-keys/k1":
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
	if _, err := c.SetDeploymentAutoTopUp(ctx, "42", true); err != nil {
		t.Fatalf("SetDeploymentAutoTopUp: %v", err)
	}
	if _, err := c.CreateAPIKey(ctx, "n", ""); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := c.DeleteAPIKey(ctx, "k1"); err != nil {
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

func TestConsoleWithoutActionLogIsNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"deployments":[],"pagination":{"hasMore":false}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"dseq":"1"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	if _, err := c.CreateDeployment(context.Background(), actionLogTestSDL, 5); err != nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = c.CreateDeployment(ctx, actionLogTestSDL, 5)
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
