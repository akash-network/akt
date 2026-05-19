package views_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/tui/views"
)

func TestDetailViewEmpty(t *testing.T) {
	dv := views.NewDetailView()

	if dv.HasContent() {
		t.Error("HasContent() = true for new DetailView, want false")
	}
}

func TestDetailViewSetContent(t *testing.T) {
	dv := views.NewDetailView()
	dv.SetContent("Deployment #100", "Status: active\nProvider: akash1abc")
	dv.SetSize(80, 24)

	if !dv.HasContent() {
		t.Error("HasContent() = false after SetContent, want true")
	}

	out := dv.View()
	plain := ansi.Strip(out)

	if !strings.Contains(plain, "Status: active") {
		t.Error("View() missing content 'Status: active'")
	}
	if !strings.Contains(plain, "Provider: akash1abc") {
		t.Error("View() missing content 'Provider: akash1abc'")
	}
	if !strings.Contains(plain, "Deployment #100") {
		t.Error("View() missing title 'Deployment #100'")
	}
}

func TestDetailViewClear(t *testing.T) {
	dv := views.NewDetailView()
	dv.SetContent("Title", "Some content")

	dv.Clear()

	if dv.HasContent() {
		t.Error("HasContent() = true after Clear, want false")
	}
}

func TestDetailViewScroll(t *testing.T) {
	dv := views.NewDetailView()

	// Build 50-line content.
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf("Line %d", i))
	}
	dv.SetContent("Long Content", strings.Join(lines, "\n"))
	dv.SetSize(80, 10)

	// Scroll down many times — should not panic.
	for i := 0; i < 60; i++ {
		dv.ScrollDown()
	}
	out := dv.View()
	if out == "" {
		t.Error("View() returned empty string after scrolling down")
	}

	// Scroll up many times — should not panic.
	for i := 0; i < 70; i++ {
		dv.ScrollUp()
	}
	out = dv.View()
	if out == "" {
		t.Error("View() returned empty string after scrolling up")
	}
}

func TestDetailViewBackHint(t *testing.T) {
	dv := views.NewDetailView()
	dv.SetContent("Title", "Body text")
	dv.SetSize(80, 24)

	out := dv.View()
	plain := ansi.Strip(out)

	if !strings.Contains(plain, "esc") {
		t.Error("View() missing back hint containing 'esc'")
	}
}
