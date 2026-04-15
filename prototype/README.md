# akt TUI Prototype

Interactive terminal prototype for the unified Akash CLI (`akt`). Built with [Bubbletea v2](https://charm.land/bubbletea), [Lipgloss v2](https://charm.land/lipgloss), and [Bubbles v2](https://charm.land/bubbles).

All data is mocked — no chain connection, no keyring, no provider gateway. This is a navigation and interaction proof-of-concept.

## Run

```bash
go run .
```

Requires **Go 1.26+**. Dependencies install automatically on first run.

## Screenshot mode

Renders all views to stdout for visual review (no interactive TUI):

```bash
go run . --screenshot
```

## Views

### Primary (number keys 1-7)

| Key | View | Description |
|-----|------|-------------|
| `1` | Home | Balance, active deployments, network summary |
| `2` | Deployments | List with search (`/`), state filter (`f`), close (`d`) |
| `3` | Leases | List with state filter, order info and settlement data |
| `4` | Providers | Directory with resource utilization |
| `5` | Monitor | Network consensus, provider fleet, oracle/BME (Tab cycles) |
| `6` | Governance | Proposals with vote tally, vote dialog (`v`) |
| `7` | Staking | Validators with delegate/redelegate/undelegate (`d/r/u`) |

### Drill-down (Enter from list, Esc to go back)

- Deployment Detail — DSEQ, owner, state, resources, endpoints, log viewer (`l`)
- Lease Detail — order info, settlement data, provider status
- Provider Detail — attributes, resource bars
- Proposal Detail — vote tally with progress bars, vote dialog
- Validator Detail — delegation info, staking actions

### Overlays

| Key | Overlay |
|-----|---------|
| `:` or `Ctrl+P` | Command palette (fuzzy search all views) |
| `?` | Context-sensitive help (keybinding reference) |
| `d` (from deployment) | Close confirmation with fee preview |
| `v` (from proposal) | Vote dialog (Yes/No/Abstain/Veto) |
| `d/r/u` (from validator) | Delegate/Redelegate/Undelegate dialog |
| `l` (from deployment detail) | Scrollable log viewer |

## Navigation model

- **Number keys** (1-7) jump to primary views from anywhere
- **Enter** drills into detail, **Esc** pops back
- **Tab/Shift-Tab** cycles Monitor sub-dashboards (Network, Provider, Oracle/BME)
- **j/k** navigates lists, **f** cycles state filters
- **q** goes home (or quits from home)

## Project structure

```
main.go          # Full TUI application (~3,400 lines)
theme/theme.go   # Design tokens: colors, styles, component patterns
go.mod           # Module definition
go.sum           # Dependency checksums
DESIGN.md        # Design system reference (AI-consumable)
```

## Status

This prototype covers the core navigation model from the [TUI View Map](https://www.figma.com/design/pNKxyIne8RUs8dZkgBna6f). Known gaps:

- **Workflows** — `akt workflow` system not yet defined (placeholder code exists but is disabled)
- **Shell** — interactive terminal from deployment detail (stubbed in footer hints)
- **Node/Host Detail** — drill-down from Monitor > Provider Fleet (naming TBD per Artur)
