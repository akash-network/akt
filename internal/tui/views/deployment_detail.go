package views

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/ui/theme"
)

const numTabs = 4

var tabLabels = [numTabs]string{"overview", "lease", "escrow", "endpoints"}

// DeploymentDetailView is the drill-down detail view for a single deployment.
// It contains 4 sub-tabs: overview, lease, escrow, and endpoints.
type DeploymentDetailView struct {
	deployment *store.DeploymentRecord
	leases     []*store.LeaseRecord
	bids       []*store.BidRecord
	tab        int // 0=overview, 1=lease, 2=escrow, 3=endpoints
	width      int
	height     int
	scroll     int
}

// NewDeploymentDetailView creates a new empty deployment detail view.
func NewDeploymentDetailView() DeploymentDetailView {
	return DeploymentDetailView{}
}

// SetDeployment sets the deployment record to display.
func (v *DeploymentDetailView) SetDeployment(d *store.DeploymentRecord) {
	v.deployment = d
	v.scroll = 0
}

// SetLeases sets the lease records for this deployment.
func (v *DeploymentDetailView) SetLeases(leases []*store.LeaseRecord) {
	v.leases = leases
}

// SetBids sets the bid records for this deployment.
func (v *DeploymentDetailView) SetBids(bids []*store.BidRecord) {
	v.bids = bids
}

// SetSize updates the view dimensions.
func (v *DeploymentDetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// NextTab advances to the next sub-tab.
func (v *DeploymentDetailView) NextTab() {
	v.tab = (v.tab + 1) % numTabs
	v.scroll = 0
}

// PrevTab moves to the previous sub-tab.
func (v *DeploymentDetailView) PrevTab() {
	v.tab = (v.tab - 1 + numTabs) % numTabs
	v.scroll = 0
}

// SetTab sets the active sub-tab directly.
func (v *DeploymentDetailView) SetTab(n int) {
	if n >= 0 && n < numTabs {
		v.tab = n
		v.scroll = 0
	}
}

// ScrollUp scrolls the content up by one line.
func (v *DeploymentDetailView) ScrollUp() {
	if v.scroll > 0 {
		v.scroll--
	}
}

// ScrollDown scrolls the content down by one line.
func (v *DeploymentDetailView) ScrollDown() {
	v.scroll++
}

// HasData returns true if a deployment is loaded.
func (v DeploymentDetailView) HasData() bool {
	return v.deployment != nil
}

// View renders the deployment detail panel.
func (v DeploymentDetailView) View() string {
	if v.deployment == nil {
		return theme.Muted.Render("  No deployment selected")
	}

	w := v.width
	if w < 40 {
		w = 40
	}

	var b strings.Builder

	// Header strip
	b.WriteString(v.renderHeader(w))
	b.WriteByte('\n')

	// Tab bar
	b.WriteString(v.renderTabBar())
	b.WriteString("\n\n")

	// Tab content
	content := v.renderTabContent(w)

	// Apply scrolling
	lines := strings.Split(content, "\n")
	maxLines := v.height - 7 // header(2) + tab bar(1) + blank(1) + back hint(2) + padding(1)
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

	for i := start; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}

	// Back hint and scroll indicator
	b.WriteByte('\n')
	b.WriteString(theme.Muted.Render("  esc: back"))
	if len(lines) > maxLines {
		b.WriteString(lipgloss.NewStyle().Foreground(theme.ColorMuted).Render(
			"  j/k: scroll  1-4: tabs",
		))
	}
	b.WriteByte('\n')

	return b.String()
}

// renderHeader renders the header strip with deployment name, DSEQ, state, and owner.
func (v DeploymentDetailView) renderHeader(w int) string {
	d := v.deployment

	// Deployment name from SDL path basename or fallback
	name := "deployment"
	if d.SDLPath != "" {
		name = filepath.Base(d.SDLPath)
	}

	dseq := theme.Heading.Render(strconv.FormatUint(d.DSeq, 10))
	state := components.StateTag(d.State)

	// Truncate owner for display
	owner := d.Owner
	if len(owner) > 20 {
		owner = owner[:10] + "…" + owner[len(owner)-8:]
	}
	ownerStr := theme.Muted.Render(owner)

	header := fmt.Sprintf("  %s  %s  %s  %s",
		theme.Secondary.Render(name), dseq, state, ownerStr)

	return header + "\n" + theme.HRule(w)
}

