package deployment

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	sdk "github.com/cosmos/cosmos-sdk/types"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/akt/internal/mcp/marshal"
)

// ToolListDeployments returns the tool definition for listing deployments.
func ToolListDeployments() mcp.Tool {
	return mcp.NewTool(
		"akash_list_deployments",
		mcp.WithDescription("List deployments on the Akash network with optional filters. Returns deployment details, groups, and escrow account info."),
		mcp.WithString("owner",
			mcp.Description("Filter by owner bech32 address. If omitted, uses the configured key's address."),
		),
		mcp.WithString("state",
			mcp.Enum("active", "closed"),
			mcp.Description("Filter by deployment state: 'active', 'closed'. If omitted, returns all states."),
		),
	)
}

// HandleListDeployments returns the handler for listing deployments.
func HandleListDeployments(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.AddressOrDefault(cl.ClientContext(), marshal.OptionalString(req, "owner"))
		if err != nil {
			return marshal.ErrResultf("failed to resolve owner address: %v", err), nil
		}

		if owner == "" {
			return marshal.ErrResult("owner address is required: pass owner, or configure a default account for the active context"), nil
		}

		filters := dtypes.DeploymentFilters{
			Owner: owner,
			State: marshal.OptionalString(req, "state"),
		}

		resp, err := cl.Query().Deployment().Deployments(ctx, &dtypes.QueryDeploymentsRequest{
			Filters: filters,
		})
		if err != nil {
			return marshal.ErrResultf("failed to list deployments: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// ToolGetDeployment returns the tool definition for getting a single deployment.
func ToolGetDeployment() mcp.Tool {
	return mcp.NewTool(
		"akash_get_deployment",
		mcp.WithDescription("Get details of a specific deployment by owner address and deployment sequence number (dseq)."),
		mcp.WithString("owner",
			mcp.Description("Owner bech32 address. If omitted, uses the configured key's address."),
		),
		mcp.WithNumber("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number."),
			marshal.PositiveInteger(),
		),
	)
}

// HandleGetDeployment returns the handler for getting a deployment.
func HandleGetDeployment(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.AddressOrDefault(cl.ClientContext(), marshal.OptionalString(req, "owner"))
		if err != nil {
			return marshal.ErrResultf("failed to resolve owner address: %v", err), nil
		}
		if owner == "" {
			return marshal.ErrResult("owner address is required"), nil
		}

		dseq, err := marshal.RequireUint64(req, "dseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		resp, err := cl.Query().Deployment().Deployment(ctx, &dtypes.QueryDeploymentRequest{
			ID: dv1.DeploymentID{
				Owner: owner,
				DSeq:  dseq,
			},
		})
		if err != nil {
			return marshal.ErrResultf("failed to get deployment: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// ToolGetGroup returns the tool definition for getting a deployment group.
func ToolGetGroup() mcp.Tool {
	return mcp.NewTool(
		"akash_get_group",
		mcp.WithDescription("Get details of a specific deployment group by owner, dseq, and gseq."),
		mcp.WithString("owner",
			mcp.Description("Owner bech32 address. If omitted, uses the configured key's address."),
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
	)
}

// HandleGetGroup returns the handler for getting a deployment group.
func HandleGetGroup(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.AddressOrDefault(cl.ClientContext(), marshal.OptionalString(req, "owner"))
		if err != nil {
			return marshal.ErrResultf("failed to resolve owner address: %v", err), nil
		}
		if owner == "" {
			return marshal.ErrResult("owner address is required"), nil
		}

		dseq, err := marshal.RequireUint64(req, "dseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		gseq, err := marshal.RequireUint32(req, "gseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		resp, err := cl.Query().Deployment().Group(ctx, &dtypes.QueryGroupRequest{
			ID: dv1.GroupID{
				Owner: owner,
				DSeq:  dseq,
				GSeq:  gseq,
			},
		})
		if err != nil {
			return marshal.ErrResultf("failed to get group: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// ToolCloseDeployment returns the tool definition for closing a deployment.
func ToolCloseDeployment() mcp.Tool {
	return mcp.NewTool(
		"akash_close_deployment",
		mcp.WithDescription("Close a deployment. This will terminate all services and release resources. Requires transaction signing capability."),
		mcp.WithNumber("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number to close."),
			marshal.PositiveInteger(),
		),
	)
}

// HandleCloseDeployment returns the handler for closing a deployment.
func HandleCloseDeployment(cl v1beta3.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dseq, err := marshal.RequireUint64(req, "dseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		owner, err := marshal.AddressOrDefault(cl.ClientContext(), "")
		if err != nil {
			return marshal.ErrResultf("failed to resolve owner address: %v", err), nil
		}
		if owner == "" {
			return marshal.ErrResult("owner address is required: configure a default account"), nil
		}

		msg := &dtypes.MsgCloseDeployment{
			ID: dv1.DeploymentID{
				Owner: owner,
				DSeq:  dseq,
			},
		}

		resp, err := cl.Tx().BroadcastMsgs(ctx, []sdk.Msg{msg})
		if err != nil {
			return marshal.ErrResultf("failed to close deployment: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}
