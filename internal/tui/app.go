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
	"pkg.akt.dev/akt/internal/tui/data"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/tui/views"
	"pkg.akt.dev/akt/internal/ui/theme"

	aclient "pkg.akt.dev/go/node/client"
	aclientv1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	mtypes "pkg.akt.dev/go/node/market/v1"
	rest "pkg.akt.dev/go/provider/client"
	"pkg.akt.dev/go/util/pubsub"
)

// chromeHeight is the number of lines consumed by the shell chrome
// (header, nav bar + hrule, breadcrumb, plus newlines between them).
// header=1, "\n"=1, navBar=2 (tabs + hrule), "\n"=1, breadcrumb=1, "\n"=1.
// No separate footer — hints live in the nav bar per design.
const chromeHeight = 7

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
	keys       keys.KeyMap
	router     Router
	palette    *views.Palette // will be nil until palette.go is created
	confirm    *components.ConfirmDialog
	help       *views.HelpOverlay
	logView    *views.LogViewer
	toast      *components.Toast
	standalone bool

	// Monitor model (nil if no RPC). Non-key messages forwarded to it
	// regardless of active view to keep WS/tick chains alive.
	// The monitor is NOT placed on the router stack — it's a special case
	// because its tick/WS chains must always run via a.monitor, and placing
	// it on the stack would create a duplicate that forks those chains.
	monitor       tea.Model
	monitorActive bool // true when the monitor view is displayed

	// Sync bridge
	bridge *syncBridge

	// Data service — injected into views for data loading.
	data data.Service

	// Chrome state
	resolvedCtx *aktctx.Context
	syncState   *store.SyncState
	storeStats  *store.StoreStats

	// Log stream lifecycle (requires keyring/provider auth)
	logCtx    context.Context
	logCancel context.CancelFunc
	logStream *rest.ServiceLogs
	keyring   sdkkeyring.Keyring
	clientCtx sdkclient.Context
	dataStore store.Store

	width, height int
}

