package console

import (
	"fmt"
	"strings"

	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
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
		Use:   "create [name] [expires-at]",
		Short: "Create an API key (the secret is shown ONCE)",
		Args:  cflags.PositionalOrFlagArgs(flagdefs.FlagName, flagdefs.FlagExpiresAt),
		Example: `  # Name (and optional RFC 3339 expiry) as positional arguments
  akt console apikey create ci 2027-01-01T00:00:00Z`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			name, _ := cmd.Flags().GetString(flagdefs.FlagName)
			expiresAt, _ := cmd.Flags().GetString(flagdefs.FlagExpiresAt)
			if len(args) > 0 {
				name = args[0]
			}
			if len(args) > 1 {
				expiresAt = args[1]
			}
			if name == "" {
				return fmt.Errorf("name is required: pass it as the first argument")
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

	cmd.Flags().String(flagdefs.FlagName, "", "Human-readable key name; alternative to the positional argument")
	cmd.Flags().String(flagdefs.FlagExpiresAt, "", "Expiry as an RFC 3339 timestamp (empty = no expiry)")

	return cmd
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

			if err := cl.DeleteAPIKey(cmd.Context(), args[0]); err != nil {
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
