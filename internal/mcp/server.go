// Package mcp implements an MCP (Model Context Protocol) server that exposes
// Akash Network tools for AI assistant integration. The server runs over stdio
// transport and uses the chain-sdk client for chain connectivity.
package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"

	"pkg.akt.dev/akt/internal/mcp/tools/audit"
	"pkg.akt.dev/akt/internal/mcp/tools/bank"
	"pkg.akt.dev/akt/internal/mcp/tools/cert"
	"pkg.akt.dev/akt/internal/mcp/tools/deployment"
	"pkg.akt.dev/akt/internal/mcp/tools/market"
	"pkg.akt.dev/akt/internal/mcp/tools/node"
	"pkg.akt.dev/akt/internal/mcp/tools/provider"
)

// Server wraps the MCP server with Akash tools.
type Server struct {
	mcp *mcpserver.MCPServer
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
func New(ctx context.Context, cctx sdkclient.Context, enableWrites bool) (*Server, error) {
	srv := mcpserver.NewMCPServer(
		"akash-mcp",
		"0.1.0",
		mcpserver.WithToolCapabilities(true),
	)

	s := &Server{mcp: srv}

	if enableWrites {
		cl, err := v1beta3.NewClient(ctx, cctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create chain client: %w", err)
		}
		s.registerQueryTools(cl)
		s.registerWriteTools(cl)
	} else {
		cl, err := v1beta3.NewLightClient(cctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create chain client: %w", err)
		}
		s.registerQueryTools(cl)
	}

	return s, nil
}

// ServeStdio starts the server over stdio transport.
func (s *Server) ServeStdio(ctx context.Context) error {
	return mcpserver.ServeStdio(s.mcp, mcpserver.WithStdioContextFunc(func(_ context.Context) context.Context {
		return ctx
	}))
}

// registerQueryTools registers all read-only query tools.
func (s *Server) registerQueryTools(cl v1beta3.LightClient) {
	// Node tools
	s.addTool(node.ToolNodeStatus(), node.HandleNodeStatus(cl))
	s.addTool(node.ToolBlockHeight(), node.HandleBlockHeight(cl))

	// Bank tools
	s.addTool(bank.ToolAccountBalance(), bank.HandleAccountBalance(cl))

	// Deployment query tools
	s.addTool(deployment.ToolListDeployments(), deployment.HandleListDeployments(cl))
	s.addTool(deployment.ToolGetDeployment(), deployment.HandleGetDeployment(cl))
	s.addTool(deployment.ToolGetGroup(), deployment.HandleGetGroup(cl))

	// Market query tools
	s.addTool(market.ToolListOrders(), market.HandleListOrders(cl))
	s.addTool(market.ToolGetOrder(), market.HandleGetOrder(cl))
	s.addTool(market.ToolListBids(), market.HandleListBids(cl))
	s.addTool(market.ToolGetBid(), market.HandleGetBid(cl))
	s.addTool(market.ToolListLeases(), market.HandleListLeases(cl))
	s.addTool(market.ToolGetLease(), market.HandleGetLease(cl))

	// Provider on-chain query tools
	s.addTool(provider.ToolListProviders(), provider.HandleListProviders(cl))
	s.addTool(provider.ToolGetProvider(), provider.HandleGetProvider(cl))

	// Provider REST query tools
	s.addTool(provider.ToolProviderStatus(), provider.HandleProviderStatus(cl))
	s.addTool(provider.ToolLeaseStatus(), provider.HandleLeaseStatus(cl))
	s.addTool(provider.ToolServiceStatus(), provider.HandleServiceStatus(cl))

	// Audit tools
	s.addTool(audit.ToolListAuditedProviders(), audit.HandleListAuditedProviders(cl))

	// Cert tools
	s.addTool(cert.ToolListCertificates(), cert.HandleListCertificates(cl))
}

// registerWriteTools registers write tools that require --enable-writes.
// These tools perform on-chain transactions or mutating provider REST calls.
func (s *Server) registerWriteTools(cl v1beta3.Client) {
	// Deployment tx tools
	s.addTool(deployment.ToolCloseDeployment(), deployment.HandleCloseDeployment(cl))

	// Market tx tools
	s.addTool(market.ToolCreateLease(), market.HandleCreateLease(cl))
	s.addTool(market.ToolCloseLease(), market.HandleCloseLease(cl))

	// Provider REST mutation tools
	s.addTool(provider.ToolSubmitManifest(), provider.HandleSubmitManifest(cl))
}

func (s *Server) addTool(tool mcp.Tool, handler mcpserver.ToolHandlerFunc) {
	s.mcp.AddTool(tool, handler)
}
