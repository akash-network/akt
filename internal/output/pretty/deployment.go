package pretty

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
)

func init() {
	Register((*dvbeta.QueryDeploymentsResponse)(nil), PrettyFormatterFunc(formatDeploymentsList))
	Register((*dvbeta.QueryDeploymentResponse)(nil), PrettyFormatterFunc(formatDeploymentDetail))
	Register((*dvbeta.QueryGroupResponse)(nil), PrettyFormatterFunc(formatGroupDetail))
	// QueryParamsResponse is registered in params.go.
}

func formatDeploymentsList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*dvbeta.QueryDeploymentsResponse)

	cols := []ColDef{
		{Header: "ID"},
		{Header: "STATE"},
		{Header: "GROUPS", Align: AlignRight},
		{Header: "CREATED AT", Align: AlignRight},
	}
	rows := make([][]string, 0, len(res.Deployments))

	for _, d := range res.Deployments {
		dep := d.Deployment
		rows = append(rows, []string{
			dep.ID.String(),
			ColorState(dep.State.String()),
			fmt.Sprintf("%d", len(d.Groups)),
			FormatHeight(dep.CreatedAt),
		})
	}

	WriteTableCols(w, cols, rows)
	return nil
}

func formatDeploymentDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*dvbeta.QueryDeploymentResponse)
	dep := res.Deployment

	fmt.Fprintln(w, Section("Deployment"))
	KV(w, "DSEQ", Bold(fmt.Sprintf("%d", dep.ID.DSeq)))
	KV(w, "Owner", dep.ID.Owner)
	KV(w, "State", ColorState(dep.State.String()))
	KV(w, "Hash", hex.EncodeToString(dep.Hash))
	KV(w, "Created At", FormatHeight(dep.CreatedAt))

	for i, g := range res.Groups {
		Newline(w)
		fmt.Fprintf(w, "%s %s\n", Section(fmt.Sprintf("Group %d:", i+1)), Bold(g.GroupSpec.Name))
		KV(w, "GSeq", fmt.Sprintf("%d", g.ID.GSeq))
		KV(w, "State", ColorState(g.State.String()))

		formatResourceUnits(w, g.GroupSpec.Resources)
	}

	Newline(w)
	fmt.Fprintln(w, Section("Escrow"))
	esc := res.EscrowAccount
	KV(w, "Account ID", fmt.Sprintf("%s/%s", esc.ID.Scope.String(), esc.ID.XID))
	KV(w, "State", ColorState(esc.State.State.String()))
	KV(w, "Owner", esc.State.Owner)
	if len(esc.State.Funds) > 0 {
		for _, f := range esc.State.Funds {
			KV(w, "Balance", FormatDecAmount(f.Amount, f.Denom))
		}
	}
	if len(esc.State.Transferred) > 0 {
		for _, c := range esc.State.Transferred {
			KV(w, "Spent", FormatDecAmount(c.Amount, c.Denom))
		}
	}

	return nil
}

func formatGroupDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*dvbeta.QueryGroupResponse)
	g := res.Group

	fmt.Fprintln(w, Section("Group"))
	KV(w, "Name", Bold(g.GroupSpec.Name))
	KV(w, "Owner", g.ID.Owner)
	KV(w, "DSeq", fmt.Sprintf("%d", g.ID.DSeq))
	KV(w, "GSeq", fmt.Sprintf("%d", g.ID.GSeq))
	KV(w, "State", ColorState(g.State.String()))
	KV(w, "Created At", FormatHeight(g.CreatedAt))

	formatResourceUnits(w, g.GroupSpec.Resources)

	return nil
}

// PrintGroupsList formats a list of deployment groups for display.
// This is used when querying groups for a deployment (no gseq specified).
func PrintGroupsList(cmd *cobra.Command, cctx sdkclient.Context, groups dvbeta.Groups) error {
	output, _ := cmd.Flags().GetString(cflags.FlagOutput)
	if output == cflags.OutputJSON || output == cflags.OutputYAML {
		outCctx := cctx.WithOutputFormat(output)
		if output == cflags.OutputYAML {
			outCctx = cctx.WithOutputFormat("text")
		}
		for _, g := range groups {
			if err := outCctx.PrintProto(&g); err != nil {
				return err
			}
		}
		return nil
	}

	return formatGroupsList(os.Stdout, groups)
}

func formatGroupsList(w io.Writer, groups dvbeta.Groups) error {
	for i, g := range groups {
		if i > 0 {
			Newline(w)
		}
		fmt.Fprintf(w, "%s %s\n", Section(fmt.Sprintf("Group %d:", i+1)), Bold(g.GroupSpec.Name))
		KV(w, "ID", g.ID.String())
		KV(w, "State", ColorState(g.State.String()))
		KV(w, "Created At", FormatHeight(g.CreatedAt))

		formatResourceUnits(w, g.GroupSpec.Resources)
	}

	return nil
}

// formatResourceUnits renders a list of resource units with full spec details.
func formatResourceUnits(w io.Writer, resources dvbeta.ResourceUnits) {
	for i, r := range resources {
		Newline(w)
		fmt.Fprintf(w, "  %s\n", Section(fmt.Sprintf("Resource %d", i+1)))
		KV(w, "  Count", fmt.Sprintf("%d", r.Count))
		KV(w, "  Price", Bold(FormatDecCoin(r.Price)))

		res := r.Resources
		if res.CPU != nil {
			KV(w, "  CPU", FormatCPU(res.CPU.Units.Val))
		}
		if res.Memory != nil {
			KV(w, "  Memory", FormatResourceBytes(res.Memory.Quantity.Val))
		}
		if res.GPU != nil && !res.GPU.Units.Val.IsZero() {
			KV(w, "  GPU", res.GPU.Units.Val.String())
		}
		for _, s := range res.Storage {
			name := s.Name
			if name == "" {
				name = "default"
			}
			KV(w, fmt.Sprintf("  Storage (%s)", name), FormatResourceBytes(s.Quantity.Val))
		}
	}
}
