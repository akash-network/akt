package tui

import (
	"charm.land/bubbles/v2/key"
	"github.com/spf13/viper"
)

// KeyMap defines all configurable keybindings for the TUI.
// The set of bindings matches the names in tui.custom-keybindings
// (see SPEC.md section 8.6).
type KeyMap struct {
	// Global
	Quit          key.Binding
	Command       key.Binding // : — opens command palette
	CommandSearch key.Binding // ctrl+p — also opens command palette
	Help          key.Binding
	Back          key.Binding

	// Navigation (lists, palette, detail views)
	CursorUp   key.Binding
	CursorDown key.Binding
	Select     key.Binding

	// Primary view shortcuts (per design: 1-6)
	Deployments key.Binding
	Leases      key.Binding
	Providers   key.Binding
	Monitor     key.Binding
	Governance  key.Binding
	Staking     key.Binding
}

// DefaultKeyMap returns the vim-style default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command palette"),
		),
		CommandSearch: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "command palette"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		CursorUp: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "up"),
		),
		CursorDown: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Deployments: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "deployments"),
		),
		Leases: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "leases"),
		),
		Providers: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "providers"),
		),
		Monitor: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "monitor"),
		),
		Governance: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "governance"),
		),
		Staking: key.NewBinding(
			key.WithKeys("6"),
			key.WithHelp("6", "staking"),
		),
	}
}

// KeyMapFromConfig builds a KeyMap from Viper config. It starts from the
// vim-style defaults and applies overrides when tui.keybindings is "custom".
func KeyMapFromConfig(v *viper.Viper) KeyMap {
	km := DefaultKeyMap()

	if v == nil || v.GetString("tui.keybindings") != "custom" {
		return km
	}

	// configKey maps a config name (tui.custom-keybindings.<name>) to a
	// pointer to the binding it controls plus the help text to preserve.
	type entry struct {
		binding *key.Binding
		help    string
	}

	entries := map[string]entry{
		"quit":            {&km.Quit, "quit"},
		"command-palette": {&km.Command, "command palette"},
		"help":            {&km.Help, "help"},
		"back":            {&km.Back, "back"},
		"cursor-up":       {&km.CursorUp, "up"},
		"cursor-down":     {&km.CursorDown, "down"},
		"select":          {&km.Select, "select"},
		"deployments":     {&km.Deployments, "deployments"},
		"leases":          {&km.Leases, "leases"},
		"providers":       {&km.Providers, "providers"},
		"monitor":         {&km.Monitor, "monitor"},
		"governance":      {&km.Governance, "governance"},
		"staking":         {&km.Staking, "staking"},
	}

	for name, e := range entries {
		keys := v.GetStringSlice("tui.custom-keybindings." + name)
		if len(keys) == 0 {
			continue
		}
		*e.binding = key.NewBinding(
			key.WithKeys(keys...),
			key.WithHelp(keys[0], e.help),
		)
	}

	// command-palette override applies to both : and ctrl+p triggers.
	// If the user only overrides command-palette, copy the keys to
	// CommandSearch so both triggers stay in sync.
	if keys := v.GetStringSlice("tui.custom-keybindings.command-palette"); len(keys) > 0 {
		km.CommandSearch = key.NewBinding(
			key.WithKeys(keys...),
			key.WithHelp(keys[0], "command palette"),
		)
	}

	return km
}
