package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/akt/internal/monitor/cache"
	"pkg.akt.dev/akt/internal/monitor/consensus"
	"pkg.akt.dev/akt/internal/monitor/governance"
	"pkg.akt.dev/akt/internal/monitor/rpc"
	"pkg.akt.dev/akt/internal/output/pretty"

	oracletypes "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/util/pubsub"
)

// HubTab represents the active hub dashboard in akt monitor.
type HubTab int

const (
	HubNetwork   HubTab = iota // Consensus, validators, governance
	HubProvider                // Provider fleet monitoring
	HubOracleBME               // Oracle prices + BME state
)

const hubTabCount = 3

// Tab represents the active sub-tab within the Network dashboard.
type Tab int

const (
	TabOverview Tab = iota
	TabValidators
	TabGovernance
	TabParameters
)

const networkTabCount = 4

const (
	RenderInterval                 = 100 * time.Millisecond // throttle re-renders
	ChainSyncInterval              = 10 * time.Minute
	ProviderCheckInterval          = 200 * time.Millisecond
	CacheSaveInterval              = 30 * time.Second
	GovernanceSyncInterval         = 5 * time.Minute
	GovernanceProposalSyncInterval = 30 * time.Second
	OracleSyncInterval             = 30 * time.Second
	MaxConcurrentChecks            = 10
	MaxBlockHistory                = 50
	consensusReconnectBaseDelay    = 250 * time.Millisecond
	consensusReconnectMaxDelay     = 8 * time.Second
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

// BackMsg is sent by an embedded Model (Embedded=true in ModelConfig)
// when the user requests to leave the monitor view (q or Esc with nothing
// expanded). The parent TUI handles this by navigating back.
type BackMsg struct{}

// StatusInfo holds status bar information exposed by the monitor model
// so the parent TUI can render a unified status bar.
type StatusInfo struct {
	Endpoint      string
	WSConnected   bool
	HubTab        HubTab
	ActiveTab     Tab
	DetailShowing bool
}

// StatusInfo returns the current status bar information.
// This allows the parent TUI to render a unified status bar
// when the monitor model is embedded.
func (m Model) StatusInfo() StatusInfo {
	return StatusInfo{
		Endpoint:      m.client.Endpoint(),
		WSConnected:   m.wsConnected,
		HubTab:        m.hubTab,
		ActiveTab:     m.activeTab,
		DetailShowing: m.detail.Showing,
	}
}

// TabHelpText returns the keybinding help text for the active hub/tab.
func (si StatusInfo) TabHelpText() string {
	switch si.HubTab {
	case HubProvider:
		return "Tab: dashboard | j/k: scroll | enter: detail | r: refresh"
	case HubOracleBME:
		return "Tab: dashboard | j/k: scroll | r: refresh"
	default: // HubNetwork
		switch si.ActiveTab {
		case TabValidators:
			return "Tab: dashboard | 1-4: sub-tab | j/k: scroll | r: refresh"
		case TabGovernance:
			return "Tab: dashboard | 1-4: sub-tab | j/k: scroll | r: refresh"
		case TabParameters:
			return "Tab: dashboard | 1-4: sub-tab | j/k: select | r: refresh"
		default:
			return "Tab: dashboard | 1-4: sub-tab | j/k: select | enter: expand"
		}
	}
}

// Model represents the application state
type Model struct {
	// Core dependencies
	client         *rpc.Client
	rpcClient      *rpc.RPCProviderClient
	httpClient     *http.Client
	runtimeContext context.Context
	runtimeTasks   *RuntimeTaskGroup
	// Auxiliary query seams keep cancellation and error handling independently
	// testable at the model boundary.
	validatorMonikersQuery func(context.Context) (map[string]string, error)
	oracleStateQuery       func(context.Context) (*rpc.OracleState, error)
	bmeStateQuery          func(context.Context) (*rpc.BMEState, error)
	governanceParamsQuery  func(context.Context) (*governance.AllParams, error)
	activeLeaseQuery       func(context.Context, string) (map[string]bool, error)
	// Provider query seams keep the command lifecycle independently testable.
	providerStatusGRPC func(context.Context, string, bool) ([]rpc.ProviderNodeWithGPU, error)
	providerStatusREST func(context.Context, *http.Client, string) (*rpc.ProviderStatusResponse, error)
	// insecureSkipVerify applies uniformly to provider REST and gRPC probes.
	insecureSkipVerify bool
	cache              cache.ProviderStore
	monikerCache       *cache.MonikerCache
	embedded           bool // when true, q sends BackMsg instead of tea.Quit

	// Consensus state
	state        *consensus.State
	monikers     map[string]string            // pubkey to moniker
	blockHistory []BlockRecord                // completed blocks, newest first
	snapshotCh   <-chan rpc.ConsensusSnapshot // WebSocket consensus stream
	wsConnected  bool                         // true if WebSocket is active
	// consensusRetryAttempt drives capped exponential reconnect delay.
	consensusRetryAttempt int

	// Per-validator block signing history. Maps validator index to a
	// slice of booleans (newest first). true = signed (precommited),
	// false = missed.
	valSignHistory  map[int][]bool
	proposerHistory []int // proposer index for each historical block (newest first)
	maxSignHistory  int   // max entries per validator

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
	lastProposerHeight   int64     // height for which proposer was last fetched
	firstHeightSeen      bool      // true after the first height change (data unreliable, skip recording)
	knownProposerIndex   int       // last known proposer index from HTTP fetch
	knownProposerAddr    string    // last known proposer address

	// UI state
	width              int
	height             int
	hubTab             HubTab               // active hub dashboard (Network, Provider, BME)
	activeTab          Tab                  // sub-tab within the Network dashboard
	overviewScroll     int                  // scroll position for block history in overview tab
	selectedBlock      int                  // highlighted block row (0=current, 1+=history index)
	expandedBlock      int                  // which block is expanded (-1=none, 0=current, 1+=history)
	expandedScroll     int                  // scroll within the expanded validator list
	expandedValidators []BlockValidatorVote // frozen snapshot of validators for expanded block
	expandedValidator  int                  // which validator is expanded (-1=none)
	quitting           bool

	// Provider state (embedded)
	providers ProviderList
	loader    ProviderLoader
	detail    ProviderDetail
	// detailRequestID correlates async provider detail results with the most
	// recent selection, including repeated requests to the same provider.
	detailRequestID uint64

	// Governance state
	governanceProposals    *govv1.QueryProposalsResponse
	governanceProposalsErr error
	governanceParams       *governance.AllParams

	// Bubbles component models for tables/lists/viewports
	providerTable   table.Model
	nodeTable       table.Model
	validatorTable  table.Model
	blockTable      table.Model
	govModuleIdx    int // selected module index in governance.ModuleOrder
	govModuleScroll int // first visible module index (scroll offset)
	govModuleHeight int // visible rows for the module list
	govProposalView viewport.Model
	govParamView    viewport.Model

	// Event bus — shared across the application; carries all typed ABCI
	// events.  Individual dashboards subscribe and filter by type.
	bus        pubsub.Bus
	subscriber pubsub.Subscriber

	// Oracle state — populated from bus events
	oracle OracleState
}

// Message types
type (
	renderTickMsg              time.Time // periodic render trigger for throttled updates
	providerCheckTickMsg       time.Time
	chainSyncTickMsg           time.Time
	cacheSaveTickMsg           time.Time
	governanceProposalsTickMsg time.Time
	governanceParamsTickMsg    time.Time

	// busEventMsg carries a single typed event from the shared pubsub bus.
	busEventMsg struct {
		event pubsub.Event
		ok    bool // false when subscriber channel closed
	}

	// consensusSnapshotMsg is sent when a WebSocket consensus update arrives.
	consensusSnapshotMsg struct {
		snapshot rpc.ConsensusSnapshot
		ok       bool // false when the channel was closed (reconnect needed)
	}
	consensusReconnectMsg struct{}

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
		err       error
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
		hostURI   string
		requestID uint64
		nodes     []rpc.ProviderNodeWithGPU
		err       error
	}

	// governanceParamsMsg is sent when governance params fetch completes
	governanceProposalsMsg struct {
		proposals *govv1.QueryProposalsResponse
		err       error
	}

	governanceParamsMsg struct {
		params *governance.AllParams
		err    error
	}

	// oracleStateMsg is sent when the initial/periodic oracle REST
	// fetch completes.
	oracleStateMsg struct {
		state *rpc.OracleState
		err   error
	}

	// bmeStateMsg is sent when the initial/periodic BME REST
	// fetch completes.
	bmeStateMsg struct {
		state *rpc.BMEState
		err   error
	}

	// initialSigningMsg is sent after fetching the latest commit to seed
	// signing history so the oldest block in the bar shows accurate data.
	initialSigningMsg struct {
		height     int64
		signers    map[string]bool // uppercase hex addresses of validators that signed
		validators []consensus.Validator
		err        error
	}

	// proposerMsg carries the current proposer info fetched via HTTP.
	proposerMsg struct {
		height        int64
		proposerIndex int
		proposerAddr  string
		err           error
	}
)

