package pretty

import (
	"bytes"
	"strings"
	"testing"

	"cosmossdk.io/math"
	"github.com/charmbracelet/x/exp/golden"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
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

func coinPrice(denom string, amount int64, price string) *types.CoinPrice {
	return &types.CoinPrice{
		Coin:  sdk.NewInt64Coin(denom, amount),
		Price: math.LegacyMustNewDecFromStr(price),
	}
}

func ledgerID(denom, toDenom string, seq int64) types.LedgerRecordID {
	return types.LedgerRecordID{
		Denom:    denom,
		ToDenom:  toDenom,
		Source:   "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
		Height:   1234,
		Sequence: seq,
	}
}

func TestRenderBMELedger(t *testing.T) {
	tests := map[string]struct {
		records []types.QueryLedgerRecordEntry
	}{
		"Empty": {
			records: nil,
		},
		"Executed": {
			records: []types.QueryLedgerRecordEntry{{
				ID:     ledgerID("uakt", "uact", 1),
				Status: types.LedgerRecordSatusExecuted,
				Record: &types.QueryLedgerRecordEntry_ExecutedRecord{
					ExecutedRecord: &types.LedgerRecord{
						BurnedFrom:          "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
						MintedTo:            "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
						Burned:              coinPrice("uakt", 5000000, "0.003125"),
						Minted:              coinPrice("uact", 15625, "1.000000000000000000"),
						Spread:              sdk.NewInt64Coin("uakt", 2500),
						RemintCreditIssued:  coinPrice("uakt", 1000000, "0.003125"),
						RemintCreditAccrued: coinPrice("uakt", 500000, "0.003125"),
					},
				},
			}},
		},
		// A zero spread is a real value: it must render as an amount, not as
		// an absent one.
		"ExecutedZeroSpread": {
			records: []types.QueryLedgerRecordEntry{{
				ID:     ledgerID("uakt", "uact", 2),
				Status: types.LedgerRecordSatusExecuted,
				Record: &types.QueryLedgerRecordEntry_ExecutedRecord{
					ExecutedRecord: &types.LedgerRecord{
						Burned: coinPrice("uakt", 5000000, "0.003125"),
						Minted: coinPrice("uact", 15625, "1"),
						Spread: sdk.NewInt64Coin("uakt", 0),
					},
				},
			}},
		},
		// proto3 omits zero values, so a node that never set the spread sends
		// a Coin whose inner Int is nil. Every method on it panics; the
		// renderer must degrade to a zero amount instead.
		"ExecutedSparseWire": {
			records: []types.QueryLedgerRecordEntry{{
				ID:     ledgerID("uakt", "uact", 3),
				Status: types.LedgerRecordSatusExecuted,
				Record: &types.QueryLedgerRecordEntry_ExecutedRecord{
					ExecutedRecord: &types.LedgerRecord{
						Burned: &types.CoinPrice{Coin: sdk.Coin{Denom: "uakt"}},
						Spread: sdk.Coin{Denom: "uakt"},
					},
				},
			}},
		},
		"Pending": {
			records: []types.QueryLedgerRecordEntry{{
				ID:     ledgerID("uakt", "uact", 4),
				Status: types.LedgerRecordSatusPending,
				Record: &types.QueryLedgerRecordEntry_PendingRecord{
					PendingRecord: &types.LedgerPendingRecord{
						Owner:       "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
						To:          "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
						CoinsToBurn: sdk.NewInt64Coin("uakt", 5000000),
						DenomToMint: "uact",
						Attempts:    1,
					},
				},
			}},
		},
		"Canceled": {
			records: []types.QueryLedgerRecordEntry{{
				ID:     ledgerID("uact", "uakt", 5),
				Status: types.LedgerRecordSatusCanceled,
				Record: &types.QueryLedgerRecordEntry_CanceledRecord{
					CanceledRecord: &types.LedgerCanceledRecord{
						Owner:        "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
						CancelReason: types.BMCancelReasonInsufficientFunds,
						CoinsToBurn:  sdk.NewInt64Coin("uact", 500000),
						DenomToMint:  "uakt",
					},
				},
			}},
		},
		"AllStates": {
			records: []types.QueryLedgerRecordEntry{
				{
					ID:     ledgerID("uakt", "uact", 6),
					Status: types.LedgerRecordSatusExecuted,
					Record: &types.QueryLedgerRecordEntry_ExecutedRecord{
						ExecutedRecord: &types.LedgerRecord{
							Burned: coinPrice("uakt", 5000000, "0.003125"),
							Minted: coinPrice("uact", 15625, "1"),
							Spread: sdk.NewInt64Coin("uakt", 0),
						},
					},
				},
				{
					ID:     ledgerID("uakt", "uact", 7),
					Status: types.LedgerRecordSatusPending,
					Record: &types.QueryLedgerRecordEntry_PendingRecord{
						PendingRecord: &types.LedgerPendingRecord{
							CoinsToBurn: sdk.NewInt64Coin("uakt", 250000),
							DenomToMint: "uact",
						},
					},
				},
				{
					ID:     ledgerID("uact", "uakt", 8),
					Status: types.LedgerRecordSatusCanceled,
					Record: &types.QueryLedgerRecordEntry_CanceledRecord{
						CanceledRecord: &types.LedgerCanceledRecord{
							CancelReason: types.BMCancelReasonMaxAttempts,
							CoinsToBurn:  sdk.NewInt64Coin("uact", 500000),
							DenomToMint:  "uakt",
						},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, RenderBMELedger(tc.records))
		})
	}
}

// The status column used to carry bare one-character codes ("e", "p",
// "c:<reason>") that were defined nowhere. It must spell the state out.
func TestRenderBMELedgerStatusesAreWords(t *testing.T) {
	out := RenderBMELedger([]types.QueryLedgerRecordEntry{
		{
			ID:     ledgerID("uakt", "uact", 1),
			Status: types.LedgerRecordSatusExecuted,
			Record: &types.QueryLedgerRecordEntry_ExecutedRecord{
				ExecutedRecord: &types.LedgerRecord{Spread: sdk.NewInt64Coin("uakt", 0)},
			},
		},
		{
			ID:     ledgerID("uakt", "uact", 2),
			Status: types.LedgerRecordSatusPending,
			Record: &types.QueryLedgerRecordEntry_PendingRecord{
				PendingRecord: &types.LedgerPendingRecord{
					CoinsToBurn: sdk.NewInt64Coin("uakt", 5000000),
					DenomToMint: "uact",
				},
			},
		},
		{
			ID:     ledgerID("uact", "uakt", 3),
			Status: types.LedgerRecordSatusCanceled,
			Record: &types.QueryLedgerRecordEntry_CanceledRecord{
				CanceledRecord: &types.LedgerCanceledRecord{
					CancelReason: types.BMCancelReasonInsufficientFunds,
					CoinsToBurn:  sdk.NewInt64Coin("uact", 500000),
				},
			},
		},
	})

	for _, want := range []string{"Executed", "Pending", "Canceled (insufficient funds)"} {
		if !strings.Contains(out, want) {
			t.Errorf("ledger status %q missing from output:\n%s", want, out)
		}
	}
	// The zero spread renders as an amount, not as "-".
	if !strings.Contains(out, "0 AKT") {
		t.Errorf("a zero spread must render as an amount, got:\n%s", out)
	}
}

// A Coin whose Amount proto3 omitted arrives with a nil inner Int; calling any
// method on it panics. RenderBMELedger must survive a sparse record.
func TestRenderBMELedgerToleratesNilAmounts(t *testing.T) {
	out := RenderBMELedger([]types.QueryLedgerRecordEntry{{
		ID:     ledgerID("uakt", "uact", 1),
		Status: types.LedgerRecordSatusExecuted,
		Record: &types.QueryLedgerRecordEntry_ExecutedRecord{
			ExecutedRecord: &types.LedgerRecord{
				Burned: &types.CoinPrice{Coin: sdk.Coin{Denom: "uakt"}},
				Spread: sdk.Coin{Denom: "uakt"},
			},
		},
	}})

	if !strings.Contains(out, "0 AKT") {
		t.Errorf("nil coin amounts should render as 0 AKT, got:\n%s", out)
	}
	// A record with no spread denom at all has nothing to show.
	out = RenderBMELedger([]types.QueryLedgerRecordEntry{{
		ID:     ledgerID("uakt", "uact", 2),
		Status: types.LedgerRecordSatusExecuted,
		Record: &types.QueryLedgerRecordEntry_ExecutedRecord{
			ExecutedRecord: &types.LedgerRecord{},
		},
	}})
	if !strings.Contains(out, "Executed") {
		t.Errorf("an empty executed record must still render a row, got:\n%s", out)
	}
}

func TestRenderBMEPendingConversion(t *testing.T) {
	tests := map[string]struct {
		out string
	}{
		"BurnMint": {
			out: RenderBMEPendingConversion(
				"akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
				sdk.NewInt64Coin("uakt", 1000000), "uact"),
		},
		"MintACT": {
			out: RenderBMEMintACT(
				"akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
				sdk.NewInt64Coin("uakt", 500000)),
		},
		"BurnACT": {
			out: RenderBMEBurnACT(
				"akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl",
				sdk.NewInt64Coin("uact", 500000)),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			golden.RequireEqual(t, tc.out)
		})
	}
}

// The chain settles a BME conversion in a later block, so the tx output must
// say the conversion is pending and name the query that shows the settlement.
// Without it a debited balance with no minted coins looks like lost funds.
func TestRenderBMEPendingConversionExplainsDeferredSettlement(t *testing.T) {
	const owner = "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl"

	cases := map[string]struct {
		out       string
		wantDenom string
	}{
		"BurnMint": {RenderBMEPendingConversion(owner, sdk.NewInt64Coin("uakt", 1000000), "uact"), "uact"},
		"MintACT":  {RenderBMEMintACT(owner, sdk.NewInt64Coin("uakt", 500000)), "uact"},
		"BurnACT":  {RenderBMEBurnACT(owner, sdk.NewInt64Coin("uact", 500000)), "uakt"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{
				owner,
				tc.wantDenom,
				"pending",
				"settles in a later block",
				"not known yet",
				"akt q bme ledger --owner " + owner + " --status ledger_record_status_pending",
			} {
				if !strings.Contains(tc.out, want) {
					t.Errorf("missing %q from conversion output:\n%s", want, tc.out)
				}
			}
			// Never claim the conversion is done.
			if strings.Contains(tc.out, "Minted:") {
				t.Errorf("a pending conversion must not report a minted amount:\n%s", tc.out)
			}
		})
	}
}

