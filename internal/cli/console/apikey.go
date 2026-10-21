package console

import (
	"fmt"
	"strings"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	consoleapi "pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
)

func apikeyCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apikey",
		RunE:  sdkclient.ValidateCmd,
		Short: "Manage Console API keys",
	}

	cmd.AddCommand(
		apikeyListCmd(mgrFn),
		apikeyCreateCmd(mgrFn),
		apikeyDeleteCmd(mgrFn),
	)

	return cmd
}

func apikeyListCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List API keys with the organization and project each acts in (secrets are never shown)",
		Args:    cobra.NoArgs,
		Example: `  akt console apikey list`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			keys, err := cl.ForProject("").ListAPIKeys(cmd.Context())
			if err != nil {
				return fmt.Errorf("list API keys: %w", err)
			}

			return printJSON(cmd, keys)
		},
	}
}

func apikeyCreateCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name> [expires-at]",
		Short: "Create an API key (the secret is shown ONCE)",
		Long: "Create an API key in the organization the current key acts in. Pass " +
			"--console-project on this command to limit the new key to one project; without it the " +
			"key reaches every project its owner can, even when the context selects a project.",
		Args: cobra.MaximumNArgs(2),
		Example: `  # Name (and optional RFC 3339 expiry) as positional arguments
  akt console apikey create ci 2027-01-01T00:00:00Z

  # A key that acts only in one project
  akt console apikey create ci-checkout --console-project checkout-api`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			// FEEDBACK(2026-07): --name/--expires-at disabled for the
			// positional-only UX trial; the positional <name> [expires-at]
			// arguments are the only source. Restore by uncommenting if
			// users ask for the flag form back.
			// name, _ := cmd.Flags().GetString("name")
			// expiresAt, _ := cmd.Flags().GetString("expires-at")
			name, expiresAt := "", ""
			if len(args) > 0 {
				name = args[0]
			}
			if len(args) > 1 {
				expiresAt = args[1]
			}
			if name == "" {
				return fmt.Errorf("name is required: pass it as the first argument")
			}

			cl = cl.ForProject("")
			projectID, err := apikeyProjectBinding(cmd, cl)
			if err != nil {
				return fmt.Errorf("create API key: %w", err)
			}

			created, err := cl.CreateAPIKeyForProject(cmd.Context(), name, expiresAt, projectID)
			if err != nil {
				return fmt.Errorf("create API key: %w", err)
			}

			// The secret is shown exactly once, by design.
			return printJSON(cmd, struct {
				ID             string  `json:"id"`
				Name           string  `json:"name"`
				APIKey         string  `json:"apiKey"`
				OrganizationID *string `json:"organizationId,omitempty"`
				ProjectID      *string `json:"projectId,omitempty"`
				Warning        string  `json:"warning"`
			}{created.ID, created.Name, created.APIKey, created.OrganizationID, created.ProjectID, "Store this key now. It will not be shown again."})
		},
	}

	// FEEDBACK(2026-07): --name disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().String("name", "", "Human-readable key name; alternative to the positional argument")
	// FEEDBACK(2026-07): --expires-at disabled for the positional-only UX
	// trial (use the positional form instead). Restore by uncommenting if
	// users ask for the flag form back.
	// cmd.Flags().String("expires-at", "", "Expiry as an RFC 3339 timestamp (empty = no expiry)")

	return cmd
}

// apikeyProjectBinding returns the id of the project a new key is limited
// to: only a --console-project given on this command line binds the key.
// The context's project or AKT_CONSOLE_PROJECT never does, so selecting a
// project to work in cannot silently narrow the keys created from it.
func apikeyProjectBinding(cmd *cobra.Command, cl *consoleapi.Client) (string, error) {
	flag := cmd.Flags().Lookup(flagdefs.FlagConsoleProject)
	if flag == nil || !flag.Changed || strings.TrimSpace(flag.Value.String()) == "" {
		return "", nil
	}

	project, err := cl.FindProject(cmd.Context(), flag.Value.String())
	if err != nil {
		return "", err
	}

	return project.ID, nil
}

func apikeyDeleteCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete an API key by ID",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console apikey delete 0b8052e2-...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			if err := cl.ForProject("").DeleteAPIKey(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("delete API key %s: %w", args[0], err)
			}

			return printConsoleResult(cmd, fmt.Sprintf("API key %s deleted.", args[0]), struct {
				ID      string `json:"id" yaml:"id"`
				Deleted bool   `json:"deleted" yaml:"deleted"`
			}{ID: args[0], Deleted: true})
		},
	}
}

func defaultJWTScope() []string {
	return []string{"status", "logs", "events", "shell", "send-manifest", "get-manifest"}
}

func jwtCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jwt",
		RunE:  sdkclient.ValidateCmd,
		Short: "Mint provider-scoped JWTs",
	}

	create := &cobra.Command{
		Use:     "create",
		Short:   "Create a short-lived JWT for direct provider access",
		Args:    cobra.NoArgs,
		Example: `  akt console jwt create --ttl 600 --scope status,logs`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			ttl, _ := cmd.Flags().GetInt(flagdefs.FlagTTL)
			if ttl < 1 || ttl > consoleapi.MaxJWTTTLSeconds {
				return fmt.Errorf("--ttl must be between 1 and %d seconds, got %d", consoleapi.MaxJWTTTLSeconds, ttl)
			}

			scopeCSV, _ := cmd.Flags().GetString(flagdefs.FlagScope)

			scope := defaultJWTScope()
			if scopeCSV != "" {
				scope = nil
				for _, s := range strings.Split(scopeCSV, ",") {
					if s = strings.TrimSpace(s); s != "" {
						scope = append(scope, s)
					}
				}
			}
			if len(scope) == 0 {
				return fmt.Errorf("--scope must contain at least one scope")
			}

			token, err := cl.CreateJWTToken(cmd.Context(), ttl, scope)
			if err != nil {
				return fmt.Errorf("create JWT token: %w", err)
			}

			return printJSON(cmd, struct {
				Token string   `json:"token"`
				TTL   int      `json:"ttl"`
				Scope []string `json:"scope"`
			}{token, ttl, scope})
		},
	}

	create.Flags().Int(flagdefs.FlagTTL, 300, fmt.Sprintf("Token lifetime in seconds (maximum %d)", consoleapi.MaxJWTTTLSeconds))
	create.Flags().String(flagdefs.FlagScope, "", "Comma-separated scopes (default: "+strings.Join(defaultJWTScope(), ",")+")")

	cmd.AddCommand(create)

	return cmd
}
