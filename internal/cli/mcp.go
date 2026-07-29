package cli

import (
	"fmt"
	"os"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktmcp "pkg.akt.dev/akt/internal/mcp"
)

func mcpCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server for AI assistant integration",
		Long: `Start an MCP (Model Context Protocol) server over stdio transport.

Exposes Akash Network tools for use by AI assistants. Configuration is
resolved from the active akt context (network, keyring, default account).

By default, only read-only query tools are available. Write
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

			// The tx client validates the sign mode, which normally arrives
			// from the --sign-mode flag that AddTxFlagsToCmd installs on tx
			// commands. This command has no tx flags, so the field is empty
			// and the write client fails to build at all. Default it to the
			// same value the flag would have.
			if enableWrites && cctx.SignModeStr == "" {
				cctx = cctx.WithSignModeStr(flags.SignModeDirect)
			}

			srv, err := aktmcp.New(ctx, cctx, enableWrites, consoleClientFor(cmd, mgrFn))
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

	cmd.Flags().String("console-api-key", "",
		"Console API key. Overrides the context credential and AKT_CONSOLE_API_KEY.")
	cmd.Flags().Bool("enable-writes", false,
		"Enable write tools (on-chain transactions and provider mutations). "+
			"Without this flag, only read-only query tools are available.")

	return cmd
}

// consoleClientFor resolves a Console API client for the MCP server, or nil
// when no key is available. Resolution matches the console command group
// (SPEC §7.1): --console-api-key flag > context credential (which already
// prefers AKT_CONSOLE_API_KEY over the stored value) > bare environment.
//
// nil rather than an error: a chain-only context is a perfectly good MCP
// setup, and registering Console tools that would fail on every call is worse
// than not offering them.
func consoleClientFor(cmd *cobra.Command, mgrFn func() *aktctx.Manager) *console.Client {
	key, _ := cmd.Flags().GetString("console-api-key")

	baseURL := ""
	if m := mgrFn(); m != nil {
		override := ""
		if f := cmd.Flags().Lookup("context"); f != nil {
			override = f.Value.String()
		}

		if rc, err := m.Resolve(m.ActiveContext(override)); err == nil && rc != nil {
			if key == "" {
				key = rc.ConsoleAPIKey
			}
			baseURL = rc.ConsoleAPIURL
		}
	}

	if key == "" {
		key = os.Getenv(aktctx.EnvConsoleAPIKey)
	}

	if key == "" {
		return nil
	}

	return console.New(baseURL, key)
}
