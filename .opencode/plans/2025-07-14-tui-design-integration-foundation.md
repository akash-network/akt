# TUI Design Integration — Foundation (Theme + Components + Shell)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply the JSX/Go prototype design system to the existing akt TUI — replacing the color palette, adding shared reusable components, and updating the app shell to match the prototype's visual design.

**Architecture:** Bottom-up integration in 3 layers: (1) Theme overhaul replaces current 256-color tokens with the prototype's Zinc scale + Red accent true-color palette. (2) Shared components extracted into `internal/tui/components/` provide reusable table, state tag, KV detail, confirmation dialog, and toast primitives. (3) Shell updates (header, nav bar, breadcrumb, footer) align the existing `app.go` with the prototype's layout. The monitor views (`internal/monitor/ui/`) are migrated to use the new theme via their existing style aliases in `styles.go`.

**Tech Stack:** Go, Bubbletea v2, Lipgloss v2, Bubbles v2

**Design Reference:** `design/prototype/akash-tui-v2/DESIGN.md` (rendering rules, color tokens), `design/prototype/tui-data.jsx` (color constants `C`), `design/prototype/tui-views.jsx` (JSX wireframes — primary visual authority)

---

## File Structure

### Modified Files
| File | Responsibility | Changes |
|------|---------------|---------|
| `internal/ui/theme/theme.go` | Unified color palette + base styles | Full rewrite: Zinc scale colors, Red accent, 5-level typography, state colors, component styles |
| `internal/ui/theme/theme_test.go` | Theme unit tests | Update for new color tokens and style names |
| `internal/monitor/ui/styles.go` | Monitor style aliases | Update aliases to reference new theme exports |
| `internal/tui/app.go` | Root TUI model | Replace header/nav/footer rendering with prototype layout |
| `internal/tui/keymap.go` | Keybinding definitions | Minor: add deploy shortcut key `D` |
| `internal/tui/views/help.go` | Help overlay | Restyle with new theme tokens |
| `internal/tui/views/command.go` | Command palette | Restyle with new theme tokens (Red prompt, Slate800 selected) |
| `internal/tui/views/listview.go` | Generic list component | Replace with prototype's fixed-width column table pattern |
| `internal/tui/commands/registry.go` | Command registry | Add missing commands from JSX prototype (Deploy from SDL, wallet, logs, shell) |

### New Files
| File | Responsibility |
|------|---------------|
| `internal/tui/components/table.go` | Reusable resource table: fixed-width columns, `▸` cursor, state tags, selected row highlight |
| `internal/tui/components/table_test.go` | Table component unit tests |
| `internal/tui/components/statetag.go` | Inline `│state│` tags with color mapping per SPEC §10.6 |
| `internal/tui/components/statetag_test.go` | State tag tests |
| `internal/tui/components/kvdetail.go` | Section title + red rule + label:value pair renderer |
| `internal/tui/components/kvdetail_test.go` | KV detail tests |
| `internal/tui/components/confirm.go` | Modal confirmation dialog overlay (close/vote/delegate/unbond/redelegate) |
| `internal/tui/components/confirm_test.go` | Confirm dialog tests |
| `internal/tui/components/toast.go` | Transient bottom-right notification |
| `internal/tui/components/toast_test.go` | Toast tests |
| `internal/tui/components/progress.go` | Shared progress bar (█/░ blocks) extracted from monitor |
| `internal/tui/components/footer.go` | Context-sensitive footer hint bar renderer |
| `internal/tui/components/footer_test.go` | Footer tests |

---

## Task 1: Theme Overhaul

**Files:**
- Modify: `internal/ui/theme/theme.go`
- Modify: `internal/ui/theme/theme_test.go`

This task replaces the entire color palette and style definitions. The current theme uses 256-color codes. The prototype uses true-color hex values for a Zinc/neutral scale with a single Red accent.

- [ ] **Step 1: Read the current theme file**

Read `internal/ui/theme/theme.go` and `internal/ui/theme/theme_test.go` to understand the current exports and test expectations.

- [ ] **Step 2: Write failing tests for the new color tokens**

Add tests to `internal/ui/theme/theme_test.go` that verify the new Zinc scale colors exist and have the expected hex values. The test should verify:
- All 11 Zinc scale colors (Slate950 through Slate50) are defined
- Accent colors (Red, RedDim, RedBg) are defined
- State colors (Green, GreenDim, Yellow, YellowDim) are defined
- Style exports for each typography level exist

```go
func TestZincScaleColors(t *testing.T) {
    // Verify the Zinc scale colors are the correct hex values
    colors := map[string]lipgloss.Color{
        "Slate950": theme.Slate950,
        "Slate900": theme.Slate900,
        "Slate800": theme.Slate800,
        "Slate700": theme.Slate700,
        "Slate600": theme.Slate600,
        "Slate500": theme.Slate500,
        "Slate400": theme.Slate400,
        "Slate300": theme.Slate300,
        "Slate200": theme.Slate200,
        "Slate100": theme.Slate100,
        "Slate50":  theme.Slate50,
    }
    for name, c := range colors {
        if c == "" {
            t.Errorf("color %s is empty", name)
        }
    }
}

func TestAccentColors(t *testing.T) {
    if theme.Red == "" {
        t.Error("Red accent color is empty")
    }
    if theme.RedDim == "" {
        t.Error("RedDim color is empty")
    }
    if theme.RedBg == "" {
        t.Error("RedBg color is empty")
    }
}

func TestStateColors(t *testing.T) {
    if theme.Green == "" {
        t.Error("Green state color is empty")
    }
    if theme.Yellow == "" {
        t.Error("Yellow state color is empty")
    }
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/ui/theme/ -v -run TestZincScale`
Expected: FAIL — `Slate950` etc. not defined

- [ ] **Step 4: Rewrite theme.go with the new design system**

Replace the entire content of `internal/ui/theme/theme.go` with the Zinc scale palette, Red accent, state colors, and all lipgloss styles. The exact color values come from `design/prototype/akash-tui-v2/DESIGN.md`:

