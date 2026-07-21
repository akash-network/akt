# Console Compatibility Matrix (AKT-649)

Status of `akt` coverage for every Akash Console capability. "Covered" means
the capability is reachable from `akt` end-to-end; "Deferred" entries carry an
explicit rationale and, where applicable, the building block that already
exists.

Reference points: the Console API surface (SPEC §7), the `console-axi`
reference CLI, and the Console web app.

## Covered

| Console capability | akt equivalent | Notes |
|---|---|---|
| Authenticate with API key | `akt console login/logout/whoami`; per-context credential (`akt context edit --console-api-key`) | Key resolution: flag > `AKT_CONSOLE_API_KEY` > per-context file (SPEC §7.1). Switching context switches Console identity. |
| Create deployment (managed wallet) | `akt console deployment create <sdl> --deposit <usd>`; `akt deploy` in a `console-api` context | Deposit in USD (min $0.50). Manifest auto-cached per context for the follow-up lease. |
| List / inspect deployments | `akt console deployment list/get` | Pagination via `--skip/--limit`. |
| Update deployment SDL | `akt console deployment update`; `akt update` workflow (console routing) | |
| Close deployment | `akt console deployment close`; `akt close` workflow (console routing) | Idempotent: already-closed reports success. |
| Escrow deposit | `akt console deployment deposit` | USD-denominated. |
| Auto top-up settings | `akt console deployment settings [--auto-top-up]` | `/v2/deployment-settings`, PATCH with POST fallback. |
| View bids / create lease | `akt console bid list <dseq>`, `akt console lease create` | The `akt deploy` workflow automates bid wait/selection. |
| Wallet balances & managed wallets | `akt console wallet balance/list` | µACT rendered as USD (1 ACT = 1 USD). |
| Wallet auto-reload | `akt console wallet settings [--auto-reload]` | The only headless funding path, matching the reference CLI. |
| Cost estimate & usage history | `akt console wallet cost`, `akt console usage` | |
| Provider marketplace browse | `akt console provider list/get/regions/auditors` | Public endpoints; no key required. |
| GPU availability & pricing | `akt console gpu` | Public. |
| Template catalog | `akt console template list/get/sdl` | `template sdl` pipes raw SDL for `akt deploy`. |
| API key management | `akt console apikey list/create/delete` | Create shows the secret exactly once. |
| Provider-scoped JWT minting | `akt console jwt create` | Building block for provider access from managed contexts. |
| Action audit trail | Automatic: state-changing Console calls are recorded in the per-context action log (`akt context log`) | Beyond Console parity — Console itself has no client-side audit trail. |

## Deferred (with rationale)

| Console capability | Status | Rationale / building block |
|---|---|---|
| Logs, events, exec, shell for managed deployments | Deferred | The reference CLI reaches providers through Console's websocket relay (`provider-proxy`), an intricate in-band-JWT protocol. akt already has direct provider-gateway streaming (`akt provider lease-logs/lease-events/lease-shell`) for wallet contexts; for managed contexts the missing piece is feeding a Console-minted JWT (`akt console jwt create`, already available) into those commands. Follow-up: accept `--auth-token` on `akt provider` commands, or wire the relay. |
| Bid screening (`sdl screen`) as a command | Deferred | The client implements `POST /v1/bid-screening` (`console.ScreenBids`); only the CLI surface is missing. The `akt deploy` workflow's bid wait covers the main use case. Follow-up: `akt console screen <sdl>`. |
| SDL scaffolds (`sdl init/scaffolds`) | Deferred | Authoring aids, not Console API functionality; akt validates SDL on read via `pkg.akt.dev/go/sdl`. Template catalog (`akt console template sdl`) covers the "give me a starting point" flow. |
| Adding funds by card / 3DS payment | Not portable | Stripe checkout with 3DS is inherently interactive/web-only; the reference CLI defers to auto-reload as well. Headless path: `wallet settings --auto-reload true` + per-deployment auto top-up. |
| Account signup / email verification / team management | Not portable | Web-only Console account flows without public API endpoints. |
| Certificate management for managed wallets | Not applicable | The managed-wallet model signs server-side; no client certificates exist (mirrors the reference CLI, which has no cert commands). |

## Behavioral differences vs the reference CLI (intentional)

- Output follows akt conventions (`-o json|yaml|pretty`) rather than TOON.
- Credentials are per-context (SPEC §7.1) rather than a single global config
  file, so multiple Console accounts can be used side by side.
- The manifest cache is per-context (`contexts/<name>/manifests/`) rather than
  global, for the same reason.
- The composite `deploy` command is the existing `akt deploy` workflow with
  auth-aware routing (SPEC §7.4) rather than a separate code path.
