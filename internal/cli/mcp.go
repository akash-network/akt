package cli

import (
	"fmt"
	"os"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	aktmcp "pkg.akt.dev/akt/internal/mcp"
)

func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server for AI assistant integration",
		Long: `Start an MCP (Model Context Protocol) server over stdio transport.

Exposes Akash Network tools for use by AI assistants. Configuration is
resolved from the active akt context (network, keyring, default account).

By default, only read-only query tools are available (21 tools). Write
tools (on-chain transactions and provider mutations) require explicit
opt-in via --enable-writes to prevent AI agents from sending unapproved
transactions.

Read-only tools include: node status, account balances, deployments,
orders, bids, leases, providers, audited attributes, and certificates.

Write tools (with --enable-writes): close deployment, create lease,
close lease, and submit manifest.`,
		Args: cobra.NoArgs,
		Example: `  # Read-only mode (safe for AI agents)
  akt mcp

  # With write tools enabled
  akt mcp --enable-writes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cctx := sdkclient.GetClientContextFromCmd(cmd)

			enableWrites, _ := cmd.Flags().GetBool("enable-writes")

			srv, err := aktmcp.New(ctx, cctx, enableWrites)
			if err != nil {
				return fmt.Errorf("failed to create MCP server: %w", err)
			}

			mode := "read-only"
			if enableWrites {
				mode = "read-write"
			}

			_, _ = fmt.Fprintf(os.Stderr, "akt mcp: starting stdio server (node=%s, chain=%s, mode=%s)\n", cctx.NodeURI, cctx.ChainID, mode)

			return srv.ServeStdio(ctx)
		},
	}

	cmd.Flags().Bool("enable-writes", false,
		"Enable write tools (on-chain transactions and provider mutations). "+
			"Without this flag, only read-only query tools are available.")

	return cmd
}
