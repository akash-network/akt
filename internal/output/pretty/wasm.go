package pretty

import (
	"fmt"
	"io"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	"github.com/CosmWasm/wasmd/x/wasm/types"
)

func init() {
	Register((*types.QueryCodesResponse)(nil), PrettyFormatterFunc(formatWasmCodeList))
	Register((*types.QueryContractsByCodeResponse)(nil), PrettyFormatterFunc(formatWasmContractsByCode))
	Register((*types.QueryCodeInfoResponse)(nil), PrettyFormatterFunc(formatWasmCodeInfo))
	Register((*types.QueryContractInfoResponse)(nil), PrettyFormatterFunc(formatWasmContractInfo))
	Register((*types.QueryContractHistoryResponse)(nil), PrettyFormatterFunc(formatWasmContractHistory))
	Register((*types.QueryPinnedCodesResponse)(nil), PrettyFormatterFunc(formatWasmPinnedCodes))
	Register((*types.QueryContractsByCreatorResponse)(nil), PrettyFormatterFunc(formatWasmContractsByCreator))
}

// RenderWasmCodeList renders a WASM codes list as a styled string.
func RenderWasmCodeList(res *types.QueryCodesResponse) string {
	var buf strings.Builder
	cols := []ColDef{
		{Header: "CODE ID", Align: AlignRight},
		{Header: "CREATOR"},
		{Header: "CHECKSUM"},
	}
	rows := make([][]string, 0, len(res.CodeInfos))
	for _, ci := range res.CodeInfos {
		checksum := ci.DataHash.String()
		if len(checksum) > 16 {
			checksum = checksum[:16] + "..."
		}
		rows = append(rows, []string{Bold(fmt.Sprintf("%d", ci.CodeID)), ci.Creator, checksum})
	}
	WriteTableColsOrEmpty(&buf, cols, rows, "(no codes)")
	return buf.String()
}

func formatWasmCodeList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderWasmCodeList(msg.(*types.QueryCodesResponse)))
	return err
}

// RenderWasmContractsByCode renders contracts for a code as a styled string.
func RenderWasmContractsByCode(res *types.QueryContractsByCodeResponse) string {
	var buf strings.Builder
	headers := []string{"#", "CONTRACT ADDRESS"}
	rows := make([][]string, 0, len(res.Contracts))
	for i, addr := range res.Contracts {
		rows = append(rows, []string{fmt.Sprintf("%d", i+1), addr})
	}
	WriteTableOrEmpty(&buf, headers, rows, "(no contracts)")
	return buf.String()
}

func formatWasmContractsByCode(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderWasmContractsByCode(msg.(*types.QueryContractsByCodeResponse)))
	return err
}

// RenderWasmCodeInfo renders WASM code info as a styled string.
func RenderWasmCodeInfo(res *types.QueryCodeInfoResponse) string {
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Code Info"))
	KV(&buf, "Code ID", Bold(fmt.Sprintf("%d", res.CodeID)))
	KV(&buf, "Creator", res.Creator)
	KV(&buf, "Checksum", res.Checksum.String())
	KV(&buf, "Instantiate", res.InstantiatePermission.Permission.String())
	if len(res.InstantiatePermission.Addresses) > 0 {
		for _, addr := range res.InstantiatePermission.Addresses {
			KV(&buf, "Address", addr)
		}
	}
	return buf.String()
}

func formatWasmCodeInfo(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderWasmCodeInfo(msg.(*types.QueryCodeInfoResponse)))
	return err
}

// RenderWasmContractInfo renders WASM contract info as a styled string.
func RenderWasmContractInfo(res *types.QueryContractInfoResponse) string {
	var buf strings.Builder
	fmt.Fprintln(&buf, Section("Contract"))
	KV(&buf, "Address", res.Address)
	KV(&buf, "Code ID", Bold(fmt.Sprintf("%d", res.CodeID)))
	KV(&buf, "Label", res.Label)
	KV(&buf, "Creator", res.Creator)
	if res.Admin != "" {
		KV(&buf, "Admin", res.Admin)
	}
	if res.IBCPortID != "" {
		KV(&buf, "IBC Port", res.IBCPortID)
	}
	return buf.String()
}

func formatWasmContractInfo(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderWasmContractInfo(msg.(*types.QueryContractInfoResponse)))
	return err
}

// RenderWasmContractHistory renders contract history as a styled string.
func RenderWasmContractHistory(res *types.QueryContractHistoryResponse) string {
	var buf strings.Builder
	cols := []ColDef{
		{Header: "OPERATION"},
		{Header: "CODE ID", Align: AlignRight},
		{Header: "UPDATED"},
	}
	rows := make([][]string, 0, len(res.Entries))
	for _, e := range res.Entries {
		updated := "-"
		if e.Updated != nil {
			updated = fmt.Sprintf("block %d", e.Updated.BlockHeight)
		}
		rows = append(rows, []string{e.Operation.String(), fmt.Sprintf("%d", e.CodeID), updated})
	}
	WriteTableColsOrEmpty(&buf, cols, rows, "(no history entries)")
	return buf.String()
}

func formatWasmContractHistory(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderWasmContractHistory(msg.(*types.QueryContractHistoryResponse)))
	return err
}

// RenderWasmPinnedCodes renders pinned codes as a styled string.
func RenderWasmPinnedCodes(res *types.QueryPinnedCodesResponse) string {
	var buf strings.Builder
	headers := []string{"CODE ID"}
	rows := make([][]string, 0, len(res.CodeIDs))
	for _, id := range res.CodeIDs {
		rows = append(rows, []string{Bold(fmt.Sprintf("%d", id))})
	}
	WriteTableOrEmpty(&buf, headers, rows, "(no pinned codes)")
	return buf.String()
}

func formatWasmPinnedCodes(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderWasmPinnedCodes(msg.(*types.QueryPinnedCodesResponse)))
	return err
}

// RenderWasmContractsByCreator renders contracts by creator as a styled string.
func RenderWasmContractsByCreator(res *types.QueryContractsByCreatorResponse) string {
	var buf strings.Builder
	headers := []string{"#", "CONTRACT ADDRESS"}
	rows := make([][]string, 0, len(res.ContractAddresses))
	for i, addr := range res.ContractAddresses {
		rows = append(rows, []string{fmt.Sprintf("%d", i+1), addr})
	}
	WriteTableOrEmpty(&buf, headers, rows, "(no contracts)")
	return buf.String()
}

func formatWasmContractsByCreator(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderWasmContractsByCreator(msg.(*types.QueryContractsByCreatorResponse)))
	return err
}
