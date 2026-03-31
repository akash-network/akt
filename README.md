# akt

Unified CLI and TUI for the [Akash Network](https://akash.network).

## Overview

`akt` replaces the user-facing CLI functionality currently spread across `akash-network/node` (`akash`), `akash-network/provider` (`provider-services`), and `akash-network/chain-sdk/go/cli` with a single binary. It adds a context system for managing multiple networks/accounts, a local deployment store, and an interactive TUI with real-time consensus and provider monitoring (incorporating [`aktop`](https://github.com/cloud-j-luna/aktop) functionality).

See [DESIGN.md](DESIGN.md) for architecture and [SPEC.md](SPEC.md) for the full technical specification.

## Status

Under active development. Phase 1 (Foundation) is in progress. Provider gateway, deployment store, sync engine, and full TUI views are in progress.

## What's Implemented

### Config & Context System

- YAML config at `~/.config/akt/config.yaml` (XDG-compliant, Viper-backed)
- Named contexts composing a network + keyring + per-context store and action log
- Shared network definitions with built-in templates (mainnet, testnet, sandbox)
- Shared keyrings referenced by multiple contexts
- First-run bootstrap that fetches networks from `akash-network/net`
- Config live-reload via fsnotify
- Environment variable overrides (`AKT_CONTEXT`, `AKT_HOME`, `AKT_FROM`, etc.)

### Key Management

Full key lifecycle: add (mnemonic, ledger, multisig, recover), delete, list, show, export, import, rename, mnemonic generation, and address parsing (bech32/hex).

### Transaction & Query Commands

All `tx` and `query` commands from the Akash chain, clean-copied from `chain-sdk/go/cli`:

- **Akash modules**: deployment, market, provider, cert, audit, escrow, oracle, bme
- **Cosmos SDK modules**: bank, staking, distribution, gov, authz, feegrant, slashing, vesting, upgrade, crisis, evidence, mint, params
- **IBC**: ibc-core, ibc-transfer
- **WASM**: store, instantiate, execute, migrate, query
- **Auth utilities**: sign, sign-batch, multisign, validate-signatures, broadcast, encode, decode

### Consensus Monitor (`akt top`)

Standalone TUI for real-time consensus state monitoring with WebSocket vote streaming, provider/moniker caching (bbolt), and gRPC provider queries.

### Action Log

Per-context append-only JSONL log recording all tx, query, workflow, provider, and context operations. Supports filtering by type, time range, deployment sequence, and account. Automatic rotation at 10 MB.

### Output Formatting

All commands support `--output table|json|yaml`. Table is the default for TTY; JSON/YAML for machine consumption.

### Workflow Engine

Core engine with 3-level definition resolution (per-context > global > embedded), Go template evaluation, 8 step types (tx, query, wait, prompt, provider, output, shell, check), and retry/error handling. Built-in workflow definitions (deploy, update, close) are not yet wired.

### TUI Shell

Bubbletea application with dashboard, view routing (query, tx, top), vim-style `:` command input, `ctrl+p` search dialog, and configurable keybindings. Resource views are scaffolded but not yet populated with live data.

## Build

Requires Go 1.26+, make, and the [Akash chain SDK](https://github.com/akash-network/chain-sdk) checked out alongside this repo (the `go.work` file references `../chain-sdk/go`).

```bash
make akt
```

The binary is placed in `.cache/bin/akt`.

Run tests:

```bash
go test ./...
```

### Isolated Test Environment

The `_run/test` directory provides an isolated environment for running `akt` without affecting your real config. It uses [direnv](https://direnv.net) to set `AKT_HOME` to `.cache/run/test/.akt`, keeping all config, keyrings, and state scoped to the test run.

```bash
cd _run/test
direnv allow

# Initialize the test environment directories
make akt-init

# Now run akt commands against the isolated home
akt version
akt context list
akt context network list
```

Everything is stored under `.cache/run/test/.akt` -- delete that directory to start fresh.

## Usage

### First Run

On first launch, `akt` bootstraps interactively -- fetching available networks and prompting you to select which ones to configure:

```bash
akt version
```

If no config exists, the bootstrap wizard runs automatically before any command.

### Context Management

```bash
# List all contexts
akt context list

# Create a context using an existing network
akt context create prod --network mainnet --default-account alice --set-current

# Switch active context
akt context use staging

# Show active context details (resolved network, keyring, store path)
akt context show

# Edit a context
akt context edit prod --default-account bob

# Rename or delete
akt context rename prod production
akt context delete staging --yes
```

### Network Management

```bash
# Create from built-in template
akt context network create mainnet --template mainnet

# Create custom network
akt context network create local --chain-id localnet-1 --rpc http://localhost:26657

# List networks and which contexts use them
akt context network list

# Show full network details
akt context network show mainnet

# Edit network (affects all contexts using it)
akt context network edit mainnet --gas-prices 0.04uakt
```

### Key Management

```bash
# Add a new key
akt context keys add alice

# Recover from mnemonic
akt context keys add alice --recover

# List keys in the current context's keyring
akt context keys list

# Show key details
akt context keys show alice

# Print only the bech32 address (useful for scripting)
akt context keys show alice --address

# Export / import
akt context keys export alice > alice.key
akt context keys import alice-backup alice.key

# Parse an address between formats
akt context keys parse akash1abc...
```

### Transactions

```bash
# Send tokens
akt tx bank send alice akash1dest... 1000000uakt

# Create a deployment
akt tx deployment create deployment.yaml

# Close a deployment
akt tx deployment close --dseq 12345

# Create a lease
akt tx market lease create --dseq 12345 --gseq 1 --oseq 1 --provider akash1prov...

# Delegate to a validator
akt tx staking delegate akashvaloper1... 1000000uakt

# Vote on a governance proposal
akt tx gov vote 42 yes

# Generate and publish a client certificate
akt tx cert generate client
akt tx cert publish client
```

All transaction commands accept `--from`, `--gas`, `--fees`, `--broadcast-mode`, `--yes`, `--dry-run`, and other standard flags. Defaults come from the active context.

### Queries

```bash
# Check balances
akt query bank balances akash1abc...

# List your deployments
akt query deployment deployments --owner akash1abc...

# Get a specific deployment
akt query deployment deployments --dseq 12345

# List active leases
akt query market leases --owner akash1abc... --state active

# List bids for a deployment
akt query market bids --dseq 12345

# Query a provider
akt query provider get akash1prov...

# Query staking validators
akt query staking validators

# Query a WASM contract
akt query wasm contract-state smart akash1contract... '{"get_count":{}}'
```

The `query` command is also available via the `q` alias:

```bash
akt q bank balances akash1abc...
```

### Consensus Monitor

```bash
# Launch the real-time consensus monitor TUI
akt top

# Connect to a specific RPC endpoint
akt top --rpc https://rpc.akashnet.net:443
```

### Output Formats

```bash
# Table (default in terminal)
akt context list

# JSON
akt context list --output json

# YAML
akt query bank balances akash1abc... --output yaml
```

### Action Log

```bash
# View recent actions for the current context
akt context log

# Filter by type and limit
akt context log --type tx --limit 10

# Show actions since a duration or timestamp
akt context log --since 1h
akt context log --since 2026-01-01
```

### TUI Mode

Running `akt` with no subcommand launches the interactive TUI dashboard:

```bash
akt
```

Navigation: `q` query panel, `t` tx panel, `1` consensus monitor, `:` command input, `ctrl+p` search, `?` help, `esc` back, `ctrl+c` quit.

## Roadmap

Upcoming work (see [SPEC.md](SPEC.md) for full details):

- **Phase 2**: Deployment store (bbolt), sync engine, `akt deploy` workflow, provider gateway commands (`akt provider status/lease-status/lease-logs/lease-shell/send-manifest/auth jwt`), store export/import
- **Phase 3**: Full TUI with live resource views (deployments, leases, providers, governance, validators), consensus/provider/governance monitoring views, command palette, themes
- **Phase 4**: Plugin system, additional TUI resource views (wasm, oracle, bme, ibc, escrow), performance optimization

## License

See [LICENSE](LICENSE).
