package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/store"
	storebbolt "pkg.akt.dev/akt/internal/store/bbolt"
)

func TestUniqueDeploymentOwner(t *testing.T) {
	ctx := context.Background()
	s, err := storebbolt.Open(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	owner, err := store.UniqueDeploymentOwner(ctx, nil, 1)
	require.NoError(t, err)
	require.Empty(t, owner)
	owner, err = store.UniqueDeploymentOwner(ctx, s, 0)
	require.NoError(t, err)
	require.Empty(t, owner)
	owner, err = store.UniqueDeploymentOwner(ctx, s, 7)
	require.NoError(t, err)
	require.Empty(t, owner)

	for _, deployment := range []*store.DeploymentRecord{
		{Owner: "akash1owner-b", DSeq: 8},
		{Owner: "akash1owner-b", DSeq: 7},
	} {
		require.NoError(t, s.PutDeployment(ctx, deployment))
	}
	owner, err = store.UniqueDeploymentOwner(ctx, s, 7)
	require.NoError(t, err)
	require.Equal(t, "akash1owner-b", owner)

	require.NoError(t, s.PutDeployment(ctx, &store.DeploymentRecord{
		Owner: "akash1owner-a",
		DSeq:  7,
	}))
	owner, err = store.UniqueDeploymentOwner(ctx, s, 7)
	require.Empty(t, owner)
	require.EqualError(t, err, "deployment 7 has multiple local owners (akash1owner-a, akash1owner-b); pass an owner to `akt store sync <address>` instead of guessing")

	require.NoError(t, s.Close())
	owner, err = store.UniqueDeploymentOwner(ctx, s, 7)
	require.Empty(t, owner)
	require.ErrorContains(t, err, "list local deployments")
}

func TestStoreKeysPreserveCompleteIdentity(t *testing.T) {
	require.Equal(t, "akash1owner:7", store.DeploymentKey("akash1owner", 7))

	leaseID := store.LeaseID{
		Owner:    "akash1owner",
		DSeq:     7,
		GSeq:     2,
		OSeq:     3,
		Provider: "akash1provider",
	}
	require.Equal(t, "akash1owner:7:2:3:akash1provider", store.LeaseKey(leaseID))
	require.Equal(t, "akash1owner:7:2:3:akash1provider", store.BidKey(store.BidID(leaseID)))
}
