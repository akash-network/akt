package pretty

import (
	"fmt"
	"io"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
)

func init() {
	Register((*ptypes.QueryProvidersResponse)(nil), PrettyFormatterFunc(formatProvidersList))
	Register((*ptypes.QueryProviderResponse)(nil), PrettyFormatterFunc(formatProviderDetail))
}

// RenderProviderList renders a providers list as a styled string.
func RenderProviderList(res *ptypes.QueryProvidersResponse) string {
	var buf strings.Builder
	headers := []string{"OWNER", "HOST URI", "EMAIL", "WEBSITE"}
	rows := make([][]string, 0, len(res.Providers))
	for _, p := range res.Providers {
		rows = append(rows, []string{p.Owner, p.HostURI, p.Info.EMail, p.Info.Website})
	}
	WriteTable(&buf, headers, rows)
	return buf.String()
}

func formatProvidersList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderProviderList(msg.(*ptypes.QueryProvidersResponse)))
	return err
}

// RenderProviderDetail renders a provider detail as a styled string.
func RenderProviderDetail(res *ptypes.QueryProviderResponse) string {
	var buf strings.Builder
	p := res.Provider
	fmt.Fprintln(&buf, Section("Provider"))
	KV(&buf, "Owner", p.Owner)
	KV(&buf, "Host URI", p.HostURI)
	KV(&buf, "Email", p.Info.EMail)
	KV(&buf, "Website", p.Info.Website)
	if len(p.Attributes) > 0 {
		Newline(&buf)
		fmt.Fprintln(&buf, Section("Attributes"))
		for _, attr := range p.Attributes {
			KV(&buf, attr.Key, attr.Value)
		}
	}
	return buf.String()
}

func formatProviderDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderProviderDetail(msg.(*ptypes.QueryProviderResponse)))
	return err
}
