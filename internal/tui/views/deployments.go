package views

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/data"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
)

// DeploymentsView is a full tea.Model list view for deployment records.
// It embeds BaseListView for cursor/scroll handling and satisfies the
// ViewComponent interface so the App shell can push it onto the nav stack.
type DeploymentsView struct {
	BaseListView
	svc    data.Service
	owner  string
	data   []*store.DeploymentRecord
	filter string // "", "active", "closed"
}

// NewDeploymentsView creates a DeploymentsView wired to the given data service.
func NewDeploymentsView(svc data.Service, km keys.KeyMap, owner string) *DeploymentsView {
	cfg := components.ResourceTableConfig{
		Columns:   deploymentsColumns(),
		EmptyText: "No deployments found",
	}
	return &DeploymentsView{
		BaseListView: NewBaseListView(cfg, km),
		svc:          svc,
		owner:        owner,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init kicks off the initial data load.
func (d *DeploymentsView) Init() tea.Cmd {
	return d.svc.LoadDeployments(d.owner)
}

// Update handles messages for the deployments list.
func (d *DeploymentsView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.DeploymentsLoadedMsg:
		if msg.Err == nil {
			d.data = msg.Deployments
			d.applyFilter()
		}
		return d, nil
	}

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, d.Keys.Select):
			rec := d.selectedRecord()
			if rec != nil {
				detail := NewDeploymentDetailView(d.svc, d.Keys, rec)
				return d, CmdFunc(messages.PushViewMsg{View: detail})
			}
		case key.Matches(kmsg, d.Keys.Logs):
			rec := d.selectedRecord()
			if rec != nil {
				return d, CmdFunc(messages.StartLogStreamMsg{Owner: rec.Owner, DSeq: rec.DSeq})
			}
		case key.Matches(kmsg, d.Keys.Close):
			rec := d.selectedRecord()
			if rec != nil {
				return d, CmdFunc(messages.ShowConfirmMsg{
					Kind: components.ConfirmClose,
					Data: components.ConfirmData{
						Title:  "Close Deployment",
						Body:   fmt.Sprintf("Close deployment %d? This action is irreversible.", rec.DSeq),
						Danger: true,
					},
				})
			}
		case key.Matches(kmsg, d.Keys.Filter):
			d.cycleFilter()
			return d, d.svc.LoadDeployments(d.owner)
		}
		// Fall through to BaseListView for cursor keys
		d.BaseListView.Update(msg)
	}
	return d, nil
}

// View delegates rendering to the embedded BaseListView table.
func (d *DeploymentsView) View() tea.View {
	return d.BaseListView.View()
}

// ─── ViewComponent ───────────────────────────────────────────────────

