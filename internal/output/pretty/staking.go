package pretty

import (
	"fmt"
	"io"
	"strings"

	"cosmossdk.io/math"

	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func init() {
	Register((*stakingtypes.QueryValidatorsResponse)(nil), PrettyFormatterFunc(formatValidatorsList))
	Register((*stakingtypes.Validator)(nil), PrettyFormatterFunc(formatValidatorDetail))
	Register((*stakingtypes.DelegationResponse)(nil), PrettyFormatterFunc(formatDelegationDetail))
	Register((*stakingtypes.QueryDelegatorDelegationsResponse)(nil), PrettyFormatterFunc(formatDelegatorDelegations))
	Register((*stakingtypes.Pool)(nil), PrettyFormatterFunc(formatStakingPool))
}

// RenderValidatorList renders a validators list as a styled string.
func RenderValidatorList(res *stakingtypes.QueryValidatorsResponse) string {
	var buf strings.Builder
	headers := []string{"MONIKER", "OPERATOR", "STATUS", "VOTING POWER", "COMMISSION"}
	rows := make([][]string, 0, len(res.Validators))
	for _, v := range res.Validators {
		status := bondStatusLabel(v.GetStatus())
		votingPower := FormatDecAsAKT(math.LegacyNewDecFromInt(v.Tokens))
		commission := formatCommissionRate(v.Commission.CommissionRates.Rate)
		rows = append(rows, []string{
			Bold(v.Description.Moniker),
			v.OperatorAddress,
			ColorState(status),
			votingPower,
			commission,
		})
	}
	WriteTableOrEmpty(&buf, headers, rows, "(no validators)")
	return buf.String()
}

// RenderValidatorDetail renders a validator detail as a styled string.
func RenderValidatorDetail(v *stakingtypes.Validator) string {
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Validator"))
	KV(&buf, "Moniker", Bold(v.Description.Moniker))
	KV(&buf, "Operator", v.OperatorAddress)
	KV(&buf, "Status", ColorState(bondStatusLabel(v.GetStatus())))
	KV(&buf, "Jailed", fmt.Sprintf("%t", v.Jailed))
	KV(&buf, "Tokens", Bold(FormatDecAsAKT(math.LegacyNewDecFromInt(v.Tokens))))
	KV(&buf, "Delegator Shares", v.DelegatorShares.String())
	Newline(&buf)
	fmt.Fprintln(&buf, Section("Description"))
	if v.Description.Identity != "" {
		KV(&buf, "Identity", v.Description.Identity)
	}
	if v.Description.Website != "" {
		KV(&buf, "Website", v.Description.Website)
	}
	if v.Description.SecurityContact != "" {
		KV(&buf, "Security", v.Description.SecurityContact)
	}
	if v.Description.Details != "" {
		KV(&buf, "Details", v.Description.Details)
	}
	Newline(&buf)
	fmt.Fprintln(&buf, Section("Commission"))
	KV(&buf, "Rate", formatCommissionRate(v.Commission.CommissionRates.Rate))
	KV(&buf, "Max Rate", formatCommissionRate(v.Commission.CommissionRates.MaxRate))
	KV(&buf, "Max Change Rate", formatCommissionRate(v.Commission.CommissionRates.MaxChangeRate))
	KV(&buf, "Updated At", v.Commission.UpdateTime.Format("2006-01-02 15:04:05 UTC"))
	if v.UnbondingHeight > 0 {
		Newline(&buf)
		fmt.Fprintln(&buf, Section("Unbonding"))
		KV(&buf, "Height", FormatHeight(v.UnbondingHeight))
		KV(&buf, "Time", v.UnbondingTime.Format("2006-01-02 15:04:05 UTC"))
	}
	return buf.String()
}

// RenderDelegationDetail renders a delegation detail as a styled string.
func RenderDelegationDetail(res *stakingtypes.DelegationResponse) string {
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Delegation"))
	KV(&buf, "Delegator", res.Delegation.DelegatorAddress)
	KV(&buf, "Validator", res.Delegation.ValidatorAddress)
	KV(&buf, "Shares", res.Delegation.Shares.String())
	KV(&buf, "Balance", Bold(FormatCoin(res.Balance)))
	return buf.String()
}

// RenderDelegatorDelegations renders a delegator's delegations as a styled string.
func RenderDelegatorDelegations(res *stakingtypes.QueryDelegatorDelegationsResponse) string {
	var buf strings.Builder
	headers := []string{"VALIDATOR", "SHARES", "BALANCE"}
	rows := make([][]string, 0, len(res.DelegationResponses))
	for _, d := range res.DelegationResponses {
		rows = append(rows, []string{
			d.Delegation.ValidatorAddress,
			d.Delegation.Shares.String(),
			Bold(FormatCoin(d.Balance)),
		})
	}
	WriteTableOrEmpty(&buf, headers, rows, "(no delegations)")
	return buf.String()
}

// RenderStakingPool renders a staking pool as a styled string.
func RenderStakingPool(pool *stakingtypes.Pool) string {
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Staking Pool"))
	KV(&buf, "Bonded Tokens", Bold(FormatDecAsAKT(math.LegacyNewDecFromInt(IntOrZero(pool.BondedTokens)))))
	KV(&buf, "Not Bonded", FormatDecAsAKT(math.LegacyNewDecFromInt(IntOrZero(pool.NotBondedTokens))))
	return buf.String()
}

// bondStatusLabel maps a BondStatus enum to a human-readable lowercase label.
func bondStatusLabel(status stakingtypes.BondStatus) string {
	switch status {
	case stakingtypes.Bonded:
		return "bonded"
	case stakingtypes.Unbonding:
		return "unbonding"
	case stakingtypes.Unbonded:
		return "unbonded"
	default:
		return strings.ToLower(status.String())
	}
}

// formatCommissionRate formats a commission rate as a percentage string (e.g., "5.00%").
func formatCommissionRate(rate math.LegacyDec) string {
	pct := rate.MulInt64(100)
	return fmt.Sprintf("%s%%", pct.String())
}

func formatValidatorsList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderValidatorList(msg.(*stakingtypes.QueryValidatorsResponse)))
	return err
}

func formatValidatorDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderValidatorDetail(msg.(*stakingtypes.Validator)))
	return err
}

func formatDelegationDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderDelegationDetail(msg.(*stakingtypes.DelegationResponse)))
	return err
}

func formatDelegatorDelegations(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderDelegatorDelegations(msg.(*stakingtypes.QueryDelegatorDelegationsResponse)))
	return err
}

func formatStakingPool(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderStakingPool(msg.(*stakingtypes.Pool)))
	return err
}
