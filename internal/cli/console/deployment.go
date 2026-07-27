package console

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/transport"
)

// parseConsoleUSD parses a positional deposit/amount argument using the
// unified cross-rail deposit syntax (transport.ParseDeposit, SPEC §7.4):
// bare numbers, "5usd", and "$5" are USD on the console rail; coin forms
// ("5000000uakt") fail with the transport package's cross-rail error before
// any request is made.
func parseConsoleUSD(arg string) (float64, error) {
	dep, err := transport.ParseDeposit(arg)
	if err != nil {
		return 0, err
	}

	if _, err := dep.RailValue(transport.KindConsole); err != nil {
		return 0, err
	}

	// ""/"auto" defers to the rail default, but the console rail has none:
	// callers' minimum/positivity checks reject the zero value with a
	// message pointing at the positional argument.
	return dep.USD, nil
}

func deploymentCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "Manage Console deployments",
		Long:  "Create, inspect, update, fund, and close deployments through the Console managed wallet.",
	}

	cmd.AddCommand(
		deploymentListCmd(mgrFn),
		deploymentGetCmd(mgrFn),
		deploymentCreateCmd(mgrFn),
		deploymentUpdateCmd(mgrFn),
		deploymentCloseCmd(mgrFn),
		deploymentDepositCmd(mgrFn),
		deploymentSettingsCmd(mgrFn),
	)

	return cmd
}

func deploymentListCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List deployments",
		Args:    cobra.NoArgs,
		Example: `  akt console deployment list --limit 10`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			skip, _ := cmd.Flags().GetInt("skip")
			limit, _ := cmd.Flags().GetInt("limit")

			list, err := cl.ListDeployments(cmd.Context(), skip, limit)
			if err != nil {
				return fmt.Errorf("list deployments: %w", err)
			}

			return printJSON(cmd, list)
		},
	}

	cmd.Flags().Int("skip", 0, "Pagination offset")
	cmd.Flags().Int("limit", 20, "Page size")

	return cmd
}

func deploymentGetCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "get <dseq>",
		Short:   "Show a deployment with its leases and escrow account",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console deployment get 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			detail, err := cl.GetDeployment(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get deployment %s: %w", args[0], err)
			}

			return printJSON(cmd, detail)
		},
	}
}

func deploymentCreateCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <sdl-file> [deposit-usd]",
		Short: "Create a deployment (managed wallet signs server-side)",
		Long: "Create a deployment from an SDL file with a USD deposit. The returned manifest " +
			"is cached per-context so `akt console lease create` can send it without re-passing it.",
		Args: cobra.RangeArgs(1, 2),
		Example: `  # Deposit as positional argument
  akt console deployment create deploy.yaml 5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, rc, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			// FEEDBACK(2026-07): --deposit disabled for the positional-only
			// UX trial; the positional [deposit-usd] argument is the only
			// source (zero fallback). Restore by uncommenting if users ask
			// for the flag form back.
			// deposit, _ := cmd.Flags().GetFloat64("deposit")
			deposit := 0.0
			if len(args) > 1 {
				deposit, err = parseConsoleUSD(args[1])
				if err != nil {
					return err
				}
			}
			// NOTE: internal/workflow/adapters/console.go carries a private
			// copy of this minimum (minConsoleDepositUSD); it should switch
			// to the shared transport.MinConsoleDepositUSD constant.
			if deposit < transport.MinConsoleDepositUSD {
				return fmt.Errorf("deposit must be at least %s (got %s): pass it as the [deposit-usd] argument", formatUSD(transport.MinConsoleDepositUSD), formatUSD(deposit))
			}

			sdl, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read SDL file: %w", err)
			}

			result, err := cl.CreateDeployment(cmd.Context(), string(sdl), deposit)
			if err != nil {
				return fmt.Errorf("create deployment: %w", err)
			}

			// Cache the manifest so `lease create` can default to it.
			note := ""
			switch {
			case rc == nil:
				note = "manifest not cached (no active context); pass --manifest to `lease create`"
			case result.Manifest != "":
				if err := console.SaveManifest(rc.Root, rc.Name, result.DSeq.String(), result.Manifest); err != nil {
					note = fmt.Sprintf("manifest not cached: %v", err)
				}
			}

			txHash := ""
			if result.SignTx != nil {
				txHash = result.SignTx.TransactionHash
			}

			return printJSON(cmd, struct {
				DSeq   string `json:"dseq"`
				TxHash string `json:"txHash,omitempty"`
				State  string `json:"state"`
				Note   string `json:"note,omitempty"`
			}{result.DSeq.String(), txHash, "open", note})
		},
	}

	// FEEDBACK(2026-07): --deposit disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().Float64("deposit", 0, "Deposit amount in USD (minimum 0.5); alternative to the positional argument")

	return cmd
}

func deploymentUpdateCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "update <dseq> <sdl-file>",
		Short:   "Update a deployment's SDL",
		Args:    cobra.ExactArgs(2),
		Example: `  akt console deployment update 12345 deploy.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			sdl, err := os.ReadFile(args[1])
			if err != nil {
				return fmt.Errorf("read SDL file: %w", err)
			}

			detail, err := cl.UpdateDeployment(cmd.Context(), args[0], string(sdl))
			if err != nil {
				return fmt.Errorf("update deployment %s: %w", args[0], err)
			}

			return printJSON(cmd, detail)
		},
	}
}

func deploymentCloseCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "close <dseq>",
		Short:   "Close a deployment (idempotent: already-closed is a no-op)",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console deployment close 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			err = cl.CloseDeployment(cmd.Context(), args[0])
			switch {
			case err == nil:
				fmt.Fprintf(cmd.OutOrStdout(), "Deployment %s closed.\n", args[0])
				return nil

			case errors.Is(err, console.ErrAlreadyClosed):
				fmt.Fprintf(cmd.OutOrStdout(), "Deployment %s already closed.\n", args[0])
				return nil

			default:
				return fmt.Errorf("close deployment %s: %w", args[0], err)
			}
		},
	}
}

func deploymentDepositCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deposit <dseq> [amount-usd]",
		Short: "Add funds to a deployment's escrow",
		Args:  cobra.RangeArgs(1, 2),
		Example: `  # Amount as positional argument
  akt console deployment deposit 12345 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			// FEEDBACK(2026-07): --amount disabled for the positional-only
			// UX trial; the positional [amount-usd] argument is the only
			// source (zero fallback). Restore by uncommenting if users ask
			// for the flag form back.
			// amount, _ := cmd.Flags().GetFloat64("amount")
			amount := 0.0
			if len(args) > 1 {
				amount, err = parseConsoleUSD(args[1])
				if err != nil {
					return err
				}
			}
			if amount <= 0 {
				return fmt.Errorf("amount must be a positive USD amount (got %s): pass it as the [amount-usd] argument", formatUSD(amount))
			}

			if err := cl.Deposit(cmd.Context(), args[0], amount); err != nil {
				return fmt.Errorf("deposit to deployment %s: %w", args[0], err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deposited %s to deployment %s.\n", formatUSD(amount), args[0])
			return nil
		},
	}

	// FEEDBACK(2026-07): --amount disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().Float64("amount", 0, "Amount to add in USD; alternative to the positional argument")

	return cmd
}

func deploymentSettingsCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings <dseq> [true|false]",
		Short: "View or change a deployment's auto-top-up setting",
		Args:  cobra.RangeArgs(1, 2),
		Example: `  # Show current settings
  akt console deployment settings 12345

  # Enable auto-top-up
  akt console deployment settings 12345 true`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			// FEEDBACK(2026-07): --auto-top-up disabled for the
			// positional-only UX trial; the positional [true|false] argument
			// is the only source. Restore by uncommenting if users ask for
			// the flag form back.
			// if len(args) > 1 || cmd.Flags().Changed("auto-top-up") {
			// 	value, _ := cmd.Flags().GetString("auto-top-up")
			// 	if len(args) > 1 {
			// 		value = args[1]
			// 	}
			if len(args) > 1 {
				value := args[1]

				enabled, err := parseBoolValue(value, "auto-top-up")
				if err != nil {
					return err
				}

				settings, err := cl.SetDeploymentAutoTopUp(cmd.Context(), args[0], enabled)
				if err != nil {
					return fmt.Errorf("update deployment settings for %s: %w", args[0], err)
				}

				return printJSON(cmd, settings)
			}

			settings, err := cl.GetDeploymentSettings(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get deployment settings for %s: %w", args[0], err)
			}

			return printJSON(cmd, settings)
		},
	}

	// FEEDBACK(2026-07): --auto-top-up disabled for the positional-only UX
	// trial (use the positional form instead). Restore by uncommenting if
	// users ask for the flag form back.
	// cmd.Flags().String("auto-top-up", "", "Enable or disable auto-top-up (true|false)")

	return cmd
}

// --- bids and leases --------------------------------------------------------

func bidCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bid",
		Short: "Inspect provider bids",
	}

	cmd.AddCommand(&cobra.Command{
		Use:     "list <dseq>",
		Short:   "List bids for a deployment's open orders",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console bid list 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			bids, err := cl.FetchBids(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("list bids for deployment %s: %w", args[0], err)
			}

			if len(bids) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No bids yet (providers may still be bidding). Re-run in a few seconds.")
				return nil
			}

			return printJSON(cmd, bids)
		},
	})

	return cmd
}

func leaseCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease",
		Short: "Create leases from accepted bids",
	}

	create := &cobra.Command{
		Use:   "create <dseq> [provider]",
		Short: "Accept a bid by creating a lease and sending the manifest",
		Long: "Accept a bid by creating a lease and sending the deployment manifest to the " +
			"winning provider. The manifest defaults to the one cached by `deployment create`.",
		Args: cobra.RangeArgs(1, 2),
		Example: `  # Provider as positional argument (gseq/oseq default to 1)
  akt console lease create 12345 akash1provider...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, rc, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			dseq := args[0]
			gseq, _ := cmd.Flags().GetUint32("gseq")
			oseq, _ := cmd.Flags().GetUint32("oseq")
			// FEEDBACK(2026-07): --provider disabled for the positional-only
			// UX trial; the positional [provider] argument is the only
			// source. Restore by uncommenting if users ask for the flag form
			// back.
			// provider, _ := cmd.Flags().GetString("provider")
			provider := ""
			if len(args) > 1 {
				provider = args[1]
			}
			if provider == "" {
				return fmt.Errorf("provider is required: pass it as the [provider] argument")
			}
			manifestFile, _ := cmd.Flags().GetString("manifest")

			var manifest string
			switch {
			case manifestFile != "":
				data, err := os.ReadFile(manifestFile)
				if err != nil {
					return fmt.Errorf("read manifest file: %w", err)
				}
				manifest = string(data)

			case rc != nil:
				manifest, err = console.LoadManifest(rc.Root, rc.Name, dseq)
				if err != nil {
					return fmt.Errorf("no cached manifest for deployment %s: pass --manifest <file>, or recreate with `akt console deployment create` (%w)", dseq, err)
				}

			default:
				return fmt.Errorf("no cached manifest available without an active context: pass --manifest <file>")
			}

			detail, err := cl.CreateLease(cmd.Context(), manifest, []console.LeaseRequest{{
				DSeq:     dseq,
				GSeq:     gseq,
				OSeq:     oseq,
				Provider: provider,
			}})
			if err != nil {
				return fmt.Errorf("create lease for deployment %s: %w", dseq, err)
			}

			return printJSON(cmd, detail)
		},
	}

	create.Flags().Uint32("gseq", 1, "Group sequence number")
	create.Flags().Uint32("oseq", 1, "Order sequence number")
	// FEEDBACK(2026-07): --provider disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// create.Flags().String("provider", "", "Provider address; alternative to the positional argument")
	create.Flags().String("manifest", "", "Manifest file (defaults to the one cached by `deployment create`)")

	cmd.AddCommand(create)

	return cmd
}
