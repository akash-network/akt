package components_test

import (
	"strings"
	"testing"

	"pkg.akt.dev/akt/internal/tui/components"
)

func TestResourceTableRender(t *testing.T) {
	cfg := components.ResourceTableConfig{
		Columns: []components.TableColumn{
			{Header: "NAME", Width: 12},
			{Header: "STATUS", Width: 10},
			{Header: "AGE", Width: 8},
		},
		EmptyText: "No resources",
	}
	tbl := components.NewResourceTable(cfg)
	tbl.SetRows([]components.TableRow{
		{Cells: []string{"deploy-1", "active", "2d"}, ID: "1"},
		{Cells: []string{"deploy-2", "closed", "5d"}, ID: "2"},
	})
	tbl.SetSize(80, 20)

	out := tbl.View()
	if out == "" {
		t.Fatal("View returned empty string")
	}
	// Headers should be present
	if !strings.Contains(out, "NAME") {
		t.Error("output missing header NAME")
	}
	if !strings.Contains(out, "STATUS") {
		t.Error("output missing header STATUS")
	}
	if !strings.Contains(out, "AGE") {
		t.Error("output missing header AGE")
	}
	// Data should be present
	if !strings.Contains(out, "deploy-1") {
		t.Error("output missing row data deploy-1")
	}
	if !strings.Contains(out, "deploy-2") {
		t.Error("output missing row data deploy-2")
	}
	// Row count
	if !strings.Contains(out, "2 items") {
		t.Error("output missing row count '2 items'")
	}
	// Horizontal rules (─)
	if !strings.Contains(out, "─") {
		t.Error("output missing horizontal rule")
	}
	// Selected cursor on first row
	if !strings.Contains(out, "▸") {
		t.Error("output missing cursor indicator ▸")
	}
}

func TestResourceTableCursor(t *testing.T) {
	cfg := components.ResourceTableConfig{
		Columns: []components.TableColumn{
			{Header: "NAME", Width: 10},
		},
	}
	tbl := components.NewResourceTable(cfg)
	tbl.SetRows([]components.TableRow{
		{Cells: []string{"row-0"}, ID: "0"},
		{Cells: []string{"row-1"}, ID: "1"},
		{Cells: []string{"row-2"}, ID: "2"},
	})
	tbl.SetSize(40, 20)

	// Initial cursor at 0
	if idx := tbl.SelectedIndex(); idx != 0 {
		t.Errorf("initial SelectedIndex = %d, want 0", idx)
	}
	if row := tbl.SelectedRow(); row == nil || row.ID != "0" {
		t.Errorf("initial SelectedRow ID = %v, want '0'", row)
	}

	// CursorDown → 1
	tbl.CursorDown()
	if idx := tbl.SelectedIndex(); idx != 1 {
		t.Errorf("after CursorDown SelectedIndex = %d, want 1", idx)
	}

	// CursorUp → 0
	tbl.CursorUp()
	if idx := tbl.SelectedIndex(); idx != 0 {
		t.Errorf("after CursorUp SelectedIndex = %d, want 0", idx)
	}

	// CursorUp at top stays at 0
	tbl.CursorUp()
	if idx := tbl.SelectedIndex(); idx != 0 {
		t.Errorf("CursorUp at top SelectedIndex = %d, want 0", idx)
	}

	// CursorBottom → 2
	tbl.CursorBottom()
	if idx := tbl.SelectedIndex(); idx != 2 {
		t.Errorf("after CursorBottom SelectedIndex = %d, want 2", idx)
	}

	// CursorDown at bottom stays at 2
	tbl.CursorDown()
	if idx := tbl.SelectedIndex(); idx != 2 {
		t.Errorf("CursorDown at bottom SelectedIndex = %d, want 2", idx)
	}

	// CursorTop → 0
	tbl.CursorTop()
	if idx := tbl.SelectedIndex(); idx != 0 {
		t.Errorf("after CursorTop SelectedIndex = %d, want 0", idx)
	}
}

func TestResourceTableEmpty(t *testing.T) {
	cfg := components.ResourceTableConfig{
		Columns: []components.TableColumn{
			{Header: "NAME", Width: 10},
			{Header: "VALUE", Width: 10},
		},
		EmptyText: "Nothing to show",
	}
	tbl := components.NewResourceTable(cfg)
	tbl.SetSize(60, 20)

	out := tbl.View()
	if !strings.Contains(out, "Nothing to show") {
		t.Error("empty table missing EmptyText")
	}
	if !strings.Contains(out, "0 items") {
		t.Error("empty table missing '0 items' count")
	}
	// Should have no cursor
	if strings.Contains(out, "▸") {
		t.Error("empty table should not have cursor")
	}
	// SelectedRow should be nil
	if row := tbl.SelectedRow(); row != nil {
		t.Error("empty table SelectedRow should be nil")
	}
}

func TestResourceTableScrolling(t *testing.T) {
	cfg := components.ResourceTableConfig{
		Columns: []components.TableColumn{
			{Header: "IDX", Width: 6},
		},
	}
	tbl := components.NewResourceTable(cfg)

	// Create 20 rows
	rows := make([]components.TableRow, 20)
	for i := range rows {
		rows[i] = components.TableRow{
			Cells: []string{strings.Repeat("x", 4)},
			ID:    string(rune('A' + i)),
		}
	}
	tbl.SetRows(rows)
	// height=10 → visible rows = 10 - 4 = 6
	tbl.SetSize(40, 10)

	// Initially at top, first row visible
	out := tbl.View()
	if !strings.Contains(out, "▸") {
		t.Error("scrolling: missing cursor at top")
	}

	// Move cursor to row 8 (beyond visible window of 6)
	for i := 0; i < 8; i++ {
		tbl.CursorDown()
	}
	if idx := tbl.SelectedIndex(); idx != 8 {
		t.Errorf("scrolling: SelectedIndex = %d, want 8", idx)
	}

	// Render should still contain cursor (scrolled view)
	out = tbl.View()
	if !strings.Contains(out, "▸") {
		t.Error("scrolling: missing cursor after scrolling down")
	}
	if !strings.Contains(out, "20 items") {
		t.Error("scrolling: missing total row count")
	}

	// Move to bottom
	tbl.CursorBottom()
	if idx := tbl.SelectedIndex(); idx != 19 {
		t.Errorf("scrolling: CursorBottom SelectedIndex = %d, want 19", idx)
	}
	out = tbl.View()
	if !strings.Contains(out, "▸") {
		t.Error("scrolling: missing cursor at bottom")
	}

	// Move back to top
	tbl.CursorTop()
	if idx := tbl.SelectedIndex(); idx != 0 {
		t.Errorf("scrolling: CursorTop SelectedIndex = %d, want 0", idx)
	}
}

func TestResourceTableRenderFunc(t *testing.T) {
	called := false
	cfg := components.ResourceTableConfig{
		Columns: []components.TableColumn{
			{Header: "NAME", Width: 10},
			{
				Header: "STATE",
				Width:  12,
				RenderFunc: func(value string) string {
					called = true
					return "[" + value + "]"
				},
			},
		},
	}
	tbl := components.NewResourceTable(cfg)
	tbl.SetRows([]components.TableRow{
		{Cells: []string{"item-1", "active"}, ID: "1"},
	})
	tbl.SetSize(60, 20)

	out := tbl.View()
	if !called {
		t.Error("RenderFunc was not called")
	}
	if !strings.Contains(out, "[active]") {
		t.Error("output missing RenderFunc result '[active]'")
	}
}
