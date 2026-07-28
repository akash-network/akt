package views_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/tui/views"
)

func TestNewHelpOverlayInactive(t *testing.T) {
	h := views.NewHelpOverlay()
	if h.Active() {
		t.Error("NewHelpOverlay() should be inactive")
	}
}

func TestHelpOverlayOpenCloseLifecycle(t *testing.T) {
	h := views.NewHelpOverlay()

	h.Open("Deployments")
	if !h.Active() {
		t.Error("Active() = false after Open, want true")
	}

	h.Close()
	if h.Active() {
		t.Error("Active() = true after Close, want false")
	}
}

func TestHelpOverlayInactiveViewEmpty(t *testing.T) {
	h := views.NewHelpOverlay()
	if out := h.View().Content; out != "" {
		t.Errorf("inactive View() = %q, want empty string", out)
	}
}

func TestHelpOverlayViewContainsSectionHeaders(t *testing.T) {
	h := views.NewHelpOverlay()
	h.Open("Dashboard")
	h.SetSize(120, 60)

	out := ansi.Strip(h.View().Content)

	sections := []string{"NAVIGATION", "LISTS", "ACTIONS", "OVERLAYS"}
	for _, section := range sections {
		if !strings.Contains(out, section) {
			t.Errorf("View() missing section header %q", section)
		}
	}
}

func TestHelpOverlayViewContainsKeybindings(t *testing.T) {
	h := views.NewHelpOverlay()
	h.Open("Dashboard")
	h.SetSize(120, 60)

	out := ansi.Strip(h.View().Content)

	// Spot-check some keybindings from each section.
	bindings := []string{
		"Switch views",    // Navigation
		"Next item",       // Lists
		"New deployment",  // Actions
		"Command palette", // Overlays
	}
	for _, b := range bindings {
		if !strings.Contains(out, b) {
			t.Errorf("View() missing keybinding description %q", b)
		}
	}
}

func TestHelpOverlayViewContainsCloseHint(t *testing.T) {
	h := views.NewHelpOverlay()
	h.Open("")
	h.SetSize(120, 60)

	out := ansi.Strip(h.View().Content)
	if !strings.Contains(out, "esc") {
		t.Error("View() missing close hint with 'esc'")
	}
}

func TestHelpOverlayViewContainsContext(t *testing.T) {
	h := views.NewHelpOverlay()
	h.Open("Providers")
	h.SetSize(120, 60)

	out := ansi.Strip(h.View().Content)
	if !strings.Contains(out, "Providers") {
		t.Error("View() missing context label 'Providers'")
	}
}
