# Phase 4 Sub-Plan 1: Store + Sync Engine

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the local deployment store (bbolt backend) and chain sync engine, enabling `akt` to track deployment/lease/bid state locally and keep it synchronized with on-chain events.

**Architecture:** The Store is a Go interface with a bbolt backend. Records (deployments, leases, bids) are JSON-encoded in named buckets. The Sync Engine subscribes to CometBFT WebSocket events, routes them through a reconciler that maps chain events to store CRUD operations, and handles startup reconciliation for missed blocks. The shared events service (`internal/events/`) already exists and publishes parsed ABCI events to a pubsub bus — the sync engine subscribes to this bus.

**Tech Stack:** Go, bbolt (`go.etcd.io/bbolt` v1.4.0 — already in go.mod), existing `internal/events/` pubsub service, CometBFT RPC WebSocket, chain-sdk query clients.

**Depends on:** Phase 3 complete (context system, chain client, config).

**TASKS.md coverage:** T038, T039, T040, T043, T044, T047, T048, T049, T050, T051, T052, T053.

---

## File Structure

```
internal/store/
├── store.go              # Store interface, record types, filter types, ID types
├── bbolt/
│   ├── store.go          # bbolt Store implementation (CRUD, lifecycle)
│   ├── store_test.go     # Store CRUD + sync state tests (T038)
│   ├── migrate.go        # Schema migration framework
│   ├── migrate_test.go   # Migration tests (T039)
│   ├── export.go         # Export/Import (YAML/JSON)
│   └── export_test.go    # Export/Import round-trip tests (T043)
internal/sync/
├── engine.go             # Sync engine (WebSocket subscription, event routing)
├── reconcile.go          # Startup reconciliation, gap detection
└── engine_test.go        # Sync engine tests (T040)
internal/events/
├── bus.go                # Pubsub bus wrapper (T047 — extend existing service.go)
└── bus_test.go           # Bus tests (T044)
internal/cli/store/
└── commands.go           # akt store status/export/import CLI commands (T051)
```

---

## Task 1: Store Interface and Record Types (T048 partial)

**Files:**
- Create: `internal/store/store.go`

This task defines the Store interface and all types verbatim from SPEC.md §4.1–4.3. No implementation yet — just the contract.

- [ ] **Step 1: Create store package with interface and types**

