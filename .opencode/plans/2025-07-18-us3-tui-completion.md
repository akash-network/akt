# US3 TUI Completion — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete all remaining User Story 3 (TUI Mode) tasks — tests for existing code, design documents, data-driven views (providers, governance, staking), live sync integration, log streaming, help/toast wiring, and E2E smoke tests.

**Architecture:** The TUI `App` shell with navigation, theme, keymap, monitor hub (all 3 dashboards), and store-backed views (deployments, leases, deployment detail) is already complete. This plan adds: (1) chain query methods for governance proposals, full validator data, audit attributes, and lease counts via new methods on `rpc.Client`; (2) data binding for the three stub views (providers, governance, staking); (3) a reactive sync pipeline connecting the pubsub event bus → sync engine → store → TUI refresh messages; (4) wiring the help overlay and toast notifications into the App; (5) provider gateway log streaming into the LogViewer; (6) comprehensive unit tests for all existing views, components, and caches; (7) 11 standalone UX design documents; and (8) E2E TUI smoke tests.

**Tech Stack:** Go 1.24, Bubbletea v2, Lipgloss v2, Bubbles v2, bbolt, CometBFT RPC, Cosmos SDK REST, `pkg.akt.dev/go/util/pubsub`

**Prerequisites:**
- DESIGN.md and SPEC.md read in full
- User Story 1 and User Story 2 complete
- `go.work` with chain-sdk local replace active
- `make akt` builds successfully

---

## File Map

### New Files to Create

| File | Responsibility |
|------|---------------|
| `internal/monitor/rpc/proposals.go` | REST query for governance proposals `/cosmos/gov/v1/proposals` |
| `internal/monitor/rpc/proposals_test.go` | Tests for proposals query |
| `internal/monitor/rpc/staking_detailed.go` | REST query for full validator data `/cosmos/staking/v1beta1/validators` |
| `internal/monitor/rpc/staking_detailed_test.go` | Tests for staking query |
| `internal/monitor/rpc/audit.go` | REST query for provider audit attributes `/akash/audit/v1beta3/audit/attributes/list` |
| `internal/monitor/rpc/audit_test.go` | Tests for audit query |
| `internal/tui/sync_bridge.go` | Reactive pipeline: bus subscriber → sync.Engine → store re-read → TUI msg |
| `internal/tui/sync_bridge_test.go` | Tests for sync bridge |
| `internal/tui/nav_test.go` | Navigation/view-switching tests |
| `internal/tui/keymap_test.go` | Keymap loading and binding tests |
| `internal/tui/commands/registry_test.go` | Command registry filter tests |
| `internal/tui/views/dashboard_test.go` | Dashboard view render tests |
| `internal/tui/views/deployments_test.go` | Deployments view data binding tests |
| `internal/tui/views/leases_test.go` | Leases view data binding tests |
| `internal/tui/views/providers_test.go` | Providers view data binding tests |
| `internal/tui/views/governance_test.go` | Governance view data binding tests |
| `internal/tui/views/staking_test.go` | Staking view data binding tests |
| `internal/tui/views/deployment_detail_test.go` | Deployment detail sub-tab tests |
| `internal/tui/views/logviewer_test.go` | Log viewer lifecycle tests |
| `internal/tui/views/command_test.go` | Command palette filter/select tests |
| `internal/tui/views/help_test.go` | Help overlay render tests |
| `internal/tui/views/listview_test.go` | Generic ListView tests |
| `internal/tui/views/detailview_test.go` | Generic DetailView tests |
| `internal/tui/messages/messages_test.go` | Message type validation tests |
| `internal/monitor/cache/cache_test.go` | Provider cache CRUD + scheduling tests |
| `internal/monitor/cache/moniker_test.go` | Moniker cache tests |
| `e2e/tui_test.go` | E2E TUI smoke tests |
| `design/tui-app-shell.md` | Design doc: application shell layout |
| `design/tui-navigation.md` | Design doc: navigation model |
| `design/tui-theme.md` | Design doc: theme system |
| `design/tui-keybindings.md` | Design doc: keybinding scheme |
| `design/tui-resource-views.md` | Design doc: resource view wireframes |
| `design/tui-consensus-validators.md` | Design doc: consensus & validator views |
| `design/tui-provider-fleet.md` | Design doc: provider fleet monitor |
| `design/tui-governance-params.md` | Design doc: governance params view |
| `design/tui-oracle-bme.md` | Design doc: oracle/BME dashboard |
| `design/tui-command-palette.md` | Design doc: command palette |
| `design/tui-confirm-logviewer.md` | Design doc: confirmation dialog & log viewer |

### Files to Modify

| File | Changes |
|------|---------|
| `internal/monitor/rpc/grpc.go` | Add `GetActiveLeaseCount()` for per-provider lease counts |
| `internal/monitor/cache/cache.go` | Add `LeaseCount int`, `Audited bool`, `Auditors []string` to `CachedProvider`; add `MarkProviderAuditStatus()` and `MarkProviderLeaseCount()` to `ProviderStore` |
| `internal/tui/app.go` | Wire help overlay (`?` key), toast rendering in `View()`, expose bus to App struct, add `rpcClient` field, add data loading commands for providers/governance/staking, handle new message types, integrate sync bridge |
| `internal/tui/views/providers.go` | Add `SetData()` accepting cached provider records |
| `internal/tui/views/governance.go` | Add `SetData()` accepting proposal records |
| `internal/tui/views/staking.go` | Add `SetData()` accepting validator records |
| `internal/tui/views/deployment_detail.go` | Remove `truncAddr()`, display full addresses per AGENTS.md |
| `internal/tui/views/logviewer.go` | Add streaming integration method for provider log streaming |
| `internal/tui/messages/messages.go` | Add `ProvidersLoadedMsg`, `ProposalsLoadedMsg`, `ValidatorsLoadedMsg`, `LogLineMsg`, `LogStreamEndedMsg` |

