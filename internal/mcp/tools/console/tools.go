// Package console exposes the Console API as MCP tools, so an assistant
// working against a managed (console-api) context reaches the same rail the
// `akt console` commands use rather than the chain directly.
package console

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"pkg.akt.dev/akt/internal/console"
	"pkg.akt.dev/akt/internal/mcp/marshal"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// ToolListDeployments returns the tool definition for listing deployments.
func ToolListDeployments() mcp.Tool {
	return mcp.NewTool(
		"console_list_deployments",
		mcp.WithDescription("List the Console-managed deployments belonging to the configured API key."),
		mcp.WithNumber("skip",
			mcp.Description("Number of deployments to skip, for paging. Defaults to 0."),
			marshal.NonNegativeInteger(),
		),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum deployments to return. Defaults to %d, capped at %d.", defaultListLimit, maxListLimit)),
			marshal.NonNegativeInteger(),
		),
	)
}

// HandleListDeployments returns the handler for console_list_deployments.
func HandleListDeployments(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		skip := 0
		if v, ok, err := marshal.OptionalUint64(req, "skip"); err != nil {
			return marshal.ErrResult(err.Error()), nil
		} else if ok {
			if v > uint64(^uint(0)>>1) {
				return marshal.ErrResult("parameter skip is out of range for this platform"), nil
			}
			skip = int(v)
		}

		limit := defaultListLimit
		if v, ok, err := marshal.OptionalUint64(req, "limit"); err != nil {
			return marshal.ErrResult(err.Error()), nil
		} else if ok && v > 0 {
			if v > maxListLimit {
				limit = maxListLimit
			} else {
				limit = int(v)
			}
		}
		// An assistant asking for everything should not be able to turn one
		// tool call into an unbounded page request.
		if limit > maxListLimit {
			limit = maxListLimit
		}

		resp, err := cl.ListDeployments(ctx, skip, limit)
		if err != nil {
			return marshal.ErrResultf("failed to list deployments: %v", err), nil
		}

		return marshal.ToTextResult(resp)
	}
}

// ToolGetDeployment returns the tool definition for fetching one deployment.
func ToolGetDeployment() mcp.Tool {
	return mcp.NewTool(
		"console_get_deployment",
		mcp.WithDescription("Get a Console-managed deployment by its deployment sequence number (dseq), including its leases."),
		mcp.WithString("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number, e.g. 1784752234037."),
		),
	)
}

// HandleGetDeployment returns the handler for console_get_deployment.
func HandleGetDeployment(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dseq, err := marshal.RequireString(req, "dseq")
		if err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		resp, err := cl.GetDeployment(ctx, dseq)
		if err != nil {
			return marshal.ErrResultf("failed to get deployment %s: %v", dseq, err), nil
		}

		return marshal.ToTextResult(resp)
	}
}

// ToolListBids returns the tool definition for listing bids on a deployment.
func ToolListBids() mcp.Tool {
	return mcp.NewTool(
		"console_list_bids",
		mcp.WithDescription("List the provider bids received for a Console-managed deployment. Bids may take a few seconds to arrive after a deployment is created."),
		mcp.WithString("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number to fetch bids for."),
		),
	)
}

// HandleListBids returns the handler for console_list_bids.
func HandleListBids(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dseq, err := marshal.RequireString(req, "dseq")
		if err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		bids, err := cl.FetchBids(ctx, dseq)
		if err != nil {
			return marshal.ErrResultf("failed to fetch bids for %s: %v", dseq, err), nil
		}

		// An empty list is only worth waiting on if the deployment exists.
		// Returning [] for a dseq that does not exist, next to a description
		// saying bids take a few seconds to arrive, tells an assistant to poll
		// forever.
		if len(bids) == 0 {
			if _, err := cl.GetDeployment(ctx, dseq); err != nil {
				return marshal.ErrResultf("no bids for %s: %v", dseq, err), nil
			}
		}

		return marshal.ToTextResult(bids)
	}
}

// ToolWalletBalance returns the tool definition for the managed wallet balance.
func ToolWalletBalance() mcp.Tool {
	return mcp.NewTool(
		"console_wallet_balance",
		mcp.WithDescription("Get the Console-managed wallet's available, in-deployment, and total balances in USD."),
	)
}

