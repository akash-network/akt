package audit

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"

	atypes "pkg.akt.dev/go/node/audit/v1"

	"pkg.akt.dev/akt/internal/mcp/marshal"
)

// ToolListAuditedProviders returns the tool definition for listing audited providers.
func ToolListAuditedProviders() mcp.Tool {
	return mcp.NewTool(
		"akash_list_audited_providers",
		mcp.WithDescription("List audited provider attributes. Returns providers that have been audited along with their signed attributes."),
		mcp.WithString("owner",
			mcp.Description("Filter by provider owner bech32 address."),
		),
		mcp.WithString("auditor",
			mcp.Description("Filter by auditor bech32 address."),
		),
	)
}

// HandleListAuditedProviders returns the handler for listing audited providers.
func HandleListAuditedProviders(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner := marshal.OptionalString(req, "owner")
		auditor := marshal.OptionalString(req, "auditor")

		// Route to the appropriate query based on provided filters
		if owner != "" && auditor != "" {
			resp, err := cl.Query().Audit().ProviderAuditorAttributes(ctx, &atypes.QueryProviderAuditorRequest{
				Owner:   owner,
				Auditor: auditor,
			})
			if err != nil {
				return marshal.ErrResultf("failed to query provider auditor attributes: %v", err), nil
			}
			return marshal.ToTextResult(resp)
		}

		if owner != "" {
			resp, err := cl.Query().Audit().ProviderAttributes(ctx, &atypes.QueryProviderAttributesRequest{
				Owner: owner,
			})
			if err != nil {
				return marshal.ErrResultf("failed to query provider attributes: %v", err), nil
			}
			return marshal.ToTextResult(resp)
		}

		if auditor != "" {
			resp, err := cl.Query().Audit().AuditorAttributes(ctx, &atypes.QueryAuditorAttributesRequest{
				Auditor: auditor,
			})
			if err != nil {
				return marshal.ErrResultf("failed to query auditor attributes: %v", err), nil
			}
			return marshal.ToTextResult(resp)
		}

		// No filters - list all
		resp, err := cl.Query().Audit().AllProvidersAttributes(ctx, &atypes.QueryAllProvidersAttributesRequest{})
		if err != nil {
			return marshal.ErrResultf("failed to list audited providers: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}
