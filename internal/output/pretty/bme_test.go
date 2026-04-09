package pretty

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/bme/v1"
)

func TestRenderBMEStatus(t *testing.T) {
	tests := map[string]struct {
		status *types.QueryStatusResponse
	}{
		"Healthy": {
			status: &types.QueryStatusResponse{
				Status:          types.MintStatusHealthy,
				MintsAllowed:    true,
				RefundsAllowed:  true,
				CollateralRatio: math.LegacyMustNewDecFromStr("1.500000000000000000"),
				WarnThreshold:   math.LegacyMustNewDecFromStr("1.200000000000000000"),
				HaltThreshold:   math.LegacyMustNewDecFromStr("1.050000000000000000"),
			},
		},
		"Warning": {
			status: &types.QueryStatusResponse{
				Status:          types.MintStatusWarning,
				MintsAllowed:    true,
				RefundsAllowed:  true,
				CollateralRatio: math.LegacyMustNewDecFromStr("1.150000000000000000"),
				WarnThreshold:   math.LegacyMustNewDecFromStr("1.200000000000000000"),
				HaltThreshold:   math.LegacyMustNewDecFromStr("1.050000000000000000"),
			},
		},
		"HaltCR": {
			status: &types.QueryStatusResponse{
				Status:          types.MintStatusHaltCR,
				MintsAllowed:    false,
				RefundsAllowed:  false,
				CollateralRatio: math.LegacyMustNewDecFromStr("1.010000000000000000"),
				WarnThreshold:   math.LegacyMustNewDecFromStr("1.200000000000000000"),
				HaltThreshold:   math.LegacyMustNewDecFromStr("1.050000000000000000"),
			},
		},
		"HaltOracle": {
			status: &types.QueryStatusResponse{
				Status:          types.MintStatusHaltOracle,
				MintsAllowed:    false,
				RefundsAllowed:  true,
				CollateralRatio: math.LegacyMustNewDecFromStr("1.500000000000000000"),
				WarnThreshold:   math.LegacyMustNewDecFromStr("1.200000000000000000"),
				HaltThreshold:   math.LegacyMustNewDecFromStr("1.050000000000000000"),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderBMEStatus(tc.status))
		})
	}
}

func TestRenderBMEVaultState(t *testing.T) {
	tests := map[string]struct {
		resp *types.QueryVaultStateResponse
	}{
		"WithBalances": {
			resp: &types.QueryVaultStateResponse{
				VaultState: types.State{
					Balances:      sdk.NewCoins(sdk.NewInt64Coin("uakt", 5000000), sdk.NewInt64Coin("uusdc", 10000000)),
					TotalBurned:   sdk.NewCoins(sdk.NewInt64Coin("uakt", 1000000)),
					TotalMinted:   sdk.NewCoins(sdk.NewInt64Coin("uusdc", 2000000)),
					RemintCredits: sdk.NewCoins(sdk.NewInt64Coin("uakt", 500000)),
				},
			},
		},
		"Empty": {
			resp: &types.QueryVaultStateResponse{
				VaultState: types.State{},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderBMEVaultState(tc.resp))
		})
	}
}

func TestRenderBMELedger(t *testing.T) {
	tests := map[string]struct {
		records []types.QueryLedgerRecordEntry
	}{
		"Empty": {
			records: nil,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderBMELedger(tc.records))
		})
	}
}
