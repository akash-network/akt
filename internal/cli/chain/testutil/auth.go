package testutil

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/testutil"

	cli "pkg.akt.dev/akt/internal/cli/chain"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
)

func TxSignExec(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	cmd := cli.GetSignCommand()

	return ExecTestCLICmd(ctx, cctx, cmd, TestFlags().With(args...).WithChainID(cctx.ChainID)...)
}

func TxBroadcastExec(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetBroadcastCommand(), args...)
}

func TxEncodeExec(ctx context.Context, cctx client.Context, filename string, extraArgs ...string) (testutil.BufferWriter, error) {
	args := TestFlags().
		WithFlag(cflags.FlagKeyringBackend, keyring.BackendTest).
		With(filename).
		With(extraArgs...)

	return ExecTestCLICmd(ctx, cctx, cli.GetEncodeCommand(), args...)
}

func TxValidateSignaturesExec(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	args = TestFlags().With(args...).WithChainID(cctx.ChainID)

	return ExecTestCLICmd(ctx, cctx, cli.GetValidateSignaturesCommand(), args...)
}

func TxMultiSignExec(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	args = TestFlags().With(args...).WithChainID(cctx.ChainID)

	return ExecTestCLICmd(ctx, cctx, cli.GetAuthMultiSignCmd(), args...)
}

func TxSignBatchExec(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetSignBatchCommand(), args...)
}

func TxDecodeExec(ctx context.Context, cctx client.Context, encodedTx string, extraArgs ...string) (testutil.BufferWriter, error) {
	args := TestFlags().
		WithFlag(cflags.FlagKeyringBackend, keyring.BackendTest).
		With(encodedTx).
		With(extraArgs...)

	return ExecTestCLICmd(ctx, cctx, cli.GetDecodeCommand(), args...)
}

func ExecQueryAccount(ctx context.Context, cctx client.Context, address fmt.Stringer, extraArgs ...string) (testutil.BufferWriter, error) {
	args := TestFlags().
		With(address.String()).
		WithOutputJSON().
		With(extraArgs...)

	return ExecTestCLICmd(ctx, cctx, cli.GetQueryAuthAccountCmd(), args...)
}

func ExecSend(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxBankSendTxCmd(), args...)
}

// DONTCOVER
