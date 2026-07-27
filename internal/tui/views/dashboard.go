package views

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/data"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/ui/theme"
)

// maxActiveDeployments is the number of active deployments shown in the dashboard panel.
const maxActiveDeployments = 4

// maxActivityEntries is the number of recent activity entries shown.
const maxActivityEntries = 8

// ActivityEntry represents a single recent activity event.
type ActivityEntry struct {
	Time string // "14:02:11"
	Kind string // "tx", "evt", "gov"
	Text string // description
}

// Compile-time check: *Dashboard must satisfy ViewComponent.
var _ ViewComponent = (*Dashboard)(nil)

// DashboardContext holds static chrome info passed at construction time.
type DashboardContext struct {
	ContextName string
	ChainID     string
	Account     string
	RPCEndpoint string
	Version     string
}

// Dashboard is the landing view that shows a summary of the user's Akash state.
type Dashboard struct {
	width  int
	height int

	// Dependencies
	svc data.Service     // for data loading
	ctx DashboardContext // static context info
	km  keys.KeyMap      // key bindings

	// Context
	contextName string
	chainID     string
	account     string
	rpcEndpoint string
	version     string

	// Store data
	stats       *store.StoreStats
	syncState   *store.SyncState
	deployments []*store.DeploymentRecord // active only
	syncActive  bool                      // true when the sync bridge is running

	// Legacy card data (kept for backward compat with existing setters)
	balance        string // formatted balance (e.g., "148.52 AKT")
	validatorCount int    // active validators
	proposalCount  int    // proposals in voting

	// Wallet
	liquid, staked, rewards, escrow string
	price, priceChange              string
	priceHistory                    []float64

	// Network
	blockTime                  string
	activeProv                 int
	bonded, inflation          string
	blockTimes                 []float64
	blockTimeAvg, blockTimeMax string

	// Activity
	activity []ActivityEntry
}

// NewDashboard returns a new Dashboard wired to the given data service,
// static context, and key map. It applies context data immediately.
func NewDashboard(svc data.Service, ctx DashboardContext, km keys.KeyMap) *Dashboard {
	d := &Dashboard{
		svc: svc,
		ctx: ctx,
		km:  km,
	}
	// Apply static context via existing setters.
	d.SetContext(ctx.ContextName, ctx.ChainID, ctx.Account)
	d.SetRPCEndpoint(ctx.RPCEndpoint)
	d.SetVersion(ctx.Version)
	return d
}

// ─── tea.Model + ViewComponent ───────────────────────────────────────

// Init fires the initial data loads for the dashboard.
func (d *Dashboard) Init() tea.Cmd {
	if d.svc == nil {
		return nil
	}
	return tea.Batch(
		d.svc.LoadDeployments(d.ctx.Account),
		d.svc.LoadStoreStats(),
		d.svc.LoadSyncState(),
		d.svc.LoadBalance(d.ctx.Account),
	)
}

// Update handles incoming messages and updates dashboard state.
func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.DeploymentsLoadedMsg:
		if msg.Err == nil {
			var active []*store.DeploymentRecord
			for _, dep := range msg.Deployments {
				if dep.State == "active" {
					active = append(active, dep)
				}
			}
			d.SetActiveDeployments(active)
		}
	case messages.StoreStatsMsg:
		if msg.Err == nil {
			d.SetStats(msg.Stats)
		}
	case messages.SyncStateMsg:
		if msg.Err == nil {
			d.SetSyncState(msg.State)
			if msg.State != nil {
				d.SetSyncBridgeActive(true)
			}
		}
	case messages.BalanceLoadedMsg:
		if msg.Err == nil {
			d.SetBalance(msg.Amount)
			d.SetWallet(msg.Amount, "", "", "")
		}
	}
	return d, nil
}

// Breadcrumb returns the navigation label for this view.
func (d *Dashboard) Breadcrumb() string {
	return "Dashboard"
}

// ShortHelp returns the footer hint pairs for the dashboard.
func (d *Dashboard) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "1-6", Desc: "navigate"},
		{Key: ":", Desc: "command"},
		{Key: "?", Desc: "help"},
		{Key: "D", Desc: "deploy", Accent: true},
	}
}

