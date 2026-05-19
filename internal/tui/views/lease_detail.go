package views

import (
	"fmt"
	"strings"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/ui/theme"
)

// LeaseDetailView is the drill-down detail view for a single lease.
// It renders 5 sections: Lease, Order, Settlement, Provider Status, and Endpoints.
type LeaseDetailView struct {
	lease  *store.LeaseRecord
	width  int
	height int
	scroll int
}

// NewLeaseDetailView creates a new empty lease detail view.
func NewLeaseDetailView() LeaseDetailView {
	return LeaseDetailView{}
}

// Lease returns the currently displayed lease record, or nil.
func (v LeaseDetailView) Lease() *store.LeaseRecord {
	return v.lease
}

// SetLease sets the lease record to display.
func (v *LeaseDetailView) SetLease(l *store.LeaseRecord) {
	v.lease = l
	v.scroll = 0
}

// SetSize updates the view dimensions.
func (v *LeaseDetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp scrolls the content up by one line.
func (v *LeaseDetailView) ScrollUp() {
	if v.scroll > 0 {
		v.scroll--
	}
}

// ScrollDown scrolls the content down by one line.
func (v *LeaseDetailView) ScrollDown() {
	v.scroll++
}

// View renders the lease detail panel.
func (v LeaseDetailView) View() string {
	if v.lease == nil {
		return theme.Muted.Render("  No lease selected")
	}

	w := v.width
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
		components.KV("Price", valOrDash(l.Price)+"/block"),
		components.KV("Age", age),
	}, "\n"))

	// Section 2: Order
	orderID := fmt.Sprintf("%d/%d/%d", l.ID.DSeq, l.ID.GSeq, l.ID.OSeq)
	bidID := fmt.Sprintf("%d/%d/%d/%s", l.ID.DSeq, l.ID.GSeq, l.ID.OSeq, truncateAddr(l.ID.Provider))
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

	// Apply scrolling
	lines := strings.Split(content, "\n")
	maxLines := v.height - 4 // back hint + padding
	if maxLines < 3 {
		maxLines = 3
	}

	start := v.scroll
	if start >= len(lines) {
		start = max(0, len(lines)-1)
	}
	end := start + maxLines
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(theme.Muted.Render("  esc: back"))
	if len(lines) > maxLines {
		b.WriteString(theme.Muted.Render("  j/k: scroll"))
	}
	b.WriteByte('\n')

	return b.String()
}

// truncateAddr truncates an address to the first 12 characters followed by "...".
// If the address is 15 characters or shorter, it is returned as-is.
func truncateAddr(addr string) string {
	if len(addr) <= 15 {
		return addr
	}
	return addr[:12] + "..."
}
