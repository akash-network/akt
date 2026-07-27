package e2e

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
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

func TestTUIVersionCommand(t *testing.T) {
	stdout, _, exitCode := runAktWithTimeout(t, 5*time.Second, "version")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(strings.TrimSpace(stdout)) == 0 {
		t.Fatal("akt version should produce output")
	}
}

func TestTUINoArgNoTTY(t *testing.T) {
	// Without a TTY, akt should print and exit (per SPEC §2.0 step 4).
	// It must not hang waiting for TUI input, and with a fresh home it
	// must not silently run the first-run bootstrap (which would fetch
	// networks over the wire and write a config with fallback answers).
	// A temp home makes this hermetic — the CI failure mode was exactly
	// bare akt on a machine with no existing config.
	home := t.TempDir()
	_, stderr, _ := runAktWithTimeout(t, 5*time.Second, "--home", home)

	if _, err := os.Stat(filepath.Join(home, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("headless first run must not write a config, stderr:\n%s", stderr)
	}
}

func TestTUIContextListEmptyHome(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)
	stdout, _, _ := runAkt(t, home, "context", "list")
	// With empty config (no contexts), should show an empty table or message.
	// Just verify it doesn't panic.
	_ = stdout
}

func TestTUICompletionBash(t *testing.T) {
	stdout, _, exitCode := runAktWithTimeout(t, 5*time.Second, "completion", "bash")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "bash") && !strings.Contains(stdout, "complete") {
		t.Fatal("bash completion should contain shell completion code")
	}
}
