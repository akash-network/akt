# Resource List & Detail Views — UX Design

## Overview

The resource views are the primary data-browsing surfaces of the akt TUI. They present deployments, leases, and providers as scrollable, filterable tables built on the shared `ResourceTable` component, and provide a drill-down deployment detail view with four sub-tabs. Each list view follows the same visual grammar: a column-header row, a horizontal rule, data rows with a cursor indicator, a bottom rule, and an item count. The deployment detail view replaces the list when the user presses Enter on a deployment row.

## Wireframe — Deployments List

```
  DSEQ       IMAGE          STATE        CPU  MEMORY       GPU  PROVIDER       AGE       COST
────────────────────────────────────────────────────────────────────────────────────────────────
  12345678   web.yaml       │active│     4    8Gi          —    akash1ab…xyz    3d        500uakt
▸ 12345679   api.yaml       │active│     2    4Gi          —    akash1cd…uvw    1d        250uakt
  12345680   ml-train.yaml  │closed│     8    16Gi         A100 akash1ef…rst    14d       1200uakt
────────────────────────────────────────────────────────────────────────────────────────────────
  3 items
```

## Wireframe — Leases List

```
  DSEQ       PROVIDER                            STATE       PRICE              ESCROW       OPENED
──────────────────────────────────────────────────────────────────────────────────────────────────────
▸ 12345679   akash1cd…uvw                        │active│    12.5uakt/block    500uakt      1d
  12345678   akash1ab…xyz                        │active│    25.0uakt/block    1000uakt     3d
  12345680   akash1ef…rst                        │closed│    50.0uakt/block    —            14d
──────────────────────────────────────────────────────────────────────────────────────────────────────
  3 items
```

## Wireframe — Providers List

```
  HOST                          REGION       GPU              CPU      MEMORY       LEASES    AUDIT     VERSION
────────────────────────────────────────────────────────────────────────────────────────────────────────────────
▸ provider1.akash.network       us-west      —                —        —            —         —         —
  provider2.akash.network       eu-central   —                —        —            —         —         —
────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  2 items
```

## Wireframe — Deployment Detail

```
  web.yaml  12345679  │active│  akash1cd…uvw
────────────────────────────────────────────────────────────────────────────────

  1 overview   2 lease   3 escrow   4 endpoints

  Resources
  ──────────────────────────────────────────────
  cpu             4
  memory          8Gi
  gpu             —
  storage         20Gi

  Placement
  ──────────────────────────────────────────────
  provider        akash1cd…uvw
  region          us-west
  uptime          1d 4h
  cost            12.5uakt/block

  SDL
  ──────────────────────────────────────────────
  hash            abc123def456…
  path            /home/user/web.yaml

  esc: back  j/k: scroll  1-4: tabs
```

## Component Specifications

### Deployments List View (`DeploymentsView`)

| Element | Specification |
|---|---|
| **Columns** | 9 total: DSEQ (10w, left), IMAGE (auto, left), STATE (12w, left, custom `StateTag` renderer), CPU (6w, right), MEMORY (8w, right), GPU (10w, left), PROVIDER (auto, left), AGE (10w, right), COST (14w, right) |
| **Column widths** | Fixed widths specified per column; `Width: 0` columns (IMAGE, PROVIDER) auto-fill remaining space equally. Cursor prefix consumes 2 chars; column gaps are 1 space each. |
| **IMAGE column** | Displays `filepath.Base(r.SDLPath)` or "—" if no SDL path |
| **STATE column** | Uses `components.StateTag()` custom renderer: inline `│label│` with color-mapped borders (green=active, yellow=paused, muted=closed) |
| **AGE column** | Relative time via `relativeTime()`: `<1m` → `Ns`, `<1h` → `Nm`, `<24h` → `Nh`, else `Nd` |
| **COST column** | Raw deposit string from `DeploymentRecord.Deposit` or "—" |
| **Empty state** | Centered text: "No deployments. Use 'akt deploy <sdl>' to create one." in Slate500 |
| **Cursor** | `▸` prefix in AccentRed bold on selected row; selected row text in Slate200 bold; normal rows in Slate300 |
| **Filter** | Three-state cycle: `""` (all) → `"active"` → `"closed"` → `""`. Triggered by `CycleFilter()`. Rebuilds rows from full dataset. |
| **Scrolling** | Visible rows = `height - 4` (header + header rule + bottom rule + count). Scroll offset auto-adjusts to keep cursor visible. |
| **Truncation** | Cells exceeding column width are truncated with "…" suffix |

### Leases List View (`LeasesView`)

| Element | Specification |
|---|---|
| **Columns** | 6 total: DSEQ (10w, left), PROVIDER (auto, left), STATE (10w, left, `StateTag`), PRICE (18w, right), ESCROW (12w, right), OPENED (12w, left) |
| **PROVIDER column** | `LeaseRecord.ID.Provider` address or "—" |
| **OPENED column** | Relative time via shared `relativeTime()` |
| **Filter** | Same three-state cycle as deployments: all → active → closed |
| **Empty state** | "No active leases." |

### Providers List View (`ProvidersView`)

| Element | Specification |
|---|---|
| **Columns** | 8 total: HOST (auto, left), REGION (12w, left), GPU (16w, left), CPU (8w, right), MEMORY (10w, right), LEASES (8w, right), AUDIT (8w, right), VERSION (10w, left) |
| **Data binding** | `SetData(ptypes.Providers)` maps chain provider data; region extracted from `Attributes` via `attrValue()`. Resource columns show "—" (TBD — requires provider status queries). |
| **Empty state** | Two-line message: "Provider data requires chain connection.\nUse akt monitor provider for real-time fleet monitoring." |

### Deployment Detail View (`DeploymentDetailView`)

