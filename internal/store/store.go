package store

import (
	"context"
	"io"
	"strconv"
	"strings"
)

// ExportFormat represents the serialization format for import/export.
type ExportFormat int

const (
	FormatYAML ExportFormat = iota
	FormatJSON
)

// Store defines the interface for the local deployment store.
// Implementations must be safe for concurrent use.
type Store interface {
	// Deployment operations
	PutDeployment(ctx context.Context, d *DeploymentRecord) error
	GetDeployment(ctx context.Context, owner string, dseq uint64) (*DeploymentRecord, error)
	ListDeployments(ctx context.Context, filter DeploymentFilter) ([]*DeploymentRecord, error)
	DeleteDeployment(ctx context.Context, owner string, dseq uint64) error

	// Lease operations
	PutLease(ctx context.Context, l *LeaseRecord) error
	GetLease(ctx context.Context, id LeaseID) (*LeaseRecord, error)
	ListLeases(ctx context.Context, filter LeaseFilter) ([]*LeaseRecord, error)
	DeleteLease(ctx context.Context, id LeaseID) error

	// Bid operations
	PutBid(ctx context.Context, b *BidRecord) error
	GetBid(ctx context.Context, id BidID) (*BidRecord, error)
	ListBids(ctx context.Context, filter BidFilter) ([]*BidRecord, error)

	// Sync state management
	GetSyncState(ctx context.Context) (*SyncState, error)
	PutSyncState(ctx context.Context, s *SyncState) error

	// Schema management
	SchemaVersion() uint64
	Migrate(ctx context.Context) error

	// Import/Export
	Export(ctx context.Context, w io.Writer, format ExportFormat) error
	Import(ctx context.Context, r io.Reader, format ExportFormat, merge bool) error

	// Stats returns aggregate statistics about the store contents.
	Stats(ctx context.Context) (*StoreStats, error)

	// Lifecycle
	Close() error
}

// DeploymentRecord represents a locally-tracked deployment.
type DeploymentRecord struct {
	// Identity
	Owner string `json:"owner" yaml:"owner"`
	DSeq  uint64 `json:"dseq"  yaml:"dseq"`

	// State
	State   string `json:"state"      yaml:"state"`   // active, closed
	Version []byte `json:"version"    yaml:"version"` // on-chain version hash

	// SDL
	SDLHash string `json:"sdl_hash"   yaml:"sdl_hash"` // SHA256 of SDL content
	SDLPath string `json:"sdl_path"   yaml:"sdl_path"` // local path to SDL file (if known)

	// Financials
	Deposit       string `json:"deposit"        yaml:"deposit"`        // initial deposit
	EscrowBalance string `json:"escrow_balance" yaml:"escrow_balance"` // current escrow balance
	Transferred   string `json:"transferred"    yaml:"transferred"`    // total transferred to providers

	// Timestamps
	CreatedAt     int64 `json:"created_at"     yaml:"created_at"`     // Unix timestamp
	UpdatedAt     int64 `json:"updated_at"     yaml:"updated_at"`     // last store update
	ClosedAt      int64 `json:"closed_at"      yaml:"closed_at"`      // 0 if not closed
	CreatedHeight int64 `json:"created_height" yaml:"created_height"` // block height

	// User metadata (local-only, not on chain)
	Labels map[string]string `json:"labels" yaml:"labels"` // user-defined key-value pairs
	Notes  string            `json:"notes"  yaml:"notes"`  // free-form notes
	Tags   []string          `json:"tags"   yaml:"tags"`   // user-defined tags

	// Sync metadata
	RecordVersion uint64 `json:"record_version" yaml:"record_version"` // monotonic, for sync
}

// LeaseID uniquely identifies a lease on the Akash network.
type LeaseID struct {
	Owner    string `json:"owner"    yaml:"owner"`
	DSeq     uint64 `json:"dseq"     yaml:"dseq"`
	GSeq     uint32 `json:"gseq"     yaml:"gseq"`
	OSeq     uint32 `json:"oseq"     yaml:"oseq"`
	Provider string `json:"provider" yaml:"provider"`
}

// LeaseRecord represents a locally-tracked lease.
type LeaseRecord struct {
	ID LeaseID `json:"id" yaml:"id"`

	// State
	State string `json:"state" yaml:"state"` // active, closed, insufficient_funds

	// Pricing
	Price string `json:"price" yaml:"price"` // price per block

	// Provider info
	ProviderURI string `json:"provider_uri" yaml:"provider_uri"` // provider gateway URL

	// Endpoints (populated after manifest send)
	Endpoints []LeaseEndpoint `json:"endpoints" yaml:"endpoints"`

	// Timestamps
	CreatedAt int64 `json:"created_at" yaml:"created_at"`
	ClosedAt  int64 `json:"closed_at"  yaml:"closed_at"`

	// Sync
	RecordVersion uint64 `json:"record_version" yaml:"record_version"`
}

