package theme

import "charm.land/lipgloss/v2"

// ─── Palette ───────────────────────────────────────────────────────────────────
// Shadcn Zinc scale mapped to hex. Every color in the UI comes from this set.
// The only non-neutral is Akash Red, used exclusively for primary accent.

var (
	// Slate scale (Shadcn Zinc)
	Slate950 = lipgloss.Color("#09090b") // App background
	Slate900 = lipgloss.Color("#18181b") // Card / panel background
	Slate800 = lipgloss.Color("#27272a") // Elevated surface, selected row
	Slate700 = lipgloss.Color("#3f3f46") // Border (≈ Shadcn border-input)
	Slate600 = lipgloss.Color("#52525b") // Muted border, subtle dividers
	Slate500 = lipgloss.Color("#71717a") // Placeholder, disabled text
	Slate400 = lipgloss.Color("#a1a1aa") // Secondary text, descriptions
	Slate300 = lipgloss.Color("#d4d4d8") // Body text
	Slate200 = lipgloss.Color("#e4e4e7") // Primary text, values
	Slate100 = lipgloss.Color("#f4f4f5") // Headers, column titles
	Slate50  = lipgloss.Color("#fafafa") // Maximum emphasis (use sparingly)

	// Akash Red — the only accent color in the system
	Red    = lipgloss.Color("#ff4136") // Primary accent: cursor, focus ring, active tab
	RedDim = lipgloss.Color("#b91c1c") // Muted accent: inactive highlight, border accent
	RedBg  = lipgloss.Color("#1c0a09") // Alert/destructive background tint
)

// ─── Semantic Aliases ──────────────────────────────────────────────────────────

var (
	BgApp      = Slate950
	BgSurface  = Slate900
	BgElevated = Slate800
	BgAccent   = RedBg

	Border      = Slate700
	BorderMuted = Slate600
	BorderFocus = Red

	TextPrimary   = Slate200
	TextSecondary = Slate400
	TextMuted     = Slate500
	TextHeading   = Slate100
	TextEmphasis  = Slate50

	Accent      = Red
	AccentMuted = RedDim
)

// ─── Component Styles ──────────────────────────────────────────────────────────

// Header
var (
	HeaderStyle = lipgloss.NewStyle().
			Background(Slate900).
			Foreground(Slate300).
			Padding(0, 1)

	HeaderAppName = lipgloss.NewStyle().
			Foreground(Red).
			Bold(true)

	HeaderContext = lipgloss.NewStyle().
			Foreground(Slate100).
			Bold(true)

	HeaderMeta = lipgloss.NewStyle().
			Foreground(Slate500)

	HeaderValue = lipgloss.NewStyle().
			Foreground(Slate300)

	SyncOK   = lipgloss.NewStyle().Foreground(Slate300)
	SyncFail = lipgloss.NewStyle().Foreground(Red)
)

// Breadcrumb
var (
	BreadcrumbSeparator = lipgloss.NewStyle().Foreground(Slate600)
	BreadcrumbSegment   = lipgloss.NewStyle().Foreground(Slate500)
	BreadcrumbActive    = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
)

// Tab Bar
var (
	TabActive = lipgloss.NewStyle().
			Foreground(Slate950).
			Background(Red).
			Bold(true).
			Padding(0, 1)

	TabInactive = lipgloss.NewStyle().
			Foreground(Slate500).
			Padding(0, 1)
)

// Table (padded — for standalone cells)
var (
	TableHeader = lipgloss.NewStyle().
			Foreground(Slate500).
			Padding(0, 1)

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

	TableRowSelected = lipgloss.NewStyle().
				Background(Slate800)

	TableCursor = lipgloss.NewStyle().
			Foreground(Red).
			Bold(true)
)

// Table (inline — no padding, use with fixed-width format strings)
var (
	ColHeader = lipgloss.NewStyle().Foreground(Slate500)
	Col       = lipgloss.NewStyle().Foreground(Slate300)
	ColBold   = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
	ColMuted  = lipgloss.NewStyle().Foreground(Slate500)
	ColAccent = lipgloss.NewStyle().Foreground(Red).Bold(true)
)

// Section Headers
var (
	SectionTitle = lipgloss.NewStyle().
			Foreground(Slate100).
			Bold(true)

	KVKey = lipgloss.NewStyle().
		Foreground(Slate500).
		Width(16)

	KVValue = lipgloss.NewStyle().
		Foreground(Slate200)

	KVValueBold = lipgloss.NewStyle().
			Foreground(Slate100).
			Bold(true)

	KVValueMuted = lipgloss.NewStyle().
			Foreground(Slate500)
)

