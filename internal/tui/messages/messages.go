package messages

import (
	"pkg.akt.dev/akt/internal/store"
)

// DeploymentsLoadedMsg carries deployment data from the store.
type DeploymentsLoadedMsg struct {
	Deployments []*store.DeploymentRecord
	Err         error
}

// LeasesLoadedMsg carries lease data from the store.
type LeasesLoadedMsg struct {
	Leases []*store.LeaseRecord
	Err    error
}

// BidsLoadedMsg carries bid data for a specific deployment.
type BidsLoadedMsg struct {
	DSeq uint64
	Bids []*store.BidRecord
	Err  error
}

// StoreStatsMsg carries aggregate store stats.
type StoreStatsMsg struct {
	Stats *store.StoreStats
	Err   error
}

// SyncStateMsg carries the current sync state.
type SyncStateMsg struct {
	State *store.SyncState
	Err   error
}

// ViewDataRefreshMsg requests data refresh for the current view.
type ViewDataRefreshMsg struct{}
