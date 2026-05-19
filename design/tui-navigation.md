# Navigation Model — UX Design

## Overview

The akt TUI uses a flat navigation model based on an `activeView` enum rather than a navigation stack. The root `App` model holds a single `view` field that determines which content is rendered in the main area. Users switch views via number keys (1-6), the command palette (`:`/`Ctrl+P`), or `Esc` to return to the dashboard. The only "drill-down" is from the Deployments list to DeploymentDetail, which uses `Esc` to return to the parent list. There is no general-purpose back stack.

## Wireframe

### View Topology

```
                              ┌─────────────┐
                    Esc       │  Dashboard   │       Esc (from any view)
               ┌──────────── │  (default)   │ ◄──────────────────────┐
               │              └──────┬───────┘                        │
               │                     │                                │
               │    ┌────────────────┼────────────────┐               │
               │    │    Number keys / Command palette │               │
               │    ▼         ▼         ▼         ▼   ▼         ▼     │
          ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
    1 ──► │Deploys  │  │ Leases  │  │Providers│  │ Monitor │  │  Gov    │  │ Staking │
          │         │  │         │  │         │  │         │  │         │  │         │
          └────┬────┘  └─────────┘  └─────────┘  └─────────┘  └─────────┘  └─────────┘
               │ Enter                                2            3            4  5  6
               ▼
          ┌──────────┐
          │Deployment│
          │  Detail  │ ◄── Esc returns to Deployments (not Dashboard)
          └──────────┘
```

### Command Palette Overlay

```
┌──────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│              ┌──────────────────────────────────────────┐                │
│              │ :dep                                      │                │
│              ├──────────────────────────────────────────┤                │
│              │ > Deployments    View all deployments     │                │
│              │   Deploy         Create new deployment    │                │
│              │   Deploy from SDL  Create from SDL file   │                │
│              └──────────────────────────────────────────┘                │
│                                                                          │
│──────────────────────────────────────────────────────────────────────────│
│ ↑/↓ navigate  ↵ select  esc close                                       │
└──────────────────────────────────────────────────────────────────────────┘
```

## Component Specifications

### View Enum

The `activeView` type is an integer enum defined in `internal/tui/app.go`:

| Constant | Value | View Name | Access |
|----------|-------|-----------|--------|
| `viewDashboard` | 0 | Dashboard | Default / Esc from any top-level view |
| `viewDeployments` | 1 | Deployments | Key `1` / command `deployments` |
| `viewLeases` | 2 | Leases | Key `2` / command `leases` |
| `viewProviders` | 3 | Providers | Key `3` / command `providers` |
| `viewMonitor` | 4 | Monitor | Key `4` / command `monitor` |
| `viewGovernance` | 5 | Governance | Key `5` / command `governance` |
| `viewStaking` | 6 | Staking | Key `6` / command `staking` |
| `viewDeploymentDetail` | 7 | Deployment Detail | `Enter` on deployment list item |

### Navigation Triggers

| Trigger | Mechanism | Data Loading Side-Effect |
|---------|-----------|--------------------------|
| Number key `1` | `key.Matches(kmsg, a.keys.Deployments)` | `loadDeployments(store, owner)` |
| Number key `2` | `key.Matches(kmsg, a.keys.Leases)` | `loadLeases(store, owner)` |
| Number key `3` | `key.Matches(kmsg, a.keys.Providers)` | `loadChainProviders(lightClient)` |
| Number key `4` | `key.Matches(kmsg, a.keys.Monitor)` | None (monitor runs continuously) |
| Number key `5` | `key.Matches(kmsg, a.keys.Governance)` | `loadProposals(lightClient)` |
| Number key `6` | `key.Matches(kmsg, a.keys.Staking)` | `loadValidators(lightClient)` |
| `Esc` | `key.Matches(kmsg, a.keys.Back)` | None |
| `Enter` on deployment | `key.Matches(kmsg, a.keys.Select)` | `loadDeploymentLeases()` + `loadBids()` |
| Command palette submit | `views.CommandSubmitMsg` | Varies by command |

### Command Palette Navigation

The command palette is a centered overlay activated by `:` or `Ctrl+P`. It dispatches `views.CommandSubmitMsg` with the selected command value, which is handled by `handleCommand()`:

| Command Input | Aliases | Target View | Data Load |
|---------------|---------|-------------|-----------|
| `dashboard` | `home` | `viewDashboard` | None |
| `deployments` | `dep` | `viewDeployments` | `loadDeployments()` |
| `leases` | — | `viewLeases` | `loadLeases()` |
| `providers` | `prov` | `viewProviders` | `loadChainProviders()` |
| `monitor` | `consensus`, `top` | `viewMonitor` | None |
| `governance` | `gov` | `viewGovernance` | `loadProposals()` |
| `staking` | `validators`, `val` | `viewStaking` | `loadValidators()` |
| `quit` | `q`, `exit` | — | `tea.Quit` |
| `certificates`, `escrow`, `orders`, `bids`, `deploy`, `help` | — | `viewDashboard` | None (not yet implemented) |

### Back Behavior

