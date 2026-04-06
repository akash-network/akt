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
	res := msg.(*stakingtypes.QueryValidatorsResponse)

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

	WriteTable(w, headers, rows)
	return nil
}

func formatValidatorDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	v := msg.(*stakingtypes.Validator)

	fmt.Fprintln(w, Section("Validator"))
	KV(w, "Moniker", Bold(v.Description.Moniker))
	KV(w, "Operator", v.OperatorAddress)
	KV(w, "Status", ColorState(bondStatusLabel(v.GetStatus())))
	KV(w, "Jailed", fmt.Sprintf("%t", v.Jailed))
	KV(w, "Tokens", Bold(FormatDecAsAKT(math.LegacyNewDecFromInt(v.Tokens))))
	KV(w, "Delegator Shares", v.DelegatorShares.String())

	Newline(w)
	fmt.Fprintln(w, Section("Description"))
	if v.Description.Identity != "" {
		KV(w, "Identity", v.Description.Identity)
	}
	if v.Description.Website != "" {
		KV(w, "Website", v.Description.Website)
	}
	if v.Description.SecurityContact != "" {
		KV(w, "Security", v.Description.SecurityContact)
	}
	if v.Description.Details != "" {
		KV(w, "Details", v.Description.Details)
	}

	Newline(w)
	fmt.Fprintln(w, Section("Commission"))
	KV(w, "Rate", formatCommissionRate(v.Commission.CommissionRates.Rate))
	KV(w, "Max Rate", formatCommissionRate(v.Commission.CommissionRates.MaxRate))
	KV(w, "Max Change Rate", formatCommissionRate(v.Commission.CommissionRates.MaxChangeRate))
	KV(w, "Updated At", v.Commission.UpdateTime.Format("2006-01-02 15:04:05 UTC"))

	if v.UnbondingHeight > 0 {
		Newline(w)
		fmt.Fprintln(w, Section("Unbonding"))
		KV(w, "Height", FormatHeight(v.UnbondingHeight))
		KV(w, "Time", v.UnbondingTime.Format("2006-01-02 15:04:05 UTC"))
	}

	return nil
}

func formatDelegationDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*stakingtypes.DelegationResponse)

	fmt.Fprintln(w, Section("Delegation"))
	KV(w, "Delegator", res.Delegation.DelegatorAddress)
	KV(w, "Validator", res.Delegation.ValidatorAddress)
	KV(w, "Shares", res.Delegation.Shares.String())
	KV(w, "Balance", Bold(FormatCoin(res.Balance)))

	return nil
}

func formatDelegatorDelegations(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*stakingtypes.QueryDelegatorDelegationsResponse)

	headers := []string{"VALIDATOR", "SHARES", "BALANCE"}
	rows := make([][]string, 0, len(res.DelegationResponses))

	for _, d := range res.DelegationResponses {
		rows = append(rows, []string{
			d.Delegation.ValidatorAddress,
			d.Delegation.Shares.String(),
			Bold(FormatCoin(d.Balance)),
		})
	}

	WriteTable(w, headers, rows)
	return nil
}

func formatStakingPool(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	pool := msg.(*stakingtypes.Pool)

	fmt.Fprintln(w, Section("Staking Pool"))
	KV(w, "Bonded Tokens", Bold(FormatDecAsAKT(math.LegacyNewDecFromInt(pool.BondedTokens))))
	KV(w, "Not Bonded", FormatDecAsAKT(math.LegacyNewDecFromInt(pool.NotBondedTokens)))

	return nil
}
