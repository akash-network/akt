package store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
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
		ID:    sstore.LeaseID{Owner: "akash1owner", DSeq: 1, Provider: "akash1provider"},
		State: "active",
	}))
	require.NoError(t, s.PutBid(ctx, &sstore.BidRecord{
		ID:    sstore.BidID{Owner: "akash1owner", DSeq: 1, Provider: "akash1winner"},
		State: "matched",
	}))
	require.NoError(t, s.PutBid(ctx, &sstore.BidRecord{
		ID:    sstore.BidID{Owner: "akash1owner", DSeq: 1, Provider: "akash1loser"},
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
