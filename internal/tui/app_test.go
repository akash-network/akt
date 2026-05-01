package tui

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/spf13/viper"

	"pkg.akt.dev/akt/internal/tui/commands"
	"pkg.akt.dev/akt/internal/tui/views"
)

const (
	testAppWidth  = 80
	testAppHeight = 24
)

func newTestApp(view activeView, standalone bool) App {
	v := viper.New()
	km := KeyMapFromConfig(v)
	reg := commands.DefaultRegistry()

	return App{
		keys:        km,
		view:        view,
		standalone:  standalone,
		width:       testAppWidth,
		height:      testAppHeight,
		deployments: views.NewListView(views.ListViewConfig{Title: "Deployments", Empty: "No deployments"}),
		leases:      views.NewListView(views.ListViewConfig{Title: "Leases", Empty: "No leases"}),
		providers:   views.NewListView(views.ListViewConfig{Title: "Providers", Empty: "No providers"}),
		governance:  views.NewListView(views.ListViewConfig{Title: "Governance", Empty: "No proposals"}),
		staking:     views.NewListView(views.ListViewConfig{Title: "Staking", Empty: "No validators"}),
		detail:      views.NewDetailView(),
		palette: views.NewCommandPalette(reg, views.PaletteKeys{
			CursorUp:   km.CursorUp,
			CursorDown: km.CursorDown,
			Select:     km.Select,
			Close:      km.Back,
		}),
	}
}

func TestAppRenderHeader(t *testing.T) {
	a := newTestApp(viewDashboard, false)
	golden.RequireEqual(t, a.renderHeader())
}

func TestAppRenderDashboard(t *testing.T) {
	a := newTestApp(viewDashboard, false)
	golden.RequireEqual(t, a.renderDashboard(20))
}

func TestAppRenderStatusBar(t *testing.T) {
	tests := map[string]struct {
		view       activeView
		standalone bool
	}{
		"Dashboard":   {viewDashboard, false},
		"Deployments": {viewDeployments, false},
		"Leases":      {viewLeases, false},
		"Providers":   {viewProviders, false},
		"Monitor":     {viewMonitor, false},
		"Governance":  {viewGovernance, false},
		"Staking":     {viewStaking, false},
		"Standalone":  {viewDashboard, true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a := newTestApp(tc.view, tc.standalone)
			golden.RequireEqual(t, a.renderStatusBar())
		})
	}
}

func TestAppRenderPaletteStatusBar(t *testing.T) {
	a := newTestApp(viewDashboard, false)
	golden.RequireEqual(t, a.renderPaletteStatusBar())
}

func TestAppRenderCentered(t *testing.T) {
	tests := map[string]struct {
		text string
	}{
		"Short":     {"Hello"},
		"Multiline": {"akt - Akash Network\nWelcome"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a := newTestApp(viewDashboard, false)
			golden.RequireEqual(t, a.renderCentered(20, tc.text))
		})
	}
}
