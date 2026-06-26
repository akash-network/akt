# TUI Resource Views Implementation Plan (Part 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement real resource views (Dashboard, Deployments, Leases, Providers, Governance, Staking, Log Viewer) with data binding to the local store and chain client.

**Architecture:** First, wire data sources (store.Store, context.Manager) into the TUI App via an expanded Config struct. Then implement each view as a standalone file in `internal/tui/views/` that receives data via bubbletea messages. Views use the shared components from Part 1 (ResourceTable, StateTag, KVDetail, Footer, ConfirmDialog). Data is loaded asynchronously via tea.Cmd functions — views show loading states until data arrives.

**Tech Stack:** Go, Bubbletea v2, Lipgloss v2, Bubbles v2, bbolt store, Cosmos SDK client.Context

**Design Reference:** `design/prototype/tui-views.jsx` (primary), `design/prototype/tui-data.jsx` (mock data structure), `design/prototype/tui-monitor.jsx` (log viewer, shell), `design/prototype/akash-tui-v2/main.go` (Go patterns)

---

## File Structure

### Modified Files
| File | Changes |
|------|---------|
| `internal/tui/app.go` | Expand Config + App struct with store/context references, add data loading commands, dispatch view messages, integrate new views |
| `internal/tui/keymap.go` | Add keybindings for view-specific actions (d=close, l=logs, s=shell, v=vote, u=update, f=filter) |
| `internal/tui/commands/registry.go` | Wire command palette entries to push view navigation |

### New Files
| File | Responsibility |
|------|---------------|
| `internal/tui/messages/messages.go` | Shared bubbletea message types for data loading (DataLoadedMsg, ErrorMsg, etc.) |
| `internal/tui/views/dashboard.go` | Dashboard landing view: wallet summary, active deployments, network stats, recent activity |
| `internal/tui/views/deployments.go` | Deployments list view with store data binding |
| `internal/tui/views/deployment_detail.go` | Deployment detail with 4 sub-tabs (overview, lease, escrow, endpoints) |
| `internal/tui/views/leases.go` | Leases list view with store data binding |
| `internal/tui/views/providers.go` | Providers list view (on-chain provider query) |
| `internal/tui/views/governance.go` | Governance proposals list (chain query) |
| `internal/tui/views/staking.go` | Validators/staking list (chain query) |
| `internal/tui/views/logviewer.go` | Streaming log viewer overlay |

---

## Task 1: Data Plumbing — Wire Store and Context into TUI App

**Files:**
- Modify: `internal/tui/app.go`
- Create: `internal/tui/messages/messages.go`

This is the foundational task. The TUI App currently holds no store or context references. We need to:
1. Expand `Config` to accept a `store.Store` and `*context.Context` (resolved context)
2. Expand `App` to hold these references
3. Define shared message types for async data loading
4. Add initial data loading on view switch

- [ ] **Step 1: Create the messages package**

Create `internal/tui/messages/messages.go` with shared bubbletea message types:

```go
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

// ErrorMsg carries a generic error.
type ErrorMsg struct {
    Err     error
    Context string // what was being attempted
}

// ViewDataRefreshMsg requests data refresh for the current view.
type ViewDataRefreshMsg struct{}
```

- [ ] **Step 2: Expand the Config struct**

In `internal/tui/app.go`, add optional store and context fields to `Config`:

```go
type Config struct {
    Viper            *viper.Viper
    RPCEndpoint      string
    RESTEndpoint     string
    CacheDir         string
    Insecure         bool
    Standalone       bool
    InitialDashboard string

    // Data sources (optional — TUI works in degraded mode without them)
    Store          store.Store        // local deployment store (nil = no store)
    ResolvedCtx    *aktctx.Context    // resolved akt context (nil = no context)
}
```

- [ ] **Step 3: Expand the App struct**

Add data references to the `App` struct:

