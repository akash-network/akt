# Keybinding System — UX Design

## Overview

The akt TUI keybinding system provides vim-style keyboard navigation with full configurability. All bindings are defined in a `KeyMap` struct that is initialized from defaults and optionally overridden via Viper configuration (`tui.keybindings: custom` + `tui.custom-keybindings` map). The system uses the `charm.land/bubbles/v2/key` package for binding definition and matching. Bindings are organized into four tiers: global, list navigation, view shortcuts, and context-specific actions.

## Wireframe

### Keybinding Scope Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│  GLOBAL SCOPE (always active)                                           │
│  Ctrl+C quit  :  command palette  Ctrl+P command palette  ? help       │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  VIEW SHORTCUTS (active when no overlay is open)                  │  │
│  │  1 deployments  2 leases  3 providers  4 monitor  5 gov  6 stake │  │
│  │  Esc back to dashboard                                            │  │
│  │                                                                   │  │
│  │  ┌─────────────────────────────────────────────────────────────┐  │  │
│  │  │  LIST NAVIGATION (active in list views)                     │  │  │
│  │  │  j/↓ down  k/↑ up  Enter select                            │  │  │
│  │  │                                                             │  │  │
│  │  │  ┌───────────────────────────────────────────────────────┐  │  │  │
│  │  │  │  CONTEXT ACTIONS (view-specific)                      │  │  │  │
│  │  │  │  d close  u update  l logs  s shell  v vote           │  │  │  │
│  │  │  │  D deploy  f filter  / search  Tab next               │  │  │  │
│  │  │  └───────────────────────────────────────────────────────┘  │  │  │
│  │  └─────────────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │  OVERLAY SCOPE (intercepts all keys when active)                  │  │
│  │  Priority: ConfirmDialog > LogViewer > HelpOverlay > Palette      │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

## Component Specifications

### KeyMap Struct

The `KeyMap` struct in `internal/tui/keymap.go` defines all configurable bindings:

```
KeyMap {
    // Global
    Quit          key.Binding    // ctrl+c
    Command       key.Binding    // :
    CommandSearch key.Binding    // ctrl+p
    Help          key.Binding    // ?
    Back          key.Binding    // esc

    // Navigation (lists, palette, detail views)
    CursorUp     key.Binding    // k, up
    CursorDown   key.Binding    // j, down
    Select       key.Binding    // enter

    // Primary view shortcuts
    Deployments  key.Binding    // 1
    Leases       key.Binding    // 2
    Providers    key.Binding    // 3
    Monitor      key.Binding    // 4
    Governance   key.Binding    // 5
    Staking      key.Binding    // 6

    // View-specific actions
    Close        key.Binding    // d
    Update       key.Binding    // u
    Logs         key.Binding    // l
    Shell        key.Binding    // s
    Vote         key.Binding    // v
    Deploy       key.Binding    // D
    Filter       key.Binding    // f
    Search       key.Binding    // /
    TabNext      key.Binding    // tab
}
```

### Default Keybindings

#### Global Keys

| Binding | Default Key(s) | Help Text | Action |
|---------|---------------|-----------|--------|
| `Quit` | `ctrl+c` | quit | Quit application immediately |
| `Command` | `:` | command palette | Open command palette overlay |
| `CommandSearch` | `ctrl+p` | command palette | Open command palette overlay (alternative) |
| `Help` | `?` | help | Open help overlay for current view |
| `Back` | `esc` | back | Return to parent view / close overlay |

#### List Navigation Keys

| Binding | Default Key(s) | Help Text | Action |
|---------|---------------|-----------|--------|
| `CursorUp` | `k`, `up` | up | Move cursor up in list |
| `CursorDown` | `j`, `down` | down | Move cursor down in list |
| `Select` | `enter` | select | Open detail for selected item |

These bindings apply to all list views: Deployments, Leases, Providers, Governance, Staking.

#### View Shortcut Keys

| Binding | Default Key | Help Text | Target View |
|---------|------------|-----------|-------------|
| `Deployments` | `1` | deployments | `viewDeployments` |
| `Leases` | `2` | leases | `viewLeases` |
| `Providers` | `3` | providers | `viewProviders` |
| `Monitor` | `4` | monitor | `viewMonitor` |
| `Governance` | `5` | governance | `viewGovernance` |
| `Staking` | `6` | staking | `viewStaking` |

