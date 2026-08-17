package provider

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	manifest "pkg.akt.dev/go/manifest/v2beta3"
	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	mtypes "pkg.akt.dev/go/node/market/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	rest "pkg.akt.dev/go/provider/client"
	ajwt "pkg.akt.dev/go/util/jwt"

	aktclient "pkg.akt.dev/akt/internal/client"
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
		mcp.WithString("provider",
			mcp.Required(),
			mcp.Description("Provider owner bech32 address. The registered on-chain host URI selects the gateway."),
		),
	)
}

// HandleProviderStatus returns the handler for querying provider status.
func HandleProviderStatus(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, providerURL, err := resolveRegisteredProvider(ctx, req, cl)
		if err != nil {
			return marshal.ErrResultf("resolve provider gateway: %v", err), nil
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
		mcp.WithString("provider",
			mcp.Required(),
			mcp.Description("Provider owner bech32 address. The registered on-chain host URI selects the gateway."),
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
		providerOwner, providerURL, err := resolveRegisteredProvider(ctx, req, cl)
		if err != nil {
			return marshal.ErrResultf("resolve provider gateway: %v", err), nil
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

		rcl, err := gatewayClient(ctx, cl, providerURL, providerOwner, ajwt.PermissionDeployment{
			Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Services: []string{"*"},
		}, authType)
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
		mcp.WithString("provider",
			mcp.Required(),
			mcp.Description("Provider owner bech32 address. The registered on-chain host URI selects the gateway."),
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
		providerOwner, providerURL, err := resolveRegisteredProvider(ctx, req, cl)
		if err != nil {
			return marshal.ErrResultf("resolve provider gateway: %v", err), nil
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

		rcl, err := gatewayClient(ctx, cl, providerURL, providerOwner, ajwt.PermissionDeployment{
			Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Services: []string{service},
		}, authType)
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
		mcp.WithString("provider",
			mcp.Required(),
			mcp.Description("Provider owner bech32 address. The registered on-chain host URI selects the gateway and this address is recorded as the stable audit identity."),
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
		providerAddress, err := marshal.RequireString(req, "provider")
		if err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		providerOwner, err := sdk.AccAddressFromBech32(providerAddress)
		if err != nil {
			return marshal.ErrResultf("invalid provider address %q: %v", providerAddress, err), nil
		}
		providerAddress = providerOwner.String()

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
		if err := mani.Validate(); err != nil {
			return marshal.ErrResultf("%v", err), nil
		}
		services := manifestServiceNames(mani)

		providerURL, err := registeredProviderURL(ctx, cl, providerAddress)
		if err != nil {
			aktprovider.RecordAction(ctx, "send-manifest", providerAddress, dseq, err)
			return marshal.ErrResultf("resolve provider gateway: %v", err), nil
		}

		rcl, err := gatewayClient(ctx, cl, providerURL, providerOwner, ajwt.PermissionDeployment{
			Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeSendManifest},
			DSeq:     dseq,
			Services: services,
		}, authType)
		if err != nil {
			aktprovider.RecordAction(ctx, "send-manifest", providerAddress, dseq, err)
			return marshal.ErrResultf("failed to create provider client: %v", err), nil
		}

		err = rcl.SubmitManifest(ctx, dseq, mani)
		aktprovider.RecordAction(ctx, "send-manifest", providerAddress, dseq, err)
		if err != nil {
			return marshal.ErrResultf("failed to submit manifest: %v", err), nil
		}

		return marshal.ToTextResult(map[string]string{"status": "manifest submitted successfully"})
	}
}

func registeredProviderURL(ctx context.Context, cl v1beta3.LightClient, owner string) (string, error) {
	if cl == nil {
		return "", fmt.Errorf("provider query client is unavailable")
	}
	queries := cl.Query()
	if queries == nil || queries.Provider() == nil {
		return "", fmt.Errorf("provider query client is unavailable")
	}

	response, err := queries.Provider().Provider(ctx, &ptypes.QueryProviderRequest{Owner: owner})
	if err != nil {
		return "", fmt.Errorf("query provider %s: %w", owner, err)
	}
	if response == nil || response.Provider.HostURI == "" {
		return "", fmt.Errorf("provider %s has no registered host URI", owner)
	}

	return response.Provider.HostURI, nil
}

func resolveRegisteredProvider(
	ctx context.Context,
	req mcp.CallToolRequest,
	cl v1beta3.LightClient,
) (sdk.AccAddress, string, error) {
	providerAddress, err := marshal.RequireString(req, "provider")
	if err != nil {
		return nil, "", err
	}
	providerOwner, err := sdk.AccAddressFromBech32(providerAddress)
	if err != nil {
		return nil, "", fmt.Errorf("invalid provider address %q: %w", providerAddress, err)
	}
	providerAddress = providerOwner.String()

	providerURL, err := registeredProviderURL(ctx, cl, providerAddress)
	if err != nil {
		return nil, "", err
	}

	return providerOwner, providerURL, nil
}

func gatewayClient(
	ctx context.Context,
	cl v1beta3.LightClient,
	providerURL string,
	providerAddress sdk.AccAddress,
	deployment ajwt.PermissionDeployment,
	authType string,
) (rest.Client, error) {
	cctx := cl.ClientContext()
	addr, err := aktclient.ResolveAccountAddress(cctx)
	if err != nil {
		return nil, err
	}
	cctx = cctx.WithFromAddress(addr)
	return aktprovider.NewScopedGatewayClient(
		ctx,
		cctx,
		addr,
		providerURL,
		providerAddress,
		deployment,
		authType,
		cctx.Keyring,
	)
}

func manifestServiceNames(mani manifest.Manifest) []string {
	seen := make(map[string]struct{})
	services := make([]string, 0)
	for _, group := range mani {
		for _, service := range group.Services {
			if _, exists := seen[service.Name]; exists {
				continue
			}
			seen[service.Name] = struct{}{}
			services = append(services, service.Name)
		}
	}
	return services
}
