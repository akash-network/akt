# AGENTS.md

## Project: akt - Akash Network Unified CLI & TUI

### Required Context

Before beginning any task in this repository, you MUST read the following files in full:

1. **DESIGN.md** - Architecture design document covering the overall structure, goals, and design rationale.
2. **SPEC.md** - Detailed technical specification covering configuration, CLI commands, flags, store, sync engine, TUI, plugin system, output formats, error handling, and the phased implementation plan.

These documents are the **source of truth** for how the project should be built.

### Spec-First Development

All changes MUST be reflected in SPEC.md and DESIGN.md **before** implementation begins:

1. **Propose** the change by describing what needs updating in the spec/design docs.
2. **Update** SPEC.md and/or DESIGN.md with the agreed-upon design.
3. **Implement** the code changes to match the updated spec.

Never implement code that contradicts or isn't documented in the spec. If the spec is wrong or incomplete, fix the spec first.

### Go Workspace

If a `go.work` file is present in the repository root, agents MUST read it and take into account all `replace` directives defined there. Local module replacements affect import resolution and must be followed when navigating or editing dependency code.

This repository currently has **no** `go.work` of its own, but a parent directory does and Go discovers it automatically. That workspace does not `use` this module, so a plain `go` command resolves against it and fails before compiling. Every direct `go` invocation (`go build`, `go test`, `go list`, `go vet`) MUST therefore run with `GOWORK=off` — which is what `.envrc` exports in a direnv shell and what CI sets globally in `.github/workflows/ci.yml`.

### Git Conventions

All commit messages MUST follow the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) specification:

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

**Types** (required):
- `feat` — a new feature
- `fix` — a bug fix
- `docs` — documentation-only changes
- `style` — formatting, missing semicolons, etc. (no code change)
- `refactor` — code change that neither fixes a bug nor adds a feature
- `perf` — performance improvement
- `test` — adding or correcting tests
- `build` — changes to the build system or dependencies
- `ci` — changes to CI configuration
- `chore` — other changes that don't modify src or test files

**Rules**:
- Subject line: lowercase, imperative mood, no period at the end, max 72 characters
- Body: wrap at 72 characters, explain *what* and *why* (not *how*)
- Breaking changes: add `!` after type/scope (e.g., `feat!:`) or a `BREAKING CHANGE:` footer

### Code Conventions

- **Language**: Go
- **Module path**: `pkg.akt.dev/akt`
- **Config format**: YAML with 2-space indentation
- **Config management**: Viper (reading, writing, env binding, flag binding, live-reload)
- **Global variables**: Default to no package-level `var` usage. If you believe a global is necessary, you MUST ask for direction first (you may offer alternatives or suggestions). Only proceed after explicit approval and, if needed, a spec update.
- **`init`** - every `init` usage must be explicitly approved
- **No reading flags into Go variables**: Do not use `cmd.Flags().StringVar(&myVar, ...)`. Bind flags to Viper keys; read values from Viper or the cobra command at point of use.
- **Addresses**: Never truncate or shorten addresses in output. Addresses (bech32, operator, consensus) must always be displayed in full. Truncation risks ambiguity and breaks copy-paste.
- **Amounts and prices**: All micro-denominated values (`u`-prefixed denoms) must be scaled to the most readable unit in pretty output: base (>= 1M micro, e.g., `5.3 AKT`), milli (>= 1K micro, e.g., `3 mAKT`), or micro (< 1K, e.g., `500 uAKT`). Trailing zeros must always be stripped. This applies uniformly to every pretty output: balances, prices, escrow, staking, rewards, fees. Use `FormatCoin()` from `internal/output/pretty/helpers.go` — never format amounts manually.
- **Pretty/TUI visual parity**: The pretty-printed output of a single-shot CLI command (e.g. `akt q bme status`) and the corresponding section in the TUI or `akt monitor` dashboard must be visually identical. Both paths must call the same `Render*` functions from `internal/output/pretty/`. Never duplicate formatting logic in the TUI — always delegate to the shared renderers.
- **Build**: `GOWORK=off make akt` — the compiled binary is placed in `.cache/bin/`. `make` needs the direnv-managed environment (`AKT_ROOT`, `AKT_DEVCACHE_*`); without direnv, build the binary directly with `GOWORK=off go build -o .cache/bin/akt ./cmd/akt`, as CI does.
- **Tests**: `GOWORK=off go test ./...`. The `e2e/` package shells out to the compiled binary at `.cache/bin/akt`, so build before running the full suite; to run unit tests alone, exclude it the way CI does (`go test $(go list ./... | grep -v /e2e)` with `GOWORK=off` exported). The localnet and live-Console e2e tests self-skip unless `AKT_E2E_LOCALNET`/`AKT_E2E_RPC` or `AKT_E2E_CONSOLE_API_KEY` is set.

