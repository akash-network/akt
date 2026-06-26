package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/views"
)

// mockView is a minimal ViewComponent for testing.
type mockView struct {
	label   string
	initCmd tea.Cmd
	sized   bool
	w, h    int
	updated bool
}

func newMockView(label string) *mockView {
	return &mockView{label: label}
}

func (m *mockView) Init() tea.Cmd                           { return m.initCmd }
func (m *mockView) Update(msg tea.Msg) (tea.Model, tea.Cmd) { m.updated = true; return m, nil }
func (m *mockView) View() tea.View                          { return tea.NewView("view:" + m.label) }
func (m *mockView) SetSize(w, h int)                        { m.sized = true; m.w = w; m.h = h }
func (m *mockView) Breadcrumb() string                      { return m.label }
func (m *mockView) ShortHelp() []components.HintPair        { return nil }
func (m *mockView) Refresh() tea.Cmd                        { return nil }

// Verify mockView implements ViewComponent
var _ views.ViewComponent = (*mockView)(nil)

func TestRouterPush(t *testing.T) {
	r := NewRouter()
	r.SetSize(80, 24)

	v1 := newMockView("Dashboard")
	r.Push(v1)

	if r.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", r.Depth())
	}
	if r.Active() != v1 {
		t.Error("Active() should be v1")
	}
	if !v1.sized {
		t.Error("Push should call SetSize")
	}

	v2 := newMockView("Deployments")
	r.Push(v2)

	if r.Depth() != 2 {
		t.Errorf("Depth() = %d, want 2", r.Depth())
	}
	if r.Active() != v2 {
		t.Error("Active() should be v2 after push")
	}
}

func TestRouterPop(t *testing.T) {
	r := NewRouter()
	r.SetSize(80, 24)

	v1 := newMockView("Dashboard")
	v2 := newMockView("Deployments")
	r.Push(v1)
	r.Push(v2)

	r.Pop()
	if r.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", r.Depth())
	}
	if r.Active() != v1 {
		t.Error("Active() should be v1 after pop")
	}
}

func TestRouterPopMinOne(t *testing.T) {
	r := NewRouter()
	v1 := newMockView("Dashboard")
	r.Push(v1)

	r.Pop()
	if r.Depth() != 1 {
		t.Errorf("Pop on single-view stack should be no-op, got depth %d", r.Depth())
	}
}

func TestRouterReplace(t *testing.T) {
	r := NewRouter()
	r.SetSize(80, 24)

	v1 := newMockView("Dashboard")
	v2 := newMockView("Deployments")
	r.Push(v1)

	r.Replace(v2)
	if r.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1 after replace", r.Depth())
	}
	if r.Active() != v2 {
		t.Error("Active() should be v2 after replace")
	}
}

func TestRouterBreadcrumb(t *testing.T) {
	r := NewRouter()
	r.Push(newMockView("Dashboard"))
	r.Push(newMockView("Deployments"))
	r.Push(newMockView("Detail"))

	want := "Dashboard > Deployments > Detail"
	if got := r.Breadcrumb(); got != want {
		t.Errorf("Breadcrumb() = %q, want %q", got, want)
	}
}

func TestRouterUpdateDelegatesToActive(t *testing.T) {
	r := NewRouter()
	v1 := newMockView("Dashboard")
	v2 := newMockView("Deployments")
	r.Push(v1)
	r.Push(v2)

	r.Update(tea.KeyPressMsg{})
	if !v2.updated {
		t.Error("Update should delegate to active view")
	}
	if v1.updated {
		t.Error("Update should NOT delegate to non-active views")
	}
}

func TestRouterSetSizePropagatesOnlyToActive(t *testing.T) {
	r := NewRouter()
	v1 := newMockView("Dashboard")
	v2 := newMockView("Deployments")
	r.Push(v1)
	r.Push(v2)

	// Reset the sized flag that was set by Push
	v1.sized = false
	v2.sized = false

	r.SetSize(100, 50)
	if v1.sized {
		t.Error("SetSize should NOT propagate to non-active views")
	}
	if !v2.sized {
		t.Error("SetSize should propagate to active view")
	}
	if v2.w != 100 || v2.h != 50 {
		t.Errorf("Active view size = (%d, %d), want (100, 50)", v2.w, v2.h)
	}
}

func TestRouterViewEmpty(t *testing.T) {
	r := NewRouter()
	if got := r.View().Content; got != "" {
		t.Errorf("View() on empty router = %q, want empty", got)
	}
}

func TestRouterView(t *testing.T) {
	r := NewRouter()
	r.Push(newMockView("Dashboard"))
	if got := r.View().Content; got != "view:Dashboard" {
		t.Errorf("View() = %q, want %q", got, "view:Dashboard")
	}
}
