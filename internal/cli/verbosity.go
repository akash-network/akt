package cli

import "pkg.akt.dev/akt/internal/cliutil"

// Re-export from cliutil so callers in internal/cli can use them directly.

var (
	Verbosity = cliutil.Verbosity
	IsQuiet   = cliutil.IsQuiet
	IsVerbose = cliutil.IsVerbose
	IsDebug   = cliutil.IsDebug
)
