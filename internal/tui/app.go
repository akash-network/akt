package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	cmthttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/spf13/viper"

	aktevents "pkg.akt.dev/akt/internal/events"
	monitorcache "pkg.akt.dev/akt/internal/monitor/cache"
	monitorrpc "pkg.akt.dev/akt/internal/monitor/rpc"
	monitorui "pkg.akt.dev/akt/internal/monitor/ui"
	"pkg.akt.dev/akt/internal/tui/commands"
	"pkg.akt.dev/akt/internal/tui/views"

	"pkg.akt.dev/go/util/pubsub"
)

// activeView tracks which panel is displayed in the main area.
type activeView int

const (
	viewDashboard activeView = iota
	viewDeployments
	viewLeases
	viewProviders
	viewMonitor
	viewGovernance
	viewStaking
)

// statusBarHeight is the number of lines reserved for the bottom status bar.
const statusBarHeight = 3

// Config holds the parameters needed to start the TUI.
type Config struct {
	Viper            *viper.Viper
	RPCEndpoint      string // resolved from active context; may be empty
	RESTEndpoint     string // resolved from active context; may be empty
	CacheDir         string // path to cache directory (e.g. ~/.config/akt/cache)
	Insecure         bool   // skip TLS verification for provider queries
	Standalone       bool   // when true, disables command palette and view switching (e.g. akt monitor)
	InitialDashboard string // which monitor dashboard to start on: "network" (default), "provider", "bme"
}

// App is the root bubbletea model for the akt TUI.
type App struct {
	keys       KeyMap
	view       activeView
	palette    views.CommandPalette
	standalone bool // disables command palette and view switching
	width      int
	height     int

	// Primary view components — reusable ListView for each resource type.
	deployments views.ListView
	leases      views.ListView
	providers   views.ListView
	governance  views.ListView
	staking     views.ListView
	detail      views.DetailView

	// monitorModel is the real-time monitor from internal/monitor/ui.
	// It is nil when no RPC endpoint is available.
	monitorModel tea.Model
	monitorReady bool // true after monitorModel.Init() cmds have been dispatched
}

