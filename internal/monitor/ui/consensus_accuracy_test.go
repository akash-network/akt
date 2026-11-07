package ui

import (
	"errors"
	"testing"
	"time"

	"pkg.akt.dev/akt/internal/monitor/rpc"
)

func TestBlockIntervalsUseHeadersDespiteDelayedOrMissingDelivery(t *testing.T) {
	start := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	m := Model{
		blockHistory: []BlockRecord{
			{Height: 12},
			{Height: 11},
		},
		blockTable: newTestBlockTableModel(nil, nil),
	}
	// Headers arrive out of order, long after their timestamps. The interval
	// must depend only on consecutive committed heights, never wall time.
	m.handleCommittedBlock(&rpc.CommittedBlock{
		Height: 12,
		Time:   start.Add(7 * time.Second),
	})
	m.handleCommittedBlock(&rpc.CommittedBlock{
		Height: 10,
		Time:   start,
	})
	if m.blockHistory[0].Elapsed != 0 || m.blockHistory[1].Elapsed != 0 {
		t.Fatal("missing adjacent header produced a fabricated interval")
	}
	m.handleCommittedBlock(&rpc.CommittedBlock{
		Height: 11,
		Time:   start.Add(3 * time.Second),
	})
	if m.blockHistory[0].Elapsed != 4*time.Second || m.blockHistory[1].Elapsed != 3*time.Second {
		t.Fatalf("intervals = %v/%v", m.blockHistory[0].Elapsed, m.blockHistory[1].Elapsed)
	}
}

func TestStreamDisconnectMarksLivePercentagesStaleUntilBootstrap(t *testing.T) {
	m := Model{
		state:          testConsensusStateAt(42, .60, .25),
		wsConnected:    true,
		snapshotCh:     make(chan rpc.ConsensusSnapshot),
		blockTable:     newTestBlockTableModel(nil, nil),
		validatorTable: newTestValidatorTableModel(nil, nil, nil, nil),
	}
	wantErr := errors.New("transport disconnected")
	updated, _ := m.Update(consensusSnapshotMsg{
		ok: true,
		snapshot: rpc.ConsensusSnapshot{
			Err: wantErr,
		},
	})
	m = lifecycleModelValue(t, updated)
	if m.wsConnected || !errors.Is(m.state.Error, wantErr) {
		t.Fatal("disconnected stream still reported live percentages")
	}
	updated, _ = m.Update(consensusSnapshotMsg{
		ok: true,
		snapshot: rpc.ConsensusSnapshot{
			State: testConsensusStateAt(45, .85, .85),
			Reset: true,
		},
	})
	m = lifecycleModelValue(t, updated)
	if !m.wsConnected || m.state.Error != nil || len(m.blockHistory) != 0 {
		t.Fatal("reconnect did not replace stale state or fabricated gap history")
	}
}
