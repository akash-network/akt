package components_test

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/tui/components"
)

func TestProgressPrimitivesClampAndLabel(t *testing.T) {
	t.Parallel()

	zero := ansi.Strip(components.ProgressBar(-1, 12))
	full := ansi.Strip(components.ProgressBar(2, 12))
	if zero == full {
		t.Fatalf("clamped empty and full progress bars are identical: %q", zero)
	}
	if got := ansi.StringWidth(zero); got != 12 {
		t.Fatalf("empty progress width = %d, want 12", got)
	}
	if got := ansi.StringWidth(full); got != 12 {
		t.Fatalf("full progress width = %d, want 12", got)
	}

	labelled := ansi.Strip(components.ProgressBarWithLabel(0.5, 20, "10/20"))
	if !strings.Contains(labelled, "10/20") {
		t.Fatalf("labelled progress bar = %q, want embedded label", labelled)
	}
	if got := ansi.StringWidth(labelled); got != 20 {
		t.Fatalf("labelled progress width = %d, want 20", got)
	}
	if got := ansi.StringWidth(ansi.Strip(components.ProgressBarWithLabel(-1, 8, ""))); got != 8 {
		t.Fatalf("unlabelled clamped progress width = %d, want 8", got)
	}
	if got := ansi.StringWidth(ansi.Strip(components.ProgressBarWithLabel(2, 3, "long"))); got != 3 {
		t.Fatalf("oversized-label progress width = %d, want 3", got)
	}

	for _, pct := range []float64{0.666, 0.667, 1} {
		formatted := ansi.Strip(components.FormatPercent(pct))
		if !strings.Contains(formatted, "%") || ansi.StringWidth(formatted) != 6 {
			t.Fatalf("FormatPercent(%v) = %q, want six-column percentage", pct, formatted)
		}
	}
}

func TestSparklineNormalizesAndUsesNewestWindow(t *testing.T) {
	t.Parallel()

	if got := components.Sparkline(nil, 10, color.White); got != "" {
		t.Fatalf("Sparkline(nil) = %q, want empty", got)
	}
	if got := components.Sparkline([]float64{1}, 0, color.White); got != "" {
		t.Fatalf("Sparkline(width=0) = %q, want empty", got)
	}

	plain := ansi.Strip(components.Sparkline([]float64{-50, 1, 2, 3}, 3, color.White))
	if got, want := plain, "▁▄█"; got != want {
		t.Fatalf("rightmost normalized sparkline = %q, want %q", got, want)
	}
	equal := ansi.Strip(components.Sparkline([]float64{7, 7, 7}, 3, color.White))
	if got, want := equal, "▁▁▁"; got != want {
		t.Fatalf("equal-value sparkline = %q, want %q", got, want)
	}
}

func TestPanelHeightAndLineCounting(t *testing.T) {
	t.Parallel()

	if got := components.ContentLineCount(""); got != 1 {
		t.Fatalf("ContentLineCount(empty) = %d, want one renderable line", got)
	}
	if got := components.ContentLineCount("one\ntwo"); got != 2 {
		t.Fatalf("ContentLineCount(two lines) = %d, want 2", got)
	}

	content := "one\ntwo"
	auto := components.TitledPanel("STATUS", content, 30)
	fixed := components.TitledPanelHeight("STATUS", content, 30, 8)
	plainAuto := ansi.Strip(auto)
	if !strings.Contains(plainAuto, "STATUS") ||
		!strings.Contains(plainAuto, "one") || !strings.Contains(plainAuto, "two") {
		t.Fatalf("auto-height panel omitted title/content: %q", ansi.Strip(auto))
	}
	if ansi.StringWidth(strings.Split(ansi.Strip(fixed), "\n")[0]) != 30 {
		t.Fatalf("fixed panel does not honor requested outer width: %q", ansi.Strip(fixed))
	}
}

func TestResourceTableWidthAndNarrowFlexibleColumns(t *testing.T) {
	t.Parallel()

	table := components.NewResourceTable(components.ResourceTableConfig{Columns: []components.TableColumn{
		{Header: "A", Width: 0},
		{Header: "B", Width: 0},
	}})
	table.SetSize(12, 8)
	if got := table.Width(); got != 12 {
		t.Fatalf("Width() = %d, want 12", got)
	}
	table.SetRows([]components.TableRow{{Cells: []string{"alphabet", "z"}}})
	plain := ansi.Strip(table.View())
	if !strings.Contains(plain, "alphabet") {
		t.Fatalf("narrow flexible table omitted its row: %q", plain)
	}
	if got := ansi.StringWidth(strings.Split(plain, "\n")[0]); got != 20 {
		t.Fatalf("narrow table render width = %d, want safe minimum 20", got)
	}
}