// newApp returns a new App model. monitorModel may be nil when the
// monitor is not available (e.g. no RPC endpoint configured).
func newApp(cfg Config, monitorModel tea.Model) App {
	km := keys.KeyMapFromConfig(cfg.Viper)
	svc := data.NewLoader(cfg.Store, cfg.LightClient)

	helpOverlay := views.NewHelpOverlay()
	logViewer := views.NewLogViewer()
	reg := commands.DefaultRegistry()
	pal := views.NewCommandPalette(reg, views.PaletteKeys{
		CursorUp:   km.CursorUp,
		CursorDown: km.CursorDown,
		Select:     km.Select,
		Close:      km.Back,
	})

	a := App{
		keys:        km,
		standalone:  cfg.Standalone,
		monitor:     monitorModel,
		data:        svc,
		resolvedCtx: cfg.ResolvedCtx,
		dataStore:   cfg.Store,
		keyring:     cfg.Keyring,
		clientCtx:   cfg.ClientCtx,
		help:        &helpOverlay,
		logView:     &logViewer,
		palette:     &pal,
	}

	// Build dashboard context from config.
	dctx := views.DashboardContext{}
	if cfg.ResolvedCtx != nil {
		dctx.ContextName = cfg.ResolvedCtx.Name
		dctx.ChainID = cfg.ResolvedCtx.Network.ChainID
		dctx.Account = cfg.ResolvedCtx.DefaultAccount
		if len(cfg.ResolvedCtx.Network.Endpoints.RPC) > 0 {
			dctx.RPCEndpoint = cfg.ResolvedCtx.Network.Endpoints.RPC[0]
		}
	}
	dctx.Version = "dev"

	// Push initial dashboard view onto router.
	dash := views.NewDashboard(svc, dctx, km)
	a.router.Push(dash)

	return a
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	var cmds []tea.Cmd

	if a.monitor != nil {
		cmds = append(cmds, a.monitor.Init())
	}

	if a.bridge != nil {
		cmds = append(cmds, a.bridge.waitForEvent())
	}

	// Initial data loads
	cmds = append(cmds, a.data.LoadStoreStats())
	cmds = append(cmds, a.data.LoadSyncState())

	var owner string
	if a.resolvedCtx != nil {
		owner = a.resolvedCtx.DefaultAccount
	}
	cmds = append(cmds, a.data.LoadDeployments(owner))
	cmds = append(cmds, a.data.LoadBalance(owner))

	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Non-key messages ALWAYS forwarded to monitor to keep WS/tick chains alive.
	// This must happen BEFORE the type switch because many cases return early.
	_, isKey := msg.(tea.KeyPressMsg)
	_, isWindowSize := msg.(tea.WindowSizeMsg)
	if !isKey && !isWindowSize && a.monitor != nil {
		var topCmd tea.Cmd
		a.monitor, topCmd = a.monitor.Update(msg)
		cmds = append(cmds, topCmd)
	}

	switch msg := msg.(type) {
	// ── 1. Window resize ────────────────────────────────────────────
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		mainH := a.height - chromeHeight
		if mainH < 1 {
			mainH = 1
		}
		a.router.SetSize(a.width, mainH)
		if a.help != nil {
			a.help.SetSize(a.width, mainH)
		}
		if a.logView != nil {
			a.logView.SetSize(a.width, mainH)
		}
		// Forward to monitor
		if a.monitor != nil {
			monitorHeight := msg.Height - chromeHeight
			if a.standalone {
				monitorHeight = msg.Height
			}
			if monitorHeight < 1 {
				monitorHeight = 1
			}
			adjusted := tea.WindowSizeMsg{
				Width:  msg.Width,
				Height: monitorHeight,
			}
			a.monitor, _ = a.monitor.Update(adjusted)
		}
		return a, nil

	// ── 2. Navigation messages from views ───────────────────────────
	case messages.PushViewMsg:
		if vc, ok := msg.View.(views.ViewComponent); ok {
			cmd := a.router.Push(vc)
			return a, cmd
		}
		return a, nil

	case messages.PopViewMsg:
		cmd := a.router.Pop()
		return a, cmd

	// ── 3. Overlay messages ─────────────────────────────────────────
	case messages.ShowConfirmMsg:
		cd := components.NewConfirmDialog(msg.Kind, msg.Data)
		cd.SetSize(a.width, a.height)
		cd.Open()
		a.confirm = &cd
		return a, nil

	case messages.ShowToastMsg:
		cmd := a.showToast(msg.Message, components.ToastTone(msg.Tone))
		return a, cmd

	case messages.StartLogStreamMsg:
		if a.logView != nil {
			dseq := fmt.Sprintf("%d", msg.DSeq)
			a.logView.Open("deployment", dseq, "")
			if cmd := a.startLogStream(msg.Owner, msg.DSeq); cmd != nil {
				return a, cmd
			}
		}
		return a, nil

	case messages.StopLogStreamMsg:
		if a.logCancel != nil {
			a.logCancel()
			a.logCancel = nil
			a.logCtx = nil
			a.logStream = nil
		}
		if a.logView != nil {
			a.logView.Close()
		}
		return a, nil

	// ── 4. Chrome state updates ─────────────────────────────────────
	case messages.SyncStateMsg:
		if msg.Err == nil {
			a.syncState = msg.State
		}
		cmd := a.router.Update(msg)
		cmds = append(cmds, cmd)
		return a, tea.Batch(cmds...)

	case messages.StoreStatsMsg:
		if msg.Err == nil {
			a.storeStats = msg.Stats
		}
		cmd := a.router.Update(msg)
		cmds = append(cmds, cmd)
		return a, tea.Batch(cmds...)

	case messages.BalanceLoadedMsg:
		cmd := a.router.Update(msg)
		return a, cmd

	// ── 5. ViewDataRefreshMsg ───────────────────────────────────────
	case messages.ViewDataRefreshMsg:
		if active := a.router.Active(); active != nil {
			cmds = append(cmds, active.Refresh())
		}
		if a.bridge != nil {
			cmds = append(cmds, a.bridge.waitForEvent())
		}
		return a, tea.Batch(cmds...)

	// ── 6. Confirm / Cancel ─────────────────────────────────────────
	case components.ConfirmMsg:
		a.confirm = nil
		return a, nil

	case components.CancelMsg:
		a.confirm = nil
		return a, nil

	// ── 7. Log line / close ─────────────────────────────────────────
	case messages.LogLineMsg:
		if a.logView != nil && a.logView.Active() {
			a.logView.AppendLine(views.LogLine{Scope: msg.Name, Message: msg.Message})
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
		a.monitorActive = false
		for a.router.Depth() > 1 {
			a.router.Pop()
		}
		return a, nil
	}

	// ── 8. Standalone mode: monitor owns the full input surface ─────
	if a.standalone {
		if kmsg, ok := msg.(tea.KeyPressMsg); ok {
			if a.monitor != nil {
				var monitorCmd tea.Cmd
				a.monitor, monitorCmd = a.monitor.Update(kmsg)
				return a, tea.Batch(append(cmds, monitorCmd)...)
			}
			if key.Matches(kmsg, a.keys.Quit) {
				return a, tea.Quit
			}
		}
		return a, tea.Batch(cmds...)
	}

	// ── 10. Overlay priority for keys ───────────────────────────────
	if a.confirm != nil && a.confirm.Active() {
		if kmsg, ok := msg.(tea.KeyPressMsg); ok {
			if key.Matches(kmsg, a.keys.Quit) {
				return a, tea.Quit
			}
			cmd := a.confirm.Update(msg)
			return a, tea.Batch(append(cmds, cmd)...)
		}
		return a, tea.Batch(cmds...)
	}

	if a.logView != nil && a.logView.Active() {
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
				a.logView.Close()
			case " ":
				a.logView.TogglePause()
			case "c":
				a.logView.Clear()
			case "k", "up":
				a.logView.ScrollUp()
			case "j", "down":
				a.logView.ScrollDown()
			case "G":
				a.logView.ScrollToBottom()
			case "s":
				a.logView.CycleServiceFilter()
			}
			return a, tea.Batch(cmds...)
		}
		return a, tea.Batch(cmds...)
	}

	if a.help != nil && a.help.Active() {
		if kmsg, ok := msg.(tea.KeyPressMsg); ok {
			if key.Matches(kmsg, a.keys.Quit) {
				return a, tea.Quit
			}
			if kmsg.String() == "esc" {
				a.help.Close()
			}
			return a, tea.Batch(cmds...)
		}
		return a, tea.Batch(cmds...)
	}

	if a.palette != nil && a.palette.Active() {
		if kmsg, ok := msg.(tea.KeyPressMsg); ok {
			if key.Matches(kmsg, a.keys.Quit) {
				return a, tea.Quit
			}
			cmd := a.palette.Update(msg)
			return a, tea.Batch(append(cmds, cmd)...)
		}
		return a, tea.Batch(cmds...)
	}

	// ── 11. Global keys ─────────────────────────────────────────────
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(kmsg, a.keys.Quit) {
			return a, tea.Quit
		}

		// When monitor is active, forward keys to it (except global keys below).
		if a.monitorActive && a.monitor != nil {
			// Monitor-local 1-3 and dashboard navigation take precedence.
			// Shell-level 4-6 and Esc switch away below.
			if !key.Matches(kmsg, a.keys.Command, a.keys.CommandSearch, a.keys.Help,
				a.keys.Monitor, a.keys.Governance, a.keys.Staking, a.keys.Back) {
				var topCmd tea.Cmd
				a.monitor, topCmd = a.monitor.Update(msg)
				return a, topCmd
			}
		}

		// Command palette
		if key.Matches(kmsg, a.keys.Command) || key.Matches(kmsg, a.keys.CommandSearch) {
			if a.palette != nil {
				a.palette.Open()
			}
			return a, nil
		}

		// Help
		if key.Matches(kmsg, a.keys.Help) {
			if a.help != nil {
				bc := a.router.Breadcrumb()
				a.help.Open(bc)
			}
			return a, nil
		}

		// Number keys 1-6 for view switching: pop to root, then replace with target view.
		owner := ""
		if a.resolvedCtx != nil {
			owner = a.resolvedCtx.DefaultAccount
		}

		switch {
		case key.Matches(kmsg, a.keys.Deployments):
			a.monitorActive = false
			for a.router.Depth() > 1 {
				a.router.Pop()
			}
			v := views.NewDeploymentsView(a.data, a.keys, owner)
			return a, a.router.Replace(v)
		case key.Matches(kmsg, a.keys.Leases):
			a.monitorActive = false
			for a.router.Depth() > 1 {
				a.router.Pop()
			}
			v := views.NewLeasesView(a.data, a.keys, owner)
			return a, a.router.Replace(v)
		case key.Matches(kmsg, a.keys.Providers):
			a.monitorActive = false
			for a.router.Depth() > 1 {
				a.router.Pop()
			}
			v := views.NewProvidersView(a.data, a.keys)
			return a, a.router.Replace(v)
		case key.Matches(kmsg, a.keys.Monitor):
			for a.router.Depth() > 1 {
				a.router.Pop()
			}
			a.monitorActive = a.monitor != nil
			return a, nil
		case key.Matches(kmsg, a.keys.Governance):
			a.monitorActive = false
			for a.router.Depth() > 1 {
				a.router.Pop()
			}
			v := views.NewGovernanceView(a.data, a.keys)
			return a, a.router.Replace(v)
		case key.Matches(kmsg, a.keys.Staking):
			a.monitorActive = false
			for a.router.Depth() > 1 {
				a.router.Pop()
			}
			v := views.NewStakingView(a.data, a.keys)
			return a, a.router.Replace(v)
		case key.Matches(kmsg, a.keys.Back):
			a.monitorActive = false
			if a.router.Depth() > 1 {
				a.router.Pop()
			}
			return a, nil
		}

		// Delegate all other keys to active view
		cmd := a.router.Update(msg)
		return a, cmd
	}

	// Forward remaining data messages to active view
	cmd := a.router.Update(msg)
	cmds = append(cmds, cmd)
	return a, tea.Batch(cmds...)
}

