# TUI Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite `internal/tui/` so every view is a self-contained `tea.Model`, the root App is a thin ~250-line router, and the architecture matches bubbletea's Elm Architecture.

**Architecture:** Navigation stack (`Router`) holds `ViewComponent` interface values. Each view owns its `Init()`/`Update()`/`View()`, loads its own data via injected `data.Service`, and communicates upward via typed messages (`PushViewMsg`, `PopViewMsg`, `ShowConfirmMsg`). Data loading is extracted to `data/Loader`. Components and commands packages are untouched. Monitor model wrapped via adapter.

**Tech Stack:** Go, bubbletea v2, lipgloss v2, bubbles v2, bbolt

**Spec:** `docs/superpowers/specs/2025-05-30-tui-rewrite-design.md`

---

## Confirmed Design Decisions

These decisions were confirmed during plan review and amend the original design spec:

| # | Issue | Decision |
|---|-------|----------|
| 1 | KeyMap circular import (`views` can't import `tui`) | **Move KeyMap to `internal/tui/keys/` package.** Single source of truth — both `tui` and `views` import `keys`. No ViewKeys adapter struct needed. |
| 2 | App rewrite strategy (long non-compiling window) | **Direct rewrite in Task 9**, accept non-compilation until Task 24. |
| 3 | Dead code test files (`listview_test.go`, `detailview_test.go`) | **Delete alongside source files** in Task 1. |
| 4 | Existing `app_test.go` (uses old view enum API) | **Rewrite in Task 9** alongside `app.go`. Add explicit monitor forwarding test. |
| 5 | `palette_test.go` references `CommandPalette` | **Update in Task 10** when creating `palette.go` — rename to `Palette`. |
| 6 | Monitor non-key message forwarding (subtle critical behavior) | **Add explicit test in Task 9** verifying non-key messages forwarded to monitor regardless of active view. |
| 7 | `ViewDataRefreshMsg` handling by views | **Add `Refresh() tea.Cmd` to ViewComponent interface.** Each view implements it to re-fire its own data loads. App calls `router.Active().Refresh()` on `ViewDataRefreshMsg`. |

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `internal/tui/keys/keymap.go` | KeyMap struct + defaults + config loading (moved from `tui/keymap.go`) |
| `internal/tui/router.go` | Navigation stack (push/pop/replace/breadcrumb) |
| `internal/tui/data/service.go` | `Service` interface for data loading |
| `internal/tui/data/loader.go` | Concrete `Loader` implementation |
| `internal/tui/views/view.go` | `ViewComponent` interface (with `Refresh()`) + shared helpers (`truncate`) |
| `internal/tui/views/base_list.go` | `BaseListView` composing `ResourceTable` with key handling |
| `internal/tui/views/base_detail.go` | `BaseDetailView` with scroll management |
| `internal/tui/views/monitor.go` | `MonitorAdapter` wrapping `internal/monitor/ui` |
| `internal/tui/views/palette.go` | Migrated from `views/command.go` (renamed + implements ViewComponent-compatible API) |

### Rewritten Files
| File | What Changes |
|------|-------------|
| `internal/tui/app.go` | Full rewrite: thin router shell (~250 lines) |
| `internal/tui/app_test.go` | Full rewrite: uses new Router API, adds monitor forwarding test |
| `internal/tui/messages/messages.go` | Add nav/overlay message types |
| `internal/tui/views/dashboard.go` | Becomes full `tea.Model` with `Refresh()` |
| `internal/tui/views/deployments.go` | Becomes full `tea.Model` with `BaseListView` + `Refresh()` |
| `internal/tui/views/deployment_detail.go` | Becomes full `tea.Model` with `BaseDetailView` + `Refresh()` |
| `internal/tui/views/leases.go` | Becomes full `tea.Model` with `BaseListView` + `Refresh()` |
| `internal/tui/views/lease_detail.go` | Becomes full `tea.Model` with `BaseDetailView` + `Refresh()` |
| `internal/tui/views/providers.go` | Becomes full `tea.Model` with `BaseListView` + `Refresh()` |
| `internal/tui/views/provider_detail.go` | Becomes full `tea.Model` with `BaseDetailView` + `Refresh()` |
| `internal/tui/views/governance.go` | Becomes full `tea.Model` with `BaseListView` + `Refresh()` |
| `internal/tui/views/proposal_detail.go` | Becomes full `tea.Model` with `BaseDetailView` + `Refresh()` |
| `internal/tui/views/staking.go` | Becomes full `tea.Model` with `BaseListView` + `Refresh()` |
| `internal/tui/views/validator_detail.go` | Becomes full `tea.Model` with `BaseDetailView` + `Refresh()` |
| `internal/tui/views/logviewer.go` | Minor: accepts `keys.KeyMap`, internal key handling |
| `internal/tui/views/help.go` | Minor: accepts `keys.KeyMap`, internal key handling |
| `internal/tui/views/palette_test.go` | Rename `CommandPalette` → `Palette` references |

### Deleted Files
| File | Reason |
|------|--------|
| `internal/tui/views/tx.go` | Dead code (placeholder, never rendered) |
| `internal/tui/views/query.go` | Dead code (placeholder, never rendered) |
| `internal/tui/views/listview.go` | Unused abstraction (replaced by `base_list.go`) |
| `internal/tui/views/detailview.go` | Unused abstraction (replaced by `base_detail.go`) |
| `internal/tui/views/listview_test.go` | Tests dead code (deleted with source) |
| `internal/tui/views/detailview_test.go` | Tests dead code (deleted with source) |
| `internal/tui/views/command.go` | Renamed to `palette.go` |
| `internal/tui/keymap.go` | Moved to `internal/tui/keys/keymap.go` |
| `internal/tui/keymap_test.go` | Moved to `internal/tui/keys/keymap_test.go` |

### Unchanged Files
| File | Reason |
|------|--------|
| `internal/tui/sync_bridge.go` | No changes needed (imports updated for `keys` package) |
| `internal/tui/commands/registry.go` | No changes needed |
| `internal/tui/components/*.go` | All kept as-is |

---

## Task Execution Order

Tasks must be executed in order. Each task builds on the previous one. The project will not compile between tasks 2.5-9 (infrastructure setup). Task 2.5 (KeyMap move) should compile on its own. Starting from task 10, each task makes the project compile and run with progressively more views working.

**Total: 29 tasks** (1, 2, 2.5, 3-28)

---

### Task 1: Delete Dead Code

**Files:**
- Delete: `internal/tui/views/tx.go`
- Delete: `internal/tui/views/query.go`
- Delete: `internal/tui/views/listview.go`
- Delete: `internal/tui/views/detailview.go`
- Delete: `internal/tui/views/listview_test.go`
- Delete: `internal/tui/views/detailview_test.go`
- Delete: `internal/tui/views/command.go`

- [ ] **Step 1: Delete dead and superseded files (including their tests)**

```bash
cd /Users/amr/go/src/github.com/akash-network/akt
rm internal/tui/views/tx.go
rm internal/tui/views/query.go
rm internal/tui/views/listview.go
rm internal/tui/views/detailview.go
rm internal/tui/views/listview_test.go
rm internal/tui/views/detailview_test.go
rm internal/tui/views/command.go
```

- [ ] **Step 2: Commit**

```bash
git add -A internal/tui/views/
git commit -m "refactor(tui): delete dead code and unused abstractions

Remove tx.go, query.go (placeholders never rendered), listview.go,
detailview.go (unused abstractions) and their test files, command.go
(will be replaced by palette.go in subsequent commit)."
```

---

### Task 2: Add Navigation Messages

**Files:**
- Modify: `internal/tui/messages/messages.go`

- [ ] **Step 1: Add navigation, overlay, and log stream message types**

Add the following types to the existing `messages.go` file, after the existing message types. These are the messages views will return to communicate with the App:

```go
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
```

Add the required imports to the import block:

```go
import (
	tea "github.com/charmbracelet/bubbletea/v2"
	"pkg.akt.dev/akt/internal/tui/components"
)
```

Note: `PushViewMsg.View` uses `tea.Model` instead of `views.ViewComponent` to avoid a circular import between `messages` and `views`. The App will type-assert to `ViewComponent` at runtime.

- [ ] **Step 2: Commit**

```bash
git add internal/tui/messages/messages.go
git commit -m "feat(tui): add navigation and overlay message types

Add PushViewMsg, PopViewMsg, ShowConfirmMsg, ShowToastMsg,
StartLogStreamMsg, StopLogStreamMsg. These enable views to
communicate with the App without direct coupling."
```

---

### Task 2.5: Move KeyMap to `internal/tui/keys/` Package

**Files:**
- Move: `internal/tui/keymap.go` → `internal/tui/keys/keymap.go`
- Move: `internal/tui/keymap_test.go` → `internal/tui/keys/keymap_test.go`
- Modify: `internal/tui/app.go` (update imports)
- Modify: `internal/tui/sync_bridge.go` (update imports if needed)

**Rationale:** Both `tui` and `views` packages need access to `KeyMap`. Moving it to a shared `keys` package eliminates the circular import without duplicating the type.

- [ ] **Step 1: Create `internal/tui/keys/` directory and move files**

Create the `keys` package directory. Move `keymap.go` and `keymap_test.go` into it. Change `package tui` to `package keys` in both files. Update the `KeyMapFromConfig` function and any references.

- [ ] **Step 2: Update imports in `app.go` and any other files**

Update `internal/tui/app.go` to import `pkg.akt.dev/akt/internal/tui/keys` and reference `keys.KeyMap`, `keys.DefaultKeyMap()`, `keys.KeyMapFromConfig()`. Check `sync_bridge.go` for any KeyMap references (none expected, but verify).

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/amr/go/src/github.com/akash-network/akt && go build ./internal/tui/keys/ && go build ./internal/tui/
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/keys/ internal/tui/keymap.go internal/tui/keymap_test.go internal/tui/app.go
git commit -m "refactor(tui): move KeyMap to internal/tui/keys/ package

Breaks the circular import between tui and views. Both packages
can now import keys.KeyMap directly. No behavior changes."
```

---

### Task 3: Create ViewComponent Interface and Helpers

**Files:**
- Create: `internal/tui/views/view.go`

- [ ] **Step 1: Create the ViewComponent interface and shared helpers**

```go
package views

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea/v2"

	"pkg.akt.dev/akt/internal/tui/components"
)