**Color tokens to define:**
```go
// Zinc scale (neutral grays)
Slate950 = lipgloss.Color("#09090b") // App background
Slate900 = lipgloss.Color("#18181b") // Card/panel/overlay background
Slate800 = lipgloss.Color("#27272a") // Elevated surface, selected row
Slate700 = lipgloss.Color("#3f3f46") // Borders, horizontal rules
Slate600 = lipgloss.Color("#52525b") // Muted borders, subtle dividers
Slate500 = lipgloss.Color("#71717a") // Column headers, muted labels
Slate400 = lipgloss.Color("#a1a1aa") // Secondary text, inactive nav
Slate300 = lipgloss.Color("#d4d4d8") // Body text, table values
Slate200 = lipgloss.Color("#e4e4e7") // Primary text, bold values
Slate100 = lipgloss.Color("#f4f4f5") // Headings, section titles
Slate50  = lipgloss.Color("#fafafa") // Maximum emphasis

// Accent
Red    = lipgloss.Color("#ff4136") // Primary accent
RedDim = lipgloss.Color("#b91c1c") // Muted accent (destructive tag border)
RedBg  = lipgloss.Color("#1c0a09") // Destructive pill background

// State colors
Green    = lipgloss.Color("#22c55e") // active, open, bonded, passed
GreenDim = lipgloss.Color("#166534") // Green state tag border
Yellow    = lipgloss.Color("#eab308") // paused, insufficient_funds, unbonding
YellowDim = lipgloss.Color("#854d0e") // Yellow state tag border
Blue   = lipgloss.Color("#65b3ff")
Purple = lipgloss.Color("#c08cff")
```

**Styles to define (matching prototype DESIGN.md):**
```go
// Typography hierarchy
Heading      // Slate100, bold
PrimaryValue // Slate200, bold
Body         // Slate300
Secondary    // Slate400
Muted        // Slate500

// Header bar
HeaderStyle    // Background Slate900, full-width
HeaderAppName  // Red, bold
HeaderContext  // Slate200, bold
HeaderMeta     // Slate500
HeaderValue    // Slate200
SyncOK         // Green

// Nav tabs
NavTabActive   // Background Red, Foreground Slate950, bold, Padding(0,1)
NavTabInactive // Foreground Slate400, Padding(0,1)

// Breadcrumb
BreadcrumbSegment   // Slate500
BreadcrumbActive    // Slate200, bold
BreadcrumbSeparator // Slate600

// Footer
FooterKey  // Slate400, bold
FooterDesc // Slate600

// Table
TableHeader      // Slate500, no bold
TableRow         // Slate300
TableRowSelected // Background Slate800, Slate200, bold
TableCursor      // Red, bold ("▸")

// KV detail
SectionTitle // Slate100, bold
SectionRule  // Red (accent rule under section headings)
KVLabel      // Slate500, width 16
KVValue      // Slate200

// Panel/card
PanelBorder  // Slate700
PanelBg      // Slate900

// State tags (exported for component use)
StateGreen      // Green fg, GreenDim border
StateYellow     // Yellow fg, YellowDim border
StateClosed     // Slate500 fg, Slate700 border

// Overlay
OverlayBg      // Slate900
OverlayBorder  // Slate700

// Buttons
ButtonPrimary   // Background Red, Foreground Slate950, bold
ButtonSecondary // Background Slate800, Foreground Slate300

// Progress bar colors
BarFilled // Slate200
BarEmpty  // Slate700
```

**Backward compatibility:** The old color names (`ColorPrimary`, `ColorSuccess`, etc.) and old style names (`Bold`, `Success`, `TabActive`, `SectionHeader`, etc.) must be preserved as **aliases** pointing to the new tokens so the monitor code doesn't break until it's updated. Add a `// Deprecated: use X instead` comment on each alias.

**Must also preserve and update:**
- `VoteYes` / `VoteNo` → map to Green/Red from new palette
- `GridVoted` / `GridNotVoted` → Green/Slate500
- `ProgressPrimary` / `ProgressSuccess` / `ProgressPrecommit` → new palette colors
- `DetailHeader`, `DetailLabel`, `DetailValue` → SectionTitle, KVLabel, KVValue aliases

**Helper function:** Add `HRule(w int) string` that renders a full-width horizontal rule in Slate700 (the prototype uses this pattern).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ui/theme/ -v`
Expected: All tests PASS

- [ ] **Step 6: Run full test suite to check nothing is broken**

Run: `go test ./internal/... 2>&1 | head -50`
Expected: Tests pass. Monitor golden tests will likely fail — that's expected and handled in Task 2.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/theme/
git commit -m "feat(theme): replace color palette with Zinc scale + Red accent design system

Rewrite the unified theme package with the prototype's design system:
- Zinc neutral scale (#09090b → #fafafa) replaces 256-color tokens
- Red accent (#ff4136) as single accent color
- State colors (green/yellow/gray) for inline tags
- Typography hierarchy (Heading/PrimaryValue/Body/Secondary/Muted)
- Header, nav, breadcrumb, footer, table, KV detail styles
- Backward-compatible aliases for existing monitor code

Refs: design/prototype/akash-tui-v2/DESIGN.md"
```

---

## Task 2: Update Monitor Style Aliases

**Files:**
- Modify: `internal/monitor/ui/styles.go`

The monitor's `styles.go` defines local aliases to `theme.*` exports. After Task 1, some old theme names are deprecated aliases. Update to use the new canonical names.

- [ ] **Step 1: Read the current styles.go**

Read `internal/monitor/ui/styles.go` to see all current aliases.

- [ ] **Step 2: Update aliases to use new theme token names**

Replace the deprecated references with the new canonical names:

| Old reference | New reference |
|---|---|
| `theme.Title` | `theme.Heading` |
| `theme.SectionHeader` | `theme.SectionTitle` |
| `theme.Label` | `theme.KVLabel` |
| `theme.Value` | `theme.KVValue` |
| `theme.HelpBar` | `theme.FooterDesc` (or create a new `HelpBar` alias) |
| `theme.StatusBar` | `theme.Secondary` |
| `theme.TabActive` | `theme.NavTabActive` |
| `theme.TabInactive` | `theme.NavTabInactive` |
| `theme.DetailHeader` | `theme.SectionTitle` |
| `theme.DetailLabel` | `theme.KVLabel` |
| `theme.DetailValue` | `theme.KVValue` |

Keep `ProgressBar`, `FormatPercent`, `DoubleProgressBar`, `FormatVoteGrid` functions — their logic stays the same, just the colors they reference change via the theme.

- [ ] **Step 3: Regenerate golden test files**

Run: `go test ./internal/monitor/ui/ -update`

This regenerates all ~70 golden files with the new colors. Then verify:

Run: `go test ./internal/monitor/ui/ -v`
Expected: All PASS

- [ ] **Step 4: Verify full build**

Run: `make akt`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add internal/monitor/ui/styles.go internal/monitor/ui/testdata/
git commit -m "refactor(monitor): update style aliases to new theme tokens

Migrate monitor styles from deprecated theme names to the new Zinc
scale token names. Regenerate golden test files with new colors."
```

---

## Task 3: State Tag Component

**Files:**
- Create: `internal/tui/components/statetag.go`
- Create: `internal/tui/components/statetag_test.go`

Inline state tags for table rows: `│active│`, `│closed│` with color mapping per SPEC §10.6. Uses Unicode `│` (box-drawing) characters as borders — NOT lipgloss `RoundedBorder()` which creates 3-line boxes and breaks table row alignment. This is a critical rendering rule from the Go prototype's DESIGN.md.

- [ ] **Step 1: Write failing tests**

Create `internal/tui/components/statetag_test.go`:

```go
package components

