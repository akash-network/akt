package views

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/ui/theme"
)

// maxRecentDeployments is the number of deployments shown in the dashboard mini-table.
const maxRecentDeployments = 5

// Dashboard is the landing view that shows a summary of the user's Akash state.
type Dashboard struct {
	width  int
	height int

	// Data
	contextName string
	chainID     string
	account     string
	stats       *store.StoreStats
	syncState   *store.SyncState
	deployments []*store.DeploymentRecord // active only
	syncActive  bool                      // true when the sync bridge is running

	// New card data
	balance        string // formatted balance (e.g., "148.52 AKT")
	validatorCount int    // active validators
	proposalCount  int    // proposals in voting
}

// NewDashboard returns a new empty Dashboard.
func NewDashboard() Dashboard {
	return Dashboard{}
}

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

// View renders the dashboard.
func (d Dashboard) View() string {
	w := d.width
	if w < 20 {
		w = 80
	}

	var lines []string

	lines = append(lines, "")
	lines = append(lines, d.renderSummaryCards(w))
	lines = append(lines, "")
	lines = append(lines, d.renderRecentDeployments(w)...)
	lines = append(lines, "")
	lines = append(lines, d.renderNetwork(w)...)

	return strings.Join(lines, "\n")
}

// ─── Summary Cards ───────────────────────────────────────────────────

func (d Dashboard) renderSummaryCards(w int) string {
	cardW := (w - 6) / 3
	if cardW < 18 {
		cardW = 18
	}

	cardStyle := lipgloss.NewStyle().
		Width(cardW).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.Slate700).
		Padding(0, 1)

	// Balance card
	bal := d.balance
	if bal == "" {
		bal = "\u2014"
	}
	balanceCard := cardStyle.Render(
		theme.KVLabel.Render("Balance") + "\n" +
			theme.KVValueBold.Render(bal))

	// Deployments card
	var activeCount, totalCount string
	if d.stats != nil {
		activeCount = fmt.Sprintf("%d", d.stats.ActiveDeployments)
		totalCount = fmt.Sprintf("  %d total", d.stats.Deployments)
	} else {
		activeCount = "\u2014"
		totalCount = ""
	}
	deplCard := cardStyle.Render(
		theme.KVLabel.Render("Deployments") + "\n" +
			theme.KVValueBold.Render(activeCount) +
			theme.KVValue.Render(" active") +
			theme.KVValueMuted.Render(totalCount))

	// Leases card
	var leaseCount, leaseRate string
	if d.stats != nil {
		leaseCount = fmt.Sprintf("%d", d.stats.Leases)
	} else {
		leaseCount = "\u2014"
	}
	leaseRate = ""
	leaseCard := cardStyle.Render(
		theme.KVLabel.Render("Leases") + "\n" +
			theme.KVValueBold.Render(leaseCount) +
			theme.KVValue.Render(" active") +
			theme.KVValueMuted.Render(leaseRate))

	return lipgloss.JoinHorizontal(lipgloss.Top,
		" "+balanceCard, " ", deplCard, " ", leaseCard)
}

// ─── Recent Deployments Mini-Table ───────────────────────────────────

func (d Dashboard) renderRecentDeployments(w int) []string {
	var lines []string

	lines = append(lines, " "+theme.SectionTitle.Render("Recent Deployments"))
	lines = append(lines, " "+theme.HRule(w-2))

	if len(d.deployments) == 0 {
		lines = append(lines, "  "+theme.ColMuted.Render(
			"No deployments. Use 'akt deploy <sdl>' to create one."))
		return lines
	}

	// Column widths
	const (
		colDSeq   = 7
		colState  = 13
		colImage  = 20
		colPrice  = 12
		colEscrow = 10
	)

	// Header row
	lines = append(lines, "  "+
		col(theme.ColHeader, colDSeq, "DSEQ")+
		col(theme.ColHeader, colState, "STATE")+
		col(theme.ColHeader, colImage, "IMAGE")+
		col(theme.ColHeader, colPrice, "PRICE/BLK")+
		col(theme.ColHeader, colEscrow, "ESCROW")+
		theme.ColHeader.Render("AGE"))

	limit := len(d.deployments)
	if limit > maxRecentDeployments {
		limit = maxRecentDeployments
	}

	for _, dep := range d.deployments[:limit] {
		tag := components.StateTag(dep.State)
		tagW := components.StateTagWidth(dep.State)
		tagPad := ""
		if tagW < colState {
			tagPad = strings.Repeat(" ", colState-tagW)
		}

		image := "\u2014"
		if dep.SDLPath != "" {
			image = filepath.Base(dep.SDLPath)
		}

		price := "\u2014"
		if dep.Deposit != "" {
			price = dep.Deposit
		}

		escrow := "\u2014"
		if dep.EscrowBalance != "" {
			escrow = dep.EscrowBalance
		}

		age := "\u2014"
		if dep.CreatedAt > 0 {
			age = relativeTime(dep.CreatedAt)
		}

		row := "  " +
			col(theme.ColBold, colDSeq, fmt.Sprintf("%d", dep.DSeq)) +
			tag + tagPad +
			col(theme.Col, colImage, image) +
			col(theme.Col, colPrice, price) +
			col(theme.Col, colEscrow, escrow) +
			theme.ColMuted.Render(age)
		lines = append(lines, row)
	}

	lines = append(lines, "  "+theme.ColMuted.Render(
		fmt.Sprintf("Press 2 to see all %d deployments", len(d.deployments))))

	return lines
}

// ─── Network Status Strip ────────────────────────────────────────────

func (d Dashboard) renderNetwork(w int) []string {
	var lines []string

	lines = append(lines, " "+theme.SectionTitle.Render("Network"))
	lines = append(lines, " "+theme.HRule(w-2))

	chain := d.chainID
	if chain == "" {
		chain = "\u2014"
	}

	var blockHeight string
	if d.syncState != nil && d.syncState.LastBlockHeight > 0 {
		blockHeight = commaGroup(d.syncState.LastBlockHeight)
	} else {
		blockHeight = "\u2014"
	}

	validators := "\u2014"
	if d.validatorCount > 0 {
		validators = fmt.Sprintf("%d active", d.validatorCount)
	}

	proposals := "\u2014"
	if d.proposalCount > 0 {
		proposals = fmt.Sprintf("%d voting", d.proposalCount)
	}

	lines = append(lines,
		"  "+theme.KVLabel.Width(8).Render("Chain")+theme.KVValue.Render(chain)+
			"   "+theme.KVLabel.Width(8).Render("Height")+theme.KVValueBold.Render(blockHeight)+
			"   "+theme.KVLabel.Width(12).Render("Validators")+theme.KVValue.Render(validators)+
			"   "+theme.KVLabel.Width(12).Render("Proposals")+theme.KVValue.Render(proposals))

	return lines
}

// ─── Helpers ─────────────────────────────────────────────────────────

// col renders text into a fixed-width column using a lipgloss style.
func col(style lipgloss.Style, width int, text string) string {
	return style.Render(fmt.Sprintf("%-*s", width, text))
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
