package ui

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"pkg.akt.dev/akt/internal/top/cache"
	"pkg.akt.dev/akt/internal/top/consensus"
	"pkg.akt.dev/akt/internal/top/governance"
	"pkg.akt.dev/akt/internal/top/rpc"
)

// Tab represents the active view tab
type Tab int

const (
	TabOverview Tab = iota
	TabValidators
	TabProviders
	TabGovernance
)

const (
	RenderInterval         = 100 * time.Millisecond // throttle re-renders
	ChainSyncInterval      = 10 * time.Minute
	ProviderCheckInterval  = 200 * time.Millisecond
	CacheSaveInterval      = 30 * time.Second
	GovernanceSyncInterval = 5 * time.Minute
	MaxConcurrentChecks    = 10
	MaxBlockHistory        = 50
)

// BlockValidatorVote stores a single validator's vote state for a block.
type BlockValidatorVote struct {
	Index       int
	Address     string
	PubKey      string // base64 consensus pubkey for moniker lookup
	VotingPower int64
	Prevoted    bool
	Precommited bool
}

// BlockRecord stores the final vote state of a completed block.
type BlockRecord struct {
	Height           int64
	PrevotePercent   float64
	PrecommitPercent float64
	Round            int
	Step             int
	Elapsed          time.Duration        // total time the block took
	Timestamp        time.Time            // when the record was captured
	Validators       []BlockValidatorVote // vote snapshot per validator
}

// Model represents the application state
type Model struct {
	// Core dependencies
	client       *rpc.Client
	rpcClient    *rpc.RPCProviderClient
	httpClient   *http.Client
	cache        cache.ProviderStore
	monikerCache *cache.MonikerCache

	// Consensus state
	state        *consensus.State
	monikers     map[string]string            // pubkey to moniker
	blockHistory []BlockRecord                // completed blocks, newest first
	snapshotCh   <-chan rpc.ConsensusSnapshot // WebSocket consensus stream
	wsConnected  bool                         // true if WebSocket is active

	// Per-validator block signing history. Maps validator index to a
	// slice of booleans (newest first). true = signed (precommited),
	// false = missed.
	valSignHistory map[int][]bool
	maxSignHistory int // max entries per validator

	// Render throttle: accumulate state from WebSocket events but
	// only apply to the rendered model on a periodic tick.
	pendingState *consensus.State // latest unapplied state from WebSocket

	// Peak vote percentages for the current block (tracks the highest
	// seen values so that a transient 0% at NewHeight doesn't erase
	// the real progress).
	peakHeight           int64
	peakPrevotePercent   float64
	peakPrecommitPercent float64
	peakRound            int
	peakStep             int
	blockStartTime       time.Time // StartTime of the current block

	// UI state
	width              int
	height             int
	activeTab          Tab
	scrollPos          int                  // for scrolling validator list
	overviewScroll     int                  // for scrolling block history in overview tab
	selectedBlock      int                  // highlighted block row (0=current, 1+=history index)
	expandedBlock      int                  // which block is expanded (-1=none, 0=current, 1+=history)
	expandedScroll     int                  // scroll within the expanded validator list
	expandedValidators []BlockValidatorVote // frozen snapshot of validators for expanded block
	quitting           bool

	// Provider state (embedded)
	providers ProviderList
	loader    ProviderLoader
	detail    ProviderDetail

	// Governance state
	governanceParams   *governance.AllParams
	governanceSelected int
	governanceScroll   int
}

// Message types
type (
	renderTickMsg        time.Time // periodic render trigger for throttled updates
	providerCheckTickMsg time.Time
	chainSyncTickMsg     time.Time
	cacheSaveTickMsg     time.Time

	// consensusSnapshotMsg is sent when a WebSocket consensus update arrives.
	consensusSnapshotMsg struct {
		snapshot rpc.ConsensusSnapshot
		ok       bool // false when the channel was closed (reconnect needed)
	}

	stateMsg struct {
		state *consensus.State
		err   error
	}

	monikersMsg struct {
		monikers map[string]string
		err      error
	}

	// providerCheckedMsg is sent when a single provider has been checked
	providerCheckedMsg struct {
		owner     string
		isOnline  bool
		version   string
		cpuAvail  uint64
		cpuTotal  uint64
		memAvail  uint64
		memTotal  uint64
		gpuAvail  uint64
		gpuTotal  uint64
		gpuModels []string
	}

	// chainSyncMsg is sent after syncing providers from chain
	chainSyncMsg struct {
		newProviders         []string
		activeLeaseProviders map[string]bool
		err                  error
	}

	// providerDetailMsg is sent when provider detail fetch completes
	providerDetailMsg struct {
		nodes []rpc.ProviderNodeWithGPU
		err   error
	}

	// governanceParamsMsg is sent when governance params fetch completes
	governanceParamsMsg struct {
		params *governance.AllParams
		err    error
	}
)

