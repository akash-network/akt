package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/glyphs"
	"pkg.akt.dev/akt/internal/output/pretty"

	"pkg.akt.dev/akt/internal/monitor/consensus"
	"pkg.akt.dev/akt/internal/monitor/governance"
	"pkg.akt.dev/akt/internal/monitor/rpc"
)

// Column width constants for provider list
const (
	colWidthIndex    = 4
	colWidthProvider = 36
	colWidthVersion  = 14
	colWidthCPU      = 12
	colWidthMem      = 12
	colWidthGPU      = 18
	colWidthCountry  = 4
	colWidthNodeName = 20
)

// Layout constants
const (
	providerListOverhead = 25 // header, tabs, status bar, scroll indicator, etc.
	nodeListOverhead     = 14 // header for detail view with node list
	minVisibleNodes      = 3  // minimum visible rows for node list
	minVisibleProviders  = 5  // minimum visible rows for provider list
)

// Address display constants
const (
	addrPrefixLen = 8 // characters to show at start of truncated address
	addrSuffixLen = 4 // characters to show at end of truncated address
)

// ProviderDetailState holds the state for provider detail view
type ProviderDetailState struct {
	Showing  bool
	Provider *rpc.Provider
	Nodes    []rpc.ProviderNodeWithGPU
	Loading  bool
	Error    error
}

// ProviderViewState holds the state for the providers tab
type ProviderViewState struct {
	Providers []rpc.Provider
	Versions  []string
	Selected  string
	Loading   bool
	Loaded    int
	Total     int
	Detail    ProviderDetailState
}

// ViewContext holds all data needed to render the view
type ViewContext struct {
	State              *consensus.State
	Endpoint           string
	Width              int
	Height             int
	HubTab             HubTab
	ActiveTab          Tab
	Embedded           bool // when true, skip the bottom help/status lines
	Monikers           map[string]string
	Providers          ProviderViewState
	GovernanceParams   *governance.AllParams
	BlockHistory       []BlockRecord
	ExpandedBlock      int // -1=none, 0=current, 1+=history index
	ExpandedScroll     int
	ExpandedValidators []BlockValidatorVote // frozen snapshot
	ExpandedValidator  int                  // expanded validator (-1=none)
	ValSignHistory     map[int][]bool       // per-validator signing history
	ProposerHistory    []int                // proposer index per historical block (newest first)
	CurrentProposer    int                  // current block's proposer index (-1=unknown)
	WSConnected        bool
	Oracle             OracleState

	// Bubbles component models
	ProviderTable  table.Model
	NodeTable      table.Model
	ValidatorTable table.Model
	BlockTable     table.Model
	GovModuleIdx    int // selected module index
	GovModuleScroll int // first visible module index
	GovModuleHeight int // visible rows for module list
	GovParamView    viewport.Model
}

// RenderView renders the complete view
func RenderView(ctx ViewContext) string {
	w := ctx.Width
	if w < 40 {
		w = 40
	}

	var b strings.Builder

	// Title bar — centered, full width
	title := titleStyle.Width(w).Align(lipgloss.Center).Render("akt monitor - Akash Network")
	b.WriteString(title)
	b.WriteString("\n")

	// Hub tab bar — shows Network / Provider / BME
	b.WriteString(renderHubTabBar(ctx.HubTab, w))
	b.WriteString("\n")

	// Sub-tab bar for the Network dashboard
	if ctx.HubTab == HubNetwork {
		b.WriteString(renderTabBar(ctx.ActiveTab, w))
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n")
	}

	// Dispatch to the active hub dashboard
	switch ctx.HubTab {
	case HubNetwork:
		b.WriteString(renderNetworkDashboard(ctx))
	case HubProvider:
		b.WriteString(renderProviderDashboard(ctx))
	case HubOracleBME:
		b.WriteString(renderOracleBMEDashboard(ctx))
	}

	b.WriteString("\n")

	// Help & status — full width (skipped when the parent TUI provides its own status bar)
	if !ctx.Embedded {
		b.WriteString(renderStatusBar(ctx.Endpoint, ctx.ActiveTab, ctx.Providers.Detail.Showing, ctx.WSConnected, w))
	}

	return b.String()
}

// renderNetworkDashboard renders the Network hub dashboard content.
func renderNetworkDashboard(ctx ViewContext) string {
	// Error state
	if ctx.State == nil {
		return errorStyle.Render("Connecting to RPC endpoint...")
	}

	var b strings.Builder

	if ctx.State.Error != nil {
		b.WriteString(errorStyle.Width(ctx.Width).Render(fmt.Sprintf("Error: %v", ctx.State.Error)))
		b.WriteString("\n\n")
		if ctx.State.Height == 0 {
			return b.String()
		}
	}

	switch ctx.ActiveTab {
	case TabOverview:
		b.WriteString(renderOverviewTab(ctx))
	case TabValidators:
		b.WriteString(renderValidatorsTab(ctx))
	case TabGovernance:
		b.WriteString(renderGovernanceTab(ctx))
	}

	return b.String()
}

// renderProviderDashboard renders the Provider hub dashboard content.
func renderProviderDashboard(ctx ViewContext) string {
	if ctx.Providers.Detail.Showing {
		return renderProviderDetailView(ctx)
	}
	return renderProvidersTab(ctx)
}

// renderOracleBMEDashboard renders the combined Oracle/BME dashboard as a
// two-column layout: Oracle on the left, BME on the right. Each panel
// delegates to the shared pretty.Render* functions so that the TUI output
// is visually identical to `akt q bme status --output pretty`.
func renderOracleBMEDashboard(ctx ViewContext) string {
	w := ctx.Width
	if w < 40 {
		w = 40
	}
	leftW := w / 2
	rightW := w - leftW

	left := renderOraclePanel(ctx, leftW)
	right := renderBMEPanel(ctx, rightW)

	leftStyled := lipgloss.NewStyle().Width(leftW).Render(left)
	rightStyled := lipgloss.NewStyle().Width(rightW).Render(right)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, rightStyled)
}

