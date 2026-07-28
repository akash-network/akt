package views

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/ui/theme"
)

// maxLogLines is the maximum number of log lines retained in the viewer.
const maxLogLines = 500

// LogLine represents a single parsed log entry.
type LogLine struct {
	Timestamp string
	Level     string // "INFO", "WARN", "ERR"
	Scope     string // service/component name
	Message   string
}

// LogViewer is a streaming log viewer overlay that shows container logs.
type LogViewer struct {
	title   string
	dseq    string
	service string
	lines   []LogLine
	paused  bool
	scroll  int
	width   int
	height  int
	active  bool

	// Service filter: cycles through known services.
	serviceFilter string
	knownServices []string
}

// NewLogViewer returns a new inactive LogViewer.
func NewLogViewer() LogViewer {
	return LogViewer{}
}

// Open activates the log viewer, resetting state for the given deployment.
func (v *LogViewer) Open(title, dseq, service string) {
	v.active = true
	v.title = title
	v.dseq = dseq
	v.service = service
	v.lines = nil
	v.scroll = 0
	v.paused = false
	v.serviceFilter = ""
	v.knownServices = nil
}

// Close deactivates the log viewer.
func (v *LogViewer) Close() {
	v.active = false
}

// Update handles key events for the log viewer overlay.
func (v *LogViewer) Update(msg tea.Msg) tea.Cmd {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case "esc":
			return CmdFunc(messages.StopLogStreamMsg{})
		case " ":
			v.TogglePause()
		case "c":
			v.Clear()
		case "k", "up":
			v.ScrollUp()
		case "j", "down":
			v.ScrollDown()
		case "G":
			v.ScrollToBottom()
		case "s":
			v.CycleServiceFilter()
		}
	}
	return nil
}

// Active returns whether the log viewer overlay is visible.
func (v LogViewer) Active() bool {
	return v.active
}

// AppendLine adds a single log line. When not paused, auto-scrolls to bottom.
// Trims to maxLogLines. Tracks unique service names for filtering.
func (v *LogViewer) AppendLine(line LogLine) {
	v.trackService(line.Scope)
	v.lines = append(v.lines, line)
	if len(v.lines) > maxLogLines {
		v.lines = v.lines[len(v.lines)-maxLogLines:]
	}
	if !v.paused {
		v.scrollToEnd()
	}
}

// AppendLines adds multiple log lines at once.
func (v *LogViewer) AppendLines(lines []LogLine) {
	v.lines = append(v.lines, lines...)
	if len(v.lines) > maxLogLines {
		v.lines = v.lines[len(v.lines)-maxLogLines:]
	}
	if !v.paused {
		v.scrollToEnd()
	}
}

// TogglePause flips the paused state. When unpausing, scrolls to bottom.
func (v *LogViewer) TogglePause() {
	v.paused = !v.paused
	if !v.paused {
		v.scrollToEnd()
	}
}

// Clear empties all lines and resets scroll.
func (v *LogViewer) Clear() {
	v.lines = nil
	v.scroll = 0
}

// trackService adds a service name to knownServices if not already present.
func (v *LogViewer) trackService(scope string) {
	if scope == "" {
		return
	}
	for _, s := range v.knownServices {
		if s == scope {
			return
		}
	}
	v.knownServices = append(v.knownServices, scope)
}

// CycleServiceFilter cycles through: "" (all) → service1 → service2 → ... → "" (all).
func (v *LogViewer) CycleServiceFilter() {
	if len(v.knownServices) == 0 {
		v.serviceFilter = ""
		return
	}
	if v.serviceFilter == "" {
		v.serviceFilter = v.knownServices[0]
		return
	}
	for i, s := range v.knownServices {
		if s == v.serviceFilter {
			if i+1 < len(v.knownServices) {
				v.serviceFilter = v.knownServices[i+1]
			} else {
				v.serviceFilter = ""
			}
			return
		}
	}
	// Current filter not found in known services; reset.
	v.serviceFilter = ""
}

// ServiceFilter returns the current service filter value.
func (v *LogViewer) ServiceFilter() string {
	return v.serviceFilter
}

// SetSize sets the available width and height for the overlay.
func (v *LogViewer) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp moves the viewport up by one line (only when paused).
func (v *LogViewer) ScrollUp() {
	if !v.paused {
		return
	}
	if v.scroll > 0 {
		v.scroll--
	}
}

// ScrollDown moves the viewport down by one line (only when paused).
func (v *LogViewer) ScrollDown() {
	if !v.paused {
		return
	}
	limit := v.maxScroll()
	if v.scroll < limit {
		v.scroll++
	}
}

// ScrollToBottom scrolls to the most recent log lines.
func (v *LogViewer) ScrollToBottom() {
	v.scrollToEnd()
}

// scrollToEnd sets scroll to the maximum offset.
func (v *LogViewer) scrollToEnd() {
	v.scroll = v.maxScroll()
}

// logAreaHeight returns the number of visible log lines in the viewport.
// Accounts for header (1 line + rule), footer (rule + 1 line), border (2),
// and padding (2).
func (v *LogViewer) logAreaHeight() int {
	// border top/bottom = 2, padding top/bottom = 2, header = 2, footer = 2
	h := v.height - 8
	if h < 1 {
		h = 1
	}
	return h
}