// Refresh re-fires the same data loads as Init.
func (d *Dashboard) Refresh() tea.Cmd {
	return d.Init()
}

// ─── Existing Setters (unchanged) ────────────────────────────────────

// SetContext sets the context name, chain ID, and account address.
func (d *Dashboard) SetContext(name, chainID, account string) {
	d.contextName = name
	d.chainID = chainID
	d.account = account
}

// SetStats sets the store statistics.
func (d *Dashboard) SetStats(stats *store.StoreStats) {
	d.stats = stats
}

// SetSyncState sets the sync state.
func (d *Dashboard) SetSyncState(state *store.SyncState) {
	d.syncState = state
}

// SetActiveDeployments sets the active deployments to display.
func (d *Dashboard) SetActiveDeployments(depls []*store.DeploymentRecord) {
	d.deployments = depls
}

// SetSyncBridgeActive sets whether the sync bridge is running.
func (d *Dashboard) SetSyncBridgeActive(active bool) {
	d.syncActive = active
}

// SetBalance sets the formatted balance string (e.g., "148.52 AKT").
func (d *Dashboard) SetBalance(amount string) {
	d.balance = amount
}

// SetValidatorCount sets the number of active validators.
func (d *Dashboard) SetValidatorCount(active int) {
	d.validatorCount = active
}

// SetProposalCount sets the number of proposals currently in voting.
func (d *Dashboard) SetProposalCount(voting int) {
	d.proposalCount = voting
}

// SetSize sets the available width and height.
func (d *Dashboard) SetSize(w, h int) {
	d.width = w
	d.height = h
}

// ─── New Setters ─────────────────────────────────────────────────────

// SetWallet sets the wallet balance fields.
func (d *Dashboard) SetWallet(liquid, staked, rewards, escrow string) {
	d.liquid = liquid
	d.staked = staked
	d.rewards = rewards
	d.escrow = escrow
}

// SetPrice sets the current price and 24h change.
func (d *Dashboard) SetPrice(price, change string) {
	d.price = price
	d.priceChange = change
}

// SetPriceHistory sets the sparkline data for price history.
func (d *Dashboard) SetPriceHistory(data []float64) {
	d.priceHistory = data
}

// SetBlockTimes sets the block time sparkline data and summary stats.
func (d *Dashboard) SetBlockTimes(data []float64, avg, peak string) {
	d.blockTimes = data
	d.blockTimeAvg = avg
	d.blockTimeMax = peak
}

// SetNetworkInfo sets network-level statistics.
func (d *Dashboard) SetNetworkInfo(blockTime string, activeProv int, bonded, inflation string) {
	d.blockTime = blockTime
	d.activeProv = activeProv
	d.bonded = bonded
	d.inflation = inflation
}

// SetRecentActivity sets the recent activity entries.
func (d *Dashboard) SetRecentActivity(entries []ActivityEntry) {
	d.activity = entries
}

// SetRPCEndpoint sets the RPC endpoint string.
func (d *Dashboard) SetRPCEndpoint(endpoint string) {
	d.rpcEndpoint = endpoint
}

// SetVersion sets the version badge string.
func (d *Dashboard) SetVersion(version string) {
	d.version = version
}

// ─── View ────────────────────────────────────────────────────────────

