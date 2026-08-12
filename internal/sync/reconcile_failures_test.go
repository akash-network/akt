package sync_test

import (
	"context"
	"errors"
	"testing"

	"pkg.akt.dev/akt/internal/store"
	syncpkg "pkg.akt.dev/akt/internal/sync"
)

type reconcileFailureQuerier struct {
	height       int64
	failureStage string
	err          error
	heightCalls  int
}

func (querier *reconcileFailureQuerier) CurrentHeight(context.Context) (int64, error) {
	querier.heightCalls++
	if querier.failureStage == "height" {
		return 0, querier.err
	}
	return querier.height, nil
}

func (querier *reconcileFailureQuerier) Deployments(
	_ context.Context,
	owner string,
) ([]*store.DeploymentRecord, error) {
	if querier.failureStage == "deployments" {
		return nil, querier.err
	}
	return []*store.DeploymentRecord{{Owner: owner, DSeq: 71, State: "active"}}, nil
}

func (querier *reconcileFailureQuerier) Leases(
	_ context.Context,
	owner string,
	dseq uint64,
) ([]*store.LeaseRecord, error) {
	if querier.failureStage == "leases" {
		return nil, querier.err
	}
	return []*store.LeaseRecord{{
		ID: store.LeaseID{
			Owner: owner, DSeq: dseq, GSeq: 1, OSeq: 1, Provider: "akash1provider",
		},
		State: "active",
	}}, nil
}

func (querier *reconcileFailureQuerier) Bids(
	_ context.Context,
	owner string,
	dseq uint64,
) ([]*store.BidRecord, error) {
	if querier.failureStage == "bids" {
		return nil, querier.err
	}
	return []*store.BidRecord{{
		ID: store.BidID{
			Owner: owner, DSeq: dseq, GSeq: 1, OSeq: 1, Provider: "akash1provider",
		},
		State: "open",
	}}, nil
}

func TestReconcileNowNeverAdvancesCheckpointAfterQueryFailure(t *testing.T) {
	sentinel := errors.New("chain query failed")
	for _, stage := range []string{"height", "deployments", "leases", "bids"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)
			if err := s.PutSyncState(ctx, &store.SyncState{LastBlockHeight: 70}); err != nil {
				t.Fatalf("seed sync state: %v", err)
			}

			engine := syncpkg.New(s, []string{testOwner})
			stats, err := engine.ReconcileNow(ctx, &reconcileFailureQuerier{
				height: 71, failureStage: stage, err: sentinel,
			})
			if !errors.Is(err, sentinel) {
				t.Fatalf("error = %v, want query failure", err)
			}
			if stats.Height != 0 {
				t.Errorf("reported height = %d, want 0 for an incomplete reconciliation", stats.Height)
			}

			state, stateErr := s.GetSyncState(ctx)
			if stateErr != nil {
				t.Fatalf("GetSyncState: %v", stateErr)
			}
			if state == nil || state.LastBlockHeight != 70 {
				t.Fatalf("checkpoint = %#v, want original height 70", state)
			}
		})
	}
}

type syncStateFailureStore struct {
	store.Store
	getErr error
	putErr error
}

func (s *syncStateFailureStore) GetSyncState(ctx context.Context) (*store.SyncState, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.Store.GetSyncState(ctx)
}

func (s *syncStateFailureStore) PutSyncState(context.Context, *store.SyncState) error {
	return s.putErr
}

func TestReconcileStoreFailuresRemainVisible(t *testing.T) {
	t.Run("read checkpoint", func(t *testing.T) {
		sentinel := errors.New("checkpoint unreadable")
		querier := &reconcileFailureQuerier{height: 81}
		engine := syncpkg.New(&syncStateFailureStore{
			Store: openTestStore(t), getErr: sentinel,
		}, []string{testOwner})

		err := engine.Reconcile(context.Background(), querier)
		if !errors.Is(err, sentinel) {
			t.Fatalf("error = %v, want checkpoint read failure", err)
		}
		if querier.heightCalls != 0 {
			t.Fatalf("height calls = %d, want none after checkpoint read failure", querier.heightCalls)
		}
	})

	t.Run("write checkpoint", func(t *testing.T) {
		sentinel := errors.New("checkpoint is read-only")
		engine := syncpkg.New(&syncStateFailureStore{
			Store: openTestStore(t), putErr: sentinel,
		}, []string{testOwner})

		stats, err := engine.ReconcileNow(context.Background(), &reconcileFailureQuerier{height: 81})
		if !errors.Is(err, sentinel) {
			t.Fatalf("error = %v, want checkpoint write failure", err)
		}
		if stats.Height != 81 || stats.Deployments != 1 || stats.Leases != 1 || stats.Bids != 1 {
			t.Fatalf("completed stats = %#v, want chain writes reported despite checkpoint failure", stats)
		}
	})
}
