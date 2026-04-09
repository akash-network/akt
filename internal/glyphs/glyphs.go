// Package glyphs provides a centralized registry of all PUA-range (Nerd Font)
// glyphs used throughout the akt UI, with ASCII fallbacks for terminals that
// do not have a Nerd Font installed.
//
// All rendering code must reference glyphs via this package rather than using
// inline PUA string literals. The active glyph set is selected at startup by
// [Init] based on the resolved glyph mode (auto/nerd/ascii).
package glyphs

import (
	"fmt"
	"strings"
	"sync"
)

// Mode controls which glyph variant is used for rendering.
type Mode string

const (
	// ModeAuto selects ascii glyphs (font detection is disabled).
	// Users who have a Nerd Font can opt in with ModeNerd.
	ModeAuto Mode = "auto"
	// ModeNerd forces Nerd Font (PUA range) glyphs.
	ModeNerd Mode = "nerd"
	// ModeASCII forces pure-ASCII fallback glyphs.
	ModeASCII Mode = "ascii"
)

// Set holds every glyph used by the application. Each field has a nerd and an
// ascii variant. Field names are semantic: they describe the visual role, not
// the specific Unicode codepoint.
type Set struct {
	// CheckboxOn is a checked/selected indicator for multiselect rows.
	CheckboxOn string
	// CheckboxOff is an unchecked/unselected indicator for multiselect rows.
	CheckboxOff string
	// Cursor is a row-selection pointer (e.g., caret next to highlighted item).
	Cursor string
	// SelectAll is the icon shown next to the "Select all" row in multiselect.
	SelectAll string

	// VoteYes indicates a confirmed vote (prevote/precommit) in monitor grids.
	VoteYes string
	// VoteNo indicates a missing vote in monitor grids.
	VoteNo string

	// Star marks the block proposer in the monitor validator view.
	Star string

	// DotFilled is a filled dot for selected version indicators.
	DotFilled string
	// DotOpen is an open dot for unselected version indicators.
	DotOpen string
}

// nerdSet contains the Nerd Font (Font Awesome PUA range) glyphs.
var nerdSet = Set{
	CheckboxOn:  "\uf00c", // nf-fa-check
	CheckboxOff: "\uf10c", // nf-fa-circle_o
	Cursor:      "\uf0da", // nf-fa-caret_right
	SelectAll:   "\uf0c8", // nf-fa-th_large

	VoteYes: "\uf00c", // nf-fa-check
	VoteNo:  "\uf00d", // nf-fa-times

	Star: "\uf005", // nf-fa-star

	DotFilled: "\uf111", // nf-fa-circle
	DotOpen:   "\uf10c", // nf-fa-circle_o
}

// asciiSet contains pure-ASCII fallbacks that render correctly in any font.
var asciiSet = Set{
	CheckboxOn:  "[x]",
	CheckboxOff: "[ ]",
	Cursor:      ">",
	SelectAll:   "#",

	VoteYes: "+",
	VoteNo:  "-",

	Star: "*",

	DotFilled: "*",
	DotOpen:   "o",
}

var (
	initOnce  sync.Once
	activeSet *Set
)

// Init resolves the glyph mode and locks in the active glyph set for the
// lifetime of the process. It must be called once during startup (typically
// in root command PersistentPreRunE). Subsequent calls are no-ops.
//
// When mode is [ModeAuto], ascii is used (font detection is disabled).
// Users who have a Nerd Font can opt in with --glyph-mode=nerd.
func Init(mode Mode) {
	initOnce.Do(func() {
		resolved := resolve(mode)
		switch resolved {
		case ModeNerd:
			activeSet = &nerdSet
		default:
			activeSet = &asciiSet
		}
	})
}

// G returns the active glyph set. If [Init] has not been called, it defaults
// to ascii to guarantee safe rendering.
func G() *Set {
	if activeSet == nil {
		return &asciiSet
	}
	return activeSet
}

// resolve determines the final mode from the requested mode.
// ModeAuto resolves to ModeASCII (font detection is disabled — it put the
// terminal in raw mode and leaked goroutines that consumed keystrokes).
// Users who have a Nerd Font can opt in with --glyph-mode=nerd.
func resolve(mode Mode) Mode {
	switch mode {
	case ModeNerd:
		return ModeNerd
	case ModeASCII:
		return ModeASCII
	case ModeAuto:
		return ModeASCII
	default:
		return ModeASCII
	}
}

// ParseMode converts a user-supplied string to a [Mode]. Returns an error for
// unrecognised values.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "auto", "":
		return ModeAuto, nil
	case "nerd":
		return ModeNerd, nil
	case "ascii":
		return ModeASCII, nil
	default:
		return "", fmt.Errorf("invalid glyph mode %q: must be auto, nerd, or ascii", s)
	}
}
