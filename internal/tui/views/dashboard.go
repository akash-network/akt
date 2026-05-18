package views

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/ui/theme"
)

// maxActiveDeployments is the number of active deployments shown on the dashboard.
const maxActiveDeployments = 4

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

	// Section width for rules (leave 2 chars padding).
	sw := w - 2
	if sw < 10 {
		sw = 10
	}

	var sections []string

	sections = append(sections, d.renderWelcome(sw))
	sections = append(sections, d.renderAccount(sw))
	sections = append(sections, d.renderActiveDeployments(sw))
	sections = append(sections, d.renderNetwork(sw))
	sections = append(sections, d.renderShortcuts(sw))

	return strings.Join(sections, "\n\n")
}

// ─── Welcome Banner ──────────────────────────────────────────────────

func (d Dashboard) renderWelcome(w int) string {
	banner := lipgloss.NewStyle().Foreground(theme.AccentRed).Bold(true).Render(
		"  akt",
	)

	account := d.account
	if account == "" {
		account = "unknown"
	}
	welcome := theme.Heading.Render(fmt.Sprintf("welcome back, %s", account))

	// Context info line.
	ctx := d.contextName
	if ctx == "" {
		ctx = "\u2014" // em dash
	}
	chain := d.chainID
	if chain == "" {
		chain = "\u2014"
	}

	info := theme.Muted.Render("context ") +
		theme.Body.Render(ctx) +
		theme.Muted.Render(" \u00b7 chain ") +
		theme.Body.Render(chain)

	// Sync badge.
	var syncBadge string
	if d.syncState != nil {
		syncBadge = theme.SyncOK.Render("\u25cf synced")
	} else {
		syncBadge = theme.Muted.Render("\u25cb no sync")
	}

	return banner + "  " + welcome + "\n" +
		info + "  " + syncBadge
}

// ─── Account Panel ───────────────────────────────────────────────────

func (d Dashboard) renderAccount(w int) string {
	address := d.account
	if address == "" {
		address = "\u2014"
	}

	var totalDepls, activeDepls, closedDepls string
	if d.stats != nil {
		totalDepls = fmt.Sprintf("%d", d.stats.Deployments)
		activeDepls = fmt.Sprintf("%d", d.stats.ActiveDeployments)
		closedDepls = fmt.Sprintf("%d", d.stats.ClosedDeployments)
	} else {
		totalDepls = "\u2014"
		activeDepls = "\u2014"
		closedDepls = "\u2014"
	}

	var syncStatus string
	if d.syncState != nil {
		syncStatus = "synced"
	} else {
		syncStatus = "not synced"
	}

	pairs := []components.KVPair{
		{Label: "address", Value: address},
		{Label: "deployments", Value: totalDepls},
		{Label: "active", Value: activeDepls},
		{Label: "closed", Value: closedDepls},
		{Label: "sync status", Value: syncStatus},
	}

	return components.SectionWithKV("Account", w, pairs)
}

// ─── Active Deployments Panel ────────────────────────────────────────

func (d Dashboard) renderActiveDeployments(w int) string {
	heading := components.Section("Active Deployments", w)

	if len(d.deployments) == 0 {
		empty := theme.Muted.Render("No active deployments")
		return heading + "\n" + empty
	}

	var lines []string
	limit := len(d.deployments)
	if limit > maxActiveDeployments {
		limit = maxActiveDeployments
	}

	for _, dep := range d.deployments[:limit] {
		name := deploymentDisplayName(dep)
		cost := dep.Deposit
		if cost == "" {
			cost = "\u2014"
		}
		line := theme.Body.Render(name) + "  " + theme.Muted.Render(cost)
		lines = append(lines, line)
	}

	if len(d.deployments) > maxActiveDeployments {
		more := fmt.Sprintf("... and %d more", len(d.deployments)-maxActiveDeployments)
		lines = append(lines, theme.Muted.Render(more))
	}

	return heading + "\n" + strings.Join(lines, "\n")
}

// deploymentDisplayName returns a display name for a deployment.
func deploymentDisplayName(dep *store.DeploymentRecord) string {
	if dep.SDLPath != "" {
		return fmt.Sprintf("%d (%s)", dep.DSeq, dep.SDLPath)
	}
	return fmt.Sprintf("%d", dep.DSeq)
}

// ─── Network Panel ───────────────────────────────────────────────────

func (d Dashboard) renderNetwork(w int) string {
	chain := d.chainID
	if chain == "" {
		chain = "\u2014"
	}

	var blockHeight, lastSync string
	if d.syncState != nil {
		if d.syncState.LastBlockHeight > 0 {
			blockHeight = fmt.Sprintf("%d", d.syncState.LastBlockHeight)
		} else {
			blockHeight = "\u2014"
		}
		if d.syncState.LastSyncTime > 0 {
			t := time.Unix(d.syncState.LastSyncTime, 0)
			lastSync = t.Format(time.RFC3339)
		} else {
			lastSync = "\u2014"
		}
	} else {
		blockHeight = "\u2014"
		lastSync = "\u2014"
	}

	pairs := []components.KVPair{
		{Label: "chain", Value: chain},
		{Label: "last block", Value: blockHeight},
		{Label: "last sync", Value: lastSync},
	}

	return components.SectionWithKV("Network", w, pairs)
}

// ─── Shortcuts Panel ─────────────────────────────────────────────────

func (d Dashboard) renderShortcuts(w int) string {
	heading := components.Section("Shortcuts", w)

	shortcuts := []components.KVPair{
		{Label: "1-6", Value: "navigate views"},
		{Label: "Enter", Value: "drill down"},
		{Label: "Esc", Value: "go back"},
		{Label: ":", Value: "command palette"},
		{Label: "?", Value: "help"},
		{Label: "D", Value: "new deployment"},
	}

	return heading + "\n" + components.KVBlock(shortcuts)
}
