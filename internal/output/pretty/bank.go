package pretty

import (
	"fmt"
	"io"
	"strings"

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

// RenderCoinsTable renders a coins list as a styled string.
func RenderCoinsTable(coins sdk.Coins) string {
	var buf strings.Builder
	cols := []ColDef{
		{Header: "BALANCES", Align: AlignRight},
	}
	rows := make([][]string, 0, len(coins))
	for _, coin := range coins {
		rows = append(rows, []string{Bold(FormatCoin(coin))})
	}
	WriteTableColsOrEmpty(&buf, cols, rows, "(no balances)")
	return buf.String()
}

// RenderBalance renders a single balance as a styled string.
func RenderBalance(res *types.QueryBalanceResponse) string {
	if res.Balance == nil {
		return Dim("(no balance)") + "\n"
	}
	return Bold(FormatCoin(*res.Balance)) + "\n"
}

func formatAllBalances(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderCoinsTable(msg.(*types.QueryAllBalancesResponse).Balances))
	return err
}

func formatSpendableBalances(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderCoinsTable(msg.(*types.QuerySpendableBalancesResponse).Balances))
	return err
}

func formatBalance(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderBalance(msg.(*types.QueryBalanceResponse)))
	return err
}

func formatTotalSupply(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderCoinsTable(msg.(*types.QueryTotalSupplyResponse).Supply))
	return err
}
