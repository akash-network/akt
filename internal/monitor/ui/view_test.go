package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"pkg.akt.dev/akt/internal/monitor/consensus"
	"pkg.akt.dev/akt/internal/monitor/rpc"
)

func TestRenderView(t *testing.T) {
	tests := map[string]struct {
		ctx ViewContext
	}{
		"NetworkOverview": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabOverview), withEmbedded(false)),
		},
		"NetworkValidators": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabValidators), withEmbedded(false)),
		},
		"NetworkGovernance": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabGovernance), withEmbedded(false)),
		},
		"ProviderList": {
			ctx: newTestViewContext(withHubTab(HubProvider), withEmbedded(false)),
		},
		"OracleBME": {
			ctx: newTestViewContext(withHubTab(HubOracleBME), withEmbedded(false)),
		},
		"Embedded": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabOverview), withEmbedded(true)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderView(tc.ctx))
		})
	}
}

func TestRenderHubTabBar(t *testing.T) {
	tests := map[string]struct {
		active HubTab
	}{
		"Network":   {HubNetwork},
		"Provider":  {HubProvider},
		"OracleBME": {HubOracleBME},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderHubTabBar(tc.active, testWidth))
		})
	}
}

func TestRenderTabBar(t *testing.T) {
	tests := map[string]struct {
		active Tab
	}{
		"Overview":   {TabOverview},
		"Validators": {TabValidators},
		"Governance": {TabGovernance},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderTabBar(tc.active, testWidth))
		})
	}
}

func TestRenderBlockProgress(t *testing.T) {
	tests := map[string]struct {
		ctx ViewContext
	}{
		"WithState": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabOverview)),
		},
		"NoState": {
			ctx: newTestViewContext(withNilState()),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderBlockProgress(tc.ctx))
		})
	}
}

func TestRenderBlockDetailOverlay(t *testing.T) {
	state := newTestConsensusState(10)
	validators := make([]BlockValidatorVote, len(state.Validators))
	for i, v := range state.Validators {
		validators[i] = BlockValidatorVote{
			Index: v.Index, Address: v.Address, PubKey: v.PubKey,
			VotingPower: v.VotingPower, Prevoted: v.Prevoted, Precommited: v.Precommited,
		}
	}

	tests := map[string]struct {
		ctx ViewContext
	}{
		"CurrentBlock": {
			ctx: newTestViewContext(withExpandedBlock(0, validators)),
		},
		"HistoryBlock": {
			ctx: newTestViewContext(withExpandedBlock(1, validators)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderBlockDetailOverlay(tc.ctx))
		})
	}
}

func TestRenderExpandedValidators(t *testing.T) {
	state := newTestConsensusState(20)
	monikers := newTestMonikers(20)
	validators := make([]BlockValidatorVote, len(state.Validators))
	for i, v := range state.Validators {
		validators[i] = BlockValidatorVote{
			Index: v.Index, Address: v.Address, PubKey: v.PubKey,
			VotingPower: v.VotingPower, Prevoted: v.Prevoted, Precommited: v.Precommited,
		}
	}

	tests := map[string]struct {
		maxRows   int
		scrollPos int
	}{
		"Few":          {maxRows: 30, scrollPos: 0},
		"ManyScrolled": {maxRows: 10, scrollPos: 5},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderExpandedValidators(validators, monikers, tc.maxRows, tc.scrollPos, testWidth))
		})
	}
}

func TestRenderConsensusSection(t *testing.T) {
	tests := map[string]struct {
		state *consensus.State
	}{
		"Normal": {
			state: newTestConsensusState(10),
		},
		"HighRound": {
			state: func() *consensus.State {
				s := newTestConsensusState(10)
				s.Round = 5
				s.Step = 3
				return s
			}(),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderConsensusSection(tc.state))
		})
	}
}

func TestRenderVoteSection(t *testing.T) {
	tests := map[string]struct {
		state *consensus.State
	}{
		"BelowThreshold": {
			state: func() *consensus.State {
				s := newTestConsensusState(10)
				s.PrevotePercent = 0.5
				s.PrecommitPercent = 0.3
				return s
			}(),
		},
		"AboveThreshold": {
			state: func() *consensus.State {
				s := newTestConsensusState(10)
				s.PrevotePercent = 0.95
				s.PrecommitPercent = 0.92
				return s
			}(),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderVoteSection(tc.state))
		})
	}
}

func TestRenderGridSection(t *testing.T) {
	tests := map[string]struct {
		state *consensus.State
	}{
		"AllVoted": {
			state: func() *consensus.State {
				s := newTestConsensusState(10)
				s.PrevoteBitArray = "xxxxxxxxxx"
				return s
			}(),
		},
		"NoneVoted": {
			state: func() *consensus.State {
				s := newTestConsensusState(10)
				s.PrevoteBitArray = "__________"
				return s
			}(),
		},
		"Mixed": {
			state: newTestConsensusState(10),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderGridSection(tc.state, testWidth))
		})
	}
}