// renderOraclePanel renders the left column of the Oracle/BME dashboard.
func renderOraclePanel(ctx ViewContext, width int) string {
	var b strings.Builder

	if len(ctx.Oracle.Aggregated) == 0 {
		switch ctx.Oracle.Version {
		case "none":
			b.WriteString(mutedStyle.Render("  Oracle module not active on this network."))
		case "":
			b.WriteString(mutedStyle.Render("  Loading oracle data..."))
		default:
			b.WriteString(mutedStyle.Render(
				fmt.Sprintf("  Oracle %s detected — waiting for aggregated prices...", ctx.Oracle.Version)))
		}
		b.WriteString("\n")
		return b.String()
	}

	// Header
	b.WriteString(headerStyle.Width(width).Render("Oracle Prices"))
	b.WriteString("\n")

	// Sort denoms for stable display order.
	denoms := make([]string, 0, len(ctx.Oracle.Aggregated))
	for d := range ctx.Oracle.Aggregated {
		denoms = append(denoms, d)
	}
	sort.Strings(denoms)

	for _, denom := range denoms {
		ev := ctx.Oracle.Aggregated[denom]
		if ev == nil {
			continue
		}
		ap := ev.Price

		twap := pretty.TrimDecTrailingZeros(ap.TWAP.String())
		median := pretty.TrimDecTrailingZeros(ap.MedianPrice.String())
		minP := pretty.TrimDecTrailingZeros(ap.MinPrice.String())
		maxP := pretty.TrimDecTrailingZeros(ap.MaxPrice.String())

		b.WriteString(fmt.Sprintf("  %s\n", valueStyle.Render(ap.Denom)))
		b.WriteString(fmt.Sprintf("    %s %s\n",
			labelStyle.Render("TWAP:"),
			valueStyle.Render(twap)))
		b.WriteString(fmt.Sprintf("    %s %s\n",
			labelStyle.Render("Median:"),
			mutedStyle.Render(median)))
		b.WriteString(fmt.Sprintf("    %s %s  %s %s\n",
			labelStyle.Render("Min:"),
			mutedStyle.Render(minP),
			labelStyle.Render("Max:"),
			mutedStyle.Render(maxP)))
		b.WriteString(fmt.Sprintf("    %s %s  %s %d bps\n",
			labelStyle.Render("Sources:"),
			mutedStyle.Render(fmt.Sprintf("%d", ap.NumSources)),
			labelStyle.Render("Deviation:"),
			ap.DeviationBps))
		b.WriteString("\n")
	}

	return b.String()
}

// renderBMEPanel renders the right column of the Oracle/BME dashboard.
func renderBMEPanel(ctx ViewContext, width int) string {
	var b strings.Builder

	if ctx.Oracle.BMEStatus == nil {
		b.WriteString(mutedStyle.Render("  Loading BME status..."))
		b.WriteString("\n")
	} else {
		b.WriteString(pretty.RenderBMEStatus(ctx.Oracle.BMEStatus))
	}

	b.WriteString("\n")

	if len(ctx.Oracle.BMELedger) > 0 {
		b.WriteString(pretty.RenderBMELedger(ctx.Oracle.BMELedger))
	}

	return b.String()
}

// renderHubTabBar renders the hub-level dashboard tab bar.
func renderHubTabBar(active HubTab, width int) string {
	hubs := []struct {
		name string
		hub  HubTab
	}{
		{"Network", HubNetwork},
		{"Provider", HubProvider},
		{"Oracle/BME", HubOracleBME},
	}

	var tabs []string
	for _, h := range hubs {
		if h.hub == active {
			tabs = append(tabs, tabActiveStyle.Render(h.name))
		} else {
			tabs = append(tabs, tabInactiveStyle.Render(h.name))
		}
	}

	bar := strings.Join(tabs, "  ")
	return lipgloss.NewStyle().Width(width).Render(bar)
}

// renderTabBar renders the tab navigation bar spanning full width
func renderTabBar(activeTab Tab, width int) string {
	tabs := []struct {
		label  string
		active bool
	}{
		{"1: Overview", activeTab == TabOverview},
		{"2: Validators", activeTab == TabValidators},
		{"3: Governance", activeTab == TabGovernance},
	}

	// Compute per-tab width: distribute evenly across terminal width.
	tabWidth := (width - len(tabs) + 1) / len(tabs)
	if tabWidth < 12 {
		tabWidth = 12
	}

	var parts []string
	for _, t := range tabs {
		label := fmt.Sprintf(" %-*s", tabWidth-1, t.label)
		if t.active {
			parts = append(parts, tabActiveStyle.Render(label))
		} else {
			parts = append(parts, tabInactiveStyle.Render(label))
		}
	}

	return strings.Join(parts, "")
}

func renderOverviewTab(ctx ViewContext) string {
	return renderBlockProgress(ctx)
}

func renderValidatorsTab(ctx ViewContext) string {
	state := ctx.State
	if state == nil || len(state.Validators) == 0 {
		return mutedStyle.Render("Loading validators...")
	}

	// If a validator is expanded, show overlay detail panel instead of the table.
	if ctx.ExpandedValidator >= 0 && ctx.ExpandedValidator < len(state.Validators) {
		return renderValidatorDetailOverlay(ctx)
	}

	w := ctx.Width
	header := headerStyle.Width(w).Render(
		fmt.Sprintf("Validators (%d) — Block Signing History", len(state.Validators)))

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(ctx.ValidatorTable.View())
	b.WriteString("\n")

	return b.String()
}

// renderValidatorDetailOverlay renders a full overlay panel for the
// selected validator, replacing the validator table.
func renderValidatorDetailOverlay(ctx ViewContext) string {
	state := ctx.State
	v := state.Validators[ctx.ExpandedValidator]
	w := ctx.Width

	var b strings.Builder

	name := getValidatorDisplayName(v, ctx.Monikers)
	title := fmt.Sprintf("Validator: %s", name)
	b.WriteString(headerStyle.Width(w).Render(title))
	b.WriteString("\n")

	b.WriteString(renderValidatorDetailPanel(v, ctx.Monikers, ctx.ValSignHistory, state.TotalVotingPower, w))
	b.WriteString("\n")

	b.WriteString(mutedStyle.Render("  esc/h/\u2190: back"))
	b.WriteString("\n")

	return b.String()
}

