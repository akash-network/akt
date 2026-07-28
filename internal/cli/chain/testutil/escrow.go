package testutil

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"

	cli "pkg.akt.dev/akt/internal/cli/chain"
)

// ExecEscrowDeposit is used for testing deposits into escrow account
func ExecEscrowDeposit(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxEscrowDeposit(), args...)
}
