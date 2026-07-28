package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// execTxCmdArgsOnly executes a tx command without wiring any client into the
// command context. Reaching the tx pipeline (connection, keyring, broadcast)
// would therefore panic or fail on client discovery — so a clean guard error
// proves the command failed fast during cobra argument validation.
func execTxCmdArgsOnly(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)

	return cmd.Execute()
}

// TestTxDeploymentCloseRequiresDSeq pins the fail-fast guard: with --dseq
// disabled for the positional-only trial, `tx deployment close` without a
// positional dseq must return the friendly guard error instead of building
// MsgCloseDeployment{DSeq: 0} and entering the sign/broadcast pipeline.
func TestTxDeploymentCloseRequiresDSeq(t *testing.T) {
	err := execTxCmdArgsOnly(t, GetTxDeploymentCloseCmd())
	require.ErrorIs(t, err, errDSeqRequired)
}

// TestTxDeploymentCloseRejectsZeroDSeq: an explicit positional "0" is just as
// invalid as a missing dseq and must hit the same guard.
func TestTxDeploymentCloseRejectsZeroDSeq(t *testing.T) {
	err := execTxCmdArgsOnly(t, GetTxDeploymentCloseCmd(), "0")
	require.ErrorIs(t, err, errDSeqRequired)
}

// TestTxDeploymentUpdateRequiresDSeq pins the fail-fast guard for
// `tx deployment update <sdl-file>` without a dseq, which previously queried
// deployment 0. The SDL file deliberately does not exist: the guard must fire
// before the file is ever read.
func TestTxDeploymentUpdateRequiresDSeq(t *testing.T) {
	err := execTxCmdArgsOnly(t, GetTxDeploymentUpdateCmd(), "does-not-exist.yaml")
	require.ErrorIs(t, err, errDSeqRequired)
}

// TestTxDeploymentUpdateRejectsZeroDSeq: an explicit positional "0" must hit
// the same guard as a missing dseq.
func TestTxDeploymentUpdateRejectsZeroDSeq(t *testing.T) {
	err := execTxCmdArgsOnly(t, GetTxDeploymentUpdateCmd(), "does-not-exist.yaml", "0")
	require.ErrorIs(t, err, errDSeqRequired)
}