import (
    "testing"

    "charm.land/lipgloss/v2"
)

func TestStateTag(t *testing.T) {
    tag := StateTag("active")
    if tag == "" {
        t.Fatal("StateTag returned empty string")
    }
    // Should contain the state name
    if !containsText(tag, "active") {
        t.Errorf("StateTag should contain 'active', got: %s", tag)
    }
}

func TestStateTagWidth(t *testing.T) {
    // Width should be label length + 2 (for │ borders)
    w := StateTagWidth("active")
    if w != 8 { // len("active") + 2
        t.Errorf("expected width 8, got %d", w)
    }
}

func TestStateTagAbbreviation(t *testing.T) {
    tag := StateTag("insufficient_funds")
    if !containsText(tag, "low funds") {
        t.Errorf("insufficient_funds should abbreviate to 'low funds', got: %s", tag)
    }
}

func TestStateTagColorMapping(t *testing.T) {
    // Green states
    for _, s := range []string{"active", "open", "bonded", "passed", "valid"} {
        tag := StateTag(s)
        if tag == "" {
            t.Errorf("StateTag(%q) returned empty", s)
        }
    }
    // Yellow states
    for _, s := range []string{"paused", "insufficient_funds", "unbonding", "voting_period"} {
        tag := StateTag(s)
        if tag == "" {
            t.Errorf("StateTag(%q) returned empty", s)
        }
    }
    // Gray/closed states
    for _, s := range []string{"closed", "rejected", "failed", "revoked"} {
        tag := StateTag(s)
        if tag == "" {
            t.Errorf("StateTag(%q) returned empty", s)
        }
    }
}

