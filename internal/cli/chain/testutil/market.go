package testutil

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"

	"pkg.akt.dev/akt/internal/cli/chain"
)

// ExecCreateBid is used for testing create bid tx
func ExecCreateBid(ctx context.Context, cctx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxMarketBidCreateCmd(), extraArgs...)
}

// ExecCloseBid is used for testing close bid tx
func ExecCloseBid(ctx context.Context, cctx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxMarketBidCloseCmd(), extraArgs...)
}

// ExecCreateLease is used for creating a lease
func ExecCreateLease(ctx context.Context, cctx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxMarketLeaseCreateCmd(), extraArgs...)
}

// ExecCloseLease is used for testing close order tx
func ExecCloseLease(ctx context.Context, cctx client.Context, extraArgs ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetTxMarketLeaseCloseCmd(), extraArgs...)
}

// ExecQueryOrder is used for testing order query (single or list based on flags)
func ExecQueryOrder(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetQueryMarketOrderCmd(), args...)
}

// ExecQueryBid is used for testing bid query (single or list based on flags)
func ExecQueryBid(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetQueryMarketBidCmd(), args...)
}

// ExecQueryLease is used for testing lease query (single or list based on flags)
func ExecQueryLease(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return ExecTestCLICmd(ctx, cctx, cli.GetQueryMarketLeaseCmd(), args...)
}
