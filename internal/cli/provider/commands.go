// Package provider implements the `akt provider` CLI commands for interacting
// with provider gateway APIs.
package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	manifest "pkg.akt.dev/go/manifest/v2beta3"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	rest "pkg.akt.dev/go/provider/client"
	"pkg.akt.dev/go/sdl"

	"pkg.akt.dev/akt/internal/capability"
	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	aktclient "pkg.akt.dev/akt/internal/client"
	"pkg.akt.dev/akt/internal/output"
	aktprovider "pkg.akt.dev/akt/internal/provider"
)

// leaseProviderHelp is appended to every lease-scoped command's long help so
// the auto-resolution documented in SPEC §2.4 is discoverable from --help.
const leaseProviderHelp = "The provider is resolved from the deployment's active lease on chain, " +
	"so --provider is only needed to choose between several active leases or to override the lookup."

// Commands returns the `akt provider` command group.
func Commands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		RunE:  sdkclient.ValidateCmd,
		Short: "Provider gateway operations",
		Long:  "Interact with Akash provider gateway APIs: query status, manage leases, send manifests, and more.",
		// Capability gating: gateway discovery and wallet auth need chain access.
		Annotations: map[string]string{capability.AnnotationKey: string(capability.Provider)},
	}

	cmd.PersistentFlags().String("auth-type", "", "Provider auth type: jwt (default) or mtls")
	cmd.PersistentFlags().String("provider", "", "Provider address (bech32); default: the deployment's active lease")
	cmd.PersistentFlags().String("provider-url", "", "Provider gateway URL (e.g. https://provider.example.com:8443)")

	cmd.AddCommand(
		statusCmd(),
		leaseStatusCmd(),
		leaseLogsCmd(),
		leaseEventsCmd(),
		leaseShellCmd(),
		sendManifestCmd(),
		getManifestCmd(),
		migrateHostnamesCmd(),
		migrateEndpointsCmd(),
	)

	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [provider-addr]",
		Short: "Query provider status",
		Long:  "Query the live status of a provider including cluster capacity, active leases, and bid engine status.",
		Args:  cobra.MaximumNArgs(1),
		Example: `  # Query provider status by address
  akt provider status akash1...

  # Query provider status using --provider flag
  akt provider status --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if cmd.Flags().Changed("auth-type") {
				return fmt.Errorf("--auth-type does not apply to public provider status")
			}

			providerAddr, providerURL, err := resolveProvider(cmd, args)
			if err != nil {
				return err
			}

			cl, err := aktprovider.NewPublicGatewayClient(ctx, providerAddr, providerURL)
			if err != nil {
				return err
			}

			status, err := cl.Status(ctx)
			if err != nil {
				return aktprovider.GatewayError("query provider status", err)
			}

			return printJSON(cmd, status)
		},
	}
}

func leaseStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease-status [dseq]",
		Short: "Query lease deployment status",
		Long: "Query the live status of a lease from a provider, including service status, forwarded " +
			"ports, and IPs. " + leaseProviderHelp,
		Args: cobra.MaximumNArgs(1),
		Example: `  # Query lease status (provider resolved from the active lease)
  akt provider lease-status 12345

  # Pin the provider when several leases are active
  akt provider lease-status 12345 --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			lid, providerURL, err := resolveAuthenticatedLease(cmd, args)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerURL)
			if err != nil {
				return err
			}

			status, err := cl.LeaseStatus(ctx, lid)
			if err != nil {
				return aktprovider.GatewayError("query lease status", err)
			}

			return printJSON(cmd, status)
		},
	}

	addLeaseFlags(cmd)

	return cmd
}

func leaseLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease-logs [dseq]",
		Short: "Stream container logs",
		Long: "Stream container logs from a lease. Supports filtering by service and following " +
			"output. " + leaseProviderHelp,
		Args: cobra.MaximumNArgs(1),
		Example: `  # Stream logs for all services (provider resolved from the active lease)
  akt provider lease-logs 12345

  # Follow logs for a specific service
  akt provider lease-logs 12345 --service web --follow

  # Show last 100 lines
  akt provider lease-logs 12345 --tail 100

  # Pin the provider when several leases are active
  akt provider lease-logs 12345 --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			follow, _ := cmd.Flags().GetBool("follow")
			service, _ := cmd.Flags().GetString("service")
			tail, _ := cmd.Flags().GetInt64("tail")
			if err := aktprovider.ValidateLogTail(follow, tail); err != nil {
				return err
			}

			lid, providerURL, err := resolveAuthenticatedLease(cmd, args)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerURL)
			if err != nil {
				return err
			}

			if err := aktprovider.CheckLease(ctx, cl, lid); err != nil {
				return err
			}

			logs, err := cl.LeaseLogs(ctx, lid, service, follow, tail)
			if err != nil {
				return aktprovider.GatewayError("stream lease logs", err)
			}

			return consumeLeaseLogs(ctx, cmd, logs, service, follow, tail)
		},
	}

	addLeaseFlags(cmd)
	cmd.Flags().BoolP("follow", "f", false, "Follow log output")
	cmd.Flags().String("service", "", "Filter logs by service name")
	cmd.Flags().Int64("tail", -1, "Number of lines to show from the end of the logs")

	return cmd
}

func leaseEventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease-events [dseq]",
		Short: "Stream Kubernetes events",
		Long:  "Stream Kubernetes events for a lease from the provider. " + leaseProviderHelp,
		Args:  cobra.MaximumNArgs(1),
		Example: `  # Stream events (provider resolved from the active lease)
  akt provider lease-events 12345

  # Follow events
  akt provider lease-events 12345 --follow

  # Pin the provider when several leases are active
  akt provider lease-events 12345 --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			follow, _ := cmd.Flags().GetBool("follow")

			lid, providerURL, err := resolveAuthenticatedLease(cmd, args)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerURL)
			if err != nil {
				return err
			}

			if err := aktprovider.CheckLease(ctx, cl, lid); err != nil {
				return err
			}

			events, err := cl.LeaseEvents(ctx, lid, "", follow)
			if err != nil {
				return aktprovider.GatewayError("stream lease events", err)
			}

			return consumeLeaseEvents(ctx, cmd, events, follow)
		},
	}

	addLeaseFlags(cmd)
	cmd.Flags().BoolP("follow", "f", false, "Follow event output")

	return cmd
}

func leaseShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease-shell [-- command...]",
		Short: "Open interactive shell",
		Long: "Open an interactive shell session to a container in a lease. The remote command " +
			"defaults to /bin/sh. " + leaseProviderHelp,
		Args: cobra.ArbitraryArgs,
		Example: `  # Open the default /bin/sh shell
  akt provider lease-shell --dseq 12345 --service web

  # Open a bash shell
  akt provider lease-shell --dseq 12345 --service web -- /bin/bash

  # Run a single command
  akt provider lease-shell --dseq 12345 --service web -- ls -la

  # Capture an explicit command as structured stdout and stderr
  akt provider lease-shell --dseq 12345 --service web -o json -- pwd

  # Pin the provider when several leases are active
  akt provider lease-shell --dseq 12345 --provider akash1... --service web`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			shellCtx, cancelShell := context.WithCancel(ctx)
			defer cancelShell()
			interactive := len(args) == 0
			if err := output.ValidateShellOutput(cmd, interactive); err != nil {
				return err
			}

			// lease-shell consumes its positional args as the remote command,
			// so dseq must come from the --dseq flag here.
			lid, providerURL, err := resolveAuthenticatedLease(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerURL)
			if err != nil {
				return err
			}

			service, _ := cmd.Flags().GetString("service")
			if service == "" {
				return fmt.Errorf("--service is required for lease-shell")
			}
			tty, _ := cmd.Flags().GetBool("tty")
			stdinOverride, _ := cmd.Flags().GetBool("stdin")
			stdin := aktprovider.SelectShellStdin(
				shellCtx,
				os.Stdin,
				interactive,
				term.IsTerminal(int(os.Stdin.Fd())),
				cmd.Flags().Changed("stdin"),
				stdinOverride,
			)

			err = output.RunShellOutput(cmd, interactive, tty, func(stdout, stderr io.Writer, shellTTY bool) error {
				return aktprovider.RunLeaseShell(shellCtx, cl, lid, service, 0, leaseShellCommand(args),
					stdin, stdout, stderr, shellTTY, nil)
			})
			recordProviderAction(ctx, "lease-shell", lid.Provider, lid.DSeq, err)

			return err
		},
	}

	addLeaseShellFlags(cmd)
	cmd.Flags().String("service", "", "Service name (required)")
	cmd.Flags().BoolP("tty", "t", true, "Allocate a TTY")
	cmd.Flags().Bool("stdin", false, "Force stdin attachment for an explicit terminal command")

	return cmd
}

func sendManifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send-manifest <sdl-file>",
		Short: "Send SDL manifest to provider",
		Long: "Parse an SDL file and submit the resulting manifest to the deployment's providers. " +
			"Without --provider the manifest goes to every provider holding an active lease for " +
			"the deployment; each one is attempted even if an earlier one rejects it.",
		Args: cobra.ExactArgs(1),
		Example: `  # Send the manifest to every provider with an active lease
  akt provider send-manifest deploy.yaml --dseq 12345

  # Send it to one specific provider
  akt provider send-manifest deploy.yaml --dseq 12345 --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			scope, err := leaseScopeFromCmd(cmd, nil)
			if err != nil {
				return err
			}

			if _, _, _, err := gatewayAuthenticationFromCmd(cmd); err != nil {
				return err
			}

			sdlManifest, err := sdl.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read SDL file: %w", err)
			}

			mani, err := sdlManifest.Manifest()
			if err != nil {
				return fmt.Errorf("build manifest from SDL: %w", err)
			}

			providers, err := sendManifestTargets(cmd, scope, chainLeaseQuery(cmd))
			if err != nil {
				return err
			}

			var failures []error
			for _, provider := range providers {
				err := submitManifest(cmd, provider, scope.DSeq, mani)
				recordProviderAction(ctx, "send-manifest", provider, scope.DSeq, err)
				if err != nil {
					failures = append(failures, fmt.Errorf("provider %s: %w", provider, err))
					continue
				}

				fmt.Fprintf(cmd.OutOrStdout(), "Manifest submitted successfully to %s.\n", provider)
			}

			return errors.Join(failures...)
		},
	}

	cmd.Flags().Uint64("dseq", 0, "Deployment sequence number")

	return cmd
}

// sendManifestTargets resolves the providers a manifest is delivered to: the
// explicit --provider, or every provider with an active lease for the
// deployment (SPEC §2.4).
func sendManifestTargets(cmd *cobra.Command, scope leaseScope, leases leaseQuery) ([]string, error) {
	if addrStr, _ := cmd.Flags().GetString("provider"); addrStr != "" {
		addr, err := sdk.AccAddressFromBech32(addrStr)
		if err != nil {
			return nil, fmt.Errorf("invalid provider address %q: %w", addrStr, err)
		}

		return []string{addr.String()}, nil
	}

	providers, err := activeLeaseProviders(cmd.Context(), scope, leases)
	if err != nil {
		return nil, err
	}

	// A single explicit gateway URL cannot stand in for several providers.
	if providerURL, _ := cmd.Flags().GetString("provider-url"); providerURL != "" && len(providers) > 1 {
		return nil, fmt.Errorf(
			"--provider-url addresses one gateway but deployment %d has %d active leases; add --provider to choose one",
			scope.DSeq, len(providers))
	}

	return providers, nil
}

// submitManifest delivers the manifest to one provider, resolving that
// provider's gateway on the way.
func submitManifest(cmd *cobra.Command, provider string, dseq uint64, mani manifest.Manifest) error {
	providerURL, err := gatewayURL(cmd, provider, providerHostURILookup(cmd))
	if err != nil {
		return err
	}

	cl, err := gatewayClientFromCmd(cmd, providerURL)
	if err != nil {
		return err
	}

	if err := cl.SubmitManifest(cmd.Context(), dseq, mani); err != nil {
		return aktprovider.GatewayError("submit manifest", err)
	}

	return nil
}

func getManifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-manifest [dseq]",
		Short: "Retrieve current manifest",
		Long:  "Retrieve the current manifest for a lease from the provider. " + leaseProviderHelp,
		Args:  cobra.MaximumNArgs(1),
		Example: `  # Get manifest for a lease (provider resolved from the active lease)
  akt provider get-manifest 12345

  # Pin the provider when several leases are active
  akt provider get-manifest 12345 --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			lid, providerURL, err := resolveAuthenticatedLease(cmd, args)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerURL)
			if err != nil {
				return err
			}

			mani, err := cl.GetManifest(ctx, lid)
			if err != nil {
				return aktprovider.GatewayError("get manifest", err)
			}

			return printJSON(cmd, mani)
		},
	}

	addLeaseFlags(cmd)

	return cmd
}

func migrateHostnamesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-hostnames",
		Short: "Migrate hostnames between deployments",
		Long: "Migrate hostnames onto a deployment from whichever deployment currently holds them " +
			"on the same provider. --dseq/--gseq address the destination lease; the provider is " +
			"resolved from that deployment's active lease unless --provider names one.",
		Args: cobra.NoArgs,
		Example: `  # Migrate hostnames to a new deployment
  akt provider migrate-hostnames --dseq 12345 --hostnames example.com,app.example.com

  # Pin the destination group and provider
  akt provider migrate-hostnames --dseq 12345 --gseq 1 --provider akash1... --hostnames example.com`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			hostnames, _ := cmd.Flags().GetStringSlice("hostnames")
			if len(hostnames) == 0 {
				return fmt.Errorf("--hostnames is required")
			}

			lid, providerURL, err := resolveAuthenticatedLease(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerURL)
			if err != nil {
				return err
			}

			err = cl.MigrateHostnames(ctx, hostnames, lid.DSeq, lid.GSeq)
			recordProviderAction(ctx, "migrate-hostnames", lid.Provider, lid.DSeq, err)
			if err != nil {
				return aktprovider.GatewayError("migrate hostnames", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Hostnames migrated successfully.")
			return nil
		},
	}

	cmd.Flags().Uint64("dseq", 0, "Destination deployment sequence number")
	cmd.Flags().Uint32("gseq", 1, "Destination group sequence number")
	cmd.Flags().StringSlice("hostnames", nil, "Hostnames to migrate (comma-separated)")

	return cmd
}

func migrateEndpointsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-endpoints",
		Short: "Migrate endpoints between deployments",
		Long: "Migrate IP endpoints onto a deployment from whichever deployment currently holds them " +
			"on the same provider. --dseq/--gseq address the destination lease; the provider is " +
			"resolved from that deployment's active lease unless --provider names one.",
		Args: cobra.NoArgs,
		Example: `  # Migrate endpoints to a new deployment
  akt provider migrate-endpoints --dseq 12345 --endpoints ep1,ep2

  # Pin the destination group and provider
  akt provider migrate-endpoints --dseq 12345 --gseq 1 --provider akash1... --endpoints ep1`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			endpoints, _ := cmd.Flags().GetStringSlice("endpoints")
			if len(endpoints) == 0 {
				return fmt.Errorf("--endpoints is required")
			}

			lid, providerURL, err := resolveAuthenticatedLease(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerURL)
			if err != nil {
				return err
			}

			err = cl.MigrateEndpoints(ctx, endpoints, lid.DSeq, lid.GSeq)
			recordProviderAction(ctx, "migrate-endpoints", lid.Provider, lid.DSeq, err)
			if err != nil {
				return aktprovider.GatewayError("migrate endpoints", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Endpoints migrated successfully.")
			return nil
		},
	}

	cmd.Flags().Uint64("dseq", 0, "Destination deployment sequence number")
	cmd.Flags().Uint32("gseq", 1, "Destination group sequence number")
	cmd.Flags().StringSlice("endpoints", nil, "Endpoints to migrate (comma-separated)")

	return cmd
}

// --- helpers ---

// addLeaseFlags adds the common lease identification flags to the commands
// that take a positional [dseq] (lease-status, lease-logs, lease-events,
// get-manifest). lease-shell keeps its --dseq via addLeaseShellFlags.
func addLeaseFlags(cmd *cobra.Command) {
	// FEEDBACK(2026-07): --dseq disabled for the positional-only UX trial
	// (use the positional [dseq] argument instead). Restore by uncommenting
	// if users ask for the flag form back.
	// cmd.Flags().Uint64("dseq", 0, "Deployment sequence number")
	cmd.Flags().Uint32("gseq", 1, "Group sequence number")
	cmd.Flags().Uint32("oseq", 1, "Order sequence number")
}

// addLeaseShellFlags adds the lease identification flags for lease-shell,
// which consumes its positional args as the remote command and therefore
// keeps --dseq (no positional twin).
func addLeaseShellFlags(cmd *cobra.Command) {
	cmd.Flags().Uint64("dseq", 0, "Deployment sequence number")
	cmd.Flags().Uint32("gseq", 1, "Group sequence number")
	cmd.Flags().Uint32("oseq", 1, "Order sequence number")
}

// resolveProvider resolves the provider address and its on-chain HostURI. An
// explicit --provider-url remains an override for diagnostics and private
// gateways.
func resolveProvider(cmd *cobra.Command, args []string) (sdk.AccAddress, string, error) {
	return resolveProviderWithLookup(cmd, args, providerHostURILookup(cmd))
}

func providerHostURILookup(cmd *cobra.Command) hostURIQuery {
	return func(ctx context.Context, owner string) (string, error) {
		cctx, err := providerQueryContext(cmd)
		if err != nil {
			return "", err
		}

		res, err := ptypes.NewQueryClient(cctx).Provider(ctx, &ptypes.QueryProviderRequest{Owner: owner})
		if err != nil {
			return "", fmt.Errorf("query provider %s: %w", owner, err)
		}

		return res.Provider.HostURI, nil
	}
}

func providerQueryContext(cmd *cobra.Command) (sdkclient.Context, error) {
	cctx, err := chaincli.GetClientQueryContext(cmd)
	if err != nil {
		return sdkclient.Context{}, fmt.Errorf("initialize provider query client: %w", err)
	}
	if err := chaincli.SetCmdClientContext(cmd, cctx); err != nil {
		return sdkclient.Context{}, fmt.Errorf("store provider query client: %w", err)
	}

	return cctx, nil
}

