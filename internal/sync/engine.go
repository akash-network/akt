// Package sync provides a sync engine that maps chain events to local store
// CRUD operations. The engine does not run background goroutines — callers
// invoke HandleEvent for each event received from the pubsub bus.
package sync

import (
	"context"
	"time"

	dtypes "pkg.akt.dev/go/node/deployment/v1"
	mtypes "pkg.akt.dev/go/node/market/v1"

	"pkg.akt.dev/akt/internal/store"
)

// Engine processes chain events and persists them to the local store.
type Engine struct {
	store   store.Store
	tracked map[string]bool // set of tracked owner addresses
}

// New creates a sync engine that processes events for the given tracked accounts.
func New(s store.Store, trackedAccounts []string) *Engine {
	tracked := make(map[string]bool, len(trackedAccounts))
	for _, addr := range trackedAccounts {
		tracked[addr] = true
	}
	return &Engine{
		store:   s,
		tracked: tracked,
	}
}

// HandleEvent processes a single chain event via type-switch. Events for
// owners not in the tracked set are silently ignored. Unknown event types
// are also ignored (no error).
func (e *Engine) HandleEvent(ctx context.Context, ev interface{}) error {
	switch evt := ev.(type) {
	case *dtypes.EventDeploymentCreated:
		return e.handleDeploymentCreated(ctx, evt)
	case *dtypes.EventDeploymentUpdated:
		return e.handleDeploymentUpdated(ctx, evt)
	case *dtypes.EventDeploymentClosed:
		return e.handleDeploymentClosed(ctx, evt)
	case *mtypes.EventBidCreated:
		return e.handleBidCreated(ctx, evt)
	case *mtypes.EventBidClosed:
		return e.handleBidClosed(ctx, evt)
	case *mtypes.EventLeaseCreated:
		return e.handleLeaseCreated(ctx, evt)
	case *mtypes.EventLeaseClosed:
		return e.handleLeaseClosed(ctx, evt)
	default:
		return nil
	}
}

func (e *Engine) isTracked(owner string) bool {
	return e.tracked[owner]
}

func (e *Engine) handleDeploymentCreated(ctx context.Context, evt *dtypes.EventDeploymentCreated) error {
	if !e.isTracked(evt.ID.Owner) {
		return nil
	}
	now := time.Now().Unix()
	return e.store.PutDeployment(ctx, &store.DeploymentRecord{
		Owner:     evt.ID.Owner,
		DSeq:      evt.ID.DSeq,
		State:     "active",
		Version:   evt.Hash,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (e *Engine) handleDeploymentUpdated(ctx context.Context, evt *dtypes.EventDeploymentUpdated) error {
	if !e.isTracked(evt.ID.Owner) {
		return nil
	}
	rec, err := e.store.GetDeployment(ctx, evt.ID.Owner, evt.ID.DSeq)
	if err != nil {
		return err
	}
	if rec == nil {
		// Deployment not in store; create it.
		now := time.Now().Unix()
		rec = &store.DeploymentRecord{
			Owner:     evt.ID.Owner,
			DSeq:      evt.ID.DSeq,
			State:     "active",
			CreatedAt: now,
		}
	}
	rec.Version = evt.Hash
	rec.UpdatedAt = time.Now().Unix()
	return e.store.PutDeployment(ctx, rec)
}

func (e *Engine) handleDeploymentClosed(ctx context.Context, evt *dtypes.EventDeploymentClosed) error {
	if !e.isTracked(evt.ID.Owner) {
		return nil
	}
	rec, err := e.store.GetDeployment(ctx, evt.ID.Owner, evt.ID.DSeq)
	if err != nil {
		return err
	}
	if rec == nil {
		rec = &store.DeploymentRecord{
			Owner: evt.ID.Owner,
			DSeq:  evt.ID.DSeq,
		}
	}
	now := time.Now().Unix()
	rec.State = "closed"
	rec.ClosedAt = now
	rec.UpdatedAt = now
	return e.store.PutDeployment(ctx, rec)
}

func (e *Engine) handleBidCreated(ctx context.Context, evt *mtypes.EventBidCreated) error {
	if !e.isTracked(evt.ID.Owner) {
		return nil
	}
	return e.store.PutBid(ctx, &store.BidRecord{
		ID: store.BidID{
			Owner:    evt.ID.Owner,
			DSeq:     evt.ID.DSeq,
			GSeq:     evt.ID.GSeq,
			OSeq:     evt.ID.OSeq,
			Provider: evt.ID.Provider,
		},
		State:     "open",
		Price:     evt.Price.String(),
		CreatedAt: time.Now().Unix(),
	})
}

func (e *Engine) handleBidClosed(ctx context.Context, evt *mtypes.EventBidClosed) error {
	if !e.isTracked(evt.ID.Owner) {
		return nil
	}
	bidID := store.BidID{
		Owner:    evt.ID.Owner,
		DSeq:     evt.ID.DSeq,
		GSeq:     evt.ID.GSeq,
		OSeq:     evt.ID.OSeq,
		Provider: evt.ID.Provider,
	}
	rec, err := e.store.GetBid(ctx, bidID)
	if err != nil {
		return err
	}
	if rec == nil {
		rec = &store.BidRecord{ID: bidID}
	}
	rec.State = "closed"
	return e.store.PutBid(ctx, rec)
}

func (e *Engine) handleLeaseCreated(ctx context.Context, evt *mtypes.EventLeaseCreated) error {
	if !e.isTracked(evt.ID.Owner) {
		return nil
	}

	// Store the lease.
	err := e.store.PutLease(ctx, &store.LeaseRecord{
		ID: store.LeaseID{
			Owner:    evt.ID.Owner,
			DSeq:     evt.ID.DSeq,
			GSeq:     evt.ID.GSeq,
			OSeq:     evt.ID.OSeq,
			Provider: evt.ID.Provider,
		},
		State:     "active",
		Price:     evt.Price.String(),
		CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		return err
	}

	// Update matching bid to "matched".
	bidID := store.BidID{
		Owner:    evt.ID.Owner,
		DSeq:     evt.ID.DSeq,
		GSeq:     evt.ID.GSeq,
		OSeq:     evt.ID.OSeq,
		Provider: evt.ID.Provider,
	}
	bid, err := e.store.GetBid(ctx, bidID)
	if err != nil {
		return err
	}
	if bid != nil {
		bid.State = "matched"
		return e.store.PutBid(ctx, bid)
	}
	return nil
}

func (e *Engine) handleLeaseClosed(ctx context.Context, evt *mtypes.EventLeaseClosed) error {
	if !e.isTracked(evt.ID.Owner) {
		return nil
	}
	leaseID := store.LeaseID{
		Owner:    evt.ID.Owner,
		DSeq:     evt.ID.DSeq,
		GSeq:     evt.ID.GSeq,
		OSeq:     evt.ID.OSeq,
		Provider: evt.ID.Provider,
	}
	rec, err := e.store.GetLease(ctx, leaseID)
	if err != nil {
		return err
	}
	if rec == nil {
		rec = &store.LeaseRecord{ID: leaseID}
	}
	rec.State = "closed"
	rec.ClosedAt = time.Now().Unix()
	return e.store.PutLease(ctx, rec)
}
