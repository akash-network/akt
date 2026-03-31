package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"

	"pkg.akt.dev/akt/internal/top/consensus"
	"pkg.akt.dev/akt/internal/top/governance"
	"pkg.akt.dev/akt/internal/top/rpc"
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
	Showing     bool
	Provider    *rpc.Provider
	Nodes       []rpc.ProviderNodeWithGPU
	Loading     bool
	Error       error
	ScrollPos   int
	SelectedIdx int
}

// ProviderViewState holds the state for the providers tab
type ProviderViewState struct {
	Providers []rpc.Provider
	Versions  []string
	Selected  string
	ScrollPos int
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
	ActiveTab          Tab
	Monikers           map[string]string
	ScrollPos          int
	Providers          ProviderViewState
	GovernanceParams   *governance.AllParams
	GovernanceSelected int
	GovernanceScroll   int
	BlockHistory       []BlockRecord
	OverviewScroll     int
	SelectedBlock      int
	ExpandedBlock      int // -1=none, 0=current, 1+=history index
	ExpandedScroll     int
	ExpandedValidators []BlockValidatorVote // frozen snapshot
	ValSignHistory     map[int][]bool       // per-validator signing history
	WSConnected        bool
}

// RenderView renders the complete view
func RenderView(ctx ViewContext) string {
	w := ctx.Width
	if w < 40 {
		w = 40
	}

	var b strings.Builder

	// Title bar — centered, full width
	title := titleStyle.Width(w).Align(lipgloss.Center).Render("akt top - Akash Network Monitor")
	b.WriteString(title)
	b.WriteString("\n")

	// Tab bar — full width
	b.WriteString(renderTabBar(ctx.ActiveTab, w))
	b.WriteString("\n\n")

	// Error state
	if ctx.State == nil {
		b.WriteString(errorStyle.Render("Connecting to RPC endpoint..."))
		b.WriteString("\n")
		b.WriteString(renderStatusBar(ctx.Endpoint, ctx.ActiveTab, ctx.Providers.Detail.Showing, ctx.WSConnected, w))
		return b.String()
	}

	if ctx.State.Error != nil {
		b.WriteString(errorStyle.Width(w).Render(fmt.Sprintf("Error: %v", ctx.State.Error)))
		b.WriteString("\n\n")
		// Still show last known state if available
		if ctx.State.Height == 0 {
			b.WriteString(renderStatusBar(ctx.Endpoint, ctx.ActiveTab, ctx.Providers.Detail.Showing, ctx.WSConnected, w))
			return b.String()
		}
	}

	// Render based on active tab
	switch ctx.ActiveTab {
	case TabOverview:
		b.WriteString(renderOverviewTab(ctx))
	case TabValidators:
		b.WriteString(renderValidatorsTab(ctx))
	case TabProviders:
		if ctx.Providers.Detail.Showing {
			b.WriteString(renderProviderDetailView(ctx.Providers.Detail, ctx.Height))
		} else {
			b.WriteString(renderProvidersTab(ctx.Providers, ctx.Height))
		}
	case TabGovernance:
		b.WriteString(renderGovernanceTab(ctx.GovernanceParams, ctx.Height, ctx.GovernanceSelected, ctx.GovernanceScroll))
	}

	b.WriteString("\n")

	// Help & status — full width
	b.WriteString(renderStatusBar(ctx.Endpoint, ctx.ActiveTab, ctx.Providers.Detail.Showing, ctx.WSConnected, w))

	return b.String()
}

