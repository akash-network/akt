package views

import (
	"strings"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// ProviderDetailView is the drill-down detail view for a single on-chain
// provider. It renders two sections: provider info and attributes.
type ProviderDetailView struct {
	provider *ptypes.Provider
	width    int
	height   int
	scroll   int
}

// NewProviderDetailView creates a new empty provider detail view.
func NewProviderDetailView() ProviderDetailView {
	return ProviderDetailView{}
}

// SetProvider sets the provider to display and resets scroll.
func (v *ProviderDetailView) SetProvider(p *ptypes.Provider) {
	v.provider = p
	v.scroll = 0
}

// Provider returns the currently displayed provider, or nil.
func (v ProviderDetailView) Provider() *ptypes.Provider {
	return v.provider
}

// SetSize updates the available width and height for rendering.
func (v *ProviderDetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp scrolls the content up by one line.
func (v *ProviderDetailView) ScrollUp() {
	if v.scroll > 0 {
		v.scroll--
	}
}

// ScrollDown scrolls the content down by one line.
func (v *ProviderDetailView) ScrollDown() {
	v.scroll++
}

// View renders the provider detail panel.
func (v ProviderDetailView) View() string {
	if v.provider == nil {
		return theme.Muted.Render("  No provider selected")
	}

	p := v.provider
	w := v.width
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

	// Apply scrolling
	visibleH := v.height - 4
	if visibleH < 3 {
		visibleH = 3
	}

	start := v.scroll
	if start >= len(lines) {
		start = max(0, len(lines)-1)
	}
	end := start + visibleH
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}

	return b.String()
}

// kvPair renders a single key-value line with 4-space indent and the standard
// KVLabel width. The value is expected to be pre-rendered (already styled).
// This helper is shared across provider, validator, and proposal detail views.
func kvPair(label, value string) string {
	return "    " + theme.KVLabel.Render(label) + value
}
