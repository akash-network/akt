# Akash CLI (`akt`) - Technical Specification

This document is the detailed technical specification for the `akt` CLI. For architecture overview and design rationale, see [DESIGN.md](DESIGN.md).

---

## Table of Contents

1. [Configuration](#1-configuration)
2. [CLI Command Reference](#2-cli-command-reference)
3. [Flag Specification](#3-flag-specification)
4. [Store Specification](#4-store-specification)
5. [Action Log Specification](#5-action-log-specification)
6. [Sync Engine Specification](#6-sync-engine-specification)
7. [TUI Specification](#7-tui-specification)
8. [Plugin System Specification](#8-plugin-system-specification)
9. [Output Format Specification](#9-output-format-specification)
10. [Error Handling](#10-error-handling)
11. [Phased Implementation Plan](#11-phased-implementation-plan)

---

## 1. Configuration

### 1.1 Home Directory

The akt home directory contains all configuration, contexts, keyrings, stores, and caches.

Resolution order:
1. `--home` flag (highest priority)
2. `AKT_HOME` env var
3. `$XDG_CONFIG_HOME/akt` (if `XDG_CONFIG_HOME` is set)
4. `~/.config/akt` (default)

The active context within the home is selected via `AKT_CONTEXT` env var or `--context` flag (by name, not path).

### 1.2 Config File Schema (`config.yaml`)

A context is a composition of four objects: a **network** (shared), a **keyring** (shared), a **state store** (unique), and an **action log** (unique). Networks and keyrings are defined as top-level shared resources and referenced by name from contexts.

All akt-generated YAML files include a document start line (`---`).

```yaml
---
# ~/.config/akt/config.yaml

# Schema version for forward compatibility
version: 1

# The currently active context
current-context: prod

# Named networks -- shared, can be referenced by multiple contexts
# Instantiatable from built-in templates (mainnet, testnet, sandbox)
networks:
  - name: mainnet
    chain-id: akashnet-2
    endpoints:
      rpc:
        - https://rpc.akashnet.net:443
        - https://rpc-akash.ecostake.com:443
      api:
        - https://api.akashnet.net:443
        - https://akash-api.polkachu.com:443
      grpc:
        - grpc.akashnet.net:443
    gas-prices: "0.025uakt"
    gas-adjustment: "1.5"

  - name: testnet
    chain-id: testnet-02
    endpoints:
      rpc:
        - https://rpc.testnet-02.aksh.pw:443
      api:
        - https://api.testnet-02.aksh.pw:443
      grpc:
        - grpc.testnet-02.aksh.pw:443
    gas-prices: "0.025uakt"
    gas-adjustment: "1.5"

  - name: sandbox
    chain-id: sandbox-01
    endpoints:
      rpc:
        - https://rpc.sandbox-01.aksh.pw:443
      api:
        - https://api.sandbox-01.aksh.pw:443
      grpc:
        - grpc.sandbox-01.aksh.pw:443
    gas-prices: "0.025uakt"
    gas-adjustment: "1.5"

  - name: mainnet-custom              # Example: forked from mainnet for one context
    chain-id: akashnet-2
    endpoints:
      rpc:
        - https://my-private-rpc.example.com:443
      api:
        - https://my-private-api.example.com:443
      grpc:
        - my-private-grpc.example.com:443
    gas-prices: "0.04uakt"
    gas-adjustment: "2.0"

# Keyring definitions -- shared, referenced by contexts
# Adding a key to a keyring makes it available to ALL contexts using that keyring
keyrings:
  - name: default
    backend: os                         # os | file | test | kwallet | pass
    dir: ""                             # empty = <config-root>/keyrings/<name>/

  - name: test-keyring
    backend: test
    dir: ""

# Named contexts -- each composes a network + keyring + unique store + unique action log
contexts:
  - name: prod
    network: mainnet                    # references a network definition
    keyring: default                    # references a keyring definition
    default-account: "alice"            # account name or address; empty = prompt
    gas: auto                           # gas limit override (or "auto")
    fees: ""                            # fixed fees override (overrides network gas-prices)
    provider-defaults:
      auth-type: jwt                    # jwt | mtls

  - name: staging
    network: testnet
    keyring: default
    default-account: "testaccount"
    gas: auto
    fees: ""
    provider-defaults:
      auth-type: jwt

  - name: monitoring                   # read-only context, shares mainnet network
    network: mainnet
    keyring: default
    default-account: ""
    gas: auto
    fees: ""
    provider-defaults:
      auth-type: jwt

# TUI settings
tui:
  theme: dark                           # dark | light
  keybindings: vim                      # vim | default | custom
  custom-keybindings: {}                # only used when keybindings=custom
  refresh-interval: 5s                  # how often to refresh views (in addition to sync events)

# Plugin settings
plugins:
  paths:                                # additional directories to scan for plugins
    - ~/.local/bin
  disabled: []                          # list of plugin names to disable

# Defaults
defaults:
  output: table                         # table | json | yaml
  broadcast-mode: sync                  # sync | async | block
  interactive: true                     # optional; allow TUI mode; false = CLI-only (override with -i)
```

### 1.3 Network Schema

Networks define chain connectivity. They are shared resources -- a single network definition can be referenced by multiple contexts.

| Field            | Type     | Required | Default       | Description                                       |
| ---------------- | -------- | -------- | ------------- | ------------------------------------------------- |
| `name`           | string   | yes      | --            | Unique network identifier                         |
| `chain-id`       | string   | yes      | --            | Blockchain chain ID                               |
| `endpoints.rpc`  | []string | yes      | --            | RPC endpoint URLs (ordered by priority, failover) |
| `endpoints.api`  | []string | no       | []            | REST API endpoint URLs                            |
| `endpoints.grpc` | []string | no       | []            | gRPC endpoint URLs                                |
| `gas-prices`     | string   | no       | `"0.025uakt"` | Default gas price for transactions                |
| `gas-adjustment` | string   | no       | `"1.5"`       | Gas estimation multiplier (when gas=auto)         |

**Network sharing rules:**
- Multiple contexts can reference the same network by name.
- Editing a network (e.g., changing an RPC endpoint) affects all contexts that reference it.
- When editing a network from within a context, the user is prompted:
  - **Edit parent**: Modify the shared network definition. All contexts using it see the change.
  - **Fork**: Create a copy of the network with a new name (e.g., `mainnet` -> `mainnet-<context>`). The current context switches to the fork. Other contexts are unaffected.

**Network templates**: Built-in templates for `mainnet`, `testnet`, and `sandbox` are available via `akt context network create --template <name>`. Templates populate all fields with known-good defaults.

### 1.4 Context Schema

Contexts compose a network, keyring, and context-specific settings. The state store and action log are implicitly created at `<config-root>/contexts/<name>/`.

| Field                         | Type   | Required | Default     | Description                                                        |
| ----------------------------- | ------ | -------- | ----------- | ------------------------------------------------------------------ |
| `name`                        | string | yes      | --          | Unique context identifier                                          |
| `network`                     | string | yes      | --          | Name of network definition to use                                  |
| `keyring`                     | string | no       | `"default"` | Name of keyring definition to use                                  |
| `default-account`             | string | no       | `""`        | Default `--from` value (account name or bech32 address)            |
| `gas`                         | string | no       | `"auto"`    | Gas limit or `"auto"` (overrides network gas settings per-context) |
| `fees`                        | string | no       | `""`        | Fixed fees (overrides network gas-prices if set)                   |
| `provider-defaults.auth-type` | string | no       | `"jwt"`     | Provider auth: `jwt` or `mtls`                                     |

### 1.5 Keyring Schema

Keyrings are shared wallet storage. Adding a key to a keyring makes it immediately available to all contexts that reference that keyring.

| Field     | Type   | Required | Default | Description                                            |
| --------- | ------ | -------- | ------- | ------------------------------------------------------ |
| `name`    | string | yes      | --      | Unique keyring identifier                              |
| `backend` | string | no       | `"os"`  | Backend type: `os`, `file`, `test`, `kwallet`, `pass`  |
| `dir`     | string | no       | `""`    | Keyring directory (default: `<root>/keyrings/<name>/`) |

### 1.6 Config Management (Viper)

Configuration is managed via [Viper](https://github.com/spf13/viper). Viper handles config file reading/writing, environment variable binding, flag binding, and live-reload (via fsnotify built into Viper).

**Key principles:**
- **No global variables.** All state is passed explicitly via function parameters or struct fields.
- **No reading flags into Go variables.** Flags are bound to Viper keys; values are read from Viper or the cobra command at point of use.
- **Viper provides the resolution order natively:** flags > env vars > config file > defaults.

**Viper setup:**
- Config file: `<home>/config.yaml` (YAML format, 2-space indent)
- Env prefix: `AKT` (e.g., `AKT_CONTEXT` maps to `context`)
- Flag binding: Global persistent flags (`--home`, `--context`, `--output`) are bound to Viper keys in the root command's `PersistentPreRunE`
- Live-reload: `viper.WatchConfig()` with `viper.OnConfigChange()` callback

### 1.7 Context Propagation

The context is resolved once at application startup and propagated through the entire session as a single object that all services receive.

**Resolution order** (highest priority wins, provided natively by Viper):

1. Command-line flags (`--context`, `--chain-id`, `--node`, `--from`, etc.)
2. Environment variables (`AKT_CONTEXT`, `AKT_CHAIN_ID`, `AKT_NODE`, `AKT_FROM`, etc.) -- `AKT_CONTEXT` is the primary env var that selects the active context by name
3. Context config (which references its network and keyring)
4. Built-in defaults

The resolved context is injected into every service: chain client, provider gateway, sync engine, store, action log, and TUI models.

### 1.8 Live Reload

The config file (`config.yaml`) is watched for changes using Viper's built-in `WatchConfig()` which uses fsnotify.

**CLI mode behavior:**
- If the config changes during a long-running command (e.g., `akt deploy`), the change is picked up for subsequent operations within that command.
- Flags and env vars still take precedence over config values.
- Example: If the config's RPC endpoint changes and no `--node` flag or `AKT_NODE` env var is set, subsequent queries within the session use the new endpoint.

**TUI mode behavior:**
- Config changes are detected and applied **immediately** to all subsequent actions.
- In TUI mode, config changes override flags and env vars. This is intentional: the TUI is a long-lived session where the user expects real-time configuration to take effect.
- The TUI header updates to reflect the new network/account state.
- Active WebSocket connections are re-established if endpoints change.
- A notification is shown in the status bar when config is reloaded.

**What triggers a reload:**
- Any write to `config.yaml`.
- Network definition changes (endpoints, gas-prices, etc.) propagate to all contexts using that network.
- Keyring changes are picked up on next key operation.
- Context-level changes (default-account, gas, fees) apply to subsequent actions.

### 1.9 Environment Variable Mapping

All environment variables use the `AKT_` prefix. When set, they override the corresponding config value.

| Environment Variable  | Overrides                                               | Example                        |
| --------------------- | ------------------------------------------------------- | ------------------------------ |
| `AKT_HOME`            | Home directory (overrides XDG default)                  | `/path/to/.akt`                |
| `AKT_CONTEXT`         | Active context name (overrides `current-context`)       | `prod`                         |
| `AKT_CHAIN_ID`        | `networks[*].chain-id` (via context's network)          | `akashnet-2`                   |
| `AKT_NODE`            | `networks[*].endpoints.rpc[0]` (via context's network)  | `https://rpc.akashnet.net:443` |
| `AKT_GRPC_ADDR`       | `networks[*].endpoints.grpc[0]` (via context's network) | `grpc.akashnet.net:443`        |
| `AKT_FROM`            | `contexts[*].default-account`                           | `alice`                        |
| `AKT_KEYRING_BACKEND` | `keyrings[*].backend` (via context's keyring)           | `os`                           |
| `AKT_KEYRING_DIR`     | `keyrings[*].dir` (via context's keyring)               | `/path/to/keyring`             |
| `AKT_GAS`             | `contexts[*].gas`                                       | `auto`                         |
| `AKT_GAS_PRICES`      | `networks[*].gas-prices` (via context's network)        | `0.025uakt`                    |
| `AKT_GAS_ADJUSTMENT`  | `networks[*].gas-adjustment` (via context's network)    | `1.5`                          |
| `AKT_FEES`            | `contexts[*].fees`                                      | `5000uakt`                     |
| `AKT_BROADCAST_MODE`  | `defaults.broadcast-mode`                               | `sync`                         |
| `AKT_OUTPUT`          | `defaults.output`                                       | `json`                         |

### 1.9 Built-in Network Templates

The `akt context network create --template <name>` command creates a network definition from a built-in template.

**Template: `mainnet`**
```yaml
name: mainnet
chain-id: akashnet-2
endpoints:
  rpc:
    - https://rpc.akashnet.net:443
    - https://rpc-akash.ecostake.com:443
  api:
    - https://api.akashnet.net:443
    - https://akash-api.polkachu.com:443
  grpc:
    - grpc.akashnet.net:443
gas-prices: "0.025uakt"
gas-adjustment: "1.5"
```

**Template: `testnet`**
```yaml
name: testnet
chain-id: testnet-02
endpoints:
  rpc:
    - https://rpc.testnet-02.aksh.pw:443
  api:
    - https://api.testnet-02.aksh.pw:443
  grpc:
    - grpc.testnet-02.aksh.pw:443
gas-prices: "0.025uakt"
gas-adjustment: "1.5"
```

**Template: `sandbox`**
```yaml
name: sandbox
chain-id: sandbox-01
endpoints:
  rpc:
    - https://rpc.sandbox-01.aksh.pw:443
  api:
    - https://api.sandbox-01.aksh.pw:443
  grpc:
    - grpc.sandbox-01.aksh.pw:443
gas-prices: "0.025uakt"
gas-adjustment: "1.5"
```

---

## 2. CLI Command Reference

Implementation note: `tx` and `query` commands are clean-copied from `akash-network/chain-sdk/go/cli` into `internal/cli/chain`. Only CLI code is copied; all other chain-sdk packages are imported directly. Command flags default to the resolved akt context values unless explicitly overridden.

### 2.1 Command Tree Overview

```
akt
├── context                              # Context management
│   ├── create <name>                    # Create a new context
│   ├── delete <name>                    # Delete a context
│   ├── edit <name>                      # Edit context (fork/edit-parent for network)
│   ├── list                             # List all contexts
│   ├── show                             # Show active context with full details
│   ├── use <name>                       # Switch active context
│   ├── rename <old> <new>              # Rename a context
│   ├── log                              # View action log for current context
│   ├── network                          # Network management (shared resource)
│   │   ├── create <name>                # Create a new network
│   │   ├── delete <name>                # Delete a network (fails if in use)
│   │   ├── edit <name>                  # Edit network definition
│   │   ├── list                         # List all networks (show which contexts use each)
│   │   └── show <name>                  # Show network details
│   └── keys                             # Key management
│       ├── add <name>                   # Add key (mnemonic, ledger, or multisig)
│       ├── delete <name>                # Delete key
│       ├── export <name>                # Export private key (encrypted)
│       ├── import <name> <keyfile>      # Import private key
│       ├── list                         # List all keys
│       ├── show <name|address>          # Show key details
│       ├── rename <old> <new>          # Rename key
│       ├── mnemonic                     # Generate mnemonic
│       └── parse <hex-or-bech32>        # Parse address formats
├── tx                                   # Transaction commands
│   ├── bank
│   │   ├── send <from> <to> <amount>
│   │   └── multi-send <from> <to1,to2,...> <amount>
│   ├── deployment
│   │   ├── create <sdl-file>
│   │   ├── update <sdl-file>
│   │   ├── close
│   │   └── group
│   │       ├── close
│   │       ├── pause
│   │       └── start
│   ├── market
│   │   ├── bid
│   │   │   ├── create
│   │   │   └── close
│   │   └── lease
│   │       ├── create
│   │       ├── withdraw
│   │       └── close
│   ├── provider
│   │   ├── create <config-file>
│   │   └── update <config-file>
│   ├── cert
│   │   ├── generate
│   │   │   ├── client
│   │   │   └── server <domains...>
│   │   ├── publish
│   │   │   ├── client
│   │   │   └── server
│   │   └── revoke
│   │       ├── client
│   │       └── server
│   ├── audit
│   │   └── attr
│   │       ├── create <provider>
│   │       └── delete <provider>
│   ├── staking
│   │   ├── create-validator <validator.json>
│   │   ├── edit-validator
│   │   ├── delegate <validator-addr> <amount>
│   │   ├── redelegate <src-val> <dst-val> <amount>
│   │   ├── unbond <validator-addr> <amount>
│   │   └── cancel-unbond <val-addr> <amount> <creation-height>
│   ├── distribution
│   │   ├── withdraw-rewards <validator-addr>
│   │   ├── withdraw-all-rewards
│   │   ├── set-withdraw-addr <withdraw-addr>
│   │   ├── fund-community-pool <amount>
│   │   └── fund-validator-rewards-pool <val_addr> <amount>
│   ├── gov
│   │   ├── submit-proposal <proposal.json>
│   │   ├── submit-legacy-proposal
│   │   ├── deposit <proposal-id> <deposit>
│   │   ├── vote <proposal-id> <option>
│   │   ├── weighted-vote <proposal-id> <weighted-options>
│   │   ├── draft-proposal
│   │   └── cancel-proposal <proposal-id>
│   ├── authz
│   │   ├── grant <grantee> <authorization> --expiration <exp>
│   │   ├── revoke <grantee> <msg-type>
│   │   └── exec <tx-json-file>
│   ├── feegrant
│   │   ├── grant <granter> <grantee>
│   │   └── revoke <granter> <grantee>
│   ├── escrow
│   │   └── deposit <deployment> <amount>
│   ├── slashing
│   │   └── unjail
│   ├── vesting
│   │   ├── create-vesting-account <to> <amount> <end_time>
│   │   ├── create-permanent-locked-account <to> <amount>
│   │   └── create-periodic-vesting-account <to> <periods_file>
│   ├── upgrade
│   │   ├── software-upgrade <name>
│   │   └── cancel-software-upgrade
│   ├── crisis
│   │   └── invariant-broken <module> <invariant>
│   ├── wasm
│   │   ├── store <wasm-file>
│   │   ├── instantiate <code-id> <init-args>
│   │   ├── instantiate2 <code-id> <init-args> <salt>
│   │   ├── execute <contract> <args>
│   │   ├── migrate <contract> <code-id> <args>
│   │   ├── set-contract-admin <contract> <new-admin>
│   │   ├── clear-contract-admin <contract>
│   │   ├── update-instantiate-config <code-id>
│   │   └── set-contract-label <contract> <label>
│   ├── oracle
│   │   └── feed <asset-denom> <base-denom> <price> <timestamp>
│   ├── bme
│   │   ├── burn-mint <coins-to-burn> <denom-to-mint>
│   │   ├── mint-act <coins-to-burn>
│   │   └── burn-act <coins-to-burn>
│   ├── sign <file>
│   ├── sign-batch <file> [file2...]
│   ├── multisign <file> <name> <sigfile1> [sigfile2...]
│   ├── validate-signatures <file>
│   ├── broadcast <file>
│   ├── encode <file>
│   └── decode <file>
├── query (alias: q)                     # Query commands
│   ├── bank
│   │   ├── balances <address>
│   │   ├── spendable-balances <address>
│   │   ├── total
│   │   ├── denom-metadata
│   │   └── send-enabled [denom1...]
│   ├── deployment
│   │   ├── deployments [--owner] [--dseq] [--state]
│   │   ├── group get
│   │   └── params
│   ├── market
│   │   ├── orders [--owner] [--dseq] [--gseq] [--oseq] [--state]
│   │   ├── bids [--owner] [--dseq] [--gseq] [--oseq] [--provider] [--state]
│   │   ├── leases [--owner] [--dseq] [--gseq] [--oseq] [--provider] [--state]
│   │   └── params
│   ├── provider
│   │   ├── list
│   │   └── get <address>
│   ├── cert
│   │   └── list
│   ├── audit
│   │   ├── list
│   │   └── get [owner] [auditor]
│   ├── escrow
│   │   ├── accounts
│   │   ├── payments
│   │   └── blocks-remaining
│   ├── staking
│   │   ├── validator <val-addr>
│   │   ├── validators
│   │   ├── delegation <del-addr> <val-addr>
│   │   ├── delegations <del-addr>
│   │   ├── delegations-to <val-addr>
│   │   ├── unbonding-delegation <del-addr> <val-addr>
│   │   ├── unbonding-delegations <del-addr>
│   │   ├── unbonding-delegations-from <val-addr>
│   │   ├── redelegation <del-addr> <src-val> <dst-val>
│   │   ├── redelegations <del-addr>
│   │   ├── redelegations-from <val-addr>
│   │   ├── historical-info <height>
│   │   ├── pool
│   │   └── params
│   ├── distribution
│   │   ├── params
│   │   ├── validator-distribution-info <val-addr>
│   │   ├── validator-outstanding-rewards <val-addr>
│   │   ├── commission <val-addr>
│   │   ├── slashes <val-addr> <start-height> <end-height>
│   │   ├── rewards <del-addr> [val-addr]
│   │   └── community-pool
│   ├── gov
│   │   ├── proposal <id>
│   │   ├── proposals
│   │   ├── vote <proposal-id> <voter>
│   │   ├── votes <proposal-id>
│   │   ├── params
│   │   ├── param <type>
│   │   ├── proposer <proposal-id>
│   │   ├── deposit <proposal-id> <depositor>
│   │   ├── deposits <proposal-id>
│   │   └── tally <proposal-id>
│   ├── auth
│   ├── authz
│   ├── feegrant
│   ├── evidence
│   ├── mint
│   ├── params
│   ├── slashing
│   ├── wasm
│   │   ├── list-code
│   │   ├── list-contract-by-code <code-id>
│   │   ├── code <code-id> <output-file>
│   │   ├── code-info <code-id>
│   │   ├── contract <address>
│   │   ├── contract-history <address>
│   │   ├── contract-state all <address>
│   │   ├── contract-state raw <address> <key>
│   │   ├── contract-state smart <address> <query>
│   │   ├── pinned
│   │   ├── params
│   │   ├── build-address <code-hash> <creator> <salt>
│   │   ├── list-contracts-by-creator <creator>
│   │   └── libwasmvm-version
│   ├── oracle
│   │   ├── prices
│   │   ├── aggregated-price <denom>
│   │   └── params
│   ├── bme
│   │   ├── params
│   │   ├── vault-state
│   │   ├── status
│   │   └── ledger
│   ├── ibc
│   ├── ibc-transfer
│   ├── upgrade
│   ├── block [height]
│   ├── blocks
│   ├── block-results [height]
│   ├── tx <hash>
│   ├── txs
│   └── module-name-to-address <module>
├── deploy <sdl-file>                    # Workflow: full deployment lifecycle
├── provider                             # Provider gateway commands
│   ├── status [provider-addr]
│   ├── lease-status
│   ├── lease-logs
│   ├── lease-events
│   ├── lease-shell
│   ├── send-manifest <sdl-file>
│   ├── get-manifest
│   ├── migrate-hostnames
│   └── migrate-endpoints
├── sdl-to-manifest <sdl-file>           # SDL utility
├── store                                # Local store management
│   ├── status
│   ├── export
│   └── import <file>
├── plugin                               # Plugin management
│   ├── install <source>
│   ├── list
│   └── remove <name>
├── events                               # Live blockchain event streaming
│                                        # (no subcommand launches TUI mode)
├── version                              # Version information
└── completion                           # Shell completion scripts
```

### 2.2 Context Commands

#### `akt context create <name>`

Create a new named context. A context references a network and keyring by name.

| Flag                | Type   | Default     | Description                                |
| ------------------- | ------ | ----------- | ------------------------------------------ |
| `--network`         | string | `""`        | Network name to use (required; must exist) |
| `--keyring`         | string | `"default"` | Keyring name to use                        |
| `--default-account` | string | `""`        | Default account name                       |
| `--gas`             | string | `"auto"`    | Gas limit override                         |
| `--fees`            | string | `""`        | Fixed fees override                        |
| `--set-current`     | bool   | `false`     | Set as current context after creation      |

**Examples:**
```bash
# Create context using an existing network
akt context create prod --network mainnet --default-account alice --set-current

# Create context for monitoring (no default account)
akt context create monitoring --network mainnet

# Create context for testnet
akt context create staging --network testnet --keyring test-keyring --default-account testaccount
```

#### `akt context use <name>`

Switch the active context. This changes which network, keyring, store, and action log are used for all subsequent operations.

```bash
akt context use staging
```

#### `akt context list`

List all configured contexts. Marks the current context with `*`. Shows which network and keyring each context references.

```bash
$ akt context list
  NAME          NETWORK     KEYRING     DEFAULT-ACCOUNT   CHAIN-ID
* prod          mainnet     default     alice             akashnet-2
  staging       testnet     test-kr     testaccount       testnet-02
  monitoring    mainnet     default                       akashnet-2
```

#### `akt context show`

Print the current context name and full details (resolved network, keyring, store path, action log path, and all effective settings).

```bash
$ akt context show
Context:         prod
Network:         mainnet
  Chain ID:      akashnet-2
  RPC:           https://rpc.akashnet.net:443 (+1 backup)
  API:           https://api.akashnet.net:443 (+1 backup)
  gRPC:          grpc.akashnet.net:443
  Gas Prices:    0.025uakt
  Gas Adj:       1.5
Keyring:         default (backend: os)
Default Account: alice
Gas:             auto
Fees:            (none)
Provider Auth:   jwt
Store:           ~/.config/akt/contexts/prod/store/
Action Log:      ~/.config/akt/contexts/prod/actions.log
```

#### `akt context edit <name>`

Edit context-level settings. For network-level changes (endpoints, gas-prices), prompts the user to choose:
- **Edit parent network**: Modify the shared network. Changes apply to all contexts using it.
- **Fork network**: Create a private copy of the network for this context only.

| Flag                | Type   | Default | Description                            |
| ------------------- | ------ | ------- | -------------------------------------- |
| `--network`         | string | `""`    | Switch to a different network          |
| `--keyring`         | string | `""`    | Switch to a different keyring          |
| `--default-account` | string | `""`    | Change default account                 |
| `--gas`             | string | `""`    | Change gas setting                     |
| `--fees`            | string | `""`    | Change fees setting                    |
| `--fork-network`    | bool   | `false` | Force fork when editing network fields |

```bash
# Change default account
akt context edit prod --default-account bob

# Switch to a different network
akt context edit staging --network sandbox

# Edit the network's RPC (prompts: edit parent or fork?)
akt context edit prod --rpc https://my-private-rpc:443
```

#### `akt context delete <name>`

Delete a context. Prompts for confirmation unless `--yes` is passed. Cannot delete the current context (switch first). Removes the context's store and action log.

| Flag          | Type | Default | Description                                                 |
| ------------- | ---- | ------- | ----------------------------------------------------------- |
| `--yes`       | bool | `false` | Skip confirmation                                           |
| `--keep-data` | bool | `false` | Delete context config but keep store and action log on disk |

#### `akt context rename <old> <new>`

Rename a context. Updates `current-context` if the renamed context was active. Renames the context data directory.

#### `akt context log`

View the action log for the current context.

| Flag        | Type   | Default | Description                                                         |
| ----------- | ------ | ------- | ------------------------------------------------------------------- |
| `--context` | string | current | Context to view log for                                             |
| `--limit`   | int    | `50`    | Number of entries to show                                           |
| `--type`    | string | `""`    | Filter by action type: `tx`, `query`, `workflow`, `error`           |
| `--since`   | string | `""`    | Show entries since timestamp or duration (e.g., `1h`, `2024-01-01`) |

```bash
$ akt context log --limit 5
  TIME                    TYPE      SUMMARY                                    STATUS
  2026-03-23 10:15:32     tx        deployment create (dseq: 12345)            success
  2026-03-23 10:15:45     tx        market lease create (dseq: 12345)          success
  2026-03-23 10:15:50     workflow  send-manifest -> akash1prov1...            success
  2026-03-23 10:20:01     query     deployment deployments --dseq 12345        success
  2026-03-23 10:25:00     tx        deployment close (dseq: 12345)             success
```

### 2.2.1 Network Commands

#### `akt context network create <name>`

Create a new network definition.

| Flag               | Type     | Default       | Description                                            |
| ------------------ | -------- | ------------- | ------------------------------------------------------ |
| `--template`       | string   | `""`          | Use built-in template: `mainnet`, `testnet`, `sandbox` |
| `--chain-id`       | string   | `""`          | Chain ID (required if no template)                     |
| `--rpc`            | []string | `[]`          | RPC endpoint URLs                                      |
| `--api`            | []string | `[]`          | REST API endpoint URLs                                 |
| `--grpc`           | []string | `[]`          | gRPC endpoint URLs                                     |
| `--gas-prices`     | string   | `"0.025uakt"` | Default gas prices                                     |
| `--gas-adjustment` | string   | `"1.5"`       | Gas estimation multiplier                              |

**Examples:**
```bash
# Create from built-in template
akt context network create mainnet --template mainnet

# Create custom network
akt context network create local --chain-id localnet-1 --rpc http://localhost:26657

# Create all standard networks at once
akt context network create mainnet --template mainnet && \
akt context network create testnet --template testnet && \
akt context network create sandbox --template sandbox
```

#### `akt context network edit <name>`

Edit a network definition. Changes apply to all contexts using this network.

```bash
# Add a backup RPC endpoint
akt context network edit mainnet --rpc https://rpc.akashnet.net:443,https://my-backup-rpc:443

# Change gas prices
akt context network edit mainnet --gas-prices 0.04uakt
```

#### `akt context network delete <name>`

Delete a network definition. Fails if any context references it. Use `--force` to delete anyway (affected contexts become invalid).

#### `akt context network list`

List all networks and which contexts reference each.

```bash
$ akt context network list
  NAME              CHAIN-ID       RPC                          USED BY
  mainnet           akashnet-2     rpc.akashnet.net:443         prod, monitoring
  testnet           testnet-02     rpc.testnet-02.aksh.pw:443   staging
  sandbox           sandbox-01     rpc.sandbox-01.aksh.pw:443
  mainnet-custom    akashnet-2     my-private-rpc:443           (none)
```

#### `akt context network show <name>`

Show full network details.

### 2.2.2 Keys Commands

#### `akt context keys show <name|address>`

Show key details. By default prints name, type, address, and public key. Use `--address` (short `-a`) to print only the bech32 address for scripting.

| Flag         | Short | Type | Default | Description                         |
| ------------ | ----- | ---- | ------- | ----------------------------------- |
| `--address`  | `-a`  | bool | `false` | Print only the bech32 address       |

**Examples:**
```bash
akt context keys show test1
akt context keys show test1 --address
akt context keys show test1 -a
```

### 2.3 Workflow Engine

Workflow commands (`akt deploy`, `akt update`, `akt close`) are driven by a **declarative workflow engine**. Instead of hardcoded command logic, each workflow is a YAML definition that the engine interprets step by step. Users can override built-in workflows or create custom ones.

#### 2.3.1 Workflow Definition Location

Workflow definitions are resolved in order:
1. **Per-context**: `<home>/contexts/<ctx>/workflows/<name>.yaml`
2. **Global user**: `<home>/workflows/<name>.yaml`
3. **Embedded built-in**: compiled into the binary

Top-level commands (`akt deploy`, `akt update`, `akt close`) are thin wrappers that load and run the corresponding workflow. CLI flags are **auto-generated** from the workflow's declared `params` section.

#### 2.3.2 Workflow Definition Format

```yaml
name: deploy
description: Create a deployment, wait for bids, select provider, create lease, send manifest
version: 1

params:
  sdl-file:
    type: file
    required: true
    description: Path to SDL deployment file
  deposit:
    type: string
    default: "5000000uakt"
    description: Initial deposit amount
  bid-timeout:
    type: duration
    default: "5m"
    description: Maximum time to wait for bids
  bid-select:
    type: string
    default: "interactive"
    description: "Bid selection: interactive, cheapest, provider=<addr>"

steps:
  - name: create-deployment
    type: tx
    msg: deployment.MsgCreateDeployment
    params:
      sdl: "{{ index .Params \"sdl-file\" }}"
      deposit: "{{ .Params.deposit }}"
    output:
      dseq: "{{ .Result.dseq }}"
    on-error: abort

  - name: wait-for-bids
    type: wait
    query: market.bids
    params:
      owner: "{{ .Account }}"
      dseq: "{{ (index .Steps \"create-deployment\").dseq }}"
    timeout: "{{ index .Params \"bid-timeout\" }}"
    until: "{{ ge (len .Result.bids) 1 }}"
    on-error: abort

  - name: select-bid
    type: prompt
    mode: "{{ index .Params \"bid-select\" }}"
    data: "{{ (index .Steps \"wait-for-bids\").bids }}"
    display:
      columns: [provider, price, audited]

  - name: create-lease
    type: tx
    msg: market.MsgCreateLease
    params:
      bid-id: "{{ (index .Steps \"select-bid\").bid.id }}"
    on-error: abort

  - name: send-manifest
    type: provider
    action: send-manifest
    params:
      provider: "{{ (index .Steps \"select-bid\").bid.provider }}"
      dseq: "{{ (index .Steps \"create-deployment\").dseq }}"
      sdl: "{{ index .Params \"sdl-file\" }}"
    retry:
      max: 3
      delay: "5s"
    on-error: abort

  - name: display-result
    type: output
    template: |
      Deployment active!
        DSEQ: {{ (index .Steps "create-deployment").dseq }}
```

#### 2.3.3 Step Types

| Type       | Description                                    | Key fields                                    |
|------------|------------------------------------------------|-----------------------------------------------|
| `tx`       | Broadcast a transaction                        | `msg`, `params`, `output`, `on-error`         |
| `query`    | Execute a chain query                          | `query`, `params`, `output`                   |
| `wait`     | Poll a query until a condition is met          | `query`, `params`, `until`, `timeout`         |
| `prompt`   | Interactive user input (bid selection, confirm) | `mode`, `data`, `display`, `output`          |
| `provider` | Provider gateway call                          | `action`, `params`, `retry`                   |
| `output`   | Display formatted output                       | `template`                                    |
| `shell`    | Run a shell command (for custom workflows)     | `command`, `output`                           |
| `check`    | Assert a condition, skip/abort if not met      | `condition`, `on-fail: skip\|abort`           |

#### 2.3.4 Template Variables

Steps use Go templates (`{{ }}`) with access to:

| Variable            | Description                                    | Example                             |
|---------------------|------------------------------------------------|-------------------------------------|
| `.Account`          | Default account from context                   | `{{ .Account }}`                    |
| `.Params.<name>`    | Workflow parameter value                       | `{{ .Params.deposit }}`             |
| `.Steps.<name>.<key>` | Output from a previous step                 | `{{ (index .Steps "create").dseq }}` |
| `.Result`           | Raw result of current step (in output/until)   | `{{ .Result.dseq }}`               |
| `.WorkflowID`       | Unique ID for this workflow run                | `{{ .WorkflowID }}`                |

#### 2.3.5 Error Handling

Each step's `on-error` field controls behavior on failure:

| Value      | Behavior                            |
|------------|-------------------------------------|
| `abort`    | Stop the workflow (default)         |
| `continue` | Log error and proceed to next step  |
| `skip`     | Skip silently                       |

Steps can also define `retry` with `max` attempts and `delay` between retries.

#### 2.3.6 Param Types

| Type       | Description                       | Flag type      |
|------------|-----------------------------------|----------------|
| `string`   | Plain string                      | `--name value` |
| `int`      | Integer                           | `--name 5`     |
| `bool`     | Boolean                           | `--name`       |
| `duration` | Go duration string                | `--name 5m`    |
| `file`     | File path (positional if first)   | positional arg |

#### 2.3.7 Built-in Workflows

Three workflows ship as embedded defaults:

- **deploy**: Full deployment lifecycle (create deployment -> wait bids -> select -> lease -> manifest -> active)
- **update**: Update deployment (update tx -> send manifest)
- **close**: Close deployment (close tx)

#### `akt deploy <sdl-file>`

The flagship workflow command. Orchestrates the full deployment lifecycle:

1. **Create deployment transaction** on chain from the SDL file.
2. **Wait for bids** from providers (configurable timeout).
3. **Select a bid** -- interactive (present bids in a table for user selection) or automatic (cheapest, or by provider filter).
4. **Create lease transaction** with the selected bid.
5. **Send manifest** to the provider.
6. **Wait for deployment to become active** (provider acknowledges, containers start).
7. **Display endpoint URLs** for the deployed services.

| Flag               | Type     | Default         | Description                                                 |
| ------------------ | -------- | --------------- | ----------------------------------------------------------- |
| `--from`           | string   | context default | Account to deploy from                                      |
| `--deposit`        | string   | auto-detect     | Initial deposit amount                                      |
| `--bid-timeout`    | duration | `5m`            | Maximum time to wait for bids                               |
| `--min-bids`       | int      | `1`             | Minimum bids before selection                               |
| `--bid-select`     | string   | `"interactive"` | Bid selection: `interactive`, `cheapest`, `provider=<addr>` |
| `--no-wait-bids`   | bool     | `false`         | Exit after creating deployment (don't wait for bids)        |
| `--no-wait-lease`  | bool     | `false`         | Exit after creating lease (don't send manifest)             |
| `--no-wait-active` | bool     | `false`         | Exit after sending manifest (don't wait for active)         |
| `--label`          | string   | `""`            | User label for local store metadata                         |
| `--note`           | string   | `""`            | User note for local store metadata                          |
| `--yes`            | bool     | `false`         | Skip all confirmations                                      |
| `--dry-run`        | bool     | `false`         | Print what would happen without executing                   |

**Transaction flags** (inherited): `--gas`, `--gas-prices`, `--fees`, `--gas-adjustment`, `--broadcast-mode`

**Interactive mode** (TTY detected):
```
$ akt deploy deployment.yaml

Creating deployment from deployment.yaml...
  Owner:    akash1abc...
  DSEQ:     12345
  Deposit:  5000000uakt

Transaction broadcast successfully. Waiting for bids...

Received 3 bids:

  #  PROVIDER              PRICE/BLOCK    AUDITED
  1  akash1prov1...        12.5 uakt      yes
  2  akash1prov2...        15.0 uakt      yes
  3  akash1prov3...        10.0 uakt      no

Select bid [1-3]: 1

Creating lease with provider akash1prov1...
Sending manifest...
Waiting for deployment to become active...

Deployment active!

  Service      URL
  web          http://abc123.provider1.akash.network
  api          http://def456.provider1.akash.network:8080
```

**Non-interactive mode** (piped):
```bash
akt deploy deployment.yaml --bid-select cheapest --yes --output json
```

### 2.4 Provider Gateway Commands

#### `akt provider status [provider-addr]`

Query provider status. If `provider-addr` is omitted, uses the provider from the active lease context.

| Flag          | Type   | Default         | Description              |
| ------------- | ------ | --------------- | ------------------------ |
| `--provider`  | string | `""`            | Provider address         |
| `--auth-type` | string | context default | Auth type: `jwt`, `mtls` |

#### `akt provider lease-status`

Query lease deployment status from the provider gateway.

| Flag          | Type   | Default         | Description         |
| ------------- | ------ | --------------- | ------------------- |
| `--dseq`      | uint64 | required        | Deployment sequence |
| `--gseq`      | uint32 | `1`             | Group sequence      |
| `--oseq`      | uint32 | `1`             | Order sequence      |
| `--provider`  | string | required        | Provider address    |
| `--from`      | string | context default | Owner account       |
| `--auth-type` | string | context default | Auth type           |

#### `akt provider lease-logs`

Stream container logs from a lease.

| Flag          | Type   | Default         | Description               |
| ------------- | ------ | --------------- | ------------------------- |
| `--dseq`      | uint64 | required        | Deployment sequence       |
| `--gseq`      | uint32 | `1`             | Group sequence            |
| `--oseq`      | uint32 | `1`             | Order sequence            |
| `--provider`  | string | required        | Provider address          |
| `--from`      | string | context default | Owner account             |
| `--service`   | string | `""`            | Filter by service name    |
| `--follow`    | bool   | `false`         | Stream logs continuously  |
| `--tail`      | int64  | `-1`            | Lines from end (-1 = all) |
| `--auth-type` | string | context default | Auth type                 |

#### `akt provider lease-events`

Stream Kubernetes events from a lease.

Same flags as `lease-logs` (minus `--service`, `--tail`), plus `--follow`.

#### `akt provider lease-shell`

Open an interactive shell into a running container.

| Flag          | Type   | Default         | Description         |
| ------------- | ------ | --------------- | ------------------- |
| `--dseq`      | uint64 | required        | Deployment sequence |
| `--gseq`      | uint32 | `1`             | Group sequence      |
| `--oseq`      | uint32 | `1`             | Order sequence      |
| `--provider`  | string | required        | Provider address    |
| `--from`      | string | context default | Owner account       |
| `--service`   | string | required        | Service name        |
| `--tty`       | bool   | `true`          | Allocate a TTY      |
| `--stdin`     | bool   | `true`          | Attach stdin        |
| `--auth-type` | string | context default | Auth type           |

Remaining arguments after `--` are passed as the shell command. Default: `/bin/sh`.

```bash
akt provider lease-shell --dseq 12345 --provider akash1prov... --service web -- /bin/bash
```

#### `akt provider send-manifest <sdl-file>`

Send an SDL manifest to provider(s) for an existing lease.

| Flag          | Type   | Default         | Description                                                   |
| ------------- | ------ | --------------- | ------------------------------------------------------------- |
| `--dseq`      | uint64 | required        | Deployment sequence                                           |
| `--from`      | string | context default | Owner account                                                 |
| `--provider`  | string | `""`            | Specific provider (default: all providers with active leases) |
| `--auth-type` | string | context default | Auth type                                                     |

#### `akt provider get-manifest`

Retrieve the current manifest from a provider.

#### `akt provider migrate-hostnames`

Migrate hostnames from one deployment to another on the same provider.

| Flag                 | Type     | Default         | Description                |
| -------------------- | -------- | --------------- | -------------------------- |
| `--dseq`             | uint64   | required        | Source deployment sequence |
| `--destination-dseq` | uint64   | required        | Target deployment sequence |
| `--from`             | string   | context default | Owner account              |
| `--provider`         | string   | required        | Provider address           |
| `--hostnames`        | []string | required        | Hostnames to migrate       |
| `--auth-type`        | string   | context default | Auth type                  |

#### `akt provider migrate-endpoints`

Same pattern as `migrate-hostnames` but for IP endpoints.

### 2.5 Store Commands

#### `akt store status`

Display local store information for the current context.

```
$ akt store status
Context:      mainnet
Store Path:   ~/.config/akt/stores/mainnet/
Database:     deployments.db (2.4 MB)
Schema:       v3

Records:
  Deployments:  47 (12 active, 35 closed)
  Leases:       52
  Bids:         156

Sync State:
  Last Block:   18234567
  Last Sync:    2026-03-23 10:15:32 UTC
  Status:       synced
```

#### `akt store export`

Export the local store to YAML or JSON.

| Flag             | Type   | Default  | Description                                |
| ---------------- | ------ | -------- | ------------------------------------------ |
| `--output`       | string | `"yaml"` | Export format: `yaml`, `json`              |
| `--file`         | string | `""`     | Output file (default: stdout)              |
| `--filter-state` | string | `""`     | Filter by state: `active`, `closed`, `all` |

#### `akt store import <file>`

Import records from a previously exported file.

| Flag        | Type | Default | Description                           |
| ----------- | ---- | ------- | ------------------------------------- |
| `--merge`   | bool | `true`  | Merge with existing records (default) |
| `--replace` | bool | `false` | Replace entire store contents         |
| `--dry-run` | bool | `false` | Show what would be imported           |

---

## 3. Flag Specification

### 3.1 Global Persistent Flags

Applied to every command via the root command's `PersistentFlags()`.

| Flag        | Short | Type   | Default                  | Description                                                      |
| ----------- | ----- | ------ | ------------------------ | ---------------------------------------------------------------- |
| `--home`    |       | string | `$AKT_HOME` or XDG default | Home directory for config, contexts, and keyrings             |
| `--context` |       | string | config `current-context` | Active context name (overrides AKT_CONTEXT)                      |
| `--output`  | `-o`  | string | `"table"`                | Output format: `table`, `json`, `yaml`                           |
| `--raw`     |       | bool   | `false`                  | Output raw chain response instead of user-friendly formatting    |
| `--debug`   | `-d`  | bool   | `false`                  | Enable debug logging                                             |

### 3.2 Transaction Flags

Added to all `tx` commands via `AddTxFlagsToCmd()`.

| Flag                 | Short | Type     | Default                     | Description                                                   |
| -------------------- | ----- | -------- | --------------------------- | ------------------------------------------------------------- |
| `--from`             |       | string   | context default             | Signing account (name or address)                             |
| `--gas`              |       | string   | context default or `"auto"` | Gas limit or `"auto"`                                         |
| `--gas-prices`       |       | string   | context default             | Gas prices (e.g., `0.025uakt`)                                |
| `--gas-adjustment`   |       | string   | context default or `"1.5"`  | Gas estimation multiplier                                     |
| `--fees`             |       | string   | `""`                        | Fixed fees (overrides gas-prices)                             |
| `--broadcast-mode`   | `-b`  | string   | `"sync"`                    | `sync`, `async`, or `block`                                   |
| `--sign-mode`        |       | string   | `""`                        | Signing mode: `direct`, `amino-json`, `direct-aux`, `textual` |
| `--keyring-backend`  |       | string   | context default             | Keyring backend override                                      |
| `--keyring-dir`      |       | string   | context default             | Keyring directory override                                    |
| `--note`             |       | string   | `""`                        | Transaction memo/note                                         |
| `--timeout-height`   |       | uint64   | `0`                         | Block height timeout                                          |
| `--timeout-duration` |       | duration | `0`                         | Time-based timeout                                            |
| `--sequence`         |       | uint64   | `0`                         | Account sequence (0 = auto)                                   |
| `--account-number`   |       | uint64   | `0`                         | Account number (0 = auto)                                     |
| `--fee-granter`      |       | string   | `""`                        | Fee granter address                                           |
| `--fee-payer`        |       | string   | `""`                        | Fee payer address                                             |
| `--generate-only`    |       | bool     | `false`                     | Build but don't sign/broadcast                                |
| `--offline`          |       | bool     | `false`                     | Offline mode (no RPC queries)                                 |
| `--dry-run`          |       | bool     | `false`                     | Simulate the transaction                                      |
| `--yes`              | `-y`  | bool     | `false`                     | Skip confirmation prompts                                     |
| `--ledger`           |       | bool     | `false`                     | Use Ledger hardware wallet                                    |
| `--unordered`        |       | bool     | `false`                     | Unordered transaction                                         |

### 3.3 Query Flags

Added to all `query` commands via `AddQueryFlagsToCmd()`.

| Flag              | Type   | Default         | Description                                 |
| ----------------- | ------ | --------------- | ------------------------------------------- |
| `--node`          | string | context default | RPC endpoint override                       |
| `--grpc-addr`     | string | context default | gRPC endpoint override                      |
| `--grpc-insecure` | bool   | `false`         | Use insecure gRPC connection                |
| `--height`        | int64  | `0`             | Query at specific block height (0 = latest) |

### 3.4 Pagination Flags

Added to list-type query commands via `AddPaginationFlagsToCmd()`.

| Flag            | Type   | Default | Description                     |
| --------------- | ------ | ------- | ------------------------------- |
| `--page`        | uint64 | `1`     | Page number                     |
| `--limit`       | uint64 | `100`   | Results per page                |
| `--offset`      | uint64 | `0`     | Result offset                   |
| `--page-key`    | string | `""`    | Pagination key for next page    |
| `--count-total` | bool   | `false` | Include total count in response |
| `--reverse`     | bool   | `false` | Reverse result order            |

### 3.5 Akash Resource ID Flags

Used by deployment, market, and escrow commands. These compose hierarchically: LeaseID includes BidID includes OrderID includes GroupID includes DeploymentID.

| Flag         | Type   | Default                   | Description                |
| ------------ | ------ | ------------------------- | -------------------------- |
| `--owner`    | string | context `default-account` | Deployment owner address   |
| `--dseq`     | uint64 | `0`                       | Deployment sequence number |
| `--gseq`     | uint32 | `1`                       | Group sequence number      |
| `--oseq`     | uint32 | `1`                       | Order sequence number      |
| `--provider` | string | `""`                      | Provider address           |

### 3.6 Deployment Query Flags

Used by `query deployment deployments`. When `--dseq` is provided, returns a single deployment (replaces the old `get` subcommand). When omitted, returns a filtered list (replaces the old `list` subcommand).

| Flag      | Type   | Default         | Description                                                 |
| --------- | ------ | --------------- | ----------------------------------------------------------- |
| `--owner` | string | context default | Filter by owner (defaults to current account if set)        |
| `--dseq`  | uint64 | `0`             | Get specific deployment by sequence (0 = list all matching) |
| `--state` | string | `""`            | Filter by state: `active`, `closed`, or empty for all       |

### 3.7 Market Query Flags

Used by `query market orders`, `query market bids`, and `query market leases`. When resource ID flags are fully specified (e.g., `--dseq` + `--gseq` + `--oseq` for an order), returns a single record. When partially specified or omitted, returns a filtered list.

| Flag         | Type   | Default | Description                               |
| ------------ | ------ | ------- | ----------------------------------------- |
| `--owner`    | string | `""`    | Filter by deployment owner                |
| `--dseq`     | uint64 | `0`     | Filter by deployment sequence             |
| `--gseq`     | uint32 | `0`     | Filter by group sequence                  |
| `--oseq`     | uint32 | `0`     | Filter by order sequence                  |
| `--provider` | string | `""`    | Filter by provider (bids and leases only) |
| `--state`    | string | `""`    | Filter by state                           |

---

## 4. Store Specification

### 4.1 Store Interface

```go
package store

import (
    "context"
    "io"
)

// ExportFormat represents the serialization format for import/export.
type ExportFormat int

const (
    FormatYAML ExportFormat = iota
    FormatJSON
)

// Store defines the interface for the local deployment store.
// Implementations must be safe for concurrent use.
type Store interface {
    // Deployment operations
    PutDeployment(ctx context.Context, d *DeploymentRecord) error
    GetDeployment(ctx context.Context, owner string, dseq uint64) (*DeploymentRecord, error)
    ListDeployments(ctx context.Context, filter DeploymentFilter) ([]*DeploymentRecord, error)
    DeleteDeployment(ctx context.Context, owner string, dseq uint64) error

    // Lease operations
    PutLease(ctx context.Context, l *LeaseRecord) error
    GetLease(ctx context.Context, id LeaseID) (*LeaseRecord, error)
    ListLeases(ctx context.Context, filter LeaseFilter) ([]*LeaseRecord, error)
    DeleteLease(ctx context.Context, id LeaseID) error

    // Bid operations
    PutBid(ctx context.Context, b *BidRecord) error
    GetBid(ctx context.Context, id BidID) (*BidRecord, error)
    ListBids(ctx context.Context, filter BidFilter) ([]*BidRecord, error)

    // Sync state management
    GetSyncState(ctx context.Context) (*SyncState, error)
    PutSyncState(ctx context.Context, s *SyncState) error

    // Schema management
    SchemaVersion() uint64
    Migrate(ctx context.Context) error

    // Import/Export
    Export(ctx context.Context, w io.Writer, format ExportFormat) error
    Import(ctx context.Context, r io.Reader, format ExportFormat, merge bool) error

    // Lifecycle
    Close() error
}
```

### 4.2 Record Types

#### DeploymentRecord

```go
type DeploymentRecord struct {
    // Identity
    Owner string `json:"owner" yaml:"owner"`
    DSeq  uint64 `json:"dseq"  yaml:"dseq"`

    // State
    State     string `json:"state"      yaml:"state"`      // active, closed
    Version   []byte `json:"version"    yaml:"version"`     // on-chain version hash

    // SDL
    SDLHash   string `json:"sdl_hash"   yaml:"sdl_hash"`   // SHA256 of SDL content
    SDLPath   string `json:"sdl_path"   yaml:"sdl_path"`   // local path to SDL file (if known)

    // Financials
    Deposit       string `json:"deposit"        yaml:"deposit"`        // initial deposit
    EscrowBalance string `json:"escrow_balance" yaml:"escrow_balance"` // current escrow balance
    Transferred   string `json:"transferred"    yaml:"transferred"`    // total transferred to providers

    // Timestamps
    CreatedAt   int64 `json:"created_at"   yaml:"created_at"`   // Unix timestamp
    UpdatedAt   int64 `json:"updated_at"   yaml:"updated_at"`   // last store update
    ClosedAt    int64 `json:"closed_at"    yaml:"closed_at"`    // 0 if not closed
    CreatedHeight int64 `json:"created_height" yaml:"created_height"` // block height

    // User metadata (local-only, not on chain)
    Labels map[string]string `json:"labels" yaml:"labels"` // user-defined key-value pairs
    Notes  string            `json:"notes"  yaml:"notes"`  // free-form notes
    Tags   []string          `json:"tags"   yaml:"tags"`   // user-defined tags

    // Sync metadata
    RecordVersion uint64 `json:"record_version" yaml:"record_version"` // monotonic, for sync
}
```

#### LeaseRecord

```go
type LeaseID struct {
    Owner    string `json:"owner"    yaml:"owner"`
    DSeq     uint64 `json:"dseq"     yaml:"dseq"`
    GSeq     uint32 `json:"gseq"     yaml:"gseq"`
    OSeq     uint32 `json:"oseq"     yaml:"oseq"`
    Provider string `json:"provider" yaml:"provider"`
}

type LeaseRecord struct {
    ID LeaseID `json:"id" yaml:"id"`

    // State
    State string `json:"state" yaml:"state"` // active, closed, insufficient_funds

    // Pricing
    Price string `json:"price" yaml:"price"` // price per block

    // Provider info
    ProviderURI string `json:"provider_uri" yaml:"provider_uri"` // provider gateway URL

    // Endpoints (populated after manifest send)
    Endpoints []LeaseEndpoint `json:"endpoints" yaml:"endpoints"`

    // Timestamps
    CreatedAt int64 `json:"created_at" yaml:"created_at"`
    ClosedAt  int64 `json:"closed_at"  yaml:"closed_at"`

    // Sync
    RecordVersion uint64 `json:"record_version" yaml:"record_version"`
}

type LeaseEndpoint struct {
    Service      string `json:"service"       yaml:"service"`
    ExternalPort uint32 `json:"external_port" yaml:"external_port"`
    URI          string `json:"uri"           yaml:"uri"`
}
```

#### BidRecord

```go
type BidID struct {
    Owner    string `json:"owner"    yaml:"owner"`
    DSeq     uint64 `json:"dseq"     yaml:"dseq"`
    GSeq     uint32 `json:"gseq"     yaml:"gseq"`
    OSeq     uint32 `json:"oseq"     yaml:"oseq"`
    Provider string `json:"provider" yaml:"provider"`
}

type BidRecord struct {
    ID BidID `json:"id" yaml:"id"`

    // State
    State string `json:"state" yaml:"state"` // open, matched, closed, lost

    // Pricing
    Price string `json:"price" yaml:"price"`

    // Provider info
    ProviderAttributes map[string]string `json:"provider_attributes" yaml:"provider_attributes"`
    ProviderAudited    bool             `json:"provider_audited"    yaml:"provider_audited"`

    // Timestamps
    CreatedAt int64 `json:"created_at" yaml:"created_at"`

    // Sync
    RecordVersion uint64 `json:"record_version" yaml:"record_version"`
}
```

#### SyncState

```go
type SyncState struct {
    // Last successfully processed block height
    LastBlockHeight int64 `json:"last_block_height" yaml:"last_block_height"`

    // Timestamp of last successful sync
    LastSyncTime int64 `json:"last_sync_time" yaml:"last_sync_time"`

    // Accounts being tracked (owner addresses)
    TrackedAccounts []string `json:"tracked_accounts" yaml:"tracked_accounts"`

    // Schema version of the database
    SchemaVersion uint64 `json:"schema_version" yaml:"schema_version"`
}
```

### 4.3 Filter Types

```go
type DeploymentFilter struct {
    Owner string   // filter by owner address
    State string   // filter by state (active, closed, or empty for all)
    Tags  []string // filter by tags (AND logic)
    Label string   // filter by label key=value
}

type LeaseFilter struct {
    Owner    string
    DSeq     uint64 // 0 = no filter
    Provider string
    State    string
}

type BidFilter struct {
    Owner    string
    DSeq     uint64
    Provider string
    State    string
}
```

### 4.4 bbolt Bucket Structure

```
deployments/
  <owner>:<dseq> -> DeploymentRecord (JSON-encoded)

leases/
  <owner>:<dseq>:<gseq>:<oseq>:<provider> -> LeaseRecord (JSON-encoded)

bids/
  <owner>:<dseq>:<gseq>:<oseq>:<provider> -> BidRecord (JSON-encoded)

sync/
  state -> SyncState (JSON-encoded)

meta/
  schema_version -> uint64 (binary)
```

### 4.5 Schema Versioning

- Schema version is a monotonically increasing `uint64` stored in the `meta/schema_version` key.
- Each version has a corresponding migration function: `func migrateV<N>(tx *bbolt.Tx) error`.
- On `Store.Migrate()`, all pending migrations are applied in order within a single bbolt transaction.
- Migrations are forward-only. Downgrade is not supported; use `import` from a prior export.

### 4.6 Export Format

Exports include a header with metadata:

```yaml
---
# akt store export
version: 1
context: mainnet
schema_version: 3
exported_at: "2026-03-23T10:15:32Z"
sync_state:
  last_block_height: 18234567
  last_sync_time: 1742724932
deployments:
  - owner: akash1abc...
    dseq: 12345
    state: active
    sdl_hash: sha256:abc123...
    # ... full DeploymentRecord fields
leases:
  - id:
      owner: akash1abc...
      dseq: 12345
      gseq: 1
      oseq: 1
      provider: akash1prov...
    # ... full LeaseRecord fields
bids:
  - id:
      owner: akash1abc...
      dseq: 12345
      gseq: 1
      oseq: 1
      provider: akash1prov...
    # ... full BidRecord fields
```

---

## 5. Action Log Specification

The action log is an append-only log unique to each context. It records every user action performed within the context, providing an audit trail and enabling troubleshooting.

### 5.1 Action Log Location

Each context has its own action log at:
```
<config-root>/contexts/<context-name>/actions.log
```

### 5.2 Action Types

| Type       | Description                                | Logged Fields                                                         |
| ---------- | ------------------------------------------ | --------------------------------------------------------------------- |
| `tx`       | A blockchain transaction                   | Msg type, msg body, tx hash, height, gas used, result code, error     |
| `query`    | A chain query                              | Query path, parameters, result summary, duration                      |
| `workflow` | A multi-step workflow (e.g., `akt deploy`) | Workflow name, step sequence, each step's type and result             |
| `provider` | A provider gateway operation               | Operation (send-manifest, lease-logs, etc.), provider address, result |
| `context`  | A context management operation             | Operation (switch, edit, etc.), old/new values                        |
| `error`    | A failed operation                         | Original action type, error message, context                          |

### 5.3 Action Entry Format

Each log entry is a single JSON line (JSONL format) for easy parsing:

```jsonl
{"ts":"2026-03-23T10:15:32Z","type":"tx","action":"deployment.MsgCreateDeployment","dseq":12345,"tx_hash":"ABC123...","height":18234567,"gas_used":200000,"code":0}
{"ts":"2026-03-23T10:15:45Z","type":"tx","action":"market.MsgCreateLease","dseq":12345,"provider":"akash1prov1...","tx_hash":"DEF456...","height":18234568,"gas_used":150000,"code":0}
{"ts":"2026-03-23T10:15:50Z","type":"provider","action":"send-manifest","dseq":12345,"provider":"akash1prov1...","status":"success"}
{"ts":"2026-03-23T10:20:01Z","type":"query","action":"deployment.deployments","params":{"dseq":12345},"duration_ms":120}
{"ts":"2026-03-23T10:25:00Z","type":"tx","action":"deployment.MsgCloseDeployment","dseq":12345,"tx_hash":"GHI789...","height":18234600,"gas_used":100000,"code":0}
{"ts":"2026-03-23T10:30:00Z","type":"error","action":"tx.bank.send","error":"insufficient funds","account":"alice"}
```

### 5.4 Action Entry Schema

```go
type ActionEntry struct {
    // Common fields (all action types)
    Timestamp  time.Time       `json:"ts"`
    Type       ActionType      `json:"type"`       // tx, query, workflow, provider, context, error
    Action     string          `json:"action"`     // specific action identifier

    // Transaction fields (type=tx)
    TxHash     string          `json:"tx_hash,omitempty"`
    Height     int64           `json:"height,omitempty"`
    GasUsed    int64           `json:"gas_used,omitempty"`
    ResultCode uint32          `json:"code,omitempty"`

    // Resource ID fields (type=tx, query, provider)
    DSeq       uint64          `json:"dseq,omitempty"`
    GSeq       uint32          `json:"gseq,omitempty"`
    OSeq       uint32          `json:"oseq,omitempty"`
    Provider   string          `json:"provider,omitempty"`
    Account    string          `json:"account,omitempty"`

    // Query fields (type=query)
    Params     json.RawMessage `json:"params,omitempty"`
    DurationMs int64           `json:"duration_ms,omitempty"`

    // Workflow fields (type=workflow)
    WorkflowID string          `json:"workflow_id,omitempty"` // groups steps of a single workflow
    Step       int             `json:"step,omitempty"`
    StepName   string          `json:"step_name,omitempty"`

    // Error fields (type=error, or on any failed action)
    Error      string          `json:"error,omitempty"`
    Status     string          `json:"status,omitempty"`   // success, failed, timeout
}
```

### 5.5 Log Interface

```go
type ActionLog interface {
    // Append a new entry to the log
    Log(ctx context.Context, entry ActionEntry) error

    // Read entries with filtering
    Read(ctx context.Context, filter ActionFilter) ([]ActionEntry, error)

    // Export the full log
    Export(ctx context.Context, w io.Writer) error

    // Lifecycle
    Close() error
}

type ActionFilter struct {
    Type    ActionType    // filter by action type (empty = all)
    Since   time.Time     // entries after this time
    Limit   int           // max entries to return (0 = no limit)
    DSeq    uint64        // filter by deployment sequence
    Account string        // filter by account
}
```

### 5.6 Log Rotation

- Log files are rotated when they exceed 10 MB.
- Rotated logs are named `actions.log.1`, `actions.log.2`, etc.
- A maximum of 5 rotated logs are kept (total ~50 MB per context).
- `akt context log` reads across all rotated files transparently.

---

## 6. Sync Engine Specification

### 6.1 Overview

The sync engine keeps the local deployment store synchronized with on-chain state. It runs as a background goroutine during active CLI/TUI sessions. There is no persistent daemon.

### 6.2 Subscription

The engine subscribes to the RPC WebSocket endpoint for two event types:

- `tm.event='Tx'` -- Transaction events (filtered by module events related to deployment, market, escrow)
- `tm.event='NewBlock'` -- New block events (for block height tracking and periodic reconciliation)

Event filter query for a specific owner:
```
tm.event='Tx' AND message.sender='<owner-address>'
```

Additional filters for market events:
```
tm.event='Tx' AND akash.market.v1beta4.EventBidCreated.owner='<owner-address>'
tm.event='Tx' AND akash.market.v1beta4.EventLeaseCreated.owner='<owner-address>'
```

### 6.3 Event Processing

Events are routed to the reconciler based on their type:

| Chain Event                      | Store Action                                                         |
| -------------------------------- | -------------------------------------------------------------------- |
| `deployment.MsgCreateDeployment` | Create `DeploymentRecord` with state=`active`                        |
| `deployment.MsgUpdateDeployment` | Update `DeploymentRecord` version and SDL hash                       |
| `deployment.MsgCloseDeployment`  | Set state=`closed`, set `closed_at`                                  |
| `deployment.MsgCloseGroup`       | Update corresponding group records                                   |
| `market.EventBidCreated`         | Create `BidRecord` with state=`open`                                 |
| `market.EventBidClosed`          | Set bid state=`closed`                                               |
| `market.EventLeaseCreated`       | Create `LeaseRecord` with state=`active`, update bid state=`matched` |
| `market.EventLeaseClosed`        | Set lease state=`closed`                                             |
| `escrow.EventAccountSettled`     | Update `DeploymentRecord` escrow balance                             |
| `escrow.EventPaymentCompleted`   | Update transferred amount                                            |

### 6.4 Startup Reconciliation

On first launch for a context (no `SyncState` in store):

1. Query all deployments for each tracked account: `query deployment deployments --owner <addr>`.
2. For each deployment, query leases: `query market leases --owner <addr> --dseq <dseq>`.
3. For each deployment, query bids: `query market bids --owner <addr> --dseq <dseq>`.
4. Store all records.
5. Set `SyncState.LastBlockHeight` to the current chain height.

On subsequent launches (existing `SyncState`):

1. Query the current chain height.
2. If `current_height - last_block_height > 1000`, perform a full reconciliation.
3. Otherwise, query transaction events in the missed block range and apply them.

### 6.5 Reconnection Strategy

| Attempt | Delay        | Notes      |
| ------- | ------------ | ---------- |
| 1       | 1s + jitter  |            |
| 2       | 2s + jitter  |            |
| 3       | 4s + jitter  |            |
| 4       | 8s + jitter  |            |
| 5       | 16s + jitter |            |
| 6       | 32s + jitter |            |
| 7+      | 60s + jitter | Cap at 60s |

Jitter: random value in `[0, 0.5 * delay)`.

On reconnection, the engine reconciles all blocks missed during the disconnection period.

### 6.6 Multi-Account Tracking

The sync engine tracks all accounts present in the current context's keyring. When a new key is added, the engine re-reconciles to pick up deployments from the new account.

---

## 7. TUI Specification

The TUI incorporates the real-time monitoring functionality of [`aktop`](https://github.com/cloud-j-luna/aktop) -- a community-built terminal UI for Akash consensus and provider monitoring. The consensus view, validator voting view, provider fleet monitor, and governance parameters view are derived from `aktop` and integrated as first-class views in the `akt` TUI.

### 7.1 Application Shell Layout

The TUI uses a three-region layout that fills the terminal:

| Region | Height | Content |
|--------|--------|---------|
| **Header** | 1 line | App name, active context, chain-id, account, block height, sync status |
| **Main area** | fills remaining | Active view (resource list or detail pane) |
| **Status bar** | 1 line | Shortcut hints, command input, help |

Header example: `akt  Context: mainnet (akashnet-2)  Account: alice  Block: 18234567  Synced`

Status bar example: `<1> Deployments  <2> Leases  <3> Providers  <:> Command  <?> Help  <q> Quit`

### 7.2 Navigation Model

Navigation uses a **stack-based** model:

- **Resource selector**: Press `:` to open command palette, or number keys for quick access to common views.
- **Drill-down**: Press `Enter` on a list item to open its detail view.
- **Back**: Press `Esc` to go back to the previous view (pops the navigation stack).
- **Breadcrumb**: The header shows the current navigation path (e.g., `Deployments > 12345 > Leases`).

Navigation stack example: `Dashboard → Deployments List → Deployment #12345 → Lease Detail` (Esc pops each level).

### 7.3 Resource Views

#### 7.3.1 Deployments List View

**Columns**: DSEQ, State, Provider (truncated address), Price/Block, Escrow Balance, Age.

**Actions**:
- `Enter` -- Open deployment detail view
- `d` -- Close deployment (with confirmation dialog)
- `u` -- Update deployment (prompts for SDL file path)
- `l` -- Open log viewer for the deployment's active lease
- `/` -- Focus the filter input (fuzzy search across all columns)
- `f` -- Cycle state filter: all -> active -> closed

**Sorting**: Click column header or press `s` then column key to sort. Default: DSEQ descending.

#### 7.3.2 Deployment Detail View

Shows deployment metadata (owner, created, deposit, escrow balance, version, SDL path, labels, notes), active lease details (provider, price, endpoints), and bid table (provider, price, state, audited).

**Actions**:
- `Esc` -- Back to deployments list
- `d` -- Close this deployment
- `u` -- Update this deployment
- `l` -- Stream logs from active lease
- `e` -- Stream events from active lease
- `s` -- Open shell into active lease
- `y` -- Toggle YAML/formatted view of raw on-chain data

#### 7.3.3 Leases List View

**Columns**: DSEQ, GSeq, OSeq, Provider, State, Price/Block, Age.

**Actions**: Enter detail, l logs, e events, s shell, w withdraw, / filter.

#### 7.3.4 Providers List View

**Columns**: Address, Host URI, Audited, Active Leases.

**Actions**: Enter detail, a attributes, / filter.

#### 7.3.5 Governance Proposals View

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt  Context: mainnet > Governance                                          │
├──────────────────────────────────────────────────────────────────────────────┤
│  Proposals                                                Filter: voting     │
│──────────────────────────────────────────────────────────────────────────────│
│  ID    TITLE                              STATUS    YES     NO      ABSTAIN  │
│  142   Upgrade to v0.36.0                 voting    72.3%   1.2%    5.1%    │
│> 141   Community pool spend: dev fund     voting    45.6%   12.3%   8.7%    │
│  140   Parameter change: min deposit      passed    89.2%   3.1%    2.4%    │
│  ...                                                                         │
├──────────────────────────────────────────────────────────────────────────────┤
│  <Enter> Detail  <v> Vote  <D> Deposit  </> Filter  <?> Help                │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### 7.3.6 Validators View

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt  Context: mainnet > Validators                                          │
├──────────────────────────────────────────────────────────────────────────────┤
│  Validators (100 active, 50 inactive)                  Filter: active        │
│──────────────────────────────────────────────────────────────────────────────│
│  RANK  MONIKER            VOTING POWER   COMMISSION  UPTIME   DELEGATED     │
│  1     Forbole            8.2%           5%          99.9%    12.5M AKT     │
│> 2     Polkachu           6.1%           5%          99.8%    9.2M AKT      │
│  3     Cosmostation       5.8%           10%         99.7%    8.7M AKT      │
│  ...                                                                         │
├──────────────────────────────────────────────────────────────────────────────┤
│  <Enter> Detail  <d> Delegate  <u> Undelegate  <r> Redelegate  </> Filter   │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### 7.3.7 Log Viewer

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt  Context: mainnet > Deployments > 12345 > Logs                          │
├──────────────────────────────────────────────────────────────────────────────┤
│  Service: web  (all services)                              Following: yes    │
│──────────────────────────────────────────────────────────────────────────────│
│  [web] 2026-03-23 10:15:32  Server starting on port 8080                     │
│  [web] 2026-03-23 10:15:33  Connected to database                           │
│  [web] 2026-03-23 10:15:34  Ready to accept connections                      │
│  [api] 2026-03-23 10:15:35  API gateway initialized                         │
│  [web] 2026-03-23 10:16:01  GET /health 200 2ms                             │
│  [web] 2026-03-23 10:16:15  GET /api/v1/users 200 45ms                      │
│  [api] 2026-03-23 10:16:15  Forwarded request to backend                     │
│  [web] 2026-03-23 10:16:30  POST /api/v1/deploy 201 120ms                   │
│  ...                                                                         │
│  ...                                                                         │
│  ...                                                                         │
│  [web] 2026-03-23 10:17:45  GET /health 200 1ms                             │
│  [web] 2026-03-23 10:18:00  Scheduled job completed                          │
├──────────────────────────────────────────────────────────────────────────────┤
│  <Esc> Back  <f> Toggle follow  <s> Service filter  </> Search  <w> Wrap    │
└──────────────────────────────────────────────────────────────────────────────┘
```

#### 7.3.8 Consensus Monitor View (from aktop)

Real-time consensus state monitoring. Polls the RPC `/consensus_state` endpoint at a configurable interval (default 1s).

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt  Context: mainnet > Consensus                                           │
├──────────────────────────────────────────────────────────────────────────────┤
│  Consensus State                                                             │
│                                                                              │
│  Height:       18,234,567              Round:     0                          │
│  Step:         Precommit               Elapsed:   1.2s                       │
│  Proposer:     akash1abc...xyz (idx 42)                                      │
│──────────────────────────────────────────────────────────────────────────────│
│  Vote Progress                                                               │
│                                                                              │
│  Prevotes:    ████████████████████████████░░░░░░░░░░░░  67.2%  (71.3M/106.1M)│
│  Precommits:  ██████████████████████████████████░░░░░░░  82.1%  (87.1M/106.1M)│
│──────────────────────────────────────────────────────────────────────────────│
│  Validator Votes (prevotes)                                                  │
│                                                                              │
│  ●●●●●●●●●●●●●○●●●●●●●●●○○●●●●●●●●●●●●●●●●●●●●●●●●○                       │
│  ●●●●●●●○●●●●●●●●●●●●●●●●●●●●●●●○●●●●●●●●●●●●●●●●●●                       │
│                                                                              │
│  ● voted (89)  ○ not voted (11)                                              │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│  <r> Refresh  <2> Validators  <3> Provider Monitor  <?> Help                │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Data sources**:
- `GET {rpc}/consensus_state` -- Round state, vote bit arrays, proposer info
- `GET {rpc}/validators?per_page=100` -- Validator set (cached per session)

**Displayed data**:
- Height (with thousand separators), Round, Step (human-readable: NewHeight, NewRound, Propose, Prevote, PrevoteWait, Precommit, PrecommitWait, Commit)
- Elapsed time since round start
- Proposer address (truncated) and index
- Prevote/Precommit progress bars: `█`/`░` (40 chars wide), percentage (green if >= 66.7%, yellow otherwise), power fraction
- Validator vote grid: `●` (voted, green) / `○` (not voted, muted), line-wrapped to terminal width

**Refresh interval**: Configurable, default 1s. Supports fast mode (250ms).

#### 7.3.9 Validator Voting View (from aktop)

Detailed validator list with real-time vote status.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt  Context: mainnet > Validators (Voting)                                 │
├──────────────────────────────────────────────────────────────────────────────┤
│  Consensus: Height 18,234,567  Round 0  Step Precommit  Elapsed 1.2s        │
│                                                                              │
│  Prevotes:    ████████████████████████████░░░░░░░░░░░░  67.2%  (71.3M/106.1M)│
│  Precommits:  ██████████████████████████████████░░░░░░░  82.1%  (87.1M/106.1M)│
│──────────────────────────────────────────────────────────────────────────────│
│     #   VALIDATOR                               POWER      PREVOTE  PRECOMMIT│
│  *  1   Forbole                                 8.7M         ✓        ✓      │
│     2   Polkachu                                6.5M         ✓        ✓      │
│>    3   Cosmostation                            5.9M         ✓        ✗      │
│     4   Figment                                 4.2M         ✗        ✗      │
│     5   Chorus One                              3.8M         ✓        ✓      │
│     ...                                                                      │
│                                                                              │
│  Showing 1-25 of 100 (j/k to scroll)                                        │
├──────────────────────────────────────────────────────────────────────────────┤
│  <1> Consensus  <j/k> Scroll  <g/G> Top/Bottom  <r> Refresh  <?> Help       │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Columns**:
- Proposer indicator: `*` (yellow bold) for current proposer
- Index: validator index in the set
- Validator: Moniker (resolved from consensus pubkey via `/cosmos/staking/v1beta1/validators`; emoji-stripped; cached in `~/.config/akt/cache/monikers.json`)
- Voting Power: formatted as K/M/B
- Prevote: `✓` (green) or `✗` (red)
- Precommit: `✓` (green) or `✗` (red)

**Actions**: j/k scroll, g/G jump to top/bottom, r refresh.

#### 7.3.10 Provider Fleet Monitor View (from aktop)

Real-time monitoring of all Akash providers -- version distribution, resource utilization, and health status.

**Provider List Sub-View**:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt  Context: mainnet > Provider Monitor                                    │
├──────────────────────────────────────────────────────────────────────────────┤
│  Scanning providers... 142/247 checked, 98 online  ████████████████░░░░ 57% │
│──────────────────────────────────────────────────────────────────────────────│
│  Provider Version Distribution                                               │
│                                                                              │
│  ► 0.6.4        ●●●●●●●●●●●●●●●●●●●●●●●○○○○○○○○○○○  62  63.3%             │
│    0.6.3        ○○○○○○○○○○○●●●●●●●●○○○○○○○○○○○○○○○  18  18.4%             │
│    0.6.2        ○○○○○○○○○○○○○○○○○●●●●●○○○○○○○○○○○○  10  10.2%             │
│    0.6.1-rc2   ○○○○○○○○○○○○○○○○○○○○●●●○○○○○○○○○○○   8   8.2%             │
│                                     h/l: select version                      │
│──────────────────────────────────────────────────────────────────────────────│
│  Providers (98 online, showing v0.6.4: 62)                                   │
│                                                                              │
│     #   URL                                VERSION      CPU       MEMORY    GPU│
│  >  1   provider1.akash.network            0.6.4     42/64     128/256Gi  4 H100│
│     2   provider2.akash.network            0.6.4     18/32      64/128Gi  -  │
│     3   provider3.example.com              0.6.4    120/256    512/1024Gi  8 A100│
│     ...                                                                      │
│  Showing 1-20 of 62 (j/k scroll, Enter detail)                              │
├──────────────────────────────────────────────────────────────────────────────┤
│  <Enter> Detail  <h/l> Version  <j/k> Scroll  <r> Re-scan  <?> Help        │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Provider Detail Sub-View** (Enter on a provider):

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt  Context: mainnet > Provider Monitor > provider1.akash.network          │
├──────────────────────────────────────────────────────────────────────────────┤
│  Provider Info                                                               │
│  Name:     Equinix Dallas                                                    │
│  URL:      provider1.akash.network                                           │
│  Version:  0.6.4                                                             │
│  Location: US                                                                │
│  CPU:      42/64 cores           Memory: 128/256 Gi                          │
│──────────────────────────────────────────────────────────────────────────────│
│  Nodes (4 nodes, 12 GPUs total)                                              │
│                                                                              │
│  NODE                 CPU          MEMORY           GPU                      │
│  node-gpu-01          8/16         32/64 Gi         2/4 NVIDIA H100 (80Gi)  │
│  node-gpu-02          8/16         32/64 Gi         2/4 NVIDIA H100 (80Gi)  │
│  node-cpu-01         12/16         32/64 Gi         -                        │
│  node-cpu-02         14/16         32/64 Gi         -                        │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│  <Esc> Back  <j/k> Scroll  <r> Refresh  <?> Help                           │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Data sources**:
- On-chain provider list: ABCI query via RPC for `akash.provider.v1beta4.Query/Providers`
- Per-provider health check: gRPC on port 8444 (preferred, includes GPU model info), REST `/status` + `/version` (fallback)
- Active leases: REST `/akash/market/v1beta5/leases/list?filters.state=active` (for priority scheduling)

**Provider cache** (stored at `~/.config/akt/cache/providers.json`):
- Smart scheduling: online providers checked every 1m, recently offline every 5m, long-term offline every 6h
- Priority queue: unchecked first, then online, then recently offline, then long-term offline
- Max 10 concurrent provider checks
- Cache saved to disk every 30s
- Chain re-sync (full provider list refresh) every 10m

**Version distribution**: Versions sorted newest-first (semver-aware, handles `-rc` suffixes). Dot visualization with `●` for selected version, `○` for others.

**Resource display**: CPU in cores (millicores/1000), Memory in Mi/Gi/Ti (binary units), GPU with model name and count.

#### 7.3.11 Governance Parameters View (from aktop)

Module-by-module governance parameter browsing.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt  Context: mainnet > Governance Params                                   │
├──────────────────────────────────────────────────────────────────────────────┤
│  Modules          │  Parameters: staking                                     │
│                   │                                                          │
│    gov            │  {                                                       │
│    mint           │    "unbonding_time": "1814400s",                         │
│  ▶ staking        │    "max_validators": 100,                               │
│    slashing       │    "max_entries": 7,                                     │
│    distribution   │    "historical_entries": 10000,                          │
│    auth           │    "bond_denom": "uakt",                                │
│    bank           │    "min_commission_rate": "0.000000000000000000"          │
│    deployment     │  }                                                       │
│    market         │                                                          │
│    transfer       │                                                          │
│    ibc            │                                                          │
│    crisis         │                                                          │
│                   │                                                          │
├──────────────────────────────────────────────────────────────────────────────┤
│  <j/k> Module  <h/l> Scroll params  <r> Refresh  <?> Help                   │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Data sources**:
- Direct module endpoints: `/cosmos/gov/v1beta1/params/voting`, `/cosmos/mint/v1beta1/params`, etc.
- Generic params subspace: `/cosmos/params/v1beta1/subspaces` + `/cosmos/params/v1beta1/params?subspace=...&key=...`

**Modules displayed**: gov, mint, staking, slashing, distribution, auth, bank, deployment, market, transfer, ibc, crisis.

**Refresh interval**: 5 minutes.

#### 7.3.12 Additional Views

The following views follow the same list/detail pattern as above:

- **Certificates**: List with Serial, State, Owner. Detail shows certificate content, expiry.
- **Escrow Accounts**: List with ID, Owner, State, Balance. Detail shows payment history.
- **Orders**: List with DSEQ, GSeq, OSeq, State. Detail shows order spec and bids.
- **Bids**: List with DSEQ, Provider, Price, State. Detail shows provider attributes.
- **Wasm Contracts**: List with Address, Code ID, Label. Detail shows contract info, state queries.
- **Oracle Prices**: Table of asset pairs with latest price, source, timestamp.
- **BME State**: Vault state, mint status, recent ledger entries.
- **IBC Channels**: List with Channel ID, Port, Counterparty, State.

### 7.4 Command Palette

Activated with `:` (colon), the command palette provides fuzzy search across:

- Resource types (deployments, leases, providers, etc.)
- Actions (create deployment, close deployment, vote on proposal, etc.)
- Context switching (use mainnet, use testnet)
- Navigation (go to deployment #12345)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  :depl                                                                       │
│──────────────────────────────────────────────────────────────────────────────│
│  > Deployments           View all deployments                                │
│    Deploy                Create new deployment                               │
│    Deployment #12345     Go to specific deployment                           │
│    Deployment Close      Close a deployment                                  │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 7.5 Confirmation Dialog

Transaction actions (close, update, delegate, vote, etc.) show a confirmation dialog:

```
┌──────────────────────────────────────────────┐
│  Close Deployment #12345?                    │
│                                              │
│  This will close all active leases and       │
│  return remaining escrow balance.             │
│                                              │
│  Owner:    akash1abc...                      │
│  Balance:  4,200,000 uakt                    │
│                                              │
│  Gas:      auto (estimated: ~200,000)        │
│  Fees:     ~5,000 uakt                       │
│                                              │
│        [ Cancel (Esc) ]  [ Confirm (Enter) ] │
└──────────────────────────────────────────────┘
```

### 7.6 Keybinding Specification

#### Default Keybindings (vim-style)

**Global**:
| Key           | Action                                  |
| ------------- | --------------------------------------- |
| `Ctrl+c`      | Quit application                        |
| `q`           | Open query commands panel               |
| `t`           | Open transaction commands panel         |
| `Ctrl+p`      | Open command search dialog              |
| `:`           | Open command palette                    |
| `?`           | Toggle help overlay                     |
| `Esc`         | Go back / close overlay / cancel        |
| `1`-`9`       | Quick-switch to numbered resource views |
| `Ctrl+r`      | Force refresh current view              |
| `Tab`         | Cycle between panes (if split view)     |

**List Navigation**:
| Key         | Action                             |
| ----------- | ---------------------------------- |
| `j`, `Down` | Move cursor down                   |
| `k`, `Up`   | Move cursor up                     |
| `g`, `Home` | Go to first item                   |
| `G`, `End`  | Go to last item                    |
| `Ctrl+d`    | Page down                          |
| `Ctrl+u`    | Page up                            |
| `Enter`     | Open detail view for selected item |
| `/`         | Open filter/search input           |
| `n`         | Next search result                 |
| `N`         | Previous search result             |
| `s`         | Open sort options                  |
| `f`         | Cycle state filter                 |

**Detail View**:
| Key         | Action                     |
| ----------- | -------------------------- |
| `j`, `Down` | Scroll down                |
| `k`, `Up`   | Scroll up                  |
| `y`         | Toggle YAML/formatted view |
| `Esc`       | Back to list               |

**Resource-Specific Actions**:
| Key          | Context                              | Action                                     |
| ------------ | ------------------------------------ | ------------------------------------------ |
| `d`          | Deployment list/detail               | Close deployment                           |
| `u`          | Deployment list/detail               | Update deployment                          |
| `l`          | Deployment/Lease                     | View logs                                  |
| `e`          | Deployment/Lease                     | View events                                |
| `s`          | Deployment/Lease                     | Open shell                                 |
| `w`          | Lease detail                         | Withdraw from escrow                       |
| `v`          | Governance proposal                  | Vote                                       |
| `D`          | Governance proposal                  | Deposit                                    |
| `d`          | Validator detail                     | Delegate                                   |
| `u`          | Validator detail                     | Undelegate                                 |
| `r`          | Validator detail                     | Redelegate                                 |
| `h`, `Left`  | Provider monitor                     | Select previous version                    |
| `l`, `Right` | Provider monitor                     | Select next version                        |
| `Enter`      | Provider monitor list                | Open provider detail (node list, GPU info) |
| `Esc`        | Provider monitor detail              | Back to provider list                      |
| `h`, `Left`  | Governance params                    | Scroll params left/up                      |
| `l`, `Right` | Governance params                    | Scroll params right/down                   |
| `r`          | Consensus/Validator/Provider monitor | Manual refresh                             |

#### Keybinding Configuration

When `tui.keybindings` is set to `custom`, the `tui.custom-keybindings` map allows remapping:

```yaml
tui:
  keybindings: custom
  custom-keybindings:
    quit: ["q", "ctrl+c"]
    command-palette: [":"]
    help: ["?", "F1"]
    back: ["esc", "backspace"]
    cursor-up: ["k", "up"]
    cursor-down: ["j", "down"]
    page-up: ["ctrl+u", "pageup"]
    page-down: ["ctrl+d", "pagedown"]
    select: ["enter"]
    search: ["/"]
    filter-cycle: ["f"]
    sort: ["s"]
    # Resource actions
    action-close: ["d"]
    action-update: ["u"]
    action-logs: ["l"]
    action-events: ["e"]
    action-shell: ["S"]
    action-vote: ["v"]
    action-delegate: ["D"]
```

### 7.7 Theme System

Themes are defined using lipgloss styles. The TUI ships with `dark` and `light` themes.

```yaml
tui:
  theme: dark  # or light, or a custom theme name
  custom-themes:
    my-theme:
      colors:
        primary: "#7B2FBE"
        secondary: "#04B575"
        accent: "#FF6B6B"
        background: "#1A1B26"
        foreground: "#C0CAF5"
        muted: "#565F89"
        border: "#3B4261"
        error: "#F7768E"
        warning: "#E0AF68"
        success: "#9ECE6A"
        info: "#7AA2F7"
      styles:
        header-bg: "#24283B"
        status-bar-bg: "#1A1B26"
        selected-row-bg: "#283457"
        cursor-row-fg: "#C0CAF5"
```

### 7.8 TUI Component Hierarchy (bubbletea models)

```
App (root model)
├── Header              (component: context info, sync status, block height)
├── Navigation          (manages view stack, breadcrumbs)
│   ├── Dashboard       (home view)
│   ├── ResourceView    (generic, parameterized by resource type)
│   │   ├── ResourceTable     (bubbles/table with sort, filter, pagination)
│   │   └── DetailPane        (bubbles/viewport with YAML/JSON toggle)
│   ├── ConsensusView   (from aktop: real-time consensus state)
│   │   ├── ConsensusHeader   (height, round, step, elapsed, proposer)
│   │   ├── VoteProgress      (prevote/precommit progress bars with power fractions)
│   │   └── VoteGrid          (● / ○ validator bit-array grid)
│   ├── ValidatorVotingView  (from aktop: validator voting status)
│   │   ├── ConsensusHeader   (compact)
│   │   ├── VoteProgress      (compact)
│   │   └── ValidatorTable    (scrollable: moniker, power, prevote/precommit status)
│   ├── ProviderMonitorView  (from aktop: provider fleet health)
│   │   ├── ScanProgress      (progress bar: X/Y checked, Z online)
│   │   ├── VersionDist       (dot visualization per version, h/l to select)
│   │   ├── ProviderTable     (scrollable: URL, version, CPU, memory, GPU)
│   │   └── ProviderDetail    (sub-view: info + node list with GPU details)
│   ├── GovernanceParamsView (from aktop: module parameter browser)
│   │   ├── ModuleList        (left panel: selectable module list)
│   │   └── ParamsPane        (right panel: scrollable pretty-printed JSON)
│   ├── LogViewer       (bubbles/viewport with auto-scroll, service filter)
│   └── ...
├── CommandPalette      (overlay: bubbles/textinput with fuzzy list)
├── ConfirmDialog       (overlay: transaction confirmation)
├── HelpOverlay         (overlay: keybinding reference, bubbles/help)
└── StatusBar           (component: shortcuts, last action, errors)
```

---

## 8. Plugin System Specification

### 8.1 Plugin Discovery

Plugins are discovered by scanning for executables matching the pattern `akt-<name>` in:

1. `~/.config/akt/plugins/` (local plugin directory)
2. Directories listed in `plugins.paths` config
3. `$PATH` directories

Discovery order determines precedence (first match wins). Plugins listed in `plugins.disabled` are skipped.

### 8.2 Plugin Execution

When `akt <name> [args...]` is invoked and `<name>` does not match a built-in command:

1. Search for a plugin named `akt-<name>`.
2. If found, execute it as a subprocess.
3. Pass all remaining arguments (`args...`) to the plugin.
4. Set `AKT_*` environment variables for context information.
5. Inherit stdin, stdout, stderr from the parent process.
6. Exit with the plugin's exit code.

### 8.3 Plugin Environment Variables

The following environment variables are set for plugin processes:

| Variable              | Description                                     |
| --------------------- | ----------------------------------------------- |
| `AKT_PLUGIN`          | Set to `1` to indicate plugin execution context |
| `AKT_HOME`            | Home directory path                             |
| `AKT_CONTEXT`         | Current context name                            |
| `AKT_CHAIN_ID`        | Current chain ID                                |
| `AKT_NODE`            | Primary RPC endpoint                            |
| `AKT_GRPC_ADDR`       | Primary gRPC endpoint                           |
| `AKT_FROM`            | Default account                                 |
| `AKT_KEYRING_BACKEND` | Keyring backend                                 |
| `AKT_KEYRING_DIR`     | Keyring directory                               |
| `AKT_OUTPUT`          | Output format                                   |
| `AKT_STORE_PATH`      | Store database path                             |

### 8.4 Plugin Manifest (Optional)

A `plugin.yaml` file placed next to the plugin binary provides metadata:

```yaml
name: sdl-lint
version: 1.0.0
description: Lint and validate Akash SDL files
usage: "akt sdl-lint <sdl-file> [--strict]"
short-description: Lint SDL files
long-description: |
  Validates Akash SDL files for correctness, best practices,
  and common mistakes. Reports warnings and errors with
  line numbers and suggestions.
requires:
  - context        # plugin needs an active context
  - keyring        # plugin needs keyring access
min-akt-version: "0.1.0"
```

### 8.5 Plugin Management Commands

#### `akt plugin list`

```
$ akt plugin list
NAME          VERSION  SOURCE                             PATH
sdl-lint      1.0.0    github.com/user/akt-sdl-lint       ~/.config/akt/plugins/akt-sdl-lint
deploy-ci     0.3.2    (manual)                           /usr/local/bin/akt-deploy-ci
```

#### `akt plugin install <source>`

Install a plugin from a Git repository URL or local path.

```bash
# From GitHub (downloads latest release binary)
akt plugin install github.com/user/akt-sdl-lint

# From local path (creates symlink)
akt plugin install --local /path/to/akt-my-plugin
```

#### `akt plugin remove <name>`

Remove an installed plugin.

```bash
akt plugin remove sdl-lint
```

### 8.6 Security Considerations

- Plugins are unsigned executables. Users are responsible for trusting plugin sources.
- The `akt plugin install` command warns about running untrusted code.
- Plugins cannot modify the `akt` config or store directly (they only receive environment variables).
- A future version may add plugin signing and a plugin registry.

---

## 9. Output Format Specification

### 9.1 JSON

Machine-readable format. Used when `--output json` or when stdout is not a TTY (piped).

- Pretty-printed with 2-space indentation.
- All field names use `snake_case`.
- Protobuf messages are serialized using Cosmos SDK's JSON codec (respects `@type` annotations).
- Lists are wrapped in a top-level object with a `data` array and optional `pagination` object.

```json
{
  "data": [
    {
      "dseq": 12345,
      "owner": "akash1abc...",
      "state": "active",
      "version": "abc123..."
    }
  ],
  "pagination": {
    "next_key": "...",
    "total": "47"
  }
}
```

### 9.2 YAML

Human-readable structured format. Used when `--output yaml`.

- Standard YAML marshaling with Cosmos SDK types.
- Emitted as a single document with a leading `---`.
- Same field naming as JSON (`snake_case`).

```yaml
---
data:
  - dseq: 12345
    owner: akash1abc...
    state: active
    version: abc123...
pagination:
  next_key: "..."
  total: "47"
```

### 9.3 Table

Human-friendly tabular format. Default when stdout is a TTY.

- Column-aligned headers in bold (when TTY supports it).
- Long values truncated with `...` to fit terminal width.
- Address fields show first 10 and last 3 characters with `...` in between.
- Colors used for state indicators: green for active/passed, red for closed/failed, yellow for pending.
- Colors disabled automatically when not a TTY or when `NO_COLOR` env is set.

```
DSEQ       STATE     PROVIDER            PRICE/BLK   BALANCE     AGE
12345      active    akash1prov1...xyz    12.5 uakt   4.2 AKT    3d
12344      active    akash1prov2...abc    15.0 uakt   2.1 AKT    5d
```

---

## 10. Error Handling

### 10.1 Error Types

All errors are structured with context:

```go
type CLIError struct {
    Code       int      // exit code
    Message    string   // user-facing message
    Cause      error    // underlying error
    Suggestion string   // actionable suggestion
    Context    string   // what was being attempted
}
```

### 10.2 Exit Codes

| Code  | Meaning                                                               |
| ----- | --------------------------------------------------------------------- |
| `0`   | Success                                                               |
| `1`   | General error                                                         |
| `2`   | Usage error (invalid flags, missing arguments)                        |
| `3`   | Configuration error (invalid config, missing context)                 |
| `4`   | Connection error (cannot reach RPC/gRPC endpoint)                     |
| `5`   | Transaction error (broadcast failure, out of gas, insufficient funds) |
| `6`   | Authentication error (keyring access failure, signing error)          |
| `7`   | Store error (database corruption, migration failure)                  |
| `127` | Plugin not found                                                      |

### 10.3 User-Facing Error Messages

Errors presented to users include:

1. **What happened**: A clear description of the failure.
2. **Context**: What operation was being performed.
3. **Suggestion**: An actionable next step.

```
Error: cannot connect to RPC endpoint

  Context:  querying deployment list
  Endpoint: https://rpc.akashnet.net:443
  Cause:    connection refused

  Suggestion: Check your network connection, or try a different endpoint:
    akt context edit mainnet --rpc https://rpc-akash.ecostake.com:443
```

---

## 11. Phased Implementation Plan

### Phase 1: Foundation

**Duration**: ~6-8 weeks

**Goal**: A functional CLI that can replace basic `akash tx` and `akash query` operations.

#### Deliverables

| #    | Deliverable           | Description                                                                                                                                                                                 | Acceptance Criteria                                                                                     |
| ---- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| 1.1  | Project scaffold      | Go module, Makefile, goreleaser, CI                                                                                                                                                         | `go build ./...` succeeds, `akt version` works                                                          |
| 1.2  | Config system         | YAML config read/write, XDG paths, env var loading, fsnotify watcher                                                                                                                        | Config round-trips correctly, env vars override config, live-reload works                               |
| 1.3  | Network management    | Shared network definitions, CRUD, templates (mainnet/testnet/sandbox)                                                                                                                       | `akt context network *` commands work, templates provision correctly, sharing across contexts works     |
| 1.4  | Context manager       | Context CRUD, switching, composition (network + keyring + store + log), fork/edit-parent for networks                                                                                       | All `akt context *` commands work, fork/edit-parent works, context propagation works                    |
| 1.5  | Context live-reload   | Config file watching, propagation of changes to running session                                                                                                                             | Config changes reflected in session; TUI overrides flags/env; CLI respects override chain               |
| 1.6  | Keyring integration   | Shared multi-keyring support, keys visible to all contexts using the keyring                                                                                                                | Keys can be created, listed, and used for signing; adding key to shared keyring visible in all contexts |
| 1.7  | Action log            | Append-only JSONL action log per context, log reading/filtering                                                                                                                             | All tx/query actions logged, `akt context log` shows entries, log rotation works                        |
| 1.8  | Chain client          | Full and light client with multi-endpoint failover                                                                                                                                          | Successful tx broadcast and query with automatic failover when primary endpoint is down                 |
| 1.9  | Transaction commands  | All `tx` module commands (bank, deployment, market, provider, cert, audit, staking, distribution, gov, authz, feegrant, escrow, wasm, oracle, bme, slashing, vesting, upgrade, crisis, IBC) | Each command matches the behavioral output of the current `akash` binary                                |
| 1.10 | Query commands        | All `query` module commands                                                                                                                                                                 | Each command matches the behavioral output of the current `akash` binary                                |
| 1.11 | Key commands          | All `keys` subcommands                                                                                                                                                                      | Full key lifecycle works (create, export, import, delete, show, list)                                   |
| 1.12 | Auth utility commands | sign, sign-batch, multisign, validate-signatures, broadcast, encode, decode                                                                                                                 | Offline signing workflow works end-to-end                                                               |
| 1.13 | Output formatting     | JSON, YAML, and Table formatters                                                                                                                                                            | All three formats produce correct output for all commands                                               |
| 1.14 | Global flags + env    | All global flags, env var mapping, override chain                                                                                                                                           | Override chain works: flag > env > config > default                                                     |
| 1.15 | Shell completion      | bash, zsh, fish completion scripts                                                                                                                                                          | Tab completion works for commands, flags, and context/network names                                     |
| 1.16 | E2E test suite        | Core test coverage for context, network, tx, query                                                                                                                                          | Tests pass in CI against a local testnet                                                                |

### Phase 2: Store + Workflow Commands

**Duration**: ~4-6 weeks

**Goal**: Local state tracking and high-level workflow commands.

#### Deliverables

| #    | Deliverable             | Description                                                                                                                    | Acceptance Criteria                                                       |
| ---- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| 2.1  | Store interface         | Go interface definition + bbolt implementation                                                                                 | All CRUD operations work, concurrent access is safe                       |
| 2.2  | Schema migrations       | Versioned schema with forward migration                                                                                        | Migration from v1 to vN applies correctly                                 |
| 2.3  | Sync engine             | WebSocket subscription, event routing, reconciliation                                                                          | Store updates within 2 seconds of on-chain state change                   |
| 2.4  | Startup reconciliation  | Full reconciliation on first launch, incremental on subsequent                                                                 | All user deployments appear in store after first sync                     |
| 2.5  | `akt deploy` workflow   | Full lifecycle: create -> bids -> select -> lease -> manifest -> active                                                        | Interactive and non-interactive modes both complete successfully          |
| 2.6  | Provider gateway client | Auth (JWT/mTLS), status, lease-status, logs, events, shell                                                                     | All provider commands work against a running provider                     |
| 2.7  | Provider commands       | status, lease-status, lease-logs, lease-events, lease-shell, send-manifest, get-manifest, migrate-hostnames, migrate-endpoints | Each command matches the behavioral output of current `provider-services` |
| 2.8  | `akt sdl-to-manifest`   | SDL to manifest conversion                                                                                                     | Output matches current `provider-services sdl-to-manifest`                |
| 2.9  | Store export/import     | YAML and JSON export, import with merge/replace                                                                                | Round-trip: export then import produces identical store state             |
| 2.10 | Store status command    | Display store info, sync state, record counts                                                                                  | Accurate reporting of store contents                                      |
| 2.11 | Events command          | Live blockchain event streaming                                                                                                | `akt events` shows real-time events                                       |

### Phase 3: TUI Mode

**Duration**: ~6-8 weeks

**Goal**: A fully interactive terminal UI for real-time Akash management.

#### Deliverables

| #    | Deliverable                         | Description                                                                                                                               | Acceptance Criteria                                                                               |
| ---- | ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| 3.1  | Application shell                   | Header, main area, status bar using bubbletea + lipgloss                                                                                  | Shell renders correctly, resizes gracefully                                                       |
| 3.2  | Navigation system                   | Stack-based navigation, breadcrumbs, back/forward                                                                                         | User can navigate between views without state loss                                                |
| 3.3  | Resource table component            | Generic sortable, filterable table with pagination                                                                                        | Sort, filter, and pagination work for all resource types                                          |
| 3.4  | Detail pane component               | Scrollable viewport with YAML/JSON toggle                                                                                                 | Detail view renders correctly for all resource types                                              |
| 3.5  | Deployments view                    | List + detail views with actions                                                                                                          | User can browse, inspect, and act on deployments                                                  |
| 3.6  | Leases view                         | List + detail views with actions                                                                                                          | User can browse and manage leases                                                                 |
| 3.7  | Providers view                      | List + detail views                                                                                                                       | User can browse providers and their attributes                                                    |
| 3.8  | Log viewer                          | Streaming viewport with service filter and search                                                                                         | Logs stream in real-time, search works                                                            |
| 3.9  | Consensus monitor (from aktop)      | Real-time consensus state: height/round/step, vote progress bars, validator vote grid                                                     | Consensus state updates at configurable interval, vote percentages match chain state              |
| 3.10 | Validator voting view (from aktop)  | Scrollable validator list with moniker resolution, prevote/precommit status, proposer indicator                                           | All validators shown with correct vote status, monikers resolved and cached                       |
| 3.11 | Provider fleet monitor (from aktop) | Version distribution visualization, provider health scanning with priority scheduling, per-provider detail with node-level CPU/memory/GPU | Providers scanned with smart scheduling, version distribution accurate, GPU models shown via gRPC |
| 3.12 | Provider cache                      | Smart-scheduled provider cache with disk persistence                                                                                      | Cache persists across sessions, scheduling intervals respected (1m/5m/6h)                         |
| 3.13 | Governance params view (from aktop) | Module-by-module parameter browser with pretty-printed JSON                                                                               | All 12 modules' params displayed correctly                                                        |
| 3.14 | Command palette                     | Fuzzy search across resources and actions                                                                                                 | User can quickly navigate to any resource or action                                               |
| 3.15 | Confirmation dialog                 | Transaction confirmation with fee preview                                                                                                 | All destructive actions require confirmation                                                      |
| 3.16 | Help overlay                        | Keybinding reference panel                                                                                                                | Help shows all available actions for current view                                                 |
| 3.17 | Live sync integration               | Store updates trigger TUI re-renders                                                                                                      | View updates within 2 seconds of chain state change                                               |
| 3.18 | Configurable keybindings            | Config-driven key mapping                                                                                                                 | Custom keybindings work correctly                                                                 |
| 3.19 | Theme system                        | Dark/light themes, custom color config                                                                                                    | Both built-in themes render correctly                                                             |

### Phase 4: Extended Features

**Duration**: ~4-6 weeks

**Goal**: Complete feature set, extensibility, and polish.

#### Deliverables

| #    | Deliverable              | Description                                        | Acceptance Criteria                                |
| ---- | ------------------------ | -------------------------------------------------- | -------------------------------------------------- |
| 4.1  | Plugin discovery         | Scan PATH and plugin dir for `akt-*` binaries      | `akt plugin list` shows discovered plugins         |
| 4.2  | Plugin execution         | Fork/exec with AKT_* env vars                      | Plugin receives correct context, stdin/stdout work |
| 4.3  | Plugin management        | install, list, remove commands                     | Full plugin lifecycle works                        |
| 4.4  | Plugin manifest          | Optional plugin.yaml parsing                       | Manifest info shown in `akt plugin list` and help  |
| 4.5  | Certificates view (TUI)  | List and detail for certificates                   | Certificate management in TUI                      |
| 4.6  | Governance view (TUI)    | Proposals with voting actions                      | User can browse proposals and vote from TUI        |
| 4.7  | Validators view (TUI)    | Validator list with delegation actions             | User can delegate/undelegate from TUI              |
| 4.8  | Escrow view (TUI)        | Escrow account list and detail                     | Escrow state visible in TUI                        |
| 4.9  | Wasm view (TUI)          | Contract list, info, state queries                 | Wasm contract browsing in TUI                      |
| 4.10 | Oracle view (TUI)        | Price table with history                           | Oracle prices visible in TUI                       |
| 4.11 | BME view (TUI)           | Vault state, status, ledger                        | BME state visible in TUI                           |
| 4.12 | IBC view (TUI)           | Channel list with state                            | IBC channels visible in TUI                        |
| 4.13 | TUI transaction actions  | Create deployment, fund escrow, etc. from TUI      | Full transaction workflow in TUI                   |
| 4.14 | Performance optimization | Lazy loading, virtual scrolling                    | Lists with >10,000 items render smoothly           |
| 4.15 | Comprehensive E2E tests  | Full coverage of all commands and TUI interactions | CI passes with >80% coverage                       |
