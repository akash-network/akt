package pretty

import (
	"fmt"
	"io"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	ctypes "pkg.akt.dev/go/node/cert/v1"
)

func init() {
	Register((*ctypes.QueryCertificatesResponse)(nil), PrettyFormatterFunc(formatCertificatesList))
}

// RenderCertificateList renders a certificates list as a styled string.
func RenderCertificateList(res *ctypes.QueryCertificatesResponse) string {
	var buf strings.Builder
	headers := []string{"SERIAL", "STATE"}
	rows := make([][]string, 0, len(res.Certificates))
	for _, c := range res.Certificates {
		rows = append(rows, []string{c.Serial, ColorState(c.Certificate.State.String())})
	}
	WriteTableOrEmpty(&buf, headers, rows, "(no certificates)")
	return buf.String()
}

func formatCertificatesList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderCertificateList(msg.(*ctypes.QueryCertificatesResponse)))
	return err
}
