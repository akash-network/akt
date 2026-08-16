package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	aktctx "pkg.akt.dev/akt/internal/context"
)

type mcpOutputErrorWriter struct {
	err error
}

func (w mcpOutputErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type mcpOutputShortWriter struct{}

func (mcpOutputShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func runCanceledMCP(t *testing.T, stderr io.Writer, quiet bool) (string, error) {
	t.Helper()

	cmd := mcpCmd(func() *aktctx.Manager { return nil })
	cmd.Flags().Bool(flagdefs.FlagQuiet, false, "test quiet mode")
	if err := cmd.Flags().Set(flagdefs.FlagConsoleAPIKey, "test-api-key"); err != nil {
		t.Fatalf("set Console API key: %v", err)
	}
	if quiet {
		if err := cmd.Flags().Set(flagdefs.FlagQuiet, "true"); err != nil {
			t.Fatalf("set quiet: %v", err)
		}
	}

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(stderr)
	cctx := sdkclient.Context{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(context.WithValue(ctx, sdkclient.ClientContextKey, &cctx))

	err := cmd.RunE(cmd, nil)
	return stdout.String(), err
}

func TestMCPStartupBannerUsesCobraStderrAndHonorsQuiet(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		name := "diagnostic stream"
		if quiet {
			name = "quiet"
		}
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			stdout, err := runCanceledMCP(t, &stderr, quiet)
			if err != nil {
				t.Fatalf("run MCP command: %v", err)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}

			want := "akt mcp: starting stdio server (node=, chain=, mode=read-only)\n"
			if quiet {
				want = ""
			}
			if stderr.String() != want {
				t.Fatalf("stderr = %q, want %q", stderr.String(), want)
			}
		})
	}
}

func TestMCPStartupBannerPropagatesDestinationFailures(t *testing.T) {
	hardErr := errors.New("MCP diagnostic destination failed")
	tests := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "hard error", writer: mcpOutputErrorWriter{err: hardErr}, want: hardErr},
		{name: "short write", writer: mcpOutputShortWriter{}, want: io.ErrShortWrite},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runCanceledMCP(t, tc.writer, false)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}
