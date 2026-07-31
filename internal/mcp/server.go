// Package mcp implements an MCP (Model Context Protocol) server that exposes
// Akash Network tools for AI assistant integration. The server runs over stdio
// transport and uses the chain-sdk client for chain connectivity.
package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	client "pkg.akt.dev/go/node/client/v1beta3"

	aktconsole "pkg.akt.dev/akt/internal/console"
	"pkg.akt.dev/akt/internal/mcp/tools/audit"
	"pkg.akt.dev/akt/internal/mcp/tools/bank"
	"pkg.akt.dev/akt/internal/mcp/tools/cert"
	consoletools "pkg.akt.dev/akt/internal/mcp/tools/console"
	"pkg.akt.dev/akt/internal/mcp/tools/deployment"
	"pkg.akt.dev/akt/internal/mcp/tools/market"
	"pkg.akt.dev/akt/internal/mcp/tools/node"
	"pkg.akt.dev/akt/internal/mcp/tools/provider"
)

// Server wraps the MCP server with Akash tools.
type Server struct {
	mcp *mcpserver.MCPServer

	// schemas records each registered tool's declared input schema, so the
	// argument-validating middleware can check a call against it.
	schemas map[string]mcp.ToolInputSchema
}

// New creates a new MCP server from an akt-resolved SDK client context.
//
// When enableWrites is false (the default), only read-only query tools are
// registered. This prevents AI agents from sending unapproved transactions
// or performing mutating operations.
//
// When enableWrites is true, write tools (on-chain transactions and provider
// REST mutations) are additionally registered. The user must explicitly opt
// in by passing --enable-writes.
func New(ctx context.Context, cctx sdkclient.Context, enableWrites bool, consoleClient *aktconsole.Client) (*Server, error) {
	// WithRecovery turns a panicking tool handler into an error result for
	// that one call. Without it the panic unwinds through the stdio loop and
	// takes the process down, so a single bad call ends the session and every
	// other tool with it -- including the Console rail, which had nothing to
	// do with it.
	s := &Server{schemas: map[string]mcp.ToolInputSchema{}}

	srv := mcpserver.NewMCPServer(
		"akash-mcp",
		"0.1.0",
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithRecovery(),
		mcpserver.WithToolHandlerMiddleware(s.validateArguments),
	)

	s.mcp = srv

	// A chain client is not a prerequisite when the Console rail is
	// available: a managed context reaches deployments, wallet, providers and
	// pricing over the Console API and never touches a node, so refusing to
	// start without a wallet and an RPC endpoint would deny that user a
	// server they can fully use. The failure is only fatal when it leaves the
	// server with nothing at all.
	chainErr := s.registerChainTools(ctx, cctx, enableWrites)

	// The Console rail is additive, and registered only when a key resolved --
	// otherwise every call would fail on auth.
	if consoleClient != nil {
		s.registerConsoleQueryTools(consoleClient)

		if enableWrites {
			s.registerConsoleWriteTools(consoleClient)
		}
	}

	if chainErr != nil && consoleClient == nil {
		return nil, fmt.Errorf("no tools available: chain client unavailable (%w) and no Console API key configured", chainErr)
	}

	return s, nil
}

// registerChainTools registers the chain-backed tools, returning the error
// that prevented it rather than failing the server. A context with no RPC
// endpoint -- which a console-api context is allowed to be -- simply has no
// chain tools.
func (s *Server) registerChainTools(ctx context.Context, cctx sdkclient.Context, enableWrites bool) error {
	// Checked before building a client, because the constructors accept an
	// empty context happily and hand back something that fails on every call.
	// Registering those tools would advertise a chain rail that cannot work,
	// and would mask the case where no rail is available at all.
	if cctx.NodeURI == "" {
		return errors.New("no chain RPC endpoint configured for the active context")
	}

	if enableWrites {
		cl, err := client.NewClient(ctx, cctx)
		if err != nil {
			return err
		}

		s.registerQueryTools(cl)
		s.registerWriteTools(cl)

		return nil
	}

	cl, err := client.NewLightClient(cctx)
	if err != nil {
		return err
	}

	s.registerQueryTools(cl)

	return nil
}

// registerConsoleQueryTools registers the read-only Console API tools.
func (s *Server) registerConsoleQueryTools(cl *aktconsole.Client) {
	s.addQueryTool(consoletools.ToolListDeployments(), consoletools.HandleListDeployments(cl))
	s.addQueryTool(consoletools.ToolGetDeployment(), consoletools.HandleGetDeployment(cl))
	s.addQueryTool(consoletools.ToolListBids(), consoletools.HandleListBids(cl))
	s.addQueryTool(consoletools.ToolWalletBalance(), consoletools.HandleWalletBalance(cl))
	s.addQueryTool(consoletools.ToolUsageHistory(), consoletools.HandleUsageHistory(cl))
	s.addQueryTool(consoletools.ToolListProviders(), consoletools.HandleListProviders(cl))
	s.addQueryTool(consoletools.ToolGetProvider(), consoletools.HandleGetProvider(cl))
	s.addQueryTool(consoletools.ToolGPUPrices(), consoletools.HandleGPUPrices(cl))
}

// registerConsoleWriteTools registers the Console API tools that spend funds
// or tear down workloads.
func (s *Server) registerConsoleWriteTools(cl *aktconsole.Client) {
	s.addWriteTool(consoletools.ToolCloseDeployment(), consoletools.HandleCloseDeployment(cl))
	s.addWriteTool(consoletools.ToolDeposit(), consoletools.HandleDeposit(cl))
}

