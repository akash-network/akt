package store

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/store/bbolt"
	syncpkg "pkg.akt.dev/akt/internal/sync"
)

type storeOutputBoundaryWriter struct {
	err   error
	short bool
}

func (w storeOutputBoundaryWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.short && len(p) > 0 {
		return len(p) - 1, nil
	}

	return len(p), nil
}

type storeMatchingWriter struct {
	err    error
	match  string
	writes []string
}

func (w *storeMatchingWriter) Write(p []byte) (int, error) {
	w.writes = append(w.writes, string(p))
	if strings.Contains(string(p), w.match) {
		return 0, w.err
	}

	return len(p), nil
}

func TestStoreStatusAndSyncPropagateDestinationFailures(t *testing.T) {
	hardErr := errors.New("store destination failed")
	failures := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "hard error", writer: storeOutputBoundaryWriter{err: hardErr}, want: hardErr},
		{name: "short write", writer: storeOutputBoundaryWriter{short: true}, want: io.ErrShortWrite},
	}

	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			t.Run("status", func(t *testing.T) {
				home := t.TempDir()
				cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })
				cmd.SetOut(failure.writer)
				cmd.SetErr(io.Discard)

				if err := cmd.Execute(); !errors.Is(err, failure.want) {
					t.Fatalf("status error = %v, want %v", err, failure.want)
				}
			})

			t.Run("sync result", func(t *testing.T) {
				cmd := &cobra.Command{}
				cmd.SetOut(failure.writer)
				cmd.SetErr(io.Discard)
				err := renderSyncResult(cmd, []string{"akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"}, syncpkg.ReconcileStats{Height: 17})
				if !errors.Is(err, failure.want) {
					t.Fatalf("sync result error = %v, want %v", err, failure.want)
				}
			})
		})
	}
}

func TestStoreImportNoticesPropagateDestinationFailures(t *testing.T) {
	const owner = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"
	const payload = `{"version":1,"schema_version":1,"deployments":[{"owner":"` + owner + `","dseq":7,"state":"active"}],"leases":[],"bids":[]}`
	hardErr := errors.New("store notice destination failed")
	failures := []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "hard error", writer: storeOutputBoundaryWriter{err: hardErr}, want: hardErr},
		{name: "short write", writer: storeOutputBoundaryWriter{short: true}, want: io.ErrShortWrite},
	}

	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			for _, dryRun := range []bool{true, false} {
				name := "complete"
				if dryRun {
					name = "dry run"
				}
				t.Run(name, func(t *testing.T) {
					home := t.TempDir()
					path := filepath.Join(t.TempDir(), "backup.json")
					if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
						t.Fatalf("write backup: %v", err)
					}

					cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
					cmd.Flags().Bool("quiet", false, "test quiet mode")
					cmd.SetOut(io.Discard)
					cmd.SetErr(failure.writer)
					args := []string{path}
					if dryRun {
						args = append(args, "--dry-run")
					}
					cmd.SetArgs(args)

					err := cmd.Execute()
					if !errors.Is(err, failure.want) {
						t.Fatalf("import error = %v, want %v", err, failure.want)
					}

					if dryRun {
						if _, statErr := os.Stat(aktctx.StoreDBPath(home, "mainnet")); !errors.Is(statErr, os.ErrNotExist) {
							t.Fatalf("dry run created selected store: %v", statErr)
						}
						return
					}

					s, openErr := bbolt.OpenContext(context.Background(), home, "mainnet")
					if openErr != nil {
						t.Fatalf("open imported store: %v", openErr)
					}
					t.Cleanup(func() { _ = s.Close() })
					record, getErr := s.GetDeployment(context.Background(), owner, 7)
					if getErr != nil {
						t.Fatalf("get imported deployment: %v", getErr)
					}
					if record == nil || record.State != "active" {
						t.Fatalf("imported deployment = %#v, want active record", record)
					}
				})
			}
		})
	}
}

func TestStoreReplacementPromptPropagatesDestinationFailure(t *testing.T) {
	writeErr := errors.New("replacement prompt destination failed")
	cmd := &cobra.Command{}
	cmd.SetErr(storeOutputBoundaryWriter{err: writeErr})
	cmd.SetIn(strings.NewReader("yes\n"))

	confirmed, err := confirmStoreReplacement(cmd, "mainnet")
	if confirmed {
		t.Fatal("a failed confirmation prompt must not confirm replacement")
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("confirmation error = %v, want destination error", err)
	}
	if err == nil || !strings.Contains(err.Error(), "write replacement confirmation") {
		t.Fatalf("confirmation error = %v, want boundary context", err)
	}
}

func TestStoreImportCancellationPropagatesDestinationFailure(t *testing.T) {
	const owner = "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx"
	const payload = `{"version":1,"schema_version":1,"deployments":[{"owner":"` + owner + `","dseq":7,"state":"active"}],"leases":[],"bids":[]}`

	home := t.TempDir()
	path := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	writeErr := errors.New("cancellation notice destination failed")
	diagnostics := &storeMatchingWriter{err: writeErr, match: "Import cancelled."}
	cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
	cmd.Flags().Bool("quiet", false, "test quiet mode")
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(diagnostics)
	cmd.SetArgs([]string{path, "--replace"})

	if err := cmd.Execute(); !errors.Is(err, writeErr) {
		t.Fatalf("cancel import error = %v, want destination error", err)
	}
	written := strings.Join(diagnostics.writes, "")
	if !strings.Contains(written, "Replace every record") || !strings.Contains(written, "Import cancelled.") {
		t.Fatalf("diagnostics = %q, want prompt then cancellation notice", written)
	}
	if _, err := os.Stat(aktctx.StoreDBPath(home, "mainnet")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled import created or changed the store: %v", err)
	}
}