// renderTabBar renders the sub-tab bar.
func (v DeploymentDetailView) renderTabBar() string {
	activeNum := lipgloss.NewStyle().Foreground(theme.AccentRed).Bold(true)
	activeText := lipgloss.NewStyle().Foreground(theme.Slate100).Bold(true)
	inactiveNum := lipgloss.NewStyle().Foreground(theme.Slate500)
	inactiveText := lipgloss.NewStyle().Foreground(theme.Slate400)

	var tabs []string
	for i, label := range tabLabels {
		num := strconv.Itoa(i + 1)
		if i == v.tab {
			tabs = append(tabs, activeNum.Render(num)+" "+activeText.Render(label))
		} else {
			tabs = append(tabs, inactiveNum.Render(num)+" "+inactiveText.Render(label))
		}
	}

	return "  " + strings.Join(tabs, "   ")
}

// renderTabContent renders the content for the active tab.
func (v DeploymentDetailView) renderTabContent(w int) string {
	contentW := w - 4 // indent padding
	if contentW < 30 {
		contentW = 30
	}

	var content string
	switch v.tab {
	case 0:
		content = v.renderOverviewTab(contentW)
	case 1:
		content = v.renderLeaseTab(contentW)
	case 2:
		content = v.renderEscrowTab(contentW)
	case 3:
		content = v.renderEndpointsTab(contentW)
	}

	return content
}

// renderOverviewTab renders the overview tab content.
func (v DeploymentDetailView) renderOverviewTab(w int) string {
	d := v.deployment
	var sections []string

	// Resources section
	sections = append(sections, components.SectionWithKV("Resources", w, []components.KVPair{
		{Label: "cpu", Value: labelVal(d.Labels, "cpu")},
		{Label: "memory", Value: labelVal(d.Labels, "memory")},
		{Label: "gpu", Value: labelVal(d.Labels, "gpu")},
		{Label: "storage", Value: labelVal(d.Labels, "storage")},
	}))

	// Placement section
	sections = append(sections, components.SectionWithKV("Placement", w, []components.KVPair{
		{Label: "provider", Value: v.providerAddr()},
		{Label: "region", Value: labelVal(d.Labels, "region")},
		{Label: "uptime", Value: v.uptimeStr()},
		{Label: "cost", Value: v.costStr()},
	}))

	// SDL info section
	sdlHash := valOrDash(d.SDLHash)
	sdlPath := valOrDash(d.SDLPath)
	sections = append(sections, components.SectionWithKV("SDL", w, []components.KVPair{
		{Label: "hash", Value: sdlHash},
		{Label: "path", Value: sdlPath},
	}))

	return strings.Join(sections, "\n\n")
}

