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
	"time"

	tea "charm.land/bubbletea/v2"
	providerv1beta4 "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/akt/internal/monitor/cache"
	"pkg.akt.dev/akt/internal/monitor/rpc"
)

type monitorProviderStore struct {
	providers    map[string]*cache.CachedProvider
	priority     []string
	due          []string
	onlineCalls  []string
	offlineCalls []string
	syncCalls    int
	saveErr      error
}

func (s *monitorProviderStore) HasProviders() bool { return len(s.providers) > 0 }

func (s *monitorProviderStore) GetProvider(owner string) (*cache.CachedProvider, bool) {
	p, ok := s.providers[owner]
	return p, ok
}

func (s *monitorProviderStore) GetAllProviders() map[string]*cache.CachedProvider {
	return s.providers
}

func (s *monitorProviderStore) GetOnlineProviders() []*cache.CachedProvider {
	result := make([]*cache.CachedProvider, 0, len(s.providers))
	for _, p := range s.providers {
		if p.IsOnline {
			result = append(result, p)
		}
	}
	return result
}

func (s *monitorProviderStore) MarkProviderOnline(
	owner, version string,
	cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal uint64,
	gpuModels []string,
) {
	s.onlineCalls = append(s.onlineCalls, owner)
	if p, ok := s.providers[owner]; ok {
		p.IsOnline = true
		p.Version = version
		p.CPUAvailable = cpuAvail
		p.CPUTotal = cpuTotal
		p.MemAvailable = memAvail
		p.MemTotal = memTotal
		p.GPUAvailable = gpuAvail
		p.GPUTotal = gpuTotal
		p.GPUModels = gpuModels
	}
}

func (s *monitorProviderStore) MarkProviderOffline(owner string) {
	s.offlineCalls = append(s.offlineCalls, owner)
	if p, ok := s.providers[owner]; ok {
		p.IsOnline = false
	}
}

func (s *monitorProviderStore) SyncWithChain(providers []rpc.OnChainProvider) []string {
	s.syncCalls++
	var added []string
	if s.providers == nil {
		s.providers = make(map[string]*cache.CachedProvider)
	}
	for _, provider := range providers {
		if _, ok := s.providers[provider.Owner]; ok {
			continue
		}
		s.providers[provider.Owner] = &cache.CachedProvider{
			HostURI: provider.HostURI,
			Name:    provider.Attributes["organization"],
		}
		added = append(added, provider.Owner)
	}
	return added
}

func (s *monitorProviderStore) GetProvidersDueForCheck() []string {
	return append([]string(nil), s.due...)
}

func (s *monitorProviderStore) GetProvidersByPriority() []string {
	return append([]string(nil), s.priority...)
}

func (s *monitorProviderStore) ProviderCount() int { return len(s.providers) }

func (s *monitorProviderStore) OnlineCount() int {
	count := 0
	for _, p := range s.providers {
		if p.IsOnline {
			count++
		}
	}
	return count
}

func (s *monitorProviderStore) Save() error { return s.saveErr }

func TestRebuildProviderListEnforcesUsableUniqueGateways(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{
		"old-duplicate": {
			HostURI:        "https://shared.example.test:8443",
			IsOnline:       true,
			Version:        "0.6.3",
			LastSeenOnline: old,
		},
		"new-duplicate": {
			HostURI:        "https://shared.example.test:8443",
			IsOnline:       true,
			Version:        "0.6.4",
			LastSeenOnline: old.Add(time.Hour),
		},
		"selected": {
			HostURI:        "https://selected.example.test:8443",
			IsOnline:       true,
			Version:        "0.6.2",
			LastSeenOnline: old,
		},
		"offline": {
			HostURI: "https://offline.example.test:8443",
			Version: "0.6.5",
		},
		"local": {
			HostURI:  "http://127.0.0.1:8443",
			IsOnline: true,
			Version:  "0.6.5",
		},
		"unknown-version": {
			HostURI:  "https://unknown.example.test:8443",
			IsOnline: true,
			Version:  "unknown",
		},
	}}
	m := Model{
		cache:         store,
		providerTable: newTestProviderTableModel(nil),
		providers: ProviderList{
			Version: "0.6.2",
		},
	}

	m.rebuildProviderList()
	if got, want := len(m.providers.Items), 2; got != want {
		t.Fatalf("usable unique providers = %d, want %d: %#v", got, want, m.providers.Items)
	}
	if m.providers.Items[0].Owner != "selected" {
		t.Fatalf("first provider = %q, want selected version first", m.providers.Items[0].Owner)
	}
	if m.providers.Items[1].Owner != "new-duplicate" || m.providers.Items[1].AkashVersion != "0.6.4" {
		t.Fatalf("deduplicated provider = %#v, want most recently seen gateway owner", m.providers.Items[1])
	}
	if !reflect.DeepEqual(m.providers.Versions, []string{"0.6.4", "0.6.2"}) {
		t.Fatalf("versions = %v", m.providers.Versions)
	}
	if got := len(m.providerTable.Rows()); got != 1 {
		t.Fatalf("selected-version table rows = %d, want 1", got)
	}
}

