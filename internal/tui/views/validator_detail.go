package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"pkg.akt.dev/akt/internal/tui/components"
	"pkg.akt.dev/akt/internal/tui/keys"
	"pkg.akt.dev/akt/internal/tui/messages"
	"pkg.akt.dev/akt/internal/ui/theme"
)

var _ ViewComponent = (*ValidatorDetailView)(nil)

// ValidatorDetailView is the drill-down detail view for a single validator.
type ValidatorDetailView struct {
	BaseDetailView
	km        keys.KeyMap
	validator *stakingtypes.Validator
	rank      int
}

// NewValidatorDetailView creates a detail view pre-loaded with a validator record.
func NewValidatorDetailView(km keys.KeyMap, validator *stakingtypes.Validator, rank int) *ValidatorDetailView {
	return &ValidatorDetailView{
		BaseDetailView: NewBaseDetailView(),
		km:             km,
		validator:      validator,
		rank:           rank,
	}
}

// ─── tea.Model ───────────────────────────────────────────────────────

// Init returns nil — data is already set via the constructor.
func (v *ValidatorDetailView) Init() tea.Cmd { return nil }

// Update handles key events for the validator detail view.
func (v *ValidatorDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kmsg, v.km.Back):
			return v, CmdFunc(messages.PopViewMsg{})
		case key.Matches(kmsg, v.km.Close):
			if v.validator != nil {
				return v, CmdFunc(messages.ShowConfirmMsg{
					Kind: components.ConfirmDelegate,
					Data: components.ConfirmData{
						Title: "Delegate Tokens",
						Body:  fmt.Sprintf("Delegate to %s?", v.validator.GetMoniker()),
					},
				})
			}
		default:
			// j/k scroll handled by BaseDetailView
			v.BaseDetailView.Update(msg)
		}
	}
	return v, nil
}

// View renders the validator detail panel.
func (v *ValidatorDetailView) View() tea.View {
	if v.validator == nil {
		return tea.NewView(theme.Muted.Render("  No validator selected"))
	}

	w := v.W
	if w < 40 {
		w = 40
	}

	val := v.validator
	statusLabel := validatorStatusLabel(val)

	var lines []string

	// Section 1: Validator info (moniker as title)
	lines = append(lines, "  "+theme.SectionTitle.Render(val.GetMoniker()))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		kvPair("Rank", theme.KVValueBold.Render(fmt.Sprintf("#%d", v.rank))),
		kvPair("Address", theme.KVValue.Render(val.OperatorAddress)),
		kvPair("Tokens", theme.KVValue.Render(formatTokens(val.Tokens))),
		kvPair("Commission", theme.KVValue.Render(formatCommissionRate(val.Commission.CommissionRates.Rate))),
		kvPair("Status", theme.StateBadge(statusLabel).Render(statusLabel)),
		kvPair("Uptime", theme.KVValue.Render("—")),
	)
	lines = append(lines, "")

	// Section 2: Your Delegation (placeholder)
	lines = append(lines, "  "+theme.SectionTitle.Render("Your Delegation"))
	lines = append(lines, "  "+theme.HRuleAccent(w-4))
	lines = append(lines,
		kvPair("Delegated", theme.KVValue.Render("—")),
		kvPair("Rewards", theme.KVValue.Render("—")),
		kvPair("Share", theme.KVValueMuted.Render("—")),
	)

	// Apply scrolling via BaseDetailView
	visibleH := v.H - 4
	if visibleH < 3 {
		visibleH = 3
	}

	visible := v.BaseDetailView.VisibleWindow(lines, visibleH)

	return tea.NewView(strings.Join(visible, "\n"))
}

// ─── ViewComponent ───────────────────────────────────────────────────

// SetSize delegates to the embedded BaseDetailView.
func (v *ValidatorDetailView) SetSize(w, h int) {
	v.BaseDetailView.SetSize(w, h)
}

// Breadcrumb returns the navigation label for this view.
func (v *ValidatorDetailView) Breadcrumb() string {
	return "Detail"
}

// ShortHelp returns the footer hint pairs for the validator detail view.
func (v *ValidatorDetailView) ShortHelp() []components.HintPair {
	return []components.HintPair{
		{Key: "j/k", Desc: "scroll"},
		{Key: "d", Desc: "delegate", Accent: true},
		{Key: "esc", Desc: "back"},
	}
}

// Refresh returns nil — detail views have no data to reload.
func (v *ValidatorDetailView) Refresh() tea.Cmd { return nil }

// ─── Helpers ─────────────────────────────────────────────────────────

// validatorStatusLabel returns a human-readable status string for a validator.
func validatorStatusLabel(val *stakingtypes.Validator) string {
	switch val.GetStatus() {
	case stakingtypes.Bonded:
		return "bonded"
	case stakingtypes.Unbonding:
		return "unbonding"
	case stakingtypes.Unbonded:
		return "unbonded"
	default:
		return strings.ToLower(val.GetStatus().String())
	}
}
