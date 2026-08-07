package sync

import (
	"context"
	"math/rand"
	"sort"
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

// ReconcileStats counts what a reconciliation wrote. `akt store sync` reports
// it so the user can tell "nothing was found" apart from "nothing ran".
type ReconcileStats struct {
	Accounts    int
	Deployments int
	Leases      int
	Bids        int
	Height      int64
}

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
		if _, err := e.fullReconcile(ctx, q); err != nil {
			return err
		}
	}

	return e.putSyncState(ctx, currentHeight)
}

// ReconcileNow performs the full reconciliation of SPEC §6.4 unconditionally
// and reports what it wrote. It backs `akt store sync` (SPEC §2.5): a user who
// asks for a sync is asking for chain state to be re-read, not for the block
// gap to be consulted.
func (e *Engine) ReconcileNow(ctx context.Context, q Querier) (ReconcileStats, error) {
	currentHeight, err := q.CurrentHeight(ctx)
	if err != nil {
		return ReconcileStats{}, err
	}

	stats, err := e.fullReconcile(ctx, q)
	if err != nil {
		return stats, err
	}

	stats.Height = currentHeight

	if err := e.putSyncState(ctx, currentHeight); err != nil {
		return stats, err
	}

	return stats, nil
}

// putSyncState records the height reached and the accounts covered.
func (e *Engine) putSyncState(ctx context.Context, height int64) error {
	trackedList := make([]string, 0, len(e.tracked))
	for addr := range e.tracked {
		trackedList = append(trackedList, addr)
	}
	// Map iteration order is random; sorting keeps the persisted list stable
	// across runs so an export diff reflects real changes only.
	sort.Strings(trackedList)

	return e.store.PutSyncState(ctx, &store.SyncState{
		LastBlockHeight: height,
		LastSyncTime:    time.Now().Unix(),
		TrackedAccounts: trackedList,
		SchemaVersion:   e.store.SchemaVersion(),
	})
}

// fullReconcile queries the chain for all deployments, leases, and bids
// for each tracked account and stores them locally.
//
// Chain state is merged onto the record already in the store rather than
// replacing it: the chain does not carry the local-only fields (SDL path and
// hash, labels, notes, tags, endpoints) that a workflow run recorded, so a
// straight overwrite would make `akt store sync` destroy the very data the
// workflow persistence of SPEC §6.6 exists to keep.
func (e *Engine) fullReconcile(ctx context.Context, q Querier) (ReconcileStats, error) {
	stats := ReconcileStats{Accounts: len(e.tracked)}
	now := time.Now().Unix()

	owners := make([]string, 0, len(e.tracked))
	for owner := range e.tracked {
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	for _, owner := range owners {
		deps, err := q.Deployments(ctx, owner)
		if err != nil {
			return stats, err
		}

		for _, d := range deps {
			existing, err := e.store.GetDeployment(ctx, d.Owner, d.DSeq)
			if err != nil {
				return stats, err
			}
			if err := e.store.PutDeployment(ctx, mergeDeployment(existing, d, now)); err != nil {
				return stats, err
			}
			stats.Deployments++

			leases, err := q.Leases(ctx, owner, d.DSeq)
			if err != nil {
				return stats, err
			}
			for _, l := range leases {
				existing, err := e.store.GetLease(ctx, l.ID)
				if err != nil {
					return stats, err
				}
				if err := e.store.PutLease(ctx, mergeLease(existing, l, now)); err != nil {
					return stats, err
				}
				stats.Leases++
			}

			bids, err := q.Bids(ctx, owner, d.DSeq)
			if err != nil {
				return stats, err
			}
			for _, b := range bids {
				existing, err := e.store.GetBid(ctx, b.ID)
				if err != nil {
					return stats, err
				}
				if err := e.store.PutBid(ctx, mergeBid(existing, b, now)); err != nil {
					return stats, err
				}
				stats.Bids++
			}
		}
	}

	return stats, nil
}

// mergeDeployment layers fresh chain state over the local record, keeping the
// fields the chain cannot report.
func mergeDeployment(existing, fresh *store.DeploymentRecord, now int64) *store.DeploymentRecord {
	merged := *fresh
	if merged.UpdatedAt == 0 {
		merged.UpdatedAt = now
	}

	if existing != nil {
		merged.SDLHash = existing.SDLHash
		merged.SDLPath = existing.SDLPath
		merged.Deposit = existing.Deposit
		merged.Labels = existing.Labels
		merged.Notes = existing.Notes
		merged.Tags = existing.Tags
		merged.CreatedAt = existing.CreatedAt
		merged.ClosedAt = existing.ClosedAt
	}

	if merged.CreatedAt == 0 {
		merged.CreatedAt = now
	}
	if merged.State == "closed" && merged.ClosedAt == 0 {
		merged.ClosedAt = now
	}

	return &merged
}

// mergeLease layers fresh chain state over the local lease record.
func mergeLease(existing, fresh *store.LeaseRecord, now int64) *store.LeaseRecord {
	merged := *fresh

	if existing != nil {
		merged.ProviderURI = existing.ProviderURI
		merged.Endpoints = existing.Endpoints
		merged.CreatedAt = existing.CreatedAt
		merged.ClosedAt = existing.ClosedAt
	}

	if merged.CreatedAt == 0 {
		merged.CreatedAt = now
	}
	if merged.State == "closed" && merged.ClosedAt == 0 {
		merged.ClosedAt = now
	}

	return &merged
}

// mergeBid layers fresh chain state over the local bid record.
func mergeBid(existing, fresh *store.BidRecord, now int64) *store.BidRecord {
	merged := *fresh

	if existing != nil {
		if fresh.ProviderAttributes == nil {
			merged.ProviderAttributes = existing.ProviderAttributes
			merged.ProviderAudited = existing.ProviderAudited
		}
		merged.CreatedAt = existing.CreatedAt
	}

	if merged.CreatedAt == 0 {
		merged.CreatedAt = now
	}

	return &merged
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
