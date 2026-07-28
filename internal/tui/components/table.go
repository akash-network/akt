package components

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// Alignment controls how cell text is aligned within a column.
type Alignment int

const (
	// AlignLeft pads text on the right.
	AlignLeft Alignment = iota
	// AlignRight pads text on the left.
	AlignRight
)

// TableColumn defines a single column in a ResourceTable.
type TableColumn struct {
	Header     string
	Width      int                       // character width (0 = fill remaining space)
	Align      Alignment                 // left or right alignment
	RenderFunc func(value string) string // optional custom cell renderer (e.g., state tags)
}

// TableRow holds the cell values for a single row.
type TableRow struct {
	Cells []string
	ID    string // optional identifier
}

// ResourceTableConfig configures a ResourceTable.
type ResourceTableConfig struct {
	Columns   []TableColumn
	EmptyText string // shown when no rows
}

// ResourceTable is a reusable table component for list views. It renders
// fixed-width columns using fmt.Sprintf("%-*s") to prevent row wrapping.
type ResourceTable struct {
	config   ResourceTableConfig
	rows     []TableRow
	filtered []TableRow
	cursor   int
	offset   int // scroll offset for visible window
	width    int
	height   int
	sortCol  int  // column to sort by (-1 = no sort)
	sortAsc  bool // sort direction
	filter   string
}

// NewResourceTable creates a new ResourceTable with the given configuration.
func NewResourceTable(cfg ResourceTableConfig) ResourceTable {
	return ResourceTable{config: cfg, sortCol: -1}
}

// SetRows replaces the table rows and rebuilds the filtered/sorted view.
func (t *ResourceTable) SetRows(rows []TableRow) {
	t.rows = rows
	t.applyFilterAndSort()
}

// Width returns the current table width.
func (t *ResourceTable) Width() int {
	return t.width
}

// SetSize updates the available width and height for rendering.
func (t *ResourceTable) SetSize(w, h int) {
	t.width = w
	t.height = h
	t.ensureVisible()
}

// SelectedIndex returns the current cursor position.
func (t *ResourceTable) SelectedIndex() int {
	return t.cursor
}

// SelectedRow returns the currently highlighted row, or nil if empty.
func (t *ResourceTable) SelectedRow() *TableRow {
	if len(t.filtered) == 0 || t.cursor >= len(t.filtered) {
		return nil
	}
	return &t.filtered[t.cursor]
}

// CursorDown moves the cursor down one row.
func (t *ResourceTable) CursorDown() {
	if t.cursor < len(t.filtered)-1 {
		t.cursor++
		t.ensureVisible()
	}
}

// CursorUp moves the cursor up one row.
func (t *ResourceTable) CursorUp() {
	if t.cursor > 0 {
		t.cursor--
		t.ensureVisible()
	}
}

// CursorTop moves the cursor to the first row.
func (t *ResourceTable) CursorTop() {
	t.cursor = 0
	t.ensureVisible()
}

// CursorBottom moves the cursor to the last row.
func (t *ResourceTable) CursorBottom() {
	if len(t.filtered) > 0 {
		t.cursor = len(t.filtered) - 1
		t.ensureVisible()
	}
}

// visibleRows returns the number of rows that fit in the visible area.
// Subtracts 4 lines for: header, header rule, bottom rule, row count.
func (t ResourceTable) visibleRows() int {
	v := t.height - 4
	if v < 1 {
		v = 1
	}
	return v
}

// ensureVisible adjusts the scroll offset so the cursor is visible.
func (t *ResourceTable) ensureVisible() {
	vis := t.visibleRows()
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+vis {
		t.offset = t.cursor - vis + 1
	}
}

