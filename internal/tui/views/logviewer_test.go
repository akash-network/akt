package views_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/tui/views"
)

func TestNewLogViewerInactive(t *testing.T) {
	lv := views.NewLogViewer()
	if lv.Active() {
		t.Error("NewLogViewer() should be inactive")
	}
}

func TestLogViewerOpenCloseLifecycle(t *testing.T) {
	lv := views.NewLogViewer()

	lv.Open("my-deploy", "12345", "web")
	if !lv.Active() {
		t.Error("Active() = false after Open, want true")
	}

	lv.Close()
	if lv.Active() {
		t.Error("Active() = true after Close, want false")
	}
}

func TestLogViewerAppendLine(t *testing.T) {
	lv := views.NewLogViewer()
	lv.Open("deploy", "100", "svc")
	lv.SetSize(80, 40)

	lv.AppendLine(views.LogLine{
		Timestamp: "12:00:00",
		Level:     "INFO",
		Scope:     "web",
		Message:   "Server started",
	})
	lv.AppendLine(views.LogLine{
		Timestamp: "12:00:01",
		Level:     "WARN",
		Scope:     "web",
		Message:   "High memory usage",
	})

	out := lv.View()
	if out == "" {
		t.Fatal("View() returned empty string for active viewer with lines")
	}

	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Server started") {
		t.Error("View() missing log message 'Server started'")
	}
	if !strings.Contains(plain, "High memory usage") {
		t.Error("View() missing log message 'High memory usage'")
	}
}

func TestLogViewerTogglePause(t *testing.T) {
	lv := views.NewLogViewer()
	lv.Open("deploy", "100", "svc")
	lv.SetSize(80, 40)

	// Initially not paused — View should show "streaming".
	out := ansi.Strip(lv.View())
	if !strings.Contains(out, "streaming") {
		t.Error("View() should show 'streaming' when not paused")
	}

	// Toggle pause.
	lv.TogglePause()
	out = ansi.Strip(lv.View())
	if !strings.Contains(out, "paused") {
		t.Error("View() should show 'paused' after TogglePause")
	}

	// Toggle again — back to streaming.
	lv.TogglePause()
	out = ansi.Strip(lv.View())
	if !strings.Contains(out, "streaming") {
		t.Error("View() should show 'streaming' after second TogglePause")
	}
}

func TestLogViewerClear(t *testing.T) {
	lv := views.NewLogViewer()
	lv.Open("deploy", "100", "svc")
	lv.SetSize(80, 40)

	lv.AppendLine(views.LogLine{
		Timestamp: "12:00:00",
		Level:     "INFO",
		Scope:     "web",
		Message:   "Should be cleared",
	})

	lv.Clear()

	out := ansi.Strip(lv.View())
	if strings.Contains(out, "Should be cleared") {
		t.Error("View() still contains cleared message")
	}
}

func TestLogViewerInactiveViewEmpty(t *testing.T) {
	lv := views.NewLogViewer()
	if out := lv.View(); out != "" {
		t.Errorf("inactive View() = %q, want empty string", out)
	}
}

func TestLogViewerViewContainsDeploymentInfo(t *testing.T) {
	lv := views.NewLogViewer()
	lv.Open("my-app", "99999", "api")
	lv.SetSize(80, 40)

	out := ansi.Strip(lv.View())
	if !strings.Contains(out, "my-app") {
		t.Error("View() missing deployment title 'my-app'")
	}
	if !strings.Contains(out, "99999") {
		t.Error("View() missing dseq '99999'")
	}
	if !strings.Contains(out, "api") {
		t.Error("View() missing service name 'api'")
	}
}

func TestLogViewerAppendLines(t *testing.T) {
	lv := views.NewLogViewer()
	lv.Open("deploy", "100", "svc")
	lv.SetSize(80, 40)

	lines := []views.LogLine{
		{Timestamp: "12:00:00", Level: "INFO", Scope: "web", Message: "Line one"},
		{Timestamp: "12:00:01", Level: "INFO", Scope: "web", Message: "Line two"},
		{Timestamp: "12:00:02", Level: "ERR", Scope: "web", Message: "Line three"},
	}
	lv.AppendLines(lines)

	out := ansi.Strip(lv.View())
	for _, l := range lines {
		if !strings.Contains(out, l.Message) {
			t.Errorf("View() missing message %q", l.Message)
		}
	}
}
