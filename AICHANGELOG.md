# AI Changelog

## Unreleased

### Fixed

- **Structured shell error composition is lossless**: when both the remote
  command and JSON/YAML rendering fail, the returned error wraps both causes
  instead of reducing the renderer failure to uninspectable text.

- **Lease shell ignored structured output and emitted arbitrary remote bytes**:
  Console and keyring-backed shell commands now share one formatting boundary.
  Pretty mode remains interactive; JSON and YAML require an explicit command,
  disable PTY allocation, and return separate `stdout` and `stderr` fields.

- **Provider gateway commands disagreed across discovery, output, and
  streaming paths**: provider addresses now resolve their on-chain host URI,
  YAML preserves the JSON data model, and missing leases fail before log,
  event, or shell streams are opened. Bounded logs enforce service and exact
  tail filters, one-shot EOF completes cleanly, follow EOF remains an error,
  shell defaults to `/bin/sh`, and closed local stdin no longer overrides a
  successful remote result. Gateway failures retain response bodies when the
  provider supplies one, and both keyring and Console rails use the same
  boundary helpers.

- **Provider gateway behavior was underspecified across authentication rails**:
  provider addresses now resolve through their on-chain host URI unless an
  explicit URL overrides it; gateway streams verify the lease, apply log
  filters locally, and distinguish normal completion from interrupted follow
  mode. Structured output, shell stdin completion, and detailed gateway error
  contracts are defined once for both chain-backed and Console-backed calls.

- **`query wasm build-address` decoded salt twice**: the command now
  normalizes the selected hex, ASCII, or base64 representation to one canonical
  hex value before the derivation decodes it. A `00` salt works, every encoding
  selector derives the same address for the same bytes, the pure computation
  uses the local query boundary, and JSON/YAML return string scalars.

- **Wasm predictable-address input was underspecified**: `build-address` is a
  local derivation whose default salt encoding is hexadecimal; its three
  explicit encoding selectors are mutually exclusive and decode to salt bytes
  exactly once. Structured formats render the result as a string scalar.

- **Offline transaction utility contracts are now explicit**: construction
  emits one real JSON object, signing data goes to stdout or its requested
  document, zero means one unlimited message batch, and interactive proposal
  drafting refuses a missing TTY before terminal rendering. This documents the
  remediation behavior before implementation.

- **Offline transaction utilities hung or corrupted their data stream**:
  unlimited reward withdrawal now invokes one complete batch instead of a
  zero-step loop and refuses an empty message set instead of succeeding with
  no output; generated transaction bytes render as transaction objects instead
  of base64 strings; signing and signature validation write data to stdout or
  the requested document. Proposal drafting refuses non-TTY input before
  starting its selector.
- **Unsupported and empty transaction groups looked executable**: the Akash
  app has no crisis message handler, the evidence transaction group has no
  concrete submission type, and upstream IBC channel-v2 has no packet actions.
  These groups are now omitted instead of appearing as a doomed transaction or
  successful help-only leaf.

- **Transaction execution-boundary behavior is now explicit**: every local or
  dependency-owned transaction leaf must initialize from the selected context
  before message construction or account lookup, fixed fees override all gas
  price sources, and non-zero simulation codes are command failures without
  becoming action-log entries. This documents the remediation contract before
  implementation.

- **Transaction leaves bypassed context setup and fee/failure semantics**:
  five local leaves no longer panic before construction; adopted IBC and
  upgrade handlers plus signature validation retain the selected RPC client;
  failed simulations return an error without logging a mutation. Transaction
  defaults now honor flag, environment, and context precedence, and fixed fees
  clear gas prices before the factory is built. Dry-run also normalizes SDK
  gas-auto into simulation-only mode so an address signer never triggers a
  key lookup, while help remains configuration- and network-free.

- **MCP numeric arguments lacked a precise boundary contract**: sequence
  identifiers are positive whole numbers and pagination values are
  non-negative whole numbers. Tool schemas expose those constraints, and the
  server refuses fractional, negative, non-finite, and out-of-range values
  instead of coercing them or silently applying defaults.

- **Remediation changes violated the repository's lint rules**: network
  cloning no longer shadows Go's built-in `copy`, and Console acknowledgement
  tests assert decoded booleans without redundant literals.

- **Key detail and address parsing commands ignored machine output**:
  `context keys show` and `context keys parse` now render canonical JSON and
  YAML values through the selected command writer. Address-only output stays
  raw in pretty mode and becomes a quoted scalar in machine formats.

- **Workflow dry-runs planned invalid invocations**: generated workflow
  commands now enforce required and typed parameters before printing a plan,
  including readable and valid SDL files, unified deposit syntax, positive
  sequence IDs and durations, and complete bid selectors. Wait and prompt
  steps reject invalid resolved values instead of silently substituting
  defaults.

- **Transaction identity and signer overrides were resolved too late**:
  online transaction construction now rejects a chain ID that disagrees with
  the selected context before flags can overwrite that identity; explicit
  offline construction remains portable. `--from` and `AKT_FROM` are applied
  before the SDK client context is built, with flag, environment, then context
  default precedence.

- **Transaction mode typos crossed the CLI boundary**: all assembled
  `--sign-mode` and `--broadcast-mode` flags now enforce their advertised
  enums, including dependency-owned commands, and help names `eip-191` and
  `block` everywhere those flags appear.

- **Certificate and batch-multisign flags leaked across sibling commands**:
  certificate generation, publication, and revocation now read every option
  from the executing leaf. Batch multisign likewise reads
  `--no-auto-increment` locally instead of an unbound package-global Viper
  key.

- **Transaction mode defaults and accepted values disagreed across help and
  execution**: the transaction contract now names `direct` as the sign-mode
  default and defines both sign and broadcast modes as closed enums, including
  the supported `eip-191` and `block` values.

- **Context-owned commands could split one invocation across two contexts**:
  `context show`, `context log`, and Console login/logout fell back to
  `current-context` after the root had accepted `--context`. They now share
  the selected-context rule for both the flag and `AKT_CONTEXT`; root keyring
  and action-log setup use that same target. Console auth emits redacted
  structured acknowledgements, and structured context details include
  resolved network, keyring, capabilities, store, and action-log paths.

- **`context edit --fork-network` was an accepted no-op**: context edit now
  exposes the documented network fields, validates that a fork has an edit to
  apply, and performs context plus parent/fork changes in one copy-on-write
  config save. A private `<network>-<context>` fork leaves every sibling
  context on the original network; rejected edits no longer mutate manager
  state before returning an error.

- **`context log --type` accepted values it could never match**: the filter is
  now validated against the six documented action types before opening a log,
  and an explicit context is resolved before its path is accessed.

- **Remaining invocation-boundary behavior was underspecified**: DESIGN and
  SPEC now define one selected context for context details, logs, and Console
  credential mutations; flag/environment account precedence; transaction
  chain and sign-mode validation; real network forking; structured key output;
  and typed workflow preflight that runs before dry-run plans. These contracts
  make the outstanding full-surface reproductions executable before their code
  changes are applied.

- **The transaction sign-mode table named an unsupported mode**: the client
  implements `direct`, `amino-json`, `direct-aux`, and `eip-191`, while SPEC
  named `textual` and leaf help omitted the fourth implemented mode. The
  boundary contract now follows the actual signer set that validation will
  enforce.

- **SDL output selection and redirected store status were cosmetic flags**:
  `sdl init` silently emitted YAML after an explicit JSON/YAML selection,
  `sdl validate` ignored structured output entirely, and `store status` leaked
  ANSI styling into redirects and `NO_COLOR` output. Raw SDL generation now
  refuses explicit format selection, validation has stable JSON/YAML results,
  and the store pretty writer strips styling outside an interactive terminal.

- **Alternate query pre-runs skipped verbose diagnostics**: dependency-owned
  IBC commands, direct CometBFT block queries, and local derivations now honor
  `-v` consistently. Network queries report the selected endpoint and chain on
  stderr; local queries identify their local execution and selected chain.

- **Vendored and local query leaves could bypass core CLI guarantees**:
  explicit node and height overrides were ignored by several leaves, two IBC
  parameter queries panicked on transport errors, IBC YAML emitted JSON, two
  paginated calls returned a lookahead item, local scalar results were invalid
  JSON, and one IBC path was registered twice. The query boundary now defines
  endpoint/snapshot refusal rules, safe vendored error handling, normalized
  structured output, hard pagination limits, and unique selectable paths.

- **Console structured output reflected Go transport internals**: YAML changed
  JSON field names and turned raw strings/objects into integer arrays, while
  logs, deployment close/deposit, and template SDL ignored the selected format.
  Console JSON is now the canonical data model for YAML conversion, gateway
  streams have record-oriented JSONL/YAML output, mutations have explicit
  acknowledgement schemas, and template SDL preserves its exact source in a
  structured wrapper.

- **Invalid SDL scaffold values were reported as internal failures**: every
  explicit generation parameter now follows the same parser and lint contract
  as `sdl validate`, exits as a usage error without partial stdout, and names
  the responsible flag. Image references use standards-based parsing so empty
  tags and malformed digests are rejected; internal errors are reserved for
  invalid built-in defaults.

- **CLI groups and output flags could accept the wrong input at exit 0**:
  unknown tokens under completion, IBC, and upgrade groups printed help as a
  successful command, while leaf-local `--output` strings bypassed the root
  enum and mapped misspellings such as `josn` to pretty output. Group
  validation is now applied to the assembled command tree, and every output
  flag validates its command-specific enum before configuration or network
  work. Bare certificate lists now follow the same owner-scoping contract as
  deployments and market resources: they use the context default account or
  refuse locally instead of querying every certificate on the network. The
  localnet query and deployment lifecycle coverage now supplies its validator
  address explicitly so CI exercises that scoped contract.

- **`akt q staking params`/`pool` could panic on a sparse response**: proto3 omits zero-valued fields, so an unset `LegacyDec`/`Int` unmarshals with a nil inner `big.Int` and any arithmetic on it panics — `FormatPercentDec` and `FormatDecAsAKT` did exactly that. Two independent reviewers hit it. Formatting now goes through `DecOrZero`/`IntOrZero`, so an omitted field renders as `0` instead of crashing the command. 3 regression tests.

- **A failed config write reported success**: `SaveConfig` deferred `f.Close()` on the file it had just created, discarding the flush error — a full disk or I/O failure while writing `config.yaml` returned nil. The store's YAML export had the same bug (a truncated export reported success). Both now close explicitly and return the error.


