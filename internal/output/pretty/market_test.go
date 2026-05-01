package pretty

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"
	sdk "github.com/cosmos/cosmos-sdk/types"

	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
)

func TestRenderBidList(t *testing.T) {
	tests := map[string]struct {
		resp *mvbeta.QueryBidsResponse
	}{
		"Empty": {
			resp: &mvbeta.QueryBidsResponse{
				Bids: []mvbeta.QueryBidResponse{},
			},
		},
		"WithBids": {
			resp: &mvbeta.QueryBidsResponse{
				Bids: []mvbeta.QueryBidResponse{
					{
						Bid: mvbeta.Bid{
							ID: mv1.BidID{
								Owner:    "akash1qwerty",
								DSeq:     100,
								GSeq:     1,
								OSeq:     1,
								Provider: "akash1provider1",
							},
							State:     mvbeta.BidOpen,
							Price:     sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("12.5")),
							CreatedAt: 18234567,
						},
					},
					{
						Bid: mvbeta.Bid{
							ID: mv1.BidID{
								Owner:    "akash1qwerty",
								DSeq:     100,
								GSeq:     1,
								OSeq:     1,
								Provider: "akash1provider2",
							},
							State:     mvbeta.BidActive,
							Price:     sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("10.0")),
							CreatedAt: 18234568,
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderBidList(tc.resp))
		})
	}
}

func TestRenderBidDetail(t *testing.T) {
	tests := map[string]struct {
		resp *mvbeta.QueryBidResponse
	}{
		"Basic": {
			resp: &mvbeta.QueryBidResponse{
				Bid: mvbeta.Bid{
					ID: mv1.BidID{
						Owner:    "akash1qwerty",
						DSeq:     100,
						GSeq:     1,
						OSeq:     1,
						Provider: "akash1provider1",
					},
					State:     mvbeta.BidOpen,
					Price:     sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("12.5")),
					CreatedAt: 18234567,
				},
			},
		},
		"WithResourcesOffer": {
			resp: &mvbeta.QueryBidResponse{
				Bid: mvbeta.Bid{
					ID: mv1.BidID{
						Owner:    "akash1qwerty",
						DSeq:     200,
						GSeq:     1,
						OSeq:     1,
						Provider: "akash1provider1",
					},
					State:     mvbeta.BidActive,
					Price:     sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("8.75")),
					CreatedAt: 17000000,
					ResourcesOffer: mvbeta.ResourcesOffer{
						{Count: 2},
						{Count: 5},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderBidDetail(tc.resp))
		})
	}
}

func TestRenderLeaseList(t *testing.T) {
	tests := map[string]struct {
		resp *mvbeta.QueryLeasesResponse
	}{
		"Empty": {
			resp: &mvbeta.QueryLeasesResponse{
				Leases: []mvbeta.QueryLeaseResponse{},
			},
		},
		"WithLeases": {
			resp: &mvbeta.QueryLeasesResponse{
				Leases: []mvbeta.QueryLeaseResponse{
					{
						Lease: mv1.Lease{
							ID: mv1.LeaseID{
								Owner:    "akash1qwerty",
								DSeq:     100,
								GSeq:     1,
								OSeq:     1,
								Provider: "akash1provider1",
							},
							State:     mv1.LeaseActive,
							Price:     sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("12.5")),
							CreatedAt: 18234567,
						},
					},
					{
						Lease: mv1.Lease{
							ID: mv1.LeaseID{
								Owner:    "akash1zxcvbn",
								DSeq:     200,
								GSeq:     1,
								OSeq:     1,
								Provider: "akash1provider2",
							},
							State:     mv1.LeaseClosed,
							Price:     sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("10.0")),
							CreatedAt: 17000000,
							ClosedOn:  18000000,
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderLeaseList(tc.resp))
		})
	}
}

func TestRenderLeaseDetail(t *testing.T) {
	tests := map[string]struct {
		resp *mvbeta.QueryLeaseResponse
	}{
		"Active": {
			resp: &mvbeta.QueryLeaseResponse{
				Lease: mv1.Lease{
					ID: mv1.LeaseID{
						Owner:    "akash1qwerty",
						DSeq:     100,
						GSeq:     1,
						OSeq:     1,
						Provider: "akash1provider1",
					},
					State:     mv1.LeaseActive,
					Price:     sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("12.5")),
					CreatedAt: 18234567,
				},
			},
		},
		"Closed": {
			resp: &mvbeta.QueryLeaseResponse{
				Lease: mv1.Lease{
					ID: mv1.LeaseID{
						Owner:    "akash1zxcvbn",
						DSeq:     200,
						GSeq:     1,
						OSeq:     1,
						Provider: "akash1provider2",
					},
					State:     mv1.LeaseClosed,
					Price:     sdk.NewDecCoinFromDec("uakt", math.LegacyMustNewDecFromStr("10.0")),
					CreatedAt: 17000000,
					ClosedOn:  18000000,
					Reason:    mv1.LeaseClosedReasonOwner,
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderLeaseDetail(tc.resp))
		})
	}
}

func TestRenderOrderList(t *testing.T) {
	tests := map[string]struct {
		resp *mvbeta.QueryOrdersResponse
	}{
		"Empty": {
			resp: &mvbeta.QueryOrdersResponse{
				Orders: mvbeta.Orders{},
			},
		},
		"WithOrders": {
			resp: &mvbeta.QueryOrdersResponse{
				Orders: mvbeta.Orders{
					{
						ID: mv1.OrderID{
							Owner: "akash1qwerty",
							DSeq:  100,
							GSeq:  1,
							OSeq:  1,
						},
						State:     mvbeta.OrderOpen,
						CreatedAt: 18234567,
					},
					{
						ID: mv1.OrderID{
							Owner: "akash1zxcvbn",
							DSeq:  200,
							GSeq:  1,
							OSeq:  1,
						},
						State:     mvbeta.OrderActive,
						CreatedAt: 17000000,
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderOrderList(tc.resp))
		})
	}
}

func TestRenderOrderDetail(t *testing.T) {
	tests := map[string]struct {
		resp *mvbeta.QueryOrderResponse
	}{
		"Basic": {
			resp: &mvbeta.QueryOrderResponse{
				Order: mvbeta.Order{
					ID: mv1.OrderID{
						Owner: "akash1qwerty",
						DSeq:  100,
						GSeq:  1,
						OSeq:  1,
					},
					State:     mvbeta.OrderOpen,
					CreatedAt: 18234567,
				},
			},
		},
		"WithSpec": {
			resp: &mvbeta.QueryOrderResponse{
				Order: mvbeta.Order{
					ID: mv1.OrderID{
						Owner: "akash1qwerty",
						DSeq:  200,
						GSeq:  1,
						OSeq:  1,
					},
					State:     mvbeta.OrderActive,
					CreatedAt: 17000000,
					Spec:      dvbeta.GroupSpec{Name: "my-web-app"},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderOrderDetail(tc.resp))
		})
	}
}