// ModelConfig holds configuration options for creating a new Model.
type ModelConfig struct {
	Client             *rpc.Client
	RPCClient          *rpc.RPCProviderClient
	RuntimeContext     context.Context
	RuntimeTasks       *RuntimeTaskGroup
	Cache              cache.ProviderStore
	MonikerCache       *cache.MonikerCache
	InsecureSkipVerify bool
	Embedded           bool       // when true, q sends BackMsg instead of tea.Quit
	InitialDashboard   string     // "network" (default), "provider", "oracle", "bme"
	Bus                pubsub.Bus // shared event bus; may be nil
}

// NewModel creates a new UI model
func NewModel(cfg ModelConfig) Model {
	monikers := cfg.MonikerCache.Get()
	runtimeContext := cfg.RuntimeContext
	if runtimeContext == nil {
		runtimeContext = context.Background()
	}

	var initialHub HubTab
	switch cfg.InitialDashboard {
	case "provider":
		initialHub = HubProvider
	case "oracle", "bme", "oracle-bme":
		initialHub = HubOracleBME
	default:
		initialHub = HubNetwork
	}

	m := Model{
		client:             cfg.Client,
		rpcClient:          cfg.RPCClient,
		httpClient:         rpc.NewProviderHTTPClient(cfg.InsecureSkipVerify),
		runtimeContext:     runtimeContext,
		runtimeTasks:       cfg.RuntimeTasks,
		insecureSkipVerify: cfg.InsecureSkipVerify,
		cache:              cfg.Cache,
		monikerCache:       cfg.MonikerCache,
		embedded:           cfg.Embedded,
		monikers:           monikers,
		width:              80,
		height:             24,
		hubTab:             initialHub,
		activeTab:          TabOverview,
		expandedBlock:      -1,
		expandedValidator:  -1,
		knownProposerIndex: -1,
		valSignHistory:     make(map[int][]bool),
		maxSignHistory:     50,
		bus:                cfg.Bus,
		oracle: OracleState{
			Aggregated: make(map[string]*oracletypes.EventAggregatedPrice),
		},
		loader: ProviderLoader{
			FirstRun: !cfg.Cache.HasProviders(),
			InFlight: make(map[string]bool),
		},
	}

	// Initialize bubbles table components.
	providerCols := []table.Column{
		{Title: "#", Width: colWidthIndex},
		{Title: "Provider", Width: colWidthProvider},
		{Title: "Version", Width: colWidthVersion},
		{Title: "CPU", Width: colWidthCPU},
		{Title: "Memory", Width: colWidthMem},
		{Title: "GPU", Width: colWidthGPU},
		{Title: "Loc", Width: colWidthCountry},
	}
	m.providerTable = table.New(table.WithColumns(providerCols), table.WithFocused(true))

	validatorCols := []table.Column{
		{Title: "#", Width: 5},
		{Title: "Validator", Width: 28},
		{Title: "Power", Width: 18},
		{Title: "Blocks (newest \u2190)", Width: 40},
	}
	m.validatorTable = table.New(table.WithColumns(validatorCols), table.WithFocused(true))

	blockCols := []table.Column{
		{Title: "Height", Width: colHeight},
		{Title: "PV", Width: colPV},
		{Title: "PC", Width: colPC},
		{Title: "Elapsed", Width: colElapsed},
		{Title: "R/S", Width: colRS},
	}
	m.blockTable = table.New(table.WithColumns(blockCols), table.WithFocused(true))

	nodeCols := []table.Column{
		{Title: "Node", Width: colWidthNodeName},
		{Title: "CPU", Width: 14},
		{Title: "Memory", Width: 16},
		{Title: "GPU", Width: 30},
	}
	m.nodeTable = table.New(table.WithColumns(nodeCols), table.WithFocused(true))

	// Configure table styles: transparent cell style so pre-styled
	// strings in rows pass through; header uses muted style.
	ts := table.DefaultStyles()
	ts.Header = mutedStyle
	ts.Cell = lipgloss.NewStyle()
	ts.Selected = highlightStyle
	m.providerTable.SetStyles(ts)
	m.validatorTable.SetStyles(ts)
	m.blockTable.SetStyles(ts)
	m.nodeTable.SetStyles(ts)

	m.govProposalView = viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	m.govParamView = viewport.New(viewport.WithWidth(60), viewport.WithHeight(20))
	m.govModuleHeight = 20 // updated in resizeComponents

	// Subscribe to the shared event bus if available.
	if cfg.Bus != nil {
		sub, err := cfg.Bus.Subscribe()
		if err == nil {
			m.subscriber = sub
		}
	}

	m.resizeComponents()

	return m
}

func (m Model) runRuntimeTask(task func() tea.Msg) tea.Msg {
	return m.runtimeTasks.run(task)
}

// fetchProposer queries /consensus_state to get the current block proposer.
func (m Model) fetchProposer() tea.Msg {
	return m.runRuntimeTask(func() tea.Msg {
		state, err := m.client.GetConsensusStateWithValidators(m.runtimeContext)
		if err != nil {
			return proposerMsg{err: err}
		}
		return proposerMsg{
			height:        state.Height,
			proposerIndex: state.ProposerIndex,
			proposerAddr:  state.ProposerAddress,
		}
	})
}

// fetchInitialSigning queries the latest commit to seed signing history
// so the oldest block in the bar shows accurate data.
func (m Model) fetchInitialSigning() tea.Msg {
	return m.runRuntimeTask(func() tea.Msg {
		height, signers, err := m.client.GetLatestCommit(m.runtimeContext)
		if err != nil {
			return initialSigningMsg{err: err}
		}
		validators, err := m.client.GetValidatorsAtHeight(m.runtimeContext, height)
		if err != nil {
			return initialSigningMsg{err: err}
		}
		return initialSigningMsg{height: height, signers: signers, validators: validators}
	})
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.connectWebSocket,
		m.fetchMonikers,
		m.fetchInitialSigning,
		m.fetchProposer,
		m.fetchGovernanceProposals,
		m.fetchGovernanceParams,
		m.fetchOracleState,
		m.fetchBMEState,
		m.renderTick(),
		m.governanceProposalSyncTick(),
		m.governanceSyncTick(),
		tea.Sequence(m.loadFromCache, m.syncChain),
		m.providerCheckTick(),
		m.chainSyncTick(),
		m.cacheSaveTick(),
	}

	if m.subscriber != nil {
		cmds = append(cmds, m.waitForBusEvent())
	}

	return tea.Batch(cmds...)
}

