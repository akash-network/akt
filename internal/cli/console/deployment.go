package console

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
)

// minDepositUSD is the Console API's minimum deployment deposit.
const minDepositUSD = 0.5

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
		Use:   "create <sdl-file>",
		Short: "Create a deployment (managed wallet signs server-side)",
		Long: "Create a deployment from an SDL file with a USD deposit. The returned manifest " +
			"is cached per-context so `akt console lease create` can send it without re-passing it.",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console deployment create deploy.yaml --deposit 5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, rc, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			deposit, _ := cmd.Flags().GetFloat64("deposit")
			if deposit < minDepositUSD {
				return fmt.Errorf("--deposit must be at least %s (minimum deposit), got %s", formatUSD(minDepositUSD), formatUSD(deposit))
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

	cmd.Flags().Float64("deposit", 0, "Deposit amount in USD (minimum 0.5)")
	_ = cmd.MarkFlagRequired("deposit")

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
		Use:     "deposit <dseq>",
		Short:   "Add funds to a deployment's escrow",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console deployment deposit 12345 --amount 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			amount, _ := cmd.Flags().GetFloat64("amount")
			if amount <= 0 {
				return fmt.Errorf("--amount must be a positive USD amount, got %s", formatUSD(amount))
			}

			if err := cl.Deposit(cmd.Context(), args[0], amount); err != nil {
				return fmt.Errorf("deposit to deployment %s: %w", args[0], err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deposited %s to deployment %s.\n", formatUSD(amount), args[0])
			return nil
		},
	}

	cmd.Flags().Float64("amount", 0, "Amount to add in USD")
	_ = cmd.MarkFlagRequired("amount")

	return cmd
}

func deploymentSettingsCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings <dseq>",
		Short: "View or change a deployment's auto-top-up setting",
		Args:  cobra.ExactArgs(1),
		Example: `  # Show current settings
  akt console deployment settings 12345

  # Enable auto-top-up
  akt console deployment settings 12345 --auto-top-up true`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			if cmd.Flags().Changed("auto-top-up") {
				value, _ := cmd.Flags().GetString("auto-top-up")

				enabled, err := parseBoolValue(value, "--auto-top-up")
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

	cmd.Flags().String("auto-top-up", "", "Enable or disable auto-top-up (true|false)")

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
		Use:   "create <dseq>",
		Short: "Accept a bid by creating a lease and sending the manifest",
		Long: "Accept a bid by creating a lease and sending the deployment manifest to the " +
			"winning provider. The manifest defaults to the one cached by `deployment create`.",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console lease create 12345 --gseq 1 --oseq 1 --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, rc, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			dseq := args[0]
			gseq, _ := cmd.Flags().GetUint32("gseq")
			oseq, _ := cmd.Flags().GetUint32("oseq")
			provider, _ := cmd.Flags().GetString("provider")
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
	create.Flags().String("provider", "", "Provider address (required)")
	create.Flags().String("manifest", "", "Manifest file (defaults to the one cached by `deployment create`)")
	_ = create.MarkFlagRequired("provider")

	cmd.AddCommand(create)

	return cmd
}
