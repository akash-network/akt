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
7. [Console API Specification](#7-console-api-specification)
8. [TUI Specification](#8-tui-specification)
9. [Plugin System Specification](#9-plugin-system-specification)
10. [Output Format Specification](#10-output-format-specification)
11. [Error Handling](#11-error-handling)
12. [Phased Implementation Plan](#12-phased-implementation-plan)

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
    auth-method: keyring                # keyring (default) or console-api
    keyring: default                    # references a keyring definition
    default-account: "alice"            # account name or address; empty = prompt
    tracked-accounts: []                # accounts to sync; empty = [default-account]; ["*"] = all keyring accounts
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

  - name: console                      # Console API context (managed wallet)
    network: mainnet                    # network used for query commands
    auth-method: console-api            # use Console managed wallet instead of keyring
    console-api-url: ""                 # empty = default (https://console-api.akash.network)
    # keyring, default-account, and provider-defaults are not used with console-api auth
    # API key: --console-api-key flag > AKT_CONSOLE_API_KEY env var > per-context
    # credential file (contexts/<name>/console-api-key, see §7.1); never stored here

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
  output: pretty                        # pretty | json | yaml
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

**Endpoint port inference:** Ports are optional in RPC and API endpoint URLs. When a port is not explicitly specified, it is inferred from the URL scheme: `http` → 80, `https` → 443 (likewise `ws` → 80, `wss` → 443). The underlying cosmos-sdk and CometBFT libraries require an explicit port in the host string, so `akt` normalizes endpoints at startup by appending the scheme-default port when one is absent. For example, `https://rpc.akashnet.net` is equivalent to `https://rpc.akashnet.net:443`. The `tcp` scheme (used internally by the cosmos-sdk `--node` flag) defaults to port 80 since it is treated as an alias for `http`.

**Network sharing rules:**
- Multiple contexts can reference the same network by name.
- Editing a network (e.g., changing an RPC endpoint) affects all contexts that reference it.
- When editing a network from within a context, the user is prompted:
  - **Edit parent**: Modify the shared network definition. All contexts using it see the change.
  - **Fork**: Create a copy of the network with a new name (e.g., `mainnet` -> `mainnet-<context>`). The current context switches to the fork. Other contexts are unaffected.

**Network templates**: Built-in templates for `mainnet`, `testnet`, and `sandbox` are available via `akt context network create --template <name>`. Templates populate all fields with known-good defaults.

### 1.4 Context Schema

Contexts compose a network, keyring, and context-specific settings. The state store and action log are implicitly created at `<config-root>/contexts/<name>/`.

| Field                         | Type   | Required | Default                                | Description                                                                  |
| ----------------------------- | ------ | -------- | -------------------------------------- | ---------------------------------------------------------------------------- |
| `name`                        | string | yes      | --                                     | Unique context identifier                                                    |
| `network`                     | string | see note | --                                     | Name of network definition to use. Required for `keyring` auth; optional for `console-api` auth (a network-less context operates through the Console API alone and chain commands are capability-gated until a network is attached, §2.10). |
| `auth-method`                 | string | no       | `"keyring"`                            | Authentication method: `keyring` or `console-api`                            |
| `console-api-url`             | string | no       | `"https://console-api.akash.network"` | Console API base URL (only with `console-api` auth)                          |
| `keyring`                     | string | no       | `"default"`                            | Keyring name for signing keys (only with `keyring` auth)                     |
| `default-account`             | string | no       | `""`                                   | Default `--from` value (only with `keyring` auth)                            |
| `tracked-accounts`            | []string | no     | `[]`                                   | Accounts to sync (empty = `[default-account]`; `["*"]` = all keyring accounts). See §6.7. |
| `gas`                         | string | no       | `"auto"`                               | Gas limit or `"auto"` (only with `keyring` auth)                             |
| `fees`                        | string | no       | `""`                                   | Fixed fees (only with `keyring` auth)                                        |
| `provider-defaults.auth-type` | string | no       | `"jwt"`                                | Provider gateway auth: `jwt` or `mtls` (only with `keyring` auth)            |

**Console API key**: the per-context Console API key is deliberately **not** a config.yaml field. It is stored as a separate credential file at `<config-root>/contexts/<name>/console-api-key` with `0600` permissions and is managed via `akt context create/edit --console-api-key`. See §7.1 for the full resolution order and handling rules.

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
| `AKT_CONSOLE_API_KEY` | Console API key (overrides the per-context stored credential; see §7.1) | `akt_abc123...`                |

### 1.10 Built-in Network Templates

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

### 1.11 Terminal Requirements and Glyph Registry

`akt` uses ASCII-safe glyphs exclusively. There is no Nerd Font mode and no glyph-mode flag. Standard Unicode characters (block drawing `█░▀`, arrows `←↑→↓`, box drawing `─`, circles `●`) are used freely since they render correctly in virtually all terminal fonts. Nerd Font PUA-range glyphs are never emitted.

**Glyph registry**: All glyphs are defined in a centralized registry (`internal/glyphs/`). Rendering code references glyphs via the registry using semantic names, never as inline string literals.

**Glyph mapping**:

| Semantic Name | Glyph | Usage |
|---|---|---|
| `CheckboxOn` | `[x]` | Multiselect checked item |
| `CheckboxOff` | `[ ]` | Multiselect unchecked item |
| `Cursor` | `>` | Row selection indicator |
| `SelectAll` | `#` | Select-all icon |
| `VoteYes` | `+` | Vote grid / prevote confirmed |
| `VoteNo` | `-` | Vote grid / prevote missing |
| `Star` | `*` | Block proposer indicator |
| `DotFilled` | `*` | Selected version dot |
| `DotOpen` | `o` | Unselected version dot |

---

## 2. CLI Command Reference

Implementation note: `tx` and `query` commands are clean-copied from `akash-network/chain-sdk/go/cli` into `internal/cli/chain`. Only CLI code is copied; all other chain-sdk packages are imported directly. Command flags default to the resolved akt context values unless explicitly overridden.

**Help text requirement**: Every command and subcommand must populate cobra's `Example` field with at least one usage example. The example should demonstrate the most common use case with realistic argument values. Commands with multiple modes of operation (e.g., list vs get, interactive vs scripted) should include one example per mode. This ensures that `akt <command> --help` is self-contained -- users should never need to consult external documentation for basic usage.

### 2.0 Root Command Behavior (`akt` with no subcommand)

When `akt` is invoked with no subcommand, the following flow determines what happens:

1. **No config exists** (first run): The bootstrap wizard runs (§1.11, `internal/bootstrap/wizard.go`). It prompts the user to select networks, select a keyring backend (`os`, `file`, or `test`; default: `os`), and configure an initial context. It then offers optional Akash Console onboarding: the user may enter a Console API key (validated best-effort against `/v1/user/me`, stored as the initial context's per-context credential per §7.1) and choose whether deployments for that context should be routed through Console (`auth-method: console-api`). Both prompts default to "no" and are skipped entirely in non-interactive runs. The wizard runs only when stdin is a terminal: in headless environments it declines to bootstrap (no network fetch, no config written) and prints guidance to create a config via `akt context network create` / `akt context create`; the root command then continues to step 2 without a config. After bootstrap completes, the root command continues to step 2.
> **TUI shell status (2026-07): DISABLED pending UX feedback.** Bare `akt` prints the help text and `--interactive`/`-i` reports that the TUI is disabled. The launch path remains compiled behind `AKT_EXPERIMENTAL_TUI=1` for feedback sessions, and `akt monitor` (§2.6) is unaffected. Steps 2–5 below describe the behavior that resumes when the TUI is re-enabled.

2. **Config exists, `defaults.interactive` is `true` (default), and a TTY is attached**: The interactive TUI application launches (§8).
3. **Config exists, `defaults.interactive` is `false`**: Print the help text (equivalent to `akt --help`). The user has opted out of TUI mode and must use explicit subcommands.
4. **Config exists, no TTY attached** (e.g., `akt | cat`): Print the help text. The TUI requires a terminal.
5. **`--interactive` / `-i` flag is set**: Force TUI launch regardless of `defaults.interactive` setting. Still requires a TTY — if no TTY is attached with `-i`, print an error.

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
│   │   ├── update <sdl-file> [dseq]
│   │   ├── close [dseq]
│   │   └── group
│   │       ├── close [dseq] [gseq]
│   │       ├── pause [dseq] [gseq]
│   │       └── start [dseq] [gseq]
│   ├── market
│   │   ├── bid
│   │   │   ├── create
│   │   │   └── close
│   │   └── lease
│   │       ├── create [dseq] [provider]
│   │       ├── withdraw [dseq] [provider]
│   │       └── close [dseq] [provider]
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
├── query (alias: q)                     # Query commands (see §3.8 for filter syntax)
│   ├── bank
│   │   ├── balances <address>
│   │   ├── spendable-balances <address>
│   │   ├── total
│   │   ├── denom-metadata
│   │   └── send-enabled [denom1...]
│   ├── deployment [filter] [state]      # [owner/]dseq [state]; no arg → list; --state flag — **disabled pending feedback** (positional only, 2026-07)
│   │   ├── group [filter]               # [owner/]dseq[/gseq]
│   │   └── params
│   ├── market
│   │   ├── order [filter] [state]       # [owner/]dseq[/gseq/oseq] [state]; --state flag — **disabled pending feedback** (positional only, 2026-07)
│   │   ├── bid [filter] [state]         # [owner/]dseq[/gseq/oseq[/prov]] [state]; --by; --state — **disabled pending feedback** (positional only, 2026-07)
│   │   ├── lease [filter] [state]       # [owner/]dseq[/gseq/oseq[/prov]] [state]; --by; --state — **disabled pending feedback** (positional only, 2026-07)
│   │   └── params
│   ├── provider [address]               # List or get (address → single)
│   ├── cert [owner] [state]             # Owner or default account, plus [state]; --owner/--state flags — **disabled pending feedback** (positional only, 2026-07)
│   ├── audit [owner]                    # Owner or default account; --auditor flag
│   ├── escrow [filter]                  # [owner[/dseq]]; --state flag
│   │   ├── payment [filter]             # [owner[/dseq]]
│   │   └── blocks-remaining [filter]    # [owner/]dseq
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
│   ├── ibc                              # IBC core (passthrough from cosmos-sdk, subcommands not expanded)
│   ├── ibc-transfer                     # IBC transfer (passthrough from cosmos-sdk, subcommands not expanded)
│   ├── upgrade
│   ├── block [height]
│   ├── blocks [query]                   # CometBFT query expr; --query override — **disabled pending feedback** (positional only, 2026-07)
│   ├── block-results [height]
│   ├── tx <hash>
│   ├── txs [events]                     # event list expr; --events override — **disabled pending feedback** (positional only, 2026-07)
│   └── module-name-to-address <module>
├── deploy <sdl-file>                    # Workflow: full deployment lifecycle
├── update <sdl-file> [dseq]             # Workflow: update deployment + send manifest
├── close [dseq]                         # Workflow: close deployment
├── console                              # Akash Console managed-wallet API (§2.9)
│   ├── login [key]                      # Validate + store per-context API key credential
│   ├── logout                           # Remove stored credential
│   ├── whoami                           # Authenticated user info
│   ├── deployment                       # list | get | create | update | close | deposit | settings
│   ├── bid list <dseq>                  # Bids for a deployment's open orders
│   ├── lease create <dseq> [provider]   # Accept a bid (uses cached manifest)
│   ├── wallet                           # list | balance | settings | cost
│   ├── usage [from] [to]                # Daily spend history
│   ├── provider                         # list | get | regions | auditors (public, no key)
│   ├── gpu                              # GPU availability/pricing (public, no key)
│   ├── template                         # list | get | sdl (public, no key)
│   ├── apikey                           # list | create | delete
│   └── jwt create                       # Mint provider-scoped JWT
├── provider                             # Provider gateway commands
│   ├── status [provider-addr]
│   ├── lease-status [dseq]
│   ├── lease-logs [dseq]
│   ├── lease-events [dseq]
│   ├── lease-shell
│   ├── send-manifest <sdl-file>
│   ├── get-manifest [dseq]
│   ├── migrate-hostnames
│   └── migrate-endpoints
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
├── monitor [rpc-endpoint]               # Real-time monitoring hub (network/provider/oracle-bme dashboards)
│   ├── network [rpc-endpoint]           # Network: consensus, validators, governance
│   ├── provider [rpc-endpoint]          # Provider fleet: versions, health, resources
│   ├── oracle [rpc-endpoint]            # Oracle/BME dashboard (alias)
│   └── bme [rpc-endpoint]              # Oracle/BME dashboard (alias)
├── mcp                                  # MCP server for AI assistant integration (stdio)
├── version                              # Version information
└── completion                           # Shell completion scripts
```

### 2.2 Context Commands

#### `akt context create <name>`

Create a new named context. A context references a network and keyring by name.

| Flag                  | Type   | Default     | Description                                                      |
| --------------------- | ------ | ----------- | ---------------------------------------------------------------- |
| `--network`           | string | `""`        | Network name to use (required; must exist)                       |
| `--auth-method`       | string | `"keyring"` | Authentication method: `keyring` or `console-api`                |
| `--console-api-url`   | string | `""`        | Console API base URL (empty = default; only with `console-api`)  |
| `--console-api-key`   | string | `""`        | Console API key stored as a per-context credential (§7.1; never written to config.yaml) |
| `--keyring`           | string | `"default"` | Keyring name (only with `keyring` auth)                          |
| `--default-account`   | string | `""`        | Default account name (only with `keyring` auth)                  |
| `--gas`               | string | `"auto"`    | Gas limit override (only with `keyring` auth)                    |
| `--fees`              | string | `""`        | Fixed fees override (only with `keyring` auth)                   |
| `--set-current`       | bool   | `false`     | Set as current context after creation                            |

**Examples:**
```bash
# Create context using an existing network (keyring auth, default)
akt context create prod --network mainnet --default-account alice --set-current

# Create context for monitoring (no default account)
akt context create monitoring --network mainnet

# Create context for testnet
akt context create staging --network testnet --keyring test-keyring --default-account testaccount

# Create context using Console API managed wallet
# Store the API key as the context credential with --console-api-key,
# or later via `akt console login` (see §7.1)
akt context create console --network mainnet --auth-method console-api --set-current
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
| `--auth-method`     | string | `""`    | Change authentication method: `keyring` or `console-api` |
| `--console-api-url` | string | `""`    | Change Console API base URL (empty = default) |
| `--console-api-key` | string | `""`    | Set the per-context Console API key (empty string removes it; §7.1) |
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

| Flag        | Type   | Default   | Description                                                         |
| ----------- | ------ | --------- | ------------------------------------------------------------------- |
| `--context` | string | current   | Context to view log for                                             |
| `--limit`   | int    | `50`      | Number of entries to show                                           |
| `--type`    | string | `""`      | Filter by action type: `tx`, `workflow`, `provider`, `context`, `console`, `error` (see §5.6) |
| `--since`   | string | `""`      | Show entries since timestamp or duration (e.g., `1h`, `2024-01-01`) |
| `--output`  | string | `pretty`  | Output format: `pretty` (table), `json` (raw JSONL entries, one per line) |

```bash
$ akt context log --limit 5
  TIME                    TYPE      SUMMARY                                    STATUS
  2026-03-23 10:15:32     tx        deployment create (dseq: 12345)            success
  2026-03-23 10:15:45     tx        market lease create (dseq: 12345)          success
  2026-03-23 10:15:50     workflow  send-manifest -> akash1prov1...            success
  2026-03-23 10:20:01     context   edit (default-account: bob)                success
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

**Transports**: actions are defined once — as workflow definitions — and translated per transport by `internal/transport`. Each transport carries the same abstract steps onto its backing rail: the **chain** transport (keyring auth) signs and broadcasts transactions locally plus provider-gateway calls, while the **console** transport (console-api auth) maps the same steps onto Console API REST calls (§7.4–§7.5). Because the command surface (positionals and flags) is generated from the workflow definition and the transport is chosen per context at execution time, `akt deploy/update/close` accept identical arguments on both rails, and adding a new action never requires per-rail redesign. Cross-rail argument syntax (notably `--deposit`, §7.4) is normalized in the transport layer.

#### 2.3.1 Workflow Definition Location

Workflow definitions are resolved in order:
1. **Per-context**: `<home>/contexts/<ctx>/workflows/<name>.yaml`
2. **Global user**: `<home>/workflows/<name>.yaml`
3. **Embedded built-in**: compiled into the binary

Top-level commands (`akt deploy`, `akt update`, `akt close`) are thin wrappers that load and run the corresponding workflow. CLI flags are **auto-generated** from the workflow's declared `params` section.

**Dynamic surfacing**: Workflow commands are generated at CLI startup from the set of workflows returned by the loader. A top-level command exists **if and only if** a workflow with that name resolves. The embedded built-ins (`deploy`, `update`, `close`) always resolve and therefore always appear; user-defined workflows in the global or per-context directories add further top-level commands, and removing a user workflow removes its command. Help output reflects only the resolved set.

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
    default: "auto"
    description: "Initial deposit: 5usd or $5 (USD, console-api contexts), 5000000uakt (coin, keyring contexts), auto = chain minimum (keyring)"
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
| `foreach`  | Iterate over a query result, executing a sub-step for each item | `query`, `params`, `as`, `step`, `on-error`  |

**`foreach` step detail:**

The `foreach` step queries a data source and executes a nested `step` definition for each item in the result. The current item is available as `.Item` in templates within the nested step.

```yaml
- name: send-manifests
  type: foreach
  query: market.leases
  params:
    owner: "{{ .Account }}"
    dseq: "{{ .Params.dseq }}"
    state: active
  as: lease
  step:
    type: provider
    action: send-manifest
    params:
      provider: "{{ .Item.lease.id.provider }}"
      dseq: "{{ .Params.dseq }}"
      sdl: "{{ index .Params \"sdl-file\" }}"
    retry:
      max: 3
      delay: "5s"
  on-error: continue
```

The `as` field names the iteration variable (available as `.Item`). `on-error` at the `foreach` level controls behavior when any individual iteration fails: `continue` proceeds to the next item, `abort` stops the entire workflow.

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

#### 2.3.6 Error Recovery and Partial State

When a workflow aborts due to a step failure, the user may be left with partial on-chain state (e.g., a deployment was created but no lease was established, consuming escrow). The workflow engine handles this as follows:

1. **Abort message includes partial state summary**: When a workflow aborts, the error output lists all successfully completed steps and their results (e.g., "Deployment created with DSEQ 12345"). This gives the user the information needed to clean up manually.

2. **Recovery suggestions**: The abort message includes actionable suggestions based on which step failed:
   - Failed after `create-deployment`: Suggest `akt close <dseq>` to close the orphaned deployment and reclaim the escrow deposit.
   - Failed after `create-lease` but before `send-manifest`: Suggest `akt provider send-manifest <sdl-file> --dseq <dseq>` to retry manifest submission.
   - Failed during `send-manifest`: Suggest retrying with `akt provider send-manifest` directly.

3. **JSONL mode**: In JSONL mode, the error line includes a `"recovery"` field with the suggested command:
   ```jsonl
   {"workflow":"deploy","id":"wf_abc123","step":"send-manifest","result":"error","errors":["provider gateway timeout"],"txs":[],"recovery":"akt provider send-manifest deploy.yaml --dseq 12345"}
   ```

4. **No automatic rollback**: The workflow engine does not automatically roll back completed steps. On-chain transactions are irreversible. The user must explicitly close deployments or leases they no longer want.

5. **Future: `--resume` flag**: A future enhancement may add a `--resume <workflow-id>` flag that re-runs a workflow from the last failed step, using the stored outputs from completed steps. This is not part of the initial implementation.

#### 2.3.7 Param Types

| Type       | Description                       | Flag type      |
|------------|-----------------------------------|----------------|
| `string`   | Plain string                      | `--name value` |
| `int`      | Integer                           | `--name 5`     |
| `bool`     | Boolean                           | `--name`       |
| `duration` | Go duration string                | `--name 5m`    |
| `file`     | File path (positional if first)   | positional arg |

#### 2.3.8 Execution Modes

Workflows support two execution modes:

**TUI mode** (default when TTY is attached):
- Interactive bubbletea UI with progress display, spinners, and colored status output.
- `prompt` steps render interactive selection tables (e.g., bid selection).
- `output` steps render formatted text.
- Errors are displayed inline with the step that failed.

**JSONL mode** (`--output jsonl`, or auto-selected when no TTY is attached):
- Emits one JSONL line to stdout per completed workflow step.
- No interactive prompts -- `prompt` steps must be pre-resolved via flags (e.g., `--bid-select cheapest`). If an unresolvable prompt step is reached, the workflow aborts with an error line.
- Each line is a self-contained JSON object:

```jsonl
{"workflow":"deploy","id":"wf_abc123","step":"create-deployment","result":"completed","errors":[],"txs":[{"hash":"ABCD1234...","height":12345,"gas_used":150000,"code":0}]}
{"workflow":"deploy","id":"wf_abc123","step":"wait-for-bids","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_abc123","step":"select-bid","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_abc123","step":"create-lease","result":"completed","errors":[],"txs":[{"hash":"EFGH5678...","height":12350,"gas_used":120000,"code":0}]}
{"workflow":"deploy","id":"wf_abc123","step":"send-manifest","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_abc123","step":"display-result","result":"completed","errors":[],"txs":[]}
```

On error:
```jsonl
{"workflow":"deploy","id":"wf_abc123","step":"send-manifest","result":"error","errors":["provider gateway timeout after 3 retries"],"txs":[]}
```

**JSONL line schema:**

| Field      | Type     | Description                                                     |
| ---------- | -------- | --------------------------------------------------------------- |
| `workflow` | string   | Workflow name (e.g., `deploy`, `update`, `close`)               |
| `id`       | string   | Unique workflow run ID (generated at start, same for all steps) |
| `step`     | string   | Step name from the workflow definition                          |
| `result`   | string   | `completed`, `error`, or `skipped` (step skipped by its on-error policy) |
| `errors`   | []string | Array of error messages (empty when `result` is `completed`)    |
| `txs`      | []object | Array of raw transaction results (empty for non-tx steps)       |

**Transaction object schema (within `txs`):**

| Field      | Type   | Description                                   |
| ---------- | ------ | --------------------------------------------- |
| `hash`     | string | Transaction hash                              |
| `height`   | int64  | Block height where the tx was included        |
| `gas_used` | int64  | Gas consumed (omitted when the executor does not report gas) |
| `code`     | uint32 | Response code (0 = success)                   |
| `raw_log`  | string | Raw log output (present on error, omitted otherwise) |

The `--output jsonl` and `--output json` values serve different purposes: `--output json` affects how individual command results are formatted (JSON serialization), while `--output jsonl` controls the workflow execution mode, emitting structured JSONL progress for the entire multi-step workflow.

#### 2.3.9 Built-in Workflows

Three workflows ship as embedded defaults:

- **deploy**: Full deployment lifecycle (create deployment -> wait bids -> select -> lease -> manifest -> active)
- **update**: Update deployment (update tx -> send manifest to all providers with active leases)
- **close**: Close deployment (close tx, returning remaining escrow balance)

The `update` and `close` workflows wrap the same on-chain transactions as `akt tx deployment update` and `akt tx deployment close`, but add orchestration (manifest re-send for update, confirmation prompts) and unified output modes (TUI progress or JSONL). Users who need only the raw transaction can use the `tx` commands directly.

**Update workflow definition:**

```yaml
name: update
description: Update a deployment and send the new manifest to providers
version: 1

params:
  sdl-file:
    type: file
    required: true
    description: Path to updated SDL deployment file
  dseq:
    type: int
    required: true
    description: Deployment sequence to update

steps:
  - name: update-deployment
    type: tx
    msg: deployment.MsgUpdateDeployment
    params:
      sdl: "{{ index .Params \"sdl-file\" }}"
      dseq: "{{ .Params.dseq }}"
    on-error: abort

  - name: display-result
    type: output
    template: |
      Deployment updated!
        DSEQ: {{ .Params.dseq }}
```

Manifest re-send to providers with active leases (a `foreach` over `market.leases` running `provider`/`send-manifest` sub-steps) is planned for when the engine gains the `foreach` step type; until then, keyring users re-send manifests with `akt provider send-manifest` after an update, and console-api contexts get manifest handling from the Console API automatically.

**Close workflow definition:**

```yaml
name: close
description: Close a deployment and return remaining escrow balance
version: 1

params:
  dseq:
    type: int
    required: true
    description: Deployment sequence to close

steps:
  - name: close-deployment
    type: tx
    msg: deployment.MsgCloseDeployment
    params:
      dseq: "{{ .Params.dseq }}"
    on-error: abort

  - name: display-result
    type: output
    template: |
      Deployment closed.
        DSEQ: {{ .Params.dseq }}
```

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
| `--deposit`        | string   | `auto`          | Initial deposit, unified syntax on both rails (see §7.4): `5usd`/`$5`, `5000000uakt`, or `auto` |
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
| `--output`         | string   | `pretty`        | Output format: `pretty` (TUI), `jsonl` (JSONL step output, see 2.3.8), `json`, `yaml` |

**Transaction flags** (inherited): `--gas`, `--gas-prices`, `--fees`, `--gas-adjustment`, `--broadcast-mode`

**TUI mode** (TTY detected, default):
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

**JSONL mode** (for CI/CD, scripting, automation):
```bash
$ akt deploy deployment.yaml --bid-select cheapest --yes -o jsonl
{"workflow":"deploy","id":"wf_a1b2c3","step":"create-deployment","result":"completed","errors":[],"txs":[{"hash":"ABCD1234...","height":12345,"gas_used":150000,"code":0}]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"wait-for-bids","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"select-bid","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"create-lease","result":"completed","errors":[],"txs":[{"hash":"EFGH5678...","height":12350,"gas_used":120000,"code":0}]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"send-manifest","result":"completed","errors":[],"txs":[]}
{"workflow":"deploy","id":"wf_a1b2c3","step":"display-result","result":"completed","errors":[],"txs":[]}
```

Each line can be parsed independently with `jq`:
```bash
akt deploy deployment.yaml --bid-select cheapest --yes -o jsonl | jq -r 'select(.step == "create-deployment") | .txs[0].hash'
```

#### `akt update <sdl-file> [dseq]`

Update an existing deployment with a new SDL. Orchestrates:

1. **Update deployment transaction** on chain with the new SDL.
2. **Send updated manifest** to the provider(s) with active leases.

The `dseq` is a positional argument (consistent with the filter argument pattern used by query commands). It can also be specified via the `--dseq` flag.

| Flag               | Type     | Default         | Description                                                 |
| ------------------ | -------- | --------------- | ----------------------------------------------------------- |
| `--from`           | string   | context default | Account that owns the deployment                            |
| `--dseq`           | uint64   | `0`             | Deployment sequence (alternative to positional arg)         |
| `--yes`            | bool     | `false`         | Skip all confirmations                                      |
| `--dry-run`        | bool     | `false`         | Print what would happen without executing                   |
| `--output`         | string   | `pretty`        | Output format: `pretty` (TUI), `jsonl` (JSONL step output), `json`, `yaml` |

**Transaction flags** (inherited): `--gas`, `--gas-prices`, `--fees`, `--gas-adjustment`, `--broadcast-mode`

**Examples:**
```bash
# Interactive (default) — dseq as positional arg
akt update deployment.yaml 12345

# dseq via flag (equivalent)
akt update deployment.yaml --dseq 12345

# Non-interactive
akt update deployment.yaml 12345 --yes

# CI/CD pipeline
akt update deployment.yaml 12345 --yes -o jsonl
```

**JSONL mode:**
```bash
$ akt update deployment.yaml --dseq 12345 --yes -o jsonl
{"workflow":"update","id":"wf_x1y2z3","step":"update-deployment","result":"completed","errors":[],"txs":[{"hash":"ABCD1234...","height":12400,"gas_used":140000,"code":0}]}
{"workflow":"update","id":"wf_x1y2z3","step":"send-manifest","result":"completed","errors":[],"txs":[]}
```

#### `akt close [dseq]`

Close a deployment, terminating all active leases and returning remaining escrow balance.

The `dseq` is a positional argument (consistent with the filter argument pattern used by query commands). It can also be specified via the `--dseq` flag.

| Flag               | Type     | Default         | Description                                                 |
| ------------------ | -------- | --------------- | ----------------------------------------------------------- |
| `--from`           | string   | context default | Account that owns the deployment                            |
| `--dseq`           | uint64   | `0`             | Deployment sequence (alternative to positional arg)         |
| `--yes`            | bool     | `false`         | Skip all confirmations                                      |
| `--dry-run`        | bool     | `false`         | Print what would happen without executing                   |
| `--output`         | string   | `pretty`        | Output format: `pretty` (TUI), `jsonl` (JSONL step output), `json`, `yaml` |

**Transaction flags** (inherited): `--gas`, `--gas-prices`, `--fees`, `--gas-adjustment`, `--broadcast-mode`

**Examples:**
```bash
# Interactive (with confirmation prompt) — dseq as positional arg
akt close 12345

# dseq via flag (equivalent)
akt close --dseq 12345

# Non-interactive
akt close 12345 --yes

# CI/CD pipeline
akt close 12345 --yes -o jsonl
```

**JSONL mode:**
```bash
$ akt close --dseq 12345 --yes -o jsonl
{"workflow":"close","id":"wf_p1q2r3","step":"close-deployment","result":"completed","errors":[],"txs":[{"hash":"WXYZ9876...","height":12500,"gas_used":100000,"code":0}]}
```

### 2.4 Provider Gateway Commands

#### `akt provider status [provider-addr]`

Query provider status. If `provider-addr` is omitted, uses the provider from the active lease context.

| Flag          | Type   | Default         | Description              |
| ------------- | ------ | --------------- | ------------------------ |
| `--provider`  | string | `""`            | Provider address         |
| `--auth-type` | string | context default | Auth type: `jwt`, `mtls` |

#### `akt provider lease-status [dseq]`

Query lease deployment status from the provider gateway. The positional `dseq` supplies the deployment sequence. The `--dseq` flag is **disabled pending feedback** (positional only, 2026-07).

| Flag          | Type   | Default         | Description         |
| ------------- | ------ | --------------- | ------------------- |
| `--dseq`      | uint64 | required unless positional `dseq` given | Deployment sequence — **disabled pending feedback** (positional only, 2026-07) |
| `--gseq`      | uint32 | `1`             | Group sequence      |
| `--oseq`      | uint32 | `1`             | Order sequence      |
| `--provider`  | string | required        | Provider address    |
| `--from`      | string | context default | Owner account       |
| `--auth-type` | string | context default | Auth type           |

#### `akt provider lease-logs [dseq]`

Stream container logs from a lease. The positional `dseq` supplies the deployment sequence. The `--dseq` flag is **disabled pending feedback** (positional only, 2026-07).

| Flag          | Type   | Default         | Description               |
| ------------- | ------ | --------------- | ------------------------- |
| `--dseq`      | uint64 | required unless positional `dseq` given | Deployment sequence — **disabled pending feedback** (positional only, 2026-07) |
| `--gseq`      | uint32 | `1`             | Group sequence            |
| `--oseq`      | uint32 | `1`             | Order sequence            |
| `--provider`  | string | required        | Provider address          |
| `--from`      | string | context default | Owner account             |
| `--service`   | string | `""`            | Filter by service name    |
| `--follow`    | bool   | `false`         | Stream logs continuously  |
| `--tail`      | int64  | `-1`            | Lines from end (-1 = all) |
| `--auth-type` | string | context default | Auth type                 |

#### `akt provider lease-events [dseq]`

Stream Kubernetes events from a lease. The positional `dseq` behaves as in `lease-logs` (`--dseq` — **disabled pending feedback**, positional only, 2026-07).

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

#### `akt provider get-manifest [dseq]`

Retrieve the current manifest from a provider. The positional `dseq` supplies the deployment sequence. The `--dseq` flag is **disabled pending feedback** (positional only, 2026-07).

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
Store Path:   ~/.config/akt/contexts/mainnet/store/
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

### 2.6 Monitor Command

#### `akt monitor [rpc-endpoint]`

Hub-based real-time monitoring tool for network state, provider fleet health, oracle prices, and BME state. See [DESIGN.md section 1.4](DESIGN.md#14-the-monitor-command) for design goals and rationale.

`akt monitor` launches a hub that defaults to the **Network** dashboard and allows switching between three dashboards via Tab/Shift-Tab:

| Hub Tab | Dashboard | Content |
|---------|-----------|---------|
| **Network** (default) | Consensus, validators, governance | [§8.3.8](#838-consensus-monitor-view-from-aktop), [§8.3.9](#839-validator-voting-view-from-aktop), [§8.3.11](#8311-governance-parameters-view-from-aktop) |
| **Provider** | Fleet health, versions, resources | [§8.3.10](#8310-provider-fleet-monitor-view) |
| **Oracle/BME** | Prices, health, vault state, mint status, ledger | [§8.3.12](#8312-oraclebme-monitor-view) |

Each dashboard is also directly accessible via its CLI subcommand. When launched via a subcommand, the hub starts on that dashboard but Tab/Shift-Tab still allows switching to other dashboards.

`akt monitor` is especially critical during coordinated chain upgrades, when the network halts at a target block height and online block explorers become unreliable. It connects directly to the user's RPC endpoint to provide an authoritative view of upgrade progress: which round and step the chain is in, which validators have come back online and are voting, and whether 2/3+ voting power has reached precommit.

**Endpoint resolution** (first match wins, shared by all subcommands):

1. Positional argument: `akt monitor https://rpc.akashnet.net:443`
2. `--rpc` flag: `akt monitor --rpc https://rpc.akashnet.net:443`
3. Active context RPC endpoint (from `context → network → endpoints.rpc[0]`)

If no endpoint can be resolved, the command exits with an error.

**Shared flags** (apply to `akt monitor` and all subcommands):

| Flag              | Type   | Default         | Description                                              |
| ----------------- | ------ | --------------- | -------------------------------------------------------- |
| `--rpc`           | string | context default | RPC endpoint (WebSocket-capable)                         |
| `--rest`          | string | auto-derived    | REST API endpoint (for governance/oracle/BME queries). Default: derived from the context's `endpoints.api[0]`. If no API endpoint is configured, falls back to the RPC host on port 1317 (standard Cosmos REST port). |
| `--insecure`      | bool   | `false`         | Skip TLS certificate verification                        |
| `--clean-cache`   | bool   | `false`         | Clear the local cache before start                       |

**Hub navigation:**

| Key | Action |
|-----|--------|
| `Tab` | Switch to next dashboard (Network → Provider → Oracle/BME → Network) |
| `Shift-Tab` | Switch to previous dashboard |
| `1`/`2`/`3` | Switch sub-tab within the active dashboard (Network only) |

**Standalone operation**: `akt monitor` (and all subcommands) requires only an RPC endpoint. It does not require a keyring, default account, or chain-id. A monitoring-only context (with no `default-account`) or a bare `--rpc` flag is sufficient, making it usable by anyone observing the network.

#### `akt monitor network [rpc-endpoint]`

Launches directly into the Network dashboard. This is the replacement for the former `akt top` command.

The Network dashboard has three sub-tabs:

| Key | Tab              | Description                                                                | Spec reference |
| --- | ---------------- | -------------------------------------------------------------------------- | -------------- |
| `1` | **Overview**     | Consensus state (height, round, step, elapsed, proposer), vote progress bars (prevote/precommit with power fractions), validator vote grid (`●`/`○`) | [§8.3.8](#838-consensus-monitor-view-from-aktop) |
| `2` | **Validators**   | Scrollable validator list with moniker, voting power, prevote/precommit status, block signing history bar, proposer indicator | [§8.3.9](#839-validator-voting-view-from-aktop) |
| `3` | **Governance**   | Module-by-module governance parameter browser with pretty-printed JSON | [§8.3.11](#8311-governance-parameters-view-from-aktop) |

#### `akt monitor provider [rpc-endpoint]`

Launches directly into the Provider dashboard. Displays real-time provider fleet monitoring:

- **Scan progress**: Progress bar showing checked/total providers and online count.
- **Version distribution**: Versions sorted newest-first (semver-aware, handles `-rc` suffixes). Dot visualization with `●` for selected version, `○` for others. h/l to select version filter.
- **Provider list**: Scrollable table with URL, version, CPU, memory, GPU, location. Filtered by selected version.
- **Provider detail**: Enter on a provider shows node-level breakdown with CPU, memory, GPU model + count.

Data sources: on-chain provider list (ABCI query), per-provider health (gRPC port 8444 preferred, REST `/status` + `/version` fallback), active leases (REST, for priority scheduling).

Cache: smart scheduling (online: 1m, recently offline: 5m, long-term offline: 6h), priority queue, max 10 concurrent checks, chain re-sync every 10m.

#### `akt monitor oracle [rpc-endpoint]` / `akt monitor bme [rpc-endpoint]`

Both commands are aliases that launch directly into the **Oracle/BME** dashboard. The combined dashboard displays:

- **Aggregated prices section**: Per-denom aggregated price data with TWAP, median, min/max, source count, deviation (bps), and timestamp.
- **Price health section**: Health status (color-coded: green=healthy, red=unhealthy), failure reasons (when unhealthy), minimum sources check, deviation check, total vs healthy source counts.
- **Price history section**: Recent price feed entries table with asset denom, base denom, price, source, and timestamp.
- **BME status section**: Fields in order: Status (color-coded: green=healthy, yellow=warning, red=halt CR/halt Oracle), Mints (Allowed/Halted), Refunds (Allowed/Halted), Collateral Ratio, Thresholds (nested: Warn, Halt).
- **Vault section**: Balances, total burned, total minted, remint credits. All amounts formatted using `FormatCoin()`.
- **Ledger section**: Recent ledger entries table with route, ID, status, burned, minted, spread, remint accrued, remint issued.

Data sources:
- Oracle: REST `/akash/oracle/v2/prices`, `/akash/oracle/v2/aggregated-price/{denom}` + real-time bus events
- BME: REST `/akash/bme/v1/status`, `/akash/bme/v1/vault-state`, `/akash/bme/v1/ledger`

Refresh intervals: oracle aggregated prices every 30s, BME status/vault every 30s, price history and ledger every 2m.

### 2.7 Events Command

#### `akt events`

Stream live blockchain events in real-time. When launched with no subcommand and a TTY is attached, opens an interactive TUI event viewer. When piped or with `--output json`, emits one JSON object per event to stdout.

| Flag          | Type     | Default         | Description                                              |
| ------------- | -------- | --------------- | -------------------------------------------------------- |
| `--module`    | string   | `""`            | Filter by module (e.g., `deployment`, `market`, `bank`). Empty = all modules. |
| `--type`      | string   | `""`            | Filter by event type (e.g., `MsgCreateDeployment`, `EventBidCreated`). Empty = all types. |
| `--follow`    | bool     | `true`          | Stream events continuously. When `false`, prints events from recent blocks and exits. |
| `--height`    | int64    | `0`             | Start from a specific block height (0 = current). Only used with `--follow=false`. |
| `--output`    | string   | `pretty`        | Output format: `pretty` (TUI viewer when TTY, colorized text otherwise), `json` (one JSON object per event line). |

**TUI mode** (TTY attached, default): Interactive event viewer with auto-scroll, module/type column highlighting, and `Tab` to cycle module filters. `q` or `Ctrl+c` to quit.

**JSON mode** (`--output json` or piped): Emits one JSON line per event for scripting:

```jsonl
{"height":18234567,"module":"deployment","type":"MsgCreateDeployment","attributes":{"owner":"akash1abc...","dseq":"12345"}}
{"height":18234567,"module":"market","type":"EventBidCreated","attributes":{"owner":"akash1abc...","dseq":"12345","provider":"akash1prov..."}}
```

**Data source**: RPC WebSocket subscription to `tm.event='Tx'` and `tm.event='NewBlock'` events. Events are parsed from ABCI event attributes and published through the shared event bus (`internal/events/`).

**Standalone operation**: Like `akt monitor`, requires only an RPC endpoint. No keyring or default account needed.

**Examples:**
```bash
# Stream all events (TUI viewer)
akt events

# Stream only deployment events
akt events --module deployment

# Stream as JSON for piping
akt events --output json | jq '.attributes'

# Stream market events for scripting
akt events --module market --output json
```

### 2.8 MCP Command

#### `akt mcp`

Start an MCP (Model Context Protocol) server over stdio transport for AI assistant integration. The server exposes Akash Network tools that AI assistants can invoke to query chain state, check provider status, and (with explicit permission) perform mutating operations.

Configuration is resolved from the active akt context (network, keyring, default account). No additional environment variables are required beyond a configured context.

| Flag               | Type | Default | Description                                                                       |
| ------------------ | ---- | ------- | --------------------------------------------------------------------------------- |
| `--enable-writes`  | bool | `false` | Enable write tools (on-chain transactions and provider mutations). Without this flag, only read-only query tools are available. |

**Permission model:**

By default, only read-only query tools are registered. This prevents AI agents from sending unapproved transactions or performing mutating operations. The `--enable-writes` flag must be explicitly passed to enable write tools. This flag covers both on-chain transactions (which require keyring signing) and mutating provider REST API calls (e.g., submitting manifests).

**Read-only tools (always available, 21 tools):**

| Tool Name                      | Description                                                                |
| ------------------------------ | -------------------------------------------------------------------------- |
| `akash_node_status`            | Node sync status (block height, hash, catching up)                         |
| `akash_block_height`           | Current block height                                                       |
| `akash_account_balance`        | Token balances for an account                                              |
| `akash_list_deployments`       | List deployments with optional filters                                     |
| `akash_get_deployment`         | Get deployment details by owner/dseq                                       |
| `akash_get_group`              | Get deployment group details                                               |
| `akash_list_orders`            | List market orders with optional filters                                   |
| `akash_get_order`              | Get order details                                                          |
| `akash_list_bids`              | List bids with optional filters                                            |
| `akash_get_bid`                | Get bid details                                                            |
| `akash_list_leases`            | List leases with optional filters                                          |
| `akash_get_lease`              | Get lease details                                                          |
| `akash_list_providers`         | List registered providers                                                  |
| `akash_get_provider`           | Get provider details by address                                            |
| `akash_provider_status`        | Live provider status via REST API                                          |
| `akash_lease_status`           | Live lease status from provider                                            |
| `akash_service_status`         | Service status within a lease                                              |
| `akash_list_audited_providers` | List audited provider attributes                                           |
| `akash_list_certificates`      | List on-chain certificates                                                 |

**Write tools (only with `--enable-writes`, 4 tools):**

| Tool Name                      | Description                                                                |
| ------------------------------ | -------------------------------------------------------------------------- |
| `akash_close_deployment`       | Close a deployment (on-chain transaction)                                  |
| `akash_create_lease`           | Create a lease from a bid (on-chain transaction)                           |
| `akash_close_lease`            | Close an active lease (on-chain transaction)                               |
| `akash_submit_manifest`        | Submit manifest to provider (provider REST mutation)                       |

**Transport:** stdio (JSON-RPC over stdin/stdout). Designed for use with MCP-compatible AI assistants (e.g., Claude Desktop).

**Client implementation:** Uses `v1beta3.LightClient` from chain-sdk for read-only mode, `v1beta3.Client` for write mode.

**Default account handling:** Tools that accept an `owner` parameter (e.g., `akash_list_deployments`, `akash_list_leases`) default to the context's `default-account` when the parameter is omitted. If no `default-account` is configured (e.g., a monitoring-only context), the `owner` parameter is **required** — the tool returns an error explaining that the owner must be specified explicitly when no default account is available.

**Examples:**

```bash
# Read-only mode (default, safe for AI agents)
akt mcp

# With write tools enabled (explicit user consent)
akt mcp --enable-writes
```

**Claude Desktop configuration (`~/.claude/claude_desktop_config.json`):**

```json
{
  "mcpServers": {
    "akash": {
      "command": "akt",
      "args": ["mcp"]
    }
  }
}
```

With write tools:

```json
{
  "mcpServers": {
    "akash": {
      "command": "akt",
      "args": ["mcp", "--enable-writes"]
    }
  }
}
```

### 2.9 Console Commands

The `akt console` group drives the Akash Console managed-wallet API (§7): deployments are created, funded, and closed by the Console's server-side wallet — no local keyring or gas handling is involved. Authenticated commands resolve the API key per §7.1 (`--console-api-key` flag > `AKT_CONSOLE_API_KEY` > per-context stored credential) and fail with a pointer to `akt console login` when none is found. The base URL resolves per §7.2 (`--console-api-url` flag > context `console-api-url` > default). Public catalog commands (`provider`, `gpu`, `template`) work without a key and without a configured context.

**Group persistent flags:**

| Flag                | Type   | Default | Description                                                          |
| ------------------- | ------ | ------- | -------------------------------------------------------------------- |
| `--console-api-url` | string | `""`    | Console API base URL (overrides the context setting)                 |
| `--console-api-key` | string | `""`    | Console API key (overrides env var and stored credential, session only) |

**Authentication:**

| Command                    | Description                                                                                                     |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `akt console login [key]`  | Validate a key against `GET /v1/user/me` and store it as the active context's credential (§7.1). The key is taken from the positional argument or a hidden stdin prompt. Requires an active context. Prints the username; never prints the key. |
| `akt console logout`       | Remove the active context's stored credential.                                                                    |
| `akt console whoami`       | Show the authenticated user (username, email, verified).                                                          |

**Deployments** (positional `dseq` per §3.8.2):

| Command                                             | Flags                                          | Description                                                                                   |
| --------------------------------------------------- | ---------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| `akt console deployment list`                       | `--skip` (0), `--limit` (20)                   | List deployments with pagination.                                                              |
| `akt console deployment get <dseq>`                 |                                                | Deployment with leases and escrow account.                                                     |
| `akt console deployment create <sdl-file> [deposit-usd]` | `--deposit <usd>` (alternative to positional; min 0.5) — **disabled pending feedback** (positional only, 2026-07) | Create a deployment; prints `dseq` + tx hash. The returned manifest is cached at `contexts/<name>/manifests/<dseq>.json` for `lease create`. |
| `akt console deployment update <dseq> <sdl-file>`   |                                                | Update the deployment's SDL.                                                                   |
| `akt console deployment close <dseq>`               |                                                | Close a deployment. Idempotent: an already-closed deployment prints a note and exits 0.        |
| `akt console deployment deposit <dseq> [amount-usd]` | `--amount <usd>` (alternative to positional; > 0) — **disabled pending feedback** (positional only, 2026-07) | Add funds to the deployment's escrow.                                                          |
| `akt console deployment settings <dseq> [true\|false]` | `--auto-top-up true\|false` (alternative) — **disabled pending feedback** (positional only, 2026-07) | Show settings when no value is given; set auto-top-up when a positional or flag value is present. |
| `akt console bid list <dseq>`                       |                                                | List bids for the deployment's open orders.                                                    |
| `akt console lease create <dseq> [provider]`        | `--gseq` (1), `--oseq` (1), `--provider` (alternative to positional) — **disabled pending feedback** (positional only, 2026-07), `--manifest <file>` | Accept a bid; the manifest defaults to the one cached by `deployment create`. |

**Wallet and usage:**

| Command                       | Flags                        | Description                                                     |
| ----------------------------- | ---------------------------- | ---------------------------------------------------------------- |
| `akt console wallet list`     |                              | List managed wallets (balances shown as `$X.XX`).                |
| `akt console wallet balance`  |                              | Available / in-deployment / total balance in USD.                |
| `akt console wallet settings [true\|false]` | `--auto-reload true\|false` (alternative) — **disabled pending feedback** (positional only, 2026-07) | Show settings when no value is given; set auto-reload otherwise. |
| `akt console wallet cost`     |                              | Estimated weekly cost in USD.                                    |
| `akt console usage [from] [to]` | `--from`, `--to` (YYYY-MM-DD, alternatives) — **disabled pending feedback** (positional only, 2026-07) | Daily spend history for the managed wallet. Omitted dates use the API defaults (last 30 days). |

**Public catalog** (no API key required):

| Command                             | Flags            | Description                                             |
| ----------------------------------- | ---------------- | -------------------------------------------------------- |
| `akt console provider list`         | `--limit` (20, 0 = all) | List providers (limit applied client-side).       |
| `akt console provider get <address>` |                 | One provider's full catalog record.                      |
| `akt console provider regions`      |                  | Regions providers advertise.                             |
| `akt console provider auditors`     |                  | Known auditors.                                          |
| `akt console gpu`                   |                  | Network-wide GPU availability and price catalog.         |
| `akt console template list`         |                  | Template catalog.                                        |
| `akt console template get <id>`     |                  | One template.                                            |
| `akt console template sdl <id>`     |                  | Print the template's raw SDL to stdout (for piping).     |

**API keys and JWTs:**

| Command                          | Flags                                          | Description                                                    |
| -------------------------------- | ---------------------------------------------- | ---------------------------------------------------------------- |
| `akt console apikey list`        |                                                | List API keys (secrets are never shown).                         |
| `akt console apikey create <name> [expires-at]` | `--name`, `--expires-at` (RFC 3339, alternatives) — **disabled pending feedback** (positional only, 2026-07) | Create a key; the secret is printed exactly once with a warning. |
| `akt console apikey delete <id>` |                                                | Delete a key; a missing key (404) is a no-op.                    |
| `akt console jwt create`         | `--ttl` (300), `--scope` (csv, default `status,logs,events,shell,send-manifest,get-manifest`) | Mint a short-lived provider-scoped JWT. |

**Provider gateway (live lease operations):**

Managed (Console-API) contexts reach provider gateways directly, without a wallet or local key: each command resolves the deployment's first active lease via the Console API, looks up the provider's `hostUri`, and mints a scoped JWT via `POST /v1/create-jwt-token` that the gateway accepts as `Authorization: Bearer`. One-shot calls use a 300 s token; streaming/interactive modes (`--follow`, `--watch`, `shell`) use 3600 s. Without an active lease the commands fail listing the states of the leases that do exist.

| Command                                            | Flags                                       | Description                                                                                        |
| -------------------------------------------------- | ------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `akt console logs <dseq> [service]`                | `--follow`, `--tail N`, `--service` (alternative) — **disabled pending feedback** (positional only, 2026-07) | Stream container logs from the lease's provider (JWT scope `logs`).                           |
| `akt console events <dseq>`                        | `--follow`                                  | Stream Kubernetes events from the lease's provider (JWT scope `events`).                            |
| `akt console status <dseq>`                        | `--watch`, `--interval` (5s)                | Live lease status from the provider gateway (JWT scope `status`); with `--watch`, snapshots are re-printed each interval until interrupted. `deployment get` remains the Console-API view. |
| `akt console shell <dseq> <service> [-- command...]` |                                           | Interactive shell in a lease container, default `/bin/sh`; exec is the same operation with an explicit command (JWT scope `shell`). TTY auto-detected. |
| `akt console screen <sdl-file>`                    |                                             | Client-side bid screening: derive resources from the SDL and list the providers able to run it (public endpoint, no key needed). |

Per the positional-primary convention (§3.8), every console command takes its primary value(s) positionally; the equivalent flags remain as overrides and a positional value wins when both are given. (2026-07: the flag twins marked *disabled pending feedback* above are commented out in code for the positional-only UX trial — the positional form is the only way while the trial runs; the original flag definitions are preserved in `FEEDBACK(2026-07)` comments for restoration.) Output is indented JSON; USD values are rendered as `$X.XX`. State-changing calls are recorded in the context's action log as `type=console` entries (§5.6). No command ever prints a Console API key, except the one-time secret from `apikey create`.

---

### 2.10 Capability Gating

The active context's configuration determines a **feature set**: which transports are usable and therefore which command groups can work.

| Capability | Derived from | Gates |
|---|---|---|
| `chain-query` | network has ≥1 RPC endpoint | `query`, `monitor` |
| `chain-tx` | network has ≥1 RPC endpoint | `tx` |
| `provider` | network has ≥1 RPC endpoint | `provider` |
| `console` | Console API key resolvable (§7.1) | Console-backed command groups |

Commands declare requirements via a cobra annotation (`akt.requires`, package `internal/capability`); alternatives are separated by `|` (e.g. workflow commands require `chain-tx|console`). A command whose requirement the context cannot satisfy fails fast with the missing capability and its remedy instead of erroring mid-transport.

Presentation is configurable while UX feedback is collected (`defaults.command-gating`):

| Mode | Behavior |
|---|---|
| `dim` (default) | Unavailable commands stay listed, marked `[unavailable]` in help, and fail fast with an explanation. |
| `hide` | Unavailable commands are removed from help listings (direct invocation still fails fast with the explanation). |
| `off` | No gating; commands fail wherever the missing transport is first touched. |

Example: a network-less `console-api` context (API key only) lists and runs only Console commands; `tx`, `query`, `provider`, and `monitor` are dimmed or hidden and explain that an RPC endpoint must be added.

---

### 2.11 SDL Commands

Transport-independent SDL authoring, ported from console-axi (`src/sdl/templates`, `src/sdl/lint.ts`). All `akt sdl` subcommands run entirely locally: no context, key, or RPC endpoint is required, and the group declares no capability requirements.

#### `akt sdl scaffolds`

List the built-in SDL scaffolds (alias: `akt sdl templates`, matching the reference CLI's historical name).

| Scaffold        | Shape                                                                                        |
| --------------- | -------------------------------------------------------------------------------------------- |
| `web`           | Single web service with one HTTP port exposed to the internet (nginx:1.27, 0.5 CPU / 512Mi)  |
| `gpu`           | GPU workload (ML/inference) with an nvidia model requirement (pytorch image, a100, 4 CPU / 16Gi) |
| `multi-service` | App + postgres:16 database with a persistent volume (`beta2`) and service-to-service networking |
| `ip-lease`      | Service with a dedicated public IP — SDL v2.1 `endpoints` + `expose ... to ip`               |

#### `akt sdl init <scaffold>`

Generate SDL YAML on stdout, pipeable into `akt sdl validate -` or redirected to a file for `akt deploy`. The output is self-checked against the validator before printing. Flags are generation parameters with per-scaffold defaults — not positional-argument twins — so the zero-flag invocation always produces a deployable SDL. Pricing defaults to a 10000 uact/block ceiling (100000 for `gpu`) so bids arrive.

| Flag          | Type        | Description                                              |
| ------------- | ----------- | --------------------------------------------------------- |
| `--name`      | string      | Service name (default per scaffold: `web` / `app`)        |
| `--image`     | string      | Container image; must be tagged, e.g. `nginx:1.27`        |
| `--port`      | int         | Container port (default 80; 8080 for `gpu`)               |
| `--as`        | int         | External port (default 80)                                |
| `--cpu`       | string      | CPU units, e.g. `0.5` or `500m`                           |
| `--memory`    | string      | Memory size, e.g. `512Mi`, `2Gi`                          |
| `--storage`   | string      | Storage size, e.g. `1Gi` (sizes the persistent volume for `multi-service`) |
| `--count`     | int         | Replica count (default 1)                                 |
| `--price`     | int         | Max price per block in uact                               |
| `--env`       | stringArray | Environment variable `KEY=value` (repeatable)             |
| `--gpu`       | int         | GPU units (`gpu` scaffold, default 1)                     |
| `--gpu-model` | string      | NVIDIA GPU model (`gpu` scaffold, default `a100`)         |

#### `akt sdl validate <file>`

Validate an SDL offline (`-` reads stdin). Parsing and schema/relational validation use `pkg.akt.dev/go/sdl` — the same parser behind `akt deploy` and the chain tx commands — followed by lint rules ported from the reference:

- **Unpinned image** (error): every service image must carry an explicit tag or `@sha256:` digest; untagged images and `:latest` are rejected as non-reproducible.
- **Pricing denom**: `uact` passes; `uakt` produces a **warning**, not an error — a deliberate deviation from the reference, which hard-rejects `uakt` because it only serves the managed Console API. akt serves both rails: `uakt` is valid on-chain, but console-api (managed) contexts price in `uact`. Any other denom is an error, matching the reference.

Exit `0` when valid, printing a summary (`valid: N service(s), M group(s), K warning(s)`) plus any warnings; exit `1` when invalid, listing every parse/lint error.

```bash
# Generate, self-check, and validate
akt sdl init web --image nginx:1.27 > deploy.yaml
akt sdl validate deploy.yaml

# Pipe without touching disk
akt sdl init gpu --gpu-model h100 | akt sdl validate -
```

---

---

## 3. Flag Specification

### 3.1 Global Persistent Flags

Applied to every command via the root command's `PersistentFlags()`.

| Flag        | Short | Type   | Default                  | Description                                                      |
| ----------- | ----- | ------ | ------------------------ | ---------------------------------------------------------------- |
| `--home`    |       | string | `$AKT_HOME` or XDG default | Home directory for config, contexts, and keyrings             |
| `--context` |       | string | config `current-context` | Active context name (overrides AKT_CONTEXT)                      |
| `--output`  | `-o`  | string | `"pretty"`               | Output format: `pretty`, `json`, `yaml`. For workflows, also accepts `jsonl` (see 2.3.8). |
| `--interactive` | `-i` | bool | `false`              | Force interactive mode even when `defaults.interactive` is `false` in config or no TTY is detected. Has no effect when interactive mode is already enabled (the default). **Two effects**: (1) Workflow commands (`deploy`, `update`, `close`) use TUI progress display instead of JSONL. (2) Commands that auto-suppress prompts and spinners in non-TTY contexts will show them. Does **not** launch the root TUI application. |
| `--verbose` | `-v`  | count  | `0`                      | Increase output verbosity. Stacks: `-v` (level 1) shows operational detail (gas estimates, endpoint selection, config resolution); `-vv` (level 2) adds debug diagnostics (RPC request/response dumps, full stack traces). Default (no flag) shows progress/status messages. Mutually exclusive with `--quiet`. |
| `--quiet`   | `-q`  | bool   | `false`                  | Suppress all informational output (progress messages, status lines, confirmations). Only data output (query results, transaction results) and errors are emitted. Useful for scripting. Mutually exclusive with `-v`. |

#### 3.1.1 Confirmation and Override Conventions

Two flags control confirmation and safety bypass behavior across the CLI. They serve different purposes and must not be conflated:

| Flag | Short | Semantics | Example |
|---|---|---|---|
| `--yes` | `-y` | Skip interactive confirmation prompts. The operation proceeds as if the user answered "yes" to all prompts. The operation itself is unchanged. | `akt tx deployment close 12345 --yes` |
| `--force` | | Override a safety guard that would otherwise prevent the operation. The operation may behave differently or bypass a check. | `akt context network delete mainnet --force` (deletes even if contexts reference it) |

`--yes` / `-y` is **not a global flag**. It is added individually to commands that have confirmation prompts: all `tx` commands (via `AddTxFlagsToCmd()`, §3.2), workflow commands (`deploy`, `update`, `close`), and destructive context/store management commands (`context delete`, `store import --replace`). `--force` is used sparingly on specific commands where a structural safety check exists (e.g., deleting a network that is referenced by contexts). Commands should never use `--force` as a synonym for `--yes`.

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

**Pretty output for transaction results**: When `--output pretty` is active (the global default), transaction results are rendered in a two-section layout: a common transaction summary (hash, signer, height, gas, fee, status) followed by a message-specific detail section. See [§10.11](#1011-transaction-result-formatting) for the full specification.

**CLI-mode progress feedback**: When a TTY is attached and `--quiet` is not set, `tx` commands display progress status on stderr during multi-second operations. This applies to all `tx` commands, not just workflows.

| Phase | stderr output | Timing |
|---|---|---|
| Gas simulation | `Simulating transaction...` | Shown when `--gas auto` (the default) triggers simulation |
| Broadcast | `Broadcasting transaction...` | Shown immediately after signing |
| Confirmation wait | `Waiting for mempool acceptance...` | Shown when `--broadcast-mode sync` (the default) waits for CheckTx (mempool acceptance). Note: `sync` does not wait for block inclusion — use `--broadcast-mode block` for that. |

These status lines are written to stderr (see [§10.1.1](#1011-stream-separation-stdout-vs-stderr)) so they never interfere with piped data output. When the operation completes, the final transaction result is written to stdout in the format selected by `--output`. When `--quiet` is set or no TTY is attached, status lines are suppressed -- only the final result (stdout) and errors (stderr) are emitted. When `-v` is set, additional detail is shown (e.g., selected endpoint, simulated gas amount, raw CheckTx response).

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

Used by `tx` commands and `provider` gateway commands for resource identification. These compose hierarchically: LeaseID includes BidID includes OrderID includes GroupID includes DeploymentID.

> **Note:** For `query` commands, resource identity filtering is done via the positional filter argument instead of these flags. See [section 3.8](#38-resource-filter-argument) for the filter syntax. The flags below continue to be used by `tx` and `provider` commands where no positional twin exists (e.g. `tx deployment create`, `tx market bid create/close`, `provider send-manifest`, `provider lease-shell`, `provider migrate-*`).
>
> **2026-07 positional-only trial:** on commands where a positional twin exists — `tx deployment close/update`, `tx deployment group close/pause/start`, `tx market lease create/withdraw/close`, `provider lease-status/lease-logs/lease-events/get-manifest` — the duplicated identity flags are **disabled pending feedback** (positional only, 2026-07); see the per-flag notes below.

| Flag         | Type   | Default                   | Description                |
| ------------ | ------ | ------------------------- | -------------------------- |
| `--owner`    | string | context `default-account` | Deployment owner address   |
| `--dseq`     | uint64 | `0`                       | Deployment sequence number — **disabled pending feedback** (positional only, 2026-07) on `tx deployment close/update`, `tx deployment group *`, `tx market lease *`, `provider lease-status/lease-logs/lease-events/get-manifest` |
| `--gseq`     | uint32 | `1`                       | Group sequence number — **disabled pending feedback** (positional only, 2026-07) on `tx deployment group *` |
| `--oseq`     | uint32 | `1`                       | Order sequence number      |
| `--provider` | string | `""`                      | Provider address — **disabled pending feedback** (positional only, 2026-07) on `tx market lease *` |

### 3.6 Deployment Query Flags

Used by `query deployment`. Resource identity (owner, dseq) is supplied via the positional filter argument (see [section 3.8](#38-resource-filter-argument)); the duplicated `--owner`/`--dseq` filter flags are **disabled pending feedback** (positional only, 2026-07). When enough components are provided to identify a single deployment, returns detail format; otherwise returns a filtered list.

| Flag      | Type   | Default | Description                                           |
| --------- | ------ | ------- | ----------------------------------------------------- |
| `--state` | string | `""`    | Filter by state: `active`, `closed`, or empty for all — **disabled pending feedback** (positional only, 2026-07; use the positional state keyword, including the second positional form `akt query deployment akash1x/12345 active`) |

### 3.7 Market Query Flags

Used by `query market order`, `query market bid`, and `query market lease`. Resource identity is supplied via the positional filter argument (see [section 3.8](#38-resource-filter-argument)); the duplicated `--owner`/`--dseq`/`--gseq`/`--oseq`/`--provider` filter flags are **disabled pending feedback** (positional only, 2026-07). When enough components are provided to uniquely identify a single resource, returns detail format; otherwise returns a filtered list. `--by` remains enabled (it is a mode switch with no positional twin).

| Flag      | Type   | Default   | Description                               |
| --------- | ------ | --------- | ----------------------------------------- |
| `--by`    | string | `"owner"` | Filter perspective: `owner` or `provider`. Controls how the filter argument is parsed. See [section 3.8](#38-resource-filter-argument). Only applies to `bid` and `lease` queries. |
| `--state` | string | `""`      | Filter by state — **disabled pending feedback** (positional only, 2026-07; use the positional state keyword, including the second positional form `akt query market lease 12345 active`) |

### 3.8 Resource Filter Argument

Akash `query` commands for deployment, market, certificate, audit, and escrow resources accept an optional positional **filter argument** that replaces flag-based identity filtering (the `--owner`, `--dseq`, `--gseq`, `--oseq`, `--provider` flags documented in section 3.5). This supports the [argument-driven filtering](DESIGN.md#11-goals) design goal. The deployment, order, bid, lease, and cert queries additionally accept an optional **second positional argument** — a state keyword — so identity+state combinations stay expressible without `--state` (e.g. `akt query deployment akash1x/12345 active`, `akt query market lease 123 active`).

#### 3.8.1 General Format

The filter is a single positional argument using `/` as the separator. Components are parsed **left to right** following the Akash resource hierarchy.

**Owner perspective** (`--by owner`, the default):

```
[owner/]dseq[/gseq[/oseq[/provider]]]
```

**Provider perspective** (`--by provider`, bids and leases only):

```
[provider/]dseq[/gseq[/oseq[/owner]]]
```

When no filter argument is given, the command lists all matching resources for the context's default account (in owner mode) or requires an explicit provider address (in provider mode).

#### 3.8.2 Smart Type Detection

The first component is classified by its format:

| Format                        | Detection                     | Example            |
| ----------------------------- | ----------------------------- | ------------------ |
| Bech32 address (`akash1...`)  | Leading address (owner or provider per `--by` mode) | `akash1abc...def` |
| Unsigned integer              | `dseq` — the leading address defaults to the context's `default-account` (in `--by owner` mode) | `12345` |
| State keyword (per-resource vocabulary) | State filter — equivalent to `--state <keyword>` | `active` |

Subsequent `/`-separated components are parsed positionally: after the leading address comes `dseq` (uint64), then `gseq` (uint32), then `oseq` (uint32), then the trailing address (provider or owner, opposite of the leading address).

State keywords are only recognized as the sole/first component of the filter argument; they do not combine with identity paths inside a single argument. Since 2026-07 the identity+state combination is expressed with the optional **second positional argument** (`akt query deployment akash1abc/12345 active`); two state keywords (a bare-keyword first argument plus a second argument) are an error, and the `--state` flag is **disabled pending feedback** (positional only, 2026-07). Each resource has its own state vocabulary, derived from the on-chain state enums:

| Resource       | State keywords                        |
| -------------- | ------------------------------------- |
| `deployment`   | `active`, `closed`                    |
| `market order` | `open`, `active`, `closed`            |
| `market bid`   | `open`, `active`, `lost`, `closed`    |
| `market lease` | `active`, `insufficient_funds`, `closed` |

#### 3.8.3 Get-vs-List Heuristic

- If enough components are specified to **uniquely identify** a single resource, the command returns a single-item **detail** response.
- Otherwise, the command returns a **filtered list** response.

| Command                | Unique identity requires              |
| ---------------------- | ------------------------------------- |
| `query deployment`     | owner + dseq                          |
| `query deployment group` | owner + dseq + gseq                 |
| `query market order`   | owner + dseq + gseq + oseq           |
| `query market bid`     | owner + dseq + gseq + oseq + provider |
| `query market lease`   | owner + dseq + gseq + oseq + provider |
| `query escrow`         | owner + dseq (scope + xid)           |

#### 3.8.4 Defaults for Omitted Components

| Component        | Default when omitted                                                         |
| ---------------- | ---------------------------------------------------------------------------- |
| Leading address  | Context `default-account` (in `--by owner` mode); **required** in `--by provider` mode |
| `dseq`           | 0 — list all (no dseq filter)                                                |
| `gseq`           | 0 in list context (no filter); 1 when needed for a unique get                |
| `oseq`           | 0 in list context (no filter); 1 when needed for a unique get                |
| Trailing address | Empty — no filter on the trailing identity component                         |

#### 3.8.5 Per-Command Filter Scope

| Command                  | Max filter depth                               | `--by` support |
| ------------------------ | ---------------------------------------------- | -------------- |
| `query deployment`       | `[owner/]dseq`                                 | no             |
| `query deployment group` | `[owner/]dseq[/gseq]`                          | no             |
| `query market order`     | `[owner/]dseq[/gseq/oseq]`                     | no             |
| `query market bid`       | `[owner/]dseq[/gseq/oseq[/provider]]`          | yes            |
| `query market lease`     | `[owner/]dseq[/gseq/oseq[/provider]]`          | yes            |
| `query provider`         | `[address]`                                     | no             |
| `query cert`             | `[owner] [state]`                               | no             |
| `query audit`            | `[owner]`                                       | no             |
| `query escrow`           | `[owner[/dseq]]`                                | no             |
| `query escrow payment`   | `[owner[/dseq]]`                                | no             |
| `query escrow blocks-remaining` | `[owner/]dseq`                           | no             |

#### 3.8.6 Examples

**Deployment queries** (`akt query deployment`):

```bash
akt query deployment                           # List all deployments for default account
akt query deployment 12345                     # Get deployment dseq 12345 (owner from context)
akt query deployment akash1abc...              # List all deployments for that owner
akt query deployment akash1abc.../12345        # Get specific deployment
akt query deployment --state active            # DISABLED pending feedback (2026-07): use `akt query deployment active`
akt query deployment active                    # List active deployments (positional state keyword)
akt query deployment 12345 active              # Identity + state via the second positional argument
akt query deployment akash1abc.../12345 active # Same, with explicit owner
akt query deployment 12345 --state active      # DISABLED pending feedback (2026-07): use `akt query deployment 12345 active`
```

**Market lease queries — owner perspective** (default):

```bash
akt query market lease                         # List all leases for default account
akt query market lease 12345                   # List leases for dseq 12345 (owner from context)
akt query market lease 12345/1/1               # List leases for order 12345/1/1
akt query market lease akash1abc.../12345/1/1/akash1prov...  # Get specific lease
akt query market lease --state active          # DISABLED pending feedback (2026-07): use `akt query market lease active`
akt query market lease active                  # List active leases (positional state keyword)
akt query market lease 12345 active            # Identity + state via the second positional argument
akt query market bid open                      # List open bids (positional state keyword)
```

**Market lease queries — provider perspective** (`--by provider`):

```bash
akt query market lease --by provider akash1prov...              # List all leases for that provider
akt query market lease --by provider akash1prov.../12345        # List leases for provider + dseq
akt query market lease --by provider akash1prov.../12345/1/1/akash1owner...  # Get specific lease
```

**Other queries:**

```bash
akt query cert                                 # List certs for default account
akt query cert akash1abc...                    # List certs for that owner
akt query escrow 12345                         # List escrow accounts for dseq 12345 (owner from context)
akt query escrow akash1abc.../12345            # Specific escrow account
```

### 3.9 Interactive Prompt Patterns

Interactive prompts appear when a command requires user input and a TTY is attached. All prompts are suppressed (or resolved to defaults) when `--yes` is passed, no TTY is attached, or `--quiet` is set. Prompts are rendered on **stderr** so they never pollute piped stdout data.

#### 3.9.1 Prompt Types

**Confirmation prompt**: A yes/no question before a destructive or costly action. The default answer is always "no" (safe default). `--yes` / `-y` skips the prompt and answers "yes".

```
Close deployment 12345? This will terminate all active leases. [y/N]: _
```

**Single-select prompt**: The user picks exactly one option from a list. Arrow keys (or `j`/`k`) move the cursor; `Enter` selects. Used for account selection, keyring backend selection, and fork-vs-edit-parent.

```
Select keyring backend  ↑↓ move  enter confirm

  > [x]  os          System keyring (recommended)
    [ ]  file        File-based encrypted keyring
    [ ]  test        Unencrypted test keyring (development only)
```

**Multi-select prompt**: The user toggles items on/off and confirms the batch. Space toggles; Enter confirms. A "Select all" row is the first item. Used for network selection during bootstrap.

```
Select networks  ↑↓ move  space toggle  enter confirm

  > [x]  # Select all

    [x]  mainnet             akashnet-2        [3 rpc, 2 api, 1 grpc]
    [x]  testnet             testnet-02        [1 rpc, 1 api, 1 grpc]
    [x]  sandbox             sandbox-01        [1 rpc, 1 api, 1 grpc]

  q quit
```

**Value input prompt**: Free-form text or numeric input with an optional default. Used for deposit amounts, gas overrides, or custom names. Input is validated before acceptance.

```
Initial deposit amount [5000000uakt]: _
```

#### 3.9.2 Prompt Rendering Conventions

All interactive prompts follow these rendering rules:

| Rule | Detail |
|------|--------|
| **Cursor indicator** | `>` prefix (ASCII, from glyph registry `Cursor`) on the active row |
| **Selected item** | `[x]` checkbox (glyph `CheckboxOn`) in green |
| **Unselected item** | `[ ]` checkbox (glyph `CheckboxOff`) in dim |
| **Active row** | Subtle dark background highlight (`\033[48;5;236m`) |
| **Hint line** | Dim text showing available keys (e.g., `↑↓ move  space toggle  enter confirm`) |
| **Raw terminal mode** | Prompts use `golang.org/x/term` raw mode for immediate key response |
| **Cleanup on exit** | Terminal state is always restored via `defer term.Restore()`, even on `Ctrl+C` |
| **Stderr output** | All prompt rendering writes to stderr, not stdout |

#### 3.9.3 TTY Detection and Non-Interactive Fallback

Commands auto-detect whether a TTY is attached to stdin. When no TTY is present:

| Prompt type | Non-TTY behavior |
|-------------|-----------------|
| Confirmation | Treated as "no" unless `--yes` is set |
| Single-select | Uses the default option (first item or flag value) |
| Multi-select | Uses all items (the default-selected state) |
| Value input | Uses the default value; errors if no default and no flag override |

The `--interactive` / `-i` flag does **not** override TTY detection for prompts. If there is no TTY, prompts cannot render regardless of `-i`. The `-i` flag only affects TUI-mode launch and workflow execution mode selection (see §3.1).

#### 3.9.4 Fork-vs-Edit-Parent Flow

When `akt context edit <name>` modifies a network-level field (e.g., `--rpc`), and the context references a shared network, the user is prompted:

```
Network "mainnet" is shared by 2 contexts: prod, monitoring.
  1. Edit parent — change applies to all contexts using "mainnet"
  2. Fork — create a copy "mainnet-prod" for this context only

Select [1-2]: _
```

| Selection | Behavior |
|-----------|----------|
| **Edit parent** | Modifies the shared network definition. All referencing contexts see the change. |
| **Fork** | Creates a new network `<network>-<context>` with the modification applied. The current context switches to the forked network. Other contexts are unaffected. |

With `--fork-network` flag: skips the prompt and always forks.
With `--yes` flag: skips the prompt and always edits the parent (the less disruptive default for automation).

#### 3.9.5 Account Selection

When a command requires `--from` and no default account is configured in the context, the user is prompted to select from available keys in the context's keyring:

```
No default account set. Select an account:

    NAME         ADDRESS                                        TYPE
  > alice        akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu  local
    bob          akash1z3a2m5v6smf7m9gk3dxtrwep9xyeqahxqy6045  local
    hardware     akash1v3jkvemgd94xkmrddehhqutjwd682anh9zw2p2  ledger

Select [1-3]: _
```

Non-TTY fallback: errors with a message suggesting `--from <account>` or setting `default-account` in the context config.

#### 3.9.6 Transaction Confirmation

All `tx` commands show a confirmation prompt before broadcast (when TTY is attached and `--yes` is not set):

```
Transaction Summary
  Type:       MsgCloseDeployment
  Signer:     akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5lzv7xu
  Chain:      akashnet-2
  Gas:        auto (estimated: 150,000)
  Fee:        3.75 mAKT

Confirm broadcast? [y/N]: _
```

With `--generate-only`: no confirmation (tx is printed, not broadcast).
With `--dry-run`: no confirmation (simulation only).
With `--yes`: skips the prompt and broadcasts immediately.

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
| `console`  | A state-changing Console API operation     | Operation (create-deployment, close-deployment, etc.), dseq, result   |
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

### 5.6 Entry Writing Rules

The action log records entries for the following command categories:

| Command category | Logged | Entry type | When |
|---|---|---|---|
| `tx *` | Always | `tx` | After broadcast (success or failure). On success: includes tx hash, height, gas used. On failure: includes error message and result code. |
| `query *` | Never by default | `query` | Read-only queries are not state changes and are not recorded by default (see verbose row below). |
| Workflow commands (`deploy`, `update`, `close`) | Always | `workflow` | One entry per workflow step. Each entry includes the step name, result, and workflow run ID. |
| `provider *` (state-changing: `send-manifest`, `migrate-hostnames`, `migrate-endpoints`, `lease-shell`) | Always | `provider` | After the provider gateway operation completes (success or failure). Read-only provider queries (`status`, `lease-status`, `lease-logs`, `lease-events`, `get-manifest`) are not recorded. |
| `context *` | Always | `context` | After context management operation (switch, edit, create, delete). |
| Console API state changes (create/update/close deployment, create lease, deposit) | Always | `console` | After the Console API call completes (success or failure). Read-only Console queries are not recorded. |
| All commands | On failure | `error` | When any command fails. Includes original action type and error message. |
| `query` (read-only, no side effects) | When `-v` is set (future) | `query` | Verbose-mode query logging for debugging is planned but not yet implemented. Internal queries (e.g. by the sync engine) are never logged. |

The action logger is opened in the root command's `PersistentPreRunE` and closed in `PersistentPostRunE`. Commands retrieve it via `cliutil.ActionLogFromContext(cmd.Context())`.

### 5.7 Log Rotation

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

1. Query all deployments for each tracked account: `query deployment --owner <addr>`.
2. For each deployment, query leases: `query market lease --owner <addr> --dseq <dseq>`.
3. For each deployment, query bids: `query market bid --owner <addr> --dseq <dseq>`.
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

### 6.6 Workflow-to-Store Integration

Workflow commands (`akt deploy`, `akt update`, `akt close`) do **not** write to the local store directly. Instead, the sync engine detects the on-chain events produced by workflow transactions and updates the store through the normal event processing pipeline (§6.3).

This means there is a brief delay (typically 1-2 seconds) between a workflow completing and the store reflecting the new state. The workflow's output (DSEQ, lease details, endpoint URLs) is displayed directly from the transaction results and provider responses — it does not depend on the store.

If the sync engine is not running (e.g., no WebSocket connection), the store is reconciled on the next startup (§6.4).

### 6.7 Multi-Account Tracking

The sync engine tracks accounts configured in the context's `tracked-accounts` setting. By default, only the context's `default-account` is tracked. Users can add additional accounts to track deployments across multiple wallets within a single context.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tracked-accounts` | []string | `["<default-account>"]` | List of account names or addresses to sync. Default: only the default account. Set to `"*"` to track all accounts in the context's keyring. |

When `tracked-accounts` is `["*"]`, the sync engine tracks all accounts present in the current context's keyring. When a new key is added, the engine re-reconciles to pick up deployments from the new account.

The `tracked-accounts` field is context-specific (not shared via keyring or network). Each context can track a different subset of accounts.

---

## 7. Console API Specification

When a context's `auth-method` is `console-api`, deployment operations are routed through the [Akash Console Managed Wallet API](https://akash.network/docs/api-documentation/console-api/api-reference/) instead of signing and broadcasting transactions locally.

### 7.1 Authentication

All Console API requests require the `x-api-key` header. The API key is resolved in this order:

1. `--console-api-key` flag (highest priority, session only)
2. `AKT_CONSOLE_API_KEY` environment variable
3. The per-context stored credential (see below)
4. If none is available, the command fails with an error explaining how to configure a key.

**Per-context credential storage.** A Console API key can be stored on a context so that switching contexts switches Console identity with no manual credential juggling. The key is stored at:

```
<config-root>/contexts/<context-name>/console-api-key
```

- The file is created with `0600` permissions and contains only the key.
- The key is **never** written to `config.yaml` and is **never** printed by any command or recorded in logs; `akt context show` reports only whether a key is configured.
- Because the credential lives in the context's data directory, `akt context rename` moves it and `akt context delete` removes it (unless `--keep-data`).
- The credential is written via `akt context create --console-api-key ...` or `akt context edit --console-api-key ...`; passing an empty string via `akt context edit --console-api-key ""` removes it.
- Different contexts hold independent keys; Console calls always use the active context's key (subject to the flag/env overrides above).

API keys are created at [console.akash.network](https://console.akash.network) > Settings > API Keys.

### 7.2 Base URL

| Source              | Value                                  |
| ------------------- | -------------------------------------- |
| Default             | `https://console-api.akash.network`    |
| Context override    | `console-api-url` field in config.yaml |
| Flag override       | `--console-api-url`                    |

### 7.3 Endpoints

All requests include `Content-Type: application/json` and `x-api-key` headers.

The endpoints below cover the deployment lifecycle used by command and workflow routing (§7.4-§7.5). The client's full surface — user info, wallets, usage, provider/GPU/template catalogs, API keys, and provider-scoped JWTs — is documented per command in §2.9 and contract-tested against the vendored OpenAPI spec (`internal/console/testdata/openapi.json`).

#### `POST /v1/deployments` -- Create Deployment

| Field           | Type   | Required | Description                           |
| --------------- | ------ | -------- | ------------------------------------- |
| `data.sdl`      | string | yes      | SDL content as string                 |
| `data.deposit`  | number | yes      | Deposit in USD (minimum $0.50)        |

Returns `{ data: { dseq: string, manifest: string } }`.

#### `GET /v1/deployments` -- List Deployments

| Parameter | Type   | Required | Default | Description                  |
| --------- | ------ | -------- | ------- | ---------------------------- |
| `skip`    | number | no       | 0       | Number of deployments to skip |
| `limit`   | number | no       | 10      | Max deployments to return     |

Returns paginated list of deployments with leases and escrow state.

#### `GET /v1/deployments/{dseq}` -- Get Deployment

Path parameter: `dseq` (deployment sequence ID). Returns deployment details with leases and escrow.

#### `PUT /v1/deployments/{dseq}` -- Update Deployment

| Field      | Type   | Required | Description              |
| ---------- | ------ | -------- | ------------------------ |
| `data.sdl` | string | yes      | Updated SDL content      |

Returns updated deployment with leases and escrow.

#### `DELETE /v1/deployments/{dseq}` -- Close Deployment

Returns `{ data: { success: boolean } }`.

#### `GET /v1/bids?dseq={dseq}` -- Fetch Bids

Query parameter: `dseq` (required). Returns array of bids with provider details, pricing, and resource offers.

#### `POST /v1/leases` -- Create Lease

| Field      | Type     | Required | Description                              |
| ---------- | -------- | -------- | ---------------------------------------- |
| `manifest` | string   | yes      | Manifest JSON from deployment creation   |
| `leases`   | []object | yes      | Array of `{ dseq, gseq, oseq, provider }` |

Returns deployment with created leases and escrow state.

#### `POST /v1/deposit-deployment` -- Add Deposit

| Field          | Type   | Required | Description        |
| -------------- | ------ | -------- | ------------------ |
| `data.deposit` | number | yes      | Amount in USD      |
| `data.dseq`    | string | yes      | Deployment seq ID  |

#### `GET /v2/deployment-settings/{dseq}` -- Get Deployment Settings

Returns auto top-up configuration for a deployment. Settings are auto-created with auto top-up **enabled** by default.

#### `POST /v2/deployment-settings` -- Create Deployment Settings

| Field                    | Type    | Required | Description              |
| ------------------------ | ------- | -------- | ------------------------ |
| `data.dseq`              | string  | yes      | Deployment sequence ID   |
| `data.autoTopUpEnabled`  | boolean | no       | Enable auto top-up       |

### 7.4 Workflow Engine Integration

When a workflow runs in a context with `auth-method: console-api`, the workflow engine automatically routes `type: tx` steps through the Console API instead of building and broadcasting chain transactions locally.

**Routing rules for workflow steps:**

| Step type | `keyring` auth | `console-api` auth |
|---|---|---|
| `tx` | Build, sign, and broadcast transaction locally via chain client | Map message type to Console API endpoint (see §7.5 table). If the message type has no Console API mapping, abort with an unsupported-command error. |
| `query` | Query chain RPC/gRPC directly | Query chain RPC/gRPC directly (unchanged) |
| `wait` | Poll chain query | Poll chain query (unchanged) |
| `prompt` | Interactive prompt | Interactive prompt (unchanged) |
| `provider` | Provider gateway call (JWT/mTLS) | Not supported — Console API contexts do not interact with provider gateways directly. The Console API handles manifest submission internally during lease creation. |
| `foreach` | Iterate and execute nested step | Same, with nested step routing rules applied |

**Deposit handling**: `--deposit` accepts one unified syntax on both rails, parsed in one place (`internal/transport.ParseDeposit`) and translated per transport. The `usd` unit is case-insensitive and always wins over coin parsing.

| Form | Examples | `keyring` (chain rail) | `console-api` (console rail) |
|---|---|---|---|
| USD | `5usd`, `$5`, `5.50usd` | Error: `USD deposits require a console-api context; specify a coin amount like 5000000uakt` | Sent as USD in the Console API's `data.deposit` field (Console minimum: 0.50 USD) |
| Coin | `5000000uakt`, `5akt` | Attached to the deployment as the coin deposit | Error: `console deposits are in USD; use e.g. 5usd` |
| Bare number | `5`, `5.50` | Error (coins require a denomination — the historical chain behavior, with cross-rail guidance) | Interpreted as USD, same as `5usd` |
| `auto` / empty | `auto` | Chain-minimum deployment deposit, queried on chain | Error: an explicit USD deposit is required |

**Manifest handling**: The Console API's `POST /v1/deployments` returns a `manifest` field in the response. The workflow engine stores this value and passes it to `POST /v1/leases` when creating leases, instead of calling the provider's `send-manifest` endpoint directly.

### 7.5 Command Routing

When a context uses `console-api` auth, the following commands are routed through the Console API instead of direct chain transactions:

| CLI Command                          | Console API Endpoint                 | Notes                                    |
| ------------------------------------ | ------------------------------------ | ---------------------------------------- |
| `akt tx deployment create <sdl>`     | `POST /v1/deployments`               | `--deposit` flag in USD                  |
| `akt tx deployment update <sdl>`     | `PUT /v1/deployments/{dseq}`         |                                          |
| `akt tx deployment close`            | `DELETE /v1/deployments/{dseq}`      |                                          |
| `akt query market bid <dseq>`        | `GET /v1/bids?dseq=`                 |                                          |
| `akt tx market lease create`         | `POST /v1/leases`                    | Requires manifest from deployment create |
| `akt tx escrow deposit`              | `POST /v1/deposit-deployment`        | `--deposit` flag in USD                  |
| `akt query deployment`               | `GET /v1/deployments`                | Paginated via `--skip`/`--limit`         |

Commands **not** listed above (e.g., `akt query bank balances`, `akt query staking validators`, `akt tx gov vote`) are **not supported** with `console-api` auth. They require direct chain access via `keyring` auth. Running an unsupported command with `console-api` auth produces an error:

```
Error: command "tx gov vote" is not supported with console-api auth.
Use a context with auth-method: keyring for this operation.
```

### 7.6 Error Handling

| HTTP Status | Handling                                                          |
| ----------- | ----------------------------------------------------------------- |
| 401         | Invalid or expired API key. Point the user at the key resolution chain (§7.1) and `akt console login`. |
| 402         | Insufficient funds in Console account.                            |
| 404         | Deployment not found (dseq does not exist or not owned by user).  |
| 429         | Rate limited. Retry with backoff (safe for every method: the request was rejected before processing). |
| 5xx         | Console API server error. Retry with backoff (max 3 attempts) for idempotent methods (GET/HEAD/PUT/DELETE) only. POST is never replayed on 5xx: the request may have been processed despite the error (e.g. a gateway 502 after a completed write), and replaying it could duplicate a deployment or a USD deposit. |

### 7.7 Differences from Keyring Auth

| Aspect             | `keyring` auth                      | `console-api` auth                        |
| ------------------ | ----------------------------------- | ----------------------------------------- |
| Key management     | Local (OS keyring, file, ledger)    | None (Console-managed)                    |
| Transaction signing | Local                              | Console backend                           |
| Deposit currency   | uakt                               | USD                                       |
| Payment method     | On-chain AKT                       | Credit card via Console                   |
| Supported commands | All tx + query                     | Deployment lifecycle + queries via chain  |
| Provider auth      | JWT / mTLS                         | Not applicable                            |
| Gas/fees           | Configurable per-context            | Managed by Console                        |

---

## 8. TUI Specification

The TUI incorporates the real-time monitoring functionality of [`aktop`](https://github.com/cloud-j-luna/aktop) -- a community-built terminal UI for Akash consensus and provider monitoring. The consensus view, validator voting view, provider fleet monitor, and governance parameters view are derived from `aktop` and integrated as first-class views in the `akt` TUI.

### 8.1 Application Shell Layout

The TUI uses a three-region layout that fills the terminal:

| Region | Height | Content |
|--------|--------|---------|
| **Header** | 1 line | App name, active context, chain-id, account, block height, sync status |
| **Main area** | fills remaining | Active view (resource list or detail pane) |
| **Status bar** | 1-3 lines (dynamic) | View-specific hints, connection info, global keybindings |

Header example: `akt  Context: mainnet (akashnet-2)  Account: alice  Block: 18234567  Synced`

**Status bar** is dynamically sized based on content. It renders up to three lines:

| Line | Content | Shown when |
|------|---------|------------|
| **Line 1** | View-specific keybinding hints (e.g., `<j/k> Scroll  <Enter> Detail  <d> Close`) | Always (changes per active view/tab) |
| **Line 2** | Connection info (RPC endpoint, WebSocket status) | Active view uses a network connection (e.g., monitor, sync) |
| **Line 3** | Global keybindings (`<:> Command  <Esc> Back  <Ctrl+c> Quit`) | Always |

Lines with no content are omitted — the status bar shrinks to 1 or 2 lines when Line 2 has nothing to show. The main area height adjusts accordingly.

Status bar example (3-line, on monitor view):
```
<1> Overview  <2> Validators  <3> Governance  <j/k> Scroll  <r> Refresh
RPC: rpc.akashnet.net:443  WS: connected
<:> Command  <Esc> Back  <Ctrl+c> Quit
```

Status bar example (2-line, on deployments list):
```
<j/k> Scroll  <Enter> Detail  <d> Close  <u> Update  <l> Logs  </> Filter
<:> Command  <Esc> Back  <Ctrl+c> Quit
```

### 8.2 Navigation Model

Navigation uses a **stack-based** model:

- **Resource selector**: Press `:` to open command palette, or number keys for quick access to common views.
- **Drill-down**: Press `Enter` on a list item to open its detail view.
- **Back**: Press `Esc` to go back to the previous view (pops the navigation stack).
- **Breadcrumb**: The header shows the current navigation path (e.g., `Deployments > 12345 > Leases`).

Navigation stack example: `Dashboard → Deployments List → Deployment #12345 → Lease Detail` (Esc pops each level).

### 8.3 Resource Views

#### 8.3.1 Deployments List View

**Columns**: DSEQ, State, Provider, Price/Block, Escrow Balance, Age.

**Actions**:
- `Enter` -- Open deployment detail view
- `d` -- Close deployment (with confirmation dialog)
- `u` -- Update deployment (prompts for SDL file path)
- `l` -- Open log viewer for the deployment's active lease
- `/` -- Focus the filter input (fuzzy search across all columns)
- `f` -- Cycle state filter: all -> active -> closed

**Sorting**: Click column header or press `s` then column key to sort. Default: DSEQ descending.

#### 8.3.2 Deployment Detail View

Shows deployment metadata (owner, created, deposit, escrow balance, version, SDL path, labels, notes), active lease details (provider, price, endpoints), and bid table (provider, price, state, audited).

**Actions**:
- `Esc` -- Back to deployments list
- `d` -- Close this deployment
- `u` -- Update this deployment
- `l` -- Stream logs from active lease
- `e` -- Stream events from active lease
- `s` -- Open shell into active lease
- `y` -- Toggle YAML/formatted view of raw on-chain data

#### 8.3.3 Leases List View

**Columns**: DSEQ, GSeq, OSeq, Provider, State, Price/Block, Age.

**Actions**: Enter detail, l logs, e events, s shell, w withdraw, / filter.

#### 8.3.4 Providers List View

**Columns**: Address, Host URI, Audited, Active Leases.

**Actions**: Enter detail, a attributes, / filter.

#### 8.3.5 Governance Proposals View

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

#### 8.3.6 Validators View

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

#### 8.3.7 Log Viewer

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

#### 8.3.8 Consensus Monitor View (from aktop)

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
- Proposer address and index
- Prevote/Precommit progress bars: `█`/`░` (40 chars wide), percentage (green if >= 66.7%, yellow otherwise), power fraction
- Validator vote grid: `●` (voted, green) / `○` (not voted, muted), line-wrapped to terminal width

**Refresh interval**: Configurable, default 1s. Supports fast mode (250ms).

**Components:** bubbles/progress (vote progress bars), custom (vote grid, consensus state display).

#### 8.3.9 Validator Voting View (from aktop)

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

**Components:** bubbles/table (validator list with selection), custom (signing history bar, proposer indicator).

#### 8.3.10 Provider Fleet Monitor View (from aktop)

> This view is available both in the main TUI (`akt` with no subcommand) and as the Provider dashboard in `akt monitor` / `akt monitor provider`. See [§2.6](#26-monitor-command).

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

**Components:** bubbles/progress (scan progress bar), bubbles/table (provider list, node detail table), custom (version dot chart).

#### 8.3.11 Governance Parameters View (from aktop)

Module-by-module governance parameter browsing. The right pane renders pretty-formatted key-value output (same `Render*Params()` functions as CLI `--output pretty`) instead of raw JSON. This follows the Pretty/TUI visual parity rule (§10.8).

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt  Context: mainnet > Governance Params                                   │
├──────────────────────────────────────────────────────────────────────────────┤
│  Modules          │ Staking Parameters                                       │
│                   │   Unbonding Time:      21 days                           │
│    Governance     │   Max Validators:      100                               │
│    Minting        │   Max Entries:         7                                 │
│  ▶ Staking        │   Historical Entries:  10,000                            │
│    Slashing       │   Bond Denom:          uakt                              │
│    Distribution   │   Min Commission:      0%                                │
│    Auth           │                                                          │
│    Bank           │                                                          │
│    Deployment     │                                                          │
│    Market         │                                                          │
│    Transfer       │                                                          │
│    IBC            │                                                          │
│    Crisis         │                                                          │
│                   │                                                          │
├──────────────────────────────────────────────────────────────────────────────┤
│  <j/k> Module  <h/l> Scroll params  <r> Refresh  <?> Help                    │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Data sources**:
- Direct module endpoints: `/cosmos/gov/v1beta1/params/voting`, `/cosmos/mint/v1beta1/params`, etc.
- Generic params subspace: `/cosmos/params/v1beta1/subspaces` + `/cosmos/params/v1beta1/params?subspace=...&key=...`

**Modules displayed** (14 total):

| Module | CLI `Render*Params()` | TUI governance tab | Source |
|---|---|---|---|
| Staking | yes | yes | `/cosmos/staking/v1beta1/params` |
| Governance | yes | yes | `/cosmos/gov/v1/params` |
| Minting | yes | yes | `/cosmos/mint/v1beta1/params` |
| Slashing | yes | yes | `/cosmos/slashing/v1beta1/params` |
| Distribution | yes | yes | `/cosmos/distribution/v1beta1/params` |
| Auth | yes | yes | `/cosmos/auth/v1beta1/params` |
| Deployment | yes | yes | `/akash/deployment/v1beta4/params` |
| Market | yes | yes | `/akash/market/v1beta5/params` |
| Wasm | yes | yes | `/cosmwasm/wasm/v1/codes/params` |
| Oracle | yes | yes | `/akash/oracle/v2/params` |
| Bank | TUI-only | yes | `/cosmos/bank/v1beta1/params` |
| Transfer | TUI-only | yes | `/ibc/apps/transfer/v1/params` |
| IBC | TUI-only | yes | `/ibc/core/client/v1/params` |
| Crisis | TUI-only | yes | `/cosmos/params/v1beta1/params?subspace=crisis&key=ConstantFee` |

"CLI `Render*Params()`" means the module has a registered `PrettyFormatter` for `akt query <module> params`. "TUI-only" modules are displayed in the TUI governance tab via `RenderModuleParamsFromJSON()` but don't have a standalone CLI query command in akt (their params are queried via the generic Cosmos SDK params subspace).

**Refresh interval**: 5 minutes.

**Components:** bubbles/list (module selector), bubbles/viewport (parameter display).

#### 8.3.12 Oracle/BME Monitor View

Combined oracle price and BME state monitoring. Available as the Oracle/BME dashboard in `akt monitor`, `akt monitor oracle`, or `akt monitor bme` (the latter two are aliases). The dashboard uses a two-column layout: Oracle data on the left, BME data (status, vault, ledger) on the right.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  akt monitor - Akash Network  Network  Provider  [Oracle/BME]                │
├──────────────────────────────────────┬───────────────────────────────────────┤
│  Aggregated Prices                   │  BME Status                           │
│                                      │                                       │
│  DENOM   TWAP        SOURCES         │  Status:           Healthy            │
│  uakt    0.003125    4               │  Mints:            Allowed            │
│  uatom   6.850000    3               │  Refunds:          Allowed            │
│                                      │  Collateral Ratio: 1.523              │
│  Price Health                        │  Thresholds:                          │
│    Healthy:     yes                  │      Warn:         1.100              │
│    Min Sources: yes                  │      Halt:         1.050              │
│                                      │                                       │
│                                      │  Ledger                               │
│                                      │  ROUTE  ID  STATUS  BURNED  ...       │
├──────────────────────────────────────┴───────────────────────────────────────┤
│  <Tab> Dashboard  <j/k> Scroll  <r> Refresh  <?> Help                        │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Data sources**:
- `/akash/oracle/v2/aggregated-price/{denom}` — TWAP, median, min/max, source count, deviation, health
- `/akash/oracle/v2/prices` — Price feed history with pagination
- `/akash/bme/v1/status` — Mint status, collateral ratio, thresholds, mints/refunds allowed
- `/akash/bme/v1/vault-state` — Balances, total burned, total minted, remint credits
- `/akash/bme/v1/ledger` — Recent ledger entries

**Refresh intervals**: Oracle aggregated prices every 30s, BME status/vault every 30s, price history and ledger every 2m.

**Color coding**: Oracle health: Healthy (green), Unhealthy (red). BME status: healthy (green), warning (yellow), halt CR (red), halt Oracle (red). Mints/Refunds: Allowed (green), Halted (red).

**Amount formatting**: All micro-denominated values scaled using `FormatCoin()` — same rules as pretty output (§10.7). Prices displayed with full decimal precision, trailing zeros stripped.

**Components:** bubbles/viewport (scrollable content panels), shared pretty.Render* functions for CLI/TUI parity.

#### 8.3.13 Additional Views

The following views follow the same list/detail pattern as above:

- **Certificates**: List with Serial, State, Owner. Detail shows certificate content, expiry.
- **Escrow Accounts**: List with ID, Owner, State, Balance. Detail shows payment history.
- **Orders**: List with DSEQ, GSeq, OSeq, State. Detail shows order spec and bids.
- **Bids**: List with DSEQ, Provider, Price, State. Detail shows provider attributes.
- **Wasm Contracts**: List with Address, Code ID, Label. Detail shows contract info, state queries.
- **IBC Channels**: List with Channel ID, Port, Counterparty, State.

### 8.4 Command Palette

Activated with `:` (colon) or `Ctrl+P`, the command palette is a centered floating overlay that provides case-insensitive substring search across all available commands. Both keybindings open the same palette.

The palette is split into two parts:

1. **Input line** (top): A single-line text input with `:` prompt. Typing filters the command list below in real time.
2. **Command list** (bottom): A scrollable list of all registered commands, filtered by the input value. Each entry shows the command name (left-aligned) and a short description (right-aligned, dimmed). The currently selected entry is highlighted with a `>` cursor prefix and a contrasting background.

```
┌────────────────────────────────────────────────────────────────────┐
│                                                                    │
│         ┌──────────────────────────────────────────────┐           │
│         │ :depl                                        │           │
│         ├──────────────────────────────────────────────┤           │
│         │ > Deployments      View all deployments      │           │
│         │   Deploy           Create new deployment     │           │
│         │   Deployment Close Close a deployment        │           │
│         │                                              │           │
│         └──────────────────────────────────────────────┘           │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

**Overlay sizing**: Width is ~60% of the terminal (minimum 50, maximum 80 columns). Height adapts to the number of visible filtered commands, capped at ~50% of the terminal height.

**Filtering**: Case-insensitive substring match against the command name and any registered aliases. For example, typing `dep` matches both "Deployments" and "Deploy". When the input is empty, all commands are shown.

**Keyboard handling while the palette is open**:

| Key | Action |
|-----|--------|
| Any printable character | Appends to input, re-filters list, resets cursor to first match |
| `Backspace` | Deletes last character, re-filters list |
| `j` or `Down` | Moves cursor down in the filtered list (wraps to top) |
| `k` or `Up` | Moves cursor up in the filtered list (wraps to bottom) |
| `Enter` | Selects the highlighted command and closes the palette |
| `Esc` | Closes the palette without action |

**Registered commands** (initial set, extensible):

| Category | Name | Description | Aliases | Key |
|-----------|------|-------------|---------|-----|
| navigation | Dashboard | Go to dashboard | home | Esc |
| navigation | Deployments | View all deployments | dep | 1 |
| navigation | Leases | View leases | | 2 |
| navigation | Providers | View providers | prov | 3 |
| navigation | Monitor | Real-time network monitor | monitor, consensus | 4 |
| navigation | Governance | View governance proposals | gov | 5 |
| navigation | Staking | View validators and staking | val, validators | 6 |
| navigation | Query | Query commands panel | | q |
| navigation | Tx | Transaction commands panel | | t |
| navigation | Certificates | View certificates | cert | |
| navigation | Escrow | View escrow accounts | | |
| navigation | Orders | View orders | | |
| navigation | Bids | View bids | | |
| action | Deploy | Create new deployment | | |
| app | Quit | Quit application | exit | Ctrl+c |
| app | Help | Show help | ? | ? |

**Command dispatch**: When a command is selected, the TUI dispatches the corresponding action (navigate to a view, open a workflow, quit the application, etc.). Commands targeting views that are not yet implemented navigate to the dashboard.

**Keybinding configurability**: All palette navigation keys (`cursor-up`, `cursor-down`, `select`, `back`) are resolved through the configurable `KeyMap` (see section 8.6). When `tui.keybindings` is `custom`, the palette respects user overrides from `tui.custom-keybindings`. The palette receives its bindings via the `PaletteKeys` struct, which the root App populates from the global `KeyMap` at startup.

### 8.5 Confirmation Dialog

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

### 8.6 Keybinding Specification

#### Default Keybindings (vim-style)

**Global**:
| Key           | Action                                  |
| ------------- | --------------------------------------- |
| `Ctrl+c`      | Quit application                        |
| `1`           | Deployments view                        |
| `2`           | Leases view                             |
| `3`           | Providers view                          |
| `4`           | Monitor view                            |
| `5`           | Governance view                         |
| `6`           | Staking view                            |
| `q`           | Open query commands panel               |
| `t`           | Open transaction commands panel         |
| `Ctrl+p`      | Open command palette (same as `:`)      |
| `:`           | Open command palette (same as `Ctrl+p`) |
| `?`           | Toggle help overlay                     |
| `Esc`         | Go back / close overlay / cancel        |
| `7`-`9`       | Reserved for future views               |
| `Ctrl+r`      | Force refresh current view              |
| `Tab`         | Cycle between monitor dashboards (in monitor view) or panes (in split view) |
| `Shift-Tab`   | Cycle to previous monitor dashboard     |

**Note:** When the monitor view (4) is active, `1`/`2`/`3` switch sub-tabs within the Network dashboard instead of navigating to global views. All other number keys retain their global behavior.

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
| `r`          | Monitor (all dashboards)         | Manual refresh                             |

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

### 8.7 Theme System

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

### 8.8 TUI Component Hierarchy (bubbletea models)

```
App (root model)
├── Header              (lipgloss styled string)
├── Navigation          (manages view stack, breadcrumbs)
│   ├── Dashboard       (home view)
│   ├── ResourceView    (generic, parameterized by resource type)
│   │   ├── ResourceTable     (bubbles/table with custom cell renderers)
│   │   └── DetailPane        (bubbles/viewport with YAML/JSON toggle)
│   ├── MonitorView      (hub: Tab/Shift-Tab cycles dashboards)
│   │   ├── NetworkDashboard   (from aktop: consensus, validators, governance)
│   │   │   ├── ConsensusView    (bubbles/progress for vote bars, custom vote grid)
│   │   │   ├── ValidatorView    (bubbles/table, custom signing history bar)
│   │   │   └── GovernanceView   (bubbles/list for modules, bubbles/viewport for params)
│   │   ├── ProviderDashboard  (from aktop: provider fleet health)
│   │   │   ├── ScanProgress     (bubbles/progress)
│   │   │   ├── VersionDist      (custom dot visualization)
│   │   │   ├── ProviderTable    (bubbles/table)
│   │   │   └── ProviderDetail   (info + bubbles/table for nodes)
│   │   └── OracleBMEDashboard (combined oracle prices + BME state)
│   │       ├── AggregatedSection (bubbles/viewport for scrollable content)
│   │       ├── BMEStatusSection  (pretty.RenderBMEStatus — shared with CLI)
│   │       ├── VaultSection      (pretty.RenderBMEVault — shared with CLI)
│   │       └── LedgerSection     (pretty.RenderBMELedger — shared with CLI)
│   ├── LogViewer       (bubbles/viewport with auto-scroll, service filter)
│   └── ...
├── CommandPalette      (overlay: bubbles/textinput + bubbles/list, activated by : or Ctrl+P)
├── ConfirmDialog       (overlay: transaction confirmation)
├── HelpOverlay         (overlay: bubbles/help keybinding reference)
└── StatusBar           (1-3 dynamic lines: view hints, connection info, global keys)
```

---

## 9. Plugin System Specification

### 9.1 Plugin Discovery

Plugins are discovered by scanning for executables matching the pattern `akt-<name>` in:

1. `~/.config/akt/plugins/` (local plugin directory)
2. Directories listed in `plugins.paths` config
3. `$PATH` directories

Discovery order determines precedence (first match wins). Plugins listed in `plugins.disabled` are skipped.

### 9.2 Plugin Execution

When `akt <name> [args...]` is invoked and `<name>` does not match a built-in command:

1. Search for a plugin named `akt-<name>`.
2. If found, execute it as a subprocess.
3. Pass all remaining arguments (`args...`) to the plugin.
4. Set `AKT_*` environment variables for context information.
5. Inherit stdin, stdout, stderr from the parent process.
6. Exit with the plugin's exit code.

### 9.3 Plugin Environment Variables

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

### 9.4 Plugin Manifest (Optional)

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

### 9.5 Plugin Management Commands

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

### 9.6 Security Considerations

- Plugins are unsigned executables. Users are responsible for trusting plugin sources.
- The `akt plugin install` command warns about running untrusted code.
- Plugins cannot modify the `akt` config or store directly (they only receive environment variables).
- A future version may add plugin signing and a plugin registry.

---

## 10. Output Format Specification

Query commands use a **registry-based pretty output system** (`internal/output/pretty/`). Each protobuf response type has a registered formatter that controls how it is displayed. The `--output` (`-o`) flag controls the output format.

### 10.1 Output Behavior Matrix

| `--output` (`-o`) | Behavior |
|-------------------|----------|
| `pretty` (default) | Pretty tables with lipgloss colors, state coding, sections |
| `json`            | Machine-readable JSON (compact, no colors, no formatting) |
| `yaml`            | Machine-readable YAML (no colors, no formatting) |

When `--output pretty` (the default), output is styled using **lipgloss**. Colors are disabled automatically when stdout is not a TTY or when the `NO_COLOR` environment variable is set.

### 10.1.1 Stream Separation (stdout vs stderr)

All CLI output follows strict stream separation for scriptability:

| Stream | Content |
|--------|---------|
| **stdout** | Data output only: query results, transaction results, exported store data, version string, completion scripts. This is the machine-parseable payload. |
| **stderr** | Everything else: error messages, warnings, progress indicators, spinners, status lines, verbose/debug logging, confirmation prompts, and informational messages (e.g., "Broadcasting transaction..."). |

This separation ensures that piping and redirection work correctly:

```bash
# Data goes to file, progress/errors visible on terminal
akt query deployment 12345 -o json > deployment.json

# Suppress informational output, keep only data
akt query deployment 12345 -o json 2>/dev/null

# Chain commands reliably
akt query deployment 12345 -o json | jq '.deployment.state'
```

When `--quiet` is set, stderr informational output (progress, status lines) is suppressed; only errors are emitted to stderr. When `--verbose` is set, additional operational detail is emitted to stderr.

### 10.2 Dispatch Architecture

Every query command calls `pretty.PrintQueryResult(cmd, cctx, msg)`:

1. Read `--output` flag from `cmd`.
2. If `json` → delegate to `cctx.PrintProto(msg)` with JSON output format.
3. If `yaml` → delegate to `cctx.PrintProto(msg)` with text/YAML output format.
4. If `pretty` (default) → look up a `PrettyFormatter` for `msg` by its protobuf full name.
   - Found → call `formatter.Format(w, cmd, cctx, msg)`.
   - Not found → fall back to JSON output via `cctx.PrintProto(msg)`.

This means unregistered protobuf types automatically get JSON output -- no regressions when new types are added.

### 10.3 Pretty Table Format

Default when `--output pretty` (or omitted).

**List results** use tabwriter-aligned columns:

- Headers in bold (lipgloss).
- State columns color-coded (see 10.6).
- Key identifiers (DSEQ, moniker, proposal ID) in bold.
- Addresses always displayed in full. Never truncated -- addresses are machine-parseable identifiers and truncation risks ambiguity.
- Micro-denominated amounts (`u`-prefixed denoms) are converted to the most readable unit: base (e.g., AKT), milli (mAKT), or micro (uAKT). Trailing zeros are always stripped. Applies to all `u`-prefixed denoms (uakt, uatom, uosmo, etc.).
- `DecCoin` prices shown with denom suffix (e.g., `12.500000 uakt`).
- Block heights formatted with comma grouping (e.g., `1,234,567`).

```
  DSEQ       OWNER                                          STATE    GROUPS  CREATED AT
  12345      akash1abcdefghijklmnopqrstuvwxyz012345678901    active   2       1,234,567
  12346      akash1abcdefghijklmnopqrstuvwxyz012345678901    closed   1       1,234,500
```

**Single-item results** use grouped key-value pairs with section headers:

```
Deployment
  DSEQ:       12345
  Owner:      akash1abcdef...full
  State:      active
  Hash:       A1B2C3...
  Created At: 1,234,567

Group 1: "web"
  State:      open
  Resources:
    CPU:      0.5 units × 1
    Memory:   512 Mi × 1
    Storage:  512 Mi × 1
    Price:    12.500000 uakt/block

Escrow
  State:      open
  Balance:    5.000000 AKT
  Spent:      0.500000 AKT
```

### 10.4 Pretty JSON Format

Used when `--output json`. Output is machine-readable JSON via Cosmos SDK `clientCtx.PrintProto()` (no colors, no syntax highlighting):

| Element | Color |
|---------|-------|
| Keys | Cyan |
| Strings | Green |
| Numbers | Yellow |
| Booleans | Magenta |
| Null | Dim/gray |
| Braces/brackets | Default |

Field names use `snake_case`. Protobuf messages are serialized using the Cosmos SDK JSON codec.

### 10.5 Pretty YAML Format

Used when `--output yaml`. Output is machine-readable YAML via Cosmos SDK `clientCtx.PrintProto()` (no colors, no syntax highlighting):

| Element | Color |
|---------|-------|
| Keys | Cyan |
| Strings | Green |
| Numbers | Yellow |
| Booleans | Magenta |
| Null | Dim/gray |
| Document markers (`---`) | Dim/gray |

Same field naming as JSON (`snake_case`).

### 10.6 State Color Mapping

All state fields across Akash and Cosmos SDK types use a consistent color scheme:

| Color  | States |
|--------|--------|
| Green  | `active`, `open`, `bonded`, `passed`, `valid` |
| Yellow | `paused`, `insufficient_funds`, `overdrawn`, `unbonding`, `voting_period`, `deposit_period` |
| Red    | `closed`, `lost`, `unbonded`, `rejected`, `failed`, `jailed`, `revoked` |
| Gray   | `invalid`, `unspecified` |

### 10.7 Amount Formatting

| Input | Display |
|-------|---------|
| `5000000uakt` | `5 AKT` |
| `5300000uakt` | `5.3 AKT` |
| `3000uakt` | `3 mAKT` |
| `500uakt` | `500 uAKT` |
| `DecCoin{Amount: 12.5, Denom: "uakt"}` | `12.5 uakt` (price context — DecCoins keep their precision, trailing zeros stripped) |
| `[]Coin` (multiple denoms) | One row per denom |
| Zero amounts | `0 AKT` (not omitted) |

Any `u`-prefixed denom (the standard Cosmos micro-denomination convention) is automatically scaled to the most readable unit. Thresholds: >= 1,000,000 micro → base denom (e.g., `5.3 AKT`); >= 1,000 micro → milli denom (e.g., `3 mAKT`); < 1,000 micro → micro denom (e.g., `500 uAKT`). Trailing zeros are always stripped. Non-micro denoms and IBC denoms (`ibc/...`) are shown as-is.

### 10.8 Pretty/TUI Visual Parity

The pretty-printed output of a single-shot CLI command (e.g. `akt q bme status`) and the corresponding section in the TUI or `akt monitor` dashboard **must be visually identical**. Both code paths must call the same shared `Render*` functions from `internal/output/pretty/`.

This rule applies to every resource type that appears in both contexts: BME status, BME ledger, oracle prices, and any future query result displayed in the monitor dashboards. The `Render*` functions return strings and take only the protobuf response type as input — no `*cobra.Command` or `sdkclient.Context` dependency — so they are callable from both the CLI formatter layer and the TUI view layer.

**Never duplicate formatting logic in the TUI.** If a pretty formatter exists for a query result, the TUI must delegate to it.

### 10.9 Address Formatting

Addresses are **always displayed in full**. Never truncated or shortened by default. Addresses are machine-parseable identifiers; truncation risks ambiguity and breaks copy-paste workflows. Users who need shorter output can pipe through `cut` or `jq`.

| Context | Format | Example |
|---------|--------|---------|
| Table column | Full | `akash1abcdefghijklmnopqrstuvwxyz012345678901` |
| Detail view | Full | `akash1abcdefghijklmnopqrstuvwxyz012345678901` |
| Validator operator | Full | `akashvaloper1abcdefghijklmnopqrstuvwxyz0123456` |

### 10.10 Per-Module Formatter Specification

#### Deployment

**`QueryDeploymentsResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| DSEQ | `Deployment.ID.DSeq` | Bold |
| OWNER | `Deployment.ID.Owner` | Full address |
| STATE | `Deployment.State` | Color-coded |
| GROUPS | `len(Groups)` | Count |
| CREATED AT | `Deployment.CreatedAt` | Comma-grouped height |

**`QueryDeploymentResponse`** (detail): Sections for Deployment (DSEQ, owner, state, hash, created), each Group (name, state, resources with CPU/memory/storage/price), and Escrow (state, balance, spent).

#### Market -- Bids

**`QueryBidsResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| PROVIDER | `Bid.ID.Provider` | Full address |
| DSEQ | `Bid.ID.DSeq` | Bold |
| GSEQ | `Bid.ID.GSeq` | |
| OSEQ | `Bid.ID.OSeq` | |
| PRICE/BLOCK | `Bid.Price` | Bold DecCoin |
| STATE | `Bid.State` | Color-coded |

#### Market -- Leases

**`QueryLeasesResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| DSEQ | `Lease.ID.DSeq` | Bold |
| PROVIDER | `Lease.ID.Provider` | Full address |
| GSEQ | `Lease.ID.GSeq` | |
| OSEQ | `Lease.ID.OSeq` | |
| PRICE/BLOCK | `Lease.Price` | DecCoin |
| STATE | `Lease.State` | Color-coded |
| CLOSED REASON | `Lease.Reason` | Shown only when closed, otherwise `-` |

#### Market -- Orders

**`QueryOrdersResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| DSEQ | `Order.ID.DSeq` | Bold |
| GSEQ | `Order.ID.GSeq` | |
| OSEQ | `Order.ID.OSeq` | |
| STATE | `Order.State` | Color-coded |
| CREATED AT | `Order.CreatedAt` | Comma-grouped height |

#### Bank

**`QueryAllBalancesResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| DENOM | `Balance.Denom` | `uakt` row bolded |
| AMOUNT | `Balance.Amount` | With AKT conversion for uakt |

**`QuerySpendableBalancesResponse`**: Same format as all balances.

#### Provider

**`QueryProvidersResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| OWNER | `Provider.Owner` | Full address |
| HOST URI | `Provider.HostURI` | |
| EMAIL | `Provider.Info.EMail` | |
| WEBSITE | `Provider.Info.Website` | |

**`QueryProviderResponse`** (detail): Sections for Provider (owner, host, email, website) and Attributes (key-value list).

#### Staking

**`QueryValidatorsResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| MONIKER | `Validator.Description.Moniker` | Bold |
| OPERATOR | `Validator.OperatorAddress` | Full address |
| STATUS | `Validator.Status` | Color-coded (bonded/unbonding/unbonded) |
| VOTING POWER | `Validator.Tokens` | AKT conversion, comma-grouped |
| COMMISSION | `Validator.Commission.Rate` | Percentage (e.g., `5.00%`) |

**Single validator** (detail): Sections for Identity (moniker, operator, consensus pubkey, jailed), Staking (status, tokens, delegator shares), Commission (rate, max rate, max change rate), and Description (details, website, security contact).

#### Governance

**`QueryProposalsResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| ID | `Proposal.Id` | Bold |
| TITLE | `Proposal.Title` | Full text |
| STATUS | `Proposal.Status` | Color-coded |
| SUBMIT TIME | `Proposal.SubmitTime` | ISO date |
| VOTING END | `Proposal.VotingEndTime` | ISO date |

**Single proposal** (detail): Sections for Proposal (ID, title, status, type, description), Timeline (submit, deposit end, voting start, voting end), and Tally (yes/no/abstain/no_with_veto with percentages).

#### Escrow

**`QueryAccountsResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| SCOPE | `Account.ID.Scope` | |
| XID | `Account.ID.XID` | |
| OWNER | `AccountState.Owner` | Full address |
| STATE | `AccountState.State` | Color-coded |
| BALANCE | `AccountState.Funds` | AKT conversion |
| SPENT | `AccountState.Transferred` | AKT conversion |

#### Certificate

**`QueryCertificatesResponse`** (list):
| Column | Source | Format |
|--------|--------|--------|
| OWNER | Certificate owner | Full address |
| SERIAL | Certificate serial | |
| STATE | Certificate state | Color-coded |

#### Audit

**Audit list**: Table with AUDITOR, PROVIDER, and ATTRIBUTES columns.

#### Distribution

**Rewards**: Per-validator table with VALIDATOR, REWARD columns. Total rewards row at the bottom.
**Commission**: Key-value with amounts.
**Community pool**: Table of DENOM, AMOUNT.

#### Auth

**Account** (detail): Key-value with address, pub key type, account number, sequence.
**Accounts** (list): Table with ADDRESS, TYPE, ACCOUNT #, SEQUENCE.

#### Feegrant

**Grants** (list): Table with GRANTER, GRANTEE, ALLOWANCE TYPE, EXPIRATION.

#### WASM

**Code list**: Table with CODE ID, CREATOR, CHECKSUM.
**Contract** (detail): Key-value with address, code ID, admin, label, created.
**Contract state smart**: Pretty-printed JSON (results are arbitrary user-defined JSON).

#### Oracle

**Prices**: Table with ASSET, BASE, PRICE, TIMESTAMP.

#### BME

**Status/Vault**: Key-value sections with amounts.
**Ledger**: Table of entries.

#### Miscellaneous

**Slashing signing info**: Table with VALIDATOR, MISSED BLOCKS, JAILED UNTIL.
**Upgrade plan**: Key-value with name, height, info.
**Module address**: Simple `Module: <address>` output.

#### Module Parameters (all modules)

Every module's params query (`akt query <module> params`) has a registered pretty formatter. Durations are displayed human-readable (e.g., "21 days"), percentages as "33.4%", coins via `FormatCoins()`, booleans as color-coded "Yes"/"No". Each module's `Render<Module>Params()` function is public (exported) so the TUI governance tab can call the same renderers for visual parity (§10.8). A `RenderModuleParamsFromJSON(module, rawJSON)` bridge function unmarshals REST JSON for the TUI path.

**Staking** — `*staking.Params`:
| Field | Source | Format |
|-------|--------|--------|
| Unbonding Time | `UnbondingTime` | `FormatDuration()` (e.g., "21 days") |
| Max Validators | `MaxValidators` | Integer |
| Max Entries | `MaxEntries` | Integer |
| Historical Entries | `HistoricalEntries` | Comma-grouped via `FormatHeight()` |
| Bond Denom | `BondDenom` | Raw string |
| Min Commission | `MinCommissionRate` | Percentage (e.g., "0%") |

**Governance** — `*gov/v1.QueryParamsResponse`:
| Field | Source | Format |
|-------|--------|--------|
| Voting Period | `Params.VotingPeriod` | `FormatDuration()` |
| Min Deposit | `Params.MinDeposit` | `FormatCoins()` |
| Max Deposit Period | `Params.MaxDepositPeriod` | `FormatDuration()` |
| Quorum | `Params.Quorum` | Percentage |
| Threshold | `Params.Threshold` | Percentage |
| Veto Threshold | `Params.VetoThreshold` | Percentage |
| Expedited Voting Period | `Params.ExpeditedVotingPeriod` | `FormatDuration()` |
| Expedited Threshold | `Params.ExpeditedThreshold` | Percentage |
| Expedited Min Deposit | `Params.ExpeditedMinDeposit` | `FormatCoins()` |
| Burn Vote Quorum | `Params.BurnVoteQuorum` | `FormatBool()` |
| Burn Proposal Deposit | `Params.BurnProposalDepositPrevote` | `FormatBool()` |
| Burn Vote Veto | `Params.BurnVoteVeto` | `FormatBool()` |

**Minting** — `*mint.Params`:
| Field | Source | Format |
|-------|--------|--------|
| Mint Denom | `MintDenom` | Raw string |
| Inflation Rate Change | `InflationRateChange` | Percentage |
| Inflation Max | `InflationMax` | Percentage |
| Inflation Min | `InflationMin` | Percentage |
| Goal Bonded | `GoalBonded` | Percentage |
| Blocks Per Year | `BlocksPerYear` | Comma-grouped |

**Slashing** — `*slashing.Params`:
| Field | Source | Format |
|-------|--------|--------|
| Signed Blocks Window | `SignedBlocksWindow` | Comma-grouped |
| Min Signed Per Window | `MinSignedPerWindow` | Percentage |
| Downtime Jail Duration | `DowntimeJailDuration` | `FormatDuration()` |
| Slash Double Sign | `SlashFractionDoubleSign` | Percentage |
| Slash Downtime | `SlashFractionDowntime` | Percentage |

**Distribution** — `*distribution.Params`:
| Field | Source | Format |
|-------|--------|--------|
| Community Tax | `CommunityTax` | Percentage |
| Withdraw Addr Enabled | `WithdrawAddrEnabled` | `FormatBool()` |

**Auth** — `*auth.Params`:
| Field | Source | Format |
|-------|--------|--------|
| Max Memo Characters | `MaxMemoCharacters` | Comma-grouped |
| Tx Sig Limit | `TxSigLimit` | Integer |
| Tx Size Cost/Byte | `TxSizeCostPerByte` | Integer |
| Sig Verify ED25519 | `SigVerifyCostED25519` | Integer |
| Sig Verify Secp256k1 | `SigVerifyCostSecp256k1` | Integer |

**Deployment** — `*deployment/v1beta4.QueryParamsResponse`:
| Field | Source | Format |
|-------|--------|--------|
| Min Deposits | `Params.MinDeposits` | `FormatCoins()` |

**Market** — `*market/v1beta5.QueryParamsResponse`:
| Field | Source | Format |
|-------|--------|--------|
| Order Max Bids | `Params.OrderMaxBids` | Integer |
| Bid Min Deposits | `Params.BidMinDeposits` | `FormatCoins()` |

**Wasm** — `*wasm.Params`:
| Field | Source | Format |
|-------|--------|--------|
| Code Upload Access | `CodeUploadAccess.Permission` | Permission label |
| Upload Addresses | `CodeUploadAccess.Addresses` | Comma-joined or "any" |
| Instantiate Default | `InstantiateDefaultPermission` | Permission label |

**Oracle** — `*oracle/v2.QueryParamsResponse`:
| Field | Source | Format |
|-------|--------|--------|
| Sources | `Params.Sources` | Comma-joined list |
| Min Price Sources | `Params.MinPriceSources` | Integer |
| Max Staleness | `Params.MaxPriceStalenessPeriod` | `FormatDuration()` |
| TWAP Window | `Params.TwapWindow` | `FormatDuration()` |
| Max Deviation | `Params.MaxPriceDeviationBps` | Integer + " bps" |
| Price Retention | `Params.PriceRetention` | `FormatDuration()` |
| Prune Epoch | `Params.PruneEpoch` | Raw string |
| Max Prune/Epoch | `Params.MaxPrunePerEpoch` | Comma-grouped |

### 10.11 Transaction Result Formatting

Transaction commands use the same registry-based pretty output system as query commands. The `--output` flag defaults to `pretty` for both `tx` and `query` commands, providing a consistent user experience.

#### 10.11.1 Output Modes

| `--output` | Behavior |
|---|---|
| `pretty` (default) | Two-section layout: common summary + message-specific detail |
| `json` | Raw `TxResponse` JSON via `cctx.PrintProto()` |
| `yaml` | Raw `TxResponse` YAML via `cctx.PrintProto()` |

#### 10.11.2 Two-Section Layout

Every transaction result in pretty mode renders two sections separated by a blank line.

**Section 1: Transaction Summary**

Common to all transactions. Uses the same `Section()` + `KV()` formatting as query detail views.

| Field | Source | Format |
|---|---|---|
| Hash | `TxResponse.TxHash` | Full hex string, bold |
| Signer | First entry from `tx.AuthInfo.SignerInfos`, resolved to bech32 | Full address |
| Height | `TxResponse.Height` | Comma-grouped via `FormatHeight()` |
| Gas Used | `TxResponse.GasUsed` / `TxResponse.GasWanted` | `used / wanted`, both comma-grouped |
| Fee | `tx.AuthInfo.Fee.Amount` | `FormatCoins()` (micro-denom scaling) |
| Status | `TxResponse.Code` | Green "success" if code=0; Red "failed: `<RawLog>`" otherwise |

The section header is `Transaction`, rendered with `Section()` (bold + underline).

**Section 2: Message Detail**

Each message in the transaction body (`tx.Body.Messages`) is rendered using a registered `TxPrettyFormatter` for that message's protobuf type URL.

**Single-message transactions** (1 message): The message detail section renders directly with a descriptive section header (e.g., "Send", "Deployment Created", "Delegate").

**Multi-message transactions** (2+ messages): Each message renders as a numbered sub-section with prefix: "Message N: \<title\>" (e.g., "Message 1: Withdraw Rewards", "Message 2: Delegate").

**Unregistered message types**: If no `TxPrettyFormatter` is registered for a message type, the message is rendered as syntax-highlighted JSON of the decoded message body (using `WriteHighlightedJSON`).

**Failed transactions**: When `TxResponse.Code != 0`, the message detail section still renders (showing what was attempted), but the section header appends " (failed)" in red for multi-message transactions: "Message 1: Send (failed)". For single-message transactions, the failure is already visible in the Status field of section 1.

#### 10.11.3 TxPrettyFormatter Interface

```go
// TxPrettyFormatter formats a transaction message for pretty output.
// It receives the decoded message, the full TxResponse (for events/logs),
// and the message index within the transaction.
type TxPrettyFormatter interface {
    // FormatTx renders the message-specific detail section.
    // The common transaction summary (section 1) is rendered by the caller.
    FormatTx(w io.Writer, cmd *cobra.Command, cctx client.Context, msg sdk.Msg, resp *sdk.TxResponse, msgIndex int) error

    // Title returns the human-readable section header for this message type.
    // Examples: "Send", "Deployment Created", "Delegate", "Vote"
    Title() string
}
```

Registration follows the same pattern as query formatters:

```go
func RegisterTx(msg sdk.Msg, formatter TxPrettyFormatter)
func LookupTx(msg sdk.Msg) (TxPrettyFormatter, bool)
```

A convenience adapter is provided for simple cases:

```go
type TxPrettyFormatterFunc struct {
    TitleStr  string
    FormatFn  func(w io.Writer, cmd *cobra.Command, cctx client.Context, msg sdk.Msg, resp *sdk.TxResponse, msgIndex int) error
}
```

#### 10.11.4 Dispatch Flow

`PrintTxResult(cmd, cctx, txResponse)` is called by all `tx` commands after broadcast:

1. Read `--output` flag.
2. If `json` → `cctx.PrintProto(txResponse)`.
3. If `yaml` → `cctx.PrintProto(txResponse)` with YAML format.
4. If `pretty`:
   a. Render Section 1 (common summary) from `TxResponse` fields.
   b. Decode `TxResponse.Tx` to extract messages.
   c. For each message, look up `TxPrettyFormatter` by proto type.
   d. Render Section 2 for each message (registered formatter or JSON fallback).

#### 10.11.5 Per-Module Message Formatter Specification

##### Bank

**`MsgSend`** — Title: "Send"

| Field | Source | Format |
|---|---|---|
| From | `MsgSend.FromAddress` | Full address |
| To | `MsgSend.ToAddress` | Full address |
| Amount | `MsgSend.Amount` | `FormatCoins()` |

**`MsgMultiSend`** — Title: "Multi Send"

| Field | Source | Format |
|---|---|---|
| Inputs | `MsgMultiSend.Inputs` | One row per input: address + `FormatCoins()` |
| Outputs | `MsgMultiSend.Outputs` | One row per output: address + `FormatCoins()` |

##### Deployment

**`MsgCreateDeployment`** — Title: "Deployment Created"

| Field | Source | Format |
|---|---|---|
| Owner | Message + events | Full address |
| DSEQ | Events (`akash.deployment.*.EventDeploymentCreated`) | Bold |
| Deposit | `MsgCreateDeployment.Deposit` | `FormatCoin()` |
| Groups | `len(MsgCreateDeployment.Groups)` | Count |

**`MsgUpdateDeployment`** — Title: "Deployment Updated"

| Field | Source | Format |
|---|---|---|
| Owner | Message | Full address |
| DSEQ | `MsgUpdateDeployment.ID.DSeq` | Bold |

**`MsgCloseDeployment`** — Title: "Deployment Closed"

| Field | Source | Format |
|---|---|---|
| Owner | Message | Full address |
| DSEQ | `MsgCloseDeployment.ID.DSeq` | Bold |

**`MsgCloseGroup`** — Title: "Group Closed"

| Field | Source | Format |
|---|---|---|
| Owner | Message | Full address |
| DSEQ | Message | Bold |
| GSEQ | Message | |

**`MsgPauseGroup`** — Title: "Group Paused"

Same fields as MsgCloseGroup.

**`MsgStartGroup`** — Title: "Group Started"

Same fields as MsgCloseGroup.

##### Market

**`MsgCreateBid`** — Title: "Bid Created"

| Field | Source | Format |
|---|---|---|
| Provider | Message | Full address |
| DSEQ | Message | Bold |
| GSEQ | Message | |
| OSEQ | Message | |
| Price | `MsgCreateBid.Price` | `FormatDecCoin()` |

**`MsgCloseBid`** — Title: "Bid Closed"

| Field | Source | Format |
|---|---|---|
| Provider | Message | Full address |
| DSEQ | Message | Bold |
| GSEQ | Message | |
| OSEQ | Message | |

**`MsgCreateLease`** — Title: "Lease Created"

| Field | Source | Format |
|---|---|---|
| Owner | Message | Full address |
| DSEQ | Message | Bold |
| GSEQ | Message | |
| OSEQ | Message | |
| Provider | Message | Full address |

**`MsgWithdrawLease`** — Title: "Lease Withdrawn"

Same fields as MsgCreateLease.

**`MsgCloseLease`** — Title: "Lease Closed"

Same fields as MsgCreateLease.

##### Provider

**`MsgCreateProvider`** — Title: "Provider Created"

| Field | Source | Format |
|---|---|---|
| Owner | Message | Full address |
| Host URI | Message | |
| Attributes | Message | Key-value list |

**`MsgUpdateProvider`** — Title: "Provider Updated"

Same fields as MsgCreateProvider.

##### Certificate

**`MsgCreateCertificate`** — Title: "Certificate Published"

| Field | Source | Format |
|---|---|---|
| Owner | Message | Full address |
| Serial | Events | |

**`MsgRevokeCertificate`** — Title: "Certificate Revoked"

| Field | Source | Format |
|---|---|---|
| Owner | Message | Full address |
| Serial | Message | |

##### Audit

**`MsgSignProviderAttributes`** — Title: "Attributes Signed"

| Field | Source | Format |
|---|---|---|
| Auditor | Message | Full address |
| Provider | Message | Full address |
| Attributes | Message | Key-value list |

**`MsgDeleteProviderAttributes`** — Title: "Attributes Deleted"

| Field | Source | Format |
|---|---|---|
| Auditor | Message | Full address |
| Provider | Message | Full address |
| Keys | Message | Comma-separated list |

##### Staking

**`MsgDelegate`** — Title: "Delegate"

| Field | Source | Format |
|---|---|---|
| Delegator | Message | Full address |
| Validator | Message | Full address |
| Amount | Message | `FormatCoin()` |

**`MsgBeginRedelegate`** — Title: "Redelegate"

| Field | Source | Format |
|---|---|---|
| Delegator | Message | Full address |
| From | `SrcValidatorAddress` | Full address |
| To | `DstValidatorAddress` | Full address |
| Amount | Message | `FormatCoin()` |
| Completion | Events | ISO timestamp |

**`MsgUndelegate`** — Title: "Undelegate"

| Field | Source | Format |
|---|---|---|
| Delegator | Message | Full address |
| Validator | Message | Full address |
| Amount | Message | `FormatCoin()` |
| Completion | Events | ISO timestamp |

**`MsgCancelUnbondingDelegation`** — Title: "Cancel Unbonding"

| Field | Source | Format |
|---|---|---|
| Delegator | Message | Full address |
| Validator | Message | Full address |
| Amount | Message | `FormatCoin()` |

**`MsgCreateValidator`** — Title: "Validator Created"

| Field | Source | Format |
|---|---|---|
| Operator | Message | Full operator address |
| Moniker | Message | |
| Self-Delegation | Message | `FormatCoin()` |
| Commission | Message | Percentage |

**`MsgEditValidator`** — Title: "Validator Edited"

| Field | Source | Format |
|---|---|---|
| Operator | Message | Full operator address |
| Moniker | Message | Shown if changed |
| Commission | Message | Percentage, shown if changed |

##### Distribution

**`MsgWithdrawDelegatorReward`** — Title: "Withdraw Rewards"

| Field | Source | Format |
|---|---|---|
| Delegator | Message | Full address |
| Validator | Message | Full address |
| Rewards | Events (`withdraw_rewards.amount`) | `FormatCoins()` |

**`MsgWithdrawValidatorCommission`** — Title: "Withdraw Commission"

| Field | Source | Format |
|---|---|---|
| Validator | Message | Full address |
| Commission | Events | `FormatCoins()` |

**`MsgSetWithdrawAddress`** — Title: "Set Withdraw Address"

| Field | Source | Format |
|---|---|---|
| Delegator | Message | Full address |
| Withdraw Address | Message | Full address |

**`MsgFundCommunityPool`** — Title: "Fund Community Pool"

| Field | Source | Format |
|---|---|---|
| Depositor | Message | Full address |
| Amount | Message | `FormatCoins()` |

**`MsgFundValidatorRewardsPool`** — Title: "Fund Validator Rewards"

| Field | Source | Format |
|---|---|---|
| Depositor | Message | Full address |
| Validator | Message | Full address |
| Amount | Message | `FormatCoins()` |

##### Governance

**`MsgSubmitProposal`** — Title: "Proposal Submitted"

| Field | Source | Format |
|---|---|---|
| Proposer | Message | Full address |
| Proposal ID | Events (`submit_proposal.proposal_id`) | Bold |
| Title | Message | |
| Deposit | Message | `FormatCoins()` |

**`MsgDeposit`** — Title: "Proposal Deposit"

| Field | Source | Format |
|---|---|---|
| Depositor | Message | Full address |
| Proposal ID | Message | Bold |
| Amount | Message | `FormatCoins()` |

**`MsgVote`** — Title: "Vote"

| Field | Source | Format |
|---|---|---|
| Voter | Message | Full address |
| Proposal ID | Message | Bold |
| Option | Message | Human-readable (Yes/No/Abstain/NoWithVeto) |

**`MsgVoteWeighted`** — Title: "Weighted Vote"

| Field | Source | Format |
|---|---|---|
| Voter | Message | Full address |
| Proposal ID | Message | Bold |
| Options | Message | List of option=weight pairs |

##### Authz

**`MsgGrant`** — Title: "Authorization Granted"

| Field | Source | Format |
|---|---|---|
| Granter | Message | Full address |
| Grantee | Message | Full address |
| Type | Authorization type URL | Short type name |
| Expiration | Message | ISO timestamp |

**`MsgRevoke`** — Title: "Authorization Revoked"

| Field | Source | Format |
|---|---|---|
| Granter | Message | Full address |
| Grantee | Message | Full address |
| Msg Type | Message | Short type name |

**`MsgExec`** — Title: "Authz Exec"

Renders inner messages using their own registered formatters (recursive). Each inner message is shown as a sub-item.

##### Feegrant

**`MsgGrantAllowance`** — Title: "Fee Allowance Granted"

| Field | Source | Format |
|---|---|---|
| Granter | Message | Full address |
| Grantee | Message | Full address |
| Type | Allowance type | Short type name |
| Expiration | Allowance | ISO timestamp (if set) |

**`MsgRevokeAllowance`** — Title: "Fee Allowance Revoked"

| Field | Source | Format |
|---|---|---|
| Granter | Message | Full address |
| Grantee | Message | Full address |

##### Escrow

**Deposit** — Title: "Escrow Deposit"

| Field | Source | Format |
|---|---|---|
| Owner | Message | Full address |
| DSEQ | Message | Bold |
| Amount | Message | `FormatCoin()` |

##### Slashing

**`MsgUnjail`** — Title: "Unjail"

| Field | Source | Format |
|---|---|---|
| Validator | Message | Full address |

##### Vesting

**`MsgCreateVestingAccount`** — Title: "Vesting Account Created"

| Field | Source | Format |
|---|---|---|
| From | Message | Full address |
| To | Message | Full address |
| Amount | Message | `FormatCoins()` |
| End Time | Message | ISO timestamp |

**`MsgCreatePermanentLockedAccount`** — Title: "Permanent Locked Account Created"

| Field | Source | Format |
|---|---|---|
| From | Message | Full address |
| To | Message | Full address |
| Amount | Message | `FormatCoins()` |

**`MsgCreatePeriodicVestingAccount`** — Title: "Periodic Vesting Account Created"

| Field | Source | Format |
|---|---|---|
| From | Message | Full address |
| To | Message | Full address |
| Periods | Message | Count + total amount |

##### Upgrade

**`MsgSoftwareUpgrade`** — Title: "Software Upgrade"

| Field | Source | Format |
|---|---|---|
| Authority | Message | Full address |
| Name | Plan | |
| Height | Plan | Comma-grouped |

**`MsgCancelUpgrade`** — Title: "Upgrade Cancelled"

| Field | Source | Format |
|---|---|---|
| Authority | Message | Full address |

##### Crisis

**`MsgVerifyInvariant`** — Title: "Verify Invariant"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Module | Message | |
| Route | Message | |

##### WASM

**`MsgStoreCode`** — Title: "Code Stored"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Code ID | Events (`store_code.code_id`) | Bold |

**`MsgInstantiateContract`** — Title: "Contract Instantiated"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Code ID | Message | |
| Contract | Events (`instantiate.contract_address`) | Full address, bold |
| Label | Message | |
| Admin | Message | Full address (or "none") |

**`MsgInstantiateContract2`** — Title: "Contract Instantiated"

Same fields as MsgInstantiateContract, plus:

| Field | Source | Format |
|---|---|---|
| Salt | Message | Hex string |

**`MsgExecuteContract`** — Title: "Contract Executed"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Contract | Message | Full address |
| Funds | Message | `FormatCoins()` (if any) |

**`MsgMigrateContract`** — Title: "Contract Migrated"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Contract | Message | Full address |
| New Code ID | Message | |

**`MsgUpdateAdmin`** — Title: "Contract Admin Updated"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Contract | Message | Full address |
| New Admin | Message | Full address |

**`MsgClearAdmin`** — Title: "Contract Admin Cleared"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Contract | Message | Full address |

**`MsgUpdateInstantiateConfig`** — Title: "Instantiate Config Updated"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Code ID | Message | |

**`MsgSetContractLabel`** — Title: "Contract Label Set"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Contract | Message | Full address |
| Label | Message | |

##### Oracle

**`MsgFeed`** — Title: "Price Feed"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Asset | Message | |
| Base | Message | |
| Price | Message | |

##### BME

**`MsgBurnMint`** — Title: "Burn Mint"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Burned | Message | `FormatCoins()` |
| Minted Denom | Message | |

**`MsgMintACT`** — Title: "Mint ACT"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Burned | Message | `FormatCoins()` |

**`MsgBurnACT`** — Title: "Burn ACT"

| Field | Source | Format |
|---|---|---|
| Sender | Message | Full address |
| Burned | Message | `FormatCoins()` |

---

## 11. Error Handling

### 11.1 Error Types

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

### 11.2 Exit Codes

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

### 11.3 User-Facing Error Messages

All errors are written to **stderr**, never stdout. This is critical for scriptability -- piping stdout to another command or file must never be polluted with error messages (see [§10.1.1](#1011-stream-separation-stdout-vs-stderr)).

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

### 11.4 Typo Suggestions

When a user types an unknown command or subcommand, `akt` suggests the closest match using Levenshtein distance (cobra's built-in `SuggestionsMinimumDistance`). The suggestion threshold is set to 2 (maximum edit distance).

```
$ akt qurey deployment
Error: unknown command "qurey" for "akt"

Did you mean this?
    query

Run 'akt --help' for usage.
```

This also applies to subcommands:

```
$ akt context nework list
Error: unknown command "nework" for "akt context"

Did you mean this?
    network

Run 'akt context --help' for usage.
```

Cobra provides this feature via `Command.SuggestionsMinimumDistance` and `Command.SuggestFor`. All top-level and subcommands must have suggestions enabled (the cobra default). Commands with common aliases should register them via `SuggestFor` (e.g., the `query` command suggests for `q`).

---

## 12. Phased Implementation Plan

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
| 1.7  | Action log            | Append-only JSONL action log per context, log reading/filtering                                                                                                                             | All mutating actions (tx, workflow, provider, context, console) logged per §5.6 — read-only queries are not recorded by default; `akt context log` shows entries newest-first; log rotation works |
| 1.8  | Chain client          | Full and light client with multi-endpoint failover                                                                                                                                          | Successful tx broadcast and query with automatic failover when primary endpoint is down                 |
| 1.9  | Transaction commands  | All `tx` module commands (bank, deployment, market, provider, cert, audit, staking, distribution, gov, authz, feegrant, escrow, wasm, oracle, bme, slashing, vesting, upgrade, crisis, IBC) | Each command matches the behavioral output of the current `akash` binary                                |
| 1.10 | Query commands        | All `query` module commands                                                                                                                                                                 | Each command matches the behavioral output of the current `akash` binary                                |
| 1.10a | Resource filter parsing | `internal/filter/` package implementing the `/`-separated positional filter argument (§3.8) for Akash query commands: deployment, market (order/bid/lease), cert, audit, escrow. Smart type detection (bech32 vs uint), `--by provider` mode, get-vs-list heuristic. | `akt query deployment 12345`, `akt query market lease akash1.../12345/1/1/akash1prov...`, and all §3.8.6 examples work correctly |
| 1.11 | Key commands          | All `keys` subcommands                                                                                                                                                                      | Full key lifecycle works (create, export, import, delete, show, list)                                   |
| 1.12 | Auth utility commands | sign, sign-batch, multisign, validate-signatures, broadcast, encode, decode                                                                                                                 | Offline signing workflow works end-to-end                                                               |
| 1.13 | Output formatting     | Pretty output registry with per-type formatters for all query results. `--output pretty` (default) renders lipgloss-styled tables/sections. `--output json\|yaml` produces machine-readable output. See section 10. | All query results render pretty tables (list) or sectioned key-value (detail) by default. `--output json` or `--output yaml` produces machine-readable output. |
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
| 2.8  | Store export/import     | YAML and JSON export, import with merge/replace                                                                                | Round-trip: export then import produces identical store state             |
| 2.9  | Store status command    | Display store info, sync state, record counts                                                                                  | Accurate reporting of store contents                                      |
| 2.10 | Events command          | Live blockchain event streaming                                                                                                | `akt events` shows real-time events                                       |
| 2.11 | Console API client      | `auth-method: console-api` support, API key resolved flag > env > per-context stored credential (§7.1), deployment CRUD via Console managed wallet API, `akt console` command group (§2.9) | Create, update, close deployments; list bids; create leases via Console API with the API key resolved per §7.1 |
| 2.12 | MCP server              | `akt mcp` command with stdio JSON-RPC transport, 21 read-only tools, 4 write tools gated behind `--enable-writes`              | Read-only tools query chain state; write tools send transactions. Config resolved from active context. |

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
| 3.9  | Monitor hub (`akt monitor`)         | Hub-based monitoring with Tab/Shift-Tab navigation between Network, Provider, and Oracle/BME dashboards. Subcommands: `monitor network`, `monitor provider`, `monitor oracle`/`monitor bme`. | Hub launches, Tab/Shift-Tab cycles dashboards, subcommands open correct dashboard |
| 3.10 | Network dashboard (from aktop)      | Consensus state (height/round/step, vote progress bars, vote grid), validator voting view (scrollable list, moniker resolution, signing history), governance params   | Consensus updates via WebSocket, validator monikers resolved and cached, governance params for all 12 modules |
| 3.11 | Provider dashboard (from aktop)     | Version distribution visualization, provider health scanning with priority scheduling, per-provider detail with node-level CPU/memory/GPU | Providers scanned with smart scheduling, version distribution accurate, GPU models shown via gRPC |
| 3.12 | Provider cache                      | Smart-scheduled provider cache with disk persistence                                                                                      | Cache persists across sessions, scheduling intervals respected (1m/5m/6h)                         |
| 3.13 | Oracle/BME dashboard                | Combined oracle prices (TWAP, median, health) and BME state (mint status, vault, ledger). REST-based polling + real-time bus events.        | Oracle prices and BME state displayed, color-coded, amounts formatted correctly                    |
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
| 4.10 | IBC view (TUI)           | Channel list with state                            | IBC channels visible in TUI                        |
| 4.11 | TUI transaction actions  | Create deployment, fund escrow, etc. from TUI      | Full transaction workflow in TUI                   |
| 4.12 | Performance optimization | Lazy loading, virtual scrolling                    | Lists with >10,000 items render smoothly           |
| 4.13 | Comprehensive E2E tests  | Full coverage of all commands and TUI interactions | CI passes with >80% coverage                       |