---

## Dependency Graph

```
Group A (parallel, no deps):          Group B (parallel, no deps on A):
  Task 1: Tests for existing code       Task 4: GetProposals() query
  Task 2: Wire help/toast/fix addr      Task 5: GetValidatorsDetailed() query
  Task 3: Design documents (11 docs)    Task 6: GetActiveLeaseCount() + GetAuditAttributes()

                    Group C (depends on B):
                      Task 7: Governance view data binding
                      Task 8: Staking view data binding
                      Task 9: Providers view data binding

                              Group D (depends on C):
                                Task 10: Live sync pipeline
                                Task 11: Log streaming backend

                                        Group E (depends on D):
                                          Task 12: Dashboard enhancements
                                          Task 13: E2E TUI smoke tests
```

---

## Task 1: Tests for Existing TUI Code

**TASKS.md:** T064, T065, T066, T067, T068, T069, T070

**Purpose:** Add unit test coverage for all existing TUI code that currently has zero tests: navigation logic, keymap, command registry, views, and caches.

**Files:**
- Create: `internal/tui/nav_test.go`
- Create: `internal/tui/keymap_test.go`
- Create: `internal/tui/commands/registry_test.go`
- Create: `internal/tui/views/dashboard_test.go`
- Create: `internal/tui/views/deployments_test.go`
- Create: `internal/tui/views/leases_test.go`
- Create: `internal/tui/views/deployment_detail_test.go`
- Create: `internal/tui/views/logviewer_test.go`
- Create: `internal/tui/views/command_test.go`
- Create: `internal/tui/views/help_test.go`
- Create: `internal/tui/views/listview_test.go`
- Create: `internal/tui/views/detailview_test.go`
- Create: `internal/tui/messages/messages_test.go`
- Create: `internal/monitor/cache/cache_test.go`
- Create: `internal/monitor/cache/moniker_test.go`
- Read: `internal/tui/app.go` (understand Update() view switching logic)
- Read: `internal/tui/keymap.go` (understand DefaultKeyMap and KeyMapFromConfig)
- Read: `internal/tui/commands/registry.go` (understand Registry, Filter, DefaultRegistry)
- Read: All existing view files in `internal/tui/views/`
- Read: `internal/monitor/cache/cache.go` and `moniker.go`

### Subtask 1a: Navigation Tests (T064)

- [ ] **Step 1:** Read `internal/tui/app.go` lines 200-500 (the `Update()` method) to understand all view transition paths: number keys 1-6, Esc back behavior, Enter drill-in to deployment detail, command palette navigation, monitor BackMsg handling.

- [ ] **Step 2:** Create `internal/tui/nav_test.go` with tests for:
  - Number key 1 switches to `viewDeployments`
  - Number key 2 switches to `viewLeases`
  - Number key 3 switches to `viewProviders`
  - Number key 4 switches to `viewMonitor`
  - Number key 5 switches to `viewGovernance`
  - Number key 6 switches to `viewStaking`
  - Esc from `viewDeployments` returns to `viewDashboard`
  - Esc from `viewDeploymentDetail` returns to `viewDeployments`
  - Enter on deployment in `viewDeployments` (with selected record) transitions to `viewDeploymentDetail`
  - Command palette "deployments" command switches to `viewDeployments`

  These tests should construct an `App` via `newTestApp()` (the existing test helper in `app_test.go`), send `tea.KeyPressMsg` through `Update()`, and verify the resulting `activeView` field. Since `activeView` is unexported, the tests need to be in package `tui` (not `tui_test`).

- [ ] **Step 3:** Run tests: `go test ./internal/tui/ -run TestNav -v`
  Expected: All pass

- [ ] **Step 4:** Commit: `test(tui): add navigation view-switching tests`

### Subtask 1b: Keymap Tests (T066)

- [ ] **Step 1:** Create `internal/tui/keymap_test.go` with tests for:
  - `DefaultKeyMap()` returns non-nil KeyMap with all fields populated
  - `DefaultKeyMap()` binds `j`/`down` to CursorDown, `k`/`up` to CursorUp
  - `DefaultKeyMap()` binds `1`-`6` to view shortcuts
  - `DefaultKeyMap()` binds `?` to Help, `:` and `ctrl+p` to Command
  - `KeyMapFromConfig()` with nil Viper returns same as DefaultKeyMap
  - `KeyMapFromConfig()` with custom bindings overrides specific keys

  Tests should be in package `tui_test` (black-box). Verify key bindings by checking `key.Matches(tea.KeyPressMsg{Code: ...}, km.CursorDown)` returns true.

- [ ] **Step 2:** Run: `go test ./internal/tui/ -run TestKeyMap -v`

- [ ] **Step 3:** Commit: `test(tui): add keymap default and custom binding tests`

### Subtask 1c: Command Registry Tests (T067 partial)

- [ ] **Step 1:** Create `internal/tui/commands/registry_test.go` with tests for:
  - `DefaultRegistry()` returns registry with 19 commands
  - `DefaultRegistry().All()` returns all 19
  - `Filter("")` returns all commands
  - `Filter("dep")` matches "Deployments" and "Deploy" (substring on name)
  - `Filter("DEP")` matches same (case-insensitive)
  - `Filter("xyz")` returns empty
  - `Filter("monitor")` matches "Monitor" (via alias match)
  - `Filter("prov")` matches "Providers" (via alias)
  - Categories are present: "navigation", "action", "context", "app"

- [ ] **Step 2:** Run: `go test ./internal/tui/commands/ -v`

- [ ] **Step 3:** Commit: `test(tui): add command registry filter and lookup tests`

### Subtask 1d: View Tests (T068)