#### Context-Specific Action Keys

| Binding | Default Key | Help Text | Context |
|---------|------------|-----------|---------|
| `Close` | `d` | close | Deployments: close deployment; Staking: delegate |
| `Update` | `u` | update | Deployments: update deployment |
| `Logs` | `l` | logs | Deployments: open log viewer |
| `Shell` | `s` | shell | Deployments: open shell (planned) |
| `Vote` | `v` | vote | Governance: vote on proposal |
| `Deploy` | `D` | deploy | Dashboard/Deployments: new deployment |
| `Filter` | `f` | filter | List views: cycle state filter |
| `Search` | `/` | search | List views: open search input |
| `TabNext` | `tab` | next tab | Deployment Detail: cycle sub-tabs; Monitor: cycle dashboards |

### Key Dispatch Order

The `Update()` method processes keys in this order:

1. **App-level messages** (non-key): `DeploymentsLoadedMsg`, `WindowSizeMsg`, etc.
2. **Non-key forwarding**: Non-key messages always forwarded to monitor model.
3. **Standalone mode**: Only `Ctrl+C` intercepted; all other keys go to monitor.
4. **Overlay interception** (in priority order):
   - Confirm dialog active: all keys to dialog
   - Log viewer active: `Esc` close, `Space` pause, `c` clear, `j/k` scroll, `G` bottom
   - Help overlay active: `Esc` close
   - Command palette active: all keys to palette
5. **Global keys**: `Ctrl+C` quit, `:` / `Ctrl+P` palette, `?` help
6. **Monitor forwarding**: When `viewMonitor`, all keys go to monitor model
7. **Deployment detail keys**: `Esc` back, `Tab` next tab, `j/k` scroll, `1-4` tab jump
8. **Cursor navigation**: `j/k` dispatched to current list view
9. **View-specific actions**: `Enter` on deployments, `l` logs, `d` close
10. **View switching**: `1-6` number keys, `Esc` back to dashboard

### Overlay-Specific Keys (Hardcoded)

These keys are handled directly in `Update()` and are not part of the configurable `KeyMap`:

#### Log Viewer

| Key | Action |
|-----|--------|
| `esc` | Close log viewer |
| `space` | Toggle pause/resume |
| `c` | Clear log buffer |
| `k`, `up` | Scroll up |
| `j`, `down` | Scroll down |
| `G` | Scroll to bottom |

#### Help Overlay

| Key | Action |
|-----|--------|
| `esc` | Close help overlay |

#### Deployment Detail (Hardcoded Tab Jump)

| Key | Action |
|-----|--------|
| `1` | Jump to tab 0 |
| `2` | Jump to tab 1 |
| `3` | Jump to tab 2 |
| `4` | Jump to tab 3 |

### Custom Keybinding Configuration

When `tui.keybindings` is set to `"custom"` in Viper config, `KeyMapFromConfig()` reads overrides from `tui.custom-keybindings.<name>`:

| Config Key | KeyMap Field | Default |
|------------|-------------|---------|
| `quit` | `Quit` | `ctrl+c` |
| `command-palette` | `Command` + `CommandSearch` | `:` / `ctrl+p` |
| `help` | `Help` | `?` |
| `back` | `Back` | `esc` |
| `cursor-up` | `CursorUp` | `k`, `up` |
| `cursor-down` | `CursorDown` | `j`, `down` |
| `select` | `Select` | `enter` |
| `deployments` | `Deployments` | `1` |
| `leases` | `Leases` | `2` |
| `providers` | `Providers` | `3` |
| `monitor` | `Monitor` | `4` |
| `governance` | `Governance` | `5` |
| `staking` | `Staking` | `6` |
| `close` | `Close` | `d` |
| `update` | `Update` | `u` |
| `logs` | `Logs` | `l` |
| `shell` | `Shell` | `s` |
| `vote` | `Vote` | `v` |
| `deploy` | `Deploy` | `D` |
| `filter` | `Filter` | `f` |
| `search` | `Search` | `/` |
| `tab-next` | `TabNext` | `tab` |

