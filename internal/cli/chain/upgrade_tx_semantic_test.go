package cli_test

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	chain "pkg.akt.dev/akt/internal/cli/chain"
	chaintest "pkg.akt.dev/akt/internal/cli/chain/testutil"
	flagdefs "pkg.akt.dev/akt/internal/flags"
)

func executeGeneratedUpgrade(
	t *testing.T,
	f generatedTxFixture,
	cmd *cobra.Command,
	args ...string,
) ([]byte, error) {
	t.Helper()
	callArgs := chaintest.TestFlags().
		With(args...).
		WithFrom(f.from.String()).
		WithGenerateOnly().
		WithGas(200000).
		WithChainID(f.cctx.ChainID).
		WithOutputJSON()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out, err := chaintest.ExecTestCLICmd(ctx, f.cctx, cmd, callArgs...)
	if out == nil {
		return nil, err
	}
	return append([]byte(nil), out.Bytes()...), err
}

func TestUpgradeTransactionsReadCanonicalFlags(t *testing.T) {
	f := newGeneratedTxFixture(t)
	common := []string{
		"--" + flagdefs.FlagTitle + "=Canonical upgrade",
		"--" + flagdefs.FlagSummary + "=Exercise canonical upgrade flags",
		"--" + flagdefs.FlagDeposit + "=7uakt",
	}

	t.Run("software upgrade", func(t *testing.T) {
		args := append([]string{"v2.0.0"}, common...)
		args = append(args,
			"--"+flagdefs.FlagUpgradeHeight+"=12345",
			"--"+flagdefs.FlagUpgradeInfo+"={}",
			"--"+flagdefs.FlagNoValidate+"=true",
		)
		output, err := executeGeneratedUpgrade(t, f, chain.NewCmdSubmitUpgradeProposal(), args...)
		require.NoError(t, err, "command output:\n%s", output)
	})

	t.Run("validation options", func(t *testing.T) {
		args := append([]string{"invalid-info"}, common...)
		args = append(args,
			"--"+flagdefs.FlagUpgradeHeight+"=12346",
			"--"+flagdefs.FlagUpgradeInfo+"=not-json",
			"--"+flagdefs.FlagDaemonName+"=akt",
			"--"+flagdefs.FlagNoChecksumRequired+"=true",
		)
		_, err := executeGeneratedUpgrade(t, f, chain.NewCmdSubmitUpgradeProposal(), args...)
		require.Error(t, err)
	})

	t.Run("cancel", func(t *testing.T) {
		output, err := executeGeneratedUpgrade(t, f, chain.NewCmdSubmitCancelUpgradeProposal(), common...)
		require.NoError(t, err, "command output:\n%s", output)
	})
}
