# Dashboard Design-Parity Rework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the TUI dashboard to match the HTML prototype pixel-for-pixel — a 3-column grid with welcome banner, wallet panel, active deployments panel, network panel, recent activity feed, and shortcuts panel, all using title-in-border panel styling.

**Architecture:** The dashboard is a single-file rewrite of `internal/tui/views/dashboard.go`. It renders a 2D grid layout using manual line-by-line construction (lipgloss `JoinHorizontal` for columns, newline-joined strings for rows). Panels use a custom `titledPanel()` helper that draws `┌─ TITLE ─┐` style borders with the title inset into the top border line. Sparklines use Unicode block characters (`▁▂▃▄▅▆▇█`). All data arrives through setter methods — no chain queries in the view.

**Tech Stack:** Go, lipgloss v2, bubbletea v2

**Design reference (authoritative):** The HTML prototype screenshot (Image 1 from the user) showing the full dashboard at 162×42 terminal size. The JSX source: `design/prototype/tui-views.jsx` lines 8-124 and `design/prototype/tui-data.jsx`.

---

## File Map

| File | Action | Task |
|------|--------|------|
| `internal/tui/components/panel.go` | Create | Task 1 |
| `internal/tui/components/sparkline.go` | Create | Task 2 |
| `internal/tui/views/dashboard.go` | Rewrite | Task 3 |
| `internal/tui/views/dashboard_test.go` | Rewrite | Task 4 |
| `internal/tui/app.go` | Modify | Task 5 |
| `internal/tui/messages/messages.go` | Modify | Task 5 |

---

### Task 1: Titled Panel Component

Create a reusable `titledPanel()` component that renders content inside a bordered box with the title embedded in the top border line, matching the design's `┌─ WALLET ─┐` pattern.

**Files:**
- Create: `internal/tui/components/panel.go`

- [ ] **Step 1: Create the titled panel component**

Create `internal/tui/components/panel.go`:

```go
package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// TitledPanel renders content inside a bordered box with the title embedded
// in the top border line:
//
//	┌─ TITLE ──────────────────┐
//	│ content line 1            │
//	│ content line 2            │
//	└───────────────────────────┘
//
// The width is the outer width including border characters.
func TitledPanel(title, content string, width int) string {
	borderFg := lipgloss.NewStyle().Foreground(theme.Slate700)
	titleStyle := lipgloss.NewStyle().Foreground(theme.Slate500).Bold(true)

	innerW := width - 4 // 2 border chars + 2 padding spaces

	// ─── Top border: ┌─ TITLE ─...─┐
	titleRendered := titleStyle.Render(title)
	titleVisualW := lipgloss.Width(titleRendered)
	fillW := innerW - titleVisualW - 2 // 2 = spaces around title
	if fillW < 0 {
		fillW = 0
	}
	topLine := borderFg.Render("┌─ ") + titleRendered + borderFg.Render(" "+strings.Repeat("─", fillW)+"─┐")

	// ─── Content lines: │ content │
	contentLines := strings.Split(content, "\n")
	var body strings.Builder
	for _, line := range contentLines {
		lineW := lipgloss.Width(line)
		pad := innerW - lineW
		if pad < 0 {
			pad = 0
		}
		body.WriteString(borderFg.Render("│") + " " + line + strings.Repeat(" ", pad) + " " + borderFg.Render("│") + "\n")
	}

	// ─── Bottom border: └─...─┘
	bottomLine := borderFg.Render("└" + strings.Repeat("─", width-2) + "┘")

	return topLine + "\n" + body.String() + bottomLine
}
```

- [ ] **Step 2: Build and verify**

```bash
go build ./internal/tui/components/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/components/panel.go
git commit -m "feat(tui): add TitledPanel component with title-in-border styling"
```

---

### Task 2: Sparkline Component

Create a sparkline component that renders a mini bar chart using Unicode block characters (`▁▂▃▄▅▆▇█`), used for price history and block times in the dashboard.

**Files:**
- Create: `internal/tui/components/sparkline.go`

- [ ] **Step 1: Create the sparkline component**

Create `internal/tui/components/sparkline.go`:

