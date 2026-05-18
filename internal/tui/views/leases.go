package views

import (
	"strconv"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
)

// LeasesView renders a table of lease records with filtering.
type LeasesView struct {
	table  components.ResourceTable
	data   []*store.LeaseRecord
	width  int
	height int
	filter string // "", "active", "closed"
}

// NewLeasesView creates a new LeasesView with the standard column layout.
func NewLeasesView() LeasesView {
	return LeasesView{
		table: components.NewResourceTable(components.ResourceTableConfig{
			Columns: []components.TableColumn{
				{Header: "DSEQ", Width: 10, Align: components.AlignLeft},
				{Header: "PROVIDER", Width: 0, Align: components.AlignLeft},
				{Header: "STATE", Width: 10, Align: components.AlignLeft, RenderFunc: components.StateTag},
				{Header: "PRICE", Width: 18, Align: components.AlignRight},
				{Header: "ESCROW", Width: 12, Align: components.AlignRight},
				{Header: "OPENED", Width: 12, Align: components.AlignLeft},
			},
			EmptyText: "No active leases.",
		}),
	}
}

// SetData stores the records and rebuilds the table rows using the current filter.
func (v *LeasesView) SetData(records []*store.LeaseRecord) {
	v.data = records
	v.applyFilter()
}

// SetSize updates the available width and height for rendering.
func (v *LeasesView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.table.SetSize(w, h)
}

// CycleFilter rotates through "" -> "active" -> "closed" -> "" and re-applies.
func (v *LeasesView) CycleFilter() {
	switch v.filter {
	case "":
		v.filter = "active"
	case "active":
		v.filter = "closed"
	case "closed":
		v.filter = ""
	}
	v.applyFilter()
}

// CursorUp moves the cursor up one row.
func (v *LeasesView) CursorUp() {
	v.table.CursorUp()
}

// CursorDown moves the cursor down one row.
func (v *LeasesView) CursorDown() {
	v.table.CursorDown()
}

// View renders the leases table.
func (v LeasesView) View() string {
	return v.table.View()
}

// applyFilter rebuilds the table rows from v.data using the current filter.
func (v *LeasesView) applyFilter() {
	var filtered []*store.LeaseRecord
	for _, r := range v.data {
		if v.filter != "" && r.State != v.filter {
			continue
		}
		filtered = append(filtered, r)
	}

	rows := make([]components.TableRow, len(filtered))
	for i, r := range filtered {
		rows[i] = components.TableRow{
			ID:    strconv.FormatUint(r.ID.DSeq, 10),
			Cells: leaseCells(r),
		}
	}
	v.table.SetRows(rows)
}

// leaseCells formats a LeaseRecord into cell values matching the column layout.
func leaseCells(r *store.LeaseRecord) []string {
	// DSEQ
	dseq := strconv.FormatUint(r.ID.DSeq, 10)

	// PROVIDER
	provider := r.ID.Provider
	if provider == "" {
		provider = "—"
	}

	// STATE
	state := r.State

	// PRICE
	price := "—"
	if r.Price != "" {
		price = r.Price
	}

	// ESCROW: not directly on LeaseRecord; show "—"
	escrow := "—"

	// OPENED: relative time since created
	opened := "—"
	if r.CreatedAt > 0 {
		opened = relativeTime(r.CreatedAt)
	}

	return []string{dseq, provider, state, price, escrow, opened}
}
