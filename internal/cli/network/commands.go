package network

import (
	"fmt"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
	"pkg.akt.dev/akt/internal/output/pretty"
)

// Commands returns the "network" command tree.
func Commands(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
		RunE:  sdkclient.ValidateCmd,
		Short: "Manage network definitions",
		Long:  "Networks define chain connectivity (chain-id, endpoints, gas-prices). They are shared resources that can be referenced by multiple contexts.",
	}

	cmd.AddCommand(
		createCmd(mgr),
		editCmd(mgr),
		deleteCmd(mgr),
		listCmd(mgr),
		showCmd(mgr),
	)

	return cmd
}

func createCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new network definition",
		Args:  cobra.ExactArgs(1),
		Example: `  # Create from built-in template
  akt context network create mainnet --template mainnet

  # Create a custom network
  akt context network create local --chain-id localnet-1 --rpc http://localhost:26657`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()
			name := args[0]

			template, _ := cmd.Flags().GetString(flagdefs.FlagTemplate)
			if template != "" {
				return m.CreateNetworkFromTemplate(name, template)
			}

			chainID, _ := cmd.Flags().GetString(flagdefs.FlagChainID)
			if chainID == "" {
				return fmt.Errorf("--chain-id is required when not using --template")
			}

			rpc, _ := cmd.Flags().GetStringSlice(flagdefs.FlagRPC)
			if len(rpc) == 0 {
				return fmt.Errorf("at least one --rpc endpoint is required")
			}

			api, _ := cmd.Flags().GetStringSlice(flagdefs.FlagAPI)
			grpc, _ := cmd.Flags().GetStringSlice(flagdefs.FlagGRPCEndpoint)
			gasPrices, _ := cmd.Flags().GetString(flagdefs.FlagGasPrices)
			gasAdjustment, _ := cmd.Flags().GetString(flagdefs.FlagGasAdjustment)

			net := aktctx.Network{
				Name:          name,
				ChainID:       chainID,
				Endpoints:     aktctx.Endpoints{RPC: rpc, API: api, GRPC: grpc},
				GasPrices:     gasPrices,
				GasAdjustment: gasAdjustment,
			}

			return m.CreateNetwork(net)
		},
	}

	cmd.Flags().String(flagdefs.FlagTemplate, "", "Use built-in template: mainnet, testnet, sandbox")
	cmd.Flags().String(flagdefs.FlagChainID, "", "Chain ID (required if no --template)")
	cmd.Flags().StringSlice(flagdefs.FlagRPC, nil, "RPC endpoint URLs")
	cmd.Flags().StringSlice(flagdefs.FlagAPI, nil, "REST API endpoint URLs")
	cmd.Flags().StringSlice(flagdefs.FlagGRPCEndpoint, nil, "gRPC endpoint URLs")
	cmd.Flags().String(flagdefs.FlagGasPrices, "0.025uakt", "Default gas prices")
	cmd.Flags().String(flagdefs.FlagGasAdjustment, "1.5", "Gas estimation multiplier")

	return cmd
}

func editCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "edit <name>",
		Short:             "Edit a network definition",
		Long:              "Changes apply to ALL contexts using this network.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeNetworkNames(mgr),
		Example: `  # Add a backup RPC endpoint
  akt context network edit mainnet --rpc https://rpc.akashnet.net:443,https://rpc-backup.example.com:443

  # Change gas prices
  akt context network edit mainnet --gas-prices 0.04uakt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()

			return m.UpdateNetwork(args[0], func(n *aktctx.Network) error {
				if cmd.Flags().Changed(flagdefs.FlagChainID) {
					n.ChainID, _ = cmd.Flags().GetString(flagdefs.FlagChainID)
				}

				if cmd.Flags().Changed(flagdefs.FlagRPC) {
					n.Endpoints.RPC, _ = cmd.Flags().GetStringSlice(flagdefs.FlagRPC)
				}

				if cmd.Flags().Changed(flagdefs.FlagAPI) {
					n.Endpoints.API, _ = cmd.Flags().GetStringSlice(flagdefs.FlagAPI)
				}

				if cmd.Flags().Changed(flagdefs.FlagGRPCEndpoint) {
					n.Endpoints.GRPC, _ = cmd.Flags().GetStringSlice(flagdefs.FlagGRPCEndpoint)
				}

				if cmd.Flags().Changed(flagdefs.FlagGasPrices) {
					n.GasPrices, _ = cmd.Flags().GetString(flagdefs.FlagGasPrices)
				}

				if cmd.Flags().Changed(flagdefs.FlagGasAdjustment) {
					n.GasAdjustment, _ = cmd.Flags().GetString(flagdefs.FlagGasAdjustment)
				}

				return nil
			})
		},
	}

	cmd.Flags().String(flagdefs.FlagChainID, "", "Chain ID")
	cmd.Flags().StringSlice(flagdefs.FlagRPC, nil, "RPC endpoint URLs")
	cmd.Flags().StringSlice(flagdefs.FlagAPI, nil, "REST API endpoint URLs")
	cmd.Flags().StringSlice(flagdefs.FlagGRPCEndpoint, nil, "gRPC endpoint URLs")
	cmd.Flags().String(flagdefs.FlagGasPrices, "", "Default gas prices")
	cmd.Flags().String(flagdefs.FlagGasAdjustment, "", "Gas estimation multiplier")

	return cmd
}

func deleteCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:               "delete <name>",
		Short:             "Delete a network definition",
		Long:              "Fails if any context references this network.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeNetworkNames(mgr),
		Example:           `  akt context network delete mainnet-custom`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mgr().DeleteNetwork(args[0])
		},
	}
}

func listCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List all networks",
		Args:    cobra.NoArgs,
		Example: `  akt context network list`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := mgr()
			nets := m.ListNetworks()

			// Structured output is a list, empty or not: an empty result
			// must not turn a JSON array into prose (SPEC §10.3).
			if f := output.FormatFromCmd(cmd); f != output.FormatTable {
				type networkRow struct {
					Name    string   `json:"name"     yaml:"name"`
					ChainID string   `json:"chain_id" yaml:"chain-id"`
					RPC     []string `json:"rpc"      yaml:"rpc"`
					UsedBy  []string `json:"used_by"  yaml:"used-by"`
				}

				data := make([]networkRow, 0, len(nets))
				for _, n := range nets {
					data = append(data, networkRow{
						Name:    n.Name,
						ChainID: n.ChainID,
						RPC:     n.Endpoints.RPC,
						UsedBy:  m.NetworkUsers(n.Name),
					})
				}

				return output.Fprint(cmd.OutOrStdout(), f, data)
			}

			if len(nets) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(),
					"No networks configured. Create one with: akt context network create <name> --template mainnet")
				return err
			}

			// Pretty output goes through the shared renderer, so the CLI and
			// the TUI network list stay identical (SPEC §10.8).
			_, err := fmt.Fprint(output.TerminalAwareWriter(cmd.OutOrStdout()), pretty.RenderNetworkList(nets, m.NetworkUsers))
			return err
		},
	}
}

func showCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:               "show <name>",
		Short:             "Show network details",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeNetworkNames(mgr),
		Example:           `  akt context network show mainnet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()
			net := m.GetNetwork(args[0])
			if net == nil {
				return fmt.Errorf("network %q not found", args[0])
			}

			if f := output.FormatFromCmd(cmd); f != output.FormatTable {
				return output.Fprint(cmd.OutOrStdout(), f, struct {
					Network any      `json:"network" yaml:"network"`
					UsedBy  []string `json:"usedBy"  yaml:"usedBy"`
				}{net, m.NetworkUsers(net.Name)})
			}

			_, err := fmt.Fprint(output.TerminalAwareWriter(cmd.OutOrStdout()), pretty.RenderNetworkShow(*net, m.NetworkUsers(net.Name)))
			return err
		},
	}
}