// ModelConfig holds configuration options for creating a new Model
type ModelConfig struct {
	Client             *rpc.Client
	RPCClient          *rpc.RPCProviderClient
	Cache              cache.ProviderStore
	MonikerCache       *cache.MonikerCache
	InsecureSkipVerify bool
}

// NewModel creates a new UI model
func NewModel(cfg ModelConfig) Model {
	monikers := cfg.MonikerCache.Get()
	return Model{
		client:         cfg.Client,
		rpcClient:      cfg.RPCClient,
		httpClient:     rpc.NewProviderHTTPClient(cfg.InsecureSkipVerify),
		cache:          cfg.Cache,
		monikerCache:   cfg.MonikerCache,
		monikers:       monikers,
		width:          80,
		height:         24,
		activeTab:      TabOverview,
		expandedBlock:  -1,
		valSignHistory: make(map[int][]bool),
		maxSignHistory: 50,
		loader: ProviderLoader{
			FirstRun: !cfg.Cache.HasProviders(),
			InFlight: make(map[string]bool),
		},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.connectWebSocket,
		m.fetchMonikers,
		m.fetchGovernanceParams,
		m.renderTick(),
		m.providerCheckTick(),
		m.chainSyncTick(),
		m.cacheSaveTick(),
		m.governanceSyncTick(),
	}

	if m.cache.HasProviders() {
		// Load from cache immediately
		cmds = append(cmds, m.loadFromCache)
	}

	// Always sync with chain on startup
	cmds = append(cmds, m.syncChain)

	return tea.Batch(cmds...)
}

// connectWebSocket establishes a WebSocket subscription for real-time
// consensus state.  Returns wsConnectedMsg on success or stateMsg with
// error on failure.
func (m Model) connectWebSocket() tea.Msg {
	ctx := context.Background()
	ch, err := m.client.SubscribeConsensusState(ctx)
	if err != nil {
		return stateMsg{err: err}
	}
	return wsConnectedMsg{ch: ch}
}

type wsConnectedMsg struct {
	ch <-chan rpc.ConsensusSnapshot
}

// loadFromCache loads providers from cache and updates the UI
func (m Model) loadFromCache() tea.Msg {
	return chainSyncMsg{
		newProviders: nil, // No new providers, just loading from cache
		err:          nil,
	}
}

func (m Model) syncChain() tea.Msg {
	ctx := context.Background()

	onChainProviders, err := m.fetchProviders(ctx)
	if err != nil {
		return chainSyncMsg{err: err}
	}

	activeLeaseProviders, err := m.rpcClient.GetActiveLeaseProviders(ctx, m.client.RESTEndpoint())
	if err != nil {
		activeLeaseProviders = make(map[string]bool)
	}

	newProviders := m.cache.SyncWithChain(onChainProviders)

	return chainSyncMsg{
		newProviders:         newProviders,
		activeLeaseProviders: activeLeaseProviders,
	}
}

func (m Model) fetchProviders(ctx context.Context) ([]rpc.OnChainProvider, error) {
	if m.loader.FirstRun && !m.cache.HasProviders() {
		providers, err := m.rpcClient.GetProvidersFromSeed(ctx)
		if err == nil {
			return providers, nil
		}
	}
	return m.rpcClient.GetProvidersOnChain(ctx)
}

func (m Model) checkProvider(owner string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		p, exists := m.cache.GetProvider(owner)
		if !exists {
			return providerCheckedMsg{owner: owner, isOnline: false}
		}

		// Try gRPC first for full GPU info, fall back to REST
		nodes, err := rpc.QueryProviderStatusGRPC(ctx, p.HostURI)
		if err != nil {
			// Fall back to REST (no GPU model info)
			status, restErr := rpc.QueryProviderStatus(ctx, m.httpClient, p.HostURI)
			if restErr != nil {
				return providerCheckedMsg{owner: owner, isOnline: false}
			}
			cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal := aggregateResourcesREST(status)
			version := m.queryProviderVersion(ctx, p.HostURI)
			return providerCheckedMsg{
				owner:    owner,
				isOnline: true,
				version:  version,
				cpuAvail: cpuAvail,
				cpuTotal: cpuTotal,
				memAvail: memAvail,
				memTotal: memTotal,
				gpuAvail: gpuAvail,
				gpuTotal: gpuTotal,
			}
		}

		cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal, gpuModels := aggregateResourcesGRPC(nodes)
		version := m.queryProviderVersion(ctx, p.HostURI)

		return providerCheckedMsg{
			owner:     owner,
			isOnline:  true,
			version:   version,
			cpuAvail:  cpuAvail,
			cpuTotal:  cpuTotal,
			memAvail:  memAvail,
			memTotal:  memTotal,
			gpuAvail:  gpuAvail,
			gpuTotal:  gpuTotal,
			gpuModels: gpuModels,
		}
	}
}