```go
// internal/store/store.go
package store

import (
	"context"
	"io"
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

	// Stats returns record counts and database metadata.
	Stats(ctx context.Context) (*StoreStats, error)

	// Lifecycle
	Close() error
}

// --- Record Types (SPEC §4.2) ---

type DeploymentRecord struct {
	Owner         string            `json:"owner"           yaml:"owner"`
	DSeq          uint64            `json:"dseq"            yaml:"dseq"`
	State         string            `json:"state"           yaml:"state"`
	Version       []byte            `json:"version"         yaml:"version"`
	SDLHash       string            `json:"sdl_hash"        yaml:"sdl_hash"`
	SDLPath       string            `json:"sdl_path"        yaml:"sdl_path"`
	Deposit       string            `json:"deposit"         yaml:"deposit"`
	EscrowBalance string            `json:"escrow_balance"  yaml:"escrow_balance"`
	Transferred   string            `json:"transferred"     yaml:"transferred"`
	CreatedAt     int64             `json:"created_at"      yaml:"created_at"`
	UpdatedAt     int64             `json:"updated_at"      yaml:"updated_at"`
	ClosedAt      int64             `json:"closed_at"       yaml:"closed_at"`
	CreatedHeight int64             `json:"created_height"  yaml:"created_height"`
	Labels        map[string]string `json:"labels"          yaml:"labels"`
	Notes         string            `json:"notes"           yaml:"notes"`
	Tags          []string          `json:"tags"            yaml:"tags"`
	RecordVersion uint64            `json:"record_version"  yaml:"record_version"`
}

type LeaseID struct {
	Owner    string `json:"owner"    yaml:"owner"`
	DSeq     uint64 `json:"dseq"     yaml:"dseq"`
	GSeq     uint32 `json:"gseq"     yaml:"gseq"`
	OSeq     uint32 `json:"oseq"     yaml:"oseq"`
	Provider string `json:"provider" yaml:"provider"`
}

type LeaseRecord struct {
	ID            LeaseID         `json:"id"             yaml:"id"`
	State         string          `json:"state"          yaml:"state"`
	Price         string          `json:"price"          yaml:"price"`
	ProviderURI   string          `json:"provider_uri"   yaml:"provider_uri"`
	Endpoints     []LeaseEndpoint `json:"endpoints"      yaml:"endpoints"`
	CreatedAt     int64           `json:"created_at"     yaml:"created_at"`
	ClosedAt      int64           `json:"closed_at"      yaml:"closed_at"`
	RecordVersion uint64          `json:"record_version" yaml:"record_version"`
}

type LeaseEndpoint struct {
	Service      string `json:"service"       yaml:"service"`
	ExternalPort uint32 `json:"external_port" yaml:"external_port"`
	URI          string `json:"uri"           yaml:"uri"`
}

type BidID struct {
	Owner    string `json:"owner"    yaml:"owner"`
	DSeq     uint64 `json:"dseq"     yaml:"dseq"`
	GSeq     uint32 `json:"gseq"     yaml:"gseq"`
	OSeq     uint32 `json:"oseq"     yaml:"oseq"`
	Provider string `json:"provider" yaml:"provider"`
}

type BidRecord struct {
	ID                 BidID             `json:"id"                  yaml:"id"`
	State              string            `json:"state"               yaml:"state"`
	Price              string            `json:"price"               yaml:"price"`
	ProviderAttributes map[string]string `json:"provider_attributes" yaml:"provider_attributes"`
	ProviderAudited    bool              `json:"provider_audited"    yaml:"provider_audited"`
	CreatedAt          int64             `json:"created_at"          yaml:"created_at"`
	RecordVersion      uint64            `json:"record_version"      yaml:"record_version"`
}

type SyncState struct {
	LastBlockHeight int64    `json:"last_block_height" yaml:"last_block_height"`
	LastSyncTime    int64    `json:"last_sync_time"    yaml:"last_sync_time"`
	TrackedAccounts []string `json:"tracked_accounts"  yaml:"tracked_accounts"`
	SchemaVersion   uint64   `json:"schema_version"    yaml:"schema_version"`
}

// --- Filter Types (SPEC §4.3) ---

type DeploymentFilter struct {
	Owner string
	State string
	Tags  []string
	Label string
}

type LeaseFilter struct {
	Owner    string
	DSeq     uint64
	Provider string
	State    string
}

type BidFilter struct {
	Owner    string
	DSeq     uint64
	Provider string
	State    string
}

// --- Stats ---

type StoreStats struct {
	SchemaVersion      uint64 `json:"schema_version"`
	DeploymentsActive  int    `json:"deployments_active"`
	DeploymentsClosed  int    `json:"deployments_closed"`
	Leases             int    `json:"leases"`
	Bids               int    `json:"bids"`
	DBSizeBytes        int64  `json:"db_size_bytes"`
}

// --- Key Helpers ---

// DeploymentKey returns the bucket key for a deployment record.
func DeploymentKey(owner string, dseq uint64) string {
	return owner + ":" + uitoa(dseq)
}

// LeaseKey returns the bucket key for a lease record.
func LeaseKey(id LeaseID) string {
	return id.Owner + ":" + uitoa(uint64(id.DSeq)) + ":" +
		uitoa32(id.GSeq) + ":" + uitoa32(id.OSeq) + ":" + id.Provider
}

// BidKey returns the bucket key for a bid record.
func BidKey(id BidID) string {
	return id.Owner + ":" + uitoa(uint64(id.DSeq)) + ":" +
		uitoa32(id.GSeq) + ":" + uitoa32(id.OSeq) + ":" + id.Provider
}

func uitoa(v uint64) string {
	return strconv.FormatUint(v, 10)
}

func uitoa32(v uint32) string {
	return strconv.FormatUint(uint64(v), 10)
}
```

