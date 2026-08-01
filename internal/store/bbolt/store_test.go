package bbolt

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/store"
)

func openTestStore(t *testing.T) *BoltStore {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestEmptyListsReturnNonNilSlices(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	deployments, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	require.NotNil(t, deployments)
	require.Empty(t, deployments)

	leases, err := s.ListLeases(ctx, store.LeaseFilter{})
	require.NoError(t, err)
	require.NotNil(t, leases)
	require.Empty(t, leases)

	bids, err := s.ListBids(ctx, store.BidFilter{})
	require.NoError(t, err)
	require.NotNil(t, bids)
	require.Empty(t, bids)
}

func TestDeploymentCRUD(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	dep := &store.DeploymentRecord{
		Owner:     "akash1abc",
		DSeq:      100,
		State:     "active",
		SDLHash:   "sha256:deadbeef",
		Deposit:   "5000000uakt",
		CreatedAt: 1700000000,
		UpdatedAt: 1700000000,
		Labels:    map[string]string{"env": "prod"},
		Tags:      []string{"web", "gpu"},
	}

	// Put
	err := s.PutDeployment(ctx, dep)
	require.NoError(t, err)

	// Get — verify round-trip
	got, err := s.GetDeployment(ctx, "akash1abc", 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, dep.Owner, got.Owner)
	assert.Equal(t, dep.DSeq, got.DSeq)
	assert.Equal(t, dep.State, got.State)
	assert.Equal(t, dep.SDLHash, got.SDLHash)
	assert.Equal(t, dep.Deposit, got.Deposit)
	assert.Equal(t, dep.Labels, got.Labels)
	assert.Equal(t, dep.Tags, got.Tags)

	// List with owner filter
	list, err := s.ListDeployments(ctx, store.DeploymentFilter{Owner: "akash1abc"})
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// List with state filter
	list, err = s.ListDeployments(ctx, store.DeploymentFilter{State: "active"})
	require.NoError(t, err)
	assert.Len(t, list, 1)

	list, err = s.ListDeployments(ctx, store.DeploymentFilter{State: "closed"})
	require.NoError(t, err)
	assert.Len(t, list, 0)

	// Delete
	err = s.DeleteDeployment(ctx, "akash1abc", 100)
	require.NoError(t, err)

	got, err = s.GetDeployment(ctx, "akash1abc", 100)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestLeaseCRUD(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	leaseID := store.LeaseID{
		Owner:    "akash1abc",
		DSeq:     100,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1provider",
	}
	lease := &store.LeaseRecord{
		ID:          leaseID,
		State:       "active",
		Price:       "100uakt",
		ProviderURI: "https://provider.example.com",
		CreatedAt:   1700000000,
	}

	// Put
	err := s.PutLease(ctx, lease)
	require.NoError(t, err)

	// Get by LeaseID
	got, err := s.GetLease(ctx, leaseID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, lease.ID, got.ID)
	assert.Equal(t, lease.State, got.State)
	assert.Equal(t, lease.Price, got.Price)
	assert.Equal(t, lease.ProviderURI, got.ProviderURI)

	// List with owner filter
	list, err := s.ListLeases(ctx, store.LeaseFilter{Owner: "akash1abc"})
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// List with dseq filter
	list, err = s.ListLeases(ctx, store.LeaseFilter{DSeq: 100})
	require.NoError(t, err)
	assert.Len(t, list, 1)

	list, err = s.ListLeases(ctx, store.LeaseFilter{DSeq: 999})
	require.NoError(t, err)
	assert.Len(t, list, 0)

	// Delete
	err = s.DeleteLease(ctx, leaseID)
	require.NoError(t, err)

	got, err = s.GetLease(ctx, leaseID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestBidCRUD(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	bidID := store.BidID{
		Owner:    "akash1abc",
		DSeq:     100,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1provider",
	}
	bid := &store.BidRecord{
		ID:    bidID,
		State: "open",
		Price: "50uakt",
		ProviderAttributes: map[string]string{
			"region": "us-west",
		},
		CreatedAt: 1700000000,
	}

	// Put
	err := s.PutBid(ctx, bid)
	require.NoError(t, err)

	// Get by BidID
	got, err := s.GetBid(ctx, bidID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bid.ID, got.ID)
	assert.Equal(t, bid.State, got.State)
	assert.Equal(t, bid.Price, got.Price)

	// List with owner filter
	list, err := s.ListBids(ctx, store.BidFilter{Owner: "akash1abc"})
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// List with provider filter
	list, err = s.ListBids(ctx, store.BidFilter{Provider: "akash1provider"})
	require.NoError(t, err)
	assert.Len(t, list, 1)

	list, err = s.ListBids(ctx, store.BidFilter{Provider: "akash1other"})
	require.NoError(t, err)
	assert.Len(t, list, 0)
}

func TestSyncState(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Initially nil
	got, err := s.GetSyncState(ctx)
	require.NoError(t, err)
	assert.Nil(t, got)

	// Put
	state := &store.SyncState{
		LastBlockHeight: 1000,
		LastSyncTime:    1700000000,
		TrackedAccounts: []string{"akash1abc", "akash1def"},
		SchemaVersion:   1,
	}
	err = s.PutSyncState(ctx, state)
	require.NoError(t, err)

	// Get back
	got, err = s.GetSyncState(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(1000), got.LastBlockHeight)
	assert.Equal(t, int64(1700000000), got.LastSyncTime)
	assert.Equal(t, []string{"akash1abc", "akash1def"}, got.TrackedAccounts)

	// Update
	state.LastBlockHeight = 2000
	state.LastSyncTime = 1700001000
	err = s.PutSyncState(ctx, state)
	require.NoError(t, err)

	got, err = s.GetSyncState(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(2000), got.LastBlockHeight)
	assert.Equal(t, int64(1700001000), got.LastSyncTime)
}

func TestSchemaVersion(t *testing.T) {
	s := openTestStore(t)
	assert.Equal(t, uint64(1), s.SchemaVersion())
}

func TestStats(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// 3 deployments: 2 active, 1 closed
	for i := uint64(1); i <= 2; i++ {
		require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
			Owner: "akash1abc",
			DSeq:  i,
			State: "active",
		}))
	}
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1abc",
		DSeq:  3,
		State: "closed",
	}))

	// 2 leases
	for i := uint64(1); i <= 2; i++ {
		require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{
			ID: store.LeaseID{
				Owner:    "akash1abc",
				DSeq:     i,
				GSeq:     1,
				OSeq:     1,
				Provider: "akash1prov",
			},
			State: "active",
		}))
	}

	// 1 bid
	require.NoError(t, s.PutBid(ctx, &store.BidRecord{
		ID: store.BidID{
			Owner:    "akash1abc",
			DSeq:     1,
			GSeq:     1,
			OSeq:     1,
			Provider: "akash1prov",
		},
		State: "open",
	}))

	stats, err := s.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.Deployments)
	assert.Equal(t, int64(2), stats.ActiveDeployments)
	assert.Equal(t, int64(1), stats.ClosedDeployments)
	assert.Equal(t, int64(2), stats.Leases)
	assert.Equal(t, int64(1), stats.Bids)
}

func TestConcurrentAccess(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			dep := &store.DeploymentRecord{
				Owner: fmt.Sprintf("akash1owner%d", idx),
				DSeq:  uint64(idx),
				State: "active",
			}
			err := s.PutDeployment(ctx, dep)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	list, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	assert.Len(t, list, 10)
}

func TestGetNonExistent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Deployment
	got, err := s.GetDeployment(ctx, "akash1nobody", 999)
	require.NoError(t, err)
	assert.Nil(t, got)

	// Lease
	lease, err := s.GetLease(ctx, store.LeaseID{
		Owner:    "akash1nobody",
		DSeq:     999,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1noprov",
	})
	require.NoError(t, err)
	assert.Nil(t, lease)

	// Bid
	bid, err := s.GetBid(ctx, store.BidID{
		Owner:    "akash1nobody",
		DSeq:     999,
		GSeq:     1,
		OSeq:     1,
		Provider: "akash1noprov",
	})
	require.NoError(t, err)
	assert.Nil(t, bid)
}
