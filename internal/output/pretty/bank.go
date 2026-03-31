package pretty

import (
	"fmt"
	"io"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"
)

func init() {
	Register((*types.QueryAllBalancesResponse)(nil), PrettyFormatterFunc(formatAllBalances))
	Register((*types.QuerySpendableBalancesResponse)(nil), PrettyFormatterFunc(formatSpendableBalances))
	Register((*types.QueryBalanceResponse)(nil), PrettyFormatterFunc(formatBalance))
	Register((*types.QueryTotalSupplyResponse)(nil), PrettyFormatterFunc(formatTotalSupply))
}

func formatAllBalances(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryAllBalancesResponse)
	return writeCoinsTable(w, res.Balances)
}

func formatSpendableBalances(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QuerySpendableBalancesResponse)
	return writeCoinsTable(w, res.Balances)
}

func formatBalance(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryBalanceResponse)
	if res.Balance == nil {
		fmt.Fprintln(w, Dim("(no balance)"))
		return nil
	}

	fmt.Fprintln(w, Bold(FormatCoin(*res.Balance)))
	return nil
}

func formatTotalSupply(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryTotalSupplyResponse)
	return writeCoinsTable(w, res.Supply)
}

func writeCoinsTable(w io.Writer, coins sdk.Coins) error {
	if len(coins) == 0 {
		fmt.Fprintln(w, Dim("(no balances)"))
		return nil
	}

	cols := []ColDef{
		{Header: "BALANCES", Align: AlignRight},
	}
	rows := make([][]string, 0, len(coins))

	for _, coin := range coins {
		rows = append(rows, []string{Bold(FormatCoin(coin))})
	}

	WriteTableCols(w, cols, rows)
	return nil
}
