package pretty

import (
	"encoding/json"
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	oracletypes "pkg.akt.dev/go/node/oracle/v2"
)

func TestRenderStakingParams(t *testing.T) {
	tests := map[string]struct {
		params *stakingtypes.Params
	}{
		"Default": {
			params: &stakingtypes.Params{
				UnbondingTime:     504 * time.Hour,
				MaxValidators:     100,
				MaxEntries:        7,
				HistoricalEntries: 10000,
				BondDenom:         "uakt",
				MinCommissionRate: math.LegacyZeroDec(),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderStakingParams(tc.params))
		})
	}
}

func TestRenderGovParams(t *testing.T) {
	tests := map[string]struct {
		res *govv1.QueryParamsResponse
	}{
		"WithExpedited": {
			res: func() *govv1.QueryParamsResponse {
				votingPeriod := 2 * 24 * time.Hour
				maxDepositPeriod := 14 * 24 * time.Hour
				expeditedVotingPeriod := 24 * time.Hour

				return &govv1.QueryParamsResponse{
					Params: &govv1.Params{
						VotingPeriod:               &votingPeriod,
						MinDeposit:                 sdk.NewCoins(sdk.NewInt64Coin("uakt", 50000000000)),
						MaxDepositPeriod:           &maxDepositPeriod,
						Quorum:                     "0.334000000000000000",
						Threshold:                  "0.500000000000000000",
						VetoThreshold:              "0.334000000000000000",
						ExpeditedVotingPeriod:      &expeditedVotingPeriod,
						ExpeditedThreshold:         "0.667000000000000000",
						ExpeditedMinDeposit:        sdk.NewCoins(sdk.NewInt64Coin("uakt", 100000000000)),
						BurnVoteQuorum:             false,
						BurnProposalDepositPrevote: true,
						BurnVoteVeto:               true,
					},
				}
			}(),
		},
		"NilParams": {
			res: &govv1.QueryParamsResponse{
				Params: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderGovParams(tc.res))
		})
	}
}

func TestRenderMintParams(t *testing.T) {
	tests := map[string]struct {
		params *minttypes.Params
	}{
		"Default": {
			params: &minttypes.Params{
				MintDenom:           "uakt",
				InflationRateChange: math.LegacyMustNewDecFromStr("0.130000000000000000"),
				InflationMax:        math.LegacyMustNewDecFromStr("0.200000000000000000"),
				InflationMin:        math.LegacyMustNewDecFromStr("0.070000000000000000"),
				GoalBonded:          math.LegacyMustNewDecFromStr("0.670000000000000000"),
				BlocksPerYear:       6311520,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderMintParams(tc.params))
		})
	}
}

func TestRenderSlashingParams(t *testing.T) {
	tests := map[string]struct {
		params *slashingtypes.Params
	}{
		"Default": {
			params: &slashingtypes.Params{
				SignedBlocksWindow:      100,
				MinSignedPerWindow:      math.LegacyMustNewDecFromStr("0.500000000000000000"),
				DowntimeJailDuration:    10 * time.Minute,
				SlashFractionDoubleSign: math.LegacyMustNewDecFromStr("0.050000000000000000"),
				SlashFractionDowntime:   math.LegacyMustNewDecFromStr("0.010000000000000000"),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderSlashingParams(tc.params))
		})
	}
}

func TestRenderDistributionParams(t *testing.T) {
	tests := map[string]struct {
		params *distrtypes.Params
	}{
		"Default": {
			params: &distrtypes.Params{
				CommunityTax:        math.LegacyMustNewDecFromStr("0.020000000000000000"),
				WithdrawAddrEnabled: true,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderDistributionParams(tc.params))
		})
	}
}

func TestRenderAuthParams(t *testing.T) {
	tests := map[string]struct {
		params *authtypes.Params
	}{
		"Default": {
			params: &authtypes.Params{
				MaxMemoCharacters:      256,
				TxSigLimit:             7,
				TxSizeCostPerByte:      10,
				SigVerifyCostED25519:   590,
				SigVerifyCostSecp256k1: 1000,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderAuthParams(tc.params))
		})
	}
}

func TestRenderDeploymentParams(t *testing.T) {
	tests := map[string]struct {
		res *dvbeta.QueryParamsResponse
	}{
		"Default": {
			res: &dvbeta.QueryParamsResponse{
				Params: dvbeta.Params{
					MinDeposits: sdk.NewCoins(sdk.NewInt64Coin("uakt", 5000000)),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderDeploymentParams(tc.res))
		})
	}
}

func TestRenderMarketParams(t *testing.T) {
	tests := map[string]struct {
		res *mvbeta.QueryParamsResponse
	}{
		"Default": {
			res: &mvbeta.QueryParamsResponse{
				Params: mvbeta.Params{
					OrderMaxBids:   20,
					BidMinDeposits: sdk.NewCoins(sdk.NewInt64Coin("uakt", 500000)),
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderMarketParams(tc.res))
		})
	}
}

func TestRenderWasmParams(t *testing.T) {
	tests := map[string]struct {
		params *wasmtypes.Params
	}{
		"Default": {
			params: &wasmtypes.Params{
				CodeUploadAccess: wasmtypes.AccessConfig{
					Permission: wasmtypes.AccessTypeEverybody,
				},
				InstantiateDefaultPermission: wasmtypes.AccessTypeEverybody,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderWasmParams(tc.params))
		})
	}
}

func TestRenderOracleParams(t *testing.T) {
	tests := map[string]struct {
		res *oracletypes.QueryParamsResponse
	}{
		"Default": {
			res: &oracletypes.QueryParamsResponse{
				Params: oracletypes.Params{
					Sources:                 []string{"coingecko", "osmosis"},
					MinPriceSources:         3,
					MaxPriceStalenessPeriod: 30 * time.Minute,
					TwapWindow:              10 * time.Minute,
					MaxPriceDeviationBps:    500,
					PriceRetention:          24 * time.Hour,
					PruneEpoch:              "hour",
					MaxPrunePerEpoch:        100,
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderOracleParams(tc.res))
		})
	}
}

func TestRenderModuleParamsFromJSON(t *testing.T) {
	tests := map[string]struct {
		module string
		raw    json.RawMessage
	}{
		"Staking": {
			module: "staking",
			raw: json.RawMessage(`{
				"params": {
					"unbonding_time": "1814400s",
					"max_validators": 100,
					"max_entries": 7,
					"historical_entries": 10000,
					"bond_denom": "uakt",
					"min_commission_rate": "0.000000000000000000"
				}
			}`),
		},
		"Unknown": {
			module: "foobar",
			raw:    json.RawMessage(`{"key": "value"}`),
		},
		"Empty": {
			module: "staking",
			raw:    json.RawMessage(nil),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderModuleParamsFromJSON(tc.module, tc.raw))
		})
	}
}
