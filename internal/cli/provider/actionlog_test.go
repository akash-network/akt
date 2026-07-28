package provider

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
)

func TestRecordProviderAction(t *testing.T) {
	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	defer l.Close()

	ctx := cliutil.WithActionLog(context.Background(), l)

	recordProviderAction(ctx, "send-manifest", "akash1prov", 42, nil)
	recordProviderAction(ctx, "migrate-hostnames", "akash1prov", 42, errors.New("boom"))

	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Newest first.
	if entries[0].Action != "migrate-hostnames" || entries[0].Status != "failed" || entries[0].Error != "boom" {
		t.Errorf("failure entry wrong: %+v", entries[0])
	}
	if entries[1].Action != "send-manifest" || entries[1].Status != "success" {
		t.Errorf("success entry wrong: %+v", entries[1])
	}
	if entries[1].Type != actionlog.TypeProvider || entries[1].Provider != "akash1prov" || entries[1].DSeq != 42 {
		t.Errorf("provider/dseq fields wrong: %+v", entries[1])
	}
}

func TestRecordProviderActionNoLogger(t *testing.T) {
	// Must be a no-op without a logger in context.
	recordProviderAction(context.Background(), "send-manifest", "akash1prov", 1, nil)
}
