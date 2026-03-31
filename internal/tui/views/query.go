package views

import (
	"github.com/charmbracelet/lipgloss"
)

// QueryView is a placeholder for the query commands panel.
type QueryView struct {
	width  int
	height int
}

// NewQueryView returns a new query view.
func NewQueryView() QueryView {
	return QueryView{}
}

// SetSize updates the view dimensions.
func (v *QueryView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// View renders the query panel.
func (v QueryView) View() string {
	style := lipgloss.NewStyle().
		Width(v.width).
		Height(v.height).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render("Query Commands (q)")
}
