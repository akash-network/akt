# Akash CLI (`akt`) - Architecture Design

## 1. Overview

`akt` is the unified command-line interface and terminal user interface for the Akash Network. It replaces the CLI functionality currently spread across `akash-network/node`, `akash-network/provider`, and `akash-network/chain-sdk/go/cli` with a single, cohesive tool that supports both traditional CLI commands and an interactive k9s-style TUI.

### 1.1 Goals

- **Unified entry point**: One binary (`akt`) for all Akash user interactions -- deployments, leases, provider queries, certificate management, governance, staking, and more.
- **Switchable contexts**: named contexts allowing users to work across multiple networks, accounts, and environments seamlessly.
- **Dual-mode interface**: Traditional CLI for scripting and automation, plus an interactive TUI for real-time monitoring and management.
- **Real-time network monitoring**: Live consensus state visualization, validator voting status, and provider fleet health monitoring -- incorporating the functionality of [`aktop`](https://github.com/cloud-j-luna/aktop) directly into the TUI.
- **Local state tracking**: A local store that tracks deployment lifecycle, syncs with chain state, and supports backup/restore.
- **Plugin extensibility**: A plugin system for community-contributed commands and integrations.

### 1.2 Non-Goals

- Running an Akash provider node (the `run` command and Kubernetes operators remain in `akash-network/provider`).
- Running a blockchain validator node (the `start` command and server infrastructure remain in `akash-network/node`).
- Genesis preparation, chain initialization, and testnet scaffolding commands.

### 1.3 Relationship to Existing Repositories

| Repository                       | Current Role                                                                                             | After `akt`                                                                                                                                                 |
|----------------------------------|----------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `akash-network/chain-sdk/go/cli` | All CLI command definitions (tx, query, keys, server)                                                    | **Deprecated.** Commands clean-copied into `akt`. Package removed once `akt` reaches parity.                                                                |
| `akash-network/node`             | Blockchain node binary (`akash`). Imports chain-sdk CLI, adds server/genesis/testnet commands.           | **Keeps** only node-operator commands: `start`, `comet`, `export`, `prepare-genesis`, `testnet`, `testnetify`, `auth jwt`. Stops exporting user-facing CLI. |
| `akash-network/provider`         | Provider binary (`provider-services`). Imports chain-sdk CLI, adds provider gateway + operator commands. | **Keeps** only provider-operator commands: `run`, `operator *`, `tools *`, `migrate`. Stops exporting user-facing CLI.                                      |
| `ovrclk/akt`                     | MVP CLI prototype. Config system, account/network/deploy commands.                                       | Design reference. Concepts (profiles, git-like config) evolved into the context system.                                                                     |
| `cloud-j-luna/aktop`             | Community TUI for monitoring Akash consensus state, validator voting, and provider operations.            | Design reference and prior art for TUI. Its consensus/validator/provider monitoring views inform the TUI design. Functionality to be subsumed by `akt` TUI.  |
| **`akash-network/akt`**          | **New.** This repository.                                                                                | The unified user CLI and TUI.                                                                                                                               |

---

## 2. High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                            akt binary                               │
├──────────────┬────────────────────────────┬─────────────────────────┤
│   CLI Mode   │        TUI Mode            │     Plugin Host         │
│   (cobra)    │   (bubbletea + bubbles     │   (exec-based plugins)  │
│              │    + lipgloss)              │                         │
├──────────────┴────────────────────────────┴─────────────────────────┤
│                        Command Layer                                │
│   ┌───────────┐  ┌──────────────┐  ┌──────────────────────────┐    │
│   │  tx/*     │  │  query/*     │  │  workflow/* (deploy,     │    │
│   │  commands │  │  commands    │  │   lease-shell, etc.)     │    │
│   └───────────┘  └──────────────┘  └──────────────────────────┘    │
├─────────────────────────────────────────────────────────────────────┤
│                        Core Services                                │
│   ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────┐ ┌──────────┐│
│   │ Context  │ │ Keyring  │ │ Client   │ │ Provider  │ │Consensus ││
│   │ Manager  │ │ Manager  │ │ (chain)  │ │ Gateway   │ │ Monitor  ││
│   └──────────┘ └──────────┘ └──────────┘ └───────────┘ └──────────┘│
├─────────────────────────────────────────────────────────────────────┤
│                        Storage Layer                                │
│   ┌─────────────┐ ┌──────────────┐ ┌────────────┐ ┌─────────────┐ │
│   │ Config      │ │ Deployment   │ │ Sync       │ │ Provider    │ │
│   │ (YAML)      │ │ Store(bbolt) │ │ Engine     │ │ Cache       │ │
│   └─────────────┘ └──────────────┘ └────────────┘ └─────────────┘ │
├─────────────────────────────────────────────────────────────────────┤
│                        Transport Layer                              │
│   ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐  │
│   │  RPC       │  │  gRPC      │  │  REST/API  │  │  Provider   │  │
│   │  Client    │  │  Client    │  │  Client    │  │  gRPC/REST  │  │
│   └────────────┘  └────────────┘  └────────────┘  └────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 3. Core Design Concepts

### 3.1 Context System

The context system is the foundational concept of the new CLI. Every operation executes within a named context that defines the target environment.

```
                    ┌──────────────────────────┐
                    │    Context: "mainnet"     │
                    ├──────────────────────────┤
                    │  chain-id: akashnet-2     │
                    │  endpoints:               │
                    │    rpc: [rpc1, rpc2]      │
                    │    api: [api1, api2]      │
                    │    grpc: [grpc1]          │
                    │  keyring: "default"       │
                    │  default-account: "alice" │
                    │  fee-defaults:            │
                    │    gas: auto              │
                    │    gas-prices: 0.025uakt  │
                    │  store-path: stores/      │
                    │              mainnet/     │
                    └──────────────────────────┘
```

**Resolution order** (highest priority wins):

1. Command-line flags (`--context`, `--chain-id`, `--node`, `--from`, etc.)
2. Environment variables (`AKT_CONTEXT`, `AKT_CHAIN_ID`, `AKT_NODE`, `AKT_FROM`, etc.)
3. Active context in config file
4. Built-in defaults

Contexts are independent and self-contained. A user can have `mainnet`, `testnet`, and `sandbox` contexts simultaneously, each with its own accounts, endpoints, and local deployment store.

### 3.2 Dual-Mode Architecture

```
User invocation
       │
       ├── akt [no args or 'ui']  ───>  TUI Mode (bubbletea)
       │                                     │
       │                                     ├── Resource Browser
       │                                     ├── Detail Views
       │                                     ├── Command Palette
       │                                     └── Live Sync
       │
       └── akt <command> [args]   ───>  CLI Mode (cobra)
                                             │
                                             ├── tx / query (raw chain)
                                             ├── workflow commands
                                             ├── context management
                                             └── piped output (JSON/YAML/table)
```

Both modes share the same core services (context, keyring, client, store). The TUI wraps these services in bubbletea models; the CLI wraps them in cobra command handlers.

**Smart interactivity**: Commands auto-detect whether a TTY is attached. When interactive, they show prompts, spinners, and colored output. When piped, they output machine-readable formats silently. The `--interactive` and `--yes` flags override this behavior.

### 3.3 Storage Architecture

```
~/.config/akt/                          # XDG_CONFIG_HOME/akt
├── config.yaml                         # Global config (contexts, keyrings, settings)
├── keyrings/                           # Keyring backends
│   ├── default/                        # Default keyring (OS keyring backend)
│   └── hardware/                       # Example: Ledger-backed keyring
├── stores/                             # Per-context deployment stores
│   ├── mainnet/
│   │   ├── deployments.db              # bbolt database
│   │   └── certs/                      # Local certificate cache
│   ├── testnet/
│   │   ├── deployments.db
│   │   └── certs/
│   └── sandbox/
│       ├── deployments.db
│       └── certs/
└── plugins/                            # Locally installed plugins
    └── akt-sdl-lint                    # Example plugin binary
```

The config root defaults to `$XDG_CONFIG_HOME/akt` (typically `~/.config/akt`). This can be overridden with `$AKT_HOME` or `--home`.

### 3.4 Sync Engine

The sync engine runs as a background goroutine during active CLI/TUI sessions. It keeps the local deployment store in sync with on-chain state.

```
┌────────────────┐     subscribe        ┌──────────────────┐
│  RPC WebSocket │ ──────────────────>  │   Event Router    │
│  (NewBlock,    │                      │                   │
│   Tx events)   │                      │   Filters by:     │
└────────────────┘                      │   - owner addr    │
                                        │   - dseq          │
       ┌────────────────────────────────│   - event type    │
       │                                └──────────────────┘
       v                                         │
┌──────────────┐                                 v
│  Deployment  │<──── update ──────────  ┌───────────────────┐
│  Store       │                         │  State Reconciler  │
│  (bbolt)     │                         │  - new deployments │
│              │                         │  - bid received    │
│  Records:    │                         │  - lease created   │
│   - dseq     │                         │  - lease active    │
│   - state    │                         │  - deployment      │
│   - bids     │                         │    closed          │
│   - leases   │                         └───────────────────┘
│   - provider │
│   - cost     │
│   - sdl hash │
│   - metadata │
└──────────────┘
```

**Startup behavior**: On first launch for a context, the sync engine performs a full reconciliation by querying all deployments owned by accounts in the context. Subsequent launches use incremental sync from the last known block height.

**Reconnection**: On WebSocket disconnect, the engine uses exponential backoff (1s, 2s, 4s, ... up to 60s) with jitter. Missed blocks are reconciled by querying the range between last-synced height and current height.

---

## 4. Package Structure

```
github.com/akash-network/akt/
├── cmd/
│   └── akt/
│       └── main.go                      # Binary entry point
├── internal/
│   ├── cli/                             # CLI mode (cobra commands)
│   │   ├── root.go                      # Root command, global flags, PersistentPreRunE
│   │   ├── tx/                          # Transaction commands (per-module files)
│   │   │   ├── bank.go
│   │   │   ├── deployment.go
│   │   │   ├── market.go
│   │   │   ├── provider.go
│   │   │   ├── cert.go
│   │   │   ├── audit.go
│   │   │   ├── staking.go
│   │   │   ├── distribution.go
│   │   │   ├── gov.go
│   │   │   ├── authz.go
│   │   │   ├── feegrant.go
│   │   │   ├── escrow.go
│   │   │   ├── wasm.go
│   │   │   ├── oracle.go
│   │   │   ├── bme.go
│   │   │   ├── slashing.go
│   │   │   ├── vesting.go
│   │   │   ├── upgrade.go
│   │   │   ├── crisis.go
│   │   │   └── ibc.go
│   │   ├── query/                       # Query commands (per-module files)
│   │   │   └── ... (mirrors tx/ modules + mint, params, evidence, module)
│   │   ├── workflow/                    # High-level workflow commands
│   │   │   ├── deploy.go               # Full deployment lifecycle
│   │   │   ├── lease.go                # Lease management shortcuts
│   │   │   └── manifest.go             # Manifest operations
│   │   ├── context/                     # Context management commands
│   │   │   ├── use.go
│   │   │   ├── list.go
│   │   │   ├── create.go
│   │   │   ├── delete.go
│   │   │   ├── edit.go
│   │   │   ├── current.go
│   │   │   └── rename.go
│   │   ├── keys/                        # Key management commands
│   │   │   ├── add.go
│   │   │   ├── delete.go
│   │   │   ├── list.go
│   │   │   ├── show.go
│   │   │   ├── export.go
│   │   │   ├── import.go
│   │   │   ├── rename.go
│   │   │   ├── mnemonic.go
│   │   │   └── parse.go
│   │   ├── provider/                    # Provider gateway commands
│   │   │   ├── status.go
│   │   │   ├── lease_status.go
│   │   │   ├── lease_logs.go
│   │   │   ├── lease_events.go
│   │   │   ├── lease_shell.go
│   │   │   ├── manifest.go
│   │   │   └── migrate.go
│   │   ├── store/                       # Store management commands
│   │   │   ├── export.go
│   │   │   ├── import_.go
│   │   │   └── status.go
│   │   └── plugin/                      # Plugin management
│   │       ├── install.go
│   │       ├── list.go
│   │       └── remove.go
│   ├── tui/                             # TUI mode (bubbletea)
│   │   ├── app.go                       # Root bubbletea model
│   │   ├── navigation.go               # View routing and navigation stack
│   │   ├── keymap.go                    # Configurable keybindings
│   │   ├── theme.go                     # lipgloss styles and theming
│   │   ├── views/                       # TUI view models (one per resource type)
│   │   │   ├── dashboard.go
│   │   │   ├── deployments.go
│   │   │   ├── leases.go
│   │   │   ├── bids.go
│   │   │   ├── orders.go
│   │   │   ├── providers.go
│   │   │   ├── certificates.go
│   │   │   ├── escrow.go
│   │   │   ├── governance.go
│   │   │   ├── validators.go
│   │   │   ├── consensus.go            # Real-time consensus state (from aktop)
│   │   │   ├── provider_monitor.go     # Provider fleet health monitor (from aktop)
│   │   │   ├── wasm.go
│   │   │   ├── oracle.go
│   │   │   ├── bme.go
│   │   │   ├── ibc.go
│   │   │   └── logs.go
│   │   ├── components/                  # Reusable TUI components
│   │   │   ├── resource_table.go
│   │   │   ├── detail_pane.go
│   │   │   ├── status_bar.go
│   │   │   ├── header.go
│   │   │   ├── command_palette.go
│   │   │   ├── confirm_dialog.go
│   │   │   ├── log_stream.go
│   │   │   ├── progress_bar.go         # Progress bar component (votes, scanning)
│   │   │   ├── vote_grid.go            # Validator vote visualization grid
│   │   │   └── help_overlay.go
│   │   └── messages/                    # Custom bubbletea messages
│   │       ├── sync.go
│   │       ├── chain.go
│   │       ├── consensus.go            # Consensus state tick/update messages
│   │       ├── providermon.go          # Provider scan progress messages
│   │       └── navigation.go
│   ├── context/                         # Context management core
│   │   ├── manager.go                   # CRUD, switching, resolution
│   │   ├── config.go                    # Config file I/O
│   │   └── defaults.go                  # Built-in network presets
│   ├── keyring/                         # Keyring abstraction
│   │   ├── manager.go                   # Multi-keyring management
│   │   └── resolver.go                  # Context-aware keyring resolution
│   ├── client/                          # Chain client
│   │   ├── client.go                    # Full client (tx + query)
│   │   ├── light.go                     # Light client (query only)
│   │   ├── failover.go                  # Multi-endpoint failover
│   │   └── modules/                     # Module-specific helpers
│   │       ├── bank.go
│   │       ├── deployment.go
│   │       ├── market.go
│   │       └── ...
│   ├── provider/                        # Provider gateway client
│   │   ├── gateway.go                   # REST/gRPC gateway client
│   │   ├── auth.go                      # JWT and mTLS auth
│   │   └── stream.go                    # Log/event streaming
│   ├── store/                           # Local deployment store
│   │   ├── interface.go                 # Store interface definition
│   │   ├── bbolt/                       # bbolt backend implementation
│   │   │   └── store.go
│   │   ├── schema.go                    # Schema versioning and migrations
│   │   ├── records.go                   # Record types
│   │   ├── export.go                    # YAML/JSON export
│   │   └── import.go                    # YAML/JSON import
│   ├── sync/                            # Chain sync engine
│   │   ├── engine.go                    # WebSocket subscription + event routing
│   │   ├── reconciler.go               # State reconciliation
│   │   └── filters.go                   # Event filtering
│   ├── consensus/                       # Consensus state monitoring (from aktop)
│   │   ├── monitor.go                   # Real-time consensus state polling
│   │   ├── types.go                     # RoundState, HeightVote, ValidatorStatus
│   │   └── parser.go                    # BitArray parsing, vote counting
│   ├── providermon/                     # Provider fleet monitoring (from aktop)
│   │   ├── scanner.go                   # Provider health checking (gRPC + REST)
│   │   ├── cache.go                     # Smart-scheduled provider cache
│   │   └── types.go                     # ProviderStatus, NodeInfo, GPUInfo
│   ├── plugin/                          # Plugin system
│   │   ├── discovery.go                 # Plugin path scanning
│   │   ├── executor.go                  # Exec-based plugin runner
│   │   └── protocol.go                  # Plugin communication protocol
│   ├── flags/                           # Shared flag definitions
│   │   ├── global.go                    # --context, --output, --debug, --home
│   │   ├── tx.go                        # Transaction flags
│   │   ├── query.go                     # Query flags
│   │   ├── pagination.go               # Pagination flags
│   │   ├── deployment.go               # Deployment/Group/Order/Bid/Lease ID flags
│   │   └── provider.go                  # Provider-specific flags
│   └── output/                          # Output formatting
│       ├── formatter.go                 # JSON/YAML/Table router
│       ├── table.go                     # Table renderer
│       └── printer.go                   # Output helpers
├── pkg/                                 # Public API (for plugins)
│   └── types/                           # Shared types for plugin protocol
│       └── env.go                       # AKT_* environment variable names
├── e2e/                                 # End-to-end tests
│   ├── suite.go
│   ├── context_test.go
│   ├── deploy_test.go
│   └── ...
├── Makefile
├── goreleaser.yaml
├── go.mod
├── go.sum
├── DESIGN.md                            # This file
├── SPEC.md                              # Technical specification
└── LICENSE
```

---

## 5. Key Design Decisions

### 5.1 Clean Copy from chain-sdk

Commands are copied from `akash-network/chain-sdk/go/cli` with the following cleanups:

- **Remove dead code**: Unused utilities, stub files (`escrow.go` is empty), and unfinished features.
- **Standardize patterns**: Every command follows the same RunE handler pattern -- resolve context, get client, build message, execute, format output.
- **Adapt to context system**: Replace direct flag reading (e.g., `--node`, `--chain-id`) with context-aware resolution that falls back through the override chain.
- **Preserve behavior**: All tx/query operations produce the same on-chain results. Flag names remain the same where feasible.

The `chain-sdk` CLI package (`pkg.akt.dev/go/cli`) will be deprecated and eventually removed once `akt` reaches full feature parity.

### 5.2 Cobra for CLI, Bubbletea for TUI

- **Cobra** handles command parsing, flag management, help generation, and shell completion for CLI mode. It is the standard in the Go and Cosmos SDK ecosystem.
- **Bubbletea v2** (Elm Architecture: Model-Update-View) handles the interactive TUI. Its functional design isolates state management and rendering.
- **Lipgloss v2** provides CSS-like styling for terminal output in both modes -- table formatting in CLI, full layout composition in TUI.
- **Bubbles v2** provides battle-tested components: table, viewport, text input, spinner, help, key bindings, list, progress bar, paginator.

### 5.3 Store Interface for Multiple Backends

The deployment store is defined as a Go interface, with bbolt as the default backend. This allows:

- **Testing**: In-memory backends for unit tests.
- **Future backends**: SQLite, remote/networked stores, or other embedded databases.
- **Import/Export**: Backends implement serialization to YAML/JSON for backup, restore, and machine portability.

The store is sync-ready: every record has a `version` field (monotonically increasing) and `updated_at` timestamp. The sync engine updates records through the same interface, enabling future remote sync without changing the data model.

### 5.4 Plugin System (exec-based)

Following kubectl's proven plugin model:

- Plugins are executables named `akt-<name>` found in `$PATH` or `~/.config/akt/plugins/`.
- `akt <name> [args]` delegates to the plugin binary if no built-in command matches.
- Plugins receive context information via environment variables (`AKT_CONTEXT`, `AKT_CHAIN_ID`, `AKT_NODE`, `AKT_FROM`, `AKT_HOME`, `AKT_OUTPUT`, etc.).
- An optional plugin manifest (`plugin.yaml` next to the binary) declares metadata, required context fields, and help text.
- Built-in management: `akt plugin install <url>`, `akt plugin list`, `akt plugin remove <name>`.

### 5.5 Multi-Endpoint Failover

Each context can define multiple endpoints per transport type (RPC, API, gRPC). The client layer implements automatic failover:

1. Try the first endpoint in the list.
2. On connection failure or timeout (configurable, default 5s), try the next.
3. On successful connection, promote that endpoint to the top of the list for subsequent requests within the session.
4. Health checks run periodically (every 30s) to detect degraded endpoints proactively.

This replaces the MVP's manual backup-endpoint approach with transparent, automatic resilience.

---

## 6. Implementation Phases

### Phase 1: Foundation (Context + Core CLI)

**Goal**: A functional CLI that can replace basic `akash tx` and `akash query` operations.

- Context system: config file I/O, context CRUD, switching, resolution chain
- Keyring management with per-context overrides
- Chain client with multi-endpoint failover
- Core tx commands: bank, deployment, market, provider, cert, audit, staking, distribution, gov, authz, feegrant, escrow, wasm, oracle, bme, slashing, vesting, upgrade, crisis, IBC
- Core query commands: all matching modules
- Key management commands
- Output formatting (JSON/YAML/Table)
- Global flags and environment variable support
- Built-in network presets (mainnet, testnet, sandbox)
- Version command with build-time injection
- Shell completion (bash, zsh, fish)
- Basic e2e test suite

### Phase 2: Store + Workflow Commands

**Goal**: Local state tracking and high-level workflow commands that orchestrate multi-step operations.

- bbolt-based deployment store with full interface implementation
- Store schema versioning and migration framework
- Sync engine: WebSocket subscription, event routing, state reconciliation
- `akt deploy` workflow command: create deployment, wait for bids, select bid (interactive or auto), create lease, send manifest, wait for active, display endpoint URLs
- Provider gateway client: status, lease-status, lease-logs, lease-events, lease-shell, send-manifest, get-manifest
- Provider migration commands: migrate-hostnames, migrate-endpoints
- `akt sdl-to-manifest` utility
- Store export/import commands
- Store status command (sync state, record counts)

### Phase 3: TUI Mode

**Goal**: A fully interactive terminal UI for real-time Akash management, incorporating the monitoring functionality of [`aktop`](https://github.com/cloud-j-luna/aktop).

- Bubbletea application shell: header, main area, status bar
- Navigation system: resource type selector, breadcrumb trail, back/forward
- Resource views: deployments, leases, bids, orders, providers, certificates
- Detail panes: YAML/JSON toggle, scrollable viewport
- **Consensus monitor view** (from aktop): Real-time consensus state (height, round, step), prevote/precommit progress bars with voting power percentages, validator vote grid visualization (`●`/`○`)
- **Validator voting view** (from aktop): Scrollable validator list with moniker resolution, voting power, prevote/precommit status (`✓`/`✗`), proposer indicator
- **Provider fleet monitor view** (from aktop): Provider version distribution with dot visualization, provider health scanning with priority-based scheduling, per-provider detail with node-level CPU/memory/GPU resources, smart caching with configurable check intervals
- **Governance parameters view** (from aktop): Module-by-module parameter browsing with pretty-printed JSON display
- Real-time log streaming view
- Command palette with fuzzy search
- Transaction confirmation dialogs
- Live sync integration (store changes reflected in TUI immediately)
- Provider cache with smart scheduling (online: 1m, recently offline: 5m, long-term offline: 6h)
- Configurable keybindings (vim-style defaults)
- Theme system (light/dark, customizable colors)

### Phase 4: Extended Features

**Goal**: Complete feature set, polish, and extensibility.

- Plugin system: discovery, execution, manifest parsing, management commands
- Full resource set in TUI: governance proposals (with voting), validators (with delegation), escrow accounts, wasm contracts, oracle prices, BME state, IBC channels
- Additional TUI actions: create deployment from TUI, close deployment, fund escrow, vote on proposals
- Performance optimization: lazy loading, virtual scrolling for large lists
- Comprehensive e2e test suite covering all commands and TUI interactions
- Documentation and user guide

---

## 7. Migration Strategy

### 7.1 Command Mapping from `akash`

| Current (`akash`)             | New (`akt`)                 | Notes                              |
|-------------------------------|-----------------------------|------------------------------------|
| `akash tx bank send`          | `akt tx bank send`          | Identical behavior                 |
| `akash tx deployment create`  | `akt tx deployment create`  | Identical behavior                 |
| `akash tx deployment close`   | `akt tx deployment close`   | Identical behavior                 |
| `akash tx deployment update`  | `akt tx deployment update`  | Identical behavior                 |
| `akash tx deployment group *` | `akt tx deployment group *` | Identical behavior                 |
| `akash tx market bid *`       | `akt tx market bid *`       | Identical behavior                 |
| `akash tx market lease *`     | `akt tx market lease *`     | Identical behavior                 |
| `akash tx provider *`         | `akt tx provider *`         | Identical behavior                 |
| `akash tx cert *`             | `akt tx cert *`             | Identical behavior                 |
| `akash tx audit *`            | `akt tx audit *`            | Identical behavior                 |
| `akash tx staking *`          | `akt tx staking *`          | Identical behavior                 |
| `akash tx distribution *`     | `akt tx distribution *`     | Identical behavior                 |
| `akash tx gov *`              | `akt tx gov *`              | Identical behavior                 |
| `akash tx wasm *`             | `akt tx wasm *`             | Identical behavior                 |
| `akash tx oracle *`           | `akt tx oracle *`           | Identical behavior                 |
| `akash tx bme *`              | `akt tx bme *`              | Identical behavior                 |
| `akash query *`               | `akt query *`               | Identical behavior for all modules |
| `akash keys *`                | `akt keys *`                | Identical behavior                 |
| (none)                        | `akt context *`             | New context management             |
| (none)                        | `akt deploy`                | New workflow command               |
| (none)                        | `akt ui`                    | New TUI mode                       |
| (none)                        | `akt store *`               | New store management               |
| (none)                        | `akt plugin *`              | New plugin management              |

### 7.2 Command Mapping from `provider-services`

| Current (`provider-services`)         | New (`akt`)                      | Notes              |
|---------------------------------------|----------------------------------|--------------------|
| `provider-services status`            | `akt provider status`            | Identical behavior |
| `provider-services lease-status`      | `akt provider lease-status`      | Identical behavior |
| `provider-services lease-logs`        | `akt provider lease-logs`        | Identical behavior |
| `provider-services lease-events`      | `akt provider lease-events`      | Identical behavior |
| `provider-services lease-shell`       | `akt provider lease-shell`       | Identical behavior |
| `provider-services send-manifest`     | `akt provider send-manifest`     | Identical behavior |
| `provider-services get-manifest`      | `akt provider get-manifest`      | Identical behavior |
| `provider-services migrate-hostnames` | `akt provider migrate-hostnames` | Identical behavior |
| `provider-services migrate-endpoints` | `akt provider migrate-endpoints` | Identical behavior |
| `provider-services sdl-to-manifest`   | `akt sdl-to-manifest`            | Top-level utility  |

### 7.3 Commands NOT Migrated

These remain in their respective repositories:

**Stays in `akash-network/node`:**
- `akash start` -- blockchain node server
- `akash comet *` -- CometBFT management (show-node-id, show-validator, etc.)
- `akash export` -- state export
- `akash prepare-genesis` -- genesis preparation with Akash-specific parameters
- `akash testnet` -- testnet scaffolding
- `akash testnetify` -- mainnet-to-testnet conversion
- `akash auth jwt` -- JWT token generation for provider auth
- `akash init` -- genesis initialization
- `akash gentx`, `akash collect-gentxs`, `akash add-genesis-account`, `akash validate-genesis`
- All server-related commands (pruning, snapshot, status, rollback, module-hash)

**Stays in `akash-network/provider`:**
- `provider-services run` -- launches the full provider service
- `provider-services operator hostname` -- hostname/ingress operator
- `provider-services operator ip` -- IP/MetalLB operator
- `provider-services operator inventory` -- inventory operator
- `provider-services tools psutil *` -- hardware discovery tools
- `provider-services migrate run` -- database migration runner
- `provider-services show-cluster-ns` -- Kubernetes namespace utility
