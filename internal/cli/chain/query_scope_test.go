package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	certv1 "pkg.akt.dev/go/node/cert/v1"
	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
)

// capturingDeploymentQuery records the request the command actually sent, so a
// test can assert on the filter rather than only on the returned error.
type capturingDeploymentQuery struct {
	dvbeta.QueryClient

	lastList  *dvbeta.QueryDeploymentsRequest
	listCalls int
}

func (s *capturingDeploymentQuery) Deployments(_ context.Context, req *dvbeta.QueryDeploymentsRequest, _ ...grpc.CallOption) (*dvbeta.QueryDeploymentsResponse, error) {
	s.listCalls++
	s.lastList = req

	return &dvbeta.QueryDeploymentsResponse{}, nil
}

// capturingMarketQuery does the same for the order/bid/lease list paths.
type capturingMarketQuery struct {
	mvbeta.QueryClient

	orders *mvbeta.QueryOrdersRequest
	bids   *mvbeta.QueryBidsRequest
	leases *mvbeta.QueryLeasesRequest

	listCalls int
}

type capturingCertQuery struct {
	certv1.QueryClient

	last      *certv1.QueryCertificatesRequest
	listCalls int
}

func (s *capturingCertQuery) Certificates(_ context.Context, req *certv1.QueryCertificatesRequest, _ ...grpc.CallOption) (*certv1.QueryCertificatesResponse, error) {
	s.listCalls++
	s.last = req

	return &certv1.QueryCertificatesResponse{}, nil
}

type certQueryClient struct {
	*stubQueryClient
	cert certv1.QueryClient
}

func (s *certQueryClient) Certs() certv1.QueryClient { return s.cert }

func (s *capturingMarketQuery) Orders(_ context.Context, req *mvbeta.QueryOrdersRequest, _ ...grpc.CallOption) (*mvbeta.QueryOrdersResponse, error) {
	s.listCalls++
	s.orders = req

	return &mvbeta.QueryOrdersResponse{}, nil
}

func (s *capturingMarketQuery) Bids(_ context.Context, req *mvbeta.QueryBidsRequest, _ ...grpc.CallOption) (*mvbeta.QueryBidsResponse, error) {
	s.listCalls++
	s.bids = req

	return &mvbeta.QueryBidsResponse{}, nil
}

func (s *capturingMarketQuery) Leases(_ context.Context, req *mvbeta.QueryLeasesRequest, _ ...grpc.CallOption) (*mvbeta.QueryLeasesResponse, error) {
	s.listCalls++
	s.leases = req

	return &mvbeta.QueryLeasesResponse{}, nil
}

// execQueryCmdFrom is execQueryCmd with a default account configured. Passing
// an empty from leaves the context accountless, which is the state a
// console-api context is always in.
func execQueryCmdFrom(t *testing.T, cmd *cobra.Command, q cv1beta3.QueryClient, from string, args ...string) (string, error) {
	t.Helper()

	var queryOut bytes.Buffer

	cctx := sdkclient.Context{
		Codec:  codec.NewProtoCodec(codectypes.NewInterfaceRegistry()),
		Output: &queryOut,
	}

	if from != "" {
		addr, err := sdk.AccAddressFromBech32(from)
		require.NoError(t, err)

		cctx = cctx.WithFromAddress(addr)
	}

	cl := &stubLightClient{q: q, cctx: cctx}

	var cobraOut bytes.Buffer

	cmd.SetOut(&cobraOut)
	cmd.SetErr(&cobraOut)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(append(args, "--output", "json"))

	ctx := context.WithValue(context.Background(), ClientContextKey, &sdkclient.Context{})
	ctx = context.WithValue(ctx, ContextTypeQueryClient, cl)
	cmd.SetContext(ctx)

	err := cmd.Execute()

	return queryOut.String(), err
}

// TestQueryListWithoutOwnerIsRefused pins the core regression: on a context
// with no default account, a bare owner-mode list must fail rather than drop
// the owner filter. An empty owner is not "no results" to the query layer, it
// is "no filter", so the unscoped request came back with every deployment on
// the network presented as the caller's own.
//
// listCalls is the assertion that matters -- the request must never leave.
func TestQueryListWithoutOwnerIsRefused(t *testing.T) {
	cases := []struct {
		name string
		cmd  func() *cobra.Command
	}{
		{"deployment", GetQueryDeploymentCmds},
		{"order", GetQueryMarketOrderCmd},
		{"bid", GetQueryMarketBidCmd},
		{"lease", GetQueryMarketLeaseCmd},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dep := &capturingDeploymentQuery{}
			mkt := &capturingMarketQuery{}

			out, err := execQueryCmdFrom(t, tc.cmd(), &stubQueryClient{dep: dep, mkt: mkt}, "")
			require.Error(t, err, "a bare list with no default account must not answer")
			require.Contains(t, err.Error(), "no default account set")
			require.Zero(t, dep.listCalls, "no unscoped deployment query may be sent")
			require.Zero(t, mkt.listCalls, "no unscoped market query may be sent")
			require.Empty(t, out)
		})
	}
}

