package pretty

import (
	"testing"

	"cosmossdk.io/x/feegrant"
	"github.com/charmbracelet/x/exp/golden"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
)

func TestRenderFeeGrants(t *testing.T) {
	tests := map[string]struct {
		grants []*feegrant.Grant
	}{
		"Empty": {
			grants: nil,
		},
		"WithGrants": {
			grants: []*feegrant.Grant{
				{
					Granter: "akash1granter001",
					Grantee: "akash1grantee001",
					Allowance: &codectypes.Any{
						TypeUrl: "/cosmos.feegrant.v1beta1.BasicAllowance",
					},
				},
				{
					Granter: "akash1granter002",
					Grantee: "akash1grantee002",
					Allowance: &codectypes.Any{
						TypeUrl: "/cosmos.feegrant.v1beta1.PeriodicAllowance",
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderFeeGrants(tc.grants))
		})
	}
}