```go
type App struct {
    // ... existing fields ...

    // Data sources
    store       store.Store
    resolvedCtx *aktctx.Context

    // Cached data (populated via async messages)
    storeStats  *store.StoreStats
    syncState   *store.SyncState
}
```

- [ ] **Step 4: Add data loading commands**

Add helper functions that return `tea.Cmd` for async data loading:

```go
func loadDeployments(s store.Store, owner string) tea.Cmd {
    return func() tea.Msg {
        if s == nil {
            return messages.DeploymentsLoadedMsg{Err: fmt.Errorf("no store available")}
        }
        depls, err := s.ListDeployments(context.Background(), store.DeploymentFilter{Owner: owner})
        return messages.DeploymentsLoadedMsg{Deployments: depls, Err: err}
    }
}

func loadLeases(s store.Store, owner string) tea.Cmd {
    return func() tea.Msg {
        if s == nil {
            return messages.LeasesLoadedMsg{Err: fmt.Errorf("no store available")}
        }
        leases, err := s.ListLeases(context.Background(), store.LeaseFilter{Owner: owner})
        return messages.LeasesLoadedMsg{Leases: leases, Err: err}
    }
}

func loadStoreStats(s store.Store) tea.Cmd {
    return func() tea.Msg {
        if s == nil {
            return messages.StoreStatsMsg{Err: fmt.Errorf("no store available")}
        }
        stats, err := s.Stats(context.Background())
        return messages.StoreStatsMsg{Stats: stats, Err: err}
    }
}

func loadSyncState(s store.Store) tea.Cmd {
    return func() tea.Msg {
        if s == nil {
            return messages.SyncStateMsg{Err: fmt.Errorf("no store available")}
        }
        state, err := s.GetSyncState(context.Background())
        return messages.SyncStateMsg{State: state, Err: err}
    }
}
```

- [ ] **Step 5: Trigger data loading on view switch**

In the `Update()` method, when the user switches to a view (via number keys or palette), fire the appropriate load command. For example, when switching to viewDeployments:

```go
case "2": // was already mapped to viewDeployments
    if key.Matches(msg, a.keys.Deployments) {
        a.view = viewDeployments
        owner := ""
        if a.resolvedCtx != nil {
            owner = a.resolvedCtx.DefaultAccount
        }
        return a, loadDeployments(a.store, owner)
    }
```

- [ ] **Step 6: Handle data messages in Update()**

Add cases for the new message types:

```go
case messages.DeploymentsLoadedMsg:
    // Convert store records to ListView items and update the deployments list
    // (This will be filled in by subsequent tasks when views are implemented)
    
case messages.LeasesLoadedMsg:
    // Similar for leases

case messages.StoreStatsMsg:
    if msg.Err == nil {
        a.storeStats = msg.Stats
    }

case messages.SyncStateMsg:
    if msg.Err == nil {
        a.syncState = msg.State
    }
```

- [ ] **Step 7: Update renderHeader() to use resolved context**

Replace hardcoded placeholder values with data from `a.resolvedCtx`:

```go
func (a App) renderHeader() string {
    ctxName := "—"
    chainID := ""
    account := ""
    if a.resolvedCtx != nil {
        ctxName = a.resolvedCtx.Name
        chainID = a.resolvedCtx.Network.ChainID
        account = a.resolvedCtx.DefaultAccount
    }
    // ... render with real values ...
}
```

- [ ] **Step 8: Verify build**

Run: `go test ./internal/tui/... -v`
Run: `make akt`

- [ ] **Step 9: Commit**

```bash
git add internal/tui/messages/ internal/tui/app.go
git commit -m "feat(tui): wire store and context into TUI app for data binding

Expand Config/App with store.Store and resolved context references.
Add messages package with shared data loading message types.
Add async data loading commands (deployments, leases, stats, sync).
Replace hardcoded header values with resolved context data."
```

---

## Task 2: Dashboard View

**Files:**
- Create: `internal/tui/views/dashboard.go`
- Modify: `internal/tui/app.go` (integrate dashboard)