Each config value is a string slice (e.g., `["k", "up"]`). When a `command-palette` override is provided, it is applied to both `Command` and `CommandSearch` bindings to keep both triggers in sync.

### Example Custom Configuration

```yaml
tui:
  keybindings: custom
  custom-keybindings:
    quit: ["q", "ctrl+c"]
    command-palette: [":"]
    help: ["?", "F1"]
    back: ["esc", "backspace"]
    cursor-up: ["k", "up"]
    cursor-down: ["j", "down"]
    select: ["enter"]
    close: ["d"]
    vote: ["v"]
    deploy: ["D"]
```

## Color Tokens Used

Keybindings themselves have no color. The footer hints that display keybinding information use:

| Token | Usage |
|-------|-------|
| `FooterKey` (Slate400, bold) | Key labels in footer hints |
| `FooterDesc` (Slate600) | Description text in footer hints |
| `AccentRed` (bold) | Accent-flagged action keys (Deploy, Vote, Delegate) |

## Interaction

### Focus Model

The TUI has no explicit focus ring. Instead, the active overlay or view determines which keys are processed:

1. If an overlay is active, it captures all keys (except `Ctrl+C`).
2. If no overlay is active, the current `activeView` determines which actions are available.
3. Number keys `1-6` are always available for view switching (except in overlays and monitor view).

### Conflict Resolution

- In **monitor view**, number keys `1-3` are forwarded to the monitor model for sub-tab switching instead of triggering global view navigation.
- In **deployment detail**, number keys `1-4` jump to sub-tabs instead of triggering global view navigation.
- The `d` key means "close deployment" in the Deployments view but "delegate" in the Staking view (context-dependent via footer hints).

### Palette Key Passthrough

The command palette receives its navigation bindings via a `PaletteKeys` struct populated from the global `KeyMap` at startup:

```go
PaletteKeys{
    CursorUp:   km.CursorUp,
    CursorDown: km.CursorDown,
    Select:     km.Select,
    Close:      km.Back,
}
```

This ensures custom keybinding overrides propagate to the palette.

## Data Sources

| Source | Usage |
|--------|-------|
| `viper.Viper` | `tui.keybindings` mode flag, `tui.custom-keybindings.*` overrides |
| `DefaultKeyMap()` | Hardcoded vim-style defaults (always the starting point) |

## Implementation Reference

| Component | File |
|-----------|------|
| `KeyMap` struct | `internal/tui/keymap.go:11-42` |
| `DefaultKeyMap()` | `internal/tui/keymap.go:45-140` |
| `KeyMapFromConfig()` | `internal/tui/keymap.go:144-205` |
| Config entry map | `internal/tui/keymap.go:158-181` |
| CommandSearch sync | `internal/tui/keymap.go:197-202` |
| Key dispatch in `Update()` | `internal/tui/app.go:420-578` |
| Overlay key interception | `internal/tui/app.go:357-418` |
| Standalone mode handling | `internal/tui/app.go:340-352` |
| Log viewer keys | `internal/tui/app.go:369-391` |
| Deployment detail keys | `internal/tui/app.go:449-470` |
| Palette key passthrough | `internal/tui/app.go:143-148` |
| Footer hint definitions | `internal/tui/app.go:671-733` |

## SPEC.md Cross-Reference

- **Section 8.6 — Keybinding Specification**: Defines the complete keybinding tables for global, list navigation, detail view, and resource-specific actions. The implementation covers the core set; some spec bindings (`g`/`G` top/bottom, `Ctrl+d`/`Ctrl+u` page up/down, `n`/`N` search navigation, `s` sort, `Ctrl+r` force refresh, `Shift-Tab` previous dashboard) are not yet in the `KeyMap` struct and are either handled inline or planned for future tasks.
- **Section 8.6 — Keybinding Configuration**: The `tui.keybindings: custom` + `tui.custom-keybindings` YAML map is fully implemented in `KeyMapFromConfig()`.
- **Section 8.4 — Command Palette**: Palette keyboard handling (up/down/enter/esc) uses the same configurable bindings via `PaletteKeys`.