func aggregateResourcesREST(status *rpc.ProviderStatusResponse) (cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal uint64) {
	for _, node := range status.Cluster.Inventory.Available.Nodes {
		cpuAvail += node.Available.CPU
		cpuTotal += node.Allocatable.CPU
		memAvail += node.Available.Memory
		memTotal += node.Allocatable.Memory
		gpuAvail += node.Available.GPU
		gpuTotal += node.Allocatable.GPU
	}
	return
}

func aggregateResourcesGRPC(nodes []rpc.ProviderNodeWithGPU) (cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal uint64, gpuModels []string) {
	modelSet := make(map[string]bool)
	for _, node := range nodes {
		cpuAvail += node.CPUAvailable
		cpuTotal += node.CPUAllocatable
		memAvail += node.MemAvailable
		memTotal += node.MemAllocatable
		gpuAvail += node.GPUAvailable
		gpuTotal += node.GPUAllocatable

		// Collect unique GPU models
		for _, gpu := range node.GPUs {
			model := formatGPUModelShort(gpu)
			if model != "" && !modelSet[model] {
				modelSet[model] = true
				gpuModels = append(gpuModels, model)
			}
		}
	}
	return
}

func formatGPUModelShort(gpu rpc.GPUInfo) string {
	if gpu.Name == "" {
		return ""
	}
	// Return just the GPU name (e.g., "H100", "A100", "RTX 4090")
	return gpu.Name
}

func (m Model) queryProviderVersion(ctx context.Context, hostURI string) string {
	versionResp, err := rpc.QueryProviderVersion(ctx, m.httpClient, hostURI)
	if err != nil {
		return "unknown"
	}
	return versionResp.Akash.Version
}

func (m *Model) saveCache() {
	if err := m.cache.Save(); err != nil {
		m.loader.LastSaveError = err
	}
}

func (m *Model) dispatchProviderChecks() []tea.Cmd {
	available := MaxConcurrentChecks - len(m.loader.InFlight)
	if available <= 0 {
		return nil
	}

	var cmds []tea.Cmd
	dispatched := 0

	for _, owner := range m.loader.Queue {
		if dispatched >= available {
			break
		}
		if !m.loader.InFlight[owner] {
			m.loader.InFlight[owner] = true
			cmds = append(cmds, m.checkProvider(owner))
			dispatched++
		}
	}

	return cmds
}

// fetchMonikers fetches validator monikers only if cache is empty
func (m Model) fetchMonikers() tea.Msg {
	if m.monikerCache != nil && m.monikerCache.HasMonikers() {
		return monikersMsg{monikers: m.monikers}
	}

	ctx := context.Background()
	monikers, err := m.client.GetValidatorMonikers(ctx)
	if err != nil {
		return monikersMsg{err: err}
	}

	if m.monikerCache != nil {
		m.monikerCache.Set(monikers)
		_ = m.monikerCache.Save()
	}

	return monikersMsg{monikers: monikers}
}

func (m Model) renderTick() tea.Cmd {
	return tea.Tick(RenderInterval, func(t time.Time) tea.Msg {
		return renderTickMsg(t)
	})
}

func (m Model) providerCheckTick() tea.Cmd {
	return tea.Tick(ProviderCheckInterval, func(t time.Time) tea.Msg {
		return providerCheckTickMsg(t)
	})
}

func (m Model) chainSyncTick() tea.Cmd {
	return tea.Tick(ChainSyncInterval, func(t time.Time) tea.Msg {
		return chainSyncTickMsg(t)
	})
}

func (m Model) cacheSaveTick() tea.Cmd {
	return tea.Tick(CacheSaveInterval, func(t time.Time) tea.Msg {
		return cacheSaveTickMsg(t)
	})
}

func (m Model) governanceSyncTick() tea.Cmd {
	return tea.Tick(GovernanceSyncInterval, func(t time.Time) tea.Msg {
		return m.fetchGovernanceParams()
	})
}

func (m Model) fetchGovernanceParams() tea.Msg {
	ctx := context.Background()
	params, err := m.client.GetAllGovernanceParams(ctx)
	if err != nil {
		return governanceParamsMsg{err: err}
	}
	return governanceParamsMsg{params: params}
}

