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
