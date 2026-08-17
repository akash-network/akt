package mcp

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	protocol "github.com/mark3labs/mcp-go/mcp"

	atypes "pkg.akt.dev/go/node/audit/v1"
	ctypes "pkg.akt.dev/go/node/cert/v1"
	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	audittools "pkg.akt.dev/akt/internal/mcp/tools/audit"
	banktools "pkg.akt.dev/akt/internal/mcp/tools/bank"
	certtools "pkg.akt.dev/akt/internal/mcp/tools/cert"
	deploymenttools "pkg.akt.dev/akt/internal/mcp/tools/deployment"
	markettools "pkg.akt.dev/akt/internal/mcp/tools/market"
	providertools "pkg.akt.dev/akt/internal/mcp/tools/provider"
)

type semanticClient struct {
	query v1beta3.QueryClient
	tx    v1beta3.TxClient
	cctx  sdkclient.Context
}

func (client semanticClient) Query() v1beta3.QueryClient       { return client.query }
func (semanticClient) Node() v1beta3.NodeClient                { return nil }
func (client semanticClient) Tx() v1beta3.TxClient             { return client.tx }
func (client semanticClient) ClientContext() sdkclient.Context { return client.cctx }
func (semanticClient) PrintMessage(interface{}) error          { return nil }
func (semanticClient) PrintJSON(interface{}) error             { return nil }

type semanticQuery struct {
	v1beta3.QueryClient
	bank       banktypes.QueryClient
	audit      atypes.QueryClient
	cert       ctypes.QueryClient
	deployment dtypes.QueryClient
	market     mtypes.QueryClient
	provider   ptypes.QueryClient
}

func (query semanticQuery) Bank() banktypes.QueryClient    { return query.bank }
func (query semanticQuery) Audit() atypes.QueryClient      { return query.audit }
func (query semanticQuery) Certs() ctypes.QueryClient      { return query.cert }
func (query semanticQuery) Deployment() dtypes.QueryClient { return query.deployment }
func (query semanticQuery) Market() mtypes.QueryClient     { return query.market }
func (query semanticQuery) Provider() ptypes.QueryClient   { return query.provider }
func (semanticQuery) ClientContext() sdkclient.Context     { return sdkclient.Context{} }

type bankQueryStub struct {
	banktypes.QueryClient
	method  string
	balance *banktypes.QueryBalanceRequest
	all     *banktypes.QueryAllBalancesRequest
	err     error
}

func (stub *bankQueryStub) Balance(_ context.Context, req *banktypes.QueryBalanceRequest, _ ...grpc.CallOption) (*banktypes.QueryBalanceResponse, error) {
	stub.method, stub.balance = "balance", req
	return &banktypes.QueryBalanceResponse{}, stub.err
}

func (stub *bankQueryStub) AllBalances(_ context.Context, req *banktypes.QueryAllBalancesRequest, _ ...grpc.CallOption) (*banktypes.QueryAllBalancesResponse, error) {
	stub.method, stub.all = "all", req
	return &banktypes.QueryAllBalancesResponse{}, stub.err
}

type auditQueryStub struct {
	atypes.QueryClient
	method  string
	owner   string
	auditor string
	err     error
}

func (stub *auditQueryStub) AllProvidersAttributes(context.Context, *atypes.QueryAllProvidersAttributesRequest, ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	stub.method, stub.owner, stub.auditor = "all", "", ""
	return &atypes.QueryProvidersResponse{}, stub.err
}

func (stub *auditQueryStub) ProviderAttributes(_ context.Context, req *atypes.QueryProviderAttributesRequest, _ ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	stub.method, stub.owner, stub.auditor = "owner", req.Owner, ""
	return &atypes.QueryProvidersResponse{}, stub.err
}

func (stub *auditQueryStub) ProviderAuditorAttributes(_ context.Context, req *atypes.QueryProviderAuditorRequest, _ ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	stub.method, stub.owner, stub.auditor = "owner+auditor", req.Owner, req.Auditor
	return &atypes.QueryProvidersResponse{}, stub.err
}

func (stub *auditQueryStub) AuditorAttributes(_ context.Context, req *atypes.QueryAuditorAttributesRequest, _ ...grpc.CallOption) (*atypes.QueryProvidersResponse, error) {
	stub.method, stub.owner, stub.auditor = "auditor", "", req.Auditor
	return &atypes.QueryProvidersResponse{}, stub.err
}