// connectWebSocket establishes a WebSocket subscription for real-time
// consensus state.  Returns wsConnectedMsg on success or stateMsg with
// error on failure.
func (m Model) connectWebSocket() tea.Msg {
	return m.runRuntimeTask(func() tea.Msg {
		subscription, err := m.client.SubscribeConsensusState(m.runtimeContext)
		if err != nil {
			return stateMsg{err: err}
		}
		m.runtimeTasks.adopt(subscription.Done())
		return wsConnectedMsg{ch: subscription.Snapshots}
	})
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
	return m.runRuntimeTask(func() tea.Msg {
		ctx := m.runtimeContext
		if err := ctx.Err(); err != nil {
			return chainSyncMsg{err: err}
		}

		onChainProviders, err := m.fetchProviders(ctx)
		if err != nil {
			return chainSyncMsg{err: err}
		}

		activeLeaseProviders, err := m.queryActiveLeaseProviders(ctx, m.client.RESTEndpoint())
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return chainSyncMsg{err: ctxErr}
			}
			activeLeaseProviders = make(map[string]bool)
		}
		if err := ctx.Err(); err != nil {
			return chainSyncMsg{err: err}
		}

		newProviders := m.cache.SyncWithChain(onChainProviders)

		return chainSyncMsg{
			newProviders:         newProviders,
			activeLeaseProviders: activeLeaseProviders,
		}
	})
}

func (m Model) queryActiveLeaseProviders(ctx context.Context, restEndpoint string) (map[string]bool, error) {
	if m.activeLeaseQuery != nil {
		return m.activeLeaseQuery(ctx, restEndpoint)
	}
	return m.rpcClient.GetActiveLeaseProviders(ctx, restEndpoint)
}

func (m Model) fetchProviders(ctx context.Context) ([]rpc.OnChainProvider, error) {
	if m.loader.FirstRun && !m.cache.HasProviders() {
		providers, err := m.rpcClient.GetProvidersFromSeed(ctx)
		if err == nil {
			return providers, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
	}
	return m.rpcClient.GetProvidersOnChain(ctx)
}

func (m Model) checkProvider(owner string) tea.Cmd {
	return func() tea.Msg {
		return m.runRuntimeTask(func() tea.Msg {
			ctx := m.runtimeContext
			if err := ctx.Err(); err != nil {
				return providerCheckedMsg{owner: owner, err: err}
			}

			p, exists := m.cache.GetProvider(owner)
			if !exists {
				return providerCheckedMsg{owner: owner, isOnline: false}
			}

			// Try gRPC first for full GPU info, fall back to REST.
			nodes, err := m.queryProviderStatusGRPC(ctx, p.HostURI)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return providerCheckedMsg{owner: owner, err: ctxErr}
				}
				status, restErr := m.queryProviderStatusREST(ctx, p.HostURI)
				if restErr != nil {
					if ctxErr := ctx.Err(); ctxErr != nil {
						return providerCheckedMsg{owner: owner, err: ctxErr}
					}
					return providerCheckedMsg{owner: owner, isOnline: false}
				}
				cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal := aggregateResourcesREST(status)
				version := m.queryProviderVersion(ctx, p.HostURI)
				if ctxErr := ctx.Err(); ctxErr != nil {
					return providerCheckedMsg{owner: owner, err: ctxErr}
				}
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
			if ctxErr := ctx.Err(); ctxErr != nil {
				return providerCheckedMsg{owner: owner, err: ctxErr}
			}

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
		})
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

func (m Model) queryProviderStatusGRPC(ctx context.Context, hostURI string) ([]rpc.ProviderNodeWithGPU, error) {
	if m.providerStatusGRPC != nil {
		return m.providerStatusGRPC(ctx, hostURI, m.insecureSkipVerify)
	}
	return rpc.QueryProviderStatusGRPC(ctx, hostURI, m.insecureSkipVerify)
}

func (m Model) queryProviderStatusREST(ctx context.Context, hostURI string) (*rpc.ProviderStatusResponse, error) {
	if m.providerStatusREST != nil {
		return m.providerStatusREST(ctx, m.httpClient, hostURI)
	}
	return rpc.QueryProviderStatus(ctx, m.httpClient, hostURI)
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
	if m.runtimeContext != nil && m.runtimeContext.Err() != nil {
		return nil
	}

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
	return m.runRuntimeTask(func() tea.Msg {
		if m.monikerCache != nil && m.monikerCache.HasMonikers() {
			return monikersMsg{monikers: m.monikers}
		}

		monikers, err := m.queryValidatorMonikers(m.runtimeContext)
		if err != nil {
			return monikersMsg{err: err}
		}
		if err := m.runtimeContext.Err(); err != nil {
			return monikersMsg{err: err}
		}

		if m.monikerCache != nil {
			m.monikerCache.Set(monikers)
			_ = m.monikerCache.Save()
		}

		return monikersMsg{monikers: monikers}
	})
}

func (m Model) queryValidatorMonikers(ctx context.Context) (map[string]string, error) {
	if m.validatorMonikersQuery != nil {
		return m.validatorMonikersQuery(ctx)
	}
	return m.client.GetValidatorMonikers(ctx)
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
		return governanceParamsTickMsg(t)
	})
}

func (m Model) governanceProposalSyncTick() tea.Cmd {
	return tea.Tick(GovernanceProposalSyncInterval, func(t time.Time) tea.Msg {
		return governanceProposalsTickMsg(t)
	})
}

func (m Model) oracleSyncTick() tea.Cmd {
	return tea.Tick(OracleSyncInterval, func(t time.Time) tea.Msg {
		return m.fetchOracleState()
	})
}

func (m Model) fetchOracleState() tea.Msg {
	return m.runRuntimeTask(func() tea.Msg {
		state, err := m.queryOracleState(m.runtimeContext)
		if err != nil {
			return oracleStateMsg{err: err}
		}
		if err := m.runtimeContext.Err(); err != nil {
			return oracleStateMsg{err: err}
		}
		return oracleStateMsg{state: state}
	})
}

func (m Model) queryOracleState(ctx context.Context) (*rpc.OracleState, error) {
	if m.oracleStateQuery != nil {
		return m.oracleStateQuery(ctx)
	}
	return m.client.GetOracleState(ctx)
}

func (m Model) bmeSyncTick() tea.Cmd {
	return tea.Tick(OracleSyncInterval, func(t time.Time) tea.Msg {
		return m.fetchBMEState()
	})
}

func (m Model) fetchBMEState() tea.Msg {
	return m.runRuntimeTask(func() tea.Msg {
		state, err := m.queryBMEState(m.runtimeContext)
		if err != nil {
			return bmeStateMsg{err: err}
		}
		if err := m.runtimeContext.Err(); err != nil {
			return bmeStateMsg{err: err}
		}
		return bmeStateMsg{state: state}
	})
}

func (m Model) queryBMEState(ctx context.Context) (*rpc.BMEState, error) {
	if m.bmeStateQuery != nil {
		return m.bmeStateQuery(ctx)
	}
	return m.client.GetBMEState(ctx)
}