Add the `"strconv"` import at the top alongside `"context"` and `"io"`.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/store/`
Expected: success (no errors)

- [ ] **Step 3: Commit**

```
git add internal/store/store.go
git commit -m "feat(store): add Store interface and record types per SPEC §4.1-4.3"
```

---

## Task 2: bbolt Store — Failing Tests (T038)

**Files:**
- Create: `internal/store/bbolt/store_test.go`

Write comprehensive tests for the bbolt Store implementation. All tests use `t.TempDir()` for isolation. Tests must FAIL before implementation.

- [ ] **Step 1: Write store CRUD tests**

Create `internal/store/bbolt/store_test.go` with test functions covering:

1. `TestDeploymentCRUD` — Put, Get, List (with filters: owner, state, tags), Delete
2. `TestLeaseCRUD` — Put, Get, List (with filters: owner, dseq, provider, state), Delete
3. `TestBidCRUD` — Put, Get, List (with filters: owner, dseq, provider, state)
4. `TestSyncState` — Put, Get, update cycle
5. `TestSchemaVersion` — initial version = 1
6. `TestStats` — counts active/closed deployments, leases, bids
7. `TestConcurrentAccess` — 10 goroutines writing deployments simultaneously
8. `TestGetNonExistent` — Get returns nil, nil for missing records

Each test creates a store via `bbolt.Open(filepath.Join(t.TempDir(), "test.db"))`, exercises the operation, and asserts the result. Use `github.com/stretchr/testify/require` and `assert`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/bbolt/ -v`
Expected: compilation errors (bbolt package doesn't exist yet)

- [ ] **Step 3: Commit failing tests**

```
git add internal/store/bbolt/store_test.go
git commit -m "test(store): add bbolt store CRUD tests (T038) — failing"
```

---

## Task 3: bbolt Store — Implementation (T048)

**Files:**
- Create: `internal/store/bbolt/store.go`

Implement the Store interface backed by bbolt. Bucket structure per SPEC §4.4:
- `deployments/` — key: `owner:dseq`, value: JSON-encoded DeploymentRecord
- `leases/` — key: `owner:dseq:gseq:oseq:provider`, value: JSON-encoded LeaseRecord
- `bids/` — key: `owner:dseq:gseq:oseq:provider`, value: JSON-encoded BidRecord
- `sync/` — key: `state`, value: JSON-encoded SyncState
- `meta/` — key: `schema_version`, value: binary uint64

Key design decisions:
- All reads use `db.View()` (read-only bbolt tx)
- All writes use `db.Update()` (read-write bbolt tx)
- List operations iterate the bucket and apply filter predicates in Go
- `Open()` function creates buckets if they don't exist and initializes schema version to 1

- [ ] **Step 1: Implement the bbolt store**

Create `internal/store/bbolt/store.go` with:
- `type BoltStore struct` holding `*bbolt.DB`
- `func Open(path string) (*BoltStore, error)` — opens DB, creates buckets, sets schema v1
- All Store interface methods
- Bucket name constants: `bucketDeployments`, `bucketLeases`, `bucketBids`, `bucketSync`, `bucketMeta`

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./internal/store/bbolt/ -v`
Expected: all T038 tests pass

- [ ] **Step 3: Commit**

```
git add internal/store/bbolt/store.go
git commit -m "feat(store): implement bbolt Store backend (T048)"
```

---

## Task 4: Schema Migration Framework — Tests + Implementation (T039, T049)

**Files:**
- Create: `internal/store/bbolt/migrate.go`
- Create: `internal/store/bbolt/migrate_test.go`

- [ ] **Step 1: Write migration tests**

Create `internal/store/bbolt/migrate_test.go` with:
1. `TestMigrateFromV1` — creates a v1 store, registers a v2 migration that adds a field, calls Migrate(), verifies schema_version=2
2. `TestMigrateNoOp` — store already at latest version, Migrate() is a no-op
3. `TestMigrateMultipleVersions` — v1→v2→v3 in a single Migrate() call
4. `TestMigrateForwardOnly` — attempting to set a lower schema version errors

- [ ] **Step 2: Implement migration framework**

Create `internal/store/bbolt/migrate.go` with:
- `type Migration struct { Version uint64; Fn func(tx *bbolt.Tx) error }`
- A package-level `migrations` slice (initially empty — v1 is the base)
- `func (s *BoltStore) Migrate(ctx context.Context) error` — reads current version from `meta/schema_version`, applies all migrations with version > current in order within a single `db.Update()` tx, writes new version
- `func RegisterMigration(m Migration)` for adding migrations

- [ ] **Step 3: Run tests**

Run: `go test ./internal/store/bbolt/ -v -run TestMigrate`
Expected: all pass

- [ ] **Step 4: Commit**

```
git add internal/store/bbolt/migrate.go internal/store/bbolt/migrate_test.go
git commit -m "feat(store): add schema migration framework (T039, T049)"
```

---

## Task 5: Store Export/Import — Tests + Implementation (T043, T050)

**Files:**
- Create: `internal/store/bbolt/export.go`
- Create: `internal/store/bbolt/export_test.go`

- [ ] **Step 1: Write export/import tests**

Create `internal/store/bbolt/export_test.go` with:
1. `TestExportYAML` — populate store, export to buffer, verify YAML contains header + records
2. `TestExportJSON` — same but JSON format
3. `TestImportMerge` — export, create new store with different data, import with merge=true, verify both sets present
4. `TestImportReplace` — import with merge=false, verify only imported data present
5. `TestRoundTripYAML` — export → import → export, verify identical output
6. `TestRoundTripJSON` — same for JSON
7. `TestImportEmptyStore` — import into fresh store

- [ ] **Step 2: Implement export/import**

Create `internal/store/bbolt/export.go` with the export envelope type per SPEC §4.6:
```go
type ExportEnvelope struct {
    Version       int                   `json:"version"        yaml:"version"`
    Context       string                `json:"context"        yaml:"context"`
    SchemaVersion uint64                `json:"schema_version" yaml:"schema_version"`
    ExportedAt    string                `json:"exported_at"    yaml:"exported_at"`
    SyncState     *store.SyncState      `json:"sync_state"     yaml:"sync_state"`
    Deployments   []*store.DeploymentRecord `json:"deployments" yaml:"deployments"`
    Leases        []*store.LeaseRecord  `json:"leases"         yaml:"leases"`
    Bids          []*store.BidRecord    `json:"bids"           yaml:"bids"`
}
```

`Export()` — reads all records, builds envelope, marshals to YAML/JSON, writes to `io.Writer`.
`Import()` — reads envelope from `io.Reader`, validates schema, iterates records and calls Put methods. If `merge=false`, clears all buckets first.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/store/bbolt/ -v -run 'TestExport|TestImport|TestRoundTrip'`
Expected: all pass

- [ ] **Step 4: Commit**

```
git add internal/store/bbolt/export.go internal/store/bbolt/export_test.go
git commit -m "feat(store): add YAML/JSON export/import with merge/replace (T043, T050)"
```

---

## Task 6: Events Bus Tests + Enhancement (T044, T047)

**Files:**
- Create: `internal/events/bus.go`
- Create: `internal/events/bus_test.go`

The existing `internal/events/service.go` uses `pkg.akt.dev/go/util/pubsub.Bus` for event distribution. T047 asks for a wrapper that makes the bus easily consumable by the sync engine (subscribe by event type, filter by owner/dseq). T044 asks for tests.

- [ ] **Step 1: Write bus tests**

Create `internal/events/bus_test.go` with:
1. `TestBusPublishSubscribe` — publish an event, subscriber receives it
2. `TestBusMultipleSubscribers` — two subscribers both receive the same event
3. `TestBusUnsubscribe` — subscriber unsubscribes, no longer receives events
4. `TestBusFilterByType` — subscriber only receives events matching its type filter (using type switch in the subscriber)

These tests use the existing `pubsub.NewBus()` directly — the "bus" here is just the pubsub.Bus with a thin helper layer.

- [ ] **Step 2: Create bus helper**

Create `internal/events/bus.go` with:
- `func NewBus() pubsub.Bus` — convenience constructor
- `func SubscribeDeploymentEvents(bus pubsub.Bus) <-chan interface{}` — subscribes and filters for deployment/market/escrow events relevant to the sync engine
- Helper types for type-safe event consumption

- [ ] **Step 3: Run tests**

Run: `go test ./internal/events/ -v`
Expected: all pass

- [ ] **Step 4: Commit**

```
git add internal/events/bus.go internal/events/bus_test.go
git commit -m "feat(events): add bus helpers and tests (T044, T047)"
```

---

## Task 7: Sync Engine — Failing Tests (T040)

**Files:**
- Create: `internal/sync/engine_test.go`

- [ ] **Step 1: Write sync engine tests**

Create `internal/sync/engine_test.go` with:
1. `TestEventRouting` — feed mock chain events (deployment create, bid created, lease created, deployment close), verify correct store operations called
2. `TestFilterByOwner` — events for non-tracked accounts are ignored
3. `TestFilterByDSeq` — events filtered correctly by dseq when owner matches
4. `TestReconcileFromEvent` — each of the 10 event types from SPEC §6.3 produces the correct store mutation

Tests use mock Store (satisfying the Store interface) to verify which methods are called with which arguments.

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/sync/ -v`
Expected: compilation error (package doesn't exist)

- [ ] **Step 3: Commit failing tests**

```
git add internal/sync/engine_test.go
git commit -m "test(sync): add sync engine event routing tests (T040) — failing"
```

---

## Task 8: Sync Engine — Implementation (T052)

**Files:**
- Create: `internal/sync/engine.go`

The sync engine:
1. Takes a `store.Store` and a `pubsub.Bus` (from `internal/events/`)
2. Subscribes to the bus for deployment/market/escrow events
3. Routes events through a reconciler that maps event types to store CRUD per SPEC §6.3
4. Tracks the set of owner addresses to filter events

- [ ] **Step 1: Implement sync engine**

Create `internal/sync/engine.go` with:
- `type Engine struct` — holds Store, Bus, tracked accounts set, cancel func
- `func New(s store.Store, bus pubsub.Bus, trackedAccounts []string) *Engine`
- `func (e *Engine) Start(ctx context.Context) error` — launches background goroutine
- `func (e *Engine) Stop()` — cancels context, waits for goroutine
- `func (e *Engine) handleEvent(ev interface{})` — type-switch on chain event types, call appropriate store method
- Event type mapping per SPEC §6.3 (10 event types)

- [ ] **Step 2: Run tests**

Run: `go test ./internal/sync/ -v`
Expected: all T040 tests pass

- [ ] **Step 3: Commit**

```
git add internal/sync/engine.go
git commit -m "feat(sync): implement sync engine with event routing (T052)"
```

---

## Task 9: Startup Reconciliation (T053)

**Files:**
- Create: `internal/sync/reconcile.go`

- [ ] **Step 1: Implement startup reconciliation**

Create `internal/sync/reconcile.go` with:
- `func (e *Engine) Reconcile(ctx context.Context, client client.QueryClient) error`
  - If no SyncState exists: full reconciliation (query all deployments/leases/bids for tracked accounts, store them, set SyncState)
  - If SyncState exists and gap > 1000 blocks: full reconciliation
  - If gap ≤ 1000 blocks: incremental (query tx events in missed range)
- `func (e *Engine) fullReconcile(ctx context.Context, client client.QueryClient) error` — queries chain for all deployments per tracked account
- Reconnection with exponential backoff per SPEC §6.5 (1s→60s cap + jitter)

- [ ] **Step 2: Add reconciliation tests**

Add to `internal/sync/engine_test.go`:
1. `TestFullReconcileFirstLaunch` — no SyncState, verifies chain queries are made and store is populated
2. `TestIncrementalSync` — SyncState exists with small gap, verifies only gap blocks are queried
3. `TestLargeGapTriggersFullReconcile` — gap > 1000 triggers full reconciliation

- [ ] **Step 3: Run tests**

Run: `go test ./internal/sync/ -v`
Expected: all pass

- [ ] **Step 4: Commit**

```
git add internal/sync/reconcile.go internal/sync/engine_test.go
git commit -m "feat(sync): add startup reconciliation with gap detection (T053)"
```

---

## Task 10: Store CLI Commands (T051)

**Files:**
- Create: `internal/cli/store/commands.go`
- Modify: `internal/cli/root.go` (add store subcommand)

- [ ] **Step 1: Implement store commands**

Create `internal/cli/store/commands.go` with three cobra commands:

`akt store status` — Opens the store for the current context, calls `Stats()`, renders pretty output:
```
Context:      prod
Store Path:   ~/.config/akt/contexts/prod/store/
Database:     deployments.db (2.4 MB)
Schema:       v1

Records:
  Deployments:  47 (12 active, 35 closed)
  Leases:       52
  Bids:         156

Sync State:
  Last Block:   18234567
  Last Sync:    2026-03-23 10:15:32 UTC
  Status:       synced
```

`akt store export` — Flags: `--output` (yaml/json), `--file` (default stdout), `--filter-state`. Opens store, calls `Export()`.

`akt store import <file>` — Flags: `--merge` (default true), `--replace`, `--dry-run`. Opens store, calls `Import()`.

- [ ] **Step 2: Wire into root command**

Add `root.AddCommand(storeCmd())` in `internal/cli/root.go`.

- [ ] **Step 3: Build and verify**

Run: `make akt && .cache/bin/akt store --help`
Expected: shows status/export/import subcommands

- [ ] **Step 4: Commit**

```
git add internal/cli/store/commands.go internal/cli/root.go
git commit -m "feat(cli): add akt store status/export/import commands (T051)"
```

---

## Task 11: Update TASKS.md and AICHANGELOG.md

- [ ] **Step 1: Check off completed tasks in TASKS.md**

Mark T038, T039, T040, T043, T044, T047, T048, T049, T050, T051, T052, T053 as done.

- [ ] **Step 2: Add AICHANGELOG.md entry**

Document the store, migration, export/import, events bus, sync engine, and store CLI commands.

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: all packages pass

- [ ] **Step 4: Commit**

```
git add TASKS.md AICHANGELOG.md
git commit -m "docs: mark Phase 4 Store + Sync tasks complete"
```
