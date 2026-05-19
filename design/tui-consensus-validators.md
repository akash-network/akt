# Consensus & Validators Dashboard — UX Design

## Overview

The Network dashboard is the default hub in `akt monitor`. It provides real-time consensus state monitoring via three sub-tabs: Overview (block progress with vote bars and block history table), Validators (scrollable validator list with signing history), and Governance (module parameter browser). Data arrives through a WebSocket subscription to `/consensus_state` with a render-throttled update loop (100ms ticks). This document covers the Overview and Validators tabs; Governance is documented separately.

## Wireframe — Overview Tab (Block Progress)

```
                        akt monitor - Akash Network
 Network    Provider    Oracle/BME

 1: Overview          2: Validators         3: Governance

┌─────────────────────────────────────────────────────────────────────────┐
│  Block Progress                                                         │
│                                                                         │
│  ████████████████████████████████░░░░░░░░  PV 89.2%                    │
│  ██████████████████████████████████████░░  PC 95.1%                    │
│                                                                         │
│  Height          PV       PC       Elapsed  R/S                        │
│  * 18,234,567    89.2%    95.1%    1.2s     0/6                        │
│    18,234,566   100.0%   100.0%    5.8s     0/6                        │
│    18,234,565   100.0%   100.0%    6.1s     0/6                        │
│    18,234,564    98.7%    98.7%    5.5s     1/6                        │
│    18,234,563   100.0%   100.0%    5.9s     0/6                        │
│                                                                         │
│  q: quit | Tab/1-3: switch | j/k: select | Enter: expand | Esc: collapse│
│  RPC: https://rpc.akash.network [WS]                                   │
└─────────────────────────────────────────────────────────────────────────┘
```

## Wireframe — Block Detail Overlay (Enter on a block row)

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Block 18,234,566 — Validator Votes                                     │
│  PV: 100.0%  PC: 100.0%  Elapsed: 5.8s  R/S: 0/6                     │
│                                                                         │
│      Validator                Power              PV    PC              │
│      Forbole                  8.7M 8.2%          ✓     ✓              │
│      Polkachu                 6.5M 6.1%          ✓     ✓              │
│      Cosmostation             5.9M 5.6%          ✓     ✗              │
│      Figment                  4.2M 4.0%          ✗     ✗              │
│      Chorus One               3.8M 3.6%          ✓     ✓              │
│      ...                                                               │
│      Showing 1-25 of 100 validators (j/k to scroll)                   │
│  esc/h/←: back                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Wireframe — Validators Tab

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Validators (100) — Block Signing History                               │
│                                                                         │
│  #     Validator                    Power              Blocks (newest ←)│
│  0     Forbole                      8.7M 8.2%         ●●●●●●●●●●●●●●●●│
│  1     Polkachu                     6.5M 6.1%         ●●●●●●●●●●●●●●●●│
│  2     Cosmostation                 5.9M 5.6%         ●●●●●●●●●●●●●●●○│
│  3     Figment                      4.2M 4.0%         ●●●●●●●●●○●●●●●●│
│  4     Chorus One                   3.8M 3.6%         ●●●●●●●●●●●●●●●●│
│  5     Stakefish                    3.2M 3.0%         ●●●●●●●●●●●●●●●●│
│  ...                                                                    │
│                                                                         │
│  q: quit | r: refresh | Tab/1-3: switch tabs | j/k: scroll             │
│  RPC: https://rpc.akash.network [WS]                                   │
└─────────────────────────────────────────────────────────────────────────┘
```

## Wireframe — Validator Detail Overlay (Enter on a validator row)

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Validator: Forbole                                                     │
│  ──────────────────────────────────────────────────────────────         │
│    Validator:  Forbole                                                  │
│    Address:    akash1valcons1abc...full_hex_address                     │
│    PubKey:     base64_consensus_pubkey_truncated...                     │
│    Power:      8,700,000 (8.20%)                                       │
│    Current:    prevote ✓  precommit ✓                                  │
│    History:    48 signed, 2 missed out of 50 blocks (96.0%)            │
│    Role:       Current Proposer ★                                      │
│    Esc: collapse                                                       │
└─────────────────────────────────────────────────────────────────────────┘
```

