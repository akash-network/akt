package components

import (
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// shortLabel returns a compact display label for long state names.
func shortLabel(state string) string {
	switch state {
	case "insufficient_funds":
		return "low funds"
	case "voting_period":
		return "voting"
	case "deposit_period":
		return "deposit"
	default:
		return state
	}
}

// stateColors returns the text and border styles for a given state.
func stateColors(state string) (text, border lipgloss.Style) {
	switch state {
	case "active", "open", "bonded", "passed", "valid", "matched":
		text = lipgloss.NewStyle().Foreground(theme.GreenColor)
		border = lipgloss.NewStyle().Foreground(theme.GreenDim)
	case "paused", "insufficient_funds", "overdrawn", "unbonding",
		"voting_period", "deposit_period", "pending":
		text = lipgloss.NewStyle().Foreground(theme.YellowColor)
		border = lipgloss.NewStyle().Foreground(theme.YellowDim)
	case "closed", "lost", "unbonded", "rejected", "failed",
		"jailed", "revoked", "invalid":
		text = lipgloss.NewStyle().Foreground(theme.Slate500)
		border = lipgloss.NewStyle().Foreground(theme.Slate700)
	default:
		text = lipgloss.NewStyle().Foreground(theme.Slate500)
		border = lipgloss.NewStyle().Foreground(theme.Slate700)
	}
	return text, border
}

// StateTag renders an inline state tag like │active│ with color-mapped borders.
// The Unicode box-drawing character │ is used as the border, not a lipgloss
// rounded border (which would produce a 3-line box).
func StateTag(state string) string {
	label := shortLabel(state)
	text, border := stateColors(state)
	return border.Render("│") + text.Render(label) + border.Render("│")
}

// StateTagWidth returns the display width of a state tag for column alignment.
// The width is the label length plus 2 for the border characters.
func StateTagWidth(state string) int {
	return len(shortLabel(state)) + 2
}
