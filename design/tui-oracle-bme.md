# Oracle/BME Dashboard — UX Design

## Overview

The Oracle/BME dashboard is the third hub tab in `akt monitor`, providing real-time visibility into the Akash oracle price feeds and the BME (Burn-Mint Engine) state. It presents a two-column layout: oracle aggregated prices and price health on the left, BME status, vault state, and ledger records on the right. The dashboard is accessible via `akt monitor oracle` or `akt monitor bme` (aliases), or by pressing Tab to cycle to the Oracle/BME hub tab. It uses the shared `pretty.Render*` functions so the TUI output is visually identical to `akt q bme status --output pretty`.

## Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                        akt monitor - Akash Network                               │
│  Network   Provider   [Oracle/BME]                                               │
├─────────────────────────────────────┬────────────────────────────────────────────┤
│                                     │                                            │
│  ── Aggregated Price ──             │  ── BME Status ──                          │
│                                     │                                            │
│  Denom:       uakt                  │  Status:            Healthy                │
│  TWAP:        0.003125              │  Mints:             Allowed                │
│  Median:      0.003100              │  Refunds:           Allowed                │
│  Min:         0.003050              │  Collateral Ratio:  1.523                  │
│  Max:         0.003200              │  Thresholds:                               │
│  Sources:     4                     │      Warn:          1.100                  │
│  Deviation:   12 bps                │      Halt:          1.050                  │
│  Timestamp:   2026-03-23 10:15 UTC  │                                            │
│                                     │  ── Vault State ──                         │
│  ── Price Health ──                 │                                            │
│                                     │  Balances:          1,234.56 AKT           │
│  Healthy:         yes               │  Total Burned:      500.00 AKT             │
│  Min Sources:     yes               │  Total Minted:      480.00 USDC            │
│  Deviation OK:    yes               │  Remint Credits:    20.00 AKT              │
│  Total Sources:   4                 │                                            │
│  Healthy Sources: 4                 │  ── Ledger ──                              │
│                                     │                                            │
│                                     │  ROUTE       ID          STATUS  BURNED …  │
│                                     │  uakt→usdc   src/h/1     e       500 AKT   │
│                                     │  usdc→uakt   src/h/2     p       200 USDC  │
│                                     │  uakt→usdc   src/h/3     c:halt  100 AKT   │
│                                     │                                            │
├─────────────────────────────────────┴────────────────────────────────────────────┤
│  Tab dashboard  j/k scroll  r refresh  ? help                                    │
│  RPC: https://rpc.akashnet.net:443 [WS]                                          │
└──────────────────────────────────────────────────────────────────────────────────┘
```

## Component Specifications

### Title Bar
- **Content**: `"akt monitor - Akash Network"` centered across full terminal width.
- **Style**: `titleStyle` (theme.Heading — Slate100, bold).

### Hub Tab Bar
- **Tabs**: `Network`, `Provider`, `Oracle/BME`.
- **Active tab**: `tabActiveStyle` (theme.NavTabActive — AccentRed background, Slate950 foreground, bold, padding 0,1).
- **Inactive tabs**: `tabInactiveStyle` (theme.NavTabInactive — Slate400 foreground, padding 0,1).
- **Layout**: Tabs joined with two-space gap, rendered at full terminal width.

### Two-Column Layout
- **Split**: `leftW = width / 2`, `rightW = width - leftW`. Minimum total width: 40 columns.
- **Join**: `lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, rightStyled)` — columns are top-aligned.
- **Each column**: Constrained with `lipgloss.NewStyle().Width(colW)`.

### Left Column — Oracle Panel (`renderOraclePanel`)

#### Loading States
- **Module not active**: `"Oracle module not active on this network."` in `mutedStyle` (Slate500).
- **Loading**: `"Loading oracle data..."` in `mutedStyle`.
- **Version detected, waiting**: `"Oracle {version} detected — waiting for aggregated prices..."` in `mutedStyle`.

#### Aggregated Prices Section (via `pretty.RenderAggregatedPrice`)
- **Section header**: `"Aggregated Price"` rendered by `pretty.Section()` — bold, AccentRed, bottom border in Slate700.
- **Key-value pairs**: `pretty.KV()` with Slate500 labels (16-char width) and Slate200 bold values.
- **Fields**: Denom, TWAP (bold), Median, Min, Max, Sources (count), Deviation (bps), Timestamp.
- **Price formatting**: Full decimal precision, trailing zeros stripped via `trimDecTrailingZeros`.

#### Price Health Section (via `pretty.RenderAggregatedPrice`)
- **Section header**: `"Price Health"` rendered by `pretty.Section()`.
- **Healthy field**: `"yes"` in green (`StyleGreen` / GreenColor #22c55e) or `"no"` in red (`StyleRed` / AccentRed #ff4136) with failure reasons appended.
- **Boolean checks** (Min Sources, Deviation OK): `"yes"` green / `"no"` red via `boolCheck()`.
- **Counts** (Total Sources, Healthy Sources): Plain numeric values.

### Right Column — BME Panel (`renderBMEPanel`)

#### BME Status Section (via `pretty.RenderBMEStatus`)
- **Section header**: `"BME Status"` rendered by `pretty.Section()`.
- **Status field**: Color-coded by `mintStatusColor()`:
  - `Healthy` → green (GreenColor #22c55e)
  - `Warning` → yellow (YellowColor #eab308)
  - `Halt CR` → red (AccentRed #ff4136)
  - `Halt Oracle` → red (AccentRed #ff4136)
- **Mints / Refunds**: `"Allowed"` green or `"Halted"` red via `formatAllowedHalted()`.
- **Collateral Ratio**: Bold value via `pretty.Bold()`.
- **Thresholds**: Nested sub-section via `pretty.KVHeader()` + `pretty.SubKV()` for Warn and Halt values.

#### Vault State Section (via `pretty.RenderBMEVaultState`)
- **Section header**: `"Vault State"` rendered by `pretty.Section()`.
- **Fields**: Balances, Total Burned, Total Minted, Remint Credits.
- **Amount formatting**: All values formatted with `pretty.FormatCoins()` / `pretty.FormatCoin()` — micro-denominated values scaled per §10.7 rules.

#### Ledger Section (via `pretty.RenderBMELedger`)
- **Table columns**: ROUTE, ID, STATUS, BURNED, MINTED, SPREAD, REMINT ACCRUED, REMINT ISSUED.
- **Column alignment**: BURNED, MINTED, SPREAD, REMINT ACCRUED, REMINT ISSUED are right-aligned.
- **Status indicators**:
  - `e` (executed) → green (StyleGreen)
  - `p` (pending) → yellow (StyleYellow)
  - `c` / `c:{reason}` (canceled) → red (StyleRed)
- **Burned/Minted formatting**: `formatCoinPrice()` renders as `"amount denom @price"` for executed records.
- **Empty state**: `"(no ledger records)"` in dim style.

### Status Bar (non-embedded mode)
- **Help line**: `"q: quit | Tab/1-3: switch | j/k: select | Enter: expand | Esc: collapse"` in `helpStyle` (Slate500).
- **RPC line**: `"RPC: {endpoint} [{mode}]"` where mode is `WS` or `HTTP`, in `statusBarStyle` (Slate500).

## Color Tokens Used

| Token | Source | Hex | Usage |
|-------|--------|-----|-------|
| `theme.Heading` / `Slate100` | `theme.go:32` | `#f4f4f5` | Title bar text |
| `theme.AccentRed` | `theme.go:39` | `#ff4136` | Active tab bg, section headers, unhealthy/halted status |
| `theme.Slate950` | `theme.go:23` | `#09090b` | Active tab foreground |
| `theme.Slate400` | `theme.go:29` | `#a1a1aa` | Inactive tab text |
| `theme.Slate500` | `theme.go:28` | `#71717a` | Muted text, KV labels, status bar |
| `theme.Slate200` | `theme.go:31` | `#e4e4e7` | KV values (bold) |
| `theme.Slate700` | `theme.go:27` | `#3f3f46` | Section header bottom border |
| `theme.GreenColor` | `theme.go:47` | `#22c55e` | Healthy, Allowed, boolean "yes" |
| `theme.YellowColor` | `theme.go:49` | `#eab308` | Warning status |