// View renders the dashboard.
func (d *Dashboard) View() tea.View {
	w := d.width
	if w < 40 {
		w = 80
	}

	colW := (w - 4) / 3 // 3 columns with gaps
	wideW := colW*2 + 2 // 2-column span for activity panel

	var sections []string

	// Row 1: Welcome banner (full width)
	sections = append(sections, d.renderWelcome(w))

	// Row 2: Wallet | Active | Network (3 columns)
	// Build content for each panel first, then equalize heights.
	walletContent := d.walletContent(colW - 4)
	activeContent := d.activeContent(colW - 4)
	networkContent := d.networkContent(colW - 4)

	// Find max content lines across all three panels.
	maxLines := components.ContentLineCount(walletContent)
	if n := components.ContentLineCount(activeContent); n > maxLines {
		maxLines = n
	}
	if n := components.ContentLineCount(networkContent); n > maxLines {
		maxLines = n
	}

	row2 := lipgloss.JoinHorizontal(lipgloss.Top,
		components.TitledPanelHeight("WALLET", walletContent, colW, maxLines), " ",
		components.TitledPanelHeight(fmt.Sprintf("ACTIVE · %d", len(d.deployments)), activeContent, colW, maxLines), " ",
		components.TitledPanelHeight("NETWORK", networkContent, colW, maxLines))
	sections = append(sections, row2)

	// Row 3: Recent Activity (2 cols) | Shortcuts (1 col)
	// Row 3 should expand to fill remaining terminal height.
	activityContent := d.activityContent(wideW - 4)
	shortcutsContent := d.shortcutsContent()

	maxLines3 := components.ContentLineCount(activityContent)
	if n := components.ContentLineCount(shortcutsContent); n > maxLines3 {
		maxLines3 = n
	}

	// Calculate how many content lines row 3 needs to fill the terminal.
	// Each TitledPanel adds 2 lines (top + bottom border).
	row1Lines := strings.Count(sections[0], "\n") + 1
	row2Lines := strings.Count(row2, "\n") + 1
	usedLines := row1Lines + row2Lines + 2 // +2 for newline separators between rows
	remainingLines := d.height - usedLines
	if remainingLines < maxLines3+2 {
		remainingLines = maxLines3 + 2
	}
	// remainingLines includes the panel borders, so content lines = remaining - 2
	row3ContentLines := remainingLines - 2
	if row3ContentLines < maxLines3 {
		row3ContentLines = maxLines3
	}

	row3 := lipgloss.JoinHorizontal(lipgloss.Top,
		components.TitledPanelHeight("RECENT ACTIVITY", activityContent, wideW, row3ContentLines), " ",
		components.TitledPanelHeight("SHORTCUTS", shortcutsContent, colW, row3ContentLines))
	sections = append(sections, row3)

	return tea.NewView(strings.Join(sections, "\n"))
}

// ─── Welcome Banner ──────────────────────────────────────────────────

func (d *Dashboard) renderWelcome(w int) string {
	artStyle := lipgloss.NewStyle().Foreground(theme.AccentRed).Bold(true)
	art := artStyle.Render(" ▄▀█ █▄▀ ▀█▀") + "\n" +
		artStyle.Render(" █▀█ █ █  █ ")

	account := d.account
	if account == "" {
		account = "unknown"
	}
	greeting := lipgloss.NewStyle().Foreground(theme.Slate200).Bold(true).
		Render("welcome back, " + account)

	// Subtitle line
	ctx := d.contextName
	if ctx == "" {
		ctx = "—"
	}
	endpoint := d.rpcEndpoint
	if endpoint == "" {
		endpoint = "—"
	}
	syncAgo := "—"
	if d.syncState != nil && d.syncState.LastSyncTime > 0 {
		elapsed := time.Since(time.Unix(d.syncState.LastSyncTime, 0))
		if elapsed < time.Minute {
			syncAgo = fmt.Sprintf("%ds ago", int(elapsed.Seconds()))
		} else {
			syncAgo = fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
		}
	}
	subtitle := lipgloss.NewStyle().Foreground(theme.Slate500).
		Render("connected to " + ctx + " · rpc " + endpoint + " · last sync " + syncAgo)

	leftContent := art + "\n" + greeting + "\n" + subtitle

	// Right side: sync status + version
	var syncBadge string
	if d.syncActive {
		syncBadge = lipgloss.NewStyle().Foreground(theme.GreenColor).Render("● SYNCED")
	} else {
		syncBadge = lipgloss.NewStyle().Foreground(theme.Slate500).Render("○ no sync")
	}
	ver := d.version
	if ver == "" {
		ver = "—"
	}
	versionBadge := lipgloss.NewStyle().Foreground(theme.Slate400).Render(ver)
	rightContent := syncBadge + "  " + versionBadge

	// Calculate spacing
	leftW := lipgloss.Width(leftContent)
	_ = leftW // used below for line-by-line layout

	// Build the banner as a titled panel with empty title
	// We'll build it manually for the side-by-side layout
	innerW := w - 4 // border chars

	// Split left content into lines
	leftLines := strings.Split(leftContent, "\n")
	rightLines := []string{rightContent}

	// Pad to same number of lines
	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}
	for len(leftLines) < maxLines {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < maxLines {
		rightLines = append(rightLines, "")
	}

	// Build content with right-aligned status on first line
	var contentLines []string
	for i, left := range leftLines {
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		lw := lipgloss.Width(left)
		rw := lipgloss.Width(right)
		gap := innerW - lw - rw
		if gap < 1 {
			gap = 1
		}
		contentLines = append(contentLines, left+strings.Repeat(" ", gap)+right)
	}

	content := strings.Join(contentLines, "\n")
	return components.TitledPanel("", content, w)
}

