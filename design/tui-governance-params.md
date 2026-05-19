# Governance Parameters Sub-Tab — UX Design

## Overview

The Governance sub-tab is the third tab within the Network dashboard of `akt monitor`. It provides a split-pane module browser: a scrollable module list on the left and a pretty-printed parameter display on the right. The right pane renders parameters using the same `Render*Params()` functions as the CLI `--output pretty` mode, maintaining visual parity between CLI and TUI (SPEC §10.8). Parameters are fetched from REST endpoints and refreshed every 5 minutes. The `RenderModuleParamsFromJSON()` bridge function unmarshals REST JSON and dispatches to the appropriate typed renderer.

## Wireframe

```
                        akt monitor - Akash Network
 Network    Provider    Oracle/BME

 1: Overview          2: Validators        [3: Governance]

Governance Parameters
j/k: select module, h/l: scroll params

  Governance          │ Governance Parameters
  Minting             │ ──────────────────────────────────────
  Staking             │   Voting Period:     336h (14 days)
  Slashing            │   Min Deposit:       500,000,000 uakt
  Distribution        │   Max Deposit Pd:    336h (14 days)
  Auth                │   Quorum:            33.4%
  Bank                │   Threshold:         50.0%
  Deployment          │   Veto Threshold:    33.4%
  Market              │
  Transfer            │   Expedited
  IBC                 │     Voting Period:   24h
  Crisis              │     Threshold:       66.7%
                      │     Min Deposit:     750,000,000 uakt
                      │
                      │   Burn
                      │     Vote Quorum:     No
                      │     Deposit Prevote: Yes
                      │     Vote Veto:       Yes

q: quit | r: refresh params | Tab/1-3: switch tabs
RPC: https://rpc.akash.network [WS]
```

## Component Specifications

### Layout

| Element | Specification |
|---|---|
| **Header** | "Governance Parameters" in `headerStyle` (SectionTitle: Slate100 bold with red border-bottom, padding, margin) |
| **Help text** | `j/k: select module, h/l: scroll params` in `mutedStyle` (Slate500) |
| **Split pane** | Left column: 22 chars fixed width. Right column: `termWidth - 22`. Joined horizontally via `lipgloss.JoinHorizontal(lipgloss.Top, ...)`. |
| **Vertical space** | `governanceOverhead = 13` lines consumed by chrome (title, hub tabs, sub tabs, blanks, header, help, status bar). Available height: `height - 13`, minimum 5. |

### Module List (Left Pane)

| Element | Specification |
|---|---|
| **Component** | Custom scrolling list rendered by `renderGovModuleList()` |
| **Modules** | 12 modules from `governance.ModuleOrder`: gov, mint, staking, slashing, distribution, auth, bank, deployment, market, transfer, ibc, crisis |
| **Display names** | Mapped via `governance.PrettyModuleNames`: Governance, Minting, Staking, Slashing, Distribution, Auth, Bank, Deployment, Market, Transfer, IBC, Crisis |
| **Selected row** | `highlightStyle` (Slate200 bold), 18-char left-padded with 2-char indent |
| **Unselected rows** | Plain text, 18-char left-padded with 2-char indent |
| **Scrolling** | When module count exceeds visible rows, 1 row reserved for scroll indicator. Indicator: `N/T` (selected index / total) in mutedStyle. |
| **Scroll offset** | `govModuleScroll` tracks first visible index. Adjusted on j/k to keep selected module visible. Clamped on terminal resize. |
| **Visible rows** | `govModuleHeight = height - governanceOverhead(13)`, minimum 5 |

### Parameter View (Right Pane)

| Element | Specification |
|---|---|
| **Component** | `bubbles/viewport.Model` — scrollable text viewport |
| **Width** | `termWidth - 22` (module list width) |
| **Height** | Same as `govModuleHeight` |
| **Content** | Output of `pretty.RenderModuleParamsFromJSON(module, rawJSON)` |
| **Scrolling** | h/l keys adjust `YOffset` by 1 line |
| **Empty state** | `(no data)` if module params are nil |
| **Error state** | `Error: <message>` if fetch failed |
| **Fallback** | Syntax-highlighted JSON via `WriteHighlightedJSON()` for unknown modules or parse failures |

### Module Parameter Rendering

