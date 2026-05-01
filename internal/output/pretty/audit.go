package pretty

import (
	"fmt"
	"io"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	atypes "pkg.akt.dev/go/node/audit/v1"
)

func init() {
	Register((*atypes.QueryProvidersResponse)(nil), PrettyFormatterFunc(formatAuditList))
}

// RenderAuditList renders an audit providers list as a styled string.
func RenderAuditList(res *atypes.QueryProvidersResponse) string {
	var buf strings.Builder
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
		rows = append(rows, []string{p.Owner, p.Auditor, attrs})
	}
	WriteTable(&buf, headers, rows)
	return buf.String()
}

func formatAuditList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderAuditList(msg.(*atypes.QueryProvidersResponse)))
	return err
}
