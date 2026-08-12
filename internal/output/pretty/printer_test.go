package pretty

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/gogoproto/proto"
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

func TestPrintQueryResultPropagatesCommandWriterFailures(t *testing.T) {
	wantErr := errors.New("command stdout failed")
	protoCodec := codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	pool := &stakingtypes.Pool{
		BondedTokens:    math.NewInt(1_000_000),
		NotBondedTokens: math.NewInt(2_000_000),
	}
	fallback := &banktypes.QueryParamsResponse{}

	operations := []struct {
		name   string
		format string
		msg    proto.Message
	}{
		{name: "registered pretty", format: cflags.OutputPretty, msg: pool},
		{name: "pretty fallback JSON", format: cflags.OutputPretty, msg: fallback},
		{name: "explicit JSON", format: cflags.OutputJSON, msg: pool},
		{name: "explicit YAML", format: cflags.OutputYAML, msg: pool},
	}
	failures := []struct {
		name string
		w    io.Writer
		want error
	}{
		{name: "hard error", w: prettyBoundaryWriter{err: wantErr}, want: wantErr},
		{name: "short write", w: prettyBoundaryWriter{short: true}, want: io.ErrShortWrite},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			for _, failure := range failures {
				t.Run(failure.name, func(t *testing.T) {
					cmd := &cobra.Command{}
					cmd.Flags().String(cflags.FlagOutput, operation.format, "")
					cmd.SetOut(failure.w)

					var wrongDestination bytes.Buffer
					cctx := sdkclient.Context{Codec: protoCodec}.WithOutput(&wrongDestination)
					err := PrintQueryResult(cmd, cctx, operation.msg)
					require.ErrorIs(t, err, failure.want)
					require.Empty(t, wrongDestination.String(), "client context output must be replaced by command output")
				})
			}
		})
	}
}

func TestPrintQueryResultAnyUsesCommandWriter(t *testing.T) {
	wantErr := errors.New("command stdout failed")
	for _, failure := range []struct {
		name string
		w    io.Writer
		want error
	}{
		{name: "hard error", w: prettyBoundaryWriter{err: wantErr}, want: wantErr},
		{name: "short write", w: prettyBoundaryWriter{short: true}, want: io.ErrShortWrite},
	} {
		t.Run(failure.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetOut(failure.w)

			var wrongDestination bytes.Buffer
			cctx := sdkclient.Context{LegacyAmino: codec.NewLegacyAmino()}.WithOutput(&wrongDestination)
			err := PrintQueryResultAny(cmd, cctx, map[string]string{"status": "ready"})
			require.ErrorIs(t, err, failure.want)
			require.Empty(t, wrongDestination.String(), "client context output must be replaced by command output")
		})
	}
}

type prettyBoundaryWriter struct {
	err   error
	short bool
}

func (w prettyBoundaryWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.short && len(p) > 0 {
		return len(p) - 1, nil
	}

	return len(p), nil
}