type certQueryStub struct {
	ctypes.QueryClient
	request *ctypes.QueryCertificatesRequest
	err     error
}

func (stub *certQueryStub) Certificates(_ context.Context, req *ctypes.QueryCertificatesRequest, _ ...grpc.CallOption) (*ctypes.QueryCertificatesResponse, error) {
	stub.request = req
	return &ctypes.QueryCertificatesResponse{}, stub.err
}

type deploymentQueryStub struct {
	dtypes.QueryClient
	list  *dtypes.QueryDeploymentsRequest
	get   *dtypes.QueryDeploymentRequest
	group *dtypes.QueryGroupRequest
	err   error
}

func (stub *deploymentQueryStub) Deployments(_ context.Context, req *dtypes.QueryDeploymentsRequest, _ ...grpc.CallOption) (*dtypes.QueryDeploymentsResponse, error) {
	stub.list = req
	return &dtypes.QueryDeploymentsResponse{}, stub.err
}

func (stub *deploymentQueryStub) Deployment(_ context.Context, req *dtypes.QueryDeploymentRequest, _ ...grpc.CallOption) (*dtypes.QueryDeploymentResponse, error) {
	stub.get = req
	return &dtypes.QueryDeploymentResponse{}, stub.err
}

func (stub *deploymentQueryStub) Group(_ context.Context, req *dtypes.QueryGroupRequest, _ ...grpc.CallOption) (*dtypes.QueryGroupResponse, error) {
	stub.group = req
	return &dtypes.QueryGroupResponse{}, stub.err
}

type marketQueryStub struct {
	mtypes.QueryClient
	orders *mtypes.QueryOrdersRequest
	order  *mtypes.QueryOrderRequest
	bids   *mtypes.QueryBidsRequest
	bid    *mtypes.QueryBidRequest
	leases *mtypes.QueryLeasesRequest
	lease  *mtypes.QueryLeaseRequest
	err    error
}

func (stub *marketQueryStub) Orders(_ context.Context, req *mtypes.QueryOrdersRequest, _ ...grpc.CallOption) (*mtypes.QueryOrdersResponse, error) {
	stub.orders = req
	return &mtypes.QueryOrdersResponse{}, stub.err
}

func (stub *marketQueryStub) Order(_ context.Context, req *mtypes.QueryOrderRequest, _ ...grpc.CallOption) (*mtypes.QueryOrderResponse, error) {
	stub.order = req
	return &mtypes.QueryOrderResponse{}, stub.err
}

func (stub *marketQueryStub) Bids(_ context.Context, req *mtypes.QueryBidsRequest, _ ...grpc.CallOption) (*mtypes.QueryBidsResponse, error) {
	stub.bids = req
	return &mtypes.QueryBidsResponse{}, stub.err
}

func (stub *marketQueryStub) Bid(_ context.Context, req *mtypes.QueryBidRequest, _ ...grpc.CallOption) (*mtypes.QueryBidResponse, error) {
	stub.bid = req
	return &mtypes.QueryBidResponse{}, stub.err
}

func (stub *marketQueryStub) Leases(_ context.Context, req *mtypes.QueryLeasesRequest, _ ...grpc.CallOption) (*mtypes.QueryLeasesResponse, error) {
	stub.leases = req
	return &mtypes.QueryLeasesResponse{}, stub.err
}

func (stub *marketQueryStub) Lease(_ context.Context, req *mtypes.QueryLeaseRequest, _ ...grpc.CallOption) (*mtypes.QueryLeaseResponse, error) {
	stub.lease = req
	return &mtypes.QueryLeaseResponse{}, stub.err
}

type providerQueryStub struct {
	ptypes.QueryClient
	list *ptypes.QueryProvidersRequest
	get  *ptypes.QueryProviderRequest
	err  error
}

func (stub *providerQueryStub) Providers(_ context.Context, req *ptypes.QueryProvidersRequest, _ ...grpc.CallOption) (*ptypes.QueryProvidersResponse, error) {
	stub.list = req
	return &ptypes.QueryProvidersResponse{}, stub.err
}

func (stub *providerQueryStub) Provider(_ context.Context, req *ptypes.QueryProviderRequest, _ ...grpc.CallOption) (*ptypes.QueryProviderResponse, error) {
	stub.get = req
	return &ptypes.QueryProviderResponse{}, stub.err
}

type txStub struct {
	v1beta3.TxClient
	messages []sdk.Msg
	err      error
}

