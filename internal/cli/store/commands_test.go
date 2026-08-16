package store

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	rawbolt "go.etcd.io/bbolt"

	aktctx "pkg.akt.dev/akt/internal/context"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
)

func TestCommandsAssemblesAndRunsStatus(t *testing.T) {
	home := t.TempDir()
	cmd := Commands(
		func() string { return home },
		func() string { return "mainnet" },
		func() *aktctx.Manager { return nil },
	)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, stdout.String(), "Store")
	for _, name := range []string{"status", "sync", "export", "import"} {
		child, _, err := cmd.Find([]string{name})
		require.NoError(t, err)
		require.Equal(t, name, child.Name())
	}
}

func TestStatusPrettyOutputStripsANSIForNonTTY(t *testing.T) {
	home := t.TempDir()
	cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(nil)

	require.NoError(t, cmd.Execute())
	require.Contains(t, stdout.String(), "Store")
	require.Contains(t, stdout.String(), "Network Reconciliation")
	require.Contains(t, stdout.String(), "not yet run")
	require.Contains(t, stdout.String(), "akt store sync")
	require.NotContains(t, stdout.String(), "not synced")
	require.NotContains(t, stdout.String(), "\x1b[")
}

func TestStatusPrettyOutputHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	home := t.TempDir()
	cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(nil)

	require.NoError(t, cmd.Execute())
	require.Contains(t, stdout.String(), "Store")
	require.NotContains(t, stdout.String(), "\x1b[")
}

func TestStatusExplainsRecordsAndNetworkReconciliation(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()
	s, err := bbolt.OpenContext(ctx, home, "mainnet")
	require.NoError(t, err)
	require.NoError(t, s.PutLease(ctx, &sstore.LeaseRecord{
		ID:    sstore.LeaseID{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 1, Provider: "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl"},
		State: "active",
	}))
	require.NoError(t, s.PutBid(ctx, &sstore.BidRecord{
		ID:    sstore.BidID{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 1, Provider: "akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu"},
		State: "matched",
	}))
	require.NoError(t, s.PutBid(ctx, &sstore.BidRecord{
		ID:    sstore.BidID{Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 1, Provider: "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk"},
		State: "lost",
	}))
	require.NoError(t, s.Close())

	cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, cmd.Execute())
	output := stdout.String()
	require.Contains(t, output, "1 (1 active)")
	require.Contains(t, output, "2 (1 matched, 1 lost)")
	require.Contains(t, output, "Network Reconciliation")
	require.Contains(t, output, "not yet run")
	require.Contains(t, output, "akt store sync")
	require.NotContains(t, output, "not synced")
}