// rebuildProviderList rebuilds the provider list from cache
func (m *Model) rebuildProviderList() {
	cached := m.cache.GetAllProviders()

	// Deduplicate by HostURI - keep the most recently seen provider
	type providerWithTime struct {
		provider     rpc.Provider
		lastSeenTime time.Time
	}
	byURI := make(map[string]providerWithTime)

	for owner, p := range cached {
		if !p.IsOnline {
			continue
		}
		if strings.Contains(p.HostURI, "localhost") || strings.Contains(p.HostURI, "127.0.0.1") {
			continue
		}
		if p.Version == "" || p.Version == "unknown" {
			continue
		}

		provider := rpc.Provider{
			Owner:        owner,
			HostURI:      p.HostURI,
			Name:         p.Name,
			AkashVersion: p.Version,
			IsOnline:     p.IsOnline,
			Country:      p.Country,
			CPUAvailable: p.CPUAvailable,
			CPUTotal:     p.CPUTotal,
			MemAvailable: p.MemAvailable,
			MemTotal:     p.MemTotal,
			GPUAvailable: p.GPUAvailable,
			GPUTotal:     p.GPUTotal,
			GPUModels:    p.GPUModels,
		}

		// Keep the most recently seen provider for each URI
		existing, exists := byURI[p.HostURI]
		if !exists || p.LastSeenOnline.After(existing.lastSeenTime) {
			byURI[p.HostURI] = providerWithTime{
				provider:     provider,
				lastSeenTime: p.LastSeenOnline,
			}
		}
	}

	// Convert map to slice
	items := make([]rpc.Provider, 0, len(byURI))
	for _, pt := range byURI {
		items = append(items, pt.provider)
	}

	// Sort: selected version first, then by version (latest first), then by URL
	sort.SliceStable(items, func(i, j int) bool {
		iSelected := items[i].AkashVersion == m.providers.Version
		jSelected := items[j].AkashVersion == m.providers.Version

		// Selected version comes first
		if iSelected != jSelected {
			return iSelected
		}

		// Within same selection status, sort by version (latest first)
		cmp := rpc.CompareVersions(items[i].AkashVersion, items[j].AkashVersion)
		if cmp != 0 {
			return cmp > 0
		}
		return items[i].HostURI < items[j].HostURI
	})

	m.providers.Items = items
	m.providers.Versions = rpc.GetProviderVersions(items)

	// Update selected version if needed
	if m.providers.Version == "" && len(m.providers.Versions) > 0 {
		m.providers.Version = m.providers.Versions[0]
		m.providers.VersionIdx = 0
	}
}

// sortProviders re-sorts the provider list based on selected version
func (m *Model) sortProviders() {
	sort.SliceStable(m.providers.Items, func(i, j int) bool {
		iSelected := m.providers.Items[i].AkashVersion == m.providers.Version
		jSelected := m.providers.Items[j].AkashVersion == m.providers.Version

		// Selected version comes first
		if iSelected != jSelected {
			return iSelected
		}

		// Within same selection status, sort by version (latest first)
		cmp := rpc.CompareVersions(m.providers.Items[i].AkashVersion, m.providers.Items[j].AkashVersion)
		if cmp != 0 {
			return cmp > 0
		}
		return m.providers.Items[i].HostURI < m.providers.Items[j].HostURI
	})
}

// buildProviderQueue builds the queue of providers to check based on priority
func (m *Model) buildProviderQueue(activeLeaseProviders map[string]bool) {
	m.loader.ActiveLease = activeLeaseProviders

	// Get all providers sorted by priority
	allProviders := m.cache.GetProvidersByPriority()

	// If first run, prioritize active lease providers
	if m.loader.FirstRun && len(activeLeaseProviders) > 0 {
		var prioritized []string
		var others []string

		for _, owner := range allProviders {
			if activeLeaseProviders[owner] {
				prioritized = append(prioritized, owner)
			} else {
				others = append(others, owner)
			}
		}

		m.loader.Queue = append(prioritized, others...)
	} else {
		m.loader.Queue = allProviders
	}

	m.loader.Total = len(m.loader.Queue)
	m.loader.Checked = 0
	m.loader.Loading = len(m.loader.Queue) > 0
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	// Render throttle: apply pending consensus state at fixed intervals.
	case renderTickMsg:
		cmd := m.renderTick()
		if m.pendingState != nil {
			m.handleStateMsgInPlace(m.pendingState)
			m.pendingState = nil
		}
		return m, cmd

	// WebSocket lifecycle
	case wsConnectedMsg:
		m.snapshotCh = msg.ch
		m.wsConnected = true
		return m, m.waitForSnapshot()
	case consensusSnapshotMsg:
		if !msg.ok {
			// Channel closed — WebSocket connection lost.
			m.wsConnected = false
			m.snapshotCh = nil
			return m, m.connectWebSocket
		}
		// Buffer the state — it will be applied on the next renderTick.
		if msg.snapshot.State != nil {
			m.pendingState = msg.snapshot.State
		}
		return m, m.waitForSnapshot()

	case providerCheckTickMsg:
		cmds := m.dispatchProviderChecks()
		cmds = append(cmds, m.providerCheckTick())
		return m, tea.Batch(cmds...)
	case chainSyncTickMsg:
		return m, tea.Batch(m.syncChain, m.chainSyncTick())
	case cacheSaveTickMsg:
		m.saveCache()
		return m, m.cacheSaveTick()
	case stateMsg:
		return m.handleStateMsg(msg)
	case monikersMsg:
		if msg.err == nil {
			m.monikers = msg.monikers
		}
		return m, nil
	case chainSyncMsg:
		return m.handleChainSyncMsg(msg)
	case providerCheckedMsg:
		return m.handleProviderCheckedMsg(msg)
	case providerDetailMsg:
		return m.handleProviderDetailMsg(msg)
	case governanceParamsMsg:
		return m.handleGovernanceParamsMsg(msg)
	}
	return m, nil
}

