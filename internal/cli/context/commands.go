package context

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/capability"
	clikeys "pkg.akt.dev/akt/internal/cli/keys"
	clinetwork "pkg.akt.dev/akt/internal/cli/network"
	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
	"pkg.akt.dev/akt/internal/output/pretty"
)

// Commands returns the "context" command tree, including "network" and "keys" as subcommands.
func Commands(mgr func() *aktctx.Manager, getKeyring func() (sdkkeyring.Keyring, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		RunE:  sdkclient.ValidateCmd,
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
		Example: `  # Create a context using an existing network
  akt context create prod --network mainnet --default-account alice --set-current

  # Create a monitoring-only context (no default account)
  akt context create monitoring --network mainnet

  # Create a testnet context with a specific keyring
  akt context create staging --network testnet --keyring test-keyring --default-account testaccount`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()
			name := args[0]

			network, _ := cmd.Flags().GetString("network")
			keyring, _ := cmd.Flags().GetString("keyring")
			defaultAccount, _ := cmd.Flags().GetString("default-account")
			gas, _ := cmd.Flags().GetString("gas")
			fees, _ := cmd.Flags().GetString("fees")
			providerAuthType, _ := cmd.Flags().GetString("provider-auth-type")
			authMethod, _ := cmd.Flags().GetString("auth-method")
			consoleAPIURL, _ := cmd.Flags().GetString("console-api-url")
			consoleAPIKey, _ := cmd.Flags().GetString("console-api-key")
			setCurrent, _ := cmd.Flags().GetBool("set-current")

			// console-api contexts may be created without a network — they
			// operate through the Console API alone (chain commands are
			// capability-gated until a network is attached).
			if network == "" && authMethod != aktctx.AuthMethodConsoleAPI {
				return fmt.Errorf("--network is required unless --auth-method %s is used", aktctx.AuthMethodConsoleAPI)
			}

			ctx := aktctx.Context{
				Name:           name,
				Network:        aktctx.Network{Name: network},
				Keyring:        aktctx.Keyring{Name: keyring},
				AuthMethod:     authMethod,
				ConsoleAPIURL:  consoleAPIURL,
				DefaultAccount: defaultAccount,
				Gas:            gas,
				Fees:           fees,
				ProviderDefaults: aktctx.ProviderDefaults{
					AuthType: providerAuthType,
				},
			}

			if err := m.CreateContext(ctx); err != nil {
				return err
			}

			// The Console API key is a credential, stored per-context
			// outside config.yaml (SPEC §7.1) and never logged.
			if consoleAPIKey != "" {
				if err := aktctx.SetConsoleAPIKey(m.Root(), name, consoleAPIKey); err != nil {
					return err
				}
			}

			recordContextAction(m.Root(), name, "create", map[string]string{
				"network": network,
				"keyring": keyring,
			})

			if setCurrent {
				previous := m.CurrentContext()
				if err := m.UseContext(name); err != nil {
					return err
				}
				recordContextAction(m.Root(), name, "switch", map[string]string{
					"from": previous,
					"to":   name,
				})
			}

			return nil
		},
	}

	cmd.Flags().String("network", "", "Network name to use (required)")
	cmd.Flags().String("keyring", "default", "Keyring name to use")
	cmd.Flags().String("default-account", "", "Default account name")
	cmd.Flags().String("gas", "auto", "Gas limit override")
	cmd.Flags().String("fees", "", "Fixed fees override")
	cmd.Flags().String("provider-auth-type", aktctx.ProviderAuthJWT, "Provider gateway auth default: jwt or mtls")
	cmd.Flags().String("auth-method", "", "Authentication method: keyring (default) or console-api")
	cmd.Flags().String("console-api-url", "", "Console API base URL (empty = default; only with console-api auth)")
	cmd.Flags().String("console-api-key", "", "Console API key stored as a per-context credential (never written to config.yaml)")
	cmd.Flags().Bool("set-current", false, "Set as current context after creation")
	// --network is validated in RunE rather than MarkFlagRequired: console-api
	// contexts are allowed to omit it (network-less, Console-only operation).

	_ = cmd.RegisterFlagCompletionFunc("network", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		m := mgr()
		if m == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		nets := m.ListNetworks()
		names := make([]string, 0, len(nets))
		for _, n := range nets {
			names = append(names, n.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func useCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:               "use <name>",
		Short:             "Switch the active context",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeContextNames(mgr),
		Example:           `  akt context use staging`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()
			previous := m.CurrentContext()

			if err := m.UseContext(args[0]); err != nil {
				return err
			}

			recordContextAction(m.Root(), args[0], "switch", map[string]string{
				"from": previous,
				"to":   args[0],
			})

			return nil
		},
	}
}

func listCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List all contexts",
		Args:    cobra.NoArgs,
		Example: `  akt context list`,
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
		Use:     "show",
		Short:   "Show the active context with full details",
		Args:    cobra.NoArgs,
		Example: `  akt context show`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := mgr()

			rc, err := m.Resolve(activeContextFromCmd(cmd, m))
			if err != nil {
				return err
			}

			if f := output.FormatFromCmd(cmd); f != output.FormatTable {
				return output.Fprint(cmd.OutOrStdout(), f, newContextDetails(rc))
			}

			_, err = fmt.Fprint(output.TerminalAwareWriter(cmd.OutOrStdout()), pretty.RenderContextShow(*rc))
			return err
		},
	}
}

type contextCapabilities struct {
	ChainQuery bool `json:"chain_query" yaml:"chain_query"`
	ChainTx    bool `json:"chain_tx"    yaml:"chain_tx"`
	Provider   bool `json:"provider"    yaml:"provider"`
	Console    bool `json:"console"     yaml:"console"`
}

type contextDetails struct {
	Name                    string                  `json:"name"                       yaml:"name"`
	Network                 aktctx.Network          `json:"network"                    yaml:"network"`
	Keyring                 aktctx.Keyring          `json:"keyring"                    yaml:"keyring"`
	AuthMethod              string                  `json:"auth_method"                yaml:"auth_method"`
	ConsoleAPIURL           string                  `json:"console_api_url"            yaml:"console_api_url"`
	ConsoleAPIKeyConfigured bool                    `json:"console_api_key_configured" yaml:"console_api_key_configured"`
	DefaultAccount          string                  `json:"default_account,omitempty"  yaml:"default_account,omitempty"`
	Gas                     string                  `json:"gas"                        yaml:"gas"`
	Fees                    string                  `json:"fees,omitempty"             yaml:"fees,omitempty"`
	GasPrices               string                  `json:"gas_prices"                 yaml:"gas_prices"`
	GasAdjustment           string                  `json:"gas_adjustment"             yaml:"gas_adjustment"`
	ProviderDefaults        aktctx.ProviderDefaults `json:"provider_defaults"          yaml:"provider_defaults"`
	ProviderAuth            string                  `json:"provider_auth"              yaml:"provider_auth"`
	StorePath               string                  `json:"store_path"                 yaml:"store_path"`
	ActionLogPath           string                  `json:"action_log_path"            yaml:"action_log_path"`
	Capabilities            contextCapabilities     `json:"capabilities"               yaml:"capabilities"`
}

func newContextDetails(rc *aktctx.Context) contextDetails {
	set := capability.Resolve(rc)
	return contextDetails{
		Name:                    rc.Name,
		Network:                 rc.Network,
		Keyring:                 rc.Keyring,
		AuthMethod:              rc.AuthMethod,
		ConsoleAPIURL:           rc.ConsoleAPIURL,
		ConsoleAPIKeyConfigured: rc.ConsoleAPIKey != "",
		DefaultAccount:          rc.DefaultAccount,
		Gas:                     rc.Gas,
		Fees:                    rc.Fees,
		GasPrices:               rc.GasPrices,
		GasAdjustment:           rc.GasAdjustment,
		ProviderDefaults:        rc.ProviderDefaults,
		ProviderAuth:            rc.AuthType,
		StorePath:               aktctx.StoreDir(rc.Root, rc.Name),
		ActionLogPath:           aktctx.ActionLogPath(rc.Root, rc.Name),
		Capabilities: contextCapabilities{
			ChainQuery: set.ChainQuery,
			ChainTx:    set.ChainTx,
			Provider:   set.Provider,
			Console:    set.Console,
		},
	}
}

