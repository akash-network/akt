package console

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"pkg.akt.dev/akt/internal/cliutil"
	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
	sstore "pkg.akt.dev/akt/internal/store"
	"pkg.akt.dev/akt/internal/store/bbolt"
)

func confirmDeploymentCreate(cmd *cobra.Command, prompt bool, path string) error {
	if !prompt {
		return nil
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Create deployment from %s? [y/N] ", path); err != nil {
		return err
	}
	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		return fmt.Errorf("read deployment confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("deployment creation cancelled")
	}

	return nil
}

func deploymentCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment",
		RunE:  sdkclient.ValidateCmd,
		Short: "Manage Console deployments",
		Long: "Create, inspect, update, and close deployments through the Console managed wallet. " +
			"Deployments are funded automatically from your account credits. " +
			"Use `akt deploy <sdl-file>` for the complete create, bid, lease, and manifest flow.",
	}

	cmd.AddCommand(
		deploymentListCmd(mgrFn),
		deploymentGetCmd(mgrFn),
		deploymentCreateCmd(mgrFn),
		deploymentUpdateCmd(mgrFn),
		deploymentCloseCmd(mgrFn),
		deploymentSettingsCmd(mgrFn),
	)

	return cmd
}

func deploymentListCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [active|closed]",
		Short:   "List deployments",
		Args:    cobra.MaximumNArgs(1),
		Example: `  akt console deployment list active --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state := ""
			if len(args) == 1 {
				state = strings.ToLower(strings.TrimSpace(args[0]))
				if state != "active" && state != "closed" {
					return fmt.Errorf("deployment state must be active or closed, got %q", args[0])
				}
			}

			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			skip, _ := cmd.Flags().GetInt(flagdefs.FlagSkip)
			limit, _ := cmd.Flags().GetInt(flagdefs.FlagLimit)

			var list *console.DeploymentList
			if state == "" {
				list, err = cl.ListDeployments(cmd.Context(), skip, limit)
			} else {
				list, err = cl.ListDeploymentsByState(cmd.Context(), state, skip, limit)
			}
			if err != nil {
				return fmt.Errorf("list deployments: %w", err)
			}

			return printJSON(cmd, list)
		},
	}

	cmd.Flags().Int(flagdefs.FlagSkip, 0, "Pagination offset")
	cmd.Flags().Int(flagdefs.FlagLimit, 20, "Page size")

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
	return deploymentCreateCmdWithTerminal(mgrFn, term.IsTerminal)
}

func deploymentCreateCmdWithTerminal(
	mgrFn func() *aktctx.Manager,
	isTerminal func(int) bool,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <sdl-file>",
		Short: "Create a deployment (managed wallet signs server-side)",
		Long: "Create a deployment from an SDL file. There is no deposit: the platform funds the " +
			"deployment from your account credits. The returned manifest " +
			"is cached per-context so `akt console lease create` can send it without re-passing it. " +
			"For the complete lifecycle, use `akt deploy <sdl-file>`.",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console deployment create deploy.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, rc, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			skipConfirmation, _ := cmd.Flags().GetBool(flagdefs.FlagSkipConfirmation)
			if err := confirmDeploymentCreate(cmd,
				!skipConfirmation && isTerminal(int(os.Stdin.Fd())), args[0]); err != nil {
				return err
			}

			sdl, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read SDL file: %w", err)
			}

			result, err := cl.CreateDeployment(cmd.Context(), string(sdl))
			if err != nil {
				return fmt.Errorf("create deployment: %w", err)
			}

			// Cache the manifest so `lease create` can default to it.
			note := ""
			// Emitted only when the manifest could not be cached. `lease
			// create` needs the manifest Console rendered from the SDL, and it
			// is returned exactly once, here -- telling the user to pass
			// --manifest without handing them the file leaves a deployment
			// that can be leased but never gets a workload, quietly burning
			// escrow.
			uncachedManifest := ""

			switch {
			case rc == nil:
				note = "manifest not cached (no active context): save the manifest field below and pass it to `lease create --manifest`"
				uncachedManifest = result.Manifest
			case result.Manifest != "":
				if err := console.SaveManifest(rc.Root, rc.Name, result.DSeq.String(), result.Manifest); err != nil {
					note = fmt.Sprintf("manifest not cached (%v): save the manifest field below and pass it to `lease create --manifest`", err)
					uncachedManifest = result.Manifest
				}
			}

			txHash := ""
			if result.SignTx != nil {
				txHash = result.SignTx.TransactionHash
			}

			return printJSON(cmd, struct {
				DSeq     string `json:"dseq"`
				TxHash   string `json:"txHash,omitempty"`
				Note     string `json:"note,omitempty"`
				Manifest string `json:"manifest,omitempty"`
			}{
				DSeq:     result.DSeq.String(),
				TxHash:   txHash,
				Note:     note,
				Manifest: uncachedManifest,
			})
		},
	}

	cmd.Flags().BoolP(flagdefs.FlagSkipConfirmation, "y", false, "Skip the deployment creation confirmation")

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
		Short:   "Close an active deployment",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console deployment close 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, rc, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			err = cl.CloseDeployment(cmd.Context(), args[0])
			if err != nil {
				if errors.Is(err, console.ErrAlreadyClosed) {
					if convergeErr := convergeClosedDeployment(cmd, rc, args[0]); convergeErr != nil && !cliutil.IsQuiet(cmd) {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: the local store was not updated: %v\n", convergeErr)
					}
				}
				return fmt.Errorf("close deployment %s: %w", args[0], err)
			}

			if err := convergeClosedDeployment(cmd, rc, args[0]); err != nil && !cliutil.IsQuiet(cmd) {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: the local store was not updated: %v\n", err)
			}

			return printConsoleResult(cmd, fmt.Sprintf("Deployment %s closed.", args[0]), struct {
				DSeq  string `json:"dseq"`
				State string `json:"state"`
			}{args[0], "closed"})
		},
	}
}