```go
package components

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// sparkBlocks are the Unicode block elements from shortest to tallest.
var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline renders a mini bar chart from a slice of float64 values.
// Each value maps to one character column. The color parameter controls
// the foreground of the bars. Width limits the number of data points
// shown (rightmost values if data exceeds width).
func Sparkline(data []float64, width int, color lipgloss.Color) string {
	if len(data) == 0 || width <= 0 {
		return ""
	}

	// Use the rightmost `width` data points.
	start := 0
	if len(data) > width {
		start = len(data) - width
	}
	visible := data[start:]

	// Find min/max for normalization.
	minVal, maxVal := visible[0], visible[0]
	for _, v := range visible {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	span := maxVal - minVal
	if span == 0 {
		span = 1 // avoid division by zero; all values are equal
	}

	style := lipgloss.NewStyle().Foreground(color)
	var b strings.Builder
	for _, v := range visible {
		normalized := (v - minVal) / span
		idx := int(normalized * float64(len(sparkBlocks)-1))
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		if idx < 0 {
			idx = 0
		}
		b.WriteRune(sparkBlocks[idx])
	}

	return style.Render(b.String())
}
```

- [ ] **Step 2: Build and verify**

```bash
go build ./internal/tui/components/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/components/sparkline.go
git commit -m "feat(tui): add Sparkline component for mini bar charts"
```

---

### Task 3: Dashboard Full Rewrite

Rewrite the dashboard to match the design reference with a 3-column grid layout, welcome banner, wallet panel, active deployments panel, network panel, recent activity feed, and shortcuts panel.

**Files:**
- Rewrite: `internal/tui/views/dashboard.go`

- [ ] **Step 1: Read the current file and the design reference**

Read:
- `internal/tui/views/dashboard.go` (current — will be fully replaced)
- `design/prototype/tui-views.jsx` lines 8-124 (design reference)
- `design/prototype/tui-data.jsx` (mock data shapes)

- [ ] **Step 2: Rewrite dashboard.go**

The new dashboard must render this layout (at 162×42 terminal):

```
┌────────────────────────────────────────────────────────────────────────────────────────────┐
│ [AKT logo]  welcome back, {account}                                    ● SYNCED   V1.2.0  │
│             connected to {context} · rpc {endpoint} · last sync {ago}                      │
└────────────────────────────────────────────────────────────────────────────────────────────┘
┌─ WALLET ──────────┐  ┌─ ACTIVE · N ──────────┐  ┌─ NETWORK ─────────────┐
│ address   addr     │  │ depl-name    cost      │  │ height       16M      │
│ liquid    1,284 AKT│  │ depl-name    cost      │  │ block time   6.04s    │
│ staked    3,400 AKT│  │ depl-name    cost      │  │ active prov  184      │
│ rewards   +12 AKT  │  │ depl-name    cost      │  │ bonded       64.2M    │
│ escrow    246 AKT  │  │                        │  │ inflation    13.84%   │
│                    │  │ monthly burn  295 AKT  │  │                       │
│ price (24h)        │  │                        │  │ block times (last 60) │
│ ▁▂▃▅▆▇▅▃▂▁▂▄▆     │  │ [1] full · [D] new     │  │ ▁▂▃▅▇▅▃▂▁▂▄▆▇▅▃     │
│ $3.41 ▲ 4.2%      │  │                        │  │ avg 6.04s · max 7.8s  │
└────────────────────┘  └────────────────────────┘  └───────────────────────┘
┌─ RECENT ACTIVITY ─────────────────────────────┐  ┌─ SHORTCUTS ───────────┐
│ 14:02:11 TX  MsgSendManifest · dseq 178...    │  │ [1-6]  primary nav    │
│ 14:01:48 TX  MsgCreateLease · dseq 178...     │  │ [↵]    drill down     │
│ 13:58:22 EVT bid received · provider ...      │  │ [esc]  pop back       │
│ 13:55:09 TX  MsgCreateDeployment · dseq ...   │  │ [:]    command palette│
│ 11:41:00 GOV proposal #91 entered voting      │  │ [?]    help overlay   │
│ 09:12:34 TX  MsgCloseDeployment · dseq ...    │  │ [D]    new deployment │
└───────────────────────────────────────────────┘  └───────────────────────┘
```

