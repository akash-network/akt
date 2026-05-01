package cli

import "pkg.akt.dev/akt/internal/cliutil"

// Re-export from cliutil so existing callers (main.go) continue to work.

const (
	ExitSuccess        = cliutil.ExitSuccess
	ExitGeneral        = cliutil.ExitGeneral
	ExitUsage          = cliutil.ExitUsage
	ExitConfig         = cliutil.ExitConfig
	ExitConnection     = cliutil.ExitConnection
	ExitTransaction    = cliutil.ExitTransaction
	ExitAuth           = cliutil.ExitAuth
	ExitStore          = cliutil.ExitStore
	ExitPluginNotFound = cliutil.ExitPluginNotFound
)

type CLIError = cliutil.CLIError

var (
	ExitCode      = cliutil.ExitCode
	ErrUsage      = cliutil.ErrUsage
	ErrConfig     = cliutil.ErrConfig
	ErrConnection = cliutil.ErrConnection
	ErrTransaction = cliutil.ErrTransaction
	ErrAuth       = cliutil.ErrAuth
)
