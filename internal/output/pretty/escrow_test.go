package pretty

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"

	etypes "pkg.akt.dev/go/node/escrow/v1"

	eidv1 "pkg.akt.dev/go/node/escrow/id/v1"
	etypesv1 "pkg.akt.dev/go/node/escrow/types/v1"
)

func TestRenderEscrowAccounts(t *testing.T) {
	tests := map[string]struct {
		res *etypes.QueryAccountsResponse
	}{
		"Empty": {
			res: &etypes.QueryAccountsResponse{},
		},
		"WithAccounts": {
			res: &etypes.QueryAccountsResponse{
				Accounts: etypesv1.Accounts{
					{
						ID: eidv1.Account{
							Scope: eidv1.ScopeDeployment,
							XID:   "akash1abc123/100",
						},
						State: etypesv1.AccountState{
							State: etypesv1.StateOpen,
							Owner: "akash1abc123",
							Funds: []etypesv1.Balance{
								{Denom: "uakt", Amount: math.LegacyMustNewDecFromStr("5000000.0")},
							},
							Transferred: sdk.NewDecCoins(
								sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("1000000.0")),
							),
							SettledAt: 18234567,
						},
					},
					{
						ID: eidv1.Account{
							Scope: eidv1.ScopeDeployment,
							XID:   "akash1def456/200",
						},
						State: etypesv1.AccountState{
							State: etypesv1.StateClosed,
							Owner: "akash1def456",
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderEscrowAccounts(tc.res))
		})
	}
}

func TestRenderEscrowPayments(t *testing.T) {
	tests := map[string]struct {
		res *etypes.QueryPaymentsResponse
	}{
		"Empty": {
			res: &etypes.QueryPaymentsResponse{},
		},
		"WithPayments": {
			res: &etypes.QueryPaymentsResponse{
				Payments: etypesv1.Payments{
					{
						ID: eidv1.Payment{
							AID: eidv1.Account{
								Scope: eidv1.ScopeDeployment,
								XID:   "akash1abc123/100",
							},
							XID: "1/1/akash1prov01",
						},
						State: etypesv1.PaymentState{
							State:     etypesv1.StateOpen,
							Owner:     "akash1prov01",
							Rate:      sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("10.0")),
							Balance:   sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("2000000.0")),
							Withdrawn: sdk.NewInt64Coin("uakt", 500000),
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderEscrowPayments(tc.res))
		})
	}
}
