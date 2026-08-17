package data_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"

	"pkg.akt.dev/akt/internal/tui/data"
	"pkg.akt.dev/akt/internal/tui/messages"
	aclient "pkg.akt.dev/go/node/client"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type balanceLightClient struct {
	aclient.LightClient
	query clientv1beta3.QueryClient
}

func (c balanceLightClient) Query() clientv1beta3.QueryClient { return c.query }

type balanceQueryClient struct {
	clientv1beta3.QueryClient
	bank banktypes.QueryClient
}

func (c balanceQueryClient) Bank() banktypes.QueryClient { return c.bank }

type allBalancesClient struct {
	banktypes.QueryClient
	response *banktypes.QueryAllBalancesResponse
	err      error
}

func (c allBalancesClient) AllBalances(
	context.Context,
	*banktypes.QueryAllBalancesRequest,
	...grpc.CallOption,
) (*banktypes.QueryAllBalancesResponse, error) {
	return c.response, c.err
}

func TestLoadBalanceUsesCanonicalMicroDenomFormatting(t *testing.T) {
	t.Parallel()

	bank := allBalancesClient{response: &banktypes.QueryAllBalancesResponse{
		Balances: sdk.NewCoins(sdk.NewInt64Coin("uakt", 5_300_000)),
	}}
	loader := data.NewLoader(nil, balanceLightClient{query: balanceQueryClient{bank: bank}})

	msg, ok := loader.LoadBalance("akash1account")().(messages.BalanceLoadedMsg)
	if !ok {
		t.Fatalf("LoadBalance returned an unexpected message type")
	}
	if msg.Err != nil {
		t.Fatalf("LoadBalance returned error: %v", msg.Err)
	}
	if got, want := msg.Amount, "5.3 AKT"; got != want {
		t.Fatalf("formatted balance = %q, want %q", got, want)
	}
}
