# AI Changelog

## Unreleased

### Changed

- **Console deployments are funded automatically; the deposit surface is gone
  from that rail (CON-890)**: Console abstracted escrow away: the platform
  funds every deployment from the account's credits, `POST /v1/deployments`
  documents `deposit` as deprecated and discards it, `/v1/deposit-deployment`
  is deprecated, and an explicit `autoTopUpEnabled: false` is rejected because
  always-on funding cannot be switched off. `akt` had not followed, and was
  wrong in two ways that cost the user something rather than merely misleading
  them. First, `akt deploy` could not run at all on a console-api context: the
  workflow's `deposit: "auto"` default was rejected with "the Console workflow
  rail requires an explicit deposit in USD", so the CLI demanded a value the
  API throws away and refused to deploy without it. Second,
  `akt console deployment settings <dseq> false` failed against production, and
  `deployment create` printed that exact command as the way to opt out of
  unattended spending.

  On the console rail the deposit is now removed rather than deprecated, since
  a path the API ignores is worse than no path: `akt console deployment deposit`
  and the `console_deposit` MCP tool are gone, `deployment create` takes only an
  SDL and no longer prints an auto-top-up notice, and any explicit deposit is
  refused at the transport boundary (`Deposit.RailValue`) with a message naming
  automatic funding and linking the docs. Rejecting locally is the difference
  between telling a user their number did nothing and letting them believe it
  did. `deployment settings` now sets `runtimeLimitHours` via
  `PATCH /v2/deployment-settings/{dseq}`, which is the only per-deployment
  funding control a client can still set and which `akt` could not set at all
  before; `none` clears it. `wallet balance` and `console_wallet_balance` report
  Available/Escrow/Total, matching the shipped Console vocabulary, and
  `wallet settings` is named Auto Recharge, the account-level card charge,
  distinct from the retired per-deployment toggle.

  Two things the removal would otherwise have broken quietly. The
  `uakt`-pricing preflight read its denomination from the resolved deposit, so
  with the console deposit always empty it stopped running; it now uses `uact`
  unconditionally on that rail. And the live-mutation e2e suite bounded an
  abandoned run's spend by disabling auto-top-up, which the API now refuses.
  It caps the runtime limit instead, so the deployment auto-closes and unused
  funds are returned.

  The chain rail is untouched: `--deposit`, `auto` resolution, and the `uakt`
  escape hatch all behave as before, though its USD rejection no longer points
  at a console-api context as the place USD deposits work. The vendored
  contract spec was re-vendored from the live Console API, and SPEC/DESIGN/README
  were updated first per the spec-first rule.

### Fixed

