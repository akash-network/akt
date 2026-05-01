// Package cliutil provides shared CLI utilities (error types, status helpers,
// verbosity) that are safe to import from both internal/cli and internal/cli/chain
// without creating import cycles.
package cliutil

import (
	"errors"
	"fmt"
	"strings"
)

// Exit codes per SPEC §11.2.
const (
	ExitSuccess        = 0
	ExitGeneral        = 1
	ExitUsage          = 2
	ExitConfig         = 3
	ExitConnection     = 4
	ExitTransaction    = 5
	ExitAuth           = 6
	ExitStore          = 7
	ExitPluginNotFound = 127
)

// CLIError is a structured error with user-facing context per SPEC §11.1.
// Every CLIError renders as a three-part message:
//
//	Error: <what happened>
//	  Context:    <what was being attempted>
//	  Suggestion: <actionable next step>
type CLIError struct {
	Code       int    // exit code (see constants above)
	Message    string // user-facing summary (what happened)
	Cause      error  // underlying error (may be nil)
	Context    string // what was being attempted
	Suggestion string // actionable next step
}

func (e *CLIError) Error() string {
	var b strings.Builder

	b.WriteString("Error: ")
	b.WriteString(e.Message)

	if e.Cause != nil {
		b.WriteString("\n\n")
		b.WriteString("  Cause:      ")
		b.WriteString(e.Cause.Error())
	}

	if e.Context != "" {
		b.WriteString("\n")
		b.WriteString("  Context:    ")
		b.WriteString(e.Context)
	}

	if e.Suggestion != "" {
		b.WriteString("\n")
		b.WriteString("  Suggestion: ")
		b.WriteString(e.Suggestion)
	}

	return b.String()
}

func (e *CLIError) Unwrap() error {
	return e.Cause
}

// ExitCode extracts the exit code from an error. If the error is a *CLIError,
// its Code is returned. Otherwise returns ExitGeneral (1).
func ExitCode(err error) int {
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr.Code
	}

	return ExitGeneral
}

// Convenience constructors for common error categories.

func ErrUsage(msg string, cause error) *CLIError {
	return &CLIError{Code: ExitUsage, Message: msg, Cause: cause}
}

func ErrConfig(msg string, suggestion string) *CLIError {
	return &CLIError{Code: ExitConfig, Message: msg, Suggestion: suggestion}
}

func ErrConnection(endpoint string, cause error) *CLIError {
	return &CLIError{
		Code:       ExitConnection,
		Message:    "cannot connect to RPC endpoint",
		Cause:      cause,
		Context:    fmt.Sprintf("endpoint: %s", endpoint),
		Suggestion: "Check your network connection, or try a different endpoint.",
	}
}

func ErrTransaction(msg string, cause error) *CLIError {
	return &CLIError{Code: ExitTransaction, Message: msg, Cause: cause}
}

func ErrAuth(msg string, cause error) *CLIError {
	return &CLIError{Code: ExitAuth, Message: msg, Cause: cause}
}