**Grid layout construction:**
- `colW = (width - 4) / 3` (3 columns with 1-space gaps)
- Row 1: Welcome banner, full width — use `TitledPanel` with no title or a simple bordered box
- Row 2: Wallet panel (colW) | Active panel (colW) | Network panel (colW)  — join with `lipgloss.JoinHorizontal`
- Row 3: Recent Activity (2*colW + 2) | Shortcuts (colW) — join with `lipgloss.JoinHorizontal`

**Data interface (setters):**

Keep existing:
- `SetContext(name, chainID, account string)`
- `SetStats(s *store.StoreStats)`
- `SetSyncState(s *store.SyncState)`
- `SetActiveDeployments(deployments []*store.DeploymentRecord)`
- `SetSyncBridgeActive(active bool)`
- `SetBalance(amount string)`
- `SetValidatorCount(active int)`
- `SetProposalCount(voting int)`

Add new:
- `SetWallet(liquid, staked, rewards, escrow string)` — wallet breakdown
- `SetPrice(price string, change string)` — AKT price + % change
- `SetPriceHistory(data []float64)` — sparkline data for price
- `SetBlockTimes(data []float64, avg, max string)` — sparkline + stats
- `SetNetworkInfo(blockTime string, activeProv int, bonded string, inflation string)` — additional network fields
- `SetRecentActivity(entries []ActivityEntry)` — activity feed data
- `SetRPCEndpoint(endpoint string)` — for the welcome banner
- `SetVersion(version string)` — for the version badge

Where `ActivityEntry` is defined in `dashboard.go`:
```go
// ActivityEntry represents a single item in the recent activity feed.
type ActivityEntry struct {
    Time string // "14:02:11"
    Kind string // "tx", "evt", "gov"  
    Text string // "MsgSendManifest · dseq 17834201 · provider overclock.akash.pub"
}
```

**Panel rendering (each panel is its own method):**

`renderWelcomeBanner(w int)` — Full-width bordered panel:
- Left: AKT ASCII art logo in red
- Middle: "welcome back, {account}" bold + connection info muted
- Right: "● SYNCED" green badge + version badge

`renderWalletPanel(colW int)` — TitledPanel("WALLET", ..., colW):
- KV rows: address, liquid, staked, rewards (green if positive), escrow
- Gap
- "price (24h)" label + sparkline + "$3.41 ▲ 4.2%"

`renderActivePanel(colW int)` — TitledPanel("ACTIVE · N", ..., colW):
- List of deployment names with costs (right-aligned)
- Dashed separator
- "monthly burn" total (bold)
- "press [1] for full list · [D] new deployment"

`renderNetworkPanel(colW int)` — TitledPanel("NETWORK", ..., colW):
- KV rows: height (bold), block time, active prov., bonded, inflation
- Gap
- "block times (last 60)" label + sparkline + "avg X · max Y"

`renderRecentActivity(w int)` — TitledPanel("RECENT ACTIVITY", ..., w):
- Time (muted) | Kind badge (TX=green, EVT=blue, GOV=purple) | Text

`renderShortcuts(colW int)` — TitledPanel("SHORTCUTS", ..., colW):
- Key pill + description rows

**KV row helper**: Right-justify values within the panel. Format: `label` (muted, left) ... space fill ... `value` (right). Use `fmt.Sprintf("%-*s%*s", labelW, label, valueW, value)` or calculate manually.

**Key pill helper**: `[key]` rendered as a bordered badge — `Foreground(Slate300).Background(Slate800).Padding(0,1).Render(key)` matching the prototype's `<KeyPill>` component. For the `D` key, use red accent instead.

- [ ] **Step 3: Build and verify**

```bash
go build ./internal/tui/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/views/dashboard.go
git commit -m "feat(tui): rewrite dashboard to match design — grid layout with titled panels"
```

---

### Task 4: Update Dashboard Tests

Rewrite the dashboard tests to match the new layout.

