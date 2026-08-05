package pretty

import (
	"fmt"
	"io"
	"strings"

	"cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	types "pkg.akt.dev/go/node/bme/v1"
	"pkg.akt.dev/go/sdkutil"
)

func init() {
	Register((*types.QueryStatusResponse)(nil), PrettyFormatterFunc(formatBMEStatus))
	Register((*types.QueryVaultStateResponse)(nil), PrettyFormatterFunc(formatBMEVaultState))
	Register((*types.QueryLedgerRecordsResponse)(nil), PrettyFormatterFunc(formatBMELedger))
}

// mintStatusLabel returns a human-readable label for BME mint status.
func mintStatusLabel(s types.MintStatus) string {
	switch s {
	case types.MintStatusHealthy:
		return "Healthy"
	case types.MintStatusWarning:
		return "Warning"
	case types.MintStatusHaltCR:
		return "Halt CR"
	case types.MintStatusHaltOracle:
		return "Halt Oracle"
	default:
		return s.String()
	}
}

// ledgerStatusLabel returns a human-readable label for a BME ledger record
// status. The upstream enum names are wire identifiers
// ("ledger_record_status_pending"); users get the same full-word treatment
// mintStatusLabel gives the mint status.
func ledgerStatusLabel(s types.LedgerRecordStatus) string {
	switch s {
	case types.LedgerRecordSatusPending:
		return "Pending"
	case types.LedgerRecordSatusExecuted:
		return "Executed"
	case types.LedgerRecordSatusCanceled:
		return "Canceled"
	default:
		return s.String()
	}
}

// cancelReasonLabel renders a BME cancel reason as words. The upstream names
// are snake_case wire identifiers ("insufficient_funds").
func cancelReasonLabel(r types.LedgerCanceledRecord_BMCancelReason) string {
	return strings.ReplaceAll(r.String(), "_", " ")
}

// mintStatusColor returns a color-styled BME mint status label.
func mintStatusColor(s types.MintStatus) string {
	switch s {
	case types.MintStatusHealthy:
		return StyleGreen.Render(mintStatusLabel(s))
	case types.MintStatusWarning:
		return StyleYellow.Render(mintStatusLabel(s))
	case types.MintStatusHaltCR, types.MintStatusHaltOracle:
		return StyleRed.Render(mintStatusLabel(s))
	default:
		return StyleGray.Render(mintStatusLabel(s))
	}
}

// RenderBMEStatus renders a BME status section as a string.
// Used by both CLI pretty output and TUI monitor dashboard.
func RenderBMEStatus(res *types.QueryStatusResponse) string {
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("BME Status"))
	KV(&buf, "Status", mintStatusColor(res.Status))
	KV(&buf, "Mints", formatAllowedHalted(res.MintsAllowed))
	KV(&buf, "Refunds", formatAllowedHalted(res.RefundsAllowed))
	KV(&buf, "Collateral Ratio", Bold(formatRatio(res.CollateralRatio)))
	KVHeader(&buf, "Thresholds")
	SubKV(&buf, "Warn", formatRatio(res.WarnThreshold))
	SubKV(&buf, "Halt", formatRatio(res.HaltThreshold))
	return buf.String()
}

func formatBMEStatus(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderBMEStatus(msg.(*types.QueryStatusResponse)))
	return err
}

// RenderBMEVaultState renders a BME vault state section as a string.
// Used by both CLI pretty output and TUI monitor dashboard.
func RenderBMEVaultState(res *types.QueryVaultStateResponse) string {
	var buf strings.Builder
	s := res.VaultState

	fmt.Fprintln(&buf, Section("Vault State"))

	if len(s.Balances) > 0 {
		KV(&buf, "Balances", FormatCoins(s.Balances))
	}
	if len(s.TotalBurned) > 0 {
		KV(&buf, "Total Burned", FormatCoins(s.TotalBurned))
	}
	if len(s.TotalMinted) > 0 {
		KV(&buf, "Total Minted", FormatCoins(s.TotalMinted))
	}
	if len(s.RemintCredits) > 0 {
		KV(&buf, "Remint Credits", FormatCoins(s.RemintCredits))
	}
	return buf.String()
}

func formatBMEVaultState(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderBMEVaultState(msg.(*types.QueryVaultStateResponse)))
	return err
}

