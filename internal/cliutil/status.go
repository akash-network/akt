package cliutil

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Status writes an informational message to stderr.
// Suppressed when --quiet is set or stdout is not a TTY.
// Use for progress messages, confirmations, and operational feedback.
func Status(cmd *cobra.Command, msg string) {
	if IsQuiet(cmd) {
		return
	}

	if !IsTTY() {
		return
	}

	fmt.Fprintln(cmd.ErrOrStderr(), msg)
}

// Statusf is a formatted variant of Status.
func Statusf(cmd *cobra.Command, format string, args ...interface{}) {
	if IsQuiet(cmd) {
		return
	}

	if !IsTTY() {
		return
	}

	fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...)
}

// Verbose writes a message to stderr only when -v (or -vv) is active.
func Verbose(cmd *cobra.Command, msg string) {
	if !IsVerbose(cmd) {
		return
	}

	fmt.Fprintln(cmd.ErrOrStderr(), msg)
}

// Verbosef is a formatted variant of Verbose.
func Verbosef(cmd *cobra.Command, format string, args ...interface{}) {
	if !IsVerbose(cmd) {
		return
	}

	fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...)
}

// Debug writes a message to stderr only when -vv is active.
func Debug(cmd *cobra.Command, msg string) {
	if !IsDebug(cmd) {
		return
	}

	fmt.Fprintln(cmd.ErrOrStderr(), msg)
}

// Debugf is a formatted variant of Debug.
func Debugf(cmd *cobra.Command, format string, args ...interface{}) {
	if !IsDebug(cmd) {
		return
	}

	fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...)
}

// IsTTY returns true if stdout is connected to a terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