// HandleWalletBalance returns the handler for console_wallet_balance.
func HandleWalletBalance(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := cl.GetBalances(ctx)
		if err != nil {
			return marshal.ErrResultf("failed to get wallet balance: %v", err), nil
		}

		return marshal.ToTextResult(struct {
			AvailableUSD     float64 `json:"available_usd"`
			InDeploymentsUSD float64 `json:"in_deployments_usd"`
			TotalUSD         float64 `json:"total_usd"`
		}{
			AvailableUSD:     resp.BalanceUSD(),
			InDeploymentsUSD: resp.DeploymentsUSD(),
			TotalUSD:         resp.TotalUSD(),
		})
	}
}

// ToolUsageHistory returns the tool definition for spend history.
func ToolUsageHistory() mcp.Tool {
	return mcp.NewTool(
		"console_usage_history",
		mcp.WithDescription("Get spend history for the Console account. Both dates are optional; omit them for the account's full history."),
		mcp.WithString("start_date",
			mcp.Description("Start of the range as YYYY-MM-DD. Omit for no lower bound."),
		),
		mcp.WithString("end_date",
			mcp.Description("End of the range as YYYY-MM-DD. Omit for no upper bound."),
		),
	)
}

// HandleUsageHistory returns the handler for console_usage_history.
func HandleUsageHistory(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		startDate := marshal.OptionalString(req, "start_date")
		endDate := marshal.OptionalString(req, "end_date")
		for _, date := range []struct {
			name  string
			value string
		}{
			{name: "start_date", value: startDate},
			{name: "end_date", value: endDate},
		} {
			if date.value == "" {
				continue
			}
			if _, err := time.Parse("2006-01-02", date.value); err != nil {
				return marshal.ErrResultf("parameter %s must use YYYY-MM-DD: %v", date.name, err), nil
			}
		}

		// The endpoint needs the managed wallet's on-chain address; it is not
		// derived from the API key, and sending an empty one is rejected. Look
		// it up the same way `akt console usage` does rather than making the
		// caller supply an address they have no way to know.
		user, err := cl.GetUser(ctx)
		if err != nil {
			return marshal.ErrResultf("get user: %v", err), nil
		}

		wallets, err := cl.ListWallets(ctx, user.ID)
		if err != nil {
			return marshal.ErrResultf("list wallets: %v", err), nil
		}

		address := ""
		for _, w := range wallets {
			if w.Address != "" {
				address = w.Address
				break
			}
		}

		if address == "" {
			return marshal.ErrResult("no managed wallet with an on-chain address was found"), nil
		}

		resp, err := cl.GetUsageHistory(ctx, address, startDate, endDate)
		if err != nil {
			return marshal.ErrResultf("failed to get usage history: %v", err), nil
		}

		return marshal.ToTextResult(resp)
	}
}

// ToolListProviders returns the tool definition for listing providers.
func ToolListProviders() mcp.Tool {
	return mcp.NewTool(
		"console_list_providers",
		mcp.WithDescription("List providers known to Console, with their capacity and pricing."),
		mcp.WithString("scope",
			// The only values the API accepts. The schema used to suggest
			// 'active', which it rejects with a 400 on every call, so an
			// assistant following the documentation failed on its first try.
			mcp.Enum("all", "trial"),
			mcp.Description("Optional listing scope. Omit for the default set."),
		),
		mcp.WithString("addresses",
			mcp.Description("Optional comma-separated provider bech32 addresses to restrict the listing to."),
		),
		mcp.WithNumber("limit",
			mcp.Description(fmt.Sprintf("Maximum providers to return. Defaults to %d, capped at %d.", defaultListLimit, maxListLimit)),
			marshal.NonNegativeInteger(),
		),
	)
}

// HandleListProviders returns the handler for console_list_providers.
func HandleListProviders(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var addresses []string
		if raw := marshal.OptionalString(req, "addresses"); raw != "" {
			for _, a := range strings.Split(raw, ",") {
				if a = strings.TrimSpace(a); a != "" {
					addresses = append(addresses, a)
				}
			}
		}

		resp, err := cl.ListProviders(ctx, marshal.OptionalString(req, "scope"), addresses)
		if err != nil {
			return marshal.ErrResultf("failed to list providers: %v", err), nil
		}

		// The endpoint has no server-side paging, and the full catalogue is
		// ~1800 providers -- half a megabyte, enough to evict the conversation
		// that asked for it. console_list_deployments already caps itself for
		// this reason; the same reasoning applies here and was not carried
		// over. Truncation is reported so a short list is never mistaken for
		// the whole catalogue.
		limit := defaultListLimit
		if v, ok, err := marshal.OptionalUint64(req, "limit"); err != nil {
			return marshal.ErrResult(err.Error()), nil
		} else if ok && v > 0 {
			limit = int(min(v, uint64(maxListLimit)))
		}

		if len(resp) > limit {
			return marshal.ToTextResult(map[string]any{
				"providers": resp[:limit],
				"truncated": true,
				"returned":  limit,
				"total":     len(resp),
				"note":      "raise `limit` or filter with `addresses` to see more",
			})
		}

		return marshal.ToTextResult(resp)
	}
}

