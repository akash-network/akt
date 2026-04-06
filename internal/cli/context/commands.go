package context

import (
	"fmt"
	"strings"
	"time"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/actionlog"
	clikeys "pkg.akt.dev/akt/internal/cli/keys"
	clinetwork "pkg.akt.dev/akt/internal/cli/network"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
)

// Commands returns the "context" command tree, including "network" and "keys" as subcommands.
func Commands(mgr func() *aktctx.Manager, getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage contexts, networks, and keys",
		Long:  "A context composes a network, keyring, state store, and action log into a named environment.",
	}

	cmd.AddCommand(
		createCmd(mgr),
		useCmd(mgr),
		listCmd(mgr),
		currentCmd(mgr),
		editCmd(mgr),
		deleteCmd(mgr),
		renameCmd(mgr),
		logCmd(mgr),
		clinetwork.Commands(mgr),
		clikeys.Commands(getKeyring),
	)

	return cmd
}

func createCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()
			name := args[0]

			network, _ := cmd.Flags().GetString("network")
			keyring, _ := cmd.Flags().GetString("keyring")
			defaultAccount, _ := cmd.Flags().GetString("default-account")
			gas, _ := cmd.Flags().GetString("gas")
			fees, _ := cmd.Flags().GetString("fees")
			setCurrent, _ := cmd.Flags().GetBool("set-current")

			ctx := aktctx.Context{
				Name:           name,
				Network:        aktctx.Network{Name: network},
				Keyring:        aktctx.Keyring{Name: keyring},
				DefaultAccount: defaultAccount,
				Gas:            gas,
				Fees:           fees,
			}

			if err := m.CreateContext(ctx); err != nil {
				return err
			}

			if setCurrent {
				return m.UseContext(name)
			}

			return nil
		},
	}

	cmd.Flags().String("network", "", "Network name to use (required)")
	cmd.Flags().String("keyring", "default", "Keyring name to use")
	cmd.Flags().String("default-account", "", "Default account name")
	cmd.Flags().String("gas", "auto", "Gas limit override")
	cmd.Flags().String("fees", "", "Fixed fees override")
	cmd.Flags().Bool("set-current", false, "Set as current context after creation")
	_ = cmd.MarkFlagRequired("network")

	return cmd
}

func useCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch the active context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mgr().UseContext(args[0])
		},
	}
}

func listCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all contexts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := mgr()
			ctxs := m.ListContexts()
			current := m.CurrentContext()

			if len(ctxs) == 0 {
				fmt.Println("No contexts configured. Create one with: akt context create <name> --network <network>")
				return nil
			}

			type contextRow struct {
				Current        bool   `json:"current"                    yaml:"current"`
				Name           string `json:"name"                       yaml:"name"`
				Network        string `json:"network"                    yaml:"network"`
				Keyring        string `json:"keyring"                    yaml:"keyring"`
				DefaultAccount string `json:"default_account,omitempty"  yaml:"default-account,omitempty"`
				ChainID        string `json:"chain_id"                   yaml:"chain-id"`
			}

			data := make([]contextRow, 0, len(ctxs))
			columns := []output.Column{
				{Header: ""},
				{Header: "NAME"},
				{Header: "NETWORK"},
				{Header: "KEYRING"},
				{Header: "DEFAULT-ACCOUNT"},
				{Header: "CHAIN-ID"},
			}

			rows := make([][]string, 0, len(ctxs))
			for _, c := range ctxs {
				isCurrent := c.Name == current
				marker := " "
				if isCurrent {
					marker = "*"
				}

				chainID := ""
				if net := m.GetNetwork(c.Network.Name); net != nil {
					chainID = net.ChainID
				}

				rows = append(rows, []string{marker, c.Name, c.Network.Name, c.Keyring.Name, c.DefaultAccount, chainID})
				data = append(data, contextRow{
					Current:        isCurrent,
					Name:           c.Name,
					Network:        c.Network.Name,
					Keyring:        c.Keyring.Name,
					DefaultAccount: c.DefaultAccount,
					ChainID:        chainID,
				})
			}

			return output.PrintData(cmd, columns, rows, data)
		},
	}
}

func currentCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the active context with full details",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := mgr()

			rc, err := m.Resolve("")
			if err != nil {
				return err
			}

			fmt.Printf("Context:         %s\n", rc.Name)
			fmt.Printf("Network:         %s\n", rc.Network.Name)
			fmt.Printf("  Chain ID:      %s\n", rc.Network.ChainID)

			rpcStr := "(none)"
			if len(rc.Network.Endpoints.RPC) > 0 {
				rpcStr = rc.Network.Endpoints.RPC[0]
				if len(rc.Network.Endpoints.RPC) > 1 {
					rpcStr += fmt.Sprintf(" (+%d backup)", len(rc.Network.Endpoints.RPC)-1)
				}
			}

			fmt.Printf("  RPC:           %s\n", rpcStr)

			if len(rc.Network.Endpoints.API) > 0 {
				apiStr := rc.Network.Endpoints.API[0]
				if len(rc.Network.Endpoints.API) > 1 {
					apiStr += fmt.Sprintf(" (+%d backup)", len(rc.Network.Endpoints.API)-1)
				}

				fmt.Printf("  API:           %s\n", apiStr)
			}

			if len(rc.Network.Endpoints.GRPC) > 0 {
				fmt.Printf("  gRPC:          %s\n", rc.Network.Endpoints.GRPC[0])
			}

			fmt.Printf("  Gas Prices:    %s\n", rc.GasPrices)
			fmt.Printf("  Gas Adj:       %s\n", rc.GasAdjustment)
			fmt.Printf("Keyring:         %s (backend: %s)\n", rc.Keyring.Name, rc.Keyring.Backend)

			if rc.DefaultAccount != "" {
				fmt.Printf("Default Account: %s\n", rc.DefaultAccount)
			} else {
				fmt.Printf("Default Account: (not set)\n")
			}

			fmt.Printf("Gas:             %s\n", rc.Gas)

			if rc.Fees != "" {
				fmt.Printf("Fees:            %s\n", rc.Fees)
			} else {
				fmt.Printf("Fees:            (none)\n")
			}

			fmt.Printf("Provider Auth:   %s\n", rc.AuthType)
			fmt.Printf("Store:           %s\n", aktctx.StoreDir(rc.Root, rc.Name))
			fmt.Printf("Action Log:      %s\n", aktctx.ActionLogPath(rc.Root, rc.Name))

			return nil
		},
	}
}

func editCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()
			name := args[0]

			forkNetwork, _ := cmd.Flags().GetBool("fork-network")
			if forkNetwork && cmd.Flags().Changed("network") {
				return fmt.Errorf("cannot use --fork-network with --network; use akt network create to fork manually")
			}

			return m.UpdateContext(name, func(c *aktctx.Context) error {
				if cmd.Flags().Changed("network") {
					network, _ := cmd.Flags().GetString("network")
					c.Network = aktctx.Network{Name: network}
				}

				if cmd.Flags().Changed("keyring") {
					keyring, _ := cmd.Flags().GetString("keyring")
					c.Keyring = aktctx.Keyring{Name: keyring}
				}

				if cmd.Flags().Changed("default-account") {
					c.DefaultAccount, _ = cmd.Flags().GetString("default-account")
				}

				if cmd.Flags().Changed("gas") {
					c.Gas, _ = cmd.Flags().GetString("gas")
				}

				if cmd.Flags().Changed("fees") {
					c.Fees, _ = cmd.Flags().GetString("fees")
				}

				return nil
			})
		},
	}

	cmd.Flags().String("network", "", "Switch to a different network")
	cmd.Flags().String("keyring", "", "Switch to a different keyring")
	cmd.Flags().String("default-account", "", "Change default account")
	cmd.Flags().String("gas", "", "Change gas setting")
	cmd.Flags().String("fees", "", "Change fees setting")
	cmd.Flags().Bool("fork-network", false, "Fork the context's network before editing")

	return cmd
}

func deleteCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("Delete context %q? This removes the state store and action log. [y/N]: ", name)

				var answer string
				fmt.Scanln(&answer)

				if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			keepData, _ := cmd.Flags().GetBool("keep-data")

			return mgr().DeleteContext(name, keepData)
		},
	}

	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	cmd.Flags().Bool("keep-data", false, "Keep store and action log on disk")

	return cmd
}

func renameCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a context",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mgr().RenameContext(args[0], args[1])
		},
	}
}

func logCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "View the action log for the current context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := mgr()

			current := m.CurrentContext()
			if current == "" {
				return fmt.Errorf("no current context set")
			}

			logPath := aktctx.ActionLogPath(m.Root(), current)
			logger, err := actionlog.Open(logPath)
			if err != nil {
				return err
			}
			defer logger.Close()

			limit, _ := cmd.Flags().GetInt("limit")
			actionType, _ := cmd.Flags().GetString("type")
			since, _ := cmd.Flags().GetString("since")

			filter := actionlog.Filter{
				Limit: limit,
			}

			if actionType != "" {
				filter.Type = actionlog.ActionType(actionType)
			}

			if since != "" {
				d, err := time.ParseDuration(since)
				if err == nil {
					filter.Since = time.Now().UTC().Add(-d)
				} else {
					t, err := time.Parse(time.RFC3339, since)
					if err == nil {
						filter.Since = t
					} else {
						t, err := time.Parse("2006-01-02", since)
						if err != nil {
							return fmt.Errorf("invalid --since value %q: use a duration (1h) or date (2006-01-02)", since)
						}

						filter.Since = t
					}
				}
			}

			entries, err := logger.Read(filter)
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Println("No action log entries.")
				return nil
			}

			columns := []output.Column{
				{Header: "TIME"},
				{Header: "TYPE"},
				{Header: "ACTION"},
				{Header: "STATUS"},
			}

			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				ts := e.Timestamp.Format("2006-01-02 15:04:05")
				status := e.Status
				if e.Error != "" {
					status = "error: " + truncate(e.Error, 40)
				}

				rows = append(rows, []string{ts, string(e.Type), e.Action, status})
			}

			return output.PrintData(cmd, columns, rows, entries)
		},
	}

	cmd.Flags().Int("limit", 50, "Number of entries to show")
	cmd.Flags().String("type", "", "Filter by action type: tx, query, workflow, error")
	cmd.Flags().String("since", "", "Show entries since duration (1h) or date (2006-01-02)")

	return cmd
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}

	return s[:max-3] + "..."
}
