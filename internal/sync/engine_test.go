package sync_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dtypes "pkg.akt.dev/go/node/deployment/v1"
	mtypes "pkg.akt.dev/go/node/market/v1"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
	syncpkg "pkg.akt.dev/akt/internal/sync"
)

const testOwner = "akash1abc"

func openTestStore(t *testing.T) store.Store {
	t.Helper()
	s, err := bbolt.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestHandleDeploymentCreated(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	eng := syncpkg.New(s, []string{testOwner})

	ev := &dtypes.EventDeploymentCreated{
		ID:   dtypes.DeploymentID{Owner: testOwner, DSeq: 100},
		Hash: []byte("v1hash"),
	}
	require.NoError(t, eng.HandleEvent(ctx, ev))

	dep, err := s.GetDeployment(ctx, testOwner, 100)
	require.NoError(t, err)
	require.NotNil(t, dep)
	assert.Equal(t, "active", dep.State)
	assert.Equal(t, testOwner, dep.Owner)
	assert.Equal(t, uint64(100), dep.DSeq)
	assert.Equal(t, []byte("v1hash"), dep.Version)
}

func TestHandleDeploymentClosed(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	eng := syncpkg.New(s, []string{testOwner})

	// Create deployment first.
	require.NoError(t, eng.HandleEvent(ctx, &dtypes.EventDeploymentCreated{
		ID: dtypes.DeploymentID{Owner: testOwner, DSeq: 200},
	}))

	// Close it.
	require.NoError(t, eng.HandleEvent(ctx, &dtypes.EventDeploymentClosed{
		ID: dtypes.DeploymentID{Owner: testOwner, DSeq: 200},
	}))

	dep, err := s.GetDeployment(ctx, testOwner, 200)
	require.NoError(t, err)
	require.NotNil(t, dep)
	assert.Equal(t, "closed", dep.State)
	assert.NotZero(t, dep.ClosedAt)
}

func TestHandleBidCreated(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	eng := syncpkg.New(s, []string{testOwner})

	ev := &mtypes.EventBidCreated{
		ID: mtypes.BidID{
			Owner:    testOwner,
			DSeq:     100,
			GSeq:     1,
			OSeq:     1,
			Provider: "akash1provider",
		},
		Price: sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("50")),
	}
	require.NoError(t, eng.HandleEvent(ctx, ev))

	bid, err := s.GetBid(ctx, store.BidID{
		Owner:    testOwner,
		DSeq:     100,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1provider",
	})
	require.NoError(t, err)
	require.NotNil(t, bid)
	assert.Equal(t, "open", bid.State)
}

func TestHandleLeaseCreated(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	eng := syncpkg.New(s, []string{testOwner})

	bidID := mtypes.BidID{
		Owner:    testOwner,
		DSeq:     100,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1provider",
	}

	// Create a bid first.
	require.NoError(t, eng.HandleEvent(ctx, &mtypes.EventBidCreated{
		ID:    bidID,
		Price: sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("50")),
	}))

	// Create lease.
	leaseEv := &mtypes.EventLeaseCreated{
		ID: mtypes.LeaseID{
			Owner:    testOwner,
			DSeq:     100,
			GSeq:     1,
			OSeq:     1,
			Provider: "akash1provider",
		},
		Price: sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("50")),
	}
	require.NoError(t, eng.HandleEvent(ctx, leaseEv))

	// Verify lease is active.
	lease, err := s.GetLease(ctx, store.LeaseID{
		Owner:    testOwner,
		DSeq:     100,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1provider",
	})
	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.Equal(t, "active", lease.State)

	// Verify bid is now matched.
	bid, err := s.GetBid(ctx, store.BidID{
		Owner:    testOwner,
		DSeq:     100,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1provider",
	})
	require.NoError(t, err)
	require.NotNil(t, bid)
	assert.Equal(t, "matched", bid.State)
}

func TestHandleLeaseClosed(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	eng := syncpkg.New(s, []string{testOwner})

	leaseID := mtypes.LeaseID{
		Owner:    testOwner,
		DSeq:     100,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1provider",
	}

	// Create lease first.
	require.NoError(t, eng.HandleEvent(ctx, &mtypes.EventLeaseCreated{
		ID:    leaseID,
		Price: sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("50")),
	}))

	// Close it.
	require.NoError(t, eng.HandleEvent(ctx, &mtypes.EventLeaseClosed{
		ID: leaseID,
	}))

	lease, err := s.GetLease(ctx, store.LeaseID{
		Owner:    testOwner,
		DSeq:     100,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1provider",
	})
	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.Equal(t, "closed", lease.State)
	assert.NotZero(t, lease.ClosedAt)
}

func TestFilterByOwner(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	eng := syncpkg.New(s, []string{"akash1abc"})

	// Event for untracked owner — should be ignored.
	require.NoError(t, eng.HandleEvent(ctx, &dtypes.EventDeploymentCreated{
		ID: dtypes.DeploymentID{Owner: "akash1xyz", DSeq: 1},
	}))

	deps, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	assert.Empty(t, deps)

	// Event for tracked owner — should be processed.
	require.NoError(t, eng.HandleEvent(ctx, &dtypes.EventDeploymentCreated{
		ID: dtypes.DeploymentID{Owner: "akash1abc", DSeq: 2},
	}))

	deps, err = s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	assert.Len(t, deps, 1)
	assert.Equal(t, "akash1abc", deps[0].Owner)
}

