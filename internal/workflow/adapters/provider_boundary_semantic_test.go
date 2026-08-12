package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
)

type adapterMarketQueryServer struct {
	mtypes.UnimplementedQueryServer
	providers []string
	requests  []*mtypes.QueryLeasesRequest
}

func (server *adapterMarketQueryServer) Leases(
	_ context.Context,
	request *mtypes.QueryLeasesRequest,
) (*mtypes.QueryLeasesResponse, error) {
	copyRequest := *request
	copyRequest.Pagination = request.Pagination
	server.requests = append(server.requests, &copyRequest)

	leases := make([]mtypes.QueryLeaseResponse, 0, len(server.providers))
	for _, provider := range server.providers {
		leases = append(leases, mtypes.QueryLeaseResponse{Lease: mv1.Lease{ID: mv1.LeaseID{
			Owner:    request.Filters.Owner,
			DSeq:     request.Filters.DSeq,
			GSeq:     defaultGSeq,
			OSeq:     defaultOSeq,
			Provider: provider,
		}}})
	}

	return &mtypes.QueryLeasesResponse{Leases: leases}, nil
}

type adapterProviderQueryServer struct {
	ptypes.UnimplementedQueryServer
	hostURI   string
	noHost    string
	requested []string
}

func (server *adapterProviderQueryServer) Provider(
	_ context.Context,
	request *ptypes.QueryProviderRequest,
) (*ptypes.QueryProviderResponse, error) {
	server.requested = append(server.requested, request.Owner)
	hostURI := server.hostURI
	if request.Owner == server.noHost {
		hostURI = ""
	}

	return &ptypes.QueryProviderResponse{Provider: ptypes.Provider{
		Owner:   request.Owner,
		HostURI: hostURI,
	}}, nil
}

func TestProviderAdapterUsesChainIdentityAndAuthenticatedGateway(t *testing.T) {
	requests := make(map[string]int)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("%s %s missing JWT authorization", request.Method, request.URL.Path)
		}
		key := request.Method + " " + request.URL.Path
		requests[key]++
		switch key {
		case "PUT /deployment/42/manifest", "PUT /deployment/43/manifest":
			w.WriteHeader(http.StatusOK)
		case "PUT /deployment/45/manifest":
			http.Error(w, "manifest rejected", http.StatusBadGateway)
		case "GET /lease/44/1/1/status":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(gateway.Close)

	providerA := sdk.AccAddress(bytes.Repeat([]byte{0x31}, 20)).String()
	providerB := sdk.AccAddress(bytes.Repeat([]byte{0x32}, 20)).String()
	providerNoHost := sdk.AccAddress(bytes.Repeat([]byte{0x33}, 20)).String()
	marketServer := &adapterMarketQueryServer{providers: []string{providerB, providerA, providerA}}
	providerServer := &adapterProviderQueryServer{hostURI: gateway.URL, noHost: providerNoHost}
	connection := newAdapterQueryConnection(t, marketServer, providerServer)
	keyring, owner := newAdapterTestIdentity(t)
	cctx := sdkclient.Context{}.
		WithFromAddress(owner).
		WithKeyring(keyring).
		WithGRPCClient(connection)
	client := &providerClient{cctx: cctx, authType: "jwt"}

	if err := client.SendManifest(context.Background(), providerA, 42, []byte(testSDLPath)); err != nil {
		t.Fatalf("send manifest: %v", err)
	}
	if requests["PUT /deployment/42/manifest"] != 1 {
		t.Fatalf("single-provider manifest requests = %v", requests)
	}

	sent, err := client.SendManifestToActiveLeases(context.Background(), 43, []byte(testSDLPath))
	if err != nil {
		t.Fatalf("send manifest to active leases: %v", err)
	}
	wantProviders := []string{providerA, providerB}
	sort.Strings(wantProviders)
	if !reflect.DeepEqual(sent, wantProviders) {
		t.Fatalf("accepted providers = %v, want %v", sent, wantProviders)
	}
	if requests["PUT /deployment/43/manifest"] != 2 {
		t.Fatalf("active-lease manifest submissions = %d, want 2", requests["PUT /deployment/43/manifest"])
	}
	if len(marketServer.requests) != 1 {
		t.Fatalf("active lease queries = %d, want 1", len(marketServer.requests))
	}
	leaseRequest := marketServer.requests[0]
	if leaseRequest.Filters != (mv1.LeaseFilters{Owner: owner.String(), DSeq: 43, State: "active"}) ||
		leaseRequest.Pagination == nil || leaseRequest.Pagination.Limit != 100 {
		t.Fatalf("active lease request = %#v", leaseRequest)
	}

	status, err := client.LeaseStatus(context.Background(), providerA, 44)
	if err != nil {
		t.Fatalf("lease status: %v", err)
	}
	if !json.Valid(status) || requests["GET /lease/44/1/1/status"] != 1 {
		t.Fatalf("lease status = %s, requests %v", status, requests)
	}

	err = client.SendManifest(context.Background(), providerA, 45, []byte(testSDLPath))
	if err == nil || !strings.Contains(err.Error(), "submit manifest") || !strings.Contains(err.Error(), "502") {
		t.Fatalf("gateway rejection error = %v", err)
	}

	if _, err := client.providerHostURI(context.Background(), providerNoHost); err == nil || !strings.Contains(err.Error(), "has no host URI") {
		t.Fatalf("empty provider host error = %v", err)
	}
}

