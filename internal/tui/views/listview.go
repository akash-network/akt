package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// ListItem represents a single item in a list view.
type ListItem struct {
	// ID is an opaque identifier for the item (e.g., deployment DSEQ).
	ID string
	// Cells contains the column values for this row.
	Cells []string
}

// ListColumn defines a column in the list view.
type ListColumn struct {
	Header string
	Width  int // 0 = auto
}

// ListViewConfig configures a ListView.
type ListViewConfig struct {
	Title    string
	Columns  []ListColumn
	Empty    string // message when list is empty
	HelpKeys string // e.g., "enter: detail  r: refresh"
}

// ListView is a generic reusable list component for resource views.
// It renders a title, column headers, scrollable rows, and a cursor.
type ListView struct {
	config ListViewConfig
	items  []ListItem
	cursor int
	offset int // scroll offset
	width  int
	height int
}

// NewListView creates a new empty list view.
func NewListView(cfg ListViewConfig) ListView {
	return ListView{
		config: cfg,
	}
}

// SetItems replaces the list contents.
func (v *ListView) SetItems(items []ListItem) {
	v.items = items
	if v.cursor >= len(items) {
		v.cursor = max(0, len(items)-1)
	}
}

// SetSize updates the view dimensions.
func (v *ListView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// SelectedItem returns the currently highlighted item, or nil if empty.
func (v *ListView) SelectedItem() *ListItem {
	if len(v.items) == 0 || v.cursor >= len(v.items) {
		return nil
	}
	return &v.items[v.cursor]
}

// SelectedIndex returns the cursor position.
func (v *ListView) SelectedIndex() int {
	return v.cursor
}

// HandleKey processes navigation keys. Returns true if the key was consumed.
func (v *ListView) HandleKey(msg tea.KeyPressMsg, up, down, sel key.Binding) (consumed bool, cmd tea.Cmd) {
	switch {
	case key.Matches(msg, up):
		if v.cursor > 0 {
			v.cursor--
			v.ensureVisible()
		}
		return true, nil
	case key.Matches(msg, down):
		if v.cursor < len(v.items)-1 {
			v.cursor++
			v.ensureVisible()
		}
		return true, nil
	case key.Matches(msg, sel):
		if item := v.SelectedItem(); item != nil {
			return true, func() tea.Msg {
				return ListSelectMsg{ID: item.ID, Index: v.cursor}
			}
		}
		return true, nil
	}
	return false, nil
}

// ListSelectMsg is emitted when the user presses Enter on a list item.
type ListSelectMsg struct {
	ID    string
	Index int
}

// View renders the list.
func (v ListView) View() string {
	var b strings.Builder
	w := v.width
	if w < 40 {
		w = 40
	}

	// Title
	title := theme.SectionHeader.Width(w).Render(v.config.Title)
	b.WriteString(title)
	b.WriteString("\n")

	// Empty state
	if len(v.items) == 0 {
		empty := v.config.Empty
		if empty == "" {
			empty = "No items"
		}
		b.WriteString(theme.Muted.Render("  " + empty))
		b.WriteString("\n")
		return b.String()
	}

	// Available height for rows (title=3 lines due to border+margin, help=1, padding=1)
	rowsH := v.height - 6
	if rowsH < 3 {
		rowsH = 3
	}

	// Compute column widths
	colWidths := v.computeColumnWidths(w)

	// Column headers
	var headerParts []string
	for i, col := range v.config.Columns {
		cw := colWidths[i]
		headerParts = append(headerParts, theme.Header.Render(fmt.Sprintf("%-*s", cw, col.Header)))
	}
	b.WriteString("  " + strings.Join(headerParts, "  "))
	b.WriteString("\n")

	// Rows
	visible := v.items
	startIdx := v.offset
	endIdx := startIdx + rowsH
	if endIdx > len(visible) {
		endIdx = len(visible)
	}
	if startIdx > len(visible) {
		startIdx = len(visible)
	}

	for i := startIdx; i < endIdx; i++ {
		item := visible[i]
		isSelected := i == v.cursor

		cursor := "  "
		style := theme.Muted
		if isSelected {
			cursor = theme.Proposer.Render("> ")
			style = theme.Highlight
		}

		var cellParts []string
		for j, cell := range item.Cells {
			if j < len(colWidths) {
				cellParts = append(cellParts, style.Render(fmt.Sprintf("%-*s", colWidths[j], cell)))
			}
		}

		b.WriteString(cursor + strings.Join(cellParts, "  "))
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(v.items) > rowsH {
		info := theme.Muted.Render(fmt.Sprintf("  %d-%d of %d", startIdx+1, endIdx, len(v.items)))
		b.WriteString(info)
		b.WriteString("\n")
	}

	return b.String()
}

func (v *ListView) ensureVisible() {
	rowsH := v.height - 6
	if rowsH < 3 {
		rowsH = 3
	}
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+rowsH {
		v.offset = v.cursor - rowsH + 1
	}
}

func (v ListView) computeColumnWidths(totalWidth int) []int {
	n := len(v.config.Columns)
	if n == 0 {
		return nil
	}

	widths := make([]int, n)
	usedWidth := 2 + (n-1)*2 // indent(2) + gaps(2 each)

	// First pass: assign explicit widths
	autoCount := 0
	for i, col := range v.config.Columns {
		if col.Width > 0 {
			widths[i] = col.Width
			usedWidth += col.Width
		} else {
			autoCount++
		}
	}

	// Second pass: distribute remaining width to auto columns
	if autoCount > 0 {
		remaining := totalWidth - usedWidth
		if remaining < autoCount*8 {
			remaining = autoCount * 8
		}
		autoWidth := remaining / autoCount
		for i, col := range v.config.Columns {
			if col.Width == 0 {
				widths[i] = autoWidth
			}
		}
	}

	return widths
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