The dashboard is the landing view — the richest view in the JSX prototype. It shows wallet summary, active deployments, network stats, recent activity, and keyboard shortcuts.

Since the TUI may not have a chain query client for balance queries, the dashboard will work in **degraded mode** — showing what's available from the store (deployment counts, sync state) and omitting chain-query-dependent data (balances, block times).

- [ ] **Step 1: Create dashboard.go**

Create `internal/tui/views/dashboard.go` with a `Dashboard` model:

```go
package views

import (
    "fmt"
    "strings"

    "charm.land/lipgloss/v2"

    "pkg.akt.dev/akt/internal/store"
    "pkg.akt.dev/akt/internal/tui/components"
    "pkg.akt.dev/akt/internal/ui/theme"
)

// Dashboard is the landing view showing a summary of the user's Akash state.
type Dashboard struct {
    width  int
    height int

    // Data (populated via messages)
    contextName    string
    chainID        string
    account        string
    stats          *store.StoreStats
    syncState      *store.SyncState
    deployments    []*store.DeploymentRecord // active only, for the summary panel
}

func NewDashboard() Dashboard {
    return Dashboard{}
}

func (d *Dashboard) SetContext(name, chainID, account string) {
    d.contextName = name
    d.chainID = chainID
    d.account = account
}

func (d *Dashboard) SetStats(stats *store.StoreStats) {
    d.stats = stats
}

func (d *Dashboard) SetSyncState(state *store.SyncState) {
    d.syncState = state
}

func (d *Dashboard) SetActiveDeployments(depls []*store.DeploymentRecord) {
    d.deployments = depls
}

func (d *Dashboard) SetSize(w, h int) {
    d.width = w
    d.height = h
}

func (d Dashboard) View() string {
    // Render a 2-column layout:
    // Left: welcome banner + active deployments + shortcuts
    // Right: account info + network status
    // Use components.Section, components.KV, etc.
    // ... implementation ...
}
```

The `View()` method renders:
1. **Welcome banner** — ASCII art `akt` logo (from JSX prototype) + context/account info
2. **Account panel** — address, deployment count (from store stats), sync status
3. **Active Deployments panel** — top 4 active deployments (name, cost) from store + monthly burn total
4. **Network panel** — chain ID, last synced block height (from sync state), last sync time
5. **Shortcuts panel** — key hints (1-6, Enter, Esc, :, ?, D)

Each panel uses `components.Section()` for headings and `components.KV()` for key-value pairs.

- [ ] **Step 2: Integrate into app.go**

In `app.go`:
1. Add `dashboard views.Dashboard` field to `App` struct
2. In `newApp()`, create `views.NewDashboard()` and configure with context info
3. In `Update()`, handle `StoreStatsMsg` and `SyncStateMsg` to update dashboard
4. In `View()`, when `a.view == viewDashboard`, render `a.dashboard.View()`
5. In `Init()`, fire `loadStoreStats` and `loadSyncState` commands

- [ ] **Step 3: Verify**

Run: `go test ./internal/tui/... -v`
Run: `make akt`

- [ ] **Step 4: Commit**

```bash
git add internal/tui/views/dashboard.go internal/tui/app.go
git commit -m "feat(tui): implement dashboard landing view

Dashboard shows welcome banner, account summary (from resolved context),
active deployments panel (from store), network status (from sync state),
and keyboard shortcuts. Works in degraded mode when store is unavailable."
```

---

## Task 3: Deployments List View

**Files:**
- Create: `internal/tui/views/deployments.go`
- Modify: `internal/tui/app.go`

Replaces the empty `deployments` ListView with a purpose-built view that binds to `store.DeploymentRecord` data.

- [ ] **Step 1: Create deployments.go**

The deployments view uses `components.ResourceTable` with columns matching the JSX prototype:
- DSEQ (110px), Name/Image (1fr), State (92px, RenderFunc=StateTag), CPU (56px), Memory (64px), GPU (100px), Provider (1fr), Uptime/Age (90px), Cost (140px)