func (stub *txStub) BroadcastMsgs(_ context.Context, messages []sdk.Msg, _ ...v1beta3.BroadcastOption) (interface{}, error) {
	stub.messages = append([]sdk.Msg(nil), messages...)
	return map[string]string{"txhash": "ABC"}, stub.err
}

func toolRequest(arguments map[string]any) protocol.CallToolRequest {
	request := protocol.CallToolRequest{}
	request.Params.Arguments = arguments
	return request
}

func requireToolSuccess(t *testing.T, result *protocol.CallToolResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("handler transport error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handler result = %#v, want success", result)
	}
}

func requireToolErrorContains(t *testing.T, result *protocol.CallToolResult, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("handler transport error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("handler result = %#v, want MCP error", result)
	}
	var text string
	for _, content := range result.Content {
		if value, ok := content.(protocol.TextContent); ok {
			text += value.Text
		}
	}
	if !strings.Contains(text, want) {
		t.Fatalf("handler error = %q, want %q", text, want)
	}
}

func TestBankHandlersRouteDenomAndAllBalanceQueries(t *testing.T) {
	bank := &bankQueryStub{}
	client := semanticClient{query: semanticQuery{bank: bank}}

	result, err := banktools.HandleAccountBalance(client)(context.Background(), toolRequest(map[string]any{
		"address": "akash1owner", "denom": "uakt",
	}))
	requireToolSuccess(t, result, err)
	if bank.method != "balance" || bank.balance.Address != "akash1owner" || bank.balance.Denom != "uakt" {
		t.Fatalf("balance request = %#v", bank.balance)
	}

	result, err = banktools.HandleAccountBalance(client)(context.Background(), toolRequest(map[string]any{
		"address": "akash1owner",
	}))
	requireToolSuccess(t, result, err)
	if bank.method != "all" || bank.all.Address != "akash1owner" {
		t.Fatalf("all balances request = %#v", bank.all)
	}

	bank.err = errors.New("bank unavailable")
	result, err = banktools.HandleAccountBalance(client)(context.Background(), toolRequest(map[string]any{
		"address": "akash1owner",
	}))
	requireToolErrorContains(t, result, err, "failed to query balances: bank unavailable")
}

func TestAuditHandlerSelectsQueryFromFilters(t *testing.T) {
	audit := &auditQueryStub{}
	client := semanticClient{query: semanticQuery{audit: audit}}
	tests := []struct {
		name    string
		args    map[string]any
		method  string
		owner   string
		auditor string
	}{
		{name: "all", args: map[string]any{}, method: "all"},
		{name: "owner", args: map[string]any{"owner": "akash1owner"}, method: "owner", owner: "akash1owner"},
		{name: "auditor", args: map[string]any{"auditor": "akash1auditor"}, method: "auditor", auditor: "akash1auditor"},
		{name: "both", args: map[string]any{"owner": "akash1owner", "auditor": "akash1auditor"}, method: "owner+auditor", owner: "akash1owner", auditor: "akash1auditor"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := audittools.HandleListAuditedProviders(client)(context.Background(), toolRequest(tc.args))
			requireToolSuccess(t, result, err)
			if audit.method != tc.method || audit.owner != tc.owner || audit.auditor != tc.auditor {
				t.Fatalf("audit route = %s owner=%s auditor=%s", audit.method, audit.owner, audit.auditor)
			}
		})
	}

	audit.err = errors.New("audit unavailable")
	result, err := audittools.HandleListAuditedProviders(client)(context.Background(), toolRequest(map[string]any{}))
	requireToolErrorContains(t, result, err, "failed to list audited providers: audit unavailable")
}

func TestCertificateHandlerSendsExactFilter(t *testing.T) {
	cert := &certQueryStub{}
	client := semanticClient{query: semanticQuery{cert: cert}}
	result, err := certtools.HandleListCertificates(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "state": "valid",
	}))
	requireToolSuccess(t, result, err)
	if cert.request == nil || cert.request.Filter.Owner != "akash1owner" || cert.request.Filter.State != "valid" {
		t.Fatalf("certificate request = %#v", cert.request)
	}

	cert.err = errors.New("cert unavailable")
	result, err = certtools.HandleListCertificates(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner",
	}))
	requireToolErrorContains(t, result, err, "failed to list certificates: cert unavailable")
}

