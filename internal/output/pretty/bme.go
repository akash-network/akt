package pretty

import (
	"fmt"
	"io"
	"strings"

	"cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	types "pkg.akt.dev/go/node/bme/v1"
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
	KV(&buf, "Collateral Ratio", Bold(res.CollateralRatio.String()))
	KVHeader(&buf, "Thresholds")
	SubKV(&buf, "Warn", res.WarnThreshold.String())
	SubKV(&buf, "Halt", res.HaltThreshold.String())
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
			status = StyleGreen.Render("e")
			if rec.ExecutedRecord != nil {
				er := rec.ExecutedRecord
				if er.Burned != nil {
					burned = formatCoinPrice(er.Burned)
				}
				if er.Minted != nil {
					minted = formatCoinPrice(er.Minted)
				}
				if !er.Spread.IsZero() {
					spread = FormatCoin(er.Spread)
				}
				if er.RemintCreditIssued != nil {
					remintIssued = formatCoinPrice(er.RemintCreditIssued)
				}
				if er.RemintCreditAccrued != nil {
					remintAccrued = FormatCoin(er.RemintCreditAccrued.Coin)
				}
			}
		case *types.QueryLedgerRecordEntry_PendingRecord:
			status = StyleYellow.Render("p")
			if rec.PendingRecord != nil {
				pr := rec.PendingRecord
				burned = FormatCoin(pr.CoinsToBurn)
				minted = pr.DenomToMint
			}
		case *types.QueryLedgerRecordEntry_CanceledRecord:
			if rec.CanceledRecord != nil {
				cr := rec.CanceledRecord
				status = StyleRed.Render("c:" + cr.CancelReason.String())
				burned = FormatCoin(cr.CoinsToBurn)
				minted = cr.DenomToMint
			} else {
				status = StyleRed.Render("c")
			}
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

// formatCoinPrice formats a CoinPrice as "amount denom @price".
func formatCoinPrice(cp *types.CoinPrice) string {
	if cp == nil {
		return "-"
	}
	return fmt.Sprintf("%s @%s", FormatCoin(cp.Coin), formatDecTrimmed(cp.Price))
}

// formatDecTrimmed formats a LegacyDec trimming trailing zeros after the decimal point.
func formatDecTrimmed(d math.LegacyDec) string {
	s := d.String()
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// formatAllowedHalted renders a boolean as "Allowed" (green) or "Halted" (red).
func formatAllowedHalted(allowed bool) string {
	if allowed {
		return StyleGreen.Render("Allowed")
	}
	return StyleRed.Render("Halted")
}
