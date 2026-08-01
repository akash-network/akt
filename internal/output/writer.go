package output

import (
	"io"
	"os"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

type ansiStrippingWriter struct {
	io.Writer
}

func (w ansiStrippingWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(w.Writer, ansi.Strip(string(p))); err != nil {
		return 0, err
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