- [ ] **Step 1:** Create test files for each view. Each test follows the pattern:
  1. Create the view via `NewXView()`
  2. Call `SetSize(80, 24)`
  3. Call `View()` — verify non-empty, contains expected header text
  4. If `SetData()` exists: set sample data, verify `View()` contains data values
  5. Test cursor methods: `CursorDown()`, `CursorUp()` — verify `View()` changes
  6. If filter exists: test `CycleFilter()` changes visible rows

  Files to create: `dashboard_test.go`, `deployments_test.go`, `leases_test.go`, `deployment_detail_test.go`, `logviewer_test.go`, `command_test.go`, `help_test.go`, `listview_test.go`, `detailview_test.go`. All in package `views_test`.

  For `deployments_test.go`, use sample `store.DeploymentRecord` data. For `leases_test.go`, use sample `store.LeaseRecord` data. For `deployment_detail_test.go`, test all 4 sub-tabs. For `logviewer_test.go`, test `Open()`/`Close()`/`Active()`/`AppendLine()`/`TogglePause()`. For `command_test.go`, test `Open()`/`Close()`/`Active()` and input filtering. For `help_test.go`, test render contains all 4 sections (Navigation, Lists, Actions, Overlays).

- [ ] **Step 2:** Run: `go test ./internal/tui/views/ -v`

- [ ] **Step 3:** Commit: `test(tui): add unit tests for all view components`

### Subtask 1e: Cache Tests (T070)

- [ ] **Step 1:** Create `internal/monitor/cache/cache_test.go` with tests using a temporary bbolt database:
  - `Open()` with temp dir creates a valid store
  - `HasProviders()` returns false on empty store
  - `SyncWithChain()` with providers, then `HasProviders()` returns true
  - `GetAllProviders()` returns synced providers
  - `MarkProviderOnline()` sets version/CPU/memory/GPU fields
  - `MarkProviderOffline()` sets `IsOnline=false`
  - `GetOnlineProviders()` returns only online providers
  - `GetProvidersDueForCheck()` returns providers past their check interval
  - `GetProvidersByPriority()` returns providers in priority order (unchecked first)
  - `ProviderCount()` and `OnlineCount()` return correct counts
  - `Save()` persists to disk, `Open()` again reads them back

- [ ] **Step 2:** Create `internal/monitor/cache/moniker_test.go`:
  - `OpenMonikerCache()` creates valid cache
  - `Set()` + `Get()` round-trips moniker
  - `HasMonikers()` returns true after Set
  - `Save()` persists, reopening reads back

- [ ] **Step 3:** Run: `go test ./internal/monitor/cache/ -v`

- [ ] **Step 4:** Commit: `test(monitor): add provider cache and moniker cache tests`

### Subtask 1f: Theme Tests (T065)

- [ ] **Step 1:** Read `internal/ui/theme/theme_test.go` — this file already exists with 9 test functions covering all colors, styles, and backward compatibility. **T065 is already done.** Verify by running: `go test ./internal/ui/theme/ -v`. If all tests pass, mark T065 as complete.

- [ ] **Step 2:** Commit (if any additions needed): `test(ui): verify theme test coverage`

### Subtask 1g: Messages Tests

- [ ] **Step 1:** Create `internal/tui/messages/messages_test.go` — basic type assertion tests verifying each message type implements `tea.Msg` interface and carries the expected fields.

- [ ] **Step 2:** Run: `go test ./internal/tui/messages/ -v`

- [ ] **Step 3:** Commit: `test(tui): add message type validation tests`

---

## Task 2: Wire Help Overlay + Toast + Fix Address Truncation

**TASKS.md:** Partial T086 (wiring), address truncation bug fix

**Purpose:** Connect existing but unwired components (HelpOverlay, Toast) and fix the AGENTS.md-violating address truncation.

**Files:**
- Modify: `internal/tui/app.go` — add `helpOverlay` field, wire `?` key, render toast in `View()`, handle `ToastExpiredMsg`
- Modify: `internal/tui/views/deployment_detail.go` — remove `truncAddr()`, use full addresses

### Subtask 2a: Wire Help Overlay

- [ ] **Step 1:** Read `internal/tui/views/help.go` to understand the `HelpOverlay` API: `NewHelpOverlay()`, `Active()`, `Open()`, `Close()`, `View()`.

- [ ] **Step 2:** In `internal/tui/app.go`:
  - Add field: `helpOverlay views.HelpOverlay` to the `App` struct
  - In `newApp()`: initialize `helpOverlay: views.NewHelpOverlay()`
  - In `resize()`: call `a.helpOverlay.SetSize(w, h)` (if SetSize exists, otherwise skip — check the HelpOverlay API)
  - In `Update()` key handling: when `key.Matches(kmsg, a.keys.Help)` and no overlay is active, call `a.helpOverlay.Open()` and return
  - In `Update()`: when `helpOverlay.Active()`, intercept Esc to close it
  - In `View()`: when `a.helpOverlay.Active()`, overlay it on top of main content (same pattern as confirmDialog)

- [ ] **Step 3:** Run: `go test ./internal/tui/ -v` — existing golden tests may need updated golden files if footer rendering changes

- [ ] **Step 4:** Commit: `feat(tui): wire help overlay to ? key`

### Subtask 2b: Wire Toast Notifications

- [ ] **Step 1:** In `internal/tui/app.go`:
  - The `toast *components.Toast` field already exists
  - In `Update()`: add handler for `components.ToastExpiredMsg` — set `a.toast = nil`
  - In `View()`: after rendering the main content, if `a.toast != nil && !a.toast.Expired()`, append `a.toast.View()` to the output (position it at the bottom-right or top of the view)
  - Add a helper method `a.showToast(msg string, tone components.ToastTone)` that creates a new toast and returns the `ToastExpiredMsg` tick command

- [ ] **Step 2:** Run: `go test ./internal/tui/ -v`