// Status Badges (inline text)
var (
	BadgeActive      = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
	BadgeClosed      = lipgloss.NewStyle().Foreground(Slate500)
	BadgeDestructive = lipgloss.NewStyle().Foreground(Red).Bold(true)
	BadgeWarning     = lipgloss.NewStyle().Foreground(Slate400)

	PillActive      = lipgloss.NewStyle().Foreground(Slate200).Background(Slate800).Padding(0, 1)
	PillClosed      = lipgloss.NewStyle().Foreground(Slate500).Background(Slate900).Padding(0, 1)
	PillDestructive = lipgloss.NewStyle().Foreground(Red).Background(RedBg).Padding(0, 1).Bold(true)
)

// Status Tags — bordered pill badges for table state columns
var (
	Green    = lipgloss.Color("#22c55e") // green-500 for active state
	GreenDim = lipgloss.Color("#166534") // green-800 for active border
	Yellow   = lipgloss.Color("#eab308") // yellow-500 for warning state
	YellowDim = lipgloss.Color("#854d0e") // yellow-800 for warning border

	TagActive = lipgloss.NewStyle().
			Foreground(Green).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(GreenDim).
			Padding(0, 1)

	TagClosed = lipgloss.NewStyle().
			Foreground(Slate500).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Slate700).
			Padding(0, 1)

	TagWarning = lipgloss.NewStyle().
			Foreground(Yellow).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(YellowDim).
			Padding(0, 1)

	TagDestructive = lipgloss.NewStyle().
			Foreground(Red).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(RedDim).
			Padding(0, 1)
)

// StateTag returns a bordered tag style for a given state string.
func StateTag(state string) lipgloss.Style {
	switch state {
	case "active", "open", "bonded", "passed", "valid":
		return TagActive
	case "closed", "lost", "unbonded", "rejected", "failed", "revoked":
		return TagClosed
	case "paused", "insufficient_funds", "overdrawn", "unbonding",
		"low funds": // short label
		return TagWarning
	default:
		return TagClosed
	}
}

// Nav Tabs — pill-style for primary navigation
var (
	NavTabActive = lipgloss.NewStyle().
			Foreground(Slate950).
			Background(Red).
			Bold(true).
			Padding(0, 1)

	NavTabInactive = lipgloss.NewStyle().
			Foreground(Slate400).
			Padding(0, 1)
)

// Progress Bars
var (
	ProgressFilled = lipgloss.NewStyle().Foreground(Slate200)
	ProgressEmpty  = lipgloss.NewStyle().Foreground(Slate700)
	ProgressLabel  = lipgloss.NewStyle().Foreground(Slate500).Width(14)
	ProgressPct    = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
)

// Command Palette
var (
	PaletteBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Slate700).
			Background(Slate900)

	PaletteInput  = lipgloss.NewStyle().Foreground(Slate200).Background(Slate900).Padding(0, 1)
	PalettePrompt = lipgloss.NewStyle().Foreground(Red).Bold(true)

	PaletteItemNormal   = lipgloss.NewStyle().Foreground(Slate300).Padding(0, 1)
	PaletteItemSelected = lipgloss.NewStyle().Foreground(Slate100).Background(Slate800).Bold(true).Padding(0, 1)
	PaletteItemDesc     = lipgloss.NewStyle().Foreground(Slate500)
)

// Confirmation Dialog
var (
	DialogBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Slate600).
			Background(Slate900).
			Padding(1, 2)

	DialogTitle = lipgloss.NewStyle().Foreground(Slate100).Bold(true)
	DialogBody  = lipgloss.NewStyle().Foreground(Slate400)

	DialogButtonPrimary   = lipgloss.NewStyle().Foreground(Slate950).Background(Red).Bold(true).Padding(0, 2)
	DialogButtonSecondary = lipgloss.NewStyle().Foreground(Slate300).Background(Slate800).Padding(0, 2)
)

// Footer
var (
	FooterKey  = lipgloss.NewStyle().Foreground(Slate400).Bold(true)
	FooterDesc = lipgloss.NewStyle().Foreground(Slate600)
)

// Spinner
var (
	SpinnerStyle = lipgloss.NewStyle().Foreground(Red)
	SpinnerText  = lipgloss.NewStyle().Foreground(Slate400)
)

// Error Display
var (
	ErrorLabel      = lipgloss.NewStyle().Foreground(Red).Bold(true)
	ErrorMessage    = lipgloss.NewStyle().Foreground(Slate300)
	ErrorSuggestion = lipgloss.NewStyle().Foreground(Slate400)
	ErrorCommand    = lipgloss.NewStyle().Foreground(Slate200).Bold(true)
)

// ─── State Resolver ────────────────────────────────────────────────────────────

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

// ─── Layout Helpers ────────────────────────────────────────────────────────────

func HRule(width int) string {
	s := ""
	for i := 0; i < width; i++ {
		s += "─"
	}
	return lipgloss.NewStyle().Foreground(Slate700).Render(s)
}

func HRuleAccent(width int) string {
	s := ""
	for i := 0; i < width; i++ {
		s += "─"
	}
	return lipgloss.NewStyle().Foreground(Red).Render(s)
}