```go
package views

type DeploymentsView struct {
    table   components.ResourceTable
    data    []*store.DeploymentRecord
    width   int
    height  int
    filter  string // "", "active", "closed"
}

func NewDeploymentsView() DeploymentsView
func (v *DeploymentsView) SetData(records []*store.DeploymentRecord)
func (v *DeploymentsView) SetSize(w, h int)
func (v *DeploymentsView) CycleFilter() // "" → "active" → "closed" → ""
func (v *DeploymentsView) SelectedDseq() uint64
func (v *DeploymentsView) HandleKey(msg tea.KeyPressMsg, keys KeyMap) (consumed bool, cmd tea.Cmd)
func (v DeploymentsView) View() string
```

`SetData` converts `[]*store.DeploymentRecord` to `[]components.TableRow` using the column mapping. The state column uses `components.StateTag()` as its RenderFunc.

- [ ] **Step 2: Integrate into app.go**

Replace the generic `deployments views.ListView` with `deployments views.DeploymentsView`. Handle `messages.DeploymentsLoadedMsg` to call `SetData()`. When user presses Enter on a deployment, fire a navigation push to detail view.

- [ ] **Step 3: Verify**

Run: `make akt`

- [ ] **Step 4: Commit**

```bash
git add internal/tui/views/deployments.go internal/tui/app.go
git commit -m "feat(tui): implement deployments list view with store data binding

ResourceTable with DSEQ/Image/State/CPU/Memory/GPU/Provider/Age/Cost columns.
State column renders color-coded │state│ tags. Supports state filter cycling.
Data loaded async from store.Store.ListDeployments."
```

---

## Task 4: Deployment Detail View

**Files:**
- Create: `internal/tui/views/deployment_detail.go`
- Modify: `internal/tui/app.go`

The richest detail view — 4 sub-tabs (overview, lease, escrow, endpoints) following the JSX prototype.

- [ ] **Step 1: Create deployment_detail.go**

```go
package views

type DeploymentDetailView struct {
    deployment *store.DeploymentRecord
    leases     []*store.LeaseRecord
    bids       []*store.BidRecord
    tab        int // 0=overview, 1=lease, 2=escrow, 3=endpoints
    width      int
    height     int
    scroll     int
}

func NewDeploymentDetailView() DeploymentDetailView
func (v *DeploymentDetailView) SetDeployment(d *store.DeploymentRecord)
func (v *DeploymentDetailView) SetLeases(leases []*store.LeaseRecord)
func (v *DeploymentDetailView) SetBids(bids []*store.BidRecord)
func (v *DeploymentDetailView) SetSize(w, h int)
func (v *DeploymentDetailView) NextTab()
func (v *DeploymentDetailView) SetTab(n int)
func (v DeploymentDetailView) View() string
```

The `View()` method renders:
1. **Header strip** — deployment name/image, DSEQ, state badge, owner
2. **Sub-tab bar** — `1 overview`, `2 lease`, `3 escrow`, `4 endpoints` (Tab cycles, 1-4 jumps)
3. **Tab content:**
   - **Overview**: Resources (CPU/memory/GPU/storage) + Placement (provider/region/uptime/cost) using `components.SectionWithKV`. SDL hash shown.
   - **Lease**: Lease info (provider/state/price/opened/gseq/oseq) + Bid history table
   - **Escrow**: Balance/consumed/remaining/burn rate + progress bar + fee events
   - **Endpoints**: Forwarded URIs with service name and port

- [ ] **Step 2: Integrate navigation**

In `app.go`, add a `deploymentDetail views.DeploymentDetailView` field. When user presses Enter on deployments list, get the selected DSEQ, load deployment + leases + bids from store, populate the detail view, push navigation to detail. Wire Tab/1-4 keys to tab switching within the detail view.

