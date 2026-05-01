package pretty

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestRenderNetworkShow(t *testing.T) {
	tests := map[string]struct {
		net    aktctx.Network
		usedBy []string
	}{
		"Full": {
			net: aktctx.Network{
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
			usedBy: []string{"production", "staging"},
		},
		"Minimal": {
			net: aktctx.Network{
				Name:    "local",
				ChainID: "local-1",
				Endpoints: aktctx.Endpoints{
					RPC: []string{"http://localhost:26657"},
				},
				GasPrices:     "0uakt",
				GasAdjustment: "1.0",
			},
			usedBy: nil,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderNetworkShow(tc.net, tc.usedBy))
		})
	}
}

func TestRenderNetworkList(t *testing.T) {
	tests := map[string]struct {
		nets      []aktctx.Network
		getUsedBy func(name string) []string
	}{
		"Empty": {
			nets:      nil,
			getUsedBy: nil,
		},
		"WithNetworks": {
			nets: []aktctx.Network{
				{
					Name:    "mainnet",
					ChainID: "akashnet-2",
					Endpoints: aktctx.Endpoints{
						RPC: []string{"https://rpc.akash.network:443", "https://rpc-backup.akash.network:443"},
					},
					GasPrices:     "0.025uakt",
					GasAdjustment: "1.5",
				},
				{
					Name:    "testnet",
					ChainID: "testnet-1",
					Endpoints: aktctx.Endpoints{
						RPC: []string{"https://rpc.testnet.akash.network:443"},
					},
					GasPrices:     "0uakt",
					GasAdjustment: "1.0",
				},
			},
			getUsedBy: func(name string) []string {
				switch name {
				case "mainnet":
					return []string{"production", "staging"}
				case "testnet":
					return nil
				default:
					return nil
				}
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderNetworkList(tc.nets, tc.getUsedBy))
		})
	}
}
