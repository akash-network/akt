package ui

import (
	"time"

	"pkg.akt.dev/akt/internal/monitor/rpc"

	bmetypes "pkg.akt.dev/go/node/bme/v1"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"
)

// ProviderList holds the state for the provider list view.
type ProviderList struct {
	Items       []rpc.Provider
	Versions    []string // unique versions, sorted latest first
	Version     string   // currently selected version filter
	VersionIdx  int      // index in Versions
	ScrollPos   int      // scroll position for provider list
	SelectedIdx int      // currently highlighted provider in list
}

// ProviderLoader holds the state for background provider loading/checking.
type ProviderLoader struct {
	FirstRun      bool
	Loading       bool
	Total         int
	Checked       int
	ActiveLease   map[string]bool // providers with active leases (priority)
	Queue         []string        // providers to check
	InFlight      map[string]bool // providers currently being checked
	LastSync      time.Time
	LastSave      time.Time
	LastSaveError error
}

// ProviderDetail holds the state for the provider detail view.
type ProviderDetail struct {
	Showing   bool
	Provider  *rpc.Provider
	Nodes     []rpc.ProviderNodeWithGPU
	Loading   bool
	Error     error
	ScrollPos int
}

// OracleState holds live oracle and BME data for the Oracle/BME dashboard.
type OracleState struct {
	// Prices stores the most recent EventPriceData per source+denom.
	Prices []OraclePriceEntry

	// Aggregated stores the most recent aggregated prices per denom.
	Aggregated map[string]*oracletypes.EventAggregatedPrice

	// Events is a capped log of recent oracle events (newest first).
	Events []OracleEvent

	ScrollPos int

	// BME state — populated from REST queries.
	BMEStatus *bmetypes.QueryStatusResponse
	BMELedger []bmetypes.QueryLedgerRecordEntry
}

// OraclePriceEntry is a single price feed event.
type OraclePriceEntry struct {
	Denom     string
	Price     string
	Source    string // oracle provider address
	Timestamp time.Time
}

// OracleEvent is a display-friendly record for the oracle event log.
type OracleEvent struct {
	Type      string // "price", "stale_warning", "staled", "recovered", "aggregated"
	Denom     string
	Detail    string
	Timestamp time.Time
}

const maxOracleEvents = 100
