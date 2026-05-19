# Application Shell Layout — UX Design

## Overview

The application shell is the root visual frame of the akt TUI. It divides the terminal into three fixed regions — a 1-line header bar, a dynamically-sized main content area, and a 1-3 line status bar pinned to the bottom. The shell is implemented as the `App` model in bubbletea and is responsible for rendering chrome around whichever view is currently active. On every `WindowSizeMsg` the shell recalculates region heights and propagates the new dimensions to all child views.

## Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ akt · mainnet:akashnet-2 · akash1abc...xyz              ⎡ 18234567 ⎤  ● synced │  <- Header (1 line, Slate900 bg)
├──────────────────────────────────────────────────────────────────────────────────┤
│ Dashboard  1 Deployments  2 Leases  3 Providers  4 Monitor  5 Gov  6 Staking  D │  <- Nav bar (1 line)
│──────────────────────────────────────────────────────────────────────────────────│  <- HRule (Slate700)
│ Deployments                                                                      │  <- Breadcrumb (1 line)
│                                                                                  │
│                                                                                  │
│                          ┌─────────────────────────┐                             │
│                          │                         │                             │
│                          │   Active View Content   │                             │  <- Main area (fills remaining)
│                          │                         │                             │
│                          │                         │                             │
│                          └─────────────────────────┘                             │
│                                                                                  │
│                                                                                  │
│──────────────────────────────────────────────────────────────────────────────────│  <- Footer HRule (Slate700)
│ j/k move  ↵ open  l logs  d close  / search  D new                              │  <- Footer hints (1 line)
└──────────────────────────────────────────────────────────────────────────────────┘
```

### Detailed Region Breakdown

```
Line 0:   ┌─ HEADER ──────────────────────────────────────────────────────────────┐
          │ [AppName] · [CtxName]:[ChainID] · [Account]    [⎡ BlockH ⎤]  [Sync] │
          └────────────────────────────────────────────────────────────────────────┘

Line 1:   [Dashboard] [1 Deployments] [2 Leases] [3 Providers] ...   D deploy
Line 2:   ──────────────────────────────────────────────────────────── (HRule)
Line 3:    Deployments / Detail                                       (Breadcrumb)

Lines 4    ┌─ MAIN AREA ─────────────────────────────────────────────────────────┐
  to       │                                                                      │
 H-6:      │  height = terminal_height - chromeHeight (10)                        │
           │  Overlays (palette, confirm, help, log viewer) render here            │
           └──────────────────────────────────────────────────────────────────────┘

