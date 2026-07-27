package ui

import (
	"fmt"
	"os"
	"testing"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/glyphs"
	"pkg.akt.dev/akt/internal/monitor/consensus"
	"pkg.akt.dev/akt/internal/monitor/governance"
	"pkg.akt.dev/akt/internal/monitor/rpc"

	bmetypes "pkg.akt.dev/go/node/bme/v1"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"
)

func TestMain(m *testing.M) {
	glyphs.Init(glyphs.ModeASCII)
	os.Exit(m.Run())
}

// testWidth and testHeight are the fixed terminal dimensions for all golden tests.
const (
	testWidth  = 120
	testHeight = 40
)

// testTime is a fixed timestamp used across all test fixtures.
var testTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// newTestConsensusState builds a deterministic consensus.State with n validators.
// Validators have voting power decreasing from 1000*n to 1000.
// The first 2/3 of validators have prevoted and precommited.
func newTestConsensusState(n int) *consensus.State {
	validators := make([]consensus.ValidatorStatus, n)
	var totalPower int64
	votedCount := (n * 2) / 3 // 2/3 voted
	var votedPower int64

	for i := 0; i < n; i++ {
		power := int64((n - i) * 1000)
		totalPower += power
		voted := i < votedCount
		if voted {
			votedPower += power
		}
		validators[i] = consensus.ValidatorStatus{
			Index:       i,
			Address:     fmt.Sprintf("ABCDEF%04d", i),
			PubKey:      fmt.Sprintf("pubkey%04d", i),
			VotingPower: power,
			Prevoted:    voted,
			Precommited: voted,
			IsProposer:  i == 0,
		}
	}

	pvPct := float64(votedPower) / float64(totalPower)

	// Build a bit array string: "x" for voted, "_" for not
	bitArray := ""
	for i := 0; i < n; i++ {
		if i < votedCount {
			bitArray += "x"
		} else {
			bitArray += "_"
		}
	}

	return &consensus.State{
		Height:            18_234_567,
		Round:             0,
		Step:              1,
		StartTime:         testTime,
		Elapsed:           3500 * time.Millisecond,
		ProposerAddress:   "ABCDEF0000",
		ProposerIndex:     0,
		TotalValidators:   n,
		TotalVotingPower:  totalPower,
		PrevoteCount:      votedCount,
		PrevotePower:      votedPower,
		PrevotePercent:    pvPct,
		PrevoteBitArray:   bitArray,
		PrecommitCount:    votedCount,
		PrecommitPower:    votedPower,
		PrecommitPercent:  pvPct,
		PrecommitBitArray: bitArray,
		Validators:        validators,
	}
}

// newTestMonikers builds a moniker map keyed by pubkey for n validators.
func newTestMonikers(n int) map[string]string {
	m := make(map[string]string, n)
	names := []string{"Cosmostation", "Forbole", "Figment", "Chorus One", "Polychain",
		"Binance", "Kraken", "Coinbase", "P2P", "Staked", "Everstake", "Allnodes",
		"SG-1", "Imperator", "Lavender.Five", "Polkachu", "AutoStake", "WhisperNode",
		"ChainLayer", "StakeCito"}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("Validator-%d", i)
		if i < len(names) {
			name = names[i]
		}
		m[fmt.Sprintf("pubkey%04d", i)] = name
	}
	return m
}

// newTestProviders builds n deterministic provider entries.
func newTestProviders(n int) []rpc.Provider {
	providers := make([]rpc.Provider, n)
	versions := []string{"0.6.4", "0.6.3", "0.6.2"}
	countries := []string{"US", "DE", "SG", "JP", "GB"}
	for i := 0; i < n; i++ {
		providers[i] = rpc.Provider{
			Owner:        fmt.Sprintf("akash1provider%04d", i),
			HostURI:      fmt.Sprintf("https://provider%d.example.com:8443", i),
			Name:         fmt.Sprintf("Provider %d", i),
			AkashVersion: versions[i%len(versions)],
			IsOnline:     true,
			Country:      countries[i%len(countries)],
			CPUAvailable: uint64((i + 1) * 8000),
			CPUTotal:     uint64((i + 1) * 16000),
			MemAvailable: uint64((i + 1)) * 16 * 1024 * 1024 * 1024,
			MemTotal:     uint64((i + 1)) * 32 * 1024 * 1024 * 1024,
			GPUAvailable: uint64(i % 3),
			GPUTotal:     uint64(i%3 + 1),
			GPUModels:    []string{"H100"},
		}
	}
	return providers
}