func TestDeploymentHandlersSendExactIDsAndTransaction(t *testing.T) {
	deployment := &deploymentQueryStub{}
	tx := &txStub{}
	cctx := sdkclient.Context{}.WithFromAddress(sdk.AccAddress(bytes.Repeat([]byte{1}, 20)))
	client := semanticClient{query: semanticQuery{deployment: deployment}, tx: tx, cctx: cctx}

	result, err := deploymenttools.HandleListDeployments(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "state": "active",
	}))
	requireToolSuccess(t, result, err)
	if deployment.list == nil || deployment.list.Filters.Owner != "akash1owner" || deployment.list.Filters.State != "active" {
		t.Fatalf("deployment list request = %#v", deployment.list)
	}

	result, err = deploymenttools.HandleGetDeployment(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "dseq": float64(7),
	}))
	requireToolSuccess(t, result, err)
	if deployment.get == nil || deployment.get.ID != (dv1.DeploymentID{Owner: "akash1owner", DSeq: 7}) {
		t.Fatalf("deployment get request = %#v", deployment.get)
	}

	result, err = deploymenttools.HandleGetGroup(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "dseq": float64(7), "gseq": float64(2),
	}))
	requireToolSuccess(t, result, err)
	if deployment.group == nil || deployment.group.ID != (dv1.GroupID{Owner: "akash1owner", DSeq: 7, GSeq: 2}) {
		t.Fatalf("deployment group request = %#v", deployment.group)
	}

	result, err = deploymenttools.HandleCloseDeployment(client)(context.Background(), toolRequest(map[string]any{"dseq": float64(7)}))
	requireToolSuccess(t, result, err)
	if len(tx.messages) != 1 {
		t.Fatalf("close deployment messages = %#v", tx.messages)
	}
	closeMessage, ok := tx.messages[0].(*dtypes.MsgCloseDeployment)
	if !ok || closeMessage.ID.DSeq != 7 || closeMessage.ID.Owner == "" {
		t.Fatalf("close deployment message = %#v", tx.messages[0])
	}

	deployment.err = errors.New("deployment unavailable")
	result, err = deploymenttools.HandleGetDeployment(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "dseq": float64(7),
	}))
	requireToolErrorContains(t, result, err, "failed to get deployment: deployment unavailable")
	result, err = deploymenttools.HandleListDeployments(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner",
	}))
	requireToolErrorContains(t, result, err, "failed to list deployments: deployment unavailable")
	result, err = deploymenttools.HandleGetGroup(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "dseq": float64(7), "gseq": float64(2),
	}))
	requireToolErrorContains(t, result, err, "failed to get group: deployment unavailable")
	result, err = deploymenttools.HandleGetDeployment(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner",
	}))
	requireToolErrorContains(t, result, err, "missing required parameter: dseq")

	tx.err = errors.New("broadcast refused")
	result, err = deploymenttools.HandleCloseDeployment(client)(context.Background(), toolRequest(map[string]any{"dseq": float64(7)}))
	requireToolErrorContains(t, result, err, "failed to close deployment: broadcast refused")
}