func TestRenderValidatorsTab(t *testing.T) {
	tests := map[string]struct {
		ctx ViewContext
	}{
		"Loading": {
			ctx: newTestViewContext(withNilState()),
		},
		"Populated": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabValidators)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderValidatorsTab(tc.ctx))
		})
	}
}

func TestRenderValidatorDetailOverlay(t *testing.T) {
	ctx := newTestViewContext(withHubTab(HubNetwork), withTab(TabValidators), withExpandedValidator(0))
	golden.RequireEqual(t, renderValidatorDetailOverlay(ctx))
}

func TestRenderValidatorDetailPanel(t *testing.T) {
	state := newTestConsensusState(10)
	monikers := newTestMonikers(10)
	signHist := newTestSignHistory(10, 20)

	tests := map[string]struct {
		v consensus.ValidatorStatus
	}{
		"WithHistory": {v: state.Validators[0]},
		"Proposer": {v: func() consensus.ValidatorStatus {
			v := state.Validators[0]
			v.IsProposer = true
			return v
		}()},
		"NoHistory": {v: state.Validators[9]},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderValidatorDetailPanel(tc.v, monikers, signHist, state.TotalVotingPower, testWidth))
		})
	}
}

func TestRenderProvidersTab(t *testing.T) {
	tests := map[string]struct {
		ctx ViewContext
	}{
		"Loading": {
			ctx: newTestViewContext(withHubTab(HubProvider), withProvidersLoading(100, 50)),
		},
		"Empty": {
			ctx: func() ViewContext {
				ctx := newTestViewContext(withHubTab(HubProvider))
				ctx.Providers.Providers = nil
				ctx.ProviderTable = newTestProviderTableModel(nil)
				return ctx
			}(),
		},
		"Populated": {
			ctx: newTestViewContext(withHubTab(HubProvider)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderProvidersTab(tc.ctx))
		})
	}
}

func TestRenderProviderDetailView(t *testing.T) {
	providers := newTestProviders(1)
	p := &providers[0]
	nodes := newTestNodes(3)

	tests := map[string]struct {
		ctx ViewContext
	}{
		"WithNodes": {
			ctx: newTestViewContext(withHubTab(HubProvider), withProviderDetail(p, nodes)),
		},
		"Loading": {
			ctx: newTestViewContext(withHubTab(HubProvider), withProviderDetailLoading(p)),
		},
		"Error": {
			ctx: newTestViewContext(withHubTab(HubProvider), withProviderDetailError(p, fmt.Errorf("connection refused"))),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderProviderDetailView(tc.ctx))
		})
	}
}

func TestRenderGovernanceTab(t *testing.T) {
	tests := map[string]struct {
		ctx ViewContext
	}{
		"Loading": {
			ctx: func() ViewContext {
				ctx := newTestViewContext(withHubTab(HubNetwork), withTab(TabParameters))
				ctx.GovernanceParams = nil
				return ctx
			}(),
		},
		"WithParams": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabParameters), withGovernanceParams()),
		},
		"ScrolledToBottom": {
			ctx: func() ViewContext {
				ctx := newTestViewContext(withHubTab(HubNetwork), withTab(TabParameters), withGovernanceParams())
				ctx.GovModuleHeight = 8 // simulate small terminal: only 8 rows for the list
				ctx.GovModuleIdx = 11   // last module (crisis)
				ctx.GovModuleScroll = 5 // scrolled so items 5..11 + indicator visible
				return ctx
			}(),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderParametersTab(tc.ctx))
		})
	}
}

func TestRenderGovModuleList(t *testing.T) {
	tests := map[string]struct {
		selectedIdx int
		scrollOff   int
		visibleRows int
	}{
		"AllFit": {
			selectedIdx: 0,
			scrollOff:   0,
			visibleRows: 20, // plenty of room for 12 modules
		},
		"SmallTerminal": {
			selectedIdx: 3,
			scrollOff:   0,
			visibleRows: 8,
		},
		"ScrolledDown": {
			selectedIdx: 11,
			scrollOff:   5,
			visibleRows: 8,
		},
		"MiddleScroll": {
			selectedIdx: 6,
			scrollOff:   2,
			visibleRows: 8,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderGovModuleList(tc.selectedIdx, tc.scrollOff, tc.visibleRows))
		})
	}
}

func TestRenderOraclePanel(t *testing.T) {
	ctx := newTestViewContext(withHubTab(HubOracleBME))
	golden.RequireEqual(t, renderOraclePanel(ctx, testWidth/2))
}

func TestRenderBMEPanel(t *testing.T) {
	tests := map[string]struct {
		ctx ViewContext
	}{
		"Loading": {
			ctx: newTestViewContext(withHubTab(HubOracleBME)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderBMEPanel(tc.ctx, testWidth/2))
		})
	}
}

