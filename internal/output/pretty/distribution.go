package pretty

import (
	"fmt"
	"io"

	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

func init() {
	Register((*distrtypes.QueryDelegationTotalRewardsResponse)(nil), PrettyFormatterFunc(formatDelegationTotalRewards))
	Register((*distrtypes.ValidatorAccumulatedCommission)(nil), PrettyFormatterFunc(formatValidatorCommission))
}

func formatDelegationTotalRewards(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*distrtypes.QueryDelegationTotalRewardsResponse)

	if len(res.Rewards) == 0 {
		fmt.Fprintln(w, Dim("(no rewards)"))
		return nil
	}

	headers := []string{"VALIDATOR", "REWARD"}
	rows := make([][]string, 0, len(res.Rewards))

	for _, r := range res.Rewards {
		reward := "-"
		if len(r.Reward) > 0 {
			reward = FormatDecCoins(r.Reward)
		}
		rows = append(rows, []string{
			r.ValidatorAddress,
			reward,
		})
	}

	WriteTable(w, headers, rows)

	// Print total rewards below the table.
	if len(res.Total) > 0 {
		Newline(w)
		KV(w, "Total", Bold(FormatDecCoins(res.Total)))
	}

	return nil
}

func formatValidatorCommission(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	commission := msg.(*distrtypes.ValidatorAccumulatedCommission)

	if len(commission.Commission) == 0 {
		fmt.Fprintln(w, Dim("(no commission)"))
		return nil
	}

	fmt.Fprintln(w, Section("Validator Commission"))
	KV(w, "Commission", Bold(FormatDecCoins(commission.Commission)))

	return nil
}