func TestMarketHandlersSendExactIDsFiltersAndTransactions(t *testing.T) {
	market := &marketQueryStub{}
	tx := &txStub{}
	client := semanticClient{query: semanticQuery{market: market}, tx: tx}
	identity := map[string]any{
		"owner": "akash1owner", "dseq": float64(7), "gseq": float64(2), "oseq": float64(3), "provider": "akash1provider",
	}

	result, err := markettools.HandleListOrders(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "state": "open",
	}))
	requireToolSuccess(t, result, err)
	if market.orders == nil || market.orders.Filters.Owner != "akash1owner" || market.orders.Filters.State != "open" {
		t.Fatalf("orders request = %#v", market.orders)
	}

	result, err = markettools.HandleGetOrder(client)(context.Background(), toolRequest(identity))
	requireToolSuccess(t, result, err)
	if market.order == nil || market.order.ID != (mv1.OrderID{Owner: "akash1owner", DSeq: 7, GSeq: 2, OSeq: 3}) {
		t.Fatalf("order request = %#v", market.order)
	}

	result, err = markettools.HandleListBids(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "dseq": float64(7), "state": "open",
	}))
	requireToolSuccess(t, result, err)
	if market.bids == nil || market.bids.Filters.Owner != "akash1owner" || market.bids.Filters.DSeq != 7 || market.bids.Filters.State != "open" {
		t.Fatalf("bids request = %#v", market.bids)
	}

	result, err = markettools.HandleGetBid(client)(context.Background(), toolRequest(identity))
	requireToolSuccess(t, result, err)
	wantBid := mv1.BidID{Owner: "akash1owner", DSeq: 7, GSeq: 2, OSeq: 3, Provider: "akash1provider"}
	if market.bid == nil || market.bid.ID != wantBid {
		t.Fatalf("bid request = %#v", market.bid)
	}

	result, err = markettools.HandleListLeases(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "dseq": float64(7), "state": "active", "provider": "akash1provider",
	}))
	requireToolSuccess(t, result, err)
	if market.leases == nil || market.leases.Filters.Owner != "akash1owner" || market.leases.Filters.DSeq != 7 ||
		market.leases.Filters.State != "active" || market.leases.Filters.Provider != "akash1provider" {
		t.Fatalf("leases request = %#v", market.leases)
	}

	result, err = markettools.HandleGetLease(client)(context.Background(), toolRequest(identity))
	requireToolSuccess(t, result, err)
	wantLease := mv1.LeaseID{Owner: "akash1owner", DSeq: 7, GSeq: 2, OSeq: 3, Provider: "akash1provider"}
	if market.lease == nil || market.lease.ID != wantLease {
		t.Fatalf("lease request = %#v", market.lease)
	}

	result, err = markettools.HandleCreateLease(client)(context.Background(), toolRequest(identity))
	requireToolSuccess(t, result, err)
	if len(tx.messages) != 1 {
		t.Fatalf("create lease messages = %#v", tx.messages)
	}
	createMessage, ok := tx.messages[0].(*mtypes.MsgCreateLease)
	if !ok || createMessage.BidID != wantBid {
		t.Fatalf("create lease message = %#v", tx.messages[0])
	}

	result, err = markettools.HandleCloseLease(client)(context.Background(), toolRequest(identity))
	requireToolSuccess(t, result, err)
	closeMessage, ok := tx.messages[0].(*mtypes.MsgCloseLease)
	if !ok || closeMessage.ID != wantLease {
		t.Fatalf("close lease message = %#v", tx.messages[0])
	}

	market.err = errors.New("market unavailable")
	result, err = markettools.HandleGetLease(client)(context.Background(), toolRequest(identity))
	requireToolErrorContains(t, result, err, "failed to get lease: market unavailable")
	result, err = markettools.HandleListOrders(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner",
	}))
	requireToolErrorContains(t, result, err, "failed to list orders: market unavailable")
	result, err = markettools.HandleGetOrder(client)(context.Background(), toolRequest(identity))
	requireToolErrorContains(t, result, err, "failed to get order: market unavailable")
	result, err = markettools.HandleListBids(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "dseq": float64(7),
	}))
	requireToolErrorContains(t, result, err, "failed to list bids: market unavailable")
	result, err = markettools.HandleGetBid(client)(context.Background(), toolRequest(identity))
	requireToolErrorContains(t, result, err, "failed to get bid: market unavailable")
	result, err = markettools.HandleListLeases(client)(context.Background(), toolRequest(map[string]any{
		"owner": "akash1owner", "dseq": float64(7),
	}))
	requireToolErrorContains(t, result, err, "failed to list leases: market unavailable")

	tx.err = errors.New("broadcast refused")
	result, err = markettools.HandleCloseLease(client)(context.Background(), toolRequest(identity))
	requireToolErrorContains(t, result, err, "failed to close lease: broadcast refused")
}

func TestProviderChainHandlersSendExactRequests(t *testing.T) {
	provider := &providerQueryStub{}
	client := semanticClient{query: semanticQuery{provider: provider}}

	result, err := providertools.HandleListProviders(client)(context.Background(), toolRequest(map[string]any{}))
	requireToolSuccess(t, result, err)
	if provider.list == nil {
		t.Fatal("provider list sent no request")
	}

	result, err = providertools.HandleGetProvider(client)(context.Background(), toolRequest(map[string]any{"owner": "akash1provider"}))
	requireToolSuccess(t, result, err)
	if provider.get == nil || provider.get.Owner != "akash1provider" {
		t.Fatalf("provider get request = %#v", provider.get)
	}

	provider.err = errors.New("provider unavailable")
	result, err = providertools.HandleListProviders(client)(context.Background(), toolRequest(map[string]any{}))
	requireToolErrorContains(t, result, err, "failed to list providers: provider unavailable")
}