// ViewComponent is the contract every navigable view must satisfy.
// It extends tea.Model with methods the App shell uses for chrome
// rendering (breadcrumb, footer hints) and layout (resize).
type ViewComponent interface {
	tea.Model

	// SetSize is called when the terminal resizes. w and h are the
	// available content area dimensions (header/footer already subtracted).
	SetSize(w, h int)

	// Breadcrumb returns the navigation label for this view.
	// Examples: "Deployments", "Deployment #12345", "Lease Detail"
	Breadcrumb() string

	// ShortHelp returns the footer hint pairs for this view.
	ShortHelp() []components.HintPair

	// Refresh returns a tea.Cmd that re-fires data loads for this view.
	// Called by the App when ViewDataRefreshMsg is received (sync bridge
	// detected a store change). Views that load data in Init() should
	// re-fire those same loads here. Views with no data loading return nil.
	Refresh() tea.Cmd
}

// Truncate shortens s to maxLen characters, appending "..." if truncated.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// CommaGroup formats an integer with comma separators.
func CommaGroup(n int64) string {
	if n < 0 {
		return "-" + CommaGroup(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	return CommaGroup(n/1000) + "," + fmt.Sprintf("%03d", n%1000)
}

// CmdFunc is a convenience for creating a tea.Cmd that returns a message.
func CmdFunc(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/view.go
git commit -m "feat(tui): add ViewComponent interface and shared helpers

ViewComponent extends tea.Model with SetSize, Breadcrumb, and
ShortHelp. Add Truncate, CommaGroup, CmdFunc helpers to eliminate
duplication across views."
```

---

### Task 4: Create Data Service

**Files:**
- Create: `internal/tui/data/service.go`
- Create: `internal/tui/data/loader.go`

- [ ] **Step 1: Create the data service interface**

Create `internal/tui/data/service.go` with the `Service` interface. This defines the contract for data loading that views depend on. Each method returns a `tea.Cmd` that produces the corresponding message from `messages/messages.go`.

Read the existing data loading functions in `app.go` (lines 1527-1688) to extract their exact signatures and behavior. The `Service` interface must match these 12 functions.

- [ ] **Step 2: Create the concrete Loader**

Create `internal/tui/data/loader.go` with the `Loader` struct that implements `Service`. Move the 12 data loading functions from `app.go` into methods on `Loader`:

- `LoadDeployments` — queries `store.ListDeployments` with optional filter
- `LoadLeases` — queries `store.ListLeases` with optional filter  
- `LoadDeploymentLeases` — queries `store.ListLeases` for a specific dseq
- `LoadBids` — queries `store.ListBids` for a specific dseq
- `LoadProviders` — queries `lightClient.Provider().Providers()`
- `LoadProposals` — queries `lightClient.Gov().Proposals()`
- `LoadTallies` — queries `lightClient.Gov().TallyResult()` for each voting-period proposal
- `LoadValidators` — queries `lightClient.Staking().Validators()`
- `LoadStakingPool` — queries `lightClient.Staking().Pool()`
- `LoadBalance` — queries `lightClient.Bank().AllBalances()`, finds uakt, scales to AKT
- `LoadStoreStats` — queries `store.Stats()`
- `LoadSyncState` — queries `store.GetSyncState()`

Each method must handle nil store/lightClient gracefully (return a no-op Cmd or error message).

- [ ] **Step 3: Commit**

```bash
git add internal/tui/data/
git commit -m "feat(tui): extract data loading into data.Service

Move 12 data loading functions from app.go into data.Loader.
Views inject data.Service to load their own data via Init()/Update()."
```

---

### Task 5: Create BaseListView

**Files:**
- Create: `internal/tui/views/base_list.go`

- [ ] **Step 1: Create BaseListView**

`BaseListView` wraps `components.ResourceTable` and handles the common list view key bindings (j/k cursor, g/G jump, / search). Concrete list views embed it and add domain-specific columns, row mapping, and action keys.

```go
package views

import (
	"github.com/charmbracelet/bubbles/v2/key"
	tea "github.com/charmbracelet/bubbletea/v2"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/keys"
)

// BaseListView provides common list behavior (cursor, scroll, filter)
// by composing a ResourceTable. Concrete list views embed this and
// add their own column definitions and key handlers.
type BaseListView struct {
	Table components.ResourceTable
	Keys  keys.KeyMap
	W, H  int
}

func NewBaseListView(cfg components.ResourceTableConfig, km keys.KeyMap) BaseListView {
	return BaseListView{
		Table: components.NewResourceTable(cfg),
		Keys:  km,
	}
}

func (b *BaseListView) SetSize(w, h int) {
	b.W, b.H = w, h
	b.Table.SetSize(w, h)
}

func (b *BaseListView) SetRows(rows []components.TableRow) {
	b.Table.SetRows(rows)
}

// Update handles common list navigation keys. Returns a tea.Cmd
// (always nil for cursor movement). Concrete views should call this
// as a fallback after handling their own keys.
func (b *BaseListView) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, b.Keys.CursorDown):
			b.Table.CursorDown()
		case key.Matches(msg, b.Keys.CursorUp):
			b.Table.CursorUp()
		}
	}
	return nil
}

