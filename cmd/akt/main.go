package main

import (
	"fmt"
	"os"

	"pkg.akt.dev/akt/internal/cli"
)

// Build-time variables, set via ldflags.
// These are the only package-scope vars in the entire binary,
// required because Go's linker can only inject into package-scope vars.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	root := cli.NewRootCmd(cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})

	if err := cli.Execute(root); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}
