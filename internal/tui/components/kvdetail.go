package components

import (
	"fmt"
	"strings"

	"pkg.akt.dev/akt/internal/ui/theme"
)

const kvLabelWidth = 16

// KVPair holds a label–value pair for rendering in a detail block.
type KVPair struct {
	Label string
	Value string
}

// Section renders a section heading (bold Slate100) followed by a red accent
// rule of the given width.
func Section(title string, width int) string {
	heading := theme.SectionTitle.Render(title)
	rule := theme.SectionRule.Render(strings.Repeat("─", width))
	return heading + "\n" + rule
}

// KV renders a single key-value line: label in Slate500 (fixed width 16) and
// value in Slate200.
func KV(label, value string) string {
	l := theme.KVLabel.Render(fmt.Sprintf("%-*s", kvLabelWidth, label))
	v := theme.KVValue.Render(value)
	return l + v
}

// KVMuted renders a key-value line where both label and value use Slate500
// (muted).
func KVMuted(label, value string) string {
	l := theme.KVLabel.Render(fmt.Sprintf("%-*s", kvLabelWidth, label))
	v := theme.Muted.Render(value)
	return l + v
}

// KVBold renders a key-value line where the value uses Slate100 bold (heading
// weight).
func KVBold(label, value string) string {
	l := theme.KVLabel.Render(fmt.Sprintf("%-*s", kvLabelWidth, label))
	v := theme.Heading.Render(value)
	return l + v
}

// KVBlock renders multiple KV pairs, one per line.
func KVBlock(pairs []KVPair) string {
	lines := make([]string, len(pairs))
	for i, p := range pairs {
		lines[i] = KV(p.Label, p.Value)
	}
	return strings.Join(lines, "\n")
}

// SectionWithKV renders a Section heading followed by a KVBlock.
func SectionWithKV(title string, width int, pairs []KVPair) string {
	return Section(title, width) + "\n" + KVBlock(pairs)
}