// View implements tea.Model.
func (a App) View() tea.View {
	// Standalone monitor mode
	if a.standalone && a.monitor != nil {
		view := a.monitor.View()
		view.AltScreen = true
		return view
	}

	contentH := a.height - chromeHeight
	if contentH < 1 {
		contentH = 1
	}
	pin := lipgloss.NewStyle().Height(contentH).MaxHeight(contentH)

	// When the monitor is active, it renders its own chrome.
	if a.monitorActive && a.monitor != nil && !a.paletteActive() {
		monView := a.monitor.View()
		content := pin.Render(monView.Content)
		v := tea.NewView(content)
		v.AltScreen = true
		return v
	}

	header := a.renderHeader()
	navBar := a.renderNavBar()
	breadcrumb := a.renderBreadcrumb()

	var main string
	if a.monitorActive && a.monitor == nil {
		main = a.renderCentered(contentH, "No RPC endpoint configured.\nUse :consensus after setting up a context.")
	} else {
		main = a.router.View().Content
	}

	// Overlay compositing
	if a.logView != nil && a.logView.Active() {
		main = a.logView.View().Content
	}
	if a.confirm != nil && a.confirm.Active() {
		main = overlayCenter(main, a.confirm.View(), a.width, contentH)
	}
	if a.help != nil && a.help.Active() {
		main = overlayCenter(main, a.help.View().Content, a.width, contentH)
	}
	if a.palette != nil && a.palette.Active() {
		main = overlayCenter(main, a.palette.View().Content, a.width, contentH)
	}

	if a.toast != nil && !a.toast.Expired() {
		main = main + "\n" + a.toast.View()
	}

	pinnedMain := pin.Render(main)
	chrome := header + "\n" + navBar + "\n" + breadcrumb + "\n" + pinnedMain
	v := tea.NewView(chrome)
	v.AltScreen = true
	return v
}

