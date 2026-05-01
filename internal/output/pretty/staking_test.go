package pretty

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func TestRenderValidatorList(t *testing.T) {
	tests := map[string]struct {
		res *stakingtypes.QueryValidatorsResponse
	}{
		"Empty": {
			res: &stakingtypes.QueryValidatorsResponse{
				Validators: nil,
			},
		},
		"WithValidators": {
			res: &stakingtypes.QueryValidatorsResponse{
				Validators: []stakingtypes.Validator{
					{
						OperatorAddress: "akashvaloper1abc123",
						Description: stakingtypes.Description{
							Moniker: "ValidatorAlpha",
						},
						Tokens:          math.NewInt(5000000000),
						DelegatorShares: math.LegacyNewDec(5000000000),
						Commission: stakingtypes.Commission{
							CommissionRates: stakingtypes.CommissionRates{
								Rate: math.LegacyMustNewDecFromStr("0.050000000000000000"),
							},
						},
						Status: stakingtypes.Bonded,
					},
					{
						OperatorAddress: "akashvaloper1def456",
						Description: stakingtypes.Description{
							Moniker: "ValidatorBeta",
						},
						Tokens:          math.NewInt(1000000000),
						DelegatorShares: math.LegacyNewDec(1000000000),
						Commission: stakingtypes.Commission{
							CommissionRates: stakingtypes.CommissionRates{
								Rate: math.LegacyMustNewDecFromStr("0.100000000000000000"),
							},
						},
						Status: stakingtypes.Unbonding,
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderValidatorList(tc.res))
		})
	}
}

func TestRenderValidatorDetail(t *testing.T) {
	tests := map[string]struct {
		v *stakingtypes.Validator
	}{
		"Bonded": {
			v: &stakingtypes.Validator{
				OperatorAddress: "akashvaloper1abc123",
				Description: stakingtypes.Description{
					Moniker:         "ValidatorAlpha",
					Identity:        "DEADBEEF",
					Website:         "https://alpha.example.com",
					SecurityContact: "security@alpha.example.com",
					Details:         "A reliable validator",
				},
				Tokens:          math.NewInt(5000000000),
				DelegatorShares: math.LegacyNewDec(5000000000),
				Commission: stakingtypes.Commission{
					CommissionRates: stakingtypes.CommissionRates{
						Rate:          math.LegacyMustNewDecFromStr("0.050000000000000000"),
						MaxRate:       math.LegacyMustNewDecFromStr("0.200000000000000000"),
						MaxChangeRate: math.LegacyMustNewDecFromStr("0.010000000000000000"),
					},
					UpdateTime: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
				},
				Jailed: false,
				Status: stakingtypes.Bonded,
			},
		},
		"Unbonding": {
			v: &stakingtypes.Validator{
				OperatorAddress: "akashvaloper1def456",
				Description: stakingtypes.Description{
					Moniker: "ValidatorBeta",
				},
				Tokens:          math.NewInt(1000000000),
				DelegatorShares: math.LegacyNewDec(1000000000),
				Commission: stakingtypes.Commission{
					CommissionRates: stakingtypes.CommissionRates{
						Rate:          math.LegacyMustNewDecFromStr("0.100000000000000000"),
						MaxRate:       math.LegacyMustNewDecFromStr("0.500000000000000000"),
						MaxChangeRate: math.LegacyMustNewDecFromStr("0.050000000000000000"),
					},
					UpdateTime: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
				},
				Jailed:          false,
				Status:          stakingtypes.Unbonding,
				UnbondingHeight: 12345678,
				UnbondingTime:   time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderValidatorDetail(tc.v))
		})
	}
}

func TestRenderDelegationDetail(t *testing.T) {
	tests := map[string]struct {
		res *stakingtypes.DelegationResponse
	}{
		"Normal": {
			res: &stakingtypes.DelegationResponse{
				Delegation: stakingtypes.Delegation{
					DelegatorAddress: "akash1delegator123",
					ValidatorAddress: "akashvaloper1abc123",
					Shares:           math.LegacyNewDec(5000000),
				},
				Balance: sdk.NewInt64Coin("uakt", 5000000),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderDelegationDetail(tc.res))
		})
	}
}

func TestRenderDelegatorDelegations(t *testing.T) {
	tests := map[string]struct {
		res *stakingtypes.QueryDelegatorDelegationsResponse
	}{
		"Empty": {
			res: &stakingtypes.QueryDelegatorDelegationsResponse{
				DelegationResponses: nil,
			},
		},
		"WithDelegations": {
			res: &stakingtypes.QueryDelegatorDelegationsResponse{
				DelegationResponses: stakingtypes.DelegationResponses{
					{
						Delegation: stakingtypes.Delegation{
							DelegatorAddress: "akash1delegator123",
							ValidatorAddress: "akashvaloper1abc123",
							Shares:           math.LegacyNewDec(5000000),
						},
						Balance: sdk.NewInt64Coin("uakt", 5000000),
					},
					{
						Delegation: stakingtypes.Delegation{
							DelegatorAddress: "akash1delegator123",
							ValidatorAddress: "akashvaloper1def456",
							Shares:           math.LegacyNewDec(2000000),
						},
						Balance: sdk.NewInt64Coin("uakt", 2000000),
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderDelegatorDelegations(tc.res))
		})
	}
}

func TestRenderStakingPool(t *testing.T) {
	tests := map[string]struct {
		pool *stakingtypes.Pool
	}{
		"Normal": {
			pool: &stakingtypes.Pool{
				BondedTokens:    math.NewInt(100000000000),
				NotBondedTokens: math.NewInt(50000000000),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderStakingPool(tc.pool))
		})
	}
}