func (b BaseListView) View() string {
	return b.Table.View()
}

func (b BaseListView) Cursor() int {
	return b.Table.Cursor()
}

func (b BaseListView) SelectedID() string {
	return b.Table.SelectedID()
}
```

Note: `keys.KeyMap` comes from `internal/tui/keys/` (created in Task 2.5). This avoids the circular import between `tui` and `views` — both import `keys` instead.

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/base_list.go
git commit -m "feat(tui): add BaseListView composing ResourceTable

Handles j/k cursor movement, g/G jump, search. Concrete list views
embed this and add domain-specific columns and action keys."
```

---

### Task 6: Create BaseDetailView

**Files:**
- Create: `internal/tui/views/base_detail.go`

- [ ] **Step 1: Create BaseDetailView**

`BaseDetailView` provides scroll management (j/k) and the common "back hint" rendering that all detail views share. It extracts the scroll pattern currently duplicated in 7 files.

```go
package views

import (
	"fmt"

	"github.com/charmbracelet/bubbles/v2/key"
	tea "github.com/charmbracelet/bubbletea/v2"
)

// BaseDetailView provides common detail view behavior (scrolling).
// Concrete detail views embed this and render their own content
// as a []string of lines, then call VisibleWindow() to slice.
type BaseDetailView struct {
	Keys   key.Binding // scroll up/down bindings
	Scroll int
	W, H   int
}

func NewBaseDetailView() BaseDetailView {
	return BaseDetailView{}
}

func (b *BaseDetailView) SetSize(w, h int) {
	b.W, b.H = w, h
}

// Update handles j/k scroll keys. Returns nil Cmd.
func (b *BaseDetailView) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "j", "down":
			b.Scroll++
		case "k", "up":
			if b.Scroll > 0 {
				b.Scroll--
			}
		}
	}
	return nil
}

// VisibleWindow returns the visible slice of lines based on scroll
// position and available height. visibleH is the number of lines
// that fit in the viewport.
func (b *BaseDetailView) VisibleWindow(lines []string, visibleH int) []string {
	if visibleH <= 0 {
		return nil
	}
	start := b.Scroll
	if start >= len(lines) {
		start = max(0, len(lines)-1)
	}
	end := start + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	// Clamp scroll to prevent over-scrolling
	if start > 0 && end == len(lines) {
		start = max(0, len(lines)-visibleH)
		b.Scroll = start
		end = len(lines)
	}
	return lines[start:end]
}

// ScrollHint returns a scroll indicator string if content overflows.
func (b BaseDetailView) ScrollHint(totalLines, visibleH int) string {
	if totalLines <= visibleH {
		return ""
	}
	return fmt.Sprintf("  ↕ %d/%d", b.Scroll+1, totalLines-visibleH+1)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/base_detail.go
git commit -m "feat(tui): add BaseDetailView with scroll management

Extracts the scroll pattern duplicated in 7 detail view files into
a single composable base. Handles j/k scroll and visible window slicing."
```