func TestStatusStructuredOutputReportsCompletedReconciliation(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	s, err := bbolt.OpenContext(ctx, home, "mainnet")
	require.NoError(t, err)
	require.NoError(t, s.PutDeployment(ctx, &sstore.DeploymentRecord{
		Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 1, State: "future_state",
	}))
	require.NoError(t, s.PutSyncState(ctx, &sstore.SyncState{
		LastBlockHeight: 12_345,
		LastSyncTime:    1_700_000_000,
		SchemaVersion:   1,
	}))
	require.NoError(t, s.Close())

	cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })
	cmd.Flags().String(flagdefs.FlagOutput, "pretty", "output format")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--output", "json"})
	require.NoError(t, cmd.Execute())

	var status struct {
		Context               string               `json:"context"`
		StorePath             string               `json:"storePath"`
		DatabaseBytes         int64                `json:"databaseBytes"`
		SchemaVersion         uint64               `json:"schemaVersion"`
		Records               *sstore.StoreStats   `json:"records"`
		NetworkReconciliation reconciliationStatus `json:"networkReconciliation"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &status))
	require.Equal(t, "mainnet", status.Context)
	require.Equal(t, aktctx.StoreDBPath(home, "mainnet"), status.StorePath)
	require.Positive(t, status.DatabaseBytes)
	require.Equal(t, uint64(1), status.SchemaVersion)
	require.Equal(t, int64(1), status.Records.Deployments)
	require.Zero(t, status.Records.ActiveDeployments)
	require.Zero(t, status.Records.ClosedDeployments)
	require.Equal(t, reconciliationStatus{
		Status:          "completed",
		LastBlockHeight: 12_345,
		LastRun:         "2023-11-14T22:13:20Z",
	}, status.NetworkReconciliation)
}

func TestStatusPrettyOutputReportsCompletedReconciliation(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	s, err := bbolt.OpenContext(ctx, home, "mainnet")
	require.NoError(t, err)
	require.NoError(t, s.PutSyncState(ctx, &sstore.SyncState{
		LastBlockHeight: 12_345,
		LastSyncTime:    1_700_000_000,
	}))
	require.NoError(t, s.Close())

	cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	require.Contains(t, stdout.String(), "12,345")
	require.Contains(t, stdout.String(), "2023-11-14T22:13:20Z")
	require.Contains(t, stdout.String(), "completed")
}

func TestStatusReportsOpenAndSyncStateFailures(t *testing.T) {
	t.Run("open", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, os.WriteFile(home, []byte("file"), 0o600))

		cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		require.ErrorContains(t, err, "open store")
	})

	t.Run("sync state", func(t *testing.T) {
		ctx := context.Background()
		home := t.TempDir()
		path := aktctx.StoreDBPath(home, "mainnet")
		s, err := bbolt.OpenContext(ctx, home, "mainnet")
		require.NoError(t, err)
		require.NoError(t, s.Close())
		raw, err := rawbolt.Open(path, 0o600, nil)
		require.NoError(t, err)
		require.NoError(t, raw.Update(func(tx *rawbolt.Tx) error {
			return tx.Bucket([]byte("sync")).Put([]byte("state"), []byte("not-json"))
		}))
		require.NoError(t, raw.Close())

		cmd := statusCmd(func() string { return home }, func() string { return "mainnet" })
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		err = cmd.Execute()
		require.ErrorContains(t, err, "read sync state")
	})
}

func TestStateCountAndReconciliationDescriptions(t *testing.T) {
	require.Equal(t, "0", formatStateCounts(0, stateCount{"active", 0}))
	require.Equal(t, "3 (2 active, 1 other)", formatStateCounts(3, stateCount{"active", 2}))
	require.Equal(t, "2 (2 other)", formatStateCounts(2))

	require.Equal(t, reconciliationStatus{
		Status:  "not_yet_run",
		Command: "akt store sync",
	}, describeReconciliation(nil))
	require.Equal(t, reconciliationStatus{
		Status:  "not_yet_run",
		Command: "akt store sync",
	}, describeReconciliation(&sstore.SyncState{}))
	require.Equal(t, reconciliationStatus{
		Status:          "completed",
		LastBlockHeight: 7,
		LastRun:         "1970-01-01T00:00:09Z",
	}, describeReconciliation(&sstore.SyncState{LastBlockHeight: 7, LastSyncTime: 9}))
}

func TestExportFilePreservesExistingBackupWhenExportFails(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	dbPath := aktctx.StoreDBPath(home, "mainnet")
	s, err := bbolt.OpenContext(ctx, home, "mainnet")
	require.NoError(t, err)
	require.NoError(t, s.Close())

	raw, err := rawbolt.Open(dbPath, 0o600, nil)
	require.NoError(t, err)
	require.NoError(t, raw.Update(func(tx *rawbolt.Tx) error {
		return tx.Bucket([]byte("deployments")).Put([]byte("corrupt"), []byte("not-json"))
	}))
	require.NoError(t, raw.Close())

	destination := filepath.Join(t.TempDir(), "backup.yaml")
	const original = "known-good-backup\n"
	require.NoError(t, os.WriteFile(destination, []byte(original), 0o600))

	cmd := exportCmd(func() string { return home }, func() string { return "mainnet" })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--file", destination})

	err = cmd.Execute()
	require.ErrorContains(t, err, "decode deployments record")

	contents, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	require.Equal(t, original, string(contents))

	temps, globErr := filepath.Glob(filepath.Join(filepath.Dir(destination), ".backup.yaml.tmp-*"))
	require.NoError(t, globErr)
	require.Empty(t, temps)
}

func TestExportFileAtomicallyReplacesExistingBackup(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	s, err := bbolt.OpenContext(ctx, home, "mainnet")
	require.NoError(t, err)
	require.NoError(t, s.PutDeployment(ctx, &sstore.DeploymentRecord{
		Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx",
		DSeq:  1,
		State: "active",
	}))
	require.NoError(t, s.Close())

	destination := filepath.Join(t.TempDir(), "backup.yaml")
	require.NoError(t, os.WriteFile(destination, []byte("old backup\n"), 0o600))

	cmd := exportCmd(func() string { return home }, func() string { return "mainnet" })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--file", destination})
	require.NoError(t, cmd.Execute())

	contents, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(contents, []byte("---\n")))
	require.Contains(t, string(contents), "owner: akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx")

	temps, err := filepath.Glob(filepath.Join(filepath.Dir(destination), ".backup.yaml.tmp-*"))
	require.NoError(t, err)
	require.Empty(t, temps)
}

func TestExportWritesSelectedFormatToCommandOutput(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	s, err := bbolt.OpenContext(ctx, home, "mainnet")
	require.NoError(t, err)
	require.NoError(t, s.PutDeployment(ctx, &sstore.DeploymentRecord{
		Owner: "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", DSeq: 1, State: "active",
	}))
	require.NoError(t, s.Close())

	cmd := exportCmd(func() string { return home }, func() string { return "mainnet" })
	cmd.Flags().String(flagdefs.FlagOutput, "pretty", "output format")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--output", "json"})
	require.NoError(t, cmd.Execute())

	var envelope bbolt.ExportEnvelope
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
	require.Equal(t, "mainnet", envelope.Context)
	require.Len(t, envelope.Deployments, 1)
	require.Equal(t, "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", envelope.Deployments[0].Owner)
}

func TestExportReportsCommandWriterAndStoreOpenFailures(t *testing.T) {
	t.Run("writer", func(t *testing.T) {
		home := t.TempDir()
		cmd := exportCmd(func() string { return home }, func() string { return "mainnet" })
		cmd.SetOut(&errorWriter{err: errors.New("pipe closed")})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.Execute()
		require.ErrorContains(t, err, "write YAML document start: pipe closed")
	})

	t.Run("open store", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, os.WriteFile(home, []byte("file"), 0o600))
		cmd := exportCmd(func() string { return home }, func() string { return "mainnet" })
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})

		err := cmd.Execute()
		require.ErrorContains(t, err, "open store")
	})
}

func TestExportFileReportsTemporaryCreationAndReplacementFailures(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	s, err := bbolt.OpenContext(ctx, home, "mainnet")
	require.NoError(t, err)
	require.NoError(t, s.Close())

	tests := []struct {
		name        string
		destination func(*testing.T) string
		wantError   string
	}{
		{
			name: "temporary creation",
			destination: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "backup.yaml")
			},
			wantError: "create temporary export",
		},
		{
			name: "replacement",
			destination: func(t *testing.T) string {
				return t.TempDir()
			},
			wantError: "replace export file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			destination := tc.destination(t)
			cmd := exportCmd(func() string { return home }, func() string { return "mainnet" })
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"--file", destination})

			err := cmd.Execute()
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}

func TestWriteStoreExportPropagatesFlushAndCloseFailures(t *testing.T) {
	ctx := context.Background()
	s, err := bbolt.Open(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	tests := []struct {
		name      string
		file      *exportTestFile
		wantError string
	}{
		{
			name:      "flush",
			file:      &exportTestFile{syncErr: errors.New("disk full")},
			wantError: "flush temporary export: disk full",
		},
		{
			name:      "close",
			file:      &exportTestFile{closeErr: errors.New("close failed")},
			wantError: "close temporary export: close failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := writeStoreExport(ctx, s, tc.file, sstore.FormatJSON, "mainnet")
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}

type exportTestFile struct {
	bytes.Buffer
	syncErr  error
	closeErr error
}

type errorWriter struct {
	err error
}

func (w *errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func (f *exportTestFile) Sync() error {
	return f.syncErr
}

func (f *exportTestFile) Close() error {
	return f.closeErr
}

func TestImportQuietSuppressesInformationalMessages(t *testing.T) {
	tests := []struct {
		name    string
		content string
		args    []string
	}{
		{
			name:    "dry run",
			content: `{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`,
			args:    []string{"--dry-run"},
		},
		{
			name:    "completed import",
			content: `{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(t.TempDir(), "backup.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o600))

			cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
			cmd.Flags().Bool(flagdefs.FlagQuiet, false, "quiet")
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(append([]string{path, "--quiet"}, tc.args...))

			require.NoError(t, cmd.Execute())
			require.Empty(t, stdout.String())
			require.Empty(t, stderr.String())
		})
	}
}

