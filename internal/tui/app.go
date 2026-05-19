package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	cmthttp "github.com/cometbft/cometbft/rpc/client/http"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/viper"

	aktclient "pkg.akt.dev/akt/internal/client"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktevents "pkg.akt.dev/akt/internal/events"
	monitorcache "pkg.akt.dev/akt/internal/monitor/cache"
	monitorrpc "pkg.akt.dev/akt/internal/monitor/rpc"
	monitorui "pkg.akt.dev/akt/internal/monitor/ui"
	aktprovider "pkg.akt.dev/akt/internal/provider"
	"pkg.akt.dev/akt/internal/store"
	synce "pkg.akt.dev/akt/internal/sync"
	"pkg.akt.dev/akt/internal/tui/commands"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/tui/views"
	"pkg.akt.dev/akt/internal/ui/theme"

	aclient "pkg.akt.dev/go/node/client"
	aclientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	mtypes "pkg.akt.dev/go/node/market/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	rest "pkg.akt.dev/go/provider/client"
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
	viewDeploymentDetail
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

	// Data sources (optional — TUI works in degraded mode without them)
	Store       store.Store         // local deployment store (nil = no store)
	ResolvedCtx *aktctx.Context     // resolved akt context (nil = no context info)
	LightClient aclient.LightClient // chain query client; nil = no chain queries

	// Provider auth (optional — nil = no log streaming)
	Keyring   sdkkeyring.Keyring // keyring for provider auth
	ClientCtx sdkclient.Context  // SDK client context for provider auth
}

// App is the root bubbletea model for the akt TUI.
type App struct {
	keys       KeyMap
	view       activeView
	palette    views.CommandPalette
	standalone bool // disables command palette and view switching
	width      int
	height     int

	// Primary view components.
	dashboard        views.Dashboard
	deployments      views.DeploymentsView
	leases           views.LeasesView
	providers        views.ProvidersView
	governance       views.GovernanceView
	staking          views.StakingView
	detail           views.DetailView
	deploymentDetail views.DeploymentDetailView
	logViewer        views.LogViewer
	confirmDialog    components.ConfirmDialog
	helpOverlay      views.HelpOverlay
	toast            *components.Toast

	// monitorModel is the real-time monitor from internal/monitor/ui.
	// It is nil when no RPC endpoint is available.
	monitorModel tea.Model
	monitorReady bool // true after monitorModel.Init() cmds have been dispatched

	// Sync bridge — connects pubsub events to the sync engine.
	syncBridge *syncBridge

	// Data sources
	dataStore   store.Store
	resolvedCtx *aktctx.Context
	lightClient aclient.LightClient

	// Provider auth for log streaming
	keyring   sdkkeyring.Keyring
	clientCtx sdkclient.Context

	// Active log stream state
	logCtx    context.Context
	logCancel context.CancelFunc
	logStream *rest.ServiceLogs

	// Cached data
	storeStats *store.StoreStats
	syncState  *store.SyncState
}

