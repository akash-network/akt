package context

import (
	"reflect"
	"testing"
)

func TestNetworkTemplatesSandboxTargetsLiveNetwork(t *testing.T) {
	got, ok := NetworkTemplates()["sandbox"]
	if !ok {
		t.Fatal("sandbox network template is missing")
	}

	want := Network{
		Name:    "sandbox",
		ChainID: "sandbox-2",
		Endpoints: Endpoints{
			RPC:  []string{"https://rpc.sandbox-2.aksh.pw:443"},
			API:  []string{"https://api.sandbox-2.aksh.pw:443"},
			GRPC: []string{"grpc.sandbox-2.aksh.pw:9090"},
		},
		GasPrices:     "0.025uakt",
		GasAdjustment: "1.5",
		Faucet:        "http://faucet.sandbox-2.aksh.pw/",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sandbox template = %#v, want %#v", got, want)
	}
}

// TestNetworkTemplatesMainnetHasNoFaucet pins the absence: mainnet's
// upstream meta.json publishes no faucets entry, and akt must not invent one
// (SPEC §1.10). Presence of Network.Faucet is the sole signal akt faucet uses
// to decide a network has a faucet, so a stray value here would silently
// offer a faucet on the live network.
func TestNetworkTemplatesMainnetHasNoFaucet(t *testing.T) {
	got, ok := NetworkTemplates()["mainnet"]
	if !ok {
		t.Fatal("mainnet network template is missing")
	}
	if got.Faucet != "" {
		t.Errorf("mainnet template faucet = %q, want empty", got.Faucet)
	}
}
