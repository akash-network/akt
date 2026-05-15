package sync

import (
	"context"
	"math/rand"
	"time"

	"pkg.akt.dev/akt/internal/store"
)

// Querier abstracts the chain queries needed for reconciliation.
// Implementations typically wrap the chain-sdk query clients.
type Querier interface {
	CurrentHeight(ctx context.Context) (int64, error)
	Deployments(ctx context.Context, owner string) ([]*store.DeploymentRecord, error)
	Leases(ctx context.Context, owner string, dseq uint64) ([]*store.LeaseRecord, error)
	Bids(ctx context.Context, owner string, dseq uint64) ([]*store.BidRecord, error)
}

// gapThreshold is the block gap above which a full reconciliation is triggered
// instead of an incremental sync.
const gapThreshold = 1000

// Reconcile performs startup reconciliation per SPEC §6.4.
// On first launch (no SyncState), it does a full reconciliation.
// On subsequent launches, it checks the block gap and either does
// a full reconciliation (gap > 1000) or updates the sync state.
func (e *Engine) Reconcile(ctx context.Context, q Querier) error {
	ss, err := e.store.GetSyncState(ctx)
	if err != nil {
		return err
	}

	currentHeight, err := q.CurrentHeight(ctx)
	if err != nil {
		return err
	}

	if ss == nil || currentHeight-ss.LastBlockHeight > gapThreshold {
		// Full reconciliation: first launch or large gap.
		if err := e.fullReconcile(ctx, q); err != nil {
			return err
		}
	}

	// Update sync state with current height.
	trackedList := make([]string, 0, len(e.tracked))
	for addr := range e.tracked {
		trackedList = append(trackedList, addr)
	}

	return e.store.PutSyncState(ctx, &store.SyncState{
		LastBlockHeight: currentHeight,
		LastSyncTime:    time.Now().Unix(),
		TrackedAccounts: trackedList,
		SchemaVersion:   e.store.SchemaVersion(),
	})
}

// fullReconcile queries the chain for all deployments, leases, and bids
// for each tracked account and stores them locally.
func (e *Engine) fullReconcile(ctx context.Context, q Querier) error {
	for owner := range e.tracked {
		deps, err := q.Deployments(ctx, owner)
		if err != nil {
			return err
		}

		for _, d := range deps {
			if err := e.store.PutDeployment(ctx, d); err != nil {
				return err
			}

			leases, err := q.Leases(ctx, owner, d.DSeq)
			if err != nil {
				return err
			}
			for _, l := range leases {
				if err := e.store.PutLease(ctx, l); err != nil {
					return err
				}
			}

			bids, err := q.Bids(ctx, owner, d.DSeq)
			if err != nil {
				return err
			}
			for _, b := range bids {
				if err := e.store.PutBid(ctx, b); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// BackoffDelay returns the reconnection delay for the given attempt per SPEC §6.5.
// Uses exponential backoff with jitter: 1s, 2s, 4s, 8s, 16s, 32s, 60s cap.
func BackoffDelay(attempt int) time.Duration {
	base := time.Second << uint(attempt)
	if base > 60*time.Second {
		base = 60 * time.Second
	}

	// Jitter: random value in [0, 0.5 * base).
	jitter := time.Duration(rand.Int63n(int64(base) / 2))

	return base + jitter
}
