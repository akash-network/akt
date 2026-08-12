package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	sdkmath "cosmossdk.io/math"
	tmrpc "github.com/cometbft/cometbft/rpc/core/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	bme "pkg.akt.dev/go/node/bme/v1"
	clientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	escrowtypes "pkg.akt.dev/go/node/escrow/types/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	oracle "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/sdkutil"
)

type escrowBlocksBMEQuery struct {
	bme.QueryClient
	response *bme.QueryStatusResponse
	err      error
}

func (query *escrowBlocksBMEQuery) Status(
	context.Context,
	*bme.QueryStatusRequest,
	...grpc.CallOption,
) (*bme.QueryStatusResponse, error) {
	return query.response, query.err
}

type escrowBlocksOracleQuery struct {
	oracle.QueryClient
	request  *oracle.QueryAggregatedPriceRequest
	response *oracle.QueryAggregatedPriceResponse
	err      error
}

func (query *escrowBlocksOracleQuery) AggregatedPrice(
	_ context.Context,
	request *oracle.QueryAggregatedPriceRequest,
	_ ...grpc.CallOption,
) (*oracle.QueryAggregatedPriceResponse, error) {
	copy := *request
	query.request = &copy
	return query.response, query.err
}

type escrowBlocksMarketQuery struct {
	mvbeta.QueryClient
	request  *mvbeta.QueryLeasesRequest
	response *mvbeta.QueryLeasesResponse
	err      error
}

func (query *escrowBlocksMarketQuery) Leases(
	_ context.Context,
	request *mvbeta.QueryLeasesRequest,
	_ ...grpc.CallOption,
) (*mvbeta.QueryLeasesResponse, error) {
	copy := *request
	query.request = &copy
	return query.response, query.err
}

type escrowBlocksDeploymentQuery struct {
	dvbeta.QueryClient
	request  *dvbeta.QueryDeploymentRequest
	response *dvbeta.QueryDeploymentResponse
	err      error
}

func (query *escrowBlocksDeploymentQuery) Deployment(
	_ context.Context,
	request *dvbeta.QueryDeploymentRequest,
	_ ...grpc.CallOption,
) (*dvbeta.QueryDeploymentResponse, error) {
	copy := *request
	query.request = &copy
	return query.response, query.err
}

type escrowBlocksQueryClient struct {
	clientv1beta3.QueryClient
	bme        bme.QueryClient
	oracle     oracle.QueryClient
	market     mvbeta.QueryClient
	deployment dvbeta.QueryClient
}

func (client *escrowBlocksQueryClient) BME() bme.QueryClient           { return client.bme }
func (client *escrowBlocksQueryClient) Oracle() oracle.QueryClient     { return client.oracle }
func (client *escrowBlocksQueryClient) Market() mvbeta.QueryClient     { return client.market }
func (client *escrowBlocksQueryClient) Deployment() dvbeta.QueryClient { return client.deployment }
func (*escrowBlocksQueryClient) ClientContext() sdkclient.Context      { return sdkclient.Context{} }

type escrowBlocksNodeClient struct {
	height int64
	err    error
}

func (*escrowBlocksNodeClient) SyncInfo(context.Context) (*tmrpc.SyncInfo, error) {
	panic("unexpected SyncInfo call")
}

func (client *escrowBlocksNodeClient) CurrentBlockHeight(context.Context) (int64, error) {
	return client.height, client.err
}

type escrowBlocksLightClient struct {
	query clientv1beta3.QueryClient
	node  clientv1beta3.NodeClient
	cctx  sdkclient.Context
}

func (client *escrowBlocksLightClient) Query() clientv1beta3.QueryClient { return client.query }
func (client *escrowBlocksLightClient) Node() clientv1beta3.NodeClient   { return client.node }
func (client *escrowBlocksLightClient) ClientContext() sdkclient.Context { return client.cctx }
func (*escrowBlocksLightClient) PrintMessage(interface{}) error          { return nil }
func (*escrowBlocksLightClient) PrintJSON(interface{}) error             { return nil }

type escrowBlocksFixture struct {
	client     *escrowBlocksLightClient
	bme        *escrowBlocksBMEQuery
	oracle     *escrowBlocksOracleQuery
	market     *escrowBlocksMarketQuery
	deployment *escrowBlocksDeploymentQuery
	node       *escrowBlocksNodeClient
	output     *bytes.Buffer
}