// newTestNodes builds n deterministic provider nodes with GPU info.
func newTestNodes(n int) []rpc.ProviderNodeWithGPU {
	nodes := make([]rpc.ProviderNodeWithGPU, n)
	for i := 0; i < n; i++ {
		nodes[i] = rpc.ProviderNodeWithGPU{
			Name:           fmt.Sprintf("node-%d", i),
			CPUAllocatable: uint64((i + 1) * 16000),
			CPUAvailable:   uint64((i + 1) * 8000),
			MemAllocatable: uint64((i + 1)) * 64 * 1024 * 1024 * 1024,
			MemAvailable:   uint64((i + 1)) * 32 * 1024 * 1024 * 1024,
			GPUAllocatable: 4,
			GPUAvailable:   2,
			GPUs: []rpc.GPUInfo{
				{Vendor: "nvidia", Name: "H100", MemorySize: "80Gi"},
			},
		}
	}
	return nodes
}

// newTestBlockHistory builds n deterministic block history records.
func newTestBlockHistory(n int) []BlockRecord {
	records := make([]BlockRecord, n)
	for i := 0; i < n; i++ {
		records[i] = BlockRecord{
			Height:           int64(18_234_566 - i),
			PrevotePercent:   0.95,
			PrecommitPercent: 0.92,
			Round:            0,
			Step:             1,
			Elapsed:          time.Duration(5+i) * time.Second,
			Timestamp:        testTime.Add(-time.Duration(i) * 6 * time.Second),
		}
	}
	return records
}

// newTestSignHistory builds per-validator signing history for n validators with h blocks.
func newTestSignHistory(n, h int) map[int][]bool {
	hist := make(map[int][]bool, n)
	for i := 0; i < n; i++ {
		blocks := make([]bool, h)
		for j := 0; j < h; j++ {
			// Most validators sign most blocks; later validators miss more
			blocks[j] = (j+i)%5 != 0
		}
		hist[i] = blocks
	}
	return hist
}

// newTestProposerHistory builds a proposer history of h blocks cycling through n validators.
func newTestProposerHistory(n, h int) []int {
	hist := make([]int, h)
	for i := 0; i < h; i++ {
		hist[i] = i % n
	}
	return hist
}

// newTestBlockTableModel builds a bubbles table.Model for block history tests.
func newTestBlockTableModel(state *consensus.State, history []BlockRecord) table.Model {
	cols := []table.Column{
		{Title: "Height", Width: colHeight},
		{Title: "PV", Width: colPV},
		{Title: "PC", Width: colPC},
		{Title: "Elapsed", Width: colElapsed},
		{Title: "R/S", Width: colRS},
	}
	t := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(20))

	ts := table.DefaultStyles()
	ts.Header = mutedStyle
	ts.Cell = lipgloss.NewStyle()
	ts.Selected = highlightStyle
	t.SetStyles(ts)

	var rows []table.Row
	if state != nil && state.Height > 0 {
		elapsed := state.Elapsed
		if elapsed < 0 {
			elapsed = 0
		}
		rows = append(rows, table.Row{
			"* " + formatNumber(state.Height),
			fmt.Sprintf("%.1f%%", state.PrevotePercent*100),
			fmt.Sprintf("%.1f%%", state.PrecommitPercent*100),
			formatDuration(elapsed),
			fmt.Sprintf("%d/%d", state.Round, state.Step),
		})
	}
	for _, rec := range history {
		rows = append(rows, table.Row{
			"  " + formatNumber(rec.Height),
			fmt.Sprintf("%.1f%%", rec.PrevotePercent*100),
			fmt.Sprintf("%.1f%%", rec.PrecommitPercent*100),
			formatDuration(rec.Elapsed),
			fmt.Sprintf("%d/%d", rec.Round, rec.Step),
		})
	}
	t.SetRows(rows)
	t.UpdateViewport()
	return t
}

