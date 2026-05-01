// Package theme provides the unified color palette and base styles for the
// entire akt UI — both CLI pretty output and TUI views. Every package that
// renders styled text should import colors and styles from here instead of
// defining its own.
//
// The palette uses 256-color values so rendering is consistent across
// terminals regardless of the active color scheme.
package theme

import "charm.land/lipgloss/v2"

// ─── Color palette ───────────────────────────────────────────────────
//
// 256-color values (16-255) give deterministic rendering that does not
// shift when the user switches terminal themes.

var (
	// Brand
	ColorPrimary = lipgloss.Color("168") // Rose — Akash brand accent
	ColorAccent  = lipgloss.Color("205") // Bright pink

	// Semantic
	ColorSuccess = lipgloss.Color("78")  // Seafoam green
	ColorWarning = lipgloss.Color("220") // Gold / amber
	ColorError   = lipgloss.Color("203") // Coral red

	// Text
	ColorText       = lipgloss.Color("252") // Light gray (default body text)
	ColorBrightText = lipgloss.Color("255") // Near-white (emphasis)
	ColorMuted      = lipgloss.Color("245") // Medium gray (secondary)
	ColorDim        = lipgloss.Color("240") // Dark gray (tertiary / faint)

	// UI chrome
	ColorBorder    = lipgloss.Color("238") // Borders and separators
	ColorHighlight = lipgloss.Color("62")  // Selection / header backgrounds

	// Data accent colors
	ColorCyan    = lipgloss.Color("80")  // Teal — precommit bars, links
	ColorMagenta = lipgloss.Color("176") // Soft purple — special fields
	ColorBlue    = lipgloss.Color("75")  // Cornflower — informational
)

// ─── Base styles ─────────────────────────────────────────────────────
//
// These are the building blocks. Packages may compose them further but
// should never redefine the colors — only the layout (width, padding, etc.).

var (
	// Text emphasis
	Bold    = lipgloss.NewStyle().Bold(true)
	Dim     = lipgloss.NewStyle().Foreground(ColorDim)
	Faint   = lipgloss.NewStyle().Faint(true)
	Muted   = lipgloss.NewStyle().Foreground(ColorMuted)
	Section = lipgloss.NewStyle().Bold(true).Underline(true)

	// Semantic text
	Success = lipgloss.NewStyle().Foreground(ColorSuccess)
	Warning = lipgloss.NewStyle().Foreground(ColorWarning)
	Error   = lipgloss.NewStyle().Foreground(ColorError).Bold(true)

	// Named color styles (for direct use in formatters)
	Green   = lipgloss.NewStyle().Foreground(ColorSuccess)
	Yellow  = lipgloss.NewStyle().Foreground(ColorWarning)
	Red     = lipgloss.NewStyle().Foreground(ColorError)
	Gray    = Dim
	Cyan    = lipgloss.NewStyle().Foreground(ColorCyan)
	Magenta = lipgloss.NewStyle().Foreground(ColorMagenta)
	Blue    = lipgloss.NewStyle().Foreground(ColorBlue)

	// KV / label styles (used by pretty.KV and monitor detail views)
	Key   = lipgloss.NewStyle().Foreground(ColorMuted)
	Label = lipgloss.NewStyle().Foreground(ColorMuted)
	Value = lipgloss.NewStyle().Bold(true).Foreground(ColorBrightText)

	// Table header
	Header = lipgloss.NewStyle().Bold(true).Foreground(ColorDim)

	// Section / panel headers
	SectionHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(ColorBorder).
			PaddingBottom(1).
			MarginBottom(1)

	// Title bar
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorAccent).
		MarginBottom(1)

	// Tab bar
	TabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBrightText).
			Background(ColorPrimary).
			Padding(0, 1)

	TabInactive = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1)

	// Selection / highlighting
	Highlight = lipgloss.NewStyle().
			Foreground(ColorBrightText).
			Bold(true)

	// Vote / signing indicators
	VoteYes = lipgloss.NewStyle().Foreground(ColorSuccess)
	VoteNo  = lipgloss.NewStyle().Foreground(ColorError)

	// Grid dots (version distribution, vote grid)
	GridVoted    = lipgloss.NewStyle().Foreground(ColorSuccess)
	GridNotVoted = lipgloss.NewStyle().Foreground(ColorMuted)

	// Proposer / special role
	Proposer = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)

	// Moniker / entity name
	Moniker = lipgloss.NewStyle().Foreground(ColorText)

	// Detail view
	DetailHeader = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	DetailLabel  = lipgloss.NewStyle().Foreground(ColorMuted).Width(10)
	DetailValue  = lipgloss.NewStyle().Foreground(ColorText)

	// Progress bar colors (passed to bubbles/progress)
	ProgressPrimary   = ColorPrimary
	ProgressSuccess   = ColorSuccess
	ProgressPrecommit = ColorCyan

	// Percentage thresholds
	PercentHigh = lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess)
	PercentLow  = lipgloss.NewStyle().Foreground(ColorWarning)

	// Status bar
	StatusBar = lipgloss.NewStyle().Foreground(ColorMuted).MarginTop(1)
	HelpBar   = lipgloss.NewStyle().Foreground(ColorMuted).MarginTop(1)
)
