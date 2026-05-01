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

	var b strings.Builder

	title := theme.Bold.Render("Keyboard Shortcuts")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", innerW))
	b.WriteString("\n\n")

	// Global keys
	b.WriteString(theme.Section.Render("Global"))
	b.WriteString("\n")
	writeHelpLine(&b, innerW, "1-6", "Primary views (Deployments/Leases/Providers/Monitor/Gov/Staking)")
	writeHelpLine(&b, innerW, "esc", "Back / close detail")
	writeHelpLine(&b, innerW, ": / ctrl+p", "Command palette")
	writeHelpLine(&b, innerW, "?", "This help")
	writeHelpLine(&b, innerW, "ctrl+c", "Quit")
	b.WriteString("\n")

	// Navigation keys
	b.WriteString(theme.Section.Render("Navigation"))
	b.WriteString("\n")
	writeHelpLine(&b, innerW, "j/k or ↑/↓", "Move cursor")
	writeHelpLine(&b, innerW, "enter", "Select / drill into detail")
	writeHelpLine(&b, innerW, "tab/shift+tab", "Cycle monitor dashboards")
	b.WriteString("\n")

	// Context-specific help
	switch h.context {
	case "deployments":
		b.WriteString(theme.Section.Render("Deployments"))
		b.WriteString("\n")
		writeHelpLine(&b, innerW, "enter", "View deployment detail")
		writeHelpLine(&b, innerW, "r", "Refresh list")
	case "leases":
		b.WriteString(theme.Section.Render("Leases"))
		b.WriteString("\n")
		writeHelpLine(&b, innerW, "enter", "View lease detail")
	case "providers":
		b.WriteString(theme.Section.Render("Providers"))
		b.WriteString("\n")
		writeHelpLine(&b, innerW, "enter", "View provider detail")
	case "governance":
		b.WriteString(theme.Section.Render("Governance"))
		b.WriteString("\n")
		writeHelpLine(&b, innerW, "enter", "View proposal detail")
		writeHelpLine(&b, innerW, "v", "Vote on proposal")
	case "staking":
		b.WriteString(theme.Section.Render("Staking"))
		b.WriteString("\n")
		writeHelpLine(&b, innerW, "enter", "View validator detail")
		writeHelpLine(&b, innerW, "d", "Delegate")
	case "monitor":
		b.WriteString(theme.Section.Render("Monitor"))
		b.WriteString("\n")
		writeHelpLine(&b, innerW, "1/2/3", "Overview / Validators / Governance")
		writeHelpLine(&b, innerW, "tab", "Cycle dashboards (Network/Provider/BME)")
		writeHelpLine(&b, innerW, "r", "Refresh")
	}

	content := b.String()

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ColorHighlight).
		Padding(1, 2).
		Width(boxW)

	dialog := box.Render(content)

	return lipgloss.Place(h.width, h.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}

func writeHelpLine(b *strings.Builder, width int, key, desc string) {
	keyStyled := theme.Value.Render(key)
	descStyled := theme.Muted.Render(desc)
	b.WriteString("  ")
	b.WriteString(keyStyled)
	// Pad key column to 16 chars (accounting for ANSI)
	keyWidth := lipgloss.Width(keyStyled)
	if keyWidth < 16 {
		b.WriteString(strings.Repeat(" ", 16-keyWidth))
	}
	b.WriteString(descStyled)
	b.WriteString("\n")
}
