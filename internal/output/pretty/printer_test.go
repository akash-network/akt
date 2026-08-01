package pretty

import (
	"bytes"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

func TestPrintQueryResultUsesPlainCommandWriterOutsideTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	cmd := &cobra.Command{Use: "pool"}
	cflags.AddQueryFlagsToCmd(cmd)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	pool := &stakingtypes.Pool{
		BondedTokens:    math.NewInt(1_000_000),
		NotBondedTokens: math.NewInt(2_000_000),
	}
	require.NoError(t, PrintQueryResult(cmd, sdkclient.Context{}, pool))
	require.Contains(t, stdout.String(), "Staking Pool")
	require.NotContains(t, stdout.String(), "\x1b[")
}

func TestPrintTxResultUsesPlainCommandWriterOutsideTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	cmd := &cobra.Command{Use: "send"}
	cflags.AddQueryFlagsToCmd(cmd)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	response := &sdk.TxResponse{TxHash: "ABC123", Height: 10, GasUsed: 20, GasWanted: 30}
	require.NoError(t, PrintTxResult(cmd, sdkclient.Context{}, response))
	require.Contains(t, stdout.String(), "Transaction")
	require.NotContains(t, stdout.String(), "\x1b[")
}
