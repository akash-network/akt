package testutil

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"

	cli "pkg.akt.dev/akt/internal/cli/chain"
)

// ExecDeploymentCreate is used for testing create deployment tx
func ExecDeploymentCreate(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxDeploymentCreateCmd(), args...)
}

// ExecDeploymentUpdate is used for testing update deployment tx
func ExecDeploymentUpdate(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxDeploymentUpdateCmd(), args...)
}

// ExecDeploymentClose is used for testing close deployment tx
// requires a positional dseq and --fees
func ExecDeploymentClose(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxDeploymentCloseCmd(), args...)
}

// ExecDeploymentGroupClose is used for testing close deployment group tx
// requires positional dseq/gseq and --fees
func ExecDeploymentGroupClose(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxDeploymentGroupCloseCmd(), args...)
}

// ExecQueryDeployment is used for testing deployment query (single or list based on flags/args)
func ExecQueryDeployment(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetQueryDeploymentCmds(), args...)
}

// ExecQueryGroup is used for testing group query
func ExecQueryGroup(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetQueryDeploymentGroupCmd(), args...)
}
