package actionlog_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"pkg.akt.dev/akt/internal/actionlog"
)

// TestOversizedEntryIsRejectedBeforeAppend keeps one caller from defeating the
// file-rotation budget with a single record. Rejection must not rotate or leave
// a partial JSON line behind.
func TestOversizedEntryIsRejectedBeforeAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.log")
	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	marker := strings.Repeat("x", 11<<20)
	if err := l.Log(actionlog.Entry{
		Type:   actionlog.TypeTx,
		Action: "create-deployment",
		Params: json.RawMessage(`{"sdl":"` + marker + `"}`),
	}); err == nil {
		t.Fatal("Log accepted an entry larger than the rotation budget")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("oversized rejection appended %d bytes", len(data))
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("oversized rejection rotated the log: %v", err)
	}

	if err := l.Log(actionlog.Entry{Type: actionlog.TypeQuery, Action: "checkpoint"}); err != nil {
		t.Fatalf("Log after rejection: %v", err)
	}
	entries, err := l.Read(actionlog.Filter{})
	if err != nil || len(entries) != 1 || entries[0].Action != "checkpoint" {
		t.Fatalf("logger unusable after rejection: entries=%+v err=%v", entries, err)
	}
}

// TestReadRejectsExternallyOversizedRow proves the read boundary is independent
// of writer correctness. A locally modified, unterminated row must fail instead
// of growing memory until EOF or being reported as an empty audit history.
func TestReadRejectsExternallyOversizedRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.log")
	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 11<<20)), 0o600); err != nil {
		t.Fatalf("write hostile row: %v", err)
	}

	if _, err := l.Read(actionlog.Filter{}); err == nil {
		t.Fatal("Read accepted an externally oversized row")
	}
}

// TestInvalidParamsAreRejectedWithoutAppending prevents malformed caller data
// from corrupting the append-only stream. json.RawMessage is intentionally an
// untrusted boundary: it can contain bytes that json.Marshal must reject.
func TestInvalidParamsAreRejectedWithoutAppending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.log")
	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	if err := l.Log(actionlog.Entry{
		Type:   actionlog.TypeQuery,
		Action: "bad-params",
		Params: json.RawMessage(`{"unterminated"`),
	}); err == nil {
		t.Fatal("invalid JSON params must be rejected")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("rejected entry appended %d bytes", len(data))
	}
}

// TestOpenReportsParentPathCollision distinguishes a bad log filename from a
// parent that cannot be created. The latter is common after a damaged context
// directory and must remain diagnosable.
func TestOpenReportsParentPathCollision(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "contexts")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	_, err := actionlog.Open(filepath.Join(blocker, "prod", "actions.log"))
	if err == nil {
		t.Fatal("opening below a regular file must fail")
	}
	if !strings.Contains(err.Error(), "create action log directory") {
		t.Fatalf("error = %q, want directory context", err)
	}
}

// TestWorkflowFilterSeparatesInterleavedRuns pins the run-ID filter used when
// two executions of the same workflow write adjacent action-log entries.
func TestWorkflowFilterSeparatesInterleavedRuns(t *testing.T) {
	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	for _, entry := range []actionlog.Entry{
		{Type: actionlog.TypeWorkflow, Action: "deploy", WorkflowID: "run-a", Step: 0},
		{Type: actionlog.TypeWorkflow, Action: "deploy", WorkflowID: "run-b", Step: 0},
		{Type: actionlog.TypeWorkflow, Action: "deploy", WorkflowID: "run-a", Step: 1},
	} {
		if err := l.Log(entry); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	entries, err := l.Read(actionlog.Filter{WorkflowID: "run-a"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 2 || entries[0].Step != 1 || entries[1].Step != 0 {
		t.Fatalf("filtered entries = %+v, want run-a steps [1 0]", entries)
	}
	for _, entry := range entries {
		if entry.WorkflowID != "run-a" {
			t.Fatalf("filter leaked workflow %q", entry.WorkflowID)
		}
	}
}

// TestReadAndExportSurfaceUnreadableCurrentLog ensures a damaged current log
// is not mistaken for an empty audit trail. Rotated-file failures are tolerated
// because a generation may simply not exist; failure of the active path is a
// different contract and must reach both readers.
func TestReadAndExportSurfaceUnreadableCurrentLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.log")
	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove log: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("replace log with directory: %v", err)
	}

	if _, err := l.Read(actionlog.Filter{}); err == nil {
		t.Fatal("Read treated an unreadable active log as empty")
	}
	if err := l.Export(&strings.Builder{}); err == nil {
		t.Fatal("Export swallowed the active-log read error")
	}
}

// TestReadSurfacesDamagedRotatedGeneration distinguishes a missing historical
// generation from one that exists but cannot be decoded as a regular file.
func TestReadSurfacesDamagedRotatedGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.log")
	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	if err := l.Log(actionlog.Entry{Type: actionlog.TypeContext, Action: "create"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := os.Mkdir(path+".1", 0o700); err != nil {
		t.Fatalf("make damaged generation: %v", err)
	}

	if _, err := l.Read(actionlog.Filter{}); err == nil {
		t.Fatal("Read silently omitted an unreadable rotated generation")
	}
}

func TestConcurrentLogWritesRemainComplete(t *testing.T) {
	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	const writers = 64
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs <- l.Log(actionlog.Entry{
				Type:   actionlog.TypeProvider,
				Action: fmt.Sprintf("send-manifest-%d", index),
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Log: %v", err)
		}
	}

	entries, err := l.Read(actionlog.Filter{Type: actionlog.TypeProvider})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != writers {
		t.Fatalf("entries = %d, want %d complete writes", len(entries), writers)
	}
	seen := make(map[string]bool, writers)
	for _, entry := range entries {
		if seen[entry.Action] {
			t.Fatalf("duplicate action %q", entry.Action)
		}
		seen[entry.Action] = true
	}
}