The `Esc` key behavior depends on the current context:

| Current State | Esc Behavior |
|---------------|--------------|
| Any top-level view (Deployments, Leases, etc.) | Returns to Dashboard |
| Deployment Detail | Returns to Deployments list (special case) |
| Command palette open | Closes palette, stays on current view |
| Help overlay open | Closes help overlay |
| Log viewer open | Closes log viewer |
| Confirm dialog open | Cancels dialog |
| Monitor view (standalone mode) | Quits application (`tea.Quit`) |
| Monitor view (embedded) | Returns to Dashboard via `monitorui.BackMsg` |

### Overlay Priority

When overlays are active, they intercept all key messages. Priority order (highest first):

1. **Confirm dialog** — `a.confirmDialog.Active()` checked first
2. **Log viewer** — `a.logViewer.Active()`
3. **Help overlay** — `a.helpOverlay.Active()`
4. **Command palette** — `a.palette.Active()`
5. **Normal key dispatch** — view switching, cursor navigation, actions

### Deployment Detail Drill-Down

The only hierarchical navigation in the TUI:

1. User is on `viewDeployments` and presses `Enter`.
2. `a.deployments.SelectedRecord()` returns the selected `*store.DeploymentRecord`.
3. `a.deploymentDetail.SetDeployment(rec)` populates the detail view.
4. `a.view` is set to `viewDeploymentDetail`.
5. Two data-loading commands fire: `loadDeploymentLeases()` and `loadBids()`.
6. Within the detail view, `Tab` cycles sub-tabs, `1-4` jump to specific tabs, `j/k` scroll.
7. `Esc` sets `a.view = viewDeployments` (returns to list, not dashboard).

### Monitor View Key Forwarding

When `view == viewMonitor` and the monitor model is present, all key messages are forwarded directly to the monitor model instead of being handled by the app-level dispatcher. This allows the monitor's internal `Tab`/`Shift-Tab` dashboard switching and `1`/`2`/`3` sub-tab navigation to work without conflict. Only `Ctrl+C`, `:`, and `?` are intercepted at the app level before forwarding.

## Color Tokens Used

| Token | Usage |
|-------|-------|
| `NavTabActive` | Active view tab (AccentRed bg, Slate950 fg) |
| `NavTabInactive` | Inactive view tabs (Slate400 fg) |
| `BreadcrumbActive` | Current location text (Slate200, bold) |
| `BreadcrumbSeparator` | `/` separator in breadcrumb (Slate600) |
| `AccentRed` | Deploy button in nav bar |

## Interaction

| Key | Scope | Action |
|-----|-------|--------|
| `1` | Global (non-monitor) | Switch to Deployments |
| `2` | Global (non-monitor) | Switch to Leases |
| `3` | Global (non-monitor) | Switch to Providers |
| `4` | Global (non-monitor) | Switch to Monitor |
| `5` | Global (non-monitor) | Switch to Governance |
| `6` | Global (non-monitor) | Switch to Staking |
| `Esc` | Global | Back to parent (Dashboard or Deployments for detail) |
| `:` | Global | Open command palette |
| `Ctrl+P` | Global | Open command palette |
| `?` | Global | Open help overlay |
| `Enter` | Deployments list | Drill into Deployment Detail |
| `Tab` | Deployment Detail | Cycle to next sub-tab |
| `1-4` | Deployment Detail | Jump to specific sub-tab |
| `Tab`/`Shift-Tab` | Monitor view | Cycle monitor dashboards |

## Data Sources

| Source | Used For |
|--------|----------|
| `store.Store` | Deployment list, lease list, bids, store stats, sync state |
| `aclient.LightClient` | Governance proposals, validators, on-chain providers |
| `Config.ResolvedCtx` | Owner address for filtered queries |
| `commands.Registry` | Command palette entries (fuzzy-filtered) |

## Implementation Reference

| Component | File |
|-----------|------|
| View enum + App model | `internal/tui/app.go:41-53` |
| `Update()` key dispatch | `internal/tui/app.go:420-578` |
| `handleCommand()` | `internal/tui/app.go:778-813` |
| Overlay priority chain | `internal/tui/app.go:357-418` |
| Deployment detail drill-down | `internal/tui/app.go:511-524` |
| Monitor key forwarding | `internal/tui/app.go:441-445` |
| Command palette | `internal/tui/views/` (CommandPalette) |
| Command registry | `internal/tui/commands/registry.go` |
| Nav bar rendering | `internal/tui/app.go:909-935` |
| Breadcrumb rendering | `internal/tui/app.go:937-965` |

## SPEC.md Cross-Reference

- **Section 8.2 — Navigation Model**: Stack-based model described in spec; implementation uses flat enum with special-case back behavior for DeploymentDetail. The spec's "navigation stack" is simplified to a single `activeView` field.
- **Section 8.4 — Command Palette**: Overlay activation, filtering, keyboard handling, registered commands.
- **Section 8.8 — TUI Component Hierarchy**: `Navigation (manages view stack, breadcrumbs)` — in practice, no stack; the breadcrumb is computed from the current `activeView`.
