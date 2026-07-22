package console

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func apikeyCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apikey",
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
		Short:   "List API keys (secrets are never shown)",
		Args:    cobra.NoArgs,
		Example: `  akt console apikey list`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			keys, err := cl.ListAPIKeys(cmd.Context())
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
		Args:  cobra.MaximumNArgs(2),
		Example: `  # Name (and optional RFC 3339 expiry) as positional arguments
  akt console apikey create ci 2027-01-01T00:00:00Z

  # Equivalent flag form
  akt console apikey create --name ci --expires-at 2027-01-01T00:00:00Z`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			name, _ := cmd.Flags().GetString("name")
			expiresAt, _ := cmd.Flags().GetString("expires-at")
			if len(args) > 0 {
				name = args[0]
			}
			if len(args) > 1 {
				expiresAt = args[1]
			}
			if name == "" {
				return fmt.Errorf("name is required: pass it positionally or via --name")
			}

			created, err := cl.CreateAPIKey(cmd.Context(), name, expiresAt)
			if err != nil {
				return fmt.Errorf("create API key: %w", err)
			}

			// The secret is shown exactly once, by design.
			return printJSON(cmd, struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				APIKey  string `json:"apiKey"`
				Warning string `json:"warning"`
			}{created.ID, created.Name, created.APIKey, "Store this key now. It will not be shown again."})
		},
	}

	cmd.Flags().String("name", "", "Human-readable key name; alternative to the positional argument")
	cmd.Flags().String("expires-at", "", "Expiry as an RFC 3339 timestamp (empty = no expiry)")

	return cmd
}

func apikeyDeleteCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete an API key by ID (already-absent is a no-op)",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console apikey delete 0b8052e2-...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			// DeleteAPIKey treats 404 as a no-op.
			if err := cl.DeleteAPIKey(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("delete API key %s: %w", args[0], err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "API key %s deleted.\n", args[0])
			return nil
		},
	}
}

// defaultJWTScope mirrors the Console reference CLI's default lease scopes.
var defaultJWTScope = []string{"status", "logs", "events", "shell", "send-manifest", "get-manifest"}

func jwtCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jwt",
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

			ttl, _ := cmd.Flags().GetInt("ttl")
			if ttl <= 0 {
				return fmt.Errorf("--ttl must be a positive number of seconds, got %d", ttl)
			}

			scopeCSV, _ := cmd.Flags().GetString("scope")

			scope := defaultJWTScope
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

	create.Flags().Int("ttl", 300, "Token lifetime in seconds")
	create.Flags().String("scope", "", "Comma-separated scopes (default: "+strings.Join(defaultJWTScope, ",")+")")

	cmd.AddCommand(create)

	return cmd
}