// renderTabBar renders the tab navigation bar spanning full width
func renderTabBar(activeTab Tab, width int) string {
	tabs := []struct {
		label  string
		active bool
	}{
		{"1: Overview", activeTab == TabOverview},
		{"2: Validators", activeTab == TabValidators},
		{"3: Providers", activeTab == TabProviders},
		{"4: Governance", activeTab == TabGovernance},
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

	w := ctx.Width
	termHeight := ctx.Height
	scrollPos := ctx.ScrollPos
	monikers := ctx.Monikers
	signHistory := ctx.ValSignHistory

	// Column widths: proposer(2) + #(5) + name(fixed max 28) + power(10) + blocks(rest)
	// Name column has a max width; blocks column gets all remaining space.
	nameW := 28
	fixedCols := 2 + 5 + nameW + 10 + 5 // proposer + # + name + power + gaps
	blocksW := w - fixedCols
	if blocksW < 20 {
		blocksW = 20
	}

	header := headerStyle.Width(w).Render(
		fmt.Sprintf("Validators (%d) — Block Signing History", len(state.Validators)))

	colHeader := fmt.Sprintf("%s %s %s %s %s",
		mutedStyle.Render(fmt.Sprintf("%-2s", "")),
		mutedStyle.Render(fmt.Sprintf("%-5s", "#")),
		mutedStyle.Render(fmt.Sprintf("%-*s", nameW, "Validator")),
		mutedStyle.Render(fmt.Sprintf("%10s", "Power")),
		mutedStyle.Render(fmt.Sprintf(" %-*s", blocksW, "Blocks (newest \u2190)")))

	// Overhead: title(2) + tabs(1) + blank(1) + section header(3) +
	// col header(1) + scroll indicator(1) + status bar(3) = 12
	visibleRows := max(termHeight-12, 5)
	startIdx, endIdx := scrollRange(scrollPos, visibleRows, len(state.Validators))

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(colHeader)
	b.WriteString("\n")

	for i := startIdx; i < endIdx; i++ {
		v := state.Validators[i]
		b.WriteString(renderValidatorRowWithBlocks(v, monikers, signHistory, nameW, blocksW))
		b.WriteString("\n")
	}

	if len(state.Validators) > visibleRows {
		b.WriteString(mutedStyle.Render(fmt.Sprintf(
			"  Showing %d-%d of %d (j/k to scroll)", startIdx+1, endIdx, len(state.Validators))))
	}

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
// column header, pinned current block, status bar, and padding.
const overviewOverhead = 10

// Fixed column widths for the block table.
const (
	colHeight  = 14
	colPV      = 8
	colPC      = 8
	colElapsed = 9
	colRS      = 6
)

func renderBlockProgress(ctx ViewContext) string {
	state := ctx.State
	history := ctx.BlockHistory
	scrollPos := ctx.OverviewScroll
	termWidth := ctx.Width
	termHeight := ctx.Height

	header := headerStyle.Width(termWidth).Render("Block Progress")

	// Column header — pad raw text first, then style.
	colHeader := fmt.Sprintf("  %s  %s  %s  %s  %s",
		mutedStyle.Render(fmt.Sprintf("%-*s", colHeight, "Height")),
		mutedStyle.Render(fmt.Sprintf("%*s", colPV, "PV")),
		mutedStyle.Render(fmt.Sprintf("%*s", colPC, "PC")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colElapsed, "Elapsed")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colRS, "R/S")))

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(colHeader)
	b.WriteString("\n")

	// Current (live) block — always pinned at the top.
	if state != nil && state.Height > 0 {
		elapsed := state.Elapsed
		if elapsed < 0 {
			elapsed = 0
		}
		isSelected := ctx.SelectedBlock == 0
		b.WriteString(renderBlockRow(blockRowData{
			height:     state.Height,
			prevotePct: state.PrevotePercent,
			precommPct: state.PrecommitPercent,
			elapsed:    elapsed,
			round:      state.Round,
			step:       state.Step,
			isCurrent:  true,
			isSelected: isSelected,
			heightW:    colHeight,
		}))
		b.WriteString("\n")

		// Render expanded validator list for current block.
		if ctx.ExpandedBlock == 0 && len(ctx.ExpandedValidators) > 0 {
			b.WriteString(renderExpandedValidators(ctx.ExpandedValidators, ctx.Monikers, termHeight-overviewOverhead, ctx.ExpandedScroll, termWidth))
		}
	}

	if len(history) == 0 {
		b.WriteString(mutedStyle.Render("  Waiting for completed blocks..."))
		return b.String()
	}

	// How many history rows fit below the pinned current block.
	visibleRows := max(termHeight-overviewOverhead, 3)
	if ctx.ExpandedBlock >= 0 {
		// When a block is expanded, reduce visible block rows.
		visibleRows = max(visibleRows/3, 2)
	}

	startIdx, endIdx := scrollRange(scrollPos, visibleRows, len(history))

	for i := startIdx; i < endIdx; i++ {
		rec := history[i]
		// selectedBlock: 0=current, 1+=history. History index i maps to selectedBlock i+1.
		isSelected := ctx.SelectedBlock == i+1
		b.WriteString(renderBlockRow(blockRowData{
			height:     rec.Height,
			prevotePct: rec.PrevotePercent,
			precommPct: rec.PrecommitPercent,
			elapsed:    rec.Elapsed,
			round:      rec.Round,
			step:       rec.Step,
			isCurrent:  false,
			isSelected: isSelected,
			heightW:    colHeight,
		}))
		b.WriteString("\n")

		// Render expanded validator list for this history block.
		if ctx.ExpandedBlock == i+1 && len(ctx.ExpandedValidators) > 0 {
			b.WriteString(renderExpandedValidators(ctx.ExpandedValidators, ctx.Monikers, termHeight-overviewOverhead, ctx.ExpandedScroll, termWidth))
		}
	}

	// Scroll indicator when there are more blocks than visible.
	if len(history) > visibleRows {
		b.WriteString(mutedStyle.Render(fmt.Sprintf(
			"  Showing %d-%d of %d blocks (j/k to scroll, Enter to expand)", startIdx+1, endIdx, len(history))))
	}

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

	// Compute name column width: total width minus indent(6) + power(11) + pv(4) + pc(4) + gaps(8)
	nameW := termWidth - 6 - 11 - 4 - 4 - 8
	if nameW < 16 {
		nameW = 16
	}

	var b strings.Builder

	// Header
	valHeader := fmt.Sprintf("      %s  %s  %s  %s",
		mutedStyle.Render(fmt.Sprintf("%-*s", nameW, "Validator")),
		mutedStyle.Render(fmt.Sprintf("%11s", "Power")),
		mutedStyle.Render(fmt.Sprintf("%4s", "PV")),
		mutedStyle.Render(fmt.Sprintf("%4s", "PC")))
	b.WriteString(valHeader)
	b.WriteString("\n")

	visibleRows := max(maxRows-4, 5)
	startIdx, endIdx := scrollRange(scrollPos, visibleRows, len(sorted))

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

		pvIcon := voteNoStyle.Render("\uf00d") // nf-fa-times
		if v.Prevoted {
			pvIcon = voteYesStyle.Render("\uf00c") // nf-fa-check
		}
		pcIcon := voteNoStyle.Render("\uf00d")
		if v.Precommited {
			pcIcon = voteYesStyle.Render("\uf00c")
		}

		power := formatPower(v.VotingPower)

		line := fmt.Sprintf("      %s  %s  %s  %s",
			monikerStyle.Render(fmt.Sprintf("%-*s", nameW, name)),
			mutedStyle.Render(fmt.Sprintf("%11s", power)),
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

	marker := "  "
	if d.isSelected {
		marker = proposerStyle.Render("\uf0da ") // nf-fa-caret_right
	} else if d.isCurrent {
		marker = valueStyle.Render("\uf111 ") // nf-fa-circle
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
	bar := ProgressBar(percent, progressBarWidth)
	pct := FormatPercent(percent)
	powerStr := fmt.Sprintf("(%s / %s)", formatPower(power), formatPower(totalPower))
	return fmt.Sprintf("%s %s %s %s", labelStyle.Render(label), bar, pct, mutedStyle.Render(powerStr))
}

func renderGridSection(state *consensus.State, termWidth int) string {
	header := headerStyle.Render(fmt.Sprintf("Validator Grid (%d validators)", state.TotalValidators))

	gridWidth := clamp(termWidth-10, 20, 100)
	grid := FormatVoteGrid(state.PrevoteBitArray, gridWidth)

	legend := fmt.Sprintf("%s voted  %s not voted",
		gridVotedStyle.Render("\uf00c"),    // nf-fa-check
		gridNotVotedStyle.Render("\uf00d")) // nf-fa-times

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

func renderValidatorRowWithBlocks(v consensus.ValidatorStatus, monikers map[string]string, signHistory map[int][]bool, nameW, blocksW int) string {
	displayName := getValidatorDisplayName(v, monikers)
	if len(displayName) > nameW {
		displayName = displayName[:nameW-3] + "..."
	}

	power := formatPower(v.VotingPower)

	proposerCol := fmt.Sprintf("%-2s", "")
	if v.IsProposer {
		proposerCol = proposerStyle.Render(fmt.Sprintf("%-2s", "\uf005")) // nf-fa-star
	}

	// Build the block signing bar (newest on left).
	hist := signHistory[v.Index]
	bar := renderSigningBar(hist, blocksW)

	return fmt.Sprintf("%s %s %s %s %s",
		proposerCol,
		mutedStyle.Render(fmt.Sprintf("%-5d", v.Index)),
		monikerStyle.Render(fmt.Sprintf("%-*s", nameW, displayName)),
		mutedStyle.Render(fmt.Sprintf("%10s", power)),
		bar)
}

// renderSigningBar renders a visual bar of block signing history.
// Green = signed, Red = missed. Newest block on the left.
// Batches consecutive same-state runs into single style calls for performance.
func renderSigningBar(history []bool, width int) string {
	if len(history) == 0 {
		return mutedStyle.Render(strings.Repeat("\u2581", width))
	}

	const (
		blockChar   = '\u2588' // full block
		emptyChar   = '\u2581' // lower one-eighth block
		stateSigned = 1
		stateMissed = 2
		stateEmpty  = 3
	)

	var b strings.Builder
	runState := 0
	runLen := 0

	flush := func() {
		if runLen == 0 {
			return
		}
		s := strings.Repeat(string(blockChar), runLen)
		switch runState {
		case stateSigned:
			b.WriteString(gridVotedStyle.Render(s))
		case stateMissed:
			b.WriteString(errorStyle.Render(s))
		case stateEmpty:
			b.WriteString(mutedStyle.Render(strings.Repeat(string(emptyChar), runLen)))
		}
		runLen = 0
	}

	for i := 0; i < width; i++ {
		var cur int
		if i < len(history) {
			if history[i] {
				cur = stateSigned
			} else {
				cur = stateMissed
			}
		} else {
			cur = stateEmpty
		}

		if cur != runState {
			flush()
			runState = cur
		}
		runLen++
	}
	flush()

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
	if voted {
		return voteYesStyle.Render("\uf00c") // nf-fa-check
	}
	return voteNoStyle.Render("\uf00d") // nf-fa-times
}

func scrollRange(scrollPos, visibleRows, totalItems int) (start, end int) {
	start = scrollPos
	end = scrollPos + visibleRows
	if end > totalItems {
		end = totalItems
	}
	return
}

func renderProvidersTab(pv ProviderViewState, termHeight int) string {
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
	b.WriteString(renderProviderList(pv.Providers, pv.Selected, termHeight, pv.ScrollPos, pv.Detail.SelectedIdx))

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

	var dots string
	marker := "  "
	if version == selectedVersion {
		dots = gridVotedStyle.Render(repeatChar('\uf111', numDots)) // nf-fa-circle
		marker = proposerStyle.Render("\uf0da ")                    // nf-fa-caret_right
	} else {
		dots = mutedStyle.Render(repeatChar('\uf10c', numDots)) // nf-fa-circle_o
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

func renderProviderList(providers []rpc.Provider, selectedVersion string, termHeight, scrollPos, selectedIdx int) string {
	filtered := filterNonLocalProviders(providers)
	if len(filtered) == 0 {
		return mutedStyle.Render("No providers found")
	}

	visibleRows := max(termHeight-providerListOverhead, 5)
	if len(filtered) > visibleRows {
		visibleRows -= 2
	}

	matchCount := countVersionMatches(filtered, selectedVersion)
	header := headerStyle.Render(fmt.Sprintf("Providers (%d total, %d on %s)", len(filtered), matchCount, selectedVersion))

	colHeader := fmt.Sprintf("    %s  %s  %s  %s  %s  %s  %s",
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthIndex, "#")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthProvider, "Provider")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthVersion, "Version")),
		mutedStyle.Render(fmt.Sprintf("%*s", colWidthCPU, "CPU")),
		mutedStyle.Render(fmt.Sprintf("%*s", colWidthMem, "Memory")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthGPU, "GPU")),
		mutedStyle.Render(fmt.Sprintf("%-*s", colWidthCountry, "Loc")))

	var lines []string
	lines = append(lines, colHeader)

	startIdx, endIdx := scrollRange(scrollPos, visibleRows, len(filtered))

	for i := startIdx; i < endIdx; i++ {
		isRowSelected := i == selectedIdx
		lines = append(lines, renderProviderRow(filtered[i], i+1, selectedVersion, isRowSelected))
	}

	if len(filtered) > visibleRows {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf(
			"Showing %d-%d of %d (↑/↓ or j/k to scroll, Enter for details)", startIdx+1, endIdx, len(filtered))))
	}

	return header + "\n" + strings.Join(lines, "\n")
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
		cursor = proposerStyle.Render("\uf0da ") // nf-fa-caret_right
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
	if isSelected {
		return gridVotedStyle.Render("\uf111 ") // nf-fa-circle
	}
	return gridNotVotedStyle.Render("\uf10c ") // nf-fa-circle_o
}

