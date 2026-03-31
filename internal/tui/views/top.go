package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TopView renders the consensus monitor (aktop-style).
// Data fetching is not yet implemented; this renders the layout with placeholder values.
type TopView struct {
	width  int
	height int
}

// NewTopView returns a new consensus monitor view.
func NewTopView() TopView {
	return TopView{}
}

// SetSize updates the view dimensions.
func (v *TopView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// View renders the consensus monitor layout.
func (v TopView) View() string {
	w := v.width
	if w < 20 {
		w = 20
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Width(w).
		Padding(0, 1).
		Render("Consensus State")

	sep := lipgloss.NewStyle().
		Width(w).
		Render(strings.Repeat("─", w))

	fields := lipgloss.NewStyle().Padding(0, 1)

	state := fields.Render(fmt.Sprintf(
		"%-16s %-20s %-16s %s\n%-16s %-20s",
		"Height:", "--",
		"Round:", "--",
		"Step:", "--",
	))

	voteTitle := lipgloss.NewStyle().
		Bold(true).
		Width(w).
		Padding(0, 1).
		Render("Vote Progress")

	barW := w - 30
	if barW < 10 {
		barW = 10
	}

	emptyBar := strings.Repeat("░", barW)

	prevote := fields.Render(fmt.Sprintf("Prevotes:    %s  --%%", emptyBar))
	precommit := fields.Render(fmt.Sprintf("Precommits:  %s  --%%", emptyBar))

	gridTitle := lipgloss.NewStyle().
		Bold(true).
		Width(w).
		Padding(0, 1).
		Render("Validator Votes")

	grid := fields.Render("waiting for data...")

	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1).
		Render("r: refresh  esc: back")

	content := strings.Join([]string{
		title,
		"",
		state,
		sep,
		voteTitle,
		"",
		prevote,
		precommit,
		sep,
		gridTitle,
		"",
		grid,
		"",
		hint,
	}, "\n")

	// Pad to fill height.
	lines := strings.Count(content, "\n") + 1
	if lines < v.height {
		content += strings.Repeat("\n", v.height-lines)
	}

	return content
}
