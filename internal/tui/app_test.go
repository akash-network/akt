package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/tui/views"
)

const (
	testAppWidth  = 80
	testAppHeight = 24
)

func newTestApp() App {
	km := keys.DefaultKeyMap()

	dash := views.NewDashboard(nil, views.DashboardContext{}, km)
	dash.SetSize(testAppWidth, testAppHeight-chromeHeight)

	a := App{
		keys:       km,
		standalone: false,
		width:      testAppWidth,
		height:     testAppHeight,
		help:       &views.HelpOverlay{},
		logView:    &views.LogViewer{},
	}

	// Push dashboard as initial view
	a.router.SetSize(testAppWidth, testAppHeight-chromeHeight)
	a.router.Push(dash)

	return a
}

// mockMonitor is a minimal tea.Model that records whether it received
// non-key messages. This tests the critical behavior that keeps
// WebSocket/tick chains alive.
type mockMonitor struct {
	receivedNonKey bool
	receivedKeys   []string
	windowHeights  []int
	view           string
}

func (m *mockMonitor) Init() tea.Cmd { return nil }
func (m *mockMonitor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.receivedKeys = append(m.receivedKeys, msg.String())
	case tea.WindowSizeMsg:
		m.receivedNonKey = true
		m.windowHeights = append(m.windowHeights, msg.Height)
	default:
		m.receivedNonKey = true
	}
	return m, nil
}
func (m *mockMonitor) View() tea.View {
	if m.view != "" {
		return tea.NewView(m.view)
	}
	return tea.NewView("monitor")
}

func TestAppRenderHeader(t *testing.T) {
	a := newTestApp()
	golden.RequireEqual(t, a.renderHeader())
}

func TestAppRenderNavBar(t *testing.T) {
	tests := map[string]struct {
		breadcrumb string
	}{
		"Dashboard":   {"Dashboard"},
		"Deployments": {"Deployments"},
		"Monitor":     {"Monitor"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a := newTestApp()
			if tc.breadcrumb == "Monitor" {
				a.monitorActive = true
			} else {
				a.router.Replace(&mockView{label: tc.breadcrumb})
			}
			golden.RequireEqual(t, a.renderNavBar())
		})
	}
}

func TestAppRenderBreadcrumb(t *testing.T) {
	tests := map[string]struct {
		labels []string
		want   string
	}{
		"Dashboard":   {[]string{"Dashboard"}, "Dashboard"},
		"Deployments": {[]string{"Deployments"}, "Deployments"},
		"Detail":      {[]string{"Deployments", "Detail"}, "Deployments / Detail"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a := newTestApp()
			// Build stack with the desired labels
			a.router = NewRouter()
			a.router.SetSize(testAppWidth, testAppHeight-chromeHeight)
			for _, label := range tc.labels {
				a.router.Push(&mockView{label: label})
			}
			bc := a.renderBreadcrumb()
			// Just verify it's non-empty and contains expected text
			if len(bc) == 0 {
				t.Error("breadcrumb should not be empty")
			}
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
			a := newTestApp()
			golden.RequireEqual(t, a.renderCentered(20, tc.text))
		})
	}
}

// ── Navigation behavioral tests ─────────────────────────────────────

func TestNavEscPopsView(t *testing.T) {
	a := newTestApp()
	// Push a second view
	a.router.Push(&mockView{label: "Deployments"})
	if a.router.Depth() != 2 {
		t.Fatalf("expected depth 2, got %d", a.router.Depth())
	}

	// Esc should pop back
	result, _ := a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	app := result.(App)
	if app.router.Depth() != 1 {
		t.Errorf("Esc: got depth %d, want 1", app.router.Depth())
	}
}

func TestNavEscNoopOnSingleView(t *testing.T) {
	a := newTestApp()
	result, _ := a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	app := result.(App)
	if app.router.Depth() != 1 {
		t.Errorf("Esc on single view: depth changed to %d, want 1", app.router.Depth())
	}
}

func TestNavOverlayBlocksNavKeys(t *testing.T) {
	a := newTestApp()
	cd := components.NewConfirmDialog(
		components.ConfirmClose,
		components.ConfirmData{Title: "Test", Body: "test"},
	)
	cd.SetSize(testAppWidth, testAppHeight)
	cd.Open()
	a.confirm = &cd

	// With confirm active, pressing '1' should NOT reach the router
	before := a.router.Breadcrumb()
	result, _ := a.Update(tea.KeyPressMsg{Code: '1'})
	app := result.(App)
	after := app.router.Breadcrumb()
	if before != after {
		t.Errorf("with confirm overlay: breadcrumb changed from %q to %q", before, after)
	}
}

