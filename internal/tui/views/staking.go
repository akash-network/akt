package views

import (
	"pkg.akt.dev/akt/internal/tui/components"
)

// StakingView renders a table of validator records.
// Data binding is not yet implemented — this is a placeholder that
// defines the full column layout and shows an empty-state message.
type StakingView struct {
	table  components.ResourceTable
	width  int
	height int
}

// NewStakingView creates a new StakingView with the standard column layout.
func NewStakingView() StakingView {
	return StakingView{
		table: components.NewResourceTable(components.ResourceTableConfig{
			Columns: []components.TableColumn{
				{Header: "#", Width: 5, Align: components.AlignRight},
				{Header: "MONIKER", Width: 0, Align: components.AlignLeft},
				{Header: "POWER", Width: 10, Align: components.AlignRight},
				{Header: "VP%", Width: 8, Align: components.AlignRight},
				{Header: "COMMISSION", Width: 12, Align: components.AlignRight},
				{Header: "UPTIME", Width: 10, Align: components.AlignRight},
				{Header: "SIGNED", Width: 8, Align: components.AlignRight},
			},
			EmptyText: "Validator data requires chain connection.\nUse akt monitor network for real-time validator monitoring.",
		}),
	}
}

// SetSize updates the available width and height for rendering.
func (v *StakingView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.table.SetSize(w, h)
}

// CursorUp moves the cursor up one row.
func (v *StakingView) CursorUp() {
	v.table.CursorUp()
}

// CursorDown moves the cursor down one row.
func (v *StakingView) CursorDown() {
	v.table.CursorDown()
}

// View renders the staking table.
func (v StakingView) View() string {
	return v.table.View()
}