func TestProviderQueuePrioritizesActiveLeasesAndRespectsCapacity(t *testing.T) {
	store := &monitorProviderStore{priority: []string{"ordinary-a", "active", "ordinary-b"}}
	m := Model{
		cache: store,
		loader: ProviderLoader{
			FirstRun: true,
			InFlight: map[string]bool{"ordinary-a": true},
		},
	}

	m.buildProviderQueue(map[string]bool{"active": true})
	if !reflect.DeepEqual(m.loader.Queue, []string{"active", "ordinary-a", "ordinary-b"}) {
		t.Fatalf("provider queue = %v", m.loader.Queue)
	}
	if !m.loader.Loading || m.loader.Total != 3 || m.loader.Checked != 0 {
		t.Fatalf("loader = %#v", m.loader)
	}

	cmds := m.dispatchProviderChecks()
	if got, want := len(cmds), 2; got != want {
		t.Fatalf("dispatched checks = %d, want %d", got, want)
	}
	if !m.loader.InFlight["active"] || !m.loader.InFlight["ordinary-b"] {
		t.Fatalf("in-flight providers = %v", m.loader.InFlight)
	}

	m.loader.InFlight = make(map[string]bool, MaxConcurrentChecks)
	for i := 0; i < MaxConcurrentChecks; i++ {
		m.loader.InFlight[string(rune('a'+i))] = true
	}
	if got := m.dispatchProviderChecks(); got != nil {
		t.Fatalf("checks beyond capacity = %d, want none", len(got))
	}
}

func TestProviderCheckResultConvergesLoaderAndCache(t *testing.T) {
	store := &monitorProviderStore{
		providers: map[string]*cache.CachedProvider{
			"provider-a": {
				HostURI:  "https://provider-a.example.test:8443",
				IsOnline: false,
			},
		},
		due: []string{"provider-later"},
	}
	m := Model{
		cache:         store,
		providerTable: newTestProviderTableModel(nil),
		loader: ProviderLoader{
			FirstRun: true,
			Loading:  true,
			Queue:    []string{"provider-a"},
			InFlight: map[string]bool{"provider-a": true},
		},
	}

	updated, cmd := m.handleProviderCheckedMsg(providerCheckedMsg{
		owner:     "provider-a",
		isOnline:  true,
		version:   "0.7.0",
		cpuAvail:  2_000,
		cpuTotal:  4_000,
		memAvail:  8,
		memTotal:  16,
		gpuAvail:  1,
		gpuTotal:  2,
		gpuModels: []string{"H100"},
	})
	m = lifecycleModelValue(t, updated)
	if cmd != nil {
		t.Fatal("completed provider result unexpectedly launched a command")
	}
	if !reflect.DeepEqual(store.onlineCalls, []string{"provider-a"}) || len(store.offlineCalls) != 0 {
		t.Fatalf("cache calls online=%v offline=%v", store.onlineCalls, store.offlineCalls)
	}
	if m.loader.Loading || m.loader.FirstRun || m.loader.Checked != 1 ||
		!reflect.DeepEqual(m.loader.Queue, []string{"provider-later"}) || len(m.loader.InFlight) != 0 {
		t.Fatalf("loader after final result = %#v", m.loader)
	}
	if len(m.providers.Items) != 1 || m.providers.Items[0].AkashVersion != "0.7.0" {
		t.Fatalf("visible provider = %#v", m.providers.Items)
	}
}

