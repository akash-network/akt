package ui

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"pkg.akt.dev/akt/internal/monitor/consensus"
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
				ctx := newTestViewContext(withHubTab(HubNetwork), withTab(TabGovernance))
				ctx.GovernanceParams = nil
				return ctx
			}(),
		},
		"WithParams": {
			ctx: newTestViewContext(withHubTab(HubNetwork), withTab(TabGovernance)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderGovernanceTab(tc.ctx))
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
		tab         Tab
		wsConnected bool
	}{
		"Overview":    {TabOverview, true},
		"Validators":  {TabValidators, true},
		"Governance":  {TabGovernance, true},
		"WSConnected": {TabOverview, true},
		"HTTPOnly":    {TabOverview, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, renderStatusBar("https://rpc.akashnet.net:443", tc.tab, false, tc.wsConnected, testWidth))
		})
	}
}