func (m Model) fetchGovernanceParams() tea.Msg {
	return m.runRuntimeTask(func() tea.Msg {
		params, err := m.queryGovernanceParams(m.runtimeContext)
		if err != nil {
			return governanceParamsMsg{err: err}
		}
		if err := m.runtimeContext.Err(); err != nil {
			return governanceParamsMsg{err: err}
		}
		return governanceParamsMsg{params: params}
	})
}

func (m Model) queryGovernanceParams(ctx context.Context) (*governance.AllParams, error) {
	if m.governanceParamsQuery != nil {
		return m.governanceParamsQuery(ctx)
	}
	return m.client.GetAllGovernanceParams(ctx)
}

func (m Model) fetchGovernanceProposals() tea.Msg {
	return m.runRuntimeTask(func() tea.Msg {
		proposals, err := m.client.GetGovernanceProposals(m.runtimeContext)
		return governanceProposalsMsg{proposals: proposals, err: err}
	})
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
	m.reconcileProviderVersionSelection()

	m.rebuildProviderTableRows()
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

// rebuildProviderTableRows rebuilds the provider table rows from current data.
func (m *Model) rebuildProviderTableRows() {
	filtered := m.getFilteredProviders()
	rows := make([]table.Row, len(filtered))
	for i, p := range filtered {
		country := p.Country
		if country == "" {
			country = "--"
		}
		rows[i] = table.Row{
			fmt.Sprintf("%d", i+1),
			formatProviderURL(p.HostURI, colWidthProvider-2),
			p.AkashVersion,
			formatResourceRatio(p.CPUAvailable/1000, p.CPUTotal/1000),
			formatMemoryRatio(p.MemAvailable, p.MemTotal),
			formatProviderGPU(p),
			country,
		}
	}
	m.providerTable.SetRows(rows)
	m.providerTable.UpdateViewport()
}

// rebuildValidatorTableRows rebuilds the validator table rows from current state.
func (m *Model) rebuildValidatorTableRows() {
	if m.state == nil || len(m.state.Validators) == 0 {
		m.validatorTable.SetRows(nil)
		m.validatorTable.UpdateViewport()
		return
	}
	nameW := 28
	blocksW := max(m.width-5-nameW-18-7, 20)
	rows := make([]table.Row, len(m.state.Validators))
	for i, v := range m.state.Validators {
		displayName := getValidatorDisplayName(v, m.monikers)
		if len(displayName) > nameW {
			displayName = displayName[:nameW-3] + "..."
		}
		power := formatPower(v.VotingPower)
		pct := ""
		if m.state.TotalVotingPower > 0 {
			pct = fmt.Sprintf("%.1f%%", float64(v.VotingPower)/float64(m.state.TotalVotingPower)*100)
		}
		powerCell := fmt.Sprintf("%s %s", power, pct)
		hist := m.valSignHistory[v.Index]
		bar := renderSigningBar(hist, v.Index, m.proposerHistory, -1, blocksW)

		rows[i] = table.Row{
			fmt.Sprintf("%d", v.Index),
			displayName,
			powerCell,
			bar,
		}
	}
	m.validatorTable.SetRows(rows)
	m.validatorTable.UpdateViewport()
}

// blockRowForTable holds data for a single block row in the table.
type blockRowForTable struct {
	height    int64
	pvPct     float64
	pcPct     float64
	elapsed   time.Duration
	round     int
	step      int
	isCurrent bool
}

// rebuildBlockTableRows rebuilds the block table rows, with the current
// (live) block as the first row followed by completed history blocks.
func (m *Model) rebuildBlockTableRows() {
	var allBlocks []blockRowForTable

	// Current live block is always the first row.
	if m.state != nil && m.state.Height > 0 {
		elapsed := m.state.Elapsed
		if elapsed < 0 {
			elapsed = 0
		}
		allBlocks = append(allBlocks, blockRowForTable{
			height:    m.state.Height,
			pvPct:     m.state.PrevotePercent,
			pcPct:     m.state.PrecommitPercent,
			elapsed:   elapsed,
			round:     m.state.Round,
			step:      m.state.Step,
			isCurrent: true,
		})
	}

	// History blocks follow.
	for _, rec := range m.blockHistory {
		allBlocks = append(allBlocks, blockRowForTable{
			height:    rec.Height,
			pvPct:     rec.PrevotePercent,
			pcPct:     rec.PrecommitPercent,
			elapsed:   rec.Elapsed,
			round:     rec.Round,
			step:      rec.Step,
			isCurrent: false,
		})
	}

	if len(allBlocks) == 0 {
		m.blockTable.SetRows(nil)
		m.blockTable.UpdateViewport()
		return
	}

	rows := make([]table.Row, len(allBlocks))
	for i, blk := range allBlocks {
		marker := "  "
		if blk.isCurrent {
			marker = "* "
		}
		var elapsedStr string
		if blk.elapsed > 0 || blk.isCurrent {
			elapsedStr = formatDuration(blk.elapsed)
		} else {
			elapsedStr = "-"
		}
		rows[i] = table.Row{
			marker + formatNumber(blk.height),
			fmt.Sprintf("%.1f%%", blk.pvPct*100),
			fmt.Sprintf("%.1f%%", blk.pcPct*100),
			elapsedStr,
			fmt.Sprintf("%d/%d", blk.round, blk.step),
		}
	}
	m.blockTable.SetRows(rows)
	m.blockTable.UpdateViewport()
}

// rebuildNodeTableRows rebuilds the node table rows for provider detail.
func (m *Model) rebuildNodeTableRows() {
	if len(m.detail.Nodes) == 0 {
		m.nodeTable.SetRows(nil)
		m.nodeTable.UpdateViewport()
		return
	}
	rows := make([]table.Row, len(m.detail.Nodes))
	for i, node := range m.detail.Nodes {
		nodeName := node.Name
		if nodeName == "" {
			nodeName = fmt.Sprintf("node-%d", i+1)
		}
		if len(nodeName) > colWidthNodeName {
			nodeName = nodeName[:colWidthNodeName-3] + "..."
		}
		cpuStr := formatResourceRatio(node.CPUAvailable/1000, node.CPUAllocatable/1000)
		memStr := formatMemoryRatio(node.MemAvailable, node.MemAllocatable)
		gpuStr := formatNodeGPU(node)

		rows[i] = table.Row{
			nodeName,
			cpuStr,
			memStr,
			gpuStr,
		}
	}
	m.nodeTable.SetRows(rows)
	m.nodeTable.UpdateViewport()
}

// updateGovParamView updates the governance parameter viewport content
// based on the currently selected module in govModuleList.
func (m *Model) updateGovParamView() {
	idx := m.govModuleIdx
	if idx < 0 || idx >= len(governance.ModuleOrder) || m.governanceParams == nil {
		m.govParamView.SetContent("")
		return
	}
	module := governance.ModuleOrder[idx]
	modParams := m.governanceParams.Modules[module]
	if modParams == nil {
		m.govParamView.SetContent("(no data)")
		return
	}
	if modParams.Error != nil {
		m.govParamView.SetContent(fmt.Sprintf("Error: %v", modParams.Error))
		return
	}
	rendered := pretty.RenderModuleParamsFromJSON(module, modParams.RawJSON)
	m.govParamView.SetContent(rendered)
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
	case tea.KeyPressMsg:
		return m.handleKeyMsg(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeComponents()
		return m, nil

	// Render throttle: apply pending consensus state at fixed intervals.
	case renderTickMsg:
		cmd := m.renderTick()
		if m.pendingState != nil {
			m.handleStateMsgInPlace(m.pendingState)
			m.pendingState = nil
		}
		// Fetch proposer info when the height changes (WebSocket events
		// don't include proposer data).
		if m.state != nil && m.state.Height > m.lastProposerHeight {
			return m, tea.Batch(cmd, m.fetchProposer)
		}
		return m, cmd

	// WebSocket lifecycle
	case wsConnectedMsg:
		m.snapshotCh = msg.ch
		m.wsConnected = true
		m.consensusRetryAttempt = 0
		if m.state != nil {
			m.state.Error = nil
		}
		return m, m.waitForSnapshot()
	case consensusSnapshotMsg:
		if !msg.ok {
			// Channel closed — WebSocket connection lost.
			m.wsConnected = false
			m.snapshotCh = nil
			return m, m.scheduleConsensusReconnect()
		}
		// Buffer the state — it will be applied on the next renderTick.
		// If the new snapshot is for a higher height than the pending one,
		// apply the pending one first so its accumulated votes aren't lost.
		if msg.snapshot.State != nil {
			if m.pendingState != nil && msg.snapshot.State.Height > m.pendingState.Height {
				m.handleStateMsgInPlace(m.pendingState)
			}
			m.pendingState = msg.snapshot.State
		}
		return m, m.waitForSnapshot()
	case consensusReconnectMsg:
		if m.runtimeContext != nil && m.runtimeContext.Err() != nil {
			return m, nil
		}
		return m, m.connectWebSocket

	case providerCheckTickMsg:
		if m.runtimeContext != nil && m.runtimeContext.Err() != nil {
			return m, nil
		}
		cmds := m.dispatchProviderChecks()
		cmds = append(cmds, m.providerCheckTick())
		return m, tea.Batch(cmds...)
	case chainSyncTickMsg:
		if m.runtimeContext != nil && m.runtimeContext.Err() != nil {
			return m, nil
		}
		return m, tea.Batch(m.syncChain, m.chainSyncTick())
	case cacheSaveTickMsg:
		if m.runtimeContext != nil && m.runtimeContext.Err() != nil {
			return m, nil
		}
		m.saveCache()
		return m, m.cacheSaveTick()
	case governanceProposalsTickMsg:
		if m.runtimeContext != nil && m.runtimeContext.Err() != nil {
			return m, nil
		}
		return m, tea.Batch(m.fetchGovernanceProposals, m.governanceProposalSyncTick())
	case governanceParamsTickMsg:
		if m.runtimeContext != nil && m.runtimeContext.Err() != nil {
			return m, nil
		}
		return m, tea.Batch(m.fetchGovernanceParams, m.governanceSyncTick())
	case stateMsg:
		return m.handleStateMsg(msg)
	case monikersMsg:
		if msg.err == nil {
			m.monikers = msg.monikers
			m.rebuildValidatorTableRows()
		}
		return m, nil
	case initialSigningMsg:
		if msg.err == nil && len(msg.signers) > 0 {
			m.seedSigningHistory(msg.signers, msg.validators)
		}
		return m, nil
	case proposerMsg:
		if msg.err == nil {
			m.knownProposerIndex = msg.proposerIndex
			m.knownProposerAddr = msg.proposerAddr
			m.lastProposerHeight = msg.height
			m.applyProposerToState()
		}
		return m, nil
	case chainSyncMsg:
		return m.handleChainSyncMsg(msg)
	case providerCheckedMsg:
		return m.handleProviderCheckedMsg(msg)
	case providerDetailMsg:
		return m.handleProviderDetailMsg(msg)
	case governanceProposalsMsg:
		return m.handleGovernanceProposalsMsg(msg)
	case governanceParamsMsg:
		return m.handleGovernanceParamsMsg(msg)
	case oracleStateMsg:
		return m.handleOracleStateMsg(msg)
	case bmeStateMsg:
		return m.handleBMEStateMsg(msg)
	case busEventMsg:
		return m.handleBusEvent(msg)
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
		return m.runRuntimeTask(func() tea.Msg {
			select {
			case snap, ok := <-ch:
				return consensusSnapshotMsg{snapshot: snap, ok: ok}
			case <-m.runtimeContext.Done():
				return consensusSnapshotMsg{ok: false}
			}
		})
	}
}

func (m *Model) scheduleConsensusReconnect() tea.Cmd {
	if m.runtimeContext != nil && m.runtimeContext.Err() != nil {
		return nil
	}

	delay := consensusReconnectBaseDelay
	for range m.consensusRetryAttempt {
		if delay >= consensusReconnectMaxDelay/2 {
			delay = consensusReconnectMaxDelay
			break
		}
		delay *= 2
	}
	if m.consensusRetryAttempt < 64 {
		m.consensusRetryAttempt++
	}

	ctx := m.runtimeContext
	return func() tea.Msg {
		return m.runRuntimeTask(func() tea.Msg {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			if ctx == nil {
				<-timer.C
				return consensusReconnectMsg{}
			}
			select {
			case <-timer.C:
				return consensusReconnectMsg{}
			case <-ctx.Done():
				return nil
			}
		})
	}
}

// waitForBusEvent returns a tea.Cmd that blocks until the next event
// arrives on the shared pubsub bus subscriber.
func (m Model) waitForBusEvent() tea.Cmd {
	sub := m.subscriber
	if sub == nil {
		return nil
	}
	return func() tea.Msg {
		return m.runRuntimeTask(func() tea.Msg {
			select {
			case ev, ok := <-sub.Events():
				return busEventMsg{event: ev, ok: ok}
			case <-sub.Done():
				return busEventMsg{ok: false}
			case <-m.runtimeContext.Done():
				return busEventMsg{ok: false}
			}
		})
	}
}

// handleBusEvent routes a typed ABCI event from the bus into the
// appropriate dashboard state.  After processing, it re-arms the
// subscriber wait so the next event is picked up.
func (m *Model) handleBusEvent(msg busEventMsg) (tea.Model, tea.Cmd) {
	if !msg.ok {
		// Bus or subscriber closed — do not re-subscribe.
		m.subscriber = nil
		return m, nil
	}

	now := time.Now()

	switch ev := msg.event.(type) {
	case *oracletypes.EventPriceData:
		entry := OraclePriceEntry{
			Denom:     ev.Id.Denom,
			Price:     ev.Price.String(),
			Source:    ev.Source,
			Timestamp: ev.Timestamp,
		}
		m.oracle.Prices = append([]OraclePriceEntry{entry}, m.oracle.Prices...)
		if len(m.oracle.Prices) > maxOracleEvents {
			m.oracle.Prices = m.oracle.Prices[:maxOracleEvents]
		}
		m.prependOracleEvent("price", ev.Id.Denom, ev.Price.String(), now)

	case *oracletypes.EventAggregatedPrice:
		m.oracle.Aggregated[ev.Price.Denom] = ev
		m.prependOracleEvent("aggregated", ev.Price.Denom, ev.Price.TWAP.String(), now)

	case *oracletypes.EventPriceStaleWarning:
		m.prependOracleEvent("stale_warning", ev.Id.Denom, "", now)

	case *oracletypes.EventPriceStaled:
		m.prependOracleEvent("staled", ev.Id.Denom, "", now)

	case *oracletypes.EventPriceRecovered:
		m.prependOracleEvent("recovered", ev.Id.Denom, "", now)
	}

	// Re-arm the subscriber for the next event.
	return m, m.waitForBusEvent()
}

// prependOracleEvent adds an entry to the oracle event log.
func (m *Model) prependOracleEvent(typ, denom, detail string, ts time.Time) {
	m.oracle.Events = append([]OracleEvent{{
		Type:      typ,
		Denom:     denom,
		Detail:    detail,
		Timestamp: ts,
	}}, m.oracle.Events...)
	if len(m.oracle.Events) > maxOracleEvents {
		m.oracle.Events = m.oracle.Events[:maxOracleEvents]
	}
}

func (m *Model) handleKeyMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Handle detail view keys first
	if m.detail.Showing {
		return m.handleDetailViewKeys(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		m.saveCache()
		return m, tea.Quit
	case "q":
		if m.embedded {
			return m, func() tea.Msg { return BackMsg{} }
		}
		m.quitting = true
		m.saveCache()
		return m, tea.Quit
	case "r":
		if m.hubTab == HubProvider {
			return m, m.syncChain
		}
		if m.activeTab == TabGovernance {
			return m, m.fetchGovernanceProposals
		}
		if m.activeTab == TabParameters {
			return m, m.fetchGovernanceParams
		}
		// Consensus is event-driven, no manual refresh needed.
	case "1":
		if m.hubTab == HubNetwork {
			m.activeTab = TabOverview
		}
	case "2":
		if m.hubTab == HubNetwork {
			m.activeTab = TabValidators
			m.validatorTable.SetCursor(0)
			m.expandedValidator = -1
		}
	case "3":
		if m.hubTab == HubNetwork {
			m.activeTab = TabGovernance
		}
	case "4":
		if m.hubTab == HubNetwork {
			m.activeTab = TabParameters
		}
	case "tab":
		m.hubTab = (m.hubTab + 1) % HubTab(hubTabCount)
		m.resetScrollForTab()
	case "shift+tab":
		m.hubTab = (m.hubTab - 1 + HubTab(hubTabCount)) % HubTab(hubTabCount)
		m.resetScrollForTab()
	case "up", "k":
		var cmd tea.Cmd
		switch {
		case m.hubTab == HubProvider:
			m.providerTable, cmd = m.providerTable.Update(msg)
		case m.hubTab == HubNetwork && m.activeTab == TabGovernance:
			m.govProposalView.SetYOffset(m.govProposalView.YOffset() - 1)
		case m.hubTab == HubNetwork && m.activeTab == TabParameters:
			if m.govModuleIdx > 0 {
				m.govModuleIdx--
				if m.govModuleIdx < m.govModuleScroll {
					m.govModuleScroll = m.govModuleIdx
				}
				m.updateGovParamView()
			}
		case m.hubTab == HubNetwork && m.activeTab == TabOverview:
			if m.expandedBlock >= 0 {
				if m.expandedScroll > 0 {
					m.expandedScroll--
				}
			} else {
				m.blockTable, cmd = m.blockTable.Update(msg)
			}
		case m.hubTab == HubNetwork && m.activeTab == TabValidators:
			if m.expandedValidator < 0 {
				m.validatorTable, cmd = m.validatorTable.Update(msg)
			}
		}
		return m, cmd
	case "down", "j":
		var cmd tea.Cmd
		switch {
		case m.hubTab == HubProvider:
			m.providerTable, cmd = m.providerTable.Update(msg)
		case m.hubTab == HubNetwork && m.activeTab == TabGovernance:
			m.govProposalView.SetYOffset(m.govProposalView.YOffset() + 1)
		case m.hubTab == HubNetwork && m.activeTab == TabParameters:
			if m.govModuleIdx < len(governance.ModuleOrder)-1 {
				m.govModuleIdx++
				if m.govModuleIdx >= m.govModuleScroll+m.govModuleHeight {
					m.govModuleScroll = m.govModuleIdx - m.govModuleHeight + 1
				}
				m.updateGovParamView()
			}
		case m.hubTab == HubNetwork && m.activeTab == TabOverview:
			if m.expandedBlock >= 0 {
				m.expandedScroll++
			} else {
				m.blockTable, cmd = m.blockTable.Update(msg)
			}
		case m.hubTab == HubNetwork && m.activeTab == TabValidators:
			if m.expandedValidator < 0 {
				m.validatorTable, cmd = m.validatorTable.Update(msg)
			}
		}
		return m, cmd
	case "home", "g":
		var cmd tea.Cmd
		switch {
		case m.hubTab == HubProvider:
			m.providerTable, cmd = m.providerTable.Update(msg)
		case m.hubTab == HubNetwork && m.activeTab == TabGovernance:
			m.govProposalView.GotoTop()
		case m.hubTab == HubNetwork && m.activeTab == TabParameters:
			m.govModuleIdx = 0
			m.govModuleScroll = 0
			m.updateGovParamView()
		case m.hubTab == HubNetwork && m.activeTab == TabOverview:
			m.blockTable.SetCursor(0)
		case m.hubTab == HubNetwork && m.activeTab == TabValidators:
			m.validatorTable, cmd = m.validatorTable.Update(msg)
		}
		return m, cmd
	case "end", "G":
		var cmd tea.Cmd
		switch {
		case m.hubTab == HubProvider:
			m.providerTable, cmd = m.providerTable.Update(msg)
		case m.hubTab == HubNetwork && m.activeTab == TabGovernance:
			m.govProposalView.GotoBottom()
		case m.hubTab == HubNetwork && m.activeTab == TabParameters:
			m.govModuleIdx = len(governance.ModuleOrder) - 1
			m.govModuleScroll = max(0, m.govModuleIdx-m.govModuleHeight+1)
			m.updateGovParamView()
		case m.hubTab == HubNetwork && m.activeTab == TabOverview:
			m.blockTable, cmd = m.blockTable.Update(msg)
		case m.hubTab == HubNetwork && m.activeTab == TabValidators:
			m.validatorTable, cmd = m.validatorTable.Update(msg)
		}
		return m, cmd
	case "left", "h":
		if m.hubTab == HubProvider {
			m.selectPreviousVersion()
		} else if m.hubTab == HubNetwork && m.activeTab == TabOverview && m.expandedBlock >= 0 {
			m.expandedBlock = -1
			m.expandedScroll = 0
			m.expandedValidators = nil
			return m, nil
		} else if m.hubTab == HubNetwork && m.activeTab == TabValidators && m.expandedValidator >= 0 {
			m.expandedValidator = -1
			return m, nil
		} else if m.hubTab == HubNetwork && m.activeTab == TabGovernance {
			m.govProposalView.ScrollLeft(4)
		} else if m.hubTab == HubNetwork && m.activeTab == TabParameters {
			if m.govParamView.YOffset() > 0 {
				m.govParamView.SetYOffset(m.govParamView.YOffset() - 1)
			}
		}
	case "right", "l":
		if m.hubTab == HubProvider {
			m.selectNextVersion()
		} else if m.hubTab == HubNetwork && m.activeTab == TabGovernance {
			m.govProposalView.ScrollRight(4)
		} else if m.hubTab == HubNetwork && m.activeTab == TabParameters {
			m.govParamView.SetYOffset(m.govParamView.YOffset() + 1)
		}
	case "enter":
		if m.hubTab == HubProvider {
			return m.enterProviderDetail()
		} else if m.hubTab == HubNetwork && m.activeTab == TabOverview {
			m.toggleBlockExpansion()
		} else if m.hubTab == HubNetwork && m.activeTab == TabValidators {
			m.toggleValidatorExpansion()
		}
	case "esc", "backspace":
		if m.hubTab == HubNetwork && m.activeTab == TabOverview && m.expandedBlock >= 0 {
			m.expandedBlock = -1
			m.expandedScroll = 0
			m.expandedValidators = nil
			return m, nil
		} else if m.hubTab == HubNetwork && m.activeTab == TabValidators && m.expandedValidator >= 0 {
			m.expandedValidator = -1
			return m, nil
		} else if m.embedded {
			return m, func() tea.Msg { return BackMsg{} }
		}
	}
	return m, nil
}

func (m *Model) handleDetailViewKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		m.saveCache()
		return m, tea.Quit
	case "q":
		if m.embedded {
			return m, func() tea.Msg { return BackMsg{} }
		}
		m.quitting = true
		m.saveCache()
		return m, tea.Quit
	case "esc", "backspace":
		m.detail.Showing = false
		m.detail.Nodes = nil
		m.detail.Provider = nil
		m.detail.Error = nil
		m.detail.Loading = false
		m.nodeTable.SetCursor(0)
	case "up", "k", "down", "j", "home", "g", "end", "G":
		m.nodeTable, _ = m.nodeTable.Update(msg)
	case "tab", "shift+tab":
		// Exit detail view and switch tabs
		m.detail.Showing = false
		m.detail.Nodes = nil
		m.detail.Provider = nil
		m.detail.Error = nil
		m.detail.Loading = false
		m.nodeTable.SetCursor(0)
		// Re-handle the key for tab switching
		return m.handleKeyMsg(msg)
	}
	return m, nil
}