- **Dual-rail contexts now keep chain and Console operations available at the
  same time**: The context's existing `auth-method` selects only the preferred
  `deploy`/`update`/`close` rail, with `--deploy-via chain|console` as the clear
  editing form. Explicit transactions and opt-in MCP writes continue through
  the local keyring, while any stored Console credential can print its
  managed-wallet address and supply an omitted chain-query owner. Deployment
  planning now resolves the real deposit before checking every SDL price
  denomination, so dry-run cannot approve a transaction the chain will reject.
  The shared deploy command accepts the deposit positionally like the direct
  Console command, and chain execution rejects a missing signer before an
  empty owner can reach SDK message validation, with guidance for selecting a
  local account or switching the preferred rail.
  Successful first-run setup
  stops after its summary instead of dumping root help. A gated three-validator
  halt/restart scenario covers monitor reconnect, voting-power thresholds, and
  block resumption during upgrade-like coordination. (#90)

- **Release-range coverage no longer fails on an impossible Console deposit
  branch**: Console USD validation now returns the exact micro-ACT value used
  for escrow reconciliation. Deployment creation and deposit mutations share
  that single boundary instead of validating first and then retaining a second
  conversion error path that valid input could never reach. Exact conversion
  tests cover whole-dollar, one-decimal, and two-decimal deposits while the
  existing request-boundary tests continue to reject invalid amounts before
  HTTP work.

- **Six partially addressed backlog issues now have complete boundary
  contracts**: every successful Console leaf prints an action-specific `Next:`
  hint on stderr without contaminating structured stdout, while quiet mode
  suppresses it (#66); `context keys add` accepts Cosmos-compatible `--yes` /
  `-y` without weakening duplicate-key or secret handling (#71); provider
  status exposes a positive `--timeout` that configures the actual gateway
  request deadline (#73); already-closed or absent deployments fail before a
  close mutation and retain a failed action-log entry through direct and
  workflow paths (#75); API-key deletion proves the UUID exists before DELETE,
  preserving a missing target as failure even though the live endpoint returns
  idempotent success, and never prints `deleted: true` for an absent key (#79);
  and every rail accepts USD
  deposits only as plain decimal values with at most two fractional digits,
  rejecting exponent, non-finite, signed, and sub-cent forms before network
  work (#82). Structural and boundary tests pin each behavior.

- **Automatic transaction fees now use the selected network's configured
  policy**: First-run bootstrap stores each Akash network registry entry's
  `high_gas_price` instead of its inadequate average, and CLI initialization
  treats that stored price as the floor for gas-price-derived transactions.
  This fixes transactions whose gas limit was estimated correctly but whose
  `0.0025uakt` price produced `517uakt` while validators required `5169uakt` at
  `0.025uakt`. Higher invocation and environment prices remain unchanged,
  explicit fixed fees remain authoritative, and dry-run, generate-only,
  offline, and online construction all use the same deterministic policy. akt
  no longer trusts an individual RPC node's local minimum or retries a rejected
  broadcast with a guessed fee.

- **The open CLI correctness and UX backlog now shares enforceable
  boundaries**: workflow JSONL preserves step outputs and deployment results;
  deploy completion reports identity, URIs, readiness, and next actions;
  Console commands validate DSEQs, pagination, UUIDs, names, JWT lifetimes,
  active mutation state, and the $0.50 deposit minimum before transport work;
  dry-runs resolve the deposit and signer they will actually use; MCP query
  tools remain available when optional signing cannot initialize; and Console
  pretty output uses human semantic renderers with correct micro-ACT dollar
  and monthly cost semantics. Shell service selection, resource-based provider
  screening, state filtering, context creation confirmations, bootstrap
  warnings, keyring guidance, empty streams, and store/SDL documentation now
  follow the same command-boundary rules. This resolves the behavior tracked
  in issues #57 through #82 without masking failures with fallback values.

- **Console sandbox spend accounting now uses the owned deployment ledger**:
  the point-in-time account total has no lifecycle-run identity or observation
  height, so its pre/post change cannot prove one deployment's spend. The
  lifecycle now proves the exact pre-lease funded escrow, derives gross provider
  spend from the owned closed escrow's cumulative `uact` `transferred` value,
  requires zero terminal funds and positive paid-lease transfer, and keeps the
  signed account total change as secondary reconciliation.
  Credits and timing adjustments no longer cause false failures or reduce the
  independently measured gross spend, while missing, malformed, negative,
  regressing, wrong-denomination, or over-budget transferred state still fails
  closed.

- **Ledger prefix selection added on current main is now protected by the
  coverage gate**: the rebased keys command obtains the Bech32 account prefix
  from the Cosmos SDK configuration rather than passing a literal `cosmos`
  prefix to Ledger key registration. A command-level keyring boundary test
  proves the exact prefix supplied to `SaveLedgerKey` without requiring
  hardware, covering the statement that entered the active denominator after
  PR #84. Current counts remain generated report output rather than changelog
  policy.

- **Console deposits no longer report false failure while sandbox state is
  propagating**: The client now treats a successful deposit response as proof
  only when its deployment identity and exact `funds` plus `transferred` delta
  match the pre-submit snapshot. Missing, malformed, or stale acknowledgements
  fall back to independent GET observations for up to 30 seconds instead of
  five reads over 1.5 seconds. The non-idempotent POST is still issued exactly
  once, caller cancellation remains authoritative, and an unproved outcome is
  still recorded as pending. Regression tests cover fixed-scale balances, an
  exact response, six stale observations before convergence, the production
  timing policy, bounded cancellation, and no replay.
  Secret-safe live diagnostics now classify this local failure as
  `console_deposit_outcome_unknown` without exposing stderr.
  The generated active-union report includes the newly exercised path without
  turning that run's count into a checked-in coverage floor.

- **Current-main provider attestation code now satisfies the coverage gates**:
  semantic tests exercise gRPC authentication, TLS certificate trust,
  on-chain certificate decoding, attestation command failures, renderer
  fallbacks, and nonce-boundary cases added by the provider attestation
  feature. Self-signed gRPC certificates are now bound to the resolved provider
  address before their on-chain registration is trusted, so another provider's
  valid certificate cannot terminate a JWT-bearing connection. The Console log
  oracle's equivalent service/pod predicate also uses the lint-approved
  positive form. The generated reports include the larger denominator and its
  new coverage instead of treating a current-run TSV as an accepted floor.

- **The live Console log check now follows the provider stream contract**:
  sandbox providers report runtime pod names such as
  `web-5bfc685996-wv9vs`, while `web` is the requested SDL service filter. The
  lifecycle accepts the exact service or its hyphen-delimited pod name, still
  rejects unrelated services, and requires every record to carry a string
  `message` field. Individual blank container-log lines are valid; the complete
  bounded stream must still contain at least one substantive message from the
  real `akt` subprocess. Regression tests reject missing, null, non-string, and
  all-blank message streams without mistaking one valid blank line for a CLI
  failure. The shared production filter now rejects the incomplete runtime name
  `web-`, and malformed JSON or non-EOF websocket failures can no longer
  truncate a log stream while returning success. Normal WebSocket close frames
  remain successful even when they carry reason text, while an abnormal frame
  without reason text retains its close code and fails. Live-oracle source
  mismatch diagnostics no longer repeat provider-controlled record data.
  The stream reader is independently exercised so caller cancellation wins
  deterministically while record delivery is backpressured. Additional
  semantic boundary tests cover output failure, stdin EOF/error behavior,
  failed audit enrichment, and malformed provider attributes instead of
  weakening coverage requirements after removing the unsafe compatibility code.
  The generated active-union report exposes the remaining provider gap without
  treating that run's package counts as policy.

- **Console repeated-close E2E rejects false mutation success**: The client
  reads current deployment state before DELETE, so a terminal or absent
  deployment returns a non-zero already-closed error without a success
  document or another DELETE. The live lifecycle requires that failure,
  verifies its failed action-log entry, and independently confirms that the
  deployment remains terminal with no active lease. The same truthfulness rule
  now covers repeated deletion of an already-absent API key.

- **Console sandbox escrow verification accepts the chain's decimal coin
  encoding**: Production deposit reconciliation preserves fixed-point `funds`
  and `transferred` amounts exactly, while the independent live observer
  preserves the current `funds` used for its pre-lease proof. Both paths accept
  signed overdrawn `funds`, whole values written with 18 decimal places, and
  genuine fractional values; production still rejects negative cumulative
  transfers.
  The protected read step no longer invokes the optional existing-deployment
  diagnostic before a fixture exists; the managed-wallet lifecycle creates a
  deployment and lease, runs those shared query contracts, then closes and
  verifies the owned resource.

### Changed

- **Coverage gates now compare exact Codecov bases instead of checked-in
  statement snapshots**: main-target pull-request and main CI upload active-union,
  experimental-TUI, and tooling profiles for aggregate line comparison with
  each exact main-branch base. Each project status uses an automatic target,
  zero regression threshold, disabled removed-code leniency, and no pseudo-base
  fallback. Active code alone retains the required 100% Codecov patch status
  and the repository-owned gate that maps changed executable Go lines to exact
  statement or synthetic-edge counters.
  Generated aggregate and per-package TSVs remain current-run diagnostics, not
  numeric floors. The report job publishes full diagnostics, a profile-only
  main artifact, and a comparison artifact. Main uploads directly; pull-request
  CI performs no public tokenless upload. Instead, a
  trusted default-branch `workflow_run` validates the completed source run,
  two-parent merge identity, and generated run manifest before consuming only
  its three comparison profiles. Both checkout-free upload paths
  use OIDC, and the PR path uses only the validated PR number for its synthetic
  branch. Fixed-name artifacts explicitly replace their own prior attempt when
  a producing job is rerun, while partial reruns retain successful prerequisite
  artifacts under the same run ID. Strict-main status enforcement becomes active
  after this configuration lands on `main`, the three flags are seeded there,
  and the four external status contexts are added to the repository ruleset
  without bypass and bound to the integration ID observed on AKT's seeded
  statuses. An authorized bypass of a failing coverage result explicitly
  resets the dynamic base, so the repository configuration does not pretend it
  can preserve a ratchet across an administrative override.
  Until that bootstrap is complete, `required-ci` proves profile generation and
  the applicable main upload but not Codecov's asynchronous comparison
  conclusions. The
  default-branch follow-up cannot run for the pull request that first adds it;
  bootstrap therefore requires explicit core review, a main seed, and a later
  test pull request before the app-bound Codecov contexts become required. The
  comparison reports also reject every Go-cover-instrumentable classified
  source file that disappears, including a file omitted from an otherwise
  measured package. Declaration- or package-initializer-only files may remain
  absent because Go emits no positive statement block for them; changed
  initializers remain subject to the stricter fail-closed line-to-counter gate.
  The completeness check uses the pinned Go cover instrumenter rather than an
  approximate syntax classifier. Default-branch push-tip upload jobs are never
  canceled or displaced and wait for all required comparison uploads on the
  exact predecessor to be accepted,
  preserving the dynamic base chain during rapid pushes. Main-target
  pull-request uploads perform the same bounded readiness check against their
  exact main base; stacked pull requests retain the local gate but skip Codecov
  comparison uploads. Version-tag
  publication requires the same four successful Codecov
  conclusions on the exact tagged main commit, so removing the local snapshots
  does not weaken release coverage. The mutation-score baseline and ratchet
  remain separate and unchanged.

- **Chain CLI tests now use the chain-sdk argument-builder pattern without
  compiling test support into the release package**: the recurring offline
  transaction and signing arguments are assembled through a focused immutable
  `FlagsSet` builder in the already-classified chain test-support package. The
  builder follows `chain-sdk/go/cli.TestFlags` and registered flag constants,
  while positional inputs and literal public-interface assertions remain
  independent. This replaces the former 1,025-line release-compiled helper,
  whose broad API had only three AKT call sites, without adding the deprecated
  `pkg.akt.dev/go/cli` module as a dependency.

- **Console sandbox verification now gates trusted pull requests instead of
  running on a timer**: CI removes the daily and manual mutation paths, runs
  real reads plus the capped managed-wallet lifecycle for same-repository pull
  requests to `main`, and requires that result in `required-ci`. The
  `console-sandbox` environment limits access to pull-request merge refs and
  uses the exact `refs/pull/*/merge` policy. It requires
  `@akash-network/core` approval before releasing its three secrets; fork and
  Dependabot pull requests remain secretless, and non-environment copies of the
  Console secrets are prohibited. Sandbox jobs queue on one tenant, and every
  pull-request run disables automatic cancellation so a retarget event cannot
  interrupt an earlier sandbox cleanup. The live read harness now
  creates its context with `AKT_E2E_CONSOLE_API_URL`, rejects production-like
  endpoints before its first request, and has a subprocess routing regression
  that proves the configured endpoint receives the call. The lifecycle also
  validates the complete provider-status schema while the created deployment
  is live. Successful live reports remain downloadable artifacts; pull-request
  workflow code never receives Codecov OIDC or a reusable Codecov token. The
  specification also records that the checked-in concurrency policy is not the
  still-planned external tenant lock or orphan sweeper. Live subprocess
  diagnostics classify recognized Console HTTP status codes without printing
  stderr or response bodies, so an expired key reports `console_http_401`
  instead of an opaque process error. The harness rejects a first-party
  sandbox hostname whose Console environment does not match the key's public
  environment segment before making a request. Live catalog contracts now
  follow Console's current schema: template lists are unwrapped category
  arrays, while individual provider regions may legitimately have no provider
  membership; the mutation-capable lane still requires an aggregate provider.
  The credential boundary now fails closed to the two exact Akash sandbox API
  origins (plus loopback-only hermetic fixtures), never echoes rejected endpoint
  values, validates the raw key/origin pair before either lifecycle creates a
  client, and routes context creation through bounded redacted capture. Region
  identities are cross-checked against the independent attributes schema and
  memberships must be canonical Akash addresses. Empty template categories
  remain schema-valid while aggregate catalog health, unique category titles,
  and per-category template identity are enforced separately; cross-category
  classification of the same template remains valid. The lifecycle now runs
  deployment-dependent read contracts against its own active lease, so a clean
  tenant cannot turn them into skips; the standalone read test remains useful
  for ad hoc tenants. Conflicting nested HTTP markers are classified from the
  outermost diagnostic rather than numeric scan order. Loopback origins now
  require an explicit hermetic-only option unavailable to protected live paths,
  and CLI subprocesses strip all harness-only Console variables so rotating to
  a child key cannot leave an inherited parent credential outside leak scans.

- **The standalone monitor completion test now owns its input descriptor**:
  the test supplies a pipe while production retains the CLI-validated terminal.
  This prevents Bubble Tea from trying to register a Linux CI runner's
  non-interactive `/dev/null` input with epoll, which had failed the ordinary,
  race, and coverage test lanes after the first PR run.

- **Provider-version coverage no longer depends on randomized map order**:
  the comparison suite directly exercises release-versus-candidate ordering in
  both directions, instead of relying on `sort.Slice` to choose one direction
  while iterating a map-derived version set.

- **The generated coverage reports now reflect all three hermetic shards**:
  unit, offline-binary E2E, and one-validator localnet E2E merge into separate
  active, repository, experimental-TUI, and tooling diagnostics. The patch gate
  independently checks every changed executable active line. Counts and
  percentages remain outputs of each run instead of accepted changelog floors.

- **Monitor, provider-workflow, and Console tests now exercise their semantic
  state machines instead of only their constructors**: monitor tests drive
  cancellation, live oracle events, governance, BME refreshes, and chain-sync
  provider selection through `Model.Update`, raising its direct package result
  by 80 statements. Provider adapter tests use an in-memory chain service, real
  JWT keyring, and HTTP gateway to prove exact lease filtering, manifest fanout,
  cancellation, and error behavior, raising adapter coverage from 340/446 to
  392/446 statements. Console catalog and deployment-update tests assert exact
  public endpoints, response identity, keyless reads, credential use, SDL
  preservation, retry bounds, and validation before transport; focused
  coverage reaches 70/81 statements in `marketplace.go` and 162/194 in
  `deployment.go`.

- **Distribution, authz, feegrant, BME, mint, oracle, params, slashing, and
  evidence queries now enforce and test their RPC boundary contracts**:
  semantic command tests assert exact requests, pagination and filters,
  validation before transport, preserved transport causes, and checked output
  failures. Successful nil responses and missing required feegrant or evidence
  results now return malformed-node errors instead of panicking or reaching
  the protobuf renderer. Focused coverage across these nine query files rises
  from 134/438 to 502/502 statements.

- **BME, escrow, market, and deployment-group transactions now have semantic
  command coverage**: tests assert exact protobuf message types, complete
  identities, sequence numbers, amounts, prices, deposit sources, transaction
  output, and preserved broadcast/output failures across twelve handlers.
  Invalid input is proven to stop before broadcast. BME conversions now run
  their SDK message validation, while escrow deposits reject a zero deployment
  identifier and validate the complete deposit message. Focused file coverage
  rises from 21.05% to 100% for `bme_tx.go`, 27.59% to 100% for
  `escrow_tx.go`, 27.78% to 97.62% for `market_tx.go`, and 22.99% to 48.13%
  for `deployment_tx.go` while leaving its SDL-backed create/update paths to
  their integration harness.

- **Auth, staking, and Wasm queries now fail closed on malformed node
  responses**: every typed query rejects a nil successful response before
  rendering or dereferencing it, while account, delegation, and historical-info
  leaves also require their promised nested result. Semantic command tests
  assert exact identities and pagination, local validation before transport,
  preserved transport causes, checked output failures, and byte-for-byte Wasm
  downloads.

- **Governance commands now enforce their real boundary contracts**: query
  commands reject invalid voter and depositor addresses, parameter selectors,
  and pagination before any governance RPC, while proposal preflight failures
  retain their original cause for `errors.Is`. Semantic tests assert exact
  filtered and paginated requests, structured proposal/vote/deposit/tally and
  parameter results, dependency and writer failures, transaction messages, and
  generated nested Wasm governance messages. Unit coverage rises from 39/197
  to 165/200 statements in `gov_query.go` and from 269/713 to 385/713 in
  `gov_tx.go`.

- **Block queries now have RPC-observable command tests**: focused tests cover
  event searches, positional and flagged heights, latest-height lookup, hash
  lookup, not-found responses, malformed input, transport failures, and output
  failures. The block-results renderer now uses the shared structured-output
  boundary, removing its unreachable JSON-marshal branch, and every statement
  in the release block-query file runs in the focused profile.

- **Code-owner review now has an actual owner**: the repository adopts the
  Akash Go-project default `@akash-network/core` owner for the whole tree, so
  the existing ruleset can protect its workflows, coverage controls, policy,
  exceptions, and ownership file instead of evaluating an empty ownership map.

- **Codecov no longer advertises unavailable inline annotations**: the
  flag-scoped active patch status, pull-request report, and badge remain, while
  GitHub annotations are disabled because Codecov cannot produce them for a
  flagged status and is deprecating that integration path.

- **Wasm any-of upload permissions now fail closed at the SDK size boundary**:
  an address set larger than the upstream maximum returns a normal CLI
  validation error instead of entering the SDK helper's panic path, and gzip
  streams that contain non-Wasm bytes are rejected explicitly.

- **Chain flag coverage now exercises value-bearing boundaries**: focused tests
  cover complete and empty BME ledger filters, reject malformed owner
  addresses, accept documented provider bid-close reasons, and reject values
  outside the provider range. The focused profile raises the generated active
  union for `internal/cli/chain/flags` from 370/484 to 388/484 statements.

- **The coverage pass now verifies cryptographic and protocol semantics through
  the real binary**: offline E2E decodes and verifies exact bank transaction
  messages, signers, fees, gas, memo, signature bytes, BIP-39 checksum, and
  independently derived Akash addresses. MCP invalid-input E2E proves malformed
  calls reach neither RPC nor Console and append no audit row, while registry
  coverage exposes chain writes only when a usable keyring rail exists.

- **Oversized Wasm instantiate permission sets now fail as input errors instead
  of panicking**: the parser canonicalizes and de-duplicates full addresses,
  constructs the access configuration without the upstream panic-prone helper,
  and applies the chain's 50-address validation bound before message creation.

- **The real Console lane now crosses the managed-provider and credential
  boundaries**: the guarded sandbox lifecycle requires independently observed
  provider readiness and workload ingress, semantically validates bounded log
  and event streams, executes a deterministic non-interactive shell command,
  and proves read calls remain absent from the action log. It also creates,
  authenticates with, lists, revokes, and rejects re-revocation of a child API
  key while an independent HTTP observer corroborates tenant identity and the
  temporary home is scanned for parent and child secrets. Structured API-key
  deletion now emits a structured acknowledgement instead of plain text.

- **Audit logging now fails closed at both startup and storage boundaries**:
  a selected context cannot execute when its action log cannot be opened, one
  JSONL record cannot exceed the 10 MiB rotation budget, hostile oversized rows
  are bounded on read, and existing-but-unreadable rotated generations are no
  longer silently omitted. Version and monitor cache diagnostics also treat
  nil-error short writes as command failures.

- **Experimental TUI tests now exercise state identity and terminal behavior**:
  lease selection uses the full identity, provider addresses remain complete,
  batched logs populate service filters, embedded monitor sizing preserves the
  shell content area, and coin/validator/escrow values use canonical formatting
  without sparse-value panics. ANSI progress labels and flexible tables operate
  on display cells, and named dashboard rows retain their DSEQ. The deduplicated
  experimental slice rose from 41.57% to 59.29% without pretending live chain,
  provider-stream, or terminal-runtime gaps are unit covered.

- **First-run coverage now drives the real terminal state machine**: a
  pseudo-terminal test executes raw multi-select and single-select navigation,
  safe active-context choice, Console opt-in and secret entry, configuration
  persistence, credential permissions, stream separation, and terminal echo
  behavior. Bootstrap package coverage rises from 56.4% to 94.3%; the PTY
  helper is now an explicit test dependency.

- **Coverage evidence itself is adversarially validated**: tooling rejects
  orphan metadata, missing report profiles, overflowing totals, invalid counter
  coordinates, path-alias
  duplicates, mutable comparison refs, diff-header source spoofing, symlinked
  source escapes, and tab-unsafe manifest records. The generated tooling report
  remains the diagnostic for this package class. Sync reconciliation likewise
  rejects a repeated non-empty pagination key before another request can loop
  forever.

- **Coverage parsing now accepts Go's synthetic empty control-flow blocks**:
  zero-statement records are discarded from profile filtering, reports, and
  every denominator, while changed-source checks retain exact executed edge
  evidence separately. This prevents Go 1.26 zero-width counters from breaking
  fresh collection, falsely covering neighboring syntax, or making an executed
  changed `case`/`select` edge fail because no positive-statement block exists.

- **Selected Console mutation E2E runs now fail closed**: the managed-wallet
  lifecycle checks its explicit mutation opt-in before deciding whether to
  skip. Once selected, a missing API key or unsafe sandbox configuration is a
  test failure. The documented delivery boundary now also reflects the real
  structured logs, events, and deterministic shell assertions already in the
  lifecycle instead of listing them as unfinished.

- **Pretty-output codec failure coverage is vet-clean**: the test-only codec
  fixture now lives with the package's other test data, preserving malformed
  JSON and marshal-error assertions without making `go vet` misidentify the
  Cosmos SDK codec method as `encoding/json.Marshaler`.

- **The specified upgrade query surface is now actually shipped**: `query
  upgrade` is registered with the normal context-aware pre-run and exposes
  current plan, applied plan, and module-version queries. Semantic tests bind
  upgrade names to recorded heights and exact block-header lookups, preserve
  module filters, and reject absent records or transport failures without a
  false-success result; the fresh-chain lane also requires a real non-empty
  module-version response and a dead-endpoint transport probe.

- **Authz grants now fail closed at the transaction boundary**: semantic tests
  decode exact send, deposit, generic, staking, contract, and store-code
  authorizations from generated transactions. Concrete SDK validation rejects
  malformed or ambiguous nested grants before transaction output or broadcast
  instead of deferring them to an on-chain handler.

- **Escrow runway estimates now have a state-based query contract**: tests
  assert the exact active-lease request, BME and oracle conversion policy,
  deployment identity, multi-lease rate sum, negative-balance handling, block
  settlement, JSON/YAML parity, and every upstream failure boundary. The
  command can no longer gain coverage merely by executing its constructor or
  printing an unchecked estimate.

- **The chain CLI coverage denominator now matches the shipped command tree**:
  removed unregistered validator-node, genesis, duplicate key-management,
  crisis/evidence, auxiliary-tip, and multisign-batch copies plus their
  dead-only tests. The registered block, block-search, and block-results
  queries moved into a focused release file and now validate positive heights,
  enum ordering, missing or malformed RPC responses, structured output, and
  writer failures at the command boundary. The module graph is tidied with the
  deleted legacy QR and node-only dependencies removed and newly direct PTY,
  WebSocket, JWT, and Kubernetes imports classified accurately. Live
  certificate transactions
  (including `--to-genesis`) and the validator commission-rate builder remain
  intact pending their separate product decisions.

- **Wasm transaction parsing now rejects ambiguous or corrupt input before
  broadcast**: raw modules are normalized to gzip, while truncated files,
  corrupt or non-Wasm gzip streams, mismatched reproducible-build checksums,
  conflicting upload-permission modes, and duplicate permission addresses fail
  at the CLI boundary. Semantic tests pin exact upload, instantiate, execute,
  permission-update, and authz-grant messages, including full admin-address
  resolution and immutable-contract intent.

- **Release gates now fail closed before publication**: release coverage uses
  the nearest earlier reachable semantic-version tag instead of `HEAD^`, and
  the event tag must be the sole tag selecting the tested commit both before
  verification and immediately before publishing. Tag publications share one
  queue across refs; manual modes retain no checkout credential and have only
  read access. Stable releases require the Homebrew token before GoReleaser
  starts, release builds default to `GOWORK=off`, and the intentional formula
  deprecation is accepted only with GoReleaser's dedicated status and matching
  diagnostics.

- **Changed-line coverage follows Go's statement-level instrumentation**:
  multiline executable syntax with no counter on the exact source line inherits
  only the first counter region of its smallest enclosing statement. Covered
  closing delimiters and function arguments no longer produce impossible patch
  failures, while initializers and other executable regions absent from the
  profile still fail closed.

- **Provider streams and bare-checkout builds now enforce their documented
  boundaries**: provider log and event WebSockets reject messages larger than
  16 MiB, and keyring-backed shell errors redact the exact JWT used for the
  request even when a provider echoes it without an authorization label. The
  root Makefile now supplies the same `.cache` defaults that direnv loads, so
  `GOWORK=off make akt` and the documented coverage targets work from a plain
  checkout without a sibling chain SDK.

- **Coverage documentation now reports the executable fuzz inventory exactly**:
  the repository currently has two native fuzz targets, not three. CI still
  runs their seed corpora only; a bounded fuzz campaign and mutation gate
  remain explicit delivery work.

- **Release publication now depends on exact-commit evidence**: version tags
  must point to a commit reachable from `main`, and the release workflow reruns
  the current hermetic lint, build/unit, race, offline E2E, fresh-chain E2E,
  coverage-report, and changed-line gates before publishing. The
  GoReleaser cross-build image is pinned to an OCI digest instead of trusting a
  movable version tag.
  Missing target lanes remain explicitly outside this interim release gate.

- **Monitor event setup now fails closed on local startup errors**: the shipped
  standalone runtime reports CometBFT client construction, WebSocket start, and
  synchronous event-service subscription failures instead of launching with an
  empty event bus. Every post-cache startup failure uses the same idempotent
  cleanup path as normal exit, stopping started event resources and releasing
  the bus and database. The upstream client's asynchronous server
  acknowledgement and resubscription behavior remains an explicit roadmap gap;
  this change does not claim that a queued subscription was accepted remotely.

- **E2E coverage shards now prove which executable produced them**: each
  stable instrumented build records the exact binary and source-manifest
  SHA-256 digests. Preparation and collection-side publication fail closed if
  the binary is missing or replaced, report jobs revalidate the manifest
  binding, and the primary Codecov uploads now run before narrower informational
  profiles. Boundary tests raise the generated tooling result from
  1,185/1,353 to 1,295/1,471 statements while covering the new command paths at
  95–100% except for operating-system-only file I/O failure branches.

- **Console deployment reconciliation now rejects unbounded pagination**:
  exhaustive snapshots and read-back checks stop at 100 pages or 10,000
  records. An endless `hasMore` sequence or oversized collection returns a
  local, credential-safe error, and deployment creation stops before its POST
  when it cannot prove a complete baseline.

- **Provider gateway calls now enforce a finite network boundary**: status,
  validation, manifest, lease/service status, and migration HTTP exchanges have
  a 30-second overall deadline and a 16 MiB response ceiling. Provider error
  detail is capped at 4 KiB, stripped of terminal controls, and redacted for
  bearer and API credentials, including oversized or chunked log/event
  WebSocket handshake failures. Established log, event, and shell streams still
  use the caller's cancellation lifetime and do not inherit the one-shot
  deadline.

- **Context output now honors Cobra stream boundaries**: context tables and
  empty results write through the command's configured stdout, while delete
  prompts and cancellation notices use configured stderr and confirmation is
  read from configured stdin. Writer and short-write failures now reach the
  command result instead of being hidden by process-global streams.

- **Coverage binaries now carry a build-consistent source manifest**: the
  instrumented binary is built as a candidate between pre-build and post-build
  source snapshots and is published only when those manifests are identical.
  A concurrent edit therefore aborts the build instead of attaching a newer
  manifest to stale or mixed executable code.

- **Coverage review now subtracts unreachable helpers instead of rewarding
  tests for them**: unused pretty-output selectors, YAML highlighters, table and
  writer conveniences, governance JSON formatters, an unwired cache migration,
  a copied query decoder, and a test-only bbolt path accessor were removed with
  their direct tests. The active denominator now tracks shipped behavior more
  closely rather than counting tests whose only consumer was the test itself;
  monitor cache documentation now names the bbolt database actually used.

- **Every public CLI renderer now participates in the output contract**:
  version metadata, shell completion, SDL scaffold catalogs, empty key and
  action-log collections, stream records, transaction simulation, deployment
  groups, provider mutation acknowledgements, store import notices, Console
  text results, workflow skipped-step diagnostics, and highlighted JSON all use
  the configured command writer and fail on hard or short writes. Key recovery,
  confirmation, import, and export read the configured command input; prompts
  and informational notices stay on stderr, while key material and other
  payloads stay on stdout. The Ledger
  fallback notice, Wasm download status, MCP startup banner, genesis summary
  and validation, and rollback result now follow the same checked boundary;
  applicable notices honor quiet mode, and a failed status write cannot be
  followed by a falsely successful Wasm artifact. Post-mutation local-store
  warnings remain intentionally best-effort because their remote or on-chain
  result has already committed.

- **Malformed keyring records now fail as boundary errors instead of panicking**:
  account, validator, consensus, and collection key-output paths validate both
  the record and its encoded public key before SDK address derivation. The
  accompanying adversarial coverage pass also exercises exact governance and
  audit transaction messages, offline auth/key utilities, workflow DSEQ and
  deposit resolution, immutable deployment groups, provider metadata, and
  monitor consensus/provider state transitions rather than padding coverage
  with command-constructor or help-only assertions.

- **Adversarial regression review closed terminal and Console false-success
  paths**: the standalone monitor once again owns Bubble Tea's alternate screen
  instead of painting into the caller's scrollback, while the embedded monitor
  leaves screen ownership to its shell. Console close now preserves
  unambiguous already-closed or not-found responses as typed failed mutations;
  a server rejection such as "cannot be closed while active leases exist"
  likewise stays failed and cannot be logged as a successful close. Console-only MCP startup
  is also defined for an environment API key with no active context, without
  manufacturing chain or provider capabilities or invoking the first-run
  wizard. Contextless MCP remains read-only: write enablement fails closed
  until a selected context supplies the required action-log destination.

- **Console mutation acknowledgements now prove their documented outcome**:
  create validates its managed-wallet receipt, update validates DSEQ and SDL
  hash, close requires `success: true`, and deposit validates identity and
  reconciles authoritative total escrow (`funds` plus `transferred`) without
  replaying a possibly accepted charge. Non-idempotent lease and one-time API-key responses remain `pending`
  when their outcome cannot be proved; blank API-key secrets and provider JWTs
  are rejected. Console-backed shell attempts now record exactly one provider
  action while status, logs, and events remain read-only and unlogged.

- **Store backup limits are symmetric and corruption-safe**: JSON and YAML
  export now enforce the same 64 MiB encoded-envelope ceiling as import before
  exposing any bytes, so the binary cannot create a backup it refuses to
  restore. Missing or null deployment, lease, or bid collections are rejected
  before mutation, and missing required bbolt buckets return named corruption
  errors instead of panicking during snapshot export. Destructive import now
  requires explicit `--replace` plus confirmation (or `--yes`); dry runs never
  prompt and `--merge=false` cannot silently select replacement.

- **Consensus monitoring now follows validator-set churn and recovers from
  startup faults**: validator identities and voting power are fetched for the
  exact WebSocket event height before applying its first event, and transient
  validator failures are never cached. A lagging or ahead RPC response whose
  height does not match the request is rejected. The monitor refuses to
  calculate a new height with stale power and reconnects after initial
  WebSocket, subscription, or validator failures instead of remaining dead
  until restart.

- **Monitor shutdown now cancels and drains every model-owned network task
  before closing shared state**: validator loading, chain refresh, provider
  status/detail probes, and retry fallbacks all share the runtime context.
  Cancellation prevents follow-up REST calls, cache writes, and rescheduling;
  both the standalone and embedded runtimes wait for in-flight commands and
  the long-lived consensus producer before shutting down the event bus and
  database. The event service retains a separate lifecycle context solely so
  graceful shutdown can send CometBFT `unsubscribe_all` after model work is
  canceled. Initial signing history now fetches validators at the sampled
  commit height, preventing validator-set churn from misattributing signatures.

- **Monitor event shutdown now flushes its queued unsubscribe before stopping
  the WebSocket client**: CometBFT's upstream call returns when its writer
  accepts `unsubscribe_all`, not when that writer puts the request on the
  connection. The runtime now uses a bounded FIFO write barrier, so slower
  coverage scheduling cannot let client shutdown overtake the unsubscribe.
  The boundary still does not claim that the server acknowledged the request.

- **MCP manifest submission validates the complete SDL-derived manifest before
  touching a provider**: semantically invalid groups and resources fail before
  registry discovery, gateway calls, or action-log writes. The exact MCP
  registry remains tested in read-only and write-enabled modes, while its
  inventory is explicitly distinguished from the still-missing whole-CLI
  semantic scenario manifest.

- **Transaction pretty output now fails when stdout is incomplete**: the public
  `PrintTxResult` path reports destination errors and short writes from its
  summary, headers, registered formatters, recursive formatters, and JSON
  fallback. Formatter errors retain their identity for `errors.Is`. Recursive
  authz output also withholds receipt-only fields because the chain indexes
  those events to the outer `MsgExec`; repeated inner messages can no longer
  display the first event value more than once.

- **Coverage infrastructure now fails closed around exact artifacts**: the local
  coverage command generates the union reports and runs the changed active
  line-to-counter gate, while Codecov performs the aggregate line comparisons.
  Release validation requires every GoReleaser build to target `cmd/akt` and
  match its build tags. CI exposes one stable `required-ci` check covering lint,
  build/unit, active race, report generation, the local patch gate, and the
  applicable upload job. Coverage binaries use reproducible trimmed paths,
  every CI and release action is pinned to an immutable commit, and the policy
  validator rejects future floating workflow action references.

- **Test-only chain flag builders no longer ship in `akt`**: a 1,025-line
  helper file, including two package globals, was compiled into every release
  even though production had no caller and test support used only three simple
  chain-ID argument constructions. Those constructions now live in the
  classified test-support package, removing 351 dead statements from the
  active denominator without excluding any user-reachable behavior.

- **Fresh-chain mutations now require an independent node oracle**: the public
  `akt` binary remains the system under test, while the pinned node image's
  native `akash` CLI supplies separately constructed transaction receipts and
  post-state JSON. Command output, committed hash, exact state transition, and
  action-log identity are bound together. Transfers to ephemeral recipients
  are restricted to harness-owned throwaway chains so an opted-in external RPC
  cannot strand funds in a key destroyed with the test home. External read-only
  account checks accept the endpoint's real denomination set and require one
  positive canonical balance; the Docker-only native observer and its exact
  two-denomination genesis contract are never imposed on an external RPC.
  Every fresh-chain query leaf now also proves it made a real request to a
  request-counting failure peer, while its help form proves it made none, so an
  unrelated local error cannot masquerade as transport reachability. The
  action-log oracle decodes raw JSONL directly from disk and independently
  collapses transaction revisions before comparing the public log command.

- **Final false-green probes hardened shipped monitor and Console response
  coverage**: coverage taxonomy validation now proves that every
  repository-owned dependency of the standalone monitor runtime remains in the
  active denominator, so a future import cannot silently pull shipped behavior
  back into the experimental TUI profile. The Console client now rejects empty
  expected responses and null data envelopes, and the live provider-status
  smoke test requires all provider status collections while validating each
  dynamically named service's identity and replica counts. The active race
  lane now uses release build tags, while Codecov's checkout-free OIDC job
  builds a Git index for correct source-network metadata and disables
  worktree-dependent file fixups. Successful protected pull-request Console
  runs now feed separate `live` and informational `union-live` artifacts;
  sandbox writes still require the independent exact mutation opt-in, and the
  live merge reuses the already-validated hermetic profiles without fetching
  unrelated history. Codecov's downloaded CLI is version-pinned, and any
  signature, checksum, or upload error fails closed; a missing required service
  status blocks merge until the retained report can be uploaded again.
  Sandbox reads and mutations use one protected environment credential. The
  sandbox jobs queue by tenant, and the live shard publishes its own
  active-package statement report before the informational union. Fork and
  Dependabot pull requests remain secretless, and CI never uses
  `pull_request_target` to execute proposed code.
  Ambiguous-create cleanup now waits through the same 30-second indexer window
  as normal create observation while preserving separate 40-second mutation
  and 20-second final-observation reserves, preventing a delayed accepted
  deployment from escaping auto-top-up disablement and close.
  Manual dispatch cannot authorize the spending Console sandbox lifecycle; it
  exposes the upstream node drift job alongside the ordinary secretless checks.
  The fixed Console lifecycle escrow now fits wholly inside its hard spend
  ceiling, and lease creation first selects the cheapest bid corroborated by
  both the CLI and raw observer whose conservative full-runtime cost also fits
  that ceiling.
  Console mutation clients now validate lease and settings acknowledgements
  before logging success; malformed lease success bodies reconcile exact state
  without replaying the POST, and omitted boolean fields cannot masquerade as
  a requested false setting.
  Raw-shard manifests now include release/test `go:embed` inputs and all Go
  package `testdata` fixtures, preventing stale workflow YAML, OpenAPI, or
  golden data from being paired with current coverage metadata. The collection
  CI, Make, and GoReleaser recipes are bound directly; release tags and the
  shipped main package are checked against the same GoReleaser recipe.
  Reporting-only exception and Codecov policy changes do not invalidate
  otherwise current raw counters.
  Taxonomy validation now also rejects active or experimental packages that are
  absent from the release dependency closure, making shipped classification a
  two-way invariant, and allows only the reviewed root-CLI bridge to import the
  experimental shell.
  Release dependency directory validation rejects repository-local nested
  modules that would otherwise ship outside every coverage denominator.
  Repository-import discovery preserves the empty import-list field on valid
  zero-import packages instead of rejecting its own `go list` output.
  Reviewed line exceptions affect only the local active line-to-counter gate; they do
  not bypass aggregate Codecov statuses and may require compensating coverage.
  Coverage shards now reject unexpected entries before artifact upload and
  again after download, limiting unit evidence to the source manifest and Go
  covdata and E2E evidence to those files plus its verified binary identity;
  collection also makes the pinned Go toolchain decode the shard so corrupt
  files cannot pass on names and non-zero lengths alone.
  The standalone monitor now initializes all cache buckets atomically and its
  critical runtime tests cover resource-initialization failure plus a real
  CometBFT WebSocket subscribe/unsubscribe cleanup lifecycle.

- **Bank balance queries now preserve canonical chain coins**: all-balances
  requests no longer ask the node to rewrite micro denominations through
  display metadata. JSON and YAML retain the exact `uakt` denomination and
  integer amount, while pretty output alone applies `FormatCoin()` (for
  example, `1000000uakt` renders as `1 AKT`). A real fresh-chain transfer test
  exposed the mismatch by proving a successful committed `MsgSend` while the
  CLI's canonical-denom balance assertion could not observe the credited row.

- **Adversarial test-safety review closed credential and external-chain
  hazards**: externally supplied RPC fixtures are now read-only unless a
  separate mutation opt-in names an expected chain ID and explicitly
  allowlists it; the harness verifies the remote chain ID, rejects production,
  and mutates only an exact resource discovered by an exhaustive paginated
  pre/post identity diff. The certificate-backed deployment lifecycle remains
  restricted to the harness-owned throwaway chain so it cannot leave a client
  certificate on an externally supplied fixture.
  The Console mutation endpoint guard now normalizes trailing-dot DNS aliases
  and classifies decoded URL paths, so production hosts or markers cannot hide
  behind DNS-equivalent names or percent-encoded separators.
  Console response capture is bounded in both the live harness and production
  client, and credential-echoing error bodies are redacted before returned
  diagnostics or action logs. Codecov OIDC now belongs to an upload-only job
  that never checks out or executes repository code. Live Console reads now
  validate collection cardinality, stable item identities, and nested field
  kinds for each command instead of accepting any correctly typed JSON value.
  The fresh-chain bank transfer now proves the exact 1,000,000 uAKT recipient
  delta and the complete single `bank.MsgSend` action-log record instead of
  accepting any non-zero balance and matching table substrings.

- **Coverage probes found output and import boundaries that could fail open**:
  workflow plan and dry-run emitters now propagate writer failures and short
  writes before any engine mutation can start. Store imports validate every
  owner/provider bech32 identity and dry-run snapshot acquisition has a bounded
  database-lock wait, so malformed backups cannot poison the store and an
  in-use database cannot hang validation indefinitely. Fresh E2E runners now
  create both the instrumented binary and source-manifest parent directories,
  so coverage collection cannot fail before the selected suite starts. The
  changed-line gate also normalizes module-root Go files to the module
  package, preventing a future root package from bypassing the 100% patch
  contract.
  Store imports now reject encoded envelopes above 64 MiB before allocating the
  whole input. MCP schema validation permits blank optional filters while still
  rejecting required or `minLength` strings; every deployment and market query
  handler now pins operation-specific dependency failures, preventing an
  aggregate coverage percentage from hiding lost error paths. Multi-message
  transaction output no longer reuses an unindexed aggregate event for every
  message.

- **Consensus reconnects could leave the monitor on a silent feed**: CometBFT
  reconnects the WebSocket transport but does not restore connection-scoped
  subscriptions. The monitor now reissues both consensus subscriptions after
  every reconnect and requires valid server acknowledgements for both before
  declaring the feed healthy; an event racing ahead of those acknowledgements
  is buffered rather than lost. Transport and model retry timers are
  context-aware and expose a completion signal, so shutdown proves the socket
  producer has stopped before shared resources close. The consumer channel
  closes if restoration fails. A
  forward-round vote received after reconnect now advances the dashboard and
  clears the preceding round's votes even when no matching round-step event was
  replayed; lower-height and lower-round events still cannot rewind it.

- **Transaction pretty-printers could attach another message's events or hide
  important fields**: every registered formatter now scopes event attributes to
  its message index, preserves full identities, uses the shared coin formatter,
  and renders the documented staking, distribution, governance, authz,
  feegrant, vesting, escrow, certificate, oracle, and WASM details. Semantic
  registry tests cover every formatter family, malformed `Any` values, sparse
  events, optional fields, and writer failures.

- **Adversarial coverage and backup audit found fail-open boundaries**: Store
  snapshot decoding now rejects unknown fields and trailing documents before
  any replace mutation; record states, timestamps, and heights are validated;
  and file export preserves an existing backup unless its replacement is fully
  written and closed. Provider MCP REST tools resolve a bech32 provider owner
  through the on-chain registry instead of accepting an arbitrary
  credential-bearing endpoint, and protected calls use granular provider,
  deployment, and operation-scoped JWT claims instead of full lease access.
  Coverage taxonomy discovery includes packages made entirely of
  build-constrained source, while repository-owned coverage tooling receives a
  separate unit report and aggregate Codecov project comparison.

- **Coverage could pass while most shipped behavior remained unmeasured**:
  DESIGN and SPEC now define separate repository, active shipped, and
  experimental-TUI denominators; cross-package unit coverage; instrumented
  subprocess coverage through `GOCOVERDIR`; unit, E2E, live, and union
  profiles; aggregate exact-base comparisons for active, experimental-TUI, and
  tooling code; and a 100% changed-line-to-counter gate for active code. A generated
  CLI and MCP manifest must assign every runnable action to a state-based
  scenario. Fresh-chain, provider,
  dual-chain, pinned `testnetify`, real-Console, monitor, and fault lanes now
  have explicit cadence, credential, spending, independent-oracle, action-log,
  and cleanup rules. Race, fuzz, and mutation gates complement the staged 95%+
  active-union Codecov line-coverage floor; dynamic comparison preserves later
  gains while work continues toward 100%. The same
  denominator audit moved the shipped standalone monitor runner and its
  cache/event lifecycle out of the experimental shell package, so `akt
  monitor` can no longer disappear from the active coverage gate. Taxonomy
  validation guards that runner's dependency closure against regaining an
  experimental-shell dependency. Raw-shard
  manifests bind tracked environment and workflow recipes plus the effective
  Go/CGO build environment and evaluated Make build tags/options, preventing
  counters compiled under a different source-selection configuration from
  being merged as current. Unit collection now uses those same release tags.
  CI retains the actual comparison revision for the local changed-line
  gate instead of assuming a shallow checkout contains `HEAD^`.
  Hosted coverage builds now receive the cache-library path required by the
  release linker flags instead of passing an invalid bare `-L` to the linker.
  Release-tag validation parses every GoReleaser build and rejects an untagged
  build, so a matching comment or sibling build cannot mask source-set drift;
  taxonomy validation also rejects tooling linked into the shipped binary.
  Codecov's active-union, experimental-TUI, and tooling project statuses and
  active patch status fail when their required reports are absent instead of
  accepting the service's permissive default.
  The documents distinguish that target from the currently delivered three
  blocking coverage lanes and list the provider, dual-chain, testnetify,
  monitor, fault, multi-actor, and mutation-testing work that remains. The same
  specification pass makes three uncovered failure contracts explicit: store
  imports validate before mutation and commit atomically, closed event
  subscriptions terminate their consumers, and consensus parsing rejects
  negative indexes while clearing votes on same-height round changes.

- **Coverage collection stopped at in-process tests and a stale 123-command
  help list**: CI now merges raw cross-package unit counters with coverage from
  instrumented `akt` subprocesses in the offline and fresh-chain lanes. A
  checked-in package taxonomy publishes repository, active, and experimental
  TUI profiles plus a separate tooling report. Codecov performs exact-base
  aggregate line comparisons for active-union, experimental-TUI, and tooling,
  while a local 100% changed-line-to-counter gate checks active code independently.
  Generated per-package TSVs are current diagnostics, and README shows the
  active-union badge. Coverage artifact uploads explicitly
  include the reviewed hidden directory and fail on an empty match, preventing
  a silently incomplete cross-job union. The aggregate gate now runs even
  after a failed or cancelled shard and explicitly rejects any non-successful
  dependency; every downloaded shard must contain non-empty Go coverage
  metadata and matching counters before conversion or merge. Dynamic project
  comparison preserves aggregate improvements without a checked-in numeric
  snapshot, and changed active source with executable statements cannot
  disappear behind an unmeasured build constraint. The changed-line gate
  runs on pull requests and default-branch pushes against the event's actual
  base, including multi-commit pushes; its local default also includes staged,
  unstaged, and untracked Go files. An explicitly selected Docker localnet now
  fails when Docker is unavailable instead of reporting a green skip, and its
  blocking node image is pinned by immutable digest. Every raw shard now also
  carries a deterministic source manifest, and the merge rejects a missing,
  malformed, mixed-revision, or stale manifest before interpreting counters.
  The fully
  instrumented binary uses the release build tags, with E2E asserting the
  reported tag set. The fully assembled Cobra tree now
  discovers and exercises every visible command path, including embedded
  workflows, while the MCP registry is checked rail by rail for exact tools,
  schemas, annotations, and boundary rejection. Semantic coverage was expanded
  across bootstrap, keyring, client identity, event delivery, MCP handlers,
  provider cache scheduling, consensus tracking, store persistence, workflow
  steps, pretty-output dependencies, and UI theme helpers. Every fresh-chain
  query-matrix entry now has JSON shape and state assertions and is repeated
  through a credential-free dead-RPC context; the negative run must fail with
  a diagnostic distinct from that leaf command's generated help document.

- **Console success testing stopped at canned HTTP responses**: a separately
  opted-in E2E suite now drives the public binary through a real managed-wallet
  create, bid, lease, provider-status, deposit, settings, SDL update, and close
  lifecycle. Mutations run through `akt`, while a separately decoded raw HTTP
  client observes Console balances, deployments, bids, leases, settings, and
  terminal state without reusing `internal/console`; action logs are inspected
  directly on disk. The suite reserves its complete USD, deployment, and lease
  plan before the first write and separately limits observed spend from the
  authoritative total balance delta after cleanup. It rejects production and
  unmarked endpoints,
  makes provider readiness mandatory after opt-in, bounds captured output,
  redacts response bodies and action payloads from failures, scans the complete
  temporary home for credentials, requires truthful repeated-close failure,
  and gives discovery, auto-top-up disable/close, terminal verification, and
  final balance observation separate cleanup deadlines. Ambiguous creates are
  rediscovered by unique post-baseline SDL hash; in-process cleanup attempts
  every match and an unresolved, duplicate, or unclosed outcome fails the run.
- **CLI flag names were scattered across command implementations as string
  literals and package-local constants**: every statically declared flag name
  now has one canonical definition in `internal/flags`. Registrations, reads,
  change checks, and Viper bindings import that registry directly; the chain
  flag package retains builders, parsers, defaults, and allowed values without
  re-exporting names. Existing flag names, defaults, shorthands, and help text
  are preserved. The new release-linked package is classified in the coverage
  manifest so coverage validation and report generation include it. Focused
  command tests cover the canonical names at registration, read, completion,
  Viper-binding, and transaction/query execution boundaries, including the
  testing and coverage surfaces merged ahead of this branch. Legacy proposal
  parsing now returns a fresh flag-name slice instead of exposing mutable
  package state.

- **Exports showed every `record_version` as zero without explaining the other
  version fields**: bbolt now assigns revision 1 to a new deployment, lease, or
  bid and advances it atomically on later writes while preserving newer
  imported revisions. Store documentation and export help distinguish record
  revision, database schema version, and export-envelope format version.

- **Stored bids always exported empty provider metadata**: workflow and
  reconciliation bid reads now enrich each unique provider with its advertised
  attributes and current audit presence. Console-only workflows use provider
  details from the Console API, and transient metadata lookup failures preserve
  previously stored values instead of failing the deployment.

- **Adversarial coverage exposed four boundary gaps**: eager account
  resolution now rejects nil or public-key-less keyring records without an SDK
  panic; MCP manifest submission derives the gateway from the audited on-chain
  provider identity; store exports take one strict bbolt snapshot and fail on
  corrupt rows; and the consensus WebSocket tracker ignores stale events
  instead of rewinding height, round, or step.

- **Console redirects and final workflow writes could bypass safety
  expectations**: the Console client now refuses redirects before forwarding
  an API key or mutation, omits hostile redirect locations from diagnostics,
  and redacts every transport error again at the action-log boundary. MCP
  schemas reject unknown, blank, and JSON-unsafe inputs, and pretty and JSONL
  workflow renderers propagate write failures to the process exit status.

- **Rejected transactions buried the useful chain explanation**: terminal
  errors now remove repeated unknown-gRPC prefixes, the Cosmos SDK message-index
  wrapper, and trailing internal Go source locations. The specific cause, such
  as the spendable and required balances, remains visible while action logs and
  exit-code handling retain the original error.

- **Deploy could stay silent for five minutes and then print its template
  condition**: the bid wait now reports bids received, elapsed time, and time
  remaining at useful intervals on interactive stderr. Its timeout says that
  no bids arrived and how long it waited, while machine output stays free of
  progress text and the internal Go-template condition is never exposed.

- **Store status hid bid and lease outcomes and called untouched reconciliation
  state a fault**: record totals now break down non-zero deployment, lease, and
  bid states, including an `other` count for unfamiliar values. The former
  `Sync State: not synced` section is now `Network Reconciliation`; it reports
  `not yet run` and points to the existing `akt store sync` command, or shows
  the height and time of the last completed snapshot.

### Fixed

- **Validator creation read a flag users could not set**: `staking
  create-validator` now registers its documented `--p2p-port` flag, validates
  the network-port range, and can emit the intended
  `node-id@ip:p2p-port` note in generated transactions.

- **Highlighted YAML could report success after losing output**: the shared
  pretty-output YAML writer now returns destination and short-write failures
  instead of discarding them, preserving the final-renderer contract for
  redirected command output.

- **MCP writes bypassed the context audit trail and advertised incomplete
  input contracts**: chain tools now reuse the transaction decorator, Console
  clients carry the selected action logger, and provider manifest submissions
  use the shared provider recorder for success, gateway rejection, and local
  authentication/setup failure. The manifest tool carries the full provider
  address separately from its gateway URL so logs retain a stable identity;
  every valid attempt writes exactly one entry and queries write none. State
  filters now publish their accepted enums, deposit schemas publish the real
  minimum, numeric identifiers reject fractional/non-positive values before a
  backend call, and usage-history dates are validated at the MCP boundary.

- **Store imports could partially mutate data before reporting failure**: the
  complete envelope and every record are now decoded and validated before one
  bbolt transaction performs merge or replace. Unsupported versions, corrupt
  identities, nil records, invalid sync state, cancellation, and write errors
  preserve the original store. Dry-run now validates a disposable snapshot and
  leaves even a previously absent selected-context directory and database
  untouched. Closing a deployment now advances both its deployment and
  matching lease revisions in the same transaction, preserves an existing
  close time, and rolls back on corrupt related state.

- **Event and consensus boundary failures could panic, spin, or display stale
  votes**: event service construction now rejects clients without subscription
  support, a closed subscription terminates and unsubscribes, and processing
  ignores unrelated or incomplete events while stopping the current block on
  RPC or publication failure.
  Consensus parsing rejects nil responses and negative height/round/step
  values, selects vote sets by their declared round rather than slice index,
  resets votes on same-height round changes, and validates WebSocket height,
  round, step, and validator indexes before applying them.

- **A corrupt keyring record could panic account resolution**: named-account
  resolution now rejects nil records and missing encoded public keys before
  calling SDK address derivation, returning a descriptive identity error.

- **Skipped workflow checks still aborted, and failed output writes looked
  successful**: `check` steps now honor `on-fail: skip` as a non-error skipped
  result, allowing the following step to run without a redundant `on-error`
  override. `output` steps now propagate destination write failures and record
  a failed result instead of claiming success after losing rendered output.

- **Console contexts exposed unsafe raw transactions and deployment state could
  drift or be duplicated**: raw `akt tx` is now specified as a keyring-only
  surface with an auth boundary independent of presentation gating, while
  managed-wallet lifecycle operations stay on shared workflows and
  `akt console`. Console deployment creation is single-submit and reconciles
  ambiguous outcomes by SDL hash instead of replaying POSTs, including on rate
  limits. Successful workflow and direct closes converge unique matching local
  deployment and lease records, and `store status`/`store sync` make network
  reconciliation discoverable and usable from ownerless Console contexts.
  Creation output no longer invents `open`, surfaces the default daily auto
  top-up and its disable command, manual lifecycle help points to `akt deploy`,
  balance allocation fields are labeled provisional, and nonzero sub-cent USD
  values no longer round to `$0.00`.

- **Confirmed transactions stayed pending forever in the action log**: the
  default sync broadcast records the honest CheckTx state before a block height
  exists, but nothing revisited that entry after inclusion. `akt context log`
  now best-effort resolves the pending hashes it is about to display through
  the active context's RPC endpoint and appends terminal success or failure
  revisions with height, gas, code, and error details. Reads collapse revisions
  by transaction hash before applying the limit, so the on-disk audit trail
  remains append-only while human and machine output show one current row per
  transaction. An unavailable node leaves the row pending without breaking
  offline log inspection.

- **Mainnet identity was duplicated in command code and context tests**:
  startup now reads the mainnet chain ID from the configured `mainnet` network,
  which first-run setup populates from `github.com/akash-network/net`, and
  passes that value to downstream monitor endpoint selection. Context tests
  assert against the configured chain ID instead of repeating a concrete
  mainnet identifier.

- **Context details rendered the network twice**: pretty `context show` output
  printed the shared network name and then immediately opened another nested
  `Network` section. The redundant name row is gone, leaving one section with
  the resolved chain ID and endpoints. JSON and YAML retain the complete
  network object, including its name.

- **Fresh lint runs reported possible nil dereferences in tests**: the tests
  already stopped with `t.Fatal` when a required command, flag, or stored value
  was absent, but staticcheck does not treat that call as terminating control
  flow. The guards now return explicitly before every dereference, keeping a
  cold-cache golangci-lint run clean without suppressing SA5011.

- **Collateral ratios still rendered at full width**: stripping trailing zeros
  only helps a value that has them, so a real on-chain ratio such as
  `1.495209570451729242` kept all eighteen decimals beside a `0.95` threshold.
  Ratios and thresholds now round to three decimal places before stripping, the
  precision SPEC §8.3.12 already illustrated. Oracle prices are deliberately
  exempt and keep full precision, because rounding AKT's `0.003125` to a ratio's
  precision would erase it. Prices instead report at the 8 decimal places the
  oracle itself publishes, so a derived TWAP (`0.536004234885265376`) no longer
  sits at twice the width of the source median (`0.53598949`) it is derived
  from. Every price in the BME and oracle views routes through one formatter.

- **Completion reports overstated what actually happened**: `akt sdl validate`
  documented only exit `0` and `1` while an unreadable file exited `2`, and an
  unreadable stdin exited `1` for the same class of failure. The command's help
  and SPEC §2.11 now carry a three-row exit-code table (`0` valid, `1` invalid,
  `2` the document could not be read), and a failed stdin read is a usage error
  so both input paths agree — nothing was parsed, so there is no validity
  verdict to report. `akt update` no longer prints an unconditional "Deployment
  updated!": the result reads the `send-manifest` step output it was already
  discarding, names each provider that accepted the new manifest, warns
  explicitly that nothing was redeployed when the deployment has no active
  leases, and states that a provider restarts a service only when its image
  reference or configuration actually changed. The bare
  `akt tx deployment update` result now says that only the on-chain SDL hash
  changed and names the two commands that deliver the manifest.

- **Key creation was missing from the activity log and a deployment appeared
  as six identical entries**: keyring mutations are now recorded like every
  other state change. `context keys add`, `--recover`, `--ledger`,
  `--multisig`, `delete`, `rename`, and `import` each write a `type=context`
  entry under a dotted `keys.*` action with the key name, type, and full
  address, on failure as well as success; `keys export` is recorded as a
  security event because it moves private key material out of the keyring,
  while the read-only `list`, `show`, `parse`, and `mnemonic` stay unrecorded.
  Mnemonics, BIP39 and armor passphrases, and key material never reach the
  log. Secret-leak regression coverage now checks the exact documented
  `name`, `type`, and `address` fields instead of treating a randomly generated
  mnemonic containing an ordinary metadata word such as `local` as a leak.
  The keys package cannot open a context's log — `internal/cli/context`
  imports it — so the single write path is injected as a recorder instead of
  duplicated. `akt context log` now renders the specified `SUMMARY` column
  instead of a bare action: workflow rows carry their step name and run id, so
  the six steps of a deploy are distinguishable and a failed middle step is
  identifiable; transaction, provider, console, and context rows carry their
  deployment, full provider address, and recorded parameters. `--workflow-id`
  isolates a single run, and the entry `step` index is no longer dropped from
  machine output for the first step of every run.

- **A successful deploy left the local store empty**: `akt store status`
  reported zero deployments, leases, and bids immediately after `akt deploy`
  completed against a live network, and no command existed to populate it.
  Nothing outside the disabled TUI ever wrote to the store: workflows relied on
  a sync engine picking up chain events, which a one-shot CLI process exits
  before receiving, and its reconciler had no chain-backed implementation at
  all. A workflow run now records its own outcome — the deployment with its
  SDL path and hash, the lease it won, and every bid it saw, marked matched or
  lost — writing it best-effort so a bookkeeping failure warns instead of
  turning a real deployment into a failed command. New `akt store sync
  [account]` reconciles the store against chain state for the context's
  `tracked-accounts` (previously a specified config key no code read),
  filling in the escrow balances, transferred amounts, and provider-side state
  a single run cannot observe, and preserving the local-only fields the chain
  does not carry. Stores are now opened through one helper that resolves the
  path from the context and applies pending schema migrations, and
  `akt context show` reports the tracked accounts. Reconciliation uses deferred
  identity access: an explicit address never opens the keyring, while a named
  tracked/default account resolves only when synchronization actually needs it.

- **Provider commands demanded a provider `akt` had already chosen, and
  suggested a shortcut that did not work**: every lease-scoped `akt provider`
  command now resolves the provider from the deployment's active lease on
  chain, exactly as `akt console status <dseq>` already does, so `akt provider
  lease-status 12345` needs no `--provider` for a deployment `akt` set up
  itself. `--provider` remains an optional override for choosing between
  several active leases, and never became a required flag. Ambiguity is refused
  rather than guessed: the error distinguishes a deployment with no leases,
  with no *active* lease (listing the states that exist), and with several
  active leases (listing every provider address in full), and points at `akt
  query market lease <dseq>`. The resolved lease also supplies `gseq`/`oseq`
  unless they were named explicitly, so a lease on a re-ordered group is
  reachable. `send-manifest` without `--provider` now delivers to every
  provider with an active lease, the behavior the spec had documented but never
  implemented. The guard order was inverted so a command missing everything
  reports the missing deployment sequence instead of blaming the provider, and
  the eight lease commands no longer advertise a positional provider argument
  that none of them accepts — on four of them that positional slot is the
  `dseq`, so following the old hint produced a second, more confusing parse
  error.

- **The documented provider lookup command did not exist**: `akt query
  provider <address>` — the form printed in README.md, SPEC §3.8.5, and
  DESIGN §7.1 — failed with `unknown command "akash1..." for "provider"`
  because the group carried no positional argument. The provider query is now
  positional-primary like `query deployment` and `query market lease`: an
  address returns that provider, no argument lists them all, and the `list` and
  `get` subcommands keep working unchanged.

- **Provider spec documented flags that did not exist**: `--from` was listed on
  every lease command but registered nowhere in the provider tree (the owner
  comes from the context's `default-account`), and `migrate-hostnames` /
  `migrate-endpoints` documented a `--destination-dseq` that has no
  counterpart in the gateway API, which addresses the destination lease alone.
  SPEC §2.4 now matches the command surface.

- **Strict keyring validation blocked commands that never used a key**: the
  startup identity boundary now distinguishes no access, deferred access, and
  required access. Public queries, provider status, MCP startup, workflow
  dry-runs, and address-only transaction generation/simulation no longer open
  an unavailable OS backend. Owner-defaulting and authenticated calls still
  resolve named accounts at the first operation that needs them.

- **Network-wide queries unlocked the configured keyring unnecessarily**:
  query initialization now carries a named default account without resolving
  it. Only an omitted owner filter that needs the account opens the keyring;
  network-wide reads and explicitly scoped queries remain non-interactive.

- **Offline commands asked for the wallet passphrase**: startup resolved the
  context's named `default-account` for every command, and resolving a name
  means unlocking the keyring — so `akt sdl validate` and `akt monitor`, both
  documented to run entirely locally, prompted for a file passphrase or an OS
  keychain unlock. Whether an invocation needs the local signing identity is
  now an explicit decision (`requiresLocalIdentity`, alongside
  `requiresConfig`/`requiresContext`) that the root passes down to the client
  context builder. Commands defined to run without a signer neither open a
  keyring nor resolve a named account; an address-valued default still
  resolves, because parsing bech32 costs nothing.

- **A configured `os` keyring silently became a different store**: the Cosmos
  SDK opens the `os` backend without pinning the backend list, and the
  underlying library skips past any backend whose opener fails — so a headless
  host with no session bus landed on `pass` or, failing that, an encrypted
  file keyring, while `config.yaml` and `akt context show` went on reporting
  `os`. That is also where the mysterious passphrase prompt came from. akt now
  resolves `os` to the platform's system credential store itself (Keychain,
  Windows Credential Manager, Secret Service/KWallet) with the backend list
  pinned, and fails fast when the host has none, naming both remedies instead
  of substituting a store the user never chose. `akt context show` and
  `akt context keyring list` report the *effective* backend next to the
  configured one, and the first-run wizard no longer offers `os` on a host
  that cannot provide it.

- **Key storage could not be chosen where it mattered**: `--keyring-backend`
  and `--keyring-dir` existed only on `tx` commands, so the documented
  `AKT_KEYRING_BACKEND`/`AKT_KEYRING_DIR` variables did nothing and there was
  no way to add a key to a `file` keyring on a box whose context said `os`.
  Both flags are now global, bound to Viper so the environment variables work,
  and applied to every keyring the invocation opens — including the one behind
  `akt context keys`. The transaction-local duplicates are removed: they
  shadowed the global flag, and their non-empty `os` default stood ready to
  override a context's persisted backend. A new `akt context keyring`
  group (`create`, `list`, `set`) makes the change persistent, which until now
  required hand-editing `config.yaml`.

- **First run left users on mainnet, in an unnamed directory, without their
  existing keys**: the bootstrap wizard silently preferred mainnet for
  `current-context`, so the shortest path through setup — Enter, Enter — armed
  the next transaction to spend real AKT on a network the user was never asked
  about. Choosing the active context is now an explicit prompt whose cursor
  starts on a test network (`sandbox`, else `testnet`, else any non-mainnet
  selection), with mainnet one keystroke away and each row stating in plain
  language what transacting there costs. The preference is a pure
  `pickInitialContext` helper so the safety property is testable, which nothing
  in the wizard's interactive body previously was.

  The wizard also never said where it was writing until the closing summary, so
  anyone who abandoned it, or who had `AKT_HOME`/`XDG_CONFIG_HOME` set by
  another tool, could not tell which of the four resolution steps had won. The
  resolved config root and config file path are now announced before the first
  prompt, together with the `--home` and `AKT_HOME` overrides and a note that
  nothing is written until the prompts complete. The closing summary grew from
  two lines to the full set of locations — config file, active context with its
  chain ID, context directory, store, action log, and keyring — using the same
  labels as `akt context show`, marking directories that do not exist yet as
  created on first use, and naming the system keyring service instead of a
  directory for the `os` backend. No generated context gets a
  `default-account`, so the summary now ends with the `akt context keys add`
  commands that make the configuration usable.

  Finally, nothing told users of the legacy `akash` CLI that their keys and
  certificates do not carry over. A read-only detector (`os.Stat` plus a `*.pem`
  glob — akt never reads, moves, modifies, or deletes anything under `~/.akash`)
  now triggers a notice naming all three reasons the legacy state is invisible:
  the OS keyring service name (`akash` vs `akt`), the keyring directory, and the
  client home directory used to locate `<address>.pem`. The notice states that
  the account is recoverable with `akt context keys add <name> --recover` at the
  same address, that the published certificate remains valid on chain
  (`akt query cert list <address>`), and that copying
  `~/.akash/<address>.pem` into the akt home restores mTLS for free — its
  password is derived from a keyring signature over the address — while
  regenerating with `akt tx cert generate client` / `akt tx cert publish client`
  costs a transaction.

  All wizard rendering — prompts, progress, summary, and the Console onboarding
  prompts — moved from stdout to stderr as SPEC §3.9.2 and §10.1.1 require; the
  wizard now writes nothing to stdout.

- **Transaction results presented an unconfirmed broadcast as a finished
  transaction**: akt broadcasts with `--broadcast-mode sync` by default, so the
  usual response is a CheckTx result — the transaction is in the mempool with no
  height, no gas accounting and no body — yet every output surface reported it
  as complete. Machine-readable output is now built from a structured result
  document instead of the raw `TxResponse`: it carries an explicit
  `status` (`confirmed`/`pending`/`failed`) plus `confirmed`, and omits
  `height`, `gas_used` and `gas_wanted` entirely rather than emitting the `"0"`
  that `PrintProto`'s `EmitDefaults` produced and that a script could not tell
  apart from a real reading. The same rule now applies to the action log (height
  and gas recorded only when reported; status `pending` until a height exists)
  and to workflow JSONL (the `height` key is dropped for an unconfirmed step).
  Pretty output no longer prints a bare `-`, `0 / 0` and a green `success` for a
  transaction that has only entered the mempool: it labels the state `pending`
  in yellow, says why height and gas are unknown, and points at
  `akt query tx <hash>` and `--broadcast-mode block`. The `Fee:` row is also no
  longer silently dropped when the body cannot be decoded — it is always
  emitted, and it now actually resolves a fee from a returned body, which it
  never could before (`cosmos.tx.v1beta1.Tx` does not satisfy `sdk.FeeTx`, so
  the old `UnpackAny` route always failed). Gas amounts use a gas formatter
  rather than the block-height formatter they were borrowing. SPEC.md §10.11.1,
  §10.11.2, §10.11.4, §10.11.6, §2.3.8, §5.4 and §5.6 updated.

- **A simulated transaction printed raw proto with a placeholder gas number**:
  `--dry-run` returns a `tx.SimulateResponse`, not a `TxResponse`, so it missed
  the pretty renderer entirely and fell through to a raw proto JSON dump. That
  dump included `gas_wanted: 0` — the placeholder the CLI substitutes for
  `--gas` on dry runs, echoed back by the node — while the adjusted gas estimate
  the dry run exists to produce was computed by the chain client and then
  discarded. Simulations now render a `Simulation` section (or a structured
  document under `-o json`/`-o yaml`) with the gas used, the gas adjustment, the
  recomputed gas estimate, and the estimated fee derived the same way
  `tx.Factory` derives it (`--fees`, else `ceil(--gas-prices × estimate)`),
  formatted through `FormatCoin`. Gas wanted is never surfaced from a
  simulation. The dead `GasEstimateResponse` type in
  `internal/cli/chain/auth_flags.go`, which upstream cosmos-sdk uses to print
  this line on a code path akt bypasses, was removed in favor of the new
  renderer. SPEC.md §10.11.7 added.

- **Currency guidance sent deployments to the denom the chain rejects**: `akt
  sdl validate` warned that `uakt` was "on-chain only" and hinted "keep `uakt`
  for on-chain deployments". That is backwards. `akt` auto-resolves the
  deployment deposit to `uact` on both rails (`DetectDeploymentDeposit`, and
  the console adapter), and the chain requires a group's price denom to equal
  the deposit denom, so following the hint produced `Mismatched denominations
  (uact != uakt)` and the deployment failed. The warning now says `uact` is the
  pricing denom on both rails and hints at switching to it (or passing a
  matching `--deposit <amount>uakt`); the unknown-denom error no longer
  advertises `uakt` as the on-chain alternative. SPEC §2.11 carried the same
  inverted claim and was corrected first.

- **BME conversions reported success while the funds were still in flight**:
  `akt tx bme mint-act|burn-act|burn-mint` printed `Status: success` and a bare
  sender/amount pair, but the chain does not execute the swap in that
  transaction — it records a pending ledger entry and settles it in a later
  block, so the burned amount had left the balance and nothing had arrived
  yet. The three message formatters now render a shared pending-conversion
  block (`pretty.RenderBMEPendingConversion`) that states the conversion is
  pending, names the destination denom, says the minted amount is not knowable
  until the oracle price is applied at settlement, and prints the follow-up
  query `akt q bme ledger --owner <signer> --status
  ledger_record_status_pending`. The three commands' help text says the same.

- **Ledger status codes were undocumented single letters**: `akt q bme ledger`
  wrote `e`, `p` and `c:<reason>` into STATUS, defined nowhere in the help,
  the spec, or a legend, while the neighbouring BME mint status already
  spelled its enum out. Statuses now render as `Executed`, `Pending` and
  `Canceled (insufficient funds)` in the same colors, and `q bme ledger` gained
  help describing each state; `--status` help lists the canceled filter it had
  omitted.

- **Oracle prices needed a denom spelling the rest of the tool never uses**:
  `akt q oracle aggregated-price` passed its positional straight through, so
  the `uakt` its own help and example advertised produced a raw gRPC status
  error — the network keys prices by base denom (`akt`), which is what the
  tool's production caller passes. The positional is now normalized (`akt`,
  `AKT`, `mAKT`, `uakt` → `akt`; the ACT family → `act`; anything else
  untouched), the example and `--asset-denom` help use the base denom, and
  every oracle query error is wrapped in the `Error:/Context:/Suggestion:`
  contract naming the denom tried and pointing at `akt q oracle prices`.

- **BME amounts were formatted three different ways in one table**: the BME
  status panel printed raw `LegacyDec` strings (`Collateral Ratio:
  1.500000000000000000`) while the oracle panel beside it on the same
  dashboard trimmed trailing zeros; the ledger table rendered two
  identically-typed price-carrying fields as `5 AKT @0.003125` and `5 AKT` in
  adjacent columns, showed a bare denom where an amount belongs on pending
  rows, and printed `-` for a zero spread. Ratios and prices now go through
  `TrimDecTrailingZeros`, every priced amount through one `formatCoinPrice`,
  a zero spread renders as `0 AKT`, and an amount that does not exist yet
  renders as a dim `pending` (the destination denom is already in ROUTE). The
  local `formatDecTrimmed` copy of the exported `TrimDecTrailingZeros` is
  gone.

- **A sparse ledger response panicked the command**: `RenderBMELedger` called
  `Spread.IsZero()` on a `Coin` whose `Amount` proto3 omits when zero, and a
  nil inner `Int` panics on any method call. Every coin and decimal read out
  of a ledger record now passes through the package's existing
  `IntOrZero`/`DecOrZero` guards, with regression coverage.

  `TestRenderBMELedger` had exactly one case (empty); it now covers executed,
  pending, canceled, zero-spread and sparse-wire records, and new tests assert
  the ledger status vocabulary, the BME transaction output, and the oracle
  denom normalization and error contract.

- **A search that found nothing printed a bare table header**: every pretty
  list renderer now states the empty result — `(no deployments)`, `(no bids)`,
  `(no networks)` — through the shared `WriteTableOrEmpty` /
  `WriteTableColsOrEmpty` helpers, replacing thirteen ad-hoc guards and
  fourteen renderers that had none. The plain table writers carry the same
  guarantee as a backstop, so no path can regress to a header with no rows, and
  `internal/output.PrintTable` is guarded too. Structured output is unchanged:
  `-o json` and `-o yaml` still emit an empty array, never prose, and
  `akt context network list` no longer replaces its JSON array with a sentence
  when no networks exist.

- **Column headers floated in the middle of their columns**: a table header is
  now padded exactly like the column it labels, so right-aligned headers such
  as `BALANCES` in `akt query bank total` sit over their amounts instead of
  drifting to the middle of a wide column. Every table in pretty output was
  affected; the centering helper is gone.

- **Capability labels pushed their values out of line in `akt context show`**:
  the view's key columns are widened together — `SubKVWidth` is now the
  documented counterpart of `KVWidth` — so `Chain transactions` and
  `Provider gateway` no longer overflow a fixed 16-column key field and every
  value in the view lands in one column. The governance parameters view had the
  same mismatch between its `KV` and `SubKV` blocks and is aligned to the same
  rule.

- **`akt console wallet settings` returned the raw API record**: both success
  paths now report the same `{autoReloadEnabled, configured}` object the
  never-configured path already returned, matching how the sibling
  `akt console deployment settings` shapes its output.

- **`akt context network list` ignored its own renderer**: pretty output now
  goes through `pretty.RenderNetworkList` — previously the only unused renderer
  in the package — so the CLI and the TUI network list stay identical, and RPC
  endpoints are printed in full instead of being truncated at 40 characters.

- **CI and release workflows used outdated GitHub Actions runtimes**:
  checkout, Go setup, and artifact upload now use their maintained v7
  releases, while golangci-lint uses the v9 action with the repository's lint
  binary still pinned at v2.11.4 for reproducible results.

- **Root help described deployment as the whole CLI**: the introduction now
  presents akt as the unified interface for chain queries and transactions,
  deployments on either payment rail, provider operations, context and key
  management, and network monitoring.

- **Monitor governance showed parameters but no proposals**: the Network
  dashboard now separates recent governance proposals from module parameters.
  Proposal rows include current tallies during voting and final tallies after
  completion, rendered through the same formatter as the query command.

- **Ctrl-C left the MCP stdio server blocked on stdin**: MCP now preserves
  command cancellation while attaching SIGINT and SIGTERM to the actual stdio
  loop, then treats an intentional cancellation as a clean shutdown.

- **The built-in sandbox context targeted a retired network**: the sandbox
  template now uses the live `sandbox-2` chain ID and the RPC, API, and gRPC
  endpoints published by the Akash network registry, restoring context
  switching and sandbox queries created from the built-in template.

- **Final live verification exposed shell and key-output boundary failures**:
  an explicit provider or Console shell command launched from a terminal now
  detaches stdin by default so a completed remote process cannot hang waiting
  for terminal input; interactive shells, pipes, and explicit `--stdin`
  overrides remain supported. `context keys add` now honors JSON/YAML for
  local, recovered, Ledger, and multisig keys while preserving the mnemonic
  backup contract for newly generated keys.

- **Workflow JSONL dry-runs emitted human prose**: `deploy`, `update`, and
  `close` now emit one valid JSONL `planned` record per step, with a shared run
  ID and empty error/transaction arrays, instead of silently ignoring their
  advertised output mode.

- **Console mutation responses could contradict the resulting chain state**:
  lease creation now reads the deployment back after a failed response and
  reports success only when every exact requested lease is active, without
  replaying the non-idempotent POST. Deployment updates retry the Console's
  transient manifest-version rejection and reconcile the expected SDL hash
  before reporting failure. Deployment details retain the API's version hash.
  Chain deployment help now recommends `auto`, so it follows the network's
  current minimum amount and denomination instead of advertising a stale
  hard-coded coin.

- **Workflow failures hid paid partial state and chain updates stopped at the
  transaction**: failed deploys now report their DSEQ, provider, continuing
  escrow risk, and exact retry and explicit-close commands without performing
  destructive cleanup automatically. Chain-backed updates deliver the revised
  manifest to every active lease provider, attempt all providers before
  failing, and remain safely retryable; Console-backed updates continue to use
  the Console API's manifest handling.

- **Standalone monitor navigation was routed to an invisible TUI view**:
  dashboard and Network sub-tab keys now reach the monitor in standalone and
  embedded modes, full-height rendering preserves the visible help/status
  footer, and resize events are delivered once. Provider version selection,
  detail-view reverse navigation, and dashboard-specific help now match the
  controls shown on screen. Governance loads the complete modern parameter
  response through RPC instead of rendering absent legacy REST fields as
  plausible zeros. The monitor cache now honors `--home`, context API endpoints
  supply auxiliary REST reads, and new mainnet templates select a verified
  WebSocket RPC first. Provider scans now verify TLS certificates by default;
  `--insecure` remains an explicit opt-in for debugging non-standard gateways.
  Ad-hoc RPCs now derive a same-origin REST endpoint instead of inheriting an
  unrelated context API, and legacy built-in mainnet contexts select the
  current WebSocket endpoint without rewriting user config. Live provider
  version sets reconcile safely, the table applies the advertised version
  filter, release candidates sort by their numeric suffix, and stale detail
  responses can no longer overwrite a newer choice. Provider gRPC probes now
  honor the same certificate-verification setting as REST probes. A standalone
  explicit RPC no longer triggers first-run config bootstrap, Cosmos `tcp`
  endpoints derive an HTTP REST peer, monitor cache failures reach the user,
  and cache cleanup removes both current and legacy files or reports failure.

- **Public provider status incorrectly required a wallet and could panic on an
  empty keyring**: CLI and MCP status calls now use an unauthenticated public
  gateway client. Protected CLI and MCP operations resolve the context's
  `jwt`/`mtls` default, then reject an invalid auth type, missing account,
  missing keyring, or absent signing key before provider discovery or
  gateway I/O. The inherited `--auth-type` flag is refused on public status
  instead of being ignored.

- **Monitor provider loading and WebSocket discovery were underspecified**:
  provider cache loading, on-chain reconciliation, health checks, periodic
  resync, and cache persistence now form one startup-owned pipeline independent
  of the visible dashboard. Monitor help examples use a verified
  WebSocket-capable RPC endpoint instead of an HTTP-only public gateway.

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

- **Transaction boundary inputs could still panic or demand an unnecessary
  key**: fee and gas-price strings are validated before the SDK factory,
  multisign assembly rejects ordinary keys and short signature batches, and
  unsigned generation can use a signer address absent from the keyring.

- **Vendored transaction separators became unnamed actionless leaves when the
  full PR set was assembled**: IBC client and transfer adapters remain grouped
  together without registering sentinel commands in the executable tree.

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

- **Four output contracts remained inconsistent after the input sweep**:
  `store import --quiet` still wrote success text, 43 adopted query help pages
  advertised an obsolete output enum, empty JSON store exports used `null`
  where YAML used `[]`, and root help claimed quiet mode suppressed result
  data. Informational import messages now honor quiet mode, adopted enum help
  is derived from the enforced values, store lists return stable empty arrays,
  and global help accurately describes informational-output suppression.

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

- **IBC connection-channel pagination ignored offsets and pages**: the
  dependency's filtered-pagination callback appends matching channels even
  while the SDK marks them as skipped, so `--offset` and `--page` returned the
  first channel again. The vendored query boundary now removes that skipped
  prefix and enforces the requested hard limit before rendering.

- **Two query boundaries still accepted misleading results**: `query gov
  proposer --height` returned the current transaction-index answer under a
  historical-looking invocation, and `query ibc client states --limit N`
  exposed the dependency's pagination lookahead record. The proposer query now
  refuses unsupported snapshot selection before network work, and the IBC
  adapter enforces the requested record limit for client-state lists.

- **Pretty output ignored redirection and `NO_COLOR`**: registered query and
  transaction formatters, deployment group rendering, and context/network
  detail commands wrote directly to the process stdout, bypassing Cobra's
  selected writer and terminal-aware styling boundary. Pretty output now flows
  through the command writer and strips all ANSI styling for files, pipes, test
  buffers, and explicit no-color sessions while retaining styling on an
  interactive TTY.

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

- **Independent command-tree contract tests used the same package helper**:
  the input-validation walker now has a domain-specific name so the scoping
  and executable-help branches compile and run together after merge.

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
- **MCP provider tools reached authenticated gateway endpoints anonymously**:
  lease status, service status, and manifest submission now use the shared
  authenticated provider gateway client. Public provider status remains
  walletless. The MCP inventory documentation now matches the
  capability-driven 27 read / 33 total maximum.

- **`console_wallet_balance` returned µACT while describing Console credits**: the MCP result now exposes `available_usd`, `in_deployments_usd`, and `total_usd`, derived from the Console balance helpers, so a `$17.94` balance cannot be mistaken for `17,940,000` dollars.

- **Help examples could be missing, stale, or aimed at internal reviewers**: the command help contract now requires every command to provide a syntactically valid example that names registered commands and flags, explains its placeholders, and contains no internal specification or agent instructions. A command-tree regression test enforces the contract so dependency-provided commands cannot silently reintroduce broken help.

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

- **Coverage measurement and gate (`make/testing.mk` + CI job)**: nothing measured coverage before. At that stage, three package sets — repo-wide (reported), akt-authored, and the 13 risk-carrying core packages (gated at 65%, then 68.7%, up from 54.9%) — established the first gate. 136 new test functions target money paths, credential handling, capability decisions, action-log writes, and the transport layer. Repo-wide was 30.9%: `internal/cli/chain` was 37.6% of all statements at 3.5%, so TASKS.md T126's ">80% overall" was never reachable as written — that was stated plainly rather than aspirationally.

- **Manual upstream-drift check**: an explicitly selected `e2e-localnet-latest` job runs the localnet suite against `ghcr.io/akash-network/node:latest`, while the blocking PR job stays pinned to the harness default. An upstream release can no longer redden an unrelated pull request, but drift remains available before release. The pinned default moved 2.1.0 → 2.1.1 (verified against both that and `latest`), and the version now lives only in `e2e/localnet_test.go` instead of being duplicated in CI.


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

- **`akt console` command group (AKT-648)**: New top-level command group porting the managed-Console CLI surface to Go per the console-axi reference: `login/logout/whoami` (login validates against `/v1/user/me` before storing the per-context credential; TTY prompt hides input), `deployment list/get/create/update/close/deposit/settings`, `bid list`, `lease create` (defaults to the per-context manifest cache), `wallet list/balance/settings/cost`, `usage`, public keyless `provider list/get/regions/auditors`, `gpu`, `template list/get/sdl` (raw SDL to stdout for piping), `apikey list/create/delete` (secret shown once), and `jwt create`. Positional-primary args per AKT-650 conventions; truthful already-closed failure; USD `$X.XX` formatting; mutations recorded in the action log. SPEC gains §2.9 "Console Commands". 8 command tests.

- **Console API client reworked to match the real API (AKT-648)**: The client's request shapes were wrong — the Console API wraps write bodies/responses in a `{"data": ...}` envelope and `deposit-deployment` takes `{data:{dseq,deposit}}`, not `{dseq,amount}`. Rebuilt the client with correct wire shapes (verified against the console-axi reference CLI) and expanded it from 8 to ~30 endpoints across deployments (incl. `/v2/deployment-settings` with PATCH→POST fallback and typed already-closed failure via `ErrAlreadyClosed`), wallet/balances/usage, public marketplace (providers, regions, auditors, GPU prices, templates, bid screening), API keys, and provider-scoped JWT minting. Adds a per-context manifest cache at `contexts/<name>/manifests/<dseq>.json` (0600) so `deployment create` → `lease create` works without re-passing the manifest. µACT↔USD helpers (1 ACT = 1 USD, 1e6 micro). 36 tests.

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