## Component Specifications

### Hub Tab Bar

| Element | Specification |
|---|---|
| **Tabs** | Network (default), Provider, Oracle/BME |
| **Active style** | `NavTabActive`: AccentRed background, Slate950 foreground, bold, padding 0,1 |
| **Inactive style** | `NavTabInactive`: Slate400 foreground, padding 0,1 |
| **Separator** | 2 spaces between tabs |
| **Width** | Full terminal width |

### Network Sub-Tab Bar

| Element | Specification |
|---|---|
| **Tabs** | `1: Overview`, `2: Validators`, `3: Governance` |
| **Per-tab width** | `(termWidth - tabCount + 1) / tabCount`, minimum 12 chars |
| **Active style** | `NavTabActive` (red bg, dark fg, bold) |
| **Inactive style** | `NavTabInactive` (Slate400 fg) |
| **Label format** | ` N: Label` left-aligned within tab width |

### Double Progress Bar (Overview Tab)

| Element | Specification |
|---|---|
| **Layout** | Two stacked `bubbles/progress` bars, one per line |
| **Top bar (PV)** | Prevote percentage. Color: `theme.ProgressSuccess` (GreenColor). Width: `termWidth - 2 (margin) - 24 (label space)`, minimum 20. |
| **Bottom bar (PC)** | Precommit percentage. Color: `theme.ProgressPrecommit` (BlueColor). Same width. |
| **Labels** | Right of each bar: `PV` + `FormatPercent()` / `PC` + `FormatPercent()`. Percent colored green if >= 66.7%, yellow otherwise. |
| **Clamping** | Both percentages clamped to [0, 1] |

### Block Table (Overview Tab)

| Element | Specification |
|---|---|
| **Component** | `bubbles/table.Model` with custom transparent cell style |
| **Columns** | Height (14w), PV (8w), PC (8w), Elapsed (9w), R/S (6w) |
| **First row** | Current live block, marked with `* ` prefix. Height in `valueStyle`, R/S highlighted if round > 0. |
| **History rows** | Completed blocks, newest first. Height in `mutedStyle`. Max 50 history entries. |
| **Vote coloring** | PV/PC percentages: green bold if >= 66.7% (`consensusThreshold`), yellow otherwise |
| **Elapsed format** | `FormatShortDuration()`: `1.2s`, `45s`, `1m 30s` |
| **Height format** | `FormatNumber()`: thousand-separated (e.g., `18,234,567`) |
| **Visible rows** | `height - overviewOverhead(10)`, minimum 3. Halved to `max(rows/3, 2)` when a block is expanded. |
| **Selection** | `bubbles/table` built-in cursor with `highlightStyle` (Slate200 bold) |

### Block Detail Overlay

| Element | Specification |
|---|---|
| **Trigger** | Enter on a block row toggles expansion. Same row again collapses. |
| **Header** | `Block N — Validator Votes` in `headerStyle` |
| **Stats line** | PV, PC (colored), Elapsed, R/S — single line |
| **Validator list** | Sorted by voting power descending. Columns: Validator name (auto-width, min 16), Power (18w, right-aligned with percentage), PV (4w, centered), PC (4w, centered). |
| **Vote icons** | `✓` (VoteYes, GreenColor) / `✗` (VoteNo, AccentRed) via `glyphs.G().VoteYes` / `glyphs.G().VoteNo` |
| **Scrolling** | j/k scrolls within the validator list. Scroll indicator: `Showing N-M of T validators (j/k to scroll)` |
| **Dismiss** | Esc, h, or ← collapses back to block table |
| **Snapshot** | Validator votes are frozen at expansion time (not live-updated) |

