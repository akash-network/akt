package bbolt

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"

	aktctx "pkg.akt.dev/akt/internal/context"
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

func TestRecordVersionsStartAtOneAndAdvancePerKey(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	deployment := &store.DeploymentRecord{Owner: "akash1abc", DSeq: 1, State: "active"}
	lease := &store.LeaseRecord{ID: store.LeaseID{
		Owner: "akash1abc", DSeq: 1, Provider: "akash1provider",
	}, State: "active"}
	bid := &store.BidRecord{ID: store.BidID{
		Owner: "akash1abc", DSeq: 1, Provider: "akash1provider",
	}, State: "open"}

	require.NoError(t, s.PutDeployment(ctx, deployment))
	require.NoError(t, s.PutLease(ctx, lease))
	require.NoError(t, s.PutBid(ctx, bid))

	storedDeployment, err := s.GetDeployment(ctx, deployment.Owner, deployment.DSeq)
	require.NoError(t, err)
	storedLease, err := s.GetLease(ctx, lease.ID)
	require.NoError(t, err)
	storedBid, err := s.GetBid(ctx, bid.ID)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), storedDeployment.RecordVersion)
	assert.Equal(t, uint64(1), storedLease.RecordVersion)
	assert.Equal(t, uint64(1), storedBid.RecordVersion)

	// The caller does not need to carry the stored revision back into a later
	// write; version advancement is atomic inside the bucket transaction.
	require.NoError(t, s.PutDeployment(ctx, deployment))
	require.NoError(t, s.PutLease(ctx, lease))
	require.NoError(t, s.PutBid(ctx, bid))

	storedDeployment, err = s.GetDeployment(ctx, deployment.Owner, deployment.DSeq)
	require.NoError(t, err)
	storedLease, err = s.GetLease(ctx, lease.ID)
	require.NoError(t, err)
	storedBid, err = s.GetBid(ctx, bid.ID)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), storedDeployment.RecordVersion)
	assert.Equal(t, uint64(2), storedLease.RecordVersion)
	assert.Equal(t, uint64(2), storedBid.RecordVersion)
}

func TestRecordVersionPreservesNewerImportedRevision(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1abc", DSeq: 1, RecordVersion: 2,
	}))
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1abc", DSeq: 1, RecordVersion: 10,
	}))

	record, err := s.GetDeployment(ctx, "akash1abc", 1)
	require.NoError(t, err)
	assert.Equal(t, uint64(10), record.RecordVersion)

	// An older incoming revision is still a local write, so it advances from
	// the stored value rather than moving the counter backwards.
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1abc", DSeq: 1, RecordVersion: 3,
	}))
	record, err = s.GetDeployment(ctx, "akash1abc", 1)
	require.NoError(t, err)
	assert.Equal(t, uint64(11), record.RecordVersion)
}

func TestLeaseAndBidRevisionOverflowPreservesStoredRecords(t *testing.T) {
	ctx := context.Background()
	id := store.LeaseID{
		Owner: "akash1owner", DSeq: 7, GSeq: 1, OSeq: 1, Provider: "akash1provider",
	}

	t.Run("lease", func(t *testing.T) {
		s := openTestStore(t)
		require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{
			ID: id, State: "active", RecordVersion: math.MaxUint64,
		}))

		err := s.PutLease(ctx, &store.LeaseRecord{
			ID: id, State: "closed", RecordVersion: math.MaxUint64,
		})
		require.ErrorContains(t, err, "version lease: record revision cannot advance")

		stored, err := s.GetLease(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "active", stored.State)
		require.Equal(t, uint64(math.MaxUint64), stored.RecordVersion)
	})

	t.Run("bid", func(t *testing.T) {
		s := openTestStore(t)
		bidID := store.BidID(id)
		require.NoError(t, s.PutBid(ctx, &store.BidRecord{
			ID: bidID, State: "open", RecordVersion: math.MaxUint64,
		}))

		err := s.PutBid(ctx, &store.BidRecord{
			ID: bidID, State: "closed", RecordVersion: math.MaxUint64,
		})
		require.ErrorContains(t, err, "version bid: record revision cannot advance")

		stored, err := s.GetBid(ctx, bidID)
		require.NoError(t, err)
		require.Equal(t, "open", stored.State)
		require.Equal(t, uint64(math.MaxUint64), stored.RecordVersion)
	})
}

