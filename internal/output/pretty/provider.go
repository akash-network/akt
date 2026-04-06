package pretty

import (
	"fmt"
	"io"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
)

func init() {
	Register((*ptypes.QueryProvidersResponse)(nil), PrettyFormatterFunc(formatProvidersList))
	Register((*ptypes.QueryProviderResponse)(nil), PrettyFormatterFunc(formatProviderDetail))
}

func formatProvidersList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*ptypes.QueryProvidersResponse)

	headers := []string{"OWNER", "HOST URI", "EMAIL", "WEBSITE"}
	rows := make([][]string, 0, len(res.Providers))

	for _, p := range res.Providers {
		rows = append(rows, []string{
			p.Owner,
			p.HostURI,
			p.Info.EMail,
			p.Info.Website,
		})
	}

	WriteTable(w, headers, rows)
	return nil
}

func formatProviderDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*ptypes.QueryProviderResponse)
	p := res.Provider

	fmt.Fprintln(w, Section("Provider"))
	KV(w, "Owner", p.Owner)
	KV(w, "Host URI", p.HostURI)
	KV(w, "Email", p.Info.EMail)
	KV(w, "Website", p.Info.Website)

	if len(p.Attributes) > 0 {
		Newline(w)
		fmt.Fprintln(w, Section("Attributes"))
		for _, attr := range p.Attributes {
			KV(w, attr.Key, attr.Value)
		}
	}

	return nil
}
