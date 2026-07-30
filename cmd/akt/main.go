package main

import (
	"fmt"
	"os"

	sdkversion "github.com/cosmos/cosmos-sdk/version"

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
	// The clean-copied chain commands build their examples from the SDK's
	// AppName, which the release ldflags set. A plain `go build` or
	// `go install` does not, leaving "<appd> query ..." in the examples of
	// roughly 95 commands. Default it here so the help is correct however the
	// binary was produced; ldflags still win when they are supplied.
	if sdkversion.AppName == "" || sdkversion.AppName == "<appd>" {
		sdkversion.AppName = "akt"
	}

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