func (m *Model) resetScrollForTab() {
	m.validatorTable.SetCursor(0)
	m.blockTable.SetCursor(0)
}

func (m *Model) getFilteredProviders() []rpc.Provider {
	providers := filterNonLocalProviders(m.providers.Items)
	if m.providers.Version == "" {
		return providers
	}

	filtered := make([]rpc.Provider, 0, len(providers))
	for _, provider := range providers {
		if provider.AkashVersion == m.providers.Version {
			filtered = append(filtered, provider)
		}
	}
	return filtered
}

func (m *Model) toggleBlockExpansion() {
	cursor := m.blockTable.Cursor()
	if m.expandedBlock == cursor {
		// Collapse.
		m.expandedBlock = -1
		m.expandedScroll = 0
		m.expandedValidators = nil
	} else {
		// Expand.
		m.expandedBlock = cursor
		m.expandedScroll = 0
		if cursor == 0 && m.state != nil {
			// Current block — snapshot live validators.
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
		} else if cursor > 0 {
			histIdx := cursor - 1
			if histIdx < len(m.blockHistory) {
				m.expandedValidators = m.blockHistory[histIdx].Validators
			}
		}
	}
	m.resizeComponents()
}

func (m *Model) toggleValidatorExpansion() {
	cursor := m.validatorTable.Cursor()
	if m.expandedValidator == cursor {
		m.expandedValidator = -1
	} else {
		m.expandedValidator = cursor
	}
	m.resizeComponents()
}

