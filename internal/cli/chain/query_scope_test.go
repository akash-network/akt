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
// an empty from leaves the context accountless, as a Console-only or
// monitoring context commonly is.
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

// listedFilter is the scope a captured list request carried.
type listedFilter struct {
	owner, provider string
	limit           uint64
}

// runBareList runs one list command against capturing stubs, requires that it
// sent exactly one list request, and returns that request's scope.
func runBareList(t *testing.T, kind, from string, args ...string) listedFilter {
	t.Helper()

	dep := &capturingDeploymentQuery{}
	mkt := &capturingMarketQuery{}
	cert := &capturingCertQuery{}
	query := &certQueryClient{stubQueryClient: &stubQueryClient{dep: dep, mkt: mkt}, cert: cert}

	cmds := map[string]func() *cobra.Command{
		"deployment":  GetQueryDeploymentCmds,
		"order":       GetQueryMarketOrderCmd,
		"bid":         GetQueryMarketBidCmd,
		"lease":       GetQueryMarketLeaseCmd,
		"certificate": GetQueryCertCertificatesCmd,
	}

	_, err := execQueryCmdFrom(t, cmds[kind](), query, from, args...)
	require.NoError(t, err)
	require.Equal(t, 1, dep.listCalls+mkt.listCalls+cert.listCalls, "exactly one list request")

	switch {
	case dep.lastList != nil:
		return listedFilter{owner: dep.lastList.Filters.Owner, limit: dep.lastList.Pagination.Limit}
	case mkt.orders != nil:
		return listedFilter{owner: mkt.orders.Filters.Owner, limit: mkt.orders.Pagination.Limit}
	case mkt.bids != nil:
		return listedFilter{owner: mkt.bids.Filters.Owner, provider: mkt.bids.Filters.Provider, limit: mkt.bids.Pagination.Limit}
	case mkt.leases != nil:
		return listedFilter{owner: mkt.leases.Filters.Owner, provider: mkt.leases.Filters.Provider, limit: mkt.leases.Pagination.Limit}
	default:
		return listedFilter{owner: cert.last.Filter.Owner, limit: cert.last.Pagination.Limit}
	}
}

// TestQueryListWithoutOwnerIsNetworkWide pins SPEC §3.8.4: a bare list needs no
// default account. It goes out once with an empty owner -- the chain's "no
// filter" -- and the default page size, instead of being refused.
func TestQueryListWithoutOwnerIsNetworkWide(t *testing.T) {
	for _, kind := range []string{"deployment", "order", "bid", "lease", "certificate"} {
		t.Run(kind, func(t *testing.T) {
			got := runBareList(t, kind, "")
			require.Empty(t, got.owner)
			require.Equal(t, uint64(100), got.limit, "the default page size applies")
		})
	}
}

// TestQueryListIgnoresDefaultAccount is the other half: a configured default
// account does not narrow a bare list either, so the same command answers the
// same way in every context.
func TestQueryListIgnoresDefaultAccount(t *testing.T) {
	for _, kind := range []string{"deployment", "order", "bid", "lease", "certificate"} {
		t.Run(kind, func(t *testing.T) {
			require.Empty(t, runBareList(t, kind, stateTestOwner).owner)
		})
	}
}

// TestQueryListByProviderWithoutProviderIsNetworkWide: --by provider with no
// address is the same network-wide list as owner mode.
func TestQueryListByProviderWithoutProviderIsNetworkWide(t *testing.T) {
	for _, kind := range []string{"bid", "lease"} {
		t.Run(kind, func(t *testing.T) {
			got := runBareList(t, kind, "", "--by", "provider")
			require.Empty(t, got.owner)
			require.Empty(t, got.provider)
		})
	}
}

// TestQueryStateKeywordListIsNetworkWide covers the reported command,
// `akt query deployment closed` on a context with no default account.
func TestQueryStateKeywordListIsNetworkWide(t *testing.T) {
	dep := &capturingDeploymentQuery{}

	_, err := execQueryCmdFrom(t, GetQueryDeploymentCmds(), &stubQueryClient{dep: dep}, "", "closed")
	require.NoError(t, err)
	require.Empty(t, dep.lastList.Filters.Owner)
	require.Equal(t, "closed", dep.lastList.Filters.State)
}

// TestQueryDseqShorthandUsesDefaultAccount: the numeric dseq shorthand is where
// the default account still applies, since the owner is part of the identity it
// names. Without an account to resolve, it fails before any request is sent.
func TestQueryDseqShorthandUsesDefaultAccount(t *testing.T) {
	mkt := &capturingMarketQuery{}

	_, err := execQueryCmdFrom(t, GetQueryMarketOrderCmd(), &stubQueryClient{mkt: mkt}, stateTestOwner, "12345")
	require.NoError(t, err)
	require.Equal(t, stateTestOwner, mkt.orders.Filters.Owner)
	require.Equal(t, uint64(12345), mkt.orders.Filters.DSeq)

	mkt = &capturingMarketQuery{}

	_, err = execQueryCmdFrom(t, GetQueryMarketOrderCmd(), &stubQueryClient{mkt: mkt}, "", "12345")
	require.ErrorContains(t, err, "no default account set")
	require.Zero(t, mkt.listCalls)
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