### CLI & Transport Conventions

- **Positional-primary commands**: A command's primary value(s) MUST be positional arguments; flags are optional overrides and may never be required (SPEC.md §3.8). `TestNoUnapprovedRequiredFlags` in `internal/cli/required_flags_test.go` walks the entire command tree and fails on any flag marked required, so a new required flag breaks the build unless it is added to the `requiredFlagAllowlist` there with a comment justifying it. Adding an allowlist entry is a last resort, not a substitute for accepting the value positionally.
- **Disabled-flag markers**: During the positional-only UX trial, a flag that duplicates a positional is commented out — never deleted — so it can be restored verbatim. Use the uniform marker: a comment reading `// FEEDBACK(2026-07): --<flag> disabled for the positional-only UX trial (use the positional form instead). Restore by uncommenting if users ask for the flag form back.` immediately followed by the original registration line, commented out and otherwise unchanged (see `internal/cli/console/wallet.go` and `internal/cli/chain/flags/deployment.go`). The same marker prefixes the commented-out read sites (`cmd.Flags().GetString(...)`), with the wording adjusted to name the positional that replaces the flag. Deleting the registration instead of commenting it out is wrong while the trial runs.
- **Capability annotations**: A command that needs a transport MUST declare it with the `capability.AnnotationKey` (`akt.requires`) cobra annotation from `internal/capability` — `chain-query`, `chain-tx`, `provider`, or `console` — with `|` separating alternatives (e.g. `chain-tx|console` for workflow commands that run on either rail). Every new command group registered in `internal/cli/root.go` that talks to a chain RPC, a provider gateway, or the Console API needs the right annotation; without one the group is offered in configurations that cannot run it and fails deep inside the transport instead of failing fast with a remedy (SPEC.md §2.10).
- **Transports, not per-rail commands**: A new *action* is defined exactly once as a workflow definition in `internal/workflow/builtin/` and MUST work on every rail — the CLI surface is generated from the definition and the rail is chosen per context at execution time. Per-rail behavior belongs in `internal/transport` and the adapters it wraps (`internal/workflow/adapters/`), never in the command layer. Cross-rail user input is normalized in exactly one place: every deposit form goes through `transport.ParseDeposit`, so identical arguments work on the chain and console rails (SPEC.md §2.3 Transports, §7.4–§7.5).
- **Action log coverage**: Every state-changing action MUST record an action log entry, and read-only queries MUST NOT (SPEC.md §5.6). Hook a new mutating command into an existing write path rather than logging ad hoc: chain transactions through the tx decorator in `internal/cli/chain/actionlog.go`, context management through `recordContextAction` in `internal/cli/context/actionlog.go`, provider gateway operations through `recordProviderAction` in `internal/cli/provider/actionlog.go`, Console API calls through `Client.WithActionLog` in `internal/console/client.go`, and workflow steps through the engine's `Logger` (`internal/workflow/actionlog.go`).

### Workflow

- **Plan before implementing.** Every solution — regardless of agent mode (plan, build, or any other) — MUST be presented as a plan and approved before any code changes are made. Do not skip the planning step even if the change seems trivial.

### Changelog

Every change MUST include a corresponding entry in `AICHANGELOG.md`. Each entry should describe the feature or issue addressed and the fix or implementation applied. Do not skip this step, even for small changes.

### Problem-Solving

- **Never guess or hack.** If something is not working, and you don't understand why, ask for instructions. Do not invent workarounds, nil-guards, or fallback paths to mask a problem you haven't fully traced.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan
<!-- SPECKIT END -->
