# TUI Rewrite Design Spec

## Problem

The current `internal/tui/` implementation has a 1,846-line god object (`app.go`) that violates bubbletea's Elm Architecture (Model-Update-View). Views are passive data bags without `Update()` methods. All key handling, data loading, and state management is centralized in `App.Update()` (624 lines, 98 case clauses). Adding a view requires touching 8+ switch statements. Views are untestable in isolation.

## Goal

Rewrite `internal/tui/` so every view is a self-contained `tea.Model` with its own `Init()`, `Update()`, `View()`. The root `App` becomes a thin router (~250 lines) that provides chrome (header/footer), overlay management, and global key dispatch. All existing behavior is preserved. The `internal/monitor/` package is NOT touched — it's wrapped via an adapter.

## Scope

- **In scope**: Everything under `internal/tui/` — app, views, data loading, navigation
- **Out of scope**: `internal/monitor/`, `internal/tui/components/` (kept as-is), `internal/tui/commands/` (kept)
- **Deleted**: `views/tx.go`, `views/query.go`, `views/listview.go`, `views/detailview.go` (dead/unused code)

## Architecture

### Package Layout

```
internal/tui/
├── app.go              # Root tea.Model: chrome, overlay dispatch, global keys (~250 lines)
├── router.go           # Navigation stack: push/pop/replace, breadcrumb (~120 lines)
├── keymap.go           # KeyMap definitions + config loading (kept unchanged)
├── sync_bridge.go      # Pubsub-to-TUI bridge (kept unchanged)
├── data/
│   ├── service.go      # DataService interface
│   └── loader.go       # Concrete loader with Store + LightClient
├── views/
│   ├── view.go         # ViewComponent interface + common helpers
│   ├── base_list.go    # BaseListView: embeds ResourceTable, handles j/k/g/G/filter
│   ├── base_detail.go  # BaseDetailView: scroll management, back hint rendering
│   ├── dashboard.go    # Full tea.Model
│   ├── deployments.go  # Full tea.Model (embeds BaseListView)
│   ├── deployment_detail.go  # Full tea.Model (embeds BaseDetailView, 4 tabs)
│   ├── leases.go       # Full tea.Model (embeds BaseListView)
│   ├── lease_detail.go # Full tea.Model (embeds BaseDetailView)
│   ├── providers.go    # Full tea.Model (embeds BaseListView)
│   ├── provider_detail.go  # Full tea.Model (embeds BaseDetailView)
│   ├── governance.go   # Full tea.Model (embeds BaseListView)
│   ├── proposal_detail.go  # Full tea.Model (embeds BaseDetailView, tally bars)
│   ├── staking.go      # Full tea.Model (embeds BaseListView)
│   ├── validator_detail.go # Full tea.Model (embeds BaseDetailView)
│   ├── logviewer.go    # Log viewer overlay (full tea.Model)
│   ├── monitor.go      # Adapter wrapping internal/monitor/ui tea.Model
│   ├── palette.go      # Command palette (migrated from command.go)
│   └── help.go         # Help overlay
├── components/         # ALL KEPT UNCHANGED
├── commands/           # KEPT UNCHANGED
└── messages/
    └── messages.go     # All message types (expanded with nav messages)
```

### ViewComponent Interface

```go
package views

import (
    tea "github.com/charmbracelet/bubbletea/v2"
    "pkg.akt.dev/akt/internal/tui/components"
)

// ViewComponent is the contract every navigable view must satisfy.
type ViewComponent interface {
    tea.Model
    SetSize(w, h int)
    Breadcrumb() string
    ShortHelp() []components.HintPair
}
```

### Router (Navigation Stack)