- [ ] **Step 3:** Commit: `feat(tui): wire toast notifications into app view`

### Subtask 2c: Fix Address Truncation

- [ ] **Step 1:** In `internal/tui/views/deployment_detail.go`:
  - Remove the `truncAddr()` function (lines 370-376)
  - Find all call sites of `truncAddr()` and replace with the raw address string
  - Specifically: `providerAddr()` at line 389 calls `truncAddr(v.leases[0].ID.Provider)` — change to just `v.leases[0].ID.Provider`
  - Search the file for any other `truncAddr` calls and replace them

  Per AGENTS.md: "Never truncate or shorten addresses in output. Addresses (bech32, operator, consensus) must always be displayed in full."

- [ ] **Step 2:** Run: `go test ./internal/tui/... -v` — update any golden files affected

- [ ] **Step 3:** Commit: `fix(tui): display full addresses instead of truncated`

---

## Task 3: Design Documents (T071-T081)

**TASKS.md:** T071, T072, T073, T074, T075, T076, T077, T078, T079, T080, T081

**Purpose:** Create 11 standalone UX design documents describing the TUI as-built, with wireframes, component specifications, and color token references.

**Files:**
- Create: `design/tui-app-shell.md` (T071)
- Create: `design/tui-navigation.md` (T072)
- Create: `design/tui-theme.md` (T073)
- Create: `design/tui-keybindings.md` (T074)
- Create: `design/tui-resource-views.md` (T075)
- Create: `design/tui-consensus-validators.md` (T076)
- Create: `design/tui-provider-fleet.md` (T077)
- Create: `design/tui-governance-params.md` (T078)
- Create: `design/tui-oracle-bme.md` (T079)
- Create: `design/tui-command-palette.md` (T080)
- Create: `design/tui-confirm-logviewer.md` (T081)
- Read: SPEC.md §8 (TUI Specification) for reference
- Read: All `internal/tui/` and `internal/monitor/ui/` source files for as-built behavior

Each document should follow this template:

```markdown
# [Component Name] — UX Design

## Overview
[One paragraph describing what this component does and its role in the TUI.]

## Wireframe
[ASCII art wireframe showing the visual layout. Use box-drawing characters.]

## Component Specifications
[Detailed spec for each visual element: position, sizing, color tokens, content sources.]

## Color Tokens Used
[Reference to specific color variables from `internal/ui/theme/theme.go`.]

## Interaction
[Keyboard shortcuts, focus behavior, transitions to/from other components.]

## Data Sources
[Where the displayed data comes from (store, RPC, cache, etc.).]

## Implementation Reference
[Exact file paths to the implementation code.]

## SPEC.md Cross-Reference
[Which SPEC.md section(s) this component implements.]
```

### Subtask 3a-3k: One per document

- [ ] **Step 1:** Read the relevant source files and SPEC.md section for each document
- [ ] **Step 2:** Create the design document with full wireframes, component specs, color tokens, interaction details, data sources, and implementation references
- [ ] **Step 3:** Commit each document individually or as a batch: `docs(design): add TUI UX design documents`

**T071** (`design/tui-app-shell.md`): Header (1 line — app name, context badge, chain-id, account, block height, sync indicator), main area (fills remaining — active view), status bar (1-3 lines — view hints, connection info, global keys). Reference `internal/tui/app.go` `renderHeader()`, `renderFooter()`, `renderNavBar()`, `renderBreadcrumb()`. SPEC.md §8.1.

**T072** (`design/tui-navigation.md`): Flat `activeView` enum (8 views), number key 1-6 direct switching, Esc back to dashboard (or to parent in deployment detail), Enter drill-in, command palette routing. No navigation stack. Reference `internal/tui/app.go` Update() lines 350-480. SPEC.md §8.2.

**T073** (`design/tui-theme.md`): Zinc/neutral palette (Slate50-950), AccentRed, GreenColor/YellowColor for states, Blue/Purple accents. Typography: Heading, PrimaryValue, Body, Secondary, Muted. Component styles: header bar, nav tabs, breadcrumb, footer, table, KV detail, panel, state tags, overlay, buttons, progress, spinner. Reference `internal/ui/theme/theme.go`. SPEC.md §8.7.

**T074** (`design/tui-keybindings.md`): Vim-style defaults, 4 sections: global (quit/command/help/back), list navigation (j/k/g/G/Ctrl-d/Ctrl-u), detail view (j/k/y/Esc), resource actions (d/u/l/e/s/w/v/D/h/l/r). Custom keybindings via `tui.custom-keybindings` YAML config. Reference `internal/tui/keymap.go`. SPEC.md §8.6.

**T075** (`design/tui-resource-views.md`): Deployments (9 columns), leases (6 columns), providers (8 columns), orders, bids. List/detail pattern. Action keys per view. Reference `internal/tui/views/deployments.go`, `leases.go`, `providers.go`, `deployment_detail.go`. SPEC.md §8.3.1-8.3.4.

**T076** (`design/tui-consensus-validators.md`): Consensus state (height/round/step/elapsed/proposer), vote progress bars (prevote/precommit with block/blank chars and power fractions), validator vote grid, signing history bar per validator. Reference `internal/monitor/ui/view.go` renderOverviewTab, renderValidatorsTab. SPEC.md §8.3.8-8.3.9.

**T077** (`design/tui-provider-fleet.md`): Scan progress bar, version distribution dot visualization (semver-sorted), provider table (URL/version/CPU/memory/GPU), provider detail sub-view (node-level). Smart caching intervals. Reference `internal/monitor/ui/view.go` renderProvidersTab, `internal/monitor/cache/cache.go`. SPEC.md §8.3.10.

**T078** (`design/tui-governance-params.md`): Split-pane module browser (14 modules), pretty-printed params in right pane using shared `Render*Params()` functions. Reference `internal/monitor/ui/view.go` renderGovernanceTab, `internal/output/pretty/`. SPEC.md §8.3.11.

