package output

import (
	"errors"
	"io"
	"os"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// CheckedWriter retains the first destination failure and turns a nil-error
// prefix write into io.ErrShortWrite. It lets void render helpers keep their
// signatures without allowing them to report a successful command after
// stdout failed.
type CheckedWriter struct {
	destination io.Writer
	err         error
}

func NewCheckedWriter(destination io.Writer) *CheckedWriter {
	if checked, ok := destination.(*CheckedWriter); ok {
		return checked
	}

	return &CheckedWriter{destination: destination}
}

// NewCheckedTerminalWriter detects terminal capabilities from the original
// destination before adding write accounting. Passing a CheckedWriter to
// TerminalAwareWriter hides an underlying *os.File and incorrectly disables
// styling on a real terminal.
func NewCheckedTerminalWriter(destination io.Writer) *CheckedWriter {
	return NewCheckedWriter(TerminalAwareWriter(destination))
}

func (w *CheckedWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	n, err := w.destination.Write(p)
	if err != nil {
		w.err = err
		return n, err
	}
	if n != len(p) {
		w.err = io.ErrShortWrite
		return n, w.err
	}

	return n, nil
}

func (w *CheckedWriter) Err() error {
	return w.err
}

// Complete preserves both a renderer failure and an independent destination
// failure for errors.Is callers.
func (w *CheckedWriter) Complete(renderErr error) error {
	if renderErr == nil {
		return w.err
	}
	if w.err == nil || errors.Is(renderErr, w.err) {
		return renderErr
	}
	return errors.Join(renderErr, w.err)
}

type ansiStrippingWriter struct {
	io.Writer
}

func (w ansiStrippingWriter) Write(p []byte) (int, error) {
	stripped := ansi.Strip(string(p))
	n, err := io.WriteString(w.Writer, stripped)
	if err != nil {
		return 0, err
	}
	if n != len(stripped) {
		return 0, io.ErrShortWrite
	}

	return len(p), nil
}

// TerminalAwareWriter preserves styling on an interactive terminal and strips
// it from redirected output, in-memory writers, and explicit no-color sessions.
func TerminalAwareWriter(w io.Writer) io.Writer {
	_, noColor := os.LookupEnv("NO_COLOR")
	file, isFile := w.(*os.File)
	if noColor || !isFile || !term.IsTerminal(int(file.Fd())) {
		return ansiStrippingWriter{Writer: w}
	}

	return w
}
