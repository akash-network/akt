package views

import (
	"strings"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// HelpOverlay renders a centered help overlay with keybinding reference.
type HelpOverlay struct {
	active bool
	width  int
	height int
	// context is the name of the current view for context-sensitive help.
	context string
}

// NewHelpOverlay creates a new help overlay.
func NewHelpOverlay() HelpOverlay {
	return HelpOverlay{}
}

// Active returns whether the overlay is visible.
func (h HelpOverlay) Active() bool {
	return h.active
}

// Open shows the help overlay.
func (h *HelpOverlay) Open(viewContext string) {
	h.active = true
	h.context = viewContext
}

// Close hides the help overlay.
func (h *HelpOverlay) Close() {
	h.active = false
}

// SetSize updates dimensions.
func (h *HelpOverlay) SetSize(w, ht int) {
	h.width = w
	h.height = ht
}

// View renders the help overlay.
func (h HelpOverlay) View() string {
	if !h.active {
		return ""
	}

	boxW := h.width * 60 / 100
	if boxW < 50 {
		boxW = 50
	}
	if boxW > 80 {
		boxW = 80
	}
	innerW := boxW - 6

	sectionTitle := lipgloss.NewStyle().Foreground(theme.AccentRed).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(theme.Slate400).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(theme.Slate300)

	// Title bar: badge "?" + "Keybindings" + context label.
	badge := lipgloss.NewStyle().Foreground(theme.AccentRed).Bold(true).Render("?")
	heading := theme.Heading.Render(" Keybindings")
	contextLabel := ""
	if h.context != "" {
		contextLabel = "  " + theme.Muted.Render(h.context)
	}
	titleBar := badge + heading + contextLabel

	var b strings.Builder
	b.WriteString(titleBar)
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(theme.Slate700).Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	// Helper to render a section.
	writeSection := func(title string, bindings [][2]string) {
		b.WriteString(sectionTitle.Render(strings.ToUpper(title)))
		b.WriteString("\n")
		for _, kv := range bindings {
			k := keyStyle.Width(12).Render(kv[0])
			d := descStyle.Render(kv[1])
			b.WriteString("  ")
			b.WriteString(k)
			b.WriteString(d)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// 4-section grid layout.
	writeSection("Navigation", [][2]string{
		{"1-6", "Switch views"},
		{"h", "Dashboard"},
		{"↵", "Drill into detail"},
		{"esc", "Back / close"},
		{"Tab/S-Tab", "Sub-tabs"},
	})

	writeSection("Lists", [][2]string{
		{"j / ↓", "Next item"},
		{"k / ↑", "Previous item"},
		{"g / G", "First / last"},
		{"/", "Fuzzy search"},
	})

	writeSection("Actions", [][2]string{
		{"D", "New deployment"},
		{"l", "Logs"},
		{"s", "Shell"},
		{"d", "Close / unbond"},
		{"v", "Vote"},
		{"r", "Redelegate"},
	})

	writeSection("Overlays", [][2]string{
		{": / Ctrl+P", "Command palette"},
		{"?", "Help"},
		{"q", "Quit"},
	})

	// Footer: version + close hint.
	footer := theme.Muted.Render("press esc to close")
	b.WriteString(lipgloss.NewStyle().Foreground(theme.Slate700).Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(footer)

	content := b.String()

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Slate600).
		Background(theme.Slate900).
		Padding(1, 2).
		Width(boxW)

	dialog := box.Render(content)

	return lipgloss.Place(h.width, h.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}
