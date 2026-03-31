package pretty

import (
	"fmt"
	"io"

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

func formatWasmCodeList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryCodesResponse)

	if len(res.CodeInfos) == 0 {
		fmt.Fprintln(w, Dim("(no codes)"))
		return nil
	}

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

		rows = append(rows, []string{
			Bold(fmt.Sprintf("%d", ci.CodeID)),
			ci.Creator,
			checksum,
		})
	}

	WriteTableCols(w, cols, rows)
	return nil
}

func formatWasmContractsByCode(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryContractsByCodeResponse)

	if len(res.Contracts) == 0 {
		fmt.Fprintln(w, Dim("(no contracts)"))
		return nil
	}

	headers := []string{"#", "CONTRACT ADDRESS"}
	rows := make([][]string, 0, len(res.Contracts))

	for i, addr := range res.Contracts {
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			addr,
		})
	}

	WriteTable(w, headers, rows)
	return nil
}

func formatWasmCodeInfo(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryCodeInfoResponse)

	fmt.Fprintln(w, Section("Code Info"))
	KV(w, "Code ID", Bold(fmt.Sprintf("%d", res.CodeID)))
	KV(w, "Creator", res.Creator)
	KV(w, "Checksum", res.Checksum.String())
	KV(w, "Instantiate", res.InstantiatePermission.Permission.String())
	if len(res.InstantiatePermission.Addresses) > 0 {
		for _, addr := range res.InstantiatePermission.Addresses {
			KV(w, "Address", addr)
		}
	}

	return nil
}

func formatWasmContractInfo(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryContractInfoResponse)

	fmt.Fprintln(w, Section("Contract"))
	KV(w, "Address", res.Address)
	KV(w, "Code ID", Bold(fmt.Sprintf("%d", res.CodeID)))
	KV(w, "Label", res.Label)
	KV(w, "Creator", res.Creator)
	if res.Admin != "" {
		KV(w, "Admin", res.Admin)
	}
	if res.IBCPortID != "" {
		KV(w, "IBC Port", res.IBCPortID)
	}

	return nil
}

func formatWasmContractHistory(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryContractHistoryResponse)

	if len(res.Entries) == 0 {
		fmt.Fprintln(w, Dim("(no history entries)"))
		return nil
	}

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

		rows = append(rows, []string{
			e.Operation.String(),
			fmt.Sprintf("%d", e.CodeID),
			updated,
		})
	}

	WriteTableCols(w, cols, rows)
	return nil
}

func formatWasmPinnedCodes(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryPinnedCodesResponse)

	if len(res.CodeIDs) == 0 {
		fmt.Fprintln(w, Dim("(no pinned codes)"))
		return nil
	}

	headers := []string{"CODE ID"}
	rows := make([][]string, 0, len(res.CodeIDs))
	for _, id := range res.CodeIDs {
		rows = append(rows, []string{Bold(fmt.Sprintf("%d", id))})
	}

	WriteTable(w, headers, rows)
	return nil
}

func formatWasmContractsByCreator(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*types.QueryContractsByCreatorResponse)

	if len(res.ContractAddresses) == 0 {
		fmt.Fprintln(w, Dim("(no contracts)"))
		return nil
	}

	headers := []string{"#", "CONTRACT ADDRESS"}
	rows := make([][]string, 0, len(res.ContractAddresses))

	for i, addr := range res.ContractAddresses {
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			addr,
		})
	}

	WriteTable(w, headers, rows)
	return nil
}
