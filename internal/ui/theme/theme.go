// Package theme provides the unified color palette and base styles for the
// entire akt UI — both CLI pretty output and TUI views. Every package that
// renders styled text should import colors and styles from here instead of
// defining its own.
//
// The palette uses a Zinc/neutral scale with true-color hex values and a
// single Red accent. State colors (green, yellow) are used exclusively in
// status tags.
package theme

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// ─── Zinc Scale ──────────────────────────────────────────────────────
//
// Shadcn Zinc scale mapped to hex. Every neutral color in the UI comes
// from this set. Brighter values = more visual emphasis.

var (
	Slate950 = lipgloss.Color("#09090b") // App background
	Slate900 = lipgloss.Color("#18181b") // Card / panel / overlay background
	Slate800 = lipgloss.Color("#27272a") // Elevated surface, selected row
	Slate700 = lipgloss.Color("#3f3f46") // Borders, horizontal rules
	Slate600 = lipgloss.Color("#52525b") // Muted borders, subtle dividers
	Slate500 = lipgloss.Color("#71717a") // Column headers, muted labels
	Slate400 = lipgloss.Color("#a1a1aa") // Secondary text, inactive nav
	Slate300 = lipgloss.Color("#d4d4d8") // Body text, table values
	Slate200 = lipgloss.Color("#e4e4e7") // Primary text, bold values
	Slate100 = lipgloss.Color("#f4f4f5") // Headings, section titles
	Slate50  = lipgloss.Color("#fafafa") // Maximum emphasis
)

// ─── Accent ──────────────────────────────────────────────────────────

var (
	AccentRed = lipgloss.Color("#ff4136") // Primary accent
	RedDim    = lipgloss.Color("#b91c1c") // Muted accent (destructive tag border)
	RedBg     = lipgloss.Color("#1c0a09") // Destructive pill background
)

// ─── State Colors ────────────────────────────────────────────────────

var (
	GreenColor  = lipgloss.Color("#22c55e") // active, open, bonded, passed
	GreenDim    = lipgloss.Color("#166534") // Green state tag border
	YellowColor = lipgloss.Color("#eab308") // paused, insufficient_funds, unbonding
	YellowDim   = lipgloss.Color("#854d0e") // Yellow state tag border
	BlueColor   = lipgloss.Color("#65b3ff")
	PurpleColor = lipgloss.Color("#c08cff")
)

// ─── Typography ──────────────────────────────────────────────────────

var (
	Heading      = lipgloss.NewStyle().Foreground(Slate100).Bold(true)
	PrimaryValue = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
	Body         = lipgloss.NewStyle().Foreground(Slate300)
	Secondary    = lipgloss.NewStyle().Foreground(Slate400)
	// Muted is defined below in the backward-compat section (Slate500 foreground).
)

// ─── Header Bar ──────────────────────────────────────────────────────

var (
	HeaderStyle   = lipgloss.NewStyle().Background(Slate900).Foreground(Slate300).Padding(0, 1)
	HeaderAppName = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
	HeaderContext = lipgloss.NewStyle().Foreground(Slate100).Bold(true)
	HeaderMeta    = lipgloss.NewStyle().Foreground(Slate500)
	HeaderValue   = lipgloss.NewStyle().Foreground(Slate300)
	SyncOK        = lipgloss.NewStyle().Foreground(Slate300)
	SyncFail      = lipgloss.NewStyle().Foreground(AccentRed)
)

// ─── Nav Tabs ────────────────────────────────────────────────────────

var (
	NavTabActive = lipgloss.NewStyle().
			Background(AccentRed).
			Foreground(Slate950).
			Bold(true).
			Padding(0, 1)

	NavTabInactive = lipgloss.NewStyle().
			Foreground(Slate400).
			Padding(0, 1)
)

// ─── Breadcrumb ──────────────────────────────────────────────────────

var (
	BreadcrumbSegment   = lipgloss.NewStyle().Foreground(Slate500)
	BreadcrumbActive    = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
	BreadcrumbSeparator = lipgloss.NewStyle().Foreground(Slate600)
)

// ─── Footer ──────────────────────────────────────────────────────────