// newApp returns a new App model. monitorModel may be nil when the
// monitor is not available (e.g. no RPC endpoint configured).
func newApp(cfg Config, topModel tea.Model) App {
	km := KeyMapFromConfig(cfg.Viper)
	reg := commands.DefaultRegistry()

	return App{
		keys:       km,
		view:       viewDashboard,
		standalone: cfg.Standalone,
		deployments: views.NewListView(views.ListViewConfig{
			Title:   "Deployments",
			Columns: []views.ListColumn{{Header: "ID"}, {Header: "STATE", Width: 12}, {Header: "GROUPS", Width: 8}, {Header: "CREATED AT", Width: 14}},
			Empty:   "No deployments. Use 'akt deploy <sdl>' to create one.",
		}),
		leases: views.NewListView(views.ListViewConfig{
			Title:   "Leases",
			Columns: []views.ListColumn{{Header: "ID"}, {Header: "PRICE/BLOCK", Width: 16}, {Header: "STATE", Width: 12}},
			Empty:   "No active leases.",
		}),
		providers: views.NewListView(views.ListViewConfig{
			Title:   "Providers",
			Columns: []views.ListColumn{{Header: "OWNER"}, {Header: "HOST URI"}, {Header: "EMAIL", Width: 20}},
			Empty:   "No providers found.",
		}),
		governance: views.NewListView(views.ListViewConfig{
			Title:   "Governance Proposals",
			Columns: []views.ListColumn{{Header: "ID", Width: 6}, {Header: "TITLE"}, {Header: "STATUS", Width: 16}, {Header: "VOTING END", Width: 18}},
			Empty:   "No proposals.",
		}),
		staking: views.NewListView(views.ListViewConfig{
			Title:   "Validators",
			Columns: []views.ListColumn{{Header: "MONIKER"}, {Header: "STATUS", Width: 12}, {Header: "VOTING POWER", Width: 16}, {Header: "COMMISSION", Width: 12}},
			Empty:   "No validators found.",
		}),
		detail: views.NewDetailView(),
		palette: views.NewCommandPalette(reg, views.PaletteKeys{
			CursorUp:   km.CursorUp,
			CursorDown: km.CursorDown,
			Select:     km.Select,
			Close:      km.Back,
		}),
		monitorModel: topModel,
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	if a.monitorModel != nil {
		a.monitorReady = true
		return a.monitorModel.Init()
	}
	return nil
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var appCmds []tea.Cmd

	// ── App-level messages (always handled) ──────────────────────────

	switch msg := msg.(type) {
	case views.CommandSubmitMsg:
		return a.handleCommand(msg.Value)

	case monitorui.BackMsg:
		if a.standalone {
			return a, tea.Quit
		}
		a.view = viewDashboard
		return a, nil

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resize()
		// Forward a reduced-height WindowSizeMsg to the top model so it
		// leaves room for the TUI's 3-line status bar.
		if a.monitorModel != nil {
			adjusted := tea.WindowSizeMsg{
				Width:  msg.Width,
				Height: msg.Height - statusBarHeight,
			}
			var topCmd tea.Cmd
			a.monitorModel, topCmd = a.monitorModel.Update(adjusted)
			appCmds = append(appCmds, topCmd)
		}
		return a, tea.Batch(appCmds...)
	}

	// ── Non-key messages: ALWAYS forward to top model first ─────────
	// The top model's background goroutines (WebSocket, ticks, provider
	// checks) produce internal messages that must be processed regardless
	// of which view is active or whether the palette is open.  If these
	// messages are swallowed, the tick/WS chains break permanently.

	if _, isKey := msg.(tea.KeyPressMsg); !isKey && a.monitorModel != nil {
		var topCmd tea.Cmd
		a.monitorModel, topCmd = a.monitorModel.Update(msg)
		appCmds = append(appCmds, topCmd)
	}

	// ── Standalone mode: all keys go directly to the sub-model ──────
	// Commands like "akt monitor" run a single view without the command
	// palette or view switching. Only Ctrl+C is intercepted.

	if a.standalone {
		if kmsg, ok := msg.(tea.KeyPressMsg); ok {
			if key.Matches(kmsg, a.keys.Quit) {
				return a, tea.Quit
			}
			if a.monitorModel != nil {
				var topCmd tea.Cmd
				a.monitorModel, topCmd = a.monitorModel.Update(msg)
				return a, topCmd
			}
		}
		return a, tea.Batch(appCmds...)
	}

	// ── Palette takes priority for key messages when active ──────────

	if a.palette.Active() {
		if kmsg, ok := msg.(tea.KeyPressMsg); ok {
			if key.Matches(kmsg, a.keys.Quit) {
				return a, tea.Quit
			}
			cmd := a.palette.Update(msg)
			return a, tea.Batch(append(appCmds, cmd)...)
		}
		return a, tea.Batch(appCmds...)
	}

	// ── Key handling ─────────────────────────────────────────────────

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		// Ctrl+C always quits regardless of active view.
		if key.Matches(kmsg, a.keys.Quit) {
			return a, tea.Quit
		}

		// Command palette can always be opened.
		if key.Matches(kmsg, a.keys.Command) || key.Matches(kmsg, a.keys.CommandSearch) {
			a.palette.Open()
			return a, nil
		}

		// When the top view is active, forward keys to the real model.
		if a.view == viewMonitor && a.monitorModel != nil {
			var topCmd tea.Cmd
			a.monitorModel, topCmd = a.monitorModel.Update(msg)
			return a, topCmd
		}

		// App-level key dispatch (non-top views).
		switch {
		case key.Matches(kmsg, a.keys.Deployments):
			a.view = viewDeployments
			return a, nil
		case key.Matches(kmsg, a.keys.Leases):
			a.view = viewLeases
			return a, nil
		case key.Matches(kmsg, a.keys.Providers):
			a.view = viewProviders
			return a, nil
		case key.Matches(kmsg, a.keys.Monitor):
			a.view = viewMonitor
			return a, nil
		case key.Matches(kmsg, a.keys.Governance):
			a.view = viewGovernance
			return a, nil
		case key.Matches(kmsg, a.keys.Staking):
			a.view = viewStaking
			return a, nil
		case key.Matches(kmsg, a.keys.Back):
			a.view = viewDashboard
			return a, nil
		}

		return a, nil
	}

	return a, tea.Batch(appCmds...)
}

