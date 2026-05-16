// Package workflow implements the `akt deploy`, `akt update`, and `akt close`
// CLI commands — thin wrappers that load embedded workflow YAML definitions
// and run them through the workflow engine.
package workflow

import (
	"fmt"

	"github.com/spf13/cobra"

	wf "pkg.akt.dev/akt/internal/workflow"
	"pkg.akt.dev/akt/internal/workflow/builtin"
)

// DeployCmd returns the `akt deploy` command.
func DeployCmd(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy <sdl-file>",
		Short: "Deploy an application to the Akash Network",
		Long: `Create a deployment from an SDL file, wait for provider bids,
select a provider, create a lease, and send the manifest.

This command orchestrates the full deployment workflow defined in
the built-in deploy.yaml workflow definition.`,
		Args: cobra.ExactArgs(1),
		Example: `  # Deploy interactively (select bid from list)
  akt deploy app.yaml

  # Deploy with automatic cheapest bid selection
  akt deploy app.yaml --bid-select cheapest

  # Deploy to a specific provider
  akt deploy app.yaml --bid-select provider=akash1abc...

  # Dry run — show execution plan without broadcasting
  akt deploy app.yaml --dry-run`,
		RunE: workflowRunE("deploy", homeFn, ctxNameFn, func(cmd *cobra.Command, args []string) (map[string]any, error) {
			params := make(map[string]any)
			params["sdl-file"] = args[0]

			deposit, _ := cmd.Flags().GetString("deposit")
			params["deposit"] = deposit

			bidTimeout, _ := cmd.Flags().GetString("bid-timeout")
			params["bid-timeout"] = bidTimeout

			bidSelect, _ := cmd.Flags().GetString("bid-select")
			params["bid-select"] = bidSelect

			return params, nil
		}),
	}

	cmd.Flags().String("deposit", "auto", "Initial deposit amount (auto = chain minimum or SDL-specified)")
	cmd.Flags().String("bid-timeout", "5m", "Maximum time to wait for bids")
	cmd.Flags().String("bid-select", "interactive", "Bid selection: interactive, cheapest, provider=<addr>")
	cmd.Flags().Int("min-bids", 1, "Minimum number of bids to collect before selection")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().Bool("dry-run", false, "Show execution plan without broadcasting transactions")

	return cmd
}

// UpdateCmd returns the `akt update` command.
func UpdateCmd(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <sdl-file> [dseq]",
		Short: "Update an existing deployment",
		Long: `Update a deployment with a new SDL file and send the updated
manifest to providers.

The deployment sequence (dseq) can be provided as a positional argument
or via the --dseq flag.`,
		Args: cobra.RangeArgs(1, 2),
		Example: `  # Update deployment 12345 with new SDL
  akt update app.yaml 12345

  # Update using --dseq flag
  akt update app.yaml --dseq 12345

  # Dry run — show execution plan
  akt update app.yaml --dseq 12345 --dry-run`,
		RunE: workflowRunE("update", homeFn, ctxNameFn, func(cmd *cobra.Command, args []string) (map[string]any, error) {
			params := make(map[string]any)
			params["sdl-file"] = args[0]

			dseq, _ := cmd.Flags().GetInt("dseq")
			if len(args) > 1 {
				var parsed int
				if _, err := fmt.Sscanf(args[1], "%d", &parsed); err != nil {
					return nil, fmt.Errorf("invalid dseq %q: %w", args[1], err)
				}
				dseq = parsed
			}

			if dseq == 0 {
				return nil, fmt.Errorf("deployment sequence (dseq) is required; provide as argument or --dseq flag")
			}

			params["dseq"] = dseq

			return params, nil
		}),
	}

	cmd.Flags().Int("dseq", 0, "Deployment sequence to update")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().Bool("dry-run", false, "Show execution plan without broadcasting transactions")

	return cmd
}

// CloseCmd returns the `akt close` command.
func CloseCmd(homeFn func() string, ctxNameFn func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close [dseq]",
		Short: "Close a deployment",
		Long: `Close a deployment and return the remaining escrow balance.

The deployment sequence (dseq) can be provided as a positional argument
or via the --dseq flag.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  # Close deployment 12345
  akt close 12345

  # Close using --dseq flag
  akt close --dseq 12345

  # Dry run — show execution plan
  akt close --dseq 12345 --dry-run`,
		RunE: workflowRunE("close", homeFn, ctxNameFn, func(cmd *cobra.Command, args []string) (map[string]any, error) {
			params := make(map[string]any)

			dseq, _ := cmd.Flags().GetInt("dseq")
			if len(args) > 0 {
				var parsed int
				if _, err := fmt.Sscanf(args[0], "%d", &parsed); err != nil {
					return nil, fmt.Errorf("invalid dseq %q: %w", args[0], err)
				}
				dseq = parsed
			}

			if dseq == 0 {
				return nil, fmt.Errorf("deployment sequence (dseq) is required; provide as argument or --dseq flag")
			}

			params["dseq"] = dseq

			return params, nil
		}),
	}

	cmd.Flags().Int("dseq", 0, "Deployment sequence to close")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().Bool("dry-run", false, "Show execution plan without broadcasting transactions")

	return cmd
}

// paramsFn extracts workflow parameters from cobra flags and positional args.
type paramsFn func(cmd *cobra.Command, args []string) (map[string]any, error)

// workflowRunE returns a RunE function that loads a named workflow,
// validates it, and prints an execution plan.
//
// TODO(T057+): Wire ChainClient and ProviderClient implementations to
// enable full workflow execution via the Engine. Currently the commands
// load and validate the workflow definition, then print a summary of
// what would be executed.
func workflowRunE(name string, homeFn func() string, ctxNameFn func() string, pFn paramsFn) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		params, err := pFn(cmd, args)
		if err != nil {
			return err
		}

		loader := wf.NewLoader(homeFn(), ctxNameFn(), builtin.Workflows())

		def, err := loader.Load(name)
		if err != nil {
			return fmt.Errorf("load workflow %q: %w", name, err)
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")

		out := cmd.OutOrStdout()

		fmt.Fprintf(out, "Workflow: %s (v%d)\n", def.Name, def.Version)
		fmt.Fprintf(out, "  %s\n\n", def.Description)

		fmt.Fprintln(out, "Parameters:")
		for k, v := range params {
			fmt.Fprintf(out, "  %-16s %v\n", k+":", v)
		}
		fmt.Fprintln(out)

		fmt.Fprintf(out, "Steps (%d):\n", len(def.Steps))
		for i, step := range def.Steps {
			fmt.Fprintf(out, "  %d. [%s] %s", i+1, step.Type, step.Name)
			if step.Msg != "" {
				fmt.Fprintf(out, " -> %s", step.Msg)
			}
			if step.Action != "" {
				fmt.Fprintf(out, " -> %s", step.Action)
			}
			if step.OnError != "" {
				fmt.Fprintf(out, " (on-error: %s)", step.OnError)
			}
			if step.Retry != nil {
				fmt.Fprintf(out, " (retry: %dx, %s delay)", step.Retry.Max, step.Retry.Delay)
			}
			fmt.Fprintln(out)
		}

		if dryRun {
			fmt.Fprintln(out, "\nDry run — no transactions broadcast.")
			return nil
		}

		// TODO(T057+): Execute the workflow via Engine.Run() once
		// ChainClient and ProviderClient are wired.
		fmt.Fprintln(out, "\nExecution requires chain client (not yet wired). Use --dry-run to preview.")

		return nil
	}
}
