# Tasks: akt — Akash Network Unified CLI & TUI

**Input**: DESIGN.md (architecture), SPEC.md (technical specification)
**Prerequisites**: DESIGN.md (required), SPEC.md (required)

**Tests**: TDD is mandated — all implementation tasks include corresponding test tasks written and failing before implementation.

**Organization**: Tasks are organized by user story (US1–US4 mapping to the 4 implementation phases from DESIGN.md §6), each delivering incremental value.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

```text
cmd/akt/                           # Binary entry point
internal/
├── actionlog/                     # Action log (unique per context)
├── bootstrap/                     # First-run bootstrap wizard
├── cli/                           # CLI mode (cobra commands)
│   ├── chain/                     # Clean-copied chain-sdk tx/query
│   ├── context/                   # Context management commands
│   ├── keys/                      # Key management commands
│   ├── network/                   # Network management commands
│   ├── provider/                  # Provider gateway commands
│   ├── store/                     # Store management commands
│   ├── workflow/                  # Workflow CLI wrappers
│   └── plugin/                    # Plugin management
├── client/                        # Chain client
├── cliutil/                       # CLI utilities (errors, status, verbosity)
├── codec/                         # Application-wide encoding config
├── context/                       # Context management core
├── events/                        # Shared blockchain event service (pubsub bus)
├── filter/                        # Resource filter argument parsing (§3.8)
├── flags/                         # Shared flag definitions
├── glyphs/                        # Glyph registry (ASCII-safe)
├── keyring/                       # Keyring abstraction
├── mcp/                           # MCP server (akt mcp)
│   ├── marshal/                   # Parameter extraction and result helpers
│   └── tools/                     # MCP tool definitions and handlers
│       ├── node/                  # Node status, block height
│       ├── bank/                  # Account balances
│       ├── deployment/            # Deployment queries and close tx
│       ├── market/                # Orders, bids, leases queries and txs
│       ├── provider/              # Provider queries and REST tools
│       ├── audit/                 # Audited provider queries
│       └── cert/                  # Certificate queries
├── monitor/                       # Real-time monitoring (akt monitor)
│   ├── ui/                        # Bubbletea model, views, styles
│   ├── consensus/                 # Consensus state types
│   ├── governance/                # Governance parameter types
│   ├── rpc/                       # RPC/WebSocket/gRPC clients
│   └── cache/                     # Persistent cache (bbolt)
├── output/                        # Output formatting
│   └── pretty/                    # Pretty output (registry-based)
├── console/                       # Console API client
├── provider/                      # Provider gateway client
├── plugin/                        # Plugin system
├── store/                         # Local deployment store
│   └── bbolt/                     # bbolt backend
├── sync/                          # Chain sync engine
├── tui/                           # TUI mode (bubbletea)
│   ├── views/                     # TUI view models
│   ├── components/                # Reusable TUI components
│   ├── commands/                  # Command palette registry
│   └── messages/                  # Custom bubbletea messages
├── ui/                            # Shared UI utilities
│   └── theme/                     # Unified theme package
└── workflow/                      # Declarative workflow engine
    ├── steps/                     # Step type implementations
    └── builtin/                   # Embedded workflow definitions
e2e/                               # End-to-end tests
```

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project scaffold and build tooling

- [x] T001 [P] Verify project scaffold: go.mod, cmd/akt/main.go, Makefile, .goreleaser.yaml, CI (GitHub Actions), .golangci.yml, LICENSE
- [x] T002 [P] Verify go.work and chain-sdk replace directives are correct

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can proceed

**CRITICAL**: No user story work can begin until this phase is complete

- [x] T003 Verify application-wide encoding config in internal/codec/

**Checkpoint**: Foundation ready — user story implementation can begin

---

## Phase 3: User Story 1 — Foundation (Context + Core CLI) (Priority: P1) 🎯 MVP

**Goal**: A functional CLI that replaces basic `akash tx` and `akash query` operations with the context system, key management, output formatting, and all chain commands.

**Independent Test**: Run `make akt && go test ./...`, then exercise context CRUD, network templates, key management, and basic tx/query against a local testnet.

