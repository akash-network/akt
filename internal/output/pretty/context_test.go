package pretty

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestRenderContextShow(t *testing.T) {
	tests := map[string]struct {
		ctx aktctx.Context
	}{
		"Full": {
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
				DefaultAccount: "deployer",
				Gas:            "auto",
				Fees:           "5000uakt",
				GasPrices:      "0.025uakt",
				GasAdjustment:  "1.5",
				AuthType:       "certificate",
				Root:           "/home/user/.akt",
			},
		},
		"Minimal": {
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
			golden.RequireEqual(t, RenderContextShow(tc.ctx))
		})
	}
}