// ─── Wallet Panel ────────────────────────────────────────────────────

// walletContent returns the inner content for the WALLET panel.
func (d *Dashboard) walletContent(innerW int) string {

	var lines []string

	// Address
	addr := d.account
	if addr == "" {
		addr = "—"
	}
	// Truncate address if too long for the panel
	maxAddrLen := innerW - 10
	if maxAddrLen < 6 {
		maxAddrLen = 6
	}
	if len(addr) > maxAddrLen {
		addr = addr[:maxAddrLen-1] + "…"
	}
	lines = append(lines, kvRight("address", lipgloss.NewStyle().Foreground(theme.Slate200).Render(addr), innerW))

	// Liquid balance — use legacy balance field as fallback
	liq := d.liquid
	if liq == "" {
		liq = d.balance
	}
	if liq == "" {
		liq = "—"
	}
	lines = append(lines, kvRight("liquid", lipgloss.NewStyle().Foreground(theme.Slate200).Render(liq), innerW))

	// Staked
	stk := d.staked
	if stk == "" {
		stk = "—"
	}
	lines = append(lines, kvRight("staked", lipgloss.NewStyle().Foreground(theme.Slate200).Render(stk), innerW))

	// Rewards
	rew := d.rewards
	if rew == "" {
		rew = "—"
	}
	rewStyle := lipgloss.NewStyle().Foreground(theme.Slate200)
	if strings.HasPrefix(rew, "+") {
		rewStyle = lipgloss.NewStyle().Foreground(theme.GreenColor)
	}
	lines = append(lines, kvRight("rewards", rewStyle.Render(rew), innerW))

	// Escrow
	esc := d.escrow
	if esc == "" {
		esc = "—"
	}
	lines = append(lines, kvRight("escrow", lipgloss.NewStyle().Foreground(theme.Slate200).Render(esc), innerW))

	// Blank line
	lines = append(lines, "")

	// Price section
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.Slate500).Render("price (24h)"))

	// Sparkline
	if len(d.priceHistory) > 0 {
		lines = append(lines, components.Sparkline(d.priceHistory, innerW, theme.GreenColor))
	}

	// Price + change
	pr := d.price
	if pr == "" {
		pr = "—"
	}
	chg := d.priceChange
	priceStr := lipgloss.NewStyle().Foreground(theme.Slate200).Render(pr)
	if chg != "" {
		changeStyle := lipgloss.NewStyle().Foreground(theme.GreenColor)
		priceStr += " " + changeStyle.Render("▲ "+chg)
	}
	lines = append(lines, priceStr)

	return strings.Join(lines, "\n")
}

// ─── Active Deployments Panel ────────────────────────────────────────

