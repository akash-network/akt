package ui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/monitor/consensus"
	"pkg.akt.dev/akt/internal/monitor/rpc"
)

func TestStatusInfoReportsActiveMonitorState(t *testing.T) {
	m := Model{
		client:      rpc.NewClient("https://rpc.example.test", "https://rest.example.test"),
		wsConnected: true,
		hubTab:      HubProvider,
		activeTab:   TabValidators,
		detail:      ProviderDetail{Showing: true},
	}

	got := m.StatusInfo()
	if got.Endpoint != "https://rpc.example.test" || !got.WSConnected ||
		got.HubTab != HubProvider || got.ActiveTab != TabValidators || !got.DetailShowing {
		t.Fatalf("StatusInfo() = %#v", got)
	}
}

func TestConsensusSnapshotBufferPreservesLastStateAtHeightBoundary(t *testing.T) {
	first := testConsensusStateAt(100, 0.75, 0.60)
	second := testConsensusStateAt(101, 0.10, 0.05)
	snapshots := make(chan rpc.ConsensusSnapshot)
	m := Model{
		pendingState:   first,
		snapshotCh:     snapshots,
		valSignHistory: make(map[int][]bool),
		maxSignHistory: 5,
		validatorTable: newTestValidatorTableModel(nil, nil, nil, nil),
		blockTable:     newTestBlockTableModel(nil, nil),
	}

	updated, cmd := m.Update(consensusSnapshotMsg{
		snapshot: rpc.ConsensusSnapshot{State: second},
		ok:       true,
	})
	m = lifecycleModelValue(t, updated)
	if cmd == nil {
		t.Fatal("snapshot handler did not rearm the subscription")
	}
	if m.state == nil || m.state.Height != first.Height {
		t.Fatalf("applied state height = %v, want %d before buffering next height", m.state, first.Height)
	}
	if m.pendingState != second {
		t.Fatalf("pending state = %p, want second snapshot %p", m.pendingState, second)
	}
	if m.peakPrevotePercent != first.PrevotePercent || m.peakPrecommitPercent != first.PrecommitPercent {
		t.Fatalf("outgoing vote peaks = %.2f/%.2f, want %.2f/%.2f",
			m.peakPrevotePercent, m.peakPrecommitPercent, first.PrevotePercent, first.PrecommitPercent)
	}
}

func TestConsensusStateBuildsExactCompletedBlockHistory(t *testing.T) {
	m := Model{
		knownProposerIndex: 1,
		knownProposerAddr:  "BB",
		valSignHistory: map[int][]bool{
			0: {false, true},
			1: {true, true},
		},
		proposerHistory: []int{0, 1},
		maxSignHistory:  2,
		validatorTable:  newTestValidatorTableModel(nil, nil, nil, nil),
		blockTable:      newTestBlockTableModel(nil, nil),
	}

	// The feed can start midway through a block. The first transition is
	// deliberately discarded because its signing record is incomplete.
	start := testConsensusStateAt(200, 0.40, 0.20)
	m.handleStateMsg(stateMsg{state: start})
	firstComplete := testConsensusStateAt(201, 0.10, 0.05)
	m.handleStateMsg(stateMsg{state: firstComplete})
	if len(m.blockHistory) != 0 || !m.firstHeightSeen {
		t.Fatalf("first height transition history=%d firstHeightSeen=%t", len(m.blockHistory), m.firstHeightSeen)
	}

	// Vote percentages may dip in an intermediate snapshot. The completed
	// record must retain the high-water marks for the exact outgoing height.
	high := testConsensusStateAt(201, 0.90, 0.80)
	high.Round = 2
	high.Step = 6
	high.Validators[0].Precommited = true
	high.Validators[1].Precommited = false
	m.handleStateMsg(stateMsg{state: high})
	low := testConsensusStateAt(201, 0.20, 0.10)
	low.Round = 2
	low.Step = 7
	low.Validators = high.Validators
	m.handleStateMsg(stateMsg{state: low})
	m.handleStateMsg(stateMsg{state: testConsensusStateAt(202, 0.01, 0.01)})

	if len(m.blockHistory) != 1 {
		t.Fatalf("completed blocks = %d, want 1", len(m.blockHistory))
	}
	record := m.blockHistory[0]
	if record.Height != 201 || record.PrevotePercent != 0.90 || record.PrecommitPercent != 0.80 ||
		record.Round != 2 || record.Step != 7 {
		t.Fatalf("completed block = %#v", record)
	}
	if len(record.Validators) != 2 || !record.Validators[0].Precommited || record.Validators[1].Precommited {
		t.Fatalf("completed validator votes = %#v", record.Validators)
	}
	if got := m.valSignHistory[0]; len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("validator 0 history = %v, want [true false]", got)
	}
	if got := m.valSignHistory[1]; len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("validator 1 history = %v, want [false true]", got)
	}
	if got := m.proposerHistory; len(got) != 2 || got[0] != 1 || got[1] != 0 {
		t.Fatalf("proposer history = %v, want [1 0]", got)
	}
}

