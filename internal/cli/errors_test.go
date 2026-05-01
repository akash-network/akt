package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestCLIError_Error_ThreeParts(t *testing.T) {
	err := &CLIError{
		Code:       ExitConnection,
		Message:    "cannot connect to RPC endpoint",
		Cause:      errors.New("connection refused"),
		Context:    "endpoint: https://rpc.akashnet.net:443",
		Suggestion: "Check your network connection.",
	}

	s := err.Error()

	if !strings.Contains(s, "Error: cannot connect to RPC endpoint") {
		t.Errorf("missing message: %s", s)
	}

	if !strings.Contains(s, "Cause:      connection refused") {
		t.Errorf("missing cause: %s", s)
	}

	if !strings.Contains(s, "Context:    endpoint: https://rpc.akashnet.net:443") {
		t.Errorf("missing context: %s", s)
	}

	if !strings.Contains(s, "Suggestion: Check your network connection.") {
		t.Errorf("missing suggestion: %s", s)
	}
}

func TestCLIError_Error_MessageOnly(t *testing.T) {
	err := &CLIError{
		Code:    ExitGeneral,
		Message: "something failed",
	}

	s := err.Error()
	if s != "Error: something failed" {
		t.Errorf("unexpected: %q", s)
	}
}

func TestCLIError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &CLIError{Code: ExitGeneral, Message: "wrapper", Cause: cause}

	if !errors.Is(err, cause) {
		t.Error("Unwrap should expose the cause")
	}
}

func TestExitCode_CLIError(t *testing.T) {
	err := &CLIError{Code: ExitConfig, Message: "bad config"}
	if got := ExitCode(err); got != ExitConfig {
		t.Errorf("expected %d, got %d", ExitConfig, got)
	}
}

func TestExitCode_PlainError(t *testing.T) {
	err := errors.New("plain")
	if got := ExitCode(err); got != ExitGeneral {
		t.Errorf("expected %d, got %d", ExitGeneral, got)
	}
}

func TestExitCode_WrappedCLIError(t *testing.T) {
	inner := &CLIError{Code: ExitTransaction, Message: "tx failed"}
	wrapped := errors.Join(errors.New("extra context"), inner)

	if got := ExitCode(wrapped); got != ExitTransaction {
		t.Errorf("expected %d through errors.As, got %d", ExitTransaction, got)
	}
}

func TestErrConnection(t *testing.T) {
	err := ErrConnection("https://rpc.example.com:443", errors.New("timeout"))
	if err.Code != ExitConnection {
		t.Errorf("expected code %d, got %d", ExitConnection, err.Code)
	}

	if !strings.Contains(err.Context, "rpc.example.com") {
		t.Errorf("context should contain endpoint: %s", err.Context)
	}
}