func renderConsensusSection(state *consensus.State) string {
	header := headerStyle.Render("Consensus State")

	elapsed := state.Elapsed
	if elapsed < 0 {
		elapsed = 0
	}

	proposerAddr := truncateAddress(state.ProposerAddress, 12)

	content := fmt.Sprintf(
		"%s %s  %s %s  %s %s\n%s %s  %s %s (index: %d)",
		labelStyle.Render("Height:"),
		valueStyle.Render(fmt.Sprintf("%-14s", formatNumber(state.Height))),
		labelStyle.Render("Round:"),
		valueStyle.Render(fmt.Sprintf("%-4d", state.Round)),
		labelStyle.Render("Step:"),
		valueStyle.Render(fmt.Sprintf("%d/%d", state.Round, state.Step)),
		labelStyle.Render("Elapsed:"),
		valueStyle.Render(fmt.Sprintf("%-14s", formatDuration(elapsed))),
		labelStyle.Render("Proposer:"),
		valueStyle.Render(proposerAddr),
		state.ProposerIndex,
	)

	return header + "\n" + content
}

func truncateAddress(addr string, maxLen int) string {
	if len(addr) <= maxLen {
		return addr
	}
	return addr[:addrPrefixLen] + "..." + addr[len(addr)-addrSuffixLen:]
}

// consensusThreshold is the 2/3 threshold for consensus.
const consensusThreshold = 0.667

// overviewOverhead is the number of lines consumed by title, tabs, header,
// progress bars, and margins above the block table.
// title(1) + hub tabs(1) + sub tabs(1) + blank(1) + section header(3) +
// progress bars(2) + blank(1) = 10
const overviewOverhead = 10

// governanceOverhead is the number of lines consumed by chrome around
// the governance module list/param view.
// title(1) + hub tabs(1) + sub tabs(1) + blank after tabs(1) +
// headerStyle "Governance Parameters" with border+padding+margin(4) +
// help text(1) + blank after help(1) +
// newline after dashboard(1) + status bar help(1) + status bar RPC(1) = 13
const governanceOverhead = 13

// Fixed column widths for the block table.
const (
	colHeight  = 14
	colPV      = 8
	colPC      = 8
	colElapsed = 9
	colRS      = 6
)

func renderBlockProgress(ctx ViewContext) string {
	// If a block is expanded, show overlay detail panel instead of the table.
	if ctx.ExpandedBlock >= 0 && len(ctx.ExpandedValidators) > 0 {
		return renderBlockDetailOverlay(ctx)
	}

	state := ctx.State
	termWidth := ctx.Width

	header := headerStyle.Width(termWidth).Render("Block Progress")

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")

	// Double progress bar: top half = prevotes (green), bottom half = precommits (cyan).
	if state != nil && state.Height > 0 {
		// Bar width stretches to fill the terminal minus margin and label space.
		// Label: "  PV xxx.x%  PC xxx.x%" = ~24 chars. Left margin = 2.
		labelSpace := 24
		margin := 2
		barW := termWidth - margin - labelSpace
		if barW < 20 {
			barW = 20
		}

		bars := strings.SplitN(DoubleProgressBar(state.PrevotePercent, state.PrecommitPercent, barW), "\n", 2)
		pvLabel := FormatPercent(state.PrevotePercent)
		pcLabel := FormatPercent(state.PrecommitPercent)
		b.WriteString(fmt.Sprintf("  %s  PV%s", bars[0], pvLabel))
		b.WriteString("\n")
		if len(bars) > 1 {
			b.WriteString(fmt.Sprintf("  %s  PC%s", bars[1], pcLabel))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Block table — includes the current block as the first row.
	if state == nil || state.Height == 0 {
		b.WriteString(mutedStyle.Render("  Waiting for block data..."))
	} else {
		b.WriteString(ctx.BlockTable.View())
	}
	b.WriteString("\n")

	return b.String()
}

// renderBlockDetailOverlay renders a full overlay panel showing the
// validator vote list for the expanded block, replacing the block table.
func renderBlockDetailOverlay(ctx ViewContext) string {
	termWidth := ctx.Width
	termHeight := ctx.Height

	var b strings.Builder

	// Determine block info based on expanded index.
	var height int64
	var pvPct, pcPct float64
	var elapsed time.Duration
	var round, step int

	if ctx.ExpandedBlock == 0 && ctx.State != nil {
		height = ctx.State.Height
		pvPct = ctx.State.PrevotePercent
		pcPct = ctx.State.PrecommitPercent
		elapsed = ctx.State.Elapsed
		if elapsed < 0 {
			elapsed = 0
		}
		round = ctx.State.Round
		step = ctx.State.Step
	} else if ctx.ExpandedBlock > 0 {
		histIdx := ctx.ExpandedBlock - 1
		if histIdx < len(ctx.BlockHistory) {
			rec := ctx.BlockHistory[histIdx]
			height = rec.Height
			pvPct = rec.PrevotePercent
			pcPct = rec.PrecommitPercent
			elapsed = rec.Elapsed
			round = rec.Round
			step = rec.Step
		}
	}

	// Header
	title := fmt.Sprintf("Block %s — Validator Votes", formatNumber(height))
	b.WriteString(headerStyle.Width(termWidth).Render(title))
	b.WriteString("\n")

	// Block stats
	b.WriteString(fmt.Sprintf("  %s %s  %s %s  %s %s  %s %s\n",
		labelStyle.Render("PV:"), colorVotePercent(pvPct),
		labelStyle.Render("PC:"), colorVotePercent(pcPct),
		labelStyle.Render("Elapsed:"), valueStyle.Render(formatDuration(elapsed)),
		labelStyle.Render("R/S:"), valueStyle.Render(fmt.Sprintf("%d/%d", round, step))))

	b.WriteString("\n")

	// Validator vote list — use most of the terminal height.
	maxRows := termHeight - 6
	b.WriteString(renderExpandedValidators(ctx.ExpandedValidators, ctx.Monikers, maxRows, ctx.ExpandedScroll, termWidth))

	// Help text
	b.WriteString(mutedStyle.Render("  esc/h/\u2190: back"))
	b.WriteString("\n")

	return b.String()
}

// renderExpandedValidators renders the validator vote list for an expanded block,
// sorted by voting power descending. The name column stretches to fill width.
func renderExpandedValidators(votes []BlockValidatorVote, monikers map[string]string, maxRows, scrollPos, termWidth int) string {
	if len(votes) == 0 {
		return ""
	}

	// Sort by voting power descending.
	sorted := make([]BlockValidatorVote, len(votes))
	copy(sorted, votes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].VotingPower > sorted[j].VotingPower
	})

	// Compute total voting power for percentage display.
	var totalPower int64
	for _, v := range votes {
		totalPower += v.VotingPower
	}

	// Compute name column width: total width minus indent(6) + power(18) + pv(4) + pc(4) + gaps(8)
	powerW := 18
	nameW := termWidth - 6 - powerW - 4 - 4 - 8
	if nameW < 16 {
		nameW = 16
	}

	var b strings.Builder

	// Header
	valHeader := fmt.Sprintf("      %s  %s  %s  %s",
		mutedStyle.Render(fmt.Sprintf("%-*s", nameW, "Validator")),
		mutedStyle.Render(fmt.Sprintf("%*s", powerW, "Power")),
		mutedStyle.Render(fmt.Sprintf("%4s", "PV")),
		mutedStyle.Render(fmt.Sprintf("%4s", "PC")))
	b.WriteString(valHeader)
	b.WriteString("\n")

	visibleRows := max(maxRows-4, 5)
	startIdx := scrollPos
	endIdx := scrollPos + visibleRows
	if endIdx > len(sorted) {
		endIdx = len(sorted)
	}

	for i := startIdx; i < endIdx; i++ {
		v := sorted[i]
		name := ""
		if monikers != nil && v.PubKey != "" {
			name = stripEmojis(strings.TrimSpace(monikers[v.PubKey]))
		}
		if name == "" {
			name = truncateAddress(v.Address, 12)
		}
		if len(name) > nameW {
			name = name[:nameW-3] + "..."
		}

		g := glyphs.G()
		pvIcon := voteNoStyle.Render(g.VoteNo)
		if v.Prevoted {
			pvIcon = voteYesStyle.Render(g.VoteYes)
		}
		pcIcon := voteNoStyle.Render(g.VoteNo)
		if v.Precommited {
			pcIcon = voteYesStyle.Render(g.VoteYes)
		}

		power := formatPower(v.VotingPower)
		pct := ""
		if totalPower > 0 {
			pct = fmt.Sprintf("%.1f%%", float64(v.VotingPower)/float64(totalPower)*100)
		}
		powerCell := fmt.Sprintf("%s %s", power, pct)

		line := fmt.Sprintf("      %s  %s  %s  %s",
			monikerStyle.Render(fmt.Sprintf("%-*s", nameW, name)),
			mutedStyle.Render(fmt.Sprintf("%*s", powerW, powerCell)),
			centerAlign(pvIcon, 4),
			centerAlign(pcIcon, 4))
		b.WriteString(line)
		b.WriteString("\n")
	}

	if len(sorted) > visibleRows {
		b.WriteString(mutedStyle.Render(fmt.Sprintf(
			"      Showing %d-%d of %d validators (j/k to scroll)", startIdx+1, endIdx, len(sorted))))
		b.WriteString("\n")
	}

	return b.String()
}