// waitForSnapshot returns a command that blocks until the next WebSocket
// consensus snapshot arrives.
func (m Model) waitForSnapshot() tea.Cmd {
	ch := m.snapshotCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		snap, ok := <-ch
		return consensusSnapshotMsg{snapshot: snap, ok: ok}
	}
}

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle detail view keys first
	if m.detail.Showing {
		return m.handleDetailViewKeys(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		m.saveCache()
		return m, tea.Quit
	case "r":
		if m.activeTab == TabProviders {
			return m, m.syncChain
		}
		if m.activeTab == TabGovernance {
			return m, m.fetchGovernanceParams
		}
		// Consensus is event-driven, no manual refresh needed.
	case "1":
		m.activeTab = TabOverview
	case "2":
		m.activeTab = TabValidators
		m.scrollPos = 0
	case "3":
		m.activeTab = TabProviders
		m.providers.ScrollPos = 0
		m.providers.SelectedIdx = 0
	case "4":
		m.activeTab = TabGovernance
	case "tab":
		m.activeTab = (m.activeTab + 1) % 4
		m.resetScrollForTab()
	case "up", "k":
		if m.activeTab == TabGovernance {
			if m.governanceSelected > 0 {
				m.governanceSelected--
				m.governanceScroll = 0
			}
		} else {
			m.scrollUp()
		}
	case "down", "j":
		if m.activeTab == TabGovernance {
			if m.governanceSelected < len(governance.ModuleOrder)-1 {
				m.governanceSelected++
				m.governanceScroll = 0
			}
		} else {
			m.scrollDown()
		}
	case "home", "g":
		m.scrollPos = 0
		m.overviewScroll = 0
		m.providers.ScrollPos = 0
		m.providers.SelectedIdx = 0
		if m.activeTab == TabGovernance {
			m.governanceSelected = 0
			m.governanceScroll = 0
		}
	case "end", "G":
		m.scrollToEnd()
		if m.activeTab == TabGovernance {
			m.governanceSelected = len(governance.ModuleOrder) - 1
			m.governanceScroll = 0
		}
	case "left", "h":
		if m.activeTab == TabOverview && m.expandedBlock >= 0 {
			m.expandedBlock = -1
			m.expandedScroll = 0
			m.expandedValidators = nil
		} else if m.activeTab == TabGovernance {
			if m.governanceScroll > 0 {
				m.governanceScroll--
			}
		} else {
			m.selectPreviousVersion()
		}
	case "right", "l":
		if m.activeTab == TabGovernance {
			// Only allow scrolling if params don't fit in window
			if m.governanceParams != nil && m.governanceSelected < len(governance.ModuleOrder) {
				module := governance.ModuleOrder[m.governanceSelected]
				if modParams, ok := m.governanceParams.Modules[module]; ok && modParams != nil && modParams.Error == nil {
					paramLines := governance.CountJSONLines(modParams.RawJSON)
					maxVisible := m.height - 6
					if paramLines > maxVisible {
						m.governanceScroll++
					}
				}
			}
		} else {
			m.selectNextVersion()
		}
	case "enter":
		if m.activeTab == TabOverview {
			m.toggleBlockExpansion()
		} else if m.activeTab == TabProviders {
			return m.enterProviderDetail()
		}
	case "esc", "backspace":
		if m.activeTab == TabOverview && m.expandedBlock >= 0 {
			m.expandedBlock = -1
			m.expandedScroll = 0
			m.expandedValidators = nil
		}
	}
	return m, nil
}

