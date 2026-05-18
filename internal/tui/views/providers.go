package views

import (
	"pkg.akt.dev/akt/internal/tui/components"
)

// ProvidersView renders a table of provider records.
// Data binding is not yet implemented — this is a placeholder that
// defines the full column layout and shows an empty-state message.
type ProvidersView struct {
	table  components.ResourceTable
	width  int
	height int
}

// NewProvidersView creates a new ProvidersView with the standard column layout.
func NewProvidersView() ProvidersView {
	return ProvidersView{
		table: components.NewResourceTable(components.ResourceTableConfig{
			Columns: []components.TableColumn{
				{Header: "HOST", Width: 0, Align: components.AlignLeft},
				{Header: "REGION", Width: 12, Align: components.AlignLeft},
				{Header: "GPU", Width: 16, Align: components.AlignLeft},
				{Header: "CPU", Width: 8, Align: components.AlignRight},
				{Header: "MEMORY", Width: 10, Align: components.AlignRight},
				{Header: "LEASES", Width: 8, Align: components.AlignRight},
				{Header: "AUDIT", Width: 8, Align: components.AlignRight},
				{Header: "VERSION", Width: 10, Align: components.AlignLeft},
			},
			EmptyText: "Provider data requires chain connection.\nUse akt monitor provider for real-time fleet monitoring.",
		}),
	}
}

// SetSize updates the available width and height for rendering.
func (v *ProvidersView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.table.SetSize(w, h)
}

// CursorUp moves the cursor up one row.
func (v *ProvidersView) CursorUp() {
	v.table.CursorUp()
}

// CursorDown moves the cursor down one row.
func (v *ProvidersView) CursorDown() {
	v.table.CursorDown()
}

// View renders the providers table.
func (v ProvidersView) View() string {
	return v.table.View()
}
