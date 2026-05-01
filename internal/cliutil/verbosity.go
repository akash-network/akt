package cliutil

import (
	"github.com/spf13/cobra"
)

// Verbosity reads the counted -v flag. Returns -1 when --quiet is set,
// 0 for default, 1 for -v, 2 for -vv. Safe to call even when flags are
// not registered (returns 0).
func Verbosity(cmd *cobra.Command) int {
	q, err := cmd.Flags().GetBool("quiet")
	if err == nil && q {
		return -1
	}

	v, err := cmd.Flags().GetCount("verbose")
	if err != nil {
		return 0
	}

	return v
}

// IsQuiet returns true when --quiet is set (verbosity < 0).
func IsQuiet(cmd *cobra.Command) bool { return Verbosity(cmd) < 0 }

// IsVerbose returns true when -v or -vv is set (verbosity >= 1).
func IsVerbose(cmd *cobra.Command) bool { return Verbosity(cmd) >= 1 }

// IsDebug returns true when -vv is set (verbosity >= 2).
func IsDebug(cmd *cobra.Command) bool { return Verbosity(cmd) >= 2 }
