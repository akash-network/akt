package views

import (
	"strings"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// DetailView is a generic reusable detail panel that displays pre-rendered
// content (from a Render* function) with a title and back hint.
type DetailView struct {
	title   string
	content string // pre-rendered styled string from a Render* function
	width   int
	height  int
	scroll  int
}

// NewDetailView creates a new detail view.
func NewDetailView() DetailView {
	return DetailView{}
}

// SetContent updates the detail view with new content.
func (v *DetailView) SetContent(title, content string) {
	v.title = title
	v.content = content
	v.scroll = 0
}

// Clear resets the detail view.
func (v *DetailView) Clear() {
	v.title = ""
	v.content = ""
	v.scroll = 0
}

// HasContent returns true if the detail view has content to display.
func (v DetailView) HasContent() bool {
	return v.content != ""
}

// SetSize updates the view dimensions.
func (v *DetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp scrolls the content up by one line.
func (v *DetailView) ScrollUp() {
	if v.scroll > 0 {
		v.scroll--
	}
}

// ScrollDown scrolls the content down by one line.
func (v *DetailView) ScrollDown() {
	v.scroll++
}

// View renders the detail panel.
func (v DetailView) View() string {
	var b strings.Builder
	w := v.width
	if w < 40 {
		w = 40
	}

	// Title
	if v.title != "" {
		title := theme.SectionHeader.Width(w).Render(v.title)
		b.WriteString(title)
		b.WriteString("\n")
	}

	// Content with scrolling
	lines := strings.Split(v.content, "\n")
	maxLines := v.height - 5 // title(3) + back hint(1) + padding(1)
	if maxLines < 3 {
		maxLines = 3
	}

	start := v.scroll
	if start >= len(lines) {
		start = max(0, len(lines)-1)
	}
	end := start + maxLines
	if end > len(lines) {
		end = len(lines)
	}

	for i := start; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}

	// Back hint
	b.WriteString("\n")
	b.WriteString(theme.Muted.Render("  esc: back"))

	// Scroll indicator
	if len(lines) > maxLines {
		scrollInfo := lipgloss.NewStyle().Foreground(theme.ColorMuted).Render(
			"  j/k: scroll",
		)
		b.WriteString(scrollInfo)
	}
	b.WriteString("\n")

	return b.String()
}