// ─── Chrome rendering ────────────────────────────────────────────────

func (a App) renderHeader() string {
	appName := theme.HeaderAppName.Render("akt")

	// Build left side: akt  v1.2.0  chain  akashnet-2  rpc  endpoint  block  height
	var parts []string
	parts = append(parts, appName)

	// Version
	ver := "dev"
	parts = append(parts, theme.HeaderMeta.Render("v"+ver))

	// Chain ID
	chainID := ""
	if a.resolvedCtx != nil {
		chainID = a.resolvedCtx.Network.ChainID
	}
	if chainID != "" {
		parts = append(parts, theme.HeaderMeta.Render("chain")+"  "+theme.HeaderContext.Render(chainID))
	}

	// RPC endpoint
	rpcEndpoint := ""
	if a.resolvedCtx != nil && len(a.resolvedCtx.Network.Endpoints.RPC) > 0 {
		rpcEndpoint = a.resolvedCtx.Network.Endpoints.RPC[0]
	}
	if rpcEndpoint != "" {
		parts = append(parts, theme.HeaderMeta.Render("rpc")+"  "+theme.HeaderContext.Render(rpcEndpoint))
	}

	// Block height
	blockStr := ""
	if a.syncState != nil && a.syncState.LastBlockHeight > 0 {
		blockStr = fmt.Sprintf("%d", a.syncState.LastBlockHeight)
		// Comma-group the block height
		if len(blockStr) > 3 {
			var result []byte
			for i, c := range blockStr {
				if i > 0 && (len(blockStr)-i)%3 == 0 {
					result = append(result, ',')
				}
				result = append(result, byte(c))
			}
			blockStr = string(result)
		}
		parts = append(parts, theme.HeaderMeta.Render("block")+"  "+theme.HeaderContext.Render(blockStr))
	}

	left := strings.Join(parts, "  ")

	// Right side: sync status + time
	var sync string
	if a.syncState != nil {
		sync = lipgloss.NewStyle().Foreground(theme.GreenColor).Render("●") +
			"  " + theme.HeaderValue.Render("synced")
	} else {
		sync = theme.HeaderMeta.Render("○  no sync")
	}

	right := sync

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
}{
	{"1", "Deployments"},
	{"2", "Leases"},
	{"3", "Providers"},
	{"4", "Monitor"},
	{"5", "Governance"},
	{"6", "Staking"},
}

