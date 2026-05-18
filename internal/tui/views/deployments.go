package views

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/tui/components"
)

// DeploymentsView renders a table of deployment records with filtering.
type DeploymentsView struct {
	table  components.ResourceTable
	data   []*store.DeploymentRecord
	width  int
	height int
	filter string // "", "active", "closed"
}

// NewDeploymentsView creates a new DeploymentsView with the standard column layout.
func NewDeploymentsView() DeploymentsView {
	return DeploymentsView{
		table: components.NewResourceTable(components.ResourceTableConfig{
			Columns: []components.TableColumn{
				{Header: "DSEQ", Width: 10, Align: components.AlignLeft},
				{Header: "IMAGE", Width: 0, Align: components.AlignLeft},
				{Header: "STATE", Width: 12, Align: components.AlignLeft, RenderFunc: components.StateTag},
				{Header: "CPU", Width: 6, Align: components.AlignRight},
				{Header: "MEMORY", Width: 8, Align: components.AlignRight},
				{Header: "GPU", Width: 10, Align: components.AlignLeft},
				{Header: "PROVIDER", Width: 0, Align: components.AlignLeft},
				{Header: "AGE", Width: 10, Align: components.AlignRight},
				{Header: "COST", Width: 14, Align: components.AlignRight},
			},
			EmptyText: "No deployments. Use 'akt deploy <sdl>' to create one.",
		}),
	}
}

// SetData stores the records and rebuilds the table rows using the current filter.
func (v *DeploymentsView) SetData(records []*store.DeploymentRecord) {
	v.data = records
	v.applyFilter()
}

// SetSize updates the available width and height for rendering.
func (v *DeploymentsView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.table.SetSize(w, h)
}

// CycleFilter rotates through "" -> "active" -> "closed" -> "" and re-applies.
func (v *DeploymentsView) CycleFilter() {
	switch v.filter {
	case "":
		v.filter = "active"
	case "active":
		v.filter = "closed"
	case "closed":
		v.filter = ""
	}
	v.applyFilter()
}

// SelectedDseq returns the DSEQ of the currently highlighted deployment, or 0.
func (v *DeploymentsView) SelectedDseq() uint64 {
	row := v.table.SelectedRow()
	if row == nil {
		return 0
	}
	dseq, _ := strconv.ParseUint(row.ID, 10, 64)
	return dseq
}

// SelectedRecord returns the deployment record at the cursor, or nil.
func (v *DeploymentsView) SelectedRecord() *store.DeploymentRecord {
	row := v.table.SelectedRow()
	if row == nil {
		return nil
	}
	dseq, _ := strconv.ParseUint(row.ID, 10, 64)
	for _, r := range v.data {
		if r.DSeq == dseq {
			return r
		}
	}
	return nil
}

// CursorUp moves the cursor up one row.
func (v *DeploymentsView) CursorUp() {
	v.table.CursorUp()
}

// CursorDown moves the cursor down one row.
func (v *DeploymentsView) CursorDown() {
	v.table.CursorDown()
}

// View renders the deployments table.
func (v DeploymentsView) View() string {
	return v.table.View()
}

// applyFilter rebuilds the table rows from v.data using the current filter.
func (v *DeploymentsView) applyFilter() {
	var filtered []*store.DeploymentRecord
	for _, r := range v.data {
		if v.filter != "" && r.State != v.filter {
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
	v.table.SetRows(rows)
}

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

	// COST: deposit or "—"
	cost := "—"
	if r.Deposit != "" {
		cost = r.Deposit
	}

	return []string{dseq, image, state, cpu, memory, gpu, provider, age, cost}
}

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