func convergeClosedDeployment(cmd *cobra.Command, rc *aktctx.Context, dseqText string) error {
	if rc == nil || rc.Root == "" || rc.Name == "" {
		return nil
	}

	// Do not create an empty database merely because a user closed something
	// that was never tracked locally.
	if _, err := os.Stat(aktctx.StoreDBPath(rc.Root, rc.Name)); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect store: %w", err)
	}

	dseq, err := strconv.ParseUint(dseqText, 10, 64)
	if err != nil || dseq == 0 {
		return fmt.Errorf("cannot match non-numeric dseq %q to a local deployment", dseqText)
	}

	ctx := context.WithoutCancel(cmd.Context())
	s, err := bbolt.OpenContext(ctx, rc.Root, rc.Name)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	owner, err := sstore.UniqueDeploymentOwner(ctx, s, dseq)
	if err != nil || owner == "" {
		return err
	}

	return s.MarkDeploymentClosed(ctx, owner, dseq, time.Now().Unix())
}

func deploymentSettingsCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "settings <dseq> [hours|none]",
		Short: "View a deployment's funding record, or set its runtime limit",
		Long: "Show a deployment's funding record, or set the runtime limit that bounds how long " +
			"it runs before the platform closes it and returns the unused funds. Pass `none` to " +
			"clear the limit and return the deployment to always-on funding. " +
			"Automatic funding itself cannot be switched off.",
		Args: cobra.RangeArgs(1, 2),
		Example: `  # Show the current funding record
  akt console deployment settings 12345

  # Stop after 12 hours
  akt console deployment settings 12345 12

  # Back to always-on funding
  akt console deployment settings 12345 none`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			// Parsed before anything is sent: a rejected limit must not
			// reach the API.
			if len(args) > 1 {
				hours, err := parseRuntimeLimit(args[1])
				if err != nil {
					return err
				}

				settings, err := cl.SetDeploymentRuntimeLimit(cmd.Context(), args[0], hours)
				if err != nil {
					return fmt.Errorf("update deployment settings for %s: %w", args[0], err)
				}

				return printJSON(cmd, renderSettings(settings))
			}

			// GET /v2/deployment-settings/{dseq} is get-or-create: it answers
			// 200 for any dseq and mints a settings record as a side effect,
			// so a command documented as a view would also report funding
			// state for deployments that do not exist and for deployments
			// belonging to other accounts. Resolve the deployment first,
			// which is the 404 the sibling `deployment get` already returns.
			if _, err := cl.GetDeployment(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("deployment %s: %w", args[0], err)
			}

			settings, err := cl.GetDeploymentSettings(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get deployment settings for %s: %w", args[0], err)
			}

			return printJSON(cmd, renderSettings(settings))
		},
	}
}

// parseRuntimeLimit reads the positional runtime limit: a positive whole
// number of hours, or "none" to clear the limit (a nil result).
func parseRuntimeLimit(value string) (*int, error) {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "none" {
		return nil, nil
	}

	hours, err := strconv.Atoi(text)
	if err != nil {
		return nil, fmt.Errorf("runtime limit must be a whole number of hours or `none`, got %q", value)
	}
	if hours < 1 {
		return nil, fmt.Errorf("runtime limit must be at least 1 hour, got %d", hours)
	}

	return &hours, nil
}

