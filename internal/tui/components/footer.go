package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// HintPair represents a single key-description pair in the footer.
// When Accent is true the key is rendered in AccentRed instead of the
// default Slate400.
type HintPair struct {
	Key    string
	Desc   string
	Accent bool
}

// accentKey is the style used for accent-flagged footer keys.
var accentKey = lipgloss.NewStyle().Foreground(theme.AccentRed).Bold(true)

// FooterHints renders a row of key-description pairs separated by two spaces.
//
//	key desc  key desc  key desc
func FooterHints(hints []HintPair) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		var k string
		if h.Accent {
			k = accentKey.Render(h.Key)
		} else {
			k = theme.FooterKey.Render(h.Key)
		}
		parts = append(parts, k+" "+theme.FooterDesc.Render(h.Desc))
	}
	return strings.Join(parts, "  ")
}

// HRule renders a full-width horizontal rule in Slate700.
func HRule(width int) string {
	return lipgloss.NewStyle().Foreground(theme.Slate700).Render(strings.Repeat("─", width))
}

// Footer renders a horizontal rule followed by a newline and footer hints.
func Footer(width int, hints []HintPair) string {
	return HRule(width) + "\n" + FooterHints(hints)
}
