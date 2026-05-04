<!-- Sync Impact Report
  Version change: (none) -> 1.0.0
  Modified principles: N/A (initial ratification)
  Added sections:
    - Core Principles (7): Spec-First Development, No Global State,
      Test-First, Flag-Minimal Operation, Pretty/TUI Visual Parity,
      Stream Separation & CLI UX, Simplicity & YAGNI
    - Technology & Safety Constraints
    - Development Workflow & Quality Gates
    - Governance
  Removed sections: N/A
  Templates requiring updates:
    - .specify/templates/plan-template.md: no update needed
      (Constitution Check references constitution generically)
    - .specify/templates/spec-template.md: no update needed
      (no constitution references)
    - .specify/templates/tasks-template.md: no update needed
      (TDD task structure aligns with Principle III)
  Follow-up TODOs: none
-->
# akt Constitution

## Core Principles

### I. Spec-First Development

All changes MUST be documented in SPEC.md and/or DESIGN.md **before**
implementation begins. These documents are the source of truth for how
the project is built.

- Propose the change by describing what needs updating in the
  spec/design docs.
- Update SPEC.md and/or DESIGN.md with the agreed-upon design.
- Implement the code changes to match the updated spec.
- Never implement code that contradicts or is not documented in the
  spec. If the spec is wrong or incomplete, fix the spec first.

**Rationale**: Eliminates drift between documentation and
implementation. Ensures every contributor--human or AI--works from
the same agreed design.

### II. No Global State

No package-level `var` declarations, no `init()` functions, and no
reading flags into Go variables. All state MUST be passed explicitly
via function parameters or struct fields.

- Every `init()` usage MUST be explicitly approved before use.
- Every package-level `var` MUST be explicitly approved before use.
  Alternatives MUST be offered first.
- Flags are bound to Viper keys; values are read from Viper or the
  cobra command at point of use--never stored in module-level
  variables.
- The application context is resolved once at startup and injected
  into all services (client, provider gateway, sync engine, TUI).
- Override chain: flags > env vars > config file > built-in defaults
  (provided natively by Viper).
- Config live-reload flows through the watcher and subscriber
  pattern, not through global variables.

**Rationale**: Explicit state propagation prevents hidden coupling,
makes testing straightforward, and ensures the Viper resolution
chain is the single source of truth for configuration values.

### III. Test-First (NON-NEGOTIABLE)

Tests MUST be written and verified to fail before implementation
code is written. The Red-Green-Refactor cycle is strictly enforced.

- Write tests that express the expected behavior.
- Verify the tests fail (Red).
- Write the minimum implementation to make them pass (Green).
- Refactor while keeping tests green.
- E2E tests are required for each implementation phase.
- Unit tests are required for core packages (context, store,
  workflow engine, output formatting, filter parsing).
- Integration tests are required for cross-service communication
  (sync engine <-> store, chain client <-> context).

**Rationale**: Test-first prevents speculative code, ensures every
behavior is verified, and catches regressions early. The phased
E2E suites (P1-18, P2-12, P3-22, P4-15) validate end-to-end
correctness at each milestone.

### IV. Flag-Minimal Operation

After initial context configuration (network, keyring, default
account), the majority of CLI operations MUST require zero
additional flags or environment variables. The context system
supplies all defaults.

- Query commands use positional filter arguments
  (`[owner/]dseq[/gseq/oseq[/provider]]`) instead of flag-based
  identity filters (`--owner`, `--dseq`, etc.).
- Smart type detection classifies the first component: bech32
  address -> owner/provider, unsigned integer -> dseq.
- When no owner is specified, the context's `default-account` is
  used.
- Non-identity filters (e.g., `--state`) remain as flags.
- Flags remain available as overrides but are not the primary
  interaction model.

**Rationale**: Reduces cognitive load and command-line noise. Users
type only the command and a resource identifier. The context system
exists precisely to eliminate repetitive flag passing.

### V. Pretty/TUI Visual Parity

The pretty-printed output of a single-shot CLI command (e.g.,
`akt q bme status`) and the corresponding section in the TUI or
`akt monitor` dashboard MUST be visually identical.

- Both code paths MUST call the same `Render*` functions from
  `internal/output/pretty/`.
- Never duplicate formatting logic in the TUI--always delegate to
  the shared renderers.
- Transaction result formatting follows the same pattern: shared
  `TxPrettyFormatter` registry for both CLI and TUI.
- All colors and styles MUST come from the unified theme package
  (`internal/ui/theme/`).
- Amounts MUST be formatted via `FormatCoin()` from
  `internal/output/pretty/helpers.go`--never manually.

**Rationale**: A single rendering path eliminates visual
inconsistencies between CLI and TUI, reduces maintenance burden,
and ensures users see identical information regardless of mode.

### VI. Stream Separation & CLI UX

