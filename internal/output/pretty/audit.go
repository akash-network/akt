package pretty

import (
	"io"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	atypes "pkg.akt.dev/go/node/audit/v1"
)

func init() {
	Register((*atypes.QueryProvidersResponse)(nil), PrettyFormatterFunc(formatAuditList))
}

func formatAuditList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*atypes.QueryProvidersResponse)

	headers := []string{"OWNER", "AUDITOR", "ATTRIBUTES"}
	rows := make([][]string, 0, len(res.Providers))

	for _, p := range res.Providers {
		attrs := ""
		for i, a := range p.Attributes {
			if i > 0 {
				attrs += ", "
			}
			attrs += a.Key + "=" + a.Value
		}
		rows = append(rows, []string{
			p.Owner,
			p.Auditor,
			attrs,
		})
	}

	WriteTable(w, headers, rows)
	return nil
}