var (
	FooterKey  = lipgloss.NewStyle().Foreground(Slate400).Bold(true)
	FooterDesc = lipgloss.NewStyle().Foreground(Slate600)
)

// ─── Table ───────────────────────────────────────────────────────────

var (
	TableHeader      = lipgloss.NewStyle().Foreground(Slate500).Padding(0, 1)
	TableRow         = lipgloss.NewStyle().Foreground(Slate300)
	TableRowSelected = lipgloss.NewStyle().Background(Slate800)
	TableCursor      = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
)

// Table (inline — no padding, use with fixed-width col() helper in views).
var (
	ColHeader = lipgloss.NewStyle().Foreground(Slate500)
	Col       = lipgloss.NewStyle().Foreground(Slate300)
	ColBold   = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
	ColMuted  = lipgloss.NewStyle().Foreground(Slate500)
	ColAccent = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
)

// ─── Padded Table Cells ─────────────────────────────────────────────

var (
	TableCell      = lipgloss.NewStyle().Foreground(Slate300).Padding(0, 1)
	TableCellBold  = lipgloss.NewStyle().Foreground(Slate200).Bold(true).Padding(0, 1)
	TableCellMuted = lipgloss.NewStyle().Foreground(Slate500).Padding(0, 1)
)

// ─── KV Detail ───────────────────────────────────────────────────────

var (
	SectionTitle = lipgloss.NewStyle().Foreground(Slate100).Bold(true)
	SectionRule  = lipgloss.NewStyle().Foreground(AccentRed)
	KVLabel      = lipgloss.NewStyle().Foreground(Slate500).Width(16)
	KVValue      = lipgloss.NewStyle().Foreground(Slate200)
	KVValueBold  = lipgloss.NewStyle().Foreground(Slate100).Bold(true)
	KVValueMuted = lipgloss.NewStyle().Foreground(Slate500)
)

// ─── Panel / Card ────────────────────────────────────────────────────

var (
	PanelBorder = lipgloss.NewStyle().Foreground(Slate700)
	PanelBg     = lipgloss.NewStyle().Background(Slate900)
)

// ─── State Tags ──────────────────────────────────────────────────────

var (
	StateGreen  = lipgloss.NewStyle().Foreground(GreenColor)
	StateYellow = lipgloss.NewStyle().Foreground(YellowColor)
	StateClosed = lipgloss.NewStyle().Foreground(Slate500)
)

// ─── Status Badges (inline text, no border) ─────────────────────────

var (
	BadgeActive      = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
	BadgeClosed      = lipgloss.NewStyle().Foreground(Slate500)
	BadgeDestructive = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
	BadgeWarning     = lipgloss.NewStyle().Foreground(Slate400)
)

// StateBadge returns a badge style for a given state string.
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

// ─── Overlay ─────────────────────────────────────────────────────────

var (
	OverlayBg     = lipgloss.NewStyle().Background(Slate900)
	OverlayBorder = lipgloss.NewStyle().Foreground(Slate700)
)

// ─── Buttons ─────────────────────────────────────────────────────────

var (
	ButtonPrimary   = lipgloss.NewStyle().Background(AccentRed).Foreground(Slate950).Bold(true)
	ButtonSecondary = lipgloss.NewStyle().Background(Slate800).Foreground(Slate300)
)

// ─── Command Palette ─────────────────────────────────────────────────

var (
	PaletteBorder       = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(Slate700).Background(Slate900)
	PaletteInput        = lipgloss.NewStyle().Foreground(Slate200).Background(Slate900).Padding(0, 1)
	PalettePrompt       = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
	PaletteItemNormal   = lipgloss.NewStyle().Foreground(Slate300).Padding(0, 1)
	PaletteItemSelected = lipgloss.NewStyle().Foreground(Slate100).Background(Slate800).Bold(true).Padding(0, 1)
	PaletteItemDesc     = lipgloss.NewStyle().Foreground(Slate500)
)

// ─── Dialog ──────────────────────────────────────────────────────────

