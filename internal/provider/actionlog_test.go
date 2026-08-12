package provider

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
)

func TestRecordActionWritesSuccessAndFailure(t *testing.T) {
	logger, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	ctx := cliutil.WithActionLog(context.Background(), logger)

	RecordAction(ctx, "send-manifest", "akash1provider", 41, nil)
	RecordAction(ctx, "send-manifest", "akash1provider", 42, errors.New("gateway refused"))

	entries, err := logger.Read(actionlog.Filter{Type: actionlog.TypeProvider})
	if err != nil {
		t.Fatalf("read action log: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2: %+v", len(entries), entries)
	}
	if got := entries[0]; got.Action != "send-manifest" || got.Provider != "akash1provider" || got.DSeq != 42 || got.Status != "failed" || got.Error != "gateway refused" {
		t.Errorf("failed entry = %+v", got)
	}
	if got := entries[1]; got.Action != "send-manifest" || got.Provider != "akash1provider" || got.DSeq != 41 || got.Status != "success" || got.Error != "" {
		t.Errorf("successful entry = %+v", got)
	}
}

func TestRecordActionWithoutLoggerIsNoop(t *testing.T) {
	RecordAction(context.Background(), "send-manifest", "akash1provider", 41, nil)
}