type blockRowData struct {
	height     int64
	prevotePct float64
	precommPct float64
	elapsed    time.Duration
	round      int
	step       int
	isCurrent  bool
	isSelected bool
	heightW    int // column width for height field
}

func renderBlockRow(d blockRowData) string {
	hw := d.heightW
	if hw < 14 {
		hw = 14
	}
	// Height
	heightStr := formatNumber(d.height)
	if d.isCurrent {
		heightStr = valueStyle.Render(fmt.Sprintf("%-*s", hw, heightStr))
	} else {
		heightStr = mutedStyle.Render(fmt.Sprintf("%-*s", hw, heightStr))
	}

	// Prevote / Precommit — colored by consensus threshold
	pvStr := colorVotePercent(d.prevotePct)
	pcStr := colorVotePercent(d.precommPct)

	// Elapsed
	var elapsedStr string
	if d.elapsed > 0 || d.isCurrent {
		elapsedStr = fmt.Sprintf("%-*s", colElapsed, formatDuration(d.elapsed))
	} else {
		elapsedStr = fmt.Sprintf("%-*s", colElapsed, "-")
	}

	// Combined R/S (round/step)
	rsStr := fmt.Sprintf("%-*s", colRS, fmt.Sprintf("%d/%d", d.round, d.step))
	if d.isCurrent {
		if d.round > 0 {
			rsStr = proposerStyle.Render(rsStr) // highlight non-zero rounds
		} else {
			rsStr = valueStyle.Render(rsStr)
		}
	} else {
		if d.round > 0 {
			rsStr = proposerStyle.Render(rsStr)
		} else {
			rsStr = mutedStyle.Render(rsStr)
		}
	}

	g := glyphs.G()
	marker := "  "
	if d.isSelected {
		marker = proposerStyle.Render(g.Cursor + " ")
	} else if d.isCurrent {
		marker = valueStyle.Render(g.DotFilled + " ")
	}

	return fmt.Sprintf("%s%s  %s  %s  %s  %s",
		marker, heightStr, pvStr, pcStr, elapsedStr, rsStr)
}

// colorVotePercent renders a vote percentage with color based on the
// consensus threshold (2/3).  Red if below, green if at or above.
func colorVotePercent(pct float64) string {
	text := fmt.Sprintf("%*.1f%%", colPV-1, pct*100)
	if pct >= consensusThreshold {
		return percentHighStyle.Render(text)
	}
	return percentLowStyle.Render(text)
}