func (m *Model) selectPreviousVersion() {
	if len(m.providers.Versions) == 0 {
		return
	}
	m.reconcileProviderVersionSelection()
	m.providers.VersionIdx--
	if m.providers.VersionIdx < 0 {
		m.providers.VersionIdx = len(m.providers.Versions) - 1
	}
	m.providers.Version = m.providers.Versions[m.providers.VersionIdx]
	m.providerTable.SetCursor(0)
	m.sortProviders()
	m.rebuildProviderTableRows()
}

func (m *Model) selectNextVersion() {
	if len(m.providers.Versions) == 0 {
		return
	}
	m.reconcileProviderVersionSelection()
	m.providers.VersionIdx = (m.providers.VersionIdx + 1) % len(m.providers.Versions)
	m.providers.Version = m.providers.Versions[m.providers.VersionIdx]
	m.providerTable.SetCursor(0)
	m.sortProviders()
	m.rebuildProviderTableRows()
}

func (m *Model) enterProviderDetail() (tea.Model, tea.Cmd) {
	filtered := m.getFilteredProviders()
	idx := m.providerTable.Cursor()
	if len(filtered) == 0 || idx >= len(filtered) {
		return m, nil
	}

	provider := filtered[idx]
	m.detail.Provider = &provider
	m.detail.Loading = true
	m.detail.Error = nil
	m.detail.Nodes = nil
	m.detail.Showing = true
	m.detailRequestID++

	return m, m.fetchProviderDetail(provider.HostURI, m.detailRequestID)
}