func TestProviderOfflineResultRemovesProviderFromVisibleList(t *testing.T) {
	store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{
		"provider-a": {
			HostURI:  "https://provider-a.example.test:8443",
			IsOnline: true,
			Version:  "0.7.0",
		},
	}}
	m := Model{
		cache:         store,
		providerTable: newTestProviderTableModel(nil),
		loader: ProviderLoader{
			Queue:    []string{"provider-a", "other"},
			InFlight: map[string]bool{"provider-a": true},
		},
	}

	updated, _ := m.handleProviderCheckedMsg(providerCheckedMsg{owner: "provider-a"})
	m = lifecycleModelValue(t, updated)
	if !reflect.DeepEqual(store.offlineCalls, []string{"provider-a"}) {
		t.Fatalf("offline calls = %v", store.offlineCalls)
	}
	if len(m.providers.Items) != 0 || !reflect.DeepEqual(m.loader.Queue, []string{"other"}) {
		t.Fatalf("visible=%#v queue=%v", m.providers.Items, m.loader.Queue)
	}
}

func TestProviderChainSyncErrorDoesNotDiscardCurrentPipeline(t *testing.T) {
	wantErr := errors.New("chain unavailable")
	m := Model{
		loader: ProviderLoader{
			Queue:    []string{"existing"},
			InFlight: map[string]bool{"existing": true},
		},
	}

	updated, cmd := m.handleChainSyncMsg(chainSyncMsg{err: wantErr})
	m = lifecycleModelValue(t, updated)
	if cmd != nil || !reflect.DeepEqual(m.loader.Queue, []string{"existing"}) || !m.loader.InFlight["existing"] {
		t.Fatalf("pipeline changed after sync failure: %#v", m.loader)
	}
}

func TestCanceledProviderResultDoesNotMutateCacheOrRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{
		"provider-a": {
			HostURI:  "https://provider-a.example.test:8443",
			IsOnline: true,
			Version:  "0.7.0",
		},
	}}
	m := Model{
		runtimeContext: ctx,
		cache:          store,
		providerTable:  newTestProviderTableModel(nil),
		loader: ProviderLoader{
			Loading:  true,
			Queue:    []string{"provider-a"},
			InFlight: map[string]bool{"provider-a": true},
		},
	}

	updated, cmd := m.handleProviderCheckedMsg(providerCheckedMsg{owner: "provider-a"})
	m = lifecycleModelValue(t, updated)
	if cmd != nil {
		t.Fatal("canceled provider result scheduled another command")
	}
	if len(store.onlineCalls) != 0 || len(store.offlineCalls) != 0 {
		t.Fatalf("canceled result mutated cache: online=%v offline=%v", store.onlineCalls, store.offlineCalls)
	}
	if len(m.loader.Queue) != 0 || len(m.loader.InFlight) != 0 {
		t.Fatalf("canceled provider remained eligible for retry: %#v", m.loader)
	}
}

