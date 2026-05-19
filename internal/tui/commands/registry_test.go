package commands_test

import (
	"testing"

	"pkg.akt.dev/akt/internal/tui/commands"
)

func TestDefaultRegistryReturnsRegistry(t *testing.T) {
	r := commands.DefaultRegistry()
	if r == nil {
		t.Fatal("DefaultRegistry() returned nil")
	}
}

func TestDefaultRegistryAllReturnsCommands(t *testing.T) {
	r := commands.DefaultRegistry()
	all := r.All()
	if len(all) == 0 {
		t.Fatal("All() returned empty slice")
	}
	// DefaultRegistry registers 20 commands (11 nav + 4 action + 3 context + 2 app).
	if got := len(all); got != 20 {
		t.Errorf("All() returned %d commands, want 20", got)
	}
}

func TestFilterEmptyReturnsAll(t *testing.T) {
	r := commands.DefaultRegistry()
	all := r.All()
	filtered := r.Filter("")
	if len(filtered) != len(all) {
		t.Errorf("Filter(\"\") returned %d commands, want %d", len(filtered), len(all))
	}
}

func TestFilterMatchesByNameCaseInsensitive(t *testing.T) {
	r := commands.DefaultRegistry()

	// "dep" should match: Deployments (name), Deploy (name), Deploy from SDL (name).
	got := r.Filter("dep")
	if len(got) == 0 {
		t.Fatal("Filter(\"dep\") returned no results")
	}

	// Verify case-insensitive: uppercase should match the same set.
	gotUpper := r.Filter("DEP")
	if len(gotUpper) != len(got) {
		t.Errorf("Filter(\"DEP\") returned %d, Filter(\"dep\") returned %d — should be equal",
			len(gotUpper), len(got))
	}
}

func TestFilterNonexistentReturnsEmpty(t *testing.T) {
	r := commands.DefaultRegistry()
	got := r.Filter("nonexistent")
	if len(got) != 0 {
		t.Errorf("Filter(\"nonexistent\") returned %d commands, want 0", len(got))
	}
}

func TestFilterMatchesViaAlias(t *testing.T) {
	r := commands.DefaultRegistry()

	// "monitor" is an alias for the Monitor command.
	got := r.Filter("monitor")
	if len(got) == 0 {
		t.Fatal("Filter(\"monitor\") returned no results, expected match via alias")
	}
	found := false
	for _, cmd := range got {
		if cmd.Name == "Monitor" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Filter(\"monitor\") did not return the Monitor command")
	}

	// "prov" is an alias for the Providers command.
	got = r.Filter("prov")
	if len(got) == 0 {
		t.Fatal("Filter(\"prov\") returned no results, expected match via alias")
	}
	found = false
	for _, cmd := range got {
		if cmd.Name == "Providers" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Filter(\"prov\") did not return the Providers command")
	}
}

func TestAllCategoriesPresent(t *testing.T) {
	r := commands.DefaultRegistry()
	all := r.All()

	categories := make(map[string]bool)
	for _, cmd := range all {
		categories[cmd.Category] = true
	}

	for _, want := range []string{"navigation", "action", "context", "app"} {
		if !categories[want] {
			t.Errorf("category %q not found in registered commands", want)
		}
	}
}

func TestNewRegistryIsEmpty(t *testing.T) {
	r := commands.NewRegistry()
	if got := len(r.All()); got != 0 {
		t.Errorf("NewRegistry().All() returned %d commands, want 0", got)
	}
}

func TestRegisterAddsCommand(t *testing.T) {
	r := commands.NewRegistry()
	r.Register(commands.Command{
		Name:        "Test",
		Description: "A test command",
		Category:    "test",
		Aliases:     []string{"tst"},
	})
	all := r.All()
	if len(all) != 1 {
		t.Fatalf("All() returned %d commands, want 1", len(all))
	}
	if all[0].Name != "Test" {
		t.Errorf("command Name = %q, want %q", all[0].Name, "Test")
	}
}