func renderVoteSection(state *consensus.State) string {
	header := headerStyle.Render("Vote Progress")
	prevoteLine := renderVoteLine("Prevotes:", state.PrevotePercent, state.PrevotePower, state.TotalVotingPower)
	precommitLine := renderVoteLine("Precommits:", state.PrecommitPercent, state.PrecommitPower, state.TotalVotingPower)
	return header + "\n" + prevoteLine + "\n" + precommitLine
}

func renderVoteLine(label string, percent float64, power, totalPower int64) string {
	bar := ProgressBar(percent, 40)
	pct := FormatPercent(percent)
	powerStr := fmt.Sprintf("(%s / %s)", formatPower(power), formatPower(totalPower))
	return fmt.Sprintf("%s %s %s %s", labelStyle.Render(label), bar, pct, mutedStyle.Render(powerStr))
}

func renderGridSection(state *consensus.State, termWidth int) string {
	header := headerStyle.Render(fmt.Sprintf("Validator Grid (%d validators)", state.TotalValidators))

	gridWidth := clamp(termWidth-10, 20, 100)
	grid := FormatVoteGrid(state.PrevoteBitArray, gridWidth)

	legend := fmt.Sprintf("%s voted  %s not voted",
		gridVotedStyle.Render(glyphs.G().VoteYes),
		gridNotVotedStyle.Render(glyphs.G().VoteNo))

	return header + "\n" + grid + "\n\n" + mutedStyle.Render(legend)
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// renderValidatorRowWithSelection renders a validator row with an optional selection cursor.
func renderValidatorRowWithSelection(v consensus.ValidatorStatus, monikers map[string]string, signHistory map[int][]bool, proposerHistory []int, totalPower int64, nameW, blocksW int, isSelected bool) string {
	return renderValidatorRowWithBlocks(v, monikers, signHistory, proposerHistory, totalPower, nameW, blocksW, isSelected)
}

func renderValidatorRowWithBlocks(v consensus.ValidatorStatus, monikers map[string]string, signHistory map[int][]bool, proposerHistory []int, totalPower int64, nameW, blocksW int, isSelected bool) string {
	displayName := getValidatorDisplayName(v, monikers)
	if len(displayName) > nameW {
		displayName = displayName[:nameW-3] + "..."
	}

	power := formatPower(v.VotingPower)
	pct := ""
	if totalPower > 0 {
		pct = fmt.Sprintf("%.1f%%", float64(v.VotingPower)/float64(totalPower)*100)
	}
	powerCell := fmt.Sprintf("%s %s", power, pct)

	leadCol := fmt.Sprintf("%-2s", "")
	if isSelected {
		leadCol = proposerStyle.Render(fmt.Sprintf("%-2s", glyphs.G().Cursor))
	}

	// Build the block signing bar (newest on left).
	hist := signHistory[v.Index]
	bar := renderSigningBar(hist, v.Index, proposerHistory, -1, blocksW)

	return fmt.Sprintf("%s %s %s %s   %s",
		leadCol,
		mutedStyle.Render(fmt.Sprintf("%-5d", v.Index)),
		monikerStyle.Render(fmt.Sprintf("%-*s", nameW, displayName)),
		mutedStyle.Render(fmt.Sprintf("%18s", powerCell)),
		bar)
}

// renderValidatorDetailPanel renders a detail panel for an expanded validator.
func renderValidatorDetailPanel(v consensus.ValidatorStatus, monikers map[string]string, signHistory map[int][]bool, totalPower int64, width int) string {
	var b strings.Builder

	// Divider
	b.WriteString(mutedStyle.Render(strings.Repeat("─", min(width, 60))))
	b.WriteString("\n")

	// Moniker / name
	name := getValidatorDisplayName(v, monikers)
	b.WriteString(fmt.Sprintf("  %s %s\n",
		labelStyle.Render("Validator:"),
		valueStyle.Render(name)))

	// Full address
	b.WriteString(fmt.Sprintf("  %s %s\n",
		labelStyle.Render("Address:"),
		mutedStyle.Render(v.Address)))

	// Consensus pubkey
	if v.PubKey != "" {
		pk := v.PubKey
		if len(pk) > 44 {
			pk = pk[:44] + "..."
		}
		b.WriteString(fmt.Sprintf("  %s %s\n",
			labelStyle.Render("PubKey:"),
			mutedStyle.Render(pk)))
	}

	// Voting power + percentage
	pct := ""
	if totalPower > 0 {
		pct = fmt.Sprintf(" (%.2f%%)", float64(v.VotingPower)/float64(totalPower)*100)
	}
	b.WriteString(fmt.Sprintf("  %s %s%s\n",
		labelStyle.Render("Power:"),
		valueStyle.Render(formatPower(v.VotingPower)),
		mutedStyle.Render(pct)))

	// Current block status
	g := glyphs.G()
	pvIcon := voteNoStyle.Render(g.VoteNo)
	if v.Prevoted {
		pvIcon = voteYesStyle.Render(g.VoteYes)
	}
	pcIcon := voteNoStyle.Render(g.VoteNo)
	if v.Precommited {
		pcIcon = voteYesStyle.Render(g.VoteYes)
	}
	b.WriteString(fmt.Sprintf("  %s prevote %s  precommit %s\n",
		labelStyle.Render("Current:"),
		pvIcon, pcIcon))

	// Signing history stats
	hist := signHistory[v.Index]
	if len(hist) > 0 {
		signed := 0
		for _, s := range hist {
			if s {
				signed++
			}
		}
		missed := len(hist) - signed
		pctSigned := float64(signed) / float64(len(hist)) * 100
		b.WriteString(fmt.Sprintf("  %s %s signed, %s missed out of %d blocks (%.1f%%)\n",
			labelStyle.Render("History:"),
			gridVotedStyle.Render(fmt.Sprintf("%d", signed)),
			errorStyle.Render(fmt.Sprintf("%d", missed)),
			len(hist),
			pctSigned))
	}

	// Proposer status
	if v.IsProposer {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			labelStyle.Render("Role:"),
			proposerStyle.Render("Current Proposer "+glyphs.G().Star)))
	}

	b.WriteString(mutedStyle.Render("  Esc: collapse"))

	return b.String()
}