// activeContent returns the inner content for the ACTIVE panel.
func (d *Dashboard) activeContent(innerW int) string {
	var lines []string

	if len(d.deployments) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Slate500).
			Render("No deployments"))
	} else {
		limit := len(d.deployments)
		if limit > maxActiveDeployments {
			limit = maxActiveDeployments
		}

		var totalCost string
		for _, dep := range d.deployments[:limit] {
			var name string
			if dep.SDLPath != "" {
				name = strings.TrimSuffix(
					strings.TrimSuffix(dep.SDLPath, ".yaml"),
					".yml")
				// Use just the base name
				parts := strings.Split(name, "/")
				name = parts[len(parts)-1]
			} else {
				name = fmt.Sprintf("dseq-%d", dep.DSeq)
			}

			cost := "—"
			if dep.Deposit != "" {
				cost = dep.Deposit
			}

			nameRendered := lipgloss.NewStyle().Foreground(theme.Slate300).Render(name)
			costRendered := lipgloss.NewStyle().Foreground(theme.Slate500).Render(cost)
			nameW := lipgloss.Width(nameRendered)
			costW := lipgloss.Width(costRendered)
			gap := innerW - nameW - costW
			if gap < 1 {
				gap = 1
			}
			lines = append(lines, nameRendered+strings.Repeat(" ", gap)+costRendered)
		}

		// Dashed separator
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Slate700).
			Render(strings.Repeat("·", innerW)))

		// Monthly burn
		if totalCost == "" {
			totalCost = "—"
		}
		burnLabel := lipgloss.NewStyle().Foreground(theme.Slate500).Render("monthly burn")
		burnValue := lipgloss.NewStyle().Foreground(theme.Slate200).Bold(true).Render(totalCost)
		burnLabelW := lipgloss.Width(burnLabel)
		burnValueW := lipgloss.Width(burnValue)
		burnGap := innerW - burnLabelW - burnValueW
		if burnGap < 1 {
			burnGap = 1
		}
		lines = append(lines, burnLabel+strings.Repeat(" ", burnGap)+burnValue)
	}

	// Blank line
	lines = append(lines, "")

	// Hint
	hint := lipgloss.NewStyle().Foreground(theme.Slate500).Render("press ") +
		keyPill("1", false) +
		lipgloss.NewStyle().Foreground(theme.Slate500).Render(" full list · ") +
		keyPill("D", true) +
		lipgloss.NewStyle().Foreground(theme.Slate500).Render(" new")
	lines = append(lines, hint)

	return strings.Join(lines, "\n")
}

// ─── Network Panel ───────────────────────────────────────────────────

// networkContent returns the inner content for the NETWORK panel.
func (d *Dashboard) networkContent(innerW int) string {
	var lines []string

	// Block height
	var blockHeight string
	if d.syncState != nil && d.syncState.LastBlockHeight > 0 {
		blockHeight = commaGroup(d.syncState.LastBlockHeight)
	} else {
		blockHeight = "—"
	}
	lines = append(lines, kvRight("height",
		lipgloss.NewStyle().Foreground(theme.Slate200).Bold(true).Render(blockHeight), innerW))

	// Block time
	bt := d.blockTime
	if bt == "" {
		bt = "—"
	}
	lines = append(lines, kvRight("block time",
		lipgloss.NewStyle().Foreground(theme.Slate200).Render(bt), innerW))

	// Active providers
	var provStr string
	if d.activeProv > 0 {
		provStr = fmt.Sprintf("%d", d.activeProv)
	} else if d.validatorCount > 0 {
		provStr = fmt.Sprintf("%d", d.validatorCount)
	} else {
		provStr = "—"
	}
	lines = append(lines, kvRight("active prov.",
		lipgloss.NewStyle().Foreground(theme.Slate200).Render(provStr), innerW))

	// Bonded
	bnd := d.bonded
	if bnd == "" {
		bnd = "—"
	}
	lines = append(lines, kvRight("bonded",
		lipgloss.NewStyle().Foreground(theme.Slate200).Render(bnd), innerW))

	// Inflation
	inf := d.inflation
	if inf == "" {
		inf = "—"
	}
	lines = append(lines, kvRight("inflation",
		lipgloss.NewStyle().Foreground(theme.Slate200).Render(inf), innerW))

	// Blank line
	lines = append(lines, "")

	// Block times sparkline
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.Slate500).Render("block times (last 60)"))

	if len(d.blockTimes) > 0 {
		lines = append(lines, components.Sparkline(d.blockTimes, innerW, theme.AccentRed))
	}

	// Avg / max
	avg := d.blockTimeAvg
	if avg == "" {
		avg = "—"
	}
	mx := d.blockTimeMax
	if mx == "" {
		mx = "—"
	}
	statsLine := lipgloss.NewStyle().Foreground(theme.Slate500).Render("avg ") +
		lipgloss.NewStyle().Foreground(theme.Slate300).Render(avg) +
		lipgloss.NewStyle().Foreground(theme.Slate500).Render(" · max ") +
		lipgloss.NewStyle().Foreground(theme.Slate300).Render(mx)
	lines = append(lines, statsLine)

	return strings.Join(lines, "\n")
}

