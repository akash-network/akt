package actionlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxLogSize      = 10 * 1024 * 1024 // 10 MB
	maxRotatedFiles = 5
)

// ActionType classifies an action log entry.
type ActionType string

const (
	TypeTx       ActionType = "tx"
	TypeQuery    ActionType = "query"
	TypeWorkflow ActionType = "workflow"
	TypeProvider ActionType = "provider"
	TypeContext  ActionType = "context"
	TypeConsole  ActionType = "console"
	TypeError    ActionType = "error"
)

// Entry is a single action log record, serialized as one JSON line.
type Entry struct {
	Timestamp  time.Time       `json:"ts"`
	Type       ActionType      `json:"type"`
	Action     string          `json:"action"`
	TxHash     string          `json:"tx_hash,omitempty"`
	Height     int64           `json:"height,omitempty"`
	GasUsed    int64           `json:"gas_used,omitempty"`
	ResultCode uint32          `json:"code,omitempty"`
	DSeq       uint64          `json:"dseq,omitempty"`
	GSeq       uint32          `json:"gseq,omitempty"`
	OSeq       uint32          `json:"oseq,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	Account    string          `json:"account,omitempty"`
	Params     json.RawMessage `json:"params,omitempty"`
	DurationMs int64           `json:"duration_ms,omitempty"`
	WorkflowID string          `json:"workflow_id,omitempty"`
	// Step is written unconditionally (SPEC §5.4): with omitempty the first
	// step of every workflow run, index 0, vanished from machine output, so
	// the entry recording where a run started looked like an entry with no
	// step at all.
	Step     int    `json:"step"`
	StepName string `json:"step_name,omitempty"`
	Error    string `json:"error,omitempty"`
	Status   string `json:"status,omitempty"`
}

// Filter defines criteria for reading log entries.
type Filter struct {
	Type    ActionType
	Since   time.Time
	Limit   int
	DSeq    uint64
	Account string
	// WorkflowID isolates the entries of a single workflow run, so two
	// interleaved runs of the same workflow can be read apart.
	WorkflowID string
}

// Logger is an append-only action log backed by a JSONL file.
// It is safe for concurrent use.
type Logger struct {
	mu   sync.Mutex
	path string
	file *os.File
}

// Open opens (or creates) an action log file at the given path.
func Open(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create action log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open action log %s: %w", path, err)
	}

	return &Logger{path: path, file: f}, nil
}

// Log appends an entry to the log. It automatically rotates if needed.
func (l *Logger) Log(entry Entry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal action log entry: %w", err)
	}

	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.maybeRotate(); err != nil {
		return err
	}

	if _, err := l.file.Write(data); err != nil {
		return fmt.Errorf("write action log: %w", err)
	}

	return nil
}

// Read reads entries from the log, most recent first, applying the filter.
func (l *Logger) Read(filter Filter) ([]Entry, error) {
	l.mu.Lock()
	// Flush before reading.
	_ = l.file.Sync()
	l.mu.Unlock()

	// Collect from rotated files (oldest first) then current file.
	var allEntries []Entry

	for i := maxRotatedFiles; i >= 1; i-- {
		rotated := fmt.Sprintf("%s.%d", l.path, i)
		entries, err := readLogFile(rotated, filter)
		if err != nil {
			continue // rotated file may not exist
		}

		allEntries = append(allEntries, entries...)
	}

	entries, err := readLogFile(l.path, filter)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	allEntries = append(allEntries, entries...)

	// Reverse to get most-recent-first.
	for i, j := 0, len(allEntries)-1; i < j; i, j = i+1, j-1 {
		allEntries[i], allEntries[j] = allEntries[j], allEntries[i]
	}

	if filter.Limit > 0 && len(allEntries) > filter.Limit {
		allEntries = allEntries[:filter.Limit]
	}

	return allEntries, nil
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}

	return nil
}

// maybeRotate checks if the current log exceeds maxLogSize and rotates if so.
// Caller must hold l.mu.
func (l *Logger) maybeRotate() error {
	info, err := l.file.Stat()
	if err != nil {
		//nolint:nilerr // a stat failure only means we cannot decide whether to
		// rotate; failing the caller here would turn an unreadable log file
		// into a failed user action, which is worse than a late rotation.
		return nil
	}

	if info.Size() < int64(maxLogSize) {
		return nil
	}

	// Close current file.
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close action log for rotation: %w", err)
	}

	// Shift rotated files: .4->.5, .3->.4, ..., .1->.2, current->.1
	for i := maxRotatedFiles - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", l.path, i)
		dst := fmt.Sprintf("%s.%d", l.path, i+1)
		_ = os.Rename(src, dst) // ok if src doesn't exist
	}

	_ = os.Rename(l.path, l.path+".1")

	// Open a fresh log.
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open new action log after rotation: %w", err)
	}

	l.file = f

	return nil
}

// readLogFile reads and filters entries from a single log file.
func readLogFile(path string, filter Filter) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var entries []Entry

	scanner := bufio.NewScanner(f)
	// A single entry may carry large params (SDL contents, raw messages);
	// allow lines up to the rotation threshold so Read never chokes on an
	// entry that Log accepted.
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}

		if !matchesFilter(e, filter) {
			continue
		}

		entries = append(entries, e)
	}

	return entries, scanner.Err()
}

func matchesFilter(e Entry, f Filter) bool {
	if f.Type != "" && e.Type != f.Type {
		return false
	}

	if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
		return false
	}

	if f.DSeq != 0 && e.DSeq != f.DSeq {
		return false
	}

	if f.Account != "" && e.Account != f.Account {
		return false
	}

	if f.WorkflowID != "" && e.WorkflowID != f.WorkflowID {
		return false
	}

	return true
}

// Export writes all entries to the given writer as JSONL.
func (l *Logger) Export(w io.Writer) error {
	entries, err := l.Read(Filter{})
	if err != nil {
		return err
	}

	// Write in chronological order (reverse of Read result).
	for i := len(entries) - 1; i >= 0; i-- {
		data, err := json.Marshal(entries[i])
		if err != nil {
			continue
		}

		if _, err := w.Write(append(data, '\n')); err != nil {
			return err
		}
	}

	return nil
}
