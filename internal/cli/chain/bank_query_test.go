package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
)

type recordingBankQueryClient struct {
	banktypes.QueryClient
	request *banktypes.QueryAllBalancesRequest
}

func (client *recordingBankQueryClient) AllBalances(
	_ context.Context,
	request *banktypes.QueryAllBalancesRequest,
	_ ...grpc.CallOption,
) (*banktypes.QueryAllBalancesResponse, error) {
	requestCopy := *request
	client.request = &requestCopy

	return &banktypes.QueryAllBalancesResponse{
		Balances: sdk.NewCoins(sdk.NewInt64Coin("uakt", 1_000_000)),
	}, nil
}

type aggregateBankQueryClient struct {
	cv1beta3.QueryClient
	bank banktypes.QueryClient
}

func (client *aggregateBankQueryClient) Bank() banktypes.QueryClient {
	return client.bank
}

func (client *aggregateBankQueryClient) ClientContext() sdkclient.Context {
	return sdkclient.Context{}
}

func executeBankBalancesQuery(t *testing.T, output string) (string, *banktypes.QueryAllBalancesRequest) {
	t.Helper()

	const address = "akash1qqqsyqcyq5rqwzqfpg9scrgwpugpzysnwduagr"

	var out bytes.Buffer
	clientContext := sdkclient.Context{
		Codec:  codec.NewProtoCodec(codectypes.NewInterfaceRegistry()),
		Output: &out,
	}
	bankClient := &recordingBankQueryClient{}
	lightClient := &stubLightClient{
		q:    &aggregateBankQueryClient{bank: bankClient},
		cctx: clientContext,
	}

	ctx := context.WithValue(context.Background(), ClientContextKey, &sdkclient.Context{})
	ctx = context.WithValue(ctx, ContextTypeQueryClient, lightClient)

	cmd := GetQueryBankBalancesCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{address, "--output", output})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	require.NoError(t, cmd.Execute())
	require.NotNil(t, bankClient.request)
	require.Equal(t, address, bankClient.request.Address)

	return out.String(), bankClient.request
}

func TestBankBalancesJSONPreservesCanonicalChainCoin(t *testing.T) {
	out, request := executeBankBalancesQuery(t, "json")
	require.False(t, request.ResolveDenom, "the query must not rewrite canonical chain denominations")

	var payload struct {
		Balances []struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"balances"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.Len(t, payload.Balances, 1)
	require.Equal(t, "uakt", payload.Balances[0].Denom)
	require.Equal(t, "1000000", payload.Balances[0].Amount)
}

func TestBankBalancesPrettyScalesCanonicalMicroDenom(t *testing.T) {
	out, request := executeBankBalancesQuery(t, "pretty")
	require.False(t, request.ResolveDenom, "pretty output must receive the canonical chain denomination")
	require.Contains(t, out, "1 AKT")
	require.False(t, strings.Contains(out, "1000000"), "pretty output leaked the unscaled amount: %s", out)
}
