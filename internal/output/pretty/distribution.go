package pretty

import (
	"fmt"
	"io"
	"strings"

	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

func init() {
	Register((*distrtypes.QueryDelegationTotalRewardsResponse)(nil), PrettyFormatterFunc(formatDelegationTotalRewards))
	Register((*distrtypes.ValidatorAccumulatedCommission)(nil), PrettyFormatterFunc(formatValidatorCommission))
}

// RenderDelegationTotalRewards renders delegation rewards as a styled string.
func RenderDelegationTotalRewards(res *distrtypes.QueryDelegationTotalRewardsResponse) string {
	var buf strings.Builder
	if len(res.Rewards) == 0 {
		fmt.Fprintln(&buf, Dim("(no rewards)"))
		return buf.String()
	}
	headers := []string{"VALIDATOR", "REWARD"}
	rows := make([][]string, 0, len(res.Rewards))
	for _, r := range res.Rewards {
		reward := "-"
		if len(r.Reward) > 0 {
			reward = FormatDecCoins(r.Reward)
		}
		rows = append(rows, []string{r.ValidatorAddress, reward})
	}
	WriteTable(&buf, headers, rows)
	if len(res.Total) > 0 {
		Newline(&buf)
		KV(&buf, "Total", Bold(FormatDecCoins(res.Total)))
	}
	return buf.String()
}

func formatDelegationTotalRewards(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderDelegationTotalRewards(msg.(*distrtypes.QueryDelegationTotalRewardsResponse)))
	return err
}

// RenderValidatorCommission renders validator commission as a styled string.
func RenderValidatorCommission(commission *distrtypes.ValidatorAccumulatedCommission) string {
	var buf strings.Builder
	if len(commission.Commission) == 0 {
		fmt.Fprintln(&buf, Dim("(no commission)"))
		return buf.String()
	}
	fmt.Fprintln(&buf, Section("Validator Commission"))
	KV(&buf, "Commission", Bold(FormatDecCoins(commission.Commission)))
	return buf.String()
}

func formatValidatorCommission(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderValidatorCommission(msg.(*distrtypes.ValidatorAccumulatedCommission)))
	return err
}
