package pretty

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	clioutput "pkg.akt.dev/akt/internal/output"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
)

func init() {
	Register((*dvbeta.QueryDeploymentsResponse)(nil), PrettyFormatterFunc(formatDeploymentsList))
	Register((*dvbeta.QueryDeploymentResponse)(nil), PrettyFormatterFunc(formatDeploymentDetail))
	Register((*dvbeta.QueryGroupResponse)(nil), PrettyFormatterFunc(formatGroupDetail))
	// QueryParamsResponse is registered in params.go.
}

// RenderDeploymentList renders a deployments list as a styled string.
// Used by both CLI pretty output and TUI deployment list views.
func RenderDeploymentList(res *dvbeta.QueryDeploymentsResponse) string {
	var buf strings.Builder

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

	WriteTableColsOrEmpty(&buf, cols, rows, "(no deployments)")
	return buf.String()
}

func formatDeploymentsList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderDeploymentList(msg.(*dvbeta.QueryDeploymentsResponse)))
	return err
}

// RenderDeploymentDetail renders a deployment detail as a styled string.
// Used by both CLI pretty output and TUI deployment detail views.
func RenderDeploymentDetail(res *dvbeta.QueryDeploymentResponse) string {
	var buf strings.Builder
	dep := res.Deployment

	fmt.Fprintln(&buf, Section("Deployment"))
	KV(&buf, "DSEQ", Bold(fmt.Sprintf("%d", dep.ID.DSeq)))
	KV(&buf, "Owner", dep.ID.Owner)
	KV(&buf, "State", ColorState(dep.State.String()))
	KV(&buf, "Hash", hex.EncodeToString(dep.Hash))
	KV(&buf, "Created At", FormatHeight(dep.CreatedAt))

	for i, g := range res.Groups {
		Newline(&buf)
		fmt.Fprintf(&buf, "%s %s\n", Section(fmt.Sprintf("Group %d:", i+1)), Bold(g.GroupSpec.Name))
		KV(&buf, "GSeq", fmt.Sprintf("%d", g.ID.GSeq))
		KV(&buf, "State", ColorState(g.State.String()))

		formatResourceUnits(&buf, g.GroupSpec.Resources)
	}

	Newline(&buf)
	fmt.Fprintln(&buf, Section("Escrow"))
	esc := res.EscrowAccount
	KV(&buf, "Account ID", fmt.Sprintf("%s/%s", esc.ID.Scope.String(), esc.ID.XID))
	KV(&buf, "State", ColorState(esc.State.State.String()))
	KV(&buf, "Owner", esc.State.Owner)
	if len(esc.State.Funds) > 0 {
		for _, f := range esc.State.Funds {
			KV(&buf, "Balance", FormatDecAmount(f.Amount, f.Denom))
		}
	}
	if len(esc.State.Transferred) > 0 {
		for _, c := range esc.State.Transferred {
			KV(&buf, "Spent", FormatDecAmount(c.Amount, c.Denom))
		}
	}

	return buf.String()
}

func formatDeploymentDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderDeploymentDetail(msg.(*dvbeta.QueryDeploymentResponse)))
	return err
}

// RenderGroupDetail renders a group detail as a styled string.
// Used by both CLI pretty output and TUI group detail views.
func RenderGroupDetail(res *dvbeta.QueryGroupResponse) string {
	var buf strings.Builder
	g := res.Group

	fmt.Fprintln(&buf, Section("Group"))
	KV(&buf, "Name", Bold(g.GroupSpec.Name))
	KV(&buf, "Owner", g.ID.Owner)
	KV(&buf, "DSeq", fmt.Sprintf("%d", g.ID.DSeq))
	KV(&buf, "GSeq", fmt.Sprintf("%d", g.ID.GSeq))
	KV(&buf, "State", ColorState(g.State.String()))
	KV(&buf, "Created At", FormatHeight(g.CreatedAt))

	formatResourceUnits(&buf, g.GroupSpec.Resources)

	return buf.String()
}

func formatGroupDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderGroupDetail(msg.(*dvbeta.QueryGroupResponse)))
	return err
}

// RenderGroupsList renders a list of groups as a styled string.
// Used by both CLI pretty output and TUI views.
func RenderGroupsList(groups dvbeta.Groups) string {
	var buf strings.Builder

	// A deployment with no groups printed nothing at all, which is
	// indistinguishable from a command that failed silently.
	if len(groups) == 0 {
		fmt.Fprintln(&buf, Dim("(no groups)"))
		return buf.String()
	}

	for i, g := range groups {
		if i > 0 {
			Newline(&buf)
		}
		fmt.Fprintf(&buf, "%s %s\n", Section(fmt.Sprintf("Group %d:", i+1)), Bold(g.GroupSpec.Name))
		KV(&buf, "ID", g.ID.String())
		KV(&buf, "State", ColorState(g.State.String()))
		KV(&buf, "Created At", FormatHeight(g.CreatedAt))

		formatResourceUnits(&buf, g.GroupSpec.Resources)
	}

	return buf.String()
}

// PrintGroupsList formats a list of deployment groups for display.
// This is used when querying groups for a deployment (no gseq specified).
func PrintGroupsList(cmd *cobra.Command, cctx sdkclient.Context, groups dvbeta.Groups) error {
	output, _ := cmd.Flags().GetString(flagdefs.FlagOutput)
	checked := clioutput.NewCheckedWriter(cmd.OutOrStdout())
	cctx = cctx.WithOutput(checked)
	if output == cflags.OutputJSON || output == cflags.OutputYAML {
		encoded := make([]json.RawMessage, len(groups))
		for i := range groups {
			raw, err := cctx.Codec.MarshalJSON(&groups[i])
			if err != nil {
				return checked.Complete(err)
			}
			encoded[i] = raw
		}
		raw, err := json.Marshal(encoded)
		if err != nil {
			return checked.Complete(err)
		}

		outCctx := cctx.WithOutputFormat("json")
		if output == cflags.OutputYAML {
			outCctx = cctx.WithOutputFormat("text")
		}
		return checked.Complete(outCctx.PrintRaw(raw))
	}

	checked = clioutput.NewCheckedTerminalWriter(cmd.OutOrStdout())
	_, err := fmt.Fprint(checked, RenderGroupsList(groups))
	return checked.Complete(err)
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