// renderSigningBar renders a visual bar of block signing history.
// Green = signed, Red = missed. Newest block on the left.
// Uses ▄ (U+2584, lower half block) for vertical gaps between rows.
// When the validator was the proposer for a block, ★ is shown instead.
// Only blocks with actual history are rendered; future/empty slots are omitted.
//
// proposerHistory maps block index (0=newest) to the proposer's validator index.
// currentProposer is the proposer for the in-progress block (index 0 in the bar
// if the current block is shown, but for validators tab the bar starts at history).
func renderSigningBar(history []bool, validatorIdx int, proposerHistory []int, currentProposer int, width int) string {
	if len(history) == 0 {
		return ""
	}

	// ● (U+25CF, black circle) is vertically centred in the cell, 1 cell wide,
	// and creates natural horizontal/vertical gaps due to its round shape.
	// ★ (U+2605) is used for proposer blocks — also 1 cell wide and centred.
	const dotChar = "\u25cf"  // black circle
	const starChar = "\u2605" // black star

	numBlocks := len(history)
	if numBlocks > width {
		numBlocks = width
	}

	var b strings.Builder
	for i := 0; i < numBlocks; i++ {
		isProposer := false
		if i < len(proposerHistory) && proposerHistory[i] == validatorIdx {
			isProposer = true
		}

		switch {
		case isProposer:
			b.WriteString(proposerStyle.Render(starChar))
		case history[i]:
			b.WriteString(gridVotedStyle.Render(dotChar))
		default:
			b.WriteString(errorStyle.Render(dotChar))
		}
	}

	return b.String()
}

func centerAlign(s string, width int) string {
	// s is an ANSI-styled string; measure the raw content (1 char)
	raw := stripANSI(s)
	sw := len(raw)
	if sw >= width {
		return s
	}
	left := (width - sw) / 2
	right := width - sw - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func getValidatorDisplayName(v consensus.ValidatorStatus, monikers map[string]string) string {
	displayName := ""
	if monikers != nil && v.PubKey != "" {
		displayName = stripEmojis(strings.TrimSpace(monikers[v.PubKey]))
	}
	if displayName == "" {
		displayName = truncateAddress(v.Address, 12)
	}
	return displayName
}

// stripEmojis removes emoji and symbol characters, keeping only letters, numbers,
// punctuation, and basic whitespace.
func stripEmojis(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r > 0x7F && unicode.Is(unicode.So, r) {
			continue
		}
		if r == '\uFE0E' || r == '\uFE0F' {
			continue
		}
		b.WriteRune(r)
	}
	// Collapse multiple spaces from removed emojis
	return strings.Join(strings.Fields(b.String()), " ")
}

func voteIndicator(voted bool) string {
	g := glyphs.G()
	if voted {
		return voteYesStyle.Render(g.VoteYes)
	}
	return voteNoStyle.Render(g.VoteNo)
}

func renderProvidersTab(ctx ViewContext) string {
	pv := ctx.Providers
	var b strings.Builder

	if pv.Loading && pv.Total > 0 {
		progress := fmt.Sprintf("Scanning providers... %d/%d checked, %d online", pv.Loaded, pv.Total, len(pv.Providers))
		b.WriteString(ProgressBar(float64(pv.Loaded)/float64(pv.Total), 40))
		b.WriteString(" ")
		b.WriteString(mutedStyle.Render(progress))
		b.WriteString("\n\n")
	}

	b.WriteString(renderVersionDistribution(pv.Providers, pv.Versions, pv.Selected))
	b.WriteString("\n\n")

	filtered := filterNonLocalProviders(pv.Providers)
	if len(filtered) == 0 {
		b.WriteString(mutedStyle.Render("No providers found"))
		return b.String()
	}

	matchCount := countVersionMatches(filtered, pv.Selected)
	header := headerStyle.Render(fmt.Sprintf("Providers (%d total, %d on %s)", len(filtered), matchCount, pv.Selected))
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(ctx.ProviderTable.View())

	return b.String()
}

func renderVersionDistribution(providers []rpc.Provider, providerVersions []string, selectedVersion string) string {
	header := headerStyle.Render("Provider Version Distribution")

	if len(providers) == 0 {
		return header + "\n" + mutedStyle.Render("Loading providers...")
	}

	filtered := filterNonLocalProviders(providers)
	if len(filtered) == 0 {
		return header + "\n" + mutedStyle.Render("No providers found")
	}

	versionCounts := countByVersion(filtered)

	var lines []string
	for _, version := range providerVersions {
		lines = append(lines, renderVersionLine(version, versionCounts[version], len(filtered), selectedVersion))
	}

	help := mutedStyle.Render("← / → or h/l: select version")
	return header + "\n" + strings.Join(lines, "\n") + "\n\n" + help
}

func countByVersion(providers []rpc.Provider) map[string]int {
	counts := make(map[string]int)
	for _, p := range providers {
		counts[p.AkashVersion]++
	}
	return counts
}

func renderVersionLine(version string, count, total int, selectedVersion string) string {
	percentage := float64(count) / float64(total) * 100
	numDots := min(count, 50)
	g := glyphs.G()

	var dots string
	marker := "  "
	if version == selectedVersion {
		dots = gridVotedStyle.Render(strings.Repeat(g.DotFilled, numDots))
		marker = proposerStyle.Render(g.Cursor + " ")
	} else {
		dots = mutedStyle.Render(strings.Repeat(g.DotOpen, numDots))
	}

	return fmt.Sprintf("%s%-12s %s %3d (%5.1f%%)", marker, version, dots, count, percentage)
}

func isLocalhost(hostURI string) bool {
	return strings.Contains(hostURI, "localhost") || strings.Contains(hostURI, "127.0.0.1")
}

