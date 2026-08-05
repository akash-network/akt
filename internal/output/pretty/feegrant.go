package pretty

import (
	"fmt"
	"io"
	"strings"

	"cosmossdk.io/x/feegrant"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"
)

func init() {
	Register((*feegrant.QueryAllowancesResponse)(nil), PrettyFormatterFunc(formatFeeGrantsByGrantee))
	Register((*feegrant.QueryAllowancesByGranterResponse)(nil), PrettyFormatterFunc(formatFeeGrantsByGranter))
}

// RenderFeeGrants renders a fee grants list as a styled string.
func RenderFeeGrants(grants []*feegrant.Grant) string {
	var buf strings.Builder
	headers := []string{"GRANTER", "GRANTEE", "ALLOWANCE TYPE"}
	rows := make([][]string, 0, len(grants))
	for _, g := range grants {
		allowanceType := "-"
		if g.Allowance != nil {
			allowanceType = shortTypeName(g.Allowance.TypeUrl)
		}
		rows = append(rows, []string{g.Granter, g.Grantee, allowanceType})
	}
	WriteTableOrEmpty(&buf, headers, rows, "(no grants)")
	return buf.String()
}

func formatFeeGrantsByGrantee(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderFeeGrants(msg.(*feegrant.QueryAllowancesResponse).Allowances))
	return err
}

func formatFeeGrantsByGranter(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderFeeGrants(msg.(*feegrant.QueryAllowancesByGranterResponse).Allowances))
	return err
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