- [ ] **Step 3: Verify**

Run: `make akt`

- [ ] **Step 4: Commit**

```bash
git add internal/tui/views/deployment_detail.go internal/tui/app.go
git commit -m "feat(tui): implement deployment detail view with 4 sub-tabs

Overview (resources, placement, SDL hash), Lease (provider info, bid history),
Escrow (balance, burn rate, progress bar), Endpoints (forwarded URIs).
Data loaded from store records. Tab/1-4 for sub-tab navigation."
```

---

## Task 5: Leases List View

**Files:**
- Create: `internal/tui/views/leases.go`
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Create leases.go**

Similar pattern to deployments: `LeasesView` wrapping `ResourceTable` with columns:
- DSEQ (110px), Provider (1fr), State (90px, StateTag), Price (180px), Escrow (120px), Opened (120px)

Binds to `[]*store.LeaseRecord` from store.

- [ ] **Step 2: Integrate into app.go**

Replace generic `leases views.ListView`. Load on view switch. Handle `LeasesLoadedMsg`.

- [ ] **Step 3: Verify and commit**

```bash
git add internal/tui/views/leases.go internal/tui/app.go
git commit -m "feat(tui): implement leases list view with store data binding"
```

---

## Task 6: Providers List View

**Files:**
- Create: `internal/tui/views/providers.go`
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Create providers.go**

The providers view shows on-chain provider data. Since we don't have a chain query client in the TUI currently, this view will initially show a placeholder message directing users to `akt monitor provider` for real-time provider data. The monitor's provider fleet view already has full provider scanning.

For now, create the view structure with table columns from the JSX prototype (Host/Region/GPU/CPU/Mem/Leases/Audit/Version) but show "Provider data requires chain connection. Use akt monitor provider for real-time fleet monitoring." as empty state.

This can be enhanced later when the chain client is wired in.

- [ ] **Step 2: Integrate and commit**

```bash
git add internal/tui/views/providers.go internal/tui/app.go
git commit -m "feat(tui): implement providers list view (placeholder, pending chain client)"
```

---

## Task 7: Governance List View

**Files:**
- Create: `internal/tui/views/governance.go`
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Create governance.go**

Similar to providers — governance proposals require chain queries. Create the table structure (ID/Title/Status/Yes/No/Abstain/Veto/Ends) but show placeholder until chain client is available.

The monitor's governance params view covers module parameters; this view is for proposals specifically.

- [ ] **Step 2: Integrate and commit**

```bash
git add internal/tui/views/governance.go internal/tui/app.go
git commit -m "feat(tui): implement governance proposals view (placeholder, pending chain client)"
```

---

## Task 8: Staking/Validators List View

**Files:**
- Create: `internal/tui/views/staking.go`
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Create staking.go**

Table columns: Rank/Moniker/Power/VP%/Commission/Uptime/Signed. Placeholder until chain client.

- [ ] **Step 2: Integrate and commit**

```bash
git add internal/tui/views/staking.go internal/tui/app.go
git commit -m "feat(tui): implement staking validators view (placeholder, pending chain client)"
```

---

## Task 9: Log Viewer

**Files:**
- Create: `internal/tui/views/logviewer.go`
- Modify: `internal/tui/app.go`

The log viewer is a streaming viewport that shows container logs from a deployment's active lease. It connects to the provider gateway via the existing `internal/provider/` client.

- [ ] **Step 1: Create logviewer.go**

```go
package views

type LogViewer struct {
    title    string
    dseq     string
    service  string
    lines    []LogLine
    paused   bool
    scroll   int
    width    int
    height   int
    active   bool
}

type LogLine struct {
    Timestamp string
    Level     string // INFO, WARN, ERR
    Scope     string // service/component name
    Message   string
}

func NewLogViewer() LogViewer
func (v *LogViewer) Open(title, dseq, service string)
func (v *LogViewer) Close()
func (v LogViewer) Active() bool
func (v *LogViewer) AppendLine(line LogLine)
func (v *LogViewer) TogglePause()
func (v *LogViewer) Clear()
func (v *LogViewer) SetSize(w, h int)
func (v *LogViewer) ScrollUp()
func (v *LogViewer) ScrollDown()
func (v LogViewer) View() string
```

