package actionlog_test

import (
	"encoding/json"
	"fmt"
	"os"
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

func TestReadCollapsesTransactionRevisionsBeforeLimit(t *testing.T) {
	l := newTestLogger(t)

	submitted := time.Date(2026, time.August, 6, 18, 29, 4, 0, time.UTC)
	later := submitted.Add(time.Minute)

	for _, entry := range []actionlog.Entry{
		{
			Timestamp: submitted,
			Type:      actionlog.TypeTx,
			Action:    "deployment.MsgCreateDeployment",
			TxHash:    "ABC123",
			Status:    "pending",
		},
		{
			Timestamp: later,
			Type:      actionlog.TypeContext,
			Action:    "edit",
			Status:    "success",
		},
		{
			Timestamp:  submitted,
			Type:       actionlog.TypeTx,
			Action:     "deployment.MsgCreateDeployment",
			TxHash:     "ABC123",
			Height:     4694579,
			GasUsed:    138868,
			ResultCode: 0,
			Status:     "success",
		},
	} {
		if err := l.Log(entry); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	result, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("read returned %d logical entries, want 2: %+v", len(result), result)
	}
	if result[0].Action != "edit" {
		t.Errorf("newest entry = %q, want edit", result[0].Action)
	}
	confirmed := result[1]
	if confirmed.TxHash != "ABC123" || confirmed.Status != "success" || confirmed.Height != 4694579 || confirmed.GasUsed != 138868 {
		t.Errorf("transaction revision was not collapsed to its terminal state: %+v", confirmed)
	}

	limited, err := l.Read(actionlog.Filter{Limit: 1})
	if err != nil {
		t.Fatalf("Read limit: %v", err)
	}
	if len(limited) != 1 || limited[0].Action != "edit" {
		t.Fatalf("limit was applied before revision collapse: %+v", limited)
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

func TestLogRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.log")

	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	// Each entry carries ~1 MB of params, so the 10 MB threshold trips
	// after ~10 entries and rotation shifts the current file to .1.
	blob, _ := json.Marshal(map[string]string{"pad": strings.Repeat("x", 1<<20)})

	const total = 12
	for i := 0; i < total; i++ {
		if err := l.Log(actionlog.Entry{
			Type:   actionlog.TypeTx,
			Action: fmt.Sprintf("action-%d", i),
			Params: blob,
		}); err != nil {
			t.Fatalf("log %d: %v", i, err)
		}
	}

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotated file %s.1 to exist: %v", path, err)
	}

	// Read must span rotated files transparently, newest first.
	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != total {
		t.Fatalf("expected %d entries across rotated files, got %d", total, len(entries))
	}
	if entries[0].Action != fmt.Sprintf("action-%d", total-1) {
		t.Errorf("newest entry = %s, want action-%d", entries[0].Action, total-1)
	}
	if entries[total-1].Action != "action-0" {
		t.Errorf("oldest entry = %s, want action-0", entries[total-1].Action)
	}
}
