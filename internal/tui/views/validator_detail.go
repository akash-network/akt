package views

import (
	"fmt"
	"strings"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"pkg.akt.dev/akt/internal/ui/theme"
)

// ValidatorDetailView is the drill-down detail view for a single validator.
type ValidatorDetailView struct {
	validator *stakingtypes.Validator
	rank      int
	width     int
	height    int
	scroll    int
}

// NewValidatorDetailView creates a new empty validator detail view.
func NewValidatorDetailView() ValidatorDetailView {
	return ValidatorDetailView{}
}

// SetValidator sets the validator to display and resets scroll.
func (v *ValidatorDetailView) SetValidator(val *stakingtypes.Validator, rank int) {
	v.validator = val
	v.rank = rank
	v.scroll = 0
}

// Validator returns the currently displayed validator, or nil.
func (v ValidatorDetailView) Validator() *stakingtypes.Validator {
	return v.validator
}

// SetSize updates the available width and height for rendering.
func (v *ValidatorDetailView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// ScrollUp scrolls the content up by one line.
func (v *ValidatorDetailView) ScrollUp() {
	if v.scroll > 0 {
		v.scroll--
	}
}

// ScrollDown scrolls the content down by one line.
func (v *ValidatorDetailView) ScrollDown() {
	v.scroll++
}

// View renders the validator detail panel.
func (v ValidatorDetailView) View() string {
	if v.validator == nil {
		return theme.Muted.Render("  No validator selected")
	}

	w := v.width
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

	// Apply scrolling
	visibleH := v.height - 4
	if visibleH < 3 {
		visibleH = 3
	}

	start := v.scroll
	if start >= len(lines) {
		start = max(0, len(lines)-1)
	}
	end := start + visibleH
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[start:end], "\n")
}

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
