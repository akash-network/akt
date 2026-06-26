# TUI Full Design Parity Rework — Spec

## Goal

Rework the `akt` TUI to match the bubbletea v2 prototype (`design/prototype/akash-tui-v2/main.go`) pixel-for-pixel. The prototype is the authoritative visual reference. Every view, overlay, column, spacing pattern, and color must match.

## Reference Documents

- **Authoritative visual reference**: `design/prototype/akash-tui-v2/main.go` (3,471 lines)
- **Prototype theme**: `design/prototype/akash-tui-v2/theme/theme.go` (316 lines)
- **Design docs**: `design/tui-*.md` (11 markdown files)
- **SPEC.md**: Sections 8 (TUI Specification), 10 (Output Format)
- **DESIGN.md**: Section 5.2 (Cobra for CLI, Bubbletea for TUI)

## Scope

The rework covers **7 categories of changes** across ~15 files:

### 1. Theme Alignment

Bring `internal/ui/theme/theme.go` into alignment with the prototype's theme. The current theme is close but has:

- **7 style property differences**:
  - `HeaderStyle` — prototype adds `Foreground(Slate300)`
  - `HeaderContext` — current `Slate200`, prototype `Slate100`
  - `HeaderValue` — current `Slate200`, prototype `Slate300`
  - `SyncOK` — current `Foreground(GreenColor)`, prototype `Foreground(Slate300)`
  - `TableHeader` — prototype adds `Padding(0,1)`
  - `TableRowSelected` — current adds `Foreground(Slate200).Bold(true)`, prototype is `Background(Slate800)` only
  - `TabInactive` (deprecated alias) — current `Slate400`, prototype `Slate500`

- **~40 missing styles** organized by category:
  - **Inline column styles (5)**: `ColHeader`, `Col`, `ColBold`, `ColMuted`, `ColAccent`
  - **Table cell extras (3)**: `TableCell`, `TableCellBold`, `TableCellMuted`
  - **KV extras (2)**: `KVValueBold`, `KVValueMuted`
  - **Status badges (7)**: `BadgeActive`, `BadgeClosed`, `BadgeDestructive`, `BadgeWarning`, `PillActive`, `PillClosed`, `PillDestructive`
  - **Status tags (4)**: `TagActive`, `TagClosed`, `TagWarning`, `TagDestructive`
  - **Command palette (6)**: `PaletteBorder`, `PaletteInput`, `PalettePrompt`, `PaletteItemNormal`, `PaletteItemSelected`, `PaletteItemDesc`
  - **Dialog (5)**: `DialogBorder`, `DialogTitle`, `DialogBody`, `DialogButtonPrimary`, `DialogButtonSecondary`
  - **Error display (4)**: `ErrorLabel`, `ErrorMessage`, `ErrorSuggestion`, `ErrorCommand`
  - **Progress extras (2)**: `ProgressLabel`, `ProgressPct`
  - **Spinner extras (1)**: `SpinnerText`
  - **Header extras (1)**: `SyncFail`
  - **Semantic aliases (14)**: `BgApp`, `BgSurface`, `BgElevated`, `BgAccent`, `Border`, `BorderMuted`, `BorderFocus`, `TextPrimary`, `TextSecondary`, `TextMuted`, `TextHeading`, `TextEmphasis`, `Accent`, `AccentMuted`

- **Missing functions**: `HRuleAccent()`, `StateTag()`, `StateBadge()`

**Approach**: Add all missing styles. Fix the 7 property differences. Add missing functions. Keep backward-compatible aliases intact — they are used by the monitor and pretty-output packages.

### 2. Dashboard Redesign

Replace the flat KV-dump dashboard with the prototype's card-based layout:

**Summary cards row** — 3 bordered cards across the top:
- Balance card: `KVKey("Balance")` + `KVValueBold("148.52 AKT")` + `KVValueMuted("  ≈ $742.60")`
- Deployments card: `KVValueBold(activeCount)` + `KVValue(" active")` + `KVValueMuted("  N total")`
- Leases card: `KVValueBold(activeLeases)` + `KVValue(" active")` + `KVValueMuted("  rate")`
- Card style: `RoundedBorder()`, `BorderForeground(Slate700)`, `Padding(0,1)`, width = `(termWidth-6)/3`
- Joined: `lipgloss.JoinHorizontal(lipgloss.Top, ...)`

**Recent Deployments mini-table** — section header + accent hrule + 5-row table:
- Columns: DSEQ (7w), STATE (13w, state tag), IMAGE (20w), PRICE/BLK (12w), ESCROW (10w), AGE (remainder)
- No cursor, no interaction
- Footer: `ColMuted("Press 2 to see all N deployments")`

**Network status strip** — single-row inline KV pairs:
- Chain, Height, Validators, Proposals — separated by 3 spaces
- KV labels at 8 or 12 chars wide

**Remove**: Welcome banner, Account section, Shortcuts section.

Data: Balance requires `SetBalance(amount string)`. Validator/proposal counts require new setters (or derive from loaded data).

### 3. Missing Detail Views (4 new files)

All detail views use the standard `SectionTitle` + `HRuleAccent(w-4)` + KV pairs pattern with 4-space indent.