// renderLeaseTab renders the lease tab content.
func (v DeploymentDetailView) renderLeaseTab(w int) string {
	var sections []string

	if len(v.leases) == 0 {
		sections = append(sections, components.Section("Active Lease", w))
		sections = append(sections, theme.Muted.Render("  No active leases"))
	} else {
		l := v.leases[0] // primary lease
		sections = append(sections, components.SectionWithKV("Active Lease", w, []components.KVPair{
			{Label: "provider", Value: truncAddr(l.ID.Provider)},
			{Label: "state", Value: components.StateTag(l.State)},
			{Label: "price", Value: valOrDash(l.Price)},
			{Label: "opened", Value: fmtTimestamp(l.CreatedAt)},
			{Label: "gseq", Value: strconv.FormatUint(uint64(l.ID.GSeq), 10)},
			{Label: "oseq", Value: strconv.FormatUint(uint64(l.ID.OSeq), 10)},
		}))
	}

	// Bid history
	sections = append(sections, components.Section("Bid History", w))
	if len(v.bids) == 0 {
		sections = append(sections, theme.Muted.Render("  No bids"))
	} else {
		var bidLines []string
		for _, bid := range v.bids {
			provider := truncAddr(bid.ID.Provider)
			price := valOrDash(bid.Price)
			state := components.StateTag(bid.State)
			bidLines = append(bidLines,
				fmt.Sprintf("  %s  %s  %s", theme.Secondary.Render(provider), price, state))
		}
		sections = append(sections, strings.Join(bidLines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

// renderEscrowTab renders the escrow tab content.
func (v DeploymentDetailView) renderEscrowTab(w int) string {
	d := v.deployment
	var sections []string

	sections = append(sections, components.SectionWithKV("Escrow", w, []components.KVPair{
		{Label: "deposit", Value: valOrDash(d.Deposit)},
		{Label: "balance", Value: valOrDash(d.EscrowBalance)},
		{Label: "transferred", Value: valOrDash(d.Transferred)},
	}))

	// Progress bar for remaining balance if data is available
	if d.Deposit != "" && d.EscrowBalance != "" {
		pct := escrowPercent(d.Deposit, d.EscrowBalance)
		if pct >= 0 {
			barW := w
			if barW > 60 {
				barW = 60
			}
			sections = append(sections,
				components.KV("remaining", components.FormatPercent(pct)),
			)
			sections = append(sections, "  "+components.ProgressBar(pct, barW))
		}
	}

	return strings.Join(sections, "\n\n")
}

// renderEndpointsTab renders the endpoints tab content.
func (v DeploymentDetailView) renderEndpointsTab(w int) string {
	sections := []string{components.Section("Endpoints", w)}

	// Collect all endpoints from all leases
	var endpoints []store.LeaseEndpoint
	for _, l := range v.leases {
		endpoints = append(endpoints, l.Endpoints...)
	}

	if len(endpoints) == 0 {
		sections = append(sections, theme.Muted.Render("  No endpoints available"))
	} else {
		for _, ep := range endpoints {
			port := strconv.FormatUint(uint64(ep.ExternalPort), 10)
			uri := valOrDash(ep.URI)
			sections = append(sections, components.KVBlock([]components.KVPair{
				{Label: "service", Value: ep.Service},
				{Label: "port", Value: port},
				{Label: "url", Value: uri},
			}))
		}
	}

	return strings.Join(sections, "\n\n")
}

// ─── Helpers ─────────────────────────────────────────────────────────

// labelVal returns the value for a label key, or "—" if not found.
func labelVal(labels map[string]string, key string) string {
	if labels == nil {
		return "—"
	}
	if v, ok := labels[key]; ok && v != "" {
		return v
	}
	return "—"
}

// valOrDash returns the value or "—" if empty.
func valOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// truncAddr truncates a long address for display.
func truncAddr(addr string) string {
	if len(addr) > 20 {
		return addr[:10] + "…" + addr[len(addr)-8:]
	}
	return addr
}

// fmtTimestamp formats a Unix timestamp for display.
func fmtTimestamp(ts int64) string {
	if ts == 0 {
		return "—"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}

// providerAddr returns the provider address from the first lease, or "—".
func (v DeploymentDetailView) providerAddr() string {
	if len(v.leases) > 0 {
		return truncAddr(v.leases[0].ID.Provider)
	}
	return "—"
}

// uptimeStr computes uptime from the first lease's created timestamp.
func (v DeploymentDetailView) uptimeStr() string {
	if len(v.leases) == 0 || v.leases[0].CreatedAt == 0 {
		return "—"
	}
	d := time.Since(time.Unix(v.leases[0].CreatedAt, 0))
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		days := int(d.Hours()) / 24
		hours := int(d.Hours()) % 24
		return fmt.Sprintf("%dd %dh", days, hours)
	}
}

// costStr returns the price from the first lease, or "—".
func (v DeploymentDetailView) costStr() string {
	if len(v.leases) > 0 && v.leases[0].Price != "" {
		return v.leases[0].Price
	}
	return "—"
}

// escrowPercent computes the fraction of deposit remaining as escrow balance.
// Returns -1 if values cannot be parsed.
func escrowPercent(deposit, balance string) float64 {
	d, err := strconv.ParseFloat(deposit, 64)
	if err != nil || d <= 0 {
		return -1
	}
	b, err := strconv.ParseFloat(balance, 64)
	if err != nil {
		return -1
	}
	pct := b / d
	if pct < 0 {
		return 0
	}
	if pct > 1 {
		return 1
	}
	return pct
}
