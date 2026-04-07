package glyphs

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"

	"golang.org/x/term"
)

// probeTimeout is the deadline for each DSR cursor-position query.
// Terminals that don't support DSR will time out after this duration.
const probeTimeout = 100 * time.Millisecond

// probeGlyphWidth renders a single glyph and measures how many terminal
// columns the cursor advances. It uses ANSI DSR (Device Status Report,
// \033[6n) to query cursor position before and after rendering.
//
// Returns the column delta (expected: 1 for a properly rendered single-width
// glyph), or an error if the terminal doesn't respond within the timeout.
func probeGlyphWidth(inFd int, glyph string) (int, error) {
	// Put terminal in raw mode to read DSR responses.
	oldState, err := term.MakeRaw(inFd)
	if err != nil {
		return 0, fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(inFd, oldState) //nolint:errcheck

	w := os.Stderr

	// Save cursor position.
	fmt.Fprint(w, "\033[s")

	// Query initial cursor column.
	col1, err := queryCursorCol(inFd, w, probeTimeout)
	if err != nil {
		fmt.Fprint(w, "\033[u") // restore cursor
		return 0, fmt.Errorf("initial cursor query failed: %w", err)
	}

	// Render the test glyph.
	fmt.Fprint(w, glyph)

	// Query cursor column after rendering.
	col2, err := queryCursorCol(inFd, w, probeTimeout)

	// Restore cursor and erase test output regardless of error.
	fmt.Fprint(w, "\033[u\033[K")

	if err != nil {
		return 0, fmt.Errorf("post-glyph cursor query failed: %w", err)
	}

	return col2 - col1, nil
}

// queryCursorCol sends a DSR query (\033[6n) and parses the terminal's
// response (\033[row;colR) to extract the current cursor column.
func queryCursorCol(inFd int, w *os.File, timeout time.Duration) (int, error) {
	// Drain any pending input before sending DSR.
	drainInput(inFd, 10*time.Millisecond)

	// Send Device Status Report request.
	fmt.Fprint(w, "\033[6n")

	// Read response with timeout.
	buf, err := readDSRResponse(inFd, timeout)
	if err != nil {
		return 0, err
	}

	// Parse response: \033[row;colR
	return parseDSRResponse(buf)
}

// readResult is the result of a single read operation, sent over a channel.
type readResult struct {
	data []byte
	err  error
}

// drainInput reads and discards any pending bytes on the input fd.
func drainInput(fd int, timeout time.Duration) {
	ch := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 128)
		n, err := os.NewFile(uintptr(fd), "stdin").Read(buf)
		if n > 0 {
			ch <- readResult{data: buf[:n]}
		} else {
			ch <- readResult{err: err}
		}
	}()

	select {
	case <-ch:
		// Drained whatever was pending.
	case <-time.After(timeout):
		// Nothing pending, that's fine.
	}
}

// readDSRResponse reads from the fd until it gets a complete DSR response
// (ending with 'R') or the timeout expires. Uses a goroutine for non-blocking
// reads since terminal fds don't support SetReadDeadline.
func readDSRResponse(fd int, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	var result bytes.Buffer
	f := os.NewFile(uintptr(fd), "stdin")

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		ch := make(chan readResult, 1)
		go func() {
			buf := make([]byte, 32)
			n, err := f.Read(buf)
			if n > 0 {
				ch <- readResult{data: buf[:n]}
			} else {
				ch <- readResult{err: err}
			}
		}()

		select {
		case r := <-ch:
			if r.err != nil {
				return nil, fmt.Errorf("read error: %w", r.err)
			}
			result.Write(r.data)
			// DSR response ends with 'R'.
			if bytes.ContainsRune(result.Bytes(), 'R') {
				return result.Bytes(), nil
			}
		case <-time.After(remaining):
			// Timeout — check if we already have a complete response.
			if result.Len() > 0 && bytes.ContainsRune(result.Bytes(), 'R') {
				return result.Bytes(), nil
			}
			return nil, fmt.Errorf("timeout waiting for DSR response")
		}
	}

	if result.Len() > 0 && bytes.ContainsRune(result.Bytes(), 'R') {
		return result.Bytes(), nil
	}
	return nil, fmt.Errorf("timeout waiting for DSR response")
}

// parseDSRResponse parses a DSR response of the form \033[row;colR
// and returns the column number.
func parseDSRResponse(buf []byte) (int, error) {
	// Find the ESC[ prefix.
	idx := bytes.Index(buf, []byte("\033["))
	if idx < 0 {
		return 0, fmt.Errorf("no ESC[ in DSR response: %q", buf)
	}
	buf = buf[idx+2:] // skip ESC[

	// Find the trailing 'R'.
	rIdx := bytes.IndexByte(buf, 'R')
	if rIdx < 0 {
		return 0, fmt.Errorf("no trailing R in DSR response: %q", buf)
	}
	buf = buf[:rIdx] // "row;col"

	// Split on ';'.
	parts := bytes.SplitN(buf, []byte(";"), 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected DSR format: %q", buf)
	}

	col, err := strconv.Atoi(string(parts[1]))
	if err != nil {
		return 0, fmt.Errorf("invalid column in DSR response: %w", err)
	}

	return col, nil
}
