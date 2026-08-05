package pretty

import (
	"fmt"
	"io"
	"time"

	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
)

func init() {
	// Slashing
	Register((*slashingtypes.ValidatorSigningInfo)(nil), PrettyFormatterFunc(formatSigningInfo))
	Register((*slashingtypes.QuerySigningInfosResponse)(nil), PrettyFormatterFunc(formatSigningInfos))

	// Upgrade
	Register((*upgradetypes.Plan)(nil), PrettyFormatterFunc(formatUpgradePlan))
	Register((*upgradetypes.QueryModuleVersionsResponse)(nil), PrettyFormatterFunc(formatModuleVersions))
}

// --- Slashing ---

func formatSigningInfo(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	info := msg.(*slashingtypes.ValidatorSigningInfo)

	fmt.Fprintln(w, Section("Validator Signing Info"))
	KV(w, "Address", info.Address)
	KV(w, "Start Height", FormatHeight(info.StartHeight))
	KV(w, "Index Offset", fmt.Sprintf("%d", info.IndexOffset))
	KV(w, "Missed Blocks", Bold(fmt.Sprintf("%d", info.MissedBlocksCounter)))
	KV(w, "Tombstoned", boolLabel(info.Tombstoned))

	if !info.JailedUntil.IsZero() && info.JailedUntil.After(time.Unix(0, 0)) {
		KV(w, "Jailed Until", info.JailedUntil.Format("2006-01-02 15:04:05 UTC"))
	}

	return nil
}

func formatSigningInfos(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*slashingtypes.QuerySigningInfosResponse)

	cols := []ColDef{
		{Header: "ADDRESS"},
		{Header: "START HEIGHT", Align: AlignRight},
		{Header: "MISSED BLOCKS", Align: AlignRight},
		{Header: "TOMBSTONED"},
		{Header: "JAILED UNTIL"},
	}

	rows := make([][]string, 0, len(res.Info))
	for _, info := range res.Info {
		jailedUntil := "-"
		if !info.JailedUntil.IsZero() && info.JailedUntil.After(time.Unix(0, 0)) {
			jailedUntil = info.JailedUntil.Format("2006-01-02 15:04")
		}

		rows = append(rows, []string{
			info.Address,
			FormatHeight(info.StartHeight),
			Bold(fmt.Sprintf("%d", info.MissedBlocksCounter)),
			boolLabel(info.Tombstoned),
			jailedUntil,
		})
	}

	WriteTableColsOrEmpty(w, cols, rows, "(no signing infos)")
	return nil
}

func boolLabel(v bool) string {
	if v {
		return StyleRed.Render("yes")
	}
	return StyleGreen.Render("no")
}

// --- Upgrade ---

func formatUpgradePlan(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	plan := msg.(*upgradetypes.Plan)

	fmt.Fprintln(w, Section("Upgrade Plan"))
	KV(w, "Name", Bold(plan.Name))
	KV(w, "Height", Bold(FormatHeight(plan.Height)))
	if plan.Info != "" {
		KV(w, "Info", plan.Info)
	}

	return nil
}

func formatModuleVersions(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*upgradetypes.QueryModuleVersionsResponse)

	cols := []ColDef{
		{Header: "MODULE"},
		{Header: "VERSION", Align: AlignRight},
	}

	rows := make([][]string, 0, len(res.ModuleVersions))
	for _, mv := range res.ModuleVersions {
		rows = append(rows, []string{
			Bold(mv.Name),
			fmt.Sprintf("%d", mv.Version),
		})
	}

	WriteTableColsOrEmpty(w, cols, rows, "(no module versions)")
	return nil
}