// --- bids and leases --------------------------------------------------------

func bidCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bid",
		RunE:  sdkclient.ValidateCmd,
		Short: "Inspect provider bids",
	}

	cmd.AddCommand(&cobra.Command{
		Use:     "list <dseq>",
		Short:   "List bids for a deployment's open orders",
		Long:    "List bids for a deployment's open orders. Use `akt deploy <sdl-file>` to wait for and select a bid as part of the complete deployment flow.",
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
				// An empty list means "no bids", which is only worth waiting
				// on if the deployment exists. Reporting "providers may still
				// be bidding" for a mistyped dseq turned a typo into an
				// indefinite retry loop, in scripts as well as by hand.
				if _, err := cl.GetDeployment(cmd.Context(), args[0]); err != nil {
					return fmt.Errorf("deployment %s: %w", args[0], err)
				}

				return printConsoleText(cmd, "No bids yet (providers may still be bidding). Re-run in a few seconds.\n")
			}

			return printJSON(cmd, bids)
		},
	})

	return cmd
}

func leaseCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease",
		RunE:  sdkclient.ValidateCmd,
		Short: "Create leases from accepted bids",
	}

	create := &cobra.Command{
		Use:   "create <dseq> [provider]",
		Short: "Accept a bid by creating a lease and sending the manifest",
		Long: "Accept a bid by creating a lease and sending the deployment manifest to the " +
			"winning provider. The manifest defaults to the one cached by `deployment create`. " +
			"Use `akt deploy <sdl-file>` to perform creation, bid selection, and lease creation together.",
		Args: cobra.RangeArgs(1, 2),
		Example: `  # Provider as positional argument (gseq/oseq default to 1)
  akt console lease create 12345 akash1provider...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, rc, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			dseq := args[0]
			gseq, _ := cmd.Flags().GetUint32(flagdefs.FlagGSeq)
			oseq, _ := cmd.Flags().GetUint32(flagdefs.FlagOSeq)
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
			manifestFile, _ := cmd.Flags().GetString(flagdefs.FlagManifest)

			var manifest string
			switch {
			case manifestFile != "":
				manifest, err = manifestFromFile(manifestFile)
				if err != nil {
					return err
				}

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

	create.Flags().Uint32(flagdefs.FlagGSeq, 1, "Group sequence number")
	create.Flags().Uint32(flagdefs.FlagOSeq, 1, "Order sequence number")
	// FEEDBACK(2026-07): --provider disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// create.Flags().String("provider", "", "Provider address; alternative to the positional argument")
	create.Flags().String(flagdefs.FlagManifest, "", "Manifest file (defaults to the one cached by `deployment create`)")

	cmd.AddCommand(create)

	return cmd
}

// manifestFromFile reads a --manifest file, rejecting anything that is not
// JSON.
//
// The file wanted here is the manifest Console renders from the SDL, not the
// SDL itself. Passing the SDL is the obvious mistake -- same file, same
// directory -- and Console replies "invalid character '-' in numeric literal",
// which is its JSON parser hitting the leading `---` of the YAML. Checking
// locally names the actual cause and costs no API call.
func manifestFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read manifest file: %w", err)
	}

	if !json.Valid(data) {
		return "", fmt.Errorf("manifest file %s is not JSON: --manifest takes the manifest Console renders "+
			"(reported by `akt console deployment create`, cached per context), not the SDL", path)
	}

	return string(data), nil
}

// renderSettings formats a deployment's funding record for display, putting the top-up
// estimate through formatUSD like every other money value on this rail.
func renderSettings(s *console.DeploymentSettings) any {
	return struct {
		DSeq                 string  `json:"dseq"`
		AutoTopUpEnabled     bool    `json:"autoTopUpEnabled"`
		EstimatedTopUpAmount string  `json:"estimatedTopUpAmount"`
		TopUpFrequencyMs     int64   `json:"topUpFrequencyMs"`
		RuntimeLimitHours    *int    `json:"runtimeLimitHours"`
		RuntimeEndsAt        *string `json:"runtimeEndsAt"`
	}{
		DSeq:                 s.DSeq.String(),
		AutoTopUpEnabled:     s.AutoTopUpEnabled,
		EstimatedTopUpAmount: formatUSD(s.EstimatedTopUpUSD()),
		TopUpFrequencyMs:     s.TopUpFrequencyMs,
		RuntimeLimitHours:    s.RuntimeLimitHours,
		RuntimeEndsAt:        s.RuntimeEndsAt,
	}
}