func TestPutRejectsCorruptStoredRevisionWithoutOverwritingIt(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	key := store.DeploymentKey("akash1owner", 7)
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketDeployments).Put([]byte(key), []byte("not-json"))
	}))

	err := s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1owner", DSeq: 7, State: "active",
	})
	require.ErrorContains(t, err, "version deployment: decode stored record revision")

	require.NoError(t, s.db.View(func(tx *bolt.Tx) error {
		require.Equal(t, []byte("not-json"), tx.Bucket(bucketDeployments).Get([]byte(key)))
		return nil
	}))
}

func TestMarkDeploymentClosedUpdatesMatchingRecordsAtomically(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	const closedAt int64 = 1_700_001_234

	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1owner",
		DSeq:  7,
		State: "active",
	}))
	activeLeaseID := store.LeaseID{
		Owner: "akash1owner", DSeq: 7, GSeq: 1, OSeq: 1, Provider: "akash1provider-a",
	}
	alreadyClosedID := store.LeaseID{
		Owner: "akash1owner", DSeq: 7, GSeq: 1, OSeq: 1, Provider: "akash1provider-b",
	}
	unrelatedID := store.LeaseID{
		Owner: "akash1owner", DSeq: 8, GSeq: 1, OSeq: 1, Provider: "akash1provider-c",
	}
	require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{ID: activeLeaseID, State: "active"}))
	require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{
		ID: alreadyClosedID, State: "closed", ClosedAt: 123,
	}))
	require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{ID: unrelatedID, State: "active"}))

	require.NoError(t, s.MarkDeploymentClosed(ctx, "akash1owner", 7, closedAt))

	deployment, err := s.GetDeployment(ctx, "akash1owner", 7)
	require.NoError(t, err)
	require.Equal(t, "closed", deployment.State)
	require.Equal(t, closedAt, deployment.ClosedAt)
	require.Equal(t, closedAt, deployment.UpdatedAt)
	require.Equal(t, uint64(2), deployment.RecordVersion)

	activeLease, err := s.GetLease(ctx, activeLeaseID)
	require.NoError(t, err)
	require.Equal(t, "closed", activeLease.State)
	require.Equal(t, closedAt, activeLease.ClosedAt)
	require.Equal(t, uint64(2), activeLease.RecordVersion)

	alreadyClosed, err := s.GetLease(ctx, alreadyClosedID)
	require.NoError(t, err)
	require.Equal(t, "closed", alreadyClosed.State)
	require.Equal(t, int64(123), alreadyClosed.ClosedAt)
	require.Equal(t, uint64(2), alreadyClosed.RecordVersion)

	unrelated, err := s.GetLease(ctx, unrelatedID)
	require.NoError(t, err)
	require.Equal(t, "active", unrelated.State)
	require.Equal(t, uint64(1), unrelated.RecordVersion)
}

func TestMarkDeploymentClosedRollsBackWhenLeaseCannotBeDecoded(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1owner",
		DSeq:  7,
		State: "active",
	}))
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketLeases).Put([]byte("corrupt"), []byte("not-json"))
	}))

	err := s.MarkDeploymentClosed(ctx, "akash1owner", 7, 99)
	require.ErrorContains(t, err, "unmarshal lease")

	deployment, err := s.GetDeployment(ctx, "akash1owner", 7)
	require.NoError(t, err)
	require.Equal(t, "active", deployment.State)
	require.Zero(t, deployment.ClosedAt)
	require.Equal(t, uint64(1), deployment.RecordVersion)
}

func TestMarkDeploymentClosedRejectsCorruptDeployment(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	key := store.DeploymentKey("akash1owner", 7)
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketDeployments).Put([]byte(key), []byte("not-json"))
	}))

	err := s.MarkDeploymentClosed(ctx, "akash1owner", 7, 99)
	require.ErrorContains(t, err, "unmarshal deployment")

	require.NoError(t, s.db.View(func(tx *bolt.Tx) error {
		require.Equal(t, []byte("not-json"), tx.Bucket(bucketDeployments).Get([]byte(key)))
		return nil
	}))
}

func TestMarkDeploymentClosedRejectsDeploymentRevisionOverflow(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:         "akash1owner",
		DSeq:          7,
		State:         "active",
		RecordVersion: math.MaxUint64,
	}))

	err := s.MarkDeploymentClosed(ctx, "akash1owner", 7, 99)
	require.ErrorContains(t, err, "store deployment")
	require.ErrorContains(t, err, "record revision cannot advance")

	deployment, err := s.GetDeployment(ctx, "akash1owner", 7)
	require.NoError(t, err)
	require.Equal(t, "active", deployment.State)
	require.Zero(t, deployment.ClosedAt)
	require.Equal(t, uint64(math.MaxUint64), deployment.RecordVersion)
}