func TestImportMergeAndReplaceMutateStoreAtomically(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name         string
		args         []string
		wantExisting bool
	}{
		{name: "merge", wantExisting: true},
		{name: "replace", args: []string{"--replace", "--yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			s, err := bbolt.OpenContext(ctx, home, "mainnet")
			require.NoError(t, err)
			require.NoError(t, s.PutDeployment(ctx, &sstore.DeploymentRecord{
				Owner: "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh", DSeq: 1, State: "active",
			}))
			require.NoError(t, s.Close())

			path := filepath.Join(t.TempDir(), "backup.json")
			require.NoError(t, os.WriteFile(path, []byte(`{
  "version": 1,
  "schema_version": 1,
  "deployments": [
    {"owner": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f", "dseq": 2, "state": "closed"}
  ],
  "leases": [],
  "bids": []
}`), 0o600))

			cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
			var stderr bytes.Buffer
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&stderr)
			cmd.SetArgs(append([]string{path}, tc.args...))
			require.NoError(t, cmd.Execute())
			require.Equal(t, "Import complete.\n", stderr.String())

			s, err = bbolt.OpenContext(ctx, home, "mainnet")
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close() })
			existing, err := s.GetDeployment(ctx, "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh", 1)
			require.NoError(t, err)
			if tc.wantExisting {
				require.NotNil(t, existing)
			} else {
				require.Nil(t, existing)
			}
			replacement, err := s.GetDeployment(ctx, "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f", 2)
			require.NoError(t, err)
			require.NotNil(t, replacement)
			require.Equal(t, "closed", replacement.State)
		})
	}
}