func TestRenderStatusBar(t *testing.T) {
	tests := map[string]struct {
		hub         HubTab
		tab         Tab
		detail      bool
		wsConnected bool
	}{
		"Overview":       {HubNetwork, TabOverview, false, true},
		"Validators":     {HubNetwork, TabValidators, false, true},
		"Governance":     {HubNetwork, TabGovernance, false, true},
		"Provider":       {HubProvider, TabOverview, false, true},
		"ProviderDetail": {HubProvider, TabOverview, true, true},
		"OracleBME":      {HubOracleBME, TabOverview, false, true},
		"WSConnected":    {HubNetwork, TabOverview, false, true},
		"HTTPOnly":       {HubNetwork, TabOverview, false, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderStatusBar(
				"https://rpc.akashnet.net:443",
				tc.hub,
				tc.tab,
				tc.detail,
				tc.wsConnected,
				testWidth,
			))
		})
	}
}

func TestRenderStatusBarDistinguishesDashboardAndNetworkNavigation(t *testing.T) {
	got := renderStatusBar(
		"https://rpc.akashnet.net:443",
		HubNetwork,
		TabOverview,
		false,
		true,
		testWidth,
	)
	for _, want := range []string{"Tab/Shift-Tab", "1-4"} {
		if !strings.Contains(got, want) {
			t.Errorf("status bar missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Tab/1-4") {
		t.Errorf("status bar conflates dashboard and sub-tab navigation:\n%s", got)
	}
}

func TestRenderNetworkDashboard(t *testing.T) {
	tests := map[string]struct {
		ctx ViewContext
	}{
		"Overview": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabOverview)),
		},
		"NilState": {
			ctx: newTestViewContext(withNilState()),
		},
		"WithError": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabOverview), withStateError(fmt.Errorf("connection refused"))),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderNetworkDashboard(tc.ctx))
		})
	}
}

func TestRenderProviderDashboard(t *testing.T) {
	providers := newTestProviders(1)
	p := &providers[0]
	nodes := newTestNodes(3)

	tests := map[string]struct {
		ctx ViewContext
	}{
		"List": {
			ctx: newTestViewContext(withHubTab(HubProvider)),
		},
		"Detail": {
			ctx: newTestViewContext(withHubTab(HubProvider), withProviderDetail(p, nodes)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderProviderDashboard(tc.ctx))
		})
	}
}

func TestRenderOracleBMEDashboard(t *testing.T) {
	tests := map[string]struct {
		ctx ViewContext
	}{
		"Empty": {
			ctx: newTestViewContext(withHubTab(HubOracleBME)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderOracleBMEDashboard(tc.ctx))
		})
	}
}

func TestRenderOverviewTab(t *testing.T) {
	tests := map[string]struct {
		ctx ViewContext
	}{
		"Normal": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabOverview)),
		},
		"NilState": {
			ctx: newTestViewContext(withNilState()),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderOverviewTab(tc.ctx))
		})
	}
}

func TestRenderVersionDistribution(t *testing.T) {
	providers := newTestProviders(5)

	tests := map[string]struct {
		providers []rpc.Provider
		versions  []string
		selected  string
	}{
		"Loading": {
			providers: nil,
			versions:  []string{"0.6.4"},
			selected:  "0.6.4",
		},
		"Populated": {
			providers: providers,
			versions:  []string{"0.6.4", "0.6.3", "0.6.2"},
			selected:  "0.6.4",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderVersionDistribution(tc.providers, tc.versions, tc.selected))
		})
	}
}

func TestRenderVoteLine(t *testing.T) {
	tests := map[string]struct {
		label      string
		percent    float64
		power      int64
		totalPower int64
	}{
		"AboveThreshold": {"Prevotes:", 0.95, 950000, 1000000},
		"BelowThreshold": {"Prevotes:", 0.45, 450000, 1000000},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderVoteLine(tc.label, tc.percent, tc.power, tc.totalPower))
		})
	}
}

func TestRenderValidatorRowWithBlocks(t *testing.T) {
	state := newTestConsensusState(10)
	monikers := newTestMonikers(10)
	signHist := newTestSignHistory(10, 20)
	proposerHist := newTestProposerHistory(10, 20)

	tests := map[string]struct {
		validator  consensus.ValidatorStatus
		isSelected bool
	}{
		"Normal": {
			validator:  state.Validators[0],
			isSelected: false,
		},
		"Selected": {
			validator:  state.Validators[0],
			isSelected: true,
		},
		"NoHistory": {
			validator: func() consensus.ValidatorStatus {
				v := state.Validators[9]
				return v
			}(),
			isSelected: false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderValidatorRowWithBlocks(tc.validator, monikers, signHist, proposerHist, state.TotalVotingPower, 28, 40, tc.isSelected))
		})
	}
}
