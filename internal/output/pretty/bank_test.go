package pretty

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
)

func TestRenderCoinsTable(t *testing.T) {
	tests := map[string]struct {
		coins sdk.Coins
	}{
		"Empty": {
			coins: nil,
		},
		"WithCoins": {
			coins: sdk.NewCoins(
				sdk.NewInt64Coin("uakt", 5000000),
				sdk.NewInt64Coin("uusdc", 10000000),
			),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderCoinsTable(tc.coins))
		})
	}
}

func TestRenderBalance(t *testing.T) {
	tests := map[string]struct {
		res *types.QueryBalanceResponse
	}{
		"Nil": {
			res: &types.QueryBalanceResponse{
				Balance: nil,
			},
		},
		"WithBalance": {
			res: &types.QueryBalanceResponse{
				Balance: func() *sdk.Coin {
					c := sdk.NewInt64Coin("uakt", 5000000)
					return &c
				}(),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderBalance(tc.res))
		})
	}
}
