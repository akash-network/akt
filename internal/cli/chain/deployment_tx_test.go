package cli

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestRawDeploymentTransactionsRejectConsoleReferencesBeforeClients(t *testing.T) {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "environment", value: "env:\n      - TOKEN=ac-secret://PRIVATE_NAME"},
		{name: "registry", value: "credentials:\n      host: registry.example.test\n      username: account\n      password: ac-secret://PRIVATE_NAME"},
	} {
		for _, command := range []struct {
			name  string
			build func() *cobra.Command
			args  []string
		}{
			{name: "create", build: GetTxDeploymentCreateCmd},
			{name: "update", build: GetTxDeploymentUpdateCmd, args: []string{"42"}},
		} {
			t.Run(command.name+"/"+field.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "deployment.yaml")
				data := "version: '2.0'\nservices:\n  web:\n    image: nginx:1.27\n    " + field.value + "\n"
				require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
				cmd := command.build()
				// No client is installed. Reaching any query, certificate lookup,
				// signer, or broadcaster would panic instead of returning this guard.
				cmd.PersistentPreRunE = func(*cobra.Command, []string) error { return nil }
				err := execTxCmdArgsOnly(t, cmd, append([]string{path}, command.args...)...)
				require.ErrorContains(t, err, "Console workflow rail")
				require.NotContains(t, err.Error(), "PRIVATE_NAME")
			})
		}
	}
}

func TestChainSDLReadFailures(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		missing       bool
	}{{"missing", "", true}, {"malformed", "services: [", false}} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "deployment.yaml")
			if !tc.missing {
				require.NoError(t, os.WriteFile(path, []byte(tc.content), 0600))
			}
			_, err := readChainDeploymentSDL(path)
			require.Error(t, err)
		})
	}
}

func TestChainSDLReadDelegatesSchemaValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployment.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: invalid\nservices: {}\n"), 0600))
	document, err := readChainDeploymentSDL(path)
	require.Error(t, err)
	require.Nil(t, document)
}
