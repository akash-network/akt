package views

import (
	"fmt"

	"cosmossdk.io/math"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"pkg.akt.dev/akt/internal/tui/components"
)

// StakingView renders a table of validator records.
type StakingView struct {
	table      components.ResourceTable
	validators []stakingtypes.Validator
	width      int
	height     int
}

// NewStakingView creates a new StakingView with the standard column layout.
func NewStakingView() StakingView {
	return StakingView{
		table: components.NewResourceTable(components.ResourceTableConfig{
			Columns: []components.TableColumn{
				{Header: "#", Width: 5, Align: components.AlignRight},
				{Header: "MONIKER", Width: 0, Align: components.AlignLeft},
				{Header: "POWER", Width: 10, Align: components.AlignRight},
				{Header: "VP%", Width: 8, Align: components.AlignRight},
				{Header: "COMMISSION", Width: 12, Align: components.AlignRight},
				{Header: "UPTIME", Width: 10, Align: components.AlignRight},
				{Header: "SIGNED", Width: 8, Align: components.AlignRight},
			},
			EmptyText: "Validator data requires chain connection.\nUse akt monitor network for real-time validator monitoring.",
		}),
	}
}

// SetSize updates the available width and height for rendering.
func (v *StakingView) SetSize(w, h int) {
	v.width = w
	v.height = h
	v.table.SetSize(w, h)
}

// CursorUp moves the cursor up one row.
func (v *StakingView) CursorUp() {
	v.table.CursorUp()
}

// CursorDown moves the cursor down one row.
func (v *StakingView) CursorDown() {
	v.table.CursorDown()
}

// SetData stores the validators and rebuilds the table rows.
func (v *StakingView) SetData(validators []stakingtypes.Validator) {
	v.validators = validators
	rows := make([]components.TableRow, len(validators))
	for i, val := range validators {
		rows[i] = components.TableRow{
			ID: val.OperatorAddress,
			Cells: []string{
				fmt.Sprintf("%d", i+1),
				val.GetMoniker(),
				formatTokens(val.Tokens),
				"—", // VP% TBD
				formatCommissionRate(val.Commission.CommissionRates.Rate),
				"—", "—", // uptime, signed TBD
			},
		}
	}
	v.table.SetRows(rows)
}

// SelectedValidator returns the validator at the cursor, or nil.
func (v *StakingView) SelectedValidator() *stakingtypes.Validator {
	row := v.table.SelectedRow()
	if row == nil {
		return nil
	}
	for i := range v.validators {
		if v.validators[i].OperatorAddress == row.ID {
			return &v.validators[i]
		}
	}
	return nil
}

// View renders the staking table.
func (v StakingView) View() string {
	return v.table.View()
}

// formatTokens formats a token amount as a human-readable string with M/K suffixes.
func formatTokens(tokens math.Int) string {
	f := tokens.ToLegacyDec().MustFloat64()
	uakt := f / 1_000_000 // convert from uakt to AKT
	switch {
	case uakt >= 1_000_000:
		return fmt.Sprintf("%.1fM", uakt/1_000_000)
	case uakt >= 1_000:
		return fmt.Sprintf("%.1fK", uakt/1_000)
	default:
		return fmt.Sprintf("%.0f", uakt)
	}
}

// formatCommissionRate formats a commission rate as a percentage string.
func formatCommissionRate(rate math.LegacyDec) string {
	pct := rate.MustFloat64() * 100
	return fmt.Sprintf("%.1f%%", pct)
}
