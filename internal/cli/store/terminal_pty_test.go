//go:build !windows

package store

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	syncpkg "pkg.akt.dev/akt/internal/sync"
)

func TestStorePrettyBoundariesPreserveANSIOnTerminal(t *testing.T) {
	unsetStoreNoColor(t)

	t.Run("status", func(t *testing.T) {
		output := captureStorePTY(t, func(terminal *os.File) error {
			cmd := statusCmd(func() string { return t.TempDir() }, func() string { return "mainnet" })
			cmd.SetOut(terminal)
			cmd.SetErr(io.Discard)
			return cmd.Execute()
		})
		require.Contains(t, ansi.Strip(output), "Store")
		require.Contains(t, output, "\x1b[", "store status must retain ANSI styling on a terminal")
	})

	t.Run("sync", func(t *testing.T) {
		output := captureStorePTY(t, func(terminal *os.File) error {
			cmd := &cobra.Command{}
			cmd.SetOut(terminal)
			return renderSyncResult(cmd, []string{
				"akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
			}, syncpkg.ReconcileStats{Height: 17})
		})
		require.Contains(t, ansi.Strip(output), "Store Sync")
		require.Contains(t, output, "\x1b[", "store sync must retain ANSI styling on a terminal")
	})
}

func captureStorePTY(t *testing.T, run func(*os.File) error) string {
	t.Helper()

	master, terminal, err := pty.Open()
	require.NoError(t, err)
	defer master.Close()
	defer terminal.Close()

	const endMarker = "__AKT_PTY_CAPTURE_COMPLETE__"
	runDone := make(chan error, 1)
	go func() {
		runErr := run(terminal)
		if _, markerErr := terminal.WriteString(endMarker); runErr == nil {
			runErr = markerErr
		}
		runDone <- runErr
	}()

	var output bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, readErr := master.Read(buf)
		if n > 0 {
			_, _ = output.Write(buf[:n])
			if markerIndex := bytes.Index(output.Bytes(), []byte(endMarker)); markerIndex >= 0 {
				require.NoError(t, <-runDone)
				return output.String()[:markerIndex]
			}
		}
		if readErr != nil {
			t.Fatalf("read pseudo-terminal output: %v", readErr)
		}
	}
}

func unsetStoreNoColor(t *testing.T) {
	t.Helper()

	value, present := os.LookupEnv("NO_COLOR")
	require.NoError(t, os.Unsetenv("NO_COLOR"))
	t.Cleanup(func() {
		if present {
			_ = os.Setenv("NO_COLOR", value)
			return
		}
		_ = os.Unsetenv("NO_COLOR")
	})
}
