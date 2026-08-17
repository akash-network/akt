package ui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	providerv1beta4 "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/akt/internal/monitor/cache"
	"pkg.akt.dev/akt/internal/monitor/rpc"
)

func TestSyncChainRejectsCancellationAndTransportFailureBeforeMutation(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		store := &monitorProviderStore{providers: make(map[string]*cache.CachedProvider)}
		msg := (Model{runtimeContext: ctx, cache: store}).syncChain().(chainSyncMsg)
		if !errors.Is(msg.err, context.Canceled) || store.syncCalls != 0 {
			t.Fatalf("canceled sync = %#v, mutations %d", msg, store.syncCalls)
		}
	})

	t.Run("provider query fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "RPC unavailable", http.StatusServiceUnavailable)
		}))
		t.Cleanup(server.Close)
		store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{"existing": {}}}
		msg := (Model{
			client:         rpc.NewClient(server.URL, server.URL),
			rpcClient:      rpc.NewRPCProviderClient(server.URL),
			runtimeContext: context.Background(),
			cache:          store,
		}).syncChain().(chainSyncMsg)
		if msg.err == nil || store.syncCalls != 0 {
			t.Fatalf("failed provider sync = %#v, mutations %d", msg, store.syncCalls)
		}
	})
}

func TestSyncChainKeepsProviderReconciliationWhenLeasePriorityReadFails(t *testing.T) {
	providers := providerv1beta4.QueryProvidersResponse{
		Providers: providerv1beta4.Providers{{
			Owner:   "akash1provider",
			HostURI: "https://provider.example.test:8443",
		}},
	}
	encoded, err := providers.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/abci_query":
			_, _ = fmt.Fprintf(w, `{"result":{"response":{"code":0,"value":%q}}}`,
				base64.StdEncoding.EncodeToString(encoded))
		case "/akash/market/v1beta5/leases/list":
			http.Error(w, "lease index unavailable", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)

	store := &monitorProviderStore{providers: make(map[string]*cache.CachedProvider)}
	msg := (Model{
		client:         rpc.NewClient(server.URL, server.URL),
		rpcClient:      rpc.NewRPCProviderClient(server.URL),
		runtimeContext: context.Background(),
		cache:          store,
	}).syncChain().(chainSyncMsg)
	if msg.err != nil {
		t.Fatalf("syncChain() error = %v", msg.err)
	}
	if store.syncCalls != 1 || !reflect.DeepEqual(msg.newProviders, []string{"akash1provider"}) {
		t.Fatalf("provider reconciliation = calls %d, result %v", store.syncCalls, msg.newProviders)
	}
	if msg.activeLeaseProviders == nil || len(msg.activeLeaseProviders) != 0 {
		t.Fatalf("failed lease priority read = %v, want known-empty priority set", msg.activeLeaseProviders)
	}
}

func TestSyncChainRejectsCancellationAfterSuccessfulLeaseRead(t *testing.T) {
	providers := providerv1beta4.QueryProvidersResponse{
		Providers: providerv1beta4.Providers{{Owner: "akash1provider"}},
	}
	encoded, err := providers.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/abci_query" {
			http.NotFound(w, request)
			return
		}
		_, _ = fmt.Fprintf(w, `{"result":{"response":{"code":0,"value":%q}}}`,
			base64.StdEncoding.EncodeToString(encoded))
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	store := &monitorProviderStore{providers: make(map[string]*cache.CachedProvider)}
	m := Model{
		client:         rpc.NewClient(server.URL, server.URL),
		rpcClient:      rpc.NewRPCProviderClient(server.URL),
		runtimeContext: ctx,
		cache:          store,
		activeLeaseQuery: func(context.Context, string) (map[string]bool, error) {
			cancel()
			return map[string]bool{"akash1provider": true}, nil
		},
	}
	msg := m.syncChain().(chainSyncMsg)
	if !errors.Is(msg.err, context.Canceled) || store.syncCalls != 0 {
		t.Fatalf("post-query cancellation = %#v, mutations %d", msg, store.syncCalls)
	}
}

func TestFetchProvidersStopsAfterCanceledSeedAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &monitorProviderStore{providers: make(map[string]*cache.CachedProvider)}
	m := Model{
		rpcClient: rpc.NewRPCProviderClient("http://127.0.0.1:1"),
		cache:     store,
		loader:    ProviderLoader{FirstRun: true},
	}
	_, err := m.fetchProviders(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled seed fallback error = %v", err)
	}
}

func TestProviderCheckReportsLocalLifetimeAndLookupFailures(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		msg := (Model{runtimeContext: ctx}).checkProvider("provider-a")().(providerCheckedMsg)
		if !errors.Is(msg.err, context.Canceled) {
			t.Fatalf("canceled check error = %v", msg.err)
		}
	})

	t.Run("provider removed before check", func(t *testing.T) {
		store := &monitorProviderStore{providers: make(map[string]*cache.CachedProvider)}
		msg := (Model{runtimeContext: context.Background(), cache: store}).checkProvider("removed")().(providerCheckedMsg)
		if msg.err != nil || msg.isOnline || msg.owner != "removed" {
			t.Fatalf("removed provider check = %#v", msg)
		}
	})
}