func newEscrowBlocksFixture() escrowBlocksFixture {
	owner := sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String()
	leasePrice := sdk.NewDecCoinFromDec(sdkutil.DenomUact, sdkmath.LegacyMustNewDecFromStr("2.5"))

	bmeQuery := &escrowBlocksBMEQuery{response: &bme.QueryStatusResponse{Status: bme.MintStatusHaltCR}}
	oracleQuery := &escrowBlocksOracleQuery{response: &oracle.QueryAggregatedPriceResponse{
		AggregatedPrice: oracle.AggregatedPrice{
			Denom: sdkutil.DenomAkt,
			TWAP:  sdkmath.LegacyMustNewDecFromStr("2"),
		},
		PriceHealth: oracle.PriceHealth{Denom: sdkutil.DenomAkt, IsHealthy: true},
	}}
	marketQuery := &escrowBlocksMarketQuery{response: &mvbeta.QueryLeasesResponse{
		Leases: []mvbeta.QueryLeaseResponse{
			{Lease: mv1.Lease{
				ID:    mv1.LeaseID{Owner: owner, DSeq: 7, GSeq: 1, OSeq: 1},
				Price: leasePrice,
			}},
			{Lease: mv1.Lease{
				ID:    mv1.LeaseID{Owner: owner, DSeq: 7, GSeq: 2, OSeq: 1},
				Price: leasePrice,
			}},
		},
	}}
	deploymentQuery := &escrowBlocksDeploymentQuery{response: &dvbeta.QueryDeploymentResponse{
		Deployment: dv1.Deployment{ID: dv1.DeploymentID{Owner: owner, DSeq: 7}},
		EscrowAccount: escrowtypes.Account{State: escrowtypes.AccountState{
			Owner:     owner,
			SettledAt: 90,
			Funds: []escrowtypes.Balance{
				{Denom: sdkutil.DenomUact, Amount: sdkmath.LegacyNewDec(1_000)},
				{Denom: sdkutil.DenomUakt, Amount: sdkmath.LegacyNewDec(100)},
				{Denom: sdkutil.DenomUact, Amount: sdkmath.LegacyNewDec(-50)},
				{Denom: "unrelated", Amount: sdkmath.LegacyNewDec(999)},
			},
		}},
	}}
	node := &escrowBlocksNodeClient{height: 100}
	output := &bytes.Buffer{}
	queryClient := &escrowBlocksQueryClient{
		bme:        bmeQuery,
		oracle:     oracleQuery,
		market:     marketQuery,
		deployment: deploymentQuery,
	}

	return escrowBlocksFixture{
		client: &escrowBlocksLightClient{
			query: queryClient,
			node:  node,
			cctx:  sdkclient.Context{Output: output},
		},
		bme:        bmeQuery,
		oracle:     oracleQuery,
		market:     marketQuery,
		deployment: deploymentQuery,
		node:       node,
		output:     output,
	}
}

func (fixture escrowBlocksFixture) execute(t *testing.T, output string, args ...string) error {
	t.Helper()
	cmd := GetQueryEscrowBlocksRemainingCmd()
	ctx := context.WithValue(context.Background(), ClientContextKey, &sdkclient.Context{})
	ctx = context.WithValue(ctx, ContextTypeQueryClient, fixture.client)
	cmd.SetContext(ctx)
	cmd.SetOut(fixture.output)
	cmd.SetErr(fixture.output)
	cmd.SetArgs(append(args, "--output", output))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd.Execute()
}

type escrowBlocksOutput struct {
	BalanceRemain          sdk.DecCoin `json:"balance_remaining" yaml:"balance_remaining"`
	BlocksRemain           int64       `json:"blocks_remaining" yaml:"blocks_remaining"`
	EstimatedTimeRemaining int64       `json:"estimated_time_remaining" yaml:"estimated_time_remaining"`
}

type escrowBlocksYAMLOutput struct {
	BalanceRemain struct {
		Denom  string `yaml:"denom"`
		Amount string `yaml:"amount"`
	} `yaml:"balance_remaining"`
	BlocksRemain           int64  `yaml:"blocks_remaining"`
	EstimatedTimeRemaining string `yaml:"estimated_time_remaining"`
}

