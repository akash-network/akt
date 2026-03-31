package views

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommandSubmitMsg is sent when the user presses Enter in the command input.
type CommandSubmitMsg struct {
	Value string
}

// CommandInput is a vim-style : command input rendered at the bottom of the screen.
type CommandInput struct {
	input  textinput.Model
	width  int
	active bool
}

// NewCommandInput returns a new command input.
func NewCommandInput() CommandInput {
	ti := textinput.New()
	ti.Prompt = ":"
	ti.CharLimit = 128
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	return CommandInput{
		input: ti,
	}
}

// Active returns whether the command input is visible.
func (c CommandInput) Active() bool {
	return c.active
}

// Open shows the command input and focuses it.
func (c *CommandInput) Open() {
	c.active = true
	c.input.SetValue("")
	c.input.Focus()
}

// Close hides the command input.
func (c *CommandInput) Close() {
	c.active = false
	c.input.Blur()
}

// SetWidth updates the input width.
func (c *CommandInput) SetWidth(w int) {
	c.width = w
	c.input.Width = w - 2 // account for prompt and padding
}

// Update handles input events. Returns a tea.Cmd; the caller should check
// for CommandSubmitMsg to process the entered command.
func (c *CommandInput) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			val := c.input.Value()
			c.Close()
			if val != "" {
				return func() tea.Msg {
					return CommandSubmitMsg{Value: val}
				}
			}
			return nil
		case "esc":
			c.Close()
			return nil
		}
	}

	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return cmd
}

// View renders the command input as a single line at the bottom.
func (c CommandInput) View() string {
	if !c.active {
		return ""
	}

	style := lipgloss.NewStyle().
		Width(c.width)

	return style.Render(c.input.View())
}
