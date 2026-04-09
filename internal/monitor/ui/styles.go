package ui

import (
	"fmt"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/glyphs"
)

var (
	// ANSI terminal palette colors (respects terminal theme like Catppuccin)
	// Uses the terminal's 16-color palette (0-15)
	primaryColor = lipgloss.ANSIColor(1)  // Red
	accentColor  = lipgloss.ANSIColor(9)  // Bright Red
	successColor = lipgloss.ANSIColor(2)  // Green
	warningColor = lipgloss.ANSIColor(3)  // Yellow
	errorColor   = lipgloss.ANSIColor(9)  // Bright Red
	mutedColor   = lipgloss.ANSIColor(8)  // Bright Black (Surface)
	textColor    = lipgloss.ANSIColor(7)  // White (Text)
	brightText   = lipgloss.ANSIColor(15) // Bright White
	borderColor  = lipgloss.ANSIColor(8)  // Bright Black (Surface)

	// Title style
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor).
			MarginBottom(1)

	// Header style for section headers
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(borderColor).
			PaddingBottom(1).
			MarginBottom(1)

	// Label style for field labels
	labelStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Width(12)

	// Value style for field values
	valueStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(brightText)

	// Double progress bar color
	precommitBarColor = lipgloss.ANSIColor(6) // Cyan — precommits

	// Percentage styles based on threshold
	percentLowStyle = lipgloss.NewStyle().
			Foreground(warningColor)

	percentHighStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(successColor)

	// Grid styles (dots and version indicators stay green)
	gridVotedStyle = lipgloss.NewStyle().
			Foreground(successColor)

	gridNotVotedStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Error style
	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// Help style
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	// Status bar style
	statusBarStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	// Muted text style (for general muted text)
	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	// Proposer style (star indicator)
	proposerStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	// Tab styles
	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(brightText).
			Background(primaryColor).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Padding(0, 1)

	// Moniker style
	monikerStyle = lipgloss.NewStyle().
			Foreground(textColor)

	// Highlight style for selected rows
	highlightStyle = lipgloss.NewStyle().
			Foreground(brightText).
			Bold(true)

	// Detail view styles
	detailHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor)

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Width(10)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(textColor)

	// Vote indicator styles
	voteYesStyle = lipgloss.NewStyle().
			Foreground(successColor)

	voteNoStyle = lipgloss.NewStyle().
			Foreground(errorColor)
)

// ProgressBar renders a progress bar with the given percentage (0-1) using bubbles/progress.
func ProgressBar(percent float64, width int) string {
	p := progress.New(
		progress.WithColors(primaryColor),
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
		progress.WithColors(primaryColor),
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
		progress.WithColors(successColor),
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
