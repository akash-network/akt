package pretty

import (
	"fmt"
	"io"

	"cosmossdk.io/x/feegrant"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"
)

func init() {
	Register((*feegrant.QueryAllowancesResponse)(nil), PrettyFormatterFunc(formatFeeGrantsByGrantee))
	Register((*feegrant.QueryAllowancesByGranterResponse)(nil), PrettyFormatterFunc(formatFeeGrantsByGranter))
}

func formatFeeGrantsByGrantee(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*feegrant.QueryAllowancesResponse)
	return writeFeeGrantsTable(w, res.Allowances)
}

func formatFeeGrantsByGranter(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*feegrant.QueryAllowancesByGranterResponse)
	return writeFeeGrantsTable(w, res.Allowances)
}

func writeFeeGrantsTable(w io.Writer, grants []*feegrant.Grant) error {
	if len(grants) == 0 {
		fmt.Fprintln(w, Dim("(no grants)"))
		return nil
	}

	headers := []string{"GRANTER", "GRANTEE", "ALLOWANCE TYPE"}
	rows := make([][]string, 0, len(grants))

	for _, g := range grants {
		allowanceType := "-"
		if g.Allowance != nil {
			allowanceType = shortTypeName(g.Allowance.TypeUrl)
		}

		rows = append(rows, []string{
			g.Granter,
			g.Grantee,
			allowanceType,
		})
	}

	WriteTable(w, headers, rows)
	return nil
}

// shortTypeName extracts the short type name from a protobuf type URL.
// For example, "/cosmos.feegrant.v1beta1.BasicAllowance" → "BasicAllowance".
func shortTypeName(typeURL string) string {
	for i := len(typeURL) - 1; i >= 0; i-- {
		if typeURL[i] == '.' {
			return typeURL[i+1:]
		}
	}
	return typeURL
}
