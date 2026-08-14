package console

import (
	"fmt"
	"strings"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
)

// Public catalog commands. None of these require a Console API key; a key is
// still sent when one is configured.

func providerCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		RunE:  sdkclient.ValidateCmd,
		Short: "Browse the Console provider catalog (no API key required)",
	}

	list := &cobra.Command{
		Use:     "list",
		Short:   "List providers",
		Args:    cobra.NoArgs,
		Example: `  akt console provider list --limit 10`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			providers, err := cl.ListProviders(cmd.Context(), "", nil)
			if err != nil {
				return fmt.Errorf("list providers: %w", err)
			}

			// The endpoint has no server-side paging; --limit trims locally.
			limit, _ := cmd.Flags().GetInt(flagdefs.FlagLimit)
			if limit > 0 && len(providers) > limit {
				providers = providers[:limit]
			}

			return printJSON(cmd, providers)
		},
	}
	list.Flags().Int(flagdefs.FlagLimit, 20, "Maximum providers to show (0 = all)")

	get := &cobra.Command{
		Use:     "get <address>",
		Short:   "Show one provider's full catalog record",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console provider get akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			detail, err := cl.GetProvider(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get provider %s: %w", args[0], err)
			}

			// Raw carries the complete document (stats etc.), not just the
			// typed summary fields.
			return printRawJSON(cmd, detail.Raw)
		},
	}

	regions := &cobra.Command{
		Use:     "regions",
		Short:   "List regions providers advertise",
		Args:    cobra.NoArgs,
		Example: `  akt console provider regions`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			out, err := cl.ListProviderRegions(cmd.Context())
			if err != nil {
				return fmt.Errorf("list provider regions: %w", err)
			}

			return printJSON(cmd, out)
		},
	}

	auditors := &cobra.Command{
		Use:     "auditors",
		Short:   "List known auditors",
		Args:    cobra.NoArgs,
		Example: `  akt console provider auditors`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			out, err := cl.ListAuditors(cmd.Context())
			if err != nil {
				return fmt.Errorf("list auditors: %w", err)
			}

			return printJSON(cmd, out)
		},
	}

	cmd.AddCommand(list, get, regions, auditors)

	return cmd
}

func gpuCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "gpu",
		Short:   "Network-wide GPU availability and prices (no API key required)",
		Args:    cobra.NoArgs,
		Example: `  akt console gpu`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			prices, err := cl.GetGPUPrices(cmd.Context())
			if err != nil {
				return fmt.Errorf("get GPU prices: %w", err)
			}

			return printJSON(cmd, prices)
		},
	}
}

func templateCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		RunE:  sdkclient.ValidateCmd,
		Short: "Browse deployment templates (no API key required)",
	}

	list := &cobra.Command{
		Use:     "list",
		Short:   "List the template catalog",
		Args:    cobra.NoArgs,
		Example: `  akt console template list`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			raw, err := cl.ListTemplates(cmd.Context())
			if err != nil {
				return fmt.Errorf("list templates: %w", err)
			}

			return printRawJSON(cmd, raw)
		},
	}

	get := &cobra.Command{
		Use:     "get <id>",
		Short:   "Show one template",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console template get hello-world`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			tmpl, err := cl.GetTemplate(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get template %s: %w", args[0], err)
			}

			return printJSON(cmd, tmpl)
		},
	}

	sdl := &cobra.Command{
		Use:   "sdl <id>",
		Short: "Print a template's raw SDL to stdout (for piping)",
		Long: "Print a template's raw SDL for direct redirection in the default output mode. " +
			"JSON and YAML output wrap the exact source in an object under the sdl field.",
		Args: cobra.ExactArgs(1),
		Example: `  # Write a template's SDL to a file, then deploy it
  akt console template sdl hello-world > deploy.yaml
  akt console deployment create deploy.yaml 5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			tmpl, err := cl.GetTemplate(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get template %s: %w", args[0], err)
			}

			if tmpl.Deploy == "" {
				return fmt.Errorf("template %s has no deploy SDL", args[0])
			}

			if format := output.FormatFromCmd(cmd); format != output.FormatTable {
				return printJSON(cmd, struct {
					SDL string `json:"sdl"`
				}{tmpl.Deploy})
			}

			out := tmpl.Deploy
			if !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
			return printConsoleText(cmd, out)
		},
	}

	cmd.AddCommand(list, get, sdl)

	return cmd
}