func TestMarkDeploymentClosedRollsBackWhenLeaseRevisionCannotAdvance(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1owner",
		DSeq:  7,
		State: "active",
	}))
	leaseID := store.LeaseID{
		Owner: "akash1owner", DSeq: 7, GSeq: 1, OSeq: 1, Provider: "akash1provider",
	}
	require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{
		ID: leaseID, State: "active", RecordVersion: math.MaxUint64,
	}))

	err := s.MarkDeploymentClosed(ctx, "akash1owner", 7, 99)
	require.ErrorContains(t, err, "store lease")
	require.ErrorContains(t, err, "record revision cannot advance")

	deployment, err := s.GetDeployment(ctx, "akash1owner", 7)
	require.NoError(t, err)
	require.Equal(t, "active", deployment.State)
	require.Zero(t, deployment.ClosedAt)
	require.Equal(t, uint64(1), deployment.RecordVersion)

	lease, err := s.GetLease(ctx, leaseID)
	require.NoError(t, err)
	require.Equal(t, "active", lease.State)
	require.Zero(t, lease.ClosedAt)
	require.Equal(t, uint64(math.MaxUint64), lease.RecordVersion)
}

func TestMarkDeploymentClosedCreatesMissingRecordAndPreservesClosureTime(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	require.NoError(t, s.MarkDeploymentClosed(ctx, "akash1owner", 7, 100))
	created, err := s.GetDeployment(ctx, "akash1owner", 7)
	require.NoError(t, err)
	require.Equal(t, "closed", created.State)
	require.Equal(t, int64(100), created.CreatedAt)
	require.Equal(t, int64(100), created.UpdatedAt)
	require.Equal(t, int64(100), created.ClosedAt)
	require.Equal(t, uint64(1), created.RecordVersion)

	require.NoError(t, s.MarkDeploymentClosed(ctx, "akash1owner", 7, 200))
	closedAgain, err := s.GetDeployment(ctx, "akash1owner", 7)
	require.NoError(t, err)
	require.Equal(t, int64(100), closedAgain.CreatedAt)
	require.Equal(t, int64(100), closedAgain.ClosedAt)
	require.Equal(t, int64(200), closedAgain.UpdatedAt)
	require.Equal(t, uint64(2), closedAgain.RecordVersion)
}

func TestMarkDeploymentClosedRequiresIdentity(t *testing.T) {
	s := openTestStore(t)

	require.Error(t, s.MarkDeploymentClosed(context.Background(), "", 1, 1))
	require.Error(t, s.MarkDeploymentClosed(context.Background(), "akash1owner", 0, 1))
}

func TestPutRejectsNilRecords(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	require.ErrorContains(t, s.PutDeployment(ctx, nil), "deployment record is nil")
	require.ErrorContains(t, s.PutLease(ctx, nil), "lease record is nil")
	require.ErrorContains(t, s.PutBid(ctx, nil), "bid record is nil")
	require.ErrorContains(t, s.PutSyncState(ctx, nil), "sync state is nil")
}

func TestPutHelpersPropagateMarshalFailuresWithoutMutation(t *testing.T) {
	marshalErr := errors.New("marshal failed")
	failMarshal := func(any) ([]byte, error) { return nil, marshalErr }
	leaseID := store.LeaseID{
		Owner: "akash1owner", DSeq: 7, GSeq: 1, OSeq: 1, Provider: "akash1provider",
	}

	for _, tc := range []struct {
		name   string
		bucket []byte
		key    []byte
		put    func(*bolt.Tx) error
	}{
		{
			name:   "deployment",
			bucket: bucketDeployments,
			key:    []byte(store.DeploymentKey("akash1owner", 7)),
			put: func(tx *bolt.Tx) error {
				return putDeploymentTxWithMarshal(tx, &store.DeploymentRecord{
					Owner: "akash1owner", DSeq: 7, State: "active",
				}, failMarshal)
			},
		},
		{
			name:   "lease",
			bucket: bucketLeases,
			key:    []byte(store.LeaseKey(leaseID)),
			put: func(tx *bolt.Tx) error {
				return putLeaseTxWithMarshal(tx, &store.LeaseRecord{
					ID: leaseID, State: "active",
				}, failMarshal)
			},
		},
		{
			name:   "bid",
			bucket: bucketBids,
			key:    []byte(store.BidKey(store.BidID(leaseID))),
			put: func(tx *bolt.Tx) error {
				return putBidTxWithMarshal(tx, &store.BidRecord{
					ID: store.BidID(leaseID), State: "open",
				}, failMarshal)
			},
		},
		{
			name:   "sync state",
			bucket: bucketSync,
			key:    keySyncState,
			put: func(tx *bolt.Tx) error {
				return putSyncStateTxWithMarshal(tx, &store.SyncState{
					LastBlockHeight: 10,
				}, failMarshal)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			err := s.db.Update(tc.put)
			require.ErrorContains(t, err, "marshal "+tc.name)
			require.ErrorIs(t, err, marshalErr)

			require.NoError(t, s.db.View(func(tx *bolt.Tx) error {
				require.Nil(t, tx.Bucket(tc.bucket).Get(tc.key))
				return nil
			}))
		})
	}
}

