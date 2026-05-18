package theme_test

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

func TestZincScaleColors(t *testing.T) {
	colors := []struct {
		name  string
		color color.Color
	}{
		{"Slate950", theme.Slate950},
		{"Slate900", theme.Slate900},
		{"Slate800", theme.Slate800},
		{"Slate700", theme.Slate700},
		{"Slate600", theme.Slate600},
		{"Slate500", theme.Slate500},
		{"Slate400", theme.Slate400},
		{"Slate300", theme.Slate300},
		{"Slate200", theme.Slate200},
		{"Slate100", theme.Slate100},
		{"Slate50", theme.Slate50},
	}
	for _, tc := range colors {
		t.Run(tc.name, func(t *testing.T) {
			if tc.color == nil {
				t.Errorf("%s is nil", tc.name)
			}
		})
	}
}

func TestAccentColors(t *testing.T) {
	colors := []struct {
		name  string
		color color.Color
	}{
		{"AccentRed", theme.AccentRed},
		{"RedDim", theme.RedDim},
		{"RedBg", theme.RedBg},
	}
	for _, tc := range colors {
		t.Run(tc.name, func(t *testing.T) {
			if tc.color == nil {
				t.Errorf("%s is nil", tc.name)
			}
		})
	}
}

func TestStateColors(t *testing.T) {
	colors := []struct {
		name  string
		color color.Color
	}{
		{"GreenColor", theme.GreenColor},
		{"GreenDim", theme.GreenDim},
		{"YellowColor", theme.YellowColor},
		{"YellowDim", theme.YellowDim},
		{"BlueColor", theme.BlueColor},
		{"PurpleColor", theme.PurpleColor},
	}
	for _, tc := range colors {
		t.Run(tc.name, func(t *testing.T) {
			if tc.color == nil {
				t.Errorf("%s is nil", tc.name)
			}
		})
	}
}

func TestTypographyStyles(t *testing.T) {
	styles := []struct {
		name  string
		style lipgloss.Style
	}{
		{"Heading", theme.Heading},
		{"PrimaryValue", theme.PrimaryValue},
		{"Body", theme.Body},
		{"Secondary", theme.Secondary},
		{"Muted", theme.Muted},
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

func TestBackwardCompatColors(t *testing.T) {
	// All old color names must still compile and be non-nil.
	colors := []struct {
		name  string
		color color.Color
	}{
		{"ColorPrimary", theme.ColorPrimary},
		{"ColorAccent", theme.ColorAccent},
		{"ColorSuccess", theme.ColorSuccess},
		{"ColorWarning", theme.ColorWarning},
		{"ColorError", theme.ColorError},
		{"ColorText", theme.ColorText},
		{"ColorBrightText", theme.ColorBrightText},
		{"ColorMuted", theme.ColorMuted},
		{"ColorDim", theme.ColorDim},
		{"ColorBorder", theme.ColorBorder},
		{"ColorHighlight", theme.ColorHighlight},
		{"ColorCyan", theme.ColorCyan},
		{"ColorMagenta", theme.ColorMagenta},
		{"ColorBlue", theme.ColorBlue},
	}
	for _, tc := range colors {
		t.Run(tc.name, func(t *testing.T) {
			if tc.color == nil {
				t.Errorf("%s is nil", tc.name)
			}
		})
	}
}

func TestBackwardCompatStyles(t *testing.T) {
	// All old style names must still compile and render.
	styles := []struct {
		name  string
		style lipgloss.Style
	}{
		{"Bold", theme.Bold},
		{"Dim", theme.Dim},
		{"Faint", theme.Faint},
		{"Muted", theme.Muted},
		{"Section", theme.Section},
		{"Success", theme.Success},
		{"Warning", theme.Warning},
		{"Error", theme.Error},
		{"Green", theme.Green},
		{"Yellow", theme.Yellow},
		{"Red", theme.Red},
		{"Gray", theme.Gray},
		{"Cyan", theme.Cyan},
		{"Magenta", theme.Magenta},
		{"Blue", theme.Blue},
		{"Key", theme.Key},
		{"Label", theme.Label},
		{"Value", theme.Value},
		{"Header", theme.Header},
		{"SectionHeader", theme.SectionHeader},
		{"Title", theme.Title},
		{"TabActive", theme.TabActive},
		{"TabInactive", theme.TabInactive},
		{"Highlight", theme.Highlight},
		{"VoteYes", theme.VoteYes},
		{"VoteNo", theme.VoteNo},
		{"GridVoted", theme.GridVoted},
		{"GridNotVoted", theme.GridNotVoted},
		{"Proposer", theme.Proposer},
		{"Moniker", theme.Moniker},
		{"DetailHeader", theme.DetailHeader},
		{"DetailLabel", theme.DetailLabel},
		{"DetailValue", theme.DetailValue},
		{"PercentHigh", theme.PercentHigh},
		{"PercentLow", theme.PercentLow},
		{"StatusBar", theme.StatusBar},
		{"HelpBar", theme.HelpBar},
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

func TestBackwardCompatProgressColors(t *testing.T) {
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

func TestSpinnerStyleExists(t *testing.T) {
	rendered := theme.SpinnerStyle.Render("x")
	if rendered == "" {
		t.Error("SpinnerStyle.Render returned empty")
	}
}

func TestHRule(t *testing.T) {
	result := theme.HRule(10)
	if result == "" {
		t.Error("HRule(10) returned empty")
	}
}

func TestNewComponentStyles(t *testing.T) {
	// Verify key new styles from the design system exist and render.
	styles := []struct {
		name  string
		style lipgloss.Style
	}{
		{"HeaderStyle", theme.HeaderStyle},
		{"HeaderAppName", theme.HeaderAppName},
		{"HeaderContext", theme.HeaderContext},
		{"HeaderMeta", theme.HeaderMeta},
		{"HeaderValue", theme.HeaderValue},
		{"SyncOK", theme.SyncOK},
		{"NavTabActive", theme.NavTabActive},
		{"NavTabInactive", theme.NavTabInactive},
		{"BreadcrumbSegment", theme.BreadcrumbSegment},
		{"BreadcrumbActive", theme.BreadcrumbActive},
		{"BreadcrumbSeparator", theme.BreadcrumbSeparator},
		{"FooterKey", theme.FooterKey},
		{"FooterDesc", theme.FooterDesc},
		{"TableHeader", theme.TableHeader},
		{"TableRow", theme.TableRow},
		{"TableRowSelected", theme.TableRowSelected},
		{"TableCursor", theme.TableCursor},
		{"SectionTitle", theme.SectionTitle},
		{"SectionRule", theme.SectionRule},
		{"KVLabel", theme.KVLabel},
		{"KVValue", theme.KVValue},
		{"PanelBorder", theme.PanelBorder},
		{"PanelBg", theme.PanelBg},
		{"StateGreen", theme.StateGreen},
		{"StateYellow", theme.StateYellow},
		{"StateClosed", theme.StateClosed},
		{"OverlayBg", theme.OverlayBg},
		{"OverlayBorder", theme.OverlayBorder},
		{"ButtonPrimary", theme.ButtonPrimary},
		{"ButtonSecondary", theme.ButtonSecondary},
		{"BarFilled", theme.BarFilled},
		{"BarEmpty", theme.BarEmpty},
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