func TestConsensusProposerIsAppliedToEveryNewState(t *testing.T) {
	m := Model{
		knownProposerIndex: 1,
		knownProposerAddr:  "full-consensus-address",
		valSignHistory:     make(map[int][]bool),
		maxSignHistory:     5,
		validatorTable:     newTestValidatorTableModel(nil, nil, nil, nil),
		blockTable:         newTestBlockTableModel(nil, nil),
	}
	state := testConsensusStateAt(300, 0.7, 0.6)
	m.handleStateMsg(stateMsg{state: state})

	if m.state.ProposerIndex != 1 || m.state.ProposerAddress != "full-consensus-address" {
		t.Fatalf("proposer = %d/%q", m.state.ProposerIndex, m.state.ProposerAddress)
	}
	for i, validator := range m.state.Validators {
		if validator.IsProposer != (i == 1) {
			t.Fatalf("validator %d proposer=%t", i, validator.IsProposer)
		}
	}
}

func TestConsensusErrorRemainsVisibleUntilConnectionRecovers(t *testing.T) {
	wantErr := errors.New("websocket unavailable")
	m := Model{}

	updated, retry := m.Update(stateMsg{err: wantErr})
	m = lifecycleModelValue(t, updated)
	if retry == nil || m.state == nil || !errors.Is(m.state.Error, wantErr) {
		t.Fatalf("failed connection state=%#v retry=%v", m.state, retry != nil)
	}

	updates := make(chan rpc.ConsensusSnapshot)
	m.consensusRetryAttempt = 5
	updated, wait := m.Update(wsConnectedMsg{ch: updates})
	m = lifecycleModelValue(t, updated)
	if wait == nil || !m.wsConnected || m.consensusRetryAttempt != 0 || m.state.Error != nil {
		t.Fatalf("recovered state connected=%t retry=%d error=%v", m.wsConnected, m.consensusRetryAttempt, m.state.Error)
	}
}

func TestExpansionSnapshotsDoNotTrackLaterMutations(t *testing.T) {
	state := testConsensusStateAt(400, 0.8, 0.7)
	m := Model{
		state:             state,
		expandedBlock:     -1,
		expandedValidator: -1,
		blockTable:        newTestBlockTableModel(state, nil),
		validatorTable:    newTestValidatorTableModel(state, nil, nil, nil),
		width:             100,
		height:            30,
	}

	m.toggleBlockExpansion()
	if m.expandedBlock != 0 || len(m.expandedValidators) != len(state.Validators) {
		t.Fatalf("expanded block=%d validators=%d", m.expandedBlock, len(m.expandedValidators))
	}
	state.Validators[0].Precommited = !state.Validators[0].Precommited
	if m.expandedValidators[0].Precommited == state.Validators[0].Precommited {
		t.Fatal("expanded current-block snapshot changed with the live state")
	}
	m.toggleBlockExpansion()
	if m.expandedBlock != -1 || m.expandedValidators != nil || m.expandedScroll != 0 {
		t.Fatalf("collapsed block state = block %d validators=%v scroll=%d",
			m.expandedBlock, m.expandedValidators, m.expandedScroll)
	}

	m.toggleValidatorExpansion()
	if m.expandedValidator != 0 {
		t.Fatalf("expanded validator = %d, want 0", m.expandedValidator)
	}
	m.toggleValidatorExpansion()
	if m.expandedValidator != -1 {
		t.Fatalf("collapsed validator = %d, want -1", m.expandedValidator)
	}
}

func TestWindowResizeClampsComponentDimensions(t *testing.T) {
	m := Model{
		expandedBlock:     -1,
		expandedValidator: -1,
		providerTable:     newTestProviderTableModel(nil),
		nodeTable:         newTestNodeTableModel(nil),
		validatorTable:    newTestValidatorTableModel(nil, nil, nil, nil),
		blockTable:        newTestBlockTableModel(nil, nil),
	}
	m.govProposalView = newTestGovParamView()
	m.govParamView = newTestGovParamView()

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 20, Height: 4})
	m = lifecycleModelValue(t, updated)
	if cmd != nil {
		t.Fatal("window resize returned an asynchronous command")
	}
	if m.width != 20 || m.height != 4 || m.govModuleHeight != 5 {
		t.Fatalf("resized model = %dx%d governance rows=%d", m.width, m.height, m.govModuleHeight)
	}
}

func testConsensusStateAt(height int64, prevote, precommit float64) *consensus.State {
	state := newTestConsensusState(2)
	state.Height = height
	state.PrevotePercent = prevote
	state.PrecommitPercent = precommit
	state.StartTime = time.Now().Add(-time.Second)
	return state
}
