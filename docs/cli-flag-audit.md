# CLI flag compatibility audit

This audit compares **every Akash 2.1.0 command registration** with akt,
including inherited flags and renamed commands. It also checks the eleven
provider gateway commands and restores akt's disabled flag alternatives.
The reference has 331 registrations and 330 unique paths: IBC client `params`
is registered twice. The checked-in fixture records all 330 paths and the
provider commands, plus five Console commands with restored alternatives,
including explicit exclusions.

Sources:

- Akash 2.1.0, node commit `151b989a58f579ac08b688b325e0dfcc4b44fecb`, using
  recursive `akash --home <disposable-home> <command> --help`.
- `pkg.akt.dev/go/cli@v0.2.4`, including hidden key flags and flag aliases.
- Provider commit `fec815211090554241924cf6158c03d6583765f5`, gateway command
  registrations and shared builders in `cmd/provider-services/cmd/helpers.go`.
- Akt's assembled Cobra tree, including hidden and inherited flags.

The snapshot is [command-flag-audit.json](../internal/cli/testdata/command-flag-audit.json).
`TestUpstreamCommandFlagCoverage` resolves every mapped command and checks
its flags. Run `GOWORK=off go test ./internal/cli -run TestUpstreamCommandFlagCoverage`.
The fixture is a versioned comparison, not a claim about future upstream flags.

## Added or restored

| Area | Flags / behavior |
| --- | --- |
| Keys | Public-key JSON/base64 import, dry-run, multisig sorting, algorithm alias, list/delete/export/rename/mnemonic options, hidden offline compatibility; import-hex, list-key-types and migrate |
| Deployment and market queries | `--owner`, `--dseq`, `--gseq`, `--oseq`, `--provider`, `--state`, as applicable |
| Certificate queries | `--owner`, `--state` |
| Escrow blocks-remaining | `--owner`, `--dseq` |
| Deployment/group/lease transactions | `--dseq`, `--gseq`, `--provider`, as applicable |
| Bid creation | `--reclamation-window` |
| Search | `query blocks --query`, `query txs --events` |
| Provider gateway | `--node`, `--from`, `--chain-id`, restored lease `--dseq`, send-manifest `--gseq/--oseq`, shell `--replica-index`, events `--service`, logs `-t`, service-status |
| Console | Wallet settings `--auto-reload`, usage `--from/--to`, logs `--service`, lease create `--provider`, API-key create `--name/--expires-at` |
| Remote node | `status --node/-n`, `events --node` |

`akash keys` maps to `akt context keys`. Deployment/group and market `get/list`
paths map to akt's unified query commands. Their get-versus-list selection,
owner defaults, state verification, and positional input stay intact. Flags
are alternatives; supplying both newly restored forms for one input fails.
Existing provider-status positional precedence stays intact.

Provider event filtering must use the gateway's singular **`service`** query
parameter (`gateway/rest/middleware.go:requestStreamParams`), forwarded through
`router.go:wsEventWriter` to the Kubernetes service-label selector. The old
SDK method accepted a service argument but discarded it. Akt's stream adapter
now sends it; it retains the existing certificate checks and stream lifecycle.

## Explicit exclusions

| Source surface | Reason |
| --- | --- |
| Node server, Comet maintenance/validator tools, genesis/testnet, snapshots, prune, export, rollback, rosetta, debug | Node/operator/local database tools outside akt's client scope (DESIGN §1.2/§5.1) |
| `tx crisis`, `tx evidence`, `tx ibc channelv2` | Empty upstream groups; no usable transaction to add |
| `query wasm libwasmvm-version` | Reports the node's linked native runtime; akt builds without that runtime |
| `auth jwt` | Unfinished upstream utility: valid time windows are rejected, `--nbf` updates expiry, and `--scope` reads access. Akt already signs gateway JWTs internally; a standalone token interface needs separate implementation |
| Node `--log_*` and `--trace` | Akt uses its own `--verbose` / `--quiet` logging interface |
| Provider `--offline` | Gateway certificate verification requires chain access, even with a supplied gateway URL |
| Hidden provider `--owner` | Upstream never reads it; the authenticated owner comes from the signing account (`--from`) |
| Provider `--sign-mode` | Transaction signing mode does not configure JWT or mTLS authentication |
| Provider events `--tail` | Upstream registers but never reads it; the event endpoint does not implement tail semantics |

Provider daemons, Kubernetes operators and migration administration remain in
provider-services. Akt-only commands retain their existing flags; the restored
Console alternatives are included because their registrations had been
explicitly disabled. Monitor `--node` and consensus changes are in PR #127,
separate from this flag-compatibility PR.

## Verification boundaries

The command-tree test checks presence, not execution. Request-level tests
compare flag and positional query filters and transaction messages, including
owner scoping and get/list selection. Provider tests exercise TLS with JWT and
mTLS, reject unregistered/revoked certificates, and check service-status paths.
WebSocket adapter tests check the service parameter and stream records.
Transaction tests capture messages without broadcasting to a live chain.