// View implements tea.Model.
func (a App) View() tea.View {
	status := a.renderStatusBar()

	// Height available for content above the status bar.
	contentH := a.height - statusBarHeight
	if contentH < 1 {
		contentH = 1
	}

	// Pin content to a fixed height so the status bar is always at
	// the very bottom of the terminal, regardless of how much (or
	// how little) the active view renders.
	pin := lipgloss.NewStyle().Height(contentH).MaxHeight(contentH)

	// When the top view is active the embedded model renders its own
	// title/tab chrome. It already accounts for the reduced height
	// (terminal - statusBarHeight). Append the unified status bar.
	if a.view == viewMonitor && a.monitorModel != nil && !a.palette.Active() {
		monView := a.monitorModel.View()
		content := pin.Render(monView.Content) + "\n" + status
		v := tea.NewView(content)
		v.AltScreen = true
		return v
	}

	header := a.renderHeader()

	// Main area height = content minus header (1) and its newline (1).
	mainH := contentH - 2
	if mainH < 1 {
		mainH = 1
	}

	var main string
	switch a.view {
	case viewDeployments:
		main = a.deployments.View()
	case viewLeases:
		main = a.leases.View()
	case viewProviders:
		main = a.providers.View()
	case viewGovernance:
		main = a.governance.View()
	case viewStaking:
		main = a.staking.View()
	case viewMonitor:
		main = a.renderCentered(mainH, "No RPC endpoint configured.\nUse :consensus after setting up a context.")
	default:
		main = a.renderDashboard(mainH)
	}

	if a.palette.Active() {
		main = a.palette.View()
		status = a.renderPaletteStatusBar()
	}

	v := tea.NewView(pin.Render(header+"\n"+main) + "\n" + status)
	v.AltScreen = true
	return v
}

// ─── Status bar (3 lines, always at bottom) ──────────────────────────

func (a App) renderStatusBar() string {
	line1Style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(a.width)

	line2Style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Width(a.width)

	line3Style := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Width(a.width).
		Padding(0, 1)

	var line1, line2 string

	switch a.view {
	case viewMonitor:
		line1, line2 = a.monitorStatusLines()
	case viewDeployments:
		line1 = "Deployments"
		line2 = "enter: detail  j/k: navigate"
	case viewLeases:
		line1 = "Leases"
		line2 = "enter: detail  j/k: navigate"
	case viewProviders:
		line1 = "Providers"
		line2 = "enter: detail  j/k: navigate"
	case viewGovernance:
		line1 = "Governance"
		line2 = "enter: detail  j/k: navigate"
	case viewStaking:
		line1 = "Staking"
		line2 = "enter: detail  j/k: navigate"
	default:
		line1 = "1: deployments  2: leases  3: providers  " +
			a.keys.Monitor.Help().Key + ": monitor  5: governance  6: staking"
		line2 = ""
	}

	// Line 3: global keybindings (omit palette hint in standalone mode).
	var line3 string
	if a.standalone {
		line3 = "q: quit  " + a.keys.Quit.Help().Key + ": quit"
	} else {
		line3 = a.keys.Command.Help().Key + ": command  " +
			a.keys.Back.Help().Key + ": back  " +
			a.keys.Quit.Help().Key + ": quit"
	}

	return line1Style.Render(line1) + "\n" +
		line2Style.Render(line2) + "\n" +
		line3Style.Render(line3)
}

// monitorStatusLines returns lines 1 and 2 for the status bar when the
// monitor is active.
func (a App) monitorStatusLines() (string, string) {
	if a.monitorModel == nil {
		return "", ""
	}

	// Type-assert to get StatusInfo from the embedded top model.
	type statusProvider interface {
		StatusInfo() monitorui.StatusInfo
	}

	sp, ok := a.monitorModel.(statusProvider)
	if !ok {
		return "", ""
	}

	si := sp.StatusInfo()

	// Line 1: tab-specific keybindings.
	line1 := si.TabHelpText()

	// Line 2: RPC endpoint + connection mode.
	mode := "HTTP"
	if si.WSConnected {
		mode = "WS"
	}
	line2 := fmt.Sprintf("RPC: %s [%s]", si.Endpoint, mode)

	return line1, line2
}

func (a App) renderPaletteStatusBar() string {
	line1Style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(a.width)

	line2Style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Width(a.width)

	line3Style := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Width(a.width).
		Padding(0, 1)

	line1 := a.keys.CursorDown.Help().Key + "/" + a.keys.CursorUp.Help().Key + ": navigate  " +
		a.keys.Select.Help().Key + ": select"
	line3 := a.keys.Back.Help().Key + ": close"

	return line1Style.Render(line1) + "\n" +
		line2Style.Render("") + "\n" +
		line3Style.Render(line3)
}

// ─── Helpers ─────────────────────────────────────────────────────────