func (a App) renderNavBar() string {
	bc := a.router.Breadcrumb()
	var parts []string

	isDashboard := !a.monitorActive && (strings.HasPrefix(bc, "Dashboard") || bc == "")
	if isDashboard {
		parts = append(parts, theme.NavTabActive.Render("Dashboard"))
	} else {
		parts = append(parts, theme.NavTabInactive.Render("Dashboard"))
	}

	for _, nav := range navItems {
		label := nav.key + " " + nav.name
		active := false
		if nav.name == "Monitor" {
			active = a.monitorActive
		} else {
			active = !a.monitorActive && strings.Contains(bc, nav.name)
		}
		if active {
			parts = append(parts, theme.NavTabActive.Render(label))
		} else {
			parts = append(parts, theme.NavTabInactive.Render(label))
		}
	}

	rightParts := a.navBarHints()
	rightStr := strings.Join(rightParts, "  ")

	bar := " " + strings.Join(parts, " ")
	gap := a.width - lipgloss.Width(bar) - lipgloss.Width(rightStr) - 1
	if gap < 1 {
		gap = 1
	}
	bar = bar + strings.Repeat(" ", gap) + rightStr

	return bar + "\n" + components.HRule(a.width)
}

// navBarHints returns the right-side shortcut hints for the nav bar.
func (a App) navBarHints() []string {
	accentStyle := lipgloss.NewStyle().Foreground(theme.AccentRed).Bold(true)
	keyStyle := theme.FooterKey
	descStyle := theme.FooterDesc

	// Common global hints always shown.
	hints := []string{
		accentStyle.Render("D") + " " + descStyle.Render("deploy"),
		keyStyle.Render(":") + " " + descStyle.Render("cmd"),
		keyStyle.Render("?") + " " + descStyle.Render("help"),
	}

	return hints
}