func (m *Model) handleDetailViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		m.saveCache()
		return m, tea.Quit
	case "esc", "backspace":
		m.detail.Showing = false
		m.detail.Nodes = nil
		m.detail.Provider = nil
		m.detail.Error = nil
		m.detail.Loading = false
		m.detail.ScrollPos = 0
	case "up", "k":
		if m.detail.ScrollPos > 0 {
			m.detail.ScrollPos--
		}
	case "down", "j":
		m.scrollDetailDown()
	case "home", "g":
		m.detail.ScrollPos = 0
	case "end", "G":
		m.scrollDetailToEnd()
	case "1", "2", "3", "tab":
		// Exit detail view and switch tabs
		m.detail.Showing = false
		m.detail.Nodes = nil
		m.detail.Provider = nil
		m.detail.Error = nil
		m.detail.Loading = false
		m.detail.ScrollPos = 0
		// Re-handle the key for tab switching
		return m.handleKeyMsg(msg)
	}
	return m, nil
}

func (m *Model) resetScrollForTab() {
	if m.activeTab == TabValidators {
		m.scrollPos = 0
	} else if m.activeTab == TabProviders {
		m.providers.ScrollPos = 0
	}
}

func (m *Model) scrollUp() {
	switch m.activeTab {
	case TabOverview:
		if m.expandedBlock >= 0 {
			// Scroll within expanded validator list.
			if m.expandedScroll > 0 {
				m.expandedScroll--
			}
		} else {
			// Move block selection up.
			if m.selectedBlock > 0 {
				m.selectedBlock--
				m.ensureBlockVisible()
			}
		}
	case TabValidators:
		if m.scrollPos > 0 {
			m.scrollPos--
		}
	case TabProviders:
		m.moveProviderSelection(-1)
	}
}

func (m *Model) scrollDown() {
	switch m.activeTab {
	case TabOverview:
		if m.expandedBlock >= 0 {
			// Scroll within expanded validator list.
			m.expandedScroll++
		} else {
			// Move block selection down.
			maxBlock := len(m.blockHistory) // 0=current, 1..N=history
			if m.selectedBlock < maxBlock {
				m.selectedBlock++
				m.ensureBlockVisible()
			}
		}
	case TabValidators:
		if m.state != nil {
			maxScroll := max(len(m.state.Validators)-(m.height-15), 0)
			if m.scrollPos < maxScroll {
				m.scrollPos++
			}
		}
	case TabProviders:
		m.moveProviderSelection(1)
	}
}

func (m *Model) moveProviderSelection(delta int) {
	filtered := m.getFilteredProviders()
	if len(filtered) == 0 {
		return
	}

	m.providers.SelectedIdx += delta
	if m.providers.SelectedIdx < 0 {
		m.providers.SelectedIdx = 0
	} else if m.providers.SelectedIdx >= len(filtered) {
		m.providers.SelectedIdx = len(filtered) - 1
	}

	m.ensureSelectionVisible()
}

func (m *Model) ensureSelectionVisible() {
	visibleRows := max(m.height-providerListOverhead, 5)
	if len(m.getFilteredProviders()) > visibleRows {
		visibleRows -= 2
	}

	if m.providers.SelectedIdx < m.providers.ScrollPos {
		m.providers.ScrollPos = m.providers.SelectedIdx
	} else if m.providers.SelectedIdx >= m.providers.ScrollPos+visibleRows {
		m.providers.ScrollPos = m.providers.SelectedIdx - visibleRows + 1
	}
}

func (m *Model) getFilteredProviders() []rpc.Provider {
	return filterNonLocalProviders(m.providers.Items)
}

func (m *Model) toggleBlockExpansion() {
	if m.expandedBlock == m.selectedBlock {
		// Collapse.
		m.expandedBlock = -1
		m.expandedScroll = 0
		m.expandedValidators = nil
	} else {
		// Expand — freeze a snapshot of the validator votes.
		m.expandedBlock = m.selectedBlock
		m.expandedScroll = 0

		if m.selectedBlock == 0 && m.state != nil {
			// Current live block — snapshot from state.
			m.expandedValidators = make([]BlockValidatorVote, len(m.state.Validators))
			for i, v := range m.state.Validators {
				m.expandedValidators[i] = BlockValidatorVote{
					Index:       v.Index,
					Address:     v.Address,
					PubKey:      v.PubKey,
					VotingPower: v.VotingPower,
					Prevoted:    v.Prevoted,
					Precommited: v.Precommited,
				}
			}
		} else if m.selectedBlock > 0 && m.selectedBlock-1 < len(m.blockHistory) {
			// History block — copy from record.
			m.expandedValidators = m.blockHistory[m.selectedBlock-1].Validators
		}
	}
}

func (m *Model) ensureBlockVisible() {
	// selectedBlock 0 = current (always visible, pinned).
	// selectedBlock 1+ maps to history index (selectedBlock-1).
	if m.selectedBlock == 0 {
		return
	}
	histIdx := m.selectedBlock - 1
	visibleRows := max(m.height-overviewOverhead, 3)
	if m.expandedBlock >= 0 {
		visibleRows = max(visibleRows/3, 2)
	}

	if histIdx < m.overviewScroll {
		m.overviewScroll = histIdx
	} else if histIdx >= m.overviewScroll+visibleRows {
		m.overviewScroll = histIdx - visibleRows + 1
	}
}

