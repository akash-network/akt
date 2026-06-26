# TUI Full Design Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rework the akt TUI to match the bubbletea v2 prototype (`design/prototype/akash-tui-v2/main.go`) pixel-for-pixel — dashboard, detail views, overlay compositing, data columns, and theme.

**Architecture:** The prototype is the authoritative visual reference. Tasks proceed bottom-up: theme first (all views depend on it), then data views, then app-level wiring. Each task produces a compiling, testable change. The existing `ResourceTable` component, `components.SectionWithKV`, `components.StateTag`, and `components.ProgressBar` are reused wherever possible.

**Tech Stack:** Go, bubbletea v2, lipgloss v2, bubbles v2, bbolt store, Cosmos SDK query clients

**Spec:** `docs/superpowers/specs/2025-07-17-tui-design-parity-spec.md`

**Prototype reference:** `design/prototype/akash-tui-v2/main.go` (3,471 lines) + `design/prototype/akash-tui-v2/theme/theme.go` (316 lines)

---

## File Map

| File | Action | Task |
|------|--------|------|
| `internal/ui/theme/theme.go` | Modify | Task 1 |
| `internal/ui/theme/theme_test.go` | Modify | Task 1 |
| `internal/tui/messages/messages.go` | Modify | Task 2 |
| `internal/tui/views/proposal_detail.go` | Create | Task 3 |
| `internal/tui/views/validator_detail.go` | Create | Task 4 |
| `internal/tui/views/lease_detail.go` | Create | Task 5 |
| `internal/tui/views/provider_detail.go` | Create | Task 6 |
| `internal/tui/views/governance.go` | Modify | Task 7 |
| `internal/tui/views/staking.go` | Modify | Task 7 |
| `internal/tui/views/dashboard.go` | Rewrite | Task 8 |
| `internal/tui/views/dashboard_test.go` | Modify | Task 8 |
| `internal/tui/app.go` | Modify | Task 9, 10, 11 |
| `internal/monitor/ui/view.go` | Modify | Task 12 |
| Various `testdata/*.golden` | Update | Task 13 |

---

### Task 1: Theme Alignment

Add missing prototype styles to the theme and fix property differences. This is the foundation — all subsequent tasks depend on it.

**Files:**
- Modify: `internal/ui/theme/theme.go`
- Modify: `internal/ui/theme/theme_test.go`

**Reference:** `design/prototype/akash-tui-v2/theme/theme.go` (the full 316-line file)

- [ ] **Step 1: Add missing inline column styles**

Add these after the existing `TableRowSelected`/`TableCursor` block (around line 112):

```go
// Table (inline — no padding, use with fixed-width col() helper in views).
var (
	ColHeader = lipgloss.NewStyle().Foreground(Slate500)
	Col       = lipgloss.NewStyle().Foreground(Slate300)
	ColBold   = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
	ColMuted  = lipgloss.NewStyle().Foreground(Slate500)
	ColAccent = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
)
```

- [ ] **Step 2: Add missing KV extras, padded table cells, and status styles**

Add after the existing `KVValue` declaration:

```go
	KVValueBold = lipgloss.NewStyle().
			Foreground(Slate100).
			Bold(true)

	KVValueMuted = lipgloss.NewStyle().
			Foreground(Slate500)
```

Add padded table cell styles:

```go
// Table (padded — for standalone cells with padding).
var (
	TableCell = lipgloss.NewStyle().
			Foreground(Slate300).
			Padding(0, 1)

	TableCellBold = lipgloss.NewStyle().
			Foreground(Slate200).
			Bold(true).
			Padding(0, 1)

	TableCellMuted = lipgloss.NewStyle().
			Foreground(Slate500).
			Padding(0, 1)
)
```

Add status badge styles:

```go
// Status Badges (inline text, no border).
var (
	BadgeActive      = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
	BadgeClosed      = lipgloss.NewStyle().Foreground(Slate500)
	BadgeDestructive = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
	BadgeWarning     = lipgloss.NewStyle().Foreground(Slate400)
)
```

Add `StateBadge` function:

```go
// StateBadge returns an inline badge style for a state string.
func StateBadge(state string) lipgloss.Style {
	switch state {
	case "active", "open", "bonded", "passed", "valid":
		return BadgeActive
	case "closed", "lost", "unbonded", "rejected", "failed", "revoked":
		return BadgeClosed
	case "paused", "insufficient_funds", "overdrawn", "unbonding":
		return BadgeWarning
	default:
		return BadgeClosed
	}
}
```

- [ ] **Step 3: Add missing palette, dialog, error, progress, and spinner styles**

```go
// Command Palette.
var (
	PaletteBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Slate700).
			Background(Slate900)

	PaletteInput    = lipgloss.NewStyle().Foreground(Slate200).Background(Slate900).Padding(0, 1)
	PalettePrompt   = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
	PaletteItemNormal   = lipgloss.NewStyle().Foreground(Slate300).Padding(0, 1)
	PaletteItemSelected = lipgloss.NewStyle().Foreground(Slate100).Background(Slate800).Bold(true).Padding(0, 1)
	PaletteItemDesc     = lipgloss.NewStyle().Foreground(Slate500)
)

// Confirmation Dialog.
var (
	DialogBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Slate600).
			Background(Slate900).
			Padding(1, 2)

	DialogTitle = lipgloss.NewStyle().Foreground(Slate100).Bold(true)
	DialogBody  = lipgloss.NewStyle().Foreground(Slate400)
	DialogButtonPrimary   = lipgloss.NewStyle().Foreground(Slate950).Background(AccentRed).Bold(true).Padding(0, 2)
	DialogButtonSecondary = lipgloss.NewStyle().Foreground(Slate300).Background(Slate800).Padding(0, 2)
)

// Error Display.
var (
	ErrorLabel      = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
	ErrorMessage    = lipgloss.NewStyle().Foreground(Slate300)
	ErrorSuggestion = lipgloss.NewStyle().Foreground(Slate400)
	ErrorCommand    = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
)

// Progress bar extras.
var (
	ProgressLabel = lipgloss.NewStyle().Foreground(Slate500).Width(14)
	ProgressPct   = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
)
```

