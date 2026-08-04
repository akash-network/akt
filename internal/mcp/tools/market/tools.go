package market

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	sdk "github.com/cosmos/cosmos-sdk/types"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/akt/internal/mcp/marshal"
)

// --- Order tools ---

// ToolListOrders returns the tool definition for listing orders.
func ToolListOrders() mcp.Tool {
	return mcp.NewTool(
		"akash_list_orders",
		mcp.WithDescription("List market orders on the Akash network with optional filters."),
		mcp.WithString("owner",
			mcp.Description("Filter by owner bech32 address."),
		),
		mcp.WithString("state",
			mcp.Description("Filter by order state: 'open', 'active', 'closed'."),
		),
	)
}

// HandleListOrders returns the handler for listing orders.
func HandleListOrders(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.AddressOrDefault(cl.ClientContext(), marshal.OptionalString(req, "owner"))
		if err != nil {
			return marshal.ErrResultf("failed to resolve owner address: %v", err), nil
		}

		if owner == "" {
			return marshal.ErrResult("owner address is required: pass owner, or configure a default account for the active context"), nil
		}

		resp, err := cl.Query().Market().Orders(ctx, &mtypes.QueryOrdersRequest{
			Filters: mtypes.OrderFilters{
				Owner: owner,
				State: marshal.OptionalString(req, "state"),
			},
		})
		if err != nil {
			return marshal.ErrResultf("failed to list orders: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// ToolGetOrder returns the tool definition for getting a single order.
func ToolGetOrder() mcp.Tool {
	return mcp.NewTool(
		"akash_get_order",
		mcp.WithDescription("Get details of a specific market order."),
		mcp.WithString("owner",
			mcp.Required(),
			mcp.Description("Owner bech32 address."),
		),
		mcp.WithNumber("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("gseq",
			mcp.Required(),
			mcp.Description("Group sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("oseq",
			mcp.Required(),
			mcp.Description("Order sequence number."),
			marshal.PositiveInteger(),
		),
	)
}

// HandleGetOrder returns the handler for getting an order.
func HandleGetOrder(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.RequireString(req, "owner")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		dseq, err := marshal.RequireUint64(req, "dseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		gseq, err := marshal.RequireUint32(req, "gseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		oseq, err := marshal.RequireUint32(req, "oseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		resp, err := cl.Query().Market().Order(ctx, &mtypes.QueryOrderRequest{
			ID: mv1.OrderID{
				Owner: owner,
				DSeq:  dseq,
				GSeq:  gseq,
				OSeq:  oseq,
			},
		})
		if err != nil {
			return marshal.ErrResultf("failed to get order: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// --- Bid tools ---

// ToolListBids returns the tool definition for listing bids.
func ToolListBids() mcp.Tool {
	return mcp.NewTool(
		"akash_list_bids",
		mcp.WithDescription("List bids on the Akash market with optional filters."),
		mcp.WithString("owner",
			mcp.Description("Filter by deployment owner bech32 address."),
		),
		mcp.WithNumber("dseq",
			mcp.Description("Filter by deployment sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithString("state",
			mcp.Description("Filter by bid state: 'open', 'active', 'lost', 'closed'."),
		),
	)
}

// HandleListBids returns the handler for listing bids.
func HandleListBids(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.AddressOrDefault(cl.ClientContext(), marshal.OptionalString(req, "owner"))
		if err != nil {
			return marshal.ErrResultf("failed to resolve owner address: %v", err), nil
		}

		if owner == "" {
			return marshal.ErrResult("owner address is required: pass owner, or configure a default account for the active context"), nil
		}

		filters := mtypes.BidFilters{
			Owner: owner,
			State: marshal.OptionalString(req, "state"),
		}

		if dseq, ok, err := marshal.OptionalUint64(req, "dseq"); err != nil {
			return marshal.ErrResult(err.Error()), nil
		} else if ok {
			filters.DSeq = dseq
		}

		resp, err := cl.Query().Market().Bids(ctx, &mtypes.QueryBidsRequest{
			Filters: filters,
		})
		if err != nil {
			return marshal.ErrResultf("failed to list bids: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// ToolGetBid returns the tool definition for getting a single bid.
func ToolGetBid() mcp.Tool {
	return mcp.NewTool(
		"akash_get_bid",
		mcp.WithDescription("Get details of a specific bid."),
		mcp.WithString("owner",
			mcp.Required(),
			mcp.Description("Deployment owner bech32 address."),
		),
		mcp.WithNumber("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("gseq",
			mcp.Required(),
			mcp.Description("Group sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("oseq",
			mcp.Required(),
			mcp.Description("Order sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithString("provider",
			mcp.Required(),
			mcp.Description("Provider bech32 address."),
		),
	)
}

// HandleGetBid returns the handler for getting a bid.
func HandleGetBid(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.RequireString(req, "owner")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		dseq, err := marshal.RequireUint64(req, "dseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		gseq, err := marshal.RequireUint32(req, "gseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		oseq, err := marshal.RequireUint32(req, "oseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		provider, err := marshal.RequireString(req, "provider")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		resp, err := cl.Query().Market().Bid(ctx, &mtypes.QueryBidRequest{
			ID: mv1.BidID{
				Owner:    owner,
				DSeq:     dseq,
				GSeq:     gseq,
				OSeq:     oseq,
				Provider: provider,
			},
		})
		if err != nil {
			return marshal.ErrResultf("failed to get bid: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// --- Lease tools ---

// ToolListLeases returns the tool definition for listing leases.
func ToolListLeases() mcp.Tool {
	return mcp.NewTool(
		"akash_list_leases",
		mcp.WithDescription("List leases on the Akash market with optional filters."),
		mcp.WithString("owner",
			mcp.Description("Filter by deployment owner bech32 address."),
		),
		mcp.WithNumber("dseq",
			mcp.Description("Filter by deployment sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithString("state",
			mcp.Description("Filter by lease state: 'active', 'closed', 'insufficient_funds'."),
		),
		mcp.WithString("provider",
			mcp.Description("Filter by provider bech32 address."),
		),
	)
}

// HandleListLeases returns the handler for listing leases.
func HandleListLeases(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.AddressOrDefault(cl.ClientContext(), marshal.OptionalString(req, "owner"))
		if err != nil {
			return marshal.ErrResultf("failed to resolve owner address: %v", err), nil
		}

		if owner == "" {
			return marshal.ErrResult("owner address is required: pass owner, or configure a default account for the active context"), nil
		}

		filters := mv1.LeaseFilters{
			Owner:    owner,
			State:    marshal.OptionalString(req, "state"),
			Provider: marshal.OptionalString(req, "provider"),
		}

		if dseq, ok, err := marshal.OptionalUint64(req, "dseq"); err != nil {
			return marshal.ErrResult(err.Error()), nil
		} else if ok {
			filters.DSeq = dseq
		}

		resp, err := cl.Query().Market().Leases(ctx, &mtypes.QueryLeasesRequest{
			Filters: filters,
		})
		if err != nil {
			return marshal.ErrResultf("failed to list leases: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// ToolGetLease returns the tool definition for getting a single lease.
func ToolGetLease() mcp.Tool {
	return mcp.NewTool(
		"akash_get_lease",
		mcp.WithDescription("Get details of a specific lease."),
		mcp.WithString("owner",
			mcp.Required(),
			mcp.Description("Deployment owner bech32 address."),
		),
		mcp.WithNumber("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("gseq",
			mcp.Required(),
			mcp.Description("Group sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("oseq",
			mcp.Required(),
			mcp.Description("Order sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithString("provider",
			mcp.Required(),
			mcp.Description("Provider bech32 address."),
		),
	)
}

// HandleGetLease returns the handler for getting a lease.
func HandleGetLease(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.RequireString(req, "owner")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		dseq, err := marshal.RequireUint64(req, "dseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		gseq, err := marshal.RequireUint32(req, "gseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		oseq, err := marshal.RequireUint32(req, "oseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		provider, err := marshal.RequireString(req, "provider")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		resp, err := cl.Query().Market().Lease(ctx, &mtypes.QueryLeaseRequest{
			ID: mv1.LeaseID{
				Owner:    owner,
				DSeq:     dseq,
				GSeq:     gseq,
				OSeq:     oseq,
				Provider: provider,
			},
		})
		if err != nil {
			return marshal.ErrResultf("failed to get lease: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// --- Lease TX tools ---

// ToolCreateLease returns the tool definition for creating a lease.
func ToolCreateLease() mcp.Tool {
	return mcp.NewTool(
		"akash_create_lease",
		mcp.WithDescription("Create a lease from a bid. This accepts a bid and creates a lease between the deployment owner and the provider."),
		mcp.WithString("owner",
			mcp.Required(),
			mcp.Description("Deployment owner bech32 address."),
		),
		mcp.WithNumber("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("gseq",
			mcp.Required(),
			mcp.Description("Group sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("oseq",
			mcp.Required(),
			mcp.Description("Order sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithString("provider",
			mcp.Required(),
			mcp.Description("Provider bech32 address."),
		),
	)
}

// HandleCreateLease returns the handler for creating a lease.
func HandleCreateLease(cl v1beta3.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.RequireString(req, "owner")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		dseq, err := marshal.RequireUint64(req, "dseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		gseq, err := marshal.RequireUint32(req, "gseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		oseq, err := marshal.RequireUint32(req, "oseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		provider, err := marshal.RequireString(req, "provider")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		msg := &mtypes.MsgCreateLease{
			BidID: mv1.BidID{
				Owner:    owner,
				DSeq:     dseq,
				GSeq:     gseq,
				OSeq:     oseq,
				Provider: provider,
			},
		}

		resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
		if err != nil {
			return marshal.ErrResultf("failed to create lease: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// ToolCloseLease returns the tool definition for closing a lease.
func ToolCloseLease() mcp.Tool {
	return mcp.NewTool(
		"akash_close_lease",
		mcp.WithDescription("Close an active lease. This terminates the lease between the deployment owner and the provider."),
		mcp.WithString("owner",
			mcp.Required(),
			mcp.Description("Deployment owner bech32 address."),
		),
		mcp.WithNumber("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("gseq",
			mcp.Required(),
			mcp.Description("Group sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithNumber("oseq",
			mcp.Required(),
			mcp.Description("Order sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithString("provider",
			mcp.Required(),
			mcp.Description("Provider bech32 address."),
		),
	)
}

// HandleCloseLease returns the handler for closing a lease.
func HandleCloseLease(cl v1beta3.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.RequireString(req, "owner")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		dseq, err := marshal.RequireUint64(req, "dseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		gseq, err := marshal.RequireUint32(req, "gseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		oseq, err := marshal.RequireUint32(req, "oseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		provider, err := marshal.RequireString(req, "provider")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		msg := &mtypes.MsgCloseLease{
			ID: mv1.LeaseID{
				Owner:    owner,
				DSeq:     dseq,
				GSeq:     gseq,
				OSeq:     oseq,
				Provider: provider,
			},
		}

		resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
		if err != nil {
			return marshal.ErrResultf("failed to close lease: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}