// ToolGetProvider returns the tool definition for one provider.
func ToolGetProvider() mcp.Tool {
	return mcp.NewTool(
		"console_get_provider",
		mcp.WithDescription("Get a single provider's detail from Console by its bech32 address."),
		mcp.WithString("address",
			mcp.Required(),
			mcp.Description("Provider bech32 address, e.g. akash1..."),
		),
	)
}

// HandleGetProvider returns the handler for console_get_provider.
func HandleGetProvider(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		addr, err := marshal.RequireString(req, "address")
		if err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		resp, err := cl.GetProvider(ctx, addr)
		if err != nil {
			return marshal.ErrResultf("failed to get provider %s: %v", addr, err), nil
		}

		return marshal.ToTextResult(resp)
	}
}

// ToolGPUPrices returns the tool definition for GPU availability and pricing.
func ToolGPUPrices() mcp.Tool {
	return mcp.NewTool(
		"console_gpu_prices",
		mcp.WithDescription("Get GPU model availability and pricing across the network, for sizing a deployment before creating it."),
	)
}

// HandleGPUPrices returns the handler for console_gpu_prices.
func HandleGPUPrices(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := cl.GetGPUPrices(ctx)
		if err != nil {
			return marshal.ErrResultf("failed to get GPU prices: %v", err), nil
		}

		return marshal.ToTextResult(resp)
	}
}

// ToolCloseDeployment returns the tool definition for closing a deployment.
func ToolCloseDeployment() mcp.Tool {
	return mcp.NewTool(
		"console_close_deployment",
		mcp.WithDescription("Close a Console-managed deployment. This terminates its leases and stops the workload; remaining escrow is returned to the wallet. This cannot be undone."),
		mcp.WithString("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number to close."),
		),
	)
}

// HandleCloseDeployment returns the handler for console_close_deployment.
func HandleCloseDeployment(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dseq, err := marshal.RequireString(req, "dseq")
		if err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		if err := cl.CloseDeployment(ctx, dseq); err != nil {
			return marshal.ErrResultf("failed to close deployment %s: %v", dseq, err), nil
		}

		return marshal.ToTextResult(map[string]string{
			"dseq":   dseq,
			"status": "closed",
		})
	}
}

// ToolDeposit returns the tool definition for topping up a deployment.
func ToolDeposit() mcp.Tool {
	return mcp.NewTool(
		"console_deposit",
		mcp.WithDescription("Add funds to a Console-managed deployment's escrow, extending how long it runs. Spends real credits from the managed wallet."),
		mcp.WithString("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number to deposit into."),
		),
		mcp.WithNumber("amount_usd",
			mcp.Required(),
			mcp.Min(console.MinDepositUSD),
			mcp.Description(fmt.Sprintf("Amount to deposit in USD. Must be at least %.2f.", console.MinDepositUSD)),
		),
	)
}

// HandleDeposit returns the handler for console_deposit.
func HandleDeposit(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dseq, err := marshal.RequireString(req, "dseq")
		if err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		amount, ok := marshal.OptionalFloat(req, "amount_usd")
		if !ok {
			return marshal.ErrResult("amount_usd is required"), nil
		}
		// Checked here as well as server-side so a wrong amount costs a tool
		// call rather than a failed API request against a funded account.
		if amount < console.MinDepositUSD {
			return marshal.ErrResultf("amount_usd must be at least %.2f, got %.2f", console.MinDepositUSD, amount), nil
		}

		if err := cl.Deposit(ctx, dseq, amount); err != nil {
			return marshal.ErrResultf("failed to deposit into %s: %v", dseq, err), nil
		}

		return marshal.ToTextResult(map[string]any{
			"dseq":        dseq,
			"amount_usd":  amount,
			"status":      "deposited",
			"description": "escrow topped up",
		})
	}
}