// ─── Recent Activity Panel ───────────────────────────────────────────

// activityContent returns the inner content for the RECENT ACTIVITY panel.
func (d *Dashboard) activityContent(innerW int) string {
	var lines []string

	if len(d.activity) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Slate500).
			Render("No recent activity"))
	} else {
		limit := len(d.activity)
		if limit > maxActivityEntries {
			limit = maxActivityEntries
		}
		for _, entry := range d.activity[:limit] {
			timeStr := lipgloss.NewStyle().Foreground(theme.Slate500).Render(entry.Time)

			var kindBadge string
			switch strings.ToLower(entry.Kind) {
			case "tx":
				kindBadge = lipgloss.NewStyle().Foreground(theme.GreenColor).Bold(true).Render("TX ")
			case "evt":
				kindBadge = lipgloss.NewStyle().Foreground(theme.BlueColor).Bold(true).Render("EVT")
			case "gov":
				kindBadge = lipgloss.NewStyle().Foreground(theme.PurpleColor).Bold(true).Render("GOV")
			default:
				kindBadge = lipgloss.NewStyle().Foreground(theme.Slate400).Render(fmt.Sprintf("%-3s", entry.Kind))
			}

			text := lipgloss.NewStyle().Foreground(theme.Slate300).Render(entry.Text)
			lines = append(lines, timeStr+"  "+kindBadge+"  "+text)
		}
	}

	return strings.Join(lines, "\n")
}

// ─── Shortcuts Panel ─────────────────────────────────────────────────

// shortcutsContent returns the inner content for the SHORTCUTS panel.
func (d *Dashboard) shortcutsContent() string {
	type shortcut struct {
		key   string
		desc  string
		isRed bool
	}

	shortcuts := []shortcut{
		{"1-6", "primary nav", false},
		{"↵", "drill down", false},
		{"esc", "pop back", false},
		{":", "command palette", false},
		{"?", "help overlay", false},
		{"D", "new deployment", true},
	}

	var lines []string
	for _, sc := range shortcuts {
		pill := keyPill(sc.key, sc.isRed)
		desc := lipgloss.NewStyle().Foreground(theme.Slate400).Render("  " + sc.desc)
		lines = append(lines, pill+desc)
	}

	return strings.Join(lines, "\n")
}

// ─── Helpers ─────────────────────────────────────────────────────────

// kvRight renders a right-justified key-value row: label on the left (muted),
// value on the right, filling the gap with spaces.
func kvRight(label, value string, width int) string {
	labelRendered := theme.Muted.Render(label)
	labelW := lipgloss.Width(labelRendered)
	valueW := lipgloss.Width(value)
	gap := width - labelW - valueW
	if gap < 1 {
		gap = 1
	}
	return labelRendered + strings.Repeat(" ", gap) + value
}

// keyPill renders a keyboard shortcut pill like [key].
func keyPill(key string, accent bool) string {
	if accent {
		return lipgloss.NewStyle().
			Foreground(theme.Slate950).
			Background(theme.AccentRed).
			Bold(true).
			Padding(0, 0).
			Render(key)
	}
	return lipgloss.NewStyle().
		Foreground(theme.Slate300).
		Background(theme.Slate800).
		Padding(0, 0).
		Render(key)
}

// commaGroup formats an integer with comma-separated thousands.
func commaGroup(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