// maxScroll returns the maximum scroll offset.
func (v *LogViewer) maxScroll() int {
	visible := v.logAreaHeight()
	if len(v.lines) <= visible {
		return 0
	}
	return len(v.lines) - visible
}

// View renders the log viewer overlay.
func (v LogViewer) View() tea.View {
	if !v.active {
		return tea.NewView("")
	}

	// Inner width: total width minus border (2) and padding (4).
	innerW := v.width - 6
	if innerW < 30 {
		innerW = 30
	}

	// ── Header bar ──
	header := v.renderHeader(innerW)

	// ── Log area ──
	logArea := v.renderLogArea(innerW)

	// ── Footer hints ──
	footer := v.renderFooter(innerW)

	content := header + "\n" + logArea + "\n" + footer

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Slate700).
		Background(theme.Slate900).
		Padding(1, 2).
		Width(v.width - 2)

	return tea.NewView(box.Render(content))
}

// renderHeader renders the header bar with badge, deployment info, and status.
func (v LogViewer) renderHeader(w int) string {
	badge := lipgloss.NewStyle().
		Foreground(theme.Slate950).
		Background(theme.AccentRed).
		Bold(true).
		Padding(0, 1).
		Render("LOGS")

	name := theme.Body.Render(v.title)
	dseq := theme.Muted.Render("dseq " + v.dseq)
	svc := theme.Muted.Render("service " + v.service)

	left := badge + "  " + name + "  " + dseq + "  " + svc

	var status string
	if v.paused {
		status = lipgloss.NewStyle().Foreground(theme.YellowColor).Render("○") +
			" " + lipgloss.NewStyle().Foreground(theme.YellowColor).Render("paused")
	} else {
		status = lipgloss.NewStyle().Foreground(theme.GreenColor).Render("●") +
			" " + lipgloss.NewStyle().Foreground(theme.GreenColor).Render("streaming")
	}

	// Right-align the status indicator.
	leftLen := lipgloss.Width(left)
	statusLen := lipgloss.Width(status)
	gap := w - leftLen - statusLen
	if gap < 1 {
		gap = 1
	}

	headerLine := left + strings.Repeat(" ", gap) + status
	rule := lipgloss.NewStyle().Foreground(theme.Slate700).Render(strings.Repeat("─", w))

	return headerLine + "\n" + rule
}

// Column widths for log line rendering.
const (
	colTimestamp = 10
	colLevel     = 6
	colScope     = 10
)

// filteredLines returns the log lines matching the current service filter.
func (v LogViewer) filteredLines() []LogLine {
	if v.serviceFilter == "" {
		return v.lines
	}
	var out []LogLine
	for _, line := range v.lines {
		if line.Scope == v.serviceFilter {
			out = append(out, line)
		}
	}
	return out
}

// renderLogArea renders the scrollable log lines.
func (v LogViewer) renderLogArea(w int) string {
	visible := v.logAreaHeight()
	lines := v.filteredLines()

	// Determine the visible slice.
	start := v.scroll
	end := start + visible
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		start = end
	}

	tsStyle := lipgloss.NewStyle().Foreground(theme.Slate500)
	scopeStyle := lipgloss.NewStyle().Foreground(theme.PurpleColor)
	msgStyle := lipgloss.NewStyle().Foreground(theme.Slate300)

	msgW := w - colTimestamp - colLevel - colScope - 3 // 3 spaces between columns
	if msgW < 10 {
		msgW = 10
	}

	var rows []string
	for _, line := range lines[start:end] {
		ts := tsStyle.Render(fmt.Sprintf("%-*s", colTimestamp, line.Timestamp))
		lvl := v.renderLevel(line.Level)
		scope := scopeStyle.Render(fmt.Sprintf("%-*s", colScope, line.Scope))
		msg := msgStyle.Render(Truncate(line.Message, msgW))
		rows = append(rows, ts+" "+lvl+" "+scope+" "+msg)
	}

	// Pad with empty lines if fewer lines than visible area.
	for len(rows) < visible {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

// renderLevel returns a styled, fixed-width level string.
func (v LogViewer) renderLevel(level string) string {
	text := fmt.Sprintf("%-*s", colLevel, level)
	switch level {
	case "WARN":
		return lipgloss.NewStyle().Foreground(theme.YellowColor).Bold(true).Render(text)
	case "ERR":
		return lipgloss.NewStyle().Foreground(theme.AccentRed).Bold(true).Render(text)
	default: // INFO and anything else
		return lipgloss.NewStyle().Foreground(theme.GreenColor).Bold(true).Render(text)
	}
}

// renderFooter renders the footer hints.
func (v LogViewer) renderFooter(w int) string {
	pauseDesc := "pause"
	if v.paused {
		pauseDesc = "resume"
	}
	filterDesc := "filter svc"
	if v.serviceFilter != "" {
		filterDesc = "svc:" + v.serviceFilter
	}
	hints := []components.HintPair{
		{Key: "space", Desc: pauseDesc},
		{Key: "c", Desc: "clear"},
		{Key: "s", Desc: filterDesc},
		{Key: "esc", Desc: "back"},
	}
	return components.Footer(w, hints)
}
