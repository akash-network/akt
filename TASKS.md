# Akash CLI (`akt`) — Project Tasks

> **Linear project:** [akt CLI](https://linear.app/akash-network/project/akt-cli-08901b17698c) (Akash Network team)

## Phase 1: Foundation

### Design

- [ ] **[P1-Design-1]** CLI output format design — table column layouts per resource type, address truncation rules, color/state indicator scheme, non-TTY fallback behavior [`AKT-202`](https://linear.app/akash-network/issue/AKT-202)
- [ ] **[P1-Design-2]** Error message UX design — error format template (what/context/suggestion), exit code mapping, debug vs user-facing output [`AKT-203`](https://linear.app/akash-network/issue/AKT-203)
- [ ] **[P1-Design-3]** Interactive prompt UX design — confirmation prompts, account selection, context switching prompts, fork-vs-edit-parent flow [`AKT-204`](https://linear.app/akash-network/issue/AKT-204)

### Implementation

- [ ] **[P1-1]** Project scaffold — go.mod, cmd/akt/main.go, Makefile, goreleaser.yaml, CI (GitHub Actions), .golangci.yml, LICENSE [`AKT-205`](https://linear.app/akash-network/issue/AKT-205)
- [ ] **[P1-2]** Config system — YAML config read/write, XDG path resolution ($AKT_HOME > $XDG_CONFIG_HOME/akt > ~/.config/akt), env var loading (AKT_* prefix), schema version [`AKT-206`](https://linear.app/akash-network/issue/AKT-206)
- [ ] **[P1-3]** Config live-reload — fsnotify watcher with polling fallback, change detection, subscriber notification pattern [`AKT-207`](https://linear.app/akash-network/issue/AKT-207)
- [ ] **[P1-4]** Network management — shared network type, CRUD commands (create/delete/edit/list/show), built-in templates (mainnet/testnet/sandbox), cross-context sharing [`AKT-208`](https://linear.app/akash-network/issue/AKT-208)
- [ ] **[P1-5]** Context manager — context type (composes network+keyring+store+log), CRUD (create/delete/edit/list/current/use/rename), fork/edit-parent for networks, context propagation & override chain (flag>env>config>default) [`AKT-209`](https://linear.app/akash-network/issue/AKT-209)
- [ ] **[P1-6]** Keyring integration — shared multi-keyring support, keyring CRUD, backend abstraction (os/file/test/kwallet/pass), keys visible to all referencing contexts [`AKT-210`](https://linear.app/akash-network/issue/AKT-210)
- [ ] **[P1-7]** Action log — append-only JSONL logger per context, ActionEntry types (tx/query/workflow/provider/context/error), reading/filtering, log rotation (10MB, 5 files) [`AKT-211`](https://linear.app/akash-network/issue/AKT-211)
- [ ] **[P1-8]** Chain client — full client (tx+query) and light client (query-only), multi-endpoint failover with health checks, connection timeout & retry, endpoint promotion [`AKT-212`](https://linear.app/akash-network/issue/AKT-212)
- [ ] **[P1-9]** Output formatting — JSON (pretty, snake_case, pagination wrapper), YAML, Table (column-aligned, color states, address truncation, terminal-width-aware), auto-detect TTY [`AKT-213`](https://linear.app/akash-network/issue/AKT-213)
- [ ] **[P1-10]** Global flags & env mapping — --context, --home, --output, --debug; AKT_* env vars; override chain resolution; AddTxFlagsToCmd, AddQueryFlagsToCmd, AddPaginationFlagsToCmd [`AKT-216`](https://linear.app/akash-network/issue/AKT-216)
- [ ] **[P1-11]** Error handling framework — CLIError type (code/message/cause/suggestion/context), structured exit codes (0-7, 127), debug logging [`AKT-214`](https://linear.app/akash-network/issue/AKT-214)
- [ ] **[P1-12]** Transaction commands — all tx modules: bank, deployment, market, provider, cert, audit, staking, distribution, gov, authz, feegrant, escrow, wasm, oracle, bme, slashing, vesting, upgrade, crisis, IBC [`AKT-215`](https://linear.app/akash-network/issue/AKT-215)
- [ ] **[P1-13]** Query commands — all query modules: bank, deployment, market, provider, cert, audit, escrow, staking, distribution, gov, auth, authz, feegrant, evidence, mint, params, slashing, wasm, oracle, bme, ibc, ibc-transfer, upgrade, block, tx [`AKT-217`](https://linear.app/akash-network/issue/AKT-217)
- [ ] **[P1-14]** Key management commands — add, delete, export, import, list, show, rename, mnemonic, parse [`AKT-218`](https://linear.app/akash-network/issue/AKT-218)
- [ ] **[P1-15]** Auth utility commands — sign, sign-batch, multisign, validate-signatures, broadcast, encode, decode [`AKT-219`](https://linear.app/akash-network/issue/AKT-219)
- [ ] **[P1-16]** Shell completion — bash, zsh, fish completion scripts via cobra, dynamic completion for context/network names [`AKT-220`](https://linear.app/akash-network/issue/AKT-220)
- [ ] **[P1-17]** Version command — build-time version/commit/date injection, --long flag for full build info [`AKT-221`](https://linear.app/akash-network/issue/AKT-221)

### Testing

- [ ] **[P1-18]** E2E test suite (Phase 1) — context CRUD, network templates, key management, basic tx/query against local testnet [`AKT-222`](https://linear.app/akash-network/issue/AKT-222)

---

## Phase 2: Store + Workflow

### Design

- [ ] **[P2-Design-1]** Deploy workflow interactive UX design — bid presentation table, bid selection flow (interactive/cheapest/provider filter), step-by-step progress display, non-interactive (piped) output
- [ ] **[P2-Design-2]** Provider command output design — lease-status layout, log stream format, event stream format, shell connection UX

### Implementation

- [ ] **[P2-1]** Store interface + bbolt backend — Store interface (deployment/lease/bid CRUD, sync state, schema, import/export), bbolt bucket structure, concurrent-safe implementation
- [ ] **[P2-2]** Schema migration framework — versioned schema (uint64), migration functions per version, forward-only in single bbolt tx
- [ ] **[P2-3]** Sync engine — WebSocket subscription (Tx+NewBlock events), event router (filter by owner/dseq/type), state reconciler (maps chain events to store CRUD)
- [ ] **[P2-4]** Startup reconciliation — full reconciliation on first launch (query all deployments/leases/bids for tracked accounts), incremental sync on subsequent launches, gap detection (>1000 blocks → full re-sync)
- [ ] **[P2-5]** akt deploy workflow — full lifecycle: create deployment tx → wait for bids → select bid (interactive/cheapest/provider) → create lease tx → send manifest → wait for active → display endpoints. Flags: --bid-timeout, --min-bids, --bid-select, --no-wait-*, --dry-run
- [ ] **[P2-6]** Provider gateway client — REST/gRPC gateway client, JWT and mTLS auth, log/event streaming (WebSocket/SSE)
- [ ] **[P2-7]** Provider CLI commands — status, lease-status, lease-logs (--follow, --tail, --service), lease-events, lease-shell (exec + TTY), send-manifest, get-manifest, migrate-hostnames, migrate-endpoints
- [ ] **[P2-8]** akt sdl-to-manifest — SDL to manifest conversion utility, match existing provider-services behavior
- [ ] **[P2-9]** Store export/import — YAML/JSON export with header metadata, import with merge/replace modes, --dry-run, round-trip fidelity
- [ ] **[P2-10]** Store status command — display store path, DB size, schema version, record counts (active/closed), sync state (last block, last sync, status)
- [ ] **[P2-11]** Events command — live blockchain event streaming (akt events), filter by module/type, TUI auto-launch when no subcommand

### Testing

- [ ] **[P2-12]** E2E test suite (Phase 2) — deploy workflow e2e, sync engine integration, store round-trip, provider commands against mock provider

---

## Phase 3: TUI Mode

### Design

- [ ] **[P3-Design-1]** TUI application shell design — header layout (context/chain-id/account/block/sync), main content area proportions, status bar layout (shortcuts/command input/help), responsive resize behavior
- [ ] **[P3-Design-2]** TUI navigation model design — stack-based navigation spec, breadcrumb rendering, number-key quick access mapping, command palette integration points
- [ ] **[P3-Design-3]** Theme system design — dark/light color palettes, semantic color tokens (primary/secondary/accent/muted/error/warning/success), lipgloss style definitions, custom theme YAML schema
- [ ] **[P3-Design-4]** Keybinding scheme design — vim-style default bindings, non-vim defaults, custom keybinding YAML schema, context-sensitive binding resolution (global vs view-specific)
- [ ] **[P3-Design-5]** Resource view wireframes — deployments (list columns, detail layout, action bar), leases, providers, orders, bids; common patterns for list→detail drill-down
- [ ] **[P3-Design-6]** Consensus & validator view design (from aktop) — consensus state layout (height/round/step/elapsed), vote progress bars (█/░ with power fractions), validator vote grid (●/○), validator voting table (moniker/power/prevote/precommit columns)
- [ ] **[P3-Design-7]** Provider fleet monitor view design (from aktop) — scan progress bar, version distribution dot visualization (●/○), provider table layout, provider detail sub-view (info + node list with GPU), smart scheduling indicator
- [ ] **[P3-Design-8]** Governance params view design — split-pane layout (module list left, params JSON right), module list styling, pretty-printed JSON formatting
- [ ] **[P3-Design-9]** Command palette design — overlay positioning, fuzzy search behavior, result categories (resources/actions/navigation), keyboard interaction model
- [ ] **[P3-Design-10]** Confirmation dialog & log viewer design — dialog overlay size/position, fee preview layout, button styling; log viewer auto-scroll, service filter selector, search highlight

### Implementation

- [ ] **[P3-1]** Bubbletea application shell — root model, header component, status bar component, main area routing, window resize handling
- [ ] **[P3-2]** Navigation system — view stack, breadcrumb trail, back/forward (Esc), number-key quick access, view lifecycle (init/focus/blur)
- [ ] **[P3-3]** Theme system implementation — lipgloss dark/light themes, custom theme loading from config, color token resolution, NO_COLOR support
- [ ] **[P3-4]** Configurable keybindings — keymap loading from config (vim/default/custom), context-sensitive binding resolution, help text generation from keymap
- [ ] **[P3-5]** Resource table component — generic sortable/filterable table (bubbles/table), column configuration per resource type, pagination, terminal-width-aware column sizing
- [ ] **[P3-6]** Detail pane component — scrollable viewport (bubbles/viewport), YAML/JSON toggle, syntax highlighting
- [ ] **[P3-7]** Command palette — overlay with text input (bubbles/textinput), fuzzy matching across resources/actions/navigation, category grouping, keyboard-driven selection
- [ ] **[P3-8]** Confirmation dialog — modal overlay for destructive actions, transaction summary (owner/gas/fees), Cancel/Confirm buttons, Enter/Esc handling
- [ ] **[P3-9]** Help overlay — keybinding reference panel (bubbles/help), view-specific action listing, toggle with ?
- [ ] **[P3-10]** Progress bar & vote grid components — progress bar (for consensus votes, provider scanning), vote grid (● voted / ○ not voted, line-wrapped)
- [ ] **[P3-11]** Dashboard view — home/landing view, summary stats (active deployments, total spend, sync status), quick actions
- [ ] **[P3-12]** Deployments view — list (DSEQ/State/Provider/Price/Balance/Age), detail (full deployment info + active lease + bids), actions (close/update/logs/events/shell)
- [ ] **[P3-13]** Leases view — list (DSEQ/GSeq/OSeq/Provider/State/Price/Age), detail (lease info + endpoints), actions (logs/events/shell/withdraw)
- [ ] **[P3-14]** Providers view — list (Address/URI/Audited/Active Leases), detail (attributes, resources)
- [ ] **[P3-15]** Log viewer — streaming viewport with auto-scroll, --service filter, /search, wrap toggle, follow toggle
- [ ] **[P3-16]** Consensus monitor view (from aktop) — real-time polling of /consensus_state, height/round/step display, prevote/precommit progress bars with voting power %, validator vote grid (●/○), configurable refresh (default 1s, fast 250ms)
- [ ] **[P3-17]** Validator voting view (from aktop) — scrollable validator table, moniker resolution (cached in monikers.json), voting power formatting (K/M/B), prevote/precommit status (✓/✗), proposer indicator (*), j/k scroll, g/G jump
- [ ] **[P3-18]** Provider fleet monitor view (from aktop) — provider list from chain, gRPC+REST health checks, version distribution (semver-sorted, dot visualization), provider table (URL/version/CPU/memory/GPU), provider detail sub-view (node list with GPU model info)
- [ ] **[P3-19]** Provider cache — smart-scheduled cache (online:1m, recent-offline:5m, long-offline:6h), priority queue, max 10 concurrent checks, disk persistence (~/.config/akt/cache/providers.json), chain re-sync every 10m
- [ ] **[P3-20]** Governance params view (from aktop) — split-pane module browser, 12 modules (gov/mint/staking/slashing/distribution/auth/bank/deployment/market/transfer/ibc/crisis), pretty-printed JSON params, 5m refresh
- [ ] **[P3-21]** Live sync integration — store change notifications trigger TUI re-renders, view updates within 2s of chain state change, sync status indicator in header

### Testing

- [ ] **[P3-22]** E2E test suite (Phase 3) — TUI smoke tests (launch, navigate, resize), component unit tests, view rendering tests

---

## Phase 4: Extended Features

### Design

- [ ] **[P4-Design-1]** Plugin UX design — install flow (trust warning, progress), plugin help integration into akt help, plugin list display format, error messaging for missing/incompatible plugins
- [ ] **[P4-Design-2]** Additional TUI view designs — certificates, governance proposals (with vote action), validators (with delegate action), escrow accounts, wasm contracts, oracle prices, BME state, IBC channels
- [ ] **[P4-Design-3]** TUI transaction action flow design — in-TUI create deployment flow (SDL path input → confirmation → progress), fund escrow flow, vote flow, delegate flow

### Implementation

- [ ] **[P4-1]** Plugin discovery — scan ~/.config/akt/plugins/ + config plugin paths + $PATH for akt-* executables, precedence rules, respect plugins.disabled list
- [ ] **[P4-2]** Plugin execution — fork/exec subprocess, pass AKT_* env vars (12 variables), inherit stdin/stdout/stderr, exit with plugin's exit code
- [ ] **[P4-3]** Plugin management commands — akt plugin install (GitHub URL or --local path), akt plugin list, akt plugin remove
- [ ] **[P4-4]** Plugin manifest — parse optional plugin.yaml (name/version/description/usage/requires/min-akt-version), display in list and help
- [ ] **[P4-5]** Certificates TUI view — list (Serial/State/Owner), detail (cert content, expiry)
- [ ] **[P4-6]** Governance TUI view — proposal list (ID/Title/Status/Yes/No/Abstain), detail, vote action (option selection + confirmation)
- [ ] **[P4-7]** Validators TUI view — validator list (Rank/Moniker/Power/Commission/Uptime), detail, delegate/undelegate/redelegate actions
- [ ] **[P4-8]** Escrow TUI view — account list (ID/Owner/State/Balance), detail (payment history)
- [ ] **[P4-9]** Wasm TUI view — contract list (Address/CodeID/Label), detail (contract info), state queries
- [ ] **[P4-10]** Oracle TUI view — price table (asset pairs, latest price, source, timestamp)
- [ ] **[P4-11]** BME TUI view — vault state, mint status, recent ledger entries
- [ ] **[P4-12]** IBC TUI view — channel list (Channel ID/Port/Counterparty/State)
- [ ] **[P4-13]** TUI transaction actions — create deployment from TUI (SDL path input), fund escrow, close deployment, vote on proposals from TUI
- [ ] **[P4-14]** Performance optimization — lazy loading for large lists, virtual scrolling (>10k items), pagination preloading

### Testing

- [ ] **[P4-15]** Comprehensive E2E test suite — full coverage of all CLI commands + TUI interactions, CI target >80% coverage

---

## Key Dependencies

```
P1-1 (scaffold) → P1-2 (config) → P1-4 (networks) → P1-5 (contexts) → P1-8 (client) → P1-12/13 (tx/query)
                                  → P1-6 (keyring) ↗
P1-8 (client) → P2-3 (sync) → P2-4 (reconciliation)
P1-12 (tx) → P2-5 (deploy workflow)
P2-1 (store) → P2-3 (sync) → P3-21 (live sync)
P3-Design-* → P3-1 through P3-22 (all TUI implementation depends on design)
P1-* + P2-* → P3-* (TUI builds on CLI foundation)
P3-1 (shell) → P3-2 (nav) → P3-12+ (views)
```
