package views

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	attrtypes "pkg.akt.dev/go/node/types/attributes/v1"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/data"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
)

var _ ViewComponent = (*ProvidersView)(nil)

// ProvidersView is a full tea.Model list view for on-chain provider records.
// It embeds BaseListView for cursor/scroll handling and satisfies the
// ViewComponent interface so the App shell can push it onto the nav stack.
type ProvidersView struct {
	BaseListView
	svc   data.Service
	items ptypes.Providers
}

// NewProvidersView creates a ProvidersView wired to the given data service.
func NewProvidersView(svc data.Service, km keys.KeyMap) *ProvidersView {
	cfg := components.ResourceTableConfig{
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
	}
	return &ProvidersView{
		BaseListView: NewBaseListView(cfg, km),
		svc:          svc,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init kicks off the initial data load.
func (v *ProvidersView) Init() tea.Cmd {
	return v.svc.LoadProviders()
}

// Update handles messages for the providers list.
func (v *ProvidersView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.ProvidersLoadedMsg:
		if msg.Err == nil {
			v.SetData(msg.Providers)
		}
		return v, nil
	}

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, v.Keys.Select):
			p := v.selectedProvider()
			if p != nil {
				detail := NewProviderDetailView(v.Keys, p)
				return v, CmdFunc(messages.PushViewMsg{View: detail})
			}
		}
		// Fall through to BaseListView for cursor keys
		v.BaseListView.Update(msg)
	}
	return v, nil
}

// View delegates rendering to the embedded BaseListView table.
func (v *ProvidersView) View() tea.View {
	return v.BaseListView.View()
}

// ─── ViewComponent ───────────────────────────────────────────────────

// SetSize delegates to the embedded BaseListView.
func (v *ProvidersView) SetSize(w, h int) {
	v.BaseListView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (v *ProvidersView) Breadcrumb() string {
	return "Providers"
}

// ShortHelp returns the footer hint pairs for the providers list.
func (v *ProvidersView) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "navigate"},
		{Key: "↵", Desc: "detail"},
		{Key: "esc", Desc: "back"},
	}
}

// Refresh re-fires the data load for this view.
func (v *ProvidersView) Refresh() tea.Cmd {
	return v.svc.LoadProviders()
}

// ─── Internal ────────────────────────────────────────────────────────

// selectedProvider returns the provider record at the cursor, or nil.
func (v *ProvidersView) selectedProvider() *ptypes.Provider {
	row := v.BaseListView.SelectedRow()
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
	v.BaseListView.SetRows(rows)
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