// View renders the table as a string.
func (t ResourceTable) View() string {
	w := t.width
	if w < 20 {
		w = 20
	}

	var b strings.Builder

	// Compute column widths
	colWidths := t.computeColumnWidths(w)

	// Header styles
	headerStyle := lipgloss.NewStyle().Foreground(theme.Slate500)
	ruleStyle := lipgloss.NewStyle().Foreground(theme.Slate700)
	countStyle := lipgloss.NewStyle().Foreground(theme.Slate500)

	// Column headers
	var headerParts []string
	for i, col := range t.config.Columns {
		cw := colWidths[i]
		headerParts = append(headerParts, headerStyle.Render(fmtCell(col.Header, cw, AlignLeft)))
	}
	b.WriteString("  " + strings.Join(headerParts, " "))
	b.WriteByte('\n')

	// Header rule
	b.WriteString(ruleStyle.Render(strings.Repeat("─", w)))
	b.WriteByte('\n')

	// Empty state
	if len(t.filtered) == 0 {
		emptyText := t.config.EmptyText
		if emptyText == "" {
			emptyText = "No items"
		}
		// Center the empty text
		pad := (w - len(emptyText)) / 2
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(lipgloss.NewStyle().Foreground(theme.Slate500).Render(emptyText))
		b.WriteByte('\n')
		// Bottom rule
		b.WriteString(ruleStyle.Render(strings.Repeat("─", w)))
		b.WriteByte('\n')
		// Row count
		b.WriteString(countStyle.Render("  0 items"))
		return b.String()
	}

	// Visible row window
	vis := t.visibleRows()
	startIdx := t.offset
	endIdx := startIdx + vis
	if endIdx > len(t.filtered) {
		endIdx = len(t.filtered)
	}

	cursorStyle := lipgloss.NewStyle().Foreground(theme.AccentRed).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(theme.Slate200).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(theme.Slate300)

	for i := startIdx; i < endIdx; i++ {
		row := t.filtered[i]
		isSelected := i == t.cursor

		// Cursor prefix
		if isSelected {
			b.WriteString(cursorStyle.Render("▸ "))
		} else {
			b.WriteString("  ")
		}

		// Cells
		var cellParts []string
		for j, col := range t.config.Columns {
			if j >= len(row.Cells) {
				break
			}
			cw := colWidths[j]
			cell := row.Cells[j]

			if col.RenderFunc != nil {
				// Custom renderer: call it, then manually pad to column width
				rendered := col.RenderFunc(cell)
				renderedW := lipgloss.Width(rendered)
				if renderedW < cw {
					rendered += strings.Repeat(" ", cw-renderedW)
				}
				cellParts = append(cellParts, rendered)
			} else {
				// Standard cell: truncate + pad with fmt.Sprintf
				style := normalStyle
				if isSelected {
					style = selectedStyle
				}
				cellParts = append(cellParts, style.Render(fmtCell(cell, cw, col.Align)))
			}
		}
		b.WriteString(strings.Join(cellParts, " "))
		b.WriteByte('\n')
	}

	// Bottom rule
	b.WriteString(ruleStyle.Render(strings.Repeat("─", w)))
	b.WriteByte('\n')

	// Row count
	b.WriteString(countStyle.Render(fmt.Sprintf("  %d items", len(t.filtered))))

	return b.String()
}

// Sort sets the sort column and direction, then re-applies filtering and sorting.
func (t *ResourceTable) Sort(col int, ascending bool) {
	t.sortCol = col
	t.sortAsc = ascending
	t.applyFilterAndSort()
}

// SetFilter sets a case-insensitive substring filter across all cells.
func (t *ResourceTable) SetFilter(query string) {
	t.filter = query
	t.applyFilterAndSort()
}

// ClearFilter removes the active filter.
func (t *ResourceTable) ClearFilter() {
	t.filter = ""
	t.applyFilterAndSort()
}

// FilteredCount returns the number of visible (filtered) rows.
func (t *ResourceTable) FilteredCount() int {
	return len(t.filtered)
}

// applyFilterAndSort rebuilds the filtered slice from rows, applying the
// current filter and sort settings, then clamps the cursor.
func (t *ResourceTable) applyFilterAndSort() {
	// Filter
	if t.filter == "" {
		t.filtered = make([]TableRow, len(t.rows))
		copy(t.filtered, t.rows)
	} else {
		t.filtered = t.filtered[:0]
		q := strings.ToLower(t.filter)
		for _, row := range t.rows {
			for _, cell := range row.Cells {
				if strings.Contains(strings.ToLower(cell), q) {
					t.filtered = append(t.filtered, row)
					break
				}
			}
		}
	}

	// Sort
	if t.sortCol >= 0 {
		col := t.sortCol
		asc := t.sortAsc
		sort.SliceStable(t.filtered, func(i, j int) bool {
			a, b := "", ""
			if col < len(t.filtered[i].Cells) {
				a = t.filtered[i].Cells[col]
			}
			if col < len(t.filtered[j].Cells) {
				b = t.filtered[j].Cells[col]
			}
			if asc {
				return a < b
			}
			return a > b
		})
	}

	// Clamp cursor
	if len(t.filtered) == 0 {
		t.cursor = 0
	} else if t.cursor >= len(t.filtered) {
		t.cursor = len(t.filtered) - 1
	}
	t.ensureVisible()
}

// fmtCell formats a cell value to a fixed width, truncating with "…" if needed.
func fmtCell(text string, width int, align Alignment) string {
	if width <= 0 {
		return text
	}
	// Truncate if text exceeds width
	runes := []rune(text)
	if len(runes) > width {
		if width > 1 {
			text = string(runes[:width-1]) + "…"
		} else {
			text = "…"
		}
	}
	if align == AlignRight {
		return fmt.Sprintf("%*s", width, text)
	}
	return fmt.Sprintf("%-*s", width, text)
}

// computeColumnWidths resolves column widths, distributing remaining space
// to width=0 columns.
func (t ResourceTable) computeColumnWidths(totalWidth int) []int {
	n := len(t.config.Columns)
	if n == 0 {
		return nil
	}

	widths := make([]int, n)
	// Account for cursor prefix (2) and column gaps (1 space each)
	usedWidth := 2 + (n - 1)

	// First pass: assign explicit widths
	autoCount := 0
	for i, col := range t.config.Columns {
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
		if remaining < autoCount*4 {
			remaining = autoCount * 4
		}
		autoWidth := remaining / autoCount
		for i, col := range t.config.Columns {
			if col.Width == 0 {
				widths[i] = autoWidth
			}
		}
	}

	return widths
}
