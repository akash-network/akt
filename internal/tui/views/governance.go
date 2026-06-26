package views

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/data"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
)

var _ ViewComponent = (*GovernanceView)(nil)

// GovernanceView is a full tea.Model list view for governance proposals.
// It embeds BaseListView for cursor/scroll handling and satisfies the
// ViewComponent interface so the App shell can push it onto the nav stack.
type GovernanceView struct {
	BaseListView
	svc       data.Service
	proposals []*govv1.Proposal
	tallies   map[uint64]*govv1.TallyResult
}

// NewGovernanceView creates a GovernanceView wired to the given data service.
func NewGovernanceView(svc data.Service, km keys.KeyMap) *GovernanceView {
	cfg := components.ResourceTableConfig{
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
	}
	return &GovernanceView{
		BaseListView: NewBaseListView(cfg, km),
		svc:          svc,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init kicks off the initial data load.
func (v *GovernanceView) Init() tea.Cmd {
	return v.svc.LoadProposals()
}

// Update handles messages for the governance list.
func (v *GovernanceView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.ProposalsLoadedMsg:
		if msg.Err == nil {
			v.proposals = msg.Proposals
			v.rebuildRows()
			// Fire tally loads for voting-period proposals
			var votingProposals []*govv1.Proposal
			for _, p := range v.proposals {
				if p.Status == govv1.StatusVotingPeriod {
					votingProposals = append(votingProposals, p)
				}
			}
			if len(votingProposals) > 0 {
				return v, v.svc.LoadTallies(votingProposals)
			}
		}
		return v, nil
	case messages.TallyLoadedMsg:
		if msg.Err == nil {
			if v.tallies == nil {
				v.tallies = make(map[uint64]*govv1.TallyResult)
			}
			for id, t := range msg.Tallies {
				v.tallies[id] = t
			}
			v.rebuildRows()
		}
		return v, nil
	}

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, v.Keys.Select):
			p := v.selectedProposal()
			if p != nil {
				var tally *govv1.TallyResult
				if v.tallies != nil {
					tally = v.tallies[p.Id]
				}
				detail := NewProposalDetailView(v.Keys, p, tally)
				return v, CmdFunc(messages.PushViewMsg{View: detail})
			}
		case key.Matches(kmsg, v.Keys.Vote):
			p := v.selectedProposal()
			if p != nil && p.Status == govv1.StatusVotingPeriod {
				return v, CmdFunc(messages.ShowConfirmMsg{
					Kind: components.ConfirmVote,
					Data: components.ConfirmData{
						Title: "Vote on Proposal",
						Body:  fmt.Sprintf("Vote on proposal #%d: %s?", p.Id, truncateTitle(p.Title, 40)),
					},
				})
			}
		}
		// Fall through to BaseListView for cursor keys
		v.BaseListView.Update(msg)
	}
	return v, nil
}

// View delegates rendering to the embedded BaseListView table.
func (v *GovernanceView) View() tea.View {
	return v.BaseListView.View()
}

// ─── ViewComponent ───────────────────────────────────────────────────

// SetSize delegates to the embedded BaseListView.
func (v *GovernanceView) SetSize(w, h int) {
	v.BaseListView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (v *GovernanceView) Breadcrumb() string {
	return "Governance"
}

// ShortHelp returns the footer hint pairs for the governance list.
func (v *GovernanceView) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "navigate"},
		{Key: "↵", Desc: "detail"},
		{Key: "v", Desc: "vote"},
		{Key: "esc", Desc: "back"},
	}
}

// Refresh re-fires the data load for this view.
func (v *GovernanceView) Refresh() tea.Cmd {
	return v.svc.LoadProposals()
}

// ─── Internal ────────────────────────────────────────────────────────

// selectedProposal returns the proposal at the cursor, or nil.
func (v *GovernanceView) selectedProposal() *govv1.Proposal {
	row := v.BaseListView.SelectedRow()
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

// rebuildRows rebuilds the table rows from the current proposals and tallies.
func (v *GovernanceView) rebuildRows() {
	rows := make([]components.TableRow, len(v.proposals))
	for i, p := range v.proposals {
		yesPct, noPct, abstainPct, vetoPct := "—", "—", "—", "—"
		if v.tallies != nil {
			if t, ok := v.tallies[p.Id]; ok && t != nil {
				yes, no, abstain, veto := tallyPercentages(t)
				yesPct = fmt.Sprintf("%.1f%%", yes)
				noPct = fmt.Sprintf("%.1f%%", no)
				abstainPct = fmt.Sprintf("%.1f%%", abstain)
				vetoPct = fmt.Sprintf("%.1f%%", veto)
			}
		}
		rows[i] = components.TableRow{
			ID: fmt.Sprintf("%d", p.Id),
			Cells: []string{
				fmt.Sprintf("%d", p.Id),
				truncateTitle(p.Title, 40),
				govStatusLabel(p.Status.String()),
				yesPct, noPct, abstainPct, vetoPct,
				formatVotingEnd(p.VotingEndTime),
			},
		}
	}
	v.BaseListView.SetRows(rows)
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