---

### Task 7: Create MonitorAdapter

**Files:**
- Create: `internal/tui/views/monitor.go`

- [ ] **Step 1: Create MonitorAdapter wrapping internal/monitor/ui**

```go
package views

import (
	tea "github.com/charmbracelet/bubbletea/v2"

	"pkg.akt.dev/akt/internal/tui/components"
)

// MonitorAdapter wraps the existing internal/monitor/ui tea.Model
// to satisfy the ViewComponent interface without modifying the
// monitor package.
type MonitorAdapter struct {
	Inner tea.Model
	w, h  int
}

func NewMonitorAdapter(inner tea.Model) *MonitorAdapter {
	return &MonitorAdapter{Inner: inner}
}

func (m *MonitorAdapter) Init() tea.Cmd {
	return m.Inner.Init()
}

func (m *MonitorAdapter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Inner, cmd = m.Inner.Update(msg)
	return m, cmd
}

func (m *MonitorAdapter) View() string {
	return m.Inner.View()
}

func (m *MonitorAdapter) SetSize(w, h int) {
	m.w, m.h = w, h
	// The monitor model expects a WindowSizeMsg for resizing.
	// Subtract 3 for the TUI status bar that the monitor doesn't know about.
	m.Inner, _ = m.Inner.Update(tea.WindowSizeMsg{
		Width:  w,
		Height: h - 3,
	})
}

func (m *MonitorAdapter) Breadcrumb() string {
	return "Monitor"
}

func (m *MonitorAdapter) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "move"},
		{Key: "tab", Desc: "switch dashboard"},
		{Key: "1/2/3", Desc: "sub-tab"},
		{Key: "r", Desc: "refresh"},
		{Key: "esc", Desc: "back"},
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/monitor.go
git commit -m "feat(tui): add MonitorAdapter wrapping monitor/ui model

Implements ViewComponent by delegating to the existing monitor model.
No changes to internal/monitor/ package."
```

---

### Task 8: Create Router

**Files:**
- Create: `internal/tui/router.go`
- Create: `internal/tui/router_test.go`

- [ ] **Step 1: Create the Router with navigation stack**

The Router manages a stack of `ViewComponent` values. The active view (top of stack) receives `Update()` calls. `Push()` adds a view, `Pop()` removes the top, `Replace()` swaps the top. Breadcrumb joins all stack labels.

Read the design spec for exact method signatures. The Router must handle:
- Push: append to stack, call `SetSize` on new view, return `view.Init()`
- Pop: remove top, return nil (previous view is already initialized)
- Replace: swap top, call `SetSize`, return `view.Init()`
- SetSize: propagate only to active (top) view
- Update: delegate to active view, replace top with returned model
- View: return active view's `View()`
- Breadcrumb: join all stack `.Breadcrumb()` values with ` > `
- Active: return top of stack

- [ ] **Step 2: Write Router tests**

Test the following scenarios:
- Push increases stack depth
- Pop decreases stack depth (min 1)
- Replace keeps same depth
- Breadcrumb joins labels
- Update delegates to active view
- SetSize propagates to active only
- Pop when depth=1 is a no-op