### Validators Table

| Element | Specification |
|---|---|
| **Component** | `bubbles/table.Model` |
| **Columns** | # (5w), Validator (28w), Power (18w), Blocks newest← (40w) |
| **Validator name** | Moniker resolved from consensus pubkey via `/cosmos/staking/v1beta1/validators`. Emoji-stripped via `stripEmojis()`. Cached in `~/.config/akt/cache/monikers.json`. Truncated with `...` if > 28 chars. Falls back to truncated hex address. |
| **Power column** | `formatPower()` (K/M/B suffixes) + percentage of total |
| **Signing history bar** | Visual bar of `●` (U+25CF) characters. Green = signed (precommited), Red = missed. `★` (U+2605) in yellow for proposer blocks. Newest block on left. Width: `max(termWidth - 5 - 28 - 18 - 7, 20)`. Max 50 entries. |
| **Visible rows** | `max(height - 14, 5)`. Halved when a validator is expanded. |
| **Selection** | `bubbles/table` built-in with `highlightStyle` |

### Validator Detail Overlay

| Element | Specification |
|---|---|
| **Trigger** | Enter on a validator row |
| **Content** | Divider (60-char `─`), then KV pairs: Validator (moniker), Address (full hex), PubKey (truncated at 44 chars), Power (with percentage), Current (prevote/precommit icons), History (signed/missed counts with percentage), Role (if current proposer: yellow bold with `★`) |
| **Dismiss** | Esc collapses |

### Consensus State Section (used in vote progress)

| Element | Specification |
|---|---|
| **Fields** | Height (thousand-separated, 14-char padded), Round (4-char padded), Step (round/step format), Elapsed (14-char padded), Proposer (truncated address: 8 prefix + `...` + 4 suffix, with index) |
| **Layout** | Two lines of label-value pairs. Labels in `labelStyle` (Slate500, 12w), values in `valueStyle` (Slate200). |

### Vote Grid Section

| Element | Specification |
|---|---|
| **Characters** | `●` (voted, GreenColor) / `○` (not voted, Slate500) from `glyphs.G()` |
| **Width** | `clamp(termWidth - 10, 20, 100)` |
| **Line wrapping** | Automatic at grid width |
| **Legend** | `● voted  ○ not voted` in muted style |

## Color Tokens Used

| Token | Usage |
|---|---|
| `theme.AccentRed` | Title bar, tab active background, vote-no icons, section rules |
| `theme.Slate950` | Tab active foreground |
| `theme.Slate500` | Labels, muted text, column headers, grid not-voted |
| `theme.Slate400` | Inactive tab text |
| `theme.Slate300` | Body text, moniker style |
| `theme.Slate200` | Values, highlight style, selected rows |
| `theme.Slate100` | Headings, section titles |
| `theme.Slate700` | Borders, dividers |
| `theme.GreenColor` | Vote-yes icons, grid voted, percent >= 66.7%, signing bar (signed) |
| `theme.YellowColor` | Percent < 66.7%, proposer indicator, proposer blocks in signing bar |
| `theme.BlueColor` | Precommit progress bar |
| `theme.ProgressSuccess` (`GreenColor`) | Prevote progress bar fill |
| `theme.ProgressPrecommit` (`BlueColor`) | Precommit progress bar fill |

## Interaction