func filterNonLocalProviders(providers []rpc.Provider) []rpc.Provider {
	var filtered []rpc.Provider
	for _, p := range providers {
		if !isLocalhost(p.HostURI) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func countVersionMatches(providers []rpc.Provider, version string) int {
	count := 0
	for _, p := range providers {
		if p.AkashVersion == version {
			count++
		}
	}
	return count
}

func renderProviderRow(p rpc.Provider, index int, selectedVersion string, isRowSelected bool) string {
	displayURL := formatProviderURL(p.HostURI, colWidthProvider-2)

	isVersionMatch := p.AkashVersion == selectedVersion
	versionDisplay := formatVersionDisplay(p.AkashVersion, isVersionMatch)
	marker := versionMarker(isVersionMatch)

	country := p.Country
	if country == "" {
		country = "--"
	}

	cpuStr := formatResourceRatio(p.CPUAvailable/1000, p.CPUTotal/1000)
	memStr := formatMemoryRatio(p.MemAvailable, p.MemTotal)
	gpuStr := formatProviderGPU(p)

	// Selection cursor
	cursor := "  "
	if isRowSelected {
		cursor = proposerStyle.Render(glyphs.G().Cursor + " ")
	}

	indexStr := fmt.Sprintf("%-*d", colWidthIndex, index)
	urlStr := fmt.Sprintf("%-*s", colWidthProvider, displayURL)
	cpuFmt := fmt.Sprintf("%*s", colWidthCPU, cpuStr)
	memFmt := fmt.Sprintf("%*s", colWidthMem, memStr)

	if isRowSelected {
		// Highlight the entire row
		return fmt.Sprintf("%s%s%s  %s  %s  %s  %s  %s  %s",
			cursor,
			marker,
			highlightStyle.Render(indexStr),
			highlightStyle.Render(urlStr),
			versionDisplay,
			highlightStyle.Render(cpuFmt),
			highlightStyle.Render(memFmt),
			formatProviderGPUStyled(gpuStr, true),
			highlightStyle.Render(country))
	}

	return fmt.Sprintf("%s%s%s  %s  %s  %s  %s  %s  %s",
		cursor,
		marker,
		mutedStyle.Render(indexStr),
		monikerStyle.Render(urlStr),
		versionDisplay,
		mutedStyle.Render(cpuFmt),
		mutedStyle.Render(memFmt),
		formatProviderGPUStyled(gpuStr, false),
		mutedStyle.Render(country))
}

// formatProviderGPU formats GPU info for the provider list.
func formatProviderGPU(p rpc.Provider) string {
	if p.GPUTotal == 0 {
		return "-"
	}

	countStr := fmt.Sprintf("%d/%d", p.GPUAvailable, p.GPUTotal)

	// Add first model name if available
	if len(p.GPUModels) > 0 {
		model := p.GPUModels[0]
		// Truncate model name if needed
		maxModelLen := colWidthGPU - len(countStr) - 2
		if len(model) > maxModelLen && maxModelLen > 3 {
			model = model[:maxModelLen-2] + ".."
		}
		return fmt.Sprintf("%s %s", countStr, model)
	}

	return countStr
}

// formatProviderGPUStyled applies styling to GPU display in provider list.
func formatProviderGPUStyled(gpuStr string, isSelected bool) string {
	formatted := fmt.Sprintf("%-*s", colWidthGPU, gpuStr)
	if isSelected {
		return highlightStyle.Render(formatted)
	}
	return mutedStyle.Render(formatted)
}

func formatProviderURL(hostURI string, maxLen int) string {
	url := strings.TrimPrefix(hostURI, "https://")
	url = strings.TrimPrefix(url, "http://")
	if idx := strings.LastIndex(url, ":"); idx > 0 {
		url = url[:idx]
	}
	if len(url) > maxLen {
		url = url[:maxLen-3] + "..."
	}
	return url
}

func formatVersionDisplay(version string, isSelected bool) string {
	formatted := fmt.Sprintf("%-*s", colWidthVersion, version)
	if isSelected {
		return gridVotedStyle.Render(formatted)
	}
	return mutedStyle.Render(formatted)
}

func versionMarker(isSelected bool) string {
	g := glyphs.G()
	if isSelected {
		return gridVotedStyle.Render(g.DotFilled + " ")
	}
	return gridNotVotedStyle.Render(g.DotOpen + " ")
}

// renderProviderDetailView renders the provider detail view with node list
func renderProviderDetailView(ctx ViewContext) string {
	state := ctx.Providers.Detail
	var b strings.Builder

	if state.Provider == nil {
		return errorStyle.Render("No provider selected")
	}

	p := state.Provider

	// Header
	b.WriteString(detailHeaderStyle.Render("Provider Details"))
	b.WriteString("\n\n")

	// Provider info
	displayURL := formatProviderURL(p.HostURI, 50)
	b.WriteString(fmt.Sprintf("%s %s\n", detailLabelStyle.Render("Name:"), detailValueStyle.Render(p.Name)))
	b.WriteString(fmt.Sprintf("%s %s\n", detailLabelStyle.Render("URL:"), detailValueStyle.Render(displayURL)))
	b.WriteString(fmt.Sprintf("%s %s\n", detailLabelStyle.Render("Version:"), gridVotedStyle.Render(p.AkashVersion)))

	country := p.Country
	if country == "" {
		country = "--"
	}
	b.WriteString(fmt.Sprintf("%s %s\n", detailLabelStyle.Render("Location:"), detailValueStyle.Render(country)))

	// Total resources
	cpuStr := formatResourceRatio(p.CPUAvailable/1000, p.CPUTotal/1000)
	memStr := formatMemoryRatio(p.MemAvailable, p.MemTotal)
	b.WriteString(fmt.Sprintf("%s CPU %s | Memory %s\n", detailLabelStyle.Render("Total:"), detailValueStyle.Render(cpuStr), detailValueStyle.Render(memStr)))
	b.WriteString("\n")

	// Loading state
	if state.Loading {
		b.WriteString(mutedStyle.Render("Fetching node details via gRPC..."))
		return b.String()
	}

	// Error state
	if state.Error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", state.Error)))
		return b.String()
	}

	// Node list
	if len(state.Nodes) == 0 {
		b.WriteString(mutedStyle.Render("No node information available"))
		return b.String()
	}

	// Count total GPUs across all nodes
	totalGPUAvail, totalGPUTotal := uint64(0), uint64(0)
	for _, node := range state.Nodes {
		totalGPUAvail += node.GPUAvailable
		totalGPUTotal += node.GPUAllocatable
	}

	nodeHeaderText := fmt.Sprintf("Nodes (%d total)", len(state.Nodes))
	if totalGPUTotal > 0 {
		nodeHeaderText = fmt.Sprintf("Nodes (%d total, %d/%d GPUs avail)", len(state.Nodes), totalGPUAvail, totalGPUTotal)
	}
	b.WriteString(detailHeaderStyle.Render(nodeHeaderText))
	b.WriteString("\n")
	b.WriteString(ctx.NodeTable.View())

	return b.String()
}

// formatNodeGPU formats GPU information for a node.
func formatNodeGPU(node rpc.ProviderNodeWithGPU) string {
	if node.GPUAllocatable == 0 {
		return "-"
	}

	// Show GPU count and model info
	countStr := fmt.Sprintf("%d/%d", node.GPUAvailable, node.GPUAllocatable)

	if len(node.GPUs) == 0 {
		return countStr
	}

	// Get first GPU model info (typically nodes have homogeneous GPUs)
	gpu := node.GPUs[0]
	modelStr := formatGPUModel(gpu)

	return fmt.Sprintf("%s %s", countStr, modelStr)
}

// formatGPUModel formats a single GPU's model information.
func formatGPUModel(gpu rpc.GPUInfo) string {
	name := gpu.Name
	if name == "" {
		name = "Unknown"
	}

	// Shorten common vendor names
	vendor := gpu.Vendor
	switch vendor {
	case "nvidia":
		vendor = "NVIDIA"
	case "amd":
		vendor = "AMD"
	}

	result := name
	if vendor != "" && !strings.HasPrefix(strings.ToUpper(name), strings.ToUpper(vendor)) {
		result = vendor + " " + name
	}

	if gpu.MemorySize != "" {
		result += " (" + gpu.MemorySize + ")"
	}

	// Truncate if too long
	if len(result) > 28 {
		result = result[:25] + "..."
	}

	return result
}

// formatGPUDisplay applies styling to GPU display string.
func formatGPUDisplay(gpuStr string, hasGPU bool) string {
	if hasGPU {
		return gridVotedStyle.Render(gpuStr)
	}
	return mutedStyle.Render(gpuStr)
}

// renderStatusBar renders the bottom status bar
func renderStatusBar(endpoint string, activeTab Tab, _ bool, wsConnected bool, width int) string {
	var helpText string
	switch activeTab {
	case TabValidators:
		helpText = "q: quit | r: refresh | Tab/1-3: switch tabs | j/k: scroll"
	case TabGovernance:
		helpText = "q: quit | r: refresh params | Tab/1-3: switch tabs"
	default:
		helpText = "q: quit | Tab/1-3: switch | j/k: select | Enter: expand | Esc: collapse"
	}
	help := helpStyle.Width(width).Render(helpText)

	mode := "HTTP"
	if wsConnected {
		mode = "WS"
	}
	status := statusBarStyle.Width(width).Render(fmt.Sprintf("RPC: %s [%s]", endpoint, mode))
	return help + "\n" + status
}

// formatDuration delegates to the shared pretty.FormatShortDuration helper.
func formatDuration(d time.Duration) string {
	return pretty.FormatShortDuration(d)
}

// formatNumber delegates to the shared pretty.FormatNumber helper.
func formatNumber(n int64) string {
	return pretty.FormatNumber(n)
}

// formatPower delegates to the shared pretty.FormatPower helper.
func formatPower(power int64) string {
	return pretty.FormatPower(power)
}

// formatResourceRatio delegates to the shared pretty.FormatResourceRatio helper.
func formatResourceRatio(available, total uint64) string {
	return pretty.FormatResourceRatio(available, total)
}

// formatMemoryRatio delegates to the shared pretty.FormatMemoryRatio helper.
func formatMemoryRatio(available, total uint64) string {
	return pretty.FormatMemoryRatio(available, total)
}

// formatBytes delegates to the shared pretty.FormatBytes helper.
func formatBytes(bytes uint64) string {
	return pretty.FormatBytes(bytes)
}

func renderGovernanceTab(ctx ViewContext) string {
	params := ctx.GovernanceParams
	if params == nil {
		return errorStyle.Render("Loading governance parameters...")
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Governance Parameters") + "\n")
	b.WriteString(mutedStyle.Render("j/k: select module, h/l: scroll params") + "\n\n")

	// Two-column layout: module list on left, params on right.
	leftCol := renderGovModuleList(ctx.GovModuleIdx, ctx.GovModuleScroll, ctx.GovModuleHeight)
	rightCol := ctx.GovParamView.View()

	moduleColWidth := 22
	leftStyled := lipgloss.NewStyle().Width(moduleColWidth).Render(leftCol)
	rightStyled := lipgloss.NewStyle().Width(ctx.Width - moduleColWidth).Render(rightCol)

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, rightStyled))

	return b.String()
}

// renderGovModuleList renders the governance module selector as a simple
// scrolling list with a cursor indicator on the selected row.
// When the list is scrollable, a scroll indicator is shown on the last line.
func renderGovModuleList(selectedIdx, scrollOffset, visibleRows int) string {
	modules := governance.ModuleOrder
	total := len(modules)
	if visibleRows <= 0 {
		visibleRows = total
	}

	needsScroll := total > visibleRows
	// Reserve 1 row for the scroll indicator when the list overflows.
	itemRows := visibleRows
	if needsScroll {
		itemRows = visibleRows - 1
	}

	end := scrollOffset + itemRows
	if end > total {
		end = total
	}

	var b strings.Builder
	for i := scrollOffset; i < end; i++ {
		name := governance.GetModuleDisplayName(modules[i])
		if i == selectedIdx {
			b.WriteString(highlightStyle.Render(fmt.Sprintf("  %-18s", name)))
		} else {
			b.WriteString(fmt.Sprintf("  %-18s", name))
		}
		b.WriteString("\n")
	}

	if needsScroll {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  %d/%d", selectedIdx+1, total)))
	}

	return b.String()
}
