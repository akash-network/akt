package pretty

import (
	"fmt"
	"io"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	types "pkg.akt.dev/go/node/oracle/v2"
)

func init() {
	Register((*types.QueryPricesResponse)(nil), PrettyFormatterFunc(formatOraclePrices))
	Register((*types.QueryAggregatedPriceResponse)(nil), PrettyFormatterFunc(formatAggregatedPrice))
}

// RenderOraclePrices renders an oracle prices table as a string.
// Used by both CLI pretty output and TUI monitor dashboard.
func RenderOraclePrices(res *types.QueryPricesResponse) string {
	var buf strings.Builder

	if len(res.Prices) == 0 {
		fmt.Fprintln(&buf, Dim("(no prices)"))
		return buf.String()
	}

	cols := []ColDef{
		{Header: "ASSET"},
		{Header: "BASE"},
		{Header: "PRICE", Align: AlignRight},
		{Header: "SOURCE", Align: AlignRight},
		{Header: "TIMESTAMP"},
	}

	rows := make([][]string, 0, len(res.Prices))
	for _, p := range res.Prices {
		price := TrimDecTrailingZeros(p.State.Price.String())
		rows = append(rows, []string{
			p.ID.Denom,
			p.ID.BaseDenom,
			Bold(price),
			fmt.Sprintf("%d", p.ID.Source),
			p.ID.Timestamp.Format("2006-01-02 15:04:05"),
		})
	}

	WriteTableCols(&buf, cols, rows)
	return buf.String()
}

func formatOraclePrices(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderOraclePrices(msg.(*types.QueryPricesResponse)))
	return err
}

// RenderAggregatedPrice renders an aggregated price section as a string.
// Used by both CLI pretty output and TUI monitor dashboard.
func RenderAggregatedPrice(res *types.QueryAggregatedPriceResponse) string {
	var buf strings.Builder

	ap := res.AggregatedPrice
	ph := res.PriceHealth

	fmt.Fprintln(&buf, Section("Aggregated Price"))
	KV(&buf, "Denom", Bold(ap.Denom))
	KV(&buf, "TWAP", Bold(TrimDecTrailingZeros(ap.TWAP.String())))
	KV(&buf, "Median", TrimDecTrailingZeros(ap.MedianPrice.String()))
	KV(&buf, "Min", TrimDecTrailingZeros(ap.MinPrice.String()))
	KV(&buf, "Max", TrimDecTrailingZeros(ap.MaxPrice.String()))
	KV(&buf, "Sources", fmt.Sprintf("%d", ap.NumSources))
	KV(&buf, "Deviation", fmt.Sprintf("%d bps", ap.DeviationBps))
	KV(&buf, "Timestamp", ap.Timestamp.Format("2006-01-02 15:04:05 UTC"))

	Newline(&buf)
	fmt.Fprintln(&buf, Section("Price Health"))
	healthy := "yes"
	if !ph.IsHealthy {
		healthy = StyleRed.Render("no")
		if len(ph.FailureReason) > 0 {
			healthy += " (" + strings.Join(ph.FailureReason, "; ") + ")"
		}
	} else {
		healthy = StyleGreen.Render("yes")
	}
	KV(&buf, "Healthy", healthy)
	KV(&buf, "Min Sources", boolCheck(ph.HasMinSources))
	KV(&buf, "Deviation OK", boolCheck(ph.DeviationOk))
	KV(&buf, "Total Sources", fmt.Sprintf("%d", ph.TotalSources))
	KV(&buf, "Healthy Sources", fmt.Sprintf("%d", ph.TotalHealthySources))

	return buf.String()
}

func formatAggregatedPrice(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderAggregatedPrice(msg.(*types.QueryAggregatedPriceResponse)))
	return err
}

func boolCheck(v bool) string {
	if v {
		return StyleGreen.Render("yes")
	}
	return StyleRed.Render("no")
}
