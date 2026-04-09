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
		keys:       km,
		view:       view,
		standalone: standalone,
		width:      testAppWidth,
		height:     testAppHeight,
		query:      views.NewQueryView(),
		tx:         views.NewTxView(),
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
		"Dashboard":  {viewDashboard, false},
		"Query":      {viewQuery, false},
		"Tx":         {viewTx, false},
		"Monitor":    {viewMonitor, false},
		"Standalone": {viewDashboard, true},
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