- [ ] **Step 3: Commit**

```bash
git add internal/tui/router.go internal/tui/router_test.go
git commit -m "feat(tui): add Router navigation stack

Stack-based view navigation with push/pop/replace. Delegates
Update/View to active view. Breadcrumb built from stack labels."
```

---

### Task 9: Rewrite App — Thin Router Shell

**Files:**
- Rewrite: `internal/tui/app.go`

This is the critical task. The new `app.go` must:

1. **Preserve all existing public API**: `Run()`, `RunMonitor()`, `Config` struct
2. **Use Router for view management** instead of the view enum
3. **Intercept navigation messages** from views (`PushViewMsg`, `PopViewMsg`)
4. **Intercept overlay messages** (`ShowConfirmMsg`, `ShowToastMsg`, `StartLogStreamMsg`)
5. **Keep monitor model forwarding** — non-key messages forwarded to monitor when it exists
6. **Keep overlay priority** — confirm > logViewer > help > palette
7. **Keep chrome rendering** — header, nav bar, breadcrumb, footer from active view's `ShortHelp()`
8. **Keep log stream lifecycle** — `startLogStream`/`stopLogStream`/`streamLogs`
9. **Keep toast system**
10. **Keep sync bridge integration**

- [ ] **Step 1: Rewrite app.go**

The new `App` struct:

```go
type App struct {
	keys       KeyMap
	router     Router
	palette    *views.Palette
	confirm    *components.ConfirmDialog
	help       *views.HelpOverlay
	logView    *views.LogViewer
	toast      *components.Toast
	standalone bool

	// Monitor model (nil if no RPC). Forwarded non-key messages
	// regardless of active view to keep WS/tick chains alive.
	monitor tea.Model

	// Sync bridge — connects pubsub events to the sync engine.
	bridge *syncBridge

	// Data service — injected into views for data loading.
	data data.Service

	// Chrome state
	chainID     string
	rpcEndpoint string
	account     string
	version     string
	syncActive  bool

	// Log stream lifecycle (requires keyring/provider auth)
	logCtx    context.Context
	logCancel context.CancelFunc
	logStream *rest.ServiceLogs
	keyring   sdkkeyring.Keyring
	clientCtx sdkclient.Context
	dataStore store.Store
	resolvedCtx *aktctx.Context

	width, height int
}
```

The `Init()` method:
- Push initial dashboard view onto router
- Fire monitor.Init() if available
- Fire syncBridge.waitForEvent() if available
- Fire data.LoadStoreStats(), data.LoadSyncState(), data.LoadDeployments(), data.LoadBalance()

The `Update()` method (~100 lines):
1. `WindowSizeMsg` → resize router + overlays
2. Forward non-key messages to monitor if exists
3. Standalone mode: only Ctrl+C, rest to router (which has MonitorAdapter)
4. Overlay priority: confirm > logViewer > help > palette
5. Intercept `PushViewMsg`, `PopViewMsg` → router.Push/Pop
6. Intercept `ShowConfirmMsg` → open confirm
7. Intercept `ShowToastMsg` → show toast
8. Intercept `StartLogStreamMsg` → startLogStream
9. Intercept `StopLogStreamMsg` → stopLogStream
10. Intercept `SyncStateMsg`, `StoreStatsMsg` → update chrome state
11. Intercept `ViewDataRefreshMsg` → re-arm bridge, refresh current view (pop data through to views via router.Update)
12. Intercept `ConfirmMsg`, `CancelMsg` → handle confirm result
13. Intercept `LogLineMsg`, `LogStreamClosedMsg` → forward to logViewer
14. Intercept `ToastExpiredMsg` → clear toast
15. Global keys: Ctrl+C, `:`, Ctrl+P, `?`, 1-6, Esc
16. Delegate everything else to `router.Update(msg)`

The `View()` method:
- If standalone with monitor: render monitor.View() directly
- Otherwise: header + navBar + breadcrumb + router.View()
- Overlay compositing: logViewer, confirm, help, palette on top
- Toast below content

Keep `Run()`, `RunMonitor()`, `buildMonitorModel()`, `buildLightClient()` largely unchanged but create `data.NewLoader()` and inject into views.

- [ ] **Step 2: Rewrite app_test.go**

Rewrite `internal/tui/app_test.go` to use the new Router-based API:
- Replace `newTestApp()` to construct App with Router and nil dependencies
- Replace `a.view == viewXxx` assertions with `a.router.Active()` type assertions
- Update golden file tests if rendering changed (header, nav, breadcrumb, footer)
- **Add explicit monitor forwarding test**: Create a mock `tea.Model`, set it as `a.monitor`, send a non-key message (e.g., a custom tick msg), and verify the mock received it even when a non-monitor view is active. This test guards the critical behavior that keeps WS/tick chains alive.
- Update `testdata/` golden files as needed

Tests will not PASS until views are rewritten (constructors changed), but the test file should COMPILE against the new App API.

- [ ] **Step 3: Verify compilation (expected to fail)**

```bash
cd /Users/amr/go/src/github.com/akash-network/akt && make akt
```

The project will NOT compile yet because views still have the old API. This is expected. The next tasks will rewrite each view.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go internal/tui/testdata/
git commit -m "refactor(tui): rewrite App as thin router shell