// newApp returns a new App model. monitorModel may be nil when the
// monitor is not available (e.g. no RPC endpoint configured).
func newApp(cfg Config, topModel tea.Model) App {
	km := KeyMapFromConfig(cfg.Viper)
	reg := commands.DefaultRegistry()

	dash := views.NewDashboard()
	if cfg.ResolvedCtx != nil {
		dash.SetContext(cfg.ResolvedCtx.Name, cfg.ResolvedCtx.Network.ChainID, cfg.ResolvedCtx.DefaultAccount)
	}

	return App{
		keys:             km,
		view:             viewDashboard,
		standalone:       cfg.Standalone,
		dataStore:        cfg.Store,
		resolvedCtx:      cfg.ResolvedCtx,
		lightClient:      cfg.LightClient,
		keyring:          cfg.Keyring,
		clientCtx:        cfg.ClientCtx,
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
		monitorModel: topModel,
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	var cmds []tea.Cmd

	if a.monitorModel != nil {
		a.monitorReady = true
		cmds = append(cmds, a.monitorModel.Init())
	}

	// Arm the sync bridge so the TUI reacts to chain events.
	if a.syncBridge != nil {
		cmds = append(cmds, a.syncBridge.waitForEvent())
	}

	// Load initial data from the store.
	cmds = append(cmds, loadStoreStats(a.dataStore))
	cmds = append(cmds, loadSyncState(a.dataStore))

	// Load deployments for the dashboard.
	var owner string
	if a.resolvedCtx != nil {
		owner = a.resolvedCtx.DefaultAccount
	}
	cmds = append(cmds, loadDeployments(a.dataStore, owner))

	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var appCmds []tea.Cmd

	// ── App-level messages (always handled) ──────────────────────────

	switch msg := msg.(type) {
	case messages.DeploymentsLoadedMsg:
		if msg.Err == nil {
			a.deployments.SetData(msg.Deployments)
			// Filter active deployments for the dashboard summary.
			var active []*store.DeploymentRecord
			for _, d := range msg.Deployments {
				if d.State == "active" {
					active = append(active, d)
				}
			}
			a.dashboard.SetActiveDeployments(active)
		}
		return a, nil

	case messages.LeasesLoadedMsg:
		if msg.Err == nil {
			a.leases.SetData(msg.Leases)
			// Also populate the deployment detail view if it's active.
			if a.view == viewDeploymentDetail {
				a.deploymentDetail.SetLeases(msg.Leases)
			}
		}
		return a, nil

	case messages.BidsLoadedMsg:
		if msg.Err == nil {
			a.deploymentDetail.SetBids(msg.Bids)
		}
		return a, nil

	case messages.ProposalsLoadedMsg:
		if msg.Err == nil {
			a.governance.SetData(msg.Proposals)
		}
		return a, nil

	case messages.ValidatorsLoadedMsg:
		if msg.Err == nil {
			a.staking.SetData(msg.Validators)
		}
		return a, nil

	case messages.ProvidersLoadedMsg:
		if msg.Err == nil {
			a.providers.SetData(msg.Providers)
		}
		return a, nil

	case messages.StoreStatsMsg:
		if msg.Err == nil {
			a.storeStats = msg.Stats
			a.dashboard.SetStats(msg.Stats)
		}
		return a, nil

	case messages.SyncStateMsg:
		if msg.Err == nil {
			a.syncState = msg.State
			a.dashboard.SetSyncState(msg.State)
		}
		return a, nil

	case messages.ViewDataRefreshMsg:
		// The sync engine has persisted new data — re-read the store
		// for the current view and re-arm the bridge for the next event.
		var refreshCmds []tea.Cmd
		owner := ""
		if a.resolvedCtx != nil {
			owner = a.resolvedCtx.DefaultAccount
		}
		switch a.view {
		case viewDashboard:
			refreshCmds = append(refreshCmds,
				loadDeployments(a.dataStore, owner),
				loadStoreStats(a.dataStore),
				loadSyncState(a.dataStore),
			)
		case viewDeployments:
			refreshCmds = append(refreshCmds, loadDeployments(a.dataStore, owner))
		case viewLeases:
			refreshCmds = append(refreshCmds, loadLeases(a.dataStore, owner))
		case viewDeploymentDetail:
			rec := a.deploymentDetail.Deployment()
			if rec != nil {
				refreshCmds = append(refreshCmds,
					loadDeploymentLeases(a.dataStore, rec.Owner, rec.DSeq),
					loadBids(a.dataStore, rec.Owner, rec.DSeq),
				)
			}
		}
		if a.syncBridge != nil {
			refreshCmds = append(refreshCmds, a.syncBridge.waitForEvent())
		}
		return a, tea.Batch(refreshCmds...)

	case components.ConfirmMsg:
		// Action confirmed — for now just return to the previous view.
		// Actual transaction dispatch will be added in a future task.
		return a, nil

	case components.CancelMsg:
		// Dialog cancelled — no action needed, dialog already closed itself.
		return a, nil

	case messages.LogLineMsg:
		if a.logViewer.Active() {
			a.logViewer.AppendLine(views.LogLine{
				Scope:   msg.Name,
				Message: msg.Message,
			})
		}
		if a.logStream != nil && a.logCancel != nil {
			return a, streamLogs(a.logCtx, a.logStream)
		}
		return a, nil

	case messages.LogStreamClosedMsg:
		a.logCancel = nil
		a.logCtx = nil
		a.logStream = nil
		return a, nil

	case components.ToastExpiredMsg:
		a.toast = nil
		return a, nil

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

	// ── Overlays take priority for key messages when active ─────────

	// Confirm dialog intercepts all keys when active.
	if a.confirmDialog.Active() {
		if kmsg, ok := msg.(tea.KeyPressMsg); ok {
			if key.Matches(kmsg, a.keys.Quit) {
				return a, tea.Quit
			}
			cmd := a.confirmDialog.Update(msg)
			return a, tea.Batch(append(appCmds, cmd)...)
		}
		return a, tea.Batch(appCmds...)
	}

	// Log viewer intercepts keys when active.
	if a.logViewer.Active() {
		if kmsg, ok := msg.(tea.KeyPressMsg); ok {
			if key.Matches(kmsg, a.keys.Quit) {
				return a, tea.Quit
			}
			k := kmsg.String()
			switch k {
			case "esc":
				if a.logCancel != nil {
					a.logCancel()
					a.logCancel = nil
					a.logCtx = nil
					a.logStream = nil
				}
				a.logViewer.Close()
			case " ":
				a.logViewer.TogglePause()
			case "c":
				a.logViewer.Clear()
			case "k", "up":
				a.logViewer.ScrollUp()
			case "j", "down":
				a.logViewer.ScrollDown()
			case "G":
				a.logViewer.ScrollToBottom()
			}
			return a, tea.Batch(appCmds...)
		}
		return a, tea.Batch(appCmds...)
	}

	// Help overlay intercepts keys when active.
	if a.helpOverlay.Active() {
		if kmsg, ok := msg.(tea.KeyPressMsg); ok {
			if key.Matches(kmsg, a.keys.Quit) {
				return a, tea.Quit
			}
			if kmsg.String() == "esc" {
				a.helpOverlay.Close()
			}
			return a, tea.Batch(appCmds...)
		}
		return a, tea.Batch(appCmds...)
	}

	// Palette takes priority for key messages when active.
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

		// Help overlay.
		if key.Matches(kmsg, a.keys.Help) {
			a.helpOverlay.Open(a.viewName())
			return a, nil
		}

		// When the top view is active, forward keys to the real model.
		if a.view == viewMonitor && a.monitorModel != nil {
			var topCmd tea.Cmd
			a.monitorModel, topCmd = a.monitorModel.Update(msg)
			return a, topCmd
		}

		// Deployment detail view key handling.
		if a.view == viewDeploymentDetail {
			switch {
			case key.Matches(kmsg, a.keys.Back):
				a.view = viewDeployments
				return a, nil
			case key.Matches(kmsg, a.keys.TabNext):
				a.deploymentDetail.NextTab()
				return a, nil
			case key.Matches(kmsg, a.keys.CursorUp):
				a.deploymentDetail.ScrollUp()
				return a, nil
			case key.Matches(kmsg, a.keys.CursorDown):
				a.deploymentDetail.ScrollDown()
				return a, nil
			}
			// Number keys 1-4 for direct tab jump.
			k := kmsg.String()
			if len(k) == 1 && k[0] >= '1' && k[0] <= '4' {
				a.deploymentDetail.SetTab(int(k[0] - '1'))
				return a, nil
			}
			return a, nil
		}

		// Cursor navigation for list views.
		switch {
		case key.Matches(kmsg, a.keys.CursorUp):
			switch a.view {
			case viewDeployments:
				a.deployments.CursorUp()
			case viewLeases:
				a.leases.CursorUp()
			case viewProviders:
				a.providers.CursorUp()
			case viewGovernance:
				a.governance.CursorUp()
			case viewStaking:
				a.staking.CursorUp()
			}
			return a, nil
		case key.Matches(kmsg, a.keys.CursorDown):
			switch a.view {
			case viewDeployments:
				a.deployments.CursorDown()
			case viewLeases:
				a.leases.CursorDown()
			case viewProviders:
				a.providers.CursorDown()
			case viewGovernance:
				a.governance.CursorDown()
			case viewStaking:
				a.staking.CursorDown()
			}
			return a, nil
		}

		// Resolve owner for data loading.
		owner := ""
		if a.resolvedCtx != nil {
			owner = a.resolvedCtx.DefaultAccount
		}

		// View-specific actions on deployments list.
		if a.view == viewDeployments {
			switch {
			case key.Matches(kmsg, a.keys.Select):
				// Enter → open deployment detail.
				rec := a.deployments.SelectedRecord()
				if rec != nil {
					a.deploymentDetail.SetDeployment(rec)
					a.view = viewDeploymentDetail
					return a, tea.Batch(
						loadDeploymentLeases(a.dataStore, rec.Owner, rec.DSeq),
						loadBids(a.dataStore, rec.Owner, rec.DSeq),
					)
				}
				return a, nil
			case key.Matches(kmsg, a.keys.Logs):
				// l → open log viewer overlay with live log streaming.
				rec := a.deployments.SelectedRecord()
				if rec != nil {
					dseq := fmt.Sprintf("%d", rec.DSeq)
					a.logViewer.Open(rec.SDLPath, dseq, "")

					// Start log streaming if we have the required auth and store.
					if cmd := a.startLogStream(rec.Owner, rec.DSeq); cmd != nil {
						return a, cmd
					}
				}
				return a, nil
			case key.Matches(kmsg, a.keys.Close):
				// d → open confirm dialog for closing deployment.
				rec := a.deployments.SelectedRecord()
				if rec != nil {
					a.confirmDialog = components.NewConfirmDialog(
						components.ConfirmClose,
						components.ConfirmData{
							Title:  "Close Deployment",
							Body:   fmt.Sprintf("Close deployment %d? This action is irreversible.", rec.DSeq),
							Danger: true,
						},
					)
					a.confirmDialog.SetSize(a.width, a.height)
					a.confirmDialog.Open()
				}
				return a, nil
			}
		}

		// App-level key dispatch (non-top views).
		switch {
		case key.Matches(kmsg, a.keys.Deployments):
			a.view = viewDeployments
			return a, loadDeployments(a.dataStore, owner)
		case key.Matches(kmsg, a.keys.Leases):
			a.view = viewLeases
			return a, loadLeases(a.dataStore, owner)
		case key.Matches(kmsg, a.keys.Providers):
			a.view = viewProviders
			return a, loadChainProviders(a.lightClient)
		case key.Matches(kmsg, a.keys.Monitor):
			a.view = viewMonitor
			return a, nil
		case key.Matches(kmsg, a.keys.Governance):
			a.view = viewGovernance
			return a, loadProposals(a.lightClient)
		case key.Matches(kmsg, a.keys.Staking):
			a.view = viewStaking
			return a, loadValidators(a.lightClient)
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
	case viewDeploymentDetail:
		main = a.deploymentDetail.View()
	case viewMonitor:
		main = a.renderCentered(contentH, "No RPC endpoint configured.\nUse :consensus after setting up a context.")
	default:
		main = a.dashboard.View()
	}

	// Overlay: log viewer renders on top of content when active.
	if a.logViewer.Active() {
		main = a.logViewer.View()
	}

	// Overlay: confirm dialog renders on top of content when active.
	if a.confirmDialog.Active() {
		main = a.confirmDialog.View()
	}

	// Overlay: help overlay renders on top of content when active.
	if a.helpOverlay.Active() {
		main = a.helpOverlay.View()
	}

	if a.palette.Active() {
		main = a.palette.View()
		footer = a.renderPaletteFooter()
	}

	// Toast notification overlays the bottom of the content area.
	if a.toast != nil && !a.toast.Expired() {
		main = main + "\n" + a.toast.View()
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
	case viewDeploymentDetail:
		hints = []components.HintPair{
			{Key: "j/k", Desc: "scroll"},
			{Key: "1-4", Desc: "tabs"},
			{Key: "tab", Desc: "next tab"},
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
	owner := ""
	if a.resolvedCtx != nil {
		owner = a.resolvedCtx.DefaultAccount
	}

	switch strings.ToLower(cmd) {
	case "quit", "q", "exit":
		return a, tea.Quit

	case "dashboard", "home":
		a.view = viewDashboard
	case "monitor", "consensus", "top":
		a.view = viewMonitor
	case "deployments", "dep":
		a.view = viewDeployments
		return a, loadDeployments(a.dataStore, owner)
	case "leases":
		a.view = viewLeases
		return a, loadLeases(a.dataStore, owner)
	case "providers", "prov":
		a.view = viewProviders
		return a, loadChainProviders(a.lightClient)
	case "governance", "gov":
		a.view = viewGovernance
		return a, loadProposals(a.lightClient)
	case "staking", "validators", "val":
		a.view = viewStaking
		return a, loadValidators(a.lightClient)

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

	a.dashboard.SetSize(a.width, mainH)
	a.deployments.SetSize(a.width, mainH)
	a.leases.SetSize(a.width, mainH)
	a.providers.SetSize(a.width, mainH)
	a.governance.SetSize(a.width, mainH)
	a.staking.SetSize(a.width, mainH)
	a.detail.SetSize(a.width, mainH)
	a.deploymentDetail.SetSize(a.width, mainH)
	a.logViewer.SetSize(a.width, mainH)
	a.confirmDialog.SetSize(a.width, mainH)
	a.helpOverlay.SetSize(a.width, mainH)
	a.palette.SetSize(a.width, mainH)
}

func (a App) renderHeader() string {
	appName := theme.HeaderAppName.Render("akt")
	sep := theme.HeaderMeta.Render(" · ")

	// Context name and chain ID from resolved context.
	ctxName := "\u2014" // em dash fallback
	chainID := ""
	account := ""
	if a.resolvedCtx != nil {
		if a.resolvedCtx.Name != "" {
			ctxName = a.resolvedCtx.Name
		}
		chainID = a.resolvedCtx.Network.ChainID
		account = a.resolvedCtx.DefaultAccount
	}

	ctx := theme.HeaderContext.Render(ctxName)
	if chainID != "" {
		ctx += theme.HeaderMeta.Render(":" + chainID)
	}

	var left string
	if account != "" {
		acct := theme.HeaderContext.Render(account)
		left = appName + sep + ctx + sep + acct
	} else {
		left = appName + sep + ctx
	}

	// Block height from sync state.
	blockStr := "\u2014" // em dash fallback
	if a.syncState != nil && a.syncState.LastBlockHeight > 0 {
		blockStr = fmt.Sprintf("%d", a.syncState.LastBlockHeight)
	}
	block := theme.HeaderMeta.Render("\u23a1 ") +
		theme.HeaderValue.Render(blockStr) +
		theme.HeaderMeta.Render(" \u23a4")

	var sync string
	if a.syncState != nil {
		sync = theme.SyncOK.Render("\u25cf synced")
	} else {
		sync = theme.HeaderMeta.Render("\u25cb no sync")
	}

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
	case viewDeploymentDetail:
		parent := theme.BreadcrumbActive.Render("Deployments")
		detail := theme.BreadcrumbActive.Render("Detail")
		return " " + parent + sep + detail
	default:
		name = "Dashboard"
	}

	return " " + theme.BreadcrumbActive.Render(name)
}

// showToast creates a toast notification and returns the expiry tick command.
func (a *App) showToast(msg string, tone components.ToastTone) tea.Cmd {
	t := components.NewToast(msg, tone)
	a.toast = &t
	return tea.Tick(components.ToastDuration, func(time.Time) tea.Msg {
		return components.ToastExpiredMsg{}
	})
}

// viewName returns a human-readable name for the current view.
func (a App) viewName() string {
	switch a.view {
	case viewDashboard:
		return "Dashboard"
	case viewDeployments:
		return "Deployments"
	case viewLeases:
		return "Leases"
	case viewProviders:
		return "Providers"
	case viewMonitor:
		return "Monitor"
	case viewGovernance:
		return "Governance"
	case viewStaking:
		return "Staking"
	case viewDeploymentDetail:
		return "Deployment Detail"
	default:
		return ""
	}
}

func (a App) renderCentered(h int, text string) string {
	style := lipgloss.NewStyle().
		Width(a.width).
		Height(h).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(text)
}

// ─── Log streaming ───────────────────────────────────────────────────

// streamLogs returns a tea.Cmd that reads the next message from a log stream.
func streamLogs(ctx context.Context, logs *rest.ServiceLogs) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return messages.LogStreamClosedMsg{Reason: "cancelled"}
		case msg, ok := <-logs.Stream:
			if !ok {
				return messages.LogStreamClosedMsg{Reason: "stream ended"}
			}
			return messages.LogLineMsg{Name: msg.Name, Message: msg.Message}
		case reason := <-logs.OnClose:
			return messages.LogStreamClosedMsg{Reason: reason}
		}
	}
}

// startLogStream looks up the active lease for a deployment and starts
// streaming logs from the provider. Returns a tea.Cmd to arm the first
// read, or nil if streaming is not possible.
func (a *App) startLogStream(owner string, dseq uint64) tea.Cmd {
	if a.keyring == nil || a.dataStore == nil {
		return nil
	}

	leases, err := a.dataStore.ListLeases(context.Background(), store.LeaseFilter{
		Owner: owner,
		DSeq:  dseq,
		State: "active",
	})
	if err != nil || len(leases) == 0 {
		return nil
	}

	lease := leases[0]
	if lease.ProviderURI == "" {
		return nil
	}

	ownerAddr, err := sdk.AccAddressFromBech32(lease.ID.Owner)
	if err != nil {
		return nil
	}

	authType := ""
	if a.resolvedCtx != nil {
		authType = a.resolvedCtx.ProviderDefaults.AuthType
	}

	ctx, cancel := context.WithCancel(context.Background())

	cl, err := aktprovider.NewGatewayClient(
		ctx,
		a.clientCtx,
		ownerAddr,
		lease.ProviderURI,
		authType,
		a.keyring,
	)
	if err != nil {
		cancel()
		return nil
	}

	leaseID := mtypes.LeaseID{
		Owner:    lease.ID.Owner,
		DSeq:     lease.ID.DSeq,
		GSeq:     lease.ID.GSeq,
		OSeq:     lease.ID.OSeq,
		Provider: lease.ID.Provider,
	}

	logs, err := cl.LeaseLogs(ctx, leaseID, "", true, 100)
	if err != nil {
		cancel()
		return nil
	}

	a.logCtx = ctx
	a.logCancel = cancel
	a.logStream = logs

	return streamLogs(ctx, logs)
}

// ─── Data loading commands ───────────────────────────────────────────

func loadDeployments(s store.Store, owner string) tea.Cmd {
	return func() tea.Msg {
		if s == nil {
			return messages.DeploymentsLoadedMsg{Err: fmt.Errorf("no store available")}
		}
		depls, err := s.ListDeployments(context.Background(), store.DeploymentFilter{Owner: owner})
		return messages.DeploymentsLoadedMsg{Deployments: depls, Err: err}
	}
}

func loadLeases(s store.Store, owner string) tea.Cmd {
	return func() tea.Msg {
		if s == nil {
			return messages.LeasesLoadedMsg{Err: fmt.Errorf("no store available")}
		}
		leases, err := s.ListLeases(context.Background(), store.LeaseFilter{Owner: owner})
		return messages.LeasesLoadedMsg{Leases: leases, Err: err}
	}
}

func loadStoreStats(s store.Store) tea.Cmd {
	return func() tea.Msg {
		if s == nil {
			return messages.StoreStatsMsg{Err: fmt.Errorf("no store available")}
		}
		stats, err := s.Stats(context.Background())
		return messages.StoreStatsMsg{Stats: stats, Err: err}
	}
}

func loadDeploymentLeases(s store.Store, owner string, dseq uint64) tea.Cmd {
	return func() tea.Msg {
		if s == nil {
			return messages.LeasesLoadedMsg{Err: fmt.Errorf("no store available")}
		}
		leases, err := s.ListLeases(context.Background(), store.LeaseFilter{Owner: owner, DSeq: dseq})
		return messages.LeasesLoadedMsg{Leases: leases, Err: err}
	}
}

func loadBids(s store.Store, owner string, dseq uint64) tea.Cmd {
	return func() tea.Msg {
		if s == nil {
			return messages.BidsLoadedMsg{Err: fmt.Errorf("no store available")}
		}
		bids, err := s.ListBids(context.Background(), store.BidFilter{Owner: owner, DSeq: dseq})
		return messages.BidsLoadedMsg{DSeq: dseq, Bids: bids, Err: err}
	}
}

func loadSyncState(s store.Store) tea.Cmd {
	return func() tea.Msg {
		if s == nil {
			return messages.SyncStateMsg{Err: fmt.Errorf("no store available")}
		}
		state, err := s.GetSyncState(context.Background())
		return messages.SyncStateMsg{State: state, Err: err}
	}
}

// ─── Chain data loading commands ─────────────────────────────────────

func loadProposals(cl aclient.LightClient) tea.Cmd {
	return func() tea.Msg {
		if cl == nil {
			return messages.ProposalsLoadedMsg{Err: fmt.Errorf("no chain client available")}
		}
		res, err := cl.Query().Gov().Proposals(context.Background(), &govv1.QueryProposalsRequest{
			Pagination: &query.PageRequest{Limit: 100, Reverse: true},
		})
		if err != nil {
			return messages.ProposalsLoadedMsg{Err: err}
		}
		return messages.ProposalsLoadedMsg{Proposals: res.Proposals}
	}
}

func loadValidators(cl aclient.LightClient) tea.Cmd {
	return func() tea.Msg {
		if cl == nil {
			return messages.ValidatorsLoadedMsg{Err: fmt.Errorf("no chain client available")}
		}
		res, err := cl.Query().Staking().Validators(context.Background(), &stakingtypes.QueryValidatorsRequest{
			Pagination: &query.PageRequest{Limit: 200},
		})
		if err != nil {
			return messages.ValidatorsLoadedMsg{Err: err}
		}
		return messages.ValidatorsLoadedMsg{Validators: res.Validators}
	}
}

func loadChainProviders(cl aclient.LightClient) tea.Cmd {
	return func() tea.Msg {
		if cl == nil {
			return messages.ProvidersLoadedMsg{Err: fmt.Errorf("no chain client available")}
		}
		res, err := cl.Query().Provider().Providers(context.Background(), &ptypes.QueryProvidersRequest{
			Pagination: &query.PageRequest{Limit: 200},
		})
		if err != nil {
			return messages.ProvidersLoadedMsg{Err: err}
		}
		return messages.ProvidersLoadedMsg{Providers: res.Providers}
	}
}

// buildLightClient attempts to create a LightClient from the resolved context
// and RPC endpoint when one is not already provided. The TUI only performs
// queries so no keyring is needed.
func buildLightClient(cfg *Config) {
	if cfg.LightClient != nil || cfg.ResolvedCtx == nil || cfg.RPCEndpoint == "" {
		return
	}

	cctx := aktclient.BuildClientContext(cfg.ResolvedCtx, nil, aktcodec.MakeEncodingConfig())

	rpcClient, err := aclient.NewClient(context.Background(), cfg.RPCEndpoint)
	if err != nil {
		return
	}
	cctx = cctx.WithClient(rpcClient)

	cl, err := aclientv1beta3.NewLightClient(cctx)
	if err != nil {
		return
	}
	cfg.LightClient = cl
}

// ─── Entry points ────────────────────────────────────────────────────

func Run(cfg Config) error {
	buildLightClient(&cfg)

	model, bus, cleanup := buildMonitorModel(cfg)
	if cleanup != nil {
		defer cleanup()
	}

	app := newApp(cfg, model)

	if bus != nil && cfg.Store != nil && cfg.ResolvedCtx != nil {
		eng := synce.New(cfg.Store, []string{cfg.ResolvedCtx.DefaultAccount})
		if bridge, err := newSyncBridge(bus, eng); err == nil {
			app.syncBridge = bridge
			app.dashboard.SetSyncBridgeActive(true)
			defer bridge.close()
		}
	}

	p := tea.NewProgram(app)
	_, err := p.Run()
	return err
}

// RunMonitor launches the TUI in standalone monitor mode.
// cfg.InitialDashboard selects which dashboard is shown first
// ("network", "provider", "bme", or "" for the default network).
func RunMonitor(cfg Config) error {
	buildLightClient(&cfg)

	model, bus, cleanup := buildMonitorModel(cfg)
	if cleanup != nil {
		defer cleanup()
	}

	app := newApp(cfg, model)
	app.view = viewMonitor

	if bus != nil && cfg.Store != nil && cfg.ResolvedCtx != nil {
		eng := synce.New(cfg.Store, []string{cfg.ResolvedCtx.DefaultAccount})
		if bridge, err := newSyncBridge(bus, eng); err == nil {
			app.syncBridge = bridge
			app.dashboard.SetSyncBridgeActive(true)
			defer bridge.close()
		}
	}

	p := tea.NewProgram(app)
	_, err := p.Run()
	return err
}

func buildMonitorModel(cfg Config) (tea.Model, pubsub.Bus, func()) {
	if cfg.RPCEndpoint == "" || cfg.CacheDir == "" {
		return nil, nil, nil
	}

	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return nil, nil, nil
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
		return nil, nil, nil
	}

	provCache, err := monitorcache.Open(db)
	if err != nil {
		db.Close()
		return nil, nil, nil
	}

	monCache, err := monitorcache.OpenMonikerCache(db)
	if err != nil {
		db.Close()
		return nil, nil, nil
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

	return model, bus, cleanup
}
