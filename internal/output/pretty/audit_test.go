package pretty

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	atypes "pkg.akt.dev/go/node/audit/v1"
	attrtypes "pkg.akt.dev/go/node/types/attributes/v1"
)

func TestRenderAuditList(t *testing.T) {
	tests := map[string]struct {
		res *atypes.QueryProvidersResponse
	}{
		"Empty": {
			res: &atypes.QueryProvidersResponse{},
		},
		"WithProviders": {
			res: &atypes.QueryProvidersResponse{
				Providers: atypes.AuditedProviders{
					{
						Owner:   "akash1provider001",
						Auditor: "akash1auditor001",
						Attributes: attrtypes.Attributes{
							{Key: "region", Value: "us-west"},
							{Key: "tier", Value: "premium"},
						},
					},
					{
						Owner:   "akash1provider002",
						Auditor: "akash1auditor001",
						Attributes: attrtypes.Attributes{
							{Key: "region", Value: "eu-central"},
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderAuditList(tc.res))
		})
	}
}