```go
package tui

type Router struct {
    stack []views.ViewComponent
    w, h  int
}

func NewRouter() Router
func (r *Router) Push(v views.ViewComponent) tea.Cmd  // push + Init + SetSize
func (r *Router) Pop() tea.Cmd                         // pop, return new-top Init if needed
func (r *Router) Replace(v views.ViewComponent) tea.Cmd // replace top
func (r *Router) Active() views.ViewComponent
func (r *Router) Depth() int
func (r *Router) Breadcrumb() string                   // join stack labels with " > "
func (r *Router) SetSize(w, h int)                     // propagate to active only
func (r *Router) Update(msg tea.Msg) tea.Cmd           // delegate to active view
func (r Router) View() string                          // active view's View()
```

Only the active (top) view receives `Update()` calls and `SetSize()` on resize. Views below the top are frozen until they become active again via `Pop()`.

### Data Service

```go
package data

import (
    tea "github.com/charmbracelet/bubbletea/v2"
    "pkg.akt.dev/akt/internal/tui/messages"
)

type Service interface {
    LoadDeployments(owner, filter string) tea.Cmd
    LoadLeases(owner, filter string) tea.Cmd
    LoadDeploymentLeases(owner string, dseq uint64) tea.Cmd
    LoadBids(owner string, dseq uint64) tea.Cmd
    LoadProviders() tea.Cmd
    LoadProposals() tea.Cmd
    LoadTallies(proposals []*govv1.Proposal) tea.Cmd
    LoadValidators() tea.Cmd
    LoadStakingPool() tea.Cmd
    LoadBalance(account string) tea.Cmd
    LoadStoreStats() tea.Cmd
    LoadSyncState() tea.Cmd
}

type Loader struct {
    store       store.Store
    lightClient aclient.LightClient
    owner       string
}
```

Each method returns a `tea.Cmd` that produces the corresponding `messages.*Msg` when executed. Views hold a `data.Service` reference and call these in their `Init()` and on user actions.

### Message Types

Existing data messages are kept unchanged. New navigation messages are added:

```go
package messages

// Navigation messages — returned by views, intercepted by App
type PushViewMsg struct{ View views.ViewComponent }
type PopViewMsg struct{}

// Overlay messages — returned by views, intercepted by App
type ShowConfirmMsg struct {
    Kind components.ConfirmKind
    Data components.ConfirmData
}
type ShowToastMsg struct {
    Message string
    Tone    int
}

// Log stream messages — returned by views, intercepted by App
type StartLogStreamMsg struct {
    Owner string
    DSeq  uint64
}
type StopLogStreamMsg struct{}
```

### App (Root Model)

```go
type App struct {
    keys     KeyMap
    router   Router
    palette  *views.Palette
    confirm  *components.ConfirmDialog
    help     *views.HelpOverlay
    logView  *views.LogViewer
    toast    *components.Toast

    monitor  tea.Model       // embedded monitor/ui.Model, nil if no RPC
    bridge   *syncBridge
    data     data.Service

    // Chrome state
    chainID, rpcEndpoint, account, version string
    syncActive bool
    blockHeight string

    // Log stream lifecycle (kept at App level — needs keyring/provider auth)
    logCtx    context.Context
    logCancel context.CancelFunc
    logStream *rest.ServiceLogs
    keyring   sdkkeyring.Keyring
    clientCtx sdkclient.Context

    // Store for log stream lease lookup
    dataStore store.Store
    resolvedCtx *aktctx.Context

    width, height int
    standalone bool
}
```

**Update() flow** (~100 lines):
1. Handle `WindowSizeMsg` → resize router + overlays
2. Forward non-key messages to monitor if it exists (keeps WS/tick alive)
3. If standalone mode → only Ctrl+C intercepted, rest to monitor
4. Overlay priority: confirm > logViewer > help > palette
5. Intercept navigation messages: `PushViewMsg`, `PopViewMsg`, `ShowConfirmMsg`, `ShowToastMsg`, `StartLogStreamMsg`, `StopLogStreamMsg`
6. Intercept data messages that update chrome: `SyncStateMsg`, `BalanceLoadedMsg`
7. Global keys: Ctrl+C, `:`, `?`, number keys 1-6, Esc
8. Delegate everything else to `router.Update(msg)`

