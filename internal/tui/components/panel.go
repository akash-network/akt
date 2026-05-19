package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// TitledPanel renders content inside a bordered box with the title embedded
// in the top border line:
//
//	┌─ TITLE ──────────────────┐
//	│ content line 1            │
//	│ content line 2            │
//	└───────────────────────────┘
//
// The width parameter is the outer width including border characters.
// Content lines are left-padded with 1 space and right-padded to fill.
func TitledPanel(title, content string, width int) string {
	borderFg := lipgloss.NewStyle().Foreground(theme.Slate700)
	titleStyle := lipgloss.NewStyle().Foreground(theme.Slate500).Bold(true)

	innerW := width - 4 // "│ " on left + " │" on right = 4 chars

	// Top border: ┌─ TITLE ─────...─┐
	titleRendered := titleStyle.Render(title)
	titleVisualW := lipgloss.Width(titleRendered)
	fillW := innerW - titleVisualW - 2 // 2 spaces around title (" TITLE ")
	if fillW < 0 {
		fillW = 0
	}
	topLine := borderFg.Render("┌─ ") + titleRendered + borderFg.Render(" " + strings.Repeat("─", fillW) + "─┐")

	// Content lines: │ content...pad │
	contentLines := strings.Split(content, "\n")
	var body strings.Builder
	for _, line := range contentLines {
		lineW := lipgloss.Width(line)
		pad := innerW - lineW
		if pad < 0 {
			pad = 0
		}
		body.WriteString(borderFg.Render("│") + " " + line + strings.Repeat(" ", pad) + " " + borderFg.Render("│") + "\n")
	}

	// Bottom border: └─────...─┘
	bottomLine := borderFg.Render("└" + strings.Repeat("─", width-2) + "┘")

	return topLine + "\n" + body.String() + bottomLine
}
