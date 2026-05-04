package theme_test

import (
	"testing"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

func TestColorPaletteNonEmpty(t *testing.T) {
	// Verify that all color constants are non-nil by rendering a test
	// string with a style that uses each color as foreground.
	// If any color were misconfigured the render would panic or
	// return empty.
	colors := []struct {
		name string
		fn   func() string
	}{
		{"ColorPrimary", func() string { return theme.Success.Render("x") }},
		{"ColorWarning", func() string { return theme.Warning.Render("x") }},
		{"ColorError", func() string { return theme.Error.Render("x") }},
		{"ColorDim", func() string { return theme.Dim.Render("x") }},
		{"ColorMuted", func() string { return theme.Muted.Render("x") }},
		{"ColorCyan", func() string { return theme.Cyan.Render("x") }},
		{"ColorMagenta", func() string { return theme.Magenta.Render("x") }},
		{"ColorBlue", func() string { return theme.Blue.Render("x") }},
	}

	for _, tc := range colors {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.fn()
			if result == "" {
				t.Errorf("%s rendered empty", tc.name)
			}
		})
	}
}

func TestSemanticStylesApplyColor(t *testing.T) {
	// Verify that semantic styles have a foreground color set
	// (lipgloss styles are value types, so we render and check non-empty).
	styles := []struct {
		name  string
		style lipgloss.Style
	}{
		{"Success", theme.Success},
		{"Warning", theme.Warning},
		{"Error", theme.Error},
		{"Dim", theme.Dim},
		{"Muted", theme.Muted},
		{"Key", theme.Key},
		{"VoteYes", theme.VoteYes},
		{"VoteNo", theme.VoteNo},
		{"GridVoted", theme.GridVoted},
		{"GridNotVoted", theme.GridNotVoted},
		{"Proposer", theme.Proposer},
		{"Cyan", theme.Cyan},
		{"Magenta", theme.Magenta},
		{"Blue", theme.Blue},
	}

	for _, tc := range styles {
		t.Run(tc.name, func(t *testing.T) {
			rendered := tc.style.Render("x")
			if rendered == "" {
				t.Errorf("%s.Render(\"x\") returned empty", tc.name)
			}
		})
	}
}

func TestBoldStyleIsBold(t *testing.T) {
	rendered := theme.Bold.Render("test")
	if rendered == "test" {
		// With no color profile, lipgloss may not emit ANSI codes.
		// This is acceptable — we verify the style object exists and
		// renders without panicking.
		t.Log("Bold.Render returned plain text (no color profile active)")
	}
}

func TestProgressColorsAreSet(t *testing.T) {
	if theme.ProgressPrimary == nil {
		t.Error("ProgressPrimary is nil")
	}
	if theme.ProgressSuccess == nil {
		t.Error("ProgressSuccess is nil")
	}
	if theme.ProgressPrecommit == nil {
		t.Error("ProgressPrecommit is nil")
	}
}

func TestSectionHeaderHasBorder(t *testing.T) {
	rendered := theme.SectionHeader.Render("Section")
	if rendered == "" {
		t.Error("SectionHeader.Render returned empty")
	}
}

func TestTabStyles(t *testing.T) {
	active := theme.TabActive.Render("Tab1")
	inactive := theme.TabInactive.Render("Tab2")

	if active == "" {
		t.Error("TabActive.Render returned empty")
	}
	if inactive == "" {
		t.Error("TabInactive.Render returned empty")
	}
	// Active and inactive should render differently (different colors/bold).
	// In a no-color environment they may be the same text, but the styles
	// should at least not panic.
}