func TestChainSyncCancellationDoesNotWriteProviderCache(t *testing.T) {
	providers := providerv1beta4.QueryProvidersResponse{
		Providers: providerv1beta4.Providers{{
			Owner:   "akash1provider",
			HostURI: "https://provider.example.test:8443",
		}},
	}
	encodedProviders, err := providers.Marshal()
	if err != nil {
		t.Fatalf("marshal provider response: %v", err)
	}

	leaseRequestStarted := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/abci_query", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"result":{"response":{"code":0,"value":%q}}}`,
			base64.StdEncoding.EncodeToString(encodedProviders))
	})
	mux.HandleFunc("/akash/market/v1beta5/leases/list", func(_ http.ResponseWriter, r *http.Request) {
		close(leaseRequestStarted)
		<-r.Context().Done()
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	store := &monitorProviderStore{providers: make(map[string]*cache.CachedProvider)}
	m := Model{
		client:         rpc.NewClient(server.URL, server.URL),
		rpcClient:      rpc.NewRPCProviderClient(server.URL),
		runtimeContext: ctx,
		cache:          store,
		loader: ProviderLoader{
			InFlight: make(map[string]bool),
		},
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- m.syncChain() }()

	select {
	case <-leaseRequestStarted:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("active lease query did not start")
	}
	cancel()

	select {
	case raw := <-result:
		msg, ok := raw.(chainSyncMsg)
		if !ok {
			t.Fatalf("sync result = %T, want chainSyncMsg", raw)
		}
		if !errors.Is(msg.err, context.Canceled) {
			t.Fatalf("sync error = %v, want context cancellation", msg.err)
		}
	case <-time.After(time.Second):
		t.Fatal("chain sync did not stop after cancellation")
	}
	if store.syncCalls != 0 || store.HasProviders() {
		t.Fatalf("canceled sync wrote provider cache: calls=%d providers=%v", store.syncCalls, store.providers)
	}
}

func TestProviderStatusCancellationDrainsWithoutFallbackOrCacheWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tasks := NewRuntimeTaskGroup()
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	store := &monitorProviderStore{providers: map[string]*cache.CachedProvider{
		"provider-a": {HostURI: "https://provider-a.example.test:8443"},
	}}
	grpcCalls := 0
	restCalls := 0
	m := Model{
		runtimeContext: ctx,
		runtimeTasks:   tasks,
		cache:          store,
		providerTable:  newTestProviderTableModel(nil),
		httpClient:     http.DefaultClient,
		loader: ProviderLoader{
			Loading:  true,
			Queue:    []string{"provider-a"},
			InFlight: map[string]bool{"provider-a": true},
		},
		providerStatusGRPC: func(queryCtx context.Context, _ string, _ bool) ([]rpc.ProviderNodeWithGPU, error) {
			grpcCalls++
			close(started)
			<-queryCtx.Done()
			close(canceled)
			<-release
			return nil, queryCtx.Err()
		},
		providerStatusREST: func(context.Context, *http.Client, string) (*rpc.ProviderStatusResponse, error) {
			restCalls++
			return nil, errors.New("REST fallback must not run after cancellation")
		},
	}

	result := make(chan tea.Msg, 1)
	go func() { result <- m.checkProvider("provider-a")() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("provider status query did not start")
	}
	cancel()
	drained := make(chan struct{})
	go func() {
		tasks.StopAndWait()
		close(drained)
	}()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("provider status query did not observe cancellation")
	}
	select {
	case <-drained:
		close(release)
		t.Fatal("runtime drain returned while provider status work was active")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("runtime drain did not finish after provider status returned")
	}

	raw := <-result
	msg, ok := raw.(providerCheckedMsg)
	if !ok {
		t.Fatalf("status result = %T, want providerCheckedMsg", raw)
	}
	if !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("status error = %v, want context cancellation", msg.err)
	}
	updated, cmd := m.handleProviderCheckedMsg(msg)
	m = lifecycleModelValue(t, updated)
	if cmd != nil || restCalls != 0 || len(store.onlineCalls) != 0 || len(store.offlineCalls) != 0 {
		t.Fatalf("canceled status fallback/cache state: cmd=%v grpc=%d rest=%d online=%v offline=%v",
			cmd, grpcCalls, restCalls, store.onlineCalls, store.offlineCalls)
	}
	if len(m.loader.Queue) != 0 || len(m.loader.InFlight) != 0 {
		t.Fatalf("canceled status remained eligible for retry: %#v", m.loader)
	}
	if msg := m.checkProvider("provider-a")(); msg != nil {
		t.Fatalf("stopped runtime accepted another provider command: %T", msg)
	}
	if grpcCalls != 1 || restCalls != 0 {
		t.Fatalf("provider calls after drain: grpc=%d rest=%d", grpcCalls, restCalls)
	}
}

func TestProviderDetailCancellationDrainsBeforeRuntimeCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tasks := NewRuntimeTaskGroup()
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	m := Model{
		runtimeContext: ctx,
		runtimeTasks:   tasks,
		providerStatusGRPC: func(queryCtx context.Context, _ string, _ bool) ([]rpc.ProviderNodeWithGPU, error) {
			close(started)
			<-queryCtx.Done()
			close(canceled)
			<-release
			return nil, queryCtx.Err()
		},
	}

	result := make(chan tea.Msg, 1)
	go func() { result <- m.fetchProviderDetail("https://provider.example.test:8443", 7)() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("provider detail query did not start")
	}
	cancel()
	drained := make(chan struct{})
	go func() {
		tasks.StopAndWait()
		close(drained)
	}()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("provider detail query did not observe cancellation")
	}
	select {
	case <-drained:
		close(release)
		t.Fatal("runtime drain returned while provider detail work was active")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("runtime drain did not finish after provider detail returned")
	}

	raw := <-result
	msg, ok := raw.(providerDetailMsg)
	if !ok {
		t.Fatalf("detail result = %T, want providerDetailMsg", raw)
	}
	if msg.hostURI != "https://provider.example.test:8443" || msg.requestID != 7 ||
		!errors.Is(msg.err, context.Canceled) {
		t.Fatalf("detail result = %#v, want correlated cancellation", msg)
	}
}

