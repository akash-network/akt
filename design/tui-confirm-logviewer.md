# Confirmation Dialog & Log Viewer — UX Design

## Overview

This document covers two modal overlay components used across the TUI. The **confirmation dialog** gates destructive or state-changing transactions (close deployment, vote, delegate, unbond, redelegate) behind an explicit user confirmation step with fee preview and account context. The **log viewer** provides a streaming, scrollable overlay for viewing container logs from deployed services, with pause/resume, clear, and scroll controls. Both components render as full-screen overlays on top of the underlying view.

---

## Part 1: Confirmation Dialog

### Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                                                                                  │
│                                                                                  │
│                                                                                  │
│              ╭──────────────────────────────────────────────────╮                 │
│              │                                                  │                 │
│              │  Close Deployment                  destructive   │                 │
│              │                                                  │                 │
│              │  This will close all active leases and return    │                 │
│              │  remaining escrow balance.                       │                 │
│              │                                                  │                 │
│              │  fee preview: 0.0142 AKT                        │                 │
│              │  from:        akash1abc...def                    │                 │
│              │                                                  │                 │
│              │  esc cancel                       ↵ confirm      │                 │
│              │                                                  │                 │
│              ╰──────────────────────────────────────────────────╯                 │
│                                                                                  │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### Vote Variant Wireframe

```
╭──────────────────────────────────────────────────╮
│                                                  │
│  Vote on Proposal #42                            │
│                                                  │
│  Cast your vote on this governance proposal.     │
│                                                  │
│  ╭─────╮  no   abstain   veto                    │
│  │ yes │                                         │
│  ╰─────╯                                         │
│                                                  │
│  fee preview: 0.005 AKT                          │
│  from:        akash1abc...def                    │
│                                                  │
│  esc cancel                       ↵ confirm      │
│                                                  │
╰──────────────────────────────────────────────────╯
```

### Component Specifications