func (m *Model) fetchProviderDetail(hostURI string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		return m.runRuntimeTask(func() tea.Msg {
			nodes, err := m.queryProviderStatusGRPC(m.runtimeContext, hostURI)
			if err != nil {
				return providerDetailMsg{hostURI: hostURI, requestID: requestID, err: err}
			}
			if err := m.runtimeContext.Err(); err != nil {
				return providerDetailMsg{hostURI: hostURI, requestID: requestID, err: err}
			}
			return providerDetailMsg{hostURI: hostURI, requestID: requestID, nodes: nodes}
		})
	}
}

func (m *Model) handleProviderDetailMsg(msg providerDetailMsg) (tea.Model, tea.Cmd) {
	if !m.detail.Showing || m.detail.Provider == nil ||
		m.detail.Provider.HostURI != msg.hostURI || m.detailRequestID != msg.requestID {
		return m, nil
	}
	m.detail.Loading = false
	if msg.err != nil {
		m.detail.Error = msg.err
	} else {
		m.detail.Nodes = msg.nodes
		m.rebuildNodeTableRows()
	}
	return m, nil
}

func (m *Model) reconcileProviderVersionSelection() {
	if len(m.providers.Versions) == 0 {
		m.providers.Version = ""
		m.providers.VersionIdx = 0
		return
	}

	for index, version := range m.providers.Versions {
		if version == m.providers.Version {
			m.providers.VersionIdx = index
			return
		}
	}

	m.providers.Version = m.providers.Versions[0]
	m.providers.VersionIdx = 0
}

func (m *Model) handleGovernanceParamsMsg(msg governanceParamsMsg) (tea.Model, tea.Cmd) {
	if msg.err == nil {
		m.governanceParams = msg.params
		m.updateGovParamView()
	}
	return m, nil
}

func (m *Model) handleGovernanceProposalsMsg(msg governanceProposalsMsg) (tea.Model, tea.Cmd) {
	m.governanceProposalsErr = msg.err
	if msg.err == nil {
		m.governanceProposals = msg.proposals
		m.govProposalView.SetContent(pretty.RenderProposalList(msg.proposals))
	}
	return m, nil
}

func (m *Model) handleOracleStateMsg(msg oracleStateMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || msg.state == nil {
		return m, m.oracleSyncTick()
	}

	// Store the detected oracle version for the UI.
	if msg.state.Version != "" {
		m.oracle.Version = msg.state.Version
	}

	// Seed aggregated prices from ABCI response into the same map the bus
	// events write to.  Bus events will overwrite these in real-time.
	for denom, agResp := range msg.state.Aggregated {
		m.oracle.Aggregated[denom] = &oracletypes.EventAggregatedPrice{
			Price: agResp.AggregatedPrice,
		}
	}

	// Seed recent price entries if the event log is empty (first load).
	if len(m.oracle.Prices) == 0 && msg.state.Prices != nil {
		for _, p := range msg.state.Prices.Prices {
			m.oracle.Prices = append(m.oracle.Prices, OraclePriceEntry{
				Denom:     p.ID.Denom,
				Price:     p.State.Price.String(),
				Source:    fmt.Sprintf("%d", p.ID.Source),
				Timestamp: p.ID.Timestamp,
			})
		}
		if len(m.oracle.Prices) > maxOracleEvents {
			m.oracle.Prices = m.oracle.Prices[:maxOracleEvents]
		}
	}

	return m, m.oracleSyncTick()
}