func TestProviderResourceAggregationPreservesTotalsAndUniqueModels(t *testing.T) {
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
	}, 2)
	status.Cluster.Inventory.Available.Nodes[0].Allocatable.CPU = 8
	status.Cluster.Inventory.Available.Nodes[0].Allocatable.Memory = 16
	status.Cluster.Inventory.Available.Nodes[0].Allocatable.GPU = 2
	status.Cluster.Inventory.Available.Nodes[0].Available.CPU = 3
	status.Cluster.Inventory.Available.Nodes[0].Available.Memory = 7
	status.Cluster.Inventory.Available.Nodes[0].Available.GPU = 1
	status.Cluster.Inventory.Available.Nodes[1].Allocatable.CPU = 10
	status.Cluster.Inventory.Available.Nodes[1].Allocatable.Memory = 20
	status.Cluster.Inventory.Available.Nodes[1].Allocatable.GPU = 4
	status.Cluster.Inventory.Available.Nodes[1].Available.CPU = 5
	status.Cluster.Inventory.Available.Nodes[1].Available.Memory = 9
	status.Cluster.Inventory.Available.Nodes[1].Available.GPU = 2
	if got := resourceTupleREST(status); got != [6]uint64{8, 18, 16, 36, 3, 6} {
		t.Fatalf("REST resources = %v", got)
	}

	nodes := []rpc.ProviderNodeWithGPU{
		{
			CPUAvailable: 3, CPUAllocatable: 8, MemAvailable: 7, MemAllocatable: 16,
			GPUAvailable: 1, GPUAllocatable: 2,
			GPUs: []rpc.GPUInfo{{Name: "H100"}, {Name: "H100"}, {Name: ""}},
		},
		{
			CPUAvailable: 5, CPUAllocatable: 10, MemAvailable: 9, MemAllocatable: 20,
			GPUAvailable: 2, GPUAllocatable: 4,
			GPUs: []rpc.GPUInfo{{Name: "A100"}},
		},
	}
	cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal, models := aggregateResourcesGRPC(nodes)
	if got := [6]uint64{cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal}; got != [6]uint64{8, 18, 16, 36, 3, 6} {
		t.Fatalf("gRPC resources = %v", got)
	}
	if !reflect.DeepEqual(models, []string{"H100", "A100"}) {
		t.Fatalf("GPU models = %v, want stable unique models", models)
	}
}

func TestProviderDetailEntryCorrelatesRequestAndRejectsEmptySelection(t *testing.T) {
	m := Model{
		providerTable: newTestProviderTableModel(nil),
		providers:     ProviderList{},
	}
	updated, cmd := m.enterProviderDetail()
	m = lifecycleModelValue(t, updated)
	if cmd != nil || m.detail.Showing {
		t.Fatal("empty provider list opened detail view")
	}

	provider := newTestProviders(1)[0]
	m.providers = ProviderList{Items: []rpc.Provider{provider}}
	m.providerTable = newTestProviderTableModel([]rpc.Provider{provider})
	updated, cmd = m.enterProviderDetail()
	m = lifecycleModelValue(t, updated)
	if cmd == nil || !m.detail.Showing || !m.detail.Loading || m.detail.Provider == nil ||
		m.detail.Provider.HostURI != provider.HostURI || m.detailRequestID != 1 {
		t.Fatalf("detail request state = %#v request=%d", m.detail, m.detailRequestID)
	}
}

func TestMatchingProviderDetailErrorIsVisible(t *testing.T) {
	wantErr := errors.New("provider rejected status")
	hostURI := "https://selected.example.test:8443"
	m := Model{
		detailRequestID: 4,
		detail: ProviderDetail{
			Showing:  true,
			Loading:  true,
			Provider: &rpc.Provider{HostURI: hostURI},
		},
		nodeTable: newTestNodeTableModel(nil),
	}

	updated, _ := m.handleProviderDetailMsg(providerDetailMsg{
		hostURI:   hostURI,
		requestID: 4,
		err:       wantErr,
	})
	m = lifecycleModelValue(t, updated)
	if m.detail.Loading || !errors.Is(m.detail.Error, wantErr) || len(m.detail.Nodes) != 0 {
		t.Fatalf("detail error state = %#v", m.detail)
	}
}

func resourceTupleREST(status *rpc.ProviderStatusResponse) [6]uint64 {
	cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal := aggregateResourcesREST(status)
	return [6]uint64{cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal}
}
