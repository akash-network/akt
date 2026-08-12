package views

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"pkg.akt.dev/akt/internal/output/pretty"
	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/data"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
)

var _ ViewComponent = (*StakingView)(nil)

// StakingView is a full tea.Model list view for validator records.
// It embeds BaseListView for cursor/scroll handling and satisfies the
// ViewComponent interface so the App shell can push it onto the nav stack.
type StakingView struct {
	BaseListView
	svc         data.Service
	validators  []stakingtypes.Validator
	totalBonded math.Int
}

// NewStakingView creates a StakingView wired to the given data service.
func NewStakingView(svc data.Service, km keys.KeyMap) *StakingView {
	cfg := components.ResourceTableConfig{
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
	}
	return &StakingView{
		BaseListView: NewBaseListView(cfg, km),
		svc:          svc,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init kicks off the initial data load.
func (v *StakingView) Init() tea.Cmd {
	return v.svc.LoadValidators()
}

// Update handles messages for the staking list.
func (v *StakingView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.ValidatorsLoadedMsg:
		if msg.Err == nil {
			v.validators = msg.Validators
			v.rebuildRows()
			// Fire staking pool load for VP% calculation
			return v, v.svc.LoadStakingPool()
		}
		return v, nil
	case messages.StakingPoolMsg:
		if msg.Err == nil {
			v.totalBonded = msg.BondedTokens
			v.rebuildRows()
		}
		return v, nil
	}

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, v.Keys.Select):
			val := v.selectedValidator()
			if val != nil {
				rank := v.BaseListView.Cursor() + 1
				detail := NewValidatorDetailView(v.Keys, val, rank)
				return v, CmdFunc(messages.PushViewMsg{View: detail})
			}
		case key.Matches(kmsg, v.Keys.Close):
			val := v.selectedValidator()
			if val != nil {
				return v, CmdFunc(messages.ShowConfirmMsg{
					Kind: components.ConfirmDelegate,
					Data: components.ConfirmData{
						Title: "Delegate Tokens",
						Body:  fmt.Sprintf("Delegate to %s?", val.GetMoniker()),
					},
				})
			}
		}
		// Fall through to BaseListView for cursor keys
		v.BaseListView.Update(msg)
	}
	return v, nil
}

// View delegates rendering to the embedded BaseListView table.
func (v *StakingView) View() tea.View {
	return v.BaseListView.View()
}

// ─── ViewComponent ───────────────────────────────────────────────────

// SetSize delegates to the embedded BaseListView.
func (v *StakingView) SetSize(w, h int) {
	v.BaseListView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (v *StakingView) Breadcrumb() string {
	return "Staking"
}

// ShortHelp returns the footer hint pairs for the staking list.
func (v *StakingView) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "navigate"},
		{Key: "↵", Desc: "detail"},
		{Key: "d", Desc: "delegate", Accent: true},
		{Key: "esc", Desc: "back"},
	}
}

// Refresh re-fires the data load for this view.
func (v *StakingView) Refresh() tea.Cmd {
	return v.svc.LoadValidators()
}

// ─── Internal ────────────────────────────────────────────────────────

// selectedValidator returns the validator at the cursor, or nil.
func (v *StakingView) selectedValidator() *stakingtypes.Validator {
	row := v.BaseListView.SelectedRow()
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

// rebuildRows rebuilds the table rows from the current validators and totalBonded.
func (v *StakingView) rebuildRows() {
	rows := make([]components.TableRow, len(v.validators))
	for i, val := range v.validators {
		vpPct := "—"
		if !v.totalBonded.IsNil() && !v.totalBonded.IsZero() {
			pct := val.Tokens.ToLegacyDec().Quo(v.totalBonded.ToLegacyDec()).MulInt64(100)
			vpPct = fmt.Sprintf("%.2f%%", pct.MustFloat64())
		}
		rows[i] = components.TableRow{
			ID: val.OperatorAddress,
			Cells: []string{
				fmt.Sprintf("%d", i+1),
				val.GetMoniker(),
				formatTokens(val.Tokens),
				vpPct,
				formatCommissionRate(val.Commission.CommissionRates.Rate),
				"—", "—", // uptime, signed TBD
			},
		}
	}
	v.BaseListView.SetRows(rows)
}

// formatTokens formats a validator's uakt tokens with the canonical amount
// formatter shared by single-shot and full-screen output.
func formatTokens(tokens math.Int) string {
	return pretty.FormatCoin(sdk.Coin{Denom: "uakt", Amount: tokens})
}

// formatCommissionRate formats a commission rate as a percentage string.
func formatCommissionRate(rate math.LegacyDec) string {
	pct := rate.MustFloat64() * 100
	return fmt.Sprintf("%.1f%%", pct)
}