Replace 1846-line god object with ~250-line shell. Uses Router
for navigation, delegates to ViewComponent interface. Includes
rewritten tests with explicit monitor forwarding test. Will not
compile until views are rewritten in subsequent tasks."
```

---

### Task 10: Rewrite Palette (from command.go)

**Files:**
- Create: `internal/tui/views/palette.go`
- Modify: `internal/tui/views/palette_test.go`

- [ ] **Step 1: Migrate the command palette**

The palette already has its own `Update()` method. Migrate it from the deleted `command.go` to `palette.go`. The key changes:
- Rename `CommandPalette` to `Palette` (cleaner name)
- Keep the existing `Update()`, `View()`, `Open()`, `Close()`, `Active()` API
- Replace `PaletteKeys` struct — accept `keys.KeyMap` directly (from `internal/tui/keys/`)
- Keep the fuzzy filtering, cursor management, and `CommandSubmitMsg` return

This view is an overlay, NOT a ViewComponent (it doesn't go on the router stack). It's managed directly by the App.

- [ ] **Step 2: Update palette_test.go**

Update `internal/tui/views/palette_test.go` to use the renamed `Palette` type:
- Replace `CommandPalette` → `Palette` in all references
- Replace `NewCommandPalette` → `NewPalette`
- Update constructor arguments to use `keys.KeyMap` instead of `PaletteKeys`

- [ ] **Step 3: Commit**

```bash
git add internal/tui/views/palette.go internal/tui/views/palette_test.go
git commit -m "feat(tui): add Palette view (migrated from command.go)

Renamed CommandPalette to Palette. Uses keys.KeyMap instead of
PaletteKeys. Same behavior: fuzzy search, cursor navigation,
CommandSubmitMsg on enter. Tests updated."
```

---

### Task 11: Rewrite Help Overlay

**Files:**
- Rewrite: `internal/tui/views/help.go`

- [ ] **Step 1: Rewrite help overlay with internal key handling**

The help overlay is another overlay (not on router stack). Add internal `Update()` so it handles its own Esc key instead of the App doing it. Keep the exact same visual output (4 sections: Navigation, Lists, Actions, Overlays).

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/help.go
git commit -m "refactor(tui): help overlay handles own esc key

HelpOverlay.Update() now handles esc internally instead of
requiring the App to check for it."
```

---

### Task 12: Rewrite LogViewer

**Files:**
- Rewrite: `internal/tui/views/logviewer.go`

- [ ] **Step 1: Rewrite log viewer with internal key handling**

The log viewer is an overlay (not on router stack). Add `Update()` that handles:
- Esc → return `StopLogStreamMsg`
- Space → toggle pause
- C → clear lines
- J/K → scroll (when paused)
- G → scroll to bottom
- S → cycle service filter

Keep exact same visual output (header bar, log area, footer hints).

The App handles `StopLogStreamMsg` by canceling the stream and closing the overlay.
The App handles `LogLineMsg` by calling `logView.AppendLine()` and re-arming the stream.

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/logviewer.go
git commit -m "refactor(tui): log viewer handles own keys internally

LogViewer.Update() handles esc/space/c/j/k/G/s. Returns
StopLogStreamMsg on esc for App to handle stream cleanup."
```

---

### Task 13: Rewrite Dashboard View

**Files:**
- Rewrite: `internal/tui/views/dashboard.go`

- [ ] **Step 1: Rewrite dashboard as full tea.Model**

The dashboard becomes a `ViewComponent`. Changes from current:
- Add `Init()` → fires `data.LoadDeployments`, `data.LoadStoreStats`, `data.LoadSyncState`, `data.LoadBalance`
- Add `Update()` → handles `DeploymentsLoadedMsg`, `StoreStatsMsg`, `SyncStateMsg`, `BalanceLoadedMsg`
- Keep `View()` rendering exactly as-is (welcome banner, wallet/deployments/network panels, activity/shortcuts)
- Add `Breadcrumb()` → "Dashboard"
- Add `ShortHelp()` → dashboard-specific hints
- Add `Refresh()` → re-fires the same 4 data loads as `Init()` (called on `ViewDataRefreshMsg`)
- Remove all setter methods (`SetActiveDeployments`, `SetStats`, etc.) — data arrives via messages in `Update()`
- Store a `data.Service` reference for `Init()` loads

The constructor: `NewDashboard(svc data.Service, ctx DashboardContext)` where `DashboardContext` contains the static chrome info (context name, chainID, account, rpcEndpoint, version).

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/dashboard.go
git commit -m "refactor(tui): dashboard is a full tea.Model

Handles own data messages, loads data in Init(). Preserves
exact visual output with welcome banner, panels, sparklines."
```

---

### Task 14: Rewrite Deployments List View

**Files:**
- Rewrite: `internal/tui/views/deployments.go`

- [ ] **Step 1: Rewrite deployments list as full tea.Model**

Embeds `BaseListView`. Changes:
- Add `Init()` → `data.LoadDeployments(owner, filter)`
- Add `Update()` → handles `DeploymentsLoadedMsg`, key handling (enter→push detail, l→start logs, d→show confirm, f→cycle filter)
- Preserve exact column layout (DSEQ, IMAGE, STATE, CPU, MEMORY, GPU, PROVIDER, AGE, ESCROW, COST)
- Add `Breadcrumb()` → "Deployments"
- Add `ShortHelp()` → deployments-specific hints
- Add `Refresh()` → re-fires `data.LoadDeployments(owner, filter)`

