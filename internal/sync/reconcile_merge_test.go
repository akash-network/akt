package sync_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/store"
	syncpkg "pkg.akt.dev/akt/internal/sync"
)

// TestReconcileNowPreservesLocalOnlyMetadata is the guard on SPEC §6.4 step 4.
// A workflow run records the SDL path and hash it deployed (SPEC §6.6); the
// chain does not carry them. If reconciliation overwrote records wholesale,
// running `akt store sync` after a deploy would erase exactly the data the
// local store exists to hold.
func TestReconcileNowPreservesLocalOnlyMetadata(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:     testOwner,
		DSeq:      100,
		State:     "active",
		SDLPath:   "/home/me/d1.yaml",
		SDLHash:   "sha256:abc",
		Deposit:   "5000000uakt",
		Notes:     "production frontend",
		Tags:      []string{"prod"},
		Labels:    map[string]string{"team": "core"},
		CreatedAt: 1690000000,
	}))
	require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{
		ID:          store.LeaseID{Owner: testOwner, DSeq: 100, GSeq: 1, OSeq: 1, Provider: "akash1prov"},
		State:       "active",
		ProviderURI: "https://provider.example:8443",
		Endpoints:   []store.LeaseEndpoint{{Service: "web", ExternalPort: 80, URI: "http://app.example"}},
		CreatedAt:   1690000000,
	}))

	eng := syncpkg.New(s, []string{testOwner})

	q := &mockQuerier{
		height: 90000,
		deps: map[string][]*store.DeploymentRecord{
			testOwner: {{
				Owner:         testOwner,
				DSeq:          100,
				State:         "active",
				CreatedHeight: 89000,
				EscrowBalance: "4500000.0uakt",
				Transferred:   "500000.0uakt",
			}},
		},
		leases: map[string][]*store.LeaseRecord{
			testOwner + ":" + store.DeploymentKey(testOwner, 100): {{
				ID:    store.LeaseID{Owner: testOwner, DSeq: 100, GSeq: 1, OSeq: 1, Provider: "akash1prov"},
				State: "active",
				Price: "25.0uakt",
			}},
		},
		bids: map[string][]*store.BidRecord{},
	}

	stats, err := eng.ReconcileNow(ctx, q)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Deployments)
	assert.Equal(t, 1, stats.Leases)
	assert.Equal(t, int64(90000), stats.Height)

	dep, err := s.GetDeployment(ctx, testOwner, 100)
	require.NoError(t, err)
	require.NotNil(t, dep)

	// Chain state wins for the fields the chain owns...
	assert.Equal(t, int64(89000), dep.CreatedHeight)
	assert.Equal(t, "4500000.0uakt", dep.EscrowBalance)
	assert.Equal(t, "500000.0uakt", dep.Transferred)

	// ...and everything only the local run knew survives.
	assert.Equal(t, "/home/me/d1.yaml", dep.SDLPath)
	assert.Equal(t, "sha256:abc", dep.SDLHash)
	assert.Equal(t, "5000000uakt", dep.Deposit)
	assert.Equal(t, "production frontend", dep.Notes)
	assert.Equal(t, []string{"prod"}, dep.Tags)
	assert.Equal(t, map[string]string{"team": "core"}, dep.Labels)
	assert.Equal(t, int64(1690000000), dep.CreatedAt)
	assert.NotZero(t, dep.UpdatedAt)

	lease, err := s.GetLease(ctx, store.LeaseID{Owner: testOwner, DSeq: 100, GSeq: 1, OSeq: 1, Provider: "akash1prov"})
	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.Equal(t, "25.0uakt", lease.Price)
	assert.Equal(t, "https://provider.example:8443", lease.ProviderURI)
	assert.Len(t, lease.Endpoints, 1)
	assert.Equal(t, int64(1690000000), lease.CreatedAt)
}

// TestReconcileNowIgnoresTheBlockGap separates `akt store sync` from startup
// reconciliation: the user asked for chain state to be re-read, so a small gap
// must not turn the command into a no-op that reports success.
func TestReconcileNowIgnoresTheBlockGap(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	require.NoError(t, s.PutSyncState(ctx, &store.SyncState{LastBlockHeight: 1000, LastSyncTime: 1000}))

	eng := syncpkg.New(s, []string{testOwner})
	q := &mockQuerier{
		height: 1001, // gap of 1, far below the startup threshold
		deps: map[string][]*store.DeploymentRecord{
			testOwner: {{Owner: testOwner, DSeq: 300, State: "active"}},
		},
		leases: map[string][]*store.LeaseRecord{},
		bids:   map[string][]*store.BidRecord{},
	}

	stats, err := eng.ReconcileNow(ctx, q)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Deployments)
	assert.Equal(t, 1, stats.Accounts)

	dep, err := s.GetDeployment(ctx, testOwner, 300)
	require.NoError(t, err)
	require.NotNil(t, dep, "ReconcileNow must query the chain regardless of the block gap")

	ss, err := s.GetSyncState(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1001), ss.LastBlockHeight)
	assert.Equal(t, []string{testOwner}, ss.TrackedAccounts)
}

// TestReconcileNowStampsClosedRecords covers the closed-state arm: the chain
// reports closure as a state, never as a wall-clock time, so the local record
// gets its closed_at stamped once and keeps it on later syncs.
func TestReconcileNowStampsClosedRecords(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	eng := syncpkg.New(s, []string{testOwner})
	q := &mockQuerier{
		height: 5000,
		deps: map[string][]*store.DeploymentRecord{
			testOwner: {{Owner: testOwner, DSeq: 55, State: "closed"}},
		},
		leases: map[string][]*store.LeaseRecord{
			testOwner + ":" + store.DeploymentKey(testOwner, 55): {{
				ID:    store.LeaseID{Owner: testOwner, DSeq: 55, GSeq: 1, OSeq: 1, Provider: "akash1prov"},
				State: "closed",
			}},
		},
		bids: map[string][]*store.BidRecord{},
	}

	_, err := eng.ReconcileNow(ctx, q)
	require.NoError(t, err)

	dep, err := s.GetDeployment(ctx, testOwner, 55)
	require.NoError(t, err)
	require.NotNil(t, dep)
	assert.NotZero(t, dep.ClosedAt)
	first := dep.ClosedAt

	lease, err := s.GetLease(ctx, store.LeaseID{Owner: testOwner, DSeq: 55, GSeq: 1, OSeq: 1, Provider: "akash1prov"})
	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.NotZero(t, lease.ClosedAt)

	_, err = eng.ReconcileNow(ctx, q)
	require.NoError(t, err)

	dep, err = s.GetDeployment(ctx, testOwner, 55)
	require.NoError(t, err)
	assert.Equal(t, first, dep.ClosedAt, "closed_at must be stamped once, not moved by every sync")
}