**Lease Detail** (`internal/tui/views/lease_detail.go`):
- Section 1 — Lease: DSEQ (bold), GSEQ/OSEQ, State (badge), Provider, Price, Age, Image
- Section 2 — Order: Order ID (bold), Bid ID, Order State (badge), Created At (block height)
- Section 3 — Settlement: Last Settled, Settled Amt, Funds Left (bold), Withdrawn
- Section 4 — Provider Status: Status (badge), Forwarded Ports, Available IPs
- Settlement and Provider Status sections show dashes until data pipeline is wired

**Provider Detail** (`internal/tui/views/provider_detail.go`):
- Section 1 — Provider: Address, URL (bold), Version, Region, Status (badge), Active Leases, Uptime
- Section 2 — Resources: CPU progress bar, Memory progress bar, GPU KV
- Resource bars use `renderProgressBar()` pattern from prototype

**Proposal Detail** (`internal/tui/views/proposal_detail.go`):
- Section 1 — Proposal: Title (bold), Type, Status (badge), Ends
- Section 2 — Vote Tally: 4 progress bars (Yes, No, Abstain, No w/ Veto)

**Validator Detail** (`internal/tui/views/validator_detail.go`):
- Section 1 — Validator: Rank (#N bold), Address, Tokens, Commission, Status (badge), Uptime
- Section 2 — Your Delegation: Delegated, Rewards, Share (muted) — shows dashes until delegation query is wired

### 4. Populate Placeholder Data

**Governance tallies**: Fetch `govv1.QueryTallyResultRequest` for voting-period proposals when loading the governance view. Populate YES/NO/ABSTAIN/VETO columns with actual percentages.

**Staking VP%**: Calculate as `validator.Tokens / totalBondedTokens * 100`. Fetch total bonded from staking pool query (`stakingtypes.QueryPoolRequest`). Uptime and Signed remain as dashes (require slashing signing info — future work).

### 5. Overlay Compositing Fix

Implement `overlayCenter(base, overlay string, w, h int) string` from the prototype:
1. Split base into lines, pad to fill terminal height
2. Split overlay into lines, measure max width
3. Calculate center: `startY = (h - overlayH) / 2` (min 2), `startX = (w - overlayW) / 2` (min 0)
4. For each overlay line, replace the corresponding base line at that row
5. Return joined result

Update `app.go View()` to use `overlayCenter()` for all overlays (palette, help, confirm, log viewer) instead of replacing `main` entirely.

### 6. App.go Navigation Updates

Add view IDs: `viewLeaseDetail`, `viewProviderDetail`, `viewProposalDetail`, `viewValidatorDetail`

Wire navigation:
- Leases list: `Enter` → load lease detail data → `viewLeaseDetail`
- Providers list: `Enter` → load provider detail data → `viewProviderDetail`
- Governance list: `Enter` → load proposal detail data → `viewProposalDetail`
- Staking list: `Enter` → load validator detail data → `viewValidatorDetail`
- All detail views: `Esc` → back to list

Add data-loading commands for each detail view.

Wire footer hints per detail view matching prototype.

Wire breadcrumb for detail views:
- Lease detail: `Leases / #DSEQ/GSEQ/OSEQ`
- Provider detail: `Providers / host.url`
- Proposal detail: `Governance / #ID`
- Validator detail: `Staking / Moniker`

### 7. Oracle Panel Completion

In `internal/monitor/ui/view.go`, fill the TODO in `renderOraclePanel()`:
- When `m.oracle.Aggregated` has data, render per-denom rows:
  - Denom, TWAP price (bold), change indicator (muted, with arrow)
- When empty, show loading text
- Match the prototype's Oracle Prices layout

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `internal/ui/theme/theme.go` | Modify | Add ~40 missing styles, fix 7 property diffs, add functions |
| `internal/tui/views/dashboard.go` | Rewrite | Card-based layout per prototype |
| `internal/tui/views/dashboard_test.go` | Modify | Update golden files |
| `internal/tui/views/lease_detail.go` | Create | Lease detail view (4 sections) |
| `internal/tui/views/provider_detail.go` | Create | Provider detail view (2 sections) |
| `internal/tui/views/proposal_detail.go` | Create | Proposal detail view (2 sections) |
| `internal/tui/views/validator_detail.go` | Create | Validator detail view (2 sections) |
| `internal/tui/views/governance.go` | Modify | Populate tally columns |
| `internal/tui/views/staking.go` | Modify | Populate VP% column |
| `internal/tui/app.go` | Modify | New view IDs, navigation, overlay compositing, data commands |
| `internal/tui/messages/messages.go` | Modify | New message types for detail data and tallies |
| `internal/monitor/ui/view.go` | Modify | Complete oracle panel rendering |
| Various `*_test.go` and `*.golden` | Modify | Update golden files to match new rendering |

## Out of Scope

- Deploy workflow view (prototype has it commented out)
- Light theme / custom theme YAML configuration
- Shell view (lease-shell interactive terminal)
- Slashing signing info queries for staking Uptime/Signed columns
- Delegation queries for "Your Delegation" in validator detail
- Settlement/escrow queries for lease detail settlement section
- Status bar deduplication (current dual-status-bar works — the footer is correct per design, the status bar is used only by monitor which manages its own chrome)