func (a App) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	switch strings.ToLower(cmd) {
	case "quit", "q", "exit":
		return a, tea.Quit

	case "dashboard", "home":
		a.view = viewDashboard
	case "monitor", "consensus", "top":
		a.view = viewMonitor
	case "deployments", "dep":
		a.view = viewDeployments
	case "leases":
		a.view = viewLeases
	case "providers", "prov":
		a.view = viewProviders
	case "governance", "gov":
		a.view = viewGovernance
	case "staking", "validators", "val":
		a.view = viewStaking

	case "certificates", "escrow", "orders", "bids",
		"deploy", "help":
		a.view = viewDashboard
	}

	return a, nil
}

func (a *App) resize() {
	// Main area for non-top views: total - header (1) - status (3) - newlines (2).
	mainH := a.height - statusBarHeight - 3
	if mainH < 1 {
		mainH = 1
	}

	a.deployments.SetSize(a.width, mainH)
	a.leases.SetSize(a.width, mainH)
	a.providers.SetSize(a.width, mainH)
	a.governance.SetSize(a.width, mainH)
	a.staking.SetSize(a.width, mainH)
	a.detail.SetSize(a.width, mainH)
	a.palette.SetSize(a.width, mainH)
}

func (a App) renderHeader() string {
	style := lipgloss.NewStyle().
		Bold(true).
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Width(a.width).
		Padding(0, 1)

	return style.Render("akt")
}

func (a App) renderDashboard(h int) string {
	return a.renderCentered(h, "akt - Akash Network")
}

func (a App) renderCentered(h int, text string) string {
	style := lipgloss.NewStyle().
		Width(a.width).
		Height(h).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(text)
}

// ─── Entry points ────────────────────────────────────────────────────

func Run(cfg Config) error {
	model, cleanup := buildMonitorModel(cfg)
	if cleanup != nil {
		defer cleanup()
	}

	app := newApp(cfg, model)
	p := tea.NewProgram(app)
	_, err := p.Run()
	return err
}

// RunMonitor launches the TUI in standalone monitor mode.
// cfg.InitialDashboard selects which dashboard is shown first
// ("network", "provider", "bme", or "" for the default network).
func RunMonitor(cfg Config) error {
	model, cleanup := buildMonitorModel(cfg)
	if cleanup != nil {
		defer cleanup()
	}

	app := newApp(cfg, model)
	app.view = viewMonitor
	p := tea.NewProgram(app)
	_, err := p.Run()
	return err
}

func buildMonitorModel(cfg Config) (tea.Model, func()) {
	if cfg.RPCEndpoint == "" || cfg.CacheDir == "" {
		return nil, nil
	}

	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return nil, nil
	}

	// Prefer monitor.db; fall back to legacy top.db for migration.
	dbPath := filepath.Join(cfg.CacheDir, "monitor.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		legacyPath := filepath.Join(cfg.CacheDir, "top.db")
		if _, legacyErr := os.Stat(legacyPath); legacyErr == nil {
			_ = os.Rename(legacyPath, dbPath)
		}
	}

	db, err := monitorcache.OpenDB(dbPath)
	if err != nil {
		return nil, nil
	}

	provCache, err := monitorcache.Open(db)
	if err != nil {
		db.Close()
		return nil, nil
	}

	monCache, err := monitorcache.OpenMonikerCache(db)
	if err != nil {
		db.Close()
		return nil, nil
	}

	// ── Shared event bus ────────────────────────────────────────────
	// The bus carries all typed ABCI events from the chain. Individual
	// consumers (dashboards, sync engine, etc.) subscribe and filter
	// by type.
	bus := pubsub.NewBus()

	var evtSvc aktevents.Service
	var cometClient *cmthttp.HTTP

	cometClient, err = cmthttp.New(cfg.RPCEndpoint, "/websocket")
	if err == nil {
		if startErr := cometClient.Start(); startErr == nil {
			evtSvc, _ = aktevents.NewService(
				context.Background(),
				cometClient,
				"akt-monitor",
				bus,
			)
		}
	}

	model := monitorui.NewModel(monitorui.ModelConfig{
		Client:             monitorrpc.NewClient(cfg.RPCEndpoint, cfg.RESTEndpoint),
		RPCClient:          monitorrpc.NewRPCProviderClient(cfg.RPCEndpoint),
		Cache:              provCache,
		MonikerCache:       monCache,
		InsecureSkipVerify: cfg.Insecure,
		Embedded:           true, // always true; parent App provides the status bar
		InitialDashboard:   cfg.InitialDashboard,
		Bus:                bus,
	})

	cleanup := func() {
		if evtSvc != nil {
			evtSvc.Shutdown()
		}
		if cometClient != nil && cometClient.IsRunning() {
			_ = cometClient.Stop()
		}
		bus.Close()
		db.Close()
	}

	return model, cleanup
}
