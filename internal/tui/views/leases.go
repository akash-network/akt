package views

import (
	"fmt"
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/data"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
)

// Compile-time check: *LeasesView must satisfy ViewComponent.
var _ ViewComponent = (*LeasesView)(nil)

// LeasesView is a full tea.Model list view for lease records.
// It embeds BaseListView for cursor/scroll handling and satisfies the
// ViewComponent interface so the App shell can push it onto the nav stack.
type LeasesView struct {
	BaseListView
	svc    data.Service
	owner  string
	data   []*store.LeaseRecord
	filter string // "", "active", "closed"
}

// NewLeasesView creates a LeasesView wired to the given data service.
func NewLeasesView(svc data.Service, km keys.KeyMap, owner string) *LeasesView {
	cfg := components.ResourceTableConfig{
		Columns: []components.TableColumn{
			{Header: "DSEQ", Width: 10, Align: components.AlignLeft},
			{Header: "GSEQ", Width: 6, Align: components.AlignRight},
			{Header: "OSEQ", Width: 6, Align: components.AlignRight},
			{Header: "PROVIDER", Width: 0, Align: components.AlignLeft},
			{Header: "STATE", Width: 10, Align: components.AlignLeft, RenderFunc: components.StateTag},
			{Header: "PRICE", Width: 18, Align: components.AlignRight},
			{Header: "ESCROW", Width: 12, Align: components.AlignRight},
			{Header: "OPENED", Width: 12, Align: components.AlignLeft},
		},
		EmptyText: "No active leases.",
	}
	return &LeasesView{
		BaseListView: NewBaseListView(cfg, km),
		svc:          svc,
		owner:        owner,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init kicks off the initial data load.
func (v *LeasesView) Init() tea.Cmd {
	if v.svc == nil {
		return nil
	}
	return v.svc.LoadLeases(v.owner)
}

// Update handles messages for the leases list.
func (v *LeasesView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.LeasesLoadedMsg:
		if msg.Err == nil {
			v.data = msg.Leases
			v.applyFilter()
		}
		return v, nil
	}

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, v.Keys.Select):
			rec := v.selectedRecord()
			if rec != nil {
				detail := NewLeaseDetailView(v.Keys, rec)
				return v, CmdFunc(messages.PushViewMsg{View: detail})
			}
		case key.Matches(kmsg, v.Keys.Logs):
			rec := v.selectedRecord()
			if rec != nil {
				return v, CmdFunc(messages.StartLogStreamMsg{Owner: rec.ID.Owner, DSeq: rec.ID.DSeq})
			}
		case key.Matches(kmsg, v.Keys.Filter):
			v.cycleFilter()
			return v, v.svc.LoadLeases(v.owner)
		}
		// Fall through to BaseListView for cursor keys
		v.BaseListView.Update(msg)
	}
	return v, nil
}

// View delegates rendering to the embedded BaseListView table.
func (v *LeasesView) View() tea.View {
	return v.BaseListView.View()
}

// ─── ViewComponent ───────────────────────────────────────────────────

// SetSize delegates to the embedded BaseListView.
func (v *LeasesView) SetSize(w, h int) {
	v.BaseListView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (v *LeasesView) Breadcrumb() string {
	return "Leases"
}

// ShortHelp returns the footer hint pairs for the leases list.
func (v *LeasesView) ShortHelp() []components.HintPair {
	hints := []components.HintPair{
		{Key: "j/k", Desc: "navigate"},
		{Key: "↵", Desc: "detail"},
		{Key: "f", Desc: "filter"},
		{Key: "l", Desc: "logs"},
		{Key: "esc", Desc: "back"},
	}
	if v.filter != "" {
		hints = append(hints, components.HintPair{Key: "filter", Desc: v.filter})
	}
	return hints
}

// Refresh re-fires the data load for this view.
func (v *LeasesView) Refresh() tea.Cmd {
	if v.svc == nil {
		return nil
	}
	return v.svc.LoadLeases(v.owner)
}

// ─── Internal ────────────────────────────────────────────────────────

// selectedRecord returns the lease record at the cursor, or nil.
func (v *LeasesView) selectedRecord() *store.LeaseRecord {
	row := v.BaseListView.SelectedRow()
	if row == nil {
		return nil
	}
	dseq, _ := strconv.ParseUint(row.ID, 10, 64)
	for _, r := range v.data {
		if r.ID.DSeq == dseq {
			return r
		}
	}
	return nil
}

// cycleFilter rotates through "" -> "active" -> "closed" -> "" and re-applies.
func (v *LeasesView) cycleFilter() {
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
	v.BaseListView.SetRows(rows)
}

// leaseCells formats a LeaseRecord into cell values matching the column layout.
func leaseCells(r *store.LeaseRecord) []string {
	// DSEQ
	dseq := strconv.FormatUint(r.ID.DSeq, 10)

	// GSEQ
	gseq := fmt.Sprintf("%d", r.ID.GSeq)

	// OSEQ
	oseq := fmt.Sprintf("%d", r.ID.OSeq)

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

	return []string{dseq, gseq, oseq, provider, state, price, escrow, opened}
}
