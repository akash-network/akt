# Command Palette — UX Design

## Overview

The command palette is a centered floating overlay that provides fast, keyboard-driven access to all TUI commands. Activated by `:` (colon) or `Ctrl+P`, it presents a text input with a `:` prompt at the top and a scrollable, filtered list of commands below. The palette supports case-insensitive substring matching against command names and aliases, enabling rapid navigation, action invocation, and application control without memorizing keybindings. It currently registers 20 commands across 4 categories (navigation, action, context, app).

## Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                                                                                  │
│                                                                                  │
│                                                                                  │
│            ╭──────────────────────────────────────────────────────╮               │
│            │  : depl                                             │               │
│            │  ──────────────────────────────────────────────────  │               │
│            │  > Deployments          View all deployments        │               │
│            │    Deploy               Create new deployment       │               │
│            │    Deploy from SDL      Create deployment from SDL  │               │
│            │                                                     │               │
│            │                                                     │               │
│            │                                                     │               │
│            │  ──────────────────────────────────────────────────  │               │
│            │  ↑↓ navigate  ↵ select  esc close                  │               │
│            ╰──────────────────────────────────────────────────────╯               │
│                                                                                  │
│                                                                                  │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### Detailed Anatomy

```
╭──────────────────────────────────────────────────────╮  ← Rounded border (Slate700)
│                                                      │  ← Padding: 1 row top
│  : query text here_                                  │  ← Text input with ":" prompt
│  ──────────────────────────────────────────────────── │  ← Separator (Slate700, "─" repeated)
│  > Command Name      Short description               │  ← Selected row (bold, Slate100/Slate800 bg)
│    Command Name      Short description               │  ← Normal row (Slate300 name, Slate500 desc)
│    Command Name      Short description               │  ← ...
│    ...                                               │  ← Scrollable (max ~50% terminal height)
│  ──────────────────────────────────────────────────── │  ← Separator
│  ↑↓ navigate  ↵ select  esc close                   │  ← Footer hints
│                                                      │  ← Padding: 1 row bottom
╰──────────────────────────────────────────────────────╯  ← Rounded border
```

## Component Specifications