func TestImportReportsFileStoreAndValidationFailures(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		cmd := importCmd(func() string { return t.TempDir() }, func() string { return "mainnet" })
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{filepath.Join(t.TempDir(), "missing.json")})

		err := cmd.Execute()
		require.ErrorContains(t, err, "open import file")
	})

	t.Run("open store", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, os.WriteFile(home, []byte("file"), 0o600))
		path := filepath.Join(t.TempDir(), "backup.json")
		require.NoError(t, os.WriteFile(path, []byte(
			`{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`,
		), 0o600))

		cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{path})

		err := cmd.Execute()
		require.ErrorContains(t, err, "open store")
	})

	t.Run("invalid input preserves store", func(t *testing.T) {
		ctx := context.Background()
		home := t.TempDir()
		s, err := bbolt.OpenContext(ctx, home, "mainnet")
		require.NoError(t, err)
		require.NoError(t, s.PutDeployment(ctx, &sstore.DeploymentRecord{
			Owner: "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh", DSeq: 1, State: "active",
		}))
		require.NoError(t, s.Close())
		path := filepath.Join(t.TempDir(), "backup.json")
		require.NoError(t, os.WriteFile(path, []byte(
			`{"version":1,"schema_version":1,"deployments":[null],"leases":[],"bids":[]}`,
		), 0o600))

		cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{path, "--replace", "--yes"})
		err = cmd.Execute()
		require.ErrorContains(t, err, "deployment 0 is null")

		s, err = bbolt.OpenContext(ctx, home, "mainnet")
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Close() })
		existing, err := s.GetDeployment(ctx, "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh", 1)
		require.NoError(t, err)
		require.NotNil(t, existing)
	})
}