- **The release pipeline would have shipped broken binaries**: `.goreleaser.yaml` passed `{{ .Env.BUILD_LDFLAGS }}` to every build, but nothing ever set that variable — the Makefile's `GORELEASER_LDFLAGS := $(ldflags)` sat two lines *above* the `ldflags :=` assignment, so it expanded to the empty string (same for `MOD` and `BUILD_TAGS`). Every released binary would have reported `dev / none / unknown` and been built with no tags. ldflags are now literal and self-contained in the goreleaser config, driven by `{{ .Version }}`/`{{ .FullCommit }}`/`{{ .Date }}`, injecting both `main.*` and the cosmos-sdk version vars. Also fixed: darwin builds linked libwasmvm with an rpath into the *builder's* Go module cache (dead on any user's machine) — the `static_wasm` tag now links the static archive, verified with `otool -L`; the pinned `goreleaser-cross` image tag did not exist (`v1.26.1` → pinned `v1.26.2`); `project_name` was `node`; `RELEASE_DOCKER_IMAGE` pointed at the node repo and was defined twice.


- **`akt version` always reported "dev" (AKT-221)**: the Makefile injected build info into the cosmos-sdk version vars but never into `main.version/commit/date`, which is what the CLI actually prints — so every built binary, including release builds, claimed `dev (commit: none, built: unknown)`. Both are injected now. The `--long` flag the task promised was also never implemented; it now prints version, commit, build date, Go version, platform, and build tags.


- **A missing `data` envelope read as success (code review)**: `doData` returned nil with the result left zero-valued whenever a response omitted the envelope — an API change, a proxy error page, or any wrong-shaped 200 would surface as a successful call with, say, an empty dseq. A caller that expects a payload now gets an error naming the request; endpoints that return no body (DELETE) still pass a nil result and succeed.

- **Per-context credential directories were world-readable (code review)**: the Console API key file is 0600, but its parent (`contexts/<name>/`) was created 0755, leaking the credential's existence and letting a local user replace it. Both `EnsureContextDirs` and the credential writer now create the context directory 0700.

- **`ParseDeposit` kept untrimmed input (code review)**: `Deposit.Raw` stored the caller's string verbatim while parsing used the trimmed value, so a padded deposit in a user workflow YAML (`"  auto "`) parsed correctly and then failed downstream when the rail adapter re-parsed `Raw`. Raw now carries the trimmed value.


- **Positional state was silently ignored on the get path (code review)**: `akt query deployment 12345 closed` fetched by identity and printed the record regardless of state — and since the sweep removed `--state`, that positional is the only state filter left. A complete identity now still returns the detail view, but a supplied state acts as an assertion: a mismatch fails with both states named ("... is closed, not active; drop the state argument to print it regardless"), which keeps the output shape stable and makes the check scriptable. Applies to deployment, order, bid, and lease.

- **`tx deployment close`/`update` lost their dseq guard (code review)**: with `--dseq` unregistered, a missing positional built a message with dseq 0 and entered the sign/broadcast pipeline — keyring unlock, then a raw chain validation failure. Both now fail fast in an Args hook (before any connection is opened) with the same friendly message the group and lease commands print.

- **`akt sdl init --count 0` reported an internal error (code review)**: explicit zero/out-of-range values for count, port, as, price, and gpu leaked past flag handling into the post-generation self-check and surfaced as "internal error: generated SDL failed validation". They are now bounded usage errors (exit 2) with the valid range named; unset flags still take the scaffold default.

- **README claimed `--state` still worked (code review)**: the doc refresh added that line just as the sweep removed the flag. Corrected to the positional-only reality, including the identity+state form and the get-path assertion behavior.


- **Help detection disabled context and capability enforcement (code review)**: `helpRequested` matched any argv token equal to `--help`/`-h`/`help`, including positional values and tokens after `--`, and it gates bootstrap, the no-context guard, and capability checks. `akt tx deployment close help` therefore skipped the guard on a zero-context machine and went straight to broadcast. Detection now matches only the flag forms, stops at the `--` terminator, and recognizes cobra's `help` command separately.

- **`akt sdl` and `akt console` were blocked without a context (code review)**: `requiresContext`'s prefix list was never updated for the new groups, so offline SDL validation and `AKT_CONSOLE_API_KEY=... akt console whoami` both failed with "no contexts configured" — making the console group's own env-key fallback unreachable. Both groups are now exempt.

- **Capability gating ignored per-invocation overrides and `off` mode (code review)**: gating was computed from the context alone, so `--node`, `--console-api-key`, and `akt monitor <endpoint>` were rejected even though each supplies its own connection details, and `defaults.command-gating: off` only affected help presentation. Overrides now grant their capability, and `off` disables enforcement as documented.

- **Gating and help disagreed on the active context (code review)**: enforcement resolved via `Resolve(--context)` while help had its own single-context fallback, re-synced only `--home`, and read the mode from config instead of viper — so an auto-selected single context showed `[unavailable]` in help yet executed ungated. Both paths now share `Manager.ActiveContext` and one mode resolution.

- **`--context` was ignored by the console group and workflow commands (code review)**: they resolved via `Resolve("")`/`CurrentContext()`, so `akt --context staging console login` stored the credential under the *current* context and `--context staging console deployment create` billed the wrong managed wallet. Both now honor the override (`ctxNameFn` for workflows and store, the inherited flag for console).

- **Network-less console contexts could not be edited (code review)**: `UpdateContext` required a resolvable network even though `CreateContext` permits omitting it for `console-api` contexts — so `akt context edit <ctx> --console-api-key ...`, the exact remedy other errors suggest, always failed with `network "" not found`.

- **`--help` on SDK module groups required a configured context**: the clean-copied cosmos-sdk group commands (`tx bank`, `tx authz`, `query staking`, …) set `DisableFlagParsing`, so cobra cannot short-circuit `--help` before the root hooks run — on a machine with no config those hooks rejected the command with "no contexts configured" before help printed (the CI failure mode; masked locally by the developer's real config). Root hooks now detect help invocations from argv and skip both bootstrap and the context requirement, so help always works. `TestAllCommandsHelp` is hermetic via `AKT_HOME` pointed at an empty temp home (the env var, because those same groups ignore an unparsed `--home` flag — a pre-existing SDK-copy quirk).

- **First-run bootstrap ran headlessly and hung CI**: with no config and no TTY, bare `akt` silently ran the bootstrap wizard — fetching the network list from the GitHub API and writing a config assembled from non-interactive fallbacks (all networks, `os` keyring) — which made `TestTUINoArgNoTTY` time out on GitHub runners (it passed locally only because the developer's real `~/.config/akt` exists). The wizard now declines to run without a terminal (no fetch, no config, guidance printed to stderr) per SPEC §2.0, and the e2e test is hermetic (fresh `--home`) so it exercises the CI condition everywhere. 2 tests.

- **`context log --type` help text listed the wrong action types**: it offered `tx, query, workflow, error`; the recorded types are `tx, workflow, provider, context, console, error` (queries are not recorded by design).

- **Chain commands violated the positional-primary convention (AKT-650)**: `tx market lease create/withdraw/close` now take `[dseq] [provider]` positionally, `query escrow blocks-remaining` accepts the §3.8 `[owner/]dseq` filter argument, `query blocks`/`query txs` take the search expression positionally, and `tx deployment group close/pause/start` no longer require `--owner` (defaults to the signer — closing your own group needs zero flags). Flags remain as overrides; positionals win. Plus a new convention guard, `TestNoUnapprovedRequiredFlags`, that walks the whole command tree and fails when any flag is marked required without an allowlisted justification (allowlisted: signer selection on `tx sign`/`sign-batch`/audit attr, governance `--title` flags, `context create --network`). SPEC §2.1/§3.8.5 updated; 3 new helper test suites.

- **Console commands violated the positional-primary convention (AKT-650)**: seven `akt console` commands required flags for their primary values. All now take them positionally with flags kept as overrides (positional wins): `usage [from] [to]`, `deployment create <sdl> [deposit-usd]`, `deployment deposit <dseq> [amount-usd]`, `deployment settings <dseq> [true|false]`, `lease create <dseq> [provider]`, `wallet settings [true|false]`, `apikey create <name> [expires-at]`. SPEC §2.9 signatures updated and a convention note added. 2 new command tests.

- **`akt console usage` failed with a 400 when `--from`/`--to` were omitted**: `GetUsageHistory` always sent `startDate`/`endDate`, and the API rejects empty strings with a `format=date` validation error. Empty dates are now omitted (the API then defaults endDate to today, startDate to 30 days prior per its OpenAPI contract) and non-empty dates are validated as YYYY-MM-DD client-side before any request. 2 regression tests.

- **Block query commands ignored the context RPC endpoint**: `akt query block/blocks/block-results` used the SDK's `GetClientQueryContext`, which falls back to the `--node` default `tcp://localhost:26657`, instead of the package-local resolver that builds the RPC client from the active context's network. All three now resolve the context endpoint like module queries; the localnet e2e block test dropped its `--node` workaround.

- **Default `--gas auto` broadcast transactions with gasWanted=0**: `ClientOptionsFromFlags` only forwarded the gas setting when the flag was explicitly changed, so the default `auto` never reached the tx factory — no simulation, gasWanted 0, and every real send failed CheckTx with out-of-gas. The gas flag is now always parsed and applied (`auto` → simulate); invalid values are rejected instead of silently ignored. Verified end-to-end on the localnet with a default-gas `tx bank send`. 3 new tests.

- **Failed broadcasts exited 0**: a CheckTx result with a non-zero code was printed as if successful and the CLI exited 0. The tx-client wrapper (now applied unconditionally, with or without an action logger) converts a non-zero result code into an error carrying the code, codespace, tx hash, and raw log, so failed transactions exit non-zero. Generate-only/simulate/offline runs are unaffected. 3 new tests.

- **Builtin deploy workflow glue could never round-trip (AKT-647)**: Step outputs rendered through Go templates with `fmt` map syntax, so the select-bid prompt step couldn't parse the wait step's bids and `create-lease` referenced a nonexistent `.bid.id`. Added a `toJson` template function, made the prompt executor emit the selected bid's identity as discrete outputs (`provider`/`dseq`/`gseq`/`oseq`/`price`, numbers as decimal strings) with genuine cheapest-price selection, gave the output step an injectable writer (JSONL mode must own stdout), and rewrote deploy.yaml's select-bid/create-lease/send-manifest params accordingly. SPEC §2.3.9's update example dropped the unimplemented `foreach` step with a note.

- **Action log Read choked on entries larger than 1 MB (AKT-211)**: `readLogFile`'s scanner buffer capped lines at 1 MB, so an entry with large params (e.g. SDL contents) that `Log` happily wrote could never be read back ("token too long"). The buffer now allows lines up to the 10 MB rotation threshold. Added `TestLogRotation` covering rotation at the size threshold plus newest-first reads spanning rotated files — a gap TASKS.md T008 claimed was covered but wasn't.

- **Action logger never closed (AKT-211)**: The per-context action logger is opened in the root command's `PersistentPreRunE` but was never closed, diverging from SPEC §5.6. Added a root `PersistentPostRunE` that closes the logger retrieved via `cliutil.ActionLogFromContext`.

- **TUI dashboard build break from botched `refactor/tui-rewrite` merge**: The merge that integrated the TUI rewrite corrupted `internal/tui/views/dashboard.go` and left unresolved conflict markers in `internal/tui/views/dashboard_test.go`, breaking `make akt` (`undefined: colW`, `d.activityContent undefined`) and `go test ./...`. Restored `networkContent()` to return inner content via `strings.Join(lines, "\n")` (the merge had reintroduced an out-of-scope `components.TitledPanel("NETWORK", content, colW)` call referencing `colW`, a `View()`-local variable, plus a stray "chain" row) and renamed `renderActivity(w int)` back to `activityContent(innerW int)` with a pointer receiver (the method `View()` actually calls). Resolved the `dashboard_test.go` conflict in favor of the rewrite (`newTestDashboard()` helper + `View().Content`). Restores the intended rewrite state with no behavior change; chain-id continues to display in the header/welcome banner rather than the network panel.

### Changed

- **Documentation brought in line with the whole branch**: DESIGN.md gained §3.1.5 (Console provider-gateway access via a scoped Console-minted JWT, and why that beats reimplementing Console's websocket relay), §3.5 (the action-to-transport translation layer, with the unified deposit syntax as the concrete cross-rail normalization), and §3.6 (capability gating: derivation, the `akt.requires` annotation, per-invocation overrides, dim/hide/off); its package listing now matches disk (dropped `internal/cli/tx|query`, `internal/filter`, `internal/flags`, which the chain clean-copy superseded; marked the `plugin` entries planned) and its "Transport Layer" naming collision with `internal/transport` is disambiguated. SPEC gained §2.12 documenting `akt version --long` and `akt completion` — the only commands in the tree with no reference section. README documents capability gating (with the `context show` Capabilities block), the `akt sdl` group, the expanded Console surface including live lease operations, the disabled TUI shell, and concrete positional-primary examples. AGENTS.md records the conventions this branch established: positional-primary commands and their guard test, the `FEEDBACK(2026-07)` disabled-flag marker, capability annotations for new command groups, actions-in-workflows/behavior-in-transport, action-log write paths, and `GOWORK=off` for direct `go` invocations. Every example was verified against the built binary; a SPEC §7.8 claim that `template sdl` pipes into `akt deploy` was corrected (only `sdl validate` reads stdin).


- **Console compatibility matrix moved into SPEC §7.8**: it lived at `docs/console-parity.md`, but `docs/` holds only dated superpowers plans and specs — a standalone reference doc there matched nothing. It now sits with the rest of the Console specification, and the content was refreshed: logs/events/status/shell, bid screening, and SDL authoring moved from "deferred" to covered (they shipped), signatures updated for the positional-only surface, and the `provider-proxy` relay reclassified as superseded rather than pending.


- **Positional-only UX trial: duplicate flags disabled (FEEDBACK 2026-07)**: every flag with a positional twin is commented out behind a uniform, greppable `FEEDBACK(2026-07)` marker (original registration lines preserved verbatim for one-step restoration): chain query identity/state flags (`--owner/--dseq/--gseq/--oseq/--provider/--state` on deployment/group/order/bid/lease, cert, escrow blocks-remaining, `--query`/`--events` on blocks/txs), tx twins (`--dseq` on deployment close/update, `--dseq/--gseq` on group cmds, `--dseq/--provider` on market lease cmds), provider `--dseq` on lease-status/logs/events/get-manifest, and eight console flags (usage dates, deposit/amount, settings toggles, lease provider, apikey name/expiry, logs service). To keep identity+state combos expressible, queries gain an optional second positional state (`akt q deployment akash1x/12345 active`) and `cert list` gains `[owner] [state]` positionals. Infrastructure flags (gas/fees/node, follow/tail/watch, pagination, `--by`, defaults-carrying gseq/oseq, group `--owner`) stay. SPEC rows annotated "disabled pending feedback" rather than deleted. Full suite green incl. live parse-stage sanity checks.

- **`akt context show` displays the feature set**: a Capabilities section now tells the user exactly what the configuration can do — chain queries/transactions, provider gateway, Console API — with the remedy inline for anything unavailable ("add an RPC endpoint to the network", "run akt console login"). Completes the capability-gating UX: commands are gated (SPEC §2.10) and the reason is discoverable.

- **TUI shell disabled pending UX feedback**: bare `akt` now prints help instead of launching the dashboard, and `--interactive`/`-i` reports that the TUI is disabled (pointing at the CLI commands and `akt monitor`, which is unaffected). The launch path stays compiled behind `AKT_EXPERIMENTAL_TUI=1` for feedback sessions. SPEC §2.0 annotated.

- **Documentation refreshed to match the delivered state**: README (status paragraph, action-log scope, workflow execution no longer "not yet wired", new Console Integration section, positional-first examples, roadmap pruned of already-delivered items), DESIGN (action-log scope, console-auth section notes the full `akt console` surface, phase status bullets), SPEC (console group added to the §2.1 command tree, context create/edit flag tables, `context log --type` values, §7.3 min deposit corrected to $0.50, §7.5/§7.6 examples, §1.9 env table, §12 milestone wording). TASKS.md verified — no checkbox changes needed. Two cross-doc contradictions flagged for follow-up rather than guessed at: README's TUI "scaffolded" wording vs TASKS T093-T104 complete, and SPEC §7.4's planned `foreach` step type.

### Added

- **Lint gate (`.golangci.yml` + CI job)**: the repo never had a lint config — TASKS.md T001 claimed one, and CI's own comment admitted lint was unwired. Adds a curated config (govet, staticcheck, errcheck, ineffassign, unused, nilerr, errorlint, bodyclose, unconvert, gochecknoinits, revive, misspell, gofmt/goimports) that took the tree from 1045 findings to 0. `internal/cli/chain` is style-excluded but keeps correctness linters, since it is a clean copy maintained by re-copying from chain-sdk; every exclusion is commented in-file with its reason, and `gochecknoglobals` was evaluated and dropped rather than blanket-excluded. The Makefile pin moved v2.3.0 → v2.11.4: v2.3.0 is built with Go 1.24 and refuses to load a config targeting this module's `go 1.26.1`.

- **Coverage measurement and gate (`make/testing.mk` + CI job)**: nothing measured coverage before. Three package sets — repo-wide (reported), akt-authored, and the 13 risk-carrying core packages (gated at 65%, currently 68.7%, up from 54.9%). 136 new test functions target money paths, credential handling, capability decisions, action-log writes, and the transport layer. Repo-wide is 30.9%: `internal/cli/chain` is 37.6% of all statements at 3.5%, so TASKS.md T126's ">80% overall" was never reachable as written — that is now stated plainly rather than aspirationally.

- **Weekly upstream-drift check**: a scheduled `e2e-localnet-latest` job runs the localnet suite against `ghcr.io/akash-network/node:latest`, while the blocking PR job stays pinned to the harness default. An upstream release can no longer redden an unrelated pull request, but drift is still caught. The pinned default moved 2.1.0 → 2.1.1 (verified against both that and `latest`), and the version now lives only in `e2e/localnet_test.go` instead of being duplicated in CI.


- **Release pipeline (`make release*` + `.github/workflows/release.yml`)**: `make/releasing.mk` previously had no release target at all. Adds `release-libs` (fetches the libwasmvm static archives every `-L./.cache/lib` already pointed at — the directory was empty), `release-check`, `release-snapshot`, `release-dry-run`, and `release`, plus a tag-triggered workflow (`v*`) with a `workflow_dispatch` check/snapshot/dry-run mode. Verified end to end locally: config check, a full four-target cgo cross-compile producing a universal darwin binary, linux amd64/arm64, archives, deb/rpm and checksums; the produced binaries were executed on darwin/arm64, linux/amd64 and linux/arm64 and report real version strings. Publishing itself is unexercised (no tag was pushed).


- **`akt sdl` command group (final console-axi parity item)**: transport-independent SDL authoring ported from the reference CLI — `sdl scaffolds` (web, gpu, multi-service, ip-lease; byte-faithful defaults incl. 10000 uact price ceilings), `sdl init <scaffold>` (deterministic YAML to stdout, pipeable into `akt deploy`/`akt console deployment create`, generation flags for image/port/resources/env/gpu), and `sdl validate <file|->` (parse via pkg.akt.dev/go/sdl + lint: unpinned/`:latest` images rejected, `uakt` pricing warns — softened from the reference's hard reject since akt serves both rails). Runs with no context, key, or RPC; no capability gate. SPEC §2.11. 19 tests.

- **Live lease operations for managed contexts (console-axi parity closed)**: `akt console logs <dseq> [service]` (`--follow/--tail`), `console events <dseq>` (`--follow`), `console status <dseq>` (`--watch/--interval`), `console shell <dseq> <service> [-- cmd]` (exec == shell with a command), and `console screen <sdl>` (public bid screening). Managed contexts reach provider gateways directly: resolve the active lease and provider hostUri from the Console API, mint a scoped short-TTL JWT (`POST /v1/create-jwt-token`), and reuse the provider client with `WithAuthToken` — the same streaming code paths as `akt provider lease-*`, no websocket relay port needed. Console subgroups and workflow commands now carry their capability annotations (`console`, `chain-tx|console`), completing the §2.10 gating coverage. 5 new tests (two-server httptest pattern).

- **Action-to-transport translation layer with unified deposits**: new `internal/transport` package formalizes the boundary the user asked for — actions (deploy/update/close and future ones) are defined once and translated per rail (`KindChain` = sign+broadcast, `KindConsole` = REST); the CLI constructs transports, never adapters. Deposits gain ONE syntax on both rails, translated at the boundary: `5usd`/`$5` (console; clear cross-rail error on chain), `5000000uakt`/`5akt` (chain; clear error on console), bare numbers keep their historical per-rail meaning, `auto`/empty passes through. A new regression test pins that `deploy/update/close` expose a byte-identical argument surface regardless of auth-method. SPEC §2.3 Transports paragraph + §7.4 deposit table. 20+ tests incl. an httptest round-trip proving `--deposit 5usd` reaches the Console wire as numeric `5`.

- **Capability-based command gating (feature sets)**: the active context's configuration now derives a feature set — chain RPC present → `chain-query`/`chain-tx`/`provider`; Console key present → `console` — and the command surface reflects it. Two presentation modes ship for UX feedback (`defaults.command-gating`): `dim` (default; unavailable commands marked `[unavailable]` in help and failing fast with the missing capability + remedy) and `hide` (removed from listings; direct invocation still fails fast), plus `off`. Enforcement runs in the root hooks and a wrapped help func so `--help` short-circuits are gated too. New `internal/capability` package + gating walk; `tx`/`query`/`provider`/`monitor` annotated (console/workflow groups follow with their in-flight refactors). 9 tests.

- **Network-less console-api contexts**: `akt context create <name> --auth-method console-api --console-api-key <key>` now works without `--network` — the context operates through the Console API alone and chain commands are capability-gated until a network is attached. This makes the "API key only, no RPC" configuration a first-class state instead of an impossible one. `--network` validation moved from MarkFlagRequired to RunE (guard-test allowlist shrinks by one).

- **OpenAPI contract tests for the Console client**: the Console API's OpenAPI 3.0 spec (vendored verbatim to `internal/console/testdata/openapi.json` from the console-axi reference) now validates every request the client produces — 34 subtests cover all 30 public methods through a kin-openapi request-validating harness, plus a negative control replaying the exact production 400 (empty `startDate`) to prove the harness detects violations. The suite immediately caught and fixed three real contract bugs: nil `SignedBy.AnyOf/AllOf`/`Attributes` slices marshaled as `null` where the schema forbids it, `ScreenBids` allowing requests without the required `resources`/`timezone`, and `ListDeployments` sending out-of-range `skip`/`limit` instead of deferring to server defaults. New test-only dependency `github.com/getkin/kin-openapi`.

- **Gated live Console smoke suite**: `e2e/console_live_test.go` runs read-only commands (`whoami`, `wallet balance`, `usage` with no flags — the exact 400 repro, `provider regions`, `gpu`, `template list`) against the production API when `AKT_E2E_CONSOLE_API_KEY` is set; skips otherwise.

- **Console onboarding in the first-run wizard (AKT-646)**: After network and keyring selection, the bootstrap wizard now offers optional Akash Console setup: enter a Console API key (hidden input; validated best-effort against `/v1/user/me` with an offline-tolerant warning; stored as the initial context's per-context credential per SPEC §7.1) and optionally route that context's deployments through Console (`auth-method: console-api`). Both prompts default to "no"; non-interactive runs skip the step entirely. SPEC §2.0 first-run description updated. 2 tests.

- **Console compatibility matrix (AKT-649)**: New `docs/console-parity.md` enumerating every Console capability with its akt equivalent, the deferred items with explicit rationale (provider-proxy log/shell streaming for managed contexts — `akt console jwt create` is the building block; `sdl screen` command surface; card/3DS funding and account flows that are inherently web-only; managed-wallet certificates that don't exist by design), and the intentional behavioral differences from the reference CLI (per-context credentials and manifest cache, akt output conventions, composite deploy = the `akt deploy` workflow with auth routing).

- **E2E suite expansion (AKT-222)**: New `e2e/offline_test.go` — keys lifecycle on the test backend (add/list/show/rename/export→import round-trip/mnemonic/parse/delete, deterministic `--source` recovery), context edit/log/rename with action-log assertions, network edit/delete (referenced-network guard), console-api context credential checks (0600 file, key never printed), AKT-650 positional-arg offline smoke tests, store import edge cases, and a table-driven `TestAllCommandsHelp` walking 113 command paths so a failure names the offending command. New `e2e/localnet_test.go` — gated localnet harness: uses `AKT_E2E_RPC` when set, else bootstraps a single-validator `ghcr.io/akash-network/node` container when `AKT_E2E_LOCALNET=1` (verified end-to-end locally: block queries, staking validators, bank balances, and a real `tx bank send` whose entry then appears in `akt context log --type tx` — the AKT-211 acceptance path), else skips.

- **CI pipeline (AKT-222)**: `.github/workflows/ci.yml` (the repo previously had no CI at all): build + unit tests, offline e2e against the built binary, and a docker-gated localnet e2e job (continue-on-error until proven on GitHub runners), with concurrency cancellation and `GOWORK=off`.

- **Workflow execution with auth-aware routing (AKT-647)**: `akt deploy/update/close` now actually execute (previously they printed a plan and bailed with "Execution requires chain client (not yet wired)"). The generated commands resolve the active context and pick the auth mechanism per action, hidden from the user: `console-api` contexts route tx steps through the Console API (provider send-manifest steps are skipped with a note — Console submits manifests internally; missing key → clear guidance to set `AKT_CONSOLE_API_KEY`, store a per-context key, or switch context), `keyring` contexts sign and broadcast locally via the chain adapters (tx flags merged onto workflow commands; missing wallet/chain client → clear error). Per-step results render in pretty mode; `-o jsonl` emits one SPEC §2.3.8 line per step; steps are recorded in the per-context action log. Full httptest-backed console deploy e2e test (create → bids → cheapest → lease from cached manifest). 17 new tests across adapters/steps/CLI.

- **`akt console` command group (AKT-648)**: New top-level command group porting the managed-Console CLI surface to Go per the console-axi reference: `login/logout/whoami` (login validates against `/v1/user/me` before storing the per-context credential; TTY prompt hides input), `deployment list/get/create/update/close/deposit/settings`, `bid list`, `lease create` (defaults to the per-context manifest cache), `wallet list/balance/settings/cost`, `usage`, public keyless `provider list/get/regions/auditors`, `gpu`, `template list/get/sdl` (raw SDL to stdout for piping), `apikey list/create/delete` (secret shown once), and `jwt create`. Positional-primary args per AKT-650 conventions; idempotent close; USD `$X.XX` formatting; mutations recorded in the action log. SPEC gains §2.9 "Console Commands". 8 command tests.

- **Console API client reworked to match the real API (AKT-648)**: The client's request shapes were wrong — the Console API wraps write bodies/responses in a `{"data": ...}` envelope and `deposit-deployment` takes `{data:{dseq,deposit}}`, not `{dseq,amount}`. Rebuilt the client with correct wire shapes (verified against the console-axi reference CLI) and expanded it from 8 to ~30 endpoints across deployments (incl. `/v2/deployment-settings` with PATCH→POST fallback and idempotent close via `ErrAlreadyClosed`), wallet/balances/usage, public marketplace (providers, regions, auditors, GPU prices, templates, bid screening), API keys, and provider-scoped JWT minting. Adds a per-context manifest cache at `contexts/<name>/manifests/<dseq>.json` (0600) so `deployment create` → `lease create` works without re-passing the manifest. µACT↔USD helpers (1 ACT = 1 USD, 1e6 micro). 36 tests.

- **Workflow execution adapters (AKT-647)**: New `internal/workflow/adapters` package implementing the engine's `steps.ChainClient`/`steps.ProviderClient` interfaces over the real chain client and provider gateway: msg builders for deployment create/update/close and market lease-create replicating the tx command logic (SDL groups + version hash, dseq from chain height, `auto` deposit via min-deposit params, update-group safety check), workflow query routing for `market.bids`/`market.leases`/`deployment.deployments` shaped for the wait-step's `until:` templates, and a provider adapter that resolves gateway URLs from on-chain HostURI. dseq is deliberately emitted as a JSON string in step outputs so Go templates don't render it in scientific notation. 22 tests.

- **State keywords as positional filter arguments (AKT-650)**: `akt query deployment active`, `akt query market order|bid|lease <state>` now work — a bare state keyword as the sole filter argument is equivalent to `--state`, with vocabularies derived from the on-chain enums (deployment `active|closed`; order `open|active|closed`; bid `open|active|lost|closed` — "matched" never existed in v1beta5, two stale flag help strings corrected; lease `active|insufficient_funds|closed`). Keywords don't combine with identity paths (clear error); a positional state wins over `--state`. SPEC §3.8.2 detection table + §3.8.6 examples updated. 18 new/extended filter test cases.

- **Positional dseq on tx deployment and provider lease commands (AKT-650)**: `tx deployment close [dseq]`, `update <sdl> [dseq]`, `group close|pause|start [dseq] [gseq]`, and `provider lease-status|lease-logs|lease-events|get-manifest [dseq]` accept the sequence positionally; the flag remains as an override and the positional wins when both are given. `lease-shell` is excluded (its positionals are the remote command). New pure helpers `DSeqFromArgs`/`GroupSeqsFromArgs` in `internal/cli/chain/flags/args.go` + 11 tests; SPEC §2.1/§2.4 updated.

- **Console API key as a per-context credential (AKT-646)**: Contexts gain `auth-method` (`keyring`|`console-api`, validated) and `console-api-url` config fields, and a per-context Console API key stored at `contexts/<name>/console-api-key` (0600, never in config.yaml). `Manager.Resolve` now populates `AuthMethod` (default keyring), `ConsoleAPIURL` (default `https://console-api.akash.network`), and `ConsoleAPIKey` resolved env-over-file per SPEC §7.1. New `internal/context/credentials.go` (Set/Stored/Resolve helpers; empty key removes the file). `akt context create/edit` gain `--auth-method`, `--console-api-url`, `--console-api-key` (edit with empty string removes the key; action log records only "updated"/"removed", never the value). `akt context show` displays the auth method and Console URL and reports the key only as configured/not set. 7 new tests + updated goldens.

- **Dynamic workflow command surfacing pinned by tests + spec (AKT-651)**: The workflow-generated command surface (commands exist iff their workflow YAML resolves; built-ins `deploy`/`update`/`close` always resolve per SPEC §2.3.9) was implemented earlier but had no unit tests and no explicit spec wording. Added 6 tests in `internal/cli/workflow/commands_test.go` covering: built-ins-only surface, user workflow appearing/absent (negative case), malformed YAML skipped, generated Use/flags for close and deploy, and user-workflow override of a built-in. SPEC §2.3.1 gains a "Dynamic surfacing" paragraph making the iff-rule normative.

- **Console API mutations recorded in the action log (AKT-211)**: New `console` action type (SPEC §5.2/§5.6 amended). The Console client's state-changing calls (create/update/close deployment, create lease, deposit) record `type=console` entries with operation, dseq, and success/failure via an optional `WithActionLog` hook — read-only Console queries are not recorded. 2 tests.

- **Workflow step action-log adapter (AKT-211)**: New `workflow.ActionLogAdapter` implements the engine's `Logger` hook, writing one `type=workflow` entry per completed step (workflow name, run ID, step index/name, status, error, tx hash/height) per SPEC §5.6. The engine already invoked `Logger.LogStep` after every step; this provides the previously missing implementation. Wired into workflow execution when the engine is constructed. 2 tests.

- **Provider gateway operations recorded in the action log (AKT-211)**: The state-changing provider commands (`send-manifest`, `migrate-hostnames`, `migrate-endpoints`, `lease-shell`) now write `type=provider` entries per SPEC §5.6 with the operation name, provider address, dseq, and success/failure status. Read-only provider queries (status, lease-status, lease-logs, lease-events, get-manifest) are not recorded, consistent with the action log's mutating-actions scope. New helper `recordProviderAction` in `internal/cli/provider/actionlog.go` + 2 tests.

- **Chain transactions recorded in the action log (AKT-211)**: All `tx *` commands now write `type=tx` entries per SPEC §5.6 through a single choke point — `TxPersistentPreRunE` wraps the discovered chain client in a logging decorator (`internal/cli/chain/actionlog.go`) whose `Tx()` intercepts `BroadcastMsgs`/`BroadcastTx`. Entries record the compact msg type (e.g. `deployment.MsgCloseDeployment`), tx hash, height, gas used, result code, signer account, and — for well-known Akash messages — dseq/gseq/oseq/provider. Failures record `status=failed` with the error; generate-only/simulate/offline runs are not logged. 4 new tests.

- **Context operations recorded in the action log (AKT-211)**: `context create/use/edit/delete/rename` now write `type=context` entries per SPEC §5.6. Entries are written to the affected context's own log (opened directly, since these commands often run without a logger for the target context); `delete` records into the current context's log because the deleted context's log is removed with it. New helper `recordContextAction` in `internal/cli/context/actionlog.go`; logging is best-effort and never fails the command. 5 new tests in `internal/cli/context/actionlog_test.go`.

- **Phase 5 TUI Mode completion (T064-T105)**: Completed all 20 remaining Phase 5 (User Story 3) tasks. Tests added: navigation behavioral tests (T064) — view switching via number keys, Esc/back, command palette routing, overlay priority, ViewDataRefreshMsg dispatch; command palette tests (T067) — open/close, filter, cursor wrapping, submit message; ListView/DetailView tests (T068) — items, cursor, selection, scrolling, rendering; dashboard tests (T092) — rendering, shortcuts, account/deployment data; E2E TUI tests (T105) — version command, no-TTY safety, context list, completion. Features implemented: ResourceTable sorting and filtering (T086) — Sort() by column, SetFilter() case-insensitive substring search, ClearFilter(), FilteredCount(); deployments view (T093) — escrow balance column, filter cycling (f key), update placeholder (u key); leases view (T094) — GSeq/OSeq columns, Enter detail, logs (l key), filter cycling (f key); providers view (T095) — populated chain attribute data (region, audit); log viewer (T096) — service filter cycling via CycleServiceFilter() and s key; governance/staking action wiring — vote (v key) and delegate (d key) open confirm dialog. Verified already-complete: T065 (theme tests), T066 (keybinding tests), T070 (provider cache tests), T071-T081 (11 design docs), T089 (confirm dialog). Live sync integration (T104) — sync bridge event pipeline fully wired (events → engine → store → TUI refresh within 2s, sync status indicator in header).

- **Sync bridge status indicator on dashboard (T12)**: Added a "sync" row to the Network panel on the TUI dashboard showing "Live" (green) when the sync bridge is active or "Offline" (muted) when it is not. New `syncActive` field and `SetSyncBridgeActive()` method on `Dashboard` struct. Wired from `app.go` `Run()` and `RunMonitor()` after successful sync bridge creation.


- **LightClient for TUI chain queries (T4-6)**: Added chain-sdk `LightClient` to the TUI for querying governance proposals, staking validators, and on-chain providers. New `LightClient` field on `Config` and `App` structs, built automatically in `Run()`/`RunMonitor()` from the resolved context and RPC endpoint (nil keyring — query-only mode). Three new message types (`ProposalsLoadedMsg`, `ValidatorsLoadedMsg`, `ProvidersLoadedMsg`) and corresponding `tea.Cmd` factories (`loadProposals`, `loadValidators`, `loadChainProviders`). Data is fetched on view switch (keys 3/5/6 and command palette equivalents) and routed to `SetData()` methods on `GovernanceView`, `StakingView`, and `ProvidersView`. Helper functions added for formatting proposal status, voting end times, token amounts, commission rates, and provider attributes.

- **Phase 4 offline E2E tests (T062)**: Added 6 E2E tests to `e2e/cli_test.go` exercising Phase 4 features: `TestStoreStatusEmpty` (verifies empty store shows "not synced" and zero records), `TestStoreExportImport` (exports empty store to file, imports into fresh context), `TestDeployHelp`/`TestUpdateHelp`/`TestCloseHelp` (verify workflow command help output contains expected keywords), `TestProviderHelp` (verifies provider subcommands listed). Store tests use a shared `setupContextHome` helper that creates a network and active context. All tests work offline with no chain connection.

- **Provider gateway client and CLI commands (T057, T058)**: Added `internal/provider/client.go` — thin wrapper around chain-sdk `rest.Client` with JWT/mTLS auth selection based on akt context config. Added `internal/cli/provider/commands.go` with 9 cobra commands: `status`, `lease-status`, `lease-logs` (--follow, --tail, --service), `lease-events`, `lease-shell`, `send-manifest`, `get-manifest`, `migrate-hostnames`, `migrate-endpoints`. Commands use the SDK client context for address/keyring resolution and the chain-sdk provider client for all gateway operations.

- **Workflow CLI commands (T054-T056)**: Added `akt deploy`, `akt update`, `akt close` as top-level cobra commands in `internal/cli/workflow/commands.go`. Each loads its corresponding embedded YAML workflow definition via the workflow Loader. The embedded definitions (`deploy.yaml`, `update.yaml`, `close.yaml`) are compiled into the binary via `//go:embed`. Commands validate the workflow, print execution plans, and accept all spec-defined flags. Full execution requires wiring the ChainClient/ProviderClient step executor implementations (noted as TODO).

- **Console API client (T042, T059)**: Added `internal/console/client.go` — HTTP client for the Akash Console Managed Wallet API (`console-api.akash.network`). Implements 8 endpoints: CreateDeployment, ListDeployments, GetDeployment, UpdateDeployment, CloseDeployment, FetchBids, CreateLease, Deposit. All requests include `x-api-key` header. Error mapping: 401→API key, 402→funds, 404→not found. Automatic retry with exponential backoff for 429/5xx (max 3 attempts). 6 tests using httptest mocks.

- **Interactive prompt UX design (T017)**: Added SPEC.md §3.9 "Interactive Prompt Patterns" with 6 subsections covering: prompt types (confirmation, single-select, multi-select, value input), rendering conventions (cursor indicators, checkbox glyphs, background highlights, hint lines, raw terminal mode, stderr output), TTY detection and non-interactive fallback behavior (per prompt type), fork-vs-edit-parent flow (shared network edit prompts with `--fork-network` and `--yes` overrides), account selection (keyring-based selection when no default account), and transaction confirmation (pre-broadcast summary with signer/chain/gas/fee). All prompts follow the `--yes` convention from §3.1.1 and render on stderr per §10.1.1.

- **Local deployment store (T038, T048, T049, T050)**: Implemented the full local deployment store per SPEC §4. New packages: `internal/store/` (Store interface, record types, filter types, key helpers) and `internal/store/bbolt/` (bbolt backend). The store tracks deployments, leases, and bids as JSON-encoded records in 5 named bbolt buckets (`deployments/`, `leases/`, `bids/`, `sync/`, `meta/`). Includes schema migration framework (versioned, forward-only, all migrations in single bbolt tx), YAML/JSON export/import with merge/replace modes, and comprehensive test suite (8 CRUD tests + 8 migration tests + 7 export/import tests = 23 tests total).

- **Sync engine (T040, T044, T047, T052, T053)**: Implemented chain sync engine at `internal/sync/`. The engine subscribes to the pubsub bus from `internal/events/` and routes 7 chain event types (deployment created/updated/closed, bid created/closed, lease created/closed) to store CRUD operations per SPEC §6.3. Filters events by tracked owner addresses. Includes startup reconciliation via `Querier` interface: full reconciliation on first launch or when block gap > 1000, with exponential backoff (1s→60s cap + jitter) per SPEC §6.5. Test suite: 11 sync engine tests + 3 bus tests.

- **Store CLI commands (T051)**: Added `akt store status/export/import` commands at `internal/cli/store/commands.go`. `status` displays store path, DB size, schema version, record counts, and sync state using the shared pretty output helpers. `export` writes YAML or JSON to stdout or file. `import` reads from file with merge (default) or replace mode.

### Changed

- **Phase 3 (US1) task verification and completion**: Verified and checked off 7 previously-implemented tasks that had stale TASKS.md checkboxes: T010 (filter tests — 535-line test suite in `internal/cli/chain/flags/filters_test.go`), T014 (tx formatter tests — 6 golden-file test cases), T033 (tx formatters — all 30+ message types registered), T034 (shell completion — bash/zsh/fish/powershell + dynamic context/network completion), T036 (bootstrap wizard — network multi-select + keyring backend selection), T037 (E2E tests — 6 offline tests in `e2e/cli_test.go`). Updated TASKS.md file paths to match actual implementation locations (e.g., `internal/filter/filter_test.go` → `internal/cli/chain/flags/filters_test.go`).

### Added

- **Transaction result pretty formatters (T033)**: Implemented all per-message `TxPrettyFormatter` registrations in `internal/output/pretty/tx_formatters.go` covering all tx modules: bank (2), deployment (6), market (5), provider (2), cert (2), audit (2), staking (6), distribution (5), gov (4), authz (3), feegrant (2), escrow (1), slashing (1), vesting (3), upgrade (2), crisis (1), wasm (9), oracle (1), bme (3). Each formatter extracts message fields and renders via `KV()`/`FormatCoin()`/`Bold()` per SPEC §10.11.5. Registration is explicit via `RegisterAllTxFormatters()` called from root command setup (no `init()`). Wired `pretty.PrintTxResult(cmd, cl.ClientContext(), resp)` into all 63 tx command call sites (replacing `cl.PrintMessage(resp)`) across 21 files. Changed `AddTxFlagsToCmd` output flag default from `"json"` to `OutputPretty` per SPEC §10.11.

- **Transaction formatter tests (T014)**: Created `internal/output/pretty/tx_test.go` with 6 golden-file test cases covering bank send, deployment create, staking delegate, gov vote, wasm execute formatters, plus a registration verification test. Uses the existing `charmbracelet/golden` snapshot pattern.

- **Shell completion command (T034)**: Added `akt completion bash/zsh/fish/powershell` subcommand in `internal/cli/root.go`. Uses cobra's built-in generation functions. Includes usage examples in the `Example` field. Exempted from `requiresContext()` since it generates static scripts.

- **Bootstrap wizard keyring backend selection (T036)**: Extended `internal/bootstrap/bootstrap.go` with a `selectKeyringBackend()` interactive single-select (os/file/test options, default: os) using the same raw-terminal pattern as the network multi-select. The selected backend is used in config instead of the hardcoded `"os"`.

- **Offline E2E test suite (T037)**: Created `e2e/cli_test.go` and `e2e/helpers_test.go` with 6 offline E2E tests exercising the compiled binary: version, help, completion generation, network template CRUD, full context lifecycle (create/list/show/rename/switch/delete), and unknown command error. Added `test` and `test-e2e` Makefile targets.

### Fixed

- **Bootstrap wizard `Defaults.Output` was `"table"` instead of `"pretty"`**: Changed to `"pretty"` to align with SPEC §1.2 and the `OutputPretty` constant.

- **SPEC.md §2.0 bootstrap description missing keyring backend detail**: Clarified that the wizard prompts for keyring backend selection (`os`, `file`, or `test`; default: `os`).

- **`registry.go` comment suggested `init()` for tx formatters**: Updated to document that query formatters use `init()` while tx formatters use explicit `RegisterAllTxFormatters()`.

- **`akt update` and `akt close` workflow commands specified**: Added `update <sdl-file>` and `close` to the SPEC.md §2.1 command tree as top-level workflow commands alongside `deploy`. Added full command specifications (flags, examples, JSONL output) after the `akt deploy` section. Expanded §2.3.8 with complete YAML workflow definitions for both `update` (update-deployment tx + send-manifest with retry) and `close` (close-deployment tx). Added clarifying note that these workflows wrap the same on-chain transactions as `akt tx deployment update/close` but add orchestration and unified output modes. Updated DESIGN.md §7.1 mapping table with `akt update` and `akt close` rows.

- **Structured error handling framework (`internal/cliutil/`)**: Implemented `CLIError` type per SPEC §11.1 with three-part error messages (what happened, context, suggestion), structured exit codes per SPEC §11.2 (0=success, 1=general, 2=usage, 3=config, 4=connection, 5=transaction, 6=auth, 7=store, 127=plugin-not-found), and convenience constructors (`ErrUsage`, `ErrConfig`, `ErrConnection`, `ErrTransaction`, `ErrAuth`). `ExitCode(err)` extracts the code from any error via `errors.As`. Wired into `cmd/akt/main.go` to use `cli.ExitCode(err)` instead of hardcoded `os.Exit(1)`. The `internal/cliutil/` package is a cycle-free leaf package importable by both `internal/cli` and `internal/cli/chain`; re-exported via type aliases in `internal/cli/` for backward compatibility.

- **stderr stream separation and progress helpers (`internal/cliutil/status.go`)**: Implemented `Status()`, `Statusf()`, `Verbose()`, `Verbosef()`, `Debug()`, `Debugf()` per SPEC §10.1.1 and §3.2. All write to `cmd.ErrOrStderr()`. `Status` is suppressed when `--quiet` is set or stdout is not a TTY. `Verbose` requires `-v`, `Debug` requires `-vv`. TTY detection uses `golang.org/x/term`. Verbosity helpers (`Verbosity()`, `IsQuiet()`, `IsVerbose()`, `IsDebug()`) moved from `internal/cli/verbosity.go` to `internal/cliutil/verbosity.go` (re-exported in `internal/cli/` for compatibility).

- **TxPrettyFormatter system (`internal/output/pretty/`)**: Implemented the `TxPrettyFormatter` interface, `TxPrettyFormatterFunc` adapter, `RegisterTx()`, and `LookupTx()` per SPEC §10.11.3 in the existing `registry.go`. Implemented `PrintTxResult()` dispatch function per SPEC §10.11.4 in new `tx.go`: reads `--output` flag; for `json`/`yaml` delegates to `cctx.PrintProto()`; for `pretty` (default) renders the two-section layout — Section 1 (common transaction summary: hash, signer, height, gas used/wanted, fee, status) and Section 2 (per-message detail via registered `TxPrettyFormatter` or highlighted JSON fallback). Multi-message transactions render numbered sub-sections. Signer extracted from `message.sender` event attribute; fee extracted via `sdk.FeeTx` codec unpacking. No per-module tx formatters registered yet — all messages fall back to highlighted JSON until formatters are added.

- **Smart type detection for query filter arguments (SPEC §3.8)**: Enhanced all `*FromArg` filter parsing functions in `internal/cli/chain/flags/` to support smart type detection: when the first `/`-separated component is a bech32 address it is used as the owner/provider; when it is a number it is treated as dseq and the owner defaults to the context's `default-account`. This enables `akt query deployment 12345` (owner from context) in addition to the existing `akt query deployment akash1.../12345`. All filter functions now accept a `defaultOwner` parameter. Query commands (`deployment_query.go`, `market_query.go`) pass `cl.ClientContext().GetFromAddress().String()` as the default. When no positional arg is given and no `--owner` flag is set, the owner also defaults to the context's default account — matching the SPEC §3.8 "flag-minimal operation" goal.

- **`--by provider` mode for bid/lease queries (SPEC §3.7)**: Added `--by` flag (values: `owner`, `provider`) to `akt query market bid` and `akt query market lease`. When `--by provider` is set, the leading address in the filter argument is treated as the provider and the trailing address as the owner, reversing the default hierarchy. `BidFiltersFromArg` and `LeaseFiltersFromArg` now accept a `byProvider bool` parameter.

- **Action log wired into CLI session**: The root command's `PersistentPreRunE` now opens the action logger for the current context and stores it in the command context via `cliutil.WithActionLog()`. Any command can retrieve it with `cliutil.ActionLogFromContext(cmd.Context())`. The logger is available for tx/query commands to record actions; individual commands will be wired incrementally.

### Fixed

- **`DefaultConfig().Output` was `"table"` instead of `"pretty"`**: The `DefaultConfig()` function in `internal/context/defaults.go` returned `Output: "table"` but the spec (§1.2) and the root command flag default both specify `"pretty"`. There is no `table` output mode. Changed to `"pretty"`.

- **`akt version` used `Run:` instead of `RunE:`**: The version command was the only leaf command using `Run:` (non-error-returning). Changed to `RunE:` with `cmd.OutOrStdout()` for consistency with all other commands.

- **Missing `Example` fields on all akt-native commands**: Added cobra `Example` fields to 22 akt-native commands: `context create/use/list/show/edit/delete/rename/log`, `context network create/edit/delete/list/show`, `context keys add/delete/list/show/export/import/rename/mnemonic/parse`, `monitor` (and all 4 subcommands), `mcp`, and `version`. The chain-ported commands (tx/query) already had examples; the akt-native commands — which are the most novel to users — had none.

### Changed

- **Verbosity flags redesigned as counted `-v` + `-q`**: Replaced the three separate boolean flags (`--verbose`/`-v`, `--quiet`/`-q`, `--debug`/`-d`) with a single counted `-v` flag and a `--quiet`/`-q` boolean. `-v` (level 1) shows operational detail (gas estimates, endpoint selection, config resolution); `-vv` (level 2) adds debug diagnostics (RPC dumps, stack traces). Default (no flag) shows progress/status. `--quiet`/`-q` suppresses all informational output (errors only). `-q` and `-v` are mutually exclusive. Added `Verbosity()`, `IsQuiet()`, `IsVerbose()`, `IsDebug()` helpers in `internal/cli/verbosity.go`. Updated SPEC.md §3.1.

- **Fixed `--output` flag default**: Changed root command `--output` flag default from `"table"` to `"pretty"` and updated description to `"pretty, json, yaml"`. There is no `table` output mode.

- **Removed stale `--skip-font-check` and `--glyph-mode` flags**: Removed the deprecated `--skip-font-check` and `--glyph-mode` persistent flags from the root command and their Viper bindings. Simplified `initGlyphs()` to read only from `defaults.glyph-mode` config key. The glyph mode flags were already removed from the spec (see earlier changelog entry) but the flag registrations persisted in code.

- **Removed `--glyph-mode` flag and Nerd Font mode**: Eliminated the `--glyph-mode` flag, `AKT_GLYPH_MODE` env var, and `defaults.glyph-mode` config key entirely. The CLI now uses ASCII-safe glyphs exclusively with no Nerd Font PUA-range output. The glyph registry (`internal/glyphs/`) is retained with semantic names but only ASCII variants. Removed the deprecated `--skip-font-check` flag. Updated SPEC.md §1.10, §3.1, §1.9 env var table, and config schema. Updated DESIGN.md §3.2.

- **Spec audit fixes (3 contradictions resolved)**:
  1. Fixed `defaults.output` in config schema (§1.2) from `table` to `pretty`. There is no `table` output mode — the two modes are `pretty` (human, default) and `json`/`yaml` (machine).
  2. Added `--interactive`/`-i` flag to the global flags table (§3.1). The flag was already implemented in code and referenced in DESIGN.md §3.2 but was missing from the spec's authoritative flag table.
  3. Added resource filter argument parsing (`internal/filter/`) as Phase 1 deliverable 1.10a (§12). Filter syntax was fully specified in §3.8 but not listed as a Phase 1 deliverable, despite query commands (which depend on it) being Phase 1.

### Added

- **CLI UX principles codified in DESIGN.md and SPEC.md**: Audited DESIGN.md and SPEC.md against CLI design best practices (familiarity, discoverability, feedback, clarity, flow, forgiveness) and addressed 7 conformance gaps:

  1. **`--verbose`/`-v` and `--quiet`/`-q` global flags** (SPEC.md §3.1): Added `--verbose` for operational detail (gas simulation, endpoint failover, config resolution) and `--quiet` for suppressing all informational output. `--debug` now implies `--verbose`. `--quiet` and `--verbose` are mutually exclusive.

  2. **stdout/stderr stream separation** (SPEC.md §10.1.1, §11.3): Added explicit rule that data output goes to stdout and everything else (errors, warnings, progress, verbose/debug logging) goes to stderr. Includes examples showing correct piping behavior.

  3. **CLI-mode tx feedback** (SPEC.md §3.2): Specified that `tx` commands show progress status on stderr during multi-second operations (gas simulation, broadcast, confirmation wait) when a TTY is attached. Suppressed by `--quiet` or non-TTY. Enhanced by `--verbose`.

  4. **Typo suggestions** (SPEC.md §11.4): Added requirement for "Did you mean?" suggestions using cobra's built-in Levenshtein distance matching (threshold 2) for unknown commands and subcommands.

  5. **Mandatory `--help` examples** (SPEC.md §2): Added rule that every command must populate cobra's `Example` field with at least one usage example showing the most common use case.

  6. **`--force` vs `--yes` convention** (SPEC.md §3.1.1): Documented the distinction: `--yes` skips confirmation prompts (operation unchanged), `--force` overrides structural safety guards (operation may bypass checks). Commands must not use `--force` as a synonym for `--yes`.

  7. **CLI UX principles summary** (DESIGN.md §3.2): Added a six-principle summary (familiarity, discoverability, feedback, clarity, flow, forgiveness) with cross-reference to the detailed SPEC.md sections.

- **Unified theme system (`internal/ui/theme/`)**: Created a centralized 256-color theme package that serves as the single source of truth for all colors and styles across the entire akt UI. Defines 15 color constants (Primary, Accent, Success, Warning, Error, Text, BrightText, Muted, Dim, Border, Highlight, Cyan, Magenta, Blue) and 30+ reusable style definitions (Bold, Dim, Section, Key, Label, Value, Header, SectionHeader, Title, TabActive/Inactive, Highlight, VoteYes/No, GridVoted/NotVoted, Proposer, Moniker, DetailHeader/Label/Value, PercentHigh/Low, StatusBar, HelpBar, Progress colors). Uses 256-color values (16-255 range) for consistent rendering across terminals regardless of color scheme. Both `internal/output/pretty/helpers.go` (CLI) and `internal/monitor/ui/styles.go` (TUI) now import from the theme package instead of defining their own colors/styles, eliminating the previous split between ANSI 16-color (monitor) and ad-hoc color codes (pretty).

- **Shared format helpers for CLI/TUI parity**: Added `FormatNumber()` (comma-separated thousands), `FormatPower()` (compact: 1.5M, 2.5K), `FormatShortDuration()` (ms/s/m for block times), `FormatBytes()` (Ki/Mi/Gi/Ti binary units), `FormatMemoryRatio()` (avail/total with byte units), and `FormatResourceRatio()` (avail/total count) to `internal/output/pretty/helpers.go`. These replace duplicate functions that previously existed only in `internal/monitor/ui/view.go`. The monitor now imports these shared helpers, eliminating code duplication.

- **Render* functions for all pretty formatters**: Extended the Render* pattern (previously only in `bme.go` and `oracle.go`) to ALL pretty formatter modules. Each module now has public `Render*` functions that return styled strings, usable by both CLI pretty output and TUI views. The registered `PrettyFormatter` functions become thin wrappers: `fmt.Fprint(w, Render*(...))`. Modules converted: `deployment.go` (RenderDeploymentList, RenderDeploymentDetail, RenderGroupDetail, RenderGroupsList), `market.go` (RenderBidList, RenderBidDetail, RenderLeaseList, RenderLeaseDetail, RenderOrderList, RenderOrderDetail), `provider.go` (RenderProviderList, RenderProviderDetail), `staking.go` (RenderValidatorList, RenderValidatorDetail, RenderDelegationDetail, RenderDelegatorDelegations, RenderStakingPool), `gov.go` (RenderProposalList, RenderProposalDetail), `bank.go` (RenderCoinsTable, RenderBalance), `cert.go` (RenderCertificateList), `distribution.go` (RenderDelegationTotalRewards, RenderValidatorCommission), `escrow.go` (RenderEscrowAccounts, RenderEscrowPayments), `feegrant.go` (RenderFeeGrants), `wasm.go` (RenderWasmCodeList, RenderWasmContractsByCode, RenderWasmCodeInfo, RenderWasmContractInfo, RenderWasmContractHistory, RenderWasmPinnedCodes, RenderWasmContractsByCreator). `auth.go` intentionally skipped (requires InterfaceRegistry from client context).

- **Context and Network Render* functions**: Created `internal/output/pretty/context.go` with `RenderContextShow()` and `RenderContextList()`, and `internal/output/pretty/network.go` with `RenderNetworkShow()` and `RenderNetworkList()`. These use the shared pretty helpers (Section, KV, KVHeader, SubKV, Bold, Dim) for styled output.

- **Reusable TUI view components**: Created `internal/tui/views/listview.go` with generic `ListView` component (configurable title, columns, empty state, cursor navigation, scroll, ListSelectMsg on Enter) and `internal/tui/views/detailview.go` with `DetailView` component (scrollable pre-rendered content from Render* functions, back hint). These are the building blocks for all resource views in the TUI — Deployments, Leases, Providers, Governance, and Staking views each instantiate a configured ListView.

- **TUI navigation per design (1-6 primary views)**: Overhauled TUI navigation to match the design PDFs. Number keys 1-6 now navigate to primary views: 1=Deployments, 2=Leases, 3=Providers, 4=Monitor, 5=Governance, 6=Staking. Removed old `q` (query panel) and `t` (tx panel) keybindings and their stub views (`views/query.go`, `views/tx.go` deleted). Dashboard is the home view accessed via Esc/back. Updated `KeyMap` struct, `DefaultKeyMap()`, `KeyMapFromConfig()`, `App` struct (5 ListViews + DetailView replace old query/tx fields), `handleCommand()`, `renderStatusBar()`, `resize()`, and command palette registry. Updated all TUI golden test files.

### Changed

- **CLI context/network commands use pretty system**: Replaced 30+ lines of raw `fmt.Printf` in `cli/context/commands.go` `currentCmd` with a single call to `pretty.RenderContextShow()`. Replaced 20+ lines of raw `fmt.Printf` in `cli/network/commands.go` `showCmd` with `pretty.RenderNetworkShow()`. Output is now styled with the unified theme (section headers, color-coded keys, proper alignment).

- **Monitor UI uses shared theme and format helpers**: `internal/monitor/ui/styles.go` no longer defines its own 9 color variables and 25 style definitions. All styles are now aliases into `internal/ui/theme/theme.go`. Progress bar functions use `theme.ProgressPrimary`, `theme.ProgressSuccess`, `theme.ProgressPrecommit`. Duplicate format functions (`formatNumber`, `formatDuration`, `formatPower`, `formatBytes`, `formatMemoryRatio`, `formatResourceRatio`) removed from `internal/monitor/ui/view.go` and replaced with calls to `pretty.FormatNumber`, `pretty.FormatShortDuration`, `pretty.FormatPower`, `pretty.FormatBytes`, `pretty.FormatMemoryRatio`, `pretty.FormatResourceRatio`.

- **MCP server (`akt mcp`)**: Integrated MCP (Model Context Protocol) server into akt as a new `akt mcp` command. The server exposes 25 Akash Network tools over stdio transport for AI assistant integration (e.g., Claude Desktop). Uses chain-sdk's `v1beta3.LightClient`/`v1beta3.Client` directly — no custom client wrapper. By default, only 21 read-only query tools are registered (node status, balances, deployments, orders, bids, leases, providers, audited attributes, certificates). 4 write tools (close deployment, create lease, close lease, submit manifest) require explicit opt-in via `--enable-writes` flag to prevent AI agents from sending unapproved transactions. Config is resolved from the active akt context. Added `github.com/mark3labs/mcp-go` dependency. New packages: `internal/mcp/` (server), `internal/mcp/marshal/` (parameter helpers), `internal/mcp/tools/{node,bank,deployment,market,provider,audit,cert}/` (tool definitions). Updated DESIGN.md §4 package structure and SPEC.md §2.1 command tree + §2.8 MCP command documentation.

### Changed

- **RPC endpoint port inference from scheme**: Endpoint URLs no longer require an explicit port. When the port is omitted, it is inferred from the URL scheme (`http`/`ws`/`tcp` → 80, `https`/`wss` → 443). The underlying cosmos-sdk and CometBFT libraries require an explicit `host:port` for dialing, so endpoints are normalized at every entry point: chain-sdk `NewClient` and `newParsedURL` (covers the RPC dialer, CometBFT HTTP client, and JSON-RPC client), chain-sdk `queryClientInfo` (covers the discovery fallback path), akt `BuildClientContext`/`InitClientContext` (covers context-resolved endpoints stored in `cctx.NodeURI`), and akt `monitorRunE` (covers TUI/monitor CometBFT and WebSocket clients). Added exported `NormalizeEndpoint()` to `chain-sdk/go/node/client/url.go`. Updated SPEC.md §1.3 to document port inference.

### Added

- **Glyph mode system (`internal/glyphs/`)**: Added a centralized glyph registry that defines all Nerd Font (PUA range) glyphs with ASCII fallbacks. The app now works without Nerd Fonts installed — defaults to ASCII-safe glyphs (`[x]`/`[ ]` for checkboxes, `>`/`+`/`-`/`*`/`o` for indicators). Three modes are supported via `--glyph-mode` flag, `AKT_GLYPH_MODE` env var, or `defaults.glyph-mode` config: `auto` (default, uses ASCII — font detection disabled), `nerd` (opt-in Nerd Font glyphs), `ascii` (force ASCII). Font detection (DSR terminal probe) is disabled because it put the terminal in raw mode and leaked goroutines that consumed keystrokes, breaking interactive input. Deleted `internal/glyphs/probe.go` and `internal/terminal/nerdfonts.go`. The `--skip-font-check` flag defaults to `true` (no-op). All 23 inline PUA glyph literals across `internal/bootstrap/bootstrap.go`, `internal/monitor/ui/view.go`, and `internal/monitor/ui/styles.go` now reference the registry. Updated DESIGN.md §3.2, SPEC.md §1.10, §3.1, and the config schema/env var tables.

### Fixed

- **Pretty output alignment (`internal/output/pretty/helpers.go`)**: Replaced the custom `displayWidth()` function (which used a regex to strip only SGR escape sequences and counted runes) with `lipgloss.Width()` which handles all ANSI sequence types (SGR, CSI, OSC) and uses `go-runewidth` for proper Unicode column-width measurement. This fixes cascading indentation in KV-formatted output (e.g., `akt q bme status`) caused by unstripped ANSI sequences inflating the measured width, leading `padRight` to produce incorrect padding.

### Added

- **Pretty output for all module params queries**: Registered `PrettyFormatter`s for all 10 module params types (staking, governance, minting, slashing, distribution, auth, deployment, market, wasm, oracle). CLI commands (`akt query <module> params`) now render human-readable key-value output instead of falling back to raw JSON when `--output pretty` is active. Each module has a public `Render<Module>Params()` function in `internal/output/pretty/params.go` for Pretty/TUI visual parity (SPEC §10.8). Added `FormatDuration()`, `FormatDurationString()`, `FormatPercent()`, `FormatPercentDec()`, and `FormatBool()` helpers to `helpers.go`. Updated the TUI governance tab (`akt monitor` > Network > Governance) to call `pretty.RenderModuleParamsFromJSON()` instead of rendering raw `json.MarshalIndent` output, producing the same KV layout as the CLI. Added `RenderModuleParamsFromJSON()` bridge function that unmarshals REST JSON for all 12 modules (including bank, transfer, ibc, crisis which are TUI-only) and delegates to the shared formatting helpers. Updated SPEC.md §10.10 with per-module params formatter field tables and §8.3.11 with a pretty-output governance tab wireframe.

### Changed

- **BME status pretty output redesign**: Renamed the `active` status label to `Healthy` in BME query and monitor output. Reordered fields to: Status, Mints (Allowed/Halted), Refunds (Allowed/Halted), Collateral Ratio, Thresholds (nested Warn/Halt). Status labels are now capitalized human-readable strings (Healthy, Warning, Halt CR, Halt Oracle) instead of raw enum values. Mints/Refunds changed from "yes/no" to "Allowed/Halted". Thresholds grouped under a header with indented sub-keys via new `KVHeader`/`SubKV` helpers. Updated SPEC.md monitor dashboard layout and BME status section description. Shared `RenderBMEStatus` ensures visual parity between `akt q bme status` and `akt monitor` dashboard.

- **Oracle/BME monitor two-column layout**: Split the Oracle/BME monitor dashboard into two side-by-side panels: Oracle on the left, BME on the right. Extracted `renderOraclePanel` and `renderBMEPanel` from the former `renderOracleBMEDashboard`, which now computes half-widths and joins them horizontally via `lipgloss.JoinHorizontal(lipgloss.Top, ...)`. Each panel is constrained to its column width with `lipgloss.NewStyle().Width()`. The shared `pretty.Render*` functions remain unchanged — the two-column layout is a monitor-only presentation concern. Updated SPEC.md §8.3.12 ASCII art to show the two-column layout with a `┬`/`┴` vertical divider.

### Added

- **Shared blockchain event service (`internal/events/`)**: Copied the events service from `chain-sdk/go/util/events` and removed the event-type whitelist so that all successfully parsed ABCI events are published to the pubsub bus. Subscribers filter by type. The bus is created once in `buildMonitorModel` alongside a CometBFT HTTP client, and passed to the monitor model. First consumer: the Oracle dashboard subscribes to `EventPriceData`, `EventAggregatedPrice`, `EventPriceStaleWarning`, `EventPriceStaled`, and `EventPriceRecovered` events from the `oracle/v2` module, displaying live aggregated prices and a scrolling event log. The bus is available for all future consumers (sync engine, BME dashboard, provider dashboard, etc.).

- **Pretty/TUI visual parity rule and shared renderers**: Established the requirement that CLI pretty output and TUI/monitor dashboard rendering of the same data must be visually identical. Extracted `RenderBMEStatus`, `RenderBMEVaultState`, `RenderBMELedger` from `internal/output/pretty/bme.go` and `RenderAggregatedPrice`, `RenderOraclePrices` from `internal/output/pretty/oracle.go` as exported string-returning functions. The CLI formatters become thin wrappers that call these and write to `io.Writer`. The monitor's `renderOracleBMEDashboard` now calls `pretty.RenderBMEStatus` and `pretty.RenderBMELedger` directly — no duplicate formatting logic. Removed `bmeStatusLabel`, `boolYesNo`, `trimDec`, `greenStyle`/`yellowStyle`/`redStyle` from the TUI. Added §10.8 to SPEC.md and a code convention to AGENTS.md documenting the rule.

- **Oracle dashboard initial state from REST**: On startup, the Oracle/BME dashboard fetches current oracle state via REST (`/akash/oracle/v2/prices`, `/akash/oracle/v2/aggregated-price/{denom}`) so that data is displayed immediately without waiting for new block events. A periodic refresh tick (30s) keeps REST data fresh as a baseline; real-time bus events overlay on top. Added `GetOracleState()` and `restGet()` helper to `internal/monitor/rpc/client.go`, plus `oracleStateMsg`, `fetchOracleState`, `handleOracleStateMsg`, and `oracleSyncTick` to the monitor model.

### Changed

- **Rework `akt top` into `akt monitor` hub**: Replaced the single `akt top` command with `akt monitor` -- a hub-based real-time monitoring tool with three dashboards: Network (consensus, validators, governance), Provider (fleet health, versions, resources), and Oracle/BME (oracle prices, BME vault state, mint status, ledger). Oracle and BME share a single dashboard since BME depends on oracle prices and neither module is on mainnet yet. Subcommands `akt monitor network`, `akt monitor provider`, `akt monitor oracle`, and `akt monitor bme` launch directly into the respective dashboard (`oracle` and `bme` are aliases for the same Oracle/BME tab). Hub navigation uses Tab/Shift-Tab to cycle dashboards (Network → Provider → Oracle/BME); 1/2/3 switch sub-tabs within the Network dashboard. Standalone operation preserved (only requires RPC endpoint, no context/keyring). Package renamed from `internal/top/` to `internal/monitor/` with all import paths updated. Cache file migrated from `top.db` to `monitor.db` (with automatic fallback). Updated DESIGN.md section 1.4, SPEC.md section 2.6 (full rewrite), command tree, TUI views, command palette, keybindings, and phased implementation plan. Oracle and BME monitoring moved from Phase 4 to Phase 3 as part of the monitor hub.

### Fixed

- **Disable Nerd Font detection at startup**: The DSR-based font probe (`CheckNerdFont`) corrupted terminal state by leaking goroutines that blocked on `os.Stdin.Read()`. These leaked readers competed with bubbletea's input handling, causing all hotkeys (including ctrl+c) to be ignored in `akt top` and the root TUI. The terminal remained corrupted even after the process exited (garbled `ls` output). Removed all `CheckNerdFont()` calls from the root command, `topCmd`, and pretty printer startup paths. The `internal/terminal` package and `--skip-font-check` flag are retained for future reimplementation using a stdin-safe detection method.

- **Fix view overflow pushing title off-screen**: The `overviewOverhead` (10) and validators tab overhead (12) constants undercounted the actual non-row lines (title, tab bar, section header with border/padding/margin, column header, scroll indicator, help, and status bar). This caused more rows to be rendered than the terminal could display, pushing the title bar off the top of the screen. Corrected to 11 (overview) and 14 (validators).

- **Fix initial signing seed showing false misses**: `GetLatestCommit` fetched `/commit` for the latest block, which returns a non-canonical commit containing only the precommit signatures the node had locally collected (~70%). Validators whose signatures arrived after the node committed appeared as missed (red) in the first dot of the signing bar. Now fetches `/commit?height=H-1` for the previous block, which is guaranteed to have the complete signature set.

### Changed

- **Transaction result pretty output design**: Extended DESIGN.md (new section 5.7) and SPEC.md (new section 10.10, updated section 3.2) with the design for pretty-mode rendering of transaction results. Transaction results render in a two-section layout: Section 1 is a common transaction summary (hash, signer, block height, gas used/wanted, fee, success/failure status) identical for all transaction types; Section 2 is a message-specific detail section whose layout depends on the message type, rendered via a `TxPrettyFormatter` registry mirroring the existing query `PrettyFormatter` pattern. Single-message transactions show the detail section directly; multi-message transactions number each sub-section. Unregistered message types fall back to syntax-highlighted JSON. The `--output` flag for `tx` commands now defaults to `pretty` (consistent with query commands). SPEC.md includes per-module formatter specifications covering all tx message types across bank, deployment, market, provider, cert, audit, staking, distribution, gov, authz, feegrant, escrow, slashing, vesting, upgrade, crisis, wasm, oracle, and bme modules.

- **`_run/test/README.md`**: Added short reference of working commands for the isolated test environment -- context/network/key management, `akt top`, TUI mode, queries with positional filter syntax, transactions, and reset instructions.

- **README update**: Updated README.md to reflect latest design and spec changes. Overview paragraph now describes `akt top` as network state + governance (not providers), and highlights the flag-minimal operation and argument-driven filtering design goals. Consensus Monitor section rewritten with 3-tab description (Overview with dual progress bar, Validators, Governance), upgrade monitoring use case, and standalone operation note. Queries section rewritten with positional filter argument syntax (`akt query deployment 12345` instead of `--dseq 12345`), including `--by provider` and `owner/dseq` path examples. Consensus Monitor usage section expanded with positional endpoint argument, `--insecure`, `--clean-cache` flags, and tab keybindings. Output Formats examples updated to use filter syntax. Roadmap clarifies provider fleet monitoring is main TUI only, not `akt top`.

- **`akt top` Overview: half-block double progress bar**: Added a dual progress bar to the Overview tab, rendered between the "Block Progress" header and the block table. Uses the upper-half-block character (`▀` U+2580) where the top half represents prevote progress (green) and the bottom half represents precommit progress (cyan). Each cell's foreground/background colors encode both metrics simultaneously. Bar width stretches to fill the available terminal width. Percentages are displayed alongside, colored green when ≥ 2/3 consensus threshold and yellow when below. Added `DoubleProgressBar()` to `styles.go` and updated `overviewOverhead` from 11 to 13.

- **`akt top` design goals and provider tab removal**: Added DESIGN.md section 1.4 documenting the `top` command's purpose, goals (real-time network health monitoring, critical tool during network upgrades, standalone operation), and non-goals (provider monitoring, user-specific resources). Added SPEC.md section 2.6 with command specification (flags, endpoint resolution, 3-tab layout). Added `top` to the SPEC.md command tree (section 2.1). Marked SPEC.md section 7.3.10 (Provider Fleet Monitor) as main-TUI-only. Removed `TabProviders` from `akt top` — the Providers tab, provider check ticks, chain sync ticks, and cache save ticks are no longer started or dispatched in the top model. All provider monitoring code (`internal/top/rpc/`, `internal/top/cache/`, provider scanning, detail views) is retained intact for reuse by the main TUI and future dedicated provider tooling. Fixed duplicate status bar in standalone `akt top`: the top model now always defers to the parent App for status bar rendering (`Embedded: true`), eliminating the duplicated RPC endpoint and help text. `BackMsg` in standalone mode now quits instead of navigating to the dashboard. Updated cobra command descriptions to remove provider references.

- **Design goals: flag-minimal operation and argument-driven filtering**: Extended DESIGN.md section 1.1 with two new goals. (1) Flag-minimal operation: after initial context configuration, the majority of CLI operations require zero additional flags or environment variables. (2) Argument-driven filtering: query commands accept a `/`-separated positional filter argument (`[owner/]dseq/gseq/oseq/provider`) instead of flag-based identity filters (`--owner`, `--dseq`, etc.). For bids and leases, `--by provider` reverses the hierarchy to `provider/dseq/gseq/oseq/owner`. Updated SPEC.md command tree (section 2.1), flag sections 3.5–3.7, and added new section 3.8 documenting the resource filter argument with smart type detection, get-vs-list heuristic, per-command scope, and examples. Updated DESIGN.md section 7.1 query command mapping table accordingly.

- **Simplify DESIGN.md package structure**: Reduced section 4 "Package Structure" to list only folder paths with descriptions, removing individual file listings. Added `internal/filter/` package for resource filter argument parsing.

### Added

- **Validator selection and detail expansion (Validators tab)**: Added cursor-based navigation (j/k) to select individual validators, matching the block selection UX on the Overview tab. Pressing Enter expands a detail panel showing the validator's full address, consensus pubkey, voting power with percentage, current prevote/precommit status, signing history stats (signed/missed counts), and proposer status. Esc collapses the panel.

- **Seed initial signing history from latest commit**: On startup, `akt top` now queries the `/commit` RPC endpoint to fetch the latest block's validator signatures. This seeds `valSignHistory` so the first block in the Validators tab shows actual signing status instead of appearing as all-missed (red) due to the WebSocket subscription starting mid-block.

- **Command palette for default TUI mode**: Replaced the minimal vim-style `:` command input and the separate `Ctrl+P` search dialog with a unified command palette. Both `:` and `Ctrl+P` now open the same centered overlay containing a text input on top and a filtered command list below. The palette uses a registry-based command system (`internal/tui/commands/registry.go`) pre-populated with navigation, action, and application commands. Typing filters the list via case-insensitive substring matching against command names and aliases. `j`/`k` or arrow keys navigate the list, `Enter` selects, `Esc` closes. Removed `internal/tui/views/search.go` (superseded). Updated SPEC.md section 8.4 to document the merged command palette behavior.

- **Configurable TUI keybindings**: All TUI keybindings -- including command palette navigation -- are now resolved through a configurable `KeyMap`. Added `CursorUp`, `CursorDown`, and `Select` bindings to the global `KeyMap` struct. Added `KeyMapFromConfig(v *viper.Viper)` which starts from vim-style defaults and applies user overrides when `tui.keybindings: custom` is set in config (reads `tui.custom-keybindings.*` string slices per SPEC section 8.6). The palette receives its bindings via a `PaletteKeys` struct to avoid circular imports between `tui` and `views` packages. Status bar hints are now derived from the active keybindings rather than hardcoded strings. Updated `Run()`/`RunTop()` to accept `*viper.Viper` and updated `root.go` call site.

- **Embed real consensus monitor into default TUI**: The `akt` default TUI now renders the same live consensus/provider/governance monitor as `akt top` when the user navigates to the Consensus view (via command palette or `1` key). Previously the TUI showed a static placeholder (`internal/tui/views/top.go`, now deleted). Added `Embedded` mode to `topui.ModelConfig` and an exported `BackMsg` type: when `Embedded=true`, `q` sends `BackMsg` instead of `tea.Quit`, and `Esc` sends `BackMsg` when nothing is expanded, allowing the parent TUI to navigate back to the dashboard. `ctrl+c` still quits the entire application. Introduced `tui.Config` struct to thread RPC endpoint, cache directory, and TLS settings from `root.go` into the TUI. The top model's background goroutines (WebSocket, ticks, provider checks) run continuously once initialized, keeping state fresh even when the user is on other views.

- **Unified 3-line status bar**: All TUI views now share a 3-line status bar pinned to the bottom of the terminal. Line 1 shows view-specific keybinding hints (dynamic per active view/tab), line 2 shows connection info (RPC endpoint + WS status when on the consensus view), and line 3 shows global keybindings (command palette, back, quit). The embedded top model's own help/RPC status lines are suppressed via `ViewContext.Embedded` to avoid double chrome. The top model receives a height-adjusted `WindowSizeMsg` (`terminal height - 3`) so its content fits above the status bar. Added `StatusInfo()` method and `TabHelpText()` to `topui.Model` so the parent TUI can read RPC endpoint, WebSocket connection state, and tab-specific help text for the status bar.