func (m *Model) scrollToEnd() {
	switch m.activeTab {
	case TabOverview:
		m.selectedBlock = len(m.blockHistory)
		m.ensureBlockVisible()
	case TabValidators:
		if m.state != nil {
			m.scrollPos = max(len(m.state.Validators)-(m.height-15), 0)
		}
	case TabProviders:
		filtered := m.getFilteredProviders()
		if len(filtered) > 0 {
			m.providers.SelectedIdx = len(filtered) - 1
			m.ensureSelectionVisible()
		}
	}
}

func (m *Model) selectPreviousVersion() {
	if m.activeTab != TabProviders || len(m.providers.Versions) == 0 {
		return
	}
	m.providers.VersionIdx--
	if m.providers.VersionIdx < 0 {
		m.providers.VersionIdx = len(m.providers.Versions) - 1
	}
	m.providers.Version = m.providers.Versions[m.providers.VersionIdx]
	m.providers.ScrollPos = 0
	m.providers.SelectedIdx = 0
	m.sortProviders()
}

func (m *Model) selectNextVersion() {
	if m.activeTab != TabProviders || len(m.providers.Versions) == 0 {
		return
	}
	m.providers.VersionIdx = (m.providers.VersionIdx + 1) % len(m.providers.Versions)
	m.providers.Version = m.providers.Versions[m.providers.VersionIdx]
	m.providers.ScrollPos = 0
	m.providers.SelectedIdx = 0
	m.sortProviders()
}

func (m *Model) enterProviderDetail() (tea.Model, tea.Cmd) {
	filtered := m.getFilteredProviders()
	if len(filtered) == 0 || m.providers.SelectedIdx >= len(filtered) {
		return m, nil
	}

	provider := filtered[m.providers.SelectedIdx]
	m.detail.Provider = &provider
	m.detail.Loading = true
	m.detail.Error = nil
	m.detail.Nodes = nil
	m.detail.Showing = true
	m.detail.ScrollPos = 0

	return m, m.fetchProviderDetail(provider.HostURI)
}

func (m *Model) fetchProviderDetail(hostURI string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		nodes, err := rpc.QueryProviderStatusGRPC(ctx, hostURI)
		if err != nil {
			return providerDetailMsg{err: err}
		}
		return providerDetailMsg{nodes: nodes}
	}
}

func (m *Model) scrollDetailDown() {
	visibleRows := max(m.height-nodeListOverhead, minVisibleNodes)
	maxScroll := max(len(m.detail.Nodes)-visibleRows, 0)
	if m.detail.ScrollPos < maxScroll {
		m.detail.ScrollPos++
	}
}

func (m *Model) scrollDetailToEnd() {
	visibleRows := max(m.height-nodeListOverhead, minVisibleNodes)
	m.detail.ScrollPos = max(len(m.detail.Nodes)-visibleRows, 0)
}

func (m *Model) handleProviderDetailMsg(msg providerDetailMsg) (tea.Model, tea.Cmd) {
	m.detail.Loading = false
	if msg.err != nil {
		m.detail.Error = msg.err
	} else {
		m.detail.Nodes = msg.nodes
	}
	return m, nil
}

func (m *Model) handleGovernanceParamsMsg(msg governanceParamsMsg) (tea.Model, tea.Cmd) {
	if msg.err == nil {
		m.governanceParams = msg.params
	}
	return m, nil
}

// handleStateMsgInPlace applies a consensus state update in-place.
// Used by the render throttle to avoid interface conversion issues.
func (m *Model) handleStateMsgInPlace(state *consensus.State) {
	m.handleStateMsg(stateMsg{state: state})
}