Add `SpinnerText` and `SyncFail`:

```go
	SpinnerText = lipgloss.NewStyle().Foreground(Slate400)
```

```go
	SyncFail = lipgloss.NewStyle().Foreground(AccentRed)
```

- [ ] **Step 4: Add HRuleAccent function**

```go
// HRuleAccent returns a full-width accent-colored horizontal rule.
func HRuleAccent(w int) string {
	return lipgloss.NewStyle().Foreground(AccentRed).Render(strings.Repeat("─", w))
}
```

- [ ] **Step 5: Fix the 7 property differences**

Update `HeaderStyle` to add `Foreground(Slate300)`:
```go
	HeaderStyle = lipgloss.NewStyle().
			Background(Slate900).
			Foreground(Slate300).
			Padding(0, 1)
```

Update `HeaderContext` from `Slate200` to `Slate100`:
```go
	HeaderContext = lipgloss.NewStyle().
			Foreground(Slate100).
			Bold(true)
```

Update `HeaderValue` from `Slate200` to `Slate300`:
```go
	HeaderValue = lipgloss.NewStyle().
			Foreground(Slate300)
```

Update `SyncOK` from `GreenColor` to `Slate300`:
```go
	SyncOK = lipgloss.NewStyle().Foreground(Slate300)
```

Update `TableRowSelected` to be background-only (remove Foreground and Bold):
```go
	TableRowSelected = lipgloss.NewStyle().
				Background(Slate800)
```

- [ ] **Step 6: Build and run existing tests**

```bash
cd /Users/amr/go/src/github.com/akash-network/akt
go build ./...
go test ./internal/ui/theme/... -v
```

Some golden file tests will fail due to the property changes. That's expected — we'll update golden files in Task 13.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/theme/theme.go
git commit -m "feat(tui): align theme with prototype — add 40+ missing styles, fix property diffs"
```

---

### Task 2: Add New Message Types

Add message types needed by the new detail views and data fetching.

**Files:**
- Modify: `internal/tui/messages/messages.go`

- [ ] **Step 1: Add new message types**

Append to `messages.go`:

```go
// TallyLoadedMsg carries vote tally results for proposals.
type TallyLoadedMsg struct {
	// Tallies maps proposal ID to its tally result.
	Tallies map[uint64]*govv1.TallyResult
	Err     error
}

// StakingPoolMsg carries the staking pool info (total bonded tokens).
type StakingPoolMsg struct {
	BondedTokens math.Int
	Err          error
}

// BalanceLoadedMsg carries the account balance.
type BalanceLoadedMsg struct {
	Amount string // formatted balance string like "148.52 AKT"
	Err    error
}
```

Add the `math` import: `cosmossdk.io/math`

- [ ] **Step 2: Build**

```bash
go build ./internal/tui/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/messages/messages.go
git commit -m "feat(tui): add message types for tallies, staking pool, and balance"
```

---

### Task 3: Proposal Detail View

Create the proposal detail view with 2 sections: Proposal info + Vote Tally progress bars.

**Files:**
- Create: `internal/tui/views/proposal_detail.go`

**Reference:** Prototype `renderProposalDetail()` at line 2309

- [ ] **Step 1: Create proposal detail view**

Create `internal/tui/views/proposal_detail.go`:

```go
package views

import (
	"fmt"
	"strings"

	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/ui/theme"
)

// ProposalDetailView displays full details for a single governance proposal.
type ProposalDetailView struct {
	proposal *govv1.Proposal
	tally    *govv1.TallyResult
	width    int
	height   int
	scroll   int
}

// NewProposalDetailView creates a new empty proposal detail view.
func NewProposalDetailView() ProposalDetailView {
	return ProposalDetailView{}
}

// SetProposal sets the proposal to display.
func (v *ProposalDetailView) SetProposal(p *govv1.Proposal) {
	v.proposal = p
	v.scroll = 0
}

// SetTally sets the vote tally result.
func (v *ProposalDetailView) SetTally(t *govv1.TallyResult) {
	v.tally = t
}