### Overlay Container
- **Position**: Centered both horizontally and vertically via `lipgloss.Place(width, height, Center, Center, dialog)`.
- **Width**: 60% of terminal width, clamped to `[50, 80]` columns.
- **Border**: `lipgloss.RoundedBorder()` in `theme.Slate700` (#3f3f46).
- **Background**: `theme.Slate900` (#18181b).
- **Padding**: 1 row vertical, 2 columns horizontal (`Padding(1, 2)`).
- **Inner width**: `boxW - 6` (border 2 + padding 4), minimum 20.

### Text Input
- **Component**: `bubbles/v2/textinput.Model`.
- **Prompt**: `": "` — styled with `theme.AccentRed` (#ff4136), bold.
- **Placeholder style**: `theme.Slate500` (#71717a).
- **Text style**: `theme.Slate200` (#e4e4e7).
- **Character limit**: 128.
- **Width**: `innerW - 1` (accounts for prompt character).

### Separator Lines
- **Character**: `"─"` repeated to fill `innerW`.
- **Color**: `theme.Slate700` (#3f3f46).
- **Placement**: Between input and list, and between list and footer.

### Command List
- **Max visible rows**: `terminalHeight / 2 - 6` (minimum 3).
- **Column layout**: Name gets ~40% of inner width, description gets the remainder minus 4 chars (cursor prefix + gap).
- **Selected row style**: Bold, `theme.Slate100` (#f4f4f5) foreground, `theme.Slate800` (#27272a) background, full-width. Prefixed with `"> "`.
- **Normal row style**: `theme.Slate300` (#d4d4d8) for name, `theme.Slate500` (#71717a) for description. Prefixed with `"  "`.
- **Empty state**: `"  no matching commands"` in dim style (Slate500).
- **Truncation**: Names and descriptions truncated with `"..."` suffix when exceeding column width.

### Footer Hints
- **Key style**: `theme.Slate400` (#a1a1aa), bold.
- **Description style**: `theme.Slate600` (#52525b).
- **Content**: `↑↓ navigate  ↵ select  esc close`.

### Command Registry
- **Total commands**: 20 (11 navigation + 4 action + 3 context + 2 app).
- **Data structure**: `commands.Command{Name, Description, Category, Aliases}`.
- **Filtering**: `Registry.Filter(query)` — case-insensitive substring match on `Name` and all `Aliases`. Empty query returns all commands.

### Registered Commands

| Category | Name | Description | Aliases |
|----------|------|-------------|---------|
| navigation | Dashboard | Go to dashboard | home |
| navigation | Deployments | View all deployments | dep |
| navigation | Leases | View leases | — |
| navigation | Providers | View providers | prov |
| navigation | Staking | View validators & delegation | validators, val |
| navigation | Governance | View governance proposals | gov |
| navigation | Monitor | Real-time network monitor | monitor, top, consensus |
| navigation | Certificates | View certificates | cert |
| navigation | Escrow | View escrow accounts | — |
| navigation | Orders | View orders | — |
| navigation | Bids | View bids | — |
| action | Deploy | Create new deployment | — |
| action | Deploy from SDL | Create deployment from SDL file | — |
| action | Tail Logs | Tail logs for deployment | logs |
| action | Open Shell | Open shell in deployment | shell |
| context | Wallet Balance | Show wallet balance | — |
| context | Switch Wallet | Switch wallet account | — |
| context | Switch RPC | Switch RPC endpoint | — |
| app | Quit | Quit application | q, exit |
| app | Help | Show help | ? |

## Color Tokens Used

| Token | Source | Hex | Usage |
|-------|--------|-----|-------|
| `theme.Slate900` | `theme.go:24` | `#18181b` | Overlay background |
| `theme.Slate800` | `theme.go:25` | `#27272a` | Selected row background |
| `theme.Slate700` | `theme.go:27` | `#3f3f46` | Border, separator lines |
| `theme.Slate600` | `theme.go:28` | `#52525b` | Footer description text |
| `theme.Slate500` | `theme.go:28` | `#71717a` | Placeholder text, dim description, empty state |
| `theme.Slate400` | `theme.go:29` | `#a1a1aa` | Footer key text |
| `theme.Slate300` | `theme.go:30` | `#d4d4d8` | Normal command name |
| `theme.Slate200` | `theme.go:31` | `#e4e4e7` | Input text |
| `theme.Slate100` | `theme.go:32` | `#f4f4f5` | Selected row text |
| `theme.AccentRed` | `theme.go:39` | `#ff4136` | Input prompt `":"` |

## Interaction

### Activation
| Key | Action |
|-----|--------|
| `:` (colon) | Opens the command palette (global keybinding) |
| `Ctrl+P` | Opens the command palette (global keybinding) |

### While Palette Is Open
| Key | Action |
|-----|--------|
| Any printable character | Appends to input, re-filters list, resets cursor to first match |
| `Backspace` | Deletes last character, re-filters list |
| `j` or `Down` | Moves cursor down (wraps to top) |
| `k` or `Up` | Moves cursor up (wraps to bottom) |
| `Enter` | Selects highlighted command, closes palette, dispatches `CommandSubmitMsg` |
| `Esc` | Closes palette without action |

### Keyboard Resolution
All navigation keys are resolved through the configurable `PaletteKeys` struct:
- `CursorUp` — bound to `k` / `Up`
- `CursorDown` — bound to `j` / `Down`
- `Select` — bound to `Enter`
- `Close` — bound to `Esc`

The parent `App` populates `PaletteKeys` from the global `KeyMap` at startup, so user keybinding overrides (§8.6) are respected.

### Focus Behavior
- On open: text input receives focus (`input.Focus()`), filtered list resets to all commands, cursor resets to 0.
- On close: text input blurs (`input.Blur()`), palette becomes invisible.
- While active: the palette intercepts all key events — underlying views do not receive input.

### Command Dispatch
- Selected command name is wrapped in `CommandSubmitMsg{Value: name}` and returned as a `tea.Cmd`.
- If no commands match but the user typed raw text, the raw text is submitted as-is for exact-match fallback (e.g., typing `"quit"` directly).
- The parent `App.handleCommand()` routes the submitted value to the appropriate action (view navigation, workflow launch, quit, etc.).

## Data Sources

The command palette is entirely client-side — it does not query any external data. The command registry is populated at startup by `commands.DefaultRegistry()` with a static set of commands. The registry is extensible: new commands can be added via `Registry.Register()`.

## Implementation Reference

| Component | File |
|-----------|------|
| Palette view & logic | `internal/tui/views/command.go` — `CommandPalette` struct, `NewCommandPalette()`, `Open()`, `Close()`, `Update()`, `View()` |
| Command registry | `internal/tui/commands/registry.go` — `Registry`, `Command`, `DefaultRegistry()`, `Filter()` |
| Message types | `internal/tui/views/command.go` — `CommandSubmitMsg`, `PaletteKeys` |
| Theme tokens | `internal/ui/theme/theme.go` |

## SPEC.md Cross-Reference

| Section | Title | Coverage |
|---------|-------|----------|
| §8.4 | Command Palette | Overlay sizing, filtering behavior, keyboard handling, registered commands table, command dispatch, keybinding configurability |
| §8.6 | Keybinding Specification | Global `:` and `Ctrl+P` bindings, palette navigation key configurability via `KeyMap` |
