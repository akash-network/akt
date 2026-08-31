package output

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckedWriterReusesExistingBoundary(t *testing.T) {
	checked := NewCheckedWriter(io.Discard)
	require.Same(t, checked, NewCheckedWriter(checked))
}

func TestCheckedWriterCompletePreservesRenderAndWriteErrors(t *testing.T) {
	renderErr := errors.New("render failed")
	writeErr := errors.New("write failed")
	checked := NewCheckedWriter(outputBoundaryWriter{err: writeErr})

	_, err := checked.Write([]byte("result"))
	require.ErrorIs(t, err, writeErr)

	completed := checked.Complete(renderErr)
	require.ErrorIs(t, completed, renderErr)
	require.ErrorIs(t, completed, writeErr)
}

func TestTerminalAwareWriterPropagatesDestinationFailures(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	wantErr := errors.New("destination failed")

	tests := []struct {
		name string
		w    io.Writer
		want error
	}{
		{name: "hard error", w: outputBoundaryWriter{err: wantErr}, want: wantErr},
		{name: "short write", w: outputBoundaryWriter{short: true}, want: io.ErrShortWrite},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := TerminalAwareWriter(test.w).Write([]byte("\x1b[31mfailed\x1b[0m"))
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestTerminalAwareWriterStripsANSIAndReportsSourceBytes(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	styled := []byte("\x1b[31mfailed\x1b[0m")
	var destination bytes.Buffer

	n, err := TerminalAwareWriter(&destination).Write(styled)
	require.NoError(t, err)
	require.Equal(t, len(styled), n)
	require.Equal(t, "failed", destination.String())
}

func TestTerminalAwareWriterStripsANSIOutsideTTYWithoutNoColor(t *testing.T) {
	value, present := os.LookupEnv("NO_COLOR")
	require.NoError(t, os.Unsetenv("NO_COLOR"))
	t.Cleanup(func() {
		if present {
			_ = os.Setenv("NO_COLOR", value)
			return
		}
		_ = os.Unsetenv("NO_COLOR")
	})

	styled := []byte("\x1b[31mfailed\x1b[0m")
	var destination bytes.Buffer

	n, err := TerminalAwareWriter(&destination).Write(styled)
	require.NoError(t, err)
	require.Equal(t, len(styled), n)
	require.Equal(t, "failed", destination.String())
}

type outputBoundaryWriter struct {
	err   error
	short bool
}

func (w outputBoundaryWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.short && len(p) > 0 {
		return len(p) - 1, nil
	}

	return len(p), nil
}
