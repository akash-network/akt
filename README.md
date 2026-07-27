# akt

Unified CLI and TUI for the [Akash Network](https://akash.network).

## Overview

`akt` replaces the user-facing CLI functionality currently spread across `akash-network/node` (`akash`), `akash-network/provider` (`provider-services`), and `akash-network/chain-sdk/go/cli` with a single binary. It adds a context system for managing multiple networks/accounts, a local deployment store, an interactive TUI, and `akt monitor` -- a hub-based real-time monitoring tool for network state, provider fleet health, and BME state (incorporating [`aktop`](https://github.com/cloud-j-luna/aktop) functionality).

The CLI is designed for **flag-minimal operation**: after initial context configuration, the majority of commands require zero additional flags. Query commands use **positional filter arguments** (`akt query deployment 12345`) instead of flag-based filters (`--owner`, `--dseq`), following the Akash resource hierarchy.

See [DESIGN.md](DESIGN.md) for architecture and [SPEC.md](SPEC.md) for the full technical specification.

## Status

Under active development. Phase 1 (Foundation) is complete: context system, key management, all chain tx/query commands, output formatting, action log, and shell completion. Workflow execution (`akt deploy/update/close`) and Akash Console integration (the `akt console` command group, per-context API keys, auth-aware workflow routing) have landed, along with offline e2e suites and CI. The deployment store, sync engine, and provider gateway commands are implemented; TUI resource views continue to mature.

## What's Implemented

### Config & Context System

- YAML config at `~/.config/akt/config.yaml` (XDG-compliant, Viper-backed)
- Named contexts with two authentication methods:
  - **keyring** (default) -- local key management, direct chain transactions
  - **console-api** -- custodial managed wallet via [Akash Console API](https://console.akash.network), no local keys needed
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

### Monitor (`akt monitor`)

Hub-based real-time monitoring tool with three dashboards, navigable via Tab/Shift-Tab. Especially critical during coordinated chain upgrades when online block explorers become unreliable. Connects directly to an RPC endpoint via WebSocket for real-time vote streaming. Requires only an RPC endpoint (no keyring, no default account).

Dashboards:
- **Network** (default) -- consensus state, validator voting, governance parameters. Sub-tabs: `1` Overview (dual progress bar, block history), `2` Validators (signing history), `3` Governance (module parameters)
- **Provider** -- provider fleet health, version distribution, per-provider resource utilization
- **Oracle/BME** -- oracle aggregated prices + BME vault state, mint status, ledger

Subcommands for direct access: `akt monitor network`, `akt monitor provider`, `akt monitor oracle` / `akt monitor bme`.

### Action Log

Per-context append-only JSONL log recording every mutating operation: tx, workflow, provider, context, and console actions. Read-only queries are not recorded by design. `akt context log` reads entries newest-first with `--type`, `--since`, and `--limit` filters. Automatic rotation at 10 MB (up to 5 rotated files kept).

### Output Formatting

All commands support `--output pretty|json|yaml` with pretty formatting by default:

- **Pretty** (default, `--output pretty`): Color-coded states (green=active, red=closed, yellow=warning), bold key identifiers, full addresses, `uakt→AKT` conversion, comma-grouped block heights.
- **JSON** (`--output json`): Machine-readable JSON output.
- **YAML** (`--output yaml`): Machine-readable YAML output.

### Workflow Engine

Core engine with 3-level definition resolution (per-context > global > embedded), Go template evaluation, 8 step types (tx, query, wait, prompt, provider, output, shell, check), and retry/error handling. Workflows support two execution modes: **TUI mode** (interactive progress display) and **JSONL mode** (`--output jsonl`, JSONL output for CI/CD and scripting). The built-in workflows (`akt deploy`, `akt update`, `akt close`) execute end-to-end with auth-aware routing: keyring contexts sign and broadcast locally, while console-api contexts route through the Console API (provider manifest steps are skipped -- Console submits manifests internally). Every step is recorded in the action log.

### Console Integration (`akt console`)

Full command group for the [Akash Console](https://console.akash.network) managed-wallet API: `login`/`logout`/`whoami`, deployment lifecycle (`deployment list/get/create/update/close/deposit/settings`), bid listing, lease creation with a per-context manifest cache, wallet commands (`wallet list/balance/settings/cost`), usage history, public provider/GPU/template catalogs, API key management, and provider-scoped JWT minting. The API key is stored per context (`contexts/<name>/console-api-key`, mode 0600, never in config.yaml) and resolved flag > env > stored credential, so switching context switches Console identity. The first-run bootstrap offers Console onboarding, and state-changing Console calls are recorded in the action log.

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

# Now run akt commands against the isolated home
akt
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

# Create a Console API context (managed wallet, no local keys)
akt context create console --network mainnet --auth-method console-api --set-current

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

# Close a deployment (positional dseq)
akt tx deployment close 12345

# Create a lease (positional dseq and provider; gseq/oseq default to 1)
akt tx market lease create 12345 akash1prov...

# Delegate to a validator
akt tx staking delegate akashvaloper1... 1000000uakt

# Vote on a governance proposal
akt tx gov vote 42 yes

# Generate and publish a client certificate
akt tx cert generate client
akt tx cert publish client
```

All transaction commands accept `--from`, `--gas`, `--fees`, `--broadcast-mode`, `--yes`, `--dry-run`, and other standard flags. Defaults come from the active context.

### Workflows

Workflow commands orchestrate multi-step operations, routing each step through the active context's auth method (local signing for `keyring`, Console API for `console-api`). They support two execution modes:

**TUI mode** (default, interactive):
```bash
# Full deployment lifecycle with interactive bid selection
akt deploy deployment.yaml
```

**JSONL mode** (for CI/CD and scripting):
```bash
# Automated deployment -- each step emits a JSONL line to stdout
akt deploy deployment.yaml --bid-select cheapest --yes -o jsonl
```

Output (one JSON object per line):
```jsonl
{"workflow":"deploy","id":"wf_a1b2c3","step":"create-deployment","result":"completed","errors":[],"txs":[{"hash":"ABCD...","height":12345,"gas_used":150000,"code":0}]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"wait-for-bids","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"select-bid","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"create-lease","result":"completed","errors":[],"txs":[{"hash":"EFGH...","height":12350,"gas_used":120000,"code":0}]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"send-manifest","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"display-result","result":"completed","errors":[],"txs":[]}
```

Parse with `jq`:
```bash
# Extract the deployment transaction hash
akt deploy deployment.yaml --bid-select cheapest --yes -o jsonl \
  | jq -r 'select(.step == "create-deployment") | .txs[0].hash'
```

### Queries

Akash query commands use a **positional filter argument** instead of `--owner`/`--dseq` flags. The filter follows the resource hierarchy (`owner/dseq/gseq/oseq/provider`) with smart type detection: a bech32 address is an owner, a number is a dseq. When no owner is given, the context's default account is used. State keywords are also positional: `akt query deployment active` lists active deployments, and identity + state combine as two positional arguments (`akt query deployment 12345 active`). When the identity pins down a single record, the state argument verifies it — the command fails if the record is in a different state instead of printing it. The `--state` flag, like the other duplicated identity filter flags, is **disabled pending feedback** (positional only, 2026-07). See [SPEC.md §3.8](SPEC.md#38-resource-filter-argument) for full details.

```bash
# Check balances
akt query bank balances akash1abc...

# List your deployments (owner defaults to context account)
akt query deployment

# Get a specific deployment by dseq (owner from context)
akt query deployment 12345

# List deployments for a specific owner
akt query deployment akash1abc...

# Get a specific deployment by owner and dseq
akt query deployment akash1abc.../12345

# List active leases (positional state keyword)
akt query market lease active

# Leases for a specific deployment
akt query market lease 12345

# Specific lease (owner from context)
akt query market lease 12345/1/1/akash1prov...

# Provider-perspective lease query
akt query market lease --by provider akash1prov...

# Query a provider
akt query provider akash1prov...

# Query staking validators
akt query staking validators

# Query a WASM contract
akt query wasm contract-state smart akash1contract... '{"get_count":{}}'
```

The `query` command is also available via the `q` alias:

```bash
akt q deployment 12345
```

### Monitor

```bash
# Launch hub (defaults to Network dashboard)
akt monitor

# Connect to a specific RPC endpoint
akt monitor https://rpc.akashnet.net:443

# Launch directly into a specific dashboard
akt monitor network
akt monitor provider
akt monitor oracle
akt monitor bme

# Or via flag
akt monitor --rpc https://rpc.akashnet.net:443

# Skip TLS verification
akt monitor --insecure

# Clear cache and start fresh
akt monitor --clean-cache
```

Hub navigation: `Tab`/`Shift-Tab` cycle between Network, Provider, Oracle/BME dashboards. Within Network: `1` Overview, `2` Validators, `3` Governance. No keyring or account needed -- just an RPC endpoint.

### Output Formats

```bash
# Pretty table (default) -- color-coded states, bold identifiers
akt query deployment

# Pretty JSON -- syntax-highlighted
akt query deployment --output json

# Machine-readable YAML
akt query bank balances akash1abc... -o yaml

# Machine-readable JSON (for scripts and pipes)
akt query deployment -o json
akt query bank balances akash1abc... -o json
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

### Console API (Managed Wallet)

Contexts with `--auth-method console-api` use the [Akash Console Managed Wallet API](https://akash.network/docs/api-documentation/console-api/api-reference/) for deployment operations. No local keys are needed -- the Console backend handles signing and broadcasting. Deposits are in USD.

```bash
# Set up a console-api context
akt context create console --network mainnet --auth-method console-api --set-current

# Store your API key as the context credential (created at
# console.akash.network > Settings > API Keys). Validates the key and
# writes it to contexts/<name>/console-api-key (0600).
akt console login

# Deploy using the Console managed wallet
akt tx deployment create deploy.yaml --deposit 5

# List your deployments
akt query deployment

# Update a deployment
akt tx deployment update deploy.yaml 12345

# Close a deployment
akt tx deployment close 12345

# Drive the Console API directly
akt console deployment list
akt console wallet balance
akt console usage
```

The API key resolves `--console-api-key` flag > `AKT_CONSOLE_API_KEY` env var > per-context stored credential. Query commands that read directly from the chain (e.g., `akt query bank balances`) still work normally. Commands that require local signing (e.g., `akt tx bank send`, `akt tx gov vote`) are not supported with `console-api` auth -- use a `keyring` context for those.

### TUI Mode

Running `akt` with no subcommand launches the interactive TUI dashboard:

```bash
akt
```

Navigation: `1`-`6` primary views (Deployments, Leases, Providers, Monitor, Governance, Staking), `q` query panel, `t` tx panel, `:` or `ctrl+p` command palette, `?` help, `esc` back, `ctrl+c` quit.

## Roadmap

Upcoming work (see [SPEC.md](SPEC.md) for full details):

- **TUI**: resource views (deployments, leases, providers, governance, validators) populated with live data from the store and sync engine
- **Console parity follow-ups**: logs/shell access for managed deployments via Console-minted JWTs, bid screening command (see [docs/console-parity.md](docs/console-parity.md))
- **Phase 4**: Plugin system, additional TUI resource views (wasm, ibc, escrow), performance optimization

## License

See [LICENSE](LICENSE).
