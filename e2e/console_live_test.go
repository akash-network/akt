package e2e

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// envConsoleLiveKey opts the live Console API smoke suite in. When unset the
// suite is skipped, so normal CI runs never touch the network.
const envConsoleLiveKey = "AKT_E2E_CONSOLE_API_KEY"

// TestConsoleLive smoke-tests READ-ONLY `akt console` commands against the
// production Console API. It complements the offline OpenAPI contract tests
// in internal/console: those pin the client to the vendored spec, this one
// catches spec drift on the live API.
//
// Opt-in: set AKT_E2E_CONSOLE_API_KEY to a valid Console API key. The suite
// stores the key on a scratch context and runs only read-only commands — it
// never creates, mutates, deposits to, or closes anything.
func TestConsoleLive(t *testing.T) {
	key := os.Getenv(envConsoleLiveKey)
	if key == "" {
		t.Skipf("skipping live Console API smoke tests: %s is not set", envConsoleLiveKey)
	}

	home := setupContextHome(t)

	// Store the key as the context's Console credential, the same way a user
	// would: akt context edit <ctx> --console-api-key <key>.
	if _, stderr, exit := runAkt(t, home, "context", "edit", "prod", "--console-api-key", key); exit != 0 {
		t.Fatalf("failed to store Console API key on context (exit %d): %s", exit, stderr)
	}

	tests := []struct {
		name    string
		args    []string
		markers []string // substrings expected in stdout
	}{
		{"whoami", []string{"console", "whoami"}, []string{"username"}},
		{"wallet balance", []string{"console", "wallet", "balance"}, []string{"available", "total"}},
		// No flags on purpose: this is the exact request shape that returned
		// HTTP 400 when the client sent empty startDate/endDate.
		{"usage", []string{"console", "usage"}, []string{"totalSpent", "history"}},
		{"provider regions", []string{"console", "provider", "regions"}, nil},
		{"gpu", []string{"console", "gpu"}, []string{"availability", "models"}},
		{"template list", []string{"console", "template", "list"}, nil},
		// The remaining read paths that need no arguments. Every console
		// subcommand not listed here either mutates (deployment create /
		// close / deposit / update, lease create, apikey create / delete)
		// or needs a live dseq (status, logs, events, shell, screen, bid
		// list) — neither belongs in a suite that must be safe to point at
		// a real funded account.
		{"deployment list", []string{"console", "deployment", "list"}, nil},
		{"provider list", []string{"console", "provider", "list"}, nil},
		{"provider auditors", []string{"console", "provider", "auditors"}, nil},
		{"wallet list", []string{"console", "wallet", "list"}, nil},
		// Lists key metadata only; the secret is returned once, at create.
		{"apikey list", []string{"console", "apikey", "list"}, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdline := "akt " + strings.Join(tc.args, " ")

			stdout, stderr, exit := runAkt(t, home, tc.args...)
			if exit != 0 {
				t.Fatalf("%s failed (exit %d)\nstdout: %s\nstderr: %s", cmdline, exit, stdout, stderr)
			}

			trimmed := strings.TrimSpace(stdout)
			if trimmed == "" {
				t.Fatalf("%s produced no output", cmdline)
			}
			if !json.Valid([]byte(trimmed)) {
				t.Fatalf("%s output is not valid JSON:\n%s", cmdline, stdout)
			}

			for _, marker := range tc.markers {
				if !strings.Contains(stdout, marker) {
					t.Errorf("%s output missing %q:\n%s", cmdline, marker, stdout)
				}
			}
		})
	}
}