| Key | Context | Action |
|---|---|---|
| `1` | Network hub | Switch to Overview sub-tab |
| `2` | Network hub | Switch to Validators sub-tab (resets cursor to 0, collapses expanded validator) |
| `3` | Network hub | Switch to Governance sub-tab |
| `Tab` | Any hub | Cycle hub: Network → Provider → Oracle/BME |
| `Shift+Tab` | Any hub | Cycle hub backward |
| `j` / `↓` | Overview (no expansion) | Move block table cursor down |
| `k` / `↑` | Overview (no expansion) | Move block table cursor up |
| `j` / `↓` | Overview (expanded) | Scroll expanded validator list down |
| `k` / `↑` | Overview (expanded) | Scroll expanded validator list up |
| `Enter` | Overview | Toggle block expansion (snapshot validators) |
| `Esc` / `h` / `←` | Overview (expanded) | Collapse block detail |
| `j` / `↓` | Validators (no expansion) | Move validator table cursor down |
| `k` / `↑` | Validators (no expansion) | Move validator table cursor up |
| `Enter` | Validators | Toggle validator detail overlay |
| `Esc` / `h` / `←` | Validators (expanded) | Collapse validator detail |
| `g` / `Home` | Any table | Jump to first row |
| `G` / `End` | Any table | Jump to last row |
| `r` | Any tab | Refresh (governance: re-fetch params; consensus is event-driven) |
| `q` | Any | Quit (or send `BackMsg` if embedded) |
| `Ctrl+C` | Any | Force quit |

## Data Sources

| Data | Source | Refresh |
|---|---|---|
| Consensus state | WebSocket `/consensus_state` subscription via `rpc.Client.SubscribeConsensusState()` | Real-time events, render-throttled at 100ms |
| Validator set | `GET /validators?per_page=100` via `rpc.Client.GetValidators()` | Cached per session |
| Validator monikers | `GET /cosmos/staking/v1beta1/validators` via `rpc.Client.GetValidatorMonikers()` | Cached in `~/.config/akt/cache/monikers.json` via `MonikerCache` |
| Proposer info | `GET /consensus_state` via `rpc.Client.GetConsensusStateWithValidators()` | Fetched on each height change (WebSocket events lack proposer data) |
| Initial signing history | `GET /commit` (latest) via `rpc.Client.GetLatestCommit()` | Once at startup to seed signing bars |
| Block history | Accumulated from WebSocket height transitions | Up to 50 blocks (`MaxBlockHistory`) |
| Per-validator signing history | Accumulated from precommit status at each height change | Up to 50 entries per validator (`maxSignHistory`) |

### WebSocket Connection Lifecycle

1. `connectWebSocket()` establishes subscription, returns `wsConnectedMsg` with channel
2. `waitForSnapshot()` blocks on channel, sends `consensusSnapshotMsg` per event
3. State buffered in `pendingState`, applied on next `renderTickMsg` (100ms)
4. On channel close: `wsConnected = false`, reconnect via `connectWebSocket`
5. Height transitions: pending state applied immediately if new height > pending height (prevents vote data loss)

### Render Throttle

- `RenderInterval = 100ms` — periodic tick applies `pendingState` to model
- WebSocket events buffered between ticks to avoid excessive re-renders
- Proposer fetch triggered when `state.Height > lastProposerHeight`

## Implementation Reference

| Component | File |
|---|---|
| View rendering (all tabs) | `internal/monitor/ui/view.go` |
| Model, state management, Update loop | `internal/monitor/ui/model.go` |
| Styles and progress bar wrappers | `internal/monitor/ui/styles.go` |
| Consensus state parsing | `internal/monitor/consensus/` |
| RPC client (WebSocket, HTTP) | `internal/monitor/rpc/` |
| Moniker cache | `internal/monitor/cache/` |
| Shared theme | `internal/ui/theme/theme.go` |
| Glyph definitions (●, ○, ✓, ✗, ★) | `internal/glyphs/` |

## SPEC.md Cross-Reference

| Section | Coverage |
|---|---|
| **§8.3.8** Consensus Monitor View | Height/round/step/elapsed/proposer display, prevote/precommit progress bars (block/blank, 40 chars, green >= 66.7%), validator vote grid (●/○) |
| **§8.3.9** Validator Voting View | Scrollable table with #, proposer indicator (*), moniker, voting power, prevote/precommit status (✓/✗), signing history bar. Sub-tab navigation via 1/2/3 keys. |
