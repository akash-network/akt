# Tasks: akt — Akash Network Unified CLI & TUI

**Input**: DESIGN.md (architecture), SPEC.md (technical specification), AGENTS.md (development rules)
**Prerequisites**: DESIGN.md (required), SPEC.md (required), constitution (ratified v1.0.0)
**Linear project**: [akt CLI](https://linear.app/akash-network/project/akt-cli-08901b17698c) (Akash Network team)

**Tests**: TDD is mandated by Constitution Principle III. All implementation tasks include corresponding test tasks written and failing before implementation.

**Organization**: Tasks are organized by the 4 implementation phases from DESIGN.md §6, each treated as an independent user story that delivers incremental value.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which phase story this task belongs to (P1, P2, P3, P4)
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
├── events/                        # Shared blockchain event service
├── filter/                        # Resource filter argument parsing
├── flags/                         # Shared flag definitions
├── glyphs/                        # Glyph registry (ASCII-safe)
├── keyring/                       # Keyring abstraction
├── mcp/                           # MCP server
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

- [ ] T001 [P] Verify project scaffold: go.mod, cmd/akt/main.go, Makefile, .goreleaser.yaml, CI (GitHub Actions), .golangci.yml, LICENSE [`AKT-205`](https://linear.app/akash-network/issue/AKT-205)
- [ ] T002 [P] Verify go.work and chain-sdk replace directives are correct

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY phase story can proceed

**CRITICAL**: No phase story work can begin until this phase is complete

- [ ] T003 Verify application-wide encoding config in internal/codec/

**Checkpoint**: Foundation ready — phase story implementation can begin

---

## Phase 3: Phase 1 — Foundation (Context + Core CLI) 🎯 MVP

**Goal**: A functional CLI that replaces basic `akash tx` and `akash query` operations with the context system, key management, output formatting, and all chain commands.

**Independent Test**: Run `make akt && go test ./...`, then exercise context CRUD, network templates, key management, and basic tx/query against a local testnet.

### Tests for Phase 1

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T004 [P] [P1] Unit tests for config system: YAML read/write, XDG path resolution, env var loading, schema version in internal/context/config_test.go [`AKT-206`](https://linear.app/akash-network/issue/AKT-206)
- [ ] T005 [P] [P1] Unit tests for config live-reload: fsnotify watcher, change detection, subscriber notification in internal/context/watcher_test.go [`AKT-207`](https://linear.app/akash-network/issue/AKT-207)
- [ ] T006 [P] [P1] Unit tests for context manager: CRUD, composition (network+keyring+store+log), switching, resolution chain, fork/edit-parent in internal/context/manager_test.go [`AKT-209`](https://linear.app/akash-network/issue/AKT-209)
- [ ] T007 [P] [P1] Unit tests for keyring manager: shared multi-keyring, backend abstraction, keys visible to all referencing contexts in internal/keyring/manager_test.go [`AKT-210`](https://linear.app/akash-network/issue/AKT-210)
- [ ] T008 [P] [P1] Unit tests for action log: append, read with filters, rotation at 10MB in internal/actionlog/log_test.go [`AKT-211`](https://linear.app/akash-network/issue/AKT-211)
- [ ] T009 [P] [P1] Unit tests for output formatting: pretty registry, JSON/YAML output, FormatCoin, FormatNumber, FormatPower in internal/output/pretty/helpers_test.go [`AKT-213`](https://linear.app/akash-network/issue/AKT-213)
- [ ] T010 [P] [P1] Unit tests for resource filter parsing: smart type detection, get-vs-list heuristic, --by provider in internal/filter/filter_test.go [`AKT-216`](https://linear.app/akash-network/issue/AKT-216)
- [ ] T011 [P] [P1] Unit tests for error handling: CLIError type, exit code extraction, three-part messages in internal/cliutil/errors_test.go [`AKT-214`](https://linear.app/akash-network/issue/AKT-214)
- [ ] T012 [P] [P1] Unit tests for glyph registry: semantic name lookup, ASCII-only output in internal/glyphs/glyphs_test.go
- [ ] T013 [P] [P1] Unit tests for unified theme: color constants, style definitions in internal/ui/theme/theme_test.go

### Implementation for Phase 1

#### P1-Design: UX Design Tasks

- [ ] T014 [P] [P1] CLI output format design: table column layouts per resource type, color/state indicator scheme, non-TTY fallback behavior in docs/ or SPEC.md §10 [`AKT-202`](https://linear.app/akash-network/issue/AKT-202)
- [ ] T015 [P] [P1] Error message UX design: error format template (what/context/suggestion), exit code mapping (0-7, 127), debug vs user-facing output in docs/ or SPEC.md §11 [`AKT-203`](https://linear.app/akash-network/issue/AKT-203)
- [ ] T016 [P] [P1] Interactive prompt UX design: confirmation prompts, account selection, context switching, fork-vs-edit-parent flow in docs/ or SPEC.md §3 [`AKT-204`](https://linear.app/akash-network/issue/AKT-204)

#### P1-Config: Configuration System

- [ ] T017 [P1] Config system: YAML config read/write, XDG path resolution ($AKT_HOME > $XDG_CONFIG_HOME/akt > ~/.config/akt), env var loading (AKT_* prefix), schema version in internal/context/config.go [`AKT-206`](https://linear.app/akash-network/issue/AKT-206)
- [ ] T018 [P1] Config live-reload: fsnotify watcher with polling fallback, change detection, subscriber notification pattern in internal/context/watcher.go [`AKT-207`](https://linear.app/akash-network/issue/AKT-207)

#### P1-Context: Context & Network System

- [ ] T019 [P1] Network management: shared network type, CRUD commands (create/delete/edit/list/show), built-in templates (mainnet/testnet/sandbox), cross-context sharing in internal/context/types.go and internal/cli/network/commands.go [`AKT-208`](https://linear.app/akash-network/issue/AKT-208)
- [ ] T020 [P1] Context manager: context type (composes network+keyring+store+log), CRUD (create/delete/edit/list/current/use/rename), fork/edit-parent for networks, context propagation & override chain (flag>env>config>default) in internal/context/manager.go [`AKT-209`](https://linear.app/akash-network/issue/AKT-209)

#### P1-Keyring: Keyring & Key Management

- [ ] T021 [P1] Keyring integration: shared multi-keyring support, keyring CRUD, backend abstraction (os/file/test/kwallet/pass), keys visible to all referencing contexts in internal/keyring/manager.go [`AKT-210`](https://linear.app/akash-network/issue/AKT-210)
- [ ] T022 [P1] Key management commands: add, delete, export, import, list, show, rename, mnemonic, parse in internal/cli/keys/commands.go [`AKT-218`](https://linear.app/akash-network/issue/AKT-218)

#### P1-Client: Chain Client

- [ ] T023 [P1] Chain client: full client (tx+query) and light client (query-only), multi-endpoint failover with health checks, connection timeout & retry, endpoint promotion in internal/client/context.go [`AKT-212`](https://linear.app/akash-network/issue/AKT-212)

#### P1-Commands: TX, Query, and Auth Commands

- [ ] T024 [P1] Transaction commands: all tx modules (bank, deployment, market, provider, cert, audit, staking, distribution, gov, authz, feegrant, escrow, wasm, oracle, bme, slashing, vesting, upgrade, crisis, IBC) in internal/cli/chain/*_tx.go [`AKT-215`](https://linear.app/akash-network/issue/AKT-215)
- [ ] T025 [P1] Query commands: all query modules (bank, deployment, market, provider, cert, audit, escrow, staking, distribution, gov, auth, authz, feegrant, evidence, mint, params, slashing, wasm, oracle, bme, ibc, ibc-transfer, upgrade, block, tx) in internal/cli/chain/*_query.go [`AKT-217`](https://linear.app/akash-network/issue/AKT-217)
- [ ] T026 [P1] Auth utility commands: sign, sign-batch, multisign, validate-signatures, broadcast, encode, decode in internal/cli/chain/auth_*.go and internal/cli/chain/broadcast.go [`AKT-219`](https://linear.app/akash-network/issue/AKT-219)

#### P1-Output: Output & Error Formatting

- [ ] T027 [P] [P1] Action log: append-only JSONL logger per context, ActionEntry types (tx/query/workflow/provider/context/error), reading/filtering, log rotation (10MB, 5 files) in internal/actionlog/log.go [`AKT-211`](https://linear.app/akash-network/issue/AKT-211)
- [ ] T028 [P] [P1] Output formatting: registry-based PrettyFormatter per protobuf type, JSON/YAML via cctx.PrintProto(), FormatCoin auto-scaling, TTY detection in internal/output/pretty/ [`AKT-213`](https://linear.app/akash-network/issue/AKT-213)
- [ ] T029 [P] [P1] Global flags & env mapping: --context, --home, --output, -v/-q; AKT_* env vars; override chain resolution; AddTxFlagsToCmd, AddQueryFlagsToCmd, AddPaginationFlagsToCmd in internal/cli/root.go and internal/flags/ [`AKT-216`](https://linear.app/akash-network/issue/AKT-216)
- [ ] T030 [P] [P1] Error handling framework: CLIError type (code/message/cause/suggestion/context), structured exit codes (0-7, 127), debug logging in internal/cliutil/errors.go [`AKT-214`](https://linear.app/akash-network/issue/AKT-214)
- [ ] T031 [P] [P1] Resource filter argument parsing: smart type detection, /‐separated path, --by provider mode, get-vs-list heuristic in internal/filter/

#### P1-Polish: Shell Completion & Version

- [ ] T032 [P] [P1] Shell completion: bash, zsh, fish completion scripts via cobra, dynamic completion for context/network names in internal/cli/root.go [`AKT-220`](https://linear.app/akash-network/issue/AKT-220)
- [ ] T033 [P] [P1] Version command: build-time version/commit/date injection, --long flag for full build info in internal/cli/root.go [`AKT-221`](https://linear.app/akash-network/issue/AKT-221)

### E2E Tests for Phase 1

- [ ] T034 [P1] E2E test suite: context CRUD, network templates, key management, basic tx/query against local testnet in e2e/ [`AKT-222`](https://linear.app/akash-network/issue/AKT-222)

**Checkpoint**: Phase 1 complete — `akt` can replace basic `akash tx` and `akash query` operations

---

## Phase 4: Phase 2 — Store + Workflow Commands

**Goal**: Local state tracking and high-level workflow commands that orchestrate multi-step deployment operations.

**Independent Test**: Run deploy workflow e2e, verify sync engine integration, store round-trip, and provider commands against a mock provider.

### Tests for Phase 2

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T035 [P] [P2] Unit tests for Store interface: deployment/lease/bid CRUD, sync state, schema versioning in internal/store/bbolt/store_test.go
- [ ] T036 [P] [P2] Unit tests for schema migration framework: versioned schema (uint64), forward-only migration in internal/store/bbolt/migrate_test.go
- [ ] T037 [P] [P2] Unit tests for sync engine: event routing (filter by owner/dseq/type), state reconciler in internal/sync/sync_test.go
- [ ] T038 [P] [P2] Unit tests for workflow engine: step execution, template evaluation, error handling, retry in internal/workflow/engine_test.go
- [ ] T039 [P] [P2] Unit tests for Console API client: request building, response parsing, error handling in internal/console/client_test.go
- [ ] T040 [P] [P2] Unit tests for store export/import: YAML/JSON round-trip, merge/replace modes, dry-run in internal/store/bbolt/export_test.go

### Design for Phase 2

- [ ] T041 [P] [P2] Deploy workflow interactive UX design: bid presentation table, bid selection flow, step-by-step progress display, JSONL output mode
- [ ] T042 [P] [P2] Provider command output design: lease-status layout, log stream format, event stream format, shell connection UX

### Implementation for Phase 2

#### P2-Store: Deployment Store

- [ ] T043 [P2] Store interface + bbolt backend: Store interface (deployment/lease/bid CRUD, sync state, schema, import/export), bbolt bucket structure (deployments/, leases/, bids/, sync/, meta/), concurrent-safe implementation in internal/store/bbolt/store.go
- [ ] T044 [P2] Schema migration framework: versioned schema (uint64), migration functions per version, forward-only in single bbolt tx in internal/store/bbolt/migrate.go
- [ ] T045 [P2] Store export/import: YAML/JSON export with header metadata, import with merge/replace modes, --dry-run, round-trip fidelity in internal/store/bbolt/export.go
- [ ] T046 [P2] Store status command: display store path, DB size, schema version, record counts (active/closed), sync state in internal/cli/store/commands.go

#### P2-Sync: Sync Engine

- [ ] T047 [P2] Sync engine: WebSocket subscription (Tx+NewBlock events), event router (filter by owner/dseq/type), state reconciler (maps chain events to store CRUD) in internal/sync/engine.go
- [ ] T048 [P2] Startup reconciliation: full reconciliation on first launch (query all deployments/leases/bids for tracked accounts), incremental sync on subsequent launches, gap detection (>1000 blocks -> full re-sync) in internal/sync/reconcile.go

#### P2-Workflow: Deploy/Update/Close

- [ ] T049 [P2] akt deploy workflow: full lifecycle (create deployment tx -> wait for bids -> select bid -> create lease tx -> send manifest -> wait for active -> display endpoints), TUI mode + JSONL mode in internal/cli/workflow/deploy.go
- [ ] T050 [P2] akt update workflow: update deployment tx + send manifest to providers with active leases in internal/cli/workflow/update.go
- [ ] T051 [P2] akt close workflow: close deployment tx with confirmation in internal/cli/workflow/close.go

#### P2-Provider: Provider Gateway

- [ ] T052 [P2] Provider gateway client: REST/gRPC gateway client, JWT and mTLS auth, log/event streaming (WebSocket/SSE) in internal/provider/client.go
- [ ] T053 [P2] Provider CLI commands: status, lease-status, lease-logs (--follow, --tail, --service), lease-events, lease-shell (exec + TTY), send-manifest, get-manifest, migrate-hostnames, migrate-endpoints in internal/cli/provider/commands.go

#### P2-Console: Console API

- [ ] T054 [P2] Console API client: auth-method console-api context support, API key via AKT_CONSOLE_API_KEY, deployment operations via Console managed wallet API in internal/console/client.go

#### P2-Events: Event Streaming

- [ ] T055 [P2] Events command: live blockchain event streaming (akt events), filter by module/type, TUI auto-launch when no subcommand in internal/cli/events.go

### E2E Tests for Phase 2

- [ ] T056 [P2] E2E test suite: deploy workflow e2e, sync engine integration, store round-trip, provider commands against mock provider in e2e/

**Checkpoint**: Phase 2 complete — `akt deploy/update/close` workflows operational, local store syncing with chain

---

## Phase 5: Phase 3 — TUI Mode

**Goal**: A fully interactive terminal UI for real-time Akash management, incorporating `akt monitor` hub with Network, Provider, and Oracle/BME dashboards.

**Independent Test**: TUI smoke tests (launch, navigate, resize), component unit tests, view rendering tests.

### Tests for Phase 3

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T057 [P] [P3] Unit tests for TUI application shell: root model init, header render, status bar render, window resize in internal/tui/app_test.go
- [ ] T058 [P] [P3] Unit tests for navigation system: view stack push/pop, breadcrumb trail, number-key dispatch in internal/tui/nav_test.go
- [ ] T059 [P] [P3] Unit tests for theme system: dark/light themes, color token resolution, NO_COLOR support in internal/ui/theme/theme_test.go
- [ ] T060 [P] [P3] Unit tests for configurable keybindings: vim/default/custom keymap loading in internal/tui/keymap_test.go
- [ ] T061 [P] [P3] Unit tests for command palette: fuzzy matching, category grouping, keyboard navigation in internal/tui/views/palette_test.go
- [ ] T062 [P] [P3] Unit tests for ListView and DetailView components: cursor, scroll, selection in internal/tui/views/listview_test.go and internal/tui/views/detailview_test.go
- [ ] T063 [P] [P3] Unit tests for monitor model: consensus state parsing, vote grid rendering, progress bars in internal/monitor/ui/view_test.go
- [ ] T064 [P] [P3] Unit tests for provider cache: smart scheduling, priority queue, disk persistence in internal/monitor/cache/cache_test.go

### Design for Phase 3

- [ ] T065 [P] [P3] TUI application shell design: header layout, main content area proportions, status bar layout, responsive resize behavior
- [ ] T066 [P] [P3] TUI navigation model design: stack-based navigation spec, breadcrumb rendering, number-key quick access mapping
- [ ] T067 [P] [P3] Theme system design: dark/light color palettes, semantic color tokens, lipgloss style definitions, custom theme YAML schema
- [ ] T068 [P] [P3] Keybinding scheme design: vim-style default bindings, non-vim defaults, custom keybinding YAML schema
- [ ] T069 [P] [P3] Resource view wireframes: deployments, leases, providers, orders, bids list/detail patterns
- [ ] T070 [P] [P3] Consensus & validator view design: consensus state layout, vote progress bars, validator vote grid, signing history bar
- [ ] T071 [P] [P3] Provider fleet monitor view design: scan progress bar, version distribution dot visualization, provider table, detail sub-view
- [ ] T072 [P] [P3] Governance params view design: split-pane layout (module list + params), pretty-printed output
- [ ] T073 [P] [P3] Command palette design: overlay positioning, fuzzy search, result categories, keyboard interaction
- [ ] T074 [P] [P3] Confirmation dialog & log viewer design: dialog overlay, fee preview, log auto-scroll, search highlight

### Implementation for Phase 3

#### P3-Shell: Application Shell & Infrastructure

- [ ] T075 [P3] Bubbletea application shell: root model, header component, status bar component, main area routing, window resize handling in internal/tui/app.go
- [ ] T076 [P3] Navigation system: view stack, breadcrumb trail, back/forward (Esc), number-key quick access (1-6), view lifecycle (init/focus/blur) in internal/tui/nav.go
- [ ] T077 [P] [P3] Theme system implementation: lipgloss dark/light themes, custom theme loading from config, color token resolution, NO_COLOR support in internal/ui/theme/theme.go
- [ ] T078 [P] [P3] Configurable keybindings: keymap loading from config (vim/default/custom), context-sensitive binding resolution, help text generation from keymap in internal/tui/keymap.go

#### P3-Components: Reusable TUI Components

- [ ] T079 [P] [P3] Resource table component: generic sortable/filterable table (bubbles/table), column configuration per resource type, pagination, terminal-width-aware in internal/tui/components/
- [ ] T080 [P] [P3] Detail pane component: scrollable viewport (bubbles/viewport), YAML/JSON toggle, syntax highlighting in internal/tui/views/detailview.go
- [ ] T081 [P] [P3] Command palette: overlay with text input (bubbles/textinput), fuzzy matching, category grouping, keyboard-driven selection in internal/tui/views/palette.go
- [ ] T082 [P] [P3] Confirmation dialog: modal overlay for destructive actions, transaction summary, Cancel/Confirm buttons in internal/tui/components/
- [ ] T083 [P] [P3] Help overlay: keybinding reference panel (bubbles/help), view-specific action listing, toggle with ? in internal/tui/components/
- [ ] T084 [P] [P3] Progress bar & vote grid components: progress bar (consensus votes, provider scanning), vote grid (voted/not voted, line-wrapped) in internal/monitor/ui/styles.go

#### P3-Views: Resource Views

- [ ] T085 [P3] Dashboard view: home/landing view, summary stats (active deployments, total spend, sync status), quick actions in internal/tui/views/dashboard.go
- [ ] T086 [P] [P3] Deployments view: list (DSEQ/State/Provider/Price/Balance/Age), detail (full info + lease + bids), actions (close/update/logs) in internal/tui/views/deployments.go
- [ ] T087 [P] [P3] Leases view: list (DSEQ/GSeq/OSeq/Provider/State/Price/Age), detail (lease info + endpoints), actions (logs/events/shell) in internal/tui/views/leases.go
- [ ] T088 [P] [P3] Providers view: list (Address/URI/Audited/Active Leases), detail (attributes, resources) in internal/tui/views/providers.go
- [ ] T089 [P] [P3] Log viewer: streaming viewport with auto-scroll, --service filter, /search, wrap toggle, follow toggle in internal/tui/views/logviewer.go

#### P3-Monitor: Monitor Hub (Network, Provider, Oracle/BME)

- [ ] T090 [P3] Consensus monitor view: real-time polling of /consensus_state, height/round/step display, prevote/precommit progress bars, validator vote grid, configurable refresh in internal/monitor/ui/view.go
- [ ] T091 [P3] Validator voting view: scrollable validator table, moniker resolution, voting power formatting, prevote/precommit status, proposer indicator, j/k scroll, signing history bar in internal/monitor/ui/view.go
- [ ] T092 [P3] Provider fleet monitor view: provider list from chain, gRPC+REST health checks, version distribution (semver-sorted, dot visualization), provider table, provider detail sub-view in internal/monitor/ui/view.go
- [ ] T093 [P3] Provider cache: smart-scheduled cache (online:1m, recent-offline:5m, long-offline:6h), priority queue, max 10 concurrent checks, disk persistence, chain re-sync every 10m in internal/monitor/cache/cache.go
- [ ] T094 [P3] Governance params view: split-pane module browser, 12 modules, pretty-printed params using shared Render*Params() functions, 5m refresh in internal/monitor/ui/view.go
- [ ] T095 [P3] Oracle/BME dashboard: aggregated prices (TWAP, median, sources, health), BME vault state, mint status, ledger, two-column layout in internal/monitor/ui/view.go

#### P3-Integration: Live Data

- [ ] T096 [P3] Live sync integration: store change notifications trigger TUI re-renders, view updates within 2s of chain state change, sync status indicator in header in internal/tui/app.go

### E2E Tests for Phase 3

- [ ] T097 [P3] E2E test suite: TUI smoke tests (launch, navigate, resize), component unit tests, view rendering tests in e2e/

**Checkpoint**: Phase 3 complete — full interactive TUI with live data, `akt monitor` hub operational

---

## Phase 6: Phase 4 — Extended Features

**Goal**: Complete feature set with plugin system, additional TUI views, in-TUI transaction actions, and performance optimization.

**Independent Test**: Full coverage of all CLI commands + TUI interactions, CI target >80% coverage.

### Tests for Phase 4

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T098 [P] [P4] Unit tests for plugin discovery: scan paths, precedence rules, disabled list in internal/plugin/discovery_test.go
- [ ] T099 [P] [P4] Unit tests for plugin execution: subprocess env vars, stdin/stdout/stderr inheritance, exit code in internal/plugin/exec_test.go
- [ ] T100 [P] [P4] Unit tests for plugin manifest parsing: YAML schema, version requirements in internal/plugin/manifest_test.go

### Design for Phase 4

- [ ] T101 [P] [P4] Plugin UX design: install flow (trust warning, progress), plugin help integration, list display format, error messaging
- [ ] T102 [P] [P4] Additional TUI view designs: certificates, governance proposals (with vote), validators (with delegate), escrow, wasm, oracle, BME, IBC
- [ ] T103 [P] [P4] TUI transaction action flow design: in-TUI create deployment, fund escrow, vote, delegate flows

### Implementation for Phase 4

#### P4-Plugin: Plugin System

- [ ] T104 [P4] Plugin discovery: scan ~/.config/akt/plugins/ + config plugin paths + $PATH for akt-* executables, precedence rules, respect plugins.disabled list in internal/plugin/discovery.go
- [ ] T105 [P4] Plugin execution: fork/exec subprocess, pass AKT_* env vars (12 variables), inherit stdin/stdout/stderr, exit with plugin's exit code in internal/plugin/exec.go
- [ ] T106 [P4] Plugin management commands: akt plugin install (GitHub URL or --local path), akt plugin list, akt plugin remove in internal/cli/plugin/commands.go
- [ ] T107 [P4] Plugin manifest: parse optional plugin.yaml (name/version/description/usage/requires/min-akt-version), display in list and help in internal/plugin/manifest.go

#### P4-Views: Additional TUI Views

- [ ] T108 [P] [P4] Certificates TUI view: list (Serial/State/Owner), detail (cert content, expiry) in internal/tui/views/certificates.go
- [ ] T109 [P] [P4] Governance TUI view: proposal list (ID/Title/Status/Yes/No/Abstain), detail, vote action (option selection + confirmation) in internal/tui/views/governance.go
- [ ] T110 [P] [P4] Validators TUI view: validator list (Rank/Moniker/Power/Commission/Uptime), detail, delegate/undelegate/redelegate actions in internal/tui/views/validators.go
- [ ] T111 [P] [P4] Escrow TUI view: account list (ID/Owner/State/Balance), detail (payment history) in internal/tui/views/escrow.go
- [ ] T112 [P] [P4] Wasm TUI view: contract list (Address/CodeID/Label), detail (contract info), state queries in internal/tui/views/wasm.go
- [ ] T113 [P] [P4] Oracle TUI view: price table (asset pairs, latest price, source, timestamp) in internal/tui/views/oracle.go
- [ ] T114 [P] [P4] BME TUI view: vault state, mint status, recent ledger entries in internal/tui/views/bme.go
- [ ] T115 [P] [P4] IBC TUI view: channel list (Channel ID/Port/Counterparty/State) in internal/tui/views/ibc.go

#### P4-Actions: In-TUI Transaction Actions

- [ ] T116 [P4] TUI transaction actions: create deployment from TUI (SDL path input), fund escrow, close deployment, vote on proposals in internal/tui/views/ and internal/tui/components/

#### P4-Performance: Optimization

- [ ] T117 [P4] Performance optimization: lazy loading for large lists, virtual scrolling (>10k items), pagination preloading in internal/tui/components/

### E2E Tests for Phase 4

- [ ] T118 [P4] Comprehensive E2E test suite: full coverage of all CLI commands + TUI interactions, CI target >80% coverage in e2e/

**Checkpoint**: Phase 4 complete — full feature set with plugin system and optimized TUI

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple phases

- [ ] T119 [P] Documentation updates (user guide, quickstart) in docs/
- [ ] T120 Code cleanup and refactoring across all packages
- [ ] T121 [P] Security audit: verify no keys in config, MCP write tool gating, destructive action guards
- [ ] T122 [P] AICHANGELOG.md entries for all implemented tasks

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all phase stories
- **Phase 1 Story (Phase 3)**: Depends on Foundational
- **Phase 2 Story (Phase 4)**: Depends on Phase 1 Story completion
- **Phase 3 Story (Phase 5)**: Depends on Phase 1 + Phase 2 Story completion
- **Phase 4 Story (Phase 6)**: Depends on Phase 1 + Phase 2 + Phase 3 Story completion
- **Polish (Phase 7)**: Depends on all desired phase stories being complete

### Within Each Phase Story

- Tests MUST be written and FAIL before implementation (Constitution Principle III)
- Design tasks before implementation tasks
- Models/types before services
- Services before CLI commands
- Core implementation before integration
- E2E tests validate the phase checkpoint

### Parallel Opportunities

**Phase 1 tests** (T004-T013) can all run in parallel
**Phase 1 design** (T014-T016) can all run in parallel
**Phase 1 implementation**: Config (T017-T018) must precede Context (T019-T020), which must precede Client (T023), which must precede Commands (T024-T026). Output/error tasks (T027-T033) can run in parallel with each other.
**Phase 2 tests** (T035-T040) can all run in parallel
**Phase 3 design** (T065-T074) can all run in parallel
**Phase 3 components** (T079-T084) can all run in parallel
**Phase 3 resource views** (T086-T089) can all run in parallel
**Phase 4 views** (T108-T115) can all run in parallel

---

## Implementation Strategy

### MVP First (Phase 1 Only)

1. Complete Phase 1: Setup (T001-T002)
2. Complete Phase 2: Foundational (T003)
3. Write Phase 1 tests (T004-T013) — verify they fail
4. Complete Phase 1 design (T014-T016)
5. Complete Phase 1 implementation (T017-T033)
6. Verify tests pass (T004-T013)
7. Run E2E (T034)
8. **STOP and VALIDATE**: `akt` can replace basic `akash tx/query`

### Incremental Delivery

1. Phase 1 → MVP CLI (tx/query/context) → validate
2. Phase 2 → Store + deploy workflow → validate
3. Phase 3 → Full TUI + monitor → validate
4. Phase 4 → Plugins + extended views → validate
5. Each phase adds value without breaking previous phases

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to implementation phase
- Constitution Principle III (Test-First) is enforced: test tasks precede implementation
- Constitution Principle I (Spec-First) is enforced: design tasks precede implementation
- Linear issue links preserved from original TASKS.md where available
- Commit after each task or logical group
- Stop at any checkpoint to validate phase independently
