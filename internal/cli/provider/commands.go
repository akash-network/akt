// Package provider implements the `akt provider` CLI commands for interacting
// with provider gateway APIs.
package provider

import (
	"context"
	"fmt"
	"io"
	"os"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	mtypes "pkg.akt.dev/go/node/market/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	rest "pkg.akt.dev/go/provider/client"
	"pkg.akt.dev/go/sdl"

	"pkg.akt.dev/akt/internal/capability"
	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	"pkg.akt.dev/akt/internal/output"
	aktprovider "pkg.akt.dev/akt/internal/provider"
)

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
	cmd.PersistentFlags().String("provider", "", "Provider address (bech32)")
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

			providerAddr, providerURL, err := resolveProvider(cmd, args)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerAddr, providerURL)
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
		Long:  "Query the live status of a lease from a provider, including service status, forwarded ports, and IPs.",
		Args:  cobra.MaximumNArgs(1),
		Example: `  # Query lease status (positional dseq)
  akt provider lease-status 12345 --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			providerAddr, providerURL, err := resolveProvider(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerAddr, providerURL)
			if err != nil {
				return err
			}

			lid, err := leaseIDFromFlags(cmd, args, providerAddr)
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
		Long:  "Stream container logs from a lease. Supports filtering by service and following output.",
		Args:  cobra.MaximumNArgs(1),
		Example: `  # Stream logs for all services (positional dseq)
  akt provider lease-logs 12345 --provider akash1...

  # Follow logs for a specific service
  akt provider lease-logs 12345 --provider akash1... --service web --follow

  # Show last 100 lines
  akt provider lease-logs 12345 --provider akash1... --tail 100`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			follow, _ := cmd.Flags().GetBool("follow")
			service, _ := cmd.Flags().GetString("service")
			tail, _ := cmd.Flags().GetInt64("tail")
			if err := aktprovider.ValidateLogTail(follow, tail); err != nil {
				return err
			}

			providerAddr, providerURL, err := resolveProvider(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerAddr, providerURL)
			if err != nil {
				return err
			}

			lid, err := leaseIDFromFlags(cmd, args, providerAddr)
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
		Long:  "Stream Kubernetes events for a lease from the provider.",
		Args:  cobra.MaximumNArgs(1),
		Example: `  # Stream events (positional dseq)
  akt provider lease-events 12345 --provider akash1...

  # Follow events
  akt provider lease-events 12345 --provider akash1... --follow`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			follow, _ := cmd.Flags().GetBool("follow")

			providerAddr, providerURL, err := resolveProvider(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerAddr, providerURL)
			if err != nil {
				return err
			}

			lid, err := leaseIDFromFlags(cmd, args, providerAddr)
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
		Long:  "Open an interactive shell session to a container in a lease. The remote command defaults to /bin/sh.",
		Args:  cobra.ArbitraryArgs,
		Example: `  # Open the default /bin/sh shell
  akt provider lease-shell --dseq 12345 --provider akash1... --service web

  # Open a bash shell
  akt provider lease-shell --dseq 12345 --provider akash1... --service web -- /bin/bash

  # Run a single command
  akt provider lease-shell --dseq 12345 --provider akash1... --service web -- ls -la`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			shellCtx, cancelShell := context.WithCancel(ctx)
			defer cancelShell()

			providerAddr, providerURL, err := resolveProvider(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerAddr, providerURL)
			if err != nil {
				return err
			}

			// lease-shell consumes its positional args as the remote command,
			// so dseq must come from the --dseq flag here.
			lid, err := leaseIDFromFlags(cmd, nil, providerAddr)
			if err != nil {
				return err
			}

			service, _ := cmd.Flags().GetString("service")
			if service == "" {
				return fmt.Errorf("--service is required for lease-shell")
			}
			tty, _ := cmd.Flags().GetBool("tty")
			attachStdin, _ := cmd.Flags().GetBool("stdin")
			var stdin io.Reader
			if attachStdin {
				stdin = aktprovider.HoldEOF(shellCtx, os.Stdin)
			}

			err = aktprovider.RunLeaseShell(shellCtx, cl, lid, service, 0, leaseShellCommand(args),
				stdin, cmd.OutOrStdout(), cmd.ErrOrStderr(), tty, nil)
			recordProviderAction(ctx, "lease-shell", lid.Provider, lid.DSeq, err)

			return err
		},
	}

	addLeaseShellFlags(cmd)
	cmd.Flags().String("service", "", "Service name (required)")
	cmd.Flags().BoolP("tty", "t", true, "Allocate a TTY")
	cmd.Flags().Bool("stdin", true, "Attach stdin")

	return cmd
}

func sendManifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send-manifest <sdl-file>",
		Short: "Send SDL manifest to provider",
		Long:  "Parse an SDL file and submit the resulting manifest to the provider for a deployment.",
		Args:  cobra.ExactArgs(1),
		Example: `  # Send manifest from SDL file
  akt provider send-manifest deploy.yaml --dseq 12345 --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			providerAddr, providerURL, err := resolveProvider(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerAddr, providerURL)
			if err != nil {
				return err
			}

			dseq, _ := cmd.Flags().GetUint64("dseq")
			if dseq == 0 {
				return fmt.Errorf("--dseq is required")
			}

			sdlManifest, err := sdl.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read SDL file: %w", err)
			}

			mani, err := sdlManifest.Manifest()
			if err != nil {
				return fmt.Errorf("build manifest from SDL: %w", err)
			}

			err = cl.SubmitManifest(ctx, dseq, mani)
			recordProviderAction(ctx, "send-manifest", providerAddr.String(), dseq, err)
			if err != nil {
				return aktprovider.GatewayError("submit manifest", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Manifest submitted successfully.")
			return nil
		},
	}

	cmd.Flags().Uint64("dseq", 0, "Deployment sequence number")

	return cmd
}

func getManifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-manifest [dseq]",
		Short: "Retrieve current manifest",
		Long:  "Retrieve the current manifest for a lease from the provider.",
		Args:  cobra.MaximumNArgs(1),
		Example: `  # Get manifest for a lease (positional dseq)
  akt provider get-manifest 12345 --provider akash1...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			providerAddr, providerURL, err := resolveProvider(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerAddr, providerURL)
			if err != nil {
				return err
			}

			lid, err := leaseIDFromFlags(cmd, args, providerAddr)
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
		Long:  "Migrate hostnames from one deployment to another on the same provider.",
		Args:  cobra.NoArgs,
		Example: `  # Migrate hostnames to a new deployment
  akt provider migrate-hostnames --dseq 12345 --gseq 1 --provider akash1... --hostnames example.com,app.example.com`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			providerAddr, providerURL, err := resolveProvider(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerAddr, providerURL)
			if err != nil {
				return err
			}

			dseq, _ := cmd.Flags().GetUint64("dseq")
			if dseq == 0 {
				return fmt.Errorf("--dseq is required")
			}

			gseq, _ := cmd.Flags().GetUint32("gseq")
			hostnames, _ := cmd.Flags().GetStringSlice("hostnames")
			if len(hostnames) == 0 {
				return fmt.Errorf("--hostnames is required")
			}

			err = cl.MigrateHostnames(ctx, hostnames, dseq, gseq)
			recordProviderAction(ctx, "migrate-hostnames", providerAddr.String(), dseq, err)
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
		Long:  "Migrate endpoints from one deployment to another on the same provider.",
		Args:  cobra.NoArgs,
		Example: `  # Migrate endpoints to a new deployment
  akt provider migrate-endpoints --dseq 12345 --gseq 1 --provider akash1... --endpoints ep1,ep2`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			providerAddr, providerURL, err := resolveProvider(cmd, nil)
			if err != nil {
				return err
			}

			cl, err := gatewayClientFromCmd(cmd, providerAddr, providerURL)
			if err != nil {
				return err
			}

			dseq, _ := cmd.Flags().GetUint64("dseq")
			if dseq == 0 {
				return fmt.Errorf("--dseq is required")
			}

			gseq, _ := cmd.Flags().GetUint32("gseq")
			endpoints, _ := cmd.Flags().GetStringSlice("endpoints")
			if len(endpoints) == 0 {
				return fmt.Errorf("--endpoints is required")
			}

			err = cl.MigrateEndpoints(ctx, endpoints, dseq, gseq)
			recordProviderAction(ctx, "migrate-endpoints", providerAddr.String(), dseq, err)
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

// leaseIDFromFlags builds a LeaseID from the command flags and an optional
// positional dseq argument. Only lease-shell still registers --dseq
// (FEEDBACK 2026-07: the flag is disabled on the positional-[dseq] commands
// for the positional-only UX trial); for those commands the flag lookup
// below yields zero and the positional argument is the sole source.
func leaseIDFromFlags(cmd *cobra.Command, args []string, provider sdk.AccAddress) (mtypes.LeaseID, error) {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	owner := cctx.GetFromAddress()

	dseq, _ := cmd.Flags().GetUint64("dseq")

	dseq, err := cflags.DSeqFromArgs(args, dseq)
	if err != nil {
		return mtypes.LeaseID{}, err
	}

	if dseq == 0 {
		return mtypes.LeaseID{}, fmt.Errorf("dseq is required: provide the positional [dseq] argument (or --dseq for lease-shell)")
	}

	gseq, _ := cmd.Flags().GetUint32("gseq")
	oseq, _ := cmd.Flags().GetUint32("oseq")

	return mtypes.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider.String(),
	}, nil
}

// resolveProvider resolves the provider address and its on-chain HostURI. An
// explicit --provider-url remains an override for diagnostics and private
// gateways.
func resolveProvider(cmd *cobra.Command, args []string) (sdk.AccAddress, string, error) {
	return resolveProviderWithLookup(cmd, args, func(ctx context.Context, owner string) (string, error) {
		cctx, err := providerQueryContext(cmd)
		if err != nil {
			return "", err
		}

		res, err := ptypes.NewQueryClient(cctx).Provider(ctx, &ptypes.QueryProviderRequest{Owner: owner})
		if err != nil {
			return "", fmt.Errorf("query provider %s: %w", owner, err)
		}

		return res.Provider.HostURI, nil
	})
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

func resolveProviderWithLookup(
	cmd *cobra.Command,
	args []string,
	lookup func(context.Context, string) (string, error),
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

	providerURL, _ := cmd.Flags().GetString("provider-url")
	if providerURL != "" {
		return addr, providerURL, nil
	}

	providerURL, err = lookup(cmd.Context(), addr.String())
	if err != nil {
		return nil, "", err
	}
	if providerURL == "" {
		return nil, "", fmt.Errorf("provider %s has no host URI on chain", addr)
	}

	return addr, providerURL, nil
}

// gatewayClientFromCmd creates a provider gateway client from the command context.
func gatewayClientFromCmd(cmd *cobra.Command, providerAddr sdk.AccAddress, providerURL string) (rest.Client, error) {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	authType, _ := cmd.Flags().GetString("auth-type")
	if authType == "mtls" {
		var err error
		cctx, err = providerQueryContext(cmd)
		if err != nil {
			return nil, err
		}
	}

	return aktprovider.NewGatewayClient(
		cmd.Context(),
		cctx,
		cctx.GetFromAddress(),
		providerURL,
		authType,
		cctx.Keyring,
	)
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
