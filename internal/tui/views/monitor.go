package views

import (
	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/tui/components"
)

// MonitorAdapter wraps the existing internal/monitor/ui tea.Model
// to satisfy the ViewComponent interface without modifying the
// monitor package.
type MonitorAdapter struct {
	Inner tea.Model
	w, h  int
}

func NewMonitorAdapter(inner tea.Model) *MonitorAdapter {
	return &MonitorAdapter{Inner: inner}
}

func (m *MonitorAdapter) Init() tea.Cmd {
	return m.Inner.Init()
}

func (m *MonitorAdapter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Inner, cmd = m.Inner.Update(msg)
	return m, cmd
}

func (m *MonitorAdapter) View() tea.View {
	return m.Inner.View()
}

func (m *MonitorAdapter) SetSize(w, h int) {
	m.w, m.h = w, h
	// The shell has already removed its chrome from h. Forward that exact
	// content height so the monitor does not reserve the status bar twice.
	m.Inner, _ = m.Inner.Update(tea.WindowSizeMsg{
		Width:  w,
		Height: h,
	})
}

func (m *MonitorAdapter) Breadcrumb() string {
	return "Monitor"
}

func (m *MonitorAdapter) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "move"},
		{Key: "tab", Desc: "switch dashboard"},
		{Key: "1/2/3", Desc: "sub-tab"},
		{Key: "r", Desc: "refresh"},
		{Key: "esc", Desc: "back"},
	}
}

func (m *MonitorAdapter) Refresh() tea.Cmd {
	return nil // monitor manages its own data lifecycle
}
