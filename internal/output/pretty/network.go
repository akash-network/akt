package pretty

import (
	"fmt"
	"strings"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// RenderNetworkShow renders a network definition as a styled string.
// Used by both CLI "akt context network show" and TUI network detail views.
func RenderNetworkShow(net aktctx.Network, usedBy []string) string {
	var buf strings.Builder

	fmt.Fprintln(&buf, Section("Network"))
	KV(&buf, "Name", Bold(net.Name))
	KV(&buf, "Chain ID", net.ChainID)
	KV(&buf, "Gas Prices", net.GasPrices)
	KV(&buf, "Gas Adjustment", net.GasAdjustment)

	Newline(&buf)
	KVHeader(&buf, "RPC Endpoints")
	for _, e := range net.Endpoints.RPC {
		SubKV(&buf, "-", e)
	}

	if len(net.Endpoints.API) > 0 {
		KVHeader(&buf, "API Endpoints")
		for _, e := range net.Endpoints.API {
			SubKV(&buf, "-", e)
		}
	}

	if len(net.Endpoints.GRPC) > 0 {
		KVHeader(&buf, "gRPC Endpoints")
		for _, e := range net.Endpoints.GRPC {
			SubKV(&buf, "-", e)
		}
	}

	if len(usedBy) > 0 {
		Newline(&buf)
		KV(&buf, "Used by", strings.Join(usedBy, ", "))
	}

	return buf.String()
}

// RenderNetworkList renders a list of networks as a styled table string.
// Used by both CLI "akt context network list" and TUI network list views.
func RenderNetworkList(nets []aktctx.Network, getUsedBy func(name string) []string) string {
	var buf strings.Builder

	cols := []ColDef{
		{Header: "NAME"},
		{Header: "CHAIN-ID"},
		{Header: "RPC"},
		{Header: "USED BY"},
	}

	rows := make([][]string, 0, len(nets))
	for _, n := range nets {
		// Endpoints are printed in full: a truncated URL is not usable,
		// and the column widens to fit like every other column.
		rpcDisplay := ""
		if len(n.Endpoints.RPC) > 0 {
			rpcDisplay = n.Endpoints.RPC[0]
			if len(n.Endpoints.RPC) > 1 {
				rpcDisplay += fmt.Sprintf(" (+%d)", len(n.Endpoints.RPC)-1)
			}
		}

		usedBy := "(none)"
		if getUsedBy != nil {
			users := getUsedBy(n.Name)
			if len(users) > 0 {
				usedBy = strings.Join(users, ", ")
			}
		}

		rows = append(rows, []string{n.Name, n.ChainID, rpcDisplay, usedBy})
	}

	WriteTableColsOrEmpty(&buf, cols, rows, "(no networks)")
	return buf.String()
}
