package actionlog_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/actionlog"
)

// TestOpenCreatesParentDirectories covers the MkdirAll branch. The action log
// path is derived from a context name, and the context directory may not exist
// yet the first time something is recorded (e.g. `context create` logging its
// own creation).
func TestOpenCreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "actions.log")

	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	if err := l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "x"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file was not created: %v", err)
	}
}

// TestOpenFailsWhenPathIsUnusable covers the OpenFile error branch. Callers
// (recordContextAction, the console client) swallow the error and degrade to
// no logging; the error must actually be returned so they can.
func TestOpenFailsWhenPathIsUnusable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.log")

	// A directory where the log file should be.
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if l, err := actionlog.Open(path); err == nil {
		_ = l.Close()
		t.Fatal("opening a directory as a log file must fail")
	}
}

// TestLogAfterCloseReportsWriteFailure covers the write-error branch of Log.
// The logger is shared across a command's lifetime; a write after the file is
// closed must surface rather than silently dropping an audit record.
func TestLogAfterCloseReportsWriteFailure(t *testing.T) {
	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "after-close"}); err == nil {
		t.Error("logging after Close must return an error")
	} else if !strings.Contains(err.Error(), "action log") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestCloseIsIdempotentEnoughForDefer pins that a second Close does not panic.
// Every caller defers Close, and several also close explicitly on the success
// path.
func TestCloseIsIdempotentEnoughForDefer(t *testing.T) {
	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// The second close reports the already-closed file; it must not panic.
	_ = l.Close()
}

// TestReadSkipsMalformedLines covers readLogFile's tolerance for corruption.
// The log is append-only JSONL written by concurrent processes; a torn write
// must not make the whole log unreadable, which is what `akt context log`
// depends on.
func TestReadSkipsMalformedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.log")

	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "good-1"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	_ = l.Close()

	// Append a torn line, a blank line, and a valid one.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, err := f.WriteString("{\"type\":\"tx\",\"action\":\"tor\n\n"); err != nil {
		t.Fatalf("write torn line: %v", err)
	}
	_ = f.Close()

	l2, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("reopen logger: %v", err)
	}
	defer l2.Close()

	if err := l2.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: "good-2"}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := l2.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	var actions []string
	for _, e := range entries {
		actions = append(actions, e.Action)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %v, want the two well-formed records", actions)
	}
	if actions[0] != "good-2" || actions[1] != "good-1" {
		t.Errorf("entries = %v, want newest-first [good-2 good-1]", actions)
	}
}

// TestReadOnMissingFileIsNotAnError covers Read's os.IsNotExist tolerance: a
// context whose log has been deleted (or never written) must read as empty.
func TestReadOnMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.log")

	l, err := actionlog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("Read on a removed log must not error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %v, want none", entries)
	}
}

// TestExportWriteErrorPropagates covers Export's writer-error branch. Export
// backs `akt context log --export`; a full disk or a closed pipe must be
// reported rather than producing a silently truncated audit file.
func TestExportWriteErrorPropagates(t *testing.T) {
	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	for _, a := range []string{"one", "two"} {
		if err := l.Log(actionlog.Entry{Type: actionlog.TypeTx, Action: a}); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	if err := l.Export(failingWriter{}); err == nil {
		t.Fatal("a failing writer must be reported")
	}

	// Sanity: the same export succeeds against a working writer, in
	// chronological order (the reverse of Read).
	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := buf.String(); strings.Index(got, "one") > strings.Index(got, "two") {
		t.Errorf("export must be chronological, got %q", got)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// TestFilterCombinesCriteria covers matchesFilter with several criteria at
// once — the shape `akt context log --type tx --since 1h` produces. An OR
// instead of an AND here would show entries the user filtered out.
func TestFilterCombinesCriteria(t *testing.T) {
	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	entries := []actionlog.Entry{
		{Type: actionlog.TypeTx, Action: "match", DSeq: 42, Account: "alice"},
		{Type: actionlog.TypeTx, Action: "wrong-dseq", DSeq: 7, Account: "alice"},
		{Type: actionlog.TypeTx, Action: "wrong-account", DSeq: 42, Account: "bob"},
		{Type: actionlog.TypeQuery, Action: "wrong-type", DSeq: 42, Account: "alice"},
	}
	for _, e := range entries {
		if err := l.Log(e); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	got, err := l.Read(actionlog.Filter{Type: actionlog.TypeTx, DSeq: 42, Account: "alice"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 || got[0].Action != "match" {
		t.Errorf("filtered entries = %+v, want only the fully-matching one", got)
	}
}

// TestLargeParamsSurviveRoundTrip pins the scanner buffer sizing: an entry
// carrying an SDL body is far larger than bufio's 64 KiB default line limit,
// and Read must not choke on a record that Log accepted.
func TestLargeParamsSurviveRoundTrip(t *testing.T) {
	l, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer l.Close()

	big := strings.Repeat("a", 200*1024)
	if err := l.Log(actionlog.Entry{
		Type:   actionlog.TypeTx,
		Action: "create-deployment",
		Params: []byte(`{"sdl":"` + big + `"}`),
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := l.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (a large entry must survive Read)", len(entries))
	}
	if !strings.Contains(string(entries[0].Params), big) {
		t.Error("large params were truncated on read")
	}
}
