package views

import (
	"github.com/charmbracelet/lipgloss"
)

// TxView is a placeholder for the transaction commands panel.
type TxView struct {
	width  int
	height int
}

// NewTxView returns a new tx view.
func NewTxView() TxView {
	return TxView{}
}

// SetSize updates the view dimensions.
func (v *TxView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// View renders the tx panel.
func (v TxView) View() string {
	style := lipgloss.NewStyle().
		Width(v.width).
		Height(v.height).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render("Transaction Commands (t)")
}
