package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"cosmossdk.io/math"
	tmrpc "github.com/cometbft/cometbft/rpc/core/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc"

	atypes "pkg.akt.dev/go/node/audit/v1"
	aclient "pkg.akt.dev/go/node/client"
	cv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mv1beta "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	attrv1 "pkg.akt.dev/go/node/types/attributes/v1"
)

type syncNodeQueryStub struct {
	cv1beta3.NodeClient
	info *tmrpc.SyncInfo
	err  error
}

func (stub *syncNodeQueryStub) SyncInfo(context.Context) (*tmrpc.SyncInfo, error) {
	return stub.info, stub.err
}

type syncLightClientStub struct {
	aclient.LightClient
	query cv1beta3.QueryClient
	node  cv1beta3.NodeClient
}

func (stub *syncLightClientStub) Query() cv1beta3.QueryClient { return stub.query }
func (stub *syncLightClientStub) Node() cv1beta3.NodeClient   { return stub.node }

type syncQueryClientStub struct {
	cv1beta3.QueryClient
	deployment dv1beta.QueryClient
	market     mv1beta.QueryClient
	provider   ptypes.QueryClient
	audit      atypes.QueryClient
}

func (stub *syncQueryClientStub) Deployment() dv1beta.QueryClient { return stub.deployment }
func (stub *syncQueryClientStub) Market() mv1beta.QueryClient     { return stub.market }
func (stub *syncQueryClientStub) Provider() ptypes.QueryClient    { return stub.provider }
func (stub *syncQueryClientStub) Audit() atypes.QueryClient       { return stub.audit }
func (*syncQueryClientStub) ClientContext() sdkclient.Context     { return sdkclient.Context{} }

type deploymentPagesStub struct {
	dv1beta.QueryClient
	requests  []*dv1beta.QueryDeploymentsRequest
	responses []*dv1beta.QueryDeploymentsResponse
	errors    []error
}

func (stub *deploymentPagesStub) Deployments(
	_ context.Context,
	request *dv1beta.QueryDeploymentsRequest,
	_ ...grpc.CallOption,
) (*dv1beta.QueryDeploymentsResponse, error) {
	copyRequest := *request
	if request.Pagination != nil {
		copyPagination := *request.Pagination
		copyPagination.Key = append([]byte(nil), request.Pagination.Key...)
		copyRequest.Pagination = &copyPagination
	}
	stub.requests = append(stub.requests, &copyRequest)

	index := len(stub.requests) - 1
	if index < len(stub.errors) && stub.errors[index] != nil {
		return nil, stub.errors[index]
	}
	if index >= len(stub.responses) {
		return nil, fmt.Errorf("unexpected deployments page %d", index+1)
	}
	return stub.responses[index], nil
}

type marketPagesStub struct {
	mv1beta.QueryClient
	leaseRequests  []*mv1beta.QueryLeasesRequest
	leaseResponses []*mv1beta.QueryLeasesResponse
	leaseErrors    []error
	bidRequests    []*mv1beta.QueryBidsRequest
	bidResponses   []*mv1beta.QueryBidsResponse
	bidErrors      []error
}

func (stub *marketPagesStub) Leases(
	_ context.Context,
	request *mv1beta.QueryLeasesRequest,
	_ ...grpc.CallOption,
) (*mv1beta.QueryLeasesResponse, error) {
	copyRequest := *request
	if request.Pagination != nil {
		copyPagination := *request.Pagination
		copyPagination.Key = append([]byte(nil), request.Pagination.Key...)
		copyRequest.Pagination = &copyPagination
	}
	stub.leaseRequests = append(stub.leaseRequests, &copyRequest)

	index := len(stub.leaseRequests) - 1
	if index < len(stub.leaseErrors) && stub.leaseErrors[index] != nil {
		return nil, stub.leaseErrors[index]
	}
	if index >= len(stub.leaseResponses) {
		return nil, fmt.Errorf("unexpected leases page %d", index+1)
	}
	return stub.leaseResponses[index], nil
}

