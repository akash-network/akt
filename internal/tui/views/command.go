package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/tui/commands"
)

// CommandSubmitMsg is sent when the user selects a command from the palette.
type CommandSubmitMsg struct {
	Value string
}

// PaletteKeys holds the keybindings used inside the command palette.
// It is populated by the parent App from the global KeyMap so the views
// package does not need to import the tui package (avoids circular deps).
type PaletteKeys struct {
	CursorUp   key.Binding
	CursorDown key.Binding
	Select     key.Binding
	Close      key.Binding
}

// CommandPalette is a centered overlay with a text input on top and a
// filtered command list below. Activated by : or Ctrl+P.
type CommandPalette struct {
	keys     PaletteKeys
	registry *commands.Registry
	input    textinput.Model
	filtered []commands.Command
	cursor   int
	width    int
	height   int
	active   bool
}

// NewCommandPalette returns a new command palette backed by the given registry
// and using the provided keybindings.
func NewCommandPalette(reg *commands.Registry, keys PaletteKeys) CommandPalette {
	ti := textinput.New()
	ti.Prompt = ":"
	ti.CharLimit = 128
	s := ti.Styles()
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ti.SetStyles(s)

	return CommandPalette{
		keys:     keys,
		registry: reg,
		input:    ti,
		filtered: reg.All(),
	}
}

// Active returns whether the palette is visible.
func (p CommandPalette) Active() bool {
	return p.active
}

// Open shows the palette, resets input, and refreshes the filtered list.
func (p *CommandPalette) Open() {
	p.active = true
	p.input.SetValue("")
	p.input.Focus()
	p.filtered = p.registry.All()
	p.cursor = 0
}

// Close hides the palette.
func (p *CommandPalette) Close() {
	p.active = false
	p.input.Blur()
}

// SetSize updates the overlay dimensions.
func (p *CommandPalette) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// Update handles input events for the palette. All navigation keys are
// resolved through the configurable PaletteKeys bindings.
func (p *CommandPalette) Update(msg tea.Msg) tea.Cmd {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, p.keys.Select):
			name := p.selectedName()
			p.Close()
			if name != "" {
				return func() tea.Msg {
					return CommandSubmitMsg{Value: name}
				}
			}
			// If no commands match but the user typed raw text, submit it
			// as-is so handleCommand can try an exact match (e.g. "quit").
			if val := p.input.Value(); val != "" {
				return func() tea.Msg {
					return CommandSubmitMsg{Value: val}
				}
			}
			return nil

		case key.Matches(kmsg, p.keys.Close):
			p.Close()
			return nil

		case key.Matches(kmsg, p.keys.CursorDown):
			if len(p.filtered) > 0 {
				p.cursor = (p.cursor + 1) % len(p.filtered)
			}
			return nil

		case key.Matches(kmsg, p.keys.CursorUp):
			if len(p.filtered) > 0 {
				p.cursor = (p.cursor - 1 + len(p.filtered)) % len(p.filtered)
			}
			return nil
		}
	}

	// Delegate typing to the text input, then re-filter.
	prevVal := p.input.Value()
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)

	if p.input.Value() != prevVal {
		p.filtered = p.registry.Filter(p.input.Value())
		p.cursor = 0
	}

	return cmd
}

// View renders the palette as a centered overlay.
func (p CommandPalette) View() string {
	if !p.active {
		return ""
	}

	// Determine box width: ~60% of terminal, clamped [50, 80].
	boxW := p.width * 60 / 100
	if boxW < 50 {
		boxW = 50
	}
	if boxW > 80 {
		boxW = 80
	}

	// Inner content width accounts for border (2) + padding (4).
	innerW := boxW - 6
	if innerW < 20 {
		innerW = 20
	}

	// Max visible rows: ~50% of terminal height, minus overhead for
	// input line (1) + separator (1) + border/padding (4).
	maxRows := p.height/2 - 6
	if maxRows < 3 {
		maxRows = 3
	}

	// Build the input section.
	p.input.SetWidth(innerW - 1) // account for prompt character
	inputLine := p.input.View()

	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Width(innerW).
		Render(strings.Repeat("─", innerW))

	// Build the command list.
	var listRows []string
	visible := p.filtered
	if len(visible) > maxRows {
		visible = visible[:maxRows]
	}

	// Column widths: name gets ~40%, description gets the rest.
	nameW := innerW * 40 / 100
	descW := innerW - nameW - 4 // 4 = cursor prefix (2) + gap (2)

	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("62"))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	for i, cmd := range visible {
		name := truncate(cmd.Name, nameW)
		desc := truncate(cmd.Description, descW)

		var row string
		if i == p.cursor {
			// Highlighted row: full-width background.
			padded := fmt.Sprintf("> %-*s  %-*s", nameW, name, descW, desc)
			row = selectedStyle.Width(innerW).Render(padded)
		} else {
			nameStr := normalStyle.Render(fmt.Sprintf("%-*s", nameW, name))
			descStr := dimStyle.Render(fmt.Sprintf("%-*s", descW, desc))
			row = fmt.Sprintf("  %s  %s", nameStr, descStr)
		}

		listRows = append(listRows, row)
	}

	if len(listRows) == 0 {
		listRows = append(listRows, dimStyle.Render("  no matching commands"))
	}

	content := inputLine + "\n" + separator + "\n" + strings.Join(listRows, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(boxW)

	dialog := box.Render(content)

	return lipgloss.Place(p.width, p.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}

// selectedName returns the Name of the currently highlighted command,
// or "" if the filtered list is empty.
func (p CommandPalette) selectedName() string {
	if len(p.filtered) == 0 {
		return ""
	}
	if p.cursor >= len(p.filtered) {
		return p.filtered[0].Name
	}
	return p.filtered[p.cursor].Name
}

// truncate shortens s to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
