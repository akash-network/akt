package cert

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"

	ctypes "pkg.akt.dev/go/node/cert/v1"

	"pkg.akt.dev/akt/internal/mcp/marshal"
)

// ToolListCertificates returns the tool definition for listing certificates.
func ToolListCertificates() mcp.Tool {
	return mcp.NewTool(
		"akash_list_certificates",
		mcp.WithDescription("List on-chain certificates. Certificates are used for mTLS authentication between tenants and providers."),
		mcp.WithString("owner",
			mcp.Description("Filter by certificate owner bech32 address. If omitted, uses the configured key's address."),
		),
		mcp.WithString("state",
			mcp.Enum("valid", "revoked"),
			mcp.Description("Filter by certificate state: 'valid', 'revoked'."),
		),
	)
}

// HandleListCertificates returns the handler for listing certificates.
func HandleListCertificates(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.AddressOrDefault(cl.ClientContext(), marshal.OptionalString(req, "owner"))
		if err != nil {
			return marshal.ErrResultf("failed to resolve owner address: %v", err), nil
		}

		if owner == "" {
			return marshal.ErrResult("owner address is required: pass owner, or configure a default account for the active context"), nil
		}

		state := marshal.OptionalString(req, "state")

		resp, err := cl.Query().Certs().Certificates(ctx, &ctypes.QueryCertificatesRequest{
			Filter: ctypes.CertificateFilter{
				Owner: owner,
				State: state,
			},
		})
		if err != nil {
			return marshal.ErrResultf("failed to list certificates: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}
