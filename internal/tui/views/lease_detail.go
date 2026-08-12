package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/ui/theme"
)

// Compile-time check: *LeaseDetailView must satisfy ViewComponent.
var _ ViewComponent = (*LeaseDetailView)(nil)

// LeaseDetailView is the drill-down detail view for a single lease.
// It renders 5 sections: Lease, Order, Settlement, Provider Status, and Endpoints.
type LeaseDetailView struct {
	BaseDetailView
	km    keys.KeyMap
	lease *store.LeaseRecord
}

// NewLeaseDetailView creates a new lease detail view pre-loaded with a lease record.
func NewLeaseDetailView(km keys.KeyMap, lease *store.LeaseRecord) *LeaseDetailView {
	return &LeaseDetailView{
		BaseDetailView: NewBaseDetailView(),
		km:             km,
		lease:          lease,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init returns nil — data is passed via constructor.
func (v *LeaseDetailView) Init() tea.Cmd {
	return nil
}

// Update handles messages for the lease detail view.
func (v *LeaseDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(kmsg, v.km.Back) {
			return v, CmdFunc(messages.PopViewMsg{})
		}
		// Delegate j/k scroll to BaseDetailView
		v.BaseDetailView.Update(msg)
	}
	return v, nil
}

// ─── ViewComponent ───────────────────────────────────────────────────

// View renders the lease detail panel.
func (v *LeaseDetailView) View() tea.View {
	if v.lease == nil {
		return tea.NewView(theme.Muted.Render("  No lease selected"))
	}

	w := v.W
	if w < 40 {
		w = 40
	}
	contentW := w - 4
	if contentW < 30 {
		contentW = 30
	}

	l := v.lease
	var sections []string

	// Section 1: Lease
	age := "—"
	if l.CreatedAt > 0 {
		age = relativeTime(l.CreatedAt)
	}
	sections = append(sections, components.Section("Lease", contentW)+"\n"+strings.Join([]string{
		components.KVBold("DSEQ", fmt.Sprintf("%d", l.ID.DSeq)),
		components.KV("GSEQ/OSEQ", fmt.Sprintf("%d/%d", l.ID.GSeq, l.ID.OSeq)),
		components.KV("State", components.StateTag(l.State)),
		components.KV("Provider", valOrDash(l.ID.Provider)),
		components.KV("Price", valOrDash(formatStoredCoins(l.Price))+"/block"),
		components.KV("Age", age),
	}, "\n"))

	// Section 2: Order
	orderID := fmt.Sprintf("%d/%d/%d", l.ID.DSeq, l.ID.GSeq, l.ID.OSeq)
	bidID := fmt.Sprintf("%d/%d/%d/%s", l.ID.DSeq, l.ID.GSeq, l.ID.OSeq, l.ID.Provider)
	sections = append(sections, components.Section("Order", contentW)+"\n"+strings.Join([]string{
		components.KVBold("Order ID", orderID),
		components.KV("Bid ID", bidID),
		components.KV("Order State", components.StateTag("matched")),
	}, "\n"))

	// Section 3: Settlement (placeholder — data not yet available)
	sections = append(sections, components.Section("Settlement", contentW)+"\n"+strings.Join([]string{
		components.KV("Last Settled", "—"),
		components.KV("Settled Amt", "—"),
		components.KVBold("Funds Left", "—"),
		components.KV("Withdrawn", "—"),
	}, "\n"))

	// Section 4: Provider Status (placeholder — requires live provider query)
	sections = append(sections, components.SectionWithKV("Provider Status", contentW, []components.KVPair{
		{Label: "Status", Value: "—"},
	}))

	// Section 5: Endpoints
	if len(l.Endpoints) > 0 {
		epSection := components.Section("Endpoints", contentW)
		var epLines []string
		for _, ep := range l.Endpoints {
			uri := valOrDash(ep.URI)
			epLines = append(epLines, components.KV(ep.Service, uri))
		}
		sections = append(sections, epSection+"\n"+strings.Join(epLines, "\n"))
	}

	content := strings.Join(sections, "\n\n")

	// Apply scrolling via BaseDetailView
	lines := strings.Split(content, "\n")
	visibleH := v.H - 4 // back hint + padding
	if visibleH < 3 {
		visibleH = 3
	}

	visible := v.BaseDetailView.VisibleWindow(lines, visibleH)

	var b strings.Builder
	for _, line := range visible {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(theme.Muted.Render("  esc: back"))
	if len(lines) > visibleH {
		b.WriteString(theme.Muted.Render("  j/k: scroll"))
	}
	b.WriteByte('\n')

	return tea.NewView(b.String())
}

// SetSize delegates to the embedded BaseDetailView.
func (v *LeaseDetailView) SetSize(w, h int) {
	v.BaseDetailView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (v *LeaseDetailView) Breadcrumb() string {
	return "Detail"
}

// ShortHelp returns the footer hint pairs for the lease detail view.
func (v *LeaseDetailView) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "scroll"},
		{Key: "esc", Desc: "back"},
	}
}

// Refresh returns nil — data is passed via constructor.
func (v *LeaseDetailView) Refresh() tea.Cmd {
	return nil
}

// Lease returns the currently displayed lease record, or nil.
func (v *LeaseDetailView) Lease() *store.LeaseRecord {
	return v.lease
}

// ─── Helpers ─────────────────────────────────────────────────────────