func TestImportReplaceRequiresExplicitConfirmation(t *testing.T) {
	const owner = "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh"
	payload := `{"version":1,"schema_version":1,"deployments":[],"leases":[],"bids":[]}`

	for _, tc := range []struct {
		name          string
		args          []string
		input         string
		quiet         bool
		wantError     string
		wantRemaining bool
	}{
		{name: "accept", args: []string{"--replace"}, input: "yes\n"},
		{name: "decline", args: []string{"--replace"}, input: "no\n", wantRemaining: true},
		{name: "end of input", args: []string{"--replace"}, wantError: "read replacement confirmation", wantRemaining: true},
		{name: "yes flag", args: []string{"--replace", "--yes"}},
		{name: "quiet still prompts", args: []string{"--replace"}, input: "no\n", quiet: true, wantRemaining: true},
		{name: "merge false is not implicit replace", args: []string{"--merge=false"}, wantError: "requires the explicit --replace flag", wantRemaining: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			ctx := context.Background()
			s, err := bbolt.OpenContext(ctx, home, "mainnet")
			require.NoError(t, err)
			require.NoError(t, s.PutDeployment(ctx, &sstore.DeploymentRecord{Owner: owner, DSeq: 1, State: "active"}))
			require.NoError(t, s.Close())

			path := filepath.Join(t.TempDir(), "backup.json")
			require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
			cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
			cmd.Flags().Bool(flagdefs.FlagQuiet, false, "quiet")
			cmd.SetIn(strings.NewReader(tc.input))
			cmd.SetOut(&bytes.Buffer{})
			var stderr bytes.Buffer
			cmd.SetErr(&stderr)
			args := append([]string{path}, tc.args...)
			if tc.quiet {
				args = append(args, "--quiet")
			}
			cmd.SetArgs(args)

			err = cmd.Execute()
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
			if !tc.quiet && len(tc.args) > 0 && tc.args[0] == "--replace" && len(tc.args) == 1 {
				require.Contains(t, stderr.String(), "Replace every record")
			}
			if tc.quiet {
				require.Contains(t, stderr.String(), "Replace every record", "quiet must not suppress a safety prompt")
				require.NotContains(t, stderr.String(), "Import cancelled")
			}

			s, err = bbolt.OpenContext(ctx, home, "mainnet")
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close() })
			existing, err := s.GetDeployment(ctx, owner, 1)
			require.NoError(t, err)
			if tc.wantRemaining {
				require.NotNil(t, existing)
			} else {
				require.Nil(t, existing)
			}
		})
	}
}

func TestImportDryRunValidatesInput(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(t.TempDir(), "backup.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "version": 2,
  "schema_version": 1,
  "deployments": [],
  "leases": [],
  "bids": []
}`), 0o600))

	cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{path, "--dry-run"})

	err := cmd.Execute()
	require.ErrorContains(t, err, "unsupported export version")
}

func TestImportDryRunDoesNotCreateSelectedContextStore(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(t.TempDir(), "backup.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "version": 1,
  "schema_version": 1,
  "deployments": [],
  "leases": [],
  "bids": []
}`), 0o600))

	cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{path, "--dry-run"})
	require.NoError(t, cmd.Execute())

	_, err := os.Stat(aktctx.ContextDir(home, "mainnet"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(aktctx.StoreDBPath(home, "mainnet"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestImportDryRunDoesNotMutateStore(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	s, err := bbolt.OpenContext(ctx, home, "mainnet")
	require.NoError(t, err)
	require.NoError(t, s.PutDeployment(ctx, &sstore.DeploymentRecord{
		Owner: "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh",
		DSeq:  1,
		State: "active",
	}))
	require.NoError(t, s.Close())

	path := filepath.Join(t.TempDir(), "backup.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "version": 1,
  "schema_version": 1,
  "deployments": [
    {"owner": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f", "dseq": 2, "state": "closed"}
  ],
  "leases": [],
  "bids": []
}`), 0o600))

	cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{path, "--replace", "--dry-run"})
	require.NoError(t, cmd.Execute())

	s, err = bbolt.OpenContext(ctx, home, "mainnet")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	existing, err := s.GetDeployment(ctx, "akash1errud3wyc0pvrs9lh67mewa6hxut0d44pfj6vh", 1)
	require.NoError(t, err)
	require.NotNil(t, existing)
	replacement, err := s.GetDeployment(ctx, "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f", 2)
	require.NoError(t, err)
	require.Nil(t, replacement)
}
