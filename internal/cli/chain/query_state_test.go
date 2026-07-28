package cli

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"

	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
)

// Valid akash bech32 addresses (same fixtures as the flags package tests).
const (
	stateTestOwner    = "akash1qqqsyqcyq5rqwzqfpg9scrgwpugpzysnwduagr"
	stateTestProvider = "akash1v3jkvemgd94xkmrddehhqutjwd682anh9zw2p2"
)

// stubDeploymentQuery serves a canned single deployment and counts get/list
// calls so tests can prove which path the command took.
type stubDeploymentQuery struct {
	dvbeta.QueryClient

	res       *dvbeta.QueryDeploymentResponse
	getCalls  int
	listCalls int
}

func (s *stubDeploymentQuery) Deployment(_ context.Context, _ *dvbeta.QueryDeploymentRequest, _ ...grpc.CallOption) (*dvbeta.QueryDeploymentResponse, error) {
	s.getCalls++
	return s.res, nil
}

func (s *stubDeploymentQuery) Deployments(_ context.Context, _ *dvbeta.QueryDeploymentsRequest, _ ...grpc.CallOption) (*dvbeta.QueryDeploymentsResponse, error) {
	s.listCalls++
	return &dvbeta.QueryDeploymentsResponse{}, nil
}

// stubMarketQuery serves canned single order/bid/lease records.
type stubMarketQuery struct {
	mvbeta.QueryClient

	order *mvbeta.QueryOrderResponse
	bid   *mvbeta.QueryBidResponse
	lease *mvbeta.QueryLeaseResponse
}

func (s *stubMarketQuery) Order(_ context.Context, _ *mvbeta.QueryOrderRequest, _ ...grpc.CallOption) (*mvbeta.QueryOrderResponse, error) {
	return s.order, nil
}

func (s *stubMarketQuery) Bid(_ context.Context, _ *mvbeta.QueryBidRequest, _ ...grpc.CallOption) (*mvbeta.QueryBidResponse, error) {
	return s.bid, nil
}

func (s *stubMarketQuery) Lease(_ context.Context, _ *mvbeta.QueryLeaseRequest, _ ...grpc.CallOption) (*mvbeta.QueryLeaseResponse, error) {
	return s.lease, nil
}

// stubQueryClient plugs the module stubs into the aggregated v1beta3 query
// client. Unstubbed modules panic if touched, which is what a test wants.
type stubQueryClient struct {
	cv1beta3.QueryClient

	dep dvbeta.QueryClient
	mkt mvbeta.QueryClient
}

func (s *stubQueryClient) Deployment() dvbeta.QueryClient   { return s.dep }
func (s *stubQueryClient) Market() mvbeta.QueryClient       { return s.mkt }
func (s *stubQueryClient) ClientContext() sdkclient.Context { return sdkclient.Context{} }

// stubLightClient satisfies aclient.LightClient so query RunE bodies execute
// without any network: queries hit the stubs above and printed output lands
// in the client context's Output buffer.
type stubLightClient struct {
	q    cv1beta3.QueryClient
	cctx sdkclient.Context
}

func (s *stubLightClient) Query() cv1beta3.QueryClient      { return s.q }
func (s *stubLightClient) Node() cv1beta3.NodeClient        { return nil }
func (s *stubLightClient) ClientContext() sdkclient.Context { return s.cctx }
func (s *stubLightClient) PrintMessage(interface{}) error   { return nil }
func (s *stubLightClient) PrintJSON(interface{}) error      { return nil }

