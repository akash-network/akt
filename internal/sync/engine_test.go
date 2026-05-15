package sync_test

import (
	"context"
	"path/filepath"
	"testing"

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