| Element | Specification |
|---|---|
| **Header strip** | `name  DSEQ  │state│  owner` — name from `filepath.Base(SDLPath)` in Secondary, DSEQ in Heading bold, state via `StateTag()`, owner in Muted. Followed by full-width `HRule(w)`. |
| **Tab bar** | 4 tabs: `1 overview`, `2 lease`, `3 escrow`, `4 endpoints`. Active tab: number in AccentRed bold + label in Slate100 bold. Inactive: number in Slate500 + label in Slate400. Tabs separated by 3 spaces. |
| **Overview tab** | Three `SectionWithKV` blocks: Resources (cpu, memory, gpu, storage), Placement (provider, region, uptime, cost), SDL (hash, path). KV labels 16-char fixed width in Slate500, values in Slate200. |
| **Lease tab** | Active Lease section with provider, state, price, opened, gseq, oseq. Bid History section listing all bids with provider (Secondary), price, state tag. |
| **Escrow tab** | Deposit, balance, transferred as KV pairs. Progress bar showing remaining balance percentage (max 60 chars wide) via `components.ProgressBar()`. |
| **Endpoints tab** | Lists all endpoints from all leases. Each endpoint rendered as a `KVBlock` with service, port, url. |
| **Scrolling** | Content scrollable with j/k. Visible lines = `height - 7` (header 2 + tab bar 1 + blank 1 + back hint 2 + padding 1). Minimum 3 visible lines. |
| **Back hint** | `esc: back` in Muted. If content overflows: `j/k: scroll  1-4: tabs` appended in ColorMuted. |
| **Min width** | Clamped to 40 chars minimum |

### Shared ResourceTable Component

| Element | Specification |
|---|---|
| **Header row** | Column names in Slate500, left-aligned within column width, 2-char left indent |
| **Header rule** | Full-width `─` in Slate700 |
| **Data rows** | Normal: Slate300. Selected: Slate200 bold. Cursor prefix: `▸ ` in AccentRed bold (selected) or `  ` (unselected). |
| **Custom renderers** | Columns with `RenderFunc` call the function then manually pad to column width |
| **Bottom rule** | Full-width `─` in Slate700 |
| **Item count** | `N items` in Slate500, 2-char left indent |
| **Column width algorithm** | Two-pass: (1) assign explicit widths, (2) distribute remaining space to `Width: 0` columns. Minimum auto-width: 4 chars per auto column. |

## Color Tokens Used

| Token | Usage |
|---|---|
| `theme.Slate500` | Column headers, KV labels, muted text, item count, inactive tab numbers |
| `theme.Slate700` | Header/bottom rules, empty bar segments |
| `theme.Slate400` | Inactive tab labels |
| `theme.Slate300` | Normal row text, body text |
| `theme.Slate200` | Selected row text, KV values, heading values |
| `theme.Slate100` | Section titles, active tab labels |
| `theme.AccentRed` | Cursor `▸`, active tab numbers, section rules |
| `theme.GreenColor` / `theme.GreenDim` | Active/open state tag text/border |
| `theme.YellowColor` / `theme.YellowDim` | Paused/insufficient_funds state tag text/border |
| `theme.ColorMuted` (`Slate500`) | Scroll hints, back hint |

## Interaction

| Key | Context | Action |
|---|---|---|
| `j` / `↓` | List views | Move cursor down one row |
| `k` / `↑` | List views | Move cursor up one row |
| `g` / `Home` | List views | Jump to first row (`CursorTop()`) |
| `G` / `End` | List views | Jump to last row (`CursorBottom()`) |
| `f` | Deployments, Leases | Cycle state filter: all → active → closed |
| `Enter` | Deployments list | Drill into deployment detail (selected DSEQ) |
| `Esc` | Deployment detail | Return to deployments list |
| `1` / `2` / `3` / `4` | Deployment detail | Switch to overview / lease / escrow / endpoints tab |
| `Tab` / `Shift+Tab` | Deployment detail | Next / previous sub-tab |
| `j` / `k` | Deployment detail | Scroll content up/down |

**Focus behavior**: List views own keyboard focus when active. Deployment detail replaces the list view entirely (not an overlay). Tab switching resets scroll position to 0.

## Data Sources

| View | Data Source | Type |
|---|---|---|
| Deployments | `store.DeploymentRecord` | Local bbolt store populated by `akt deploy` and chain queries |
| Leases | `store.LeaseRecord` | Local bbolt store populated by lease lifecycle events |
| Providers | `ptypes.Providers` (chain query) | On-chain provider registry via LightClient ABCI query |
| Deployment Detail | `store.DeploymentRecord` + `store.LeaseRecord` + `store.BidRecord` | Combined from local store |

## Implementation Reference

| Component | File |
|---|---|
| Deployments list view | `internal/tui/views/deployments.go` |
| Leases list view | `internal/tui/views/leases.go` |
| Providers list view | `internal/tui/views/providers.go` |
| Deployment detail view | `internal/tui/views/deployment_detail.go` |
| ResourceTable component | `internal/tui/components/table.go` |
| KV detail components | `internal/tui/components/kvdetail.go` |
| State tag renderer | `internal/tui/components/statetag.go` |
| Progress bar component | `internal/tui/components/progress.go` |
| Theme / color tokens | `internal/ui/theme/theme.go` |

## SPEC.md Cross-Reference

| Section | Coverage |
|---|---|
| **§8.3.1** Deployments List View | Column layout, state filtering, Enter drill-in, sorting |
| **§8.3.2** Deployment Detail View | Metadata display, lease details, bid table, sub-tab navigation |
| **§8.3.3** Leases List View | Column layout, state filtering, actions |
| **§8.3.4** Providers List View | Column layout, chain data source, detail navigation |
