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
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sandbox template = %#v, want %#v", got, want)
	}
}