// newTestValidatorTableModel builds a bubbles table.Model for validator tests.
func newTestValidatorTableModel(state *consensus.State, monikers map[string]string, signHistory map[int][]bool, proposerHistory []int) table.Model {
	cols := []table.Column{
		{Title: "#", Width: 5},
		{Title: "Validator", Width: 28},
		{Title: "Power", Width: 18},
		{Title: "Blocks (newest \u2190)", Width: 40},
	}
	t := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(20))

	ts := table.DefaultStyles()
	ts.Header = mutedStyle
	ts.Cell = lipgloss.NewStyle()
	ts.Selected = highlightStyle
	t.SetStyles(ts)

	if state == nil || len(state.Validators) == 0 {
		return t
	}
	nameW := 28
	blocksW := 40
	rows := make([]table.Row, len(state.Validators))
	for i, v := range state.Validators {
		displayName := getValidatorDisplayName(v, monikers)
		if len(displayName) > nameW {
			displayName = displayName[:nameW-3] + "..."
		}
		power := formatPower(v.VotingPower)
		pct := ""
		if state.TotalVotingPower > 0 {
			pct = fmt.Sprintf("%.1f%%", float64(v.VotingPower)/float64(state.TotalVotingPower)*100)
		}
		powerCell := fmt.Sprintf("%s %s", power, pct)
		hist := signHistory[v.Index]
		bar := renderSigningBar(hist, v.Index, proposerHistory, -1, blocksW)
		rows[i] = table.Row{
			fmt.Sprintf("%d", v.Index),
			displayName,
			powerCell,
			bar,
		}
	}
	t.SetRows(rows)
	t.UpdateViewport()
	return t
}

// newTestProviderTableModel builds a bubbles table.Model for provider list tests.
func newTestProviderTableModel(providers []rpc.Provider) table.Model {
	cols := []table.Column{
		{Title: "#", Width: colWidthIndex},
		{Title: "Provider", Width: colWidthProvider},
		{Title: "Version", Width: colWidthVersion},
		{Title: "CPU", Width: colWidthCPU},
		{Title: "Memory", Width: colWidthMem},
		{Title: "GPU", Width: colWidthGPU},
		{Title: "Loc", Width: colWidthCountry},
	}
	t := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(15))

	ts := table.DefaultStyles()
	ts.Header = mutedStyle
	ts.Cell = lipgloss.NewStyle()
	ts.Selected = highlightStyle
	t.SetStyles(ts)

	filtered := filterNonLocalProviders(providers)
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
	t.SetRows(rows)
	t.UpdateViewport()
	return t
}

// newTestNodeTableModel builds a bubbles table.Model for node detail tests.
func newTestNodeTableModel(nodes []rpc.ProviderNodeWithGPU) table.Model {
	cols := []table.Column{
		{Title: "Node", Width: colWidthNodeName},
		{Title: "CPU", Width: 14},
		{Title: "Memory", Width: 16},
		{Title: "GPU", Width: 30},
	}
	t := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(10))

	ts := table.DefaultStyles()
	ts.Header = mutedStyle
	ts.Cell = lipgloss.NewStyle()
	ts.Selected = highlightStyle
	t.SetStyles(ts)

	rows := make([]table.Row, len(nodes))
	for i, node := range nodes {
		nodeName := node.Name
		if nodeName == "" {
			nodeName = fmt.Sprintf("node-%d", i+1)
		}
		rows[i] = table.Row{
			nodeName,
			formatResourceRatio(node.CPUAvailable/1000, node.CPUAllocatable/1000),
			formatMemoryRatio(node.MemAvailable, node.MemAllocatable),
			formatNodeGPU(node),
		}
	}
	t.SetRows(rows)
	t.UpdateViewport()
	return t
}

// newTestGovParamView builds a bubbles viewport.Model for governance param display.
func newTestGovParamView() viewport.Model {
	vp := viewport.New(viewport.WithWidth(60), viewport.WithHeight(20))
	vp.SetContent("test governance params content")
	return vp
}

