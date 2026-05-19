package tui_test

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"pkg.akt.dev/akt/internal/tui"
)

func TestDefaultKeyMapReturnsNonNil(t *testing.T) {
	km := tui.DefaultKeyMap()
	// Verify the struct is populated by checking a few bindings are usable.
	if km.Quit.Keys() == nil {
		t.Error("Quit binding has nil keys")
	}
	if km.Help.Keys() == nil {
		t.Error("Help binding has nil keys")
	}
	if km.CursorUp.Keys() == nil {
		t.Error("CursorUp binding has nil keys")
	}
	if km.CursorDown.Keys() == nil {
		t.Error("CursorDown binding has nil keys")
	}
	if km.Command.Keys() == nil {
		t.Error("Command binding has nil keys")
	}
	if km.Back.Keys() == nil {
		t.Error("Back binding has nil keys")
	}
	if km.Select.Keys() == nil {
		t.Error("Select binding has nil keys")
	}
}

// bindingContains checks that a binding's Keys() slice contains the expected key.
func bindingContains(t *testing.T, name string, b key.Binding, wantKey string) {
	t.Helper()
	if !slices.Contains(b.Keys(), wantKey) {
		t.Errorf("%s: Keys() = %v, want to contain %q", name, b.Keys(), wantKey)
	}
}

func TestDefaultKeyMapBindings(t *testing.T) {
	km := tui.DefaultKeyMap()

	tests := []struct {
		name    string
		binding key.Binding
		wantKey string
	}{
		{"CursorDown/j", km.CursorDown, "j"},
		{"CursorDown/down", km.CursorDown, "down"},
		{"CursorUp/k", km.CursorUp, "k"},
		{"CursorUp/up", km.CursorUp, "up"},
		{"Help/?", km.Help, "?"},
		{"Command/:", km.Command, ":"},
		{"CommandSearch/ctrl+p", km.CommandSearch, "ctrl+p"},
		{"Quit/ctrl+c", km.Quit, "ctrl+c"},
		{"Back/esc", km.Back, "esc"},
		{"Select/enter", km.Select, "enter"},
		{"Deployments/1", km.Deployments, "1"},
		{"Deploy/D", km.Deploy, "D"},
		{"Search//", km.Search, "/"},
		{"Filter/f", km.Filter, "f"},
		{"TabNext/tab", km.TabNext, "tab"},
		{"Logs/l", km.Logs, "l"},
		{"Shell/s", km.Shell, "s"},
		{"Vote/v", km.Vote, "v"},
		{"Close/d", km.Close, "d"},
		{"Update/u", km.Update, "u"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bindingContains(t, tc.name, tc.binding, tc.wantKey)
		})
	}
}

func TestAllBindingsEnabled(t *testing.T) {
	km := tui.DefaultKeyMap()

	bindings := []struct {
		name    string
		binding key.Binding
	}{
		{"Quit", km.Quit},
		{"Command", km.Command},
		{"CommandSearch", km.CommandSearch},
		{"Help", km.Help},
		{"Back", km.Back},
		{"CursorUp", km.CursorUp},
		{"CursorDown", km.CursorDown},
		{"Select", km.Select},
		{"Deployments", km.Deployments},
		{"Leases", km.Leases},
		{"Providers", km.Providers},
		{"Monitor", km.Monitor},
		{"Governance", km.Governance},
		{"Staking", km.Staking},
		{"Close", km.Close},
		{"Update", km.Update},
		{"Logs", km.Logs},
		{"Shell", km.Shell},
		{"Vote", km.Vote},
		{"Deploy", km.Deploy},
		{"Filter", km.Filter},
		{"Search", km.Search},
		{"TabNext", km.TabNext},
	}

	for _, b := range bindings {
		t.Run(b.name, func(t *testing.T) {
			if !b.binding.Enabled() {
				t.Errorf("%s binding is disabled, want enabled", b.name)
			}
		})
	}
}

func TestKeyMapFromConfigNilViperReturnsDefaults(t *testing.T) {
	km := tui.KeyMapFromConfig(nil)
	def := tui.DefaultKeyMap()

	// Spot-check that nil Viper returns the same bindings as DefaultKeyMap.
	checks := []struct {
		name    string
		got     key.Binding
		want    key.Binding
		wantKey string
	}{
		{"CursorDown", km.CursorDown, def.CursorDown, "j"},
		{"CursorUp", km.CursorUp, def.CursorUp, "k"},
		{"Help", km.Help, def.Help, "?"},
		{"Command", km.Command, def.Command, ":"},
		{"Quit", km.Quit, def.Quit, "ctrl+c"},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !slices.Contains(tc.got.Keys(), tc.wantKey) {
				t.Errorf("KeyMapFromConfig(nil).%s missing key %q", tc.name, tc.wantKey)
			}
			if !slices.Contains(tc.want.Keys(), tc.wantKey) {
				t.Errorf("DefaultKeyMap().%s missing key %q (sanity check)", tc.name, tc.wantKey)
			}
		})
	}
}