Each module's parameters are rendered by a dedicated function that produces formatted KV output. The TUI path uses `RenderModuleParamsFromJSON()` which unmarshals REST JSON and calls the appropriate renderer:

| Module | Display Name | REST Endpoint | Renderer |
|---|---|---|---|
| `gov` | Governance | `/cosmos/gov/v1beta1/params/voting` | `renderGovParamsJSON()` → Section + KV: Voting Period, Min Deposit, Max Deposit Pd, Quorum, Threshold, Veto Threshold, Expedited sub-section, Burn sub-section |
| `mint` | Minting | `/cosmos/mint/v1beta1/params` | `renderMintParamsJSON()` → Section + KV: Denom, Rate Change, Max/Min Inflation, Goal Bonded, Blocks Per Year |
| `staking` | Staking | `/cosmos/staking/v1beta1/params` | `renderStakingParamsJSON()` → Section + KV: Unbonding Time, Max Validators, Max Entries, History Depth, Bond Denom, Min Commission |
| `slashing` | Slashing | `/cosmos/slashing/v1beta1/params` | `renderSlashingParamsJSON()` → Section + KV: Signed Window, Min Signed/Win, Downtime Jail, Slash Dbl Sign, Slash Downtime |
| `distribution` | Distribution | `/cosmos/distribution/v1beta1/params` | `renderDistributionParamsJSON()` → Section + KV: Community Tax, Withdraw Addr |
| `auth` | Auth | `/cosmos/auth/v1beta1/params` | `renderAuthParamsJSON()` → Section + KV: Max Memo Chars, Tx Sig Limit, Tx Size/Byte, Verify ED25519, Verify Secp256k |
| `bank` | Bank | `/cosmos/bank/v1beta1/params` | `renderBankParamsJSON()` → Section + KV: Default Send |
| `deployment` | Deployment | Generic params subspace | `renderDeploymentParamsJSON()` → Section + KV: Min Deposits |
| `market` | Market | Generic params subspace | `renderMarketParamsJSON()` → Section + KV: Order Max Bids, Bid Min Deposits |
| `transfer` | Transfer | Generic params subspace | `renderTransferParamsJSON()` → Section + KV: Send Enabled, Receive Enabled |
| `ibc` | IBC | Generic params subspace | `renderIBCParamsJSON()` → Section + KV: Allowed Clients |
| `crisis` | Crisis | Generic params subspace | `renderCrisisParamsJSON()` → Section + KV: Constant Fee |

### Formatting Helpers (from `internal/output/pretty/`)

| Helper | Usage |
|---|---|
| `Section(title)` | Bold underlined section heading |
| `KVWidth(w, label, value)` | Fixed-width label + value line |
| `KVHeader(name)` | Sub-section header (e.g., "Expedited", "Burn") |
| `SubKV(label, value)` | Indented sub-section KV pair |
| `FormatDuration(d)` | Human-readable duration (e.g., "21 days") |
| `FormatDurationString(s)` | Parse Go duration string then format |
| `FormatPercent(s)` | Decimal string → percentage (e.g., "0.334" → "33.4%") |
| `FormatPercentDec(d)` | `sdk.Dec` → percentage |
| `FormatCoins(coins)` | Coin array → human-readable (e.g., "500 AKT") |
| `FormatCoin(coin)` | Single coin → human-readable |
| `FormatBool(b)` | Color-coded "Yes" (green) / "No" (red) |
| `FormatHeight(n)` | Thousand-separated integer |
| `Dim(s)` | Slate400 foreground |

### CLI/TUI Visual Parity (§10.8)

The governance tab achieves visual parity with CLI output through:

1. **Shared renderers**: `RenderStakingParams()`, `RenderGovParams()`, etc. are public functions used by both CLI pretty formatters and the TUI bridge.
2. **Bridge function**: `RenderModuleParamsFromJSON(module, rawJSON)` unmarshals REST JSON into the same structures and calls the same formatting helpers.
3. **Shared formatting helpers**: `Section()`, `KVWidth()`, `FormatDuration()`, `FormatPercent()`, `FormatCoins()`, `FormatBool()` are used by both paths.
4. **Fallback**: Unknown modules or parse failures fall back to syntax-highlighted JSON via `WriteHighlightedJSON()`.

