# TUI Design System

Design reference for the `akt` TUI prototype. This document is intended to be consumed by an AI agent to reproduce the visual design and interaction patterns 1:1.

## Framework

- **Bubbletea v2** (`charm.land/bubbletea/v2`) — Elm-architecture TUI framework
- **Lipgloss v2** (`charm.land/lipgloss/v2`) — Styling (colors, borders, padding, layout)
- **Bubbles v2** (`charm.land/bubbles/v2`) — Pre-built components (text input, spinner)

All styles are defined in `theme/theme.go`. The main application is a single `main.go` file.

---

## Color Palette

The palette is a neutral Zinc scale (shadcn convention) with a single accent color. There are no other hues in the base palette — the UI is monochromatic with red accents.

### Zinc Scale

| Token | Hex | Usage |
|-------|-----|-------|
| `Slate950` | `#09090b` | App background |
| `Slate900` | `#18181b` | Card / panel / overlay background |
| `Slate800` | `#27272a` | Elevated surface, selected row highlight |
| `Slate700` | `#3f3f46` | Borders, horizontal rules |
| `Slate600` | `#52525b` | Muted borders, subtle dividers |
| `Slate500` | `#71717a` | Placeholder text, disabled text, column headers, muted labels |
| `Slate400` | `#a1a1aa` | Secondary text, descriptions, inactive nav tabs |
| `Slate300` | `#d4d4d8` | Body text, table cell values |
| `Slate200` | `#e4e4e7` | Primary text, bold values, emphasis |
| `Slate100` | `#f4f4f5` | Headings, section titles |
| `Slate50`  | `#fafafa` | Maximum emphasis (use sparingly) |

### Accent

| Token | Hex | Usage |
|-------|-----|-------|
| `Red` | `#ff4136` | Primary accent: cursor, active tab bg, focus ring, spinner, accent rules |
| `RedDim` | `#b91c1c` | Muted accent: destructive tag border |
| `RedBg` | `#1c0a09` | Destructive pill background tint |

### State Colors

Used exclusively in state tags (inline bordered pills in table rows):

| Token | Hex | States |
|-------|-----|--------|
| `Green` | `#22c55e` | active, open, bonded, passed, valid |
| `GreenDim` | `#166534` | Border for green state tags |
| `Yellow` | `#eab308` | paused, insufficient_funds, overdrawn, unbonding |
| `YellowDim` | `#854d0e` | Border for yellow state tags |
| `Slate500` | `#71717a` | closed, lost, unbonded, rejected, failed, revoked |
| `Slate700` | `#3f3f46` | Border for closed/inactive state tags |

---

## Typography

Terminal text — no font choices. Hierarchy is achieved through:

1. **Bold** — headings, active values, selected row text, accent labels
2. **Normal weight** — body text, table cells
3. **Color** — the primary hierarchy signal (brighter = more important)

### Text Hierarchy