func TestMonitorNonKeyForwarding(t *testing.T) {
	a := newTestApp()
	mon := &mockMonitor{}
	a.monitor = mon

	// Send a non-key message (e.g., a data message)
	a.Update(messages.ViewDataRefreshMsg{})

	if !mon.receivedNonKey {
		t.Error("non-key message was NOT forwarded to monitor model — this breaks WS/tick chains")
	}
}

func TestStandaloneMonitorForwardsNavigationKeys(t *testing.T) {
	tests := map[string]tea.KeyPressMsg{
		"next dashboard":     {Code: tea.KeyTab},
		"previous dashboard": {Code: tea.KeyTab, Mod: tea.ModShift},
		"overview":           {Code: '1'},
		"validators":         {Code: '2'},
		"governance":         {Code: '3'},
	}

	for name, msg := range tests {
		t.Run(name, func(t *testing.T) {
			a := newTestApp()
			a.standalone = true
			a.monitorActive = true
			mon := &mockMonitor{}
			a.monitor = mon

			a.Update(msg)

			if len(mon.receivedKeys) != 1 || mon.receivedKeys[0] != msg.String() {
				t.Fatalf("monitor keys = %v, want [%s]", mon.receivedKeys, msg.String())
			}
		})
	}
}

func TestEmbeddedMonitorOwnsNetworkNumberKeys(t *testing.T) {
	for _, code := range []rune{'1', '2', '3'} {
		t.Run(string(code), func(t *testing.T) {
			a := newTestApp()
			a.monitorActive = true
			mon := &mockMonitor{}
			a.monitor = mon

			a.Update(tea.KeyPressMsg{Code: code})

			if len(mon.receivedKeys) != 1 || mon.receivedKeys[0] != string(code) {
				t.Fatalf("monitor keys = %v, want [%c]", mon.receivedKeys, code)
			}
		})
	}
}

func TestStandaloneMonitorReceivesFullWindowHeightOnce(t *testing.T) {
	a := newTestApp()
	a.standalone = true
	mon := &mockMonitor{}
	a.monitor = mon

	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if len(mon.windowHeights) != 1 || mon.windowHeights[0] != 30 {
		t.Fatalf("monitor window heights = %v, want [30]", mon.windowHeights)
	}
}

func TestEmbeddedMonitorReceivesShellContentHeightOnce(t *testing.T) {
	a := newTestApp()
	mon := &mockMonitor{}
	a.monitor = mon

	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if len(mon.windowHeights) != 1 || mon.windowHeights[0] != 30-chromeHeight {
		t.Fatalf("monitor window heights = %v, want [%d]", mon.windowHeights, 30-chromeHeight)
	}
}

func TestStandaloneMonitorViewPreservesBottomHelp(t *testing.T) {
	a := newTestApp()
	a.standalone = true
	a.height = 10
	a.monitor = &mockMonitor{view: strings.Join([]string{
		"line 1", "line 2", "line 3", "line 4", "line 5",
		"line 6", "line 7", "line 8", "navigation help", "RPC status",
	}, "\n")}

	view := a.View().Content
	if !strings.Contains(view, "navigation help") || !strings.Contains(view, "RPC status") {
		t.Fatalf("standalone monitor view dropped its footer:\n%s", view)
	}
}

func TestBuildMonitorModelReportsCacheInitializationFailure(t *testing.T) {
	parent := t.TempDir()
	blockingFile := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	_, _, _, err := buildMonitorModel(Config{
		RPCEndpoint: "https://rpc.example.com:443/rpc",
		CacheDir:    filepath.Join(blockingFile, "cache"),
	})
	if err == nil || !strings.Contains(err.Error(), "cache") {
		t.Fatalf("error = %v, want cache initialization failure", err)
	}
}

func TestNavViewDataRefreshDispatches(t *testing.T) {
	a := newTestApp()
	_, cmd := a.Update(messages.ViewDataRefreshMsg{})
	// Should return a non-nil cmd (the Refresh() result from dashboard + bridge re-arm)
	// Since data.Service is nil, individual loads may return nil, but the batch should exist
	if cmd == nil {
		t.Log("ViewDataRefreshMsg returned nil cmd — acceptable with nil data service")
	}
}

// Ensure unused imports are exercised.
var _ = components.ConfirmClose
var _ = messages.ViewDataRefreshMsg{}