## Interaction

| Key | Action |
|-----|--------|
| `Tab` | Cycle to next hub dashboard (Oracle/BME → Network) |
| `Shift-Tab` | Cycle to previous hub dashboard (Oracle/BME → Provider) |
| `j` / `k` | Scroll within the dashboard content |
| `r` | Force refresh oracle and BME data |
| `q` | Quit the monitor |
| `?` | Toggle help overlay |

**Focus behavior**: The Oracle/BME dashboard is a single scrollable viewport — no sub-tab navigation. The hub tab bar at the top indicates the active dashboard.

**Transitions**:
- **From**: Hub tab bar cycling (Tab/Shift-Tab from Network or Provider dashboards).
- **To**: Tab/Shift-Tab cycles to Network or Provider dashboards.
- **Direct launch**: `akt monitor oracle` or `akt monitor bme` opens the monitor with Oracle/BME as the initial hub tab.

## Data Sources

| Data | REST Endpoint | Refresh Interval |
|------|---------------|------------------|
| Aggregated prices | `/akash/oracle/v2/aggregated-price/{denom}` | 30 seconds |
| Price feed history | `/akash/oracle/v2/prices` | 2 minutes |
| BME status | `/akash/bme/v1/status` | 30 seconds |
| Vault state | `/akash/bme/v1/vault-state` | 30 seconds |
| Ledger records | `/akash/bme/v1/ledger` | 2 minutes |

