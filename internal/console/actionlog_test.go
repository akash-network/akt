package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"pkg.akt.dev/akt/internal/actionlog"
)

func TestConsoleMutationsRecordedInActionLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/deployments":
			_, _ = w.Write([]byte(`{"dseq":"12345","manifest":"m"}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	defer l.Close()

	c := New(srv.URL, "test-key").WithActionLog(l)

	if _, err := c.CreateDeployment(context.Background(), "sdl", 5); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	if err := c.CloseDeployment(context.Background(), "999"); err == nil {
		t.Fatal("expected close of unknown dseq to fail")
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

func TestConsoleWithoutActionLogIsNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"dseq":"1"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	if _, err := c.CreateDeployment(context.Background(), "sdl", 5); err != nil {
		t.Fatalf("CreateDeployment without logger: %v", err)
	}
}
