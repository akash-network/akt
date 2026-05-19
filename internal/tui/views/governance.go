package views

import (
	"fmt"
	"strings"
	"time"

	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/akt/internal/tui/components"
)

// GovernanceView renders a table of governance proposals.
type GovernanceView struct {
	table     components.ResourceTable
	proposals []*govv1.Proposal
	width     int
	height    int
}

// NewGovernanceView creates a new GovernanceView with the standard column layout.
func NewGovernanceView() GovernanceView {
	return GovernanceView{
		table: components.NewResourceTable(components.ResourceTableConfig{
			Columns: []components.TableColumn{
				{Header: "#", Width: 6, Align: components.AlignRight},
				{Header: "TITLE", Width: 0, Align: components.AlignLeft},
				{Header: "STATUS", Width: 12, Align: components.AlignLeft, RenderFunc: components.StateTag},
				{Header: "YES", Width: 7, Align: components.AlignRight},
				{Header: "NO", Width: 7, Align: components.AlignRight},
				{Header: "ABSTAIN", Width: 8, Align: components.AlignRight},
				{Header: "VETO", Width: 7, Align: components.AlignRight},
				{Header: "ENDS", Width: 10, Align: components.AlignRight},
			},
			EmptyText: "Governance proposals require chain connection.\nQuery proposals with: akt query gov proposals",
		}),
	}
}

// SetSize updates the available width and height for rendering.
func (v *GovernanceView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.table.SetSize(w, h)
}

// CursorUp moves the cursor up one row.
func (v *GovernanceView) CursorUp() {
	v.table.CursorUp()
}

// CursorDown moves the cursor down one row.
func (v *GovernanceView) CursorDown() {
	v.table.CursorDown()
}

// SetData stores the proposals and rebuilds the table rows.
func (v *GovernanceView) SetData(proposals []*govv1.Proposal) {
	v.proposals = proposals
	rows := make([]components.TableRow, len(proposals))
	for i, p := range proposals {
		rows[i] = components.TableRow{
			ID: fmt.Sprintf("%d", p.Id),
			Cells: []string{
				fmt.Sprintf("%d", p.Id),
				truncateTitle(p.Title, 40),
				govStatusLabel(p.Status.String()),
				"—", "—", "—", "—", // tally TBD
				formatVotingEnd(p.VotingEndTime),
			},
		}
	}
	v.table.SetRows(rows)
}

// SelectedProposal returns the proposal at the cursor, or nil.
func (v *GovernanceView) SelectedProposal() *govv1.Proposal {
	row := v.table.SelectedRow()
	if row == nil {
		return nil
	}
	id := row.ID
	for _, p := range v.proposals {
		if fmt.Sprintf("%d", p.Id) == id {
			return p
		}
	}
	return nil
}

// View renders the governance table.
func (v GovernanceView) View() string {
	return v.table.View()
}

// truncateTitle shortens a title to maxLen characters, appending "…" if truncated.
func truncateTitle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// govStatusLabel returns a human-readable label for a governance proposal status.
func govStatusLabel(status string) string {
	s := strings.TrimPrefix(status, "PROPOSAL_STATUS_")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.ToLower(s)
}

// formatVotingEnd formats a voting end time as a relative or absolute string.
func formatVotingEnd(t *time.Time) string {
	if t == nil {
		return "—"
	}
	d := time.Until(*t)
	if d < 0 {
		return "ended"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
