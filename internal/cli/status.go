package cli

import "pkg.akt.dev/akt/internal/cliutil"

// Re-export from cliutil so callers in internal/cli can use them directly.

var (
	Status   = cliutil.Status
	Statusf  = cliutil.Statusf
	Verbose  = cliutil.Verbose
	Verbosef = cliutil.Verbosef
	Debug    = cliutil.Debug
	Debugf   = cliutil.Debugf
	IsTTY    = cliutil.IsTTY
)