// SetSize delegates to the embedded BaseListView.
func (d *DeploymentsView) SetSize(w, h int) {
	d.BaseListView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (d *DeploymentsView) Breadcrumb() string {
	return "Deployments"
}

// ShortHelp returns the footer hint pairs for the deployments list.
func (d *DeploymentsView) ShortHelp() []components.HintPair {
	hints := []components.HintPair{
		{Key: "j/k", Desc: "navigate"},
		{Key: "↵", Desc: "detail"},
		{Key: "f", Desc: "filter"},
		{Key: "l", Desc: "logs"},
		{Key: "d", Desc: "close", Accent: true},
		{Key: "esc", Desc: "back"},
	}
	if d.filter != "" {
		hints = append(hints, components.HintPair{Key: "filter", Desc: d.filter})
	}
	return hints
}

// Refresh re-fires the data load for this view.
func (d *DeploymentsView) Refresh() tea.Cmd {
	return d.svc.LoadDeployments(d.owner)
}

// ─── Internal ────────────────────────────────────────────────────────

// selectedRecord returns the deployment record at the cursor, or nil.
func (d *DeploymentsView) selectedRecord() *store.DeploymentRecord {
	row := d.BaseListView.SelectedRow()
	if row == nil {
		return nil
	}
	dseq, _ := strconv.ParseUint(row.ID, 10, 64)
	for _, r := range d.data {
		if r.DSeq == dseq {
			return r
		}
	}
	return nil
}

// cycleFilter rotates through "" -> "active" -> "closed" -> "" and re-applies.
func (d *DeploymentsView) cycleFilter() {
	switch d.filter {
	case "":
		d.filter = "active"
	case "active":
		d.filter = "closed"
	case "closed":
		d.filter = ""
	}
	d.applyFilter()
}

// applyFilter rebuilds the table rows from d.data using the current filter.
func (d *DeploymentsView) applyFilter() {
	var filtered []*store.DeploymentRecord
	for _, r := range d.data {
		if d.filter != "" && r.State != d.filter {
			continue
		}
		filtered = append(filtered, r)
	}

	rows := make([]components.TableRow, len(filtered))
	for i, r := range filtered {
		rows[i] = components.TableRow{
			ID:    strconv.FormatUint(r.DSeq, 10),
			Cells: deploymentCells(r),
		}
	}
	d.BaseListView.SetRows(rows)
}

// ─── Column Definitions ──────────────────────────────────────────────

// deploymentsColumns returns the standard column layout for the deployments table.
func deploymentsColumns() []components.TableColumn {
	return []components.TableColumn{
		{Header: "DSEQ", Width: 10, Align: components.AlignLeft},
		{Header: "IMAGE", Width: 0, Align: components.AlignLeft},
		{Header: "STATE", Width: 12, Align: components.AlignLeft, RenderFunc: components.StateTag},
		{Header: "CPU", Width: 6, Align: components.AlignRight},
		{Header: "MEMORY", Width: 8, Align: components.AlignRight},
		{Header: "GPU", Width: 10, Align: components.AlignLeft},
		{Header: "PROVIDER", Width: 0, Align: components.AlignLeft},
		{Header: "AGE", Width: 10, Align: components.AlignRight},
		{Header: "ESCROW", Width: 14, Align: components.AlignRight},
		{Header: "COST", Width: 14, Align: components.AlignRight},
	}
}

// ─── Row Mapping ─────────────────────────────────────────────────────

// deploymentCells formats a DeploymentRecord into cell values matching the column layout.
func deploymentCells(r *store.DeploymentRecord) []string {
	// DSEQ
	dseq := strconv.FormatUint(r.DSeq, 10)

	// IMAGE: show SDLPath basename or "—"
	image := "—"
	if r.SDLPath != "" {
		image = filepath.Base(r.SDLPath)
	}

	// STATE
	state := r.State

	// CPU, MEMORY, GPU: from labels if available, else "—"
	cpu := labelOrDash(r.Labels, "cpu")
	memory := labelOrDash(r.Labels, "memory")
	gpu := labelOrDash(r.Labels, "gpu")

	// PROVIDER: from labels if available, else "—"
	provider := labelOrDash(r.Labels, "provider")

	// AGE: relative time since created
	age := "—"
	if r.CreatedAt > 0 {
		age = relativeTime(r.CreatedAt)
	}

	// ESCROW: escrow balance or "—"
	escrow := "—"
	if r.EscrowBalance != "" {
		escrow = r.EscrowBalance
	}

	// COST: deposit or "—"
	cost := "—"
	if r.Deposit != "" {
		cost = r.Deposit
	}

	return []string{dseq, image, state, cpu, memory, gpu, provider, age, escrow, cost}
}

// ─── Helpers ─────────────────────────────────────────────────────────

// labelOrDash returns the label value for key, or "—" if not present.
func labelOrDash(labels map[string]string, key string) string {
	if labels != nil {
		if v, ok := labels[key]; ok && v != "" {
			return v
		}
	}
	return "—"
}

// relativeTime formats a Unix timestamp as a human-readable relative duration.
func relativeTime(unix int64) string {
	d := time.Since(time.Unix(unix, 0))
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
