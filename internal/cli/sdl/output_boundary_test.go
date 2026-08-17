package sdl

import (
	"errors"
	"io"
	"testing"

	"pkg.akt.dev/akt/internal/output"
)

type sdlOutputBoundaryWriter struct {
	err   error
	short bool
}

func (w sdlOutputBoundaryWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.short && len(p) > 0 {
		return len(p) - 1, nil
	}

	return len(p), nil
}

func TestPlainSDLCommandsPropagateDestinationFailures(t *testing.T) {
	validPath := writeFixture(t, validSDL)
	invalidPath := writeFixture(t, "{{{ this is not valid SDL")
	hardErr := errors.New("SDL destination failed")

	failures := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "hard error", writer: sdlOutputBoundaryWriter{err: hardErr}, want: hardErr},
		{name: "short write", writer: sdlOutputBoundaryWriter{short: true}, want: io.ErrShortWrite},
	}
	tests := []struct {
		name     string
		args     []string
		toStderr bool
	}{
		{name: "scaffold table", args: []string{"scaffolds"}},
		{name: "generated SDL", args: []string{"init", "web"}},
		{name: "valid result", args: []string{"validate", validPath}},
		{name: "invalid diagnostics", args: []string{"validate", invalidPath}, toStderr: true},
	}

	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					cmd := Commands()
					cmd.PersistentFlags().VarP(output.NewFormatFlag("pretty"), "output", "o", "test output")
					cmd.SilenceErrors = true
					cmd.SilenceUsage = true
					cmd.SetOut(io.Discard)
					cmd.SetErr(io.Discard)
					if test.toStderr {
						cmd.SetErr(failure.writer)
					} else {
						cmd.SetOut(failure.writer)
					}
					cmd.SetArgs(test.args)

					err := cmd.Execute()
					if !errors.Is(err, failure.want) {
						t.Fatalf("error = %v, want destination failure %v", err, failure.want)
					}
				})
			}
		})
	}
}
