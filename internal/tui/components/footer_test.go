package components_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/tui/components"
)

func TestFooterHints(t *testing.T) {
	hints := []components.HintPair{
		{Key: "q", Desc: "quit"},
		{Key: "?", Desc: "help"},
	}
	out := components.FooterHints(hints)
	if out == "" {
		t.Fatal("FooterHints returned empty string")
	}
	if !strings.Contains(out, "q") {
		t.Error("output missing key 'q'")
	}
	if !strings.Contains(out, "quit") {
		t.Error("output missing desc 'quit'")
	}
	if !strings.Contains(out, "?") {
		t.Error("output missing key '?'")
	}
	if !strings.Contains(out, "help") {
		t.Error("output missing desc 'help'")
	}
}

func TestFooterHintsWithAccent(t *testing.T) {
	hints := []components.HintPair{
		{Key: "esc", Desc: "back", Accent: true},
	}
	out := components.FooterHints(hints)
	if out == "" {
		t.Fatal("FooterHints with accent returned empty string")
	}
	if !strings.Contains(out, "esc") {
		t.Error("output missing accent key 'esc'")
	}
	if !strings.Contains(out, "back") {
		t.Error("output missing accent desc 'back'")
	}
}

func TestHRule(t *testing.T) {
	const width = 40
	out := components.HRule(width)
	if out == "" {
		t.Fatal("HRule returned empty string")
	}
	// Use lipgloss to measure the printed (ANSI-stripped) width of the output.
	if w := lipgloss.Width(out); w != width {
		t.Errorf("HRule visible width = %d, want %d", w, width)
	}
}

func TestFooter(t *testing.T) {
	hints := []components.HintPair{
		{Key: "q", Desc: "quit"},
	}
	out := components.Footer(60, hints)
	if out == "" {
		t.Fatal("Footer returned empty string")
	}
	if !strings.Contains(out, "\n") {
		t.Error("Footer missing newline between rule and hints")
	}
	if !strings.Contains(out, "q") {
		t.Error("Footer missing key")
	}
	if !strings.Contains(out, "quit") {
		t.Error("Footer missing desc")
	}
}