// The registered tx formatters must render the pending-conversion block. The
// common summary above them already printed "Status: success" — that is
// acceptance of the request — so a silent message detail is exactly what made a
// conversion look complete while the funds were still in flight.
func TestTxFmtBMEConversionsReportPendingSettlement(t *testing.T) {
	ensureFormattersRegistered()

	const owner = "akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl"

	msgs := map[string]sdk.Msg{
		"MsgBurnMint": &types.MsgBurnMint{
			Owner: owner, To: owner,
			CoinsToBurn: sdk.NewInt64Coin("uakt", 1000000),
			DenomToMint: "uact",
		},
		"MsgMintACT": &types.MsgMintACT{
			Owner: owner, To: owner,
			CoinsToBurn: sdk.NewInt64Coin("uakt", 500000),
		},
		"MsgBurnACT": &types.MsgBurnACT{
			Owner: owner, To: owner,
			CoinsToBurn: sdk.NewInt64Coin("uact", 500000),
		},
	}

	for name, msg := range msgs {
		t.Run(name, func(t *testing.T) {
			f, ok := LookupTx(msg)
			if !ok {
				t.Fatalf("no tx formatter registered for %s", name)
			}

			var buf bytes.Buffer
			if err := f.FormatTx(&buf, nil, sdkclient.Context{}, msg, &sdk.TxResponse{}, 0); err != nil {
				t.Fatalf("FormatTx: %v", err)
			}

			out := buf.String()
			for _, want := range []string{
				"pending",
				"settles in a later block",
				"akt q bme ledger --owner " + owner + " --status ledger_record_status_pending",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("%s detail is missing %q:\n%s", name, want, out)
				}
			}
		})
	}
}