func TestEscrowBlocksRemainingDerivesExactSnapshotEstimate(t *testing.T) {
	fixture := newEscrowBlocksFixture()
	owner := fixture.deployment.response.Deployment.ID.Owner
	require.NoError(t, fixture.execute(t, "json", owner+"/7"))

	require.Equal(t, sdkutil.DenomAkt, fixture.oracle.request.Denom)
	require.Equal(t, mv1.LeaseFilters{
		Owner: owner,
		DSeq:  7,
		State: mv1.LeaseActive.String(),
	}, fixture.market.request.Filters)
	require.Nil(t, fixture.market.request.Pagination)
	require.Equal(t, dv1.DeploymentID{Owner: owner, DSeq: 7}, fixture.deployment.request.ID)

	var output escrowBlocksOutput
	require.NoError(t, json.Unmarshal(fixture.output.Bytes(), &output))
	require.Equal(t, sdk.NewDecCoinFromDec(sdkutil.DenomUact, sdkmath.LegacyNewDec(1_150)), output.BalanceRemain)
	require.Equal(t, int64(230), output.BlocksRemain)
	require.Equal(t, int64(1_380_000_000_000), output.EstimatedTimeRemaining)
}

func TestEscrowBlocksRemainingYAMLMatchesJSONSemantics(t *testing.T) {
	fixture := newEscrowBlocksFixture()
	owner := fixture.deployment.response.Deployment.ID.Owner
	require.NoError(t, fixture.execute(t, "yaml", owner+"/7"))

	var output escrowBlocksYAMLOutput
	require.NoError(t, yaml.Unmarshal(fixture.output.Bytes(), &output))
	require.Equal(t, sdkutil.DenomUact, output.BalanceRemain.Denom)
	require.Equal(t, "1150.000000000000000000", output.BalanceRemain.Amount)
	require.Equal(t, int64(230), output.BlocksRemain)
	require.Equal(t, "23m0s", output.EstimatedTimeRemaining)
}

func TestEscrowBlocksRemainingExcludesAKTWhenConversionIsUnavailable(t *testing.T) {
	for name, configure := range map[string]func(*escrowBlocksFixture){
		"circuit breaker inactive": func(fixture *escrowBlocksFixture) {
			fixture.bme.response.Status = bme.MintStatusHealthy
		},
		"price unhealthy": func(fixture *escrowBlocksFixture) {
			fixture.oracle.response.PriceHealth.IsHealthy = false
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newEscrowBlocksFixture()
			configure(&fixture)
			owner := fixture.deployment.response.Deployment.ID.Owner
			require.NoError(t, fixture.execute(t, "json", owner+"/7"))

			var output escrowBlocksOutput
			require.NoError(t, json.Unmarshal(fixture.output.Bytes(), &output))
			require.Equal(t, sdk.NewDecCoinFromDec(sdkutil.DenomUact, sdkmath.LegacyNewDec(950)), output.BalanceRemain)
			require.Equal(t, int64(190), output.BlocksRemain)
		})
	}
}

func TestEscrowBlocksRemainingFailsClosedAtEveryInputAndTransportBoundary(t *testing.T) {
	owner := sdk.AccAddress(bytes.Repeat([]byte{1}, 20)).String()

	for name, args := range map[string][]string{
		"missing identity": nil,
		"invalid identity": {"not-a-filter"},
		"missing dseq":     {owner + "/0"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newEscrowBlocksFixture()
			err := fixture.execute(t, "json", args...)
			require.Error(t, err)
			require.Empty(t, fixture.output.String())
		})
	}

	for name, configure := range map[string]func(*escrowBlocksFixture){
		"BME": func(fixture *escrowBlocksFixture) {
			fixture.bme.err = errors.New("BME unavailable")
		},
		"oracle": func(fixture *escrowBlocksFixture) {
			fixture.oracle.err = errors.New("oracle unavailable")
		},
		"market": func(fixture *escrowBlocksFixture) {
			fixture.market.err = errors.New("market unavailable")
		},
		"no lease": func(fixture *escrowBlocksFixture) {
			fixture.market.response.Leases = nil
		},
		"height": func(fixture *escrowBlocksFixture) {
			fixture.node.err = errors.New("height unavailable")
		},
		"deployment": func(fixture *escrowBlocksFixture) {
			fixture.deployment.err = errors.New("deployment unavailable")
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newEscrowBlocksFixture()
			configure(&fixture)
			err := fixture.execute(t, "json", owner+"/7")
			require.Error(t, err)
			require.Empty(t, fixture.output.String())
		})
	}
}
