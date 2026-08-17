// Package console implements the `akt console` CLI commands for the Akash
// Console managed-wallet API (SPEC §2.9, §7). Deployments created here are
// signed server-side by the Console managed wallet; no local keyring is used.
package console

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"pkg.akt.dev/akt/internal/capability"
	"pkg.akt.dev/akt/internal/cliutil"
	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
)

func Commands(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console",
		RunE:  sdkclient.ValidateCmd,
		Short: "Akash Console managed-wallet operations",
		Long: "Interact with the Akash Console API: create and manage deployments through " +
			"the Console managed wallet, inspect bids and leases, and browse the public " +
			"provider/GPU/template catalog. Authenticated commands need a Console API key " +
			"(run `akt console login`); catalog commands work without one. For a complete " +
			"create, bid, lease, and manifest flow, use `akt deploy <sdl-file>`.",
	}

	cmd.PersistentFlags().String("console-api-url", "", "Console API base URL (overrides the context setting)")
	cmd.PersistentFlags().String("console-api-key", "", "Console API key (overrides AKT_CONSOLE_API_KEY and the stored credential)")

	cmd.AddCommand(
		// Credential management and public marketplace/catalog commands
		// work without a stored key, so they carry no capability gate.
		loginCmd(mgrFn),
		logoutCmd(mgrFn),
		providerCmds(mgrFn),
		gpuCmd(mgrFn),
		templateCmds(mgrFn),
		screenCmd(mgrFn),

		// Everything below needs a resolvable Console API key (SPEC §2.10).
		requireConsole(whoamiCmd(mgrFn)),
		requireConsole(deploymentCmds(mgrFn)),
		requireConsole(bidCmds(mgrFn)),
		requireConsole(leaseCmds(mgrFn)),
		requireConsole(walletCmds(mgrFn)),
		requireConsole(usageCmd(mgrFn)),
		requireConsole(apikeyCmds(mgrFn)),
		requireConsole(jwtCmds(mgrFn)),
		requireConsole(logsCmd(mgrFn)),
		requireConsole(eventsCmd(mgrFn)),
		requireConsole(statusCmd(mgrFn)),
		requireConsole(shellCmd(mgrFn)),
	)

	return cmd
}

// requireConsole tags a command as needing the console capability so the
// gating layer (internal/capability, SPEC §2.10) can dim or hide it when the
// active context has no resolvable Console API key.
func requireConsole(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[capability.AnnotationKey] = string(capability.Console)

	return cmd
}

// --- authentication ---------------------------------------------------------

func loginCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "login [key]",
		Short: "Validate a Console API key and store it for the active context",
		Long: "Validate a Console API key against the Console API and store it as the active " +
			"context's credential. The key can be passed as an argument or entered " +
			"at a hidden prompt. Keys are created at console.akash.network > Settings > API Keys.",
		Args: cobra.MaximumNArgs(1),
		Example: `  # Prompt for the key (input hidden)
  akt console login

  # Pass the key directly
  akt console login sk-console-...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := resolveContextFromCmd(cmd, mgrFn())
			if err != nil {
				return fmt.Errorf("login requires an active context to store the key: %w", err)
			}

			var key string
			if len(args) > 0 {
				key = args[0]
			} else {
				key, err = promptForKey(cmd)
				if err != nil {
					return err
				}
			}

			if key == "" {
				return fmt.Errorf("no API key provided")
			}

			baseURL, _ := cmd.Flags().GetString("console-api-url")
			if baseURL == "" {
				baseURL = rc.ConsoleAPIURL
			}

			// Validate against the live user endpoint before persisting.
			probe := console.New(baseURL, key)
			user, err := probe.GetUser(cmd.Context())
			if err != nil {
				return fmt.Errorf("validate Console API key: %w", err)
			}

			if err := aktctx.SetConsoleAPIKey(rc.Root, rc.Name, key); err != nil {
				return err
			}

			return printConsoleResult(cmd,
				fmt.Sprintf("Logged in as %s (context %q)", user.Username, rc.Name),
				struct {
					Username      string `json:"username"      yaml:"username"`
					Context       string `json:"context"       yaml:"context"`
					Authenticated bool   `json:"authenticated" yaml:"authenticated"`
				}{Username: user.Username, Context: rc.Name, Authenticated: true},
			)
		},
	}
}

func logoutCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Short:   "Remove the stored Console API key for the active context",
		Args:    cobra.NoArgs,
		Example: `  akt console logout`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := resolveContextFromCmd(cmd, mgrFn())
			if err != nil {
				return err
			}

			if err := aktctx.SetConsoleAPIKey(rc.Root, rc.Name, ""); err != nil {
				return err
			}

			return printConsoleResult(cmd,
				fmt.Sprintf("Removed Console API key for context %q", rc.Name),
				struct {
					Context       string `json:"context"       yaml:"context"`
					Authenticated bool   `json:"authenticated" yaml:"authenticated"`
				}{Context: rc.Name, Authenticated: false},
			)
		},
	}
}

func whoamiCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "whoami",
		Short:   "Show the authenticated Console user",
		Args:    cobra.NoArgs,
		Example: `  akt console whoami`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			user, err := cl.GetUser(cmd.Context())
			if err != nil {
				return fmt.Errorf("get user: %w", err)
			}

			return printJSON(cmd, struct {
				Username      string `json:"username"`
				Email         string `json:"email"`
				EmailVerified bool   `json:"emailVerified"`
			}{user.Username, user.Email, user.EmailVerified})
		},
	}
}

// promptForKey reads a Console API key from stdin: with echo off when stdin is
// a terminal, as a plain line otherwise.
func promptForKey(cmd *cobra.Command) (string, error) {
	return promptForKeyWithTerminal(cmd, int(os.Stdin.Fd()), term.IsTerminal, term.ReadPassword)
}

func promptForKeyWithTerminal(
	cmd *cobra.Command,
	fd int,
	isTerminal func(int) bool,
	readPassword func(int) ([]byte, error),
) (string, error) {
	if isTerminal(fd) {
		if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Console API key: "); err != nil {
			return "", fmt.Errorf("write API key prompt: %w", err)
		}

		data, err := readPassword(fd)
		if _, writeErr := fmt.Fprintln(cmd.ErrOrStderr()); writeErr != nil {
			return "", fmt.Errorf("write API key prompt terminator: %w", writeErr)
		}
		if err != nil {
			return "", fmt.Errorf("read API key: %w", err)
		}

		return strings.TrimSpace(string(data)), nil
	}

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read API key from stdin: %w", err)
	}

	return strings.TrimSpace(line), nil
}

// --- shared helpers ---------------------------------------------------------

// clientFromCmd builds a Console API client from the command's flags and the
// resolved context. Resolution (SPEC §7.1/§7.2): --console-api-key flag >
// context (env > stored credential); --console-api-url flag > context URL.
// A missing context is tolerated so public catalog endpoints work keyless;
// requireKey enforces a key for authenticated endpoints. The returned context
// is nil when no context could be resolved.
func clientFromCmd(cmd *cobra.Command, mgrFn func() *aktctx.Manager, requireKey bool) (*console.Client, *aktctx.Context, error) {
	var rc *aktctx.Context
	if m := mgrFn(); m != nil {
		// Honor the global --context override: credentials are read from and
		// written to the context the user named, and Console operations bill
		// that context's managed wallet.
		rc, _ = resolveContextFromCmd(cmd, m) // missing context is OK for public endpoints
	}

	key, _ := cmd.Flags().GetString("console-api-key")
	baseURL, _ := cmd.Flags().GetString("console-api-url")

	if rc != nil {
		if key == "" {
			key = rc.ConsoleAPIKey // env var > stored credential
		}
		if baseURL == "" {
			baseURL = rc.ConsoleAPIURL
		}
	} else if key == "" {
		key = os.Getenv(aktctx.EnvConsoleAPIKey)
	}

	if requireKey && key == "" {
		if rc == nil {
			return nil, nil, fmt.Errorf("no Console API key configured and no active context: set %s or --console-api-key, or create a context and run `akt console login`", aktctx.EnvConsoleAPIKey)
		}

		return nil, nil, fmt.Errorf("no Console API key configured for context %q: run `akt console login`, set %s, or `akt context edit %s --console-api-key <key>`",
			rc.Name, aktctx.EnvConsoleAPIKey, rc.Name)
	}

	cl := console.New(baseURL, key).WithActionLog(cliutil.ActionLogFromContext(cmd.Context()))

	return cl, rc, nil
}

func resolveContextFromCmd(cmd *cobra.Command, m *aktctx.Manager) (*aktctx.Context, error) {
	return m.Resolve(cliutil.SelectedContextName(cmd, m))
}

// printJSON renders v in the format --output asks for and writes it to the
// command's output.
//
// It used to marshal JSON unconditionally, so `-o yaml` returned JSON with
// exit 0 on every command in this group and a YAML consumer silently parsed
// the wrong format. Table stays JSON: the console payloads are nested API
// objects rather than rows, and there is no column layout to render them as.
func printJSON(cmd *cobra.Command, v interface{}) error {
	format := output.FormatFromCmd(cmd)
	if format != output.FormatYAML {
		format = output.FormatJSON
	}

	if err := output.FprintJSONSemantics(cmd.OutOrStdout(), format, v); err != nil {
		return fmt.Errorf("marshal output: %w", err)
	}

	return nil
}

// printRawJSON re-indents a raw JSON document and writes it to the command's
// output.
func printRawJSON(cmd *cobra.Command, raw json.RawMessage) error {
	return printJSON(cmd, raw)
}

// printConsoleResult keeps the concise human acknowledgement used by default
// while giving JSON and YAML callers a stable object to parse.
func printConsoleResult(cmd *cobra.Command, pretty string, structured any) error {
	if output.FormatFromCmd(cmd) == output.FormatTable {
		return printConsoleText(cmd, pretty+"\n")
	}

	return printJSON(cmd, structured)
}

func printConsoleText(cmd *cobra.Command, text string) error {
	checked := output.NewCheckedWriter(cmd.OutOrStdout())
	_, err := fmt.Fprint(checked, text)
	return checked.Complete(err)
}

// formatUSD keeps normal currency values at cents while retaining meaningful
// precision for the sub-cent usage and cost values returned by Console.
func formatUSD(v float64) string {
	if v == 0 {
		return "$0.00"
	}

	prefix := "$"
	if v < 0 {
		prefix = "-$"
	}
	value := math.Abs(v)
	if value < 0.000001 {
		return prefix + "<0.000001"
	}
	if value < 0.01 {
		formatted := strconv.FormatFloat(value, 'f', 6, 64)
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
		return prefix + formatted
	}

	return fmt.Sprintf("%s%.2f", prefix, value)
}

// parseBoolValue parses a strict true|false string flag value.
func parseBoolValue(value, flag string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false, got %q", flag, value)
	}
}
