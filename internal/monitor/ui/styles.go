package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
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

	// Progress bar styles
	progressBarWidth = 40

	progressFullStyle = lipgloss.NewStyle().
				Foreground(primaryColor)

	progressEmptyStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// Double progress bar colors (half-block ▀: top = prevotes, bottom = precommits)
	precommitBarColor = lipgloss.ANSIColor(6) // Cyan — precommits (bottom half)
	barEmptyBgColor   = lipgloss.ANSIColor(0) // Black — empty track

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

// ProgressBar renders a progress bar with the given percentage (0-1)
func ProgressBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}

	filled := int(float64(width) * percent)
	empty := width - filled

	bar := progressFullStyle.Render(repeatChar('█', filled)) +
		progressEmptyStyle.Render(repeatChar('░', empty))

	return bar
}

// repeatChar repeats a character n times
func repeatChar(char rune, n int) string {
	if n <= 0 {
		return ""
	}
	result := make([]rune, n)
	for i := range result {
		result[i] = char
	}
	return string(result)
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

// ProgressBarWithLabel renders a fixed-width progress bar with a text label
// centred inside it. The filled portion uses the "full" style and the empty
// portion uses the "empty" style. The label text is overlaid at the centre,
// inheriting the style of whichever region it falls in.
func ProgressBarWithLabel(percent float64, width int, label string) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}

	filled := int(float64(width) * percent)
	if filled > width {
		filled = width
	}

	// Centre the label within the bar.
	labelStart := (width - len(label)) / 2
	if labelStart < 0 {
		labelStart = 0
	}

	var bar string
	for i := 0; i < width; i++ {
		var ch string
		if i >= labelStart && i < labelStart+len(label) {
			ch = string(label[i-labelStart])
		} else if i < filled {
			ch = "█"
		} else {
			ch = "░"
		}

		if i < filled {
			bar += progressFullStyle.Render(ch)
		} else {
			bar += progressEmptyStyle.Render(ch)
		}
	}
	return bar
}

// DoubleProgressBar renders a single-line dual progress bar using the
// upper-half-block character (▀ U+2580). The top half of each cell
// represents prevote progress (green) and the bottom half represents
// precommit progress (cyan). Empty portions use a dark background.
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

	pvFilled := int(float64(width) * prevotePct)
	pcFilled := int(float64(width) * precommitPct)
	if pvFilled > width {
		pvFilled = width
	}
	if pcFilled > width {
		pcFilled = width
	}

	// Build the bar by grouping consecutive cells with the same fg/bg
	// into a single Render call for efficiency.
	type cellStyle struct {
		fg lipgloss.TerminalColor
		bg lipgloss.TerminalColor
	}

	styleFor := func(i int) cellStyle {
		var fg, bg lipgloss.TerminalColor
		if i < pvFilled {
			fg = successColor // green — prevotes (top half)
		} else {
			fg = barEmptyBgColor
		}
		if i < pcFilled {
			bg = precommitBarColor // cyan — precommits (bottom half)
		} else {
			bg = barEmptyBgColor
		}
		return cellStyle{fg, bg}
	}

	var bar string
	i := 0
	for i < width {
		cs := styleFor(i)
		// Count consecutive cells with the same style.
		j := i + 1
		for j < width && styleFor(j) == cs {
			j++
		}
		segment := repeatChar('▀', j-i)
		bar += lipgloss.NewStyle().Foreground(cs.fg).Background(cs.bg).Render(segment)
		i = j
	}

	return bar
}

// FormatVoteGrid formats the bit array pattern into a colored grid
func FormatVoteGrid(pattern string, width int) string {
	if pattern == "" {
		return mutedStyle.Render("No vote data")
	}

	var result string
	for i, char := range pattern {
		if i > 0 && i%width == 0 {
			result += "\n"
		}
		if char == 'x' {
			result += gridVotedStyle.Render("\uf00c") // nf-fa-check
		} else {
			result += gridNotVotedStyle.Render("\uf00d") // nf-fa-times
		}
	}
	return result
}
