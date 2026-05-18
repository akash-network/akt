package views

import (
	"pkg.akt.dev/akt/internal/tui/components"
)

// GovernanceView renders a table of governance proposals.
// Data binding is not yet implemented — this is a placeholder that
// defines the full column layout and shows an empty-state message.
type GovernanceView struct {
	table  components.ResourceTable
	width  int
	height int
}

// NewGovernanceView creates a new GovernanceView with the standard column layout.
func NewGovernanceView() GovernanceView {
	return GovernanceView{
		table: components.NewResourceTable(components.ResourceTableConfig{
			Columns: []components.TableColumn{
				{Header: "#", Width: 6, Align: components.AlignRight},
				{Header: "TITLE", Width: 0, Align: components.AlignLeft},
				{Header: "STATUS", Width: 12, Align: components.AlignLeft, RenderFunc: components.StateTag},
				{Header: "YES", Width: 7, Align: components.AlignRight},
				{Header: "NO", Width: 7, Align: components.AlignRight},
				{Header: "ABSTAIN", Width: 8, Align: components.AlignRight},
				{Header: "VETO", Width: 7, Align: components.AlignRight},
				{Header: "ENDS", Width: 10, Align: components.AlignRight},
			},
			EmptyText: "Governance proposals require chain connection.\nQuery proposals with: akt query gov proposals",
		}),
	}
}

// SetSize updates the available width and height for rendering.
func (v *GovernanceView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.table.SetSize(w, h)
}

// CursorUp moves the cursor up one row.
func (v *GovernanceView) CursorUp() {
	v.table.CursorUp()
}

// CursorDown moves the cursor down one row.
func (v *GovernanceView) CursorDown() {
	v.table.CursorDown()
}

// View renders the governance table.
func (v GovernanceView) View() string {
	return v.table.View()
}