**State storage**: All oracle and BME data is held in `OracleState` within the `ViewContext`:
- `OracleState.Aggregated` — map of denom → `EventAggregatedPrice` (from oracle v2 types).
- `OracleState.Prices` — slice of `OraclePriceEntry` (denom, price, source, timestamp).
- `OracleState.Events` — capped log of recent oracle events (max 100, newest first).
- `OracleState.BMEStatus` — `*bmetypes.QueryStatusResponse`.
- `OracleState.BMELedger` — `[]bmetypes.QueryLedgerRecordEntry`.
- `OracleState.Version` — detected oracle API version (`"v1"`, `"v2"`, `"none"`, or `""` for undetermined).

## Implementation Reference

| Component | File |
|-----------|------|
| Dashboard renderer | `internal/monitor/ui/view.go` — `renderOracleBMEDashboard()`, `renderOraclePanel()`, `renderBMEPanel()` |
| Oracle/BME state types | `internal/monitor/ui/state.go` — `OracleState`, `OraclePriceEntry`, `OracleEvent` |
| BME pretty renderers | `internal/output/pretty/bme.go` — `RenderBMEStatus()`, `RenderBMEVaultState()`, `RenderBMELedger()` |
| Oracle pretty renderers | `internal/output/pretty/oracle.go` — `RenderOraclePrices()`, `RenderAggregatedPrice()` |
| Style aliases | `internal/monitor/ui/styles.go` — all monitor styles aliased from `internal/ui/theme/theme.go` |
| Theme tokens | `internal/ui/theme/theme.go` |
| Hub tab bar | `internal/monitor/ui/view.go` — `renderHubTabBar()` |
| View context | `internal/monitor/ui/view.go` — `ViewContext` struct (lines 69–100) |

## SPEC.md Cross-Reference

| Section | Title | Coverage |
|---------|-------|----------|
| §2.6 | Monitor Command | Hub tab structure, `akt monitor oracle`/`akt monitor bme` aliases, endpoint resolution, shared flags |
| §8.3.12 | Oracle/BME Monitor View | Two-column layout wireframe, data sources, refresh intervals, color coding rules, amount formatting, component list |
