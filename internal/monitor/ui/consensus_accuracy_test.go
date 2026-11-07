package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/monitor/consensus"
	"pkg.akt.dev/akt/internal/monitor/rpc"
)

func TestRenderTickAdvancesOnlyLiveElapsedTime(t *testing.T) {
	for _, connected := range []bool{true, false} {
		m := Model{
			state: &consensus.State{
				Height:         42,
				StartTime:      time.Now().Add(-time.Second),
				Elapsed:        time.Millisecond,
				PrevotePercent: .60,
			},
			wsConnected: connected,
			blockTable:  newTestBlockTableModel(nil, nil),
		}
		updated, next := m.Update(renderTickMsg(time.Now()))
		m = lifecycleModelValue(t, updated)
		require.NotNil(t, next)
		require.Equal(t, .60, m.state.PrevotePercent)
		if connected {
			require.GreaterOrEqual(t, m.state.Elapsed, time.Second)
		} else {
			require.Equal(t, time.Millisecond, m.state.Elapsed)
		}
	}
}

func TestSnapshotErrorBeforeBootstrapIsVisible(t *testing.T) {
	wantErr := errors.New("bootstrap unavailable")
	m := Model{}
	updated, _ := m.Update(consensusSnapshotMsg{
		ok:       true,
		snapshot: rpc.ConsensusSnapshot{Err: wantErr},
	})
	m = lifecycleModelValue(t, updated)
	require.False(t, m.wsConnected)
	require.ErrorIs(t, m.state.Error, wantErr)
}

func TestSnapshotBlockPrunesExpiredHeadersAndPreservesIntervalBoundary(t *testing.T) {
	stamp := time.Date(2026, time.November, 7, 12, 0, 0, 0, time.UTC)
	const height = MaxBlockHistory + 50
	const boundary = height - MaxBlockHistory - 1
	m := Model{
		blockTimes: map[int64]time.Time{
			boundary - 1: stamp.Add(-2 * time.Second),
			boundary:     stamp.Add(-time.Second),
		},
		blockTable: newTestBlockTableModel(nil, nil),
	}
	updated, _ := m.Update(consensusSnapshotMsg{
		ok: true,
		snapshot: rpc.ConsensusSnapshot{
			Block: &rpc.CommittedBlock{
				Height: height,
				Time:   stamp,
			},
		},
	})
	m = lifecycleModelValue(t, updated)
	require.NotContains(t, m.blockTimes, int64(boundary-1))
	require.Equal(t, stamp.Add(-time.Second), m.blockTimes[boundary])
	require.Equal(t, stamp, m.blockTimes[height])
}

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
