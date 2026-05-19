package views_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/tui/views"
)

// Test key bindings used across ListView tests.
var (
	keyUp  = key.NewBinding(key.WithKeys("k"))
	keyDn  = key.NewBinding(key.WithKeys("j"))
	keySel = key.NewBinding(key.WithKeys("enter"))
)

func newTestListView() views.ListView {
	return views.NewListView(views.ListViewConfig{
		Title: "Deployments",
		Columns: []views.ListColumn{
			{Header: "DSEQ", Width: 10},
			{Header: "STATUS", Width: 10},
		},
		Empty: "No deployments",
	})
}

func threeItems() []views.ListItem {
	return []views.ListItem{
		{ID: "100", Cells: []string{"100", "active"}},
		{ID: "200", Cells: []string{"200", "closed"}},
		{ID: "300", Cells: []string{"300", "active"}},
	}
}

func TestListViewEmpty(t *testing.T) {
	lv := newTestListView()
	lv.SetSize(80, 20)

	if item := lv.SelectedItem(); item != nil {
		t.Errorf("SelectedItem() = %v, want nil for empty list", item)
	}

	out := lv.View()
	if out == "" {
		t.Error("View() returned empty string; want non-empty even when list is empty")
	}
}

func TestListViewSetItems(t *testing.T) {
	lv := newTestListView()
	lv.SetItems(threeItems())
	lv.SetSize(80, 20)

	// First item should be selected by default.
	item := lv.SelectedItem()
	if item == nil {
		t.Fatal("SelectedItem() = nil after SetItems with 3 items")
	}
	if item.ID != "100" {
		t.Errorf("SelectedItem().ID = %q, want %q", item.ID, "100")
	}
	if idx := lv.SelectedIndex(); idx != 0 {
		t.Errorf("SelectedIndex() = %d, want 0", idx)
	}
}

func TestListViewCursorNavigation(t *testing.T) {
	lv := newTestListView()
	lv.SetItems(threeItems())
	lv.SetSize(80, 20)

	// Move down with "j".
	consumed, _ := lv.HandleKey(tea.KeyPressMsg{Code: 'j'}, keyUp, keyDn, keySel)
	if !consumed {
		t.Error("HandleKey(j) was not consumed")
	}
	if idx := lv.SelectedIndex(); idx != 1 {
		t.Errorf("after j: SelectedIndex() = %d, want 1", idx)
	}

	// Move up with "k".
	consumed, _ = lv.HandleKey(tea.KeyPressMsg{Code: 'k'}, keyUp, keyDn, keySel)
	if !consumed {
		t.Error("HandleKey(k) was not consumed")
	}
	if idx := lv.SelectedIndex(); idx != 0 {
		t.Errorf("after k: SelectedIndex() = %d, want 0", idx)
	}
}

func TestListViewSelectEmitsMessage(t *testing.T) {
	lv := newTestListView()
	lv.SetItems(threeItems())
	lv.SetSize(80, 20)

	// Move to second item.
	lv.HandleKey(tea.KeyPressMsg{Code: 'j'}, keyUp, keyDn, keySel)

	// Press enter.
	consumed, cmd := lv.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, keyUp, keyDn, keySel)
	if !consumed {
		t.Error("HandleKey(enter) was not consumed")
	}
	if cmd == nil {
		t.Fatal("HandleKey(enter) returned nil cmd, want ListSelectMsg")
	}

	msg := cmd()
	sel, ok := msg.(views.ListSelectMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want views.ListSelectMsg", msg)
	}
	if sel.ID != "200" {
		t.Errorf("ListSelectMsg.ID = %q, want %q", sel.ID, "200")
	}
	if sel.Index != 1 {
		t.Errorf("ListSelectMsg.Index = %d, want 1", sel.Index)
	}
}

func TestListViewRendersTitle(t *testing.T) {
	lv := newTestListView()
	lv.SetSize(80, 20)

	out := lv.View()
	if out == "" {
		t.Fatal("View() returned empty string")
	}

	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Deployments") {
		t.Error("View() missing title 'Deployments'")
	}
}
