package views

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/ui/theme"
)

var _ ViewComponent = (*ProposalDetailView)(nil)

// ProposalDetailView is the drill-down detail view for a single governance proposal.
// It displays proposal info and vote tally progress bars.
type ProposalDetailView struct {
	BaseDetailView
	km       keys.KeyMap
	proposal *govv1.Proposal
	tally    *govv1.TallyResult
}

// NewProposalDetailView creates a detail view pre-loaded with a proposal and tally.
func NewProposalDetailView(km keys.KeyMap, proposal *govv1.Proposal, tally *govv1.TallyResult) *ProposalDetailView {
	return &ProposalDetailView{
		BaseDetailView: NewBaseDetailView(),
		km:             km,
		proposal:       proposal,
		tally:          tally,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init returns nil — data is already set via the constructor.
func (v *ProposalDetailView) Init() tea.Cmd { return nil }

// Update handles key events for the proposal detail view.
func (v *ProposalDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, v.km.Back):
			return v, CmdFunc(messages.PopViewMsg{})
		case key.Matches(kmsg, v.km.Vote):
			if v.proposal != nil && v.proposal.Status == govv1.StatusVotingPeriod {
				return v, CmdFunc(messages.ShowConfirmMsg{
					Kind: components.ConfirmVote,
					Data: components.ConfirmData{
						Title: "Vote on Proposal",
						Body:  fmt.Sprintf("Vote on proposal #%d: %s?", v.proposal.Id, truncateTitle(v.proposal.Title, 40)),
					},
				})
			}
		default:
			// j/k scroll handled by BaseDetailView
			v.BaseDetailView.Update(msg)
		}
	}
	return v, nil
}

// View renders the proposal detail panel.
func (v *ProposalDetailView) View() tea.View {
	if v.proposal == nil {
		return tea.NewView(theme.Muted.Render("  No proposal selected"))
	}

	w := v.W
	if w < 40 {
		w = 40
	}

	p := v.proposal
	var lines []string

	// Section 1: Proposal #ID
	lines = append(lines, "  "+theme.SectionTitle.Render(fmt.Sprintf("Proposal #%d", p.Id)))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		"    "+theme.KVLabel.Render("Title")+theme.KVValueBold.Render(p.Title),
		kvPair("Type", theme.KVValue.Render(proposalType(p))),
		"    "+theme.KVLabel.Render("Status")+theme.StateBadge(govStatusLabel(p.Status.String())).Render(govStatusLabel(p.Status.String())),
		kvPair("Ends", theme.KVValue.Render(formatVotingEnd(p.VotingEndTime))),
	)
	lines = append(lines, "")

	// Section 2: Vote Tally
	lines = append(lines, "  "+theme.SectionTitle.Render("Vote Tally"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))

	barW := w - 50
	if barW < 20 {
		barW = 20
	}

	yes, no, abstain, veto := tallyPercentages(v.tally)
	lines = append(lines, renderProgressLine("Yes", yes, fmt.Sprintf("%.1f%%", yes), barW))
	lines = append(lines, renderProgressLine("No", no, fmt.Sprintf("%.1f%%", no), barW))
	lines = append(lines, renderProgressLine("Abstain", abstain, fmt.Sprintf("%.1f%%", abstain), barW))
	lines = append(lines, renderProgressLine("No w/ Veto", veto, fmt.Sprintf("%.1f%%", veto), barW))

	// Apply scrolling via BaseDetailView
	visibleH := v.H - 4
	if visibleH < 3 {
		visibleH = 3
	}

	visible := v.BaseDetailView.VisibleWindow(lines, visibleH)

	var b strings.Builder
	for _, line := range visible {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	return tea.NewView(b.String())
}

// ─── ViewComponent ───────────────────────────────────────────────────

// SetSize delegates to the embedded BaseDetailView.
func (v *ProposalDetailView) SetSize(w, h int) {
	v.BaseDetailView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (v *ProposalDetailView) Breadcrumb() string {
	return "Detail"
}

// ShortHelp returns the footer hint pairs for the proposal detail view.
func (v *ProposalDetailView) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "scroll"},
		{Key: "v", Desc: "vote"},
		{Key: "esc", Desc: "back"},
	}
}

// Refresh returns nil — detail views have no data to reload.
func (v *ProposalDetailView) Refresh() tea.Cmd { return nil }

// ─── Helpers ─────────────────────────────────────────────────────────

// renderProgressLine renders one progress bar line with label, filled/empty blocks, and percentage.
func renderProgressLine(label string, pct float64, detail string, barW int) string {
	filled := int(math.Round(float64(barW) * pct / 100.0))
	empty := barW - filled
	if empty < 0 {
		empty = 0
	}

	bar := theme.BarFilled.Render(strings.Repeat("█", filled)) +
		theme.BarEmpty.Render(strings.Repeat("░", empty))
	pctStr := theme.ProgressPct.Render(detail)

	return "    " + theme.ProgressLabel.Render(label) + bar + " " + pctStr
}

// tallyPercentages converts a TallyResult into yes/no/abstain/veto percentages (0-100).
func tallyPercentages(t *govv1.TallyResult) (yes, no, abstain, veto float64) {
	if t == nil {
		return 0, 0, 0, 0
	}

	yesN, _ := strconv.ParseFloat(t.YesCount, 64)
	noN, _ := strconv.ParseFloat(t.NoCount, 64)
	abstainN, _ := strconv.ParseFloat(t.AbstainCount, 64)
	vetoN, _ := strconv.ParseFloat(t.NoWithVetoCount, 64)

	total := yesN + noN + abstainN + vetoN
	if total == 0 {
		return 0, 0, 0, 0
	}

	return yesN / total * 100,
		noN / total * 100,
		abstainN / total * 100,
		vetoN / total * 100
}

// proposalType extracts a short type name from the first message's TypeUrl.
func proposalType(p *govv1.Proposal) string {
	if len(p.Messages) == 0 {
		return "—"
	}
	typeURL := p.Messages[0].TypeUrl
	if idx := strings.LastIndex(typeURL, "."); idx >= 0 {
		return typeURL[idx+1:]
	}
	return typeURL
}