// renderProviderDetailView renders the provider detail view with node list
func renderProviderDetailView(state ProviderDetailState, termHeight int) string {
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

	// Node table header
	nodeHeader := fmt.Sprintf("  %s  %s  %s  %s",
		mutedStyle.Render(fmt.Sprintf("%-20s", "Name")),
		mutedStyle.Render(fmt.Sprintf("%14s", "CPU")),
		mutedStyle.Render(fmt.Sprintf("%16s", "Memory")),
		mutedStyle.Render(fmt.Sprintf("%-30s", "GPU")))
	b.WriteString(nodeHeader)
	b.WriteString("\n")

	// Calculate visible rows for nodes
	visibleRows := max(termHeight-nodeListOverhead, minVisibleNodes)

	startIdx := state.ScrollPos
	endIdx := min(startIdx+visibleRows, len(state.Nodes))

	for i := startIdx; i < endIdx; i++ {
		node := state.Nodes[i]
		cpuNodeStr := formatResourceRatio(node.CPUAvailable/1000, node.CPUAllocatable/1000)
		memNodeStr := formatMemoryRatio(node.MemAvailable, node.MemAllocatable)

		nodeName := node.Name
		if nodeName == "" {
			nodeName = fmt.Sprintf("node-%d", i+1)
		}
		if len(nodeName) > colWidthNodeName {
			nodeName = nodeName[:colWidthNodeName-3] + "..."
		}

		// Format GPU info
		gpuStr := formatNodeGPU(node)

		line := fmt.Sprintf("  %s  %s  %s  %s",
			monikerStyle.Render(fmt.Sprintf("%-*s", colWidthNodeName, nodeName)),
			detailValueStyle.Render(fmt.Sprintf("%14s", cpuNodeStr)),
			detailValueStyle.Render(fmt.Sprintf("%16s", memNodeStr)),
			formatGPUDisplay(gpuStr, node.GPUAllocatable > 0))
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(state.Nodes) > visibleRows {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Showing %d-%d of %d nodes", startIdx+1, endIdx, len(state.Nodes))))
	}

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
func renderStatusBar(endpoint string, activeTab Tab, showingDetail bool, wsConnected bool, width int) string {
	var helpText string
	switch activeTab {
	case TabValidators:
		helpText = "q: quit | r: refresh | Tab/1-4: switch tabs | j/k: scroll"
	case TabGovernance:
		helpText = "q: quit | r: refresh params | Tab/1-4: switch tabs"
	case TabProviders:
		if showingDetail {
			helpText = "Esc: back | j/k: scroll nodes | q: quit"
		} else {
			helpText = "q: quit | r: refresh | Tab/1-4: switch | h/l: version | j/k: scroll | Enter: details"
		}
	default:
		helpText = "q: quit | Tab/1-4: switch | j/k: select | Enter: expand | Esc: collapse"
	}
	help := helpStyle.Width(width).Render(helpText)

	mode := "HTTP"
	if wsConnected {
		mode = "WS"
	}
	status := statusBarStyle.Width(width).Render(fmt.Sprintf("RPC: %s [%s]", endpoint, mode))
	return help + "\n" + status
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%.0fs", int(d.Minutes()), d.Seconds()-float64(int(d.Minutes()))*60)
}

// formatNumber formats a number with thousand separators
func formatNumber(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

// formatPower formats voting power in a compact way
func formatPower(power int64) string {
	if power >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(power)/1_000_000_000)
	}
	if power >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(power)/1_000_000)
	}
	if power >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(power)/1_000)
	}
	return fmt.Sprintf("%d", power)
}

