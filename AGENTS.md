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
- **Build**: `make akt` — the compiled binary is placed in `.cache/bin/`
- **Tests**: `go test ./...`

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
