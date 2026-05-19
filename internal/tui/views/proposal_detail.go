package views

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// ProposalDetailView is the drill-down detail view for a single governance proposal.
// It displays proposal info and vote tally progress bars.
type ProposalDetailView struct {
	proposal *govv1.Proposal
	tally    *govv1.TallyResult
	width    int
	height   int
	scroll   int
}

// NewProposalDetailView creates a new empty proposal detail view.
func NewProposalDetailView() ProposalDetailView {
	return ProposalDetailView{}
}

// SetProposal sets the proposal to display.
func (v *ProposalDetailView) SetProposal(p *govv1.Proposal) {
	v.proposal = p
	v.scroll = 0
}

// SetTally sets the tally result to display.
func (v *ProposalDetailView) SetTally(t *govv1.TallyResult) {
	v.tally = t
}

// SetSize updates the view dimensions.
func (v *ProposalDetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp scrolls the content up by one line.
func (v *ProposalDetailView) ScrollUp() {
	if v.scroll > 0 {
		v.scroll--
	}
}

// ScrollDown scrolls the content down by one line.
func (v *ProposalDetailView) ScrollDown() {
	v.scroll++
}

// View renders the proposal detail panel.
func (v ProposalDetailView) View() string {
	if v.proposal == nil {
		return theme.Muted.Render("  No proposal selected")
	}

	w := v.width
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

	// Apply scrolling
	visibleH := v.height - 4
	if visibleH < 3 {
		visibleH = 3
	}

	start := v.scroll
	if start >= len(lines) {
		start = max(0, len(lines)-1)
	}
	end := start + visibleH
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}

	return b.String()
}

// ─── Helpers ─────────────────────────────────────────────────────────

// truncateStr truncates a string to maxLen characters, appending "…" if truncated.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

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