**T079** (`design/tui-oracle-bme.md`): Two-column layout — oracle (aggregated prices, price health) on left, BME (status, vault, ledger) on right. Color coding for health status. `FormatCoin()` for amounts. Reference `internal/monitor/ui/view.go` renderOracleBMEDashboard. SPEC.md §8.3.12.

**T080** (`design/tui-command-palette.md`): Centered floating overlay, 60% width (min 50, max 80 columns), text input with `:` prompt, scrollable filtered list, 19 commands across 4 categories. Reference `internal/tui/views/command.go`, `internal/tui/commands/registry.go`. SPEC.md §8.4.

**T081** (`design/tui-confirm-logviewer.md`): Confirmation dialog (5 kinds: close/vote/delegate/unbond/redelegate), fee preview, cancel/confirm buttons. Log viewer: streaming viewport, 500-line buffer, pause/resume, clear, service filter, search. Reference `internal/tui/components/confirm.go`, `internal/tui/views/logviewer.go`. SPEC.md §8.5, §8.3.7.

---

## Task 4: Add `GetProposals()` to rpc Package

**TASKS.md:** Prerequisite for T093 governance view

**Purpose:** Add a governance proposals query method that fetches proposals from the Cosmos REST LCD endpoint.

**Files:**
- Create: `internal/monitor/rpc/proposals.go`
- Create: `internal/monitor/rpc/proposals_test.go`

### Step-by-step

- [ ] **Step 1:** Create `internal/monitor/rpc/proposals.go` with a `Proposal` struct (ID, Title, Summary, Status, VotingStartTime, VotingEndTime, SubmitTime, DepositEndTime, FinalTallyResult), a `TallyResult` struct (YesCount, NoCount, AbstainCount, NoWithVetoCount), and two methods on `*Client`:
  - `GetProposals(ctx context.Context) ([]Proposal, error)` — paginated REST query to `/cosmos/gov/v1/proposals?pagination.limit=100&pagination.reverse=true`
  - `GetProposalTally(ctx context.Context, proposalID string) (*TallyResult, error)` — REST query to `/cosmos/gov/v1/proposals/{id}/tally` for live tally of voting-period proposals

  Follow the same HTTP request pattern used by `GetValidatorMonikers()` in `client.go`: create request with context, use `c.httpClient.Do(req)`, decode JSON, handle pagination via `next_key`.

- [ ] **Step 2:** Create `internal/monitor/rpc/proposals_test.go` with an `httptest.NewServer` that returns mock JSON responses. Test:
  - `GetProposals()` returns proposals from mock server
  - `GetProposals()` handles empty response
  - `GetProposals()` handles pagination
  - `GetProposalTally()` returns tally data
  - Error cases: server down, non-200 status

- [ ] **Step 3:** Run: `go test ./internal/monitor/rpc/ -run TestGetProposal -v`

- [ ] **Step 4:** Commit: `feat(rpc): add governance proposals REST query`

---

## Task 5: Add `GetValidatorsDetailed()` to rpc Package

**TASKS.md:** Prerequisite for T093 staking view

**Purpose:** Add a full validator data query that captures tokens, commission, status, jailed, etc. from the REST LCD endpoint. The existing `GetValidatorMonikers()` calls the same endpoint but only extracts moniker and pubkey.

**Files:**
- Create: `internal/monitor/rpc/staking_detailed.go`
- Create: `internal/monitor/rpc/staking_detailed_test.go`

### Step-by-step

- [ ] **Step 1:** Create `internal/monitor/rpc/staking_detailed.go` with a `ValidatorDetailed` struct containing all fields from the `/cosmos/staking/v1beta1/validators` response: OperatorAddress, ConsensusPubkey (with Key sub-field), Jailed, Status, Tokens, DelegatorShares, Description (Moniker, Identity, Website, SecurityContact, Details), Commission (CommissionRates with Rate, MaxRate, MaxChangeRate).

  Add method `GetValidatorsDetailed(ctx context.Context) ([]ValidatorDetailed, error)` on `*Client` — paginated REST query to `/cosmos/staking/v1beta1/validators?pagination.limit=100`.

- [ ] **Step 2:** Create `internal/monitor/rpc/staking_detailed_test.go` with httptest mock. Test pagination, empty response, error cases.

- [ ] **Step 3:** Run: `go test ./internal/monitor/rpc/ -run TestGetValidators -v`

- [ ] **Step 4:** Commit: `feat(rpc): add full validator data REST query`

---

## Task 6: Add Provider Lease Count + Audit Queries

**TASKS.md:** Prerequisite for T093 providers view

**Purpose:** Add queries for per-provider active lease count and audit attribute status.

**Files:**
- Modify: `internal/monitor/rpc/grpc.go` — Add `GetActiveLeaseCount()`
- Create: `internal/monitor/rpc/audit.go` — Audit attributes query
- Create: `internal/monitor/rpc/audit_test.go`
- Modify: `internal/monitor/cache/cache.go` — Add `LeaseCount`, `Audited`, `Auditors` to `CachedProvider`; add `MarkProviderAuditStatus()` and `MarkProviderLeaseCount()` to interface

### Step-by-step

- [ ] **Step 1:** In `internal/monitor/rpc/grpc.go`, add `GetActiveLeaseCount(ctx context.Context, restEndpoint string) (map[string]int, error)` on `*RPCProviderClient`. This paginates through `/akash/market/v1beta5/leases/list?filters.state=active&pagination.limit=100`, counts leases per provider address, and returns a `map[string]int`.

- [ ] **Step 2:** Create `internal/monitor/rpc/audit.go` with `GetAuditedProviders(ctx context.Context) (map[string][]string, error)` on `*Client`. Paginates through `/akash/audit/v1beta3/audit/attributes/list?pagination.limit=100`, returns `map[string][]string` (provider owner → auditor addresses).

