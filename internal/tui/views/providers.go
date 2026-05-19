package views

import (
	attrtypes "pkg.akt.dev/go/node/types/attributes/v1"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/akt/internal/tui/components"
)

// ProvidersView renders a table of provider records.
// Data binding is not yet implemented — this is a placeholder that
// defines the full column layout and shows an empty-state message.
type ProvidersView struct {
	table  components.ResourceTable
	items  ptypes.Providers
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

// SetData stores the providers and rebuilds the table rows.
func (v *ProvidersView) SetData(providers ptypes.Providers) {
	v.items = providers
	rows := make([]components.TableRow, len(providers))
	for i, p := range providers {
		audit := "—"
		if len(p.Attributes) > 0 {
			audit = "yes"
		}

		rows[i] = components.TableRow{
			ID: p.Owner,
			Cells: []string{
				p.HostURI,
				attrValue(p.Attributes, "region"),
				"—", // GPU: requires live provider status query
				"—", // CPU: requires live provider status query
				"—", // Memory: requires live provider status query
				"—", // Leases: requires live provider status query
				audit,
				"—", // Version: requires live provider status query
			},
		}
	}
	v.table.SetRows(rows)
}

// SelectedProvider returns the provider record for the currently highlighted row, or nil.
func (v *ProvidersView) SelectedProvider() *ptypes.Provider {
	row := v.table.SelectedRow()
	if row == nil {
		return nil
	}
	for i := range v.items {
		if v.items[i].Owner == row.ID {
			return &v.items[i]
		}
	}
	return nil
}

// View renders the providers table.
func (v ProvidersView) View() string {
	return v.table.View()
}

// attrValue returns the value of the first attribute matching key, or "—".
func attrValue(attrs attrtypes.Attributes, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value
		}
	}
	return "—"
}
