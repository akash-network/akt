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
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/views"
	"pkg.akt.dev/akt/internal/ui/theme"

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

// chromeHeight is the number of lines consumed by the shell chrome
// (header, nav bar + hrule, breadcrumb, footer hrule + hints).
// header=1, navBar=2 (tabs + hrule), breadcrumb=1, footer=2 (hrule + hints), newlines=4.
const chromeHeight = 10

// View implements tea.Model.
func (a App) View() tea.View {
	footer := a.renderFooter()

	// Height available for content between chrome.
	contentH := a.height - chromeHeight
	if contentH < 1 {
		contentH = 1
	}

	// Pin content to a fixed height so the footer is always at
	// the very bottom of the terminal, regardless of how much (or
	// how little) the active view renders.
	pin := lipgloss.NewStyle().Height(contentH).MaxHeight(contentH)

	// When the monitor view is active the embedded model renders its own
	// title/tab chrome. It already accounts for the reduced height
	// (terminal - statusBarHeight). Append the unified footer.
	if a.view == viewMonitor && a.monitorModel != nil && !a.palette.Active() {
		monView := a.monitorModel.View()
		content := pin.Render(monView.Content) + "\n" + footer
		v := tea.NewView(content)
		v.AltScreen = true
		return v
	}

	header := a.renderHeader()
	navBar := a.renderNavBar()
	breadcrumb := a.renderBreadcrumb()

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
		main = a.renderCentered(contentH, "No RPC endpoint configured.\nUse :consensus after setting up a context.")
	default:
		main = a.renderDashboard(contentH)
	}

	if a.palette.Active() {
		main = a.palette.View()
		footer = a.renderPaletteFooter()
	}

	chrome := header + "\n" + navBar + "\n" + breadcrumb + "\n" + main
	v := tea.NewView(pin.Render(chrome) + "\n" + footer)
	v.AltScreen = true
	return v
}

// ─── Footer (horizontal rule + hint pairs, always at bottom) ─────────

func (a App) renderFooter() string {
	var hints []components.HintPair

	switch a.view {
	case viewDashboard:
		hints = []components.HintPair{
			{Key: "1-6", Desc: "navigate"},
			{Key: ":", Desc: "command"},
			{Key: "?", Desc: "help"},
			{Key: "D", Desc: "deploy", Accent: true},
		}
	case viewDeployments:
		hints = []components.HintPair{
			{Key: "j/k", Desc: "move"},
			{Key: "↵", Desc: "open"},
			{Key: "l", Desc: "logs"},
			{Key: "d", Desc: "close"},
			{Key: "/", Desc: "search"},
			{Key: "D", Desc: "new", Accent: true},
		}
	case viewLeases:
		hints = []components.HintPair{
			{Key: "j/k", Desc: "move"},
			{Key: "↵", Desc: "detail"},
			{Key: "esc", Desc: "back"},
		}
	case viewProviders:
		hints = []components.HintPair{
			{Key: "j/k", Desc: "move"},
			{Key: "↵", Desc: "detail"},
			{Key: "esc", Desc: "back"},
		}
	case viewMonitor:
		hints = []components.HintPair{
			{Key: "j/k", Desc: "move"},
			{Key: "tab", Desc: "switch"},
			{Key: "esc", Desc: "back"},
		}
	case viewGovernance:
		hints = []components.HintPair{
			{Key: "j/k", Desc: "move"},
			{Key: "↵", Desc: "detail"},
			{Key: "v", Desc: "vote", Accent: true},
			{Key: "esc", Desc: "back"},
		}
	case viewStaking:
		hints = []components.HintPair{
			{Key: "j/k", Desc: "move"},
			{Key: "↵", Desc: "detail"},
			{Key: "d", Desc: "delegate", Accent: true},
			{Key: "esc", Desc: "back"},
		}
	}

	return components.Footer(a.width, hints)
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

func (a App) renderPaletteFooter() string {
	hints := []components.HintPair{
		{Key: "↑/↓", Desc: "navigate"},
		{Key: "↵", Desc: "select"},
		{Key: "esc", Desc: "close"},
	}
	return components.Footer(a.width, hints)
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
	// Main area for non-monitor views: total height minus chrome.
	mainH := a.height - chromeHeight
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
	appName := theme.HeaderAppName.Render("akt")
	sep := theme.HeaderMeta.Render(" · ")

	ctx := theme.HeaderContext.Render("prod") +
		theme.HeaderMeta.Render(":akashnet-2")

	acct := theme.HeaderContext.Render("alice") +
		theme.HeaderMeta.Render(" akash1abc…def")

	block := theme.HeaderMeta.Render("⎡ ") +
		theme.HeaderValue.Render("18,234,567") +
		theme.HeaderMeta.Render(" ⎤")

	sync := theme.SyncOK.Render("● synced")

	left := appName + sep + ctx + sep + acct
	right := block + "  " + sync

	innerW := a.width - 2 // account for HeaderStyle Padding(0,1)
	gap := innerW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return theme.HeaderStyle.Width(a.width).Render(
		left + strings.Repeat(" ", gap) + right,
	)
}

// navItems defines the primary navigation tabs.
var navItems = []struct {
	key  string
	name string
	view activeView
}{
	{"1", "Deployments", viewDeployments},
	{"2", "Leases", viewLeases},
	{"3", "Providers", viewProviders},
	{"4", "Monitor", viewMonitor},
	{"5", "Governance", viewGovernance},
	{"6", "Staking", viewStaking},
}

func (a App) renderNavBar() string {
	var parts []string
	for _, nav := range navItems {
		label := nav.key + " " + nav.name
		if a.view == nav.view {
			parts = append(parts, theme.NavTabActive.Render(label))
		} else {
			parts = append(parts, theme.NavTabInactive.Render(label))
		}
	}

	// When on Dashboard, show "Dashboard" as active highlight instead of any tab.
	if a.view == viewDashboard {
		parts = append([]string{theme.NavTabActive.Render("Dashboard")}, parts...)
	}

	deployBtn := lipgloss.NewStyle().Foreground(theme.AccentRed).Render("D deploy")

	bar := " " + strings.Join(parts, " ")
	gap := a.width - lipgloss.Width(bar) - lipgloss.Width(deployBtn) - 1
	if gap < 1 {
		gap = 1
	}
	bar = bar + strings.Repeat(" ", gap) + deployBtn

	return bar + "\n" + components.HRule(a.width)
}

func (a App) renderBreadcrumb() string {
	sep := theme.BreadcrumbSeparator.Render(" / ")

	var name string
	switch a.view {
	case viewDashboard:
		name = "Dashboard"
	case viewDeployments:
		name = "Deployments"
	case viewLeases:
		name = "Leases"
	case viewProviders:
		name = "Providers"
	case viewMonitor:
		name = "Monitor"
	case viewGovernance:
		name = "Governance"
	case viewStaking:
		name = "Staking"
	default:
		name = "Dashboard"
	}

	// For now, single-segment breadcrumb (active).
	// When detail views are added, non-active parent segments will precede.
	_ = sep // used when multi-segment breadcrumbs are added
	return " " + theme.BreadcrumbActive.Render(name)
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
