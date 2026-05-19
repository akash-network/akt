package views

import (
	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/key"

	"pkg.akt.dev/akt/internal/tui/commands"
)

// CommandSubmitMsg is returned when the user selects a command from the palette.
type CommandSubmitMsg struct {
	Value string
}

// PaletteKeys holds the key bindings used by the command palette.
type PaletteKeys struct {
	CursorUp   key.Binding
	CursorDown key.Binding
	Select     key.Binding
	Close      key.Binding
}

// Palette is the command palette overlay (stub — will be fully implemented in Task 10).
// It is also aliased as CommandPalette for backward compatibility with tests.
type Palette = CommandPalette

// CommandPalette is the command palette overlay.
type CommandPalette struct {
	active   bool
	width    int
	height   int
	cursor   int
	input    string
	keys     PaletteKeys
	registry *commands.Registry
}

// NewCommandPalette creates a new command palette.
func NewCommandPalette(reg *commands.Registry, keys PaletteKeys) CommandPalette {
	return CommandPalette{
		registry: reg,
		keys:     keys,
	}
}

// Active returns whether the palette is visible.
func (p *CommandPalette) Active() bool {
	return p.active
}

// Open shows the palette and resets input.
func (p *CommandPalette) Open() {
	p.active = true
	p.input = ""
	p.cursor = 0
}

// Close hides the palette.
func (p *CommandPalette) Close() {
	p.active = false
}

// SetSize updates the available dimensions.
func (p *CommandPalette) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// Update handles key messages when the palette is active.
func (p *CommandPalette) Update(msg tea.Msg) tea.Cmd {
	kmsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}

	switch {
	case key.Matches(kmsg, p.keys.Close):
		p.Close()
		return nil
	case key.Matches(kmsg, p.keys.Select):
		p.Close()
		items := p.filtered()
		if len(items) == 0 {
			return nil
		}
		selected := items[p.cursor%len(items)]
		return func() tea.Msg {
			return CommandSubmitMsg{Value: selected.Name}
		}
	case key.Matches(kmsg, p.keys.CursorUp):
		if p.cursor > 0 {
			p.cursor--
		} else {
			items := p.filtered()
			if len(items) > 0 {
				p.cursor = len(items) - 1
			}
		}
		return nil
	case key.Matches(kmsg, p.keys.CursorDown):
		items := p.filtered()
		if len(items) > 0 {
			p.cursor = (p.cursor + 1) % len(items)
		}
		return nil
	default:
		// Text input
		if kmsg.Text != "" {
			p.input += kmsg.Text
			p.cursor = 0
		}
		return nil
	}
}

// filtered returns commands matching the current input.
func (p *CommandPalette) filtered() []commands.Command {
	if p.registry == nil {
		return nil
	}
	all := p.registry.All()
	if p.input == "" {
		return all
	}
	var out []commands.Command
	for _, cmd := range all {
		if containsFold(cmd.Name, p.input) || containsFold(cmd.Description, p.input) {
			out = append(out, cmd)
		}
	}
	return out
}

// containsFold reports whether s contains substr (case-insensitive).
func containsFold(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if eqFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func eqFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// View renders the palette overlay.
func (p *CommandPalette) View() tea.View {
	if !p.active {
		return tea.NewView("")
	}

	items := p.filtered()
	if len(items) == 0 {
		return tea.NewView("  : " + p.input + "\n  no matching commands")
	}

	var lines []string
	lines = append(lines, "  : "+p.input)
	for i, cmd := range items {
		prefix := "  "
		if i == p.cursor%len(items) {
			prefix = "> "
		}
		lines = append(lines, prefix+cmd.Name+" — "+cmd.Description)
	}
	return tea.NewView(joinLines(lines))
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}
