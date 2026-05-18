package views

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/tui/components"
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
	HelpKeys string // e.g., "enter: detail  r: refresh" (ignored; footer handles this)
}

// ListView is a generic reusable list component for resource views.
// It renders an optional title and delegates table rendering to a ResourceTable.
type ListView struct {
	title string
	items []ListItem
	table components.ResourceTable
}

// NewListView creates a new empty list view.
func NewListView(cfg ListViewConfig) ListView {
	cols := make([]components.TableColumn, len(cfg.Columns))
	for i, c := range cfg.Columns {
		cols[i] = components.TableColumn{
			Header: c.Header,
			Width:  c.Width,
		}
	}

	return ListView{
		title: cfg.Title,
		table: components.NewResourceTable(components.ResourceTableConfig{
			Columns:   cols,
			EmptyText: cfg.Empty,
		}),
	}
}

// SetItems replaces the list contents.
func (v *ListView) SetItems(items []ListItem) {
	v.items = items
	rows := make([]components.TableRow, len(items))
	for i, item := range items {
		rows[i] = components.TableRow{
			ID:    item.ID,
			Cells: item.Cells,
		}
	}
	v.table.SetRows(rows)
}

// SetSize updates the view dimensions.
func (v *ListView) SetSize(w, h int) {
	// Reserve space for the title line when present.
	tableH := h
	if v.title != "" {
		tableH = h - 1
	}
	if tableH < 1 {
		tableH = 1
	}
	v.table.SetSize(w, tableH)
}

// SelectedItem returns the currently highlighted item, or nil if empty.
func (v *ListView) SelectedItem() *ListItem {
	if len(v.items) == 0 {
		return nil
	}
	idx := v.table.SelectedIndex()
	if idx < 0 || idx >= len(v.items) {
		return nil
	}
	return &v.items[idx]
}

// SelectedIndex returns the cursor position.
func (v *ListView) SelectedIndex() int {
	return v.table.SelectedIndex()
}

// HandleKey processes navigation keys. Returns true if the key was consumed.
func (v *ListView) HandleKey(msg tea.KeyPressMsg, up, down, sel key.Binding) (consumed bool, cmd tea.Cmd) {
	switch {
	case key.Matches(msg, up):
		v.table.CursorUp()
		return true, nil
	case key.Matches(msg, down):
		v.table.CursorDown()
		return true, nil
	case key.Matches(msg, sel):
		if item := v.SelectedItem(); item != nil {
			idx := v.table.SelectedIndex()
			return true, func() tea.Msg {
				return ListSelectMsg{ID: item.ID, Index: idx}
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

	if v.title != "" {
		w := v.table.Width()
		if w < 20 {
			w = 20
		}
		b.WriteString(theme.SectionHeader.Width(w).Render(v.title))
		b.WriteByte('\n')
	}

	b.WriteString(v.table.View())
	return b.String()
}