// SetSize sets the available terminal dimensions.
func (v *ProposalDetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp scrolls the content up.
func (v *ProposalDetailView) ScrollUp() { v.scroll = max(0, v.scroll-1) }

// ScrollDown scrolls the content down.
func (v *ProposalDetailView) ScrollDown() { v.scroll++ }

// View renders the proposal detail.
func (v ProposalDetailView) View() string {
	if v.proposal == nil {
		return theme.Muted.Render("No proposal selected")
	}

	w := v.width
	if w < 40 {
		w = 40
	}

	var b strings.Builder

	// Section 1: Proposal.
	b.WriteString("  " + theme.SectionTitle.Render(fmt.Sprintf("Proposal #%d", v.proposal.Id)) + "\n")
	b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")

	b.WriteString(kv("Title", theme.KVValueBold.Render(truncateStr(v.proposal.Title, w-24))))
	if v.proposal.Summary != "" {
		b.WriteString(kv("Type", theme.KVValue.Render(proposalType(v.proposal))))
	}
	b.WriteString(kv("Status", theme.StateBadge(govStatusLabel(v.proposal.Status.String())).Render(govStatusLabel(v.proposal.Status.String()))))
	if v.proposal.VotingEndTime != nil {
		b.WriteString(kv("Ends", theme.KVValue.Render(formatVotingEnd(*v.proposal.VotingEndTime))))
	}

	b.WriteString("\n")

	// Section 2: Vote Tally.
	b.WriteString("  " + theme.SectionTitle.Render("Vote Tally") + "\n")
	b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")

	if v.tally != nil {
		barW := w - 50
		if barW < 20 {
			barW = 20
		}
		yes, no, abstain, veto := tallyPercentages(v.tally)
		b.WriteString(renderProgressLine("Yes", yes, fmt.Sprintf("%.1f%%", yes), barW))
		b.WriteString(renderProgressLine("No", no, fmt.Sprintf("%.1f%%", no), barW))
		b.WriteString(renderProgressLine("Abstain", abstain, fmt.Sprintf("%.1f%%", abstain), barW))
		b.WriteString(renderProgressLine("No w/ Veto", veto, fmt.Sprintf("%.1f%%", veto), barW))
	} else {
		b.WriteString("    " + theme.Muted.Render("Tally data not available") + "\n")
	}

	// Apply scroll.
	lines := strings.Split(b.String(), "\n")
	visibleH := v.height - 4
	if visibleH < 3 {
		visibleH = 3
	}
	if v.scroll > len(lines)-visibleH {
		v.scroll = max(0, len(lines)-visibleH)
	}
	end := v.scroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[v.scroll:end], "\n")
}

// renderProgressLine renders a labeled progress bar line.
func renderProgressLine(label string, pct float64, detail string, barW int) string {
	filled := int(float64(barW) * pct / 100.0)
	if filled > barW {
		filled = barW
	}
	empty := barW - filled

	bar := theme.BarFilled.Render(strings.Repeat("█", filled)) +
		theme.BarEmpty.Render(strings.Repeat("░", empty))

	return "    " +
		theme.ProgressLabel.Render(fmt.Sprintf("%-14s", label)) +
		bar + " " +
		theme.ProgressPct.Render(detail) + "\n"
}

// tallyPercentages converts tally integers to percentages.
func tallyPercentages(t *govv1.TallyResult) (yes, no, abstain, veto float64) {
	yesI := t.YesCount
	noI := t.NoCount
	abstainI := t.AbstainCount
	vetoI := t.NoWithVetoCount

	yesF, _ := yesI.Float64()
	noF, _ := noI.Float64()
	abstainF, _ := abstainI.Float64()
	vetoF, _ := vetoI.Float64()

	total := yesF + noF + abstainF + vetoF
	if total == 0 {
		return 0, 0, 0, 0
	}
	return yesF / total * 100, noF / total * 100, abstainF / total * 100, vetoF / total * 100
}

// proposalType extracts a human-readable type from the proposal.
func proposalType(p *govv1.Proposal) string {
	if len(p.Messages) > 0 {
		typeURL := p.Messages[0].TypeUrl
		parts := strings.Split(typeURL, ".")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return typeURL
	}
	return "—"
}