**View() flow:**
- If monitor active and not overlaid: render monitor view directly
- Otherwise: `header + navBar + breadcrumb + router.View()`
- Overlay compositing: logViewer, confirm, help, palette
- Toast appended below content

### Monitor Integration

The `internal/monitor/ui.Model` is wrapped in `views.MonitorAdapter` which implements `ViewComponent`. The adapter delegates all `Init`/`Update`/`View` calls to the inner model. `SetSize` sends a `WindowSizeMsg` to the inner model. `ShortHelp` returns monitor-specific hints.

Non-key messages (ticks, WebSocket data) are still forwarded to the monitor by the `App` regardless of which view is active — this preserves the current behavior where the monitor's background goroutines don't stall when the user navigates away.

### Log Viewer

The log viewer stays as an overlay managed by `App` (not on the router stack), since:
- It overlays the current view rather than replacing it
- It requires `App`-level resources (keyring, provider auth) for stream management
- Esc closes it and returns to the view underneath

### BaseListView

```go
type BaseListView struct {
    table  components.ResourceTable
    keys   KeyMap
    w, h   int
}

func NewBaseListView(cols []components.TableColumn, empty string, keys KeyMap) BaseListView
func (b *BaseListView) SetRows(rows []components.TableRow)
func (b *BaseListView) SetSize(w, h int)
func (b *BaseListView) Update(msg tea.Msg) tea.Cmd  // handles j/k/g/G/filter
func (b BaseListView) View() string
func (b BaseListView) Cursor() int
func (b BaseListView) SelectedID() string
```

Concrete list views embed `BaseListView` and add:
- Column definitions
- Row mapping from domain types
- View-specific key handling (enter, logs, close, etc.)
- Data loading via `data.Service`

### BaseDetailView

```go
type BaseDetailView struct {
    keys   KeyMap
    scroll int
    w, h   int
}

func NewBaseDetailView(keys KeyMap) BaseDetailView
func (b *BaseDetailView) SetSize(w, h int)
func (b *BaseDetailView) Update(msg tea.Msg) tea.Cmd  // handles j/k scroll
func (b BaseDetailView) VisibleWindow(lines []string) []string
func (b BaseDetailView) ScrollHint(totalLines int) string
```

Concrete detail views embed `BaseDetailView` and add:
- Domain-specific rendering in `View()`
- Tab management (deployment detail has 4 tabs)
- Data loading for sub-resources

## Behavior Preservation

Every key binding, data flow, visual element, and user interaction from the current TUI is preserved exactly:

- Dashboard layout with 3-column panels, sparklines, active deployments, shortcuts
- All 6 list views with exact column layouts and filters
- All 6 detail views with exact sections and tabs
- Command palette with fuzzy search and 20+ commands
- Confirm dialog with vote variant
- Log viewer with pause/clear/service-filter/scroll
- Help overlay with 4 sections
- Toast notifications
- Header bar with chain info + sync status
- Nav bar with numbered tabs + hints
- Breadcrumb trail
- Number key shortcuts (1-6)
- Sync bridge → store → view refresh cycle
- Monitor model embedding with background message forwarding

## What Changes

| Before | After |
|--------|-------|
| 1,846-line `app.go` with 624-line `Update()` | ~250-line `app.go` with ~100-line `Update()` |
| Views are passive structs (no `Update`) | Every view is a full `tea.Model` |
| 12-value enum, 8 exhaustive switches | Polymorphic dispatch via `ViewComponent` |
| Flat view switching | Navigation stack with push/pop |
| Data loading in `app.go` | Extracted to `data/` package, called by views |
| Duplicated scroll logic in 7 files | `BaseDetailView` handles all scrolling |
| Duplicated list wrapper in 5 files | `BaseListView` handles all cursor/filter |
| 4 duplicate truncation functions | Single `truncate()` in `views/view.go` |
| Dead code (tx.go, query.go, unused abstractions) | Deleted |
