package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the global keybindings for the TUI.
type KeyMap struct {
	Quit          key.Binding
	Query         key.Binding
	Tx            key.Binding
	Top           key.Binding
	CommandSearch key.Binding
	Command       key.Binding
	Help          key.Binding
	Back          key.Binding
}

// DefaultKeyMap returns the default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Query: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "query"),
		),
		Tx: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "tx"),
		),
		Top: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "top"),
		),
		CommandSearch: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "search"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
	}
}
