package glyphs

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func formatSet(s Set) string {
	var b strings.Builder
	fmt.Fprintf(&b, "CheckboxOn:  %q\n", s.CheckboxOn)
	fmt.Fprintf(&b, "CheckboxOff: %q\n", s.CheckboxOff)
	fmt.Fprintf(&b, "Cursor:      %q\n", s.Cursor)
	fmt.Fprintf(&b, "SelectAll:   %q\n", s.SelectAll)
	fmt.Fprintf(&b, "VoteYes:     %q\n", s.VoteYes)
	fmt.Fprintf(&b, "VoteNo:      %q\n", s.VoteNo)
	fmt.Fprintf(&b, "Star:        %q\n", s.Star)
	fmt.Fprintf(&b, "DotFilled:   %q\n", s.DotFilled)
	fmt.Fprintf(&b, "DotOpen:     %q\n", s.DotOpen)
	return b.String()
}

func TestASCIISet(t *testing.T) {
	golden.RequireEqual(t, formatSet(asciiSet))
}

func TestNerdSet(t *testing.T) {
	golden.RequireEqual(t, formatSet(nerdSet))
}
