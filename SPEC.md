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
12. [Testing and Verification](#12-testing-and-verification)
13. [Phased Implementation Plan](#13-phased-implementation-plan)

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
        - https://rpc.akt.dev:443/rpc
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
    chain-id: sandbox-2
    endpoints:
      rpc:
        - https://rpc.sandbox-2.aksh.pw:443
      api:
        - https://api.sandbox-2.aksh.pw:443
      grpc:
        - grpc.sandbox-2.aksh.pw:9090
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

**Backend resolution is pinned.** The configured `backend` names the credential
store that akt will use, not a preference to be negotiated. `file`, `test`,
`kwallet`, and `pass` each resolve to exactly that store. `os` resolves to the
platform's system credential store and to nothing else:

| Platform  | System store for `os`             |
| --------- | ---------------------------------- |
| `darwin`  | Keychain                           |
| `windows` | Windows Credential Manager         |
| `linux`   | Secret Service (or KWallet)        |

akt MUST NOT substitute a different store when the configured one is
unavailable. In particular, a headless host with no session bus MUST NOT fall
back to an encrypted file keyring, a `pass` store, or the kernel keyring while
the configuration and `akt context show` continue to report `os`: that
silently changes where the user's keys live and prompts for a passphrase the
user never knowingly set. Opening a keyring whose configured backend cannot be
provided is a normal, fail-fast error (§2.10) that names the backend, the
platform store it needed, and the remedy — rerun with
`--keyring-backend file`, or persist the change with
`akt context keyring set <name> file`.

**Effective backend.** Because `os` is an alias, akt distinguishes the
*configured* backend from the *effective* one — the concrete store that serves
it on this host (`keychain`, `wincred`, `secret-service`, `kwallet`). Every
surface that reports a keyring reports both whenever they differ, and reports
the backend as unavailable rather than claiming a store that is not there.
Resolution is inspection only: determining the effective backend MUST NOT read
a key, unlock a store, or prompt.

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

**Local identity is resolved only where it is needed.** Identity access has
three modes, selected at the command boundary:

- **none** — the command never receives a keyring. SDL authoring (§2.11),
  monitoring (§2.6), version, completion, local store inspection/import/export,
  context management, the Console rail, and workflow dry-runs use this mode.
- **on demand** — the client context receives a deferred keyring that does not
  open its configured backend until an operation actually asks for a key. Chain
  queries, `store sync`, read-only MCP startup, public provider status, and
  address-based transaction construction or simulation use this mode.
- **required** — startup opens the keyring and resolves a named account before
  execution. Transactions that sign, workflow execution, and authenticated
  provider operations use this mode.

Opening a file or OS keyring can itself prompt, fail on a headless host, or ask
the desktop to unlock. An on-demand command therefore MUST NOT open the backend
merely because `default-account` is configured. A named default account is
resolved only when an omitted owner or an authenticated operation needs its
address. Network-wide queries, explicitly scoped queries, `akt provider
status`, and MCP startup MUST run without any keyring access. An address-valued
`default-account` or `--from` is parsed directly and never requires keyring
access for `--generate-only` or `--dry-run`; a signer name opens the deferred
keyring when it is resolved.

A named-account lookup MUST treat the returned keyring record as untrusted
persistent data. A nil record or a record without encoded public-key material
returns a descriptive account-resolution error before SDK address derivation;
malformed keyring state MUST NOT panic the CLI.

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

File creation and replacement can emit an fsnotify event before the writer has
finished the YAML payload. Reload waits for the create/write burst to settle,
then publishes only a successfully parsed configuration. A malformed or
partial write reports a warning and preserves the manager's last-good in-memory
configuration; subscribers never receive a transient empty config. Watching
may start before `config.yaml` exists by monitoring its parent directory until
the file is created.

### 1.9 Environment Variable Mapping

All environment variables use the `AKT_` prefix. When set, they override the corresponding config value.

| Environment Variable  | Overrides                                               | Example                        |
| --------------------- | ------------------------------------------------------- | ------------------------------ |
| `AKT_HOME`            | Home directory (overrides XDG default)                  | `/path/to/.akt`                |
| `AKT_CONTEXT`         | Active context name (overrides `current-context`)       | `prod`                         |
| `AKT_CHAIN_ID`        | `networks[*].chain-id` (via context's network)          | `akashnet-2`                   |
| `AKT_NODE`            | `networks[*].endpoints.rpc[0]` (via context's network)  | `https://rpc.akt.dev:443/rpc` |
| `AKT_GRPC_ADDR`       | `networks[*].endpoints.grpc[0]` (via context's network) | `grpc.akashnet.net:443`        |
| `AKT_FROM`            | `contexts[*].default-account`                           | `alice`                        |
| `AKT_KEYRING_BACKEND` | `keyrings[*].backend` (via context's keyring); same value as `--keyring-backend` (§3.1) | `os`                           |
| `AKT_KEYRING_DIR`     | `keyrings[*].dir` (via context's keyring); same value as `--keyring-dir` (§3.1) | `/path/to/keyring`             |
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
    - https://rpc.akt.dev:443/rpc
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
chain-id: sandbox-2
endpoints:
  rpc:
    - https://rpc.sandbox-2.aksh.pw:443
  api:
    - https://api.sandbox-2.aksh.pw:443
  grpc:
    - grpc.sandbox-2.aksh.pw:9090
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

### 1.12 Legacy `akash` Home

The legacy `akash` and `provider-services` CLIs keep their state in `~/.akash`.
`akt` uses its own home (§1.1) and **never** reads, writes, moves, or deletes
anything under `~/.akash`. Nothing carries over implicitly, and there is no
importer.

Three concrete incompatibilities make the legacy home unusable as-is:

| Artifact | Legacy location | `akt` location | Why it does not carry over |
|---|---|---|---|
| `os` keyring entries | Keychain/libsecret service `akash` | Keychain/libsecret service `akt` | The service name is the lookup key; entries written under `akash` are invisible to `akt`. |
| `file` / `test` keyring | `~/.akash/keyring-file`, `~/.akash/keyring-test` | `<home>/keyrings/<keyring-name>` (§1.5) | Different directory; `akt` never scans outside its own home. |
| Client mTLS certificate | `~/.akash/<address>.pem` | `<home>/<address>.pem` | The chain client's home directory is the `akt` home, so the PEM is looked up there. |

What this costs the user, precisely:

- **An account is recoverable, not lost.** `akt context keys add <name>
  --recover` with the original mnemonic reproduces the same address and the same
  on-chain identity. Nothing on chain changes.
- **A published certificate is not lost.** The certificate is on-chain state and
  remains valid and queryable with `akt query cert list <address>`. Only the
  local PEM half is in the wrong directory. Its encryption password is derived
  deterministically from a keyring signature over the account address, so once
  the same key is present in the `akt` keyring, copying
  `~/.akash/<address>.pem` to `<home>/<address>.pem` restores it byte-for-byte
  with no re-publish and no transaction. Regenerating instead
  (`akt tx cert generate client` then `akt tx cert publish client`) costs a
  transaction.

**Detection.** The first-run wizard (§2.0) detects a legacy home so the user is
told this up front instead of discovering it as a missing key. Detection is
read-only and strictly bounded to three operations: `os.Stat` on `~/.akash`, a
`*.pem` filename glob inside it, and `os.Stat` on `keyring-file/` and
`keyring-test/`. File contents are never read and legacy keyrings are never
opened. The notice is emitted only when the directory exists **and** at least
one of those artifacts is present; a bare or unrelated `~/.akash` produces no
output. The notice states explicitly that nothing in `~/.akash` is read,
modified, or deleted.

---

## 2. CLI Command Reference

Implementation note: `tx` and `query` commands are clean-copied from
`akash-network/chain-sdk/go/cli` into `internal/cli/chain`. Only CLI code is
copied; all other chain-sdk packages are imported directly. Command flags
default to the resolved akt context values unless explicitly overridden.

The release copy MUST contain only commands registered in the assembled `akt`
tree and helpers reachable from those commands. Validator-node server, node
RPC-console, state export, rollback, module-hash, genesis initialization and
genesis migration helpers remain in `akash-network/node` and MUST NOT be kept
in `internal/cli/chain` as an unregistered reference copy. The public
`query block`, `query blocks`, and `query block-results` commands are the
exception to their former server-file ownership: they remain release commands
in a focused block-query implementation. Duplicate upstream `keys` commands
MUST NOT coexist with `akt context keys`; `internal/cli/keys` is the sole key
management command owner.

An upstream command implementation that is deliberately absent from the
command tree, including unsupported crisis/evidence transactions or an
unregistered multisign-batch variant, is not release source. Orphan helpers
used only by those copies are removed rather than tested for coverage. The
staking commission-rate builder remains because the registered
create-validator action calls it. Certificate transaction code, including the
currently reachable `--to-genesis` behavior, remains unchanged until a
separate specification decision removes or replaces that public option.

**Help text requirement**: Every command and subcommand must populate cobra's `Example` field with at least one usage example. The example must name a registered command and registered flags, and it must be syntactically runnable after replacing any clearly explained placeholders. It should demonstrate the most common use case with realistic argument values. Commands with multiple modes of operation (e.g., list vs get, interactive vs scripted) should include one example per mode. User-facing help must not contain internal specification references, review notes, or instructions aimed at an automated agent. This ensures that `akt <command> --help` is self-contained -- users should never need to consult external documentation for basic usage.

**Input validation requirement**: A command group prints its help and exits 0
only when it is invoked without an action. Any non-flag token that does not
name a child command is an unknown-command error and exits non-zero, including
for vendored Cosmos SDK and IBC groups. Flags with a documented set of values
validate that set during argument parsing, before configuration or network
work begins. In particular, the standard `--output` values are `pretty`,
`json`, and `yaml`; workflow commands additionally accept `jsonl`, while
SDK-compatible key and RPC commands that advertise `text|json` accept exactly
those values. The accepted enum and the values rendered in `--help` are one
contract: when an adopted flag is normalized, its help text is normalized in
the same boundary pass.

Governance leaf queries parse proposal identifiers and validate voter and
depositor bech32 addresses and pagination before invoking the query client.
`query gov param` accepts exactly `voting`, `tallying`, or `deposit` and rejects
any other selector before transport work. Where a vote, deposit, deposit-list,
vote-list, or tally query first checks that the proposal exists, a failed
preflight wraps the underlying dependency error so it remains discoverable
with `errors.Is`.

Every accepted flag must affect the operation it describes. If a leaf cannot
apply an inherited transport, snapshot, pagination, or output flag, it rejects
that flag before configuration or network work rather than accepting and
ignoring it. When a positional value and a flag both target the same field,
supplying both is a usage error unless that command explicitly documents a
different precedence rule.
Each selectable command path is unique; dependency-owned trees are deduplicated
when they are adopted into the assembled command tree.

Typed chain query responses are untrusted transport input. A query call that
returns no error MUST still provide a non-nil response object, plus every nested
object the selected command renders as a required result. A missing successful
response is a malformed-node error. The command MUST NOT dereference it, print a
zero-valued result, or report success. Valid requests preserve the exact parsed
identity and pagination fields, transport errors remain classifiable by their
original cause, and destination failures from query rendering remain command
errors.

BME conversion and escrow-deposit commands MUST validate the required identity
and run the constructed SDK message's `ValidateBasic` contract before
broadcast. In particular, BME conversions reject a zero burn amount, and
escrow deposits reject a zero deployment identifier, zero amount, empty or
invalid source set, and duplicate sources. A rejected message produces no
transaction request or success output.

The global `--context` flag selects every context-owned resource touched by an
invocation. This includes `context show`, `context log`, Console API key login
and logout, keyring selection, stores, and action logs; no leaf may fall back to
`current-context` after the root selected an override. Account resolution uses
the same precedence chain: `--from` > `AKT_FROM` > the selected context's
`default-account`.

`--dry-run` suppresses state changes and client discovery, not input
validation. Workflow commands validate required parameters and their declared
types before printing a plan. Built-in deployment workflows additionally parse
the SDL and validate deposit syntax, positive sequence identifiers, bid
timeouts, bid selectors, output mode, and transaction chain identity. Invalid
input produces a non-zero usage error and no plan. In `--output jsonl` mode, a
valid dry-run emits one JSON object per planned step with `result:"planned"`,
empty `errors` and `txs` arrays, and one generated run ID shared by every line;
it emits no human plan text.

### 2.0 Root Command Behavior (`akt` with no subcommand)

When `akt` is invoked with no subcommand, the following flow determines what happens:

1. **No config exists** (first run): The bootstrap wizard runs (`internal/bootstrap/`; glyphs per §1.11, prompt rendering per §3.9). The wizard runs only when stdin is a terminal: in headless environments it declines to bootstrap (no network fetch, no config written) and prints guidance to create a config via `akt context network create` / `akt context create`; the root command then continues to step 2 without a config. After bootstrap completes, the root command continues to step 2.

In a terminal the wizard performs the following steps, in order. All of its output — announcements, prompts, progress, and the closing summary — goes to stderr (§3.9.2, §10.1.1); the wizard writes nothing to stdout.

**a. Announce the destination.** *Before the first prompt*, the wizard prints the resolved config root and the `config.yaml` path it will write, and names the `--home` flag and the `AKT_HOME` environment variable as the overrides that relocate them (full resolution order in §1.1). It states that nothing is written until the prompts complete. A user who abandons the wizard, or who is unsure whether `AKT_HOME`/`XDG_CONFIG_HOME` is in effect, MUST still learn where akt would place its files without having to finish setup. Printing the destination only in the closing summary does not satisfy this.

**b. Select networks.** A multi-select over the network definitions fetched from `github.com/akash-network/net`, with every network pre-selected. One context is created per selected network, named after that network.

After bootstrap, or after loading an existing config, root initialization reads
the mainnet chain ID from the configured network named `mainnet`. Downstream
command logic MUST use that invocation value and MUST NOT duplicate a concrete
mainnet chain ID in command code or package-level state.

**c. Select the keyring backend.** Single-select over `os`, `file`, and `test`; default `os`.

**d. Select the active context.** The wizard asks explicitly which of the created contexts becomes `current-context`. It MUST NOT choose one silently. The cursor starts on a test network — `sandbox` if it was selected, otherwise `testnet`, otherwise any other non-mainnet selection — so that accepting the default lands on a network where a mistake costs nothing. Mainnet is always present in the list and reachable in a single keystroke, but is never the pre-selected row: the first command a new user runs must not be able to spend real AKT because the active network was inherited from a default rather than chosen. The prompt is shown even when only one network was selected, so the active network is always something the user saw and confirmed.

**e. Optional Akash Console onboarding.** The user may enter a Console API key (validated best-effort against `/v1/user/me`, stored as the initial context's per-context credential per §7.1) and choose whether deployments for that context should be routed through Console (`auth-method: console-api`). Both prompts default to "no" and are skipped entirely in non-interactive runs.

**f. Summary.** After the config is written the wizard prints the config file path; the active context with its chain ID; and that context's context directory, store, action log, and keyring location, using the same labels as `akt context show` (`Store`, `Action Log`). `SaveConfig` creates only the home directory, so paths that do not exist yet are described as where they *will* be created rather than presented as existing. For the `os` backend the keyring line names the system keychain and the `akt` service name rather than a directory, because no directory is used. No context receives a `default-account`, so the summary closes with the next steps that make the configuration usable: `akt context keys add <name>` for a new account, `akt context keys add <name> --recover` for an existing mnemonic, and `akt context show`.

**g. Legacy Akash CLI notice.** Last, the wizard prints the legacy-home notice of §1.12 when — and only when — a legacy `~/.akash` with recoverable artifacts is detected. It is printed after the summary so it is the last thing on screen when the user goes looking for their existing keys.
> **TUI shell status (2026-07): DISABLED pending UX feedback.** Bare `akt` prints the help text and `--interactive`/`-i` reports that the TUI is disabled. The launch path remains compiled behind `AKT_EXPERIMENTAL_TUI=1` for feedback sessions, and `akt monitor` (§2.6) is unaffected. Steps 2–5 below describe the behavior that resumes when the TUI is re-enabled.

The root help introduction describes `akt` as the unified Akash Network CLI.
It names the major jobs available from the command tree: chain queries and
transactions, deployments through either payment rail, provider gateway
operations, context and key management, and network monitoring. It MUST NOT
describe workload deployment as though it were the CLI's only purpose.

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
│   ├── keyring                          # Keyring definition management (shared resource)
│   │   ├── create <name> <backend>      # Create a new keyring definition
│   │   ├── list                         # List keyrings (configured + effective backend)
│   │   └── set <name> <backend>         # Change a keyring's stored backend/dir
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
│   │   ├── update <sdl-file> <dseq>     # dseq required positionally (--dseq disabled 2026-07); fails fast when missing
│   │   ├── close <dseq>                 # dseq required positionally (--dseq disabled 2026-07); fails fast when missing
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
│   ├── provider [address]               # List or get (address → single); `list`/`get` remain as aliases
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
│   │   └── list-contracts-by-creator <creator>
│   ├── oracle
│   │   ├── prices
│   │   ├── aggregated-price <denom>     # denom accepts akt, AKT, or uakt (normalized to the oracle's base denom)
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
├── version [--long]                     # Version information (--long: full build info)
└── completion                           # Shell completion scripts
```

Authz grant construction MUST validate the concrete SDK authorization before
generating or broadcasting a transaction. This applies to send, deposit,
generic, staking, contract execution/migration, and store-code grants. Nested
contract limits and filters, raw contract messages, and store-code grant sets
MUST therefore satisfy their authoritative SDK validation rules; ambiguous
limit/filter combinations, invalid JSON, duplicate entries, empty generic
message types, and other invalid authorization state fail locally with no
transaction output and no broadcast. A valid generated `MsgGrant` MUST retain
the complete granter and grantee addresses, concrete authorization type and
fields, and requested expiration exactly.

`query escrow blocks-remaining` MUST derive its estimate from one consistent
snapshot of BME status, the `uakt` oracle price, active leases for the exact
owner/deployment sequence, current block height, and the deployment escrow
account. It MUST request only active matching leases, sum all lease rates,
ignore negative balances, always include `uact`, and include `uakt` only while
the BME circuit breaker is active and the price feed is healthy. Missing
identity, no matching lease, or any upstream query failure MUST return an error
instead of printing an estimate. JSON and YAML output MUST encode the same
balance, block count, and average-block-time duration.

The registered `query upgrade` group MUST expose `plan`, `applied
<upgrade-name>`, and `module_versions [module-name]`. `plan` prints only a real
scheduled plan and reports when none exists. `applied` MUST bind the queried
upgrade name to its recorded height and the block header fetched at that exact
height; a zero height or missing header is an error. `module_versions` MUST
preserve the optional module filter and report an absent result as not found.
All three commands use the standard context-aware chain-query pre-run and
propagate transport errors without printing a successful result.

Wasm instantiation MUST preserve the transaction intent exactly. The code ID
MUST be an unsigned integer, the label MUST be non-empty, and funds MUST parse
as normalized SDK coins. Callers MUST choose exactly one administration mode:
an explicit admin (a bech32 address or a keyring name resolved to its complete
address) or `--no-admin` for an immutable contract. Omitting both choices or
combining them is invalid and MUST fail before broadcast. The generated message
MUST retain the sender, initialization JSON bytes, normalized funds, label,
code ID, and resolved full admin address without alteration.

Wasm upload inputs MUST be either a Wasm binary or a valid gzip stream. Empty,
truncated, unrelated, or corrupt files MUST return an actionable validation
error and MUST NOT panic. When reproducible-build verification fields are used,
source, builder image, and code hash are an all-or-nothing set; the source and
builder MUST parse, the gzip stream MUST decompress within the Wasm size limit,
and the declared hash MUST exactly match the uncompressed Wasm bytes.
Upload instantiate-permission modes (`any-of` addresses, everybody, or nobody)
MUST be mutually exclusive. A false boolean value does not select that mode;
malformed booleans, invalid or duplicate full addresses, the removed single-
address form, an any-of set larger than the upstream maximum, and multiple
selected modes MUST fail before message creation. No user-supplied permission
set may trigger a panic.

Only executable transaction actions appear in this tree. The Akash app does
not register the Cosmos SDK `MsgVerifyInvariant` handler, so `tx crisis` is not
mounted. Evidence submission remains query-only until at least one concrete
evidence transaction type exists. IBC channel-v2 queries are available, but its
empty upstream transaction group is omitted until upstream supplies a packet
action. Unknown invocations of these omitted paths follow the normal usage-error
contract; they never print group help at exit 0 or reach gas simulation.

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
| `--provider-auth-type`| string | `"jwt"`     | Provider gateway auth default: `jwt` or `mtls`                   |
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
An explicit global `--context <name>` selects the context to show. Structured
output includes the fully resolved network and keyring, effective gas/provider
settings, capability booleans, `store_path`, and `action_log_path`; it never
includes the Console API key. Pretty output renders the resolved network in one
`Network` subsection. It does not repeat the shared network object's name as a
separate `Network: <name>` row; structured output retains that name in the
resolved network object.

The keyring line reports the *configured* backend. When that backend is an
alias for a platform store — `os` (§1.5) — the concrete store that serves it is
reported on an `Effective` sub-line, and a configured backend the host cannot
provide is reported as unavailable together with its remedy. Structured output
carries the same two facts as `keyring_backend_effective` (empty when
unavailable) and `keyring_backend_available`. Rendering this view never opens a
key store and never prompts.

```bash
$ akt context show
Context
  Name:             prod
  Network:
    Chain ID:        akashnet-2
    RPC:             https://rpc.akt.dev:443/rpc (+2 backup)
    API:             https://api.akashnet.net:443 (+1 backup)
    gRPC:            grpc.akashnet.net:443
    Gas Prices:      0.025uakt
    Gas Adj:         1.5

  Auth Method:      keyring
  Keyring:          default (backend: os)
    Effective:      keychain
  Default Account:  alice
  Gas:              auto
  Fees:             (none)
  Provider Auth:    jwt
  Store:            ~/.config/akt/contexts/prod/store/
  Action Log:       ~/.config/akt/contexts/prod/actions.log
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
| `--provider-auth-type` | string | unchanged | Change provider gateway auth default: `jwt` or `mtls` |
| `--auth-method`     | string | `""`    | Change authentication method: `keyring` or `console-api` |
| `--console-api-url` | string | `""`    | Change Console API base URL (empty = default) |
| `--console-api-key` | string | `""`    | Set the per-context Console API key (empty string removes it; §7.1) |
| `--fork-network`    | bool   | `false` | Force fork when editing network fields |
| `--rpc`             | []string | unchanged | Replace the selected network's RPC endpoints |
| `--api`             | []string | unchanged | Replace the selected network's REST endpoints |
| `--grpc`            | []string | unchanged | Replace the selected network's gRPC endpoints |
| `--gas-prices`      | string | unchanged | Change the selected network's gas prices |
| `--gas-adjustment`  | string | unchanged | Change the selected network's gas adjustment |
| `--yes`             | bool   | `false` | Edit a shared parent network without prompting |

```bash
# Change default account
akt context edit prod --default-account bob

# Switch to a different network
akt context edit staging --network sandbox

# Edit the network's RPC (prompts: edit parent or fork?)
akt context edit prod --rpc https://my-private-rpc:443
```

`--fork-network` is valid only with at least one network-level field and cannot
be combined with `--network`. It atomically creates
`<source-network>-<context>`, applies the requested network edits to that copy,
and switches only the named context to it. If the generated name already
exists, the command refuses rather than overwriting it. Without
`--fork-network`, editing a network used by multiple contexts prompts in a TTY;
`--yes` or a non-TTY invocation chooses the documented edit-parent default.

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

| Flag            | Type   | Default   | Description                                                         |
| --------------- | ------ | --------- | ------------------------------------------------------------------- |
| `--context`     | string | current   | Context to view log for                                             |
| `--limit`       | int    | `50`      | Number of entries to show                                           |
| `--type`        | string | `""`      | Filter by action type: `tx`, `workflow`, `provider`, `context`, `console`, `error` (see §5.6) |
| `--workflow-id` | string | `""`      | Show only the entries of one workflow run (the `run` id shown in `SUMMARY`) |
| `--since`       | string | `""`      | Show entries since timestamp or duration (e.g., `1h`, `2024-01-01`) |
| `--output`      | string | `pretty`  | Output format: `pretty` (table), `json` (raw JSONL entries, one per line) |

The pretty table has four columns: `TIME`, `TYPE`, `SUMMARY`, and `STATUS`. The
action name alone does not identify an entry — a single workflow run writes one
entry per step, all under the same workflow name — so `SUMMARY` is composed per
entry type from the fields that distinguish it:

| Type              | `SUMMARY` composition                                                                        |
| ----------------- | -------------------------------------------------------------------------------------------- |
| `workflow`        | `<workflow>/<step-name>` plus `(run <workflow-id>)`, so every step of a run is its own row     |
| `tx`              | Message action plus `(dseq: <n>)` when the entry carries one                                   |
| `provider`        | Action plus ` -> <provider-address>` and `(dseq: <n>)`                                         |
| `console`         | Action plus `(dseq: <n>)`                                                                      |
| `context`         | Action plus the recorded parameters as `(key: value, ...)`, sorted by key                      |
| `query`, `error`  | Action plus any recorded parameters                                                            |

Addresses and workflow run ids are printed in full; only the error text shown in
the `STATUS` column is shortened. The table is a summary view — machine output
(`-o json|yaml`) always serializes complete entries with every field of §5.4.

Before rendering, `akt context log` best-effort reconciles displayed `tx`
entries whose status is `pending` and whose transaction hash is non-empty. Each
hash is queried through the active context's RPC endpoint. A transaction found
in a block is recorded as a terminal `success` or `failed` revision with its
height, gas used, result code, and error text. A missing transaction, unavailable
RPC endpoint, timeout, or other lookup failure leaves the entry `pending` and
does not make this local audit command fail.

```bash
$ akt context log --limit 6
  TIME                 TYPE      SUMMARY                                                     STATUS
  2026-03-23 10:25:00  context   keys.add (address: akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx, name: bob, type: local)  success
  2026-03-23 10:20:01  context   edit (default-account: bob)                                 success
  2026-03-23 10:15:50  workflow  deploy/send-manifest (run 9f2c1ab34d55e017)                 error: provider gateway unreachable
  2026-03-23 10:15:47  workflow  deploy/create-lease (run 9f2c1ab34d55e017)                  success
  2026-03-23 10:15:45  workflow  deploy/wait-for-bids (run 9f2c1ab34d55e017)                 success
  2026-03-23 10:15:32  tx        deployment.MsgCreateDeployment (dseq: 12345)                success
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

Pretty output is rendered by `pretty.RenderNetworkList`, the same renderer the
TUI network list uses (§10.8). RPC endpoints are printed in full -- never
truncated -- with a `(+N)` suffix counting the backups that are not shown.

```bash
$ akt context network list
NAME     CHAIN-ID    RPC                                 USED BY
mainnet  akashnet-2  https://rpc.akt.dev:443/rpc (+2)    prod, monitoring
testnet  testnet-02  https://rpc.testnet-02.aksh.pw:443  staging
sandbox  sandbox-2   https://rpc.sandbox-2.aksh.pw:443   (none)
```

With no networks configured, the pretty path prints the remedy
(`No networks configured. Create one with: akt context network create <name> --template mainnet`);
`--output json`/`yaml` print an empty array (§10.3).

#### `akt context network show <name>`

Show full network details.

### 2.2.2 Keyring Commands

Keyrings are a shared resource (§1.5), managed independently of the contexts
that reference them. Without these commands the only way to change where a
context stores its keys after first run is to hand-edit `config.yaml`.

#### `akt context keyring create <name> <backend>`

Create a new keyring definition. `<backend>` is one of `os`, `file`, `test`,
`kwallet`, `pass`, `memory`; any other value is a usage error.

| Flag    | Type   | Default | Description                                                |
| ------- | ------ | ------- | ---------------------------------------------------------- |
| `--dir` | string | `""`    | Keyring directory (empty = `<root>/keyrings/<name>/`)      |

```bash
akt context keyring create headless file
```

#### `akt context keyring list`

List every configured keyring with its configured backend, the effective
backend on this host (§1.5), its directory, and the contexts that reference it.
A configured backend that cannot be provided here is reported as
`unavailable`, never as if it were in use.

```bash
$ akt context keyring list
  NAME       BACKEND   EFFECTIVE   DIR                        USED BY
  default    os        keychain    ~/.config/akt/keyrings/default   prod
  headless   file      file        ~/.config/akt/keyrings/headless  (none)
```

#### `akt context keyring set <name> <backend>`

Change a keyring's stored backend, and optionally its directory. This is the
persistent counterpart of the per-invocation `--keyring-backend` /
`--keyring-dir` overrides (§3.1) and is the remedy named by the fail-fast error
raised when a configured backend is unavailable (§1.5).

Changing the backend does **not** migrate existing keys: each backend has its
own store, so keys added under the previous backend remain there. The command
says so on stdout.

| Flag    | Type   | Default   | Description                                            |
| ------- | ------ | --------- | ------------------------------------------------------ |
| `--dir` | string | unchanged | Keyring directory (empty string is not "unchanged" only when the flag is set) |

```bash
akt context keyring set default file
akt context keyring set default file --dir /mnt/secure/keys
```

### 2.2.3 Keys Commands

#### `akt context keys add <name>`

Create, recover, or register a key. Pretty output retains the human backup
warning and mnemonic for a newly generated local key. JSON and YAML emit one
object with `name`, `address`, and `type`; multisig results also include
`threshold` and `pubkeys`, and a newly generated local key includes `mnemonic`
unless `--no-backup` was selected. Recovered keys never repeat their input
mnemonic. The selected output format applies equally to local, Ledger, and
multisig keys.

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

With `--output json|yaml`, the full form emits an object with `name`, `type`,
`address`, and `pubkey`. The `--address` form emits a quoted scalar in machine
formats and the unchanged raw address in pretty output.

#### `akt context keys parse <hex-or-bech32>`

Parse an address and render its canonical uppercase hex form plus full bech32
forms for the `akash`, `cosmos`, and `osmo` prefixes. JSON/YAML output contains
`format`, optional `hrp`, `hex`, and an `addresses` object; pretty output keeps
the aligned human-readable form.

#### Key management in the action log

Keyring mutations are state changes and are recorded in the active context's
action log (§5.6) as `type=context` entries under a dotted `keys.*` action, so
`akt context log --type context` shows key changes next to context changes
without colliding with the bare `create`/`delete`/`rename` context actions. The
entry is written after the keyring call returns, with `status=success`, or
`status=failed` plus the error text when the mutation failed, so an unsuccessful
attempt is auditable too. Commands run without a selected context (no
`current-context` and no `--context`) have no log to write to; the command
itself is unaffected.

| Command                                  | Action          | Recorded parameters                                                                          |
| ---------------------------------------- | --------------- | -------------------------------------------------------------------------------------------- |
| `keys add <name>`                        | `keys.add`      | `name`, `type` (`local`, `ledger`, `multi`), `address`; multisig adds `threshold` and `pubkeys` |
| `keys add <name> --recover` / `--source` | `keys.recover`  | `name`, `type`, `address`                                                                      |
| `keys delete <name>`                     | `keys.delete`   | `name`, `type`, `address`                                                                      |
| `keys rename <old> <new>`                | `keys.rename`   | `from`, `to`                                                                                   |
| `keys import <name> <keyfile>`           | `keys.import`   | `name`, `type`, `address`                                                                      |
| `keys export <name>`                     | `keys.export`   | `name`                                                                                         |

`keys export` changes no state, but it is the one command that moves private key
material out of the keyring. It is recorded as a security event and is the only
deliberate exception to the rule that read-only commands are not logged (§5.6).
The read-only `keys list`, `keys show`, `keys parse`, and `keys mnemonic` are
never recorded, and a cancelled `keys delete` confirmation records nothing.

Secrets never reach the log: mnemonics, BIP39 passphrases, export and import
passphrases, and armored key material are never recorded as parameters — the
same rule that makes `context edit` record `console-api-key: updated` instead of
the credential (§7.1). Addresses are recorded in full.

### 2.3 Workflow Engine

Workflow commands (`akt deploy`, `akt update`, `akt close`) are driven by a **declarative workflow engine**. Instead of hardcoded command logic, each workflow is a YAML definition that the engine interprets step by step. Users can override built-in workflows or create custom ones.

**Transports**: actions are defined once — as workflow definitions — and translated per transport by `internal/transport`. Each transport carries the same abstract steps onto its backing rail: the **chain** transport (keyring auth) signs and broadcasts transactions locally plus provider-gateway calls, while the **console** transport (console-api auth) maps the same steps onto Console API REST calls (§7.4–§7.5). Because the command surface (positionals and flags) is generated from the workflow definition and the transport is chosen per context at execution time, `akt deploy/update/close` accept identical arguments on both rails, and adding a new action never requires per-rail redesign. Cross-rail argument syntax (notably `--deposit`, §7.4) is normalized in the transport layer. A successful Console `akt deploy` result also reports that the Console enabled its default daily auto top-up and prints the exact deployment-settings command that disables it; chain results omit this Console-only state.

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
    type: sdl
    required: true
    description: Path to SDL deployment file
  deposit:
    type: deposit
    default: "auto"
    description: "Initial deposit: auto (recommended chain minimum, keyring), 5usd or $5 (console-api), or an explicit coin in the network's deposit denomination (keyring)"
  bid-timeout:
    type: duration
    default: "5m"
    description: Maximum time to wait for bids
  bid-select:
    type: bid-selection
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
    timeout-error: "no bids received (at least 1 required)"
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

Workflow parameter types are boundary contracts, not display hints:

| Type | Validation before plan or execution |
|------|-------------------------------------|
| `string` | Value is passed through; `required` rejects an empty value |
| `int` | Base-10 integer; a required sequence value must be greater than zero |
| `bool` | Cobra boolean parsing |
| `duration` | Positive Go duration such as `30s` or `5m` |
| `file` | Required path exists and is readable |
| `sdl` | File is readable and parses as a valid SDL document |
| `deposit` | Unified deposit grammar from §7.4 |
| `bid-selection` | `interactive`, `cheapest`, or `provider=<full-address>` |

Validation runs before dry-run prints its plan. User-defined workflows receive
the same validation from their declared parameter types.

#### 2.3.3 Step Types

| Type       | Description                                    | Key fields                                    |
|------------|------------------------------------------------|-----------------------------------------------|
| `tx`       | Broadcast a transaction                        | `msg`, `params`, `output`, `on-error`         |
| `query`    | Execute a chain query                          | `query`, `params`, `output`                   |
| `wait`     | Poll a query until a condition is met          | `query`, `params`, `until`, `timeout`, `timeout-error` |
| `prompt`   | Interactive user input (bid selection, confirm) | `mode`, `data`, `display`, `output`          |
| `provider` | Provider gateway call                          | `action`, `params`, `retry`                   |
| `output`   | Display formatted output                       | `template`                                    |
| `shell`    | Run a shell command (for custom workflows)     | `command`, `output`                           |
| `check`    | Assert a condition, skip/abort if not met      | `condition`, `on-fail: skip\|abort`           |
| `foreach`  | Iterate over a query result, executing a sub-step for each item | `query`, `params`, `as`, `step`, `on-error`  |

A false `check` condition with `on-fail: skip` records the step as `skipped`
and continues the workflow without also requiring `on-error: skip`. With
`on-fail: abort` (or no override), the same false condition records `failed`
and returns an error. An `output` step succeeds only when both template
rendering and the complete write to its configured destination succeed. A
writer error is a failed step and is subject to the ordinary `on-error`
policy; output loss is never reported as success.

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
Wait steps may define `timeout-error`, a user-facing explanation of what did
not arrive. On timeout the engine appends the elapsed limit to that explanation
and never exposes the internal `until` template. Without `timeout-error`, it
reports that the condition was not met before the limit without printing the
condition source.

#### 2.3.6 Error Recovery and Partial State

When a workflow aborts due to a step failure, the user may be left with partial on-chain state (e.g., a deployment was created but no lease was established, consuming escrow). The workflow engine handles this as follows:

1. **Abort message includes partial state summary**: When a deploy workflow aborts after `create-deployment` succeeds, both the human output and the returned command error identify the DSEQ and provider, when known. They also warn that escrow may continue to be consumed while the deployment or lease remains open. This gives the user the information needed to recover even when stdout was redirected or suppressed.

2. **Recovery suggestions**: The abort message includes actionable suggestions based on which step failed:
   - Failed after `create-deployment`: Suggest `akt close <dseq>` to close the orphaned deployment and reclaim the escrow deposit.
   - Failed after `create-lease` but before `send-manifest`: Suggest `akt provider send-manifest <sdl-file> --dseq <dseq> --provider <provider>` to retry manifest submission.
   - Failed during `send-manifest`: Suggest the same complete retry command, including the SDL path, DSEQ, and provider.

3. **JSONL mode**: In JSONL mode, the failed step includes `"dseq"`, `"provider"`, `"recovery"`, and `"cleanup"` fields when known. The recovery and cleanup values are complete copy-pasteable commands:
   ```jsonl
   {"workflow":"deploy","id":"wf_abc123","step":"send-manifest","result":"error","errors":["provider gateway timeout"],"txs":[],"dseq":12345,"provider":"akash1provider...","recovery":"akt provider send-manifest deploy.yaml --dseq 12345 --provider akash1provider...","cleanup":"akt close 12345"}
   ```

4. **No automatic rollback**: The workflow engine does not automatically roll back completed steps. On-chain transactions are irreversible. The user must explicitly close deployments or leases they no longer want.

5. **Future: `--resume` flag**: A future enhancement may add a `--resume <workflow-id>` flag that re-runs a workflow from the last failed step, using the stored outputs from completed steps. This is not part of the initial implementation.

#### 2.3.7 Param Types

| Type       | Description                       | Flag type      |
|------------|-----------------------------------|----------------|
| `string`        | Plain string                         | `--name value` |
| `int`           | Integer                              | `--name 5`     |
| `bool`          | Boolean                              | `--name`       |
| `duration`      | Positive Go duration                 | `--name 5m`    |
| `file`          | Readable file path (positional first)| positional arg |
| `sdl`           | Parsed SDL file (positional first)   | positional arg |
| `deposit`       | Unified §7.4 deposit                 | `--deposit 5usd` |
| `bid-selection` | Interactive/cheapest/provider mode   | `--bid-select cheapest` |

#### 2.3.8 Execution Modes

Workflows support two execution modes:

**TUI mode** (default when TTY is attached):
- Interactive bubbletea UI with progress display, spinners, and colored status output.
- `prompt` steps render interactive selection tables (e.g., bid selection).
- `output` steps render formatted text.
- Errors are displayed inline with the step that failed.
- A deployment waiting for bids reports the number received, elapsed time, and
  remaining time immediately, whenever the count changes, and at least every
  30 seconds. Progress is informational stderr output and is suppressed by
  `--quiet`, non-TTY execution, and structured workflow output modes.

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
| `result`   | string   | `planned` (dry-run), `completed`, `error`, or `skipped` (step skipped by its on-error policy) |
| `errors`   | []string | Array of error messages (empty when `result` is `completed`)    |
| `txs`      | []object | Array of raw transaction results (empty for non-tx steps)       |

**Transaction object schema (within `txs`):**

| Field      | Type   | Description                                   |
| ---------- | ------ | --------------------------------------------- |
| `hash`     | string | Transaction hash                              |
| `height`   | int64  | Block height where the tx was included. **Omitted** when the transaction has not been confirmed — under the default `--broadcast-mode sync` a step's transaction is only in the mempool, and emitting `"height":0` would be indistinguishable from a real height (see [§10.11.1](#broadcast-confirmation-state)) |
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
    type: sdl
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

  - name: send-manifest
    type: provider
    action: send-manifest-to-active-leases
    params:
      dseq: "{{ .Params.dseq }}"
      sdl: "{{ index .Params \"sdl-file\" }}"
    retry:
      max: 3
      delay: "5s"
    on-error: abort

  - name: display-result
    type: output
    template: |
      {{- $manifest := index .Steps "send-manifest" -}}
      {{- if not $manifest -}}
      Deployment updated.
        DSEQ: {{ .Params.dseq }}
        Manifest delivery: handled by the Console API.

        Note: a provider restarts a service only when its image reference or
        configuration actually changed; re-applying an identical manifest
        leaves the running workload as it is.
      {{- else if gt (index $manifest "count") 0 -}}
      Deployment updated.
        DSEQ: {{ .Params.dseq }}
        Manifest sent to {{ index $manifest "count" }} provider(s):
      {{ range index $manifest "providers" }}    {{ . }}
      {{ end }}
        Note: a provider restarts a service only when its image reference or
        configuration actually changed; re-applying an identical manifest
        leaves the running workload as it is.
      {{- else -}}
      Deployment updated on chain, but nothing was redeployed.
        DSEQ: {{ .Params.dseq }}

        WARNING: this deployment has no active leases, so the new manifest was
        not sent to any provider. Only the SDL hash changed on chain; no
        running workload was updated.
        List leases with "akt query market lease {{ .Params.dseq }} active".
      {{- end -}}
```

On the chain rail, `send-manifest-to-active-leases` queries every page of active
leases for the owner and DSEQ, de-duplicates and sorts provider addresses, and
attempts manifest submission to all of them. A provider failure does not stop
delivery attempts to the remaining providers, but the step fails unless every
provider accepts the update. No active leases is a successful no-op. The
operation is safe to retry. Console API contexts omit the provider step because
`PUT /v1/deployments/{dseq}` handles manifest delivery internally.

The step records `{"providers": [...], "count": N}`, and `display-result` MUST
read it rather than reporting an unconditional success: the update transaction
only records a new SDL hash on chain, so "updated" is a claim about the chain,
never a claim that a running workload changed. The three result forms are
specified under `akt update` below.

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

On the Console rail, the final display also identifies the default daily auto
top-up and prints `akt console deployment settings <dseq> false` as the exact
opt-out command. The chain rail does not display a Console setting.

| Flag               | Type     | Default         | Description                                                 |
| ------------------ | -------- | --------------- | ----------------------------------------------------------- |
| `--from`           | string   | context default | Account to deploy from                                      |
| `--deposit`        | string   | `auto`          | Initial deposit, unified syntax on both rails (see §7.4): `auto` (recommended for keyring), `5usd`/`$5` (Console), or an explicit network deposit coin |
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

**Result output.** The update transaction records a new SDL hash on chain; it
does not by itself change anything a provider is running. The result therefore
reports what the `send-manifest` step actually delivered, and never claims more
than that. There are exactly three forms.

Manifest delivered to one or more providers (chain rail):

```
Deployment updated.
  DSEQ: 12345
  Manifest sent to 2 provider(s):
    akash1prov1...
    akash1prov2...

  Note: a provider restarts a service only when its image reference or
  configuration actually changed; re-applying an identical manifest
  leaves the running workload as it is.
```

No active leases — the step is a successful no-op, so nothing was redeployed:

```
Deployment updated on chain, but nothing was redeployed.
  DSEQ: 12345

  WARNING: this deployment has no active leases, so the new manifest was not
  sent to any provider. Only the SDL hash changed on chain; no running
  workload was updated.
  List leases with "akt query market lease 12345 active".
```

Console API context — the provider step is filtered out because
`PUT /v1/deployments/{dseq}` delivers the manifest:

```
Deployment updated.
  DSEQ: 12345
  Manifest delivery: handled by the Console API.

  Note: a provider restarts a service only when its image reference or
  configuration actually changed; re-applying an identical manifest
  leaves the running workload as it is.
```

The bare transaction command `akt tx deployment update` sends no manifest at
all, and its pretty output says so
([§10.11](#1011-transaction-result-formatting)).

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

Query provider status. Supply the provider address positionally or with
`--provider`. The gateway URL is resolved from that provider's on-chain
`host_uri`; `--provider-url` is an explicit override for diagnostics and
private gateways. A provider with no on-chain host URI is refused before a
gateway request is attempted.

Provider `/status` is a public gateway endpoint. This command MUST NOT load a
default account, open a keyring, mint a JWT, or load an mTLS certificate. It is
valid from a monitoring-only context with chain-query access and no wallet.
The provider address is still required because it identifies the gateway and,
unless `--provider-url` is supplied, selects the on-chain `host_uri`. The
provider group's inherited `--auth-type` flag does not apply to this public
endpoint; explicitly passing it is refused instead of being silently ignored.

| Flag             | Type   | Default         | Description                                      |
| ---------------- | ------ | --------------- | ------------------------------------------------ |
| `--provider`     | string | `""`            | Provider address; alternative to positional form |
| `--provider-url` | string | on-chain record | Explicit provider gateway URL override           |

All lease-, manifest-, migration-, log-, event-, and shell-scoped provider
commands remain authenticated. Before constructing their gateway client they
resolve `--auth-type` over `provider-defaults.auth-type` from the selected
context and default to `jwt`. Before provider URL discovery or gateway network
work, they MUST validate the auth enum, default account, keyring, and that the
selected account exists in that keyring. Both JWT signing and mTLS certificate
loading require that local signing identity. Failures name the missing
provider signing identity and how to repair the context; they MUST NOT defer to
a signer as raw `key with address ... not found` output.

MCP uses the same selected context auth default for protected provider tools.
Provider status remains public; lease status, service status, and manifest
submission use the shared authenticated gateway boundary with JWT or mTLS as
configured.

**Provider resolution for lease-scoped commands.** `--provider` names the
gateway explicitly and always wins. When it is omitted, `akt` resolves the
provider from the deployment's leases on chain: it queries the leases owned by
the selected context's `default-account` for the given deployment sequence and
uses the provider of the deployment's single active lease. This is the same
identification `akt console status <dseq>` already performs on the Console rail,
so a deployment `akt` created is addressed identically on both rails. The
resolved lease also supplies `gseq`/`oseq` unless they were given explicitly.

Resolution is refused rather than guessed when it is ambiguous or impossible,
and the error names the actual cause:

- the deployment has no leases at all;
- it has leases but none active (their states are listed);
- more than one lease is active (every active provider is listed and
  `--provider` is required to choose).

Each of those errors points at `akt query market lease <dseq>` for the full
record. `--provider` is never a cobra-required flag (§3.8): the deployment
sequence is the primary value and the provider is an optional override.

`--provider-url` overrides the gateway URL, not the provider identity: the
lease path sent to the gateway still needs a provider address, so without
`--provider` the lease is read from chain even when a URL is supplied. Because
one URL cannot address several gateways, combining `--provider-url` with a
`send-manifest` fan-out over multiple active leases is refused.

The authenticated gRPC gateway boundary retains that resolved provider address
independently of both the caller's wallet address and the URL. Standard PKI
certificates MUST verify against the URL hostname. Before accepting Akash's
self-signed on-chain certificate fallback, the peer certificate subject MUST
equal the resolved provider address; a valid registered certificate belonging
to any other provider is refused before a JWT-bearing request can be sent.

Because the provider is resolved *from* the deployment sequence, the sequence is
validated first. A lease command invoked with neither a `dseq` nor a
`--provider` reports the missing deployment sequence, not the missing provider,
and never suggests a positional provider on a command whose positional slot is
the `dseq`.

The lease owner is always the selected context's `default-account`. The provider
group has no `--from` flag; select a different account with `akt context use`,
`--context`, or the context's `default-account` setting.

#### `akt provider lease-status [dseq]`

Query lease deployment status from the provider gateway. The positional `dseq` supplies the deployment sequence. The `--dseq` flag is **disabled pending feedback** (positional only, 2026-07).

| Flag          | Type   | Default         | Description         |
| ------------- | ------ | --------------- | ------------------- |
| `--dseq`      | uint64 | required unless positional `dseq` given | Deployment sequence — **disabled pending feedback** (positional only, 2026-07) |
| `--gseq`      | uint32 | active lease    | Group sequence      |
| `--oseq`      | uint32 | active lease    | Order sequence      |
| `--provider`  | string | active lease    | Provider address; resolved from the deployment's active lease when omitted |
| `--auth-type` | string | context default | Auth type           |

#### `akt provider lease-logs [dseq]`

Stream container logs from a lease. The positional `dseq` supplies the deployment sequence. The `--dseq` flag is **disabled pending feedback** (positional only, 2026-07).

| Flag          | Type   | Default         | Description               |
| ------------- | ------ | --------------- | ------------------------- |
| `--dseq`      | uint64 | required unless positional `dseq` given | Deployment sequence — **disabled pending feedback** (positional only, 2026-07) |
| `--gseq`      | uint32 | active lease    | Group sequence            |
| `--oseq`      | uint32 | active lease    | Order sequence            |
| `--provider`  | string | active lease    | Provider address; resolved from the deployment's active lease when omitted |
| `--service`   | string | `""`            | Filter by service name    |
| `--follow`    | bool   | `false`         | Stream logs continuously  |
| `--tail`      | int64  | `-1`            | Lines from end (-1 = all) |
| `--auth-type` | string | context default | Auth type                 |

`--tail` is exact for a bounded read. Values below `-1` are invalid, and
`--tail` with `--follow` is refused because an endless stream has no final
tail. Service and tail filtering are enforced by `akt` after receipt because
older provider gateways may ignore those query parameters. Each received log
WebSocket frame is limited to 16 MiB before JSON decoding.

#### `akt provider lease-events [dseq]`

Stream Kubernetes events from a lease. The positional `dseq` behaves as in `lease-logs` (`--dseq` — **disabled pending feedback**, positional only, 2026-07).

Same flags as `lease-logs` (minus `--service`, `--tail`), plus `--follow`.
Each received event WebSocket frame is limited to 16 MiB before JSON decoding.

#### `akt provider lease-shell`

Open an interactive shell into a running container.

| Flag          | Type   | Default         | Description         |
| ------------- | ------ | --------------- | ------------------- |
| `--dseq`      | uint64 | required        | Deployment sequence |
| `--gseq`      | uint32 | active lease    | Group sequence      |
| `--oseq`      | uint32 | active lease    | Order sequence      |
| `--provider`  | string | active lease    | Provider address; resolved from the deployment's active lease when omitted |
| `--service`   | string | required        | Service name        |
| `--tty`       | bool   | `true`          | Allocate a TTY      |
| `--stdin`     | bool   | `false`         | Force stdin attachment for an explicit terminal command |
| `--auth-type` | string | context default | Auth type           |

Remaining arguments after `--` are passed as the shell command. Default: `/bin/sh`.
Interactive shells and explicit commands receiving piped input attach stdin
automatically. An explicit command launched from a terminal leaves stdin
detached so its remote exit can complete; `--stdin` opts back into attachment,
and `--stdin=false` explicitly detaches piped input.
EOF on attached stdin is not a command failure: `akt` waits for the provider's
remote exit result and returns that result. This prevents a successful
non-interactive command from printing its output and then exiting non-zero
solely because local stdin was already closed.

The current chain-SDK PTY transport remains context-cancellable, and akt
redacts the exact request JWT from any returned shell error. Its failed
handshake body and received WebSocket frames are not yet size-bounded.
Replacing that transport locally or upstreaming limits to the shared provider
client is a P0 gap; shell must not be described as having the 16 MiB log/event
frame limit until that work lands.

```bash
akt provider lease-shell --dseq 12345 --provider akash1prov... --service web -- /bin/bash
```

#### `akt provider send-manifest <sdl-file>`

Send an SDL manifest to provider(s) for an existing lease.

| Flag          | Type   | Default         | Description                                                   |
| ------------- | ------ | --------------- | ------------------------------------------------------------- |
| `--dseq`      | uint64 | required        | Deployment sequence                                           |
| `--provider`  | string | `""`            | Specific provider (default: all providers with active leases) |
| `--auth-type` | string | context default | Auth type                                                     |

Unlike the other lease-scoped commands, `send-manifest` fans out: with no
`--provider` it submits the manifest to **every** provider holding an active
lease for the deployment, which is the same delivery the `update` workflow's
manifest step performs (§2.3). Every provider is attempted even when an earlier
one rejects the manifest, each accepted submission is reported by full provider
address, and the command fails unless all of them accepted. A deployment with a
single active lease therefore needs no `--provider` at all.

#### `akt provider get-manifest [dseq]`

Retrieve the current manifest from a provider. The positional `dseq` supplies the deployment sequence. The `--dseq` flag is **disabled pending feedback** (positional only, 2026-07). `--provider` is resolved from the deployment's active lease when omitted.

#### `akt provider migrate-hostnames`

Migrate hostnames onto a deployment from whichever deployment currently holds
them on the same provider. The gateway addresses the **destination** lease only,
so there is no source flag.

| Flag                 | Type     | Default         | Description                            |
| -------------------- | -------- | --------------- | -------------------------------------- |
| `--dseq`             | uint64   | required        | Destination deployment sequence        |
| `--gseq`             | uint32   | active lease    | Destination group sequence             |
| `--provider`         | string   | active lease    | Provider address; resolved from the destination deployment's active lease when omitted |
| `--hostnames`        | []string | required        | Hostnames to migrate                   |
| `--auth-type`        | string   | context default | Auth type                              |

#### `akt provider migrate-endpoints`

Same pattern as `migrate-hostnames`, with `--endpoints` naming the IP endpoints
to migrate.

**Provider gateway output and stream contract:** provider status, lease
status, and manifest reads emit JSON by default; `--output json` and
`--output yaml` preserve the same field names and scalar types. Log and event
streams emit human lines by default, one compact object per line in JSON mode,
and one YAML document per record in YAML mode. Before opening a log, event, or
shell stream, `akt` checks lease status so a missing lease is a non-zero gateway
error instead of a successful empty stream. An EOF closes a bounded one-shot
stream successfully after its records are written; the same EOF is an error
under `--follow`. Log and event frames are capped at 16 MiB; the shell exception
and P0 boundary gap are documented above. Non-success gateway responses include
both the HTTP status and the provider's trimmed response body when one is
available.

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
  Leases:       52 (12 active, 40 closed)
  Bids:         156 (3 open, 12 matched, 138 lost, 3 closed)

Network Reconciliation:
  Last Block:   18234567
  Last Run:     2026-03-23T10:15:32Z
  Status:       completed
  Run:          akt store sync
```

Record totals include a non-zero breakdown for every known state. Records with
an unrecognized state are included as `other`, so the breakdown always accounts
for the displayed total.

Network reconciliation is separate from workflow persistence. A store that has
records but has never run an explicit reconciliation displays `Status: not yet
run`, omits the unknown block and time values, and displays `Run: akt store
sync`; this is an available action, not a fault. A completed reconciliation
reports the last chain height and run time without claiming that the local
snapshot remains continuously synchronized after the one-shot command exits.
The `Run` row remains visible so the next explicit reconciliation is always
discoverable.

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
| `--yes`, `-y` | bool | `false` | Confirm a destructive replacement non-interactively |

The importer strictly decodes and validates the complete export envelope before
it opens a write transaction. The `deployments`, `leases`, and `bids` members
are required non-null arrays, including when empty. JSON and YAML reject unknown
fields, trailing documents, and type mismatches rather than treating omitted,
null, or misspelled collection names as empty collections. It rejects
unsupported envelope or schema versions,
malformed identities, unknown record states, negative record timestamps or
heights, and nil deployment, lease, or bid records. `sync_state` is optional
because an exported store may not have
completed reconciliation yet; when present it is validated. Merge and replace
imports each apply all records in one bbolt write transaction. Replace clears
existing records only inside that transaction, after validation has succeeded.
A decode, validation, migration, or write error rolls the transaction back and
leaves the prior logical store state unchanged. `--dry-run` performs the same
validation against a disposable, transactionally consistent snapshot of the
existing database, or a disposable empty database when the context has no
store. It MUST NOT create the selected context directory or database, run a
migration against that database, or otherwise mutate its filesystem state.
Replacement requires the explicit `--replace` flag and an affirmative prompt,
or `--replace --yes`; setting `--merge=false` alone is rejected and never
selects the destructive mode. A dry run never prompts because it cannot mutate
the store.
Opening an existing source snapshot is bounded by both the caller's context and
a short database-lock timeout; another process holding the store cannot make a
dry-run wait indefinitely. Import decoding reads at most 64 MiB, including one
lookahead byte used to distinguish an exact-limit document from oversized
input. Files above that limit fail before JSON/YAML decoding or database work.

Exports enforce that same 64 MiB encoded-envelope ceiling before writing the
document prefix or payload. A successful JSON or YAML backup is therefore
accepted by the same binary's importer; an oversized store fails without
leaving a partial backup. Missing metadata or data buckets are corruption
errors rather than panics.

When `--file` is set, export writes a sibling temporary file, flushes and closes
it successfully, and only then atomically renames it over the destination. An
export, flush, or close failure removes the temporary file and MUST leave an
existing backup at the destination byte-for-byte unchanged.

#### `akt store sync [account]`

Reconcile the local store against on-chain state for the context's tracked
accounts (§6.7), then record the chain height reached in the sync state. Owner
resolution is, in order: the explicit positional account; configured tracked
accounts; the default account; then, for a `console-api` context without any of
those identities, all unique owners from its local deployment records. The
derived owner set is de-duplicated and sorted. If none exists, the command
fails with a direct request to pass an account address; it never opens a
nonexistent Console keyring.

Workflow commands record their own results (§6.6), but a single run can only
observe what passed through it. `akt store sync` is the escape hatch for
everything else: deployments created before `akt` was used or from another
machine, escrow balances and transferred amounts that move every block, leases
closed by a provider rather than by the user, and any run whose best-effort
store write failed.

```
$ akt store sync
Store Sync
  Accounts:     1
      Owner:    akash1zn43lmk4dmvcjmfhtaqk4wa9zpuru3xy0kzupu
  Deployments:  3
  Leases:       3
  Bids:         7
  Height:       18,234,567
```

Every reconciled account is listed in full; addresses are never abbreviated.

The optional positional `account` reconciles a single account instead of the
context's tracked accounts. Accounts are resolved the way `--from` is: a bech32
address is used as-is, any other value is looked up in the context's keyring.

Reconciliation is the §6.4 full reconciliation, run on demand: every deployment
owned by a tracked account, then that deployment's leases and bids, are queried
and written to the store. Existing records for the same keys are overwritten
with chain state; local-only metadata that the chain does not carry (`labels`,
`notes`, `tags`, `sdl_path`, `sdl_hash`) is preserved from the existing record.

Requires a chain RPC endpoint (capability `chain-query`, §2.10). A
`console-api` context without a network cannot run it.

### 2.6 Monitor Command

#### `akt monitor [rpc-endpoint]`

Hub-based real-time monitoring tool for network state, provider fleet health, oracle prices, and BME state. See [DESIGN.md section 1.4](DESIGN.md#14-the-monitor-command) for design goals and rationale.

`akt monitor` launches a hub that defaults to the **Network** dashboard and allows switching between three dashboards via Tab/Shift-Tab:

| Hub Tab | Dashboard | Content |
|---------|-----------|---------|
| **Network** (default) | Consensus, validators, governance proposals, governance parameters | [§8.3.8](#838-consensus-monitor-view-from-aktop), [§8.3.9](#839-validator-voting-view-from-aktop), [§8.3.11](#8311-governance-monitor-views) |
| **Provider** | Fleet health, versions, resources | [§8.3.10](#8310-provider-fleet-monitor-view) |
| **Oracle/BME** | Prices, health, vault state, mint status, ledger | [§8.3.12](#8312-oraclebme-monitor-view) |

Each dashboard is also directly accessible via its CLI subcommand. When launched via a subcommand, the hub starts on that dashboard but Tab/Shift-Tab still allows switching to other dashboards.

`akt monitor` is especially critical during coordinated chain upgrades, when the network halts at a target block height and online block explorers become unreliable. It connects directly to the user's RPC endpoint to provide an authoritative view of upgrade progress: which round and step the chain is in, which validators have come back online and are voting, and whether 2/3+ voting power has reached precommit.

**Endpoint resolution** (first match wins, shared by all subcommands):

1. Positional argument: `akt monitor https://rpc.akt.dev:443/rpc`
2. `--rpc` flag: `akt monitor --rpc https://rpc.akt.dev:443/rpc`
3. Active context RPC endpoint (from `context → network → endpoints.rpc[0]`)

If no endpoint can be resolved, the command exits with an error.

Built-in network templates place a WebSocket-capable RPC first because that is
the endpoint a flagless monitor launch selects. Ordinary HTTP-only RPCs may
remain later in the network's failover list for query commands, but MUST NOT be
the primary endpoint of a built-in template advertised for monitor use.

The monitor connects to `{rpc-endpoint}/websocket`. Help examples use
`https://rpc.akt.dev:443/rpc`, whose CometBFT WebSocket service is part of the
documented endpoint. An HTTP-only RPC gateway may still serve ordinary query
commands, but it is not a valid monitor example.

**Shared flags** (apply to `akt monitor` and all subcommands):

| Flag              | Type   | Default         | Description                                              |
| ----------------- | ------ | --------------- | -------------------------------------------------------- |
| `--rpc`           | string | context default | RPC endpoint (WebSocket-capable)                         |
| `--rest`          | string | auto-derived    | REST API endpoint (for governance/oracle/BME queries). Default: derived from the context's `endpoints.api[0]`. If no API endpoint is configured, falls back to the RPC host on port 1317 (standard Cosmos REST port). |
| `--insecure`      | bool   | `false`         | Skip TLS certificate verification for REST and gRPC provider probes |
| `--clean-cache`   | bool   | `false`         | Clear the local cache before start                       |

An explicit `--rest` always wins. Without it, a context API endpoint is used
only when the selected RPC is one of that same context network's configured RPC
endpoints. For an ad-hoc positional or `--rpc` override, the REST endpoint is
derived from that RPC: a terminal `/rpc` path becomes `/rest`; otherwise the
same hostname uses the standard Cosmos REST port 1317. WebSocket and Cosmos
`tcp` schemes are converted to their HTTP equivalents for REST requests. The
monitor MUST NOT combine an overridden RPC with an unrelated active-context
API. Its cache is always `<resolved --home>/cache`; an explicit `--home` MUST
govern the monitor just as it governs contexts, keyrings, stores, and action
logs.

For implicit context resolution only, a primary endpoint that exactly matches
a retired HTTP-only endpoint from an older built-in template is replaced at
runtime by that template's current WebSocket-capable primary. This compatibility
selection does not rewrite the context. Positional and `--rpc` endpoints are
explicit user choices and are never substituted.

An explicit positional or `--rpc` endpoint MUST work with no config file and
MUST NOT trigger first-run bootstrap. Monitor cache directory creation/open
errors are fatal startup errors. `--clean-cache` removes both `monitor.db` and
the legacy `top.db`; a failed deletion is reported and the monitor does not
claim the cache was cleared.

Standalone monitor construction MUST also fail if the CometBFT event client
cannot be constructed or started, or if the event service synchronously rejects
its `NewBlockHeader` subscription. Every failure after the cache opens uses the
same idempotent teardown as a completed monitor session: cancel and drain model
tasks, shut down any started event service, stop any started CometBFT client,
close the event bus, and close the cache database. The command MUST NOT launch
with a locally detected missing event producer and silently expose an empty
bus.

**Current event-readiness boundary (2026-08):** the pinned upstream CometBFT
client reports `Subscribe` success when it queues the JSON-RPC request, before
the server acknowledges it. A later server-side subscription rejection is
handled inside that client's private listener and cannot currently be returned
through monitor construction. Waiting for the matching acknowledgement,
buffering an event that arrives before it, and surfacing terminal resubscription
failure require a repository-owned `NewBlockHeader` transport. That remains
roadmap work; local startup success MUST NOT be interpreted as proof of
server-acknowledged event readiness.

**Hub navigation:**

| Key | Action |
|-----|--------|
| `Tab` | Switch to next dashboard (Network → Provider → Oracle/BME → Network) |
| `Shift-Tab` | Switch to previous dashboard |
| `1`/`2`/`3`/`4` | Switch sub-tab within the active dashboard (Network only) |

These keys belong to the monitor whenever its view is active. In standalone
mode, keypresses are delivered directly to the monitor model; the inactive TUI
resource router MUST NOT receive them. When the monitor is embedded in the
experimental TUI shell, `Tab`, `Shift-Tab`, and `1`/`2`/`3`/`4` still take
precedence over shell-level navigation. `Esc`/Back returns to the shell in
embedded mode. In standalone mode, `q` and Ctrl-C save monitor cache state and
quit.

The standalone view uses the full terminal height and renders its own bottom
help and RPC status lines. Help distinguishes dashboard navigation
(`Tab`/`Shift-Tab`) from Network sub-tab selection (`1`/`2`/`3`/`4`); it MUST NOT
describe both controls as one ambiguous "switch tabs" action.
An embedded monitor receives exactly the height remaining after the shell's
chrome; the parent MUST NOT size it for one height and clip it to another.

**Standalone operation**: `akt monitor` (and all subcommands) requires only an RPC endpoint. It does not require a keyring, default account, or chain-id. A monitoring-only context (with no `default-account`) or a bare `--rpc` flag is sufficient, making it usable by anyone observing the network. The group declares no local identity (§1.7): a context that *does* carry a keyring and a named `default-account` must not cause the monitor to open that keyring or prompt for it.

#### `akt monitor network [rpc-endpoint]`

Launches directly into the Network dashboard. This is the replacement for the former `akt top` command.

The Network dashboard has four sub-tabs:

| Key | Tab              | Description                                                                | Spec reference |
| --- | ---------------- | -------------------------------------------------------------------------- | -------------- |
| `1` | **Overview**     | Consensus state (height, round, step, elapsed, proposer), vote progress bars (prevote/precommit with power fractions), validator vote grid (`●`/`○`) | [§8.3.8](#838-consensus-monitor-view-from-aktop) |
| `2` | **Validators**   | Scrollable validator list with moniker, voting power, prevote/precommit status, block signing history bar, proposer indicator | [§8.3.9](#839-validator-voting-view-from-aktop) |
| `3` | **Governance**   | Recent proposals with status, voting deadline, and vote tallies | [§8.3.11](#8311-governance-monitor-views) |
| `4` | **Parameters**   | Module-by-module governance parameter browser | [§8.3.11](#8311-governance-monitor-views) |

The Governance view queries the most recent proposals from
`/cosmos.gov.v1.Query/Proposals` through the selected RPC endpoint, newest
first. It remains useful when no proposal is active by showing recent completed
proposals. For a proposal in the voting period, the monitor also calls
`/cosmos.gov.v1.Query/TallyResult` and displays the current tally; completed
proposals display their final tally. The view renders through the same
`RenderProposalList` function as `akt query gov proposals`, scrolls with `j/k`,
and refreshes with `r`.

The Parameters view obtains the complete modern governance parameter object
from `/cosmos.gov.v1.Query/Params` through the monitor's selected RPC endpoint
and renders it with the same pretty renderer as `akt query gov params`. It MUST
NOT treat one legacy v1beta1 REST subtype (`voting`, `deposit`, or `tallying`)
as the combined response: doing so turns the other live values into plausible
zeros. Voting period, deposit, quorum, threshold, veto, expedited, and burn
fields shown by monitor therefore match the single-shot CLI query.

#### `akt monitor provider [rpc-endpoint]`

Launches directly into the Provider dashboard. Displays real-time provider fleet monitoring:

- **Scan progress**: Progress bar showing checked/total providers and online count.
- **Version distribution**: Versions sorted newest-first (semver-aware, handles `-rc` suffixes). Dot visualization with `●` for selected version, `○` for others. h/l to select version filter.
- **Provider list**: Scrollable table with URL, version, CPU, memory, GPU, location. Filtered by selected version.
- **Provider detail**: Enter on a provider shows node-level breakdown with CPU, memory, GPU model + count.

Provider health and detail probes verify certificates on both REST and gRPC
paths by default. `--insecure` is the single explicit opt-out for both paths.

Every provider-list rebuild reconciles the selected version by value against
the newly sorted version set. If the version disappeared, selection falls back
to the first available version; if no versions remain, selection is cleared.
The selected index MUST therefore remain in bounds while live scan results add
or remove versions. Table rows and Enter-to-detail mapping use the same selected
version filter.

Provider detail fetches are correlated by provider identity. A response is
applied only while the matching provider remains the active detail target;
responses from a provider the user left or superseded are discarded.

Data sources: on-chain provider list (ABCI query), per-provider health (gRPC port 8444 preferred, REST `/status` + `/version` fallback), active leases (REST, for priority scheduling).

Cache: smart scheduling (online: 1m, recently offline: 5m, long-term offline: 6h), priority queue, max 10 concurrent checks, chain re-sync every 10m.

The provider pipeline starts with the monitor itself, even when another
dashboard is initially visible. It loads cached providers immediately,
reconciles the full on-chain provider list at startup, dispatches due health
checks, re-syncs the chain every 10 minutes, and saves the cache every 30
seconds. `r` performs an immediate reconciliation without replacing those
periodic schedules.

#### `akt monitor oracle [rpc-endpoint]` / `akt monitor bme [rpc-endpoint]`

Both commands are aliases that launch directly into the **Oracle/BME** dashboard. The combined dashboard displays:

- **Aggregated prices section**: Per-denom aggregated price data with TWAP, median, min/max, source count, deviation (bps), and timestamp.
- **Price health section**: Health status (color-coded: green=healthy, red=unhealthy), failure reasons (when unhealthy), minimum sources check, deviation check, total vs healthy source counts.
- **Price history section**: Recent price feed entries table with asset denom, base denom, price, source, and timestamp.
- **BME status section**: Fields in order: Status (color-coded: green=healthy, yellow=warning, red=halt CR/halt Oracle), Mints (Allowed/Halted), Refunds (Allowed/Halted), Collateral Ratio, Thresholds (nested: Warn, Halt).
- **Vault section**: Balances, total burned, total minted, remint credits. All amounts formatted using `FormatCoin()`.
- **Ledger section**: Recent ledger entries table with route, ID, status, burned, minted, spread, remint accrued, remint issued. Status is always a full word — `Executed` (green), `Pending` (yellow), `Canceled (<reason>)` (red) — never a one-letter abbreviation (§10.10 BME).

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

Start an MCP (Model Context Protocol) server over stdio transport. The server exposes Akash Network tools that compatible clients can invoke to query chain state, check provider status, and (with explicit permission) perform mutating operations.

Configuration is resolved from the active akt context (network, keyring,
default account) when one exists. A resolved `--console-api-key` or
`AKT_CONSOLE_API_KEY` is also a complete Console-only MCP configuration: with
no context, the server starts with Console tools and omits chain/provider tools
instead of requiring an unrelated provider-auth setting. Console tools are
registered when a key resolves through either process-level source or the
active context credential.
This contextless mode is read-only. `--enable-writes` requires an explicitly
selected context so the mandatory Console action log has a stable destination;
the server fails before registering tools when that audit boundary is absent.
`akt mcp` never launches the interactive first-run wizard because stdio is its
protocol channel; without either rail it returns the MCP-specific no-tools
diagnostic.

| Flag                | Type   | Default | Description                                                                       |
| ------------------- | ------ | ------- | --------------------------------------------------------------------------------- |
| `--console-api-key` | string | `""`    | Console API key for this process; overrides environment and context credentials.  |
| `--enable-writes`   | bool   | `false` | Enable write tools (on-chain transactions and provider mutations). Without this flag, only read-only query tools are available. |

**Permission model:**

By default, only read-only query tools are registered. This prevents an MCP client from sending unapproved transactions or performing mutating operations. The `--enable-writes` flag must be explicitly passed to enable write tools. This flag covers both on-chain transactions (which require keyring signing), Console mutations, and mutating provider REST API calls (e.g., submitting manifests).

Enabling MCP writes does not create a second mutation path. Chain broadcasts
MUST pass through the same transaction action-log decorator used by `akt tx`,
Console tools MUST use a client carrying the selected context's action logger,
and provider mutations MUST use the shared provider action recorder. Each
attempt records the same success, pending, or failed entry required for the
equivalent CLI operation. Read-only MCP tools MUST NOT append action-log
entries.

Every provider REST tool requires the provider's full owner address and uses
the provider record's on-chain `host_uri` as its gateway URL. No tool accepts an
independent URL that could capture a wallet credential or target one gateway
while naming another provider in the audit log. Once a manifest tool's
provider, DSEQ, and manifest inputs are valid, it records exactly one provider
attempt, including provider lookup, local authentication, or
gateway-construction failures that occur before an HTTP request.

The inventory is capability-driven. A chain RPC registers 19 read tools; a
Console credential registers eight. With both configured, `tools/list`
returns 27 read-only tools. `--enable-writes` adds four chain/provider writes
and two Console writes, for 33 tools total. A server with only one rail exposes
only that rail's subset.

**Read-only tools (up to 27 tools):**

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
| `console_list_deployments`     | List deployments belonging to the configured Console account              |
| `console_get_deployment`       | Get one Console-managed deployment                                         |
| `console_list_bids`            | List bids for a Console-managed deployment                                 |
| `console_wallet_balance`       | Available, in-deployment, and total Console credits in USD                  |
| `console_usage_history`        | Get Console spend history                                                  |
| `console_list_providers`       | List providers in the Console catalog                                      |
| `console_get_provider`         | Get one provider from the Console catalog                                  |
| `console_gpu_prices`           | List current Console GPU pricing                                           |

**Write tools (only with `--enable-writes`, up to 6 tools):**

| Tool Name                      | Description                                                                |
| ------------------------------ | -------------------------------------------------------------------------- |
| `akash_close_deployment`       | Close a deployment (on-chain transaction)                                  |
| `akash_create_lease`           | Create a lease from a bid (on-chain transaction)                           |
| `akash_close_lease`            | Close an active lease (on-chain transaction)                               |
| `akash_submit_manifest`        | Resolve a provider owner/address and submit its manifest to the registered gateway |
| `console_close_deployment`     | Close a Console-managed deployment                                         |
| `console_deposit`              | Add USD credit to a Console-managed deployment                             |

**Transport:** stdio (JSON-RPC over stdin/stdout). Designed for use with any MCP-compatible client.

SIGINT and SIGTERM cancel the stdio server context, stop the blocked input
loop, allow in-flight tool workers to return, and exit cleanly without printing
`context canceled`. Closing stdin remains a normal clean shutdown. The request
contexts passed to tool handlers retain command-level cancellation while also
observing these process signals.

**Client implementation:** Uses `v1beta3.LightClient` from chain-sdk for read-only mode, `v1beta3.Client` for write mode, and the shared authenticated provider gateway client for provider REST tools. Every provider REST tool accepts a provider owner bech32 address, resolves that provider's current `host_uri` from chain state, and connects only to the registered endpoint. MCP input never supplies an arbitrary URL to a client carrying a wallet JWT or certificate. Keyring contexts attach a short-lived wallet-signed JWT for protected lease/service/manifest calls. That JWT uses granular claims restricted to the resolved provider, deployment identity, and exact operation scope; it MUST NOT use full lease access, so a provider cannot replay it for another provider or operation. Missing signing identity is reported before the request. Public provider status uses the same registered-endpoint resolution without attaching credentials.

**Money units:** `console_wallet_balance` returns explicit `available_usd`, `in_deployments_usd`, and `total_usd` numbers. Console's integer µACT wire values must not leak through this semantic interface.

**Default account handling:** Tools that accept an `owner` parameter (e.g., `akash_list_deployments`, `akash_list_leases`) default to the context's `default-account` when the parameter is omitted. If no `default-account` is configured (e.g., a monitoring-only context), the `owner` parameter is **required** — the tool returns an error explaining that the owner must be specified explicitly when no default account is available.

Starting the MCP server is an on-demand identity operation (§1.7). Listing
tools and invoking public or explicitly scoped read tools MUST NOT open the
configured keyring. A read tool that omits its owner resolves a named
`default-account` when that call is handled. On a keyring context,
`--enable-writes` explicitly opts into resolving the signer during startup so
every advertised chain/provider write tool is usable. Enabling writes on a
Console-only context does not make that context require a local keyring.

**Numeric argument contract:** Sequence identifiers (`dseq`, `gseq`, and
`oseq`) are positive whole numbers. Pagination values (`skip` and `limit`) are
non-negative whole numbers; zero retains the documented default behavior.
Their tool schemas declare these bounds and integer steps. The server rejects
negative, fractional, non-finite, out-of-range, and JSON-unsafe integer values
before a handler can coerce them to a different identifier or silently
substitute a default. Required strings and strings whose schema declares a
positive `minLength` reject empty or whitespace-only values. An optional blank
string has the same meaning as omission when the handler documents an optional
filter or default. Tool objects are closed inputs: an argument absent from the declared
schema is rejected with its exact name, so a typo cannot silently select a
default.

**Examples:**

```bash
# Read-only mode (default)
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

The `akt console` group drives the Akash Console managed-wallet API (§7): deployments are created, funded, and closed by the Console's server-side wallet — no local keyring or gas handling is involved. Authenticated commands resolve the API key per §7.1 (`--console-api-key` flag > `AKT_CONSOLE_API_KEY` > per-context stored credential) and fail with a pointer to `akt console login` when none is found. The base URL resolves per §7.2 (`--console-api-url` flag > context `console-api-url` > default). Public catalog commands (`provider`, `gpu`, `template`) work without a key and without a configured context. Group, deployment-create, bid-list, and lease-create help identifies `akt deploy <sdl-file>` as the preferred one-shot path that performs those steps together; the subcommands remain available for inspection and manual control.

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
| `akt console deployment create <sdl-file> [deposit-usd]` | `--deposit <usd>` (alternative to positional; min 0.5) — **disabled pending feedback** (positional only, 2026-07) | Create a deployment; prints `dseq` + tx hash, the Console default `autoTopUp: {enabled: true, frequency: daily}`, and the exact command that disables it. It does not invent a deployment `state`; `deployment get` is authoritative for the later open/active transition. The deposit uses the unified cross-rail syntax (§7.4): `5`, `5usd`, or `$5` (min $0.50); coin forms like `5000000uakt` fail with the cross-rail error. The returned manifest is cached at `contexts/<name>/manifests/<dseq>.json` for `lease create`. |
| `akt console deployment update <dseq> <sdl-file>`   |                                                | Update the deployment's SDL.                                                                   |
| `akt console deployment close <dseq>`               |                                                | Close a deployment. Idempotent only when the API unambiguously reports already closed or absent; a rejection that merely contains `closed` remains an error. |
| `akt console deployment deposit <dseq> [amount-usd]` | `--amount <usd>` (alternative to positional; > 0) — **disabled pending feedback** (positional only, 2026-07) | Add funds to the deployment's escrow. The amount uses the unified cross-rail syntax (§7.4): `10`, `10usd`, or `$10`; coin forms fail with the cross-rail error. |
| `akt console deployment settings <dseq> [true\|false]` | `--auto-top-up true\|false` (alternative) — **disabled pending feedback** (positional only, 2026-07) | Show settings when no value is given; set auto-top-up when a positional or flag value is present. |
| `akt console bid list <dseq>`                       |                                                | List bids for the deployment's open orders.                                                    |
| `akt console lease create <dseq> [provider]`        | `--gseq` (1), `--oseq` (1), `--provider` (alternative to positional) — **disabled pending feedback** (positional only, 2026-07), `--manifest <file>` | Accept a bid; the manifest defaults to the one cached by `deployment create`. |

**Wallet and usage:**

| Command                       | Flags                        | Description                                                     |
| ----------------------------- | ---------------------------- | ---------------------------------------------------------------- |
| `akt console wallet list`     |                              | List managed wallets. `creditAmount` is dollar-scale per the `/v1/wallets` contract (shown as `$X.XX`, no µ scaling), with the wallet's `denom` when the API reports one. |
| `akt console wallet balance`  |                              | Available / in-deployment / total balance in USD. `total` is authoritative; the allocation fields carry `allocationStatus: provisional` and a note that they can lag recent creates/closes. |
| `akt console wallet settings [true\|false]` | `--auto-reload true\|false` (alternative) — **disabled pending feedback** (positional only, 2026-07) | Show settings when no value is given; set auto-reload otherwise. Reports `{ autoReloadEnabled, configured }` on every path — after a write, after a read, and for an account that has never configured auto-reload (the API's 404, which reports `configured: false` plus a `note` naming the command that enables it). The raw API record is never printed: like `deployment settings`, the command reshapes it so one command has one output shape. |
| `akt console wallet cost`     |                              | Estimated weekly cost in USD.                                    |
| `akt console usage [from] [to]` | `--from`, `--to` (YYYY-MM-DD, alternatives) — **disabled pending feedback** (positional only, 2026-07) | Daily spend history for the managed wallet. `totalSpent` is the spend within the requested range (sum of the daily values, order-independent); `lifetimeSpent` is the API's cumulative figure as of the range end, omitted when the range is empty. Omitted dates use the API defaults (last 30 days). |

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
| `akt console apikey delete <id>` |                                                | Delete a key; a missing key (404) is a no-op. Pretty output acknowledges the full ID; JSON/YAML return `{id, deleted:true}`. |
| `akt console jwt create`         | `--ttl` (300), `--scope` (csv, default `status,logs,events,shell,send-manifest,get-manifest`) | Mint a short-lived provider-scoped JWT. |

**Provider gateway (live lease operations):**

Managed (Console-API) contexts reach provider gateways directly, without a wallet or local key: each command resolves the deployment's first active lease via the Console API, looks up the provider's `hostUri`, and mints a scoped JWT via `POST /v1/create-jwt-token` that the gateway accepts as `Authorization: Bearer`. One-shot calls use a 300 s token; streaming/interactive modes (`--follow`, `--watch`, `shell`) use 3600 s. Without an active lease the commands fail listing the states of the leases that do exist.
Log, event, and shell calls request their operation scope plus `status` because
the shared gateway boundary verifies the lease before opening a stream.
Log records identify the provider-reported runtime pod in their `name` field
(for example, `web-5bfc685996-wv9vs`), not merely the SDL service. Filtering for
`web` accepts either the exact name or a `web-` pod prefix with a non-empty
runtime suffix and rejects incomplete or boundary-less names such as `web-` or
`webhook`; structured output preserves the full runtime name. A malformed JSON
frame or non-EOF websocket read failure MUST fail the command; a prior valid
record cannot turn a truncated stream into success. WebSocket close code 1000
MUST complete successfully even when it carries optional reason text. Every
other close code MUST retain a non-empty failure reason when the peer omits
that text. Record delivery MUST observe caller cancellation while the output
consumer is backpressured and MUST NOT emit the pending record after
cancellation wins. Provider EOF remains normal completion only for a bounded
one-shot read.

| Command                                            | Flags                                       | Description                                                                                        |
| -------------------------------------------------- | ------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `akt console logs <dseq> [service]`                | `--follow`, `--tail N`, `--service` (alternative) — **disabled pending feedback** (positional only, 2026-07) | Stream container logs from the lease's provider (JWT scopes `logs,status`).                    |
| `akt console events <dseq>`                        | `--follow`                                  | Stream Kubernetes events from the lease's provider (JWT scopes `events,status`).                     |
| `akt console status <dseq>`                        | `--watch`, `--interval` (5s)                | Live lease status from the provider gateway (JWT scope `status`); with `--watch`, snapshots are re-printed each interval until interrupted. `deployment get` remains the Console-API view. |
| `akt console shell <dseq> <service> [-- command...]` | `--stdin`                                 | Interactive shell in a lease container, default `/bin/sh`; exec is the same operation with an explicit command (JWT scopes `shell,status`). TTY auto-detected; terminal stdin is detached from explicit commands unless `--stdin` is supplied. |
| `akt console screen <sdl-file>`                    |                                             | Client-side bid screening: derive resources from the SDL and list the providers able to run it (public endpoint, no key needed). |

Per the positional-primary convention (§3.8), every console command takes its primary value(s) positionally; the equivalent flags remain as overrides and a positional value wins when both are given. (2026-07: the flag twins marked *disabled pending feedback* above are commented out in code for the positional-only UX trial — the positional form is the only way while the trial runs; the original flag definitions are preserved in `FEEDBACK(2026-07)` comments for restoration.) Default structured reads are indented JSON, while human acknowledgements and streams use the command-specific pretty forms described below. USD values at or above one cent render with two decimals. A nonzero sub-cent value renders with up to six decimals and trailing zeros stripped; a magnitude below one millionth of a dollar renders as `$<0.000001` (or `-$<0.000001`) rather than the false `$0.00`. Zero remains `$0.00`. State-changing calls are recorded in the context's action log as `type=console` entries (§5.6). No command ever prints a Console API key, except the one-time secret from `apikey create`.

**Console output contract:** the API's JSON field names and value types are the
canonical structured representation. `--output json` emits that representation;
`--output yaml` is a semantic translation of the same JSON tree, preserving raw
embedded objects, strings, and integer precision. Default human output remains
command-specific. Streaming gateway commands emit one compact JSON object per
record in JSON mode and one YAML document per record in YAML mode; pretty mode
retains the human log/event lines. This applies equally to bounded and
`--follow` streams.

Shell is the one command-shaped stream. In pretty mode it remains an
interactive byte stream. With `--output json` or `--output yaml`, shell requires
an explicit command after `--`, runs it without a PTY, and emits exactly one
object with string fields `stdout` and `stderr`. A structured interactive shell
is refused before opening the provider connection. The same contract applies
to `console shell` and `provider lease-shell`. Interactive shells and commands
with piped input attach stdin automatically. Explicit commands launched from a
terminal leave it detached unless `--stdin` is supplied, so the provider can
deliver the remote exit result without waiting indefinitely for terminal
input. If both the remote command and
structured-output rendering fail, the returned error preserves both causes so
callers can classify either failure with `errors.Is`.

Mutation acknowledgements are structured in JSON/YAML mode. Deployment close
emits `{dseq, state, already_closed}` and deposit emits
`{dseq, amount_usd, status}`. Template SDL is byte-for-byte deployable YAML in
default/pretty mode; JSON/YAML mode wraps the exact source text as `{sdl: ...}`
so comments and ordering are not lost.

---

### 2.10 Capability Gating

The active context's configuration determines a **feature set**: which transports are usable and therefore which command groups can work.

| Capability | Derived from | Gates |
|---|---|---|
| `chain-query` | network has ≥1 RPC endpoint | `query`, `monitor` |
| `chain-tx` | `auth-method: keyring` and network has ≥1 RPC endpoint | `tx` |
| `provider` | network has ≥1 RPC endpoint | `provider` |
| `console` | Console API key resolvable (§7.1) | Console-backed command groups |

Commands declare requirements via a cobra annotation (`akt.requires`, package `internal/capability`); alternatives are separated by `|` (e.g. workflow commands require `chain-tx|console`). A command whose requirement the context cannot satisfy fails fast with the missing capability and its remedy instead of erroring mid-transport.

`chain-tx` checks identity mode without opening the keyring; key existence and
funding remain execution-time checks. Raw `akt tx` execution has an independent
auth boundary and is rejected under `console-api` even when command gating is
`off` or `--node` supplies an RPC endpoint. Managed-wallet writes use
`akt deploy/update/close` or `akt console`; a network override supplies a
connection, never a local signing identity.

Presentation is configurable while UX feedback is collected (`defaults.command-gating`):

| Mode | Behavior |
|---|---|
| `dim` (default) | Unavailable commands stay listed, marked `[unavailable]` in help, and fail fast with an explanation. |
| `hide` | Unavailable commands are removed from help listings (direct invocation still fails fast with the explanation). |
| `off` | No gating; commands fail wherever the missing transport is first touched. |

Example: a network-less `console-api` context (API key only) lists and runs only Console commands; `tx`, `query`, `provider`, and `monitor` are dimmed or hidden and explain that an RPC endpoint must be added.

---

### 2.11 SDL Commands

Transport-independent SDL authoring, ported from console-axi (`src/sdl/templates`, `src/sdl/lint.ts`). All `akt sdl` subcommands run entirely locally: no context, key, or RPC endpoint is required, and the group declares no capability requirements. The group declares no local identity either (§1.7), so a configured keyring is never opened and a named `default-account` is never resolved — `akt sdl validate` on a context with a `file` or `os` keyring must not prompt for a passphrase or an OS unlock.

#### `akt sdl scaffolds`

List the built-in SDL scaffolds (alias: `akt sdl templates`, matching the reference CLI's historical name).

| Scaffold        | Shape                                                                                        |
| --------------- | -------------------------------------------------------------------------------------------- |
| `web`           | Single web service with one HTTP port exposed to the internet (nginx:1.27, 0.5 CPU / 512Mi)  |
| `gpu`           | GPU workload (ML/inference) with an nvidia model requirement (pytorch image, a100, 4 CPU / 16Gi) |
| `multi-service` | App + postgres:16 database with a persistent volume (`beta2`) and service-to-service networking |
| `ip-lease`      | Service with a dedicated public IP — SDL v2.1 `endpoints` + `expose ... to ip`               |

#### `akt sdl init <scaffold>`

Generate SDL YAML on stdout, pipeable into `akt sdl validate -` or redirected to a file for `akt deploy`. The output is self-checked against the validator before printing. Flags are generation parameters with per-scaffold defaults — not positional-argument twins — so the zero-flag invocation always produces a deployable SDL. Every explicitly set generation parameter is checked with the same parser and lint rules as `akt sdl validate` before stdout is written. A value that would make the generated SDL invalid exits 2, names the changed flag(s) and validation reason, and emits no SDL. The post-generation self-check reports an internal invariant failure only when a built-in scaffold with its defaults is invalid. Pricing defaults to a 10000 uact/block ceiling (100000 for `gpu`) so bids arrive.

This command is a raw-document generator: stdout is always the deployable YAML
document. An explicitly supplied `--output`/`-o` is rejected as a usage error,
including `-o yaml`, rather than being ignored or wrapping the document. The
default output setting in configuration does not alter the generated document.

| Flag          | Type        | Description                                              |
| ------------- | ----------- | --------------------------------------------------------- |
| `--name`      | string      | Service name (default per scaffold: `web` / `app`)        |
| `--image`     | string      | Container image pinned to a non-latest tag or valid SHA-256 digest, e.g. `nginx:1.27` or `nginx@sha256:<64-hex>` |
| `--port`      | int         | Container port, 1-65535 (default 80; 8080 for `gpu`)      |
| `--as`        | int         | External port, 1-65535 (default 80)                       |
| `--cpu`       | string      | CPU units, e.g. `0.5` or `500m`                           |
| `--memory`    | string      | Memory size, e.g. `512Mi`, `2Gi`                          |
| `--storage`   | string      | Storage size, e.g. `1Gi` (sizes the persistent volume for `multi-service`) |
| `--count`     | int         | Replica count, minimum 1 (default 1)                      |
| `--price`     | int         | Max price per block in uact, minimum 1                    |
| `--env`       | stringArray | Environment variable `KEY=value` (repeatable)             |
| `--gpu`       | int         | GPU units, minimum 1 (`gpu` scaffold, default 1)          |
| `--gpu-model` | string      | NVIDIA GPU model (`gpu` scaffold, default `a100`)         |

#### `akt sdl validate <file>`

Validate an SDL offline (`-` reads stdin). Parsing and schema/relational validation use `pkg.akt.dev/go/sdl` — the same parser behind `akt deploy` and the chain tx commands — followed by lint rules ported from the reference:

- **Unpinned image** (error): every service image must carry an explicit tag or `@sha256:` digest; untagged images and `:latest` are rejected as non-reproducible.
- **Pricing denom**: `uact` passes — it is the deployment pricing denom on **both** rails. `uakt` produces a **warning**, not an error: `akt` auto-resolves the deployment deposit to `uact` on the chain rail (`DetectDeploymentDeposit`) and on the console rail alike, and the chain requires the group price denom to equal the deposit denom (`Mismatched denominations (uact != uakt)`), so a `uakt`-priced SDL fails at `ValidateBasic()` before broadcast. It stays a warning rather than an error only because passing an explicitly matching `--deposit <amount>uakt` is a legitimate escape hatch; the reference hard-rejects anything but `uact`. Any other denom is an error, matching the reference.

The command has three outcomes, and each one has its own exit code:

| Exit | Outcome                | Output                                                                              |
| ---- | ---------------------- | ----------------------------------------------------------------------------------- |
| `0`  | Valid (warnings allowed) | Summary on stdout (`valid: N service(s), M group(s), K warning(s)`) plus any warnings |
| `1`  | Invalid                | Every parse/lint error on stderr                                                     |
| `2`  | The document could not be read | Usage error on stderr ([§11.2](#112-exit-codes)); nothing was validated        |

Exit `2` covers both input paths uniformly: a missing or unreadable `<file>` and
a failed read of stdin for `-` are the same class of usage error, and neither is
reported as an invalid SDL — nothing was parsed, so there is no verdict to give.

With `--output json` or `--output yaml`, the command writes one structured result
to stdout with the following stable shape, then preserves the same exit-status
contract. `errors` and `warnings` are arrays (empty rather than null); each issue
contains `path`, `message`, and an optional `hint`. Human-readable issue lines
are not mixed into structured stdout.

```yaml
valid: true
services: 1
groups: 1
errors: []
warnings: []
```

```bash
# Generate, self-check, and validate
akt sdl init web --image nginx:1.27 > deploy.yaml
akt sdl validate deploy.yaml

# Pipe without touching disk
akt sdl init gpu --gpu-model h100 | akt sdl validate -
```

---

---

### 2.12 Version and Completion Commands

#### `akt version`

Print the binary's version. Build metadata is injected at link time by the
Makefile (`main.version`, `main.commit`, `main.date`); a binary built without
those flags reports `dev`.

| Flag     | Type | Default | Description                     |
| -------- | ---- | ------- | ------------------------------- |
| `--long` | bool | `false` | Print full build information    |

```bash
$ akt version
akt 0.0.1-143-g909f173 (commit: 909f1735b99d83a9ab52a0e6bee32ca7e7402672, built: 2026-07-27T04:20:51Z)

$ akt version --long
version:    0.0.1-143-g909f173
commit:     909f1735b99d83a9ab52a0e6bee32ca7e7402672
built:      2026-07-27T04:20:51Z
go:         go1.26.1
platform:   darwin/arm64
build tags: osusergo,netgo,ledger,muslc,gcc
```

The long form is the form to include in bug reports: the build tags and
platform determine which keyring backends and cgo-dependent features are
compiled in.

#### `akt completion <shell>`

Generate a shell completion script for `bash`, `zsh`, `fish`, or `powershell`.
Completion is dynamic for context and network names.

```bash
# Load for the current session
source <(akt completion bash)

# Install permanently (zsh)
akt completion zsh > "${fpath[1]}/_akt"
```

Both commands run without a configured context (§2.10): they require no
transport.

---

## 3. Flag Specification

### 3.1 Global Persistent Flags

Applied to every command via the root command's `PersistentFlags()`.

| Flag        | Short | Type   | Default                  | Description                                                      |
| ----------- | ----- | ------ | ------------------------ | ---------------------------------------------------------------- |
| `--home`    |       | string | `$AKT_HOME` or XDG default | Home directory for config, contexts, and keyrings             |
| `--context` |       | string | config `current-context` | Active context name (overrides AKT_CONTEXT)                      |
| `--keyring-backend` |  | string | context's keyring `backend` | Keyring backend for this invocation: `os`, `file`, `test`, `kwallet`, `pass`, `memory` (overrides AKT_KEYRING_BACKEND) |
| `--keyring-dir` |     | string | context's keyring `dir`  | Keyring directory for this invocation (overrides AKT_KEYRING_DIR) |
| `--output`  | `-o`  | string | `"pretty"`               | Output format: `pretty`, `json`, `yaml`. For workflows, also accepts `jsonl` (see 2.3.8). |
| `--interactive` | `-i` | bool | `false`              | Force interactive mode even when `defaults.interactive` is `false` in config or no TTY is detected. Has no effect when interactive mode is already enabled (the default). **Two effects**: (1) Workflow commands (`deploy`, `update`, `close`) use TUI progress display instead of JSONL. (2) Commands that auto-suppress prompts and spinners in non-TTY contexts will show them. Does **not** launch the root TUI application. |
| `--verbose` | `-v`  | count  | `0`                      | Increase output verbosity. Stacks: `-v` (level 1) shows operational detail (gas estimates, endpoint selection, config resolution); `-vv` (level 2) adds debug diagnostics (RPC request/response dumps, full stack traces). Default (no flag) shows progress/status messages. Mutually exclusive with `--quiet`. |
| `--quiet`   | `-q`  | bool   | `false`                  | Suppress all informational output (progress messages, status lines, confirmations). Only data output (query results, transaction results) and errors are emitted. Useful for scripting. Mutually exclusive with `-v`. |

`--keyring-backend` and `--keyring-dir` are **global**, not transaction-local.
Key storage is a property of the invocation, not of signing: listing, adding,
exporting, and querying keys need the same override that a transaction does,
and on a host whose configured backend is unavailable (§1.5) the override is
the only way to reach the keys at all. They are registered once on the root
command, carry an empty default so that leaving them unset never shadows the
context's persisted `keyrings[*].backend` / `keyrings[*].dir`, and are bound to
Viper so `AKT_KEYRING_BACKEND` / `AKT_KEYRING_DIR` (§1.9) resolve through the
normal flag > env > config > default chain. An unknown backend value is a usage
error. A supplied override applies to every keyring the invocation opens,
including the one behind `akt context keys`.

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
| `--sign-mode`        |       | string   | `"direct"`                  | Signing mode: `direct`, `amino-json`, `direct-aux`, `eip-191` |
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

Key storage is selected by the global `--keyring-backend` / `--keyring-dir`
flags (§3.1), which every command inherits. Transaction commands do not
register their own copies: a transaction-local duplicate shadowed the global
flag, so the documented `AKT_KEYRING_BACKEND` / `AKT_KEYRING_DIR` environment
variables never reached a `tx` invocation, and its non-empty `os` default
stood ready to override the context's persisted backend.

`--sign-mode` and `--broadcast-mode` are closed enums: values outside their
advertised sets are usage errors. For online construction, simulation, and
broadcast, an explicit `--chain-id` must agree with the selected context.
`--offline` may use a different explicit chain ID because it performs no
context-node work. These checks run before generate-only or workflow dry-run
output is emitted.

Every executable transaction leaf, including subtrees adopted from Cosmos SDK
and IBC modules, runs the same akt transaction preflight before its handler.
The preflight resolves codecs, signer, chain identity, and the RPC endpoint
from the selected context; `--node` remains the only per-invocation endpoint
override. Account lookup, gas simulation, and broadcast must therefore reach
that resolved endpoint and must never fall back to an SDK localhost default.
A missing transaction client is a normal CLI error, never a panic.

When `--fees` is non-empty it is authoritative: both configured and explicit
`--gas-prices` values are cleared before the transaction factory is built.
Without fixed fees, the effective gas price follows the normal precedence
chain (flag > environment > context network > built-in default). A simulation
response with a non-zero SDK code is a failed transaction and exits non-zero;
simulation remains non-mutating and is not written to the action log.
The active fee string is parsed before it reaches the SDK transaction factory:
fixed fees use the integer-coin grammar and gas prices use the decimal-coin
grammar. Invalid input is a normal error naming `--fees` or `--gas-prices`,
never an SDK panic.

`--generate-only` accepts a signer address that is not stored in the local
keyring. The address identifies the unsigned message and does not imply a
signing-key lookup or even opening the configured backend. `--dry-run` has the
same address-only identity contract because simulation does not sign. A signer
name still resolves through the selected keyring on demand. A transaction that
will sign opens and validates its keyring during startup.
`akt tx staking create-validator` additionally accepts `--node-id`, `--ip`,
and `--p2p-port` (default `26656`). During `--generate-only`, when node ID and
IP are both present, the unsigned transaction note is exactly
`<node-id>@<ip>:<p2p-port>`. The P2P port MUST be in the inclusive range
1–65535; an invalid explicitly supplied port is a usage error and MUST NOT
produce a transaction document.
Multisign assembly accepts only a legacy amino multisig record and validates
that each signature batch contains an entry for every transaction before
indexing it; ordinary keys and short batches are normal input errors, never
panics.

`--generate-only -o json` emits a transaction object at the top level on every
transaction leaf; implementations must not JSON-encode an existing transaction
byte payload as a base64 string. Signing commands write the signed transaction
or signature-only payload to stdout unless `--output-document` is supplied.
Prompts, progress, and validation diagnostics remain on stderr.

Commands whose purpose is an interactive editor or selector must check for a
TTY before starting terminal rendering. In a non-interactive invocation they
fail promptly with a message naming the TTY requirement; `--yes` skips
confirmations but cannot invent answers to value-selection prompts.

For batched multi-message transactions, a batch size of zero means unlimited:
the complete message set is submitted to the construction/simulation/broadcast
function exactly once. It must not be used as a loop increment. An empty
message set is a clear no-transaction error, never a successful command with
empty output. When an explicit positive batch size produces multiple
transactions, JSON and YAML output contain one top-level array; pretty output
renders each transaction in order.

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

An explicit `--node` replaces the context RPC endpoint for every query that
performs RPC work. Local derivations such as `ibc-transfer escrow-address`,
`module-name-to-address`, and `wasm build-address` reject `--node`; they also
reject `--height` because they do not read chain state. Queries that cannot
select a historical snapshot (`blocks`, `tx`, `txs`, and `gov proposer`)
likewise reject `--height`. The proposer query is derived from the current
transaction index rather than a height-addressable module query, so accepting a
historical height would return the current proposer under a historical-looking
invocation. `block` and
`block-results` accept height positionally or through `--height`, but reject an
invocation that supplies both. An explicit `--chain-id` must agree with the
selected context even for a local derivation. File-oriented queries such as
`wasm code`, whose primary result is written to a named file, reject structured
stdout formats they cannot represent.

The three direct CometBFT block queries use the active context's resolved RPC
client and the invocation context. `blocks` requires one positional CometBFT
event expression, accepts only `asc` or `desc` when `--order_by` is set, and
preserves the node's reported total alongside the requested page and limit.
`block` accepts an explicitly typed positive decimal height or hexadecimal
hash; with no identifier it resolves the latest committed positive height.
`block-results` likewise resolves the latest committed positive height when no
height is supplied. A nil status, non-positive latest height, nil search or
results response, or search entry without a block body is a boundary error and
must not be rendered as a successful empty result. Block and block-search
protobuf responses honor JSON/YAML output semantics, block-results preserves
the same structured semantics, and all three propagate output-writer failures.

`wasm build-address` interprets its salt positional as hexadecimal by default.
`--ascii`, `--hex`, and `--b64` select one mutually exclusive input encoding.
The selected representation is decoded exactly once into salt bytes before
address derivation. Pretty output is the raw bech32 address; JSON and YAML are
structured string scalars.
At verbosity level one or higher, every query pre-run writes its selected RPC
endpoint and chain ID to stderr before network work begins. Dependency-owned
query trees and direct CometBFT query leaves follow the same diagnostic path.
Purely local derivations identify themselves as local and report the selected
chain ID instead of claiming that they contacted an endpoint.

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

The requested limit is a hard upper bound on returned records. Client adapters
must trim any upstream pagination lookahead item, including IBC client-state
lists, while preserving the response pagination metadata needed to request the
next page. If a dependency's filtered-pagination callback appends records while
the SDK marks them as excluded by `--offset` or `--page`, the adapter must also
remove that skipped prefix before applying the limit. This includes
`query ibc channel connections`.

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

State keywords are only recognized as the sole/first component of the filter argument; they do not combine with identity paths inside a single argument. Since 2026-07 the identity+state combination is expressed with the optional **second positional argument** (`akt query deployment akash1abc/12345 active`); two state keywords (a bare-keyword first argument plus a second argument) are an error, and the `--state` flag is **disabled pending feedback** (positional only, 2026-07). On a fully-specified identity the second positional state **verifies** the fetched record rather than filtering (see §3.8.3). Each resource has its own state vocabulary, derived from the on-chain state enums:

| Resource       | State keywords                        |
| -------------- | ------------------------------------- |
| `deployment`   | `active`, `closed`                    |
| `market order` | `open`, `active`, `closed`            |
| `market bid`   | `open`, `active`, `lost`, `closed`    |
| `market lease` | `active`, `insufficient_funds`, `closed` |

#### 3.8.3 Get-vs-List Heuristic

- If enough components are specified to **uniquely identify** a single resource, the command returns a single-item **detail** response.
- Otherwise, the command returns a **filtered list** response.
- The optional positional **[state]** argument follows the same split: with a partial identity it filters the list; with a complete identity it **verifies** the fetched record — when the record is in a different state the command fails with an error naming both states (e.g. `deployment akash1x/12345 is closed, not active`) instead of silently printing the record. Dropping the state argument prints the record regardless of state.

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

Every owner-scoped list, including `query cert`, must resolve an omitted
leading address from the active context before sending a request. If the
context has no default account, the command refuses locally and explains that
an owner address or `default-account` is required. An empty owner is never sent
to the chain query service because it means network-wide scope.

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
akt query deployment 12345 active              # Get + state verification: errors if 12345 is not active (§3.8.3)
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
akt query market lease 12345 active            # Partial identity + state: filters the lease list (state verifies only on full-identity gets, §3.8.3)
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
akt query provider                             # List every provider on the network
akt query provider akash1prov...               # Get one provider (the `get` subcommand remains an alias)
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

An option the host cannot provide is not offered as if it could be. When the
platform has no system credential store (§1.5) the `os` row is annotated as
unavailable, is not selectable, and is not the default cursor position — the
first selectable option is. The non-TTY fallback follows the same rule: it
resolves to `os` only where `os` is actually available, and to `file`
otherwise. Offering `os` on a host that will silently store keys elsewhere is
the bug this rule exists to prevent.

**Multi-select prompt**: The user toggles items on/off and confirms the batch. Space toggles; Enter confirms. A "Select all" row is the first item. Used for network selection during bootstrap.

```
Select networks  ↑↓ move  space toggle  enter confirm

  > [x]  # Select all

    [x]  mainnet             akashnet-2        [3 rpc, 2 api, 1 grpc]
    [x]  testnet             testnet-02        [1 rpc, 1 api, 1 grpc]
    [x]  sandbox             sandbox-2         [1 rpc, 1 api, 1 grpc]

  q quit
```

**Active-context prompt**: A single-select over the contexts just created, used at the end of network selection during bootstrap (§2.0 step d). The cursor starts on a test network, never on mainnet, and each row states in plain language what transacting on that network costs.

```
Select the active context  ↑↓ move  enter confirm

  > [x]  sandbox     sandbox-2    test network - tokens have no value
    [ ]  testnet     testnet-02   test network - tokens have no value
    [ ]  mainnet     akashnet-2   live network - transactions spend real AKT
```

**Value input prompt**: Free-form text or numeric input with an optional default. Used for deposit amounts, gas overrides, or custom names. Input is validated before acceptance.

```
Initial deposit amount [auto]: _
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

`ProviderAttributes` is populated from the provider's current on-chain
registration, and `ProviderAudited` is true when at least one current audit
record exists for that provider. The deploy workflow enriches the bids it
observes before persisting them; full reconciliation performs the same lookup
for every unique bidding provider. A Console-only workflow may use the Console
provider detail response when no chain query client is available. Metadata
lookup is best-effort and must never turn an otherwise usable bid into a failed
deployment; when a refresh cannot complete, reconciliation preserves metadata
already stored for that bid.

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
- Every code path that opens a context's store — `akt store *` and the
  workflow persistence of §6.6 — opens it through one helper that resolves the
  path from the context root (§1.1) and calls `Migrate()`. A store opened
  without migrating would be read and written at whatever schema it was last
  left at, which is exactly the drift the versioning exists to prevent.

`RecordVersion` has a different scope: it is the monotonic revision of one
deployment, lease, or bid record. A newly created local record starts at 1;
each write over the same key advances it. Importing a record with a higher
revision preserves that higher value, while importing an equal or older
revision advances from the existing local value. This makes merge/import and
future remote synchronization orderable without changing the database schema.
Version advancement happens inside the same bbolt write transaction as the
record update.

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
    record_version: 4
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

The three version values are independent:

- top-level `version` is the export-envelope format;
- top-level `schema_version` is the bbolt layout and migration level;
- each `record_version` is that individual record's monotonic update revision.

An export reads the schema version, optional sync state, deployments, leases,
and bids from one bbolt read transaction so concurrent writers cannot produce a
mixed-time envelope. Unlike list and statistics views, which may continue past
one unreadable row, a backup export MUST fail and identify any corrupt record;
it MUST NOT silently omit data and emit an apparently valid backup.

File export is atomic with respect to an existing destination: encoding occurs
in a sibling temporary file, and replacement happens only after a successful
flush and close. A failed export leaves the prior destination unchanged.

Imports are all-or-nothing. The complete envelope and every record are strictly
decoded and validated before mutation. Unknown JSON/YAML fields and trailing
documents are errors. Record identities are required; deployment state is one
of `active` or `closed`, lease state is one of `active`, `closed`, or
`insufficient_funds`, and bid state is one of `open`, `matched`, `closed`, or
`lost`. Record timestamps and block heights cannot be negative. A missing
`sync_state` is valid and represents a
store that has not completed reconciliation. A merge writes every accepted
record in one bbolt transaction. A replace clears the data buckets and writes
every accepted record in one bbolt transaction. Any malformed or unsupported
envelope, nil or invalid deployment, lease, or bid record, or write failure
aborts that transaction and leaves the prior store state unchanged. The
implementation MUST NOT clear a store before it knows the replacement can be
accepted.

---

## 5. Action Log Specification

The action log is an append-only log unique to each context. It records every user action performed within the context, providing an audit trail and enabling troubleshooting.

Transaction status changes do not rewrite historical bytes. A terminal lookup
appends a revision carrying the same non-empty `tx_hash` and original timestamp.
Readers collapse transaction revisions by hash, retaining the latest appended
revision, before reversing and applying the requested limit. The submission
time and one-row-per-transaction view are therefore preserved without making
the log mutable.

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
| `context`  | A context or keyring management operation  | Operation (switch, edit, `keys.add`, etc.), old/new values            |
| `console`  | A state-changing Console API operation     | Operation (create-deployment, close-deployment, etc.), dseq, result   |
| `error`    | A failed operation                         | Original action type, error message, context                          |

### 5.3 Action Entry Format

Each log entry is a single JSON line (JSONL format) for easy parsing:

```jsonl
{"ts":"2026-03-23T10:15:32Z","type":"tx","action":"deployment.MsgCreateDeployment","dseq":12345,"tx_hash":"ABC123...","height":18234567,"gas_used":200000,"code":0}
{"ts":"2026-03-23T10:15:45Z","type":"tx","action":"market.MsgCreateLease","dseq":12345,"provider":"akash1prov1...","tx_hash":"DEF456...","height":18234568,"gas_used":150000,"code":0}
{"ts":"2026-03-23T10:15:50Z","type":"provider","action":"send-manifest","dseq":12345,"provider":"akash1prov1...","status":"success"}
{"ts":"2026-03-23T10:16:00Z","type":"workflow","action":"deploy","workflow_id":"9f2c1ab34d55e017","step":0,"step_name":"create-deployment","status":"success"}
{"ts":"2026-03-23T10:16:40Z","type":"workflow","action":"deploy","workflow_id":"9f2c1ab34d55e017","step":4,"step_name":"send-manifest","status":"failed","error":"provider gateway unreachable"}
{"ts":"2026-03-23T10:20:01Z","type":"query","action":"deployment.deployments","params":{"dseq":12345},"duration_ms":120}
{"ts":"2026-03-23T10:22:00Z","type":"context","action":"keys.add","params":{"address":"akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx","name":"bob","type":"local"},"status":"success"}
{"ts":"2026-03-23T10:25:00Z","type":"tx","action":"deployment.MsgCloseDeployment","dseq":12345,"tx_hash":"GHI789...","height":18234600,"gas_used":100000,"code":0}
{"ts":"2026-03-23T10:30:00Z","type":"error","action":"tx.bank.send","error":"insufficient funds","account":"alice"}
```

`step` is written on every entry, including entries that are not workflow steps
(§5.4); the lines above elide it where it carries no meaning.

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
    Step       int             `json:"step"`                  // 0-based index within the run; always emitted
    StepName   string          `json:"step_name,omitempty"`

    // Error fields (type=error, or on any failed action)
    Error      string          `json:"error,omitempty"`
    Status     string          `json:"status,omitempty"`   // success, pending, failed, timeout
}
```

`Height` and `GasUsed` are only set when the broadcast actually reported them. Under the default `--broadcast-mode sync` a transaction result carries neither, and recording zero would be indistinguishable from a real reading (see [§10.11.1](#broadcast-confirmation-state)).

`step` is the one field written unconditionally. With `omitempty` the first step
of every workflow run (index 0) disappeared from machine output, so the entry
that records where a run started was indistinguishable from an entry that has no
step at all; entries that are not workflow steps carry `"step":0` as a
consequence. Entries written before this rule simply have no `step` key and
decode to `0`.

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
    Type       ActionType // filter by action type (empty = all)
    Since      time.Time  // entries after this time
    Limit      int        // max entries to return (0 = no limit)
    DSeq       uint64     // filter by deployment sequence
    Account    string     // filter by account
    WorkflowID string     // filter by workflow run id (isolates one run of a workflow)
}
```

### 5.6 Entry Writing Rules

The action log records entries for the following command categories:

| Command category | Logged | Entry type | When |
|---|---|---|---|
| `tx *` and MCP chain write tools | Always | `tx` | After broadcast (success or failure). On success: includes tx hash, plus height and gas used when the broadcast mode reports them. Status is `pending` when the transaction was accepted into the mempool but not yet included in a block (the default `sync` mode), and `success` once a height is known. `akt context log` reconciles displayed pending hashes and appends a terminal revision when the node reports inclusion. On failure: includes error message and result code. |
| `query *` | Never by default | `query` | Read-only queries are not state changes and are not recorded by default (see verbose row below). |
| Workflow commands (`deploy`, `update`, `close`) | Always | `workflow` | One entry per workflow step. Each entry includes the step name, step index, result, and workflow run ID, and `akt context log` renders the step and run in `SUMMARY` so the steps of a run stay distinguishable and a failed step is identifiable (§2.2). |
| `provider *` and MCP provider writes (state-changing: `send-manifest`, `migrate-hostnames`, `migrate-endpoints`, `lease-shell`) | Always | `provider` | After the provider gateway operation completes (success or failure). Read-only provider queries (`status`, `lease-status`, `lease-logs`, `lease-events`, `get-manifest`) and MCP query tools are not recorded. |
| `context *` | Always | `context` | After context management operation (switch, edit, create, delete). |
| `context keys *` (state-changing: `add`, `add --recover`, `delete`, `rename`, `import`) | Always | `context` | After the keyring mutation returns (success or failure), under a dotted `keys.*` action. Secrets — mnemonics, BIP39 and armor passphrases, key material — are never recorded (§2.2.2). Read-only `list`, `show`, `parse`, and `mnemonic` are not recorded. |
| `context keys export` | Always | `context` | The single exception to the read-only rule: exporting private key material is recorded as a security event, with the key name only and never the armor or passphrase (§2.2.2). |
| Console API state changes from CLI, workflow, or MCP (create/update/close deployment, create lease, deposit) | Always | `console` | After the Console API call completes (success or failure). Read-only Console queries, including MCP query tools, are not recorded. |
| All commands | On failure | `error` | When any command fails. Includes original action type and error message. |
| `query` (read-only, no side effects) | When `-v` is set (future) | `query` | Verbose-mode query logging for debugging is planned but not yet implemented. Internal queries (e.g. by the sync engine) are never logged. |

The action logger is opened in the root command's `PersistentPreRunE` and closed in `PersistentPostRunE`. Commands retrieve it via `cliutil.ActionLogFromContext(cmd.Context())`.
When a selected context requires an action log, failure to open that log is a
startup error. The command MUST NOT continue without its audit destination.

Real mutation E2E must inspect the raw append-only JSONL after invoking the
public log command once to reconcile pending transactions. Assertions decode and
collapse revisions independently; `akt context log` cannot serve as the sole
oracle for the writer and reader it is testing.

Console-minted JWT access to a provider does not change this classification:
`akt console shell` records the same single `provider/lease-shell` entry as the
keyring rail, while Console status, logs, and events remain read-only.

### 5.7 Log Rotation

- Log files are rotated when they exceed 10 MB.
- Rotated logs are named `actions.log.1`, `actions.log.2`, etc.
- A maximum of 5 rotated logs are kept (total ~50 MB per context).
- `akt context log` reads across all rotated files transparently.
- One serialized JSONL record, including its trailing newline, MUST NOT exceed
  10 MB. `Log` rejects an oversized record before rotation or append, so one
  caller cannot defeat the per-context storage bound. The reader accepts every
  record size the writer accepts, but applies the same ceiling to externally
  modified files so a corrupt or hostile unterminated row cannot cause
  unbounded allocation.
- A missing rotation generation is normal. Any other read error from an
  existing generation, or failure to flush the active log before reading, is
  returned. `Read` and `Export` never present a partial audit history as a
  complete one merely because one rotated file is damaged.

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

Closing a subscription channel terminates its consumer cleanly. The worker
MUST stop receiving, release the subscription, and return. It MUST NOT treat a
closed receive as a zero-value event or loop after closure. Context cancellation
and producer closure follow the same bounded shutdown path.

The event service MUST validate its client capabilities at construction. An
RPC client that does not implement CometBFT event subscriptions is rejected
with a descriptive error; construction MUST NOT panic on a failed type
assertion.

The standalone monitor treats event-service construction failure as a fatal
startup error and releases its already-open runtime resources. With the current
upstream WebSocket client, this covers a synchronous subscription enqueue
failure only. The server's later JSON-RPC acknowledgement or rejection is not
observable through that interface; the acknowledged and reconnect-aware
transport described in §2.6 remains required before this service can claim
end-to-end readiness.

Block-results responses are also untrusted. A successful RPC call with a nil
response is ignored without panic. Both transaction result events and
`finalize_block_events` are parsed and published; an unreadable individual
event is skipped, while a bus publication failure stops processing the rest of
that block.

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
4. Store all records. Chain state overwrites the on-chain fields of an existing
   record; local-only metadata the chain does not carry (`labels`, `notes`,
   `tags`, `sdl_path`, `sdl_hash`) is preserved from the record already in the
   store, so reconciling never discards what a workflow run recorded (§6.6).
5. Set `SyncState.LastBlockHeight` to the current chain height.

Deployment, lease, and bid reconciliation MUST request every pagination page.
The engine records each non-empty continuation key separately for each query
family and fails locally if a node repeats a key; it MUST NOT issue the repeated
request or loop until context cancellation. An empty key terminates traversal.
This is an untrusted network boundary even when the preceding page decoded
successfully.

On subsequent launches (existing `SyncState`):

1. Query the current chain height.
2. If `current_height - last_block_height > 1000`, perform a full reconciliation.
3. Otherwise, query transaction events in the missed block range and apply them.

`akt store sync` (§2.5) runs this same reconciliation on demand. It is the only
way a one-shot CLI session reconciles, because no CLI invocation lives long
enough to hold a subscription (§6.6).

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

A workflow run (`akt deploy`, `akt update`, `akt close`) persists its own
outcome to the local store when the run finishes.

Chain events alone cannot populate the store. A CLI invocation is one-shot: it
broadcasts its transactions and exits, before any subscription that would carry
the resulting events could deliver them, and there is no daemon left behind to
receive them afterwards. An event-driven-only model therefore leaves the store
empty after a fully successful `akt deploy` — the local deployment record, one
of the main things that distinguishes `akt` from the older tooling, never gets
written. The run itself is the only component that observes the outcome while
it is still running, so the run is what records it.

Persistence is **best-effort and never changes the command's outcome**. By the
time the store is written the deployment is already real on chain; a
bookkeeping failure must not be reported as a deployment failure. A store error
is reported as a `warning:` line on stderr and the exit code is unchanged.
Machine-readable output is unaffected: warnings go to stderr, so `--output
jsonl` stdout stays pure.

Records are written from the steps that actually succeeded, so a partially
failed run still records the deployment it created — the same DSEQ the recovery
advice (§2.3.6) tells the user to close.

| Workflow | Source step         | Store effect                                                                                                                                                          |
| -------- | ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `deploy` | `create-deployment` | `DeploymentRecord`, state `active`, `created_height` from the transaction height, `sdl_path` from the `sdl-file` parameter, `sdl_hash` = `sha256:<hex>` of the SDL file |
| `deploy` | `wait-for-bids`     | One `BidRecord` per bid observed, with its price; the winning provider's bid is `matched`, every other bid is `lost`                                                     |
| `deploy` | `create-lease`      | `LeaseRecord` for the won lease (full lease ID), state `active`, price from `select-bid`                                                                                 |
| `update` | `update-deployment` | Existing `DeploymentRecord` updated with the new `sdl_path`/`sdl_hash` and `updated_at`                                                                                  |
| `close`  | `close-deployment`  | `DeploymentRecord` set to `closed` with `closed_at`; that deployment's leases are set to `closed`                                                                        |

Fields a workflow run cannot observe — escrow balance, transferred amount,
provider gateway URI, service endpoints, provider audit status — are left at
their zero values rather than guessed. `akt store sync` (§2.5) fills them in
from chain state.

`deposit` is recorded only when the parameter names an explicit amount. `auto`
resolves to a chain-queried minimum that the workflow never reports back, so
the field is left empty instead of storing the literal word `auto`.

The owner is taken from the transaction result, falling back to the bid or
lease identity returned by the market, and then to the context's
`default-account` when that is an address rather than a keyring name. For an
update or close whose transport response contains only a DSEQ, the persistence
path searches existing deployment records and accepts the owner only when that
DSEQ has exactly one match. A direct `akt console deployment close` uses the
same transition. No match means there is no local record to update; multiple
matches refuse to guess and emit a best-effort store warning naming the
ambiguity. Records are keyed
`<owner>:<dseq>` (§4.4), so an empty or guessed owner would corrupt the key
space. A successful close marks the deployment and all locally recorded leases
for that deployment `closed` in one atomic store transaction.

### 6.7 Multi-Account Tracking

Reconciliation (§6.4, `akt store sync`) covers the accounts configured in the
context's `tracked-accounts` setting. By default, only the context's
`default-account` is tracked. Users can add further accounts to track
deployments across multiple wallets within a single context.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tracked-accounts` | []string | `["<default-account>"]` | List of account names or addresses to sync. Default: only the default account. Set to `"*"` to track all accounts in the context's keyring. |

Entries are resolved the way `--from` is: a bech32 address is used as-is, any
other value is looked up in the context's keyring and resolved to its address.
An entry that resolves to nothing is an error naming the entry, not a silently
skipped account.

When `tracked-accounts` is `["*"]`, every account present in the current
context's keyring is tracked, so a newly added key is picked up by the next
reconciliation.

The `tracked-accounts` field is context-specific (not shared via keyring or network). Each context can track a different subset of accounts.

`akt context show` reports the configured value.

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
- `akt --context <name> console login` and `console logout` write or remove the
  named context's credential even when `current-context` names another context.
  JSON/YAML acknowledgements contain only the username (for login), context
  name, and authentication state; they never contain the key.

API keys are created at [console.akash.network](https://console.akash.network) > Settings > API Keys.

### 7.2 Base URL

| Source              | Value                                  |
| ------------------- | -------------------------------------- |
| Default             | `https://console-api.akash.network`    |
| Context override    | `console-api-url` field in config.yaml |
| Flag override       | `--console-api-url`                    |

The Console client MUST NOT follow HTTP redirects. The configured base origin
is the only destination authorized to receive `x-api-key`, request bodies, or
managed-wallet mutations. Any 3xx response is an error returned to the caller
without issuing a request to the redirect target. The returned error MUST NOT
include the `Location` value. All transport errors, not only HTTP response
bodies, MUST redact every exact API-key occurrence before they can reach a
caller or action log; the action-log boundary repeats the redaction as defense
in depth.

### 7.3 Endpoints

All requests include `Content-Type: application/json` and `x-api-key` headers.

The endpoints below cover the deployment lifecycle used by command and workflow routing (§7.4-§7.5). The client's full surface — user info, wallets, usage, provider/GPU/template catalogs, API keys, and provider-scoped JWTs — is documented per command in §2.9 and contract-tested against the vendored OpenAPI spec (`internal/console/testdata/openapi.json`).

Provider-gateway calls made after Console or keyring authentication enforce a
separate transport boundary. One-shot responses have an overall timeout and a
bounded body; exceeding either limit fails the command. Streaming logs, events,
and shell sessions remain caller-cancellable and are not cut off by the
one-shot deadline. Provider-controlled error detail is length-bounded, stripped
of terminal control sequences, and redacted for bearer/API credentials before
it is returned or recorded in an action log.

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

Path parameter: `dseq` (deployment sequence ID). Returns deployment details,
including the base64 deployment version `hash`, with leases and escrow.

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

`POST /v1/leases` is not replayed after an error because it is not
idempotent. Instead, the client reads each referenced deployment back and
treats the operation as successful only when every exact requested
`dseq/gseq/oseq/provider` lease is present and active. If read-back does not
prove that state, the original POST error is returned.

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
| USD | `5usd`, `$5`, `5.50usd` | Error directs the user to `auto` (recommended) or an explicit coin in the network's deployment deposit denomination | Sent as USD in the Console API's `data.deposit` field (Console minimum: 0.50 USD) |
| Coin | explicit `<amount><denom>` | Attached to the deployment; the denomination must match the active network's deployment deposit parameter | Error: `console deposits are in USD; use e.g. 5usd` |
| Bare number | `5`, `5.50` | Error (coins require a denomination — the historical chain behavior, with cross-rail guidance) | Interpreted as USD, same as `5usd` |
| `auto` / empty | `auto` | Chain-minimum deployment deposit, queried on chain | Error: an explicit USD deposit is required |

**Manifest handling**: The Console API's `POST /v1/deployments` returns a `manifest` field in the response. The workflow engine stores this value and passes it to `POST /v1/leases` when creating leases, instead of calling the provider's `send-manifest` endpoint directly.

### 7.5 Workflow and Command Routing

The Console adapter maps abstract workflow actions, not raw `akt tx` commands:

| Workflow action / Console command | Console API Endpoint                 | Notes                                    |
| --------------------------------- | ------------------------------------ | ---------------------------------------- |
| deployment create                 | `POST /v1/deployments`               | USD deposit; single-submit reconciliation below |
| deployment update                 | `PUT /v1/deployments/{dseq}`         |                                          |
| deployment close                  | `DELETE /v1/deployments/{dseq}`      |                                          |
| bid list                          | `GET /v1/bids?dseq=`                 |                                          |
| lease create                      | `POST /v1/leases`                    | Requires manifest from deployment create |
| escrow deposit                    | `POST /v1/deposit-deployment`        | Amount in USD                            |
| deployment list/get               | `GET /v1/deployments`                | Paginated via `--skip`/`--limit`         |

The mappings are reached through `akt deploy/update/close` and the dedicated
`akt console` group. Every raw `akt tx` command requires `keyring` auth,
including deployment, market, and escrow commands. Chain queries continue to
use RPC directly when a Console context has a network. Running a raw tx with
`console-api` auth produces an error before any tx child initializes:

```
Error: raw chain transactions require keyring auth; the active context uses console-api.
Use `akt deploy`, `akt update`, `akt close`, or `akt console` for managed-wallet operations, or switch to a keyring context.
```

### 7.6 Error Handling

| HTTP Status | Handling                                                          |
| ----------- | ----------------------------------------------------------------- |
| 401         | Invalid or expired API key. Point the user at the key resolution chain (§7.1) and `akt console login`. |
| 402         | Insufficient funds in Console account.                            |
| 404         | Deployment not found (dseq does not exist or not owned by user).  |
| 429         | Rate limited. Retry with backoff only for idempotent methods (GET/HEAD/PUT/DELETE). A non-idempotent request may already have reached the service and is never replayed. |
| 5xx         | Console API server error. Retry with backoff (max 3 attempts) for idempotent methods (GET/HEAD/PUT/DELETE) only. A non-idempotent request is never replayed: it may have been processed despite the error (e.g. a gateway 502 after a completed write), and replaying it could duplicate a deployment or a USD deposit. |
| 3xx         | Redirect refused. No second request is issued and the API key is not forwarded. |

The Console may transiently reject an otherwise valid deployment PUT with
`422 manifest version validation failed`. Because PUT is idempotent, that exact
response is retried within the same three-attempt bound. After any failed PUT,
the client reads the deployment back and compares its base64 `hash` with the
deterministic SDL version; a match proves success despite the failed response.
All other 4xx responses remain terminal. A failed lease POST follows the
read-back rule in §7.3 and is never replayed.

Deployment creation adds a stronger ambiguity protocol. Before POSTing, the
client validates the SDL, derives its base64 version hash and rendered
manifest, and snapshots every existing deployment DSEQ through the paginated
list endpoint. A complete list traversal is limited to 100 pages and 10,000
deployment records. If `hasMore` remains true after the page limit or a response
would cross the record limit, the client returns a local pagination-limit error.
It does not retain the excess records or submit a create request from an
incomplete baseline. It then submits exactly one POST. Transport errors, 429,
5xx, and a success response without a usable DSEQ are ambiguous: the client
polls the list within a fixed bound and selects deployments absent from the
snapshot whose hash equals the SDL version. Exactly one match is reconciled as
success and supplies the locally rendered manifest; zero or multiple matches
return an "outcome unknown" error that tells the user to inspect
`akt console deployment list`. The request is never repeated. The action log
records reconciled success with the DSEQ, definitive 4xx failure as `failed`,
and an unresolved ambiguous outcome as `pending` with the SDL version hash.

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

### 7.8 Console Compatibility Matrix

Status of `akt` coverage for every Akash Console capability. "Covered" means the capability is reachable from `akt` end to end; deferred entries carry an explicit rationale. Reference points: the Console API surface (§7.1–§7.7), the `console-axi` reference CLI, and the Console web app.

#### Covered

| Console capability | akt equivalent | Notes |
|---|---|---|
| Authenticate with API key | `akt console login/logout/whoami`; per-context credential (`akt context edit --console-api-key`) | Resolution: flag > `AKT_CONSOLE_API_KEY` > per-context file (§7.1). Switching context switches Console identity. |
| Create deployment (managed wallet) | `akt console deployment create <sdl> [deposit-usd]`; `akt deploy` in a `console-api` context | Deposit in USD (minimum $0.50), unified deposit syntax per §7.4. The manifest is cached per context for the follow-up lease. |
| List / inspect deployments | `akt console deployment list/get` | Pagination via `--skip`/`--limit`. |
| Update / close deployment | `akt console deployment update/close`; `akt update`, `akt close` | Close is idempotent: an already-closed deployment reports success. |
| Escrow deposit | `akt console deployment deposit <dseq> [amount-usd]` | |
| Auto top-up settings | `akt console deployment settings <dseq> [true\|false]` | `/v2/deployment-settings`, PATCH with POST fallback. |
| View bids / create lease | `akt console bid list <dseq>`, `akt console lease create <dseq> [provider]` | The `akt deploy` workflow automates the bid wait and selection. |
| Live lease status | `akt console status <dseq>` (`--watch`) | Reads the provider gateway directly using a Console-minted scoped JWT. |
| Container logs / cluster events | `akt console logs <dseq> [service]`, `akt console events <dseq>` (`--follow`) | Same streaming paths as `akt provider lease-logs/lease-events`, authenticated by the Console JWT — no websocket relay needed. |
| Exec / interactive shell | `akt console shell <dseq> <service> [-- command]` | Exec is the same command with an explicit command argument. |
| Bid screening | `akt console screen <sdl-file>` | Public endpoint; resources are derived from the SDL. |
| Wallet balances & managed wallets | `akt console wallet balance/list` | Balances are µACT rendered as USD (1 ACT = 1 USD); wallet credits are dollar-scale. |
| Wallet auto-reload | `akt console wallet settings [true\|false]` | The only headless funding path, matching the reference CLI. |
| Cost estimate & usage history | `akt console wallet cost`, `akt console usage [from] [to]` | Usage totals the requested range; the lifetime figure is reported separately. |
| Provider marketplace browse | `akt console provider list/get/regions/auditors` | Public endpoints; no key required. |
| GPU availability & pricing | `akt console gpu` | Public. |
| Template catalog | `akt console template list/get/sdl` | `template sdl` writes raw SDL to stdout; redirect it to a file for `akt deploy` (which takes a path, not stdin). |
| SDL authoring | `akt sdl scaffolds/init/validate` (§2.11) | Local scaffolding, generation, and lint; no context, key, or RPC required. |
| API key management | `akt console apikey list/create/delete` | Create shows the secret exactly once. |
| Provider-scoped JWT minting | `akt console jwt create` | Also the mechanism behind the live lease operations above. |
| Action audit trail | Automatic: state-changing Console calls are recorded in the per-context action log (`akt context log`) | Beyond Console parity — Console has no client-side audit trail. |

#### Deferred or not portable (with rationale)

| Console capability | Status | Rationale |
|---|---|---|
| Adding funds by card / 3DS payment | Not portable | Stripe checkout with 3DS is inherently interactive and web-only; the reference CLI defers to auto-reload as well. Headless path: `akt console wallet settings true` plus per-deployment auto top-up. |
| Account signup / email verification / team management | Not portable | Web-only Console account flows with no public API endpoints. |
| Certificate management for managed wallets | Not applicable | The managed wallet signs server-side, so no client certificate exists (the reference CLI has no cert commands either). |
| Console's `provider-proxy` websocket relay | Superseded | akt reaches provider gateways directly with a Console-minted JWT, which covers the same operations without reimplementing the relay's in-band auth protocol. |

#### Behavioral differences vs the reference CLI (intentional)

- Output follows akt conventions (`-o json|yaml|pretty`) rather than TOON.
- Credentials and the manifest cache are per-context (§7.1) rather than a single global config file, so several Console accounts can be used side by side.
- The composite `deploy` command is the existing `akt deploy` workflow with auth-aware routing (§7.4), not a separate code path.
- Commands take their primary values positionally (§3.8); the reference CLI is flag-based.

---

## 8. TUI Specification

> **Not shipped (2026-07).** The TUI shell is disabled and incomplete — its
> resource views are scaffolded but not populated with live data. Everything
> below describes intended design, not delivered behavior, and is not
> documented for users. The code remains in `internal/tui/` and launches only
> under `AKT_EXPERIMENTAL_TUI=1`. `akt monitor` (§2.6) is a separate
> application and is fully functional.

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
<1> Overview  <2> Validators  <3> Governance  <4> Parameters  <j/k> Scroll  <r> Refresh
RPC: rpc.akt.dev:443/rpc  WS: connected
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

Row identity is the complete owner/DSEQ/GSeq/OSeq/provider tuple. DSEQ alone is
not unique and MUST NOT select a different lease belonging to the same
deployment. Provider addresses are always displayed in full in both list and
detail identifiers.

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
- `GET {rpc}/validators?per_page=100` -- Validator set (refreshed at consensus-height boundaries; errors are never cached)

**Displayed data**:
- Height (with thousand separators), Round, Step (human-readable: NewHeight, NewRound, Propose, Prevote, PrevoteWait, Precommit, PrecommitWait, Commit)
- Elapsed time since round start
- Proposer address and index
- Prevote/Precommit progress bars: `█`/`░` (40 chars wide), percentage (green if >= 66.7%, yellow otherwise), power fraction
- Validator vote grid: `●` (voted, green) / `○` (not voted, muted), line-wrapped to terminal width

Consensus height, round, and numeric step values are untrusted input and MUST
be nonnegative before they are used as indexes or applied to the model. A
negative value is a malformed-response error, never a panic. Prevote and
precommit state is scoped to the exact `(height, round)` pair. A height change
or a same-height round change clears the prior round's vote state before new
votes are applied, so the displayed voting power cannot include votes from an
earlier round. Delayed WebSocket events whose height is lower than the current
height, whose round is lower at the same height, or whose step is lower in the
same height and round are stale and MUST be ignored rather than rewinding the
model. A valid vote at a higher height, or a higher round at the same height,
MUST advance the model and clear the prior vote set. It is forward-progress
evidence even when a reconnect caused the corresponding `NewRoundStep` event
to be missed.

The consensus WebSocket feed subscribes to both `tm.event='Vote'` and
`tm.event='NewRoundStep'` on its initial connection and after every successful
transport reconnect. A transport reconnect alone MUST NOT be treated as a
restored feed because CometBFT subscriptions are connection-scoped. The server
MUST acknowledge both subscribe requests with their valid empty JSON-RPC result
before setup succeeds; an event that races ahead of those acknowledgements MUST
be buffered and applied after setup. If either subscription cannot be restored,
the WebSocket client stops and the returned snapshot channel closes; it MUST
NOT remain open as a silent feed. Transport and model reconnect delays MUST
select on the runtime context so cancellation stops the producer, closes its
snapshot and completion channels, and leaves no later dial scheduled.
Validator indices and power apply only to the height for which they were
fetched. Before applying the first vote or round-step event at a higher height,
the feed MUST refresh the complete paginated validator set and rebuild its
tracker. If refresh fails, it MUST NOT calculate percentages with the stale
set; the connection cycle ends and retries without caching that error. An
initial connection, subscription, or validator failure MUST update the visible
error and schedule a bounded reconnect rather than leaving the monitor dead
until process restart.

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
- Validator: Moniker (resolved from consensus pubkey via `/cosmos/staking/v1beta1/validators`; emoji-stripped; cached in the `monikers` bucket of `~/.config/akt/cache/monitor.db`)
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

**Provider cache** (stored in the `providers` bucket of
`~/.config/akt/cache/monitor.db`):
- Smart scheduling: online providers checked every 1m, recently offline every 5m, long-term offline every 6h
- Priority queue: unchecked first, then online, then recently offline, then long-term offline
- Max 10 concurrent provider checks
- Cache saved to disk every 30s
- Chain re-sync (full provider list refresh) every 10m

**Version distribution**: Versions sorted newest-first (semver-aware, handles `-rc` suffixes). Dot visualization with `●` for selected version, `○` for others.

**Resource display**: CPU in cores (millicores/1000), Memory in Mi/Gi/Ti (binary units), GPU with model name and count.

**Components:** bubbles/progress (scan progress bar), bubbles/table (provider list, node detail table), custom (version dot chart).

#### 8.3.11 Governance Monitor Views

**Governance proposals (key 3).** The proposal view shows the 20 most recent
proposals plus any proposal currently in deposit or voting period, de-duplicated
by ID and sorted newest first. Columns are ID, title, status, yes, no, abstain,
veto, and voting end. Tally columns show percentages of votes cast; a proposal
with no tally yet shows `-`. The monitor populates voting-period proposals from
the live tally query before calling the shared `RenderProposalList()` renderer.
`j/k` scroll and `r` refreshes immediately.

**Governance parameters (key 4).** Module-by-module governance parameter
browsing. The right pane renders pretty-formatted key-value output (the same
`Render*Params()` functions as CLI `--output pretty`) instead of raw JSON. This
follows the Pretty/TUI visual parity rule (§10.8).

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

**Refresh intervals**: proposals every 30 seconds; parameters every 5 minutes.

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
│  akt     0.003125    4               │  Mints:            Allowed            │
│  atom    6.85        3               │  Refunds:          Allowed            │
│                                      │  Collateral Ratio: 1.523              │
│  Price Health                        │  Thresholds:                          │
│    Healthy:     yes                  │      Warn:         1.1                │
│    Min Sources: yes                  │      Halt:         1.05               │
│                                      │                                       │
│                                      │  Ledger                               │
│                                      │  ROUTE      STATUS    BURNED  ...     │
│                                      │  uakt→uact  Pending   5 AKT   ...     │
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

**Color coding**: Oracle health: Healthy (green), Unhealthy (red). BME status: healthy (green), warning (yellow), halt CR (red), halt Oracle (red). Mints/Refunds: Allowed (green), Halted (red). Ledger record status: `Executed` (green), `Pending` (yellow), `Canceled (<reason>)` (red) — spelled out, never abbreviated (§10.10 BME).

**Amount formatting**: All micro-denominated values scaled using `FormatCoin()` — same rules as pretty output (§10.7). Prices and ratios are rounded at their semantic precision and then have trailing zeros stripped; a collateral ratio of `1.5` renders as `1.5`, never `1.500000000000000000`.

Ratios and prices round differently, because their significance sits in different digits:

- **Ratios** (collateral ratio, warn/halt thresholds) are rounded to 3 decimal places before stripping. A `LegacyDec` always stringifies to 18 places and stripping only helps a value that has trailing zeros, so an on-chain ratio such as `1.495209570451729242` would otherwise render at full width beside a threshold of `0.95`. It renders as `1.495`.
- **Prices** (oracle prices, the `@price` in ledger rows) are rounded to the oracle's published 8 decimal places before stripping. This retains an AKT price around `0.003125` while keeping derived 18-place `LegacyDec` values such as a TWAP comparable with the 8-place source observations.

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

**Note:** When the monitor view (4) is active, `1`/`2`/`3`/`4` switch sub-tabs within the Network dashboard instead of navigating to global views. All other number keys retain their global behavior.

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
│   │   │   ├── GovernanceView   (shared proposal renderer in a bubbles/viewport)
│   │   │   └── ParametersView   (bubbles/list for modules, bubbles/viewport for params)
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

Shell layout works in terminal display cells. ANSI escape sequences are never
indexed as visible runes when a label is overlaid on a progress bar, and
flexible table columns distribute leftover cells instead of silently shrinking
the requested width. The shell removes its own chrome before sizing child
views; the monitor adapter forwards that content height unchanged. Batched and
single log appends both register service scopes for the same filter menu.

Every stored balance, deposit, escrow value, bid/lease price, and validator
token count that carries a Cosmos denomination is rendered with the shared
pretty coin formatter. Real decimal and integer coin strings are parsed before
escrow ratios are calculated; incompatible denominations remain unknown rather
than producing a plausible but false percentage. Named dashboard deployments
retain their DSEQ so names cannot make otherwise distinct records ambiguous.

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

When `--output pretty` (the default), output is styled using **lipgloss**. At the
final write boundary, all ANSI styling (color, bold, underline, and related
sequences) is removed when stdout is not a TTY or when the `NO_COLOR`
environment variable is present, even when its value is empty.

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

All final command renderers, including workflow pretty and JSONL reports,
propagate writer errors and short writes. Producing only part of a requested
machine-readable stream is a command failure even when the underlying query,
transaction, or workflow completed successfully.

This contract applies at every public output boundary: registered and fallback
query formatters, every transaction result type and format, generic JSON/YAML,
and tables. Commands write through `cmd.OutOrStdout()` rather than bypassing
Cobra with `os.Stdout`. ANSI removal is a transparent decorator: it reports a
short write of stripped bytes as `io.ErrShortWrite` while returning the original
input length only after the stripped payload was accepted in full.

The transaction pretty renderer applies this contract to the complete public
`PrintTxResult` path. Summary rows, section headers, registered formatters,
recursive formatters, and highlighted-JSON fallbacks share one checked writer.
It returns destination errors and `io.ErrShortWrite`, while formatter errors
remain available to `errors.Is` callers.

Structured collection fields are always arrays. An empty collection is encoded
as `[]` in both JSON and YAML, never as `null`; changing output formats must not
change the semantic data model. This applies to store exports as well as live
query and Console results.

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
- **A header is aligned exactly like the column it labels**: right-aligned columns get right-aligned headers, left-aligned columns get left-aligned headers. A header is never centered independently of its data.
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

**Empty results** are stated, never implied. A query that matches nothing must
print a dim line naming what is missing -- `(no deployments)`, `(no bids)`,
`(no networks)` -- and must never print a bare column header with no rows: a
lone header reads as a rendering failure, not as "zero results". The rule holds
for every table in pretty output, including tables nested inside a larger
detail view.

This applies to the pretty/table path only. `--output json` and `--output yaml`
still emit an empty array (§10.1.1); an empty result must never turn structured
output into prose.

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

The all-balances request MUST leave denomination metadata resolution disabled.
JSON and YAML preserve each canonical chain coin exactly (for example,
`{"denom":"uakt","amount":"1000000"}`), while pretty output passes that raw
coin through `FormatCoin()` and renders `1 AKT`. Resolving `uakt` to `akt` in
the query transport is prohibited because it both changes machine semantics
and bypasses the shared micro-unit formatter.

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
| TITLE | `Proposal.Title` | Up to 40 characters, with `...` when truncated |
| STATUS | `Proposal.Status` | Color-coded |
| YES | `Proposal.FinalTallyResult.YesCount` | Percentage of votes cast, or `-` when no tally exists |
| NO | `Proposal.FinalTallyResult.NoCount` | Percentage of votes cast, or `-` when no tally exists |
| ABSTAIN | `Proposal.FinalTallyResult.AbstainCount` | Percentage of votes cast, or `-` when no tally exists |
| VETO | `Proposal.FinalTallyResult.NoWithVetoCount` | Percentage of votes cast, or `-` when no tally exists |
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

**Aggregated price** (`akt query oracle aggregated-price <denom>`): Key-value sections for the aggregated price and its health, as mocked in §8.3.12.

The oracle module keys prices by the **base** denom (`akt`), while every other
surface in `akt` — gas, balances, fees, escrow — takes the micro denom
(`uakt`). The positional denom is therefore normalized before the query so the
spelling users already know keeps working: `akt`, `AKT`, `mAKT` and `uakt` all
resolve to `akt`, and the ACT family (`act`, `mact`, `uact`) resolves to `act`.
Any other denom passes through unchanged. Help text and examples use the base
denom (`akt query oracle aggregated-price akt`).

A failed oracle query is wrapped in a `CLIError` (§11.1) that names the denom
that was tried and suggests `akt q oracle prices` to list the denoms the oracle
carries; the raw gRPC/ABCI status must never reach the user verbatim.

#### BME

**Status/Vault**: Key-value sections with amounts. The collateral ratio and the
warn/halt thresholds are `LegacyDec` ratios, not coins: they render through
the ratio formatter (`1.5`, not `1.500000000000000000`), matching the oracle
panel beside them on the same dashboard.

**Ledger**: Table with ROUTE, ID, STATUS, BURNED, MINTED, SPREAD, REMINT ACCRUED, REMINT ISSUED.

- **ROUTE** — `<denom>→<to-denom>` from the record ID. It already carries the destination denom, so no other column repeats it.
- **ID** — `<source>/<height>/<sequence>`.
- **STATUS** — the full `LedgerRecordStatus` word, never an abbreviation: `Executed` (green), `Pending` (yellow), `Canceled (<reason>)` (red), where `<reason>` is the `BMCancelReason` name with underscores replaced by spaces (e.g. `Canceled (insufficient funds)`). This mirrors the `MintStatus` labels used by BME status ("Healthy", "Warning", "Halt CR", "Halt Oracle"). When the record oneof is absent the column falls back to the `status` field the entry carries alongside it, dimmed, rather than a blank cell.
- **BURNED / MINTED / REMINT ACCRUED / REMINT ISSUED** — one rendering per concept, applied uniformly across every row type:
  - an amount that carries an oracle price (`CoinPrice`) renders as `<FormatCoin(coin)> @<price>`, rounded to 8 decimal places and then stripped by `FormatPriceDec()`;
  - an amount with no price attached renders as `FormatCoin()`;
  - an amount that does not exist **yet** — the mint of a pending record, whose size depends on the oracle price at settlement — renders as a dim `pending`;
  - an amount that does not exist renders as `-`.
- **SPREAD** — always `FormatCoin()`. A zero spread renders as `0 AKT`, not `-`; only a record with no spread denom at all renders as `-`.

Every `Coin`/`LegacyDec` read out of a ledger record passes through
`IntOrZero()` / `DecOrZero()` first: proto3 omits zero values on the wire, so a
field the node never set unmarshals with a nil inner value and any method call
on it panics.

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
| `json` | Structured transaction result document (see [§10.11.6](#10116-machine-readable-transaction-result)) |
| `yaml` | The same document rendered as YAML via `FprintJSONSemantics()` |

`json`/`yaml` MUST NOT be produced by handing the raw `sdk.TxResponse` to `cctx.PrintProto()`. That printer marshals with `EmitDefaults: true`, so a transaction that has only been accepted into the mempool emits `"height":"0"`, `"gas_used":"0"` and `"gas_wanted":"0"` — values an automated consumer cannot distinguish from a real block height of zero or a real zero gas reading.

##### Broadcast Confirmation State

`akt` defaults to `--broadcast-mode sync`. In `sync` and `async` mode the node returns a CheckTx result only: `Code`, `Codespace`, `Data`, `RawLog`, `Logs` and `TxHash` are populated, while `Height`, `GasUsed`, `GasWanted` and `Timestamp` are zero and the transaction body (`TxResponse.Tx`) is `nil`. The transaction is in the mempool and has not been included in a block.

Every transaction renderer therefore classifies a result into exactly one of three states:

| State | Condition | Meaning |
|---|---|---|
| `failed` | `Code != 0` | CheckTx (or DeliverTx, in `block` mode) rejected the transaction |
| `pending` | `Code == 0 && Height == 0` | Accepted into the mempool; not yet included in a block |
| `confirmed` | `Code == 0 && Height > 0` | Included in a block at `Height` |

A `pending` result MUST NOT be presented as a completed transaction: block height, gas and fee are unknown, not zero and not blank.

#### 10.11.2 Two-Section Layout

Every transaction result in pretty mode renders two sections separated by a blank line.

**Section 1: Transaction Summary**

Common to all transactions. Uses the same `Section()` + `KV()` formatting as query detail views.

| Field | Source | Format |
|---|---|---|
| Hash | `TxResponse.TxHash` | Full hex string, bold |
| Signer | `message.sender` event attribute | Full address; omitted when the events carry no sender (always the case while `pending`) |
| Height | `TxResponse.Height` | `Height > 0`: comma-grouped via `FormatHeight()`. `pending`: dim `not yet confirmed`. `failed` before inclusion: dim `not included in a block` |
| Gas Used | `TxResponse.GasUsed` / `TxResponse.GasWanted` | `Height > 0`: `used / wanted`, both comma-grouped. `pending`: dim `not yet confirmed`. `failed` before inclusion: dim `not reported` |
| Fee | `tx.AuthInfo.Fee.Amount` | `FormatCoins()` (micro-denom scaling). Always emitted; when the body is unavailable, dim `not reported by <mode> broadcast (query the tx to see it)` |
| Status | `TxResponse.Code`, `TxResponse.Height` | Green `success` when `confirmed`; yellow `pending` + dim `(accepted into the mempool, not yet in a block)` when `pending`; red `failed: <RawLog>` otherwise |
| Confirm With | `TxResponse.TxHash` | `pending` only: `akt query tx <hash>` |
| Tip | — | `pending` only: `broadcast with --broadcast-mode block to wait for inclusion` |

The `Fee` row is never silently dropped. Prior behavior omitted the whole row when the tx body could not be decoded, which is exactly the `pending` case, so the most common transaction output was missing its fee line with no explanation.

Gas amounts are formatted by a gas-specific formatter, not by `FormatHeight()`. `FormatHeight()` renders `-` for any value `<= 0` because block height zero does not exist; gas zero does, and reusing the height formatter for gas is a comma-grouping coincidence rather than a semantic fit.

The section header is `Transaction`, rendered with `Section()` (bold + underline).

**Section 2: Message Detail**

Each message in the transaction body (`tx.Body.Messages`) is rendered using a registered `TxPrettyFormatter` for that message's protobuf type URL.

**Single-message transactions** (1 message): The message detail section renders directly with a descriptive section header (e.g., "Send", "Deployment Created", "Delegate").

**Multi-message transactions** (2+ messages): Each message renders as a numbered sub-section with prefix: "Message N: \<title\>" (e.g., "Message 1: Withdraw Rewards", "Message 2: Delegate").
Aggregate response events that omit `msg_index` are a valid fallback only for a
single-message transaction. Multi-message formatting uses indexed ABCI logs or
aggregate events carrying the exact index; otherwise receipt-only fields are
omitted rather than copied from one message to another.

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

`PrintTxResult(cmd, cctx, txResponse)` is called by all `tx` commands after broadcast. The broadcaster returns `interface{}`, so dispatch is by concrete type first and by `--output` second:

1. Read the `--output` flag.
2. `[]byte` (a `--generate-only` transaction body) → print the encoded transaction verbatim (JSON) or re-encoded (YAML).
3. `*tx.SimulateResponse` (a `--dry-run` simulation) → render the simulation result ([§10.11.7](#10117-simulation-results-dry-run)).
4. `*sdk.TxResponse`:
   a. `json`/`yaml` → build the structured document ([§10.11.6](#10116-machine-readable-transaction-result)) and emit it with `FprintJSONSemantics()`.
   b. `pretty` → render Section 1 (common summary) from `TxResponse` fields; decode `TxResponse.Tx` to extract messages; for each message look up a `TxPrettyFormatter` by proto type; render Section 2 for each message (registered formatter or JSON fallback). A `pending` result carries no body, so only Section 1 renders.
5. Anything else → `PrintProto()` (proto messages) or `PrintObjectLegacy()` (amino values).

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

`MsgUpdateDeployment` carries only the deployment ID and the new SDL hash, so a
successful transaction changes the chain record and nothing else. The formatter
therefore closes with a dim note stating that providers keep serving the
previous manifest until it is delivered, naming the two commands that deliver
it (`akt update`, `akt provider send-manifest`) with the DSEQ filled in. This
is the single place a user of the bare `tx` command learns that the running
workload is untouched.

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
The receipt assigns events to the outer `MsgExec`, not to its individual inner
messages. Inner formatters MUST receive no receipt events, so event-derived
fields such as a proposal ID or contract address are omitted instead of
reusing the first matching attribute for multiple inner messages. Fields
carried by each inner message continue to render normally.

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

A BME conversion is **not** executed by the transaction that carries it. The
chain writes a pending ledger record and settles it in a later block, once the
oracle price is available and the circuit breaker allows
(`LEDGER_RECORD_STATUS_PENDING` → `EXECUTED`, or `CANCELED` with funds
returned). The `Status: success` line of §10.11.2 therefore reports acceptance
of the *request*, not completion of the conversion. All three BME message
formatters must say so explicitly — without it the burned balance appears to
vanish with no explanation of where it went.

All three render the same block, produced by
`pretty.RenderBMEPendingConversion(owner, coinsToBurn, denomToMint)`:

| Field | Source | Format |
|---|---|---|
| Sender | Message `Owner` | Full address |
| Burned | Message `CoinsToBurn` | `FormatCoin()` |
| Minted Denom | Destination denom (per message, below) | Raw denom |
| Conversion | Constant | `pending` (yellow) + "settles in a later block" |
| Minted Amount | — | "not known yet" — the amount is set by the oracle price at settlement, which is exactly why a pending ledger row (§10.10 BME) can only show a destination denom |

followed by an indented note stating that the burned amount has already left the
balance while the minted amount has not arrived, and the concrete follow-up
command:

```
akt q bme ledger --owner <sender> --status ledger_record_status_pending
```

Destination denom per message:

| Message | Title | Destination denom |
|---|---|---|
| `MsgBurnMint` | "Burn Mint" | `DenomToMint` (carried by the message) |
| `MsgMintACT` | "Mint ACT" | `uact` — the message mints ACT by definition |
| `MsgBurnACT` | "Burn ACT" | `uakt` — the message burns ACT to mint/remint AKT |

The same deferred-settlement wording, and the same follow-up command, appear in
the `Long`/`Example` help of `akt tx bme burn-mint`, `mint-act` and `burn-act`.

#### 10.11.6 Machine-Readable Transaction Result

`--output json` and `--output yaml` emit one structured document per transaction, built from the `sdk.TxResponse` rather than marshalled from it directly. Fields whose value is unknown because the transaction has not been confirmed are **absent**, never zero.

| Field | Type | Presence |
|---|---|---|
| `txhash` | string | Always |
| `status` | string | Always — `confirmed`, `pending` or `failed` ([§10.11.1](#broadcast-confirmation-state)) |
| `confirmed` | bool | Always — `true` iff `Height > 0` |
| `code` | uint32 | Always (`0` on success) |
| `codespace` | string | When non-empty |
| `height` | int64 | **Only when `Height > 0`** |
| `gas_used` | int64 | **Only when `GasUsed > 0`** |
| `gas_wanted` | int64 | **Only when `GasWanted > 0`** |
| `timestamp` | string | When non-empty |
| `data` | string | When non-empty |
| `info` | string | When non-empty |
| `raw_log` | string | When non-empty |
| `logs` | array | When non-empty |
| `events` | array | When non-empty |
| `tx` | object | When the response carries the transaction body |

`logs`, `events` and `tx` are carried through verbatim from the codec's proto-JSON encoding of the response so that interface fields (`Any`) resolve to their concrete types. They are dropped when the client context has no codec.

A pending transaction therefore serializes as:

```json
{
  "txhash": "9F3C...",
  "status": "pending",
  "confirmed": false,
  "code": 0
}
```

The same rule applies to every other machine-readable surface that reports a transaction height:

- **Action log** ([§5.4](#54-log-entry-format)): `height` and `gas_used` are only recorded when the broadcast reported them, and `status` is `pending` for an accepted-but-unconfirmed transaction.
- **Workflow JSONL** ([§2.3.8](#238-execution-modes)): the `height` key is omitted from a tx object when the step's transaction has not been confirmed.

`PrintTxResults()` (used when one logical action is intentionally split across several transactions) emits a JSON array of these documents, applying the same rules to each element.

#### 10.11.7 Simulation Results (`--dry-run`)

`--dry-run` sets `client.Context.Simulate`, and the chain client returns a `*tx.SimulateResponse` — **not** an `sdk.TxResponse`. The response's `GasInfo.GasWanted` echoes back the gas limit that was sent with the simulated transaction, which for a dry run is the internal placeholder `0` that the CLI substitutes for the `--gas` flag. `gas_wanted` MUST NOT be surfaced from a simulation in any output mode.

The adjusted gas estimate is computed by the chain client but discarded on the simulate-only path, so the renderer recomputes it from the same inputs:

```
gas_estimate  = uint64(--gas-adjustment * GasInfo.GasUsed)
estimated_fee = --fees, when set
              = ceil(--gas-prices[i] * gas_estimate) per denom, otherwise
```

This matches `tx.Factory.BuildUnsignedTx`, so the estimate the dry run reports is the fee the real broadcast would attach.

**Pretty output** renders a single `Simulation` section:

| Field | Source | Format |
|---|---|---|
| Gas Used | `GasInfo.GasUsed` | Comma-grouped |
| Gas Adjustment | `--gas-adjustment` | Decimal, trailing zeros stripped |
| Gas Estimate | Computed above | Comma-grouped |
| Estimated Fee | Computed above | `FormatCoins()` (micro-denom scaling); `unknown (no --fees or --gas-prices set)` when neither flag provides a basis |
| Status | — | Yellow `simulated` + dim `(not broadcast)` |

**`json`/`yaml` output** emits:

| Field | Type | Presence |
|---|---|---|
| `simulated` | bool | Always `true` |
| `gas_used` | uint64 | Always |
| `gas_adjustment` | float64 | Always |
| `gas_estimate` | uint64 | Always |
| `estimated_fee` | array of coins | When a fee basis is available |

### 10.12 Table and Key-Value Layout

**Table rendering engine.** `internal/output/pretty` is the canonical table
engine for pretty output. It measures every cell with `lipgloss.Width`, so ANSI
styling never breaks alignment, and it supports per-column alignment
(`ColDef.Align`). Renderers write tables through exactly three entry points:

| Helper | Use |
|---|---|
| `WriteTableCols(w, cols, rows)` | Per-column alignment. |
| `WriteTableOrEmpty(w, headers, rows, emptyMsg)` | As `WriteTable`, with a caller-named empty message. |
| `WriteTableColsOrEmpty(w, cols, rows, emptyMsg)` | As `WriteTableCols`, with a caller-named empty message. |

The `*OrEmpty` forms are the default choice for list renderers, because every
list can come back empty. The plain column-defined form carries the same
guarantee as a backstop: given no rows it prints the generic `(no results)`
instead of a bare header, so no code path can regress to a header-only table.

`internal/output.PrintTable` is a second, legacy engine built on
`text/tabwriter`. It is not ANSI-aware and has no per-column alignment, and it
survives only for the remaining `output.PrintData` callers that render plain,
unstyled config rows (`akt context list`, `akt context history`,
`akt context keys list`). It carries the same empty-result guard. New pretty
output must use the `pretty` engine; `akt sdl list` keeps its own local
`tabwriter` for a two-column scaffold listing. Converging the two engines is
tracked separately -- until then, `output.PrintTable` is frozen: no new callers.

**Key-value column invariant.** `KV` indents keys by 2 and pads the key column
to 20; `SubKV` indents by 6 and pads to 16. Both therefore start their value at
column 23, so a nested block lines up with the block that contains it. The
padding helpers never truncate: a key longer than its column pushes its own
value out of line rather than being cut. A view whose keys do not fit the
defaults widens **both** columns together through `KVWidth` and `SubKVWidth`,
keeping `subWidth == width - 4`. `akt context show` does this: its capability
labels ("Chain transactions") do not fit the default column, so it renders at
`width 23 / subWidth 19` and every value in the view still lands in one column.

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

The process-level stderr boundary removes transport and implementation wrappers
that add no user-facing information. In particular, nested
`rpc error: code = Unknown desc =` prefixes, Cosmos SDK
`failed to execute message; message index: N:` wrappers, and trailing Go source
locations are omitted when a more specific chain explanation remains. For
example, an insufficient-funds response ends with the spendable balance and
required amount rather than an SDK file path. This is display-only: the
original error remains intact for exit-code classification, action logs, and
debugging.

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

## 12. Testing and Verification

This section defines the coverage denominator, collection method, behavioral
inventory, integration environments, and quality gates for the repository.
These requirements apply to every phase. A test that only executes a line does
not satisfy a behavioral requirement unless it also verifies the promised
result.

### 12.1 Coverage denominators

Coverage reports MUST publish all of the following denominators. They are not
interchangeable.

| Denominator | Required contents | Gate |
|---|---|---|
| `repository` | Every non-test Go statement compiled from this module, including `cmd/akt`, clean-copied code under `internal/cli/chain`, the disabled TUI shell, and repository-owned executable tooling such as `tools/coverage` | Reported on every pull request and default-branch build |
| `active` | Every statement reachable through a default release build and a shipped command or service. This includes chain commands, contexts, keyrings, Console, provider, workflows, transports, store, sync, events, output, MCP, and `akt monitor` | Primary per-package and aggregate release gate |
| `experimental-tui` | The disabled application shell and dependencies used only by that shell. The shipped standalone monitor runner is isolated under `internal/monitor/runtime` and remains active | Separate report and no-regression ratchet until the shell ships |

The active denominator MUST NOT exclude a package because its implementation
was copied from another repository. If users reach it through the shipped
`akt` binary, it is active code. Likewise, the experimental TUI profile MUST
NOT be merged into the active percentage while the gate in §2.0 remains in
place. `akt monitor` is shipped and belongs to `active`; its runtime entry point
and cache/event lifecycle MUST NOT live in a package classified as
experimental.
Taxonomy validation MUST inspect the release-tag dependency closure of
`internal/monitor/runtime` and reject any repository-owned dependency that is
not classified as active. This prevents an indirect import from moving shipped
monitor behavior behind the experimental-TUI denominator.
It MUST also enforce the reverse release-closure invariant: every package
classified `active` or `experimental-tui` is linked into the release binary.
An unlinked package belongs in `support` with a reviewed exception, or in
`tooling`; labeling it active merely to inflate or hide a denominator is an
error.
The only permitted direct import from an active package into the experimental
denominator is `internal/cli` to the gated `internal/tui` shell. Validation
MUST reject every other active-to-experimental import edge; the root command's
environment gate remains the single reviewed bridge while the trial exists.

The shipped monitor runtime owns one bbolt database. Provider, metadata, and
moniker buckets MUST be initialized in one transaction before the UI model is
constructed; a failed initialization closes the database and returns the
boundary error. Runtime tests MUST exercise that failure boundary and a real
CometBFT WebSocket subscribe/unsubscribe lifecycle, including service and
client cleanup, rather than replacing event transport with a success-only mock.
The standalone monitor model MUST set Bubble Tea's alternate-screen view flag
for normal and quitting views. The embedded monitor MUST leave that flag unset
so the containing shell remains the sole owner of terminal-screen policy.
The standalone runtime MUST own one context used by every network command the
model starts: consensus HTTP and WebSocket requests, chain synchronization,
provider status and detail probes, validator metadata, governance, oracle, and
BME refreshes. It MUST cancel that context before closing its remaining event,
bus, and database resources. Its task drain MUST include asynchronous consensus
producer work after subscription setup returns. The experimental embedded shell
MUST provide the same monitor context and drain it before closing shared event,
bus, or database resources. A completed Bubble Tea program MUST leave no monitor
network goroutine or socket behind. Consensus setup is successful only after
both event-subscription acknowledgements are received, and the initial signing
history MUST pair a sampled commit with the validator set at that exact height.
Program-completion tests MUST supply an owned readable input pipe rather than
inherit the CI runner's standard input. Production continues to use the
terminal already validated by the CLI; this test boundary prevents Linux CI's
non-interactive `/dev/null` from becoming part of the behavior under test.

Package membership MUST live in a checked-in machine-readable configuration.
Validation discovers directories containing non-test Go source independently
of the default build tags, so a package made entirely of build-constrained files
cannot bypass classification. Repository-owned executable tooling has its own
no-regression unit-coverage ratchet; test-only helper packages remain excluded.
The configuration may exclude generated code, test-only helpers compiled into
support binaries, and code that cannot be reached by a release build. Each
exclusion requires the exception record defined in §12.4. Broad path or package
exclusions without a concrete reason are prohibited.
Validation MUST inspect dependency directories as well as import paths. A
release dependency whose source directory is inside the repository root but is
not classified in the main-module taxonomy—such as a nested module selected by
a local `replace`—MUST be rejected. Repository-owned shipped code cannot escape
the denominator by changing module boundaries.

A source file compiled into a release package MUST NOT contain APIs used only
by tests. Such helpers MUST move to a `_test.go` file or a classified
test-support package. Removing accidentally release-linked test support is a
correction to the shipped source set, not a coverage exclusion; the support
package still requires the §12.4 exception record.

### 12.2 Coverage collection and profiles

Unit coverage MUST be cross-package coverage. The unit command supplies
`-coverpkg` for the applicable denominator so execution through a neighboring
package counts against the package that owns the statements. All profiles use a
merge-compatible coverage mode. Direct Go commands continue to run with
`GOWORK=off` as required by AGENTS.md.

Coverage profile identities are canonical repository-relative source paths plus
their exact statement ranges. Parsers MUST normalize import-path and absolute
path spellings before aggregation and reject a duplicate canonical range;
aliases cannot count the same statement twice.
Go may emit synthetic zero-statement coverage blocks for empty control-flow
edges. Those records carry no denominator weight, may have a zero-width source
range and a nonzero execution counter. They MUST be discarded from statement
filtering, profile publication, reports, baselines, and ratchets. Changed-source
analysis keeps them in a separate exact edge-evidence index. When the AST maps
changed executable syntax to that exact synthetic range, the edge passes only
if its own counter is positive. A synthetic counter MUST NOT lend coverage to a
neighboring token or enclosing positive-statement block, and it never changes a
coverage numerator or denominator. All positive-statement blocks still require
a strictly increasing source range.

End-to-end tests execute the public binary, not command handlers linked into the
test process. The test build MUST therefore use Go's binary coverage support:

```text
go build -cover -covermode=atomic -coverpkg=<denominator> -o <test-binary> ./cmd/akt
GOCOVERDIR=<unique-shard-directory> <test-command>
```

The instrumented binary MUST use the release binary's semantic build tags and
build metadata. Instrumentation may omit only linker or stripping options that
are incompatible with coverage; it MUST NOT select a different source or
dependency path. E2E asserts the reported build tags.

Every raw counter shard is bound to the tracked environment recipes (`.env`,
`.envrc`), the collection CI workflow, all Make recipes, and a canonical
snapshot of the effective Go build environment. Release-tag selection is
recorded directly and validated against the GoReleaser recipe. The snapshot
includes at least
GOOS, GOARCH, architecture feature levels, CGO_ENABLED and CGO compiler flags,
GOEXPERIMENT, GOFLAGS, GOTOOLCHAIN, and GOWORK. A change capable of selecting a
different source set or coverage metadata therefore requires recollection even
when no Go source file changed.

Every parallel shard receives its own `GOCOVERDIR`. CI converts counter data
with `go tool covdata`, then merges profiles only after every shard finishes.
A non-instrumented binary MUST NOT be used for a coverage-labelled E2E job.
When shards are transferred between CI jobs from a dot-directory, artifact
upload MUST explicitly include hidden paths and fail if no files match. The
artifact scope MUST be limited to the named coverage shard or report directory;
the surrounding cache and any credentials MUST NOT be uploaded. The merge job
MUST fail when any required shard is absent.
Immediately before publication, collection-side validation MUST reject every
shard entry except `source-manifest.tsv`, `binary-identity.tsv` for an E2E
shard, `covmeta.*`, and `covcounters.*`, and MUST validate every accepted form.
Unit shards MUST reject a binary identity, while every E2E shard MUST require
one. Validation MUST also ask the pinned Go toolchain to read the covdata so a
correctly named corrupt payload cannot be published.
Every accepted `covmeta.<hash>` MUST have at least one matching
`covcounters.<hash>.*` file. Metadata without counters is an incomplete shard,
not evidence of a zero-count execution.
Report-time validation repeats the check after download. This prevents an accidental credential or unrelated runner file from
entering even an informational live artifact.

Every raw shard MUST also contain a deterministic source manifest generated at
collection time. The manifest records the digest of every Go or native source
and test file, every default-, release-, or test-selected `go:embed` input,
every file beneath a Go package's `testdata` directory, module dependency lock
file, package-taxonomy input, and build recipe that can change the compiled
statement map or test behavior. Reporting-only inputs such as checked-in
baselines, reviewed patch exceptions, and Codecov display policy are excluded:
changing one of them does not change the counters and MUST NOT force an
otherwise identical expensive collection to run again. Embedded and test
fixture inputs are included even when their change does not alter coverage
metadata line ranges. Before converting or merging
counters, the report job regenerates the manifest from its checkout and
requires every blocking shard to match it exactly. A missing, malformed, or
mismatched manifest is a collection failure. This binds counters to the tested
worktree/commit and prevents a locally stale instrumented binary or cross-job
artifact from being evaluated against newer execution inputs.
Manifest serialization uses a tab-delimited CSV writer and parser, so a
tracked filename containing a tab, quote, comma, or newline remains one
unambiguous record. Every manifest input and every changed Go source inspected
from the worktree MUST be a regular file; a symlink is rejected rather than
hashing or parsing content outside the reviewed repository path.

Every instrumented E2E build MUST create `binary-identity.tsv` only after its
pre-build and post-build source manifests match. The identity MUST contain the
SHA-256 of the exact executable and the SHA-256 of that source manifest. Shard
preparation MUST verify both digests before copying the identity and manifest
into `GOCOVERDIR`; collection-side validation MUST verify them again against
the executable after the suite completes. A missing, malformed, stale, or
replaced binary therefore fails before artifact publication. Report-time
validation MUST require the identity and revalidate its manifest digest even
though the executable itself is intentionally not transferred.

CI publishes at least these profiles:

| Profile | Contents | Merge behavior |
|---|---|---|
| `unit` | Cross-package unit and component tests | Independent profile |
| `e2e` | Offline and hermetic integration lanes that execute an instrumented binary | Independent profile, with lane flags retained |
| `live` | Tests against shared or externally hosted services | Independent, never allowed to hide a hermetic gap |
| `union` | Statement union of `unit` and blocking `e2e` profiles for the same commit | Primary active coverage status |
| `union-live` | `union` plus successful live profiles | Informational because an external outage may prevent collection |

The live Console shard is collected only for same-repository pull requests
whose base branch is `main`. It executes the read contracts and the bounded
managed-wallet lifecycle against the configured non-production sandbox. The
job MUST use the protected `console-sandbox` environment. That environment
requires approval from `@akash-network/core`, allows a core author to approve
their own job, prohibits administrator bypass, and limits deployments to
pull-request merge refs through the exact `refs/pull/*/merge` policy.
Self-approval is an explicit trust grant to core
accounts for their own proposed code against the capped sandbox. GitHub MUST
require workflow approval from every
external contributor. That repository setting distinguishes organization
members from external contributors; it cannot express membership in one team.

Fork pull requests MUST remain secretless and skip the complete Console job
chain. A core member must mirror the proposed commit to a branch in this
repository before the sandbox lane can run. The workflow MUST NOT use
`pull_request_target` to execute pull-request code. GitHub does not release
secrets to a fork's `pull_request` run, and executing that code through
`pull_request_target` would cross the trust boundary before review.

The Console test job is a required input to `required-ci` for every eligible
same-repository pull request. A separate mutation opt-in and non-production API
URL are still required before the lifecycle can write. Every sandbox run MUST
share one serialized concurrency group. Every pull-request workflow run MUST
disable automatic cancellation, including a currently ineligible run, because
retargeting can otherwise let a later event cancel an earlier lifecycle while
it owns resources. Push and manual workflows MAY replace an older run. There is
no scheduled or manual sandbox mutation run. The independent `live` profile MUST publish
active-package and statement totals before it is merged into `union-live`.
Both reports are retained as pull-request artifacts, and successful report
generation is a required input to `required-ci`. They remain separate from the
hermetic `union`: a live counter cannot cover a missing hermetic assertion, and
a Console failure cannot be reclassified as a coverage regression.

The repository and experimental-TUI denominators receive equivalent reports.
Only the active `union` is the primary aggregate merge gate. A merged profile
MUST use statement-range identity and execution counts, not an arithmetic
average of percentages.

### 12.3 Ratchets and targets

The repository stores a baseline for every package in each gated denominator.
CI compares the current profile with that baseline according to these rules:

1. An existing package's statement coverage MUST NOT decrease. A prior package
   with zero executable statements is treated as fully covered: adding
   statements may transition it only to 100%, never from `0/0` to an uncovered
   denominator.
2. A coverage improvement MUST update the checked-in baseline to the current
   package and aggregate statement counts in the same change. A stale lower
   ratio, or an equivalent ratio recorded with different counts, fails because
   it would permit a later test-only regression back to the old floor. A
   baseline is never lowered. If an exact reviewed exception from
   §12.4 is unavoidable, added coverage in the same package and aggregate MUST
   compensate for the unreachable statement.
3. A new active package requires coverage for every changed executable line and
   an initial package statement coverage of at least 80%. A new critical
   package starts at 95% or higher.
4. Added or changed executable lines in active code require 100% coverage. This
   patch gate applies even while the repository is below its final aggregate
target. CI applies it to pull requests and to default-branch pushes, using
the event's actual before/base revision rather than assuming `HEAD^`.
The local changed-line command defaults its head to the current worktree and
includes tracked, staged, unstaged, and untracked Go files. CI supplies an
immutable tested commit SHA instead.
Any caller-supplied base name is resolved once to a full commit object before
source discovery, baseline comparison, or diff construction. A branch or tag
that moves after resolution cannot select different historical inputs later in
the same gate.
Diff parsing recognizes a `+++ b/...` file header only before the file's first
hunk. Added source text that happens to begin with that sequence cannot switch
the identity used for a later hunk or attribute changed lines to another
package.
Go coverage may attach a multiline statement's counter to only one portion of
that statement. If a changed executable token has no directly intersecting
positive-statement coverage block, the patch gate MUST first test for an exact
synthetic edge-evidence match as defined above. Only when no exact edge match
exists may it resolve the smallest enclosing statement and use that statement's
first positive-statement instrumented region. The token is covered only when
the selected exact edge or statement region executed. Executable syntax whose
enclosing region has neither kind of evidence MUST fail closed; declaration
initializers cannot disappear from the gate merely because the Go cover tool
omitted a direct line mapping.
5. Once the active union crosses an aggregate milestone, CI records that value
   as the new floor. The staged milestones are 80%, 90%, and 95%.
6. Reaching 95% active owned shipped statement coverage completes the initial
   target. The floor is never lowered, and package ratchets continue toward
   100%.

Ratio comparisons MUST use exact overflow-safe arithmetic across the complete
accepted counter range. A syntactically valid baseline with large statement
counts MUST NOT wrap an intermediate cross-product and bypass either baseline
weakening detection or the current-profile ratchet. The advertised local
`test-coverage-check` entry point runs both the merged per-package/aggregate
ratchets and the 100% changed-active-line gate; CI may expose those two checks
as separate named steps, but neither is optional.
Ratchet coverage MUST NOT depend on randomized map iteration, scheduler timing,
or an implementation-selected comparator call direction. Tests for ordered
logic exercise every semantically distinct direction explicitly so repeated
runs converge on the same covered-statement set.

The active and experimental-TUI ratchets each have one canonical checked-in
baseline path. The enforcement command MUST reject an alternate path; otherwise
renaming or redirecting a baseline could be mistaken for a first-time bootstrap.
Moving a package between denominators removes it from the old canonical
baseline and adds it to the new one. That move MUST preserve at least the
package ratio stored in its former denominator, satisfy the destination entry
floor, and preserve both denominator aggregate ratchets; a classification
change cannot reset package history.
When a repository introduces its first baseline file, that one-time bootstrap
may record the existing legacy packages below their eventual entry floors
because no older ratio exists. It MUST NOT exempt a package introduced by the
same change: package presence is derived from every non-test Go source file in
the comparison revision, independently of whether that revision had coverage
metadata. Packages absent there still start at 80%, or 95% when critical. The
100% changed-active-line gate applies to the complete bootstrap change as an
independent backstop.
Legacy-package discovery MUST apply the main module's directory rules at the
comparison revision, including nested `go.mod` boundaries and ignored
dot/underscore, `testdata`, and `vendor` directories. Removing such a boundary
cannot misclassify newly admitted source as legacy.
A changed active non-test Go file that contains executable statements but has no
entry in the release-equivalent coverage profile MUST fail the patch gate. This
includes files omitted by build constraints, so adding a build tag cannot hide
code from the coverage contract.
Every GoReleaser build used to validate the release-equivalent profile MUST
identify `./cmd/akt` as its main package as well as carrying the canonical build
tags. A correctly tagged auxiliary binary is not evidence for the shipped
`akt` denominator.

Critical packages are packages that control money, credentials, persistent
state, state-changing commands, action logs, workflow execution, or wire
transports. The checked-in denominator configuration marks them explicitly.
The long-term target for deterministic critical logic is 100%; only reviewed
exceptions may account for the remainder.

Coverage percentage is not permission to weaken an assertion. Reviewers MUST
reject a test whose expected result was copied from the implementation without
an independent contract or whose only assertion is successful exit when the
command promises state or structured data.

### 12.4 Coverage exceptions

The only way to exempt an exact line from the changed-line gate is a checked-in
exception with all of these fields. It does not lower or bypass a package or
aggregate ratchet:

| Field | Requirement |
|---|---|
| Scope | Exact package, file, and line or generated-code rule |
| Reason | Why meaningful execution is impossible, not merely inconvenient |
| Owner | Person or team responsible for removing or renewing the exception |
| Evidence | Build constraint, upstream limitation, or unreachable-state proof |
| Review deadline | Date no more than 180 days in the future |

An exception MUST be narrow. It MUST NOT exclude a whole package when only one
branch is unreachable. Expired exceptions fail CI. Equivalent mutants use the
same record format. An exception never removes a command or MCP tool from the
behavioral manifest.

### 12.5 Generated behavioral manifest

Tests generate the runnable CLI inventory from the fully assembled Cobra tree
and the MCP inventory from the actual tool registry. Static lists maintained by
hand are not authoritative. The generated inventory is matched against
checked-in scenario metadata with one record per runnable action.

Each record contains:

- exact command path or MCP tool name;
- capability annotation and backing rail;
- read-only or state-changing classification;
- required actors, fixtures, and prerequisite state;
- positive, boundary, and negative scenario identifiers;
- the lane or lanes that execute those scenarios;
- supported output modes and semantic assertions;
- expected action-log entry type, or an explicit `none` for read-only work;
- an unsupported classification and rationale when the selected Akash app
  cannot execute the upstream action.

CI fails when an assembled command or tool has no record, a record names an
action that no longer exists, a mutating action lacks a state assertion, or a
shipped action is covered only by `--help`. Dynamically surfaced built-in
workflows are part of this inventory. User-authored workflows outside the
repository are tested through the workflow schema and engine contracts rather
than enumerated by name.

### 12.6 Semantic end-to-end assertions

Every successful state-changing scenario follows this sequence:

1. Capture authoritative pre-state through a client independent of the `akt`
   command under test.
2. Execute the action through the instrumented public binary.
3. Decode and validate the selected output format, stderr contract, and exit
   code.
4. Poll the authoritative system until the promised post-state or a bounded
   timeout.
5. Assert exact resource identity, full addresses, amounts, status, and other
   command-specific fields.
6. Assert one appropriate action-log outcome. Pending transaction revisions
   may later collapse to one terminal view as specified in §5.
7. Repeat the action or issue the documented invalid transition when the
   command promises idempotency, ambiguity reconciliation, or a specific
   rejection.

Queries run against both empty and populated state where those states have
different meaning. List tests verify pagination boundaries and exact record
identity. JSON and YAML are decoded and compared semantically; pretty output is
checked for the contract in §10, including full addresses and readable amount
formatting. A non-empty stdout check alone is insufficient.

Read-only commands and tools MUST NOT append an action-log entry. Every mutation
MUST append the entry required by §5.6, on success and on the specified failure
paths. Tests inspect logs for secret values as well as required fields.

### 12.7 Test lanes and cadence

No single network fixture can represent every Akash capability. The behavioral
manifest assigns actions to these lanes:

A lane MAY self-skip only when it was not selected. Once its opt-in environment
variable is set, missing Docker or another declared dependency, an unreachable
daemon, fixture bootstrap failure, and missing coverage counters MUST fail the
job. An explicitly selected blocking lane MUST NOT convert infrastructure
failure into a successful skip.

An externally supplied chain endpoint is read-only by default, even when a
funded mnemonic is also supplied. Transaction scenarios against
`AKT_E2E_RPC` require a separate mutation opt-in, an explicit expected chain
ID, and a comma-separated mutation-chain allowlist containing that exact ID.
The harness MUST query the remote node's status before importing the mnemonic
and reject an expected/observed chain-ID mismatch. Known production chain IDs
MUST remain prohibited. A mutation that creates a resource snapshots the
owner's exact pre-state and may update or close only the unique new resource
identity observed after its transaction; selecting the newest or last list
entry is prohibited. Resource lists used for that identity proof MUST page to
exhaustion and reject repeated page keys or duplicate identities. Scenarios
that leave an additional persistent resource, including the client certificate
required by deployment creation, run only against a harness-owned throwaway
chain until they implement exact resource identity and cleanup for every
created object. Docker-created throwaway fixtures satisfy this boundary
without external opt-in because the harness owns their complete lifecycle.
Read-only balance checks against an external endpoint MUST accept its actual
denomination set and require at least one positive, canonically encoded coin
for a supplied funded mnemonic. They MUST NOT require the Docker fixture's
two-denomination genesis layout or construct its container-only native
observer. The Docker lane continues to require exact `akt`/native agreement for
both fixture denominations.

In the Docker fresh-chain lane, authoritative transaction receipts and
post-state MUST come from the pinned node image's native `akash` CLI or another
typed client that does not call `akt` or reuse its internal query/client code.
The harness decodes that observer's JSON itself. An `akt` query remains useful
coverage for the public read surface, but it MUST NOT be the only evidence that
an `akt` mutation worked. The command-reported transaction hash, independent
receipt, exact post-state identity, and action-log transaction hash MUST agree.
A scenario that transfers funds to a newly generated recipient is restricted
to a harness-owned throwaway chain unless it preserves a recoverable recipient
key, refunds the transfer, independently verifies final balances, and enforces
a declared maximum spend.

| Lane | Required environment and proof | Cadence |
|---|---|---|
| `offline` | No network. Config, context, keyring, SDL, construction/signing, output, capability failures, completion, and executable documentation examples | Every pull request |
| `fresh-chain` | Deterministic genesis, funded role accounts, and enough validators to exercise the scenario. Executes every reachable query and transaction with state-based assertions | Every pull request, sharded; extended validator and authority cases nightly |
| `provider` | Fresh chain, registered provider, real provider services, and local Kubernetes. Covers bids, leases, manifests, gateway auth, logs, events, shell, migrations, and a nonce-emitting workload | Nightly and release; also on pull requests that change provider, market, deployment, workflow, transport, or Console gateway code |
| `dual-chain` | Two deterministic chains and a pinned relayer. Covers IBC clients, connections, channels, transfers, acknowledgements, and timeout paths | Nightly and release; path-triggered pull requests |
| `testnetify` | Pinned source snapshot converted to a local chain with recorded source height, app hash, image digest, and snapshot checksum. Covers populated pagination, historical state, migrations, monitor, and upgrade behavior | Scheduled and release; path-triggered pull requests |
| `console` | Real Console API implementation, database, signer, indexer/proxy, chain, and provider. Covers managed-wallet reads and a complete deployment lifecycle | Protected sandbox lane on every same-repository pull request to `main`; fork changes must be mirrored by core |
| `monitor` | Real CometBFT HTTP and WebSocket endpoints plus a pseudo-terminal. Covers dashboards, input, rendering parity, cancellation, cache, and reconnect | Smoke tests every pull request; full duration and reconnect tests nightly and release |
| `fault` | Controlled node, Console, provider, database, signer, and network interruption. Covers retry, ambiguous outcomes, SIGINT, restart, concurrent store access, and recovery | Nightly and release; path-triggered pull requests |

**Current implementation status (2026-08-12):** pull requests currently block
on cross-package unit, offline instrumented-binary E2E, and a deterministic
single-validator fresh-chain lane. Harness-owned mutation scenarios bind
native-node receipts and post-state to the public command and action log. The
generated Cobra inventory exercises
structural reachability and help for every visible command; it is not yet the
checked-in semantic scenario metadata required by §12.5. MCP has registry,
schema, handler, protocol, and action-log scenarios; handler coverage includes
valid requests whose chain query or transaction dependency fails and requires
the operation-specific error result. Real Console read and managed-wallet
mutation suites require explicit external credentials and block eligible
same-repository pull requests behind the protected sandbox environment; they
never replace or contribute counters to the hermetic active union. The
provider/Kubernetes,
dual-chain, `testnetify`, full monitor/PTY, fault, multi-validator/multi-actor,
and mutation-testing lanes in this table remain normative implementation work.
Only two native fuzz targets exist and CI does not yet run a bounded fuzz
campaign, so the fuzz boundary set in §12.9 is also incomplete.
Hermetic unit tests attach the first-run wizard to a real pseudo-terminal and
drive its network, keyring, active-context, and Console decisions through the
same raw/canonical terminal transitions used by a human. Pure renderer tests
remain useful, but cannot replace this interaction test. This does not satisfy
the separate full monitor/PTY environment in the matrix above.
Release automation therefore does not yet satisfy the full matrix below, and
the completion criteria in §12.11 have not been met.
For each fresh-chain query in the current matrix, a separate request-counting
HTTP peer replaces the real RPC endpoint and must observe an actual transport
request followed by command failure. The help form must make no request. A
merely nonzero exit with an arbitrary diagnostic is not proof that a nominal
leaf command reached its transport.

The live Console read suite validates command-specific response contracts, not
only JSON syntax: required object fields are non-null; documented collection
fields have the correct JSON kind; non-empty public catalogs validate every
item's stable identity fields; and tenant-owned collections may remain empty
when that is a legitimate account state. Deployment-dependent reads select an
exact active deployment and lease rather than treating an empty tenant as a
command failure. A live provider-status response for that active lease requires
the documented services, forwarded-ports, and IP collections; the services
collection is non-empty and every dynamic service entry carries its name and
numeric availability and total counts.
Each record returned by the bounded provider-log check MUST name either the
requested SDL service or one of its hyphen-delimited runtime pods and MUST
contain a string-valued `message` field. Empty and whitespace-only messages are
valid individual container-log lines. The aggregate read MUST contain at least
one substantive message, so an all-blank stream cannot satisfy the live
contract.
The provider-region catalog requires a non-empty top-level region list, unique
non-empty region keys, non-empty descriptions, and an array-valued provider
membership field. Individual regions MAY have no current providers because
Console deliberately emits an empty membership array for catalog regions that
no provider advertises. Region keys and descriptions MUST exactly match the
independently fetched `location-region` values in Console's provider-attributes
schema. Every membership MUST be a canonical `akash` bech32 address and MUST
not repeat within one region. A mutation-capable sandbox MUST nevertheless
expose at least one provider membership across the complete catalog. The
template-list command unwraps Console's `data` envelope and returns a non-empty
top-level category array. Every category has a unique non-empty title; its
template array MAY be empty, but every present template has a non-empty ID
that is unique within that category and the aggregate catalog MUST contain at
least one template. The same template ID MAY appear in different categories
because categories classify rather than own templates.

Fresh-chain fixtures define role accounts for at least validator/funder, tenant,
provider, auditor, fee granter, grantee, vesting recipient, and governance or
module authority where the app allows it. A deliberate authorization failure is
a valid scenario for an action that cannot succeed under ordinary authority,
but the manifest must say so and assert that state did not change.

A single validator is enough for the fast smoke shard, but it is not the full
fresh-chain contract. Redelegation, jailing, governance, IBC, upgrade, and
provider lifecycle scenarios receive the extra actors or services they need.

Release automation requires a successful full lane matrix for the release
commit. An external live-service outage may be waived only with a recorded
release approval; hermetic failures cannot be waived as external outages.

Until every lane in this section is implemented, the release workflow MUST at
minimum rerun the complete current hermetic blocking matrix against the exact
tagged commit and MUST reject a version tag whose commit is not reachable from
`main`. Publishing MUST depend on that exact-commit job rather than assuming a
prior branch run tested the same object. The GoReleaser container reference
MUST include an immutable OCI digest; a version tag alone is insufficient.
The release coverage comparison MUST start at the commit peeled from the
nearest earlier reachable semantic-version tag. It MUST NOT default to
`HEAD^`, because one release may contain several commits. Resolution MUST fail
closed when the prior tag is absent or invalid, does not peel to a commit, or
is not an ancestor of the tested commit. Manual check, snapshot, and dry-run
modes use the same previous-release comparison rule.

For a tag event, the event tag MUST peel to the tested commit and MUST be the
only tag selecting that commit. The publishing job MUST repeat this check
immediately before invoking GoReleaser, after its complete tag checkout, and
must not infer the release identity with an ambiguous `git describe` result.
All tag publications share one non-cancelling concurrency group across tag
refs so two stable releases cannot race while updating the Homebrew formula.
Manual modes run separately with `contents: read`, do not receive publishing
secrets, and every checkout uses `persist-credentials: false`. Only the tag
publishing job receives `contents: write`.

Before GoReleaser starts, publication MUST require `GITHUB_TOKEN`. A stable
semantic version, meaning one with no prerelease component, MUST also require
`GORELEASER_ACCESS_TOKEN`; discovering that omission after GitHub assets have
started uploading is a partial-release failure. Release containers default to
`GOWORK=off`. A non-off override is valid only when it names the existing
repository-root `go.work` mounted at the corresponding container path.
`goreleaser check` may tolerate the intentional `brews` deprecation only when
GoReleaser returns its dedicated deprecated-property status and the
deprecation plus one-file issue summary are its only error diagnostics. A
normal validation failure, including one that also mentions a deprecated
property, remains fatal.
This interim gate reduces release risk but does not satisfy §12.11.

### 12.8 Console API truth, credentials, budgets, and cleanup

The real Console implementation is the authority for successful response
schemas and managed-wallet behavior. The vendored OpenAPI document detects
schema drift but does not replace executing real handlers. HTTP fakes remain
required for deterministic boundary failures such as malformed bodies,
timeouts, connection resets, 429 and 5xx responses, and ambiguous
non-idempotent outcomes. A canned success body written only inside this
repository is not sufficient proof of Console compatibility.

A successful response cannot be treated as state merely because its HTTP
status is 2xx. When a client operation expects a response value, the body MUST
be present and decodable. Data-enveloped operations additionally require a
present, non-null `data` value. Empty successful bodies remain valid only for
operations whose contract expects no result, such as a 204 deletion.
Endpoint invariants are validated before mutation success is logged. A lease
response must contain every exact requested active lease; deployment settings
must carry the requested dseq and boolean; wallet settings must carry the
requested boolean. A malformed success from the non-idempotent lease POST is
an ambiguous outcome and uses exact read-back reconciliation without replaying
the POST. A malformed settings acknowledgement fails rather than allowing a
missing boolean to masquerade as the requested `false` value.
Deployment create additionally requires a usable DSEQ and a present
managed-wallet receipt with code zero and a nonblank transaction hash; an
unusable 2xx response enters the existing no-replay version-hash
reconciliation. Deployment update validates the returned DSEQ and deterministic
SDL version hash before success, otherwise using its exact read-back. Close
requires a present `success: true` acknowledgement. Deposit snapshots
authoritative total escrow value, defined per denomination as current `funds`
plus cumulative `transferred`, submits its POST exactly once, and validates the
returned deployment identity. A returned total whose exact delta equals the
requested deposit proves success. A lost, malformed, or stale acknowledgement
MUST fall back to independent GET observations for a context-cancellable
30-second propagation window. It MUST NOT replay the POST and records `pending`
when the exact outcome remains unproved. Including `transferred` keeps the
proof exact while active-lease settlement consumes current funds. Console
encodes both escrow collections with the chain's fixed-point decimal grammar,
so a whole micro amount MAY arrive as
`500000.000000000000000000` and a settled balance MAY contain a genuine
fractional micro amount. Current `funds` use the chain's signed `Balance`
contract because an overdrawn escrow may be negative; cumulative `transferred`
uses non-negative decimal coins. Production reconciliation MUST retain both
collections as exact rationals, while the independent live observer MUST retain
the current `funds` it uses for the pre-lease deposit proof. Both paths MUST
parse the chain grammar without floating-point conversion, preserve values down
to the 18th decimal place, accept signed `funds`, reject malformed values, and
compare the pre/post delta exactly. Production MUST additionally reject a
negative `transferred` amount. Neither path may require each endpoint value to
be a lexical integer or discard a fractional component merely because the
expected deposit delta is an integer. A created API key requires a
nonblank ID, the requested name, and its one-time secret; an ambiguous response
is pending and is never replayed. A minted provider JWT requires a nonblank
token before it may be used.

The Console sandbox lifecycle covers deployment create, bid observation, lease
creation, live status, logs, events, a deterministic non-interactive shell
command, deposit, settings, update, child API-key lifecycle, close, and final
cleanup. Each state is verified independently through the Console API and,
where applicable, chain RPC, provider gateway, and Kubernetes workload state.
The deployment-get, bid-list, status, and default log-read contracts MUST run
against the lifecycle-owned active lease before cleanup; they MUST NOT depend
on a pre-existing tenant deployment. A separately invocable read-only smoke
test MAY reuse an existing active lease and skip when none exists.
The protected CI read step runs only the state-independent read suite before
the lifecycle. It MUST NOT invoke that optional existing-lease smoke test: the
blocking lifecycle creates the deployment and lease, runs the shared
deployment-dependent contracts against that exact resource, then closes it and
verifies cleanup.
The independent Console observer is a bounded raw HTTP client maintained by the
test harness; it MUST NOT call the `akt` binary or reuse `internal/console`.
This keeps a command-layer or shared-client decoding defect from validating its
own mutation response.

Live credentials follow these rules:

- use an isolated sandbox tenant and least-privilege, short-lived API keys;
- fail closed to the exact staging-sandbox and production-namespace-sandbox
  Akash API hostnames, using HTTPS on the default port with no base path;
  sandbox-looking third-party hosts are prohibited, while loopback HTTP remains
  available only through an explicit hermetic-test option that protected read
  setup and mutation configuration never enable;
- before network access, match a recognized Akash sandbox hostname to the API
  key's non-secret environment segment: the staging sandbox host accepts
  `staging`, while the production-namespace sandbox host accepts `production`;
  mismatch and endpoint diagnostics MUST NOT print the credential or the raw
  secret-backed endpoint value;
- preserve and validate the raw key without trimming it, perform that check in
  both read setup and mutation lifecycle before any observer or network client
  exists, and create the temporary CLI context through the same bounded,
  redacted subprocess path used by all later live commands;
- make secrets available only after the protected `console-sandbox`
  environment approves an eligible same-repository pull request to `main`;
- store the API key, API URL, and mutation opt-in only as secrets on that
  environment; repository- and organization-scoped copies selected for this
  repository are prohibited because pull-request workflow code could remove
  the environment declaration;
- keep fork pull requests secretless and never execute proposed code through
  `pull_request_target`;
- inject secrets through the CI secret store and environment, never process
  arguments, config committed to the repository, stdout, logs, or artifacts;
- remove every `AKT_E2E_CONSOLE_*` harness variable from child CLI process
  environments. Only `AKT_CONSOLE_API_KEY` is forwarded and scanned, so a
  temporary child key cannot coexist with an inherited, unscanned parent key;
- never mutate production by default; production canaries are read-only unless
  a human authorizes one bounded run;
- serialize mutations that share an account and tag every resource with a
  unique run identifier where the API permits it;
- enforce maximum deployment count, attempted USD request total, duration, and
  concurrent leases before the first mutation. A request reservation is charged
  before its subprocess starts and remains charged after any ambiguous outcome;
- separately cap observed spend. Requested deployment deposits move credit into
  escrow and are not themselves proof of spend. The harness snapshots the
  authoritative Console total balance before the first write, reads it again
  after terminal cleanup, and fails when the decrease exceeds the spend limit.
  The configured spend ceiling MUST be at least the immutable escrow request
  total, and that request total is the maximum credit available after auto
  top-up is independently observed disabled. Before creating a lease, the
  harness intersects the CLI and raw-API bid sets, rejects non-`uact` prices,
  chooses the lowest numeric price, and requires its conservatively projected
  price-per-block cost through the complete remaining paid runtime to fit the
  same ceiling. The projection assumes at least one billable block per second;
  fixed escrow remains the hard loss bound if blocks arrive faster;
- disable auto top-up as soon as a deployment identifier is known unless the
  scenario explicitly tests it.

Subprocess and observer response capture is bounded. Failure diagnostics MUST
NOT print raw stdout, stderr, HTTP response bodies, action-log entries, API
keys, or SDL/manifest payloads. They may report the operation, exit or HTTP
status, full resource identifiers, error class, and captured byte counts. The
harness scans its complete temporary akt home for the injected credential
before teardown rather than checking only config and action-log files.
For a failed Console subprocess, the error class MAY include a recognized HTTP
status (for example, `console_http_401`) extracted from bounded stderr. It MUST
NOT include the response body or any other captured text. A fixed local error
marker MAY likewise map an unproved deposit to
`console_deposit_outcome_unknown`; this exposes the safe failure phase without
copying its potentially joined remote diagnostic.

The production Console client applies the same boundary independently of the
test harness. It reads no more than the configured maximum response size and
fails a larger response without retaining or reporting its contents. Before an
HTTP error body can enter a returned error or action-log entry, every exact
occurrence of the configured API key is replaced with a fixed redaction marker.
The same rule applies to transport diagnostics, including a redirect URL
selected by a hostile peer.
Tests exercise both the returned-error and persisted-action-log paths with a
server that deliberately echoes the credential.

Cleanup is part of the test result. A test registers cleanup immediately after
each resource is created, closes every deployment, revokes child credentials,
and verifies terminal state through both Console and chain views. The target
orphan sweeper runs after the suite, using the run registry and SDL hash when
no resource label exists. Cleanup failure fails the job and reports
the full resource identifiers without printing credentials. The sandbox
account keeps only the capped balance needed for the run.

The mutation deadline expires before the overall test deadline and leaves a
fixed cleanup reserve. Cleanup has separate bounded phases for ambiguous-write
discovery, auto-top-up disablement and close requests, and terminal-state plus
final-spend observation. No discovery loop or single subprocess may consume the
final observation reserve. When a create outcome is ambiguous, cleanup finds
only post-baseline deployments carrying the run's unique SDL hash; once it
finds one, it disables auto top-up before attempting close.
The ambiguous-create discovery allowance MUST be no shorter than the normal
post-create indexer-observation allowance. The current 90-second cleanup
reserve assigns up to 30 seconds to discovery, retains 40 seconds for
auto-top-up disablement and close, and retains the final 20 seconds for
terminal-state and balance observation.

**Current implementation boundary (2026-08-12):** the opt-in live suite covers
create, bid, lease, provider status, deposit, settings, update, and idempotent
close. Its independent raw observer and final balance delta are Console-side
proof only. It also requires a provider-reported workload URI to serve a
bounded non-empty 2xx response through an independent standard HTTP client,
non-empty structured log and event streams, and an exact sentinel from
deterministic non-interactive shell execution. The shell operation must produce
exactly one successful `provider/lease-shell` action while status, logs, and
events produce none. The child API-key scenario proves the one-time credential
authenticates the same tenant through both the CLI and a raw observer, revokes
it, returns both listings to their exact baseline, proves the revoked secret
fails, and scans the temporary home for disclosure. Independent chain and
Kubernetes oracles, a persistent run registry, the orphan sweeper, and a
tenant lock beyond the serialized sandbox CI group are not
implemented yet. Until those pieces land, the suite is valuable live
compatibility and cleanup evidence but does not satisfy the complete Console
lane contract in this section, and abrupt process termination can bypass its
in-process cleanup.

GitHub concurrency groups are repository-wide and evaluated from the proposed
workflow. An organization member already allowed to run a same-repository
pull-request workflow could deliberately cancel the sandbox group before
environment approval. The checked-in non-cancellation policy therefore handles
ordinary superseding events, not malicious workflow edits; the target external
lock and orphan sweeper remain required for that failure mode.

### 12.9 Race, fuzz, and mutation gates

Pull-request CI runs the Go race detector over active packages with the same
semantic build tags as the release binary. Tests that exercise stores, action
logs, event delivery, monitor pipelines, sync, workflow callbacks, and client
retry state MUST include concurrent cases where those components promise
concurrent use.

Boundary parsers and decoders have native fuzz targets. The minimum set covers
resource filters, deposits, SDL input boundaries, store imports, Console JSON,
provider stream records, transaction result decoding, and consensus/event
parsing. Pull requests run every seed corpus for a short bounded interval;
scheduled jobs run longer campaigns and retain minimized crashing inputs as
regression corpus files. A fuzz target must assert invariants such as no panic,
bounded allocation, canonical round trip, or stable error classification. Mere
absence of a crash is not enough when a stronger invariant exists.

Mutation testing runs on critical packages. The initial package mutation scores
are recorded as a baseline and cannot decrease. New or changed critical logic
MUST introduce no surviving non-equivalent mutant. The staged aggregate target
is 90%, after which the score continues to ratchet toward 100%. Equivalent or
unbuildable mutants require the exception record from §12.4.

### 12.10 Reporting and Codecov

CI uploads unit, hermetic E2E lane, repository, active, experimental-TUI, and
union profiles to Codecov with stable flags from trusted default-branch runs.
Live and union-live profiles remain pull-request artifacts and do not receive
an OIDC upload. Codecov uses line coverage, while the repository ratchets use
Go statement counts;
Codecov statuses and the badge are therefore visualization and review aids, not
the authoritative merge gate. The repository-owned active-union ratchet and
100% changed-statement check remain authoritative, and the stable `required-ci`
aggregate carries those controls into branch protection. Codecov's project
status disables removed-code behavior that could hide an aggregate line-
coverage regression. A Codecov service outage does not change the local
coverage calculation; CI retains profiles as build artifacts and may retry the
upload.

The repository MUST have a valid CODEOWNER with write access for the complete
tree, including `.github/CODEOWNERS` itself. The configured branch ruleset's
required code-owner review therefore applies to workflow, coverage taxonomy,
exceptions, baselines, Codecov policy, design/specification, and release
changes instead of being an ownerless no-op.

The active patch status is scoped to the `active-union` flag. GitHub inline
annotations MUST be disabled because Codecov does not support annotations for a
flag-scoped patch status and is deprecating that presentation path. This does
not disable the Codecov status, pull-request comment, flag report, or badge.

The README displays a Codecov badge for `akash-network/akt` on the default
branch. The badge represents the active union profile, not the easier unit-only
profile or the repository denominator. Its link opens the Codecov coverage
report so package and line details remain inspectable.

The job that checks out and executes repository code MUST NOT hold an OIDC
token permission. It publishes the already-generated profiles as a narrowly
scoped artifact. Separate upload-only jobs have no repository checkout or
repository command execution, download only generated report artifacts, and
are the sole holders of `id-token: write` for tokenless Codecov upload. The
upload job runs only for the trusted default branch. Pull-request workflows
never receive OIDC. The upload job may
populate Git's object database and index for Codecov source-network metadata,
but MUST NOT materialize a worktree. The first activated upload must verify that
module-qualified Go profile paths map to repository sources. The action and
downloaded Codecov CLI are both pinned, and a
downloader signature or checksum failure is fatal inside the OIDC job. Service
availability remains informational through job-level `continue-on-error`; it
is never implemented by executing an unverified CLI.
The active-union upload MUST run before narrower unit, lane, repository, or
experimental profile uploads so a later informational upload failure cannot
prevent publication of the primary Codecov signal.

Coverage reports list statement totals as well as percentages. CI prints the
current and baseline value for every regressing package, changed lines that
were not executed, expired exceptions, and behavior-manifest records without a
scenario. A single aggregate percentage is not an adequate failure message.

The workflow exposes a stable `required-ci` aggregate over lint, build and unit
tests, the active race suite, repository coverage enforcement, and the
protected Console sandbox job for eligible same-repository pull requests.
Default-branch protection MUST require that exact check. Merely defining the
job in the repository does not make it a merge gate; the external repository
ruleset is a required part of the control.

### 12.11 Completion criteria

The testing overhaul is complete only when all of these conditions hold:

- the active union is at least 95% and every active package has a ratchet;
- changed active lines have 100% coverage or a valid reviewed exception;
- every assembled CLI action and MCP tool has at least one meaningful
  behavioral scenario and the required negative or boundary cases;
- every state-changing action has independent post-state and action-log proof;
- all required lanes in §12.7 pass for the release commit;
- race, fuzz corpus, and critical mutation gates pass;
- the repository, active, and experimental-TUI profiles remain separately
  visible in CI and Codecov, while the live profile remains visible in CI.

After these conditions are met, active coverage keeps ratcheting toward 100%.
The 95% milestone is a floor, not a stopping point.

---

## 13. Phased Implementation Plan

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
| 1.9  | Transaction commands  | All executable `tx` module commands (bank, deployment, market, provider, cert, audit, staking, distribution, gov, authz, feegrant, escrow, wasm, oracle, bme, slashing, vesting, upgrade, IBC); dependency stubs and messages not registered by the Akash app are omitted | Each advertised command constructs a supported action; empty or unsupported groups are absent |
| 1.10 | Query commands        | All `query` module commands                                                                                                                                                                 | Each command matches the behavioral output of the current `akash` binary                                |
| 1.10a | Resource filter parsing | `internal/filter/` package implementing the `/`-separated positional filter argument (§3.8) for Akash query commands: deployment, market (order/bid/lease), cert, audit, escrow. Smart type detection (bech32 vs uint), `--by provider` mode, get-vs-list heuristic. | `akt query deployment 12345`, `akt query market lease akash1.../12345/1/1/akash1prov...`, and all §3.8.6 examples work correctly |
| 1.11 | Key commands          | All `keys` subcommands                                                                                                                                                                      | Full key lifecycle works (create, export, import, delete, show, list)                                   |
| 1.12 | Auth utility commands | sign, sign-batch, multisign, validate-signatures, broadcast, encode, decode                                                                                                                 | Offline signing workflow works end-to-end                                                               |
| 1.13 | Output formatting     | Pretty output registry with per-type formatters for all query results. `--output pretty` (default) renders lipgloss-styled tables/sections. `--output json\|yaml` produces machine-readable output. See section 10. | All query results render pretty tables (list) or sectioned key-value (detail) by default. `--output json` or `--output yaml` produces machine-readable output. |
| 1.14 | Global flags + env    | All global flags, env var mapping, override chain                                                                                                                                           | Override chain works: flag > env > config > default                                                     |
| 1.15 | Shell completion      | bash, zsh, fish completion scripts                                                                                                                                                          | Tab completion works for commands, flags, and context/network names                                     |
| 1.16 | E2E test suite        | Core test coverage for context, network, tx, query                                                                                                                                          | Tests pass in CI against a local testnet                                                                |

GitHub-hosted CI and release workflows pin every external action to an
immutable 40-character commit SHA. Coverage-policy validation rejects tags,
branches, and abbreviated hashes in any workflow `uses:` entry; local actions
under `./` are exempt. Action upgrades must pass `actionlint`, every required
PR check, and the release workflow's manual `check` mode before merge. Tool
versions remain explicitly pinned where reproducible results require them.

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
| 2.12 | MCP server              | `akt mcp` command with stdio JSON-RPC transport, up to 27 read-only tools and 6 write tools gated behind `--enable-writes`     | Read-only tools query the configured chain and/or Console rail; write tools are opt-in. Provider calls authenticate through the shared gateway client. EOF and process signals stop the server cleanly. |

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
| 3.10 | Network dashboard (from aktop)      | Consensus state (height/round/step, vote progress bars, vote grid), validator voting view (scrollable list, moniker resolution, signing history), recent governance proposals with tallies, governance params | Consensus updates via WebSocket, validator monikers resolved and cached, active proposal tallies refresh, governance params for all 12 modules |
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
| 4.6  | Governance actions (TUI) | Voting and deposit actions for the read-only proposal view | User can vote and deposit from TUI after confirmation |
| 4.7  | Validators view (TUI)    | Validator list with delegation actions             | User can delegate/undelegate from TUI              |
| 4.8  | Escrow view (TUI)        | Escrow account list and detail                     | Escrow state visible in TUI                        |
| 4.9  | Wasm view (TUI)          | Contract list, info, state queries                 | Wasm contract browsing in TUI                      |
| 4.10 | IBC view (TUI)           | Channel list with state                            | IBC channels visible in TUI                        |
| 4.11 | TUI transaction actions  | Create deployment, fund escrow, etc. from TUI      | Full transaction workflow in TUI                   |
| 4.12 | Performance optimization | Lazy loading, virtual scrolling                    | Lists with >10,000 items render smoothly           |
| 4.13 | Comprehensive verification | Implement the coverage, behavioral manifest, and environment contract in §12 across the active CLI, MCP, Console, provider, monitor, and experimental TUI profiles | Active union coverage is at least 95% with 100% changed-line coverage and per-package no-regression ratchets; every assembled command and MCP tool has a state-based scenario; required race, fuzz, mutation, and E2E lanes pass for the release commit |