func (stub *marketPagesStub) Bids(
	_ context.Context,
	request *mv1beta.QueryBidsRequest,
	_ ...grpc.CallOption,
) (*mv1beta.QueryBidsResponse, error) {
	copyRequest := *request
	if request.Pagination != nil {
		copyPagination := *request.Pagination
		copyPagination.Key = append([]byte(nil), request.Pagination.Key...)
		copyRequest.Pagination = &copyPagination
	}
	stub.bidRequests = append(stub.bidRequests, &copyRequest)

	index := len(stub.bidRequests) - 1
	if index < len(stub.bidErrors) && stub.bidErrors[index] != nil {
		return nil, stub.bidErrors[index]
	}
	if index >= len(stub.bidResponses) {
		return nil, fmt.Errorf("unexpected bids page %d", index+1)
	}
	return stub.bidResponses[index], nil
}

type providerQueryStub struct {
	ptypes.QueryClient
	owners []string
}

func (stub *providerQueryStub) Provider(
	_ context.Context,
	request *ptypes.QueryProviderRequest,
	_ ...grpc.CallOption,
) (*ptypes.QueryProviderResponse, error) {
	stub.owners = append(stub.owners, request.Owner)
	return &ptypes.QueryProviderResponse{Provider: ptypes.Provider{
		Owner: request.Owner,
		Attributes: attrv1.Attributes{
			{Key: "region", Value: "us-west"},
		},
	}}, nil
}

type auditQueryStub struct {
	atypes.QueryClient
	owners []string
}

func (stub *auditQueryStub) ProviderAttributes(
	_ context.Context,
	request *atypes.QueryProviderAttributesRequest,
	_ ...grpc.CallOption,
) (*atypes.QueryProvidersResponse, error) {
	stub.owners = append(stub.owners, request.Owner)
	return &atypes.QueryProvidersResponse{
		Providers: atypes.AuditedProviders{{Owner: request.Owner, Auditor: "akash1auditor"}},
	}, nil
}