func TestUnknownEventIgnored(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	eng := syncpkg.New(s, []string{testOwner})

	type unknownEvent struct{ Foo string }

	err := eng.HandleEvent(ctx, &unknownEvent{Foo: "bar"})
	require.NoError(t, err)

	// Store should be empty.
	deps, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	assert.Empty(t, deps)

	leases, err := s.ListLeases(ctx, store.LeaseFilter{})
	require.NoError(t, err)
	assert.Empty(t, leases)

	bids, err := s.ListBids(ctx, store.BidFilter{})
	require.NoError(t, err)
	assert.Empty(t, bids)
}

// --- Reconciliation Tests (T053) ---

type mockQuerier struct {
	height int64
	deps   map[string][]*store.DeploymentRecord
	leases map[string][]*store.LeaseRecord
	bids   map[string][]*store.BidRecord
}

func (m *mockQuerier) CurrentHeight(_ context.Context) (int64, error) {
	return m.height, nil
}

func (m *mockQuerier) Deployments(_ context.Context, owner string) ([]*store.DeploymentRecord, error) {
	return m.deps[owner], nil
}

func (m *mockQuerier) Leases(_ context.Context, owner string, dseq uint64) ([]*store.LeaseRecord, error) {
	key := owner + ":" + store.DeploymentKey(owner, dseq)
	return m.leases[key], nil
}

func (m *mockQuerier) Bids(_ context.Context, owner string, dseq uint64) ([]*store.BidRecord, error) {
	key := owner + ":" + store.DeploymentKey(owner, dseq)
	return m.bids[key], nil
}

func TestReconcileFirstLaunch(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	eng := syncpkg.New(s, []string{testOwner})

	dep1 := &store.DeploymentRecord{Owner: testOwner, DSeq: 100, State: "active"}
	dep2 := &store.DeploymentRecord{Owner: testOwner, DSeq: 200, State: "closed"}
	lease1 := &store.LeaseRecord{
		ID:    store.LeaseID{Owner: testOwner, DSeq: 100, GSeq: 1, OSeq: 1, Provider: "akash1prov"},
		State: "active",
	}

	q := &mockQuerier{
		height: 50000,
		deps:   map[string][]*store.DeploymentRecord{testOwner: {dep1, dep2}},
		leases: map[string][]*store.LeaseRecord{
			testOwner + ":" + store.DeploymentKey(testOwner, 100): {lease1},
		},
		bids: map[string][]*store.BidRecord{},
	}

	err := eng.Reconcile(ctx, q)
	require.NoError(t, err)

	// Verify deployments stored.
	deps, err := s.ListDeployments(ctx, store.DeploymentFilter{Owner: testOwner})
	require.NoError(t, err)
	assert.Len(t, deps, 2)

	// Verify lease stored.
	leases, err := s.ListLeases(ctx, store.LeaseFilter{Owner: testOwner})
	require.NoError(t, err)
	assert.Len(t, leases, 1)

	// Verify sync state.
	ss, err := s.GetSyncState(ctx)
	require.NoError(t, err)
	require.NotNil(t, ss)
	assert.Equal(t, int64(50000), ss.LastBlockHeight)
	assert.Contains(t, ss.TrackedAccounts, testOwner)
}

func TestReconcileLargeGap(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	// Set existing sync state with old height.
	require.NoError(t, s.PutSyncState(ctx, &store.SyncState{
		LastBlockHeight: 1000,
		LastSyncTime:    1000,
	}))

	eng := syncpkg.New(s, []string{testOwner})

	dep := &store.DeploymentRecord{Owner: testOwner, DSeq: 300, State: "active"}
	q := &mockQuerier{
		height: 3000, // gap = 2000 > 1000 threshold
		deps:   map[string][]*store.DeploymentRecord{testOwner: {dep}},
		leases: map[string][]*store.LeaseRecord{},
		bids:   map[string][]*store.BidRecord{},
	}

	err := eng.Reconcile(ctx, q)
	require.NoError(t, err)

	// Full reconcile should have stored the deployment.
	d, err := s.GetDeployment(ctx, testOwner, 300)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, "active", d.State)

	// Sync state updated.
	ss, err := s.GetSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), ss.LastBlockHeight)
}

func TestReconcileSmallGap(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	// Set existing sync state with recent height.
	require.NoError(t, s.PutSyncState(ctx, &store.SyncState{
		LastBlockHeight: 1000,
		LastSyncTime:    1000,
	}))

	eng := syncpkg.New(s, []string{testOwner})

	q := &mockQuerier{
		height: 1500, // gap = 500 ≤ 1000 threshold
		deps:   map[string][]*store.DeploymentRecord{},
		leases: map[string][]*store.LeaseRecord{},
		bids:   map[string][]*store.BidRecord{},
	}

	err := eng.Reconcile(ctx, q)
	require.NoError(t, err)

	// Sync state updated to current height.
	ss, err := s.GetSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1500), ss.LastBlockHeight)
}

func TestBackoffDelay(t *testing.T) {
	// Verify exponential backoff with cap at 60s.
	d0 := syncpkg.BackoffDelay(0) // 1s + jitter
	assert.True(t, d0 >= 1*time.Second && d0 < 2*time.Second, "attempt 0: %v", d0)

	d1 := syncpkg.BackoffDelay(1) // 2s + jitter
	assert.True(t, d1 >= 2*time.Second && d1 < 3*time.Second, "attempt 1: %v", d1)

	d6 := syncpkg.BackoffDelay(6) // 64s → capped to 60s + jitter
	assert.True(t, d6 >= 60*time.Second && d6 < 90*time.Second, "attempt 6: %v", d6)

	d10 := syncpkg.BackoffDelay(10) // still capped at 60s
	assert.True(t, d10 >= 60*time.Second && d10 < 90*time.Second, "attempt 10: %v", d10)
}