func TestBMELedgerPendingCommand(t *testing.T) {
	got := BMELedgerPendingCommand("akash1abc")
	want := "akt q bme ledger --owner akash1abc --status ledger_record_status_pending"
	if got != want {
		t.Errorf("BMELedgerPendingCommand = %q, want %q", got, want)
	}
	// The status must be the exact vocabulary --status accepts.
	if types.LedgerRecordSatusPending.String() != "ledger_record_status_pending" {
		t.Errorf("upstream pending status name changed: %q", types.LedgerRecordSatusPending.String())
	}
}

// A real on-chain collateral ratio has no trailing zeros to strip, so trimming
// alone left it at full 18-decimal width beside a threshold of "0.95". Ratios
// round to ratioDecimals; prices must not, because their significance is in the
// small digits (SPEC §8.3.12).
func TestFormatRatioRoundsButFormatPriceKeepsPrecision(t *testing.T) {
	ratios := map[string]string{
		"1.495209570451729242": "1.495",
		"1.500000000000000000": "1.5",
		"0.950000000000000000": "0.95",
		"2.000000000000000000": "2",
	}

	for in, want := range ratios {
		d, err := math.LegacyNewDecFromStr(in)
		if err != nil {
			t.Fatalf("parse %q: %v", in, err)
		}

		if got := formatRatio(d); got != want {
			t.Errorf("formatRatio(%s) = %q, want %q", in, got, want)
		}
	}

	// An oracle price rounded to a ratio's precision would round to zero.
	price, err := math.LegacyNewDecFromStr("0.003125000000000000")
	if err != nil {
		t.Fatalf("parse price: %v", err)
	}

	if got := formatPrice(price); got != "0.003125" {
		t.Errorf("formatPrice = %q, want %q", got, "0.003125")
	}

	// A derived price arrives with all 18 places; it reports at the oracle's
	// own 8, so it lines up with the source prices beside it.
	derived, err := math.LegacyNewDecFromStr("0.536004234885265376")
	if err != nil {
		t.Fatalf("parse derived: %v", err)
	}

	if got := formatPrice(derived); got != "0.53600423" {
		t.Errorf("formatPrice(derived) = %q, want %q", got, "0.53600423")
	}

	if got := formatRatio(price); got == "0.003125" {
		t.Fatal("formatRatio must not be used for prices; it rounds them away")
	}
}

// A nil Dec is what proto3 leaves for an unset field; neither formatter may panic.
func TestRatioAndPriceFormattersTolerateNilDec(t *testing.T) {
	var nilDec math.LegacyDec

	if got := formatRatio(nilDec); got != "0" {
		t.Errorf("formatRatio(nil) = %q, want %q", got, "0")
	}

	if got := formatPrice(nilDec); got != "0" {
		t.Errorf("formatPrice(nil) = %q, want %q", got, "0")
	}
}
