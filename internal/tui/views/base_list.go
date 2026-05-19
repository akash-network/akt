package views

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/keys"
)

// BaseListView provides common list behavior (cursor, scroll, filter)
// by composing a ResourceTable. Concrete list views embed this and
// add their own column definitions and key handlers.
type BaseListView struct {
	Table components.ResourceTable
	Keys  keys.KeyMap
	W, H  int
}

func NewBaseListView(cfg components.ResourceTableConfig, km keys.KeyMap) BaseListView {
	return BaseListView{
		Table: components.NewResourceTable(cfg),
		Keys:  km,
	}
}

func (b *BaseListView) SetSize(w, h int) {
	b.W, b.H = w, h
	b.Table.SetSize(w, h)
}

func (b *BaseListView) SetRows(rows []components.TableRow) {
	b.Table.SetRows(rows)
}

// Update handles common list navigation keys. Returns a tea.Cmd
// (always nil for cursor movement). Concrete views should call this
// as a fallback after handling their own keys.
func (b *BaseListView) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, b.Keys.CursorDown):
			b.Table.CursorDown()
		case key.Matches(msg, b.Keys.CursorUp):
			b.Table.CursorUp()
		}
	}
	return nil
}

func (b BaseListView) View() tea.View {
	return tea.NewView(b.Table.View())
}

func (b BaseListView) Cursor() int {
	return b.Table.SelectedIndex()
}

func (b BaseListView) SelectedRow() *components.TableRow {
	return b.Table.SelectedRow()
}
