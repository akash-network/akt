package pretty

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"

	types "pkg.akt.dev/go/node/oracle/v2"
)

func TestRenderOraclePrices(t *testing.T) {
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		res *types.QueryPricesResponse
	}{
		"Empty": {
			res: &types.QueryPricesResponse{},
		},
		"WithPrices": {
			res: &types.QueryPricesResponse{
				Prices: []types.PriceData{
					{
						ID: types.PriceDataRecordID{
							Denom:     "akt",
							BaseDenom: "usd",
							Source:    1,
							Timestamp: ts,
						},
						State: types.PriceDataState{
							Price: math.LegacyMustNewDecFromStr("3.450000000000000000"),
						},
					},
					{
						ID: types.PriceDataRecordID{
							Denom:     "atom",
							BaseDenom: "usd",
							Source:    2,
							Timestamp: ts.Add(10 * time.Second),
						},
						State: types.PriceDataState{
							Price: math.LegacyMustNewDecFromStr("9.120000000000000000"),
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderOraclePrices(tc.res))
		})
	}
}

func TestRenderAggregatedPrice(t *testing.T) {
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		res *types.QueryAggregatedPriceResponse
	}{
		"Healthy": {
			res: &types.QueryAggregatedPriceResponse{
				AggregatedPrice: types.AggregatedPrice{
					Denom:        "akt",
					TWAP:         math.LegacyMustNewDecFromStr("3.500000000000000000"),
					MedianPrice:  math.LegacyMustNewDecFromStr("3.480000000000000000"),
					MinPrice:     math.LegacyMustNewDecFromStr("3.400000000000000000"),
					MaxPrice:     math.LegacyMustNewDecFromStr("3.600000000000000000"),
					NumSources:   5,
					DeviationBps: 100,
					Timestamp:    ts,
				},
				PriceHealth: types.PriceHealth{
					IsHealthy:           true,
					HasMinSources:       true,
					DeviationOk:         true,
					TotalSources:        5,
					TotalHealthySources: 5,
				},
			},
		},
		"Unhealthy": {
			res: &types.QueryAggregatedPriceResponse{
				AggregatedPrice: types.AggregatedPrice{
					Denom:        "akt",
					TWAP:         math.LegacyMustNewDecFromStr("3.500000000000000000"),
					MedianPrice:  math.LegacyMustNewDecFromStr("3.480000000000000000"),
					MinPrice:     math.LegacyMustNewDecFromStr("3.400000000000000000"),
					MaxPrice:     math.LegacyMustNewDecFromStr("3.600000000000000000"),
					NumSources:   2,
					DeviationBps: 500,
					Timestamp:    ts,
				},
				PriceHealth: types.PriceHealth{
					IsHealthy:           false,
					HasMinSources:       false,
					DeviationOk:         false,
					TotalSources:        3,
					TotalHealthySources: 1,
					FailureReason:       []string{"too few sources", "deviation too high"},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderAggregatedPrice(tc.res))
		})
	}
}