#### Overlay Container
- **Position**: Centered via `lipgloss.Place(width, height, Center, Center, dialog)`.
- **Width**: Fixed at 60 columns (`dialogWidth` constant).
- **Border**: `lipgloss.RoundedBorder()`.
  - **Normal**: `theme.Slate600` (#52525b).
  - **Danger**: `theme.AccentRed` (#ff4136) — when `ConfirmData.Danger` is true.
- **Background**: `theme.Slate900` (#18181b).
- **Padding**: 1 row vertical, 2 columns horizontal (`Padding(1, 2)`).
- **Inner width**: `dialogWidth - 6` (border 2 + padding 4) = 54, minimum 20.

#### Confirmation Kinds
| Kind | Constant | Description |
|------|----------|-------------|
| Close | `ConfirmClose` (0) | Close a deployment |
| Vote | `ConfirmVote` (1) | Cast a governance vote |
| Delegate | `ConfirmDelegate` (2) | Delegate tokens to a validator |
| Unbond | `ConfirmUnbond` (3) | Unbond tokens from a validator |
| Redelegate | `ConfirmRedelegate` (4) | Redelegate tokens between validators |

#### Title Bar
- **Style**: `theme.Heading` (Slate100, bold).
- **Danger badge**: When `ConfirmData.Danger` is true, appends `"destructive"` in `theme.AccentRed`.

#### Body Text
- **Style**: `theme.Secondary` (Slate400), constrained to `innerW`.
- **Content**: `ConfirmData.Body` — contextual description of the action.

#### Vote Options (Vote variant only)
- **Options**: `yes`, `no`, `abstain`, `veto` — rendered as horizontal button row.
- **Selected option**: Rounded border in `theme.AccentRed`, foreground `theme.Slate100`.
- **Unselected options**: No border, foreground `theme.Slate400`.
- **Layout**: `lipgloss.JoinHorizontal(lipgloss.Center, ...)` with `Padding(0, 1)` per option.

#### Fee Preview
- **Fee line**: `"fee preview:"` in `theme.Secondary` (Slate400) + value in `theme.KVValue` (Slate200).
- **Account line**: `"from:"` in `theme.Secondary` + value in `theme.KVValue`.
- **Visibility**: Only shown when `ConfirmData.Fee` or `ConfirmData.Account` is non-empty.

#### Button Row
- **Cancel button**: `theme.ButtonSecondary` (Slate800 bg, Slate300 fg) with `Padding(0, 1)`. Text: `"esc cancel"`.
- **Confirm button**: `theme.ButtonPrimary` (AccentRed bg, Slate950 fg, bold) with `Padding(0, 1)`. Text: `"↵ confirm"`.
- **Layout**: Cancel left-aligned, confirm right-aligned, gap fills remaining `innerW`.

#### Section Spacing
- All sections separated by `"\n\n"` (double newline) via `strings.Join(sections, "\n\n")`.

### Interaction

| Key | Action |
|-----|--------|
| `Esc` | Cancel — closes dialog, dispatches `CancelMsg{}` |
| `Enter` | Confirm — closes dialog, dispatches `ConfirmMsg{Kind, VoteOption}` |
| `Tab` | Cycle focus between cancel (0) and confirm (1) buttons |
| `y` | Select "yes" vote option (Vote variant only) |
| `n` | Select "no" vote option (Vote variant only) |
| `a` | Select "abstain" vote option (Vote variant only) |
| `v` | Select "veto" vote option (Vote variant only) |

**Focus behavior**:
- Default focus: confirm button (`focusBtn = 1`).
- On open: focus and vote choice reset.
- While active: dialog intercepts all key events.

### Data Sources

The confirmation dialog is populated by the parent view that triggers it:
- `ConfirmData.Title` — action-specific title (e.g., "Close Deployment #12345?").
- `ConfirmData.Body` — contextual description.
- `ConfirmData.Fee` — estimated transaction fee (from gas simulation).
- `ConfirmData.Account` — signing account address.
- `ConfirmData.Danger` — boolean flag for destructive actions.

---

## Part 2: Log Viewer

### Wireframe

```
╭──────────────────────────────────────────────────────────────────────────────────╮
│                                                                                  │
│  LOGS  my-deployment  dseq 12345  service web              ● streaming           │
│  ────────────────────────────────────────────────────────────────────────────     │
│  10:15:32   INFO   web        Server starting on port 8080                       │
│  10:15:33   INFO   web        Connected to database                              │
│  10:15:34   INFO   web        Ready to accept connections                        │
│  10:15:35   INFO   api        API gateway initialized                            │
│  10:16:01   INFO   web        GET /health 200 2ms                                │
│  10:16:15   WARN   web        Slow query detected: 450ms                         │
│  10:16:30   ERR    api        Connection refused: backend:5432                   │
│  10:16:45   INFO   web        Retry succeeded                                    │
│  10:17:00   INFO   web        GET /api/v1/users 200 45ms                         │
│                                                                                  │
│                                                                                  │
│                                                                                  │
│  ────────────────────────────────────────────────────────────────────────────     │
│  space pause  c clear  / filter  esc back                                        │
│                                                                                  │
╰──────────────────────────────────────────────────────────────────────────────────╯
```

### Paused State

```
│  LOGS  my-deployment  dseq 12345  service web              ○ paused              │
```

### Component Specifications

#### Overlay Container
- **Position**: Full-width overlay (not centered — fills available space).
- **Width**: `terminalWidth - 2` (1 column margin each side).
- **Border**: `lipgloss.RoundedBorder()` in `theme.Slate700` (#3f3f46).
- **Background**: `theme.Slate900` (#18181b).
- **Padding**: 1 row vertical, 2 columns horizontal (`Padding(1, 2)`).
- **Inner width**: `totalWidth - 6` (border 2 + padding 4), minimum 30.

#### Header Bar
- **Badge**: `"LOGS"` — `theme.Slate950` foreground, `theme.AccentRed` background, bold, `Padding(0, 1)`.
- **Deployment name**: `theme.Body` (Slate300).
- **DSEQ**: `"dseq {value}"` in `theme.Muted` (Slate500).
- **Service**: `"service {name}"` in `theme.Muted` (Slate500).
- **Status indicator** (right-aligned):
  - Streaming: `"●"` green (GreenColor #22c55e) + `"streaming"` green.
  - Paused: `"○"` yellow (YellowColor #eab308) + `"paused"` yellow.
- **Layout**: Left content + dynamic gap + right-aligned status.
- **Rule**: Full-width `"─"` in `theme.Slate700` below the header line.

#### Log Area
- **Visible height**: `terminalHeight - 8` (border 2 + padding 2 + header 2 + footer 2), minimum 1 row.
- **Column layout** (fixed widths):
  - Timestamp: 10 chars — `theme.Slate500` (#71717a).
  - Level: 6 chars — color-coded:
    - `INFO` → `theme.GreenColor` (#22c55e), bold.
    - `WARN` → `theme.YellowColor` (#eab308), bold.
    - `ERR` → `theme.AccentRed` (#ff4136), bold.
  - Scope: 10 chars — `theme.PurpleColor` (#c08cff).
  - Message: remaining width (innerW - 10 - 6 - 10 - 3 spaces) — `theme.Slate300` (#d4d4d8).
- **Column separator**: Single space between each column.
- **Message truncation**: Truncated with `"..."` when exceeding available width.
- **Empty padding**: Rows below the last log line are padded with empty strings to fill the visible area.

#### Log Line Data Structure
```go
type LogLine struct {
    Timestamp string  // e.g., "10:15:32"
    Level     string  // "INFO", "WARN", "ERR"
    Scope     string  // service/component name
    Message   string  // log message content
}
```

#### Buffer Management
- **Maximum lines**: 500 (`maxLogLines` constant).
- **Trimming**: When buffer exceeds 500 lines, oldest lines are dropped from the front.
- **Auto-scroll**: When not paused, scroll position automatically tracks the newest lines.

#### Footer
- **Rendered by**: `components.Footer(width, hints)` — horizontal rule + hint pairs.
- **Hints** (dynamic based on pause state):
  - `space` → `"pause"` or `"resume"` (toggles based on current state).
  - `c` → `"clear"`.
  - `/` → `"filter"`.
  - `esc` → `"back"`.
- **Style**: `theme.FooterKey` (Slate400, bold) for keys, `theme.FooterDesc` (Slate600) for descriptions.

### Interaction

| Key | Action |
|-----|--------|
| `Esc` | Close the log viewer overlay |
| `Space` | Toggle pause/resume — when unpausing, auto-scrolls to bottom |
| `c` | Clear all log lines and reset scroll position |
| `j` / `Down` | Scroll down one line (only when paused) |
| `k` / `Up` | Scroll up one line (only when paused) |
| `G` | Scroll to bottom (most recent logs) |
| `/` | Open filter (hint shown, implementation pending) |

**Focus behavior**:
- On open: resets all state (lines, scroll, paused) for the given deployment/service.
- While active: overlay intercepts all key events.
- Scroll is locked when streaming (not paused) — new lines auto-scroll to bottom.
- Scroll is unlocked when paused — j/k navigate the buffer.

**Transitions**:
- **Open**: `l` key on deployment or lease detail views calls `LogViewer.Open(title, dseq, service)`.
- **Close**: `Esc` calls `LogViewer.Close()`, returns to the underlying view.

### Data Sources

Log data comes from the provider gateway log stream when connected to a deployment:
- **Source**: Provider gateway WebSocket/HTTP streaming endpoint.
- **Ingestion**: `LogViewer.AppendLine(line)` or `LogViewer.AppendLines(lines)` called by the parent view as log events arrive.
- **Parsing**: Raw log lines are parsed into `LogLine` structs (timestamp, level, scope, message) before being appended.

---

## Color Tokens Used (Both Components)

| Token | Source | Hex | Usage |
|-------|--------|-----|-------|
| `theme.Slate950` | `theme.go:23` | `#09090b` | Confirm button fg, LOGS badge fg |
| `theme.Slate900` | `theme.go:24` | `#18181b` | Overlay backgrounds |
| `theme.Slate800` | `theme.go:25` | `#27272a` | Cancel button bg |
| `theme.Slate700` | `theme.go:27` | `#3f3f46` | Borders, horizontal rules |
| `theme.Slate600` | `theme.go:28` | `#52525b` | Normal dialog border, footer desc |
| `theme.Slate500` | `theme.go:28` | `#71717a` | Muted text, timestamps, DSEQ/service labels |
| `theme.Slate400` | `theme.go:29` | `#a1a1aa` | Secondary text, unselected vote options, footer keys |
| `theme.Slate300` | `theme.go:30` | `#d4d4d8` | Body text, cancel button fg, log messages |
| `theme.Slate200` | `theme.go:31` | `#e4e4e7` | KV values (fee, account) |
| `theme.Slate100` | `theme.go:32` | `#f4f4f5` | Heading text, selected vote option |
| `theme.AccentRed` | `theme.go:39` | `#ff4136` | Danger border, confirm button bg, LOGS badge bg, ERR level, selected vote border |
| `theme.GreenColor` | `theme.go:47` | `#22c55e` | INFO level, streaming indicator |
| `theme.YellowColor` | `theme.go:49` | `#eab308` | WARN level, paused indicator |
| `theme.PurpleColor` | `theme.go:52` | `#c08cff` | Log scope column |

## Implementation Reference

| Component | File |
|-----------|------|
| Confirmation dialog | `internal/tui/components/confirm.go` — `ConfirmDialog`, `ConfirmKind`, `ConfirmData`, `ConfirmMsg`, `CancelMsg` |
| Confirmation tests | `internal/tui/components/confirm_test.go` |
| Log viewer | `internal/tui/views/logviewer.go` — `LogViewer`, `LogLine`, `NewLogViewer()`, `Open()`, `Close()`, `AppendLine()`, `View()` |
| Footer component | `internal/tui/components/footer.go` — `Footer()`, `HintPair`, `FooterHints()` |
| Theme tokens | `internal/ui/theme/theme.go` |

## SPEC.md Cross-Reference

| Section | Title | Coverage |
|---------|-------|----------|
| §8.5 | Confirmation Dialog | Dialog wireframe, fee/gas preview, Cancel/Confirm buttons, transaction action flow |
| §8.3.7 | Log Viewer | Log viewer wireframe, service filter, follow mode, search, column layout |
| §8.6 | Keybinding Specification | Global keybindings for overlay close (Esc), list navigation (j/k) |