func TestCorruptRowsFailPointReadsAndDoNotHideHealthyScans(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	deploymentKey := store.DeploymentKey("akash1owner", 1)
	leaseID := store.LeaseID{Owner: "akash1owner", DSeq: 1, GSeq: 1, OSeq: 1, Provider: "akash1provider"}
	bidID := store.BidID(leaseID)

	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		for bucket, key := range map[string]string{
			string(bucketDeployments): deploymentKey,
			string(bucketLeases):      store.LeaseKey(leaseID),
			string(bucketBids):        store.BidKey(bidID),
		} {
			if err := tx.Bucket([]byte(bucket)).Put([]byte(key), []byte("not-json")); err != nil {
				return err
			}
		}
		return tx.Bucket(bucketSync).Put(keySyncState, []byte("not-json"))
	}))

	_, err := s.GetDeployment(ctx, "akash1owner", 1)
	require.ErrorContains(t, err, "unmarshal deployment")
	_, err = s.GetLease(ctx, leaseID)
	require.ErrorContains(t, err, "unmarshal lease")
	_, err = s.GetBid(ctx, bidID)
	require.ErrorContains(t, err, "unmarshal bid")
	_, err = s.GetSyncState(ctx)
	require.ErrorContains(t, err, "unmarshal sync state")

	deployments, err := s.ListDeployments(ctx, store.DeploymentFilter{})
	require.NoError(t, err)
	require.Empty(t, deployments)
	leases, err := s.ListLeases(ctx, store.LeaseFilter{})
	require.NoError(t, err)
	require.Empty(t, leases)
	bids, err := s.ListBids(ctx, store.BidFilter{})
	require.NoError(t, err)
	require.Empty(t, bids)

	stats, err := s.Stats(ctx)
	require.NoError(t, err)
	require.Zero(t, stats.Deployments)
	require.Zero(t, stats.Leases)
	require.Zero(t, stats.Bids)
}

func TestFiltersRequireEveryRequestedField(t *testing.T) {
	deployment := &store.DeploymentRecord{
		Owner: "akash1owner",
		DSeq:  1,
		State: "active",
		Tags:  []string{"gpu", "west"},
		Labels: map[string]string{
			"environment": "production",
		},
	}
	for _, tc := range []struct {
		name   string
		filter store.DeploymentFilter
		want   bool
	}{
		{name: "all", want: true},
		{name: "exact", filter: store.DeploymentFilter{Owner: "akash1owner", State: "active", Tags: []string{"gpu", "west"}, Label: "environment=production"}, want: true},
		{name: "owner", filter: store.DeploymentFilter{Owner: "akash1other"}},
		{name: "state", filter: store.DeploymentFilter{State: "closed"}},
		{name: "tag", filter: store.DeploymentFilter{Tags: []string{"gpu", "east"}}},
		{name: "nil labels", filter: store.DeploymentFilter{Label: "missing=value"}},
		{name: "label value", filter: store.DeploymentFilter{Label: "environment=staging"}},
	} {
		t.Run("deployment/"+tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, matchDeployment(deployment, tc.filter))
		})
	}

	withoutLabels := *deployment
	withoutLabels.Labels = nil
	require.False(t, matchDeployment(&withoutLabels, store.DeploymentFilter{Label: "environment=production"}))

	lease := &store.LeaseRecord{ID: store.LeaseID{
		Owner: "akash1owner", DSeq: 1, Provider: "akash1provider",
	}, State: "active"}
	require.True(t, matchLease(lease, store.LeaseFilter{Owner: "akash1owner", DSeq: 1, Provider: "akash1provider", State: "active"}))
	require.False(t, matchLease(lease, store.LeaseFilter{Owner: "akash1other"}))
	require.False(t, matchLease(lease, store.LeaseFilter{DSeq: 2}))
	require.False(t, matchLease(lease, store.LeaseFilter{Provider: "akash1other"}))
	require.False(t, matchLease(lease, store.LeaseFilter{State: "closed"}))

	bid := &store.BidRecord{ID: store.BidID(lease.ID), State: "open"}
	require.True(t, matchBid(bid, store.BidFilter{Owner: "akash1owner", DSeq: 1, Provider: "akash1provider", State: "open"}))
	require.False(t, matchBid(bid, store.BidFilter{Owner: "akash1other"}))
	require.False(t, matchBid(bid, store.BidFilter{DSeq: 2}))
	require.False(t, matchBid(bid, store.BidFilter{Provider: "akash1other"}))
	require.False(t, matchBid(bid, store.BidFilter{State: "closed"}))
}

