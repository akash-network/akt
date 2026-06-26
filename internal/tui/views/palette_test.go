package views_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"pkg.akt.dev/akt/internal/tui/commands"
	"pkg.akt.dev/akt/internal/tui/views"
)

func newTestPalette() *views.CommandPalette {
	reg := commands.DefaultRegistry()
	keys := views.PaletteKeys{
		CursorUp:   key.NewBinding(key.WithKeys("k", "up")),
		CursorDown: key.NewBinding(key.WithKeys("j", "down")),
		Select:     key.NewBinding(key.WithKeys("enter")),
		Close:      key.NewBinding(key.WithKeys("esc")),
	}
	p := views.NewCommandPalette(reg, keys)
	p.SetSize(120, 40)
	return &p
}

func TestPaletteInactive(t *testing.T) {
	p := newTestPalette()
	if p.Active() {
		t.Fatal("palette should be inactive by default")
	}
	if v := p.View().Content; v != "" {
		t.Fatal("inactive palette should render empty")
	}
}

func TestPaletteOpenClose(t *testing.T) {
	p := newTestPalette()
	p.Open()
	if !p.Active() {
		t.Fatal("palette should be active after Open()")
	}
	if v := p.View().Content; v == "" {
		t.Fatal("active palette should render non-empty")
	}
	p.Close()
	if p.Active() {
		t.Fatal("palette should be inactive after Close()")
	}
}

func TestPaletteEscCloses(t *testing.T) {
	p := newTestPalette()
	p.Open()
	p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if p.Active() {
		t.Fatal("Esc should close the palette")
	}
}

func TestPaletteEnterSubmits(t *testing.T) {
	p := newTestPalette()
	p.Open()
	cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should return a command")
	}
	result := cmd()
	submit, ok := result.(views.CommandSubmitMsg)
	if !ok {
		t.Fatalf("expected CommandSubmitMsg, got %T", result)
	}
	if submit.Value == "" {
		t.Fatal("submitted value should be non-empty")
	}
}

func TestPaletteCursorWraps(t *testing.T) {
	p := newTestPalette()
	p.Open()

	// Move cursor down past the end — it should wrap to top.
	allCmds := commands.DefaultRegistry().All()
	for i := 0; i < len(allCmds)+1; i++ {
		p.Update(tea.KeyPressMsg{Code: 'j'})
	}

	// Should not panic — cursor wraps via modulo.
	cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("should still submit after cursor wrap")
	}
}

func typeIntoPalette(p *views.CommandPalette, text string) {
	for _, ch := range text {
		p.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
}

func TestPaletteFilterReducesList(t *testing.T) {
	p := newTestPalette()
	p.Open()

	// Type "dep" to filter.
	typeIntoPalette(p, "dep")

	view := ansi.Strip(p.View().Content)
	if view == "" {
		t.Fatal("filtered palette should still render")
	}
	lower := strings.ToLower(view)
	if !strings.Contains(lower, "deploy") && !strings.Contains(lower, "dep") {
		t.Error("filtered palette should show deployment-related commands")
	}
}

func TestPaletteNoMatchShowsEmpty(t *testing.T) {
	p := newTestPalette()
	p.Open()

	typeIntoPalette(p, "zzzznotacommand")

	view := ansi.Strip(p.View().Content)
	if !strings.Contains(view, "no matching") {
		t.Errorf("palette should show 'no matching commands', got view length: %d", len(view))
	}
}
