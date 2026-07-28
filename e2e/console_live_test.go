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

// activeConsoleDSeq returns the dseq of a Console deployment that is active
// and has an active lease, or "" when the account has none. The gateway read
// paths resolve their provider from the active lease, so a deployment without
// one cannot exercise them.
func activeConsoleDSeq(t *testing.T, home string) string {
	t.Helper()

	stdout, stderr, exit := runAkt(t, home, "console", "deployment", "list")
	if exit != 0 {
		t.Fatalf("console deployment list failed (exit %d): %s", exit, stderr)
	}

	var listing struct {
		Deployments []struct {
			Deployment struct {
				State string `json:"state"`
				ID    struct {
					DSeq string `json:"dseq"`
				} `json:"id"`
			} `json:"deployment"`
			Leases []struct {
				State string `json:"state"`
			} `json:"leases"`
		} `json:"deployments"`
	}
	if err := json.Unmarshal([]byte(stdout), &listing); err != nil {
		t.Fatalf("could not parse console deployment list: %v", err)
	}

	for _, d := range listing.Deployments {
		if d.Deployment.State != "active" {
			continue
		}
		for _, l := range d.Leases {
			if l.State == "" || l.State == "active" {
				return d.Deployment.ID.DSeq
			}
		}
	}

	return ""
}

// TestConsoleLiveDeploymentReads covers the read paths that need a live dseq,
// which the argument-free matrix cannot reach: lease status, the bid list,
// and the provider-gateway log fetch. All are read-only.
//
// These run against whatever the account already has; they never create a
// deployment, because doing so spends real funds.
func TestConsoleLiveDeploymentReads(t *testing.T) {
	key := os.Getenv(envConsoleLiveKey)
	if key == "" {
		t.Skipf("skipping live Console API smoke tests: %s is not set", envConsoleLiveKey)
	}

	home := setupContextHome(t)
	if _, stderr, exit := runAkt(t, home, "context", "edit", "prod", "--console-api-key", key); exit != 0 {
		t.Fatalf("failed to store Console API key on context (exit %d): %s", exit, stderr)
	}

	dseq := activeConsoleDSeq(t, home)
	if dseq == "" {
		t.Skip("account has no active deployment with an active lease")
	}

	t.Run("status", func(t *testing.T) {
		stdout, stderr, exit := runAkt(t, home, "console", "status", dseq)
		if exit != 0 {
			t.Fatalf("console status %s failed (exit %d): %s", dseq, exit, stderr)
		}
		if !json.Valid([]byte(strings.TrimSpace(stdout))) {
			t.Fatalf("console status output is not valid JSON:\n%s", stdout)
		}
	})

	t.Run("bid list", func(t *testing.T) {
		stdout, stderr, exit := runAkt(t, home, "console", "bid", "list", dseq)
		if exit != 0 {
			t.Fatalf("console bid list %s failed (exit %d): %s", dseq, exit, stderr)
		}
		if !json.Valid([]byte(strings.TrimSpace(stdout))) {
			t.Fatalf("console bid list output is not valid JSON:\n%s", stdout)
		}
	})

	// A one-shot log fetch ends when the provider closes the connection,
	// which arrives as an EOF. That used to be reported as an error, so the
	// command exited non-zero after printing every line — this asserts the
	// exit status, not just the output.
	t.Run("logs exit status", func(t *testing.T) {
		stdout, stderr, exit := runAkt(t, home, "console", "logs", dseq)
		if exit != 0 {
			t.Fatalf("console logs %s exited %d after streaming\nstdout: %s\nstderr: %s",
				dseq, exit, stdout, stderr)
		}
	})
}
