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

	return ExecTestCLICmd(ctx, cctx, cmd, withChainID(args, cctx.ChainID)...)
}

func TxBroadcastExec(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetBroadcastCommand(), args...)
}

func TxEncodeExec(ctx context.Context, cctx client.Context, filename string, extraArgs ...string) (testutil.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--%s=%s", cflags.FlagKeyringBackend, keyring.BackendTest),
		filename,
	}

	return ExecTestCLICmd(ctx, cctx, cli.GetEncodeCommand(), append(args, extraArgs...)...)
}

func TxValidateSignaturesExec(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetValidateSignaturesCommand(), withChainID(args, cctx.ChainID)...)
}

func TxMultiSignExec(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetAuthMultiSignCmd(), withChainID(args, cctx.ChainID)...)
}

func withChainID(args []string, chainID string) []string {
	result := append([]string(nil), args...)
	return append(result, fmt.Sprintf("--%s=%s", cflags.FlagChainID, chainID))
}

func TxSignBatchExec(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetSignBatchCommand(), args...)
}

func TxDecodeExec(ctx context.Context, cctx client.Context, encodedTx string, extraArgs ...string) (testutil.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--%s=%s", cflags.FlagKeyringBackend, keyring.BackendTest),
		encodedTx,
	}

	return ExecTestCLICmd(ctx, cctx, cli.GetDecodeCommand(), append(args, extraArgs...)...)
}

func ExecQueryAccount(ctx context.Context, cctx client.Context, address fmt.Stringer, extraArgs ...string) (testutil.BufferWriter, error) {
	args := []string{address.String(), fmt.Sprintf("--%s=json", cflags.FlagOutput)}

	return ExecTestCLICmd(ctx, cctx, cli.GetQueryAuthAccountCmd(), append(args, extraArgs...)...)
}

func ExecSend(ctx context.Context, cctx client.Context, args ...string) (testutil.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxBankSendTxCmd(), args...)
}

// DONTCOVER
