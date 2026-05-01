package pretty

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

func TestRenderDelegationTotalRewards(t *testing.T) {
	tests := map[string]struct {
		res *distrtypes.QueryDelegationTotalRewardsResponse
	}{
		"NoRewards": {
			res: &distrtypes.QueryDelegationTotalRewardsResponse{},
		},
		"WithRewards": {
			res: &distrtypes.QueryDelegationTotalRewardsResponse{
				Rewards: []distrtypes.DelegationDelegatorReward{
					{
						ValidatorAddress: "akashvaloper1abc123",
						Reward:           sdk.DecCoins{sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("5000000.000000000000000000"))},
					},
					{
						ValidatorAddress: "akashvaloper1def456",
						Reward:           sdk.DecCoins{sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("1500000.000000000000000000"))},
					},
				},
				Total: sdk.DecCoins{sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("6500000.000000000000000000"))},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderDelegationTotalRewards(tc.res))
		})
	}
}

func TestRenderValidatorCommission(t *testing.T) {
	tests := map[string]struct {
		commission *distrtypes.ValidatorAccumulatedCommission
	}{
		"NoCommission": {
			commission: &distrtypes.ValidatorAccumulatedCommission{},
		},
		"WithCommission": {
			commission: &distrtypes.ValidatorAccumulatedCommission{
				Commission: sdk.DecCoins{sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("2500000.000000000000000000"))},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderValidatorCommission(tc.commission))
		})
	}
}