// newTestViewContext builds a fully populated ViewContext for golden tests.
// Optional modifier functions can be passed to customize specific fields.
func newTestViewContext(opts ...func(*ViewContext)) ViewContext {
	state := newTestConsensusState(10)
	monikers := newTestMonikers(10)
	providers := newTestProviders(5)
	history := newTestBlockHistory(5)
	signHist := newTestSignHistory(10, 20)
	proposerHist := newTestProposerHistory(10, 20)

	ctx := ViewContext{
		State:             state,
		Endpoint:          "https://rpc.akashnet.net:443",
		Width:             testWidth,
		Height:            testHeight,
		HubTab:            HubNetwork,
		ActiveTab:         TabOverview,
		Embedded:          true,
		Monikers:          monikers,
		BlockHistory:      history,
		ExpandedBlock:     -1,
		ExpandedScroll:    0,
		ExpandedValidator: -1,
		ValSignHistory:    signHist,
		ProposerHistory:   proposerHist,
		CurrentProposer:   0,
		WSConnected:       true,
		Providers: ProviderViewState{
			Providers: providers,
			Versions:  []string{"0.6.4", "0.6.3", "0.6.2"},
			Selected:  "0.6.4",
		},
		Oracle: OracleState{
			Aggregated: make(map[string]*oracletypes.EventAggregatedPrice),
		},
		BlockTable:      newTestBlockTableModel(state, history),
		ValidatorTable:  newTestValidatorTableModel(state, monikers, signHist, proposerHist),
		ProviderTable:   newTestProviderTableModel(providers),
		NodeTable:       newTestNodeTableModel(newTestNodes(3)),
		GovModuleIdx:    0,
		GovModuleScroll: 0,
		GovModuleHeight: 20,
		GovParamView:    newTestGovParamView(),
	}

	for _, opt := range opts {
		opt(&ctx)
	}
	return ctx
}

// ViewContext modifier functions for use with newTestViewContext.
func withHubTab(tab HubTab) func(*ViewContext) {
	return func(ctx *ViewContext) { ctx.HubTab = tab }
}

func withTab(tab Tab) func(*ViewContext) {
	return func(ctx *ViewContext) { ctx.ActiveTab = tab }
}

func withEmbedded(v bool) func(*ViewContext) {
	return func(ctx *ViewContext) { ctx.Embedded = v }
}

func withWSConnected(v bool) func(*ViewContext) {
	return func(ctx *ViewContext) { ctx.WSConnected = v }
}

func withNilState() func(*ViewContext) {
	return func(ctx *ViewContext) { ctx.State = nil }
}

func withExpandedBlock(idx int, validators []BlockValidatorVote) func(*ViewContext) {
	return func(ctx *ViewContext) {
		ctx.ExpandedBlock = idx
		ctx.ExpandedValidators = validators
	}
}

func withExpandedValidator(idx int) func(*ViewContext) {
	return func(ctx *ViewContext) { ctx.ExpandedValidator = idx }
}

func withProviderDetail(p *rpc.Provider, nodes []rpc.ProviderNodeWithGPU) func(*ViewContext) {
	return func(ctx *ViewContext) {
		ctx.Providers.Detail = ProviderDetailState{
			Showing:  true,
			Provider: p,
			Nodes:    nodes,
		}
	}
}

func withProviderDetailLoading(p *rpc.Provider) func(*ViewContext) {
	return func(ctx *ViewContext) {
		ctx.Providers.Detail = ProviderDetailState{
			Showing:  true,
			Provider: p,
			Loading:  true,
		}
	}
}

func withProviderDetailError(p *rpc.Provider, err error) func(*ViewContext) {
	return func(ctx *ViewContext) {
		ctx.Providers.Detail = ProviderDetailState{
			Showing:  true,
			Provider: p,
			Error:    err,
		}
	}
}

func withProvidersLoading(total, loaded int) func(*ViewContext) {
	return func(ctx *ViewContext) {
		ctx.Providers.Loading = true
		ctx.Providers.Total = total
		ctx.Providers.Loaded = loaded
	}
}

func withGovernanceParams() func(*ViewContext) {
	return func(ctx *ViewContext) {
		ctx.GovernanceParams = &governance.AllParams{
			Modules: make(map[string]*governance.ModuleParams),
		}
	}
}

func withBMEStatus(status *bmetypes.QueryStatusResponse) func(*ViewContext) {
	return func(ctx *ViewContext) {
		ctx.Oracle.BMEStatus = status
	}
}

func withStateError(err error) func(*ViewContext) {
	return func(ctx *ViewContext) {
		if ctx.State != nil {
			ctx.State.Error = err
		}
	}
}
