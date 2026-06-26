package messages

import (
	"cosmossdk.io/math"
	tea "charm.land/bubbletea/v2"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
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

// TallyLoadedMsg carries vote tally results for proposals.
type TallyLoadedMsg struct {
	// Tallies maps proposal ID to its tally result.
	Tallies map[uint64]*govv1.TallyResult
	Err     error
}

// StakingPoolMsg carries the staking pool info (total bonded tokens).
type StakingPoolMsg struct {
	BondedTokens math.Int
	Err          error
}

// BalanceLoadedMsg carries the account balance.
type BalanceLoadedMsg struct {
	Amount string // formatted balance string like "148.52 AKT"
	Err    error
}

// --- Navigation messages (returned by views, intercepted by App) ---

// PushViewMsg asks the App to push a new view onto the navigation stack.
// The View field must not be nil and must implement ViewComponent.
// The App will call View.Init() after pushing.
type PushViewMsg struct {
	View tea.Model
}

// PopViewMsg asks the App to pop the current view off the navigation stack,
// returning to the previous view. If the stack has only one view (dashboard),
// this is a no-op.
type PopViewMsg struct{}

// --- Overlay messages (returned by views, intercepted by App) ---

// ShowConfirmMsg asks the App to open the confirmation dialog.
type ShowConfirmMsg struct {
	Kind components.ConfirmKind
	Data components.ConfirmData
}

// ShowToastMsg asks the App to show a toast notification.
type ShowToastMsg struct {
	Message string
	Tone    int // components.ToastOK, ToastInfo, ToastError
}

// --- Log stream messages (returned by views, intercepted by App) ---

// StartLogStreamMsg asks the App to start streaming logs for a deployment.
type StartLogStreamMsg struct {
	Owner string
	DSeq  uint64
}

// StopLogStreamMsg asks the App to stop the active log stream.
type StopLogStreamMsg struct{}
