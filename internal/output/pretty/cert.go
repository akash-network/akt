package pretty

import (
	"io"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	ctypes "pkg.akt.dev/go/node/cert/v1"
)

func init() {
	Register((*ctypes.QueryCertificatesResponse)(nil), PrettyFormatterFunc(formatCertificatesList))
}

func formatCertificatesList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*ctypes.QueryCertificatesResponse)

	headers := []string{"SERIAL", "STATE"}
	rows := make([][]string, 0, len(res.Certificates))

	for _, c := range res.Certificates {
		rows = append(rows, []string{
			c.Serial,
			ColorState(c.Certificate.State.String()),
		})
	}

	WriteTable(w, headers, rows)
	return nil
}
