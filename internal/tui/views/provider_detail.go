package views

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/ui/theme"
)

var _ ViewComponent = (*ProviderDetailView)(nil)

// ProviderDetailView is the drill-down detail view for a single on-chain
// provider. It renders two sections: provider info and attributes.
type ProviderDetailView struct {
	BaseDetailView
	km       keys.KeyMap
	provider *ptypes.Provider
}

// NewProviderDetailView creates a detail view pre-loaded with a provider record.
func NewProviderDetailView(km keys.KeyMap, provider *ptypes.Provider) *ProviderDetailView {
	return &ProviderDetailView{
		BaseDetailView: NewBaseDetailView(),
		km:             km,
		provider:       provider,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init returns nil — data is already set via the constructor.
func (v *ProviderDetailView) Init() tea.Cmd { return nil }

// Update handles key events for the provider detail view.
func (v *ProviderDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, v.km.Back):
			return v, CmdFunc(messages.PopViewMsg{})
		default:
			// j/k scroll handled by BaseDetailView
			v.BaseDetailView.Update(msg)
		}
	}
	return v, nil
}

// View renders the provider detail panel.
func (v *ProviderDetailView) View() tea.View {
	if v.provider == nil {
		return tea.NewView(theme.Muted.Render("  No provider selected"))
	}

	p := v.provider
	w := v.W
	if w < 40 {
		w = 40
	}

	var lines []string

	// Section 1: Provider
	lines = append(lines, "  "+theme.SectionTitle.Render("Provider"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		kvPair("Address", theme.KVValue.Render(p.Owner)),
		kvPair("URL", theme.KVValueBold.Render(p.HostURI)),
		kvPair("Region", theme.KVValue.Render(attrValue(p.Attributes, "region"))),
		kvPair("Status", theme.StateBadge("active").Render("—")),
	)
	lines = append(lines, "")

	// Section 2: Attributes
	if len(p.Attributes) > 0 {
		lines = append(lines, "  "+theme.SectionTitle.Render("Attributes"))
		lines = append(lines, "  "+theme.HRuleAccent(w-4))
		for _, attr := range p.Attributes {
			lines = append(lines, kvPair(attr.Key, theme.KVValue.Render(attr.Value)))
		}
	}

	// Apply scrolling via BaseDetailView
	visibleH := v.H - 4
	if visibleH < 3 {
		visibleH = 3
	}

	visible := v.BaseDetailView.VisibleWindow(lines, visibleH)

	var b strings.Builder
	for _, line := range visible {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	return tea.NewView(b.String())
}

// ─── ViewComponent ───────────────────────────────────────────────────

// SetSize delegates to the embedded BaseDetailView.
func (v *ProviderDetailView) SetSize(w, h int) {
	v.BaseDetailView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (v *ProviderDetailView) Breadcrumb() string {
	return "Detail"
}

// ShortHelp returns the footer hint pairs for the provider detail view.
func (v *ProviderDetailView) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "scroll"},
		{Key: "esc", Desc: "back"},
	}
}

// Refresh returns nil — detail views have no data to reload.
func (v *ProviderDetailView) Refresh() tea.Cmd { return nil }

// kvPair renders a single key-value line with 4-space indent and the standard
// KVLabel width. The value is expected to be pre-rendered (already styled).
// This helper is shared across provider, validator, and proposal detail views.
func kvPair(label, value string) string {
	return "    " + theme.KVLabel.Render(label) + value
}
