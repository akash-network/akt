package views

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// BaseDetailView provides common detail view behavior (scrolling).
// Concrete detail views embed this and render their own content
// as a []string of lines, then call VisibleWindow() to slice.
type BaseDetailView struct {
	Scroll int
	W, H   int
}

func NewBaseDetailView() BaseDetailView {
	return BaseDetailView{}
}

func (b *BaseDetailView) SetSize(w, h int) {
	b.W, b.H = w, h
}

// Update handles j/k scroll keys. Returns nil Cmd.
func (b *BaseDetailView) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "j", "down":
			b.Scroll++
		case "k", "up":
			if b.Scroll > 0 {
				b.Scroll--
			}
		}
	}
	return nil
}

// VisibleWindow returns the visible slice of lines based on scroll
// position and available height. visibleH is the number of lines
// that fit in the viewport.
func (b *BaseDetailView) VisibleWindow(lines []string, visibleH int) []string {
	if visibleH <= 0 {
		return nil
	}
	if len(lines) == 0 {
		return nil
	}
	start := b.Scroll
	if start >= len(lines) {
		start = max(0, len(lines)-1)
	}
	end := start + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	// Clamp scroll to prevent over-scrolling
	if start > 0 && end == len(lines) {
		start = max(0, len(lines)-visibleH)
		b.Scroll = start
		end = len(lines)
	}
	return lines[start:end]
}

// ScrollHint returns a scroll indicator string if content overflows.
func (b BaseDetailView) ScrollHint(totalLines, visibleH int) string {
	if totalLines <= visibleH {
		return ""
	}
	return fmt.Sprintf("  ↕ %d/%d", b.Scroll+1, totalLines-visibleH+1)
}