Data output goes to stdout. Everything else--errors, warnings,
progress indicators, verbose/debug logging--goes to stderr.

- `tx` commands show progress status on stderr during multi-second
  operations when a TTY is attached and `--quiet` is not set.
- `--quiet` suppresses all informational output; only data (stdout)
  and errors (stderr) are emitted.
- Error messages follow the three-part format: what happened,
  context, and a suggestion for resolution.
- Every command MUST populate cobra's `Example` field with at least
  one usage example.
- Addresses MUST never be truncated in output. Full bech32 always.
- Six CLI UX principles govern all command design: familiarity,
  discoverability, feedback, clarity, flow, and forgiveness.

**Rationale**: Strict stream separation enables reliable piping and
scripting. Structured errors and mandatory examples make the CLI
self-documenting and user-friendly.

### VII. Simplicity & YAGNI

Start simple. Every abstraction and complexity MUST be justified.

- Clean-copy from chain-sdk with targeted cleanups--do not
  over-abstract.
- The plugin system follows kubectl's proven exec-based model--no
  custom IPC protocol.
- Glyphs are ASCII-safe only. No Nerd Font PUA-range output. All
  glyphs are defined in a centralized registry with semantic names.
- If a simpler alternative exists that meets the requirement, use
  it. Document rejected alternatives when complexity is chosen.

**Rationale**: Unnecessary abstraction increases maintenance cost
and cognitive load. The project has a large surface area (tx,
query, workflow, TUI, monitor, MCP, plugins); simplicity at each
layer is essential for long-term maintainability.

## Technology & Safety Constraints

**Language**: Go (module path `pkg.akt.dev/akt`).

**Config**: YAML with 2-space indentation, managed by Viper,
XDG-compliant paths (`$AKT_HOME` > `$XDG_CONFIG_HOME/akt` >
`~/.config/akt`).

**Core dependencies**: Cobra (CLI), Bubbletea v2 + Lipgloss v2
(TUI), bbolt (store), Viper (config), mcp-go (MCP server).

**Chain SDK relationship**: CLI tx/query code is clean-copied from
`akash-network/chain-sdk/go/cli` into `internal/cli/chain/`. All
other chain-sdk packages are imported directly. The `go.work` file
and its `replace` directives MUST be respected.

**Security**:
- Keys MUST never be persisted in config files. Keyrings use the
  Cosmos SDK keyring abstraction (os/file/test/kwallet/pass
  backends).
- Console API keys are supplied via `AKT_CONSOLE_API_KEY` env var
  or `--console-api-key` flag--never stored in config.
- MCP write tools (on-chain transactions, provider mutations)
  require explicit `--enable-writes` opt-in.
- Destructive actions require `--yes` (skip confirmation) or
  `--force` (override safety guard). These are distinct: `--yes`
  does not bypass safety checks, `--force` does.

**Build**: `make akt` -- binary placed in `.cache/bin/akt`.

**Tests**: `go test ./...`.

## Development Workflow & Quality Gates

**Plan before implementing**: Every change--regardless of perceived
simplicity--MUST be presented as a plan and approved before any
code changes are made.

**Spec-first flow**:
1. Propose the change.
2. Update SPEC.md and/or DESIGN.md.
3. Implement to match the updated spec.

**Test-first flow**:
1. Write tests expressing expected behavior.
2. Verify tests fail.
3. Implement.
4. Verify tests pass.
5. Refactor while green.

**Changelog discipline**: Every change MUST include a corresponding
entry in `AICHANGELOG.md` describing the feature/fix and the
implementation approach.

**Quality gates** (all MUST pass before a change is considered
complete):
- `make akt` builds successfully.
- `go test ./...` passes.
- SPEC.md/DESIGN.md are updated to reflect the change.
- AICHANGELOG.md entry is present.
- No unexplained `init()` or package-level `var` introduced.

**No guessing**: If something is not working and the cause is not
understood, ask for instructions. Do not invent workarounds,
nil-guards, or fallback paths to mask untraced problems.

## Governance

This constitution supersedes all other development practices. When
a conflict exists between the constitution and any other document
or convention, the constitution prevails.

**Amendments**: Any change to this constitution requires:
1. A written proposal describing the change and rationale.
2. Explicit approval.
3. A migration plan if the change affects existing code or
   workflows.
4. An updated version number following semantic versioning:
   - MAJOR: Backward-incompatible governance/principle removals
     or redefinitions.
   - MINOR: New principle/section added or materially expanded
     guidance.
   - PATCH: Clarifications, wording, typo fixes, non-semantic
     refinements.

**Compliance**: All PRs and reviews MUST verify compliance with
these principles. Complexity MUST be justified per Principle VII.

**Runtime guidance**: Use AGENTS.md for day-to-day development
guidance that supplements (but does not override) this constitution.

**Version**: 1.0.0 | **Ratified**: 2025-07-18 | **Last Amended**: 2025-07-18