On `enter`: creates `DeploymentDetailView` and returns `PushViewMsg`.
On `l`: returns `StartLogStreamMsg`.
On `d`: returns `ShowConfirmMsg` with `ConfirmClose`.
On `f`: calls `cycleFilter()` and re-fires `data.LoadDeployments()`.

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/deployments.go
git commit -m "refactor(tui): deployments list is a full tea.Model

Handles own keys (enter/l/d/f), embeds BaseListView for cursor.
Loads data via data.Service in Init()."
```

---

### Task 15: Rewrite Deployment Detail View

**Files:**
- Rewrite: `internal/tui/views/deployment_detail.go`

- [ ] **Step 1: Rewrite deployment detail as full tea.Model**

Embeds `BaseDetailView`. Changes:
- Add `Init()` → `data.LoadDeploymentLeases()` + `data.LoadBids()`
- Add `Update()` → handles `LeasesLoadedMsg`, `BidsLoadedMsg`, key handling (esc→pop, tab→next tab, 1-4→direct tab)
- Preserve exact 4-tab layout (overview, lease, escrow, endpoints)
- Add `Breadcrumb()` → "Deployments > Detail"
- Add `ShortHelp()` → deployment detail hints
- Add `Refresh()` → re-fires `data.LoadDeploymentLeases()` + `data.LoadBids()`

On `esc`: returns `PopViewMsg`.
On `tab`: cycles tab 0→1→2→3→0.
On `1-4`: direct tab jump.

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/deployment_detail.go
git commit -m "refactor(tui): deployment detail is a full tea.Model

Handles own keys (esc/tab/1-4/j/k), embeds BaseDetailView for
scroll. 4 tabs: overview, lease, escrow, endpoints."
```

---

### Task 16: Rewrite Leases List View

**Files:**
- Rewrite: `internal/tui/views/leases.go`

- [ ] **Step 1: Rewrite leases list as full tea.Model**

Same pattern as deployments list. Embeds `BaseListView`.
- `Init()` → `data.LoadLeases(owner, filter)`
- `Update()` → handles `LeasesLoadedMsg`, keys (enter→push detail, l→start logs, f→cycle filter)
- Preserve exact columns (DSEQ, GSEQ, OSEQ, PROVIDER, STATE, PRICE, ESCROW, OPENED)
- `Refresh()` → re-fires `data.LoadLeases(owner, filter)`

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/leases.go
git commit -m "refactor(tui): leases list is a full tea.Model"
```

---

### Task 17: Rewrite Lease Detail View

**Files:**
- Rewrite: `internal/tui/views/lease_detail.go`

- [ ] **Step 1: Rewrite lease detail as full tea.Model**

Embeds `BaseDetailView`.
- `Init()` → nil (data passed via constructor)
- `Update()` → handles esc→pop, j/k scroll
- Preserve exact 5 sections (Lease, Order, Settlement, Provider Status, Endpoints)
- `Refresh()` → nil (data passed via constructor, no reload needed)

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/lease_detail.go
git commit -m "refactor(tui): lease detail is a full tea.Model"
```

---

### Task 18: Rewrite Providers List View

**Files:**
- Rewrite: `internal/tui/views/providers.go`

- [ ] **Step 1: Rewrite providers list as full tea.Model**

Embeds `BaseListView`.
- `Init()` → `data.LoadProviders()`
- `Update()` → handles `ProvidersLoadedMsg`, enter→push detail
- Preserve exact columns (HOST, REGION, GPU, CPU, MEMORY, LEASES, AUDIT, VERSION)
- `Refresh()` → re-fires `data.LoadProviders()`

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/providers.go
git commit -m "refactor(tui): providers list is a full tea.Model"
```

---

### Task 19: Rewrite Provider Detail View

**Files:**
- Rewrite: `internal/tui/views/provider_detail.go`

- [ ] **Step 1: Rewrite provider detail as full tea.Model**

Embeds `BaseDetailView`.
- `Init()` → nil
- `Update()` → esc→pop, j/k scroll
- Preserve exact 2 sections (Provider info, Attributes)
- `Refresh()` → nil (data passed via constructor)

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/provider_detail.go
git commit -m "refactor(tui): provider detail is a full tea.Model"
```

---

### Task 20: Rewrite Governance List View

**Files:**
- Rewrite: `internal/tui/views/governance.go`

- [ ] **Step 1: Rewrite governance list as full tea.Model**