## Color Tokens Used

| Token | Usage |
|---|---|
| `theme.Slate100` | Section titles, headings |
| `theme.Slate500` | Help text, muted labels, KV labels, scroll indicator |
| `theme.Slate400` | Dim text, inactive elements |
| `theme.Slate200` | Highlight style (selected module), KV values |
| `theme.AccentRed` | Header border-bottom (SectionTitle style) |
| `theme.GreenColor` | `FormatBool(true)` → "Yes" |
| `theme.AccentRed` | `FormatBool(false)` → "No" |

## Interaction

| Key | Context | Action |
|---|---|---|
| `3` | Network hub | Switch to Governance sub-tab |
| `j` / `↓` | Governance | Select next module (scroll list if needed) |
| `k` / `↑` | Governance | Select previous module (scroll list if needed) |
| `g` / `Home` | Governance | Jump to first module (Governance) |
| `G` / `End` | Governance | Jump to last module (Crisis) |
| `h` / `←` | Governance | Scroll parameter viewport up by 1 line |
| `l` / `→` | Governance | Scroll parameter viewport down by 1 line |
| `r` | Governance | Re-fetch all governance parameters |
| `1` / `2` | Governance | Switch to Overview / Validators sub-tab |
| `Tab` | Any | Cycle hub dashboard |

**Module selection behavior**: When j/k moves the selected index, `updateGovParamView()` is called to re-render the right pane content. The viewport scroll position resets to top on module change.

## Data Sources

| Data | Source | Refresh |
|---|---|---|
| Standard module params | Direct REST endpoints (see table above) | 5 minutes (`GovernanceSyncInterval`) |
| Generic module params | `/cosmos/params/v1beta1/subspaces` + per-key queries | 5 minutes |
| All params | `rpc.Client.GetAllGovernanceParams()` → `governance.AllParams` | 5 minutes, or on-demand via `r` key |

### Fetch Flow

1. `fetchGovernanceParams()` called at startup and every 5 minutes via `governanceSyncTick()`
2. `rpc.Client.GetAllGovernanceParams()` queries all module endpoints in parallel
3. Results stored in `governance.AllParams.Modules` map (module name → `ModuleParams` with `RawJSON`)
4. `handleGovernanceParamsMsg()` stores params and calls `updateGovParamView()`
5. `updateGovParamView()` calls `pretty.RenderModuleParamsFromJSON(module, rawJSON)` for the selected module
6. Rendered string set as viewport content

## Implementation Reference

| Component | File |
|---|---|
| Governance tab rendering (`renderGovernanceTab`) | `internal/monitor/ui/view.go` (lines 1358-1379) |
| Module list rendering (`renderGovModuleList`) | `internal/monitor/ui/view.go` (lines 1384-1420) |
| Governance param view update (`updateGovParamView`) | `internal/monitor/ui/model.go` (lines 1011-1029) |
| Key handling (j/k/h/l for governance) | `internal/monitor/ui/model.go` (lines 1290-1386) |
| Resize logic (`resizeComponents`) | `internal/monitor/ui/model.go` (lines 1851-1862) |
| Module order & display names | `internal/monitor/governance/types.go` |
| `RenderModuleParamsFromJSON` bridge | `internal/output/pretty/params.go` (lines 330-371) |
| Per-module JSON renderers | `internal/output/pretty/params.go` (lines 390-668) |
| Typed `Render*Params()` functions | `internal/output/pretty/params.go` (lines 52-314) |
| Formatting helpers | `internal/output/pretty/` (format.go, helpers) |
| Shared theme | `internal/ui/theme/theme.go` |

## SPEC.md Cross-Reference

| Section | Coverage |
|---|---|
| **§8.3.11** Governance Parameters View | Split-pane layout: module list on left, params on right. 12 modules displayed (SPEC lists 14 including Wasm and Oracle which are in the CLI renderers but not yet in `ModuleOrder`). Right pane renders pretty-printed params using shared `Render*Params()` functions. j/k selects module, h/l scrolls params. Data from REST endpoints, 5-minute refresh. |
| **§10.8** Pretty/TUI Visual Parity | CLI and TUI use the same `Render*Params()` functions and formatting helpers. `RenderModuleParamsFromJSON()` bridge ensures identical output from REST JSON. |
