package e2e

import (
	"os"
	"path/filepath"
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
	if !strings.Contains(stdout, "akashnet-2") {
		t.Fatalf("expected context show to contain 'akashnet-2', got:\n%s", stdout)
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

// --- Phase 4: Store, Workflow, and Provider E2E tests ---

// setupContextHome creates a temp home with a network and active context,
// which is required for store commands to resolve a store directory.
func setupContextHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	initHome(t, home)

	// Create a network from the mainnet template.
	_, stderr, exitCode := runAkt(t, home, "context", "network", "create", "mainnet", "--template", "mainnet")
	if exitCode != 0 {
		t.Fatalf("failed to create mainnet network (exit %d): %s", exitCode, stderr)
	}

	// Create a context using that network and set it as current.
	_, stderr, exitCode = runAkt(t, home, "context", "create", "prod", "--network", "mainnet", "--set-current")
	if exitCode != 0 {
		t.Fatalf("failed to create context 'prod' (exit %d): %s", exitCode, stderr)
	}

	return home
}

func TestStoreStatusEmpty(t *testing.T) {
	home := setupContextHome(t)

	stdout, stderr, exitCode := runAkt(t, home, "store", "status")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}

	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "not synced") {
		t.Fatalf("expected output to contain 'not synced', got:\n%s", combined)
	}
	if !strings.Contains(combined, "0") {
		t.Fatalf("expected output to contain '0' (zero records), got:\n%s", combined)
	}
}

func TestStoreExportImport(t *testing.T) {
	home := setupContextHome(t)

	// Export the (empty) store to a temp file.
	exportFile := filepath.Join(t.TempDir(), "export.yaml")
	stdout, stderr, exitCode := runAkt(t, home, "store", "export", "--file", exportFile)
	if exitCode != 0 {
		t.Fatalf("store export failed (exit %d)\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}

	// Verify the export file was created.
	info, err := os.Stat(exportFile)
	if err != nil {
		t.Fatalf("export file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("export file is empty")
	}

	// Import the exported file into a fresh context.
	home2 := setupContextHome(t)
	stdout, stderr, exitCode = runAkt(t, home2, "store", "import", exportFile)
	if exitCode != 0 {
		t.Fatalf("store import failed (exit %d)\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
}

func TestDeployHelp(t *testing.T) {
	stdout, stderr, exitCode := runAktNoHome(t, "deploy", "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d\nstderr: %s", exitCode, stderr)
	}

	if !strings.Contains(strings.ToLower(stdout), "deploy") {
		t.Fatalf("expected help output to contain 'deploy', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "sdl-file") {
		t.Fatalf("expected help output to contain 'sdl-file', got:\n%s", stdout)
	}
}

func TestUpdateHelp(t *testing.T) {
	stdout, stderr, exitCode := runAktNoHome(t, "update", "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d\nstderr: %s", exitCode, stderr)
	}

	if !strings.Contains(strings.ToLower(stdout), "update") {
		t.Fatalf("expected help output to contain 'update', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "dseq") {
		t.Fatalf("expected help output to contain 'dseq', got:\n%s", stdout)
	}
}

func TestCloseHelp(t *testing.T) {
	stdout, stderr, exitCode := runAktNoHome(t, "close", "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d\nstderr: %s", exitCode, stderr)
	}

	if !strings.Contains(strings.ToLower(stdout), "close") {
		t.Fatalf("expected help output to contain 'close', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "dseq") {
		t.Fatalf("expected help output to contain 'dseq', got:\n%s", stdout)
	}
}

func TestProviderHelp(t *testing.T) {
	stdout, stderr, exitCode := runAktNoHome(t, "provider", "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d\nstderr: %s", exitCode, stderr)
	}

	for _, sub := range []string{"status", "lease-logs", "send-manifest"} {
		if !strings.Contains(stdout, sub) {
			t.Fatalf("expected provider help to list %q subcommand, got:\n%s", sub, stdout)
		}
	}
}