Line H-2:  ──────────────────────────────────────────────────────────── (HRule)
Line H-1:  [key desc]  [key desc]  [key desc]  ...                    (Hints)
```

## Component Specifications

### Header Bar (1 line)

| Element | Position | Style | Content Source |
|---------|----------|-------|----------------|
| App name | Left, first element | `HeaderAppName` (AccentRed, bold) | Hardcoded `"akt"` |
| Separator | Between elements | `HeaderMeta` (Slate500) | Literal `" · "` |
| Context name | Left, after app name | `HeaderContext` (Slate200, bold) | `resolvedCtx.Name` or em-dash fallback |
| Chain ID | Left, after context | `HeaderMeta` (Slate500) | `":" + resolvedCtx.Network.ChainID` |
| Account | Left, after chain ID | `HeaderContext` (Slate200, bold) | `resolvedCtx.DefaultAccount` (omitted if empty) |
| Block height | Right | `HeaderMeta` brackets + `HeaderValue` number | `syncState.LastBlockHeight` or em-dash |
| Sync indicator | Right, after block | `SyncOK` (GreenColor) or `HeaderMeta` | `"● synced"` or `"○ no sync"` |

The entire header line is rendered inside `HeaderStyle` which applies `Background(Slate900)` and `Padding(0, 1)`. The gap between left and right groups is filled with spaces to span `width - 2` (accounting for padding).

### Nav Bar (2 lines: tabs + horizontal rule)

| Element | Style | Behavior |
|---------|-------|----------|
| Active tab | `NavTabActive` (AccentRed bg, Slate950 fg, bold, pad 0,1) | Matches current `activeView` |
| Inactive tab | `NavTabInactive` (Slate400 fg, pad 0,1) | All other tabs |
| Dashboard pseudo-tab | `NavTabActive` | Prepended when `view == viewDashboard` |
| Deploy button | AccentRed foreground | Right-aligned `"D deploy"` |
| Horizontal rule | `Slate700` foreground | Full-width `"─"` repeated |

Tab labels are formatted as `"N Name"` (e.g., `"1 Deployments"`). The 6 nav items are defined in the `navItems` slice:

| Key | Label | View |
|-----|-------|------|
| 1 | Deployments | `viewDeployments` |
| 2 | Leases | `viewLeases` |
| 3 | Providers | `viewProviders` |
| 4 | Monitor | `viewMonitor` |
| 5 | Governance | `viewGovernance` |
| 6 | Staking | `viewStaking` |

### Breadcrumb (1 line)

| View | Rendered Text |
|------|---------------|
| Dashboard | ` Dashboard` |
| Deployments | ` Deployments` |
| DeploymentDetail | ` Deployments / Detail` |
| Leases | ` Leases` |
| Providers | ` Providers` |
| Monitor | ` Monitor` |
| Governance | ` Governance` |
| Staking | ` Staking` |

Segments use `BreadcrumbActive` (Slate200, bold). The separator `" / "` uses `BreadcrumbSeparator` (Slate600). All breadcrumbs are indented 1 space from the left edge.

### Main Content Area

- **Height calculation**: `contentH = terminal_height - chromeHeight` where `chromeHeight = 10` (header=1, navBar=2, breadcrumb=1, footer=2, newlines=4). Minimum 1 line.
- **Pinning**: Content is wrapped in a lipgloss style with `.Height(contentH).MaxHeight(contentH)` to ensure the footer stays at the terminal bottom regardless of content length.
- **Overlay priority** (highest to lowest): Command palette > Help overlay > Confirm dialog > Log viewer > Active view.

### Footer (2 lines: horizontal rule + hints)

The footer is rendered by `components.Footer()` which outputs a horizontal rule followed by a newline and hint pairs.

| Element | Style |
|---------|-------|
| Horizontal rule | `Slate700` foreground, full-width `"─"` |
| Key labels | `FooterKey` (Slate400, bold) |
| Key descriptions | `FooterDesc` (Slate600) |
| Accent keys | `AccentRed`, bold (for primary actions like Deploy, Vote) |

Hints are view-specific. Each view defines its own `[]HintPair`:

| View | Hints |
|------|-------|
| Dashboard | `1-6 navigate`, `: command`, `? help`, `D deploy` (accent) |
| Deployments | `j/k move`, `↵ open`, `l logs`, `d close`, `/ search`, `D new` (accent) |
| Leases | `j/k move`, `↵ detail`, `esc back` |
| Providers | `j/k move`, `↵ detail`, `esc back` |
| Monitor | `j/k move`, `tab switch`, `esc back` |
| Governance | `j/k move`, `↵ detail`, `v vote` (accent), `esc back` |
| Staking | `j/k move`, `↵ detail`, `d delegate` (accent), `esc back` |
| DeploymentDetail | `j/k scroll`, `1-4 tabs`, `tab next tab`, `esc back` |
| Palette (active) | `↑/↓ navigate`, `↵ select`, `esc close` |

## Color Tokens Used

| Token | Variable | Hex | Usage |
|-------|----------|-----|-------|
| Header background | `Slate900` | `#18181b` | `HeaderStyle.Background()` |
| App name | `AccentRed` | `#ff4136` | `HeaderAppName.Foreground()` |
| Context/account | `Slate200` | `#e4e4e7` | `HeaderContext.Foreground()` |
| Metadata/separators | `Slate500` | `#71717a` | `HeaderMeta.Foreground()` |
| Block height value | `Slate200` | `#e4e4e7` | `HeaderValue.Foreground()` |
| Sync OK | `GreenColor` | `#22c55e` | `SyncOK.Foreground()` |
| Active tab bg | `AccentRed` | `#ff4136` | `NavTabActive.Background()` |
| Active tab fg | `Slate950` | `#09090b` | `NavTabActive.Foreground()` |
| Inactive tab | `Slate400` | `#a1a1aa` | `NavTabInactive.Foreground()` |
| Horizontal rules | `Slate700` | `#3f3f46` | `HRule()` foreground |
| Breadcrumb text | `Slate200` | `#e4e4e7` | `BreadcrumbActive.Foreground()` |
| Breadcrumb separator | `Slate600` | `#52525b` | `BreadcrumbSeparator.Foreground()` |
| Footer keys | `Slate400` | `#a1a1aa` | `FooterKey.Foreground()` |
| Footer descriptions | `Slate600` | `#52525b` | `FooterDesc.Foreground()` |