func (m *Model) handleBMEStateMsg(msg bmeStateMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || msg.state == nil {
		return m, m.bmeSyncTick()
	}

	if msg.state.Status != nil {
		m.oracle.BMEStatus = msg.state.Status
	}

	if msg.state.Ledger != nil {
		m.oracle.BMELedger = msg.state.Ledger.Records
	}

	return m, m.bmeSyncTick()
}

// seedSigningHistory uses the latest commit signatures to populate the first
// entry in each validator's signing history. Matches by address because
// commit signatures are ordered by Tendermint address, not voting power.
func (m *Model) seedSigningHistory(signers map[string]bool, validators []consensus.Validator) {
	// Only seed if we have no history yet.
	if len(m.valSignHistory) > 0 {
		return
	}
	if len(validators) == 0 {
		return
	}

	for i, v := range validators {
		signed := signers[strings.ToUpper(v.Address)]
		m.valSignHistory[i] = []bool{signed}
	}
}

// applyProposerToState sets the proposer on the current state from the
// last known proposer fetched via HTTP. Called after every state update
// because WebSocket events don't include proposer info.
func (m *Model) applyProposerToState() {
	if m.state == nil || m.knownProposerIndex < 0 {
		return
	}
	m.state.ProposerIndex = m.knownProposerIndex
	m.state.ProposerAddress = m.knownProposerAddr
	for i := range m.state.Validators {
		m.state.Validators[i].IsProposer = i == m.knownProposerIndex
	}
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
		if errors.Is(msg.err, context.Canceled) {
			return m, nil
		}
		return m, m.scheduleConsensusReconnect()
	}

	newState := msg.state

	// Detect height change — snapshot the completed block.
	if m.peakHeight > 0 && newState.Height > m.peakHeight {
		// Skip the first height change: the WebSocket started mid-block,
		// so precommit data is incomplete and would show as all-missed.
		if !m.firstHeightSeen {
			m.firstHeightSeen = true
			m.peakPrevotePercent = 0
			m.peakPrecommitPercent = 0
			m.peakRound = 0
			m.peakStep = 0
			m.blockStartTime = time.Time{}
			m.peakHeight = newState.Height
			m.state = newState
			m.applyProposerToState()
			return m, nil
		}

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

		// Record which validator was proposer for this block.
		m.proposerHistory = append([]int{m.knownProposerIndex}, m.proposerHistory...)
		if len(m.proposerHistory) > m.maxSignHistory {
			m.proposerHistory = m.proposerHistory[:m.maxSignHistory]
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
	m.applyProposerToState()
	m.rebuildValidatorTableRows()
	m.rebuildBlockTableRows()
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
	if msg.err != nil || (m.runtimeContext != nil && m.runtimeContext.Err() != nil) {
		if len(m.loader.Queue) == 0 && len(m.loader.InFlight) == 0 {
			m.loader.Loading = false
		}
		return m, nil
	}
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

// resizeComponents updates all bubbles component dimensions based on
// the current terminal size. Must be called from Update (pointer receiver)
// whenever the terminal size changes or the layout state changes (e.g.
// expanded panels toggling).
func (m *Model) resizeComponents() {
	validatorRows := max(m.height-14, 5)
	if m.expandedValidator >= 0 {
		validatorRows = max(validatorRows/2, 3)
	}
	m.validatorTable.SetHeight(validatorRows)
	m.validatorTable.SetWidth(m.width)
	m.validatorTable.UpdateViewport()

	blockRows := max(m.height-overviewOverhead, 3)
	if m.expandedBlock >= 0 {
		blockRows = max(blockRows/3, 2)
	}
	m.blockTable.SetHeight(blockRows)
	m.blockTable.SetWidth(m.width)
	m.blockTable.UpdateViewport()

	providerRows := max(m.height-providerListOverhead, minVisibleProviders)
	m.providerTable.SetHeight(providerRows)
	m.providerTable.SetWidth(m.width)
	m.providerTable.UpdateViewport()

	nodeRows := max(m.height-nodeListOverhead, minVisibleNodes)
	m.nodeTable.SetHeight(nodeRows)
	m.nodeTable.SetWidth(m.width)
	m.nodeTable.UpdateViewport()

	govHeight := m.height - governanceOverhead
	if govHeight < 5 {
		govHeight = 5
	}
	m.govModuleHeight = govHeight
	// Clamp scroll if terminal grew.
	if m.govModuleScroll > max(0, len(governance.ModuleOrder)-govHeight) {
		m.govModuleScroll = max(0, len(governance.ModuleOrder)-govHeight)
	}
	m.govParamView.SetHeight(govHeight)
	m.govParamView.SetWidth(m.width - 22)
	m.govProposalView.SetHeight(govHeight)
	m.govProposalView.SetWidth(m.width)
}

// View renders the UI
func (m Model) View() tea.View {
	if m.quitting {
		view := tea.NewView("Goodbye!\n")
		view.AltScreen = !m.embedded
		return view
	}

	// Component heights are set in resizeComponents() called from Update().
	// Table rows are rebuilt in Update() handlers; do not rebuild here
	// because View() is a value-receiver and mutations would be lost.

	ctx := ViewContext{
		State:                  m.state,
		Endpoint:               m.client.Endpoint(),
		Width:                  m.width,
		Height:                 m.height,
		HubTab:                 m.hubTab,
		ActiveTab:              m.activeTab,
		Embedded:               m.embedded,
		Monikers:               m.monikers,
		GovernanceProposals:    m.governanceProposals,
		GovernanceProposalsErr: m.governanceProposalsErr,
		GovernanceParams:       m.governanceParams,
		BlockHistory:           m.blockHistory,
		ExpandedBlock:          m.expandedBlock,
		ExpandedScroll:         m.expandedScroll,
		ExpandedValidators:     m.expandedValidators,
		ExpandedValidator:      m.expandedValidator,
		ValSignHistory:         m.valSignHistory,
		ProposerHistory:        m.proposerHistory,
		CurrentProposer:        m.knownProposerIndex,
		WSConnected:            m.wsConnected,
		Oracle:                 m.oracle,
		ProviderTable:          m.providerTable,
		NodeTable:              m.nodeTable,
		ValidatorTable:         m.validatorTable,
		BlockTable:             m.blockTable,
		GovModuleIdx:           m.govModuleIdx,
		GovModuleScroll:        m.govModuleScroll,
		GovModuleHeight:        m.govModuleHeight,
		GovProposalView:        m.govProposalView,
		GovParamView:           m.govParamView,
		Providers: ProviderViewState{
			Providers: m.providers.Items,
			Versions:  m.providers.Versions,
			Selected:  m.providers.Version,
			Loading:   m.loader.Loading,
			Loaded:    m.loader.Checked,
			Total:     m.loader.Total,
			Detail: ProviderDetailState{
				Showing:  m.detail.Showing,
				Provider: m.detail.Provider,
				Nodes:    m.detail.Nodes,
				Loading:  m.detail.Loading,
				Error:    m.detail.Error,
			},
		},
	}

	view := tea.NewView(RenderView(ctx))
	view.AltScreen = !m.embedded
	return view
}