var (
	DialogBorder          = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(Slate600).Background(Slate900).Padding(1, 2)
	DialogTitle           = lipgloss.NewStyle().Foreground(Slate100).Bold(true)
	DialogBody            = lipgloss.NewStyle().Foreground(Slate400)
	DialogButtonPrimary   = lipgloss.NewStyle().Foreground(Slate950).Background(AccentRed).Bold(true).Padding(0, 2)
	DialogButtonSecondary = lipgloss.NewStyle().Foreground(Slate300).Background(Slate800).Padding(0, 2)
)

// ─── Error Display ──────────────────────────────────────────────────

var (
	ErrorLabel      = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)
	ErrorMessage    = lipgloss.NewStyle().Foreground(Slate300)
	ErrorSuggestion = lipgloss.NewStyle().Foreground(Slate400)
	ErrorCommand    = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
)

// ─── Progress Bar Colors ─────────────────────────────────────────────

var (
	BarFilled     = lipgloss.NewStyle().Foreground(Slate200)
	BarEmpty      = lipgloss.NewStyle().Foreground(Slate700)
	ProgressLabel = lipgloss.NewStyle().Foreground(Slate500).Width(14)
	ProgressPct   = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
)

// ─── Spinner ─────────────────────────────────────────────────────────

var (
	SpinnerStyle = lipgloss.NewStyle().Foreground(AccentRed)
	SpinnerText  = lipgloss.NewStyle().Foreground(Slate400)
)

// ─── Layout Helpers ──────────────────────────────────────────────────

// HRule renders a full-width horizontal rule in Slate700.
func HRule(w int) string {
	return lipgloss.NewStyle().Foreground(Slate700).Render(strings.Repeat("─", w))
}

// HRuleAccent renders a full-width horizontal rule in AccentRed.
func HRuleAccent(w int) string {
	return lipgloss.NewStyle().Foreground(AccentRed).Render(strings.Repeat("─", w))
}

// ─── Backward-Compatible Aliases ─────────────────────────────────────
//
// The names below were exported by the previous 256-color theme. They are
// preserved so that existing packages (monitor, pretty output, TUI views)
// continue to compile. Each alias maps to the closest Zinc-scale or accent
// equivalent.

var (
	// Deprecated: use AccentRed instead.
	ColorPrimary = AccentRed
	// Deprecated: use AccentRed instead.
	ColorAccent = AccentRed

	// Deprecated: use GreenColor instead.
	ColorSuccess = GreenColor
	// Deprecated: use YellowColor instead.
	ColorWarning = YellowColor
	// Deprecated: use AccentRed instead.
	ColorError = AccentRed

	// Deprecated: use Slate300 instead.
	ColorText = Slate300
	// Deprecated: use Slate200 instead.
	ColorBrightText = Slate200
	// Deprecated: use Slate500 instead.
	ColorMuted = Slate500
	// Deprecated: use Slate400 instead.
	ColorDim = Slate400

	// Deprecated: use Slate700 instead.
	ColorBorder = Slate700
	// Deprecated: use Slate800 instead.
	ColorHighlight = Slate800

	// Deprecated: use BlueColor instead.
	ColorCyan = BlueColor
	// Deprecated: use PurpleColor instead.
	ColorMagenta = PurpleColor
	// Deprecated: use BlueColor instead.
	ColorBlue = BlueColor
)

// ─── Backward-Compatible Style Aliases ───────────────────────────────