func TestProviderCheckFallsBackToRESTAndPreservesResourceIdentity(t *testing.T) {
	versionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/version" {
			http.NotFound(w, request)
			return
		}
		_, _ = fmt.Fprint(w, `{"akash":{"version":"v0.8.0"}}`)
	}))
	t.Cleanup(versionServer.Close)

	status := &rpc.ProviderStatusResponse{}
	status.Cluster.Inventory.Available.Nodes = make([]struct {
		Name        string `json:"name"`
		Allocatable struct {
			CPU    uint64 `json:"cpu"`
			GPU    uint64 `json:"gpu"`
			Memory uint64 `json:"memory"`
		} `json:"allocatable"`
		Available struct {
			CPU    uint64 `json:"cpu"`
			GPU    uint64 `json:"gpu"`
			Memory uint64 `json:"memory"`
		} `json:"available"`
	}, 1)
	status.Cluster.Inventory.Available.Nodes[0].Allocatable.CPU = 8
	status.Cluster.Inventory.Available.Nodes[0].Allocatable.Memory = 16
	status.Cluster.Inventory.Available.Nodes[0].Allocatable.GPU = 2
	status.Cluster.Inventory.Available.Nodes[0].Available.CPU = 3
	status.Cluster.Inventory.Available.Nodes[0].Available.Memory = 7
	status.Cluster.Inventory.Available.Nodes[0].Available.GPU = 1

	store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{
		"provider-a": {HostURI: versionServer.URL},
	}}
	m := Model{
		runtimeContext: context.Background(),
		cache:          store,
		httpClient:     versionServer.Client(),
		providerStatusGRPC: func(context.Context, string, bool) ([]rpc.ProviderNodeWithGPU, error) {
			return nil, errors.New("gRPC unavailable")
		},
		providerStatusREST: func(context.Context, *http.Client, string) (*rpc.ProviderStatusResponse, error) {
			return status, nil
		},
	}
	msg := m.checkProvider("provider-a")().(providerCheckedMsg)
	if msg.err != nil || !msg.isOnline || msg.version != "v0.8.0" ||
		msg.cpuAvail != 3 || msg.cpuTotal != 8 || msg.memAvail != 7 || msg.memTotal != 16 ||
		msg.gpuAvail != 1 || msg.gpuTotal != 2 {
		t.Fatalf("REST fallback result = %#v", msg)
	}
}

func TestProviderCheckMarksProviderOfflineWhenBothTransportsFail(t *testing.T) {
	store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{
		"provider-a": {HostURI: "https://provider.example.test"},
	}}
	m := Model{
		runtimeContext: context.Background(),
		cache:          store,
		providerStatusGRPC: func(context.Context, string, bool) ([]rpc.ProviderNodeWithGPU, error) {
			return nil, errors.New("gRPC unavailable")
		},
		providerStatusREST: func(context.Context, *http.Client, string) (*rpc.ProviderStatusResponse, error) {
			return nil, errors.New("REST unavailable")
		},
	}
	msg := m.checkProvider("provider-a")().(providerCheckedMsg)
	if msg.err != nil || msg.isOnline {
		t.Fatalf("dual transport failure = %#v, want offline result", msg)
	}
}

func TestProviderCheckReturnsCancellationAfterRESTFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{
		"provider-a": {HostURI: "https://provider.example.test"},
	}}
	m := Model{
		runtimeContext: ctx,
		cache:          store,
		providerStatusGRPC: func(context.Context, string, bool) ([]rpc.ProviderNodeWithGPU, error) {
			return nil, errors.New("gRPC unavailable")
		},
		providerStatusREST: func(context.Context, *http.Client, string) (*rpc.ProviderStatusResponse, error) {
			cancel()
			return nil, errors.New("REST canceled")
		},
	}
	msg := m.checkProvider("provider-a")().(providerCheckedMsg)
	if !errors.Is(msg.err, context.Canceled) || msg.isOnline {
		t.Fatalf("canceled REST fallback = %#v", msg)
	}
}

func TestProviderCheckReturnsCancellationAfterVersionRead(t *testing.T) {
	for _, transport := range []string{"grpc", "rest"} {
		t.Run(transport, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				cancel()
				_, _ = fmt.Fprint(w, `{"akash":{"version":"stale"}}`)
			}))
			t.Cleanup(server.Close)
			store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{
				"provider-a": {HostURI: server.URL},
			}}
			m := Model{
				runtimeContext: ctx,
				cache:          store,
				httpClient:     server.Client(),
				providerStatusGRPC: func(context.Context, string, bool) ([]rpc.ProviderNodeWithGPU, error) {
					if transport == "rest" {
						return nil, errors.New("use REST")
					}
					return []rpc.ProviderNodeWithGPU{{CPUAvailable: 1, CPUAllocatable: 2}}, nil
				},
				providerStatusREST: func(context.Context, *http.Client, string) (*rpc.ProviderStatusResponse, error) {
					return &rpc.ProviderStatusResponse{}, nil
				},
			}
			msg := m.checkProvider("provider-a")().(providerCheckedMsg)
			if !errors.Is(msg.err, context.Canceled) || msg.isOnline {
				t.Fatalf("canceled %s version result = %#v", transport, msg)
			}
		})
	}
}

func TestProviderCheckUsesGRPCResourceDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"akash":{"version":"v0.9.0"}}`)
	}))
	t.Cleanup(server.Close)
	store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{
		"provider-a": {HostURI: server.URL},
	}}
	m := Model{
		runtimeContext: context.Background(),
		cache:          store,
		httpClient:     server.Client(),
		providerStatusGRPC: func(context.Context, string, bool) ([]rpc.ProviderNodeWithGPU, error) {
			return []rpc.ProviderNodeWithGPU{{
				CPUAvailable: 4, CPUAllocatable: 8,
				MemAvailable: 8, MemAllocatable: 16,
				GPUAvailable: 1, GPUAllocatable: 2,
				GPUs: []rpc.GPUInfo{{Name: "H100"}},
			}}, nil
		},
	}
	msg := m.checkProvider("provider-a")().(providerCheckedMsg)
	if msg.err != nil || !msg.isOnline || msg.version != "v0.9.0" || !reflect.DeepEqual(msg.gpuModels, []string{"H100"}) {
		t.Fatalf("gRPC provider result = %#v", msg)
	}
}

func TestDefaultProviderProbeBoundaries(t *testing.T) {
	m := Model{}
	if _, err := m.queryProviderStatusGRPC(context.Background(), "://bad"); err == nil {
		t.Fatal("default gRPC probe accepted an invalid provider URI")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/status" {
			http.NotFound(w, request)
			return
		}
		_, _ = fmt.Fprint(w, `{"cluster":{"inventory":{"available":{"nodes":[]}}}}`)
	}))
	t.Cleanup(server.Close)
	m.httpClient = server.Client()
	status, err := m.queryProviderStatusREST(context.Background(), server.URL)
	if err != nil || status == nil {
		t.Fatalf("default REST probe = %#v, %v", status, err)
	}
}

func TestDispatchProviderChecksStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := Model{
		runtimeContext: ctx,
		loader: ProviderLoader{
			Queue:    []string{"provider-a"},
			InFlight: make(map[string]bool),
		},
	}
	if cmds := m.dispatchProviderChecks(); cmds != nil || len(m.loader.InFlight) != 0 {
		t.Fatalf("canceled dispatch = %d commands, in-flight %v", len(cmds), m.loader.InFlight)
	}
}

func TestFetchMonikersUsesCacheAndRejectsCanceledOrFailedReads(t *testing.T) {
	db, err := cache.OpenDB(t.TempDir() + "/monitor.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, monikerCache, err := cache.OpenStores(db)
	if err != nil {
		t.Fatal(err)
	}
	monikerCache.Set(map[string]string{"cached-key": "cached-validator"})

	cached := (Model{
		monikerCache:   monikerCache,
		monikers:       map[string]string{"cached-key": "cached-validator"},
		runtimeContext: context.Background(),
	}).fetchMonikers().(monikersMsg)
	if cached.err != nil || cached.monikers["cached-key"] != "cached-validator" {
		t.Fatalf("cached monikers = %#v", cached)
	}

	monikerCache.Set(nil)
	failureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "staking unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(failureServer.Close)
	failed := (Model{
		client:         rpc.NewClient(failureServer.URL, failureServer.URL),
		monikerCache:   monikerCache,
		runtimeContext: context.Background(),
	}).fetchMonikers().(monikersMsg)
	if failed.err == nil {
		t.Fatal("failed moniker read returned success")
	}

	ctx, cancel := context.WithCancel(context.Background())
	canceled := (Model{
		monikerCache:   monikerCache,
		runtimeContext: ctx,
		validatorMonikersQuery: func(context.Context) (map[string]string, error) {
			cancel()
			return map[string]string{"stale-key": "stale-validator"}, nil
		},
	}).fetchMonikers().(monikersMsg)
	if !errors.Is(canceled.err, context.Canceled) || monikerCache.HasMonikers() {
		t.Fatalf("canceled moniker read = %#v, cache populated %t", canceled, monikerCache.HasMonikers())
	}

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"validators":[{"description":{"moniker":"validator-a"},"consensus_pubkey":{"key":"key-a"}}],"pagination":{}}`)
	}))
	t.Cleanup(successServer.Close)
	success := (Model{
		client:         rpc.NewClient(successServer.URL, successServer.URL),
		monikerCache:   monikerCache,
		runtimeContext: context.Background(),
	}).fetchMonikers().(monikersMsg)
	if success.err != nil || success.monikers["key-a"] != "validator-a" || !monikerCache.HasMonikers() {
		t.Fatalf("successful moniker read = %#v, cache populated %t", success, monikerCache.HasMonikers())
	}
}
