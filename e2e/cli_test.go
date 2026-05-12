package e2e

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	stdout, _, exitCode := runAktNoHome(t, "version")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(strings.TrimSpace(stdout)) == 0 {
		t.Fatal("expected non-empty version output")
	}
}

func TestHelp(t *testing.T) {
	stdout, _, exitCode := runAktNoHome(t, "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Akash Network") {
		t.Fatalf("expected stdout to contain 'Akash Network', got:\n%s", stdout)
	}
}

func TestCompletionGeneration(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			stdout, _, exitCode := runAktNoHome(t, "completion", shell)
			if exitCode != 0 {
				t.Fatalf("expected exit code 0 for %s completion, got %d", shell, exitCode)
			}
			if len(strings.TrimSpace(stdout)) == 0 {
				t.Fatalf("expected non-empty %s completion output", shell)
			}
		})
	}
}

func TestNetworkTemplates(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)

	// Create mainnet
	_, _, exitCode := runAkt(t, home, "context", "network", "create", "mainnet", "--template", "mainnet")
	if exitCode != 0 {
		t.Fatalf("failed to create mainnet network, exit code %d", exitCode)
	}

	// Create testnet
	_, _, exitCode = runAkt(t, home, "context", "network", "create", "testnet", "--template", "testnet")
	if exitCode != 0 {
		t.Fatalf("failed to create testnet network, exit code %d", exitCode)
	}

	// List networks
	stdout, _, exitCode := runAkt(t, home, "context", "network", "list")
	if exitCode != 0 {
		t.Fatalf("failed to list networks, exit code %d", exitCode)
	}
	if !strings.Contains(stdout, "mainnet") {
		t.Fatalf("expected network list to contain 'mainnet', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "testnet") {
		t.Fatalf("expected network list to contain 'testnet', got:\n%s", stdout)
	}

	// Show mainnet
	stdout, _, exitCode = runAkt(t, home, "context", "network", "show", "mainnet")
	if exitCode != 0 {
		t.Fatalf("failed to show mainnet network, exit code %d", exitCode)
	}
	if !strings.Contains(stdout, "akashnet-2") {
		t.Fatalf("expected mainnet show to contain 'akashnet-2', got:\n%s", stdout)
	}
}

func TestContextLifecycle(t *testing.T) {
	home := t.TempDir()
	initHome(t, home)

	// Create network first
	_, _, exitCode := runAkt(t, home, "context", "network", "create", "mainnet", "--template", "mainnet")
	if exitCode != 0 {
		t.Fatalf("failed to create mainnet network, exit code %d", exitCode)
	}

	// Create context
	_, _, exitCode = runAkt(t, home, "context", "create", "prod", "--network", "mainnet", "--set-current")
	if exitCode != 0 {
		t.Fatalf("failed to create context 'prod', exit code %d", exitCode)
	}

	// List contexts
	stdout, _, exitCode := runAkt(t, home, "context", "list")
	if exitCode != 0 {
		t.Fatalf("failed to list contexts, exit code %d", exitCode)
	}
	if !strings.Contains(stdout, "prod") {
		t.Fatalf("expected context list to contain 'prod', got:\n%s", stdout)
	}

	// Show current context
	stdout, _, exitCode = runAkt(t, home, "context", "show")
	if exitCode != 0 {
		t.Fatalf("failed to show context, exit code %d", exitCode)
	}
	if !strings.Contains(stdout, "prod") {
		t.Fatalf("expected context show to contain 'prod', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "mainnet") {
		t.Fatalf("expected context show to contain 'mainnet', got:\n%s", stdout)
	}

	// Create a second context so we can delete the first
	_, _, exitCode = runAkt(t, home, "context", "create", "staging", "--network", "mainnet")
	if exitCode != 0 {
		t.Fatalf("failed to create context 'staging', exit code %d", exitCode)
	}

	// Rename prod -> production
	_, _, exitCode = runAkt(t, home, "context", "rename", "prod", "production")
	if exitCode != 0 {
		t.Fatalf("failed to rename context, exit code %d", exitCode)
	}

	// List again — should contain "production", not "prod"
	stdout, _, exitCode = runAkt(t, home, "context", "list")
	if exitCode != 0 {
		t.Fatalf("failed to list contexts after rename, exit code %d", exitCode)
	}
	if !strings.Contains(stdout, "production") {
		t.Fatalf("expected context list to contain 'production', got:\n%s", stdout)
	}

	// Switch to staging so we can delete production (can't delete current)
	_, _, exitCode = runAkt(t, home, "context", "use", "staging")
	if exitCode != 0 {
		t.Fatalf("failed to switch to staging context, exit code %d", exitCode)
	}

	// Delete production context
	_, _, exitCode = runAkt(t, home, "context", "delete", "production", "--yes")
	if exitCode != 0 {
		t.Fatalf("failed to delete context, exit code %d", exitCode)
	}

	// List — should not contain "production"
	stdout, _, exitCode = runAkt(t, home, "context", "list")
	if exitCode != 0 {
		t.Fatalf("failed to list contexts after delete, exit code %d", exitCode)
	}
	if strings.Contains(stdout, "production") {
		t.Fatalf("expected context list to NOT contain 'production' after delete, got:\n%s", stdout)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, stderr, exitCode := runAktNoHome(t, "nonexistent")
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for unknown command")
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("expected stderr to contain 'unknown command', got:\n%s", stderr)
	}
}