The `View()` renders:
1. Header: `LOGS` badge + deployment name + dseq + service + streaming/paused indicator
2. Log area: Columns for timestamp/level/scope/message. Level color-coded (INFO=green, WARN=yellow, ERR=red)
3. Footer hints: space=pause/resume, c=clear, /=filter, esc=back

For now, this creates the UI structure without actual log streaming (which requires provider gateway connection). The `AppendLine` method allows external code to push lines.

- [ ] **Step 2: Integrate into app.go**

Add `logViewer views.LogViewer` to App. When user presses `l` on a deployment, open the log viewer overlay. Wire key handling for space/c/Esc.

- [ ] **Step 3: Verify and commit**

```bash
git add internal/tui/views/logviewer.go internal/tui/app.go
git commit -m "feat(tui): implement log viewer with streaming viewport

Streaming log viewer with timestamp/level/scope/message columns.
Color-coded levels (INFO=green, WARN=yellow, ERR=red).
Pause/resume, clear, scroll. UI structure ready for provider gateway integration."
```

---

## Task 10: View-Specific Keybinding Updates

**Files:**
- Modify: `internal/tui/keymap.go`
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Add action keybindings to KeyMap**

```go
type KeyMap struct {
    // ... existing fields ...

    // View-specific actions
    Close     key.Binding // d — close deployment
    Update    key.Binding // u — update deployment
    Logs      key.Binding // l — view logs
    Shell     key.Binding // s — open shell
    Vote      key.Binding // v — vote on proposal
    Deploy    key.Binding // D — new deployment
    Filter    key.Binding // f — cycle state filter
    Search    key.Binding // / — fuzzy search
}
```

- [ ] **Step 2: Wire action keys in app.go Update()**

When on deployments view and `d` pressed → open confirm dialog for close. When `l` pressed → open log viewer. When `f` pressed → cycle filter. Etc.

- [ ] **Step 3: Verify and commit**

```bash
git add internal/tui/keymap.go internal/tui/app.go
git commit -m "feat(tui): add view-specific action keybindings

d=close, u=update, l=logs, s=shell, v=vote, D=deploy, f=filter, /=search.
Wired to appropriate view actions in the Update handler."
```

---

## Task 11: Final Integration Test

- [ ] **Step 1: Run full test suite**

Run: `go test ./internal/... 2>&1 | tail -20`
Expected: All PASS

- [ ] **Step 2: Build**

Run: `make akt`

- [ ] **Step 3: Update golden files if needed**

Run: `go test ./internal/tui/ -update`

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore(tui): integration test fixes for resource views"
```

---

## Summary

| Task | What | Dependencies |
|------|------|-------------|
| 1 | Data plumbing (store + context into App) | None |
| 2 | Dashboard view | Task 1 |
| 3 | Deployments list | Task 1 |
| 4 | Deployment detail (4 tabs) | Task 3 |
| 5 | Leases list | Task 1 |
| 6 | Providers list (placeholder) | Task 1 |
| 7 | Governance list (placeholder) | Task 1 |
| 8 | Staking list (placeholder) | Task 1 |
| 9 | Log viewer | Task 1 |
| 10 | View-specific keybindings | Tasks 2-9 |
| 11 | Integration test | All above |

**Parallelizable after Task 1:** Tasks 2-9 can all run in parallel.

**Note on placeholder views:** Tasks 6-8 (Providers, Governance, Staking) are created as structural placeholders with full table column definitions but no data binding. They require a chain query client (`LightClient` or `sdkclient.Context`) that isn't wired into the TUI yet. This is intentional — the view structure is in place and ready to light up when the client is added in a future task.