// resolveProviderWithLookup backs `provider status`, the one command whose
// primary value genuinely is the provider address: it accepts it positionally,
// with --provider as the flag alternative. Lease-scoped commands never come
// through here — their positional slot is the dseq and their provider is
// resolved from the lease (see lease.go).
func resolveProviderWithLookup(
	cmd *cobra.Command,
	args []string,
	lookup hostURIQuery,
) (sdk.AccAddress, string, error) {
	var addrStr string

	if len(args) > 0 {
		addrStr = args[0]
	} else {
		addrStr, _ = cmd.Flags().GetString("provider")
	}

	if addrStr == "" {
		return nil, "", fmt.Errorf("provider address is required (positional argument or --provider flag)")
	}

	addr, err := sdk.AccAddressFromBech32(addrStr)
	if err != nil {
		return nil, "", fmt.Errorf("invalid provider address %q: %w", addrStr, err)
	}

	providerURL, err := gatewayURL(cmd, addr.String(), lookup)
	if err != nil {
		return nil, "", err
	}

	return addr, providerURL, nil
}

// gatewayClientFromCmd creates a provider gateway client from the command context.
func gatewayClientFromCmd(cmd *cobra.Command, providerURL string) (rest.Client, error) {
	cctx, accountAddr, authType, err := gatewayAuthenticationFromCmd(cmd)
	if err != nil {
		return nil, err
	}
	if authType == "mtls" {
		cctx, err = providerQueryContext(cmd)
		if err != nil {
			return nil, err
		}
		accountAddr = cctx.GetFromAddress()
	}

	return aktprovider.NewGatewayClient(
		cmd.Context(),
		cctx,
		accountAddr,
		providerURL,
		authType,
		cctx.Keyring,
	)
}

func gatewayAuthenticationFromCmd(cmd *cobra.Command) (sdkclient.Context, sdk.AccAddress, string, error) {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	authType, _ := cmd.Flags().GetString("auth-type")
	addr, err := aktclient.ResolveAccountAddress(cctx)
	if err != nil {
		return sdkclient.Context{}, nil, "", err
	}
	if !addr.Empty() {
		cctx = cctx.WithFromAddress(addr)
	}
	if err := aktprovider.ValidateGatewayAuthentication(addr, authType, cctx.Keyring); err != nil {
		return sdkclient.Context{}, nil, "", err
	}
	return cctx, addr, authType, nil
}

// printJSON keeps JSON as the default provider representation while honoring
// an explicit YAML selection with the provider's JSON field semantics.
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

func consumeLeaseLogs(
	ctx context.Context,
	cmd *cobra.Command,
	logs *rest.ServiceLogs,
	service string,
	follow bool,
	tail int64,
) error {
	buffered := tail >= 0
	var tailRecords []rest.ServiceLogMessage

	streamErr := aktprovider.ConsumeStream(ctx, "log", logs.Stream, logs.OnClose, follow,
		func(msg rest.ServiceLogMessage) error {
			if !aktprovider.MatchesService(msg.Name, service) {
				return nil
			}
			if buffered {
				tailRecords = aktprovider.RetainTail(tailRecords, msg, tail)
				return nil
			}
			return printProviderLog(cmd, msg)
		})

	for _, msg := range tailRecords {
		if err := printProviderLog(cmd, msg); err != nil {
			return err
		}
	}

	return streamErr
}

func consumeLeaseEvents(
	ctx context.Context,
	cmd *cobra.Command,
	events *rest.LeaseKubeEvents,
	follow bool,
) error {
	return aktprovider.ConsumeStream(ctx, "event", events.Stream, events.OnClose, follow,
		func(event rest.LeaseEvent) error {
			return printProviderEvent(cmd, event)
		})
}

func printProviderLog(cmd *cobra.Command, msg rest.ServiceLogMessage) error {
	return output.PrintStreamRecord(cmd, msg, fmt.Sprintf("[%s] %s", msg.Name, msg.Message))
}

func printProviderEvent(cmd *cobra.Command, event rest.LeaseEvent) error {
	pretty := fmt.Sprintf("%s [%s/%s] %s: %s",
		event.Type, event.Object.Kind, event.Object.Name, event.Reason, event.Note)

	return output.PrintStreamRecord(cmd, event, pretty)
}

func leaseShellCommand(args []string) []string {
	if len(args) == 0 {
		return []string{"/bin/sh"}
	}

	return args
}
