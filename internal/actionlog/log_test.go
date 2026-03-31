package actionlog_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pkg.akt.dev/akt/internal/actionlog"
)

func newTestLogger(t *testing.T) *actionlog.Logger {
	t.Helper()

	path := filepath.Join(t.TempDir(), "actions.log")

	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = l.Close() })

	return l
}

func TestLogAndRead(t *testing.T) {
	l := newTestLogger(t)

	entries := []actionlog.Entry{
		{Type: actionlog.TypeTx, Action: "bank.send", Account: "alice", Status: "success"},
		{Type: actionlog.TypeQuery, Action: "deployment.deployments", DSeq: 123, DurationMs: 45},
		{Type: actionlog.TypeTx, Action: "deployment.close", DSeq: 123, Status: "success"},
	}

	for _, e := range entries {
		if err := l.Log(e); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	result, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}

	// Most recent first.
	if result[0].Action != "deployment.close" {
		t.Errorf("first entry should be most recent, got %q", result[0].Action)
	}
}

func TestLogTimestampAutoSet(t *testing.T) {
	l := newTestLogger(t)

	before := time.Now().UTC()
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "test"})
	after := time.Now().UTC()

	entries, _ := l.Read(actionlog.Filter{})
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	ts := entries[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not between %v and %v", ts, before, after)
	}
}

func TestFilterByType(t *testing.T) {
	l := newTestLogger(t)

	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "tx1"})
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeQuery, Action: "q1"})
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "tx2"})

	result, _ := l.Read(actionlog.Filter{Type: actionlog.TypeTx})
	if len(result) != 2 {
		t.Errorf("expected 2 tx entries, got %d", len(result))
	}
}

func TestFilterByDSeq(t *testing.T) {
	l := newTestLogger(t)

	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "a", DSeq: 100})
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "b", DSeq: 200})
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "c", DSeq: 100})

	result, _ := l.Read(actionlog.Filter{DSeq: 100})
	if len(result) != 2 {
		t.Errorf("expected 2 entries for dseq=100, got %d", len(result))
	}
}

func TestFilterBySince(t *testing.T) {
	l := newTestLogger(t)

	old := time.Now().UTC().Add(-1 * time.Hour)
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "old", Timestamp: old})
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "new"}) // auto-timestamp = now

	cutoff := time.Now().UTC().Add(-30 * time.Minute)
	result, _ := l.Read(actionlog.Filter{Since: cutoff})
	if len(result) != 1 {
		t.Errorf("expected 1 entry since 30m ago, got %d", len(result))
	}

	if result[0].Action != "new" {
		t.Error("expected the newer entry")
	}
}

func TestFilterByLimit(t *testing.T) {
	l := newTestLogger(t)

	for i := 0; i < 10; i++ {
		_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "tx"})
	}

	result, _ := l.Read(actionlog.Filter{Limit: 3})
	if len(result) != 3 {
		t.Errorf("expected 3 entries with limit, got %d", len(result))
	}
}

func TestFilterByAccount(t *testing.T) {
	l := newTestLogger(t)

	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "a", Account: "alice"})
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "b", Account: "bob"})
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "c", Account: "alice"})

	result, _ := l.Read(actionlog.Filter{Account: "alice"})
	if len(result) != 2 {
		t.Errorf("expected 2 entries for alice, got %d", len(result))
	}
}

func TestExport(t *testing.T) {
	l := newTestLogger(t)

	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "first"})
	_ = l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "second"})

	var buf strings.Builder
	if err := l.Export(&buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 JSONL lines, got %d", len(lines))
	}

	// Export is chronological (oldest first).
	if !strings.Contains(lines[0], "first") {
		t.Error("first exported line should be the oldest entry")
	}
}

func TestEmptyRead(t *testing.T) {
	l := newTestLogger(t)

	result, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected 0 entries from empty log, got %d", len(result))
	}
}
