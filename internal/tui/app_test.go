package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/spf13/viper"

	"pkg.akt.dev/akt/internal/tui/commands"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/messages"
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
		keys:             km,
		view:             view,
		standalone:       standalone,
		width:            testAppWidth,
		height:           testAppHeight,
		dashboard:        dash,
		deployments:      views.NewDeploymentsView(),
		leases:           views.NewLeasesView(),
		providers:        views.NewProvidersView(),
		governance:       views.NewGovernanceView(),
		staking:          views.NewStakingView(),
		detail:           views.NewDetailView(),
		deploymentDetail: views.NewDeploymentDetailView(),
		logViewer:        views.NewLogViewer(),
		helpOverlay:      views.NewHelpOverlay(),
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
		"Dashboard":        {viewDashboard},
		"Deployments":      {viewDeployments},
		"Leases":           {viewLeases},
		"Providers":        {viewProviders},
		"Monitor":          {viewMonitor},
		"Governance":       {viewGovernance},
		"Staking":          {viewStaking},
		"DeploymentDetail": {viewDeploymentDetail},
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
		"Dashboard":        {viewDashboard},
		"Deployments":      {viewDeployments},
		"Governance":       {viewGovernance},
		"DeploymentDetail": {viewDeploymentDetail},
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

// ── Navigation behavioral tests (T064) ─────────────────────────────

func TestNavNumberKeys(t *testing.T) {
	tests := []struct {
		key  rune
		want activeView
	}{
		{'1', viewDeployments},
		{'2', viewLeases},
		{'3', viewProviders},
		{'4', viewMonitor},
		{'5', viewGovernance},
		{'6', viewStaking},
	}

	for _, tt := range tests {
		a := newTestApp(viewDashboard, false)
		result, _ := a.Update(tea.KeyPressMsg{Code: tt.key})
		app := result.(App)
		if app.view != tt.want {
			t.Errorf("key %q: got view %d, want %d", tt.key, app.view, tt.want)
		}
	}
}

func TestNavEscBackToDashboard(t *testing.T) {
	for _, v := range []activeView{viewDeployments, viewLeases, viewProviders, viewGovernance, viewStaking} {
		a := newTestApp(v, false)
		result, _ := a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		app := result.(App)
		if app.view != viewDashboard {
			t.Errorf("from view %d: Esc got view %d, want viewDashboard", v, app.view)
		}
	}
}

func TestNavEscFromDetailToDeployments(t *testing.T) {
	a := newTestApp(viewDeploymentDetail, false)
	result, _ := a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	app := result.(App)
	if app.view != viewDeployments {
		t.Errorf("from detail: Esc got view %d, want viewDeployments", app.view)
	}
}

func TestNavCommandPaletteRouting(t *testing.T) {
	tests := []struct {
		cmd  string
		want activeView
	}{
		{"deployments", viewDeployments},
		{"leases", viewLeases},
		{"providers", viewProviders},
		{"monitor", viewMonitor},
		{"governance", viewGovernance},
		{"staking", viewStaking},
	}

	for _, tt := range tests {
		a := newTestApp(viewDashboard, false)
		result, _ := a.Update(views.CommandSubmitMsg{Value: tt.cmd})
		app := result.(App)
		if app.view != tt.want {
			t.Errorf("command %q: got view %d, want %d", tt.cmd, app.view, tt.want)
		}
	}
}

func TestNavOverlayBlocksNavKeys(t *testing.T) {
	a := newTestApp(viewDeployments, false)
	a.confirmDialog = components.NewConfirmDialog(
		components.ConfirmClose,
		components.ConfirmData{Title: "Test", Body: "test"},
	)
	a.confirmDialog.SetSize(testAppWidth, testAppHeight)
	a.confirmDialog.Open()

	// Pressing '1' should NOT change view because confirm overlay intercepts.
	result, _ := a.Update(tea.KeyPressMsg{Code: '1'})
	app := result.(App)
	if app.view != viewDeployments {
		t.Errorf("with confirm overlay: key '1' changed view to %d, want viewDeployments", app.view)
	}
}

func TestNavViewDataRefreshDispatches(t *testing.T) {
	// ViewDataRefreshMsg from views with store data should return a non-nil cmd.
	for _, v := range []activeView{viewDashboard, viewDeployments, viewLeases} {
		a := newTestApp(v, false)
		_, cmd := a.Update(messages.ViewDataRefreshMsg{})
		if cmd == nil {
			t.Errorf("view %d: ViewDataRefreshMsg returned nil cmd, want data reload", v)
		}
	}
}

// Ensure unused imports are exercised.
var _ = components.ConfirmClose
var _ = messages.ViewDataRefreshMsg{}
