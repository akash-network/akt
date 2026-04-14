package node

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"

	"pkg.akt.dev/akt/internal/mcp/marshal"
)

// ToolNodeStatus returns the MCP tool definition for querying node sync status.
func ToolNodeStatus() mcp.Tool {
	return mcp.NewTool(
		"akash_node_status",
		mcp.WithDescription("Get the sync status of the connected Akash node, including latest block height, block hash, and whether the node is catching up."),
	)
}

// HandleNodeStatus returns the handler for the node_status tool.
func HandleNodeStatus(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info, err := cl.Node().SyncInfo(ctx)
		if err != nil {
			return marshal.ErrResultf("failed to get sync info: %v", err), nil
		}
		return marshal.ToTextResult(info)
	}
}

// ToolBlockHeight returns the MCP tool definition for querying the current block height.
func ToolBlockHeight() mcp.Tool {
	return mcp.NewTool(
		"akash_block_height",
		mcp.WithDescription("Get the current block height of the connected Akash node."),
	)
}

// HandleBlockHeight returns the handler for the block_height tool.
func HandleBlockHeight(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		height, err := cl.Node().CurrentBlockHeight(ctx)
		if err != nil {
			return marshal.ErrResultf("failed to get block height: %v", err), nil
		}
		return marshal.ToTextResult(map[string]int64{"block_height": height})
	}
}