func TestChainQuerierCurrentHeightRejectsStaleAndFailedNodes(t *testing.T) {
	query := func(info *tmrpc.SyncInfo, err error) (int64, error) {
		return NewChainQuerier(&syncLightClientStub{
			node: &syncNodeQueryStub{info: info, err: err},
		}).CurrentHeight(context.Background())
	}

	height, err := query(&tmrpc.SyncInfo{LatestBlockHeight: 712}, nil)
	if err != nil || height != 712 {
		t.Fatalf("ready node height = %d, err = %v; want 712", height, err)
	}

	height, err = query(&tmrpc.SyncInfo{LatestBlockHeight: 700, CatchingUp: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "catching up") || height != 0 {
		t.Fatalf("catching-up node height = %d, err = %v; want a stale-node rejection", height, err)
	}

	sentinel := errors.New("rpc unavailable")
	height, err = query(nil, sentinel)
	if !errors.Is(err, sentinel) || height != 0 {
		t.Fatalf("failed node height = %d, err = %v; want wrapped sentinel", height, err)
	}
}

func TestChainQuerierWalksEveryPageWithExactFilters(t *testing.T) {
	owner := querierOwner
	provider := "akash1provider"
	deploymentQuery := &deploymentPagesStub{responses: []*dv1beta.QueryDeploymentsResponse{
		{
			Deployments: dv1beta.DeploymentResponses{{Deployment: dv1.Deployment{
				ID: dv1.DeploymentID{Owner: owner, DSeq: 10}, State: dv1.DeploymentActive,
			}}},
			Pagination: &query.PageResponse{NextKey: []byte("deployment-next")},
		},
		{
			Deployments: dv1beta.DeploymentResponses{{Deployment: dv1.Deployment{
				ID: dv1.DeploymentID{Owner: owner, DSeq: 20}, State: dv1.DeploymentClosed,
			}}},
		},
	}}
	marketQuery := &marketPagesStub{
		leaseResponses: []*mv1beta.QueryLeasesResponse{
			{
				Leases: []mv1beta.QueryLeaseResponse{{Lease: mv1.Lease{
					ID:    mv1.LeaseID{Owner: owner, DSeq: 10, GSeq: 1, OSeq: 1, Provider: provider},
					State: mv1.LeaseActive,
					Price: sdk.NewDecCoin("uakt", math.NewInt(7)),
				}}},
				Pagination: &query.PageResponse{NextKey: []byte("lease-next")},
			},
			{Leases: []mv1beta.QueryLeaseResponse{{Lease: mv1.Lease{
				ID:    mv1.LeaseID{Owner: owner, DSeq: 10, GSeq: 1, OSeq: 2, Provider: provider},
				State: mv1.LeaseClosed,
				Price: sdk.NewDecCoin("uakt", math.NewInt(8)),
			}}}},
		},
		bidResponses: []*mv1beta.QueryBidsResponse{
			{
				Bids: []mv1beta.QueryBidResponse{{Bid: mv1beta.Bid{
					ID:    mv1.BidID{Owner: owner, DSeq: 10, GSeq: 1, OSeq: 1, Provider: provider},
					State: mv1beta.BidOpen,
					Price: sdk.NewDecCoin("uakt", math.NewInt(6)),
				}}},
				Pagination: &query.PageResponse{NextKey: []byte("bid-next")},
			},
			{},
		},
	}
	providerQuery := &providerQueryStub{}
	auditQuery := &auditQueryStub{}
	querier := NewChainQuerier(&syncLightClientStub{query: &syncQueryClientStub{
		deployment: deploymentQuery,
		market:     marketQuery,
		provider:   providerQuery,
		audit:      auditQuery,
	}})

	deployments, err := querier.Deployments(context.Background(), owner)
	if err != nil {
		t.Fatalf("Deployments: %v", err)
	}
	if len(deployments) != 2 || deployments[0].DSeq != 10 || deployments[1].DSeq != 20 {
		t.Fatalf("deployments = %#v, want both pages in order", deployments)
	}
	assertDeploymentPageRequests(t, deploymentQuery.requests, owner, []string{"", "deployment-next"})

	leases, err := querier.Leases(context.Background(), owner, 10)
	if err != nil {
		t.Fatalf("Leases: %v", err)
	}
	if len(leases) != 2 || leases[0].ID.OSeq != 1 || leases[1].ID.OSeq != 2 {
		t.Fatalf("leases = %#v, want both pages in order", leases)
	}
	assertLeasePageRequests(t, marketQuery.leaseRequests, owner, 10, []string{"", "lease-next"})

	bids, err := querier.Bids(context.Background(), owner, 10)
	if err != nil {
		t.Fatalf("Bids: %v", err)
	}
	if len(bids) != 1 || bids[0].ProviderAttributes["region"] != "us-west" || !bids[0].ProviderAudited {
		t.Fatalf("bids = %#v, want the paged bid enriched from live provider metadata", bids)
	}
	assertBidPageRequests(t, marketQuery.bidRequests, owner, 10, []string{"", "bid-next"})
	if len(providerQuery.owners) != 1 || providerQuery.owners[0] != provider {
		t.Errorf("provider metadata calls = %v, want [%s]", providerQuery.owners, provider)
	}
	if len(auditQuery.owners) != 1 || auditQuery.owners[0] != provider {
		t.Errorf("audit metadata calls = %v, want [%s]", auditQuery.owners, provider)
	}
}

func TestChainQuerierPropagatesPageFailuresWithoutPartialResults(t *testing.T) {
	sentinel := errors.New("query interrupted")
	tests := []struct {
		name string
		run  func(Querier) (int, error)
		stub *syncQueryClientStub
	}{
		{
			name: "deployments",
			run: func(querier Querier) (int, error) {
				records, err := querier.Deployments(context.Background(), querierOwner)
				return len(records), err
			},
			stub: &syncQueryClientStub{deployment: &deploymentPagesStub{errors: []error{sentinel}}},
		},
		{
			name: "leases",
			run: func(querier Querier) (int, error) {
				records, err := querier.Leases(context.Background(), querierOwner, 1)
				return len(records), err
			},
			stub: &syncQueryClientStub{market: &marketPagesStub{leaseErrors: []error{sentinel}}},
		},
		{
			name: "bids",
			run: func(querier Querier) (int, error) {
				records, err := querier.Bids(context.Background(), querierOwner, 1)
				return len(records), err
			},
			stub: &syncQueryClientStub{market: &marketPagesStub{bidErrors: []error{sentinel}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			querier := NewChainQuerier(&syncLightClientStub{query: test.stub})
			count, err := test.run(querier)
			if !errors.Is(err, sentinel) || count != 0 {
				t.Fatalf("count = %d, err = %v; want no partial results and wrapped sentinel", count, err)
			}
		})
	}
}

func TestChainQuerierRejectsRepeatedPaginationKeys(t *testing.T) {
	deploymentPages := &deploymentPagesStub{responses: []*dv1beta.QueryDeploymentsResponse{
		{Pagination: &query.PageResponse{NextKey: []byte("loop")}},
		{Pagination: &query.PageResponse{NextKey: []byte("loop")}},
	}}
	leasePages := &marketPagesStub{leaseResponses: []*mv1beta.QueryLeasesResponse{
		{Pagination: &query.PageResponse{NextKey: []byte("loop")}},
		{Pagination: &query.PageResponse{NextKey: []byte("loop")}},
	}}
	bidPages := &marketPagesStub{bidResponses: []*mv1beta.QueryBidsResponse{
		{Pagination: &query.PageResponse{NextKey: []byte("loop")}},
		{Pagination: &query.PageResponse{NextKey: []byte("loop")}},
	}}

	tests := []struct {
		name  string
		stub  *syncQueryClientStub
		run   func(Querier) error
		calls func() int
	}{
		{
			name: "deployments",
			stub: &syncQueryClientStub{deployment: deploymentPages},
			run: func(querier Querier) error {
				_, err := querier.Deployments(context.Background(), querierOwner)
				return err
			},
			calls: func() int { return len(deploymentPages.requests) },
		},
		{
			name: "leases",
			stub: &syncQueryClientStub{market: leasePages},
			run: func(querier Querier) error {
				_, err := querier.Leases(context.Background(), querierOwner, 1)
				return err
			},
			calls: func() int { return len(leasePages.leaseRequests) },
		},
		{
			name: "bids",
			stub: &syncQueryClientStub{market: bidPages},
			run: func(querier Querier) error {
				_, err := querier.Bids(context.Background(), querierOwner, 1)
				return err
			},
			calls: func() int { return len(bidPages.bidRequests) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			querier := NewChainQuerier(&syncLightClientStub{query: test.stub})
			err := test.run(querier)
			if err == nil || !strings.Contains(err.Error(), "repeated pagination key") {
				t.Fatalf("error = %v, want repeated pagination key rejection", err)
			}
			if calls := test.calls(); calls != 2 {
				t.Fatalf("page requests = %d, want exactly two before rejecting the repeated key", calls)
			}
		})
	}
}

func assertDeploymentPageRequests(
	t *testing.T,
	requests []*dv1beta.QueryDeploymentsRequest,
	owner string,
	keys []string,
) {
	t.Helper()
	if len(requests) != len(keys) {
		t.Fatalf("deployment requests = %d, want %d", len(requests), len(keys))
	}
	for index, request := range requests {
		if request.Filters.Owner != owner || request.Pagination.Limit != queryPageLimit ||
			string(request.Pagination.Key) != keys[index] {
			t.Errorf("deployment request %d = %#v", index, request)
		}
	}
}

func assertLeasePageRequests(
	t *testing.T,
	requests []*mv1beta.QueryLeasesRequest,
	owner string,
	dseq uint64,
	keys []string,
) {
	t.Helper()
	if len(requests) != len(keys) {
		t.Fatalf("lease requests = %d, want %d", len(requests), len(keys))
	}
	for index, request := range requests {
		if request.Filters.Owner != owner || request.Filters.DSeq != dseq ||
			request.Pagination.Limit != queryPageLimit || string(request.Pagination.Key) != keys[index] {
			t.Errorf("lease request %d = %#v", index, request)
		}
	}
}

func assertBidPageRequests(
	t *testing.T,
	requests []*mv1beta.QueryBidsRequest,
	owner string,
	dseq uint64,
	keys []string,
) {
	t.Helper()
	if len(requests) != len(keys) {
		t.Fatalf("bid requests = %d, want %d", len(requests), len(keys))
	}
	for index, request := range requests {
		if request.Filters.Owner != owner || request.Filters.DSeq != dseq ||
			request.Pagination.Limit != queryPageLimit || string(request.Pagination.Key) != keys[index] {
			t.Errorf("bid request %d = %#v", index, request)
		}
	}
}
