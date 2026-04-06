package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/term"
)

// Nerd Font test glyphs from different PUA ranges.
const (
	// Powerline right-pointing triangle (U+E0B0) — present in Powerline and Nerd Fonts.
	glyphPowerline = "\ue0b0"
	// Font Awesome star (U+F005) — present in Nerd Fonts only, not in Powerline-only fonts.
	glyphFontAwesome = "\uf005"
)

// Probe timeout for each DSR query. Terminals that don't support cursor
// position reporting will time out after this duration.
const probeTimeout = 100 * time.Millisecond

var (
	checkOnce   sync.Once
	checkResult error
)

// CheckNerdFont probes the terminal to verify that a Nerd Font is active.
//
// It renders test glyphs from the Powerline (U+E0B0) and Font Awesome (U+F005)
// PUA ranges and measures cursor advance via ANSI Device Status Report (\033[6n).
//
// The check is performed at most once per process (cached via sync.Once).
// It is skipped when stdout is not a terminal (piped/non-interactive).
//
// Returns nil if a Nerd Font is detected or detection cannot be performed.
// Returns a descriptive error with install/upgrade instructions otherwise.
func CheckNerdFont() error {
	checkOnce.Do(func() {
		checkResult = doCheckNerdFont()
	})
	return checkResult
}

func doCheckNerdFont() error {
	// Skip when stderr or stdin is not a terminal (piped output, CI, etc.).
	outFd := int(os.Stderr.Fd())
	inFd := int(os.Stdin.Fd())
	if !term.IsTerminal(outFd) || !term.IsTerminal(inFd) {
		return nil
	}

	plWidth, plErr := probeGlyphWidth(inFd, glyphPowerline)
	faWidth, faErr := probeGlyphWidth(inFd, glyphFontAwesome)

	// If both probes failed (terminal doesn't support DSR), we can't detect.
	// Proceed with a warning rather than blocking the user.
	if plErr != nil && faErr != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not detect terminal font (DSR not supported). Nerd Font is recommended.")
		return nil
	}

	// Both glyphs render at 1 cell — Nerd Font detected.
	if plWidth == 1 && faWidth == 1 {
		return nil
	}

	// Powerline OK but Font Awesome missing — Powerline-only font.
	if plWidth == 1 && faWidth != 1 {
		return errors.New(`Powerline font detected, but Nerd Font required

Your terminal appears to have a Powerline font, but akt requires a full
Nerd Font which includes additional icon sets (Font Awesome, Material
Design, etc.) used throughout the interface.

  Upgrade:  https://www.nerdfonts.com/
  Tip:      Most Nerd Fonts are patched versions of popular fonts —
            look for the Nerd Font variant of your current font.

If your font is already a Nerd Font and this check is wrong, use --skip-font-check`)
	}

	// Neither glyph renders correctly — no special font installed.
	return errors.New(`Nerd Font not detected

TUI mode and pretty output require a Nerd Font to be installed and
configured in your terminal emulator. Nerd Fonts include Powerline
symbols and extended icons used throughout the interface.

  Install:  https://www.nerdfonts.com/
  Popular:  "JetBrainsMono Nerd Font", "FiraCode Nerd Font", "Hack Nerd Font"

After installing, set it as your terminal's font and restart your terminal.

If your font is already a Nerd Font and this check is wrong, use --skip-font-check`)
}

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