### Tests for User Story 1

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T004 [P] [US1] Unit tests for config system: YAML read/write, XDG path resolution, env var loading, schema version in internal/context/config_test.go
- [x] T005 [P] [US1] Unit tests for config live-reload: fsnotify watcher, change detection, subscriber notification in internal/context/watcher_test.go
- [x] T006 [P] [US1] Unit tests for context manager: CRUD, composition (network+keyring+store+log), switching, resolution chain, fork/edit-parent in internal/context/manager_test.go
- [x] T007 [P] [US1] Unit tests for keyring manager: shared multi-keyring, backend abstraction, keys visible to all referencing contexts in internal/keyring/manager_test.go
- [x] T008 [P] [US1] Unit tests for action log: append, read with filters, rotation at 10MB in internal/actionlog/log_test.go
- [x] T009 [P] [US1] Unit tests for output formatting: pretty registry, JSON/YAML output, FormatCoin, FormatNumber, FormatPower in internal/output/pretty/helpers_test.go
- [x] T010 [P] [US1] Unit tests for resource filter parsing: smart type detection, get-vs-list heuristic, --by provider mode in internal/cli/chain/flags/filters_test.go
- [x] T011 [P] [US1] Unit tests for error handling: CLIError type, exit code extraction, three-part messages in internal/cliutil/errors_test.go
- [x] T012 [P] [US1] Unit tests for glyph registry: semantic name lookup, ASCII-only output in internal/glyphs/glyphs_test.go
- [x] T013 [P] [US1] Unit tests for unified theme: color constants, style definitions in internal/ui/theme/theme_test.go
- [x] T014 [P] [US1] Unit tests for transaction result pretty formatters: TxPrettyFormatter dispatch, common summary rendering, per-message formatting in internal/output/pretty/tx_test.go

### Implementation for User Story 1

#### US1-Design: UX Design Tasks

- [x] T015 [P] [US1] CLI output format design: table column layouts per resource type, color/state indicator scheme, non-TTY fallback behavior per SPEC.md §10
- [x] T016 [P] [US1] Error message UX design: error format template (what/context/suggestion), exit code mapping (0-7, 127), debug vs user-facing output per SPEC.md §11
- [x] T017 [P] [US1] Interactive prompt UX design: confirmation prompts, account selection, context switching, fork-vs-edit-parent flow per SPEC.md §3.9

#### US1-Config: Configuration System

- [x] T018 [US1] Config system: YAML config read/write, XDG path resolution ($AKT_HOME > $XDG_CONFIG_HOME/akt > ~/.config/akt), env var loading (AKT_* prefix), schema version in internal/context/config.go
- [x] T019 [US1] Config live-reload: fsnotify watcher with polling fallback, change detection, subscriber notification pattern in internal/context/watcher.go

#### US1-Context: Context & Network System

- [x] T020 [US1] Network management: shared network type, CRUD commands (create/delete/edit/list/show), built-in templates (mainnet/testnet/sandbox), cross-context sharing in internal/context/types.go and internal/cli/network/commands.go
- [x] T021 [US1] Context manager: context type (composes network+keyring+store+log), CRUD (create/delete/edit/list/current/use/rename), fork/edit-parent for networks, context propagation & override chain (flag>env>config>default) in internal/context/manager.go

#### US1-Keyring: Keyring & Key Management

- [x] T022 [US1] Keyring integration: shared multi-keyring support, keyring CRUD, backend abstraction (os/file/test/kwallet/pass), keys visible to all referencing contexts in internal/keyring/manager.go
- [x] T023 [US1] Key management commands: add, delete, export, import, list, show, rename, mnemonic, parse in internal/cli/keys/commands.go

#### US1-Client: Chain Client

- [x] T024 [US1] Chain client: full client (tx+query) and light client (query-only), multi-endpoint failover with health checks (30s interval), connection timeout & retry, endpoint promotion in internal/client/context.go

#### US1-Commands: TX, Query, and Auth Commands