## Interaction

### Resize Behavior

On every `tea.WindowSizeMsg`:

1. `a.width` and `a.height` are updated from the message.
2. `a.resize()` is called, which computes `mainH = height - chromeHeight` (min 1) and calls `SetSize(width, mainH)` on every child view: dashboard, deployments, leases, providers, governance, staking, detail, deploymentDetail, logViewer, confirmDialog, helpOverlay, palette.
3. If the monitor model is present, a reduced `WindowSizeMsg` is forwarded with `Height: height - statusBarHeight` (3 lines reserved for the unified footer).

### Monitor View Special Case

When `view == viewMonitor` and the monitor model is present, the shell skips rendering its own header/navBar/breadcrumb. The monitor model renders its own title/tab chrome. Only the unified footer is appended below the monitor content.

### Standalone Mode

When `cfg.Standalone == true` (e.g., `akt monitor`), the command palette and view switching are disabled. Only `Ctrl+C` is intercepted at the app level; all other keys are forwarded directly to the monitor model.

### View Composition Order

The `View()` method assembles the final output as:

```
chrome = header + "\n" + navBar + "\n" + breadcrumb + "\n" + main
output = pin(chrome) + "\n" + footer
```

Where `pin` is a lipgloss style that enforces `Height(contentH)` to lock the footer to the bottom.

## Data Sources

| Data | Source | Refresh |
|------|--------|---------|
| Context name, chain ID, account | `Config.ResolvedCtx` (loaded at startup) | Static for session |
| Block height | `store.SyncState.LastBlockHeight` via `loadSyncState()` | On `ViewDataRefreshMsg` |
| Sync status | Presence of `syncState` (non-nil = synced) | On `SyncStateMsg` |
| Store stats | `store.StoreStats` via `loadStoreStats()` | On `ViewDataRefreshMsg` |

## Implementation Reference

| Component | File |
|-----------|------|
| App model (shell) | `internal/tui/app.go` |
| `renderHeader()` | `internal/tui/app.go:837-893` |
| `renderNavBar()` | `internal/tui/app.go:909-935` |
| `renderBreadcrumb()` | `internal/tui/app.go:937-965` |
| `renderFooter()` | `internal/tui/app.go:671-733` |
| `View()` | `internal/tui/app.go:589-667` |
| `resize()` | `internal/tui/app.go:816-835` |
| Footer component | `internal/tui/components/footer.go` |
| Theme tokens | `internal/ui/theme/theme.go` |

## SPEC.md Cross-Reference

- **Section 8.1 — Application Shell Layout**: Three-region layout (header, main, status bar), header content specification, status bar dynamic sizing (1-3 lines).
- **Section 8.8 — TUI Component Hierarchy**: `App (root model) > Header, Navigation, StatusBar` structure.
