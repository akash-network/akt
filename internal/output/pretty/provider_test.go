package pretty

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	attrtypes "pkg.akt.dev/go/node/types/attributes/v1"
)

func TestRenderProviderList(t *testing.T) {
	tests := map[string]struct {
		res *ptypes.QueryProvidersResponse
	}{
		"Empty": {
			res: &ptypes.QueryProvidersResponse{},
		},
		"WithProviders": {
			res: &ptypes.QueryProvidersResponse{
				Providers: []ptypes.Provider{
					{
						Owner:   "akash1provider001",
						HostURI: "https://provider1.example.com:8443",
						Info: ptypes.Info{
							EMail:   "admin@provider1.example.com",
							Website: "https://provider1.example.com",
						},
					},
					{
						Owner:   "akash1provider002",
						HostURI: "https://provider2.example.com:8443",
						Info: ptypes.Info{
							EMail:   "ops@provider2.example.com",
							Website: "https://provider2.example.com",
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderProviderList(tc.res))
		})
	}
}

func TestRenderProviderDetail(t *testing.T) {
	tests := map[string]struct {
		res *ptypes.QueryProviderResponse
	}{
		"Basic": {
			res: &ptypes.QueryProviderResponse{
				Provider: ptypes.Provider{
					Owner:   "akash1provider001",
					HostURI: "https://provider1.example.com:8443",
					Info: ptypes.Info{
						EMail:   "admin@provider1.example.com",
						Website: "https://provider1.example.com",
					},
				},
			},
		},
		"WithAttributes": {
			res: &ptypes.QueryProviderResponse{
				Provider: ptypes.Provider{
					Owner:   "akash1provider001",
					HostURI: "https://provider1.example.com:8443",
					Info: ptypes.Info{
						EMail:   "admin@provider1.example.com",
						Website: "https://provider1.example.com",
					},
					Attributes: attrtypes.Attributes{
						{Key: "region", Value: "us-west"},
						{Key: "tier", Value: "premium"},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderProviderDetail(tc.res))
		})
	}
}