- [ ] **Step 3:** In `internal/monitor/cache/cache.go`:
  - Add fields to `CachedProvider`: `LeaseCount int`, `Audited bool`, `Auditors []string`
  - Add methods to `ProviderStore` interface: `MarkProviderAuditStatus(owner string, audited bool, auditors []string)`, `MarkProviderLeaseCount(owner string, count int)`
  - Implement both in the bbolt backend (set fields on in-memory map entry)

- [ ] **Step 4:** Create `internal/monitor/rpc/audit_test.go` with httptest mock.

- [ ] **Step 5:** Run: `go test ./internal/monitor/rpc/ -run TestAudit -v && go test ./internal/monitor/cache/ -v`

- [ ] **Step 6:** Commit: `feat(rpc): add provider lease count and audit queries`

---

## Task 7: Governance View Data Binding

**TASKS.md:** T093 (partial — governance)

**Purpose:** Make the GovernanceView functional with real proposal data from the chain.

**Files:**
- Modify: `internal/tui/views/governance.go` — Add `SetData()` method with helper functions
- Modify: `internal/tui/messages/messages.go` — Add `ProposalsLoadedMsg`
- Modify: `internal/tui/app.go` — Add `rpcClient` field, `loadProposals()` command, handle `ProposalsLoadedMsg`, dispatch on view switch

### Step-by-step

- [ ] **Step 1:** Add `ProposalsLoadedMsg` to `internal/tui/messages/messages.go`:
  - Contains `Proposals []rpc.Proposal` and `Err error`

- [ ] **Step 2:** In `internal/tui/views/governance.go`, add `SetData(proposals []rpc.Proposal)`:
  - Convert each proposal to `components.TableRow` with cells: ID, Title, govStatusLabel(Status), tallyPercent for yes/no/abstain/veto, formatVotingEnd(VotingEndTime)
  - Add helper `govStatusLabel(status string) string` — maps protobuf status enum strings (e.g., "PROPOSAL_STATUS_VOTING_PERIOD") to human labels ("voting")
  - Add helper `tallyPercent(tally *rpc.TallyResult, field string) string` — computes percentage from count fields
  - Add helper `formatVotingEnd(t time.Time) string` — formats as relative time or date

- [ ] **Step 3:** In `internal/tui/app.go`:
  - Add `rpcClient *rpc.Client` field to `App` struct (or `monitorrpc.Client` — use the same type as the monitor)
  - Initialize in `newApp()` from `monitorrpc.NewClient(cfg.RPCEndpoint, cfg.RESTEndpoint)` (only when endpoints are non-empty)
  - Add `loadProposals(client *rpc.Client) tea.Cmd` — calls `client.GetProposals(ctx)`, returns `ProposalsLoadedMsg`
  - In `Update()`, handle `messages.ProposalsLoadedMsg`: if no error, call `a.governance.SetData(msg.Proposals)`
  - When switching to `viewGovernance` (key 5 or command palette), dispatch `loadProposals(a.rpcClient)`

- [ ] **Step 4:** Run: `go test ./internal/tui/... -v`

- [ ] **Step 5:** Commit: `feat(tui): wire governance view with proposal data from chain`

---

## Task 8: Staking View Data Binding

**TASKS.md:** T093 (partial — staking)

**Purpose:** Make the StakingView functional with real validator data from the chain.

**Files:**
- Modify: `internal/tui/views/staking.go` — Add `SetData()` with helper functions
- Modify: `internal/tui/messages/messages.go` — Add `ValidatorsLoadedMsg`
- Modify: `internal/tui/app.go` — Add `loadValidators()` command, handle message

### Step-by-step

- [ ] **Step 1:** Add `ValidatorsLoadedMsg` to `internal/tui/messages/messages.go`:
  - Contains `Validators []rpc.ValidatorDetailed` and `Err error`

- [ ] **Step 2:** In `internal/tui/views/staking.go`, add `SetData(validators []rpc.ValidatorDetailed, totalBonded string)`:
  - Convert each validator to `components.TableRow` with cells: rank (index+1), Moniker, formatTokens(Tokens), formatVPPercent(Tokens, totalBonded), formatCommission(Commission rate), "—" for uptime, "—" for signed
  - Add helper `formatTokens(tokens string) string` — converts from raw uakt string to human-readable using the same scaling logic as `output/pretty` helpers
  - Add helper `formatVPPercent(tokens, totalBonded string) string` — computes (tokens/totalBonded)*100
  - Add helper `formatCommission(rate string) string` — converts Dec string to percentage with 2 decimal places

  Note: Uptime and Signed columns show "—" for now — these require block signing history data that the monitor tracks but isn't easily shared. These can be filled in a future enhancement.

- [ ] **Step 3:** In `internal/tui/app.go`:
  - Add `loadValidators(client *rpc.Client) tea.Cmd` — calls `client.GetValidatorsDetailed(ctx)`, sorts by tokens descending, computes total bonded, returns `ValidatorsLoadedMsg`
  - In `Update()`, handle `ValidatorsLoadedMsg`: call `a.staking.SetData()`
  - When switching to `viewStaking` (key 6), dispatch `loadValidators(a.rpcClient)`

- [ ] **Step 4:** Run: `go test ./internal/tui/... -v`

- [ ] **Step 5:** Commit: `feat(tui): wire staking view with validator data from chain`

---

## Task 9: Providers View Data Binding

**TASKS.md:** T093 (partial — providers)

**Purpose:** Make the ProvidersView functional with cached provider data enriched with lease counts and audit status.

