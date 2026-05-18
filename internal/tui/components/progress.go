package components

import (
	"fmt"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// ProgressBar renders a progress bar with the given percentage (0-1) using
// bubbles/progress. Filled blocks use theme.Slate200 and empty blocks use
// theme.Slate700.
func ProgressBar(percent float64, width int) string {
	p := progress.New(
		progress.WithColors(theme.Slate200),
		progress.WithoutPercentage(),
	)
	p.EmptyColor = theme.Slate700
	p.SetWidth(width)
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}
	return p.ViewAs(percent)
}

// FormatPercent formats a percentage with color based on threshold.
// Values >= 66.7% are rendered in green; below that in yellow.
func FormatPercent(percent float64) string {
	pctStr := lipgloss.NewStyle().Width(6).Render(
		fmt.Sprintf("%5.1f%%", percent*100),
	)

	if percent >= 0.667 {
		return lipgloss.NewStyle().Bold(true).Foreground(theme.GreenColor).Render(pctStr)
	}
	return lipgloss.NewStyle().Foreground(theme.YellowColor).Render(pctStr)
}

// ProgressBarWithLabel renders a progress bar with a text label centered
// inside it.
func ProgressBarWithLabel(percent float64, width int, label string) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}

	p := progress.New(
		progress.WithColors(theme.Slate200),
		progress.WithoutPercentage(),
	)
	p.EmptyColor = theme.Slate700
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