- [x] T025 [US1] Transaction commands: all tx modules (bank, deployment, market, provider, cert, audit, staking, distribution, gov, authz, feegrant, escrow, wasm, oracle, bme, slashing, vesting, upgrade, crisis, IBC) in internal/cli/chain/*_tx.go
- [x] T026 [US1] Query commands: all query modules (bank, deployment, market, provider, cert, audit, escrow, staking, distribution, gov, auth, authz, feegrant, evidence, mint, params, slashing, wasm, oracle, bme, ibc, ibc-transfer, upgrade, block, tx) in internal/cli/chain/*_query.go
- [x] T027 [US1] Auth utility commands: sign, sign-batch, multisign, validate-signatures, broadcast, encode, decode in internal/cli/chain/auth_*.go and internal/cli/chain/broadcast.go

#### US1-Output: Output, Error & Filter Formatting

- [x] T028 [P] [US1] Action log: append-only JSONL logger per context, ActionEntry types (tx/query/workflow/provider/context/error), reading/filtering, log rotation (10MB, 5 files) in internal/actionlog/log.go
- [x] T029 [P] [US1] Output formatting: registry-based PrettyFormatter per protobuf type, JSON/YAML via cctx.PrintProto(), FormatCoin auto-scaling (micro→milli→base), TTY detection, stream separation (data→stdout, everything else→stderr) in internal/output/pretty/
- [x] T030 [P] [US1] Global flags & env mapping: --context, --home, --output (-o), --interactive (-i), --verbose (-v), --quiet (-q); AKT_* env vars (12 variables per §1.9); override chain resolution; AddTxFlagsToCmd, AddQueryFlagsToCmd, AddPaginationFlagsToCmd in internal/cli/root.go and internal/flags/
- [x] T031 [P] [US1] Error handling framework: CLIError type (code/message/cause/suggestion/context), structured exit codes (0-7, 127), debug logging, three-part error messages, typo suggestions (Levenshtein distance=2) in internal/cliutil/errors.go
- [x] T032 [P] [US1] Resource filter argument parsing: smart type detection (bech32 vs uint), /-separated path, --by provider mode, get-vs-list heuristic, per-command filter scope per SPEC.md §3.8 in internal/filter/filter.go
- [x] T033 [P] [US1] Transaction result pretty formatters: TxPrettyFormatter interface, common summary section (hash/signer/height/gas/fee/status), per-message detail section with registered formatters for all 30+ message types (bank, deployment, market, provider, cert, audit, staking, distribution, gov, authz, feegrant, escrow, slashing, vesting, upgrade, crisis, wasm, oracle, bme), JSON fallback for unregistered types per SPEC.md §10.11 in internal/output/pretty/tx_formatters.go

#### US1-Polish: Shell Completion, Version & Bootstrap

- [x] T034 [P] [US1] Shell completion: bash, zsh, fish, powershell completion scripts via cobra, dynamic completion for context/network names in internal/cli/root.go
- [x] T035 [P] [US1] Version command: build-time version/commit/date injection, --long flag for full build info in internal/cli/root.go
- [x] T036 [P] [US1] First-run bootstrap wizard: detect empty config, prompt for network selection (from akash-network/net repo), keyring backend selection (os/file/test), context creation in internal/bootstrap/bootstrap.go

### E2E Tests for User Story 1

- [x] T037 [US1] E2E test suite: version, help, completion generation, network template CRUD, context lifecycle (create/list/show/rename/switch/delete), unknown command error in e2e/cli_test.go

**Checkpoint**: User Story 1 complete — `akt` can replace basic `akash tx` and `akash query` operations

---

## Phase 4: User Story 2 — Store + Workflow Commands (Priority: P2)

**Goal**: Local state tracking and high-level workflow commands that orchestrate multi-step deployment operations.

**Independent Test**: Run deploy workflow e2e, verify sync engine integration, store round-trip, and provider commands against a mock provider.

### Tests for User Story 2

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T038 [P] [US2] Unit tests for Store interface: deployment/lease/bid CRUD, sync state, schema versioning in internal/store/bbolt/store_test.go
- [x] T039 [P] [US2] Unit tests for schema migration framework: versioned schema (uint64), forward-only migration in internal/store/bbolt/migrate_test.go
- [x] T040 [P] [US2] Unit tests for sync engine: event routing (filter by owner/dseq/type), state reconciler in internal/sync/engine_test.go
- [x] T041 [P] [US2] Unit tests for workflow engine: step execution, template evaluation, error handling, retry in internal/workflow/engine_test.go
- [x] T042 [P] [US2] Unit tests for Console API client: request building, response parsing, error handling in internal/console/client_test.go
- [x] T043 [P] [US2] Unit tests for store export/import: YAML/JSON round-trip, merge/replace modes, dry-run in internal/store/bbolt/export_test.go
- [x] T044 [P] [US2] Unit tests for shared events service: pubsub bus, event filtering, subscriber lifecycle in internal/events/bus_test.go

### Design for User Story 2

- [x] T045 [P] [US2] Deploy workflow interactive UX design: bid presentation table, bid selection flow, step-by-step progress display, JSONL output mode per SPEC.md §2.3.8 (covered by existing SPEC §2.3.8 wireframes)
- [x] T046 [P] [US2] Provider command output design: lease-status layout, log stream format, event stream format, shell connection UX (covered by existing SPEC §2.4)

### Implementation for User Story 2

#### US2-Events: Shared Event Service

- [x] T047 [US2] Shared blockchain event service: pubsub bus for distributing chain events (NewBlock, Tx) to sync engine and TUI, subscriber registration, event filtering by module/type in internal/events/service.go (existing) + internal/events/bus_test.go

#### US2-Store: Deployment Store

- [x] T048 [US2] Store interface + bbolt backend: Store interface (deployment/lease/bid CRUD, sync state, schema, import/export), bbolt bucket structure (deployments/, leases/, bids/, sync/, meta/), concurrent-safe implementation in internal/store/store.go + internal/store/bbolt/store.go
- [x] T049 [US2] Schema migration framework: versioned schema (uint64), migration functions per version, forward-only in single bbolt tx in internal/store/bbolt/migrate.go
- [x] T050 [US2] Store export/import: YAML/JSON export with header metadata (version, context, schema, sync state), import with merge/replace modes, --dry-run, round-trip fidelity in internal/store/bbolt/export.go
- [x] T051 [US2] Store status command: display store path, DB size, schema version, record counts (active/closed deployments, leases, bids), sync state in internal/cli/store/commands.go

#### US2-Sync: Sync Engine

- [x] T052 [US2] Sync engine: event router (filter by owner/dseq/type), state reconciler (maps 7 chain event types to store CRUD per SPEC.md §6.3) in internal/sync/engine.go
- [x] T053 [US2] Startup reconciliation: full reconciliation on first launch (query all deployments/leases/bids for tracked accounts), gap detection (>1000 blocks → full re-sync), exponential backoff (1s→60s cap + jitter) in internal/sync/reconcile.go

#### US2-Workflow: Deploy/Update/Close

- [x] T054 [US2] akt deploy workflow: embedded YAML definition + cobra command with flags, loads workflow via Loader, validates SDL in internal/cli/workflow/commands.go
- [x] T055 [US2] akt update workflow: embedded YAML definition + cobra command with --dseq and positional arg in internal/cli/workflow/commands.go
- [x] T056 [US2] akt close workflow: embedded YAML definition + cobra command with --dseq and positional arg in internal/cli/workflow/commands.go

#### US2-Provider: Provider Gateway

- [x] T057 [US2] Provider gateway client: thin wrapper around chain-sdk rest.Client with JWT/mTLS auth in internal/provider/client.go
- [x] T058 [US2] Provider CLI commands: status, lease-status, lease-logs (--follow, --tail, --service), lease-events, lease-shell, send-manifest, get-manifest, migrate-hostnames, migrate-endpoints in internal/cli/provider/commands.go

#### US2-Console: Console API

- [x] T059 [US2] Console API client: HTTP client for Console managed wallet API with x-api-key auth, 8 endpoints (deployments CRUD, bids, leases, deposit), retry with backoff for 429/5xx in internal/console/client.go

#### US2-Events: Event Streaming Command

- [x] T060 [P] [US2] Events command: live blockchain event streaming (akt events), filter by module/type, TUI auto-launch when no subcommand in internal/cli/chain/events.go

#### US2-MCP: MCP Server

- [x] T061 [P] [US2] MCP server: stdio transport (JSON-RPC over stdin/stdout), 21 read-only tools (node status, block height, balances, deployments, orders, bids, leases, providers, audits, certs), 4 write tools (close deployment, create lease, close lease, submit manifest) gated behind --enable-writes flag, LightClient for read-only / Client for write mode in internal/mcp/server.go and internal/mcp/tools/

### E2E Tests for User Story 2

- [x] T062 [US2] E2E test suite: deploy workflow e2e, sync engine integration, store round-trip, provider commands against mock provider in e2e/

**Checkpoint**: User Story 2 complete — `akt deploy/update/close` workflows operational, local store syncing with chain

---

## Phase 5: User Story 3 — TUI Mode (Priority: P3)

**Goal**: A fully interactive terminal UI for real-time Akash management, incorporating `akt monitor` hub with Network, Provider, and Oracle/BME dashboards.

**Independent Test**: TUI smoke tests (launch, navigate, resize), component unit tests, view rendering tests.

### Tests for User Story 3

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T063 [P] [US3] Unit tests for TUI application shell: root model init, header render, status bar render, window resize in internal/tui/app_test.go
- [ ] T064 [P] [US3] Unit tests for navigation system: view stack push/pop, breadcrumb trail, number-key dispatch in internal/tui/nav_test.go
- [ ] T065 [P] [US3] Unit tests for theme system: dark/light themes, color token resolution, NO_COLOR support in internal/ui/theme/theme_test.go
- [ ] T066 [P] [US3] Unit tests for configurable keybindings: vim/default/custom keymap loading in internal/tui/keymap_test.go
- [ ] T067 [P] [US3] Unit tests for command palette: fuzzy matching, category grouping, keyboard navigation in internal/tui/views/palette_test.go
- [ ] T068 [P] [US3] Unit tests for ListView and DetailView components: cursor, scroll, selection in internal/tui/views/listview_test.go and internal/tui/views/detailview_test.go
- [x] T069 [P] [US3] Unit tests for monitor model: consensus state parsing, vote grid rendering, progress bars in internal/monitor/ui/view_test.go
- [ ] T070 [P] [US3] Unit tests for provider cache: smart scheduling, priority queue, disk persistence in internal/monitor/cache/cache_test.go

### Design for User Story 3

- [ ] T071 [P] [US3] TUI application shell design: header layout (app name, context, chain-id, account, block height, sync status), main content area proportions, status bar layout (shortcut hints), responsive resize behavior
- [ ] T072 [P] [US3] TUI navigation model design: stack-based navigation spec, breadcrumb rendering, number-key quick access mapping (1-6), Esc back behavior
- [ ] T073 [P] [US3] Theme system design: dark/light color palettes, semantic color tokens (primary/secondary/accent/background/foreground/muted/border/error/warning/success/info), lipgloss style definitions, custom theme YAML schema per SPEC.md §8.7
- [ ] T074 [P] [US3] Keybinding scheme design: vim-style default bindings (j/k/g/G/Ctrl-d/Ctrl-u), non-vim defaults, custom keybinding YAML schema per SPEC.md §8.6
- [ ] T075 [P] [US3] Resource view wireframes: deployments, leases, providers, orders, bids list/detail patterns per SPEC.md §8.3
- [ ] T076 [P] [US3] Consensus & validator view design: consensus state layout (height/round/step/elapsed/proposer), vote progress bars (prevote/precommit with power fractions), validator vote grid (●/○), signing history bar per SPEC.md §8.3.8-8.3.9
- [ ] T077 [P] [US3] Provider fleet monitor view design: scan progress bar, version distribution dot visualization (●/○ semver-sorted), provider table (URL/version/CPU/memory/GPU), detail sub-view (node-level) per SPEC.md §8.3.10
- [ ] T078 [P] [US3] Governance params view design: split-pane layout (module list + params), pretty-printed output using shared Render*Params() functions per SPEC.md §8.3.11
- [ ] T079 [P] [US3] Oracle/BME dashboard design: two-column layout (oracle left, BME right), aggregated prices table, price health section, BME status/vault/ledger sections per SPEC.md §8.3.12
- [ ] T080 [P] [US3] Command palette design: overlay positioning (60% width, min 50, max 80 cols), fuzzy search, result categories (navigation/action/app), keyboard interaction per SPEC.md §8.4
- [ ] T081 [P] [US3] Confirmation dialog & log viewer design: dialog overlay (fee preview, Cancel/Confirm), log auto-scroll, search highlight, service filter per SPEC.md §8.5

### Implementation for User Story 3

#### US3-Shell: Application Shell & Infrastructure

- [x] T082 [US3] Bubbletea application shell: root model, header component (context/chain-id/account/block height/sync status), status bar component (shortcut hints), main area routing, window resize handling in internal/tui/app.go
- [x] T083 [US3] Navigation system: view stack, breadcrumb trail, back/forward (Esc), number-key quick access (1-9), Tab/Shift-Tab for monitor dashboard switching, view lifecycle (init/focus/blur) in internal/tui/nav.go
- [x] T084 [P] [US3] Theme system implementation: lipgloss dark/light themes, custom theme loading from config, color token resolution, NO_COLOR support in internal/ui/theme/theme.go
- [x] T085 [P] [US3] Configurable keybindings: keymap loading from config (vim/default/custom), context-sensitive binding resolution, help text generation from keymap, PaletteKeys struct in internal/tui/keymap.go

#### US3-Components: Reusable TUI Components

- [ ] T086 [P] [US3] Resource table component: generic sortable/filterable table (bubbles/table), column configuration per resource type, pagination, terminal-width-aware column sizing in internal/tui/components/table.go
- [x] T087 [P] [US3] Detail pane component: scrollable viewport (bubbles/viewport), YAML/JSON toggle, syntax highlighting in internal/tui/views/detailview.go
- [x] T088 [P] [US3] Command palette: centered floating overlay with text input (bubbles/textinput) + scrollable list (bubbles/list), case-insensitive substring fuzzy matching, category grouping (navigation/action/app), keyboard-driven selection (j/k/Enter/Esc), 17 registered commands per SPEC.md §8.4 in internal/tui/views/palette.go
- [ ] T089 [P] [US3] Confirmation dialog: modal overlay for destructive actions, transaction summary (owner/balance/gas/fees), Cancel (Esc) / Confirm (Enter) buttons in internal/tui/components/confirm.go
- [x] T090 [P] [US3] Help overlay: keybinding reference panel (bubbles/help), view-specific action listing, toggle with ? in internal/tui/components/help.go
- [x] T091 [P] [US3] Progress bar & vote grid components: progress bar (bubbles/progress for consensus votes + provider scanning), vote grid (●/○ voted/not-voted line-wrapped to terminal width) in internal/monitor/ui/styles.go

#### US3-Views: Resource Views

- [ ] T092 [US3] Dashboard view: home/landing view, summary stats (active deployments, total spend, sync status), quick actions in internal/tui/views/dashboard.go
- [ ] T093 [P] [US3] Deployments view: list (DSEQ/State/Provider/Price/Balance/Age), detail (full info + lease + bids), actions (d=close, u=update, l=logs, /=filter, f=state filter) in internal/tui/views/deployments.go
- [ ] T094 [P] [US3] Leases view: list (DSEQ/GSeq/OSeq/Provider/State/Price/Age), detail (lease info + endpoints), actions (l=logs, e=events, s=shell, w=withdraw) in internal/tui/views/leases.go
- [ ] T095 [P] [US3] Providers view: list (Address/URI/Audited/Active Leases), detail (attributes, resources) in internal/tui/views/providers.go
- [ ] T096 [P] [US3] Log viewer: streaming viewport with auto-scroll, --service filter, /search, wrap toggle (w), follow toggle (f) in internal/tui/views/logviewer.go

#### US3-Monitor: Monitor Hub (Network, Provider, Oracle/BME)

- [x] T097 [US3] Monitor hub: hub-based real-time monitoring with Tab/Shift-Tab dashboard switching (Network/Provider/Oracle-BME), subcommand routing (akt monitor network/provider/oracle/bme), standalone operation (RPC-only, no keyring required) in internal/monitor/ui/view.go
- [x] T098 [US3] Consensus monitor view (Network dashboard tab 1): real-time polling of /consensus_state (1s default, 250ms fast mode), height/round/step display with thousand separators, prevote/precommit progress bars (█/░ 40-chars, green ≥66.7%), validator vote grid (●=green/○=muted), proposer indicator in internal/monitor/ui/view.go
- [x] T099 [US3] Validator voting view (Network dashboard tab 2): scrollable validator table (bubbles/table), moniker resolution from /cosmos/staking/v1beta1/validators (cached in ~/.config/akt/cache/monikers.json), voting power formatting (K/M/B), prevote/precommit status (✓ green/✗ red), proposer indicator (* yellow bold), j/k scroll, signing history bar in internal/monitor/ui/view.go
- [x] T100 [US3] Governance params view (Network dashboard tab 3): split-pane module browser (bubbles/list for 12 modules + bubbles/viewport for params), pretty-printed params using shared Render*Params() functions for visual parity with CLI (§10.8), 5m refresh in internal/monitor/ui/view.go
- [x] T101 [US3] Provider fleet monitor view (Provider dashboard): provider list from chain (ABCI query), gRPC+REST health checks (gRPC 8444 preferred, REST /status+/version fallback), version distribution (semver-sorted newest-first, handles -rc suffixes, ●/○ dot visualization), provider table (URL/version/CPU/memory/GPU), provider detail sub-view (node-level CPU/memory/GPU model+count), h/l version selection in internal/monitor/ui/view.go
- [x] T102 [US3] Provider cache: smart-scheduled cache (online:1m, recently-offline:5m, long-term-offline:6h), priority queue (unchecked→online→recent-offline→long-offline), max 10 concurrent checks, disk persistence at ~/.config/akt/cache/providers.json (save every 30s), chain re-sync every 10m in internal/monitor/cache/cache.go
- [x] T103 [US3] Oracle/BME dashboard: two-column layout (oracle left, BME right), aggregated prices (TWAP/median/min-max/sources/deviation/health per-denom), price health (color-coded green=healthy/red=unhealthy), BME status (healthy=green/warning=yellow/halt=red, mints/refunds allowed/halted), vault (balances/burned/minted/remint credits via FormatCoin()), ledger table (route/ID/status/burned/minted/spread/remint), REST polling (oracle 30s, BME 30s, history+ledger 2m) in internal/monitor/ui/view.go

#### US3-Integration: Live Data

- [ ] T104 [US3] Live sync integration: store change notifications trigger TUI re-renders, view updates within 2s of chain state change, sync status indicator in header in internal/tui/app.go

### E2E Tests for User Story 3

- [ ] T105 [US3] E2E test suite: TUI smoke tests (launch, navigate, resize), component unit tests, view rendering tests in e2e/

**Checkpoint**: User Story 3 complete — full interactive TUI with live data, `akt monitor` hub operational

---

## Phase 6: User Story 4 — Extended Features (Priority: P4)

**Goal**: Complete feature set with plugin system, additional TUI views, in-TUI transaction actions, and performance optimization.

**Independent Test**: Full coverage of all CLI commands + TUI interactions, CI target >80% coverage.

### Tests for User Story 4

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T106 [P] [US4] Unit tests for plugin discovery: scan paths (plugins dir + config paths + $PATH), precedence rules (first match wins), disabled list filtering in internal/plugin/discovery_test.go
- [ ] T107 [P] [US4] Unit tests for plugin execution: subprocess env vars (12 AKT_* variables), stdin/stdout/stderr inheritance, exit code forwarding in internal/plugin/exec_test.go
- [ ] T108 [P] [US4] Unit tests for plugin manifest parsing: YAML schema, version requirements, optional fields in internal/plugin/manifest_test.go

### Design for User Story 4

- [ ] T109 [P] [US4] Plugin UX design: install flow (trust warning, progress), plugin help integration with cobra, list display format, error messaging for missing plugins (exit code 127)
- [ ] T110 [P] [US4] Additional TUI view designs: certificates, governance proposals (with vote action), validators (with delegate/undelegate/redelegate), escrow, wasm, oracle, BME, IBC per SPEC.md §8.3
- [ ] T111 [P] [US4] TUI transaction action flow design: in-TUI create deployment (SDL path input), fund escrow, close deployment, vote on proposals with confirmation dialog per SPEC.md §8.5

### Implementation for User Story 4

#### US4-Plugin: Plugin System

- [ ] T112 [US4] Plugin discovery: scan ~/.config/akt/plugins/ + config plugin paths + $PATH for akt-* executables, precedence rules (first match wins), respect plugins.disabled list in internal/plugin/discovery.go
- [ ] T113 [US4] Plugin execution: fork/exec subprocess, pass 12 AKT_* env vars (AKT_PLUGIN=1, AKT_HOME, AKT_CONTEXT, AKT_CHAIN_ID, AKT_NODE, AKT_GRPC_ADDR, AKT_FROM, AKT_KEYRING_BACKEND, AKT_KEYRING_DIR, AKT_OUTPUT, AKT_STORE_PATH), inherit stdin/stdout/stderr, exit with plugin's exit code in internal/plugin/exec.go
- [ ] T114 [US4] Plugin management commands: akt plugin install (GitHub URL or --local path with symlink), akt plugin list (name/version/source/path table), akt plugin remove in internal/cli/plugin/commands.go
- [ ] T115 [US4] Plugin manifest: parse optional plugin.yaml (name/version/description/usage/short-description/long-description/requires/min-akt-version), display in list and help, trust warning on install in internal/plugin/manifest.go

#### US4-Views: Additional TUI Views

- [ ] T116 [P] [US4] Certificates TUI view: list (Serial/State/Owner), detail (cert content, expiry) in internal/tui/views/certificates.go
- [ ] T117 [P] [US4] Governance TUI view: proposal list (ID/Title/Status/Yes/No/Abstain with percentages), detail (full proposal info + timeline + tally), vote action (v=vote, D=deposit, option selection + confirmation dialog) in internal/tui/views/governance.go
- [ ] T118 [P] [US4] Validators TUI view: validator list (Rank/Moniker/Power/Commission/Uptime/Delegated), detail (identity/staking/commission/description), actions (d=delegate, u=undelegate, r=redelegate) in internal/tui/views/validators.go
- [ ] T119 [P] [US4] Escrow TUI view: account list (Scope/XID/Owner/State/Balance/Spent), detail (payment history) in internal/tui/views/escrow.go
- [ ] T120 [P] [US4] Wasm TUI view: contract list (Address/CodeID/Label), detail (contract info, admin, created), state queries (smart query input) in internal/tui/views/wasm.go
- [ ] T121 [P] [US4] Oracle TUI view: price table (Asset/Base/Price/Timestamp) in internal/tui/views/oracle.go
- [ ] T122 [P] [US4] BME TUI view: vault state (balances/burned/minted/remint credits), mint status (healthy/warning/halt with collateral ratio), recent ledger entries table in internal/tui/views/bme.go
- [ ] T123 [P] [US4] IBC TUI view: channel list (Channel ID/Port/Counterparty/State) in internal/tui/views/ibc.go

#### US4-Actions: In-TUI Transaction Actions

- [ ] T124 [US4] TUI transaction actions: create deployment from TUI (SDL path input → deploy workflow), fund escrow (amount input → deposit tx), close deployment (confirmation → close tx), vote on proposals (option selection → vote tx), all with confirmation dialog in internal/tui/views/ and internal/tui/components/

#### US4-Performance: Optimization

- [ ] T125 [US4] Performance optimization: lazy loading for large lists (load on scroll), virtual scrolling (>10k items), pagination preloading (fetch next page before user reaches end) in internal/tui/components/

### E2E Tests for User Story 4

- [ ] T126 [US4] Comprehensive E2E test suite: full coverage of all CLI commands + TUI interactions, CI target >80% coverage in e2e/

**Checkpoint**: User Story 4 complete — full feature set with plugin system and optimized TUI

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T127 [P] Documentation updates (user guide, quickstart) in docs/
- [ ] T128 Code cleanup and refactoring across all packages
- [ ] T129 [P] Security audit: verify no keys in config, MCP write tool gating (--enable-writes), destructive action guards (--yes vs --force semantics), plugin trust warnings
- [ ] T130 [P] AICHANGELOG.md entries for all implemented tasks

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational
- **User Story 2 (Phase 4)**: Depends on User Story 1 completion
- **User Story 3 (Phase 5)**: Depends on User Story 1 + User Story 2 completion
- **User Story 4 (Phase 6)**: Depends on User Story 1 + User Story 2 + User Story 3 completion
- **Polish (Phase 7)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational — No dependencies on other stories
- **User Story 2 (P2)**: Depends on User Story 1 (needs context system, chain client, output formatting)
- **User Story 3 (P3)**: Depends on User Story 1 + User Story 2 (needs store, sync engine, workflows)
- **User Story 4 (P4)**: Depends on User Story 1 + User Story 2 + User Story 3 (extends TUI with additional views)

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Design tasks before implementation tasks
- Models/types before services
- Services before CLI commands
- Core implementation before integration
- E2E tests validate the story checkpoint

### Parallel Opportunities

**User Story 1 tests** (T004-T014) can all run in parallel
**User Story 1 design** (T015-T017) can all run in parallel
**User Story 1 implementation**: Config (T018-T019) must precede Context (T020-T021), which must precede Client (T024), which must precede Commands (T025-T027). Output/error tasks (T028-T036) can run in parallel with each other.
**User Story 2 tests** (T038-T044) can all run in parallel
**User Story 2 design** (T045-T046) can all run in parallel
**User Story 3 design** (T071-T081) can all run in parallel
**User Story 3 components** (T086-T091) can all run in parallel
**User Story 3 resource views** (T093-T096) can all run in parallel
**User Story 3 monitor views** (T097-T103): T097 hub must precede individual dashboards; individual dashboard views can run in parallel
**User Story 4 views** (T116-T123) can all run in parallel

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: "Unit tests for config system in internal/context/config_test.go"            # T004
Task: "Unit tests for config live-reload in internal/context/watcher_test.go"       # T005
Task: "Unit tests for context manager in internal/context/manager_test.go"          # T006
Task: "Unit tests for keyring manager in internal/keyring/manager_test.go"          # T007
Task: "Unit tests for action log in internal/actionlog/log_test.go"                 # T008
Task: "Unit tests for output formatting in internal/output/pretty/helpers_test.go"  # T009
Task: "Unit tests for resource filter parsing in internal/filter/filter_test.go"    # T010
Task: "Unit tests for error handling in internal/cliutil/errors_test.go"            # T011
Task: "Unit tests for glyph registry in internal/glyphs/glyphs_test.go"            # T012
Task: "Unit tests for unified theme in internal/ui/theme/theme_test.go"             # T013
Task: "Unit tests for tx result formatters in internal/output/pretty/tx_test.go"    # T014

# Launch all parallel output tasks together:
Task: "Action log in internal/actionlog/log.go"                                     # T028
Task: "Output formatting in internal/output/pretty/"                                # T029
Task: "Global flags & env mapping in internal/cli/root.go"                          # T030
Task: "Error handling framework in internal/cliutil/errors.go"                      # T031
Task: "Resource filter argument parsing in internal/filter/filter.go"               # T032
Task: "Transaction result pretty formatters in internal/output/pretty/tx_formatters.go" # T033
Task: "Shell completion in internal/cli/root.go"                                    # T034
Task: "Version command in internal/cli/root.go"                                     # T035
Task: "First-run bootstrap wizard in internal/bootstrap/wizard.go"                  # T036
```

## Parallel Example: User Story 3

```bash
# Launch all reusable TUI components together:
Task: "Resource table component in internal/tui/components/table.go"                # T086
Task: "Detail pane component in internal/tui/views/detailview.go"                   # T087
Task: "Command palette in internal/tui/views/palette.go"                            # T088
Task: "Confirmation dialog in internal/tui/components/confirm.go"                   # T089
Task: "Help overlay in internal/tui/components/help.go"                             # T090
Task: "Progress bar & vote grid in internal/monitor/ui/styles.go"                   # T091

# Launch all resource views together:
Task: "Deployments view in internal/tui/views/deployments.go"                       # T093
Task: "Leases view in internal/tui/views/leases.go"                                # T094
Task: "Providers view in internal/tui/views/providers.go"                           # T095
Task: "Log viewer in internal/tui/views/logviewer.go"                               # T096
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T002)
2. Complete Phase 2: Foundational (T003)
3. Write User Story 1 tests (T004-T014) — verify they fail
4. Complete User Story 1 design (T015-T017)
5. Complete User Story 1 implementation (T018-T036)
6. Verify tests pass (T004-T014)
7. Run E2E (T037)
8. **STOP and VALIDATE**: `akt` can replace basic `akash tx/query`

### Incremental Delivery

1. User Story 1 → MVP CLI (tx/query/context) → validate
2. User Story 2 → Store + deploy workflow + MCP server → validate
3. User Story 3 → Full TUI + monitor hub → validate
4. User Story 4 → Plugins + extended views → validate
5. Each story adds value without breaking previous stories

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- Each user story is independently completable and testable
- Tests are written and FAIL before implementation
- Design tasks precede implementation tasks
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
