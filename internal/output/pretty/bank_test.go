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
		// An IBC denom is far wider than the header, so the header only sits
		// over its own column if it is right-aligned like the amounts
		// (`akt query bank total` used to float it mid-column).
		"WithWideDenom": {
			coins: sdk.NewCoins(
				sdk.NewInt64Coin("ibc/011C19FB6113363238248C55B985A92C0A0CAF9709162EAB838EACB6A629E6AA", 1950000),
				sdk.NewInt64Coin("uakt", 5000000),
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