// LeaseEndpoint represents a service endpoint exposed by a lease.
type LeaseEndpoint struct {
	Service      string `json:"service"       yaml:"service"`
	ExternalPort uint32 `json:"external_port" yaml:"external_port"`
	URI          string `json:"uri"           yaml:"uri"`
}

// BidID uniquely identifies a bid on the Akash network.
type BidID struct {
	Owner    string `json:"owner"    yaml:"owner"`
	DSeq     uint64 `json:"dseq"     yaml:"dseq"`
	GSeq     uint32 `json:"gseq"     yaml:"gseq"`
	OSeq     uint32 `json:"oseq"     yaml:"oseq"`
	Provider string `json:"provider" yaml:"provider"`
}

// BidRecord represents a locally-tracked bid.
type BidRecord struct {
	ID BidID `json:"id" yaml:"id"`

	// State
	State string `json:"state" yaml:"state"` // open, matched, closed, lost

	// Pricing
	Price string `json:"price" yaml:"price"`

	// Provider info
	ProviderAttributes map[string]string `json:"provider_attributes" yaml:"provider_attributes"`
	ProviderAudited    bool              `json:"provider_audited"    yaml:"provider_audited"`

	// Timestamps
	CreatedAt int64 `json:"created_at" yaml:"created_at"`

	// Sync
	RecordVersion uint64 `json:"record_version" yaml:"record_version"`
}

// SyncState tracks the sync engine's progress.
type SyncState struct {
	// Last successfully processed block height
	LastBlockHeight int64 `json:"last_block_height" yaml:"last_block_height"`

	// Timestamp of last successful sync
	LastSyncTime int64 `json:"last_sync_time" yaml:"last_sync_time"`

	// Accounts being tracked (owner addresses)
	TrackedAccounts []string `json:"tracked_accounts" yaml:"tracked_accounts"`

	// Schema version of the database
	SchemaVersion uint64 `json:"schema_version" yaml:"schema_version"`
}

// DeploymentFilter specifies criteria for listing deployments.
type DeploymentFilter struct {
	Owner string   // filter by owner address
	State string   // filter by state (active, closed, or empty for all)
	Tags  []string // filter by tags (AND logic)
	Label string   // filter by label key=value
}

// LeaseFilter specifies criteria for listing leases.
type LeaseFilter struct {
	Owner    string
	DSeq     uint64 // 0 = no filter
	Provider string
	State    string
}

// BidFilter specifies criteria for listing bids.
type BidFilter struct {
	Owner    string
	DSeq     uint64
	Provider string
	State    string
}

// StoreStats contains aggregate statistics about the store contents.
type StoreStats struct {
	Deployments       int64 `json:"deployments"        yaml:"deployments"`
	ActiveDeployments int64 `json:"active_deployments" yaml:"active_deployments"`
	ClosedDeployments int64 `json:"closed_deployments" yaml:"closed_deployments"`
	Leases            int64 `json:"leases"             yaml:"leases"`
	Bids              int64 `json:"bids"               yaml:"bids"`
}

// DeploymentKey returns the colon-separated bucket key for a deployment.
// Format: <owner>:<dseq>
func DeploymentKey(owner string, dseq uint64) string {
	return owner + ":" + strconv.FormatUint(dseq, 10)
}

// LeaseKey returns the colon-separated bucket key for a lease.
// Format: <owner>:<dseq>:<gseq>:<oseq>:<provider>
func LeaseKey(id LeaseID) string {
	return strings.Join([]string{
		id.Owner,
		strconv.FormatUint(id.DSeq, 10),
		strconv.FormatUint(uint64(id.GSeq), 10),
		strconv.FormatUint(uint64(id.OSeq), 10),
		id.Provider,
	}, ":")
}

// BidKey returns the colon-separated bucket key for a bid.
// Format: <owner>:<dseq>:<gseq>:<oseq>:<provider>
func BidKey(id BidID) string {
	return strings.Join([]string{
		id.Owner,
		strconv.FormatUint(id.DSeq, 10),
		strconv.FormatUint(uint64(id.GSeq), 10),
		strconv.FormatUint(uint64(id.OSeq), 10),
		id.Provider,
	}, ":")
}
