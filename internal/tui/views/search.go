package views

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SearchDialog is a command search overlay (ctrl+p).
type SearchDialog struct {
	input  textinput.Model
	width  int
	height int
	active bool
}

// NewSearchDialog returns a new command search dialog.
func NewSearchDialog() SearchDialog {
	ti := textinput.New()
	ti.Placeholder = "Search commands..."
	ti.CharLimit = 64
	ti.Width = 40

	return SearchDialog{
		input: ti,
	}
}

// Active returns whether the dialog is visible.
func (d SearchDialog) Active() bool {
	return d.active
}

// Open shows the dialog and focuses the input.
func (d *SearchDialog) Open() {
	d.active = true
	d.input.Focus()
	d.input.SetValue("")
}

// Close hides the dialog.
func (d *SearchDialog) Close() {
	d.active = false
	d.input.Blur()
}

// SetSize updates the overlay dimensions.
func (d *SearchDialog) SetSize(w, h int) {
	d.width = w
	d.height = h
}

// Update handles input events for the search dialog.
func (d *SearchDialog) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return cmd
}

// View renders the search dialog as a centered overlay.
func (d SearchDialog) View() string {
	if !d.active {
		return ""
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(50).
		Align(lipgloss.Center)

	title := lipgloss.NewStyle().
		Bold(true).
		MarginBottom(1).
		Render("Command Search")

	content := title + "\n" + d.input.View()

	dialog := box.Render(content)

	// Center the dialog in the available space.
	return lipgloss.Place(d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}
