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

	dash := views.NewDashboard()
	dash.SetSize(testAppWidth, testAppHeight-chromeHeight)

	return App{
		keys:        km,
		view:        view,
		standalone:  standalone,
		width:       testAppWidth,
		height:      testAppHeight,
		dashboard:   dash,
		deployments: views.NewDeploymentsView(),
		leases:      views.NewLeasesView(),
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
	golden.RequireEqual(t, a.dashboard.View())
}

func TestAppRenderFooter(t *testing.T) {
	tests := map[string]struct {
		view activeView
	}{
		"Dashboard":   {viewDashboard},
		"Deployments": {viewDeployments},
		"Leases":      {viewLeases},
		"Providers":   {viewProviders},
		"Monitor":     {viewMonitor},
		"Governance":  {viewGovernance},
		"Staking":     {viewStaking},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a := newTestApp(tc.view, false)
			golden.RequireEqual(t, a.renderFooter())
		})
	}
}

func TestAppRenderPaletteFooter(t *testing.T) {
	a := newTestApp(viewDashboard, false)
	golden.RequireEqual(t, a.renderPaletteFooter())
}

func TestAppRenderNavBar(t *testing.T) {
	tests := map[string]struct {
		view activeView
	}{
		"Dashboard":   {viewDashboard},
		"Deployments": {viewDeployments},
		"Monitor":     {viewMonitor},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a := newTestApp(tc.view, false)
			golden.RequireEqual(t, a.renderNavBar())
		})
	}
}

func TestAppRenderBreadcrumb(t *testing.T) {
	tests := map[string]struct {
		view activeView
	}{
		"Dashboard":   {viewDashboard},
		"Deployments": {viewDeployments},
		"Governance":  {viewGovernance},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a := newTestApp(tc.view, false)
			golden.RequireEqual(t, a.renderBreadcrumb())
		})
	}
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
