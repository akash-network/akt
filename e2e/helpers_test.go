package e2e

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// aktBinary returns the path to the compiled akt binary.
func aktBinary(t *testing.T) string {
	t.Helper()
	// Find project root by looking for go.mod
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(filename))
	bin := filepath.Join(root, ".cache", "bin", "akt")
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Fatalf("akt binary not found at %s — run 'make akt' first", bin)
	}
	return bin
}

// runAkt runs the akt binary with the given home directory and arguments.
// Returns stdout, stderr, and exit code.
func runAkt(t *testing.T, home string, args ...string) (string, string, int) {
	t.Helper()
	bin := aktBinary(t)
	fullArgs := append([]string{"--home", home}, args...)
	cmd := exec.Command(bin, fullArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run akt: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

// initHome creates a minimal config.yaml in the given home directory
// to prevent the interactive bootstrap wizard from running.
func initHome(t *testing.T, home string) {
	t.Helper()
	cfg := `---
version: 1
current-context: ""
networks: []
keyrings:
  - name: default
    backend: test
contexts: []
defaults:
  output: pretty
  broadcast-mode: sync
`
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}
}

// runAktNoHome runs akt without --home (for commands like version/help).
func runAktNoHome(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	bin := aktBinary(t)
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run akt: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}
