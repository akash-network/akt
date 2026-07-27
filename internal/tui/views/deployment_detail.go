package views

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/data"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/ui/theme"
)

const numTabs = 4

var tabLabels = [numTabs]string{"overview", "lease", "escrow", "endpoints"}

// Compile-time check: *DeploymentDetailView must satisfy ViewComponent.
var _ ViewComponent = (*DeploymentDetailView)(nil)

// DeploymentDetailView is the drill-down detail view for a single deployment.
// It contains 4 sub-tabs: overview, lease, escrow, and endpoints.
type DeploymentDetailView struct {
	BaseDetailView
	svc        data.Service
	km         keys.KeyMap
	deployment *store.DeploymentRecord
	leases     []*store.LeaseRecord
	bids       []*store.BidRecord
	tab        int // 0=overview, 1=lease, 2=escrow, 3=endpoints
}

// NewDeploymentDetailView constructs a detail view pre-loaded with a deployment record.
func NewDeploymentDetailView(svc data.Service, km keys.KeyMap, rec *store.DeploymentRecord) *DeploymentDetailView {
	return &DeploymentDetailView{
		BaseDetailView: NewBaseDetailView(),
		svc:            svc,
		km:             km,
		deployment:     rec,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init fires the initial data loads for leases and bids.
func (v *DeploymentDetailView) Init() tea.Cmd {
	if v.svc == nil || v.deployment == nil {
		return nil
	}
	return tea.Batch(
		v.svc.LoadDeploymentLeases(v.deployment.Owner, v.deployment.DSeq),
		v.svc.LoadBids(v.deployment.Owner, v.deployment.DSeq),
	)
}

// Update handles messages for the deployment detail view.
func (v *DeploymentDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.LeasesLoadedMsg:
		if msg.Err == nil {
			v.leases = msg.Leases
		}
		return v, nil

	case messages.BidsLoadedMsg:
		if msg.Err == nil {
			v.bids = msg.Bids
		}
		return v, nil
	}

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, v.km.Back):
			return v, CmdFunc(messages.PopViewMsg{})

		case key.Matches(kmsg, v.km.TabNext):
			v.tab = (v.tab + 1) % numTabs
			v.Scroll = 0
			return v, nil
		}

		switch kmsg.String() {
		case "1":
			v.tab = 0
			v.Scroll = 0
			return v, nil
		case "2":
			v.tab = 1
			v.Scroll = 0
			return v, nil
		case "3":
			v.tab = 2
			v.Scroll = 0
			return v, nil
		case "4":
			v.tab = 3
			v.Scroll = 0
			return v, nil
		}

		// Delegate j/k scroll to BaseDetailView
		v.BaseDetailView.Update(msg)
	}
	return v, nil
}

// ─── ViewComponent ───────────────────────────────────────────────────

// View renders the deployment detail panel.
func (v *DeploymentDetailView) View() tea.View {
	if v.deployment == nil {
		return tea.NewView(theme.Muted.Render("  No deployment selected"))
	}

	w := v.W
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

	// Apply scrolling via BaseDetailView
	lines := strings.Split(content, "\n")
	visibleH := v.H - 7 // header(2) + tab bar(1) + blank(1) + back hint(2) + padding(1)
	if visibleH < 3 {
		visibleH = 3
	}

	visible := v.BaseDetailView.VisibleWindow(lines, visibleH)
	for _, line := range visible {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	// Back hint and scroll indicator
	b.WriteByte('\n')
	b.WriteString(theme.Muted.Render("  esc: back"))
	if len(lines) > visibleH {
		b.WriteString(lipgloss.NewStyle().Foreground(theme.Slate500).Render(
			"  j/k: scroll  1-4: tabs",
		))
	}
	b.WriteByte('\n')

	return tea.NewView(b.String())
}

// SetSize delegates to the embedded BaseDetailView.
func (v *DeploymentDetailView) SetSize(w, h int) {
	v.BaseDetailView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (v *DeploymentDetailView) Breadcrumb() string {
	return "Detail"
}

// ShortHelp returns the footer hint pairs for the deployment detail view.
func (v *DeploymentDetailView) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "scroll"},
		{Key: "1-4", Desc: "tabs"},
		{Key: "tab", Desc: "next tab"},
		{Key: "esc", Desc: "back"},
	}
}

// Refresh re-fires the same data loads as Init.
func (v *DeploymentDetailView) Refresh() tea.Cmd {
	return v.Init()
}

// Deployment returns the currently displayed deployment record, or nil.
func (v *DeploymentDetailView) Deployment() *store.DeploymentRecord {
	return v.deployment
}

// ─── Rendering helpers ───────────────────────────────────────────────

// renderHeader renders the header strip with deployment name, DSEQ, state, and owner.
func (v *DeploymentDetailView) renderHeader(w int) string {
	d := v.deployment

	// Deployment name from SDL path basename or fallback
	name := "deployment"
	if d.SDLPath != "" {
		name = filepath.Base(d.SDLPath)
	}

	dseq := theme.Heading.Render(strconv.FormatUint(d.DSeq, 10))
	state := components.StateTag(d.State)

	ownerStr := theme.Muted.Render(d.Owner)

	header := fmt.Sprintf("  %s  %s  %s  %s",
		theme.Secondary.Render(name), dseq, state, ownerStr)

	return header + "\n" + theme.HRule(w)
}

// renderTabBar renders the sub-tab bar.
func (v *DeploymentDetailView) renderTabBar() string {
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
func (v *DeploymentDetailView) renderTabContent(w int) string {
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
func (v *DeploymentDetailView) renderOverviewTab(w int) string {
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
func (v *DeploymentDetailView) renderLeaseTab(w int) string {
	var sections []string

	if len(v.leases) == 0 {
		sections = append(sections, components.Section("Active Lease", w))
		sections = append(sections, theme.Muted.Render("  No active leases"))
	} else {
		l := v.leases[0] // primary lease
		sections = append(sections, components.SectionWithKV("Active Lease", w, []components.KVPair{
			{Label: "provider", Value: l.ID.Provider},
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
			provider := bid.ID.Provider
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
func (v *DeploymentDetailView) renderEscrowTab(w int) string {
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
func (v *DeploymentDetailView) renderEndpointsTab(w int) string {
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

// fmtTimestamp formats a Unix timestamp for display.
func fmtTimestamp(ts int64) string {
	if ts == 0 {
		return "—"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}

// providerAddr returns the provider address from the first lease, or "—".
func (v *DeploymentDetailView) providerAddr() string {
	if len(v.leases) > 0 {
		return v.leases[0].ID.Provider
	}
	return "—"
}

// uptimeStr computes uptime from the first lease's created timestamp.
func (v *DeploymentDetailView) uptimeStr() string {
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
func (v *DeploymentDetailView) costStr() string {
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