// RenderBMELedger renders a BME ledger records table as a string.
// Used by both CLI pretty output and TUI monitor dashboard.
func RenderBMELedger(records []types.QueryLedgerRecordEntry) string {
	var buf strings.Builder

	if len(records) == 0 {
		fmt.Fprintln(&buf, Dim("(no ledger records)"))
		return buf.String()
	}

	cols := []ColDef{
		{Header: "ROUTE"},
		{Header: "ID"},
		{Header: "STATUS"},
		{Header: "BURNED", Align: AlignRight},
		{Header: "MINTED", Align: AlignRight},
		{Header: "SPREAD", Align: AlignRight},
		{Header: "REMINT ACCRUED", Align: AlignRight},
		{Header: "REMINT ISSUED", Align: AlignRight},
	}
	rows := make([][]string, 0, len(records))

	for _, r := range records {
		var status string
		burned := "-"
		minted := "-"
		spread := "-"
		remintIssued := "-"
		remintAccrued := "-"

		switch rec := r.Record.(type) {
		case *types.QueryLedgerRecordEntry_ExecutedRecord:
			status = StyleGreen.Render(ledgerStatusLabel(types.LedgerRecordSatusExecuted))
			if rec.ExecutedRecord != nil {
				er := rec.ExecutedRecord
				burned = formatCoinPrice(er.Burned)
				minted = formatCoinPrice(er.Minted)
				// Spread is a plain Coin: a zero spread is a real value and
				// renders as "0 AKT", not as an absent one.
				spread = formatCoinSafe(er.Spread)
				remintIssued = formatCoinPrice(er.RemintCreditIssued)
				remintAccrued = formatCoinPrice(er.RemintCreditAccrued)
			}
		case *types.QueryLedgerRecordEntry_PendingRecord:
			status = StyleYellow.Render(ledgerStatusLabel(types.LedgerRecordSatusPending))
			if rec.PendingRecord != nil {
				burned = formatCoinSafe(rec.PendingRecord.CoinsToBurn)
				// The mint has not run: its size depends on the oracle price at
				// settlement. The destination denom is already in ROUTE.
				minted = Dim(bmePendingAmount)
			}
		case *types.QueryLedgerRecordEntry_CanceledRecord:
			if rec.CanceledRecord != nil {
				cr := rec.CanceledRecord
				status = StyleRed.Render(fmt.Sprintf("%s (%s)",
					ledgerStatusLabel(types.LedgerRecordSatusCanceled),
					cancelReasonLabel(cr.CancelReason)))
				// The amount the record was going to burn; a canceled record
				// returns it to the owner and mints nothing, so MINTED stays "-".
				burned = formatCoinSafe(cr.CoinsToBurn)
			} else {
				// Canceled, reason unavailable.
				status = StyleRed.Render(ledgerStatusLabel(types.LedgerRecordSatusCanceled))
			}
		}

		// The oneof is empty (or a status this build does not know): fall back
		// to the status the entry carries alongside it rather than a blank cell.
		if status == "" {
			status = StyleGray.Render(ledgerStatusLabel(r.Status))
		}

		rows = append(rows, []string{
			fmt.Sprintf("%s→%s", r.ID.Denom, r.ID.ToDenom),
			fmt.Sprintf("%s/%d/%d", r.ID.Source, r.ID.Height, r.ID.Sequence),
			status,
			burned,
			minted,
			spread,
			remintAccrued,
			remintIssued,
		})
	}

	WriteTableCols(&buf, cols, rows)
	return buf.String()
}

func formatBMELedger(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderBMELedger(msg.(*types.QueryLedgerRecordsResponse).Records))
	return err
}

// formatCoinPrice formats a CoinPrice as "amount denom @price". Every amount in
// the ledger table that carries an oracle price renders through this, so the
// same concept never appears in two shapes in one table.
func formatCoinPrice(cp *types.CoinPrice) string {
	if cp == nil {
		return "-"
	}

	return fmt.Sprintf("%s @%s", formatCoinSafe(cp.Coin), formatPrice(cp.Price))
}

