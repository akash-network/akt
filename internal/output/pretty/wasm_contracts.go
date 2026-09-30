package pretty

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/output"
)

// RenderWasmContractList renders the metadata collected for a contract listing.
func RenderWasmContractList(contracts []*types.QueryContractInfoResponse) string {
	var buf strings.Builder
	cols := []ColDef{{Header: "ADDRESS"}, {Header: "LABEL"}, {Header: "CODE ID", Align: AlignRight}}
	rows := make([][]string, 0, len(contracts))
	for _, contract := range contracts {
		rows = append(rows, []string{contract.Address, contract.Label, strconv.FormatUint(contract.CodeID, 10)})
	}
	WriteTableColsOrEmpty(&buf, cols, rows, "(no contracts)")
	return buf.String()
}

// PrintWasmContractList writes a composite contract list in the selected format.
func PrintWasmContractList(cmd *cobra.Command, contracts []*types.QueryContractInfoResponse) error {
	format := output.FormatFromCmd(cmd)
	if format != output.FormatTable {
		type summary struct {
			Address string `json:"address" yaml:"address"`
			Label   string `json:"label" yaml:"label"`
			CodeID  string `json:"code_id" yaml:"code_id"`
		}
		rows := make([]summary, 0, len(contracts))
		for _, contract := range contracts {
			rows = append(rows, summary{Address: contract.Address, Label: contract.Label, CodeID: strconv.FormatUint(contract.CodeID, 10)})
		}
		return output.Fprint(cmd.OutOrStdout(), format, rows)
	}
	checked := output.NewCheckedTerminalWriter(cmd.OutOrStdout())
	_, err := fmt.Fprint(checked, RenderWasmContractList(contracts))
	return checked.Complete(err)
}
