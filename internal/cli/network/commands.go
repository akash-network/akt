package network

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
)

// Commands returns the "network" command tree.
func Commands(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
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
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()
			name := args[0]

			template, _ := cmd.Flags().GetString("template")
			if template != "" {
				return m.CreateNetworkFromTemplate(name, template)
			}

			chainID, _ := cmd.Flags().GetString("chain-id")
			if chainID == "" {
				return fmt.Errorf("--chain-id is required when not using --template")
			}

			rpc, _ := cmd.Flags().GetStringSlice("rpc")
			if len(rpc) == 0 {
				return fmt.Errorf("at least one --rpc endpoint is required")
			}

			api, _ := cmd.Flags().GetStringSlice("api")
			grpc, _ := cmd.Flags().GetStringSlice("grpc")
			gasPrices, _ := cmd.Flags().GetString("gas-prices")
			gasAdjustment, _ := cmd.Flags().GetString("gas-adjustment")

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

	cmd.Flags().String("template", "", "Use built-in template: mainnet, testnet, sandbox")
	cmd.Flags().String("chain-id", "", "Chain ID (required if no --template)")
	cmd.Flags().StringSlice("rpc", nil, "RPC endpoint URLs")
	cmd.Flags().StringSlice("api", nil, "REST API endpoint URLs")
	cmd.Flags().StringSlice("grpc", nil, "gRPC endpoint URLs")
	cmd.Flags().String("gas-prices", "0.025uakt", "Default gas prices")
	cmd.Flags().String("gas-adjustment", "1.5", "Gas estimation multiplier")

	return cmd
}

func editCmd(mgr func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a network definition",
		Long:  "Changes apply to ALL contexts using this network.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()

			return m.UpdateNetwork(args[0], func(n *aktctx.Network) error {
				if cmd.Flags().Changed("chain-id") {
					n.ChainID, _ = cmd.Flags().GetString("chain-id")
				}

				if cmd.Flags().Changed("rpc") {
					n.Endpoints.RPC, _ = cmd.Flags().GetStringSlice("rpc")
				}

				if cmd.Flags().Changed("api") {
					n.Endpoints.API, _ = cmd.Flags().GetStringSlice("api")
				}

				if cmd.Flags().Changed("grpc") {
					n.Endpoints.GRPC, _ = cmd.Flags().GetStringSlice("grpc")
				}

				if cmd.Flags().Changed("gas-prices") {
					n.GasPrices, _ = cmd.Flags().GetString("gas-prices")
				}

				if cmd.Flags().Changed("gas-adjustment") {
					n.GasAdjustment, _ = cmd.Flags().GetString("gas-adjustment")
				}

				return nil
			})
		},
	}

	cmd.Flags().String("chain-id", "", "Chain ID")
	cmd.Flags().StringSlice("rpc", nil, "RPC endpoint URLs")
	cmd.Flags().StringSlice("api", nil, "REST API endpoint URLs")
	cmd.Flags().StringSlice("grpc", nil, "gRPC endpoint URLs")
	cmd.Flags().String("gas-prices", "", "Default gas prices")
	cmd.Flags().String("gas-adjustment", "", "Gas estimation multiplier")

	return cmd
}

func deleteCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a network definition",
		Long:  "Fails if any context references this network.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mgr().DeleteNetwork(args[0])
		},
	}
}

func listCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all networks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := mgr()
			nets := m.ListNetworks()

			if len(nets) == 0 {
				fmt.Println("No networks configured. Create one with: akt network create <name> --template mainnet")
				return nil
			}

			type networkRow struct {
				Name    string   `json:"name"     yaml:"name"`
				ChainID string   `json:"chain_id" yaml:"chain-id"`
				RPC     []string `json:"rpc"      yaml:"rpc"`
				UsedBy  []string `json:"used_by"  yaml:"used-by"`
			}

			data := make([]networkRow, 0, len(nets))
			columns := []output.Column{
				{Header: "NAME"},
				{Header: "CHAIN-ID"},
				{Header: "RPC"},
				{Header: "USED BY"},
			}

			rows := make([][]string, 0, len(nets))
			for _, n := range nets {
				rpcDisplay := ""
				if len(n.Endpoints.RPC) > 0 {
					rpcDisplay = truncate(n.Endpoints.RPC[0], 40)
					if len(n.Endpoints.RPC) > 1 {
						rpcDisplay += fmt.Sprintf(" (+%d)", len(n.Endpoints.RPC)-1)
					}
				}

				users := m.NetworkUsers(n.Name)
				usedBy := "(none)"
				if len(users) > 0 {
					usedBy = strings.Join(users, ", ")
				}

				rows = append(rows, []string{n.Name, n.ChainID, rpcDisplay, usedBy})
				data = append(data, networkRow{
					Name:    n.Name,
					ChainID: n.ChainID,
					RPC:     n.Endpoints.RPC,
					UsedBy:  users,
				})
			}

			return output.PrintData(cmd, columns, rows, data)
		},
	}
}

func showCmd(mgr func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show network details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgr()
			net := m.GetNetwork(args[0])
			if net == nil {
				return fmt.Errorf("network %q not found", args[0])
			}

			fmt.Printf("Name:           %s\n", net.Name)
			fmt.Printf("Chain ID:       %s\n", net.ChainID)
			fmt.Printf("Gas Prices:     %s\n", net.GasPrices)
			fmt.Printf("Gas Adjustment: %s\n", net.GasAdjustment)

			fmt.Println("RPC Endpoints:")
			for _, e := range net.Endpoints.RPC {
				fmt.Printf("  - %s\n", e)
			}

			if len(net.Endpoints.API) > 0 {
				fmt.Println("API Endpoints:")
				for _, e := range net.Endpoints.API {
					fmt.Printf("  - %s\n", e)
				}
			}

			if len(net.Endpoints.GRPC) > 0 {
				fmt.Println("gRPC Endpoints:")
				for _, e := range net.Endpoints.GRPC {
					fmt.Printf("  - %s\n", e)
				}
			}

			users := m.NetworkUsers(net.Name)
			if len(users) > 0 {
				fmt.Printf("Used by:        %s\n", strings.Join(users, ", "))
			}

			return nil
		},
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}

	return s[:max-3] + "..."
}
