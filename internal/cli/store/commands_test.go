package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestImportQuietSuppressesInformationalMessages(t *testing.T) {
	tests := []struct {
		name    string
		content string
		args    []string
	}{
		{
			name:    "dry run",
			content: "not parsed during a dry run",
			args:    []string{"--dry-run"},
		},
		{
			name:    "completed import",
			content: `{"version":1,"deployments":[],"leases":[],"bids":[]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(t.TempDir(), "backup.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o600))

			cmd := importCmd(func() string { return home }, func() string { return "mainnet" })
			cmd.Flags().Bool("quiet", false, "quiet")
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