| Level | Color | Bold | Example |
|-------|-------|------|---------|
| Heading | `Slate100` (#f4f4f5) | Yes | Section titles ("Deployment", "Vote Tally") |
| Primary value | `Slate200` (#e4e4e7) | Yes | Key metrics ("148.52 AKT", DSEQ numbers) |
| Body | `Slate300` (#d4d4d8) | No | Table cell values, descriptions |
| Secondary | `Slate400` (#a1a1aa) | No | Inactive nav tabs, footer descriptions |
| Muted | `Slate500` (#71717a) | No | Column headers, KV labels, placeholder text, timestamps |

---

## Layout Structure

The layout is a vertical stack, always full terminal width:

```
┌─────────────────────────────────────────────────┐
│ Header Bar (dark bg: Slate900)                  │
├─────────────────────────────────────────────────┤
│ Nav Tab Bar + horizontal rule                   │
├─────────────────────────────────────────────────┤
│ Breadcrumb                                      │
├─────────────────────────────────────────────────┤
│                                                 │
│ Content Area (varies per view)                  │
│                                                 │
├─────────────────────────────────────────────────┤
│ Footer (horizontal rule + key hints)            │
└─────────────────────────────────────────────────┘
```

Vertical padding fills the gap between content and footer so the footer is always at the bottom.

### Header Bar

- Full-width background: `Slate900`
- Left: app name (`akt` in Red bold) · context (`prod` bold + `:akashnet-2` muted) · account (`alice` bold + address muted)
- Right: block height in brackets (`⎡ 18,234,567 ⎤`) + sync status (`● synced`)
- Separator between left/right: spaces (not flexbox — manual gap calculation)

### Nav Tab Bar

- 7 items: `1 Home`, `2 Deployments`, `3 Leases`, `4 Providers`, `5 Monitor`, `6 Governance`, `7 Staking`
- Active tab: **filled pill** — `Background(Red)`, `Foreground(Slate950)`, bold, `Padding(0,1)`
- Inactive tabs: `Foreground(Slate400)`, `Padding(0,1)`
- Below the tabs: a full-width horizontal rule in `Slate700`

### Breadcrumb

- Separator: ` / ` in `Slate600`
- Segments: `Slate500`
- Active (last) segment: `Slate200` bold
- Indented 1 space from left edge

### Footer

- Top: horizontal rule in `Slate700`
- Bottom: context-sensitive key hints — key in `Slate400` bold, description in `Slate600`
- Format: `key desc  key desc  key desc` (two spaces between pairs)

---

## Component Patterns

### Tables

Tables use a **fixed-width column** approach with `fmt.Sprintf("%-*s", width, text)` — NOT lipgloss padding. This prevents row wrapping.

```go
func col(style lipgloss.Style, width int, text string) string {
    padded := fmt.Sprintf("%-*s", width, text)
    return style.Render(padded)
}
```

Structure:
1. Column headers in `Slate500` (no bold)
2. Horizontal rule in `Slate700`
3. Data rows with cursor indicator
4. Bottom horizontal rule
5. Row count summary in `Slate500`

**Selected row**: `▸ ` cursor in `Red` bold, row background `Slate800`, text upgraded to `Slate200` bold.

**Unselected row**: `  ` (2 spaces), text in `Slate300` for values, `Slate500` for muted columns (provider address, age, uptime).

### State Tags (Inline)

State tags appear in table columns as bordered inline pills using Unicode pipe characters:

```
│active│   │low funds│   │closed│
```

This approach uses `│` (Unicode box-drawing) characters as borders — NOT lipgloss `RoundedBorder()` which would create 3-line boxes and break table row alignment.

```go
func stateTag(state string) string {
    label := shortState(state)
    // border.Render("│") + text.Render(label) + border.Render("│")
}
```

Color mapping:
- Green states (active, open, bonded, passed): text `#22c55e`, border `#166534`
- Yellow states (paused, insufficient_funds, unbonding): text `#eab308`, border `#854d0e`
- Closed states (closed, rejected, failed): text `#71717a`, border `#3f3f46`

Long state names are abbreviated: `insufficient_funds` → `low funds`.

Alignment: `stateTagWidth()` calculates display width (label length + 2 border chars), then manual space padding fills to the column width.

### Key-Value Detail Views

Used for all drill-down detail screens:

```
  Section Title              ← Slate100, bold
  ─────────────────────      ← Red accent rule
    Label          Value     ← Label: Slate500, width 16. Value: Slate200
    Label          Value
```

- Section title: `Slate100` bold
- Section rule: Red (`#ff4136`) — distinguishes from table rules which use `Slate700`
- KV label: `Slate500`, fixed width 16 characters
- KV value: `Slate200` (or `Slate100` bold for emphasis, `Slate500` for muted)

### Cards (Dashboard Only)

Dashboard uses bordered cards for summary metrics:

```
╭────────────────────────────────────╮
│ Label                              │
│ Bold Value  Muted detail           │
╰────────────────────────────────────╯
```

- Border: `RoundedBorder()`, `BorderForeground(Slate700)`
- Width: `(terminalWidth - 6) / 3` for 3 cards across
- Padding: `(0, 1)`
- Cards joined with `lipgloss.JoinHorizontal(lipgloss.Top, ...)`

### Progress Bars

Used in Monitor and Provider Detail views:

```
    Label         ████████████████████░░░░░░░░░ 67.2% (detail)
```

- Label: `Slate500`, width 14
- Filled blocks: `█` in `Slate200`
- Empty blocks: `░` in `Slate700`
- Percentage: `Slate200` bold
- Detail: `Slate500` in parentheses

### Overlays

All overlays (command palette, dialogs, help, log viewer) float over the content using `overlayCenter()` which composites overlay lines on top of the base view at the center of the terminal.

**Command Palette:**
- Border: `RoundedBorder()`, `Slate700`, background `Slate900`
- Prompt: `: ` in `Red` bold
- Selected item: `Slate100` on `Slate800` background, bold
- Normal item: `Slate300`
- Description: `Slate500`

**Dialogs (Confirm, Vote, Delegate):**
- Border: `RoundedBorder()`, `Slate600`, background `Slate900`
- Padding: `(1, 2)`
- Title: `Slate100` bold
- Body: `Slate400`
- Primary button: `Foreground(Slate950)`, `Background(Red)`, bold
- Secondary button: `Foreground(Slate300)`, `Background(Slate800)`

**Help Overlay:**
- Same border treatment as dialogs
- Key column: `Slate400` bold, width 10
- Description: `Slate300`
- Context label: "Context: ViewName" at top

**Log Viewer:**
- Same border as command palette (rounded, `Slate700`)
- Normal lines: `Slate300`
- `[warn]` lines: `Yellow` (#eab308)
- `[error]` lines: `Red` bold
- Footer: line count + scroll hints

---

## Navigation Architecture

### View Stack

Navigation uses a stack (`[]viewID`):

- `pushView(v)` — drill down (Enter on a list item)
- `popView()` — go back (Esc)
- `switchPrimary(v)` — replaces stack with `[Dashboard, target]` (number keys 1-7)

The stack determines the breadcrumb trail.

### Key Binding Layers

1. **Global** (always active, unless typing in an input):
   - `1-7`: Jump to primary views
   - `:` / `Ctrl+P`: Command palette
   - `?`: Help overlay
   - `q`: Go home (or quit from home)
   - `Ctrl+C`: Force quit

2. **Overlay** (intercepts all keys when open):
   - Command palette: `j/k` navigate, `Enter` select, `Esc` close, typing filters
   - Dialogs: `Tab` switch buttons, `Enter` confirm, `Esc` cancel
   - Help: `?` or `Esc` close
   - Log viewer: `j/k` scroll, `g/G` top/bottom, `Esc` close

3. **View-specific** (only when that view is active):
   - List views: `j/k` navigate, `Enter` detail, `f` filter, `Esc` back
   - Detail views: `Esc` back, plus action keys (`l` logs, `v` vote, `d/r/u` delegate)
   - Monitor: `Tab/Shift-Tab` cycle dashboards, `a/s/g` sub-tabs

### Monitor Sub-navigation

Monitor Hub has 3 dashboards cycled with Tab:
- Network (sub-tabs: Overview `a`, Validators `s`, Governance `g`)
- Provider Fleet
- Oracle/BME

Sub-tab keys use `a/s/g` instead of `1/2/3` because number keys are consumed by global primary navigation.

---

## Rendering Rules

1. **Never use lipgloss Padding on table cells** — it adds 2 chars per column and causes row wrapping. Use `fmt.Sprintf("%-*s", width, text)` via the `col()` helper.

2. **State tags must be single-line** — use `│text│` with Unicode pipes, not `RoundedBorder()` which creates 3-line boxes.

3. **Section rules under headings** use Red (`#ff4136`). Table rules and the nav separator use `Slate700` (`#3f3f46`). This creates visual hierarchy.

4. **Selected rows** get `Background(Slate800)` applied to the full row width via `TableRowSelected.Width(w).Render(row)`.

5. **Overlays** are composited by replacing lines in the base view string — not by Z-index or layering.

6. **Alt screen** is always enabled (`v.AltScreen = true`) for clean full-terminal rendering.

7. **Comma-group large numbers** (block heights, token amounts) for readability.

8. **Footer hints** are context-sensitive — each view specifies its own hint pairs. Format: bold key + muted description, 2-space gap between pairs.
