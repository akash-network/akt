package console

import (
	"fmt"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func walletCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wallet",
		Short: "Managed wallet balance, settings, and cost",
	}

	cmd.AddCommand(
		walletListCmd(mgrFn),
		walletBalanceCmd(mgrFn),
		walletSettingsCmd(mgrFn),
		walletCostCmd(mgrFn),
	)

	return cmd
}

func walletListCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List managed wallets",
		Args:    cobra.NoArgs,
		Example: `  akt console wallet list`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			user, err := cl.GetUser(cmd.Context())
			if err != nil {
				return fmt.Errorf("get user: %w", err)
			}

			wallets, err := cl.ListWallets(cmd.Context(), user.ID)
			if err != nil {
				return fmt.Errorf("list wallets: %w", err)
			}

			type walletRow struct {
				Address  string `json:"address"`
				Balance  string `json:"balance"`
				Trialing bool   `json:"trialing"`
			}

			rows := make([]walletRow, 0, len(wallets))
			for _, w := range wallets {
				rows = append(rows, walletRow{
					Address:  w.Address,
					Balance:  formatUSD(w.CreditAmount / 1e6), // µACT -> USD (1 ACT = 1 USD)
					Trialing: w.IsTrialing,
				})
			}

			return printJSON(cmd, rows)
		},
	}
}

func walletBalanceCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "balance",
		Short:   "Available / in-deployment / total balance in USD",
		Args:    cobra.NoArgs,
		Example: `  akt console wallet balance`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			b, err := cl.GetBalances(cmd.Context())
			if err != nil {
				return fmt.Errorf("get balances: %w", err)
			}

			return printJSON(cmd, struct {
				Available     string `json:"available"`
				InDeployments string `json:"inDeployments"`
				Total         string `json:"total"`
			}{formatUSD(b.BalanceUSD()), formatUSD(b.DeploymentsUSD()), formatUSD(b.TotalUSD())})
		},
	}
}

func walletSettingsCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings [true|false]",
		Short: "View or change wallet settings",
		Args:  cobra.MaximumNArgs(1),
		Example: `  # Show current settings
  akt console wallet settings

  # Enable automatic top-up (positional; --auto-reload works too)
  akt console wallet settings true`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			if len(args) > 0 || cmd.Flags().Changed("auto-reload") {
				value, _ := cmd.Flags().GetString("auto-reload")
				if len(args) > 0 {
					value = args[0]
				}

				enabled, err := parseBoolValue(value, "auto-reload")
				if err != nil {
					return err
				}

				settings, err := cl.UpdateWalletSettings(cmd.Context(), enabled)
				if err != nil {
					return fmt.Errorf("update wallet settings: %w", err)
				}

				return printJSON(cmd, settings)
			}

			settings, err := cl.GetWalletSettings(cmd.Context())
			if err != nil {
				return fmt.Errorf("get wallet settings: %w", err)
			}

			return printJSON(cmd, settings)
		},
	}

	cmd.Flags().String("auto-reload", "", "Enable or disable automatic top-up (true|false)")

	return cmd
}

func walletCostCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "cost",
		Short:   "Estimated weekly cost in USD",
		Args:    cobra.NoArgs,
		Example: `  akt console wallet cost`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			cost, err := cl.GetWeeklyCost(cmd.Context())
			if err != nil {
				return fmt.Errorf("get weekly cost: %w", err)
			}

			return printJSON(cmd, struct {
				WeeklyCost string `json:"weeklyCost"`
			}{formatUSD(cost)})
		},
	}
}

func usageCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage [from] [to]",
		Short: "Historical spend and active-deployment counts",
		Args:  cobra.MaximumNArgs(2),
		Example: `  # Last 30 days (API default)
  akt console usage

  # Explicit range, positional (flags work too and positionals win)
  akt console usage 2026-01-01 2026-01-31`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			if len(args) > 0 {
				from = args[0]
			}
			if len(args) > 1 {
				to = args[1]
			}

			// Usage history is keyed by wallet address: resolve the user's
			// first managed wallet with an on-chain address.
			user, err := cl.GetUser(cmd.Context())
			if err != nil {
				return fmt.Errorf("get user: %w", err)
			}

			wallets, err := cl.ListWallets(cmd.Context(), user.ID)
			if err != nil {
				return fmt.Errorf("list wallets: %w", err)
			}

			address := ""
			for _, w := range wallets {
				if w.Address != "" {
					address = w.Address
					break
				}
			}
			if address == "" {
				return fmt.Errorf("no managed wallet with an on-chain address was found")
			}

			points, err := cl.GetUsageHistory(cmd.Context(), address, from, to)
			if err != nil {
				return fmt.Errorf("get usage history: %w", err)
			}

			type usageRow struct {
				Date        string `json:"date"`
				Deployments int    `json:"deployments"`
				Spent       string `json:"spent"`
			}

			totalSpent := 0.0
			rows := make([]usageRow, 0, len(points))
			for _, p := range points {
				totalSpent = p.TotalUsdcSpent
				rows = append(rows, usageRow{p.Date, p.ActiveDeployments, formatUSD(p.DailyUsdcSpent)})
			}

			return printJSON(cmd, struct {
				TotalSpent string     `json:"totalSpent"`
				Days       int        `json:"days"`
				History    []usageRow `json:"history"`
			}{formatUSD(totalSpent), len(rows), rows})
		},
	}

	cmd.Flags().String("from", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().String("to", "", "End date (YYYY-MM-DD)")

	return cmd
}