func (a App) renderBreadcrumb() string {
	var bc string
	if a.monitorActive {
		bc = "Monitor"
	} else {
		bc = a.router.Breadcrumb()
		if bc == "" {
			bc = "Dashboard"
		}
	}
	parts := strings.Split(bc, " > ")
	sep := theme.BreadcrumbSeparator.Render(" / ")
	var styled []string
	for _, p := range parts {
		styled = append(styled, theme.BreadcrumbActive.Render(p))
	}
	return " " + strings.Join(styled, sep)
}

func (a App) paletteActive() bool {
	return a.palette != nil && a.palette.Active()
}

func (a App) renderCentered(h int, text string) string {
	style := lipgloss.NewStyle().
		Width(a.width).
		Height(h).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(text)
}

// overlayCenter composites an overlay string centered on top of a base view.
// Overlay lines replace the corresponding base lines at the center position.
func overlayCenter(base, overlay string, w, h int) string {
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < h {
		baseLines = append(baseLines, "")
	}

	overlayLines := strings.Split(overlay, "\n")
	overlayH := len(overlayLines)
	overlayW := 0
	for _, line := range overlayLines {
		if lw := lipgloss.Width(line); lw > overlayW {
			overlayW = lw
		}
	}

	startY := (h - overlayH) / 2
	if startY < 2 {
		startY = 2
	}
	startX := (w - overlayW) / 2
	if startX < 0 {
		startX = 0
	}

	for i, line := range overlayLines {
		row := startY + i
		if row < len(baseLines) {
			baseLines[row] = strings.Repeat(" ", startX) + line
		}
	}

	if len(baseLines) > h {
		baseLines = baseLines[:h]
	}
	return strings.Join(baseLines, "\n")
}

// ─── Helpers ─────────────────────────────────────────────────────────

func (a App) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	owner := ""
	if a.resolvedCtx != nil {
		owner = a.resolvedCtx.DefaultAccount
	}
	popToRoot := func() {
		for a.router.Depth() > 1 {
			a.router.Pop()
		}
	}

	switch strings.ToLower(cmd) {
	case "quit", "q", "exit":
		return a, tea.Quit
	case "dashboard", "home":
		popToRoot()
		dctx := views.DashboardContext{Version: "dev"}
		if a.resolvedCtx != nil {
			dctx.ContextName = a.resolvedCtx.Name
			dctx.ChainID = a.resolvedCtx.Network.ChainID
			dctx.Account = a.resolvedCtx.DefaultAccount
			if len(a.resolvedCtx.Network.Endpoints.RPC) > 0 {
				dctx.RPCEndpoint = a.resolvedCtx.Network.Endpoints.RPC[0]
			}
		}
		return a, a.router.Replace(views.NewDashboard(a.data, dctx, a.keys))
	case "deployments", "dep":
		popToRoot()
		return a, a.router.Replace(views.NewDeploymentsView(a.data, a.keys, owner))
	case "leases":
		popToRoot()
		return a, a.router.Replace(views.NewLeasesView(a.data, a.keys, owner))
	case "providers", "prov":
		popToRoot()
		return a, a.router.Replace(views.NewProvidersView(a.data, a.keys))
	case "governance", "gov":
		popToRoot()
		return a, a.router.Replace(views.NewGovernanceView(a.data, a.keys))
	case "staking", "validators", "val":
		popToRoot()
		return a, a.router.Replace(views.NewStakingView(a.data, a.keys))
	case "monitor", "consensus", "top":
		popToRoot()
		a.monitorActive = a.monitor != nil
		return a, nil
	case "certificates", "escrow", "orders", "bids", "deploy", "help":
		// Not yet implemented
		return a, nil
	}
	return a, nil
}