func TestProviderAdapterRejectsMalformedInputsAndPropagatesCancellation(t *testing.T) {
	provider := testProviderAddr().String()
	marketServer := &adapterMarketQueryServer{}
	providerServer := &adapterProviderQueryServer{hostURI: "http://provider.example.test"}
	connection := newAdapterQueryConnection(t, marketServer, providerServer)
	keyring, owner := newAdapterTestIdentity(t)
	client := &providerClient{
		cctx: sdkclient.Context{}.
			WithFromAddress(owner).
			WithKeyring(keyring).
			WithGRPCClient(connection),
		authType: "jwt",
	}

	for _, malformed := range [][]byte{
		[]byte("not: [valid"),
		[]byte("missing-deployment.yaml"),
	} {
		if err := client.SendManifest(context.Background(), provider, 1, malformed); err == nil {
			t.Fatalf("malformed SDL %q was accepted", malformed)
		}
	}
	if err := client.SendManifest(context.Background(), "not-a-provider", 1, []byte(testSDLPath)); err == nil ||
		!strings.Contains(err.Error(), "invalid provider address") {
		t.Fatalf("invalid provider error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	err := client.SendManifest(canceled, provider, 1, []byte(testSDLPath))
	if status.Code(err) != codes.Canceled || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("canceled provider lookup error = %v, want context cancellation", err)
	}

	noOwner := &providerClient{cctx: sdkclient.Context{}.WithGRPCClient(connection)}
	if _, err := noOwner.SendManifestToActiveLeases(context.Background(), 1, []byte(testSDLPath)); err == nil ||
		!strings.Contains(err.Error(), "owner address is required") {
		t.Fatalf("missing owner error = %v", err)
	}
}

func TestActiveLeaseAndManifestFanoutRejectMalformedDependencyState(t *testing.T) {
	wantErr := errors.New("lease query unavailable")
	_, err := activeLeaseProviders(context.Background(), "akash1owner", 7, func(
		context.Context,
		*mtypes.QueryLeasesRequest,
	) (*mtypes.QueryLeasesResponse, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "deployment 7") {
		t.Fatalf("lease query error = %v", err)
	}

	_, err = activeLeaseProviders(context.Background(), "akash1owner", 8, func(
		context.Context,
		*mtypes.QueryLeasesRequest,
	) (*mtypes.QueryLeasesResponse, error) {
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("nil lease response error = %v", err)
	}

	providers, err := activeLeaseProviders(context.Background(), "akash1owner", 9, func(
		context.Context,
		*mtypes.QueryLeasesRequest,
	) (*mtypes.QueryLeasesResponse, error) {
		return &mtypes.QueryLeasesResponse{Leases: []mtypes.QueryLeaseResponse{{
			Lease: mv1.Lease{ID: mv1.LeaseID{}},
		}}}, nil
	})
	if err != nil || len(providers) != 0 {
		t.Fatalf("blank provider dependency response = %v, %v", providers, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var attempted []string
	sent, err := sendManifestToProviders(ctx, []string{"provider-a", "provider-b"}, func(_ context.Context, provider string) error {
		attempted = append(attempted, provider)
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(sent, []string{"provider-a"}) ||
		!reflect.DeepEqual(attempted, []string{"provider-a"}) {
		t.Fatalf("canceled fanout = sent %v, attempted %v, error %v", sent, attempted, err)
	}
}

func newAdapterQueryConnection(
	t *testing.T,
	marketServer mtypes.QueryServer,
	providerServer ptypes.QueryServer,
) *grpc.ClientConn {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	mtypes.RegisterQueryServer(server, marketServer)
	ptypes.RegisterQueryServer(server, providerServer)
	go func() {
		_ = server.Serve(listener)
	}()

	connection, err := grpc.NewClient(
		"passthrough:///workflow-adapter-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		server.Stop()
		_ = listener.Close()
		t.Fatalf("create adapter query connection: %v", err)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		server.Stop()
		_ = listener.Close()
	})

	return connection
}

func newAdapterTestIdentity(t *testing.T) (sdkkeyring.Keyring, sdk.AccAddress) {
	t.Helper()

	keyring := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	record, _, err := keyring.NewMnemonic(
		"workflow-owner",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatalf("create workflow adapter signing key: %v", err)
	}
	owner, err := record.GetAddress()
	if err != nil {
		t.Fatalf("get workflow adapter owner: %v", err)
	}

	return keyring, owner
}