var (
	// Deprecated: use Heading instead.
	Bold = lipgloss.NewStyle().Bold(true)
	// Deprecated: use Secondary instead.
	Dim = lipgloss.NewStyle().Foreground(Slate400)
	// Deprecated: use Secondary instead.
	Faint = lipgloss.NewStyle().Faint(true)
	// Muted style — Foreground(Slate500). Also serves as the canonical
	// "muted" typography level.
	Muted = lipgloss.NewStyle().Foreground(Slate500)
	// Deprecated: use SectionTitle instead.
	Section = lipgloss.NewStyle().Bold(true).Underline(true)

	// Deprecated: use StateGreen instead.
	Success = lipgloss.NewStyle().Foreground(GreenColor)
	// Deprecated: use StateYellow instead.
	Warning = lipgloss.NewStyle().Foreground(YellowColor)
	// Deprecated: use SectionRule or AccentRed instead.
	Error = lipgloss.NewStyle().Foreground(AccentRed).Bold(true)

	// Deprecated: use StateGreen instead.
	Green = lipgloss.NewStyle().Foreground(GreenColor)
	// Deprecated: use StateYellow instead.
	Yellow = lipgloss.NewStyle().Foreground(YellowColor)
	// Deprecated: use SectionRule or AccentRed-based styles instead.
	Red = lipgloss.NewStyle().Foreground(AccentRed)
	// Deprecated: use Dim instead.
	Gray = lipgloss.NewStyle().Foreground(Slate400)
	// Deprecated: use BlueColor-based style instead.
	Cyan = lipgloss.NewStyle().Foreground(BlueColor)
	// Deprecated: use PurpleColor-based style instead.
	Magenta = lipgloss.NewStyle().Foreground(PurpleColor)
	// Deprecated: use BlueColor-based style instead.
	Blue = lipgloss.NewStyle().Foreground(BlueColor)

	// Deprecated: use KVLabel instead.
	Key = lipgloss.NewStyle().Foreground(Slate500)
	// Deprecated: use KVLabel instead.
	Label = lipgloss.NewStyle().Foreground(Slate500)
	// Deprecated: use KVValue instead.
	Value = lipgloss.NewStyle().Bold(true).Foreground(Slate200)

	// Deprecated: use TableHeader instead.
	Header = lipgloss.NewStyle().Bold(true).Foreground(Slate500)

	// Deprecated: use SectionTitle instead.
	SectionHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(AccentRed).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(Slate700).
			PaddingBottom(1).
			MarginBottom(1)

	// Deprecated: use Heading instead.
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(AccentRed).
		MarginBottom(1)

	// Deprecated: use NavTabActive instead.
	TabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(Slate950).
			Background(AccentRed).
			Padding(0, 1)

	// Deprecated: use NavTabInactive instead.
	TabInactive = lipgloss.NewStyle().
			Foreground(Slate400).
			Padding(0, 1)

	// Deprecated: use PrimaryValue instead.
	Highlight = lipgloss.NewStyle().
			Foreground(Slate200).
			Bold(true)

	// Deprecated: use StateGreen instead.
	VoteYes = lipgloss.NewStyle().Foreground(GreenColor)
	// Deprecated: use AccentRed-based style instead.
	VoteNo = lipgloss.NewStyle().Foreground(AccentRed)

	// Deprecated: use StateGreen instead.
	GridVoted = lipgloss.NewStyle().Foreground(GreenColor)
	// Deprecated: use Muted instead.
	GridNotVoted = lipgloss.NewStyle().Foreground(Slate500)

	// Deprecated: use StateYellow instead.
	Proposer = lipgloss.NewStyle().Foreground(YellowColor).Bold(true)

	// Deprecated: use Body instead.
	Moniker = lipgloss.NewStyle().Foreground(Slate300)

	// Deprecated: use SectionTitle instead.
	DetailHeader = lipgloss.NewStyle().Bold(true).Foreground(AccentRed)
	// Deprecated: use KVLabel instead.
	DetailLabel = lipgloss.NewStyle().Foreground(Slate500).Width(10)
	// Deprecated: use KVValue instead.
	DetailValue = lipgloss.NewStyle().Foreground(Slate300)

	// Deprecated: use AccentRed instead.
	ProgressPrimary = AccentRed
	// Deprecated: use GreenColor instead.
	ProgressSuccess = GreenColor
	// Deprecated: use BlueColor instead.
	ProgressPrecommit = BlueColor

	// Deprecated: use StateGreen instead.
	PercentHigh = lipgloss.NewStyle().Bold(true).Foreground(GreenColor)
	// Deprecated: use StateYellow instead.
	PercentLow = lipgloss.NewStyle().Foreground(YellowColor)

	// Deprecated: use FooterDesc instead.
	StatusBar = lipgloss.NewStyle().Foreground(Slate500).MarginTop(1)
	// Deprecated: use FooterDesc instead.
	HelpBar = lipgloss.NewStyle().Foreground(Slate500).MarginTop(1)
)
