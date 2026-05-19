package messages

import (
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"pkg.akt.dev/akt/internal/store"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
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

// ProposalsLoadedMsg carries governance proposals from a chain query.
type ProposalsLoadedMsg struct {
	Proposals []*govv1.Proposal
	Err       error
}

// ValidatorsLoadedMsg carries validator data from a chain query.
type ValidatorsLoadedMsg struct {
	Validators []stakingtypes.Validator
	Err        error
}

// ProvidersLoadedMsg carries on-chain provider data from a chain query.
type ProvidersLoadedMsg struct {
	Providers ptypes.Providers
	Err       error
}

// LogLineMsg carries a single log line from a provider log stream.
type LogLineMsg struct {
	Name    string // service name
	Message string // log content
}

// LogStreamClosedMsg signals that the log stream has ended.
type LogStreamClosedMsg struct {
	Reason string
}