// execQueryCmd runs a query command against the stub client and returns what
// the command printed (the query result output) alongside its error.
func execQueryCmd(t *testing.T, cmd *cobra.Command, q cv1beta3.QueryClient, args ...string) (string, error) {
	t.Helper()

	var queryOut bytes.Buffer
	cl := &stubLightClient{
		q: q,
		cctx: sdkclient.Context{
			Codec:  codec.NewProtoCodec(codectypes.NewInterfaceRegistry()),
			Output: &queryOut,
		},
	}

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

// TestQueryDeploymentPositionalStateMismatchFails pins regression AKT-650#1:
// `query deployment <owner>/<dseq> <state>` with a mismatching state must not
// silently print the record — the get is performed and the state verified.
func TestQueryDeploymentPositionalStateMismatchFails(t *testing.T) {
	dep := &stubDeploymentQuery{res: &dvbeta.QueryDeploymentResponse{
		Deployment: dv1.Deployment{
			ID:    dv1.DeploymentID{Owner: stateTestOwner, DSeq: 12345},
			State: dv1.DeploymentClosed,
		},
	}}

	out, err := execQueryCmd(t, GetQueryDeploymentCmds(), &stubQueryClient{dep: dep}, stateTestOwner+"/12345", "active")
	require.Error(t, err, "mismatching positional state must not silently print the record")
	require.Contains(t, err.Error(), "is closed, not active")
	require.Equal(t, 1, dep.getCalls, "complete identity must stay on the get path")
	require.Zero(t, dep.listCalls)
	require.Empty(t, out, "no record may be printed on a state mismatch")
}

// TestQueryDeploymentPositionalStateMatchPrints: when the record really is in
// the requested state, the get result is printed as before.
func TestQueryDeploymentPositionalStateMatchPrints(t *testing.T) {
	dep := &stubDeploymentQuery{res: &dvbeta.QueryDeploymentResponse{
		Deployment: dv1.Deployment{
			ID:    dv1.DeploymentID{Owner: stateTestOwner, DSeq: 12345},
			State: dv1.DeploymentActive,
		},
	}}

	out, err := execQueryCmd(t, GetQueryDeploymentCmds(), &stubQueryClient{dep: dep}, stateTestOwner+"/12345", "active")
	require.NoError(t, err)
	require.Equal(t, 1, dep.getCalls)
	require.Contains(t, out, "12345")
}

// TestQueryDeploymentIdentityOnlyUnchanged: the identity-only get path stays
// exactly as it was — no state argument, no verification, record printed.
func TestQueryDeploymentIdentityOnlyUnchanged(t *testing.T) {
	dep := &stubDeploymentQuery{res: &dvbeta.QueryDeploymentResponse{
		Deployment: dv1.Deployment{
			ID:    dv1.DeploymentID{Owner: stateTestOwner, DSeq: 12345},
			State: dv1.DeploymentClosed,
		},
	}}

	out, err := execQueryCmd(t, GetQueryDeploymentCmds(), &stubQueryClient{dep: dep}, stateTestOwner+"/12345")
	require.NoError(t, err)
	require.Equal(t, 1, dep.getCalls)
	require.Zero(t, dep.listCalls)
	require.Contains(t, out, "12345")
}

// TestQueryMarketPositionalStateMismatchFails applies the same pin to the
// order/bid/lease twins: a mismatching positional [state] on a complete
// identity is an error, never a silent print.
func TestQueryMarketPositionalStateMismatchFails(t *testing.T) {
	mkt := &stubMarketQuery{
		order: &mvbeta.QueryOrderResponse{Order: mvbeta.Order{
			ID:    mv1.OrderID{Owner: stateTestOwner, DSeq: 12345, GSeq: 1, OSeq: 1},
			State: mvbeta.OrderClosed,
		}},
		bid: &mvbeta.QueryBidResponse{Bid: mvbeta.Bid{
			ID:    mv1.BidID{Owner: stateTestOwner, DSeq: 12345, GSeq: 1, OSeq: 1, Provider: stateTestProvider},
			State: mvbeta.BidClosed,
		}},
		lease: &mvbeta.QueryLeaseResponse{Lease: mv1.Lease{
			ID:    mv1.LeaseID{Owner: stateTestOwner, DSeq: 12345, GSeq: 1, OSeq: 1, Provider: stateTestProvider},
			State: mv1.LeaseClosed,
		}},
	}

	fullBidID := fmt.Sprintf("%s/12345/1/1/%s", stateTestOwner, stateTestProvider)

	cases := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{"order", GetQueryMarketOrderCmd(), []string{stateTestOwner + "/12345/1/1", "open"}, "is closed, not open"},
		{"bid", GetQueryMarketBidCmd(), []string{fullBidID, "open"}, "is closed, not open"},
		{"lease", GetQueryMarketLeaseCmd(), []string{fullBidID, "active"}, "is closed, not active"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := execQueryCmd(t, tc.cmd, &stubQueryClient{mkt: mkt}, tc.args...)
			require.Error(t, err, "mismatching positional state must not silently print the record")
			require.Contains(t, err.Error(), tc.wantErr)
			require.Empty(t, out, "no record may be printed on a state mismatch")
		})
	}
}

// TestQueryMarketPositionalStateMatchPrints: matching states on the get path
// still print the record for order/bid/lease.
func TestQueryMarketPositionalStateMatchPrints(t *testing.T) {
	mkt := &stubMarketQuery{
		order: &mvbeta.QueryOrderResponse{Order: mvbeta.Order{
			ID:    mv1.OrderID{Owner: stateTestOwner, DSeq: 12345, GSeq: 1, OSeq: 1},
			State: mvbeta.OrderOpen,
		}},
		bid: &mvbeta.QueryBidResponse{Bid: mvbeta.Bid{
			ID:    mv1.BidID{Owner: stateTestOwner, DSeq: 12345, GSeq: 1, OSeq: 1, Provider: stateTestProvider},
			State: mvbeta.BidOpen,
		}},
		lease: &mvbeta.QueryLeaseResponse{Lease: mv1.Lease{
			ID:    mv1.LeaseID{Owner: stateTestOwner, DSeq: 12345, GSeq: 1, OSeq: 1, Provider: stateTestProvider},
			State: mv1.LeaseActive,
		}},
	}

	fullBidID := fmt.Sprintf("%s/12345/1/1/%s", stateTestOwner, stateTestProvider)

	cases := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{"order", GetQueryMarketOrderCmd(), []string{stateTestOwner + "/12345/1/1", "open"}},
		{"bid", GetQueryMarketBidCmd(), []string{fullBidID, "open"}},
		{"lease", GetQueryMarketLeaseCmd(), []string{fullBidID, "active"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := execQueryCmd(t, tc.cmd, &stubQueryClient{mkt: mkt}, tc.args...)
			require.NoError(t, err)
			require.Contains(t, out, "12345")
		})
	}
}