func activeContextFromCmd(cmd *cobra.Command, m *aktctx.Manager) string {
	return cliutil.SelectedContextName(cmd, m)
}

func editCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "edit <name>",
		Short:             "Edit a context",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeContextNames(mgr),
		Example: `  # Change default account
  akt context edit prod --default-account bob

  # Switch to a different network
  akt context edit staging --network sandbox`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()
			name := args[0]

			forkNetwork, _ := cmd.Flags().GetBool("fork-network")
			if forkNetwork && cmd.Flags().Changed("network") {
				return fmt.Errorf("cannot use --fork-network with --network; use akt context network create to fork manually")
			}
			networkFields := []string{"rpc", "api", "grpc", "gas-prices", "gas-adjustment"}
			hasNetworkChanges := false
			for _, field := range networkFields {
				if cmd.Flags().Changed(field) {
					hasNetworkChanges = true
					break
				}
			}
			if forkNetwork && !hasNetworkChanges {
				return fmt.Errorf("--fork-network requires at least one network field: --rpc, --api, --grpc, --gas-prices, or --gas-adjustment")
			}
			yes, _ := cmd.Flags().GetBool("yes")
			if yes && !hasNetworkChanges {
				return fmt.Errorf("--yes is only used when editing shared network fields")
			}

			current := m.GetContext(name)
			if current == nil {
				return fmt.Errorf("context %q not found", name)
			}
			targetNetwork := current.Network.Name
			if cmd.Flags().Changed("network") {
				targetNetwork, _ = cmd.Flags().GetString("network")
			}
			if hasNetworkChanges && targetNetwork == "" {
				return fmt.Errorf("context %q has no network to edit", name)
			}

			if hasNetworkChanges && !forkNetwork && !yes && networkSharedAfterContextUpdate(m, targetNetwork, name) && stdinIsTerminal(cmd) {
				selectedFork, err := promptNetworkEditMode(cmd, targetNetwork, name)
				if err != nil {
					return err
				}
				forkNetwork = selectedFork
			}

			forkName := ""
			if forkNetwork {
				forkName = targetNetwork + "-" + name
			}

			changed := map[string]string{}

			applyContext := func(c *aktctx.Context) error {
				if cmd.Flags().Changed("network") {
					network, _ := cmd.Flags().GetString("network")
					changed["network"] = network
					c.Network = aktctx.Network{Name: network}
				}

				if cmd.Flags().Changed("keyring") {
					keyring, _ := cmd.Flags().GetString("keyring")
					changed["keyring"] = keyring
					c.Keyring = aktctx.Keyring{Name: keyring}
				}

				if cmd.Flags().Changed("default-account") {
					c.DefaultAccount, _ = cmd.Flags().GetString("default-account")
					changed["default-account"] = c.DefaultAccount
				}

				if cmd.Flags().Changed("gas") {
					c.Gas, _ = cmd.Flags().GetString("gas")
					changed["gas"] = c.Gas
				}

				if cmd.Flags().Changed("fees") {
					c.Fees, _ = cmd.Flags().GetString("fees")
					changed["fees"] = c.Fees
				}

				if cmd.Flags().Changed("provider-auth-type") {
					c.ProviderDefaults.AuthType, _ = cmd.Flags().GetString("provider-auth-type")
					changed["provider-auth-type"] = c.ProviderDefaults.AuthType
				}

				if cmd.Flags().Changed("auth-method") {
					method, _ := cmd.Flags().GetString("auth-method")
					if method != aktctx.AuthMethodKeyring && method != aktctx.AuthMethodConsoleAPI {
						return fmt.Errorf("invalid auth-method %q: must be %q or %q", method, aktctx.AuthMethodKeyring, aktctx.AuthMethodConsoleAPI)
					}
					c.AuthMethod = method
					changed["auth-method"] = method
				}

				if cmd.Flags().Changed("console-api-url") {
					c.ConsoleAPIURL, _ = cmd.Flags().GetString("console-api-url")
					changed["console-api-url"] = c.ConsoleAPIURL
				}

				return nil
			}

			var applyNetwork func(*aktctx.Network) error
			if hasNetworkChanges {
				applyNetwork = func(network *aktctx.Network) error {
					if cmd.Flags().Changed("rpc") {
						network.Endpoints.RPC, _ = cmd.Flags().GetStringSlice("rpc")
						changed["rpc"] = strings.Join(network.Endpoints.RPC, ",")
					}
					if cmd.Flags().Changed("api") {
						network.Endpoints.API, _ = cmd.Flags().GetStringSlice("api")
						changed["api"] = strings.Join(network.Endpoints.API, ",")
					}
					if cmd.Flags().Changed("grpc") {
						network.Endpoints.GRPC, _ = cmd.Flags().GetStringSlice("grpc")
						changed["grpc"] = strings.Join(network.Endpoints.GRPC, ",")
					}
					if cmd.Flags().Changed("gas-prices") {
						network.GasPrices, _ = cmd.Flags().GetString("gas-prices")
						changed["gas-prices"] = network.GasPrices
					}
					if cmd.Flags().Changed("gas-adjustment") {
						network.GasAdjustment, _ = cmd.Flags().GetString("gas-adjustment")
						changed["gas-adjustment"] = network.GasAdjustment
					}
					return nil
				}
			}

			if err := m.UpdateContextAndNetwork(name, forkName, applyContext, applyNetwork); err != nil {
				return err
			}
			if forkName != "" {
				changed["network"] = forkName
				changed["fork-network"] = targetNetwork
			}

			// The credential is stored outside config.yaml (SPEC §7.1);
			// only "updated"/"removed" is recorded, never the key itself.
			if cmd.Flags().Changed("console-api-key") {
				key, _ := cmd.Flags().GetString("console-api-key")
				if err := aktctx.SetConsoleAPIKey(m.Root(), name, key); err != nil {
					return err
				}

				if key == "" {
					changed["console-api-key"] = "removed"
				} else {
					changed["console-api-key"] = "updated"
				}
			}

			recordContextAction(m.Root(), name, "edit", changed)

			return nil
		},
	}

	cmd.Flags().String("network", "", "Switch to a different network")
	cmd.Flags().String("keyring", "", "Switch to a different keyring")
	cmd.Flags().String("default-account", "", "Change default account")
	cmd.Flags().String("gas", "", "Change gas setting")
	cmd.Flags().String("fees", "", "Change fees setting")
	cmd.Flags().String("provider-auth-type", "", "Change provider gateway auth default: jwt or mtls")
	cmd.Flags().String("auth-method", "", "Change authentication method: keyring or console-api")
	cmd.Flags().String("console-api-url", "", "Change Console API base URL (empty = default)")
	cmd.Flags().String("console-api-key", "", "Set the per-context Console API key (empty string removes it)")
	cmd.Flags().Bool("fork-network", false, "Fork the context's network before editing")
	cmd.Flags().StringSlice("rpc", nil, "Replace the selected network's RPC endpoints")
	cmd.Flags().StringSlice("api", nil, "Replace the selected network's REST endpoints")
	cmd.Flags().StringSlice("grpc", nil, "Replace the selected network's gRPC endpoints")
	cmd.Flags().String("gas-prices", "", "Change the selected network's gas prices")
	cmd.Flags().String("gas-adjustment", "", "Change the selected network's gas adjustment")
	cmd.Flags().BoolP("yes", "y", false, "Edit a shared parent network without prompting")
	cmd.MarkFlagsMutuallyExclusive("fork-network", "yes")

	_ = cmd.RegisterFlagCompletionFunc("network", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		m := mgr()
		if m == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		nets := m.ListNetworks()
		names := make([]string, 0, len(nets))
		for _, n := range nets {
			names = append(names, n.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func networkSharedAfterContextUpdate(m *aktctx.Manager, network, contextName string) bool {
	for _, context := range m.ListContexts() {
		if context.Name != contextName && context.Network.Name == network {
			return true
		}
	}
	return false
}

func stdinIsTerminal(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func promptNetworkEditMode(cmd *cobra.Command, network, contextName string) (bool, error) {
	forkName := network + "-" + contextName
	fmt.Fprintf(cmd.ErrOrStderr(), "Network %q is shared.\n  1. Edit parent — change applies to every context using %q\n  2. Fork — create %q for context %q only\nSelect [1-2]: ", network, network, forkName, contextName)

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false, fmt.Errorf("read network edit selection: %w", err)
	}
	switch strings.TrimSpace(line) {
	case "", "1":
		return false, nil
	case "2":
		return true, nil
	default:
		return false, fmt.Errorf("invalid selection %q: enter 1 to edit the parent or 2 to fork", strings.TrimSpace(line))
	}
}

func deleteCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete <name>",
		Short:             "Delete a context",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeContextNames(mgr),
		Example: `  # Delete with confirmation prompt
  akt context delete staging

  # Skip confirmation
  akt context delete staging --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("Delete context %q? This removes the state store and action log. [y/N]: ", name)

				var answer string
				_, _ = fmt.Scanln(&answer)

				if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			keepData, _ := cmd.Flags().GetBool("keep-data")

			m := mgr()
			if err := m.DeleteContext(name, keepData); err != nil {
				return err
			}

			// The deleted context's log is gone (unless --keep-data), so
			// record the deletion in the current context's log if one exists.
			if current := m.CurrentContext(); current != "" && current != name {
				recordContextAction(m.Root(), current, "delete", map[string]string{
					"context": name,
				})
			}

			return nil
		},
	}

	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	cmd.Flags().Bool("keep-data", false, "Keep store and action log on disk")

	return cmd
}

func renameCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:               "rename <old> <new>",
		Short:             "Rename a context",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeContextNames(mgr),
		Example:           `  akt context rename staging testnet-staging`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()

			if err := m.RenameContext(args[0], args[1]); err != nil {
				return err
			}

			recordContextAction(m.Root(), args[1], "rename", map[string]string{
				"from": args[0],
				"to":   args[1],
			})

			return nil
		},
	}
}

func logCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "View the action log for the current context",
		Args:  cobra.NoArgs,
		Example: `  # Show last 50 entries (default)
  akt context log

  # Show only transaction entries from the last hour
  akt context log --type tx --since 1h

  # Show last 10 entries
  akt context log --limit 10`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := mgr()

			current := activeContextFromCmd(cmd, m)
			if current == "" {
				return fmt.Errorf("no current context set")
			}
			resolved, err := m.Resolve(current)
			if err != nil {
				return err
			}
			current = resolved.Name

			logPath := aktctx.ActionLogPath(m.Root(), current)
			logger, err := actionlog.Open(logPath)
			if err != nil {
				return err
			}
			defer func() { _ = logger.Close() }()

			limit, _ := cmd.Flags().GetInt("limit")
			actionType, _ := cmd.Flags().GetString("type")
			since, _ := cmd.Flags().GetString("since")
			if !validActionType(actionType) {
				return fmt.Errorf("invalid --type %q: must be tx, workflow, provider, context, console, or error", actionType)
			}

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

			// The human hint is only correct for the table renderer. Emitting
			// it under -o json handed a JSON consumer a line of prose at exit
			// 0, so a script that worked on a non-empty log crashed on an
			// empty one.
			if len(entries) == 0 {
				if output.FormatFromCmd(cmd) != output.FormatTable {
					return output.Print(output.FormatFromCmd(cmd), []struct{}{})
				}

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
	cmd.Flags().String("type", "", "Filter by action type: tx, workflow, provider, context, console, error")
	cmd.Flags().String("since", "", "Show entries since duration (1h) or date (2006-01-02)")

	return cmd
}

func validActionType(value string) bool {
	switch actionlog.ActionType(value) {
	case "", actionlog.TypeTx, actionlog.TypeWorkflow, actionlog.TypeProvider, actionlog.TypeContext, actionlog.TypeConsole, actionlog.TypeError:
		return true
	default:
		return false
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen-3] + "..."
}
