package cli

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	flagdefs "pkg.akt.dev/akt/internal/flags"
	aclient "pkg.akt.dev/go/node/client"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	deploymenttypes "pkg.akt.dev/go/node/deployment/v1beta4"
	markettypes "pkg.akt.dev/go/node/market/v1beta5"
)

type depositDeploymentQuery struct {
	deploymenttypes.QueryClient
}

func (*depositDeploymentQuery) Params(
	context.Context,
	*deploymenttypes.QueryParamsRequest,
	...grpc.CallOption,
) (*deploymenttypes.QueryParamsResponse, error) {
	return &deploymenttypes.QueryParamsResponse{
		Params: deploymenttypes.Params{MinDeposits: sdk.NewCoins(sdk.NewInt64Coin("uact", 17))},
	}, nil
}

type depositMarketQuery struct {
	markettypes.QueryClient
}

func (*depositMarketQuery) Params(
	context.Context,
	*markettypes.QueryParamsRequest,
	...grpc.CallOption,
) (*markettypes.QueryParamsResponse, error) {
	return &markettypes.QueryParamsResponse{
		Params: markettypes.Params{BidMinDeposit: sdk.NewInt64Coin("uakt", 23)},
	}, nil
}

type depositAggregateQuery struct {
	clientv1beta3.QueryClient
}

func (*depositAggregateQuery) Deployment() deploymenttypes.QueryClient {
	return &depositDeploymentQuery{}
}

func (*depositAggregateQuery) Market() markettypes.QueryClient {
	return &depositMarketQuery{}
}

func (*depositAggregateQuery) ClientContext() sdkclient.Context {
	return sdkclient.Context{}
}

func TestCanonicalDepositFlagSelectsExplicitAndQueriedValues(t *testing.T) {
	query := &depositAggregateQuery{}

	for _, test := range []struct {
		name     string
		detect   func(context.Context, *pflag.FlagSet, aclient.QueryClient) (sdk.Coin, error)
		queried  sdk.Coin
		explicit sdk.Coin
	}{
		{
			name:     "deployment",
			detect:   DetectDeploymentDeposit,
			queried:  sdk.NewInt64Coin("uact", 17),
			explicit: sdk.NewInt64Coin("uakt", 19),
		},
		{
			name:     "bid",
			detect:   DetectBidDeposit,
			queried:  sdk.NewInt64Coin("uakt", 23),
			explicit: sdk.NewInt64Coin("uact", 29),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			flags := pflag.NewFlagSet(test.name, pflag.ContinueOnError)
			flags.String(flagdefs.FlagDeposit, "", "")
			got, err := test.detect(context.Background(), flags, query)
			require.NoError(t, err)
			require.Equal(t, test.queried, got)

			require.NoError(t, flags.Set(flagdefs.FlagDeposit, test.explicit.String()))
			got, err = test.detect(context.Background(), flags, query)
			require.NoError(t, err)
			require.Equal(t, test.explicit, got)
		})
	}
}