// containsText strips ANSI sequences and checks for substring
func containsText(s, sub string) bool {
    plain := lipgloss.Strip(s)
    return strings.Contains(plain, sub)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/components/ -v -run TestStateTag`
Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: Implement statetag.go**

Create `internal/tui/components/statetag.go`:

```go
package components

import (
    "charm.land/lipgloss/v2"
    "pkg.akt.dev/akt/internal/ui/theme"
)

// stateCategory maps state names to color categories.
type stateCategory int

const (
    stateGreen stateCategory = iota
    stateYellow
    stateClosed
)

var stateMap = map[string]stateCategory{
    "active": stateGreen, "open": stateGreen, "bonded": stateGreen,
    "passed": stateGreen, "valid": stateGreen, "matched": stateGreen,
    "paused": stateYellow, "insufficient_funds": stateYellow,
    "overdrawn": stateYellow, "unbonding": stateYellow,
    "voting_period": stateYellow, "deposit_period": stateYellow,
    "pending": stateYellow,
    "closed": stateClosed, "lost": stateClosed, "unbonded": stateClosed,
    "rejected": stateClosed, "failed": stateClosed, "jailed": stateClosed,
    "revoked": stateClosed, "invalid": stateClosed,
}

var abbreviations = map[string]string{
    "insufficient_funds": "low funds",
    "voting_period":      "voting",
    "deposit_period":     "deposit",
}

// StateTag renders an inline state tag: │active│
// Uses Unicode box-drawing │ as borders (NOT lipgloss RoundedBorder).
func StateTag(state string) string {
    label := state
    if abbr, ok := abbreviations[state]; ok {
        label = abbr
    }

    cat, ok := stateMap[state]
    if !ok {
        cat = stateClosed
    }

    var textStyle, borderStyle lipgloss.Style
    switch cat {
    case stateGreen:
        textStyle = lipgloss.NewStyle().Foreground(theme.Green)
        borderStyle = lipgloss.NewStyle().Foreground(theme.GreenDim)
    case stateYellow:
        textStyle = lipgloss.NewStyle().Foreground(theme.Yellow)
        borderStyle = lipgloss.NewStyle().Foreground(theme.YellowDim)
    default:
        textStyle = lipgloss.NewStyle().Foreground(theme.Slate500)
        borderStyle = lipgloss.NewStyle().Foreground(theme.Slate700)
    }

    return borderStyle.Render("│") + textStyle.Render(label) + borderStyle.Render("│")
}

// StateTagWidth returns the display width of a state tag.
func StateTagWidth(state string) int {
    label := state
    if abbr, ok := abbreviations[state]; ok {
        label = abbr
    }
    return len(label) + 2 // +2 for │ borders
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/components/ -v -run TestStateTag`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/components/statetag.go internal/tui/components/statetag_test.go
git commit -m "feat(tui): add state tag component with color-mapped inline borders

Renders inline │state│ tags using Unicode box-drawing borders.
Color mapping per SPEC §10.6: green (active/open/bonded), yellow
(paused/insufficient_funds/unbonding), gray (closed/rejected/failed).
Abbreviates long state names (insufficient_funds → low funds)."
```

---

## Task 4: KV Detail Component

**Files:**
- Create: `internal/tui/components/kvdetail.go`
- Create: `internal/tui/components/kvdetail_test.go`

Renders section title + red accent rule + label:value pairs. Used by all detail views.

- [ ] **Step 1: Write failing tests**

Create `internal/tui/components/kvdetail_test.go`:

```go
package components

import (
    "strings"
    "testing"

    "charm.land/lipgloss/v2"
)

func TestSection(t *testing.T) {
    out := Section("Deployment", 60)
    if out == "" {
        t.Fatal("Section returned empty string")
    }
    plain := lipgloss.Strip(out)
    if !strings.Contains(plain, "Deployment") {
        t.Error("Section should contain title text")
    }
}

func TestKV(t *testing.T) {
    out := KV("owner", "akash1abc...def")
    if out == "" {
        t.Fatal("KV returned empty string")
    }
    plain := lipgloss.Strip(out)
    if !strings.Contains(plain, "owner") {
        t.Error("KV should contain label")
    }
    if !strings.Contains(plain, "akash1abc...def") {
        t.Error("KV should contain value")
    }
}

func TestKVBlock(t *testing.T) {
    pairs := []KVPair{
        {Label: "owner", Value: "akash1abc"},
        {Label: "dseq", Value: "12345"},
        {Label: "state", Value: "active"},
    }
    out := KVBlock(pairs)
    plain := lipgloss.Strip(out)
    if !strings.Contains(plain, "owner") || !strings.Contains(plain, "12345") {
        t.Error("KVBlock should contain all pairs")
    }
}
```

- [ ] **Step 2: Run tests — expect failure**

Run: `go test ./internal/tui/components/ -v -run TestSection`
Expected: FAIL

- [ ] **Step 3: Implement kvdetail.go**

Create `internal/tui/components/kvdetail.go`:

```go
package components

import (
    "fmt"
    "strings"

    "charm.land/lipgloss/v2"
    "pkg.akt.dev/akt/internal/ui/theme"
)

const kvLabelWidth = 16

// KVPair represents a label-value pair for detail views.
type KVPair struct {
    Label string
    Value string
}

// Section renders a section heading with a red accent rule.
//   Section Title
//   ─────────────
func Section(title string, width int) string {
    heading := theme.SectionTitle.Render(title)
    ruleWidth := width
    if ruleWidth < 4 {
        ruleWidth = 4
    }
    rule := lipgloss.NewStyle().Foreground(theme.Red).Render(strings.Repeat("─", ruleWidth))
    return heading + "\n" + rule
}

// KV renders a single label:value pair.
//   label          value
func KV(label, value string) string {
    l := theme.KVLabel.Render(fmt.Sprintf("%-*s", kvLabelWidth, label))
    v := theme.KVValue.Render(value)
    return l + v
}

// KVMuted renders a KV pair where the value is muted (Slate500).
func KVMuted(label, value string) string {
    l := theme.KVLabel.Render(fmt.Sprintf("%-*s", kvLabelWidth, label))
    v := theme.Muted.Render(value)
    return l + v
}

// KVBold renders a KV pair where the value is bold (Slate100).
func KVBold(label, value string) string {
    l := theme.KVLabel.Render(fmt.Sprintf("%-*s", kvLabelWidth, label))
    v := theme.Heading.Render(value)
    return l + v
}

// KVBlock renders multiple KV pairs as a block, one per line.
func KVBlock(pairs []KVPair) string {
    var b strings.Builder
    for i, p := range pairs {
        b.WriteString(KV(p.Label, p.Value))
        if i < len(pairs)-1 {
            b.WriteByte('\n')
        }
    }
    return b.String()
}

// SectionWithKV renders a full section: title + rule + KV pairs.
func SectionWithKV(title string, width int, pairs []KVPair) string {
    var b strings.Builder
    b.WriteString(Section(title, width))
    b.WriteByte('\n')
    b.WriteString(KVBlock(pairs))
    return b.String()
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/components/ -v -run "TestSection|TestKV"`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/components/kvdetail.go internal/tui/components/kvdetail_test.go
git commit -m "feat(tui): add KV detail section component

Renders section title with red accent rule + label:value pairs.
Label width fixed at 16 chars. Supports muted/bold value variants.
Used by all detail views (deployment, lease, provider, validator)."
```

---

## Task 5: Footer Hint Component

**Files:**
- Create: `internal/tui/components/footer.go`
- Create: `internal/tui/components/footer_test.go`

Context-sensitive footer key hints. The prototype renders these as: bold key (Slate400) + description (Slate600), with 2-space gaps between pairs.

- [ ] **Step 1: Write failing tests**

Create `internal/tui/components/footer_test.go`:

```go
package components

import (
    "strings"
    "testing"

    "charm.land/lipgloss/v2"
)

func TestFooterHints(t *testing.T) {
    hints := []HintPair{
        {Key: "j/k", Desc: "move"},
        {Key: "↵", Desc: "open"},
        {Key: "esc", Desc: "back"},
    }
    out := FooterHints(hints)
    plain := lipgloss.Strip(out)
    if !strings.Contains(plain, "j/k") || !strings.Contains(plain, "move") {
        t.Error("FooterHints should contain key and description")
    }
}

func TestFooterHintsWithAccent(t *testing.T) {
    hints := []HintPair{
        {Key: "D", Desc: "new", Accent: true},
    }
    out := FooterHints(hints)
    if out == "" {
        t.Fatal("FooterHints returned empty")
    }
}

func TestHRule(t *testing.T) {
    rule := HRule(40)
    plain := lipgloss.Strip(rule)
    if len(plain) != 40 {
        t.Errorf("HRule(40) should produce 40 chars, got %d", len(plain))
    }
}
```

- [ ] **Step 2: Run tests — expect failure**

Run: `go test ./internal/tui/components/ -v -run "TestFooter|TestHRule"`
Expected: FAIL

- [ ] **Step 3: Implement footer.go**

Create `internal/tui/components/footer.go`:

```go
package components

import (
    "strings"

    "charm.land/lipgloss/v2"
    "pkg.akt.dev/akt/internal/ui/theme"
)

// HintPair represents a key-description pair for footer hints.
type HintPair struct {
    Key    string
    Desc   string
    Accent bool // if true, render key in Red instead of Slate400
}

// FooterHints renders a row of key hints: key desc  key desc  key desc
func FooterHints(hints []HintPair) string {
    var parts []string
    for _, h := range hints {
        var keyStyle lipgloss.Style
        if h.Accent {
            keyStyle = lipgloss.NewStyle().Foreground(theme.Red).Bold(true)
        } else {
            keyStyle = theme.FooterKey
        }
        parts = append(parts, keyStyle.Render(h.Key)+" "+theme.FooterDesc.Render(h.Desc))
    }
    return strings.Join(parts, "  ")
}

// HRule renders a full-width horizontal rule in Slate700.
func HRule(width int) string {
    return lipgloss.NewStyle().Foreground(theme.Slate700).Render(strings.Repeat("─", width))
}

// Footer renders a complete footer: horizontal rule + context-sensitive hints.
func Footer(width int, hints []HintPair) string {
    return HRule(width) + "\n" + FooterHints(hints)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/components/ -v -run "TestFooter|TestHRule"`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/components/footer.go internal/tui/components/footer_test.go
git commit -m "feat(tui): add footer hint and horizontal rule components

Renders context-sensitive key hints (bold key + muted desc, 2-space gaps).
HRule renders a full-width Slate700 horizontal rule.
Footer combines both. Accent keys rendered in Red for destructive actions."
```

---

## Task 6: Resource Table Component

**Files:**
- Create: `internal/tui/components/table.go`
- Create: `internal/tui/components/table_test.go`

This is the most critical shared component. The prototype uses a specific rendering approach: **fixed-width columns via `fmt.Sprintf("%-*s")`**, NOT lipgloss padding. This prevents row wrapping — a lesson documented in the Go prototype's `DESIGN.md`.

- [ ] **Step 1: Write failing tests**

Create `internal/tui/components/table_test.go`:

```go
package components

import (
    "strings"
    "testing"

    "charm.land/lipgloss/v2"
)

func TestResourceTableRender(t *testing.T) {
    tbl := NewResourceTable(ResourceTableConfig{
        Columns: []TableColumn{
            {Header: "DSEQ", Width: 10, Align: AlignLeft},
            {Header: "STATE", Width: 10, Align: AlignLeft},
            {Header: "PROVIDER", Width: 20, Align: AlignLeft},
        },
    })
    tbl.SetRows([]TableRow{
        {Cells: []string{"18542", "active", "akash1abc...def"}},
        {Cells: []string{"18539", "closed", "akash1xyz...ghi"}},
    })
    tbl.SetSize(60, 20)

    out := tbl.View()
    if out == "" {
        t.Fatal("ResourceTable rendered empty string")
    }
    plain := lipgloss.Strip(out)
    if !strings.Contains(plain, "DSEQ") {
        t.Error("should contain column headers")
    }
    if !strings.Contains(plain, "18542") {
        t.Error("should contain first row data")
    }
}

func TestResourceTableCursor(t *testing.T) {
    tbl := NewResourceTable(ResourceTableConfig{
        Columns: []TableColumn{
            {Header: "NAME", Width: 20, Align: AlignLeft},
        },
    })
    tbl.SetRows([]TableRow{
        {Cells: []string{"first"}},
        {Cells: []string{"second"}},
        {Cells: []string{"third"}},
    })
    tbl.SetSize(40, 20)

    if tbl.SelectedIndex() != 0 {
        t.Error("initial cursor should be 0")
    }

    tbl.CursorDown()
    if tbl.SelectedIndex() != 1 {
        t.Error("cursor should be 1 after CursorDown")
    }

    tbl.CursorUp()
    if tbl.SelectedIndex() != 0 {
        t.Error("cursor should be 0 after CursorUp")
    }
}

func TestResourceTableEmpty(t *testing.T) {
    tbl := NewResourceTable(ResourceTableConfig{
        Columns: []TableColumn{
            {Header: "NAME", Width: 20, Align: AlignLeft},
        },
        EmptyText: "No items found",
    })
    tbl.SetSize(40, 20)

    out := tbl.View()
    plain := lipgloss.Strip(out)
    if !strings.Contains(plain, "No items found") {
        t.Error("empty table should show empty text")
    }
}
```

- [ ] **Step 2: Run tests — expect failure**

Run: `go test ./internal/tui/components/ -v -run TestResourceTable`
Expected: FAIL

- [ ] **Step 3: Implement table.go**

Create `internal/tui/components/table.go`. This must follow the prototype's rendering rules exactly:
- Column headers in Slate500
- `▸ ` cursor in Red bold for selected row
- `  ` (2 spaces) for unselected rows
- Selected row gets Background(Slate800), text Slate200 bold
- Unselected row text in Slate300
- State column values rendered with `StateTag()`
- All column widths fixed via `fmt.Sprintf("%-*s")`
- Scrolling with viewport when rows exceed visible area

```go
package components

import (
    "fmt"
    "strings"

    "charm.land/lipgloss/v2"
    "pkg.akt.dev/akt/internal/ui/theme"
)

// Alignment for table columns.
type Alignment int

const (
    AlignLeft Alignment = iota
    AlignRight
)

// TableColumn defines a column in a resource table.
type TableColumn struct {
    Header string
    Width  int       // character width (0 = fill remaining)
    Align  Alignment
    // RenderFunc optionally overrides cell rendering (e.g., for state tags).
    // If nil, the cell value is rendered as plain text.
    RenderFunc func(value string) string
}

// TableRow represents a single data row.
type TableRow struct {
    Cells []string
    ID    string // optional identifier for selection messages
}

// ResourceTableConfig configures a resource table.
type ResourceTableConfig struct {
    Columns   []TableColumn
    EmptyText string // shown when there are no rows
}

// ResourceTable is a reusable table component following the prototype's
// fixed-width column rendering pattern.
type ResourceTable struct {
    config ResourceTableConfig
    rows   []TableRow
    cursor int
    offset int // scroll offset
    width  int
    height int
}

// NewResourceTable creates a new resource table.
func NewResourceTable(cfg ResourceTableConfig) ResourceTable {
    if cfg.EmptyText == "" {
        cfg.EmptyText = "No items"
    }
    return ResourceTable{config: cfg}
}

// SetRows replaces table data.
func (t *ResourceTable) SetRows(rows []TableRow) {
    t.rows = rows
    if t.cursor >= len(rows) {
        t.cursor = max(0, len(rows)-1)
    }
    t.ensureVisible()
}

// SetSize sets the available rendering area.
func (t *ResourceTable) SetSize(w, h int) {
    t.width = w
    t.height = h
}

// SelectedIndex returns the current cursor position.
func (t *ResourceTable) SelectedIndex() int { return t.cursor }

// SelectedRow returns the currently selected row, or nil.
func (t *ResourceTable) SelectedRow() *TableRow {
    if t.cursor < len(t.rows) {
        return &t.rows[t.cursor]
    }
    return nil
}

// CursorDown moves the cursor down.
func (t *ResourceTable) CursorDown() {
    if t.cursor < len(t.rows)-1 {
        t.cursor++
        t.ensureVisible()
    }
}

// CursorUp moves the cursor up.
func (t *ResourceTable) CursorUp() {
    if t.cursor > 0 {
        t.cursor--
        t.ensureVisible()
    }
}

// CursorTop jumps to the first row.
func (t *ResourceTable) CursorTop() {
    t.cursor = 0
    t.offset = 0
}

// CursorBottom jumps to the last row.
func (t *ResourceTable) CursorBottom() {
    t.cursor = max(0, len(t.rows)-1)
    t.ensureVisible()
}

func (t *ResourceTable) ensureVisible() {
    visibleRows := t.visibleRows()
    if visibleRows <= 0 {
        return
    }
    if t.cursor < t.offset {
        t.offset = t.cursor
    }
    if t.cursor >= t.offset+visibleRows {
        t.offset = t.cursor - visibleRows + 1
    }
}

func (t *ResourceTable) visibleRows() int {
    // height minus header (1) minus header rule (1) minus bottom rule (1) minus count line (1) = h-4
    return max(1, t.height-4)
}

// View renders the table.
func (t ResourceTable) View() string {
    if len(t.rows) == 0 {
        return t.renderEmpty()
    }

    var b strings.Builder
    w := t.width
    cols := t.config.Columns

    // Resolve fill columns (Width=0 gets remaining space)
    resolvedWidths := t.resolveWidths(w)

    // Header row
    headerStyle := lipgloss.NewStyle().Foreground(theme.Slate500)
    b.WriteString("  ") // cursor column placeholder
    for i, c := range cols {
        b.WriteString(col(headerStyle, resolvedWidths[i], c.Header, c.Align))
        if i < len(cols)-1 {
            b.WriteString(" ")
        }
    }
    b.WriteByte('\n')

    // Header rule
    b.WriteString(lipgloss.NewStyle().Foreground(theme.Slate700).Render(strings.Repeat("─", w)))
    b.WriteByte('\n')

    // Data rows
    visible := t.visibleRows()
    end := min(t.offset+visible, len(t.rows))
    for i := t.offset; i < end; i++ {
        row := t.rows[i]
        selected := i == t.cursor

        // Cursor indicator
        if selected {
            b.WriteString(lipgloss.NewStyle().Foreground(theme.Red).Bold(true).Render("▸ "))
        } else {
            b.WriteString("  ")
        }

        // Cell values
        var cellStyle lipgloss.Style
        if selected {
            cellStyle = lipgloss.NewStyle().Foreground(theme.Slate200).Bold(true)
        } else {
            cellStyle = lipgloss.NewStyle().Foreground(theme.Slate300)
        }

        for j, c := range cols {
            cellValue := ""
            if j < len(row.Cells) {
                cellValue = row.Cells[j]
            }

            if c.RenderFunc != nil {
                // Custom renderer (e.g., state tags)
                rendered := c.RenderFunc(cellValue)
                // Pad to column width after rendering
                renderedWidth := lipgloss.Width(rendered)
                pad := resolvedWidths[j] - renderedWidth
                if pad > 0 {
                    b.WriteString(rendered + strings.Repeat(" ", pad))
                } else {
                    b.WriteString(rendered)
                }
            } else {
                b.WriteString(col(cellStyle, resolvedWidths[j], cellValue, c.Align))
            }
            if j < len(cols)-1 {
                b.WriteString(" ")
            }
        }

        // Apply selected row background
        // Note: We build the row string first, then apply background
        if i < end-1 {
            b.WriteByte('\n')
        }
    }
    b.WriteByte('\n')

    // Bottom rule
    b.WriteString(lipgloss.NewStyle().Foreground(theme.Slate700).Render(strings.Repeat("─", w)))
    b.WriteByte('\n')

    // Row count
    countStyle := lipgloss.NewStyle().Foreground(theme.Slate500)
    b.WriteString(countStyle.Render(fmt.Sprintf("  %d items", len(t.rows))))

    return b.String()
}

func (t ResourceTable) renderEmpty() string {
    emptyStyle := lipgloss.NewStyle().Foreground(theme.Slate500)
    return emptyStyle.Render(t.config.EmptyText)
}

func (t ResourceTable) resolveWidths(totalWidth int) []int {
    widths := make([]int, len(t.config.Columns))
    used := 2 // cursor column
    fillCount := 0
    for i, c := range t.config.Columns {
        if c.Width > 0 {
            widths[i] = c.Width
            used += c.Width + 1 // +1 for gap
        } else {
            fillCount++
        }
    }
    remaining := totalWidth - used
    if fillCount > 0 && remaining > 0 {
        each := remaining / fillCount
        for i, c := range t.config.Columns {
            if c.Width == 0 {
                widths[i] = max(8, each)
            }
        }
    }
    return widths
}

// col renders a cell value with fixed width. Uses fmt.Sprintf, NOT lipgloss padding.
func col(style lipgloss.Style, width int, text string, align Alignment) string {
    // Truncate if too long
    if len(text) > width {
        text = text[:width-1] + "…"
    }
    var padded string
    if align == AlignRight {
        padded = fmt.Sprintf("%*s", width, text)
    } else {
        padded = fmt.Sprintf("%-*s", width, text)
    }
    return style.Render(padded)
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/components/ -v -run TestResourceTable`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/components/table.go internal/tui/components/table_test.go
git commit -m "feat(tui): add resource table component with fixed-width columns

Reusable table following the prototype's rendering pattern:
- Fixed-width columns via fmt.Sprintf (NOT lipgloss padding)
- ▸ cursor in Red, selected row in Slate800 background
- State column support via RenderFunc
- Scroll viewport when rows exceed visible area
- Column headers in Slate500, body in Slate300"
```

---

## Task 7: Confirmation Dialog Component

**Files:**
- Create: `internal/tui/components/confirm.go`
- Create: `internal/tui/components/confirm_test.go`

Modal overlay for destructive actions. 5 variants: close deployment, vote, delegate, unbond, redelegate. The JSX prototype (`tui-overlays.jsx`) defines the layout: title + body + optional inputs (vote options, amount input) + fee preview + Cancel/Confirm buttons.

- [ ] **Step 1: Write failing tests**

Create `internal/tui/components/confirm_test.go`:

```go
package components

import (
    "strings"
    "testing"

    "charm.land/lipgloss/v2"
)

func TestConfirmDialogClose(t *testing.T) {
    d := NewConfirmDialog(ConfirmClose, ConfirmData{
        Title:   "Close deployment 18542?",
        Body:    "This will terminate all active leases.",
        Danger:  true,
        Fee:     "0.0142 AKT",
        Account: "akash1abc...def",
    })
    d.SetSize(80, 40)
    out := d.View()
    plain := lipgloss.Strip(out)
    if !strings.Contains(plain, "Close deployment") {
        t.Error("should contain title")
    }
    if !strings.Contains(plain, "cancel") && !strings.Contains(plain, "Cancel") {
        t.Error("should contain cancel action")
    }
}

func TestConfirmDialogVote(t *testing.T) {
    d := NewConfirmDialog(ConfirmVote, ConfirmData{
        Title: "Vote on proposal #91",
        Body:  "Update min deposit to 1,000 AKT",
    })
    d.SetSize(80, 40)
    out := d.View()
    plain := lipgloss.Strip(out)
    if !strings.Contains(plain, "yes") && !strings.Contains(plain, "Yes") {
        t.Error("vote dialog should show vote options")
    }
}

func TestConfirmDialogToggle(t *testing.T) {
    d := NewConfirmDialog(ConfirmClose, ConfirmData{
        Title: "Test",
        Body:  "Test body",
    })
    if d.Active() {
        t.Error("should start inactive")
    }
    d.Open()
    if !d.Active() {
        t.Error("should be active after Open")
    }
    d.Close()
    if d.Active() {
        t.Error("should be inactive after Close")
    }
}
```

- [ ] **Step 2: Run tests — expect failure**

Run: `go test ./internal/tui/components/ -v -run TestConfirmDialog`
Expected: FAIL

- [ ] **Step 3: Implement confirm.go**

Create `internal/tui/components/confirm.go` with:
- `ConfirmKind` enum: `ConfirmClose`, `ConfirmVote`, `ConfirmDelegate`, `ConfirmUnbond`, `ConfirmRedelegate`
- `ConfirmData` struct with Title, Body, Danger bool, Fee, Account fields
- `ConfirmDialog` struct with kind, data, active, cursor (Cancel=0/Confirm=1), voteCursor (for vote variant), width, height
- `NewConfirmDialog`, `Open`, `Close`, `Active`, `SetSize`, `Update`, `View` methods
- `ConfirmMsg` and `CancelMsg` bubbletea message types
- Overlay rendering: centered box (width ~480 chars → ~60 cols), `RoundedBorder()` in Slate600 (for dialogs this is OK — only state tags must avoid it), Background Slate900
- Fee preview section with KV pairs
- Vote variant: 4 selectable options (y=yes, n=no, a=abstain, v=veto) in a grid
- Delegate/Unbond variant: amount input field

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/components/ -v -run TestConfirmDialog`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/components/confirm.go internal/tui/components/confirm_test.go
git commit -m "feat(tui): add confirmation dialog overlay component

Modal confirmation dialog with 5 variants:
- Close deployment (destructive, red border)
- Vote (4-option grid: yes/no/abstain/veto)
- Delegate (amount input)
- Unbond (amount input, destructive)
- Redelegate (amount input)
All include fee preview section and Cancel/Confirm buttons."
```

---

## Task 8: Toast Notification Component

**Files:**
- Create: `internal/tui/components/toast.go`
- Create: `internal/tui/components/toast_test.go`

Transient notification that appears bottom-right and auto-dismisses after ~2.5 seconds. From the JSX prototype (`tui-overlays.jsx`, `Toast` function): 3 tones (ok=green, info=blue, err=red) with icon + message.

- [ ] **Step 1: Write failing tests**

```go
package components

import (
    "testing"
    "time"
)

func TestToastCreation(t *testing.T) {
    toast := NewToast("Deployment closed", ToastOK)
    if toast.Message != "Deployment closed" {
        t.Error("wrong message")
    }
    if toast.Tone != ToastOK {
        t.Error("wrong tone")
    }
}

func TestToastExpiry(t *testing.T) {
    toast := NewToast("test", ToastInfo)
    if toast.Expired() {
        t.Error("fresh toast should not be expired")
    }
    // Simulate time passage
    toast.CreatedAt = time.Now().Add(-3 * time.Second)
    if !toast.Expired() {
        t.Error("toast should be expired after 3 seconds")
    }
}
```

- [ ] **Step 2: Run tests — expect failure**

- [ ] **Step 3: Implement toast.go**

```go
package components

import (
    "time"

    "charm.land/lipgloss/v2"
    "pkg.akt.dev/akt/internal/ui/theme"
)

const toastDuration = 2500 * time.Millisecond

// ToastTone indicates the visual style of a toast.
type ToastTone int

const (
    ToastOK ToastTone = iota
    ToastInfo
    ToastError
)

// Toast represents a transient notification.
type Toast struct {
    Message   string
    Tone      ToastTone
    CreatedAt time.Time
}

// NewToast creates a new toast notification.
func NewToast(message string, tone ToastTone) Toast {
    return Toast{
        Message:   message,
        Tone:      tone,
        CreatedAt: time.Now(),
    }
}

// Expired returns true if the toast has passed its display duration.
func (t Toast) Expired() bool {
    return time.Since(t.CreatedAt) > toastDuration
}

// View renders the toast notification.
func (t Toast) View() string {
    var icon string
    var borderColor lipgloss.Color

    switch t.Tone {
    case ToastOK:
        icon = lipgloss.NewStyle().Foreground(theme.Green).Render("✓")
        borderColor = theme.Green
    case ToastInfo:
        icon = lipgloss.NewStyle().Foreground(theme.Blue).Render("ℹ")
        borderColor = theme.Blue
    case ToastError:
        icon = lipgloss.NewStyle().Foreground(theme.Red).Render("✗")
        borderColor = theme.Red
    }

    style := lipgloss.NewStyle().
        Background(theme.Slate900).
        Border(lipgloss.RoundedBorder()).
        BorderForeground(borderColor).
        BorderLeft(true).
        BorderRight(true).
        BorderTop(true).
        BorderBottom(true).
        Padding(0, 1).
        MaxWidth(50)

    return style.Render(icon + " " + lipgloss.NewStyle().Foreground(theme.Slate300).Render(t.Message))
}

// ToastExpiredMsg is sent when a toast expires and should be removed.
type ToastExpiredMsg struct{}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/components/ -v -run TestToast`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/components/toast.go internal/tui/components/toast_test.go
git commit -m "feat(tui): add toast notification component

Transient bottom-right notifications with 3 tones (ok/info/error).
Auto-expires after 2.5 seconds. Icon + message in bordered box."
```

---

## Task 9: Progress Bar Component

**Files:**
- Create: `internal/tui/components/progress.go`

Extract the progress bar rendering from `internal/monitor/ui/styles.go` into a shared component. The monitor will import from here.

- [ ] **Step 1: Create shared progress.go**

Extract `ProgressBar`, `FormatPercent`, `ProgressBarWithLabel`, `DoubleProgressBar` from `internal/monitor/ui/styles.go` into `internal/tui/components/progress.go`. The functions stay identical — only the package changes and the import of theme colors.

- [ ] **Step 2: Update monitor/ui/styles.go to delegate**

Replace the progress bar functions in `internal/monitor/ui/styles.go` with thin wrappers that call the shared component:

```go
func ProgressBar(percent float64, width int) string {
    return components.ProgressBar(percent, width)
}
```

- [ ] **Step 3: Run monitor tests**

Run: `go test ./internal/monitor/ui/ -v`
Expected: All PASS (no behavior change)

- [ ] **Step 4: Commit**

```bash
git add internal/tui/components/progress.go internal/monitor/ui/styles.go
git commit -m "refactor(tui): extract progress bar into shared component

Move ProgressBar, FormatPercent, DoubleProgressBar from monitor/ui/styles
into tui/components/progress for reuse across all TUI views.
Monitor delegates to shared component — no behavior change."
```

---

## Task 10: Update App Shell (Header + Nav + Breadcrumb + Footer)

**Files:**
- Modify: `internal/tui/app.go`

This is the main visual integration task for the shell. Update the header, nav bar, breadcrumb, and footer rendering to match the prototype.

- [ ] **Step 1: Read current app.go rendering methods**

Read `internal/tui/app.go` focusing on `renderHeader()`, `renderStatusBar()`, and the `View()` method.

- [ ] **Step 2: Update renderHeader()**

Replace the current header with the prototype's layout:
- Full-width background: Slate900
- Left: `akt` (Red bold) · `prod:akashnet-2` (context bold, chain muted) · `alice akash1abc…def` (account bold, address muted)
- Right: `⎡ 18,234,567 ⎤` (block height) + `● synced` (green)
- Manual gap calculation (spaces between left and right)

Use the new theme styles: `theme.HeaderStyle`, `theme.HeaderAppName`, `theme.HeaderContext`, `theme.HeaderMeta`, `theme.HeaderValue`, `theme.SyncOK`.

- [ ] **Step 3: Add renderNavBar() method**

New method that renders the primary nav tab bar from the prototype:
- 6 tabs: `1 Deployments`, `2 Leases`, `3 Providers`, `4 Monitor`, `5 Governance`, `6 Staking`
- Active tab: `theme.NavTabActive` (Red bg, Slate950 fg, bold, padding 0,1)
- Inactive: `theme.NavTabInactive` (Slate400, padding 0,1)
- `D` deploy button on the right in Red accent
- Below: full-width `HRule(width)` in Slate700

Map active tab from `a.view` enum.

- [ ] **Step 4: Add renderBreadcrumb() method**

New method that renders navigation breadcrumbs from the prototype:
- Separator: ` / ` in Slate600
- Segments: Slate500
- Active (last) segment: Slate200 bold
- Build path from current view state

- [ ] **Step 5: Update View() to compose new layout**

Update the `View()` method to compose:
```
header
navBar
breadcrumb
content (existing view dispatch)
[vertical padding to push footer to bottom]
footer (from components.Footer with view-specific hints)
```

Overlays (palette, help, confirm, toast) render on top.

- [ ] **Step 6: Update renderStatusBar() → use components.Footer**

Replace the current 3-line status bar with `components.Footer(width, hints)` where hints are view-specific. Map each `activeView` to its hint pairs (matching the Go prototype's `renderFooter` method).

- [ ] **Step 7: Run tests**

Run: `go test ./internal/tui/ -v`
Expected: Existing tests pass (may need updates for the new rendering)

- [ ] **Step 8: Verify build and manual test**

Run: `make akt && .cache/bin/akt --help`
Expected: Build succeeds

- [ ] **Step 9: Commit**

```bash
git add internal/tui/app.go
git commit -m "feat(tui): update app shell to match prototype design

Redesign header (app name · context · account · block · sync),
add nav tab bar (6 tabs with Red active pill + D deploy button),
add breadcrumb trail, replace status bar with context-sensitive
footer hints. Layout: header → nav → breadcrumb → content → footer."
```

---

## Task 11: Update Command Palette Styling

**Files:**
- Modify: `internal/tui/views/command.go`
- Modify: `internal/tui/commands/registry.go`

- [ ] **Step 1: Update command.go styling**

Update the command palette overlay to match the prototype:
- Border: `RoundedBorder()`, Slate700, background Slate900
- Prompt: `: ` in Red bold
- Selected item: Slate100 on Slate800 background, bold, with `▸` cursor
- Normal item: Slate300
- Description: Slate500 right-aligned
- Footer: key hints (↑↓ navigate, ↵ run, esc close)

- [ ] **Step 2: Update registry.go with additional commands**

Add commands from the JSX prototype's `COMMANDS` array that are missing:
- `Deploy from SDL file…` (action category)
- `Show wallet balance` (context category)
- `Switch wallet…` (context category)
- `Switch RPC endpoint…` (context category)
- `Tail logs for deployment…` (action category)
- `Open shell in deployment…` (action category)

- [ ] **Step 3: Run tests**

Run: `go test ./internal/tui/... -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add internal/tui/views/command.go internal/tui/commands/registry.go
git commit -m "feat(tui): restyle command palette and add missing commands

Update palette overlay to Zinc theme (Red prompt, Slate800 selected).
Add 6 additional commands from JSX prototype: deploy from SDL,
wallet balance, switch wallet, switch RPC, tail logs, open shell."
```

---

## Task 12: Update Help Overlay Styling

**Files:**
- Modify: `internal/tui/views/help.go`

- [ ] **Step 1: Update help.go styling**

Match the JSX prototype's help overlay (`tui-overlays.jsx`, `HelpOverlay`):
- Border: `RoundedBorder()`, Slate600, background Slate900
- Title: `?` badge (accent) + "Keybindings" (Slate100 bold) + context label
- 4-section grid: Navigation, Lists, Actions, Overlays
- Key column: Slate400 bold, width 10-12
- Description: Slate300
- Footer: version + "press esc to close"

- [ ] **Step 2: Run tests**

Run: `go test ./internal/tui/... -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/tui/views/help.go
git commit -m "feat(tui): restyle help overlay to match prototype design

4-section grid layout (Navigation/Lists/Actions/Overlays),
Zinc color scheme, context-sensitive label, accent badge."
```

---

## Task 13: Update ListView to use ResourceTable

**Files:**
- Modify: `internal/tui/views/listview.go`

- [ ] **Step 1: Refactor ListView to wrap ResourceTable**

The current `ListView` is a basic implementation. Refactor it to use the new `ResourceTable` component internally. Keep the existing `ListView` API (`SetItems`, `HandleKey`, `View`, etc.) but delegate rendering to `ResourceTable`.

This ensures all existing code that creates `ListView` instances in `app.go` continues to work, while getting the new visual treatment.

- [ ] **Step 2: Run tests**

Run: `go test ./internal/tui/... -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/tui/views/listview.go
git commit -m "refactor(tui): delegate ListView rendering to ResourceTable component

ListView now wraps ResourceTable for consistent table rendering
across all views (fixed-width columns, ▸ cursor, state tags)."
```

---

## Task 14: Final Integration Test

- [ ] **Step 1: Run full test suite**

Run: `go test ./... 2>&1 | tail -20`
Expected: All tests PASS

- [ ] **Step 2: Build and verify**

Run: `make akt`
Expected: Build succeeds

- [ ] **Step 3: Manual smoke test**

Run the TUI and verify:
- Header shows app name (red) · context · account · block · sync
- Nav bar shows 6 tabs with active tab highlighted in red
- Breadcrumb appears below nav
- Footer shows context-sensitive key hints
- Command palette (`:`) has updated styling and additional commands
- Help overlay (`?`) shows 4-section grid

Run: `.cache/bin/akt monitor --rpc https://rpc.akashnet.net:443`
Expected: Monitor launches with updated Zinc color scheme

- [ ] **Step 4: Commit any final adjustments**

```bash
git add -A
git commit -m "chore(tui): integration test fixes and polish"
```

---

## Summary

| Task | Component | Files | Dependency |
|------|-----------|-------|------------|
| 1 | Theme overhaul | theme.go, theme_test.go | None |
| 2 | Monitor style migration | monitor/ui/styles.go | Task 1 |
| 3 | State tags | components/statetag.go | Task 1 |
| 4 | KV detail | components/kvdetail.go | Task 1 |
| 5 | Footer hints | components/footer.go | Task 1 |
| 6 | Resource table | components/table.go | Tasks 1, 3 |
| 7 | Confirmation dialog | components/confirm.go | Tasks 1, 4, 5 |
| 8 | Toast notification | components/toast.go | Task 1 |
| 9 | Progress bar extraction | components/progress.go | Task 1 |
| 10 | App shell update | tui/app.go | Tasks 1, 5 |
| 11 | Command palette restyle | views/command.go | Task 1 |
| 12 | Help overlay restyle | views/help.go | Task 1 |
| 13 | ListView → ResourceTable | views/listview.go | Task 6 |
| 14 | Integration test | — | All above |

**Parallelizable:** Tasks 3-9 can all run in parallel after Task 1. Tasks 10-13 can run in parallel after their dependencies. Task 14 runs last.