Embeds `BaseListView`.
- `Init()` → `data.LoadProposals()`
- `Update()` → handles `ProposalsLoadedMsg`, `TallyLoadedMsg`, enter→push detail, v→show vote confirm
- Preserve exact columns (#, TITLE, STATUS, YES, NO, ABSTAIN, VETO, ENDS)
- On `ProposalsLoadedMsg`: filter voting-period proposals and fire `data.LoadTallies()`
- `Refresh()` → re-fires `data.LoadProposals()`

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/governance.go
git commit -m "refactor(tui): governance list is a full tea.Model"
```

---

### Task 21: Rewrite Proposal Detail View

**Files:**
- Rewrite: `internal/tui/views/proposal_detail.go`

- [ ] **Step 1: Rewrite proposal detail as full tea.Model**

Embeds `BaseDetailView`.
- `Init()` → nil
- `Update()` → esc→pop, j/k scroll, v→show vote confirm
- Preserve exact layout (proposal info + 4 tally progress bars)
- `Refresh()` → nil (data passed via constructor)

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/proposal_detail.go
git commit -m "refactor(tui): proposal detail is a full tea.Model"
```

---

### Task 22: Rewrite Staking List View

**Files:**
- Rewrite: `internal/tui/views/staking.go`

- [ ] **Step 1: Rewrite staking list as full tea.Model**

Embeds `BaseListView`.
- `Init()` → `data.LoadValidators()`
- `Update()` → handles `ValidatorsLoadedMsg`, `StakingPoolMsg`, enter→push detail, d→show delegate confirm
- Preserve exact columns (#, MONIKER, POWER, VP%, COMMISSION, UPTIME, SIGNED)
- `Refresh()` → re-fires `data.LoadValidators()`

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/staking.go
git commit -m "refactor(tui): staking list is a full tea.Model"
```

---

### Task 23: Rewrite Validator Detail View

**Files:**
- Rewrite: `internal/tui/views/validator_detail.go`

- [ ] **Step 1: Rewrite validator detail as full tea.Model**

Embeds `BaseDetailView`.
- `Init()` → nil
- `Update()` → esc→pop, j/k scroll, d→show delegate confirm
- Preserve exact 2 sections (validator info, your delegation placeholder)
- `Refresh()` → nil (data passed via constructor)

- [ ] **Step 2: Commit**

```bash
git add internal/tui/views/validator_detail.go
git commit -m "refactor(tui): validator detail is a full tea.Model"
```

---

### Task 24: Verify Build and Fix Compilation Errors

**Files:**
- Possibly touch any file with compilation issues

- [ ] **Step 1: Build the project**

```bash
cd /Users/amr/go/src/github.com/akash-network/akt && make akt
```

Fix any compilation errors. Common issues will be:
- Import path adjustments for the new `data/` package
- Type mismatches between old and new view constructors  
- Missing method implementations on ViewComponent
- KeyMap reference resolution (circular import avoidance)

- [ ] **Step 2: Run tests**

```bash
cd /Users/amr/go/src/github.com/akash-network/akt && go test ./internal/tui/...
```

Fix any test failures. Existing tests in `internal/tui/app_test.go` will need updating since the App API changed.

- [ ] **Step 3: Commit**

```bash
git add -A internal/tui/
git commit -m "fix(tui): resolve compilation errors after rewrite

Fix import paths, type mismatches, and KeyMap resolution."
```

---

### Task 25: Update E2E Tests

**Files:**
- Modify: `e2e/tui_test.go`

- [ ] **Step 1: Update TUI e2e tests**

The TUI e2e tests create an App and verify basic behavior. Update them to work with the new App constructor and Router-based navigation.

- [ ] **Step 2: Run full test suite**

```bash
cd /Users/amr/go/src/github.com/akash-network/akt && go test ./...
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tui_test.go
git commit -m "test(tui): update e2e tests for rewritten TUI

Adapt test helpers to new App constructor and Router navigation."
```

---

### Task 26: Write Router and View Tests

**Files:**
- Create: `internal/tui/views/base_list_test.go`
- Create: `internal/tui/views/base_detail_test.go`
- Create: `internal/tui/views/view_test.go`

- [ ] **Step 1: Write tests for shared view infrastructure**

Test:
- `Truncate()` edge cases (empty, shorter than max, exact max, longer)
- `CommaGroup()` (0, small, thousands, millions, negative)
- `CmdFunc()` returns correct message
- `BaseListView` cursor movement (up/down, bounds)
- `BaseDetailView` scroll (up/down, VisibleWindow slicing, ScrollHint)

- [ ] **Step 2: Run tests**

```bash
go test ./internal/tui/views/ -v -run TestTruncate,TestCommaGroup,TestBaseList,TestBaseDetail
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/views/*_test.go
git commit -m "test(tui): add tests for view infrastructure

Cover Truncate, CommaGroup, BaseListView cursor, BaseDetailView
scroll and VisibleWindow."
```

---

### Task 27: Smoke Test and Cleanup

- [ ] **Step 1: Manual smoke test**

Build and run the TUI:
```bash
cd /Users/amr/go/src/github.com/akash-network/akt && make akt
.cache/bin/akt
```

Verify:
- Dashboard renders correctly with panels
- Number keys 1-6 switch views
- Esc returns to dashboard
- Enter on list item opens detail
- Esc from detail returns to list
- `:` opens command palette
- `?` opens help
- Log viewer works (if provider available)
- Monitor view works (if RPC available)
- Confirm dialogs appear on `d`, `v`

- [ ] **Step 2: Remove any remaining dead code or TODO comments**

Search for orphaned references to the deleted view types, unused imports, etc.

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "refactor(tui): complete TUI rewrite to Elm Architecture

Every view is now a self-contained tea.Model with Init/Update/View.
App reduced from 1846 to ~250 lines. Navigation via Router stack.
Data loading extracted to data.Service. All behavior preserved."
```

---

### Task 28: Update AICHANGELOG.md

**Files:**
- Modify: `AICHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Add an entry describing the TUI rewrite:
- What: Rewrote `internal/tui/` to follow bubbletea's Elm Architecture
- Why: Previous implementation was a 1,846-line god object with 624-line Update()
- How: Each view is now a self-contained `tea.Model`, navigation via stack-based `Router`, data loading extracted to `data.Service`
- Impact: ~4,500 lines rewritten, 4 dead files deleted, all behavior preserved

- [ ] **Step 2: Commit**

```bash
git add AICHANGELOG.md
git commit -m "docs: add AICHANGELOG entry for TUI rewrite"
```
