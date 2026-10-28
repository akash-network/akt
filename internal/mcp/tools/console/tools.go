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
		mcp.WithDescription("List the Console-managed deployments belonging to the configured API key, or only those filed in one project."),
		projectParam(),
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

		scoped, err := inProject(ctx, cl, req)
		if err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		resp, err := scoped.ListDeployments(ctx, skip, limit)
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
		projectParam(),
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

		scoped, err := inProject(ctx, cl, req)
		if err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		resp, err := scoped.GetDeployment(ctx, dseq)
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
		projectParam(),
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

		scoped, err := inProject(ctx, cl, req)
		if err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		bids, err := scoped.FetchBids(ctx, dseq)
		if err != nil {
			return marshal.ErrResultf("failed to fetch bids for %s: %v", dseq, err), nil
		}

		// An empty list is only worth waiting on if the deployment exists.
		// Returning [] for a dseq that does not exist, next to a description
		// saying bids take a few seconds to arrive, tells an assistant to poll
		// forever.
		if len(bids) == 0 {
			if _, err := scoped.GetDeployment(ctx, dseq); err != nil {
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
		mcp.WithDescription("Get the Console-managed wallet's available, escrow-held, and total balances in USD. Escrow is held by running deployments, not spent, and returns to available as each one closes."),
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
			AvailableUSD float64 `json:"available_usd"`
			EscrowUSD    float64 `json:"escrow_usd"`
			TotalUSD     float64 `json:"total_usd"`
		}{
			AvailableUSD: resp.BalanceUSD(),
			EscrowUSD:    resp.DeploymentsUSD(),
			TotalUSD:     resp.TotalUSD(),
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
		projectParam(),
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

		scoped, err := inProject(ctx, cl, req)
		if err != nil {
			return marshal.ErrResult(err.Error()), nil
		}

		if err := scoped.CloseDeployment(ctx, dseq); err != nil {
			return marshal.ErrResultf("failed to close deployment %s: %v", dseq, err), nil
		}

		return marshal.ToTextResult(map[string]string{
			"dseq":   dseq,
			"status": "closed",
		})
	}
}

// ToolListOrganizations returns the tool definition for the organization the
// API key acts in.
func ToolListOrganizations() mcp.Tool {
	return mcp.NewTool(
		"console_list_organizations",
		mcp.WithDescription("List the Console organization (team or personal account) the configured API key acts in, with the caller's role there: owner, admin, member, billing, or viewer. An API key acts in exactly one organization."),
	)
}

// HandleListOrganizations returns the handler for console_list_organizations.
func HandleListOrganizations(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		organizations, err := cl.ListOrganizations(ctx)
		if err != nil {
			return marshal.ErrResultf("failed to list organizations: %v", err), nil
		}

		return marshal.ToTextResult(organizations)
	}
}

// ToolListProjects returns the tool definition for listing projects. A
// server pinned to one project does not invite the caller to switch.
func ToolListProjects(pinned bool) mcp.Tool {
	description := "List the projects of the API key's organization that it can reach, most recently active first, with whether the caller may deploy into each."
	if pinned {
		description += " This server acts only in the project it was started with; the deployment tools refuse any other."
	} else {
		description += " Pass a project's id, slug, or name as `project` to the deployment tools to act in it."
	}

	return mcp.NewTool("console_list_projects", mcp.WithDescription(description))
}

// HandleListProjects returns the handler for console_list_projects.
func HandleListProjects(cl *console.Client) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projects, err := cl.ForProject("").ListProjects(ctx)
		if err != nil {
			return marshal.ErrResultf("failed to list projects: %v", err), nil
		}

		return marshal.ToTextResult(projects)
	}
}

// projectParam is the optional project selector shared by the deployment
// tools.
func projectParam() mcp.ToolOption {
	return mcp.WithString("project",
		mcp.Description("Optional Console project id, slug, or name to act in. When the server was started with a project (--console-project, AKT_CONSOLE_PROJECT, or the context), omit this or name that same project; any other project is refused. Without a server project, omitting it acts across every project the API key reaches."),
	)
}

// inProject returns the client a call acts in. A server pinned to a project
// refuses a call that names another one, so the model cannot move an action
// out of the project the operator chose.
func inProject(ctx context.Context, cl *console.Client, req mcp.CallToolRequest) (*console.Client, error) {
	project := strings.TrimSpace(marshal.OptionalString(req, "project"))
	if project == "" {
		return cl, nil
	}
	if cl.Project() == "" {
		return cl.ForProject(project), nil
	}

	pinnedID, err := cl.ProjectID(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve this server's project: %w", err)
	}
	requested, err := cl.FindProject(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("resolve project %q: %w", project, err)
	}
	if !strings.EqualFold(requested.ID, pinnedID) {
		return nil, fmt.Errorf("this server acts only in project %s; omit the project argument to act in it", cl.Project())
	}

	return cl, nil
}