// TestQueryListByProviderWithoutProviderIsRefused: SPEC §3.8.4 makes the
// leading address required in --by provider mode. Nothing enforced it, so
// `--by provider` with no address listed the whole network.
func TestQueryListByProviderWithoutProviderIsRefused(t *testing.T) {
	cases := []struct {
		name string
		cmd  func() *cobra.Command
	}{
		{"bid", GetQueryMarketBidCmd},
		{"lease", GetQueryMarketLeaseCmd},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mkt := &capturingMarketQuery{}

			out, err := execQueryCmdFrom(t, tc.cmd(), &stubQueryClient{mkt: mkt}, "", "--by", "provider")
			require.Error(t, err)
			require.Contains(t, err.Error(), "--by provider requires a provider address")
			require.Zero(t, mkt.listCalls, "no unscoped market query may be sent")
			require.Empty(t, out)
		})
	}
}

// TestQueryListWithDefaultOwnerScopes is the other half: when the context does
// have an account, the bare list still works and the filter carries it. Without
// this the fix above could pass by refusing everything.
func TestQueryListWithDefaultOwnerScopes(t *testing.T) {
	t.Run("deployment", func(t *testing.T) {
		dep := &capturingDeploymentQuery{}

		_, err := execQueryCmdFrom(t, GetQueryDeploymentCmds(), &stubQueryClient{dep: dep}, stateTestOwner)
		require.NoError(t, err)
		require.Equal(t, 1, dep.listCalls)
		require.Equal(t, stateTestOwner, dep.lastList.Filters.Owner, "the list must be scoped to the default account")
	})

	t.Run("order", func(t *testing.T) {
		mkt := &capturingMarketQuery{}

		_, err := execQueryCmdFrom(t, GetQueryMarketOrderCmd(), &stubQueryClient{mkt: mkt}, stateTestOwner)
		require.NoError(t, err)
		require.Equal(t, stateTestOwner, mkt.orders.Filters.Owner)
	})

	t.Run("bid", func(t *testing.T) {
		mkt := &capturingMarketQuery{}

		_, err := execQueryCmdFrom(t, GetQueryMarketBidCmd(), &stubQueryClient{mkt: mkt}, stateTestOwner)
		require.NoError(t, err)
		require.Equal(t, stateTestOwner, mkt.bids.Filters.Owner)
	})

	t.Run("lease", func(t *testing.T) {
		mkt := &capturingMarketQuery{}

		_, err := execQueryCmdFrom(t, GetQueryMarketLeaseCmd(), &stubQueryClient{mkt: mkt}, stateTestOwner)
		require.NoError(t, err)
		require.Equal(t, stateTestOwner, mkt.leases.Filters.Owner)
	})

	t.Run("certificate", func(t *testing.T) {
		cert := &capturingCertQuery{}
		query := &certQueryClient{stubQueryClient: &stubQueryClient{}, cert: cert}

		_, err := execQueryCmdFrom(t, GetQueryCertCertificatesCmd(), query, stateTestOwner)
		require.NoError(t, err)
		require.Equal(t, 1, cert.listCalls)
		require.Equal(t, stateTestOwner, cert.last.Filter.Owner)
	})
}

func TestCertificateListWithoutOwnerIsRefused(t *testing.T) {
	cert := &capturingCertQuery{}
	query := &certQueryClient{stubQueryClient: &stubQueryClient{}, cert: cert}

	out, err := execQueryCmdFrom(t, GetQueryCertCertificatesCmd(), query, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no default account set")
	require.Zero(t, cert.listCalls, "no unscoped certificate query may be sent")
	require.Empty(t, out)
}

// TestQueryListByProviderWithProviderScopes: provider mode still works when the
// address is supplied, and scopes to it.
func TestQueryListByProviderWithProviderScopes(t *testing.T) {
	t.Run("bid", func(t *testing.T) {
		mkt := &capturingMarketQuery{}

		_, err := execQueryCmdFrom(t, GetQueryMarketBidCmd(), &stubQueryClient{mkt: mkt}, "", "--by", "provider", stateTestProvider)
		require.NoError(t, err)
		require.Equal(t, stateTestProvider, mkt.bids.Filters.Provider)
	})

	t.Run("lease", func(t *testing.T) {
		mkt := &capturingMarketQuery{}

		_, err := execQueryCmdFrom(t, GetQueryMarketLeaseCmd(), &stubQueryClient{mkt: mkt}, "", "--by", "provider", stateTestProvider)
		require.NoError(t, err)
		require.Equal(t, stateTestProvider, mkt.leases.Filters.Provider)
	})
}
