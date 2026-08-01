package provider

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	manifest "pkg.akt.dev/go/manifest/v2beta3"
	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	mtypes "pkg.akt.dev/go/node/market/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	rest "pkg.akt.dev/go/provider/client"

	"pkg.akt.dev/akt/internal/mcp/marshal"
	aktprovider "pkg.akt.dev/akt/internal/provider"
)

// --- On-chain provider query tools ---

// ToolListProviders returns the tool definition for listing providers.
func ToolListProviders() mcp.Tool {
	return mcp.NewTool(
		"akash_list_providers",
		mcp.WithDescription("List registered providers on the Akash network. Returns provider details including host URI and attributes."),
	)
}

// HandleListProviders returns the handler for listing providers.
func HandleListProviders(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := cl.Query().Provider().Providers(ctx, &ptypes.QueryProvidersRequest{})
		if err != nil {
			return marshal.ErrResultf("failed to list providers: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// ToolGetProvider returns the tool definition for getting a specific provider.
func ToolGetProvider() mcp.Tool {
	return mcp.NewTool(
		"akash_get_provider",
		mcp.WithDescription("Get details of a specific provider by owner address, including host URI and attributes."),
		mcp.WithString("owner",
			mcp.Required(),
			mcp.Description("Provider owner bech32 address (e.g. akash1...)."),
		),
	)
}

// HandleGetProvider returns the handler for getting a provider.
func HandleGetProvider(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		owner, err := marshal.RequireString(req, "owner")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		resp, err := cl.Query().Provider().Provider(ctx, &ptypes.QueryProviderRequest{
			Owner: owner,
		})
		if err != nil {
			return marshal.ErrResultf("failed to get provider: %v", err), nil
		}
		return marshal.ToTextResult(resp)
	}
}

// --- Provider REST tools ---

// ToolProviderStatus returns the tool definition for querying provider status via REST.
func ToolProviderStatus() mcp.Tool {
	return mcp.NewTool(
		"akash_provider_status",
		mcp.WithDescription("Get the live status of a provider including cluster capacity, active leases, and bid engine status. Connects directly to the provider's REST API."),
		mcp.WithString("provider_url",
			mcp.Required(),
			mcp.Description("Provider REST API URL (e.g. https://provider.example.com:8443)."),
		),
	)
}

// HandleProviderStatus returns the handler for querying provider status.
func HandleProviderStatus(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		providerURL, err := marshal.RequireString(req, "provider_url")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		rcl, err := aktprovider.NewPublicGatewayClient(ctx, nil, providerURL)
		if err != nil {
			return marshal.ErrResultf("failed to create provider client: %v", err), nil
		}

		status, err := rcl.Status(ctx)
		if err != nil {
			return marshal.ErrResultf("failed to get provider status: %v", err), nil
		}
		return marshal.ToTextResult(status)
	}
}

// ToolLeaseStatus returns the tool definition for querying lease status from a provider.
func ToolLeaseStatus() mcp.Tool {
	return mcp.NewTool(
		"akash_lease_status",
		mcp.WithDescription("Get the live status of a lease from a provider, including service status, forwarded ports, and IPs."),
		mcp.WithString("provider_url",
			mcp.Required(),
			mcp.Description("Provider REST API URL."),
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

// HandleLeaseStatus returns the handler for querying lease status.
func HandleLeaseStatus(cl v1beta3.LightClient, authType string) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		providerURL, err := marshal.RequireString(req, "provider_url")
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

		rcl, err := gatewayClient(ctx, cl, providerURL, authType)
		if err != nil {
			return marshal.ErrResultf("failed to create provider client: %v", err), nil
		}

		leaseID := mtypes.LeaseID{
			DSeq: dseq,
			GSeq: gseq,
			OSeq: oseq,
		}

		status, err := rcl.LeaseStatus(ctx, leaseID)
		if err != nil {
			return marshal.ErrResultf("failed to get lease status: %v", err), nil
		}
		return marshal.ToTextResult(status)
	}
}

// ToolServiceStatus returns the tool definition for querying service status.
func ToolServiceStatus() mcp.Tool {
	return mcp.NewTool(
		"akash_service_status",
		mcp.WithDescription("Get the status of a specific service within a lease, including replicas, available instances, and URIs."),
		mcp.WithString("provider_url",
			mcp.Required(),
			mcp.Description("Provider REST API URL."),
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
		mcp.WithString("service",
			mcp.Required(),
			mcp.Description("Service name as defined in the SDL/manifest."),
		),
	)
}

// HandleServiceStatus returns the handler for querying service status.
func HandleServiceStatus(cl v1beta3.LightClient, authType string) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		providerURL, err := marshal.RequireString(req, "provider_url")
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

		service, err := marshal.RequireString(req, "service")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		rcl, err := gatewayClient(ctx, cl, providerURL, authType)
		if err != nil {
			return marshal.ErrResultf("failed to create provider client: %v", err), nil
		}

		leaseID := mtypes.LeaseID{
			DSeq: dseq,
			GSeq: gseq,
			OSeq: oseq,
		}

		status, err := rcl.ServiceStatus(ctx, leaseID, service)
		if err != nil {
			return marshal.ErrResultf("failed to get service status: %v", err), nil
		}
		return marshal.ToTextResult(status)
	}
}

// --- Write tools (gated behind --enable-writes) ---

// ToolSubmitManifest returns the tool definition for submitting a manifest to a provider.
func ToolSubmitManifest() mcp.Tool {
	return mcp.NewTool(
		"akash_submit_manifest",
		mcp.WithDescription("Submit a deployment manifest to a provider. The manifest defines the services to deploy. Must be called after a lease is created."),
		mcp.WithString("provider_url",
			mcp.Required(),
			mcp.Description("Provider REST API URL."),
		),
		mcp.WithNumber("dseq",
			mcp.Required(),
			mcp.Description("Deployment sequence number."),
			marshal.PositiveInteger(),
		),
		mcp.WithString("manifest_json",
			mcp.Required(),
			mcp.Description("Manifest as a JSON string (serialized manifest.Manifest)."),
		),
	)
}

// HandleSubmitManifest returns the handler for submitting a manifest.
func HandleSubmitManifest(cl v1beta3.LightClient, authType string) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		providerURL, err := marshal.RequireString(req, "provider_url")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		dseq, err := marshal.RequireUint64(req, "dseq")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		manifestJSON, err := marshal.RequireString(req, "manifest_json")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}

		var mani manifest.Manifest
		if err := json.Unmarshal([]byte(manifestJSON), &mani); err != nil {
			return marshal.ErrResultf("invalid manifest JSON: %v", err), nil
		}

		rcl, err := gatewayClient(ctx, cl, providerURL, authType)
		if err != nil {
			return marshal.ErrResultf("failed to create provider client: %v", err), nil
		}

		if err := rcl.SubmitManifest(ctx, dseq, mani); err != nil {
			return marshal.ErrResultf("failed to submit manifest: %v", err), nil
		}

		return marshal.ToTextResult(map[string]string{"status": "manifest submitted successfully"})
	}
}

func gatewayClient(
	ctx context.Context,
	cl v1beta3.LightClient,
	providerURL string,
	authType string,
) (rest.Client, error) {
	cctx := cl.ClientContext()
	addr := cctx.GetFromAddress()
	return aktprovider.NewGatewayClient(ctx, cctx, addr, providerURL, authType, cctx.Keyring)
}