func TestOpenContextUsesCanonicalPath(t *testing.T) {
	root := t.TempDir()
	s, err := OpenContext(context.Background(), root, "sandbox")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	_, err = os.Stat(aktctx.StoreDBPath(root, "sandbox"))
	require.NoError(t, err)
	require.Equal(t, currentSchemaVersion, s.SchemaVersion())
}

func TestOpenReportsInvalidParentPath(t *testing.T) {
	notDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(notDirectory, []byte("file"), 0o600))

	s, err := Open(filepath.Join(notDirectory, "store.db"))
	require.ErrorContains(t, err, "open bbolt database")
	require.Nil(t, s)
}

func TestStoreOperationsReportClosedDatabase(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	require.NoError(t, s.Close())

	require.Error(t, s.PutDeployment(ctx, &store.DeploymentRecord{Owner: "akash1owner", DSeq: 1}))
	_, err := s.GetDeployment(ctx, "akash1owner", 1)
	require.Error(t, err)
	_, err = s.ListDeployments(ctx, store.DeploymentFilter{})
	require.Error(t, err)
	require.Error(t, s.DeleteDeployment(ctx, "akash1owner", 1))

	require.Error(t, s.PutLease(ctx, &store.LeaseRecord{}))
	_, err = s.GetLease(ctx, store.LeaseID{})
	require.Error(t, err)
	_, err = s.ListLeases(ctx, store.LeaseFilter{})
	require.Error(t, err)
	require.Error(t, s.DeleteLease(ctx, store.LeaseID{}))

	require.Error(t, s.PutBid(ctx, &store.BidRecord{}))
	_, err = s.GetBid(ctx, store.BidID{})
	require.Error(t, err)
	_, err = s.ListBids(ctx, store.BidFilter{})
	require.Error(t, err)

	require.Error(t, s.PutSyncState(ctx, &store.SyncState{}))
	_, err = s.GetSyncState(ctx)
	require.Error(t, err)
	_, err = s.Stats(ctx)
	require.Error(t, err)
}

func TestSchemaVersionReturnsZeroForCorruptMetadata(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put(keySchemaVersion, []byte{1})
	}))

	require.Zero(t, s.SchemaVersion())
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

	// 4 leases across every known state plus one future state.
	leaseStates := []string{"active", "closed", "insufficient_funds", "future"}
	for i, state := range leaseStates {
		require.NoError(t, s.PutLease(ctx, &store.LeaseRecord{
			ID: store.LeaseID{
				Owner:    "akash1abc",
				DSeq:     uint64(i + 1),
				GSeq:     1,
				OSeq:     1,
				Provider: "akash1prov",
			},
			State: state,
		}))
	}

	// 5 bids across every known state plus one future state.
	bidStates := []string{"open", "matched", "lost", "closed", "future"}
	for i, state := range bidStates {
		require.NoError(t, s.PutBid(ctx, &store.BidRecord{
			ID: store.BidID{
				Owner:    "akash1abc",
				DSeq:     uint64(i + 1),
				GSeq:     1,
				OSeq:     1,
				Provider: "akash1prov",
			},
			State: state,
		}))
	}

	stats, err := s.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.Deployments)
	assert.Equal(t, int64(2), stats.ActiveDeployments)
	assert.Equal(t, int64(1), stats.ClosedDeployments)
	assert.Equal(t, int64(4), stats.Leases)
	assert.Equal(t, int64(1), stats.ActiveLeases)
	assert.Equal(t, int64(1), stats.ClosedLeases)
	assert.Equal(t, int64(1), stats.InsufficientFundsLeases)
	assert.Equal(t, int64(5), stats.Bids)
	assert.Equal(t, int64(1), stats.OpenBids)
	assert.Equal(t, int64(1), stats.MatchedBids)
	assert.Equal(t, int64(1), stats.LostBids)
	assert.Equal(t, int64(1), stats.ClosedBids)
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
