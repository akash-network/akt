package sync_test

import (
	"context"
	"errors"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	dtypes "pkg.akt.dev/go/node/deployment/v1"
	mtypes "pkg.akt.dev/go/node/market/v1"

	"pkg.akt.dev/akt/internal/store"
	syncpkg "pkg.akt.dev/akt/internal/sync"
)

func TestDeploymentUpdatePreservesLocalRecordAndRefreshesChainHash(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	record := &store.DeploymentRecord{
		Owner:     testOwner,
		DSeq:      41,
		State:     "active",
		Version:   []byte("old-hash"),
		SDLPath:   "/workloads/web.yaml",
		Labels:    map[string]string{"team": "payments"},
		CreatedAt: 123,
		UpdatedAt: 124,
	}
	if err := s.PutDeployment(ctx, record); err != nil {
		t.Fatalf("seed deployment: %v", err)
	}

	engine := syncpkg.New(s, []string{testOwner})
	if err := engine.HandleEvent(ctx, &dtypes.EventDeploymentUpdated{
		ID:   dtypes.DeploymentID{Owner: testOwner, DSeq: 41},
		Hash: []byte("new-hash"),
	}); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	got, err := s.GetDeployment(ctx, testOwner, 41)
	if err != nil {
		t.Fatalf("GetDeployment: %v", err)
	}
	if got == nil {
		t.Fatal("updated deployment was not persisted")
	}
	if string(got.Version) != "new-hash" {
		t.Errorf("version = %q, want new-hash", got.Version)
	}
	if got.SDLPath != record.SDLPath || got.Labels["team"] != "payments" || got.CreatedAt != 123 {
		t.Errorf("local fields changed during chain update: %#v", got)
	}
	if got.UpdatedAt < record.UpdatedAt {
		t.Errorf("updated at = %d, want >= %d", got.UpdatedAt, record.UpdatedAt)
	}
}

func TestTerminalEventsConvergeWhenOpeningEventsWereMissed(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	engine := syncpkg.New(s, []string{testOwner})

	if err := engine.HandleEvent(ctx, &dtypes.EventDeploymentUpdated{
		ID:   dtypes.DeploymentID{Owner: testOwner, DSeq: 51},
		Hash: []byte("observed-late"),
	}); err != nil {
		t.Fatalf("late deployment update: %v", err)
	}
	deployment, err := s.GetDeployment(ctx, testOwner, 51)
	if err != nil || deployment == nil {
		t.Fatalf("late deployment update did not converge: record=%#v err=%v", deployment, err)
	}
	if deployment.State != "active" || string(deployment.Version) != "observed-late" {
		t.Errorf("late deployment record = %#v", deployment)
	}

	bidID := mtypes.BidID{
		Owner: testOwner, DSeq: 51, GSeq: 1, OSeq: 1, Provider: "akash1provider",
	}
	if err := engine.HandleEvent(ctx, &mtypes.EventBidClosed{ID: bidID}); err != nil {
		t.Fatalf("late bid close: %v", err)
	}
	bid, err := s.GetBid(ctx, store.BidID{
		Owner: bidID.Owner, DSeq: bidID.DSeq, GSeq: bidID.GSeq,
		OSeq: bidID.OSeq, Provider: bidID.Provider,
	})
	if err != nil || bid == nil || bid.State != "closed" {
		t.Fatalf("late bid close did not converge: record=%#v err=%v", bid, err)
	}

	leaseID := mtypes.LeaseID{
		Owner: testOwner, DSeq: 51, GSeq: 1, OSeq: 1, Provider: "akash1provider",
	}
	if err := engine.HandleEvent(ctx, &mtypes.EventLeaseClosed{ID: leaseID}); err != nil {
		t.Fatalf("late lease close: %v", err)
	}
	lease, err := s.GetLease(ctx, store.LeaseID{
		Owner: leaseID.Owner, DSeq: leaseID.DSeq, GSeq: leaseID.GSeq,
		OSeq: leaseID.OSeq, Provider: leaseID.Provider,
	})
	if err != nil || lease == nil || lease.State != "closed" || lease.ClosedAt == 0 {
		t.Fatalf("late lease close did not converge: record=%#v err=%v", lease, err)
	}

	if err := engine.HandleEvent(ctx, &dtypes.EventDeploymentClosed{
		ID: dtypes.DeploymentID{Owner: testOwner, DSeq: 52},
	}); err != nil {
		t.Fatalf("late deployment close: %v", err)
	}
	deployment, err = s.GetDeployment(ctx, testOwner, 52)
	if err != nil || deployment == nil || deployment.State != "closed" || deployment.ClosedAt == 0 {
		t.Fatalf("late deployment close did not converge: record=%#v err=%v", deployment, err)
	}
}

type leaseWriteFailureStore struct {
	store.Store
	err             error
	getBidWasCalled bool
}

func (s *leaseWriteFailureStore) PutLease(context.Context, *store.LeaseRecord) error {
	return s.err
}

func (s *leaseWriteFailureStore) GetBid(context.Context, store.BidID) (*store.BidRecord, error) {
	s.getBidWasCalled = true
	return nil, nil
}

func TestLeaseCreatedStopsBeforeMatchingBidWhenLeaseWriteFails(t *testing.T) {
	sentinel := errors.New("store is read-only")
	failedStore := &leaseWriteFailureStore{Store: openTestStore(t), err: sentinel}
	engine := syncpkg.New(failedStore, []string{testOwner})

	err := engine.HandleEvent(context.Background(), &mtypes.EventLeaseCreated{
		ID: mtypes.LeaseID{
			Owner: testOwner, DSeq: 61, GSeq: 1, OSeq: 1, Provider: "akash1provider",
		},
		Price: sdk.NewDecCoin("uakt", math.NewInt(9)),
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want lease write failure", err)
	}
	if failedStore.getBidWasCalled {
		t.Fatal("matching bid was read after the lease failed to persist")
	}
}
