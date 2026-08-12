package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
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

type queryRailClient struct {
	aclient.Client
	query cv1beta3.QueryClient
}

func (client *queryRailClient) Query() cv1beta3.QueryClient { return client.query }
func (*queryRailClient) ClientContext() sdkclient.Context   { return sdkclient.Context{} }

type queryRailModules struct {
	cv1beta3.QueryClient
	market     mv1beta.QueryClient
	deployment dv1beta.QueryClient
	provider   ptypes.QueryClient
	audit      atypes.QueryClient
}

func (modules *queryRailModules) Market() mv1beta.QueryClient     { return modules.market }
func (modules *queryRailModules) Deployment() dv1beta.QueryClient { return modules.deployment }
func (modules *queryRailModules) Provider() ptypes.QueryClient    { return modules.provider }
func (modules *queryRailModules) Audit() atypes.QueryClient       { return modules.audit }
func (*queryRailModules) ClientContext() sdkclient.Context        { return sdkclient.Context{} }

type marketQueryRailStub struct {
	mv1beta.QueryClient
	bidsRequest    *mv1beta.QueryBidsRequest
	bidsResponse   *mv1beta.QueryBidsResponse
	bidsErr        error
	leasesRequest  *mv1beta.QueryLeasesRequest
	leasesResponse *mv1beta.QueryLeasesResponse
	leasesErr      error
}

func (stub *marketQueryRailStub) Bids(
	_ context.Context,
	request *mv1beta.QueryBidsRequest,
	_ ...grpc.CallOption,
) (*mv1beta.QueryBidsResponse, error) {
	copyRequest := *request
	stub.bidsRequest = &copyRequest
	return stub.bidsResponse, stub.bidsErr
}

func (stub *marketQueryRailStub) Leases(
	_ context.Context,
	request *mv1beta.QueryLeasesRequest,
	_ ...grpc.CallOption,
) (*mv1beta.QueryLeasesResponse, error) {
	copyRequest := *request
	stub.leasesRequest = &copyRequest
	return stub.leasesResponse, stub.leasesErr
}

type deploymentQueryRailStub struct {
	dv1beta.QueryClient
	request  *dv1beta.QueryDeploymentsRequest
	response *dv1beta.QueryDeploymentsResponse
	err      error
}

func (stub *deploymentQueryRailStub) Deployments(
	_ context.Context,
	request *dv1beta.QueryDeploymentsRequest,
	_ ...grpc.CallOption,
) (*dv1beta.QueryDeploymentsResponse, error) {
	copyRequest := *request
	stub.request = &copyRequest
	return stub.response, stub.err
}

type providerQueryRailStub struct {
	ptypes.QueryClient
	owners []string
}

