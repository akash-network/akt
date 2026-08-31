package console

import (
	"errors"
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
)

func walletCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wallet",
		RunE:  sdkclient.ValidateCmd,
		Short: "Managed wallet balance, settings, and cost",
	}

	cmd.AddCommand(
		walletListCmd(mgrFn),
		walletAddressCmd(mgrFn),
		walletBalanceCmd(mgrFn),
		walletSettingsCmd(mgrFn),
		walletCostCmd(mgrFn),
	)

	return cmd
}

func walletAddressCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "address",
		Short:   "Print the managed wallet blockchain address",
		Args:    cobra.NoArgs,
		Example: `  akt console wallet address`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			address, err := cl.ManagedWalletAddress(cmd.Context())
			if err != nil {
				return err
			}

			return printJSON(cmd, struct {
				Address string `json:"address"`
			}{Address: address})
		},
	}
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
				Denom    string `json:"denom,omitempty"`
				Trialing bool   `json:"trialing"`
			}

			rows := make([]walletRow, 0, len(wallets))
			for _, w := range wallets {
				rows = append(rows, walletRow{
					Address:  w.Address,
					Balance:  formatUSD(w.CreditUSD()),
					Denom:    w.Denom,
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
				Available        string `json:"available"`
				InDeployments    string `json:"inDeployments"`
				Total            string `json:"total"`
				AllocationStatus string `json:"allocationStatus"`
				AllocationNote   string `json:"allocationNote"`
			}{
				Available:        formatUSD(b.BalanceUSD()),
				InDeployments:    formatUSD(b.DeploymentsUSD()),
				Total:            formatUSD(b.TotalUSD()),
				AllocationStatus: "provisional",
				AllocationNote:   "available and in-deployment allocations may lag recent creates and closes; total is authoritative",
			})
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

  # Enable automatic top-up
  akt console wallet settings true`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			// FEEDBACK(2026-07): --auto-reload disabled for the
			// positional-only UX trial; the positional [true|false] argument
			// is the only source. Restore by uncommenting if users ask for
			// the flag form back.
			// if len(args) > 0 || cmd.Flags().Changed("auto-reload") {
			// 	value, _ := cmd.Flags().GetString("auto-reload")
			// 	if len(args) > 0 {
			// 		value = args[0]
			// 	}
			if len(args) > 0 {
				value := args[0]

				enabled, err := parseBoolValue(value, "auto-reload")
				if err != nil {
					return err
				}

				settings, err := cl.UpdateWalletSettings(cmd.Context(), enabled)
				if err != nil {
					return fmt.Errorf("update wallet settings: %w", err)
				}

				return printJSON(cmd, renderWalletSettings(settings))
			}

			settings, err := cl.GetWalletSettings(cmd.Context())
			// An account that has never configured auto-reload has no
			// settings record, which the API reports as 404. That is the
			// normal state, not a failure -- report the defaults rather than
			// exiting non-zero.
			if errors.Is(err, console.ErrNotFound) {
				return printJSON(cmd, struct {
					AutoReloadEnabled bool   `json:"autoReloadEnabled"`
					Configured        bool   `json:"configured"`
					Note              string `json:"note"`
				}{false, false, "no wallet settings configured; enable auto-reload with `akt console wallet settings true`"})
			}
			if err != nil {
				return fmt.Errorf("get wallet settings: %w", err)
			}

			return printJSON(cmd, renderWalletSettings(settings))
		},
	}

	// FEEDBACK(2026-07): --auto-reload disabled for the positional-only UX
	// trial (use the positional form instead). Restore by uncommenting if
	// users ask for the flag form back.
	// cmd.Flags().String("auto-reload", "", "Enable or disable automatic top-up (true|false)")

	return cmd
}

// renderWalletSettings formats wallet settings for display, in the same shape
// the never-configured (404) branch reports, so `wallet settings` answers with
// one object whatever path produced it — the sibling `deployment settings`
// does the same through renderSettings.
func renderWalletSettings(s *console.WalletSettings) any {
	return struct {
		AutoReloadEnabled bool `json:"autoReloadEnabled"`
		Configured        bool `json:"configured"`
	}{
		AutoReloadEnabled: s.AutoReloadEnabled,
		Configured:        true,
	}
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

  # Explicit range (positional dates)
  akt console usage 2026-01-01 2026-01-31`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			// FEEDBACK(2026-07): --from/--to disabled for the positional-only
			// UX trial; the positional [from] [to] dates are the only source.
			// Restore by uncommenting if users ask for the flag form back.
			// from, _ := cmd.Flags().GetString("from")
			// to, _ := cmd.Flags().GetString("to")
			from, to := "", ""
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

			// totalSpent is the spend within the requested range: the sum
			// of the per-day values shown in the rows. TotalUsdcSpent is
			// the API's lifetime figure ("cumulative spent up to this
			// date" per the vendored contract), so the range maximum — not
			// whichever element happens to come last — is the lifetime
			// spend as of the range end, independent of point ordering.
			totalSpent := 0.0
			lifetimeSpent := 0.0
			rows := make([]usageRow, 0, len(points))
			for _, p := range points {
				totalSpent += p.DailyUsdcSpent
				if p.TotalUsdcSpent > lifetimeSpent {
					lifetimeSpent = p.TotalUsdcSpent
				}
				rows = append(rows, usageRow{p.Date, p.ActiveDeployments, formatUSD(p.DailyUsdcSpent)})
			}

			lifetime := ""
			if len(points) > 0 {
				lifetime = formatUSD(lifetimeSpent)
			}

			return printJSON(cmd, struct {
				TotalSpent    string     `json:"totalSpent"`
				LifetimeSpent string     `json:"lifetimeSpent,omitempty"`
				Days          int        `json:"days"`
				History       []usageRow `json:"history"`
			}{formatUSD(totalSpent), lifetime, len(rows), rows})
		},
	}

	// FEEDBACK(2026-07): --from disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().String("from", "", "Start date (YYYY-MM-DD)")
	// FEEDBACK(2026-07): --to disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().String("to", "", "End date (YYYY-MM-DD)")

	return cmd
}