// formatCoinSafe formats a Coin that the wire may have left sparse. proto3 omits
// zero values, so a Coin a node never set arrives with a nil inner Int (any
// method on it panics) and possibly an empty denom.
func formatCoinSafe(c sdk.Coin) string {
	if c.Denom == "" {
		return "-"
	}

	c.Amount = IntOrZero(c.Amount)

	return FormatCoin(c)
}

// ratioDecimals is the precision a collateral ratio and its thresholds are
// reported at (SPEC §8.3.12 renders `1.523`). A LegacyDec always stringifies to
// 18 decimal places, and stripping trailing zeros only helps a value that has
// them: a real on-chain ratio like 1.495209570451729242 has none, so without
// rounding it renders at full width next to a threshold of `0.95`. Three
// decimals is finer than any threshold the module uses.
const ratioDecimals = 3

// formatRatio formats a LegacyDec collateral ratio or threshold at
// ratioDecimals, trailing zeros stripped, guarding the nil Dec proto3 produces
// for an unset field.
//
// Prices are not ratios and must not round through here — see formatPrice.
func formatRatio(d math.LegacyDec) string {
	return TrimDecTrailingZeros(roundDec(DecOrZero(d), ratioDecimals).String())
}

// formatPrice formats a LegacyDec price. Prices keep far more precision than
// ratios — see FormatPriceDec.
func formatPrice(d math.LegacyDec) string {
	return FormatPriceDec(d)
}

// bmePendingAmount labels an amount that does not exist yet because the chain
// has not settled the conversion.
const bmePendingAmount = "pending"

// BMELedgerPendingCommand returns the query that shows a signer's unsettled BME
// conversions. It is the follow-up printed after every BME conversion tx.
func BMELedgerPendingCommand(owner string) string {
	return fmt.Sprintf("akt q bme ledger --owner %s --status %s",
		owner, types.LedgerRecordSatusPending.String())
}

// RenderBMEPendingConversion renders the message detail shared by every BME
// conversion transaction (MsgBurnMint, MsgMintACT, MsgBurnACT).
//
// The chain does not execute the swap in the transaction that carries it: it
// writes a pending ledger record and settles it in a later block, once the
// oracle price and the circuit breaker allow. A bare "Status: success" therefore
// describes acceptance of the request only, and leaves a user staring at a
// debited balance with no minted coins and no explanation. This block supplies
// the explanation and the follow-up query.
func RenderBMEPendingConversion(owner string, coinsToBurn sdk.Coin, denomToMint string) string {
	var buf strings.Builder

	KV(&buf, "Sender", owner)
	KV(&buf, "Burned", formatCoinSafe(coinsToBurn))
	KV(&buf, "Minted Denom", denomToMint)
	KV(&buf, "Conversion", StyleYellow.Render(bmePendingAmount)+" (settles in a later block)")
	KV(&buf, "Minted Amount", Dim("not known yet (set by the oracle price at settlement)"))

	Newline(&buf)
	fmt.Fprintln(&buf, Dim("  The chain accepted the request and recorded a pending ledger entry;"))
	fmt.Fprintln(&buf, Dim("  it burns and mints in a later block. Until it settles the burned"))
	fmt.Fprintf(&buf, "%s\n", Dim(fmt.Sprintf("  amount has left your balance and no %s has arrived. Track it with:", denomToMint)))
	fmt.Fprintf(&buf, "    %s\n", Bold(BMELedgerPendingCommand(owner)))

	return buf.String()
}

// RenderBMEMintACT renders the pending-conversion block for MsgMintACT, which
// mints ACT by definition and so carries no destination denom of its own.
func RenderBMEMintACT(owner string, coinsToBurn sdk.Coin) string {
	return RenderBMEPendingConversion(owner, coinsToBurn, sdkutil.DenomUact)
}

// RenderBMEBurnACT renders the pending-conversion block for MsgBurnACT, which
// burns ACT to mint/remint AKT and so carries no destination denom of its own.
func RenderBMEBurnACT(owner string, coinsToBurn sdk.Coin) string {
	return RenderBMEPendingConversion(owner, coinsToBurn, sdkutil.DenomUakt)
}

// formatAllowedHalted renders a boolean as "Allowed" (green) or "Halted" (red).
func formatAllowedHalted(allowed bool) string {
	if allowed {
		return StyleGreen.Render("Allowed")
	}
	return StyleRed.Render("Halted")
}