**Files:**
- Modify: `internal/tui/views/providers.go` — Add `SetData()` with formatting helpers
- Modify: `internal/tui/messages/messages.go` — Add `ProvidersLoadedMsg`
- Modify: `internal/tui/app.go` — Add `providerCache` field, `loadProviders()` command, handle message

### Step-by-step

- [ ] **Step 1:** Add `ProvidersLoadedMsg` to `internal/tui/messages/messages.go`:
  - Contains `Providers []*cache.CachedProvider` and `Err error`

- [ ] **Step 2:** In `internal/tui/views/providers.go`, add `SetData(providers []*cache.CachedProvider)`:
  - Filter to online providers
  - Convert each to `components.TableRow` using formatting helpers from `internal/monitor/ui/` (FormatProviderURL, FormatProviderGPU, FormatResourceRatio, FormatMemoryRatio)
  - Columns: HostURI, Country, GPU summary, CPU ratio, Memory ratio, LeaseCount, audit yes/no, Version

- [ ] **Step 3:** In `internal/tui/app.go`:
  - Add `providerCache cache.ProviderStore` field (extracted from the monitor model's config or opened from the same cache directory)
  - Add `loadProviders(cache cache.ProviderStore, rpcClient *rpc.RPCProviderClient, restClient *rpc.Client) tea.Cmd`:
    - Gets online providers from cache
    - If rpcClient available, fetches lease counts via `GetActiveLeaseCount()` and audit status via `restClient.GetAuditedProviders()`, merges into cached data
    - Returns `ProvidersLoadedMsg`
  - In `Update()`, handle `ProvidersLoadedMsg`: call `a.providers.SetData(msg.Providers)`
  - When switching to `viewProviders` (key 3), dispatch `loadProviders()`

- [ ] **Step 4:** Run: `go test ./internal/tui/... -v`

- [ ] **Step 5:** Commit: `feat(tui): wire providers view with cached provider data`

---

## Task 10: Live Sync Pipeline

**TASKS.md:** T104

**Purpose:** Connect the pubsub event bus → sync engine → store → TUI refresh so the TUI auto-updates when chain state changes.

**Files:**
- Create: `internal/tui/sync_bridge.go`
- Create: `internal/tui/sync_bridge_test.go`
- Modify: `internal/tui/app.go` — Expose bus, create sync bridge, handle refresh messages

### Architecture

```
CometBFT WebSocket → aktevents.Service → pubsub.Bus
                                              ↓
                                    sync_bridge.go subscribes
                                              ↓
                              sync.Engine.HandleEvent() called
                                              ↓
                                    store updated (bbolt)
                                              ↓
                              ViewDataRefreshMsg sent to TUI
                                              ↓
                           App.Update() dispatches re-load commands
```

### Step-by-step

- [ ] **Step 1:** Create `internal/tui/sync_bridge.go` with:
  - `syncBridge` struct containing `subscriber pubsub.Subscriber` and `engine *sync.Engine`
  - `newSyncBridge(bus pubsub.Bus, engine *sync.Engine) (*syncBridge, error)` — subscribes to bus
  - `waitForEvent() tea.Cmd` — blocks until next bus event, feeds it to `engine.HandleEvent()`, returns `messages.ViewDataRefreshMsg`
  - `close()` — shuts down subscriber

  The `waitForEvent()` pattern matches the monitor's `waitForBusEvent()`: returns a `tea.Cmd` closure that blocks on `sub.Events()` channel, processes the event, and returns a message.

- [ ] **Step 2:** Modify `internal/tui/app.go`:
  - Change `buildMonitorModel()` to also return the `pubsub.Bus` (currently the bus is local to that function and inaccessible to the App)
  - Add `syncBridge *syncBridge` field to App struct
  - In `newApp()`: create `sync.Engine` from `cfg.Store` and tracked accounts, then `newSyncBridge(bus, engine)`
  - In `Init()`: include `sb.waitForEvent()` in initial commands
  - In `Update()`: handle `messages.ViewDataRefreshMsg`:
    - Re-dispatch the data loading command for the current active view
    - Re-arm with `sb.waitForEvent()`
  - In cleanup: call `sb.close()`

- [ ] **Step 3:** Create `internal/tui/sync_bridge_test.go`:
  - Test: publishing a mock event to a `pubsub.NewBus()` produces a `ViewDataRefreshMsg` from `waitForEvent()`
  - Test: `close()` stops the subscriber and `waitForEvent()` returns nil
  - Test: nil bridge returns nil command
  - Test: engine HandleEvent is called for each bus event

- [ ] **Step 4:** Run: `go test ./internal/tui/ -run TestSyncBridge -v`

- [ ] **Step 5:** Commit: `feat(tui): add reactive sync pipeline (bus -> sync -> store -> refresh)`

---

## Task 11: Log Streaming Backend

**TASKS.md:** T096

**Purpose:** Wire provider gateway log streaming into the LogViewer component so pressing `l` on a deployment actually shows live container logs.

**Files:**
- Modify: `internal/tui/views/logviewer.go` — Ensure API supports streaming input
- Modify: `internal/tui/app.go` — Create provider client, start log stream, feed lines to LogViewer
- Modify: `internal/tui/messages/messages.go` — Add `LogLineMsg`, `LogStreamEndedMsg`

### Step-by-step

- [ ] **Step 1:** Add message types to `internal/tui/messages/messages.go`:
  - `LogLineMsg` with `Line views.LogLine` and `Err error`
  - `LogStreamEndedMsg` with `Err error`

- [ ] **Step 2:** In `internal/tui/app.go`:
  - Add a `logCancel context.CancelFunc` field — used to cancel an active log stream
  - When the LogViewer is opened (pressing `l` on a deployment in `viewDeployments`):
    1. Look up the deployment record to get DSEQ, provider address, and auth details
    2. Create a context with cancel: `ctx, cancel := context.WithCancel(context.Background())`
    3. Store `cancel` in `a.logCancel`
    4. Start a `tea.Cmd` that:
       - Creates a provider REST client (from `internal/provider`)
       - Calls the provider's lease-logs endpoint with follow=true
       - For each log line received, returns `LogLineMsg{Line: views.LogLine{...}}`
       - On stream end, returns `LogStreamEndedMsg{}`
    5. The Cmd pattern should use a channel-based approach similar to the bus event pattern, or return a single line per Cmd call and re-arm
  - In `Update()`:
    - Handle `LogLineMsg`: call `a.logViewer.AppendLine(msg.Line)`, re-arm the log stream command
    - Handle `LogStreamEndedMsg`: show toast "Log stream ended", do not re-arm
  - When LogViewer is closed (Esc):
    - Call `a.logCancel()` to stop the goroutine
    - Set `a.logCancel = nil`

  Note: The provider log endpoint returns NDJSON. The `internal/provider/` package wraps `akash-network/provider` REST client which already handles log streaming. The exact API depends on what the provider client exposes — read `internal/provider/client.go` to determine the streaming interface.

- [ ] **Step 3:** Run: `go test ./internal/tui/ -v`

- [ ] **Step 4:** Commit: `feat(tui): wire provider log streaming into log viewer`

---

## Task 12: Dashboard Enhancements

**TASKS.md:** T092

**Purpose:** Enhance the dashboard view with live data indicators — sync status, active connection indicator, and recent activity.

**Files:**
- Modify: `internal/tui/views/dashboard.go` — Add sync status indicator, connection status

### Step-by-step

- [ ] **Step 1:** Read `internal/tui/views/dashboard.go` to understand the current layout. The dashboard already shows account panel, active deployments, network panel, and shortcuts. It receives data via `SetContext()`, `SetStats()`, `SetSyncState()`, `SetActiveDeployments()`.

- [ ] **Step 2:** Enhance the dashboard:
  - In the Network panel: add a "Sync" row showing the sync bridge status — add `SetSyncBridgeActive(active bool)` method; display "Live" (green) when sync bridge is connected, "Offline" (muted) when not
  - In the Account panel: show tracking count from the context's `tracked-accounts` if available
  - Optionally: add a "Recent Activity" section showing count of recent actions (if action log is accessible via the context)

- [ ] **Step 3:** In `internal/tui/app.go`: after creating the sync bridge, call `a.dashboard.SetSyncBridgeActive(true)`. If the bridge fails to create, call `SetSyncBridgeActive(false)`.

- [ ] **Step 4:** Run: `go test ./internal/tui/... -v` — update golden files

- [ ] **Step 5:** Commit: `feat(tui): enhance dashboard with sync status and connection indicators`

---

## Task 13: E2E TUI Smoke Tests

**TASKS.md:** T105

**Purpose:** Add end-to-end smoke tests that verify the TUI launches, renders, handles basic navigation, and exits cleanly.

**Files:**
- Create: `e2e/tui_test.go`

### Step-by-step

- [ ] **Step 1:** The TUI requires a TTY and RPC connection, making full interactive E2E testing challenging. The approach uses the bubbletea test infrastructure or headless model testing.

Create `e2e/tui_test.go` with tests that:
  - Verify `akt monitor --help` prints help text and exits 0
  - Verify `akt monitor network --help` prints help text and exits 0
  - Verify `akt --help` does not launch the TUI (prints help instead)
  - If possible using bubbletea's `teatest` package: construct a `tui.App` model with a nil store and no RPC, send `tea.WindowSizeMsg`, verify `View()` output contains expected header elements, send `Ctrl+c`, verify quit

  The test binary is at `.cache/bin/akt` (built via `make akt`). CLI-level tests use `exec.Command`.

- [ ] **Step 2:** Run: `go test ./e2e/ -run TestTUI -v`

- [ ] **Step 3:** Commit: `test(e2e): add TUI smoke tests`

---

## Verification Checklist

After all tasks are complete, verify:

- [ ] `go test ./internal/tui/... -v` — all TUI tests pass
- [ ] `go test ./internal/monitor/... -v` — all monitor tests pass
- [ ] `go test ./internal/ui/... -v` — theme tests pass
- [ ] `go test ./e2e/ -v` — E2E tests pass
- [ ] `go build ./...` — project builds
- [ ] `make akt` — binary builds
- [ ] All 11 design documents exist in `design/`
- [ ] TASKS.md tasks T064-T081, T086, T089, T092-T096, T104, T105 can be marked complete
- [ ] AICHANGELOG.md updated with entries for all changes
- [ ] No `truncAddr()` calls remain in the codebase
- [ ] `?` key opens help overlay in the TUI
- [ ] Toast notifications render when actions complete
- [ ] Pressing `5` in the TUI shows governance proposals (if RPC is available)
- [ ] Pressing `6` shows validators with commission data
- [ ] Pressing `3` shows providers with GPU/CPU/memory from cache
- [ ] Store-backed views auto-refresh when chain events arrive
- [ ] Pressing `l` on a deployment streams logs (if provider is reachable)

---

## TASKS.md Mapping

| Task | TASKS.md IDs Covered |
|------|---------------------|
| Task 1 | T064, T065, T066, T067, T068, T070 |
| Task 2 | Partial T086 (wiring), bug fix |
| Task 3 | T071-T081 |
| Task 4 | Prerequisite for T093 |
| Task 5 | Prerequisite for T093 |
| Task 6 | Prerequisite for T093 |
| Task 7 | T093 (governance) |
| Task 8 | T093 (staking) |
| Task 9 | T093 (providers) |
| Task 10 | T104 |
| Task 11 | T096 |
| Task 12 | T092 |
| Task 13 | T105 |

Note: T069 (monitor view tests) is already marked complete in TASKS.md. T065 (theme tests) already has comprehensive coverage in `internal/ui/theme/theme_test.go`.