// ServeStdio starts the server over stdio transport.
func (s *Server) ServeStdio(ctx context.Context) error {
	return mcpserver.ServeStdio(s.mcp, mcpserver.WithStdioContextFunc(func(_ context.Context) context.Context {
		return ctx
	}))
}

// registerQueryTools registers all read-only query tools.
func (s *Server) registerQueryTools(cl client.LightClient) {
	// Node tools
	s.addQueryTool(node.ToolNodeStatus(), node.HandleNodeStatus(cl))
	s.addQueryTool(node.ToolBlockHeight(), node.HandleBlockHeight(cl))

	// Bank tools
	s.addQueryTool(bank.ToolAccountBalance(), bank.HandleAccountBalance(cl))

	// Deployment query tools
	s.addQueryTool(deployment.ToolListDeployments(), deployment.HandleListDeployments(cl))
	s.addQueryTool(deployment.ToolGetDeployment(), deployment.HandleGetDeployment(cl))
	s.addQueryTool(deployment.ToolGetGroup(), deployment.HandleGetGroup(cl))

	// Market query tools
	s.addQueryTool(market.ToolListOrders(), market.HandleListOrders(cl))
	s.addQueryTool(market.ToolGetOrder(), market.HandleGetOrder(cl))
	s.addQueryTool(market.ToolListBids(), market.HandleListBids(cl))
	s.addQueryTool(market.ToolGetBid(), market.HandleGetBid(cl))
	s.addQueryTool(market.ToolListLeases(), market.HandleListLeases(cl))
	s.addQueryTool(market.ToolGetLease(), market.HandleGetLease(cl))

	// Provider on-chain query tools
	s.addQueryTool(provider.ToolListProviders(), provider.HandleListProviders(cl))
	s.addQueryTool(provider.ToolGetProvider(), provider.HandleGetProvider(cl))

	// Provider REST query tools
	s.addQueryTool(provider.ToolProviderStatus(), provider.HandleProviderStatus(cl))
	s.addQueryTool(provider.ToolLeaseStatus(), provider.HandleLeaseStatus(cl))
	s.addQueryTool(provider.ToolServiceStatus(), provider.HandleServiceStatus(cl))

	// Audit tools
	s.addQueryTool(audit.ToolListAuditedProviders(), audit.HandleListAuditedProviders(cl))

	// Cert tools
	s.addQueryTool(cert.ToolListCertificates(), cert.HandleListCertificates(cl))
}

// registerWriteTools registers write tools that require --enable-writes.
// These tools perform on-chain transactions or mutating provider REST calls.
func (s *Server) registerWriteTools(cl client.Client) {
	// Deployment tx tools
	s.addWriteTool(deployment.ToolCloseDeployment(), deployment.HandleCloseDeployment(cl))

	// Market tx tools
	s.addWriteTool(market.ToolCreateLease(), market.HandleCreateLease(cl))
	s.addWriteTool(market.ToolCloseLease(), market.HandleCloseLease(cl))

	// Provider REST mutation tools
	s.addWriteTool(provider.ToolSubmitManifest(), provider.HandleSubmitManifest(cl))
}

// addQueryTool registers a read-only tool. The MCP spec defaults an
// unannotated tool to destructiveHint=true and readOnlyHint=false, so a client
// deciding what to auto-approve treats every unannotated query as dangerous.
// Annotating here rather than at each declaration keeps the policy in one
// place: registration through this method is what makes a tool read-only, so
// the two cannot drift apart.
func (s *Server) addQueryTool(tool mcp.Tool, handler mcpserver.ToolHandlerFunc) {
	tool.Annotations = queryToolAnnotations()
	s.schemas[tool.Name] = tool.InputSchema
	s.mcp.AddTool(tool, handler)
}

// queryToolAnnotations is the annotation set every read-only tool carries.
func queryToolAnnotations() mcp.ToolAnnotation {
	return mcp.ToolAnnotation{
		ReadOnlyHint:    mcp.ToBoolPtr(true),
		DestructiveHint: mcp.ToBoolPtr(false),
		// Repeating a query changes nothing beyond what the chain or the
		// provider already reports.
		IdempotentHint: mcp.ToBoolPtr(true),
		// Every tool reaches a chain node, a provider gateway or the Console
		// API, none of which are a closed local domain.
		OpenWorldHint: mcp.ToBoolPtr(true),
	}
}

// addWriteTool registers a mutating tool. These broadcast transactions or
// mutate provider state, so they stay destructive and non-idempotent -- a
// client should confirm them with the user rather than auto-approve.
func (s *Server) addWriteTool(tool mcp.Tool, handler mcpserver.ToolHandlerFunc) {
	tool.Annotations = writeToolAnnotations()
	s.schemas[tool.Name] = tool.InputSchema
	s.mcp.AddTool(tool, handler)
}

// writeToolAnnotations is the annotation set every mutating tool carries.
func writeToolAnnotations() mcp.ToolAnnotation {
	return mcp.ToolAnnotation{
		ReadOnlyHint:    mcp.ToBoolPtr(false),
		DestructiveHint: mcp.ToBoolPtr(true),
		IdempotentHint:  mcp.ToBoolPtr(false),
		OpenWorldHint:   mcp.ToBoolPtr(true),
	}
}
