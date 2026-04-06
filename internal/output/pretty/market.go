package pretty

import (
	"fmt"
	"io"

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

func formatBidsList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*mvbeta.QueryBidsResponse)

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

	WriteTableCols(w, cols, rows)
	return nil
}

func formatBidDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*mvbeta.QueryBidResponse)
	bid := res.Bid

	fmt.Fprintln(w, Section("Bid"))
	KV(w, "Provider", bid.ID.Provider)
	KV(w, "Owner", bid.ID.Owner)
	KV(w, "DSeq", Bold(fmt.Sprintf("%d", bid.ID.DSeq)))
	KV(w, "GSeq", fmt.Sprintf("%d", bid.ID.GSeq))
	KV(w, "OSeq", fmt.Sprintf("%d", bid.ID.OSeq))
	KV(w, "Price/Block", Bold(FormatDecCoin(bid.Price)))
	KV(w, "State", ColorState(bid.State.String()))
	KV(w, "Created At", FormatHeight(bid.CreatedAt))

	if len(bid.ResourcesOffer) > 0 {
		Newline(w)
		fmt.Fprintln(w, Section("Resources Offer"))
		for i, r := range bid.ResourcesOffer {
			fmt.Fprintf(w, "  Resource %d:\n", i+1)
			KV(w, "  Count", fmt.Sprintf("%d", r.Count))
		}
	}

	return nil
}

func formatLeasesList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*mvbeta.QueryLeasesResponse)

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

	WriteTableCols(w, cols, rows)
	return nil
}

func formatLeaseDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*mvbeta.QueryLeaseResponse)
	lease := res.Lease

	fmt.Fprintln(w, Section("Lease"))
	KV(w, "Owner", lease.ID.Owner)
	KV(w, "DSeq", Bold(fmt.Sprintf("%d", lease.ID.DSeq)))
	KV(w, "GSeq", fmt.Sprintf("%d", lease.ID.GSeq))
	KV(w, "OSeq", fmt.Sprintf("%d", lease.ID.OSeq))
	KV(w, "Provider", lease.ID.Provider)
	KV(w, "Price/Block", Bold(FormatDecCoin(lease.Price)))
	KV(w, "State", ColorState(lease.State.String()))
	KV(w, "Created At", FormatHeight(lease.CreatedAt))

	if lease.ClosedOn > 0 {
		KV(w, "Closed On", FormatHeight(lease.ClosedOn))
		KV(w, "Closed Reason", lease.Reason.String())
	}

	return nil
}

func formatOrdersList(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*mvbeta.QueryOrdersResponse)

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

	WriteTableCols(w, cols, rows)
	return nil
}

func formatOrderDetail(w io.Writer, _ *cobra.Command, _ sdkclient.Context, msg proto.Message) error {
	res := msg.(*mvbeta.QueryOrderResponse)
	order := res.Order

	fmt.Fprintln(w, Section("Order"))
	KV(w, "Owner", order.ID.Owner)
	KV(w, "DSeq", Bold(fmt.Sprintf("%d", order.ID.DSeq)))
	KV(w, "GSeq", fmt.Sprintf("%d", order.ID.GSeq))
	KV(w, "OSeq", fmt.Sprintf("%d", order.ID.OSeq))
	KV(w, "State", ColorState(order.State.String()))
	KV(w, "Created At", FormatHeight(order.CreatedAt))

	if order.Spec.Name != "" {
		Newline(w)
		fmt.Fprintln(w, Section("Spec"))
		KV(w, "Name", order.Spec.Name)
	}

	return nil
}
