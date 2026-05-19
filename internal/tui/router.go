package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"pkg.akt.dev/akt/internal/tui/views"
)

// Router manages a navigation stack of ViewComponent values.
// The active view (top of stack) receives Update() and View() calls.
// Views below the top are frozen until they become active again via Pop().
type Router struct {
	stack []views.ViewComponent
	w, h  int
}

// NewRouter creates an empty Router.
func NewRouter() Router {
	return Router{}
}

// Push adds a view to the top of the stack, sets its size, and
// returns its Init() command.
func (r *Router) Push(v views.ViewComponent) tea.Cmd {
	r.stack = append(r.stack, v)
	v.SetSize(r.w, r.h)
	return v.Init()
}

// Pop removes the top view from the stack. If the stack has only
// one view (the root), this is a no-op and returns nil.
func (r *Router) Pop() tea.Cmd {
	if len(r.stack) <= 1 {
		return nil
	}
	r.stack = r.stack[:len(r.stack)-1]
	// Re-set size on the newly active view in case the terminal was
	// resized while it was covered.
	if active := r.Active(); active != nil {
		active.SetSize(r.w, r.h)
	}
	return nil
}

// Replace swaps the top view with a new one, sets its size, and
// returns its Init() command. If the stack is empty, it behaves
// like Push.
func (r *Router) Replace(v views.ViewComponent) tea.Cmd {
	if len(r.stack) == 0 {
		return r.Push(v)
	}
	r.stack[len(r.stack)-1] = v
	v.SetSize(r.w, r.h)
	return v.Init()
}

// Active returns the top view on the stack, or nil if the stack
// is empty.
func (r *Router) Active() views.ViewComponent {
	if len(r.stack) == 0 {
		return nil
	}
	return r.stack[len(r.stack)-1]
}

// Depth returns the number of views on the stack.
func (r *Router) Depth() int {
	return len(r.stack)
}

// Breadcrumb returns the navigation trail by joining all stack
// views' Breadcrumb() values with " > ".
func (r *Router) Breadcrumb() string {
	if len(r.stack) == 0 {
		return ""
	}
	parts := make([]string, len(r.stack))
	for i, v := range r.stack {
		parts[i] = v.Breadcrumb()
	}
	return strings.Join(parts, " > ")
}

// SetSize stores the available dimensions and propagates to
// the active (top) view only.
func (r *Router) SetSize(w, h int) {
	r.w, r.h = w, h
	if active := r.Active(); active != nil {
		active.SetSize(w, h)
	}
}

// Update delegates the message to the active view. The returned
// tea.Model from the view's Update() replaces the top of the stack
// (to support bubbletea's immutable model pattern).
func (r *Router) Update(msg tea.Msg) tea.Cmd {
	if len(r.stack) == 0 {
		return nil
	}
	active := r.stack[len(r.stack)-1]
	updated, cmd := active.Update(msg)
	if vc, ok := updated.(views.ViewComponent); ok {
		r.stack[len(r.stack)-1] = vc
	}
	return cmd
}

// View returns the active view's rendered output.
func (r Router) View() tea.View {
	if len(r.stack) == 0 {
		return tea.NewView("")
	}
	return r.stack[len(r.stack)-1].View()
}
