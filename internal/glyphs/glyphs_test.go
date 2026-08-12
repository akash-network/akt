package glyphs

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/require"
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

func TestParseMode(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  Mode
	}{
		{input: "", want: ModeAuto},
		{input: " AUTO ", want: ModeAuto},
		{input: "NeRd", want: ModeNerd},
		{input: "ascii", want: ModeASCII},
	} {
		mode, err := ParseMode(tc.input)
		require.NoError(t, err)
		require.Equal(t, tc.want, mode)
	}

	mode, err := ParseMode("emoji")
	require.ErrorContains(t, err, "invalid glyph mode")
	require.Empty(t, mode)
}

func TestResolve(t *testing.T) {
	require.Equal(t, ModeNerd, resolve(ModeNerd))
	require.Equal(t, ModeASCII, resolve(ModeASCII))
	require.Equal(t, ModeASCII, resolve(ModeAuto))
	require.Equal(t, ModeASCII, resolve(Mode("unknown")))
}

func TestInitLocksFirstResolvedSet(t *testing.T) {
	initOnce = sync.Once{}
	activeSet = nil
	t.Cleanup(func() {
		initOnce = sync.Once{}
		activeSet = nil
	})

	require.Same(t, &asciiSet, G())
	Init(ModeNerd)
	require.Same(t, &nerdSet, G())
	Init(ModeASCII)
	require.Same(t, &nerdSet, G(), "subsequent initialization must be a no-op")
}

func TestInitDefaultsToASCII(t *testing.T) {
	initOnce = sync.Once{}
	activeSet = nil
	t.Cleanup(func() {
		initOnce = sync.Once{}
		activeSet = nil
	})

	Init(ModeAuto)
	require.Same(t, &asciiSet, G())
}
