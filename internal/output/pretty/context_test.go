package pretty

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestRenderContextShow(t *testing.T) {
	tests := map[string]struct {
		ctx aktctx.Context
		// effective is the credential store that serves ctx.Keyring.Backend
		// on the host; "" means the host cannot provide it (SPEC §1.5).
		effective string
	}{
		"Full": {
			effective: "keychain",
			ctx: aktctx.Context{
				Name: "production",
				Network: aktctx.Network{
					Name:    "mainnet",
					ChainID: "akashnet-2",
					Endpoints: aktctx.Endpoints{
						RPC:  []string{"https://rpc.akash.network:443", "https://rpc-backup.akash.network:443"},
						API:  []string{"https://api.akash.network:443"},
						GRPC: []string{"grpc.akash.network:9090"},
					},
					GasPrices:     "0.025uakt",
					GasAdjustment: "1.5",
				},
				Keyring: aktctx.Keyring{
					Name:    "default",
					Backend: "os",
				},
				DefaultAccount:  "deployer",
				TrackedAccounts: []string{"deployer", "akash1zn43lm"},
				Gas:             "auto",
				Fees:            "5000uakt",
				GasPrices:       "0.025uakt",
				GasAdjustment:   "1.5",
				AuthType:        "certificate",
				Root:            "/home/user/.akt",
			},
		},
		"Minimal": {
			effective: "test",
			ctx: aktctx.Context{
				Name: "local",
				Network: aktctx.Network{
					Name:    "local",
					ChainID: "local-1",
					Endpoints: aktctx.Endpoints{
						RPC: []string{"http://localhost:26657"},
					},
				},
				Keyring: aktctx.Keyring{
					Name:    "test",
					Backend: "test",
				},
				DefaultAccount: "",
				Gas:            "200000",
				Fees:           "",
				GasPrices:      "",
				GasAdjustment:  "",
				AuthType:       "",
				Root:           "/tmp/akt-test",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderContextShow(tc.ctx, tc.effective))
		})
	}
}

// Every value in the view lands in one column, including the capability labels
// ("Chain transactions:") that are wider than the default key column and used
// to push their own values out of line (SPEC §10.12).
func TestRenderContextShowValuesShareOneColumn(t *testing.T) {
	rc := aktctx.Context{
		Name: "production",
		Network: aktctx.Network{
			Name:      "mainnet",
			ChainID:   "akashnet-2",
			Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc.akash.network:443"}},
		},
		Keyring:        aktctx.Keyring{Name: "default", Backend: "os"},
		DefaultAccount: "deployer",
		Gas:            "auto",
		Root:           "/home/user/.akt",
	}

	var (
		column = -1
		source string
	)

	// "os" is both configured and effective here, so no divergence sub-line
	// appears and the column check stays about the capability labels.
	for _, line := range plainLines(RenderContextShow(rc, "os")) {
		got, ok := valueColumnOf(line)
		if !ok {
			continue
		}

		if column == -1 {
			column, source = got, line
			continue
		}

		if got != column {
			t.Errorf("value column %d in %q, want %d as in %q", got, line, column, source)
		}
	}

	if column == -1 {
		t.Fatal("rendered no key-value lines")
	}
}

// TestRenderContextShowReportsUnavailableKeyring pins the honest form of the
// keyring line: a configured backend the host cannot provide is reported as
// unavailable, never echoed back as if it were in use (SPEC §1.5).
func TestRenderContextShowReportsUnavailableKeyring(t *testing.T) {
	rc := aktctx.Context{
		Name:    "headless",
		Network: aktctx.Network{Name: "mainnet", ChainID: "akashnet-2"},
		Keyring: aktctx.Keyring{Name: "default", Backend: "os"},
		Root:    "/home/user/.akt",
	}

	out := RenderContextShow(rc, "")

	if !strings.Contains(out, "unavailable") {
		t.Errorf("expected the keyring line to report unavailability, got:\n%s", out)
	}
	if strings.Contains(out, "Effective:     os") {
		t.Errorf("an unavailable backend must not be reported as effective, got:\n%s", out)
	}
}

func TestRenderContextShowConsolePreferredKeepsRawTransactions(t *testing.T) {
	rc := aktctx.Context{
		Name:          "managed",
		AuthMethod:    aktctx.AuthMethodConsoleAPI,
		ConsoleAPIKey: "sk-test",
		Network: aktctx.Network{
			Name:      "mainnet",
			Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc.example"}},
		},
		Keyring: aktctx.Keyring{Name: "default", Backend: "test"},
	}

	out := RenderContextShow(rc, "test")
	if !strings.Contains(out, "Deploy Via:") || !strings.Contains(out, "console") {
		t.Fatalf("context output does not show the preferred Console rail:\n%s", out)
	}
	if !strings.Contains(out, "API Key:") || !strings.Contains(out, "configured") {
		t.Fatalf("context output does not show the configured Console credential:\n%s", out)
	}

	for _, line := range plainLines(out) {
		if !strings.Contains(line, "Chain transactions") {
			continue
		}
		if !strings.Contains(line, "available") || strings.Contains(line, "unavailable") {
			t.Fatalf("chain transaction capability = %q, want available", line)
		}
		return
	}

	t.Fatal("context output omitted the chain transaction capability")
}

func TestRenderContextShowChainPreferredShowsConsoleCredential(t *testing.T) {
	rc := aktctx.Context{
		Name:          "dual-rail",
		AuthMethod:    aktctx.AuthMethodKeyring,
		ConsoleAPIKey: "sk-test",
		Network: aktctx.Network{
			Name:      "mainnet",
			Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc.example"}},
		},
		Keyring: aktctx.Keyring{Name: "default", Backend: "test"},
	}

	out := RenderContextShow(rc, "test")
	for _, want := range []string{"Deploy Via:", "chain", "Console API", "API Key:", "configured"} {
		if !strings.Contains(out, want) {
			t.Errorf("context output does not contain %q:\n%s", want, out)
		}
	}
}
