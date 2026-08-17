package cli

import (
	"context"
	"errors"
	"testing"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	rpcclientmock "github.com/cometbft/cometbft/rpc/client/mock"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	node "github.com/cosmos/cosmos-sdk/client/grpc/node"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	flagdefs "pkg.akt.dev/akt/internal/flags"
)

type gasPriceRPC struct {
	rpcclientmock.Client
	minimum string
	err     error
	calls   int
	path    string
}

func newGasPriceRPC(minimum string) *gasPriceRPC {
	return &gasPriceRPC{
		Client:  rpcclientmock.New(),
		minimum: minimum,
	}
}

func (client *gasPriceRPC) ABCIQueryWithOptions(
	_ context.Context,
	path string,
	_ cmbytes.HexBytes,
	_ rpcclient.ABCIQueryOptions,
) (*coretypes.ResultABCIQuery, error) {
	client.calls++
	client.path = path
	if client.err != nil {
		return nil, client.err
	}

	payload, err := (&node.ConfigResponse{MinimumGasPrice: client.minimum}).Marshal()
	if err != nil {
		return nil, err
	}

	return &coretypes.ResultABCIQuery{
		Response: abci.ResponseQuery{Value: payload},
	}, nil
}

func gasPriceTestCommand(t *testing.T, prices string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{}
	cflags.AddTxFlagsToCmd(cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set(flagdefs.FlagGasPrices, prices))
	return cmd
}

func requireGasPricesEqual(t *testing.T, want, got string) {
	t.Helper()

	wantPrices, err := sdk.ParseDecCoins(want)
	require.NoError(t, err)
	gotPrices, err := sdk.ParseDecCoins(got)
	require.NoError(t, err)
	require.True(t, wantPrices.Equal(gotPrices), "gas prices = %s, want %s", gotPrices, wantPrices)
}

func TestReconcileGasPricesWithNodeSkipsCommandWithoutGasPrices(t *testing.T) {
	rpc := newGasPriceRPC("0.025uakt")
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	require.NoError(t, reconcileGasPricesWithNode(
		cmd.Context(),
		sdkclient.Context{}.WithClient(rpc),
		cmd.Flags(),
	))
	require.Zero(t, rpc.calls)
}

func TestReconcileGasPricesWithNodeUsesSelectedRPCMinimum(t *testing.T) {
	rpc := newGasPriceRPC("0.025000000000000000uakt")
	grpcClient, err := grpc.NewClient(
		"passthrough:///unused",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, grpcClient.Close()) })

	cctx := sdkclient.Context{}.
		WithClient(rpc).
		WithGRPCClient(grpcClient)
	cmd := gasPriceTestCommand(t, "0.0025uakt")

	require.NoError(t, reconcileGasPricesWithNode(cmd.Context(), cctx, cmd.Flags()))

	gasPrices, err := cmd.Flags().GetString(flagdefs.FlagGasPrices)
	require.NoError(t, err)
	requireGasPricesEqual(t, "0.025uakt", gasPrices)
	require.Equal(t, 1, rpc.calls)
	require.Equal(t, "/cosmos.base.node.v1beta1.Service/Config", rpc.path)

	gasEstimate := sdkmath.LegacyNewDec(206739)
	oldPrices, err := sdk.ParseDecCoins("0.0025uakt")
	require.NoError(t, err)
	effectivePrices, err := sdk.ParseDecCoins(gasPrices)
	require.NoError(t, err)
	require.Equal(t, "517", oldPrices[0].Amount.Mul(gasEstimate).Ceil().RoundInt().String())
	require.Equal(t, "5169", effectivePrices[0].Amount.Mul(gasEstimate).Ceil().RoundInt().String())
}

func TestReconcileGasPricesWithNodePreservesHigherPrice(t *testing.T) {
	rpc := newGasPriceRPC("0.025uakt")
	cmd := gasPriceTestCommand(t, "0.04uakt")

	require.NoError(t, reconcileGasPricesWithNode(
		cmd.Context(),
		sdkclient.Context{}.WithClient(rpc),
		cmd.Flags(),
	))

	gasPrices, err := cmd.Flags().GetString(flagdefs.FlagGasPrices)
	require.NoError(t, err)
	requireGasPricesEqual(t, "0.04uakt", gasPrices)
}

func TestReconcileGasPricesWithNodeUsesMinimumWhenPriceIsEmpty(t *testing.T) {
	rpc := newGasPriceRPC("0.025uakt")
	cmd := gasPriceTestCommand(t, "")

	require.NoError(t, reconcileGasPricesWithNode(
		cmd.Context(),
		sdkclient.Context{}.WithClient(rpc),
		cmd.Flags(),
	))

	gasPrices, err := cmd.Flags().GetString(flagdefs.FlagGasPrices)
	require.NoError(t, err)
	requireGasPricesEqual(t, "0.025uakt", gasPrices)
}