func (m *Model) handleStateMsg(msg stateMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if m.state == nil {
			m.state = &consensus.State{}
		}
		m.state.Error = msg.err
		return m, nil
	}

	newState := msg.state

	// Detect height change — snapshot the completed block.
	if m.peakHeight > 0 && newState.Height > m.peakHeight {
		now := time.Now()
		elapsed := time.Duration(0)
		if !m.blockStartTime.IsZero() {
			elapsed = now.Sub(m.blockStartTime)
		}

		// Snapshot validator votes from the current (outgoing) state
		// and record per-validator signing history.
		var valVotes []BlockValidatorVote
		if m.state != nil {
			valVotes = make([]BlockValidatorVote, len(m.state.Validators))
			for i, v := range m.state.Validators {
				valVotes[i] = BlockValidatorVote{
					Index:       v.Index,
					Address:     v.Address,
					PubKey:      v.PubKey,
					VotingPower: v.VotingPower,
					Prevoted:    v.Prevoted,
					Precommited: v.Precommited,
				}

				// Prepend signing status (newest first).
				hist := m.valSignHistory[v.Index]
				hist = append([]bool{v.Precommited}, hist...)
				if len(hist) > m.maxSignHistory {
					hist = hist[:m.maxSignHistory]
				}
				m.valSignHistory[v.Index] = hist
			}
		}

		rec := BlockRecord{
			Height:           m.peakHeight,
			PrevotePercent:   m.peakPrevotePercent,
			PrecommitPercent: m.peakPrecommitPercent,
			Round:            m.peakRound,
			Step:             m.peakStep,
			Elapsed:          elapsed,
			Timestamp:        now,
			Validators:       valVotes,
		}
		// Prepend (newest first).
		m.blockHistory = append([]BlockRecord{rec}, m.blockHistory...)
		if len(m.blockHistory) > MaxBlockHistory {
			m.blockHistory = m.blockHistory[:MaxBlockHistory]
		}

		// Reset peaks for the new height.
		m.peakPrevotePercent = 0
		m.peakPrecommitPercent = 0
		m.peakRound = 0
		m.peakStep = 0
		m.blockStartTime = time.Time{}
	}

	// Track peak vote percentages for the current height.
	m.peakHeight = newState.Height
	if newState.PrevotePercent > m.peakPrevotePercent {
		m.peakPrevotePercent = newState.PrevotePercent
	}
	if newState.PrecommitPercent > m.peakPrecommitPercent {
		m.peakPrecommitPercent = newState.PrecommitPercent
	}
	m.peakRound = newState.Round
	m.peakStep = newState.Step
	if m.blockStartTime.IsZero() {
		m.blockStartTime = newState.StartTime
	}

	m.state = newState
	return m, nil
}

func (m *Model) handleChainSyncMsg(msg chainSyncMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	m.loader.LastSync = time.Now()
	m.buildProviderQueue(msg.activeLeaseProviders)
	m.rebuildProviderList()

	cmds := m.dispatchProviderChecks()
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m *Model) handleProviderCheckedMsg(msg providerCheckedMsg) (tea.Model, tea.Cmd) {
	delete(m.loader.InFlight, msg.owner)
	m.removeFromQueue(msg.owner)
	m.loader.Checked++

	if msg.isOnline {
		m.cache.MarkProviderOnline(msg.owner, msg.version, msg.cpuAvail, msg.cpuTotal, msg.memAvail, msg.memTotal, msg.gpuAvail, msg.gpuTotal, msg.gpuModels)
	} else {
		m.cache.MarkProviderOffline(msg.owner)
	}

	m.rebuildProviderList()

	if len(m.loader.Queue) == 0 && len(m.loader.InFlight) == 0 {
		m.loader.Loading = false
		m.loader.FirstRun = false
		m.loader.Queue = m.cache.GetProvidersDueForCheck()
	}

	return m, nil
}

func (m *Model) removeFromQueue(owner string) {
	for i, o := range m.loader.Queue {
		if o == owner {
			m.loader.Queue = append(m.loader.Queue[:i], m.loader.Queue[i+1:]...)
			return
		}
	}
}

// View renders the UI
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	ctx := ViewContext{
		State:              m.state,
		Endpoint:           m.client.Endpoint(),
		Width:              m.width,
		Height:             m.height,
		ActiveTab:          m.activeTab,
		Monikers:           m.monikers,
		ScrollPos:          m.scrollPos,
		GovernanceParams:   m.governanceParams,
		GovernanceSelected: m.governanceSelected,
		GovernanceScroll:   m.governanceScroll,
		BlockHistory:       m.blockHistory,
		OverviewScroll:     m.overviewScroll,
		SelectedBlock:      m.selectedBlock,
		ExpandedBlock:      m.expandedBlock,
		ExpandedScroll:     m.expandedScroll,
		ExpandedValidators: m.expandedValidators,
		ValSignHistory:     m.valSignHistory,
		WSConnected:        m.wsConnected,
		Providers: ProviderViewState{
			Providers: m.providers.Items,
			Versions:  m.providers.Versions,
			Selected:  m.providers.Version,
			ScrollPos: m.providers.ScrollPos,
			Loading:   m.loader.Loading,
			Loaded:    m.loader.Checked,
			Total:     m.loader.Total,
			Detail: ProviderDetailState{
				Showing:     m.detail.Showing,
				Provider:    m.detail.Provider,
				Nodes:       m.detail.Nodes,
				Loading:     m.detail.Loading,
				Error:       m.detail.Error,
				ScrollPos:   m.detail.ScrollPos,
				SelectedIdx: m.providers.SelectedIdx,
			},
		},
	}

	return RenderView(ctx)
}
