package ui

import (
	"fmt"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/glyphs"
	"pkg.akt.dev/akt/internal/ui/theme"
)

// All colors and styles are sourced from the shared theme package so the
// monitor dashboard, CLI pretty output, and TUI views stay visually
// consistent.
var (
	// Styles — aliases into the shared theme.
	titleStyle        = theme.Heading
	headerStyle       = theme.SectionTitle
	labelStyle        = theme.KVLabel.Width(12)
	valueStyle        = theme.KVValue
	percentLowStyle   = theme.PercentLow
	percentHighStyle  = theme.PercentHigh
	gridVotedStyle    = theme.GridVoted
	gridNotVotedStyle = theme.GridNotVoted
	errorStyle        = theme.Error
	helpStyle         = theme.HelpBar
	statusBarStyle    = theme.StatusBar
	mutedStyle        = theme.Muted
	proposerStyle     = theme.Proposer
	tabActiveStyle    = theme.NavTabActive
	tabInactiveStyle  = theme.NavTabInactive
	monikerStyle      = theme.Moniker
	highlightStyle    = theme.Highlight
	detailHeaderStyle = theme.SectionTitle
	detailLabelStyle  = theme.KVLabel
	detailValueStyle  = theme.KVValue
	voteYesStyle      = theme.VoteYes
	voteNoStyle       = theme.VoteNo

	// Progress bar color — used by DoubleProgressBar for the precommit bar.
	precommitBarColor = theme.ProgressPrecommit
)

// ProgressBar renders a progress bar with the given percentage (0-1) using bubbles/progress.
func ProgressBar(percent float64, width int) string {
	p := progress.New(
		progress.WithColors(theme.ProgressPrimary),
		progress.WithoutPercentage(),
	)
	p.SetWidth(width)
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}
	return p.ViewAs(percent)
}

// FormatPercent formats a percentage with color based on threshold
func FormatPercent(percent float64) string {
	pctStr := lipgloss.NewStyle().Width(6).Render(
		fmt.Sprintf("%5.1f%%", percent*100),
	)

	if percent >= 0.667 {
		return percentHighStyle.Render(pctStr)
	}
	return percentLowStyle.Render(pctStr)
}

// ProgressBarWithLabel renders a progress bar with a text label centered inside it.
func ProgressBarWithLabel(percent float64, width int, label string) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}

	p := progress.New(
		progress.WithColors(theme.ProgressPrimary),
		progress.WithoutPercentage(),
	)
	p.SetWidth(width)
	bar := p.ViewAs(percent)

	// Overlay centered label on top of the bar
	if len(label) > 0 && len(label) < width {
		labelStart := (width - len(label)) / 2
		// Build the overlaid version character by character
		barRunes := []rune(bar)
		labelRunes := []rune(label)
		for i, r := range labelRunes {
			pos := labelStart + i
			if pos < len(barRunes) {
				barRunes[pos] = r
			}
		}
		return string(barRunes)
	}
	return bar
}

// DoubleProgressBar renders two stacked progress bars: top for prevotes (green),
// bottom for precommits (cyan). This replaces the previous half-block (▀) design
// with standard bubbles/progress components.
func DoubleProgressBar(prevotePct, precommitPct float64, width int) string {
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	prevotePct = clamp(prevotePct)
	precommitPct = clamp(precommitPct)

	pvBar := progress.New(
		progress.WithColors(theme.ProgressSuccess),
		progress.WithoutPercentage(),
	)
	pvBar.SetWidth(width)

	pcBar := progress.New(
		progress.WithColors(precommitBarColor),
		progress.WithoutPercentage(),
	)
	pcBar.SetWidth(width)

	return pvBar.ViewAs(prevotePct) + "\n" + pcBar.ViewAs(precommitPct)
}

// FormatVoteGrid formats the bit array pattern into a colored grid
func FormatVoteGrid(pattern string, width int) string {
	if pattern == "" {
		return mutedStyle.Render("No vote data")
	}

	g := glyphs.G()
	var result string
	for i, char := range pattern {
		if i > 0 && i%width == 0 {
			result += "\n"
		}
		if char == 'x' {
			result += gridVotedStyle.Render(g.VoteYes)
		} else {
			result += gridNotVotedStyle.Render(g.VoteNo)
		}
	}
	return result
}
