package bank

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"

	"pkg.akt.dev/akt/internal/mcp/marshal"
)

// ToolAccountBalance returns the MCP tool definition for querying account balances.
func ToolAccountBalance() mcp.Tool {
	return mcp.NewTool(
		"akash_account_balance",
		mcp.WithDescription("Get the token balances for an Akash account address. If no address is provided, uses the configured key's address."),
		mcp.WithString("address",
			mcp.Description("Bech32 account address (e.g. akash1...). If omitted, uses the configured key's address."),
		),
		mcp.WithString("denom",
			mcp.Description("Optional denomination to query a specific token balance (e.g. 'uakt'). If omitted, returns all balances."),
		),
	)
}

// HandleAccountBalance returns the handler for the account_balance tool.
func HandleAccountBalance(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr, err := marshal.AddressOrDefault(cl.ClientContext(), marshal.OptionalString(req, "address"))
		if err != nil {
			return marshal.ErrResultf("failed to resolve account address: %v", err), nil
		}
		if addr == "" {
			return marshal.ErrResult("no address provided and no default account configured"), nil
		}

		denom := marshal.OptionalString(req, "denom")

		if denom != "" {
			resp, err := cl.Query().Bank().Balance(ctx, &banktypes.QueryBalanceRequest{
				Address: addr,
				Denom:   denom,
			})
			if err != nil {
				return marshal.ErrResultf("failed to query balance: %v", err), nil
			}
			return marshal.ToTextResult(resp)
		}

		resp, err := cl.Query().Bank().AllBalances(ctx, &banktypes.QueryAllBalancesRequest{
			Address: addr,
		})
		if err != nil {
			return marshal.ErrResultf("failed to query balances: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}