func (stub *providerQueryRailStub) Provider(
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

type auditQueryRailStub struct {
	atypes.QueryClient
	owners []string
}

func (stub *auditQueryRailStub) ProviderAttributes(
	_ context.Context,
	request *atypes.QueryProviderAttributesRequest,
	_ ...grpc.CallOption,
) (*atypes.QueryProvidersResponse, error) {
	stub.owners = append(stub.owners, request.Owner)
	return &atypes.QueryProvidersResponse{
		Providers: atypes.AuditedProviders{{Owner: request.Owner, Auditor: "akash1auditor"}},
	}, nil
}

func TestChainQueryRailUsesExactFiltersAndStableResponseShapes(t *testing.T) {
	owner := "akash1owner"
	provider := "akash1provider"
	market := &marketQueryRailStub{
		bidsResponse: &mv1beta.QueryBidsResponse{Bids: []mv1beta.QueryBidResponse{{Bid: mv1beta.Bid{
			ID: mv1.BidID{
				Owner: owner, DSeq: 42, GSeq: 2, OSeq: 3, Provider: provider,
			},
			State: mv1beta.BidOpen,
			Price: sdk.NewDecCoin("uakt", math.NewInt(5)),
		}}}},
		leasesResponse: &mv1beta.QueryLeasesResponse{Leases: []mv1beta.QueryLeaseResponse{{Lease: mv1.Lease{
			ID: mv1.LeaseID{
				Owner: owner, DSeq: 42, GSeq: 2, OSeq: 3, Provider: provider,
			},
			State: mv1.LeaseActive,
			Price: sdk.NewDecCoin("uakt", math.NewInt(6)),
		}}}},
	}
	deployment := &deploymentQueryRailStub{response: &dv1beta.QueryDeploymentsResponse{
		Deployments: dv1beta.DeploymentResponses{{Deployment: dv1.Deployment{
			ID: dv1.DeploymentID{Owner: owner, DSeq: 42}, State: dv1.DeploymentActive,
		}}},
	}}
	providerQuery := &providerQueryRailStub{}
	auditQuery := &auditQueryRailStub{}
	client := NewChainClient(&queryRailClient{query: &queryRailModules{
		market: market, deployment: deployment, provider: providerQuery, audit: auditQuery,
	}})

	bidJSON, err := client.Query(context.Background(), queryMarketBids, map[string]string{
		"owner": owner, "provider": provider, "state": "open",
		"dseq": "42", "gseq": "2", "oseq": "3",
	})
	if err != nil {
		t.Fatalf("query bids: %v", err)
	}
	if market.bidsRequest == nil || market.bidsRequest.Filters != (mv1beta.BidFilters{
		Owner: owner, DSeq: 42, GSeq: 2, OSeq: 3, Provider: provider, State: "open",
	}) {
		t.Fatalf("bid filters = %#v", market.bidsRequest)
	}
	var bidDocument struct {
		Bids             []json.RawMessage `json:"bids"`
		ProviderMetadata map[string]struct {
			Attributes map[string]string `json:"attributes"`
			Audited    bool              `json:"audited"`
		} `json:"provider_metadata"`
	}
	if err := json.Unmarshal(bidJSON, &bidDocument); err != nil {
		t.Fatalf("decode bids: %v", err)
	}
	metadata := bidDocument.ProviderMetadata[provider]
	if len(bidDocument.Bids) != 1 || metadata.Attributes["region"] != "us-west" || !metadata.Audited {
		t.Fatalf("bid response = %s", bidJSON)
	}
	if len(providerQuery.owners) != 1 || providerQuery.owners[0] != provider ||
		len(auditQuery.owners) != 1 || auditQuery.owners[0] != provider {
		t.Fatalf("metadata calls = provider %v audit %v", providerQuery.owners, auditQuery.owners)
	}

	leaseJSON, err := client.Query(context.Background(), queryMarketLeases, map[string]string{
		"owner": owner, "provider": provider, "state": "active",
		"dseq": "42", "gseq": "2", "oseq": "3",
	})
	if err != nil {
		t.Fatalf("query leases: %v", err)
	}
	if market.leasesRequest == nil || market.leasesRequest.Filters != (mv1.LeaseFilters{
		Owner: owner, DSeq: 42, GSeq: 2, OSeq: 3, Provider: provider, State: "active",
	}) {
		t.Fatalf("lease filters = %#v", market.leasesRequest)
	}
	assertTopLevelArray(t, leaseJSON, "leases", 1)

	deploymentJSON, err := client.Query(context.Background(), queryDeployments, map[string]string{
		"owner": owner, "state": "active", "dseq": "42",
	})
	if err != nil {
		t.Fatalf("query deployments: %v", err)
	}
	if deployment.request == nil || deployment.request.Filters != (dv1beta.DeploymentFilters{
		Owner: owner, DSeq: 42, State: "active",
	}) {
		t.Fatalf("deployment filters = %#v", deployment.request)
	}
	assertTopLevelArray(t, deploymentJSON, "deployments", 1)
}

func TestChainQueryRailWrapsTransportFailuresWithQueryIdentity(t *testing.T) {
	sentinel := errors.New("grpc unavailable")
	tests := []struct {
		path    string
		modules *queryRailModules
	}{
		{
			path: queryMarketBids,
			modules: &queryRailModules{market: &marketQueryRailStub{
				bidsErr: sentinel,
			}},
		},
		{
			path: queryMarketLeases,
			modules: &queryRailModules{market: &marketQueryRailStub{
				leasesErr: sentinel,
			}},
		},
		{
			path: queryDeployments,
			modules: &queryRailModules{deployment: &deploymentQueryRailStub{
				err: sentinel,
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			client := NewChainClient(&queryRailClient{query: test.modules})
			_, err := client.Query(context.Background(), test.path, nil)
			if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "query "+test.path) {
				t.Fatalf("error = %v, want path-scoped transport failure", err)
			}
		})
	}
}

func assertTopLevelArray(t *testing.T, raw json.RawMessage, field string, want int) {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(document[field], &values); err != nil {
		t.Fatalf("decode %s from %s: %v", field, raw, err)
	}
	if len(values) != want {
		t.Fatalf("%s count = %d, want %d", field, len(values), want)
	}
}
