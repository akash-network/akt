package pretty

import (
	"fmt"
	"io"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cobra"

	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
)

func init() {
	Register((*mvbeta.QueryBidsResponse)(nil), PrettyFormatterFunc(formatBidsList))
	Register((*mvbeta.QueryBidResponse)(nil), PrettyFormatterFunc(formatBidDetail))
	Register((*mvbeta.QueryLeasesResponse)(nil), PrettyFormatterFunc(formatLeasesList))
	Register((*mvbeta.QueryLeaseResponse)(nil), PrettyFormatterFunc(formatLeaseDetail))
	Register((*mvbeta.QueryOrdersResponse)(nil), PrettyFormatterFunc(formatOrdersList))
	Register((*mvbeta.QueryOrderResponse)(nil), PrettyFormatterFunc(formatOrderDetail))
}

// RenderBidList renders a bids list as a styled string.
func RenderBidList(res *mvbeta.QueryBidsResponse) string {
	var buf strings.Builder
	cols := []ColDef{
		{Header: "ID"},
		{Header: "PRICE/BLOCK", Align: AlignRight},
		{Header: "STATE"},
	}
	rows := make([][]string, 0, len(res.Bids))
	for _, b := range res.Bids {
		bid := b.Bid
		rows = append(rows, []string{
			bid.ID.String(),
			Bold(FormatDecCoin(bid.Price)),
			ColorState(bid.State.String()),
		})
	}
	WriteTableColsOrEmpty(&buf, cols, rows, "(no bids)")
	return buf.String()
}

func formatBidsList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderBidList(msg.(*mvbeta.QueryBidsResponse)))
	return err
}

// RenderBidDetail renders a bid detail as a styled string.
func RenderBidDetail(res *mvbeta.QueryBidResponse) string {
	var buf strings.Builder
	bid := res.Bid
	fmt.Fprintln(&buf, Section("Bid"))
	KV(&buf, "Provider", bid.ID.Provider)
	KV(&buf, "Owner", bid.ID.Owner)
	KV(&buf, "DSeq", Bold(fmt.Sprintf("%d", bid.ID.DSeq)))
	KV(&buf, "GSeq", fmt.Sprintf("%d", bid.ID.GSeq))
	KV(&buf, "OSeq", fmt.Sprintf("%d", bid.ID.OSeq))
	KV(&buf, "Price/Block", Bold(FormatDecCoin(bid.Price)))
	KV(&buf, "State", ColorState(bid.State.String()))
	KV(&buf, "Created At", FormatHeight(bid.CreatedAt))
	if len(bid.ResourcesOffer) > 0 {
		Newline(&buf)
		fmt.Fprintln(&buf, Section("Resources Offer"))
		for i, r := range bid.ResourcesOffer {
			fmt.Fprintf(&buf, "  Resource %d:\n", i+1)
			KV(&buf, "  Count", fmt.Sprintf("%d", r.Count))
		}
	}
	return buf.String()
}

func formatBidDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderBidDetail(msg.(*mvbeta.QueryBidResponse)))
	return err
}

// RenderLeaseList renders a leases list as a styled string.
func RenderLeaseList(res *mvbeta.QueryLeasesResponse) string {
	var buf strings.Builder
	cols := []ColDef{
		{Header: "ID"},
		{Header: "PRICE/BLOCK", Align: AlignRight},
		{Header: "STATE"},
	}
	rows := make([][]string, 0, len(res.Leases))
	for _, l := range res.Leases {
		lease := l.Lease
		rows = append(rows, []string{
			lease.ID.String(),
			FormatDecCoin(lease.Price),
			ColorState(lease.State.String()),
		})
	}
	WriteTableColsOrEmpty(&buf, cols, rows, "(no leases)")
	return buf.String()
}

func formatLeasesList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderLeaseList(msg.(*mvbeta.QueryLeasesResponse)))
	return err
}

// RenderLeaseDetail renders a lease detail as a styled string.
func RenderLeaseDetail(res *mvbeta.QueryLeaseResponse) string {
	var buf strings.Builder
	lease := res.Lease
	fmt.Fprintln(&buf, Section("Lease"))
	KV(&buf, "Owner", lease.ID.Owner)
	KV(&buf, "DSeq", Bold(fmt.Sprintf("%d", lease.ID.DSeq)))
	KV(&buf, "GSeq", fmt.Sprintf("%d", lease.ID.GSeq))
	KV(&buf, "OSeq", fmt.Sprintf("%d", lease.ID.OSeq))
	KV(&buf, "Provider", lease.ID.Provider)
	KV(&buf, "Price/Block", Bold(FormatDecCoin(lease.Price)))
	KV(&buf, "State", ColorState(lease.State.String()))
	KV(&buf, "Created At", FormatHeight(lease.CreatedAt))
	if lease.ClosedOn > 0 {
		KV(&buf, "Closed On", FormatHeight(lease.ClosedOn))
		KV(&buf, "Closed Reason", lease.Reason.String())
	}
	return buf.String()
}

func formatLeaseDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderLeaseDetail(msg.(*mvbeta.QueryLeaseResponse)))
	return err
}

// RenderOrderList renders an orders list as a styled string.
func RenderOrderList(res *mvbeta.QueryOrdersResponse) string {
	var buf strings.Builder
	cols := []ColDef{
		{Header: "ID"},
		{Header: "STATE"},
		{Header: "CREATED AT", Align: AlignRight},
	}
	rows := make([][]string, 0, len(res.Orders))
	for _, order := range res.Orders {
		rows = append(rows, []string{
			order.ID.String(),
			ColorState(order.State.String()),
			FormatHeight(order.CreatedAt),
		})
	}
	WriteTableColsOrEmpty(&buf, cols, rows, "(no orders)")
	return buf.String()
}

func formatOrdersList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderOrderList(msg.(*mvbeta.QueryOrdersResponse)))
	return err
}

// RenderOrderDetail renders an order detail as a styled string.
func RenderOrderDetail(res *mvbeta.QueryOrderResponse) string {
	var buf strings.Builder
	order := res.Order
	fmt.Fprintln(&buf, Section("Order"))
	KV(&buf, "Owner", order.ID.Owner)
	KV(&buf, "DSeq", Bold(fmt.Sprintf("%d", order.ID.DSeq)))
	KV(&buf, "GSeq", fmt.Sprintf("%d", order.ID.GSeq))
	KV(&buf, "OSeq", fmt.Sprintf("%d", order.ID.OSeq))
	KV(&buf, "State", ColorState(order.State.String()))
	KV(&buf, "Created At", FormatHeight(order.CreatedAt))
	if order.Spec.Name != "" {
		Newline(&buf)
		fmt.Fprintln(&buf, Section("Spec"))
		KV(&buf, "Name", order.Spec.Name)
	}
	return buf.String()
}

func formatOrderDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	_, err := fmt.Fprint(w, RenderOrderDetail(msg.(*mvbeta.QueryOrderResponse)))
	return err
}