// kv renders a KV pair with standard 4-space indent and 16-char label width.
func kv(label, value string) string {
	return "    " + theme.KVLabel.Render(label) + value + "\n"
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/tui/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/views/proposal_detail.go
git commit -m "feat(tui): add proposal detail view with vote tally progress bars"
```

---

### Task 4: Validator Detail View

**Files:**
- Create: `internal/tui/views/validator_detail.go`

**Reference:** Prototype `renderValidatorDetailView()` at line 2426

- [ ] **Step 1: Create validator detail view**

Create `internal/tui/views/validator_detail.go`:

```go
package views

import (
	"fmt"
	"strings"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// ValidatorDetailView displays full details for a single validator.
type ValidatorDetailView struct {
	validator *stakingtypes.Validator
	rank      int
	width     int
	height    int
	scroll    int
}

// NewValidatorDetailView creates a new empty validator detail view.
func NewValidatorDetailView() ValidatorDetailView {
	return ValidatorDetailView{}
}

// SetValidator sets the validator to display.
func (v *ValidatorDetailView) SetValidator(val *stakingtypes.Validator, rank int) {
	v.validator = val
	v.rank = rank
	v.scroll = 0
}

// Validator returns the currently displayed validator, or nil.
func (v ValidatorDetailView) Validator() *stakingtypes.Validator {
	return v.validator
}

// SetSize sets the available terminal dimensions.
func (v *ValidatorDetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp scrolls the content up.
func (v *ValidatorDetailView) ScrollUp() { v.scroll = max(0, v.scroll-1) }

// ScrollDown scrolls the content down.
func (v *ValidatorDetailView) ScrollDown() { v.scroll++ }

// View renders the validator detail.
func (v ValidatorDetailView) View() string {
	if v.validator == nil {
		return theme.Muted.Render("No validator selected")
	}

	w := v.width
	if w < 40 {
		w = 40
	}
	val := v.validator

	var b strings.Builder

	// Section 1: Validator.
	moniker := val.GetMoniker()
	b.WriteString("  " + theme.SectionTitle.Render(moniker) + "\n")
	b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")

	b.WriteString(kv("Rank", theme.KVValueBold.Render(fmt.Sprintf("#%d", v.rank))))
	b.WriteString(kv("Address", theme.KVValue.Render(val.OperatorAddress)))
	b.WriteString(kv("Tokens", theme.KVValue.Render(formatTokens(val.Tokens))))
	b.WriteString(kv("Commission", theme.KVValue.Render(formatCommissionRate(val.Commission.CommissionRates.Rate))))
	b.WriteString(kv("Status", theme.StateBadge(validatorStatusLabel(val)).Render(validatorStatusLabel(val))))
	b.WriteString(kv("Uptime", theme.KVValue.Render("—")))

	b.WriteString("\n")

	// Section 2: Your Delegation (placeholder — requires delegation query).
	b.WriteString("  " + theme.SectionTitle.Render("Your Delegation") + "\n")
	b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")
	b.WriteString(kv("Delegated", theme.KVValue.Render("—")))
	b.WriteString(kv("Rewards", theme.KVValue.Render("—")))
	b.WriteString(kv("Share", theme.KVValueMuted.Render("—")))

	// Apply scroll.
	lines := strings.Split(b.String(), "\n")
	visibleH := v.height - 4
	if visibleH < 3 {
		visibleH = 3
	}
	if v.scroll > len(lines)-visibleH {
		v.scroll = max(0, len(lines)-visibleH)
	}
	end := v.scroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[v.scroll:end], "\n")
}

// validatorStatusLabel returns a human-readable label for the validator status.
func validatorStatusLabel(val *stakingtypes.Validator) string {
	switch val.GetStatus() {
	case stakingtypes.Bonded:
		return "bonded"
	case stakingtypes.Unbonding:
		return "unbonding"
	case stakingtypes.Unbonded:
		return "unbonded"
	default:
		return "unknown"
	}
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/tui/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/views/validator_detail.go
git commit -m "feat(tui): add validator detail view with delegation placeholder"
```

---

### Task 5: Lease Detail View

**Files:**
- Create: `internal/tui/views/lease_detail.go`

**Reference:** Prototype `renderLeaseDetail()` at line 1805

- [ ] **Step 1: Create lease detail view**

Create `internal/tui/views/lease_detail.go`:

```go
package views

import (
	"fmt"
	"strings"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/ui/theme"
)

// LeaseDetailView displays full details for a single lease.
type LeaseDetailView struct {
	lease  *store.LeaseRecord
	width  int
	height int
	scroll int
}

// NewLeaseDetailView creates a new empty lease detail view.
func NewLeaseDetailView() LeaseDetailView {
	return LeaseDetailView{}
}

// SetLease sets the lease record to display.
func (v *LeaseDetailView) SetLease(l *store.LeaseRecord) {
	v.lease = l
	v.scroll = 0
}

// Lease returns the currently displayed lease record, or nil.
func (v LeaseDetailView) Lease() *store.LeaseRecord {
	return v.lease
}

// SetSize sets the available terminal dimensions.
func (v *LeaseDetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp scrolls the content up.
func (v *LeaseDetailView) ScrollUp() { v.scroll = max(0, v.scroll-1) }

// ScrollDown scrolls the content down.
func (v *LeaseDetailView) ScrollDown() { v.scroll++ }

// View renders the lease detail.
func (v LeaseDetailView) View() string {
	if v.lease == nil {
		return theme.Muted.Render("No lease selected")
	}

	w := v.width
	if w < 40 {
		w = 40
	}
	l := v.lease

	var b strings.Builder

	// Section 1: Lease.
	b.WriteString("  " + theme.SectionTitle.Render("Lease") + "\n")
	b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")

	b.WriteString(kv("DSEQ", theme.KVValueBold.Render(fmt.Sprintf("%d", l.ID.DSeq))))
	b.WriteString(kv("GSEQ/OSEQ", theme.KVValue.Render(fmt.Sprintf("%d/%d", l.ID.GSeq, l.ID.OSeq))))
	b.WriteString(kv("State", theme.StateBadge(l.State).Render(l.State)))
	b.WriteString(kv("Provider", theme.KVValue.Render(l.ID.Provider)))
	b.WriteString(kv("Price", theme.KVValue.Render(l.Price+"/block")))
	if l.CreatedAt > 0 {
		age := relativeTime(l.CreatedAt)
		b.WriteString(kv("Age", theme.KVValue.Render(age)))
	}

	b.WriteString("\n")

	// Section 2: Order.
	b.WriteString("  " + theme.SectionTitle.Render("Order") + "\n")
	b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")

	orderID := fmt.Sprintf("%d/%d/%d", l.ID.DSeq, l.ID.GSeq, l.ID.OSeq)
	bidID := fmt.Sprintf("%d/%d/%d/%s", l.ID.DSeq, l.ID.GSeq, l.ID.OSeq, truncateAddr(l.ID.Provider))
	b.WriteString(kv("Order ID", theme.KVValueBold.Render(orderID)))
	b.WriteString(kv("Bid ID", theme.KVValue.Render(bidID)))
	b.WriteString(kv("Order State", theme.StateBadge("matched").Render("matched")))

	b.WriteString("\n")

	// Section 3: Settlement (data not yet available — show dashes).
	b.WriteString("  " + theme.SectionTitle.Render("Settlement") + "\n")
	b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")
	b.WriteString(kv("Last Settled", theme.KVValue.Render("—")))
	b.WriteString(kv("Settled Amt", theme.KVValue.Render("—")))
	b.WriteString(kv("Funds Left", theme.KVValueBold.Render("—")))
	b.WriteString(kv("Withdrawn", theme.KVValue.Render("—")))

	b.WriteString("\n")

	// Section 4: Provider Status (requires live provider query — show dashes).
	b.WriteString("  " + theme.SectionTitle.Render("Provider Status") + "\n")
	b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")
	b.WriteString(kv("Status", theme.KVValue.Render("—")))

	// Endpoints.
	if len(l.Endpoints) > 0 {
		b.WriteString("\n")
		b.WriteString("  " + theme.SectionTitle.Render("Endpoints") + "\n")
		b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")
		for _, ep := range l.Endpoints {
			b.WriteString("    " + theme.KVLabel.Render(ep.Service) + theme.KVValue.Render(ep.URI) + "\n")
		}
	}

	// Apply scroll.
	lines := strings.Split(b.String(), "\n")
	visibleH := v.height - 4
	if visibleH < 3 {
		visibleH = 3
	}
	if v.scroll > len(lines)-visibleH {
		v.scroll = max(0, len(lines)-visibleH)
	}
	end := v.scroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[v.scroll:end], "\n")
}

// truncateAddr truncates an address to the first 12 chars + "...".
func truncateAddr(addr string) string {
	if len(addr) <= 15 {
		return addr
	}
	return addr[:12] + "..."
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/tui/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/views/lease_detail.go
git commit -m "feat(tui): add lease detail view with order and settlement sections"
```

---

### Task 6: Provider Detail View

**Files:**
- Create: `internal/tui/views/provider_detail.go`

**Reference:** Prototype `renderProviderDetailView()` at line 1932

- [ ] **Step 1: Create provider detail view**

Create `internal/tui/views/provider_detail.go`:

```go
package views

import (
	"fmt"
	"strings"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// ProviderDetailView displays full details for a single provider.
type ProviderDetailView struct {
	provider *ptypes.Provider
	width    int
	height   int
	scroll   int
}

// NewProviderDetailView creates a new empty provider detail view.
func NewProviderDetailView() ProviderDetailView {
	return ProviderDetailView{}
}

// SetProvider sets the provider to display.
func (v *ProviderDetailView) SetProvider(p *ptypes.Provider) {
	v.provider = p
	v.scroll = 0
}

// Provider returns the currently displayed provider, or nil.
func (v ProviderDetailView) Provider() *ptypes.Provider {
	return v.provider
}

// SetSize sets the available terminal dimensions.
func (v *ProviderDetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp scrolls the content up.
func (v *ProviderDetailView) ScrollUp() { v.scroll = max(0, v.scroll-1) }

// ScrollDown scrolls the content down.
func (v *ProviderDetailView) ScrollDown() { v.scroll++ }

// View renders the provider detail.
func (v ProviderDetailView) View() string {
	if v.provider == nil {
		return theme.Muted.Render("No provider selected")
	}

	w := v.width
	if w < 40 {
		w = 40
	}
	p := v.provider

	var b strings.Builder

	// Section 1: Provider.
	b.WriteString("  " + theme.SectionTitle.Render("Provider") + "\n")
	b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")

	b.WriteString(kv("Address", theme.KVValue.Render(p.Owner)))
	b.WriteString(kv("URL", theme.KVValueBold.Render(p.HostURI)))

	region := attrValue(p.Attributes, "region")
	if region == "" {
		region = "—"
	}
	b.WriteString(kv("Region", theme.KVValue.Render(region)))
	b.WriteString(kv("Status", theme.StateBadge("active").Render("—")))

	b.WriteString("\n")

	// Section 2: Attributes.
	if len(p.Attributes) > 0 {
		b.WriteString("  " + theme.SectionTitle.Render("Attributes") + "\n")
		b.WriteString("  " + theme.HRuleAccent(w-4) + "\n")
		for _, attr := range p.Attributes {
			b.WriteString(kv(attr.Key, theme.KVValue.Render(attr.Value)))
		}
	}

	// Apply scroll.
	lines := strings.Split(b.String(), "\n")
	visibleH := v.height - 4
	if visibleH < 3 {
		visibleH = 3
	}
	if v.scroll > len(lines)-visibleH {
		v.scroll = max(0, len(lines)-visibleH)
	}
	end := v.scroll + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[v.scroll:end], "\n")
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/tui/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/views/provider_detail.go
git commit -m "feat(tui): add provider detail view with attributes section"
```

---

### Task 7: Populate Governance Tallies and Staking VP%

**Files:**
- Modify: `internal/tui/views/governance.go`
- Modify: `internal/tui/views/staking.go`

- [ ] **Step 1: Add tally data to governance view**

In `governance.go`, add a field and setter for tallies:

```go
type GovernanceView struct {
	table    components.ResourceTable
	items    []*govv1.Proposal
	tallies  map[uint64]*govv1.TallyResult // NEW
	filter   string
	width    int
	height   int
}

func (v *GovernanceView) SetTallies(tallies map[uint64]*govv1.TallyResult) {
	v.tallies = tallies
	v.rebuildRows()
}

// Tallies returns the current tally map (may be nil).
func (v *GovernanceView) Tallies() map[uint64]*govv1.TallyResult {
	return v.tallies
}
```

Update the row-building logic in `SetData` (and new `rebuildRows`) to populate tally columns. Replace the hardcoded `"—"` for YES/NO/ABSTAIN/VETO with:

```go
func (v *GovernanceView) tallyCell(propID uint64, field string) string {
	if v.tallies == nil {
		return "—"
	}
	t, ok := v.tallies[propID]
	if !ok || t == nil {
		return "—"
	}
	yes, no, abstain, veto := tallyPercentages(t)
	switch field {
	case "yes":
		return fmt.Sprintf("%.1f%%", yes)
	case "no":
		return fmt.Sprintf("%.1f%%", no)
	case "abstain":
		return fmt.Sprintf("%.1f%%", abstain)
	case "veto":
		return fmt.Sprintf("%.1f%%", veto)
	}
	return "—"
}
```

Move `tallyPercentages` to a shared location or import from proposal_detail.go. Since both views are in the same package, the function is already accessible.

- [ ] **Step 2: Add VP% to staking view**

In `staking.go`, add a field for total bonded tokens:

```go
type StakingView struct {
	table        components.ResourceTable
	items        []stakingtypes.Validator
	totalBonded  math.Int // NEW
	filter       string
	width        int
	height       int
}

func (v *StakingView) SetTotalBonded(total math.Int) {
	v.totalBonded = total
	v.rebuildRows()
}
```

Update row building to calculate VP%:

```go
vpPct := "—"
if !v.totalBonded.IsZero() {
	pct := val.Tokens.ToLegacyDec().Quo(v.totalBonded.ToLegacyDec()).MulInt64(100)
	vpPct = fmt.Sprintf("%.2f%%", pct.MustFloat64())
}
```

- [ ] **Step 3: Build**

```bash
go build ./internal/tui/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/views/governance.go internal/tui/views/staking.go
git commit -m "feat(tui): populate governance tallies and staking VP% columns"
```

---

### Task 8: Dashboard Redesign

Replace the flat KV dashboard with the prototype's card-based layout.

**Files:**
- Rewrite: `internal/tui/views/dashboard.go`
- Modify: `internal/tui/views/dashboard_test.go`

**Reference:** Prototype `renderDashboard()` at line 1422

- [ ] **Step 1: Rewrite dashboard**

This is a full rewrite of `dashboard.go`. The new dashboard has:

1. **3 summary cards** across the top using `lipgloss.JoinHorizontal`
2. **Recent Deployments mini-table** with DSEQ/STATE/IMAGE/PRICE/ESCROW/AGE columns (max 5 rows)
3. **Network status strip** as a single row of inline KV pairs

Key data setters to add: `SetBalance(amount string)`, `SetProposalCount(voting, total int)`, `SetValidatorCount(active int)`.

The view no longer shows Welcome banner, Account section, or Shortcuts section.

The card rendering uses `lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Slate700).Padding(0, 1).Width(cardW)`.

- [ ] **Step 2: Delete old golden files and run tests to regenerate**

```bash
rm -f internal/tui/testdata/TestAppRenderDashboard.golden
go test ./internal/tui/... -update
```

- [ ] **Step 3: Build and verify**

```bash
go build ./internal/tui/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/views/dashboard.go internal/tui/views/dashboard_test.go internal/tui/testdata/
git commit -m "feat(tui): redesign dashboard with summary cards and mini-table"
```

---

### Task 9: Wire Detail View Navigation in app.go

Add new view IDs, instantiate detail views, and wire Enter/Esc navigation.

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/views/providers.go` (add `SelectedProvider()`)
- Modify: `internal/tui/views/staking.go` (add `SelectedIndex()`)

**Prerequisites — add missing accessor methods:**

Before wiring navigation, add `SelectedProvider()` to `providers.go`:

```go
// SelectedProvider returns the provider at the cursor, or nil.
func (v *ProvidersView) SelectedProvider() *ptypes.Provider {
	row := v.table.SelectedRow()
	if row == nil {
		return nil
	}
	for i := range v.items {
		if v.items[i].Owner == row.ID {
			return &v.items[i]
		}
	}
	return nil
}
```

This requires storing items in the view. Add a field `items ptypes.Providers` to `ProvidersView` and set it in `SetData()`.

Add `SelectedIndex()` to `staking.go`:

```go
// SelectedIndex returns the 0-based index of the cursor in the validators slice.
func (v *StakingView) SelectedIndex() int {
	return v.table.Cursor()
}
```

Also add a `truncateStr` helper to the `views` package (in `proposal_detail.go` or a shared `helpers.go` file):

```go
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
```

- [ ] **Step 1: Add new view IDs**

In the `activeView` const block, add after `viewDeploymentDetail`:

```go
	viewLeaseDetail
	viewProviderDetail
	viewProposalDetail
	viewValidatorDetail
```

- [ ] **Step 2: Add detail view fields to App struct**

After the existing `deploymentDetail` field:

```go
	leaseDetail      views.LeaseDetailView
	providerDetail   views.ProviderDetailView
	proposalDetail   views.ProposalDetailView
	validatorDetail  views.ValidatorDetailView
```

- [ ] **Step 3: Initialize in newApp()**

In the `newApp` function, add initialization:

```go
	leaseDetail:     views.NewLeaseDetailView(),
	providerDetail:  views.NewProviderDetailView(),
	proposalDetail:  views.NewProposalDetailView(),
	validatorDetail: views.NewValidatorDetailView(),
```

- [ ] **Step 4: Wire Enter navigation for each list view**

In the `Update()` method, replace the lease Enter toast with:

```go
case key.Matches(kmsg, a.keys.Select):
	rec := a.leases.SelectedRecord()
	if rec != nil {
		a.leaseDetail.SetLease(rec)
		a.leaseDetail.SetSize(a.width, a.height-chromeHeight)
		a.view = viewLeaseDetail
	}
	return a, nil
```

Add providers Enter:

```go
if a.view == viewProviders {
	if key.Matches(kmsg, a.keys.Select) {
		// Provider detail from chain data.
		p := a.providers.SelectedProvider()
		if p != nil {
			a.providerDetail.SetProvider(p)
			a.providerDetail.SetSize(a.width, a.height-chromeHeight)
			a.view = viewProviderDetail
		}
		return a, nil
	}
}
```

Add governance Enter:

```go
if a.view == viewGovernance {
	if key.Matches(kmsg, a.keys.Select) {
		p := a.governance.SelectedProposal()
		if p != nil {
			a.proposalDetail.SetProposal(p)
			if tallies := a.governance.Tallies(); tallies != nil {
				a.proposalDetail.SetTally(tallies[p.Id])
			}
			a.proposalDetail.SetSize(a.width, a.height-chromeHeight)
			a.view = viewProposalDetail
		}
		return a, nil
	}
}
```

Add staking Enter:

```go
if a.view == viewStaking {
	if key.Matches(kmsg, a.keys.Select) {
		val := a.staking.SelectedValidator()
		if val != nil {
			rank := a.staking.SelectedIndex() + 1
			a.validatorDetail.SetValidator(val, rank)
			a.validatorDetail.SetSize(a.width, a.height-chromeHeight)
			a.view = viewValidatorDetail
		}
		return a, nil
	}
}
```

- [ ] **Step 5: Wire Esc to go back from detail views**

Add Esc handlers for each new detail view, and also add j/k scroll:

```go
if a.view == viewLeaseDetail || a.view == viewProviderDetail ||
	a.view == viewProposalDetail || a.view == viewValidatorDetail {
	switch {
	case key.Matches(kmsg, a.keys.Back):
		switch a.view {
		case viewLeaseDetail:
			a.view = viewLeases
		case viewProviderDetail:
			a.view = viewProviders
		case viewProposalDetail:
			a.view = viewGovernance
		case viewValidatorDetail:
			a.view = viewStaking
		}
		return a, nil
	case key.Matches(kmsg, a.keys.CursorDown):
		switch a.view {
		case viewLeaseDetail:
			a.leaseDetail.ScrollDown()
		case viewProviderDetail:
			a.providerDetail.ScrollDown()
		case viewProposalDetail:
			a.proposalDetail.ScrollDown()
		case viewValidatorDetail:
			a.validatorDetail.ScrollDown()
		}
		return a, nil
	case key.Matches(kmsg, a.keys.CursorUp):
		switch a.view {
		case viewLeaseDetail:
			a.leaseDetail.ScrollUp()
		case viewProviderDetail:
			a.providerDetail.ScrollUp()
		case viewProposalDetail:
			a.proposalDetail.ScrollUp()
		case viewValidatorDetail:
			a.validatorDetail.ScrollUp()
		}
		return a, nil
	}
}
```

- [ ] **Step 6: Add View() rendering cases**

In the `View()` method's switch statement, add:

```go
case viewLeaseDetail:
	main = a.leaseDetail.View()
case viewProviderDetail:
	main = a.providerDetail.View()
case viewProposalDetail:
	main = a.proposalDetail.View()
case viewValidatorDetail:
	main = a.validatorDetail.View()
```

- [ ] **Step 7: Update breadcrumb for new detail views**

In `renderBreadcrumb()`, add cases for the new detail views.

- [ ] **Step 8: Update footer hints for new detail views**

In `renderFooter()`, add hint sets for each new detail view.

- [ ] **Step 9: Update resize() to propagate sizes to new views**

- [ ] **Step 10: Build**

```bash
go build ./internal/tui/...
```

- [ ] **Step 11: Commit**

```bash
git add internal/tui/app.go
git commit -m "feat(tui): wire detail view navigation for leases, providers, proposals, validators"
```

---

### Task 10: Add Data-Fetching Commands for Tallies and Pool

Wire the new data-fetching commands in app.go so the governance tallies and staking VP% get populated.

**Files:**
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Add tally-loading command**

```go
func loadTallies(client aclient.LightClient, proposals []*govv1.Proposal) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return messages.TallyLoadedMsg{Err: fmt.Errorf("no chain client")}
		}
		tallies := make(map[uint64]*govv1.TallyResult)
		qc := govv1.NewQueryClient(client.ClientContext())
		for _, p := range proposals {
			if p.Status == govv1.StatusVotingPeriod {
				resp, err := qc.TallyResult(context.Background(), &govv1.QueryTallyResultRequest{
					ProposalId: p.Id,
				})
				if err == nil && resp != nil {
					tallies[p.Id] = resp.Tally
				}
			}
		}
		return messages.TallyLoadedMsg{Tallies: tallies}
	}
}
```

- [ ] **Step 2: Add staking pool loading command**

```go
func loadStakingPool(client aclient.LightClient) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return messages.StakingPoolMsg{Err: fmt.Errorf("no chain client")}
		}
		qc := stakingtypes.NewQueryClient(client.ClientContext())
		resp, err := qc.Pool(context.Background(), &stakingtypes.QueryPoolRequest{})
		if err != nil {
			return messages.StakingPoolMsg{Err: err}
		}
		return messages.StakingPoolMsg{BondedTokens: resp.Pool.BondedTokens}
	}
}
```

- [ ] **Step 3: Handle new messages in Update()**

Add handlers in the Update() method:

```go
case messages.TallyLoadedMsg:
	if msg.Err == nil {
		a.governance.SetTallies(msg.Tallies)
	}
	return a, nil

case messages.StakingPoolMsg:
	if msg.Err == nil {
		a.staking.SetTotalBonded(msg.BondedTokens)
	}
	return a, nil
```

- [ ] **Step 4: Dispatch tally loading after proposals load**

In the `ProposalsLoadedMsg` handler, after setting data, dispatch tally loading:

```go
case messages.ProposalsLoadedMsg:
	if msg.Err == nil {
		a.governance.SetData(msg.Proposals)
		return a, loadTallies(a.lightClient, msg.Proposals)
	}
```

- [ ] **Step 5: Dispatch pool loading after validators load**

In the `ValidatorsLoadedMsg` handler:

```go
case messages.ValidatorsLoadedMsg:
	if msg.Err == nil {
		a.staking.SetData(msg.Validators)
		return a, loadStakingPool(a.lightClient)
	}
```

- [ ] **Step 6: Build**

```bash
go build ./internal/tui/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/tui/app.go
git commit -m "feat(tui): add data-fetching for governance tallies and staking pool"
```

---

### Task 11: Overlay Compositing Fix

Implement proper overlay compositing so overlays float over the base content instead of replacing it.

**Files:**
- Modify: `internal/tui/app.go`

- [ ] **Step 1: Add overlayCenter function**

Add the `overlayCenter` function from the prototype:

```go
// overlayCenter composites an overlay string centered on top of a base view.
// Overlay lines replace the corresponding base lines at the center position.
func overlayCenter(base, overlay string, w, h int) string {
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < h {
		baseLines = append(baseLines, "")
	}

	overlayLines := strings.Split(overlay, "\n")
	overlayH := len(overlayLines)
	overlayW := 0
	for _, line := range overlayLines {
		if lw := lipgloss.Width(line); lw > overlayW {
			overlayW = lw
		}
	}

	startY := (h - overlayH) / 2
	if startY < 2 {
		startY = 2
	}
	startX := (w - overlayW) / 2
	if startX < 0 {
		startX = 0
	}

	for i, line := range overlayLines {
		row := startY + i
		if row < len(baseLines) {
			baseLines[row] = strings.Repeat(" ", startX) + line
		}
	}

	if len(baseLines) > h {
		baseLines = baseLines[:h]
	}
	return strings.Join(baseLines, "\n")
}
```

- [ ] **Step 2: Update View() to use overlayCenter for overlays**

In the `View()` method, change the overlay rendering from:

```go
if a.confirmDialog.Active() {
	main = a.confirmDialog.View()
}
```

to:

```go
if a.confirmDialog.Active() {
	main = overlayCenter(main, a.confirmDialog.View(), a.width, contentH)
}
```

Apply the same pattern for help overlay and command palette:

```go
if a.helpOverlay.Active() {
	main = overlayCenter(main, a.helpOverlay.View(), a.width, contentH)
}
if a.palette.Active() {
	main = overlayCenter(main, a.palette.View(), a.width, contentH)
	footer = a.renderPaletteFooter()
}
```

Note: The log viewer should stay as full replacement since it's designed to fill the screen.

- [ ] **Step 3: Build**

```bash
go build ./internal/tui/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tui/app.go
git commit -m "feat(tui): implement overlay compositing — overlays float over base content"
```

---

### Task 12: Complete Oracle Panel in Monitor

Fill the TODO in the Oracle panel rendering.

**Files:**
- Modify: `internal/monitor/ui/view.go`

**Reference:** Prototype `renderMonitorOracleBME()` at line 2187

- [ ] **Step 1: Implement oracle price rendering**

In `view.go`, find the `renderOraclePanel` function and replace the TODO block with actual rendering. When `m.oracle.Aggregated` has entries, render per-denom rows:

```go
func renderOraclePanel(vc ViewContext) string {
	var b strings.Builder

	b.WriteString(sectionTitleStyle.Render("Oracle Prices") + "\n")
	b.WriteString(theme.HRule(vc.Width/2 - 4) + "\n")

	if len(vc.OracleState.Aggregated) == 0 {
		b.WriteString("  " + mutedStyle.Render("Waiting for oracle data...") + "\n")
		return b.String()
	}

	for denom, agg := range vc.OracleState.Aggregated {
		price := pretty.FormatDecimal(agg.Price)
		b.WriteString("  " + theme.KVLabel.Render(denom) + theme.KVValueBold.Render(price) + "\n")
	}

	return b.String()
}
```

The exact implementation depends on the `Aggregated` map type — adapt to match the actual `OracleState` field types in `internal/monitor/ui/state.go`.

- [ ] **Step 2: Build**

```bash
go build ./internal/monitor/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/monitor/ui/view.go
git commit -m "feat(monitor): render oracle aggregated prices in Oracle/BME dashboard"
```

---

### Task 13: Update Golden Files and Final Verification

Update all golden test files to match the new rendering.

**Files:**
- Modify: Various `testdata/*.golden` files

- [ ] **Step 1: Run all tests with update flag**

```bash
cd /Users/amr/go/src/github.com/akash-network/akt
go test ./internal/tui/... -update -count=1
go test ./internal/monitor/ui/... -update -count=1
go test ./internal/ui/theme/... -update -count=1
```

- [ ] **Step 2: Review the golden file diffs**

```bash
git diff --stat internal/tui/testdata/ internal/monitor/ui/testdata/
```

Verify the changes make sense — new views should have golden files, existing views should have updates reflecting theme changes.

- [ ] **Step 3: Run full test suite**

```bash
go test ./... 2>&1 | tail -20
```

All tests should pass.

- [ ] **Step 4: Build the binary and smoke test**

```bash
go build -o akt ./cmd/akt
./akt --help
```

- [ ] **Step 5: Commit all golden files**

```bash
git add -A internal/tui/testdata/ internal/monitor/ui/testdata/ internal/ui/theme/
git commit -m "test: update golden files for TUI design parity rework"
```

- [ ] **Step 6: Final squash or fixup (optional)**

Review the commit history and squash if desired:

```bash
git log --oneline -15
```

---

## Task Dependencies

```
Task 1 (Theme) ──┬── Task 3 (Proposal Detail) ──┐
                  ├── Task 4 (Validator Detail) ──┤
                  ├── Task 5 (Lease Detail) ──────┤
                  ├── Task 6 (Provider Detail) ───┤
                  │                                ├── Task 9 (Wire Navigation)
Task 2 (Messages) ┤                               │
                  ├── Task 7 (Populate Data) ──────┤── Task 10 (Data Commands)
                  │                                │
                  └── Task 8 (Dashboard) ──────────┤
                                                   │
                            Task 11 (Overlays) ────┤
                            Task 12 (Oracle) ──────┤
                                                   │
                            Task 13 (Golden Files) ─┘
```

**Parallel opportunities:**
- Tasks 3, 4, 5, 6 (detail views) can all run in parallel after Task 1
- Task 7 (populate data) can run in parallel with detail views
- Task 8 (dashboard) can run in parallel with detail views
- Task 11 (overlays) and Task 12 (oracle) are independent of each other
- Task 13 must be last