**Files:**
- Rewrite: `internal/tui/views/dashboard_test.go`

- [ ] **Step 1: Rewrite tests**

The tests should verify:
- `View()` renders non-empty output
- Welcome banner contains account name after `SetContext()`
- Welcome banner shows "SYNCED" after `SetSyncState()`
- Wallet panel shows balance fields after `SetWallet()`
- Active panel shows deployment names after `SetActiveDeployments()`
- Network panel shows block height after `SetSyncState()`
- Shortcuts panel contains key labels ("1-6", "esc", ":", "?", "D")
- Panel borders use `┌─` and `─┐` characters (title-in-border pattern)
- Recent activity shows entries after `SetRecentActivity()`

- [ ] **Step 2: Run tests**

```bash
go test ./internal/tui/views/... -count=1 -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/views/dashboard_test.go
git commit -m "test(tui): update dashboard tests for new grid layout"
```

---

### Task 5: Wire New Data Setters in app.go

Add data-fetching for the new dashboard fields (wallet balance, staking rewards, price, network info, recent activity from action log).

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/messages/messages.go`

- [ ] **Step 1: Add new message types**

In `messages.go`, add:

```go
// WalletLoadedMsg carries wallet balance breakdown.
type WalletLoadedMsg struct {
    Liquid  string
    Staked  string
    Rewards string
    Escrow  string
    Err     error
}

// NetworkInfoMsg carries additional network data.
type NetworkInfoMsg struct {
    BlockTime  string
    ActiveProv int
    Bonded     string
    Inflation  string
    Err        error
}
```

- [ ] **Step 2: Wire data into dashboard in app.go**

In the `Init()` or on `ViewDataRefreshMsg`, set the new dashboard fields from available data:

- `SetRPCEndpoint()` from `cfg.RPCEndpoint`
- `SetVersion()` from build info
- Call `SetWallet()` with data from bank balance query (if available)
- Call `SetNetworkInfo()` from staking/mint params (if available)
- Pass `SetRecentActivity()` with entries from the action log (if available)

For data not yet available, the dashboard should show dashes gracefully.

- [ ] **Step 3: Build and run full test suite**

```bash
go build ./...
go test ./internal/tui/... -count=1
```

- [ ] **Step 4: Update golden files**

```bash
go test ./internal/tui -count=1 -args -update
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/messages/messages.go internal/tui/testdata/
git commit -m "feat(tui): wire dashboard data setters for wallet, network, activity"
```

---

## Task Dependencies

```
Task 1 (Panel component) ──┐
Task 2 (Sparkline component)├── Task 3 (Dashboard rewrite) ── Task 4 (Tests) ── Task 5 (Wire data)
```

Tasks 1 and 2 are independent and can run in parallel. Tasks 3-5 are sequential.

---

## Implementation Notes

**Title-in-border**: Lipgloss v2 has no `BorderTitle` API, so we construct the top border manually: `┌─ TITLE ─...─┐`. The `TitledPanel()` helper handles this.

**Sparklines**: Use Unicode block characters `▁▂▃▄▅▆▇█` (8 levels). Normalize data to [0,7] range. Each data point = one character column.

**Grid layout**: No CSS grid in terminals. Build each row as `lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", middlePanel, " ", rightPanel)`. Row 3 has the activity panel at `2*colW + 2` width (spans 2 columns + gap).

**AKT ASCII art**: The design shows a small monospaced AKT logo. Use a compact 3-line ASCII art:
```
 ▄▀█ █▄▀ ▀█▀
 █▀█ █ █  █
```

**Key pills**: Render as `[key]` with `Background(Slate800).Foreground(Slate300).Render(key)`. The `D` key uses `Background(AccentRed).Foreground(Slate950)`.

**Right-justified KV rows**: In the Wallet and Network panels, labels are left-aligned and values are right-aligned within the panel width. Calculate padding: `innerW - lipgloss.Width(label) - lipgloss.Width(value)` and fill with spaces.

**Data graceful degradation**: All panels must render correctly with empty/nil data — show dashes or "—" for missing values. The dashboard works in degraded mode when no chain connection exists.
