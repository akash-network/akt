package e2e

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// runAktWithTimeout runs the akt binary with a timeout to prevent hanging
// if a command accidentally launches a TUI.
func runAktWithTimeout(t *testing.T, timeout time.Duration, args ...string) (string, string, int) {
	t.Helper()
	bin := aktBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("command timed out after %s: akt %s", timeout, strings.Join(args, " "))
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run akt: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

func TestTUIMonitorHelp(t *testing.T) {
	stdout, _, exitCode := runAktWithTimeout(t, 5*time.Second, "monitor", "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Interactive TUI for monitoring Akash Network") {
		t.Fatalf("expected help to contain 'Interactive TUI for monitoring Akash Network', got:\n%s", stdout)
	}
	for _, sub := range []string{"network", "provider", "oracle", "bme"} {
		if !strings.Contains(stdout, sub) {
			t.Fatalf("expected monitor help to list %q subcommand, got:\n%s", sub, stdout)
		}
	}
}

func TestTUIMonitorNetworkHelp(t *testing.T) {
	stdout, _, exitCode := runAktWithTimeout(t, 5*time.Second, "monitor", "network", "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Network dashboard") {
		t.Fatalf("expected help to contain 'Network dashboard', got:\n%s", stdout)
	}
}

func TestTUIMonitorProviderHelp(t *testing.T) {
	stdout, _, exitCode := runAktWithTimeout(t, 5*time.Second, "monitor", "provider", "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Provider dashboard") {
		t.Fatalf("expected help to contain 'Provider dashboard', got:\n%s", stdout)
	}
}

func TestTUIMonitorOracleHelp(t *testing.T) {
	stdout, _, exitCode := runAktWithTimeout(t, 5*time.Second, "monitor", "oracle", "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Oracle/BME dashboard") {
		t.Fatalf("expected help to contain 'Oracle/BME dashboard', got:\n%s", stdout)
	}
}

func TestTUIMonitorBMEHelp(t *testing.T) {
	stdout, _, exitCode := runAktWithTimeout(t, 5*time.Second, "monitor", "bme", "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Oracle/BME dashboard") {
		t.Fatalf("expected help to contain 'Oracle/BME dashboard', got:\n%s", stdout)
	}
}

func TestTUIHelpDoesNotLaunchTUI(t *testing.T) {
	stdout, _, exitCode := runAktWithTimeout(t, 5*time.Second, "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Fatalf("expected help output to contain 'Usage:', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Akash Network") {
		t.Fatalf("expected help output to contain 'Akash Network', got:\n%s", stdout)
	}
}