// formatResourceRatio formats available/total as "avail/total"
func formatResourceRatio(available, total uint64) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d", available, total)
}

// formatMemoryRatio formats memory available/total in human-readable format
func formatMemoryRatio(available, total uint64) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%s/%s", formatBytes(available), formatBytes(total))
}

// formatBytes formats bytes into Kubernetes-style binary units (Gi/Ti/Mi)
func formatBytes(bytes uint64) string {
	const (
		Mi = 1024 * 1024
		Gi = 1024 * Mi
		Ti = 1024 * Gi
	)
	if bytes >= Ti {
		return fmt.Sprintf("%.0fTi", float64(bytes)/float64(Ti))
	}
	if bytes >= Gi {
		return fmt.Sprintf("%.0fGi", float64(bytes)/float64(Gi))
	}
	return fmt.Sprintf("%dMi", bytes/Mi)
}

func renderGovernanceTab(params *governance.AllParams, height int, selectedModule int, scrollPos int) string {
	var b strings.Builder

	if params == nil {
		b.WriteString(errorStyle.Render("Loading governance parameters..."))
		return b.String()
	}

	b.WriteString(headerStyle.Render("Governance Parameters") + "\n")
	b.WriteString(mutedStyle.Render("j/k: select module, h/l: scroll params") + "\n\n")

	moduleList := governance.ModuleOrder
	moduleColWidth := 20
	visibleRows := height - 3
	if visibleRows < 5 {
		visibleRows = 5
	}

	// Calculate which modules are visible (center selection)
	startModule := 0
	if len(moduleList) > visibleRows-1 {
		startModule = selectedModule - (visibleRows-1)/2
		if startModule < 0 {
			startModule = 0
		}
		if startModule+(visibleRows-1) > len(moduleList) {
			startModule = len(moduleList) - (visibleRows - 1)
		}
	}

	// Get parameters for selected module
	var paramLines []string
	if selectedModule >= 0 && selectedModule < len(moduleList) {
		module := moduleList[selectedModule]
		modParams := params.Modules[module]
		if modParams != nil && modParams.Error == nil {
			jsonStr, _ := governance.FormatJSON(modParams.RawJSON)
			paramLines = strings.Split(jsonStr, "\n")
		}
	}

	// Apply scroll to parameters
	if scrollPos < 0 {
		scrollPos = 0
	}
	if len(paramLines) > 0 && scrollPos >= len(paramLines) {
		scrollPos = len(paramLines) - 1
	}

	// Render each row
	for row := 0; row < visibleRows; row++ {
		moduleIdx := startModule + row
		leftCol := ""

		// Left column - module list
		if moduleIdx < len(moduleList) {
			module := moduleList[moduleIdx]
			displayName := governance.GetModuleDisplayName(module)
			modParams := params.Modules[module]

			if moduleIdx == selectedModule {
				leftCol = "\uf0da " + displayName // nf-fa-caret_right
			} else {
				leftCol = "  " + displayName
			}

			if modParams != nil && modParams.Error != nil {
				leftCol += " (err)"
			}

			// Pad to column width
			if len(leftCol) < moduleColWidth {
				leftCol += strings.Repeat(" ", moduleColWidth-len(leftCol))
			}
		} else {
			leftCol = strings.Repeat(" ", moduleColWidth)
		}

		// Right column - ALL parameters (scrolled from top)
		rightCol := ""
		paramLineIdx := row + scrollPos

		if selectedModule >= 0 && selectedModule < len(moduleList) {
			modParams := params.Modules[moduleList[selectedModule]]

			if modParams == nil {
				rightCol = "(no data)"
			} else if modParams.Error != nil {
				rightCol = fmt.Sprintf("Error: %v", modParams.Error)
			} else if paramLineIdx >= 0 && paramLineIdx < len(paramLines) {
				rightCol = paramLines[paramLineIdx]
			}
		}

		b.WriteString(leftCol + rightCol + "\n")
	}

	// Add scroll indicator only when params don't fit and user has scrolled
	if len(paramLines) > visibleRows-3 && scrollPos > 0 {
		b.WriteString(strings.Repeat(" ", moduleColWidth))
		b.WriteString(fmt.Sprintf("[%d/%d lines]", scrollPos+visibleRows-2, len(paramLines)))
	}

	return b.String()
}
