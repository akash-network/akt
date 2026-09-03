# akt

[![active coverage](https://codecov.io/gh/akash-network/akt/branch/main/graph/badge.svg?flag=active-union)](https://codecov.io/gh/akash-network/akt)

Unified CLI for the [Akash Network](https://akash.network).

## Overview

`akt` replaces the user-facing CLI functionality currently spread across `akash-network/node` (`akash`), `akash-network/provider` (`provider-services`), and `akash-network/chain-sdk/go/cli` with a single binary. It adds a context system for managing multiple networks/accounts, a local deployment store, offline SDL authoring, Akash Console integration, and `akt monitor` -- a hub-based real-time monitoring tool for network state, provider fleet health, and BME state (incorporating [`aktop`](https://github.com/cloud-j-luna/aktop) functionality).

The CLI is designed for **flag-minimal operation with no surprises**: after initial context configuration, the majority of commands require zero additional flags, and commands the active context is not configured to run say so up front instead of failing somewhere inside a transport (see [Capability Gating](#capability-gating)).

Commands take their primary values **positionally**, following the Akash resource hierarchy, rather than through filter flags:

```bash
# State keyword, identity + state, dseq, and a date range -- all positional
akt query deployment active
akt query deployment akash1abc.../12345 closed
akt tx deployment close 12345
akt console usage 2026-01-01 2026-01-31
```

The duplicated identity and filter flags (`--owner`, `--dseq`, `--gseq`, `--oseq`, `--state`, `--provider`, and the Console positional twins such as `--from`/`--to` on `console usage`) are **disabled during a UX trial** (2026-07): they are commented out in code behind `FEEDBACK(2026-07)` markers and can be restored wholesale if feedback calls for it. While the trial runs, the positional form is the only form. Transaction flags (`--from`, `--gas`, `--fees`, ...) are unaffected.

See [DESIGN.md](DESIGN.md) for architecture and [SPEC.md](SPEC.md) for the full technical specification.

## Status

Under active development. Phase 1 (Foundation) is complete: context system, key management, all chain tx/query commands, output formatting, action log, and shell completion. Workflow execution (`akt deploy/update/close`) and Akash Console integration have landed -- including live lease operations (logs, events, status, shell) against provider gateways -- along with capability gating, the offline `akt sdl` authoring group, offline e2e suites, and CI. The deployment store, sync engine, and provider gateway commands are implemented, as is the `akt monitor` dashboard.

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

### Capability Gating

The active context's configuration determines a **feature set** -- what `akt` can actually do:

- A network with at least one RPC endpoint enables `chain-query`, `chain-tx`, and `provider`, which gate the `query`, `tx`, `monitor`, and `provider` command groups.
- A resolvable Console API key enables `console`, which gates the Console-backed commands.

Commands declare their requirement as a cobra annotation (`internal/capability`), and alternatives are allowed -- the workflow commands need `chain-tx|console` and run on either rail. A command the configuration cannot run is dimmed in help (its summary prefixed with `[unavailable]`) and fails fast naming the missing capability and its remedy:

```
akt console whoami is unavailable with the current context configuration: requires
console (configure a Console API key (akt console login, or akt context edit
<context> --console-api-key <key>))
```

Presentation is configurable while UX feedback is collected, via `defaults.command-gating` in `config.yaml`:

- **`dim`** (default) -- unavailable commands stay listed, marked `[unavailable]`, and fail fast with an explanation.
- **`hide`** -- unavailable commands are removed from help listings; direct invocation still fails fast.
- **`off`** -- no gating; commands fail wherever the missing transport is first touched.

Per-invocation overrides that carry their own connection details (`--node`, `--console-api-key`, a positional `akt monitor` endpoint) grant the capability they supply, so gating never rejects a command that can connect on its own. Help is never gated. `akt context show` reports the resolved feature set. See [SPEC.md §2.10](SPEC.md#210-capability-gating).

### Key Management

Full key lifecycle: add (mnemonic, ledger, multisig, recover), delete, list, show, export, import, rename, mnemonic generation, and address parsing (bech32/hex).

### Transaction & Query Commands

All `tx` and `query` commands from the Akash chain, clean-copied from `chain-sdk/go/cli`:

- **Akash modules**: deployment, market, provider, cert, audit, escrow, oracle, bme
- **Cosmos SDK modules**: bank, staking, distribution, gov, authz, feegrant, slashing, vesting, upgrade, evidence, mint, params
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

Per-context append-only JSONL log recording every mutating operation: tx, workflow, provider, context, key management, and console actions. Read-only queries are not recorded by design, the one exception being `context keys export`, which is recorded as a security event because it moves private key material out of the keyring. Secrets themselves -- mnemonics, passphrases, key material, API keys -- are never written to the log. `akt context log` reads entries newest-first with `--type`, `--workflow-id`, `--since`, and `--limit` filters, summarizing each entry by what identifies it (the step and run of a workflow, the deployment of a transaction, the parameters of a context change). Automatic rotation at 10 MB (up to 5 rotated files kept).

### Output Formatting

All commands support `--output pretty|json|yaml` with pretty formatting by default:

- **Pretty** (default, `--output pretty`): Color-coded states (green=active, red=closed, yellow=warning), bold key identifiers, full addresses, `uakt→AKT` conversion, comma-grouped block heights.
- **JSON** (`--output json`): Machine-readable JSON output.
- **YAML** (`--output yaml`): Machine-readable YAML output.

### Workflow Engine

Core engine with 3-level definition resolution (per-context > global > embedded), Go template evaluation, 8 step types (tx, query, wait, prompt, provider, output, shell, check), and retry/error handling. Workflows support two execution modes: **interactive mode** (a progress display) and **JSONL mode** (`--output jsonl`, JSONL output for CI/CD and scripting). The built-in workflows (`akt deploy`, `akt update`, `akt close`) execute end-to-end with auth-aware routing: keyring contexts sign and broadcast locally, while console-api contexts route through the Console API (provider manifest steps are skipped -- Console submits manifests internally). Every step is recorded in the action log.

### SDL Authoring (`akt sdl`)

Transport-independent SDL scaffolding, generation, and linting. Every `akt sdl` subcommand runs entirely locally: no context, no key, no RPC endpoint, and the group declares no capability requirements.

- **`akt sdl scaffolds`** -- list the built-in scaffolds (`web`, `gpu`, `multi-service`, `ip-lease`) and the flags each honors.
- **`akt sdl init <scaffold>`** -- print a deployable SDL to stdout, self-checked against the validator before it is printed. Flags (`--image`, `--cpu`, `--memory`, `--gpu-model`, `--price`, ...) are generation parameters with per-scaffold defaults, so the zero-flag invocation always produces valid output. Pipe it into `akt sdl validate -`, or redirect it to a file for `akt deploy` / `akt console deployment create`.
- **`akt sdl validate <file>`** -- validate offline (`-` reads stdin). Parsing uses `pkg.akt.dev/go/sdl`, the same parser behind `akt deploy` and the chain tx commands, followed by lint rules.

Lint rules: an unpinned image is an **error** -- every service image must carry an explicit tag or `@sha256:` digest, so untagged images and `:latest` are rejected as non-reproducible. For pricing, `uact` passes on both rails. `uakt` is a **warning** because it requires an explicitly matching `--deposit <amount>uakt` on chain; any other denom is an error. Exit 0 when valid, 1 when not. See [SPEC.md §2.11](SPEC.md#211-sdl-commands).

### Console Integration (`akt console`)

Full command group for the [Akash Console](https://console.akash.network) managed-wallet API:

- **Authentication** -- `login` (validates the key, then stores it for the active context), `logout`, `whoami`.
- **Deployment lifecycle** -- `deployment list/get/create/update/close/settings`. There is no deposit: the platform funds every deployment from your account credits, and `settings` sets an optional runtime limit in hours. See [How Funding Works](https://akash.network/docs/getting-started/how-funding-works/).
- **Bids and leases** -- `bid list <dseq>` and `lease create <dseq> [provider]`, which defaults to the manifest cached per context by `deployment create`.
- **Wallet and usage** -- `wallet list/balance/settings/cost` and `usage [from] [to]` spend history, all rendered in USD.
- **Keyless marketplace** -- `provider list/get/regions/auditors`, `gpu`, `template list/get/sdl`, and `screen <sdl-file>` bid screening. These hit public endpoints and need neither an API key nor a configured context.
- **Credentials** -- `apikey list/create/delete` (the secret is printed exactly once) and `jwt create` for short-lived provider-scoped tokens.
- **Live lease operations** -- `logs <dseq> [service]` and `events <dseq>` (both `--follow`), `status <dseq>` (`--watch`, `--interval`), and `shell <dseq> <service> [-- command]` (exec is the same command with an explicit command). Each resolves the deployment's active lease, looks up the provider's gateway URI, and mints a scoped JWT via the Console, so **managed contexts reach providers with no wallet and no local key**. One-shot calls use a 300 s token; streaming and interactive modes use 3600 s.

The API key is stored per context at `contexts/<name>/console-api-key` (mode 0600, never written to `config.yaml`, never printed) and resolved `--console-api-key` flag > `AKT_CONSOLE_API_KEY` env var > stored credential, so switching context switches Console identity. Write it with `akt console login` or `akt context edit <context> --console-api-key <key>`; `akt context rename` moves it and `akt context delete` removes it. The first-run bootstrap offers Console onboarding, and state-changing Console calls are recorded in the action log. [SPEC.md §7.8](SPEC.md#78-console-compatibility-matrix) tracks coverage of every Console capability.

## Build

Requires Go 1.26+ and make. Go resolves the Akash chain SDK version from
`go.mod`; a sibling SDK checkout is not required. This repository has no
`go.work`, and the parent workspace does not include this module, so commands
must disable workspace discovery.

```bash
GOWORK=off make akt
```

The binary is placed in `.cache/bin/akt`. Version, commit, and build date are injected at link time:

```bash
# Short form: version, commit, build date
akt version

# Full build info: version, commit, build date, Go version, platform, build tags
akt version --long
```

Run unit tests (the parent workspace does not include this module, so direct Go
commands must disable workspace discovery):

```bash
GOWORK=off go list ./... | grep -v /e2e | xargs env GOWORK=off go test
```

The E2E package shells out to `.cache/bin/akt`; build it before the full suite:

```bash
mkdir -p .cache/bin
GOWORK=off go build -o .cache/bin/akt ./cmd/akt
GOWORK=off go test ./...
```

Coverage uses three blocking lanes: cross-package unit tests, offline CLI E2E,
and a fresh single-validator chain E2E. CI instruments the real `akt` binary,
collects subprocess counters through `GOCOVERDIR`, and publishes their statement
union rather than averaging job percentages. Package membership is explicit in
[`coverage/packages.tsv`](coverage/packages.tsv); the badge reports the active
union, while repository, experimental-TUI, and tooling profiles remain
separately visible.

```bash
# Duplicate-free cross-package unit profile and package report
GOWORK=off make test-coverage-unit

# Build the release-equivalent instrumented binary and collect offline E2E
GOWORK=off make test-coverage-binary
GOWORK=off make test-coverage-e2e-prepare COVERAGE_SHARD=e2e-offline
GOCOVERDIR="$PWD/.cache/coverage/covdata/e2e-offline" \
  GOWORK=off go test ./e2e/... -v -count=1
GOWORK=off make test-coverage-shard-ready COVERAGE_SHARD=e2e-offline

# Collect the pinned, harness-owned Docker chain lane
GOWORK=off make test-coverage-e2e-prepare COVERAGE_SHARD=e2e-localnet
AKT_E2E_LOCALNET=1 \
  GOCOVERDIR="$PWD/.cache/coverage/covdata/e2e-localnet" \
  GOWORK=off go test ./e2e/ -run TestLocalnet -v -count=1 -timeout 15m
GOWORK=off make test-coverage-shard-ready COVERAGE_SHARD=e2e-localnet

# Merge all blocking lanes and generate current statement reports
GOWORK=off make test-coverage-report

# Enforce 100% coverage for changed executable active lines
GOWORK=off make test-coverage-patch BASE_REF=origin/main
```

The local patch command checks the complete worktree, including untracked Go
files. CI supplies immutable base and head commits for pull requests and pushes.
The paths above assume the bare-checkout `.cache` default; if `AKT_DEVCACHE` is
set, use the absolute shard path printed by `test-coverage-e2e-prepare`.

Reports are written to `.cache/coverage/reports/`. Their aggregate and
per-package TSV values describe the current run; they are diagnostics, not
checked-in numeric floors. In CI, Codecov compares aggregate line coverage for
pull requests targeting `main` across the active-union, experimental-TUI, and
tooling profiles with their exact main-branch bases. The local patch command
separately requires every changed
executable line in active Go code to map to an executed Go statement or exact
synthetic-edge counter. Exact patch exceptions are keyed by line; the same
registry holds explicit test-support package denominator exclusions.
Every exception requires an owner, evidence, and review deadline in
[`coverage/exceptions.tsv`](coverage/exceptions.tsv); there are no implicit path
exclusions.

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

# Show active context details (resolved network, keyring, store path, capabilities)
akt context show

# Edit a context
akt context edit prod --default-account bob

# Create a Console API context (managed wallet, no local keys)
akt context create console --network mainnet --auth-method console-api --set-current

# Rename or delete
akt context rename prod production
akt context delete staging --yes
```

`akt context show` ends with a Capabilities block reporting the resolved feature set, naming the remedy for anything the configuration cannot do:

```
  Capabilities:
      Chain queries:       available
      Chain transactions:  available
      Provider gateway:    available
      Console API:         unavailable — run akt console login
```

Commands outside that set are dimmed in help and refuse to run. Set `defaults.command-gating` to `hide` in `config.yaml` to drop them from help listings entirely, or to `off` to disable gating.

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

# Non-interactive creation without printing a mnemonic backup
akt context keys add ci-deployer --no-backup

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

`keys add` has no confirmation prompt, so it does not accept `--yes`. Blank
names are rejected before the keyring is opened.

### SDL Authoring

`akt sdl` runs entirely locally -- it works before any context exists and never touches the network.

```bash
# List the built-in scaffolds and the flags each honors
akt sdl scaffolds

# Generate a web service SDL
akt sdl init web --image nginx:1.27 > deploy.yaml

# GPU workload with a specific model
akt sdl init gpu --gpu-model h100 --image myorg/model:1.0 > gpu.yaml

# Generate and validate without touching disk
akt sdl init web --image nginx:1.27 | akt sdl validate -

# Validate a file (exit 0 valid, exit 1 invalid)
akt sdl validate deploy.yaml
```

A valid document prints `valid: 1 service(s), 1 group(s), 0 warning(s)`. Problems are listed with a hint:

```
invalid: 1 error(s), 1 warning(s)
  error: services/web/image: image "nginx" has no tag; pin an explicit version for reproducible deployments
    hint: use "nginx:<version>" instead of an untagged image
  warning: profiles/placement/dcloud/pricing: pricing denom "uakt" does not match the default deposit; use "uact" on either rail or pass a matching explicit uakt deposit on chain
```

Redirect the generated SDL to a file to feed `akt deploy` or `akt console deployment create`; both take a file path.

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

SDL prices use canonical `uact` on both rails. A legacy `uakt` price is valid
on chain only with an explicitly matching `--deposit <amount>uakt`. The
`--deposit` argument is chain-rail only: on the chain rail an omitted or `auto`
deposit resolves the live on-chain minimum and an explicit coin is preserved,
while a console-api context funds the deployment from your account credits and
rejects a deposit outright. `--dry-run` displays the resolved chain deposit and
fails if it cannot be determined.

Successful deploys wait up to two minutes for service readiness and then print
live URIs. Use `--ready-timeout` to change the bound or `--no-wait-active` to
return immediately after manifest submission.

**Interactive mode** (default) -- a per-workflow progress display:
```bash
# Full deployment lifecycle with interactive bid selection
akt deploy deployment.yaml
```

**JSONL mode** (for CI/CD and scripting):
```bash
# Automated deployment -- each step emits a JSONL line to stdout
akt deploy deployment.yaml --bid-select cheapest --yes -o jsonl
```

Output is one JSON object per line, one line per step (`create-deployment`, `wait-for-bids`, `select-bid`, `create-lease`, `send-manifest`, `display-result`):
```jsonl
{"workflow":"deploy","id":"wf_a1b2c3","step":"create-deployment","result":"completed","errors":[],"txs":[{"hash":"ABCD...","height":12345,"gas_used":150000,"code":0}]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"wait-for-bids","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"create-lease","result":"completed","errors":[],"txs":[{"hash":"EFGH...","height":12350,"gas_used":120000,"code":0}],"outputs":{"dseq":"12345","provider":"akash1provider..."}}
{"workflow":"deploy","id":"wf_a1b2c3","step":"display-result","result":"completed","errors":[],"txs":[],"outputs":{"dseq":"12345","provider":"akash1provider...","uris":{"web":["https://example.test"]},"ready":true}}
```

Parse with `jq`:
```bash
# Extract the deployment transaction hash
akt deploy deployment.yaml --bid-select cheapest --yes -o jsonl \
  | jq -r 'select(.step == "create-deployment") | .txs[0].hash'
```

### Queries

Akash query commands use a **positional filter argument** instead of `--owner`/`--dseq` flags (see the UX trial note in the [Overview](#overview)). The filter follows the resource hierarchy (`owner/dseq/gseq/oseq/provider`) with smart type detection: a bech32 address is an owner, a number is a dseq. When no owner is given, the context's default account is used. State keywords are also positional: `akt query deployment active` lists active deployments, and identity + state combine as two positional arguments (`akt query deployment 12345 active`). When the identity pins down a single record, the state argument verifies it — the command fails if the record is in a different state instead of printing it. See [SPEC.md §3.8](SPEC.md#38-resource-filter-argument) for full details.

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
akt monitor https://rpc.akt.dev:443/rpc

# Launch directly into a specific dashboard
akt monitor network
akt monitor provider
akt monitor oracle
akt monitor bme

# Or via flag
akt monitor --rpc https://rpc.akt.dev:443/rpc

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

# JSON, for scripts and pipes
akt query deployment -o json

# YAML
akt query bank balances akash1abc... -o yaml
```

### Action Log

```bash
# View recent actions for the current context
akt context log

# Filter by type and limit
akt context log --type tx --limit 10

# Follow one deploy run step by step (the run id shown in SUMMARY)
akt context log --workflow-id 9f2c1ab34d55e017

# Show actions since a duration or timestamp
akt context log --since 1h
akt context log --since 2026-01-01
```

### Console API (Managed Wallet)

Contexts with `--auth-method console-api` use the [Akash Console Managed Wallet API](https://akash.network/docs/api-documentation/console-api/api-reference/) for deployment operations. No local keys are needed -- the Console backend handles signing and broadcasting. Funding is not something you do: you add credits to the account and the platform keeps every deployment funded from that balance. See [How Funding Works](https://akash.network/docs/getting-started/how-funding-works/).

```bash
# Set up a console-api context
akt context create console --network mainnet --auth-method console-api --set-current

# Store your API key as the context credential (created at
# console.akash.network > Settings > API Keys). Validates the key and
# writes it to contexts/<name>/console-api-key (0600).
akt console login

# Deploy using the Console managed wallet (no deposit -- credits fund it)
akt deploy deploy.yaml

# List your deployments
akt query deployment

# Update a deployment
akt tx deployment update deploy.yaml 12345

# Close a deployment
akt tx deployment close 12345

# Drive the Console API directly
akt console deployment list
akt console wallet balance

# Spend history for an explicit range (positional dates)
akt console usage 2026-01-01 2026-01-31
```

The API key resolves `--console-api-key` flag > `AKT_CONSOLE_API_KEY` env var > per-context stored credential, so switching context switches Console identity. Store or replace it without the interactive prompt with `akt context edit <context> --console-api-key <key>` (an empty string removes it). Query commands that read directly from the chain (e.g., `akt query bank balances`) still work normally. Commands that require local signing (e.g., `akt tx bank send`, `akt tx gov vote`) are not supported with `console-api` auth -- use a `keyring` context for those.

Live lease operations reach the provider gateway directly, authorized by a short-lived Console-minted JWT -- no wallet and no local key involved:

```bash
# Stream logs for one service
akt console logs 12345 web --follow

# Last 100 lines across all services
akt console logs 12345 --tail 100

# Kubernetes events for the lease
akt console events 12345 --follow

# One live status snapshot from the provider gateway
akt console status 12345

# Re-poll every 30s until Ctrl-C
akt console status 12345 --watch --interval 30s

# Interactive shell in a container (defaults to /bin/sh)
akt console shell 12345 web

# Run a single command (a.k.a. exec)
akt console shell 12345 web -- ls -la

# Which providers can run this SDL? (public endpoint, no key required)
akt console screen deploy.yaml

# Or screen explicit resources; flags override values loaded from an SDL
akt console screen deploy.yaml --gpu 1 --gpu-model a100
```

Console `uact` escrow amounts are micro-USD and display as dollars. Bid prices
reported in `uact` per block also show an estimated 30-day dollar cost using
six-second blocks; other denoms stay explicit. `akt store export` is a local
inspection and backup format, not a deployable SDL source. There is no separate
`store list`: use `akt store status` for counts and freshness, or export when
individual locally tracked records are needed.

## Roadmap

Upcoming work (see [SPEC.md](SPEC.md) for full details):

- **UX trial follow-up (2026-07)**: decide from feedback whether the duplicated identity/filter flags are restored. The default `command-gating` mode is settled as `dim`; `hide` and `off` remain available.
- **Phase 4**: Plugin system, performance optimization

## License

See [LICENSE](LICENSE).