// showToast creates a toast notification and returns the expiry tick command.
func (a *App) showToast(msg string, tone components.ToastTone) tea.Cmd {
	t := components.NewToast(msg, tone)
	a.toast = &t
	return tea.Tick(components.ToastDuration, func(time.Time) tea.Msg {
		return components.ToastExpiredMsg{}
	})
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

// ─── Entry points ────────────────────────────────────────────────────

// buildLightClient attempts to create a LightClient from the resolved context
// and RPC endpoint when one is not already provided. The TUI only performs
// queries so no keyring is needed.
func buildLightClient(cfg *Config) {
	if cfg.LightClient != nil || cfg.ResolvedCtx == nil || cfg.RPCEndpoint == "" {
		return
	}

	cctx := aktclient.BuildClientContext(cfg.ResolvedCtx, nil, aktcodec.MakeEncodingConfig(), "")

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

func Run(cfg Config) error {
	buildLightClient(&cfg)

	model, bus, cleanup, err := buildMonitorModel(cfg)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	app := newApp(cfg, model)

	if bus != nil && cfg.Store != nil && cfg.ResolvedCtx != nil {
		eng := synce.New(cfg.Store, []string{cfg.ResolvedCtx.DefaultAccount})
		if bridge, err := newSyncBridge(bus, eng); err == nil {
			app.bridge = bridge
			defer bridge.close()
		}
	}

	p := tea.NewProgram(app)
	_, err = p.Run()
	return err
}

// RunMonitor launches the TUI in standalone monitor mode.
// cfg.InitialDashboard selects which dashboard is shown first
// ("network", "provider", "bme", or "" for the default network).
func RunMonitor(cfg Config) error {
	buildLightClient(&cfg)

	model, bus, cleanup, err := buildMonitorModel(cfg)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	app := newApp(cfg, model)
	// In standalone mode, show monitor directly
	app.monitorActive = model != nil

	if bus != nil && cfg.Store != nil && cfg.ResolvedCtx != nil {
		eng := synce.New(cfg.Store, []string{cfg.ResolvedCtx.DefaultAccount})
		if bridge, err := newSyncBridge(bus, eng); err == nil {
			app.bridge = bridge
			defer bridge.close()
		}
	}

	p := tea.NewProgram(app)
	_, err = p.Run()
	return err
}

func buildMonitorModel(cfg Config) (tea.Model, pubsub.Bus, func(), error) {
	if cfg.RPCEndpoint == "" || cfg.CacheDir == "" {
		return nil, nil, nil, nil
	}

	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create monitor cache directory %s: %w", cfg.CacheDir, err)
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
		return nil, nil, nil, fmt.Errorf("open monitor cache %s: %w", dbPath, err)
	}

	provCache, err := monitorcache.Open(db)
	if err != nil {
		_ = db.Close()
		return nil, nil, nil, fmt.Errorf("initialize monitor provider cache: %w", err)
	}

	monCache, err := monitorcache.OpenMonikerCache(db)
	if err != nil {
		_ = db.Close()
		return nil, nil, nil, fmt.Errorf("initialize monitor moniker cache: %w", err)
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
		Embedded:           !cfg.Standalone,
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
		_ = db.Close()
	}

	return model, bus, cleanup, nil
}