func TestReconcileGasPricesWithNodeLeavesEmptyMinimumUnchanged(t *testing.T) {
	rpc := newGasPriceRPC("")
	cmd := gasPriceTestCommand(t, "0.0025uakt")

	require.NoError(t, reconcileGasPricesWithNode(
		cmd.Context(),
		sdkclient.Context{}.WithClient(rpc),
		cmd.Flags(),
	))

	gasPrices, err := cmd.Flags().GetString(flagdefs.FlagGasPrices)
	require.NoError(t, err)
	requireGasPricesEqual(t, "0.0025uakt", gasPrices)
}

func TestReconcileGasPricesWithNodeSkipsExplicitFeesAndOfflineMode(t *testing.T) {
	t.Run("fixed fees", func(t *testing.T) {
		rpc := newGasPriceRPC("0.025uakt")
		cmd := gasPriceTestCommand(t, "0.0025uakt")
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagFees, "517uakt"))

		require.NoError(t, reconcileGasPricesWithNode(
			cmd.Context(),
			sdkclient.Context{}.WithClient(rpc),
			cmd.Flags(),
		))
		require.Zero(t, rpc.calls)
	})

	t.Run("offline", func(t *testing.T) {
		rpc := newGasPriceRPC("0.025uakt")
		cmd := gasPriceTestCommand(t, "0.0025uakt")
		require.NoError(t, cmd.Flags().Set(flagdefs.FlagOffline, "true"))

		require.NoError(t, reconcileGasPricesWithNode(
			cmd.Context(),
			sdkclient.Context{}.WithClient(rpc),
			cmd.Flags(),
		))
		require.Zero(t, rpc.calls)
	})
}

func TestPrebuiltBroadcastDoesNotDeriveFees(t *testing.T) {
	require.False(t, commandDerivesFees(GetBroadcastCommand()))
	require.True(t, commandDerivesFees(&cobra.Command{}))
}

func TestTxPreflightReconcilesGasPricesBeforeClientDiscovery(t *testing.T) {
	rpc := newGasPriceRPC("0.025uakt")
	cctx := sdkclient.Context{}.WithClient(rpc)
	cmd := gasPriceTestCommand(t, "0.0025uakt")
	cmd.SetContext(context.WithValue(cmd.Context(), ClientContextKey, &cctx))

	err := TxPersistentPreRunE(cmd, nil)
	require.ErrorContains(t, err, "codec is not initialized")

	gasPrices, getErr := cmd.Flags().GetString(flagdefs.FlagGasPrices)
	require.NoError(t, getErr)
	requireGasPricesEqual(t, "0.025uakt", gasPrices)
}

func TestReconcileGasPricesWithNodeRejectsUnavailablePolicy(t *testing.T) {
	t.Run("malformed configured price", func(t *testing.T) {
		rpc := newGasPriceRPC("0.025uakt")
		cmd := gasPriceTestCommand(t, "not-a-price")

		err := reconcileGasPricesWithNode(
			cmd.Context(),
			sdkclient.Context{}.WithClient(rpc),
			cmd.Flags(),
		)
		require.ErrorContains(t, err, "--"+flagdefs.FlagGasPrices)
		require.Zero(t, rpc.calls)
	})

	t.Run("query failure", func(t *testing.T) {
		rpc := newGasPriceRPC("")
		rpc.err = errors.New("node unavailable")
		cmd := gasPriceTestCommand(t, "0.0025uakt")

		err := reconcileGasPricesWithNode(
			cmd.Context(),
			sdkclient.Context{}.WithClient(rpc),
			cmd.Flags(),
		)
		require.ErrorContains(t, err, "minimum gas prices")
		require.ErrorContains(t, err, "node unavailable")
	})

	t.Run("malformed response", func(t *testing.T) {
		rpc := newGasPriceRPC("not-a-price")
		cmd := gasPriceTestCommand(t, "0.0025uakt")

		err := reconcileGasPricesWithNode(
			cmd.Context(),
			sdkclient.Context{}.WithClient(rpc),
			cmd.Flags(),
		)
		require.ErrorContains(t, err, "invalid minimum gas prices")
	})

	t.Run("no common denomination", func(t *testing.T) {
		rpc := newGasPriceRPC("0.025uakt")
		cmd := gasPriceTestCommand(t, "0.01uatom")

		err := reconcileGasPricesWithNode(
			cmd.Context(),
			sdkclient.Context{}.WithClient(rpc),
			cmd.Flags(),
		)
		require.ErrorContains(t, err, "no denomination accepted by the selected RPC node")
		require.ErrorContains(t, err, "uatom")
		require.ErrorContains(t, err, "uakt")
	})
}
