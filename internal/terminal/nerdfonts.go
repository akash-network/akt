package terminal

import (
	"errors"
	"sync"

	"pkg.akt.dev/akt/internal/glyphs"
)

var (
	checkOnce   sync.Once
	checkResult error
)

// CheckNerdFont probes the terminal to verify that a Nerd Font is active.
//
// Deprecated: Use [glyphs.Init] with [glyphs.ModeAuto] instead, which
// gracefully falls back to ASCII rather than returning an error.
// This function is retained for backward compatibility but delegates to
// [glyphs.DetectMode].
//
// Returns nil if a Nerd Font is detected or detection cannot be performed.
// Returns a descriptive error with install/upgrade instructions otherwise.
func CheckNerdFont() error {
	checkOnce.Do(func() {
		mode := glyphs.DetectMode()
		if mode != glyphs.ModeNerd {
			checkResult = errors.New(`Nerd Font not detected

TUI mode and pretty output look best with a Nerd Font installed and
configured in your terminal emulator. Without one, the interface falls
back to ASCII-safe glyphs automatically.

  Install:  https://www.nerdfonts.com/
  Popular:  "JetBrainsMono Nerd Font", "FiraCode Nerd Font", "Hack Nerd Font"

To force Nerd Font glyphs: --glyph-mode nerd
To suppress this check:    --glyph-mode ascii`)
		}
	})
	return checkResult
}
