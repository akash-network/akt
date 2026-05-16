// Package provider implements the `akt provider` CLI commands for interacting
// with provider gateway APIs.
package provider

import (
	"encoding/json"
	"fmt"
	"os"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	mtypes "pkg.akt.dev/go/node/market/v1"
	rest "pkg.akt.dev/go/provider/client"
	"pkg.akt.dev/go/sdl"

	aktprovider "pkg.akt.dev/akt/internal/provider"
)

// Commands returns the `akt provider` command group.
func Commands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Provider gateway operations",
		Long:  "Interact with Akash provider gateway APIs: query status, manage leases, send manifests, and more.",
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
				return fmt.Errorf("query provider status: %w", err)
			}

			return printJSON(cmd, status)
		},
	}
}

func leaseStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease-status",
		Short: "Query lease deployment status",
		Long:  "Query the live status of a lease from a provider, including service status, forwarded ports, and IPs.",
		Args:  cobra.NoArgs,
		Example: `  # Query lease status
  akt provider lease-status --dseq 12345 --provider akash1...`,
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

			lid, err := leaseIDFromFlags(cmd, providerAddr)
			if err != nil {
				return err
			}

			status, err := cl.LeaseStatus(ctx, lid)
			if err != nil {
				return fmt.Errorf("query lease status: %w", err)
			}

			return printJSON(cmd, status)
		},
	}

	addLeaseFlags(cmd)

	return cmd
}

func leaseLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease-logs",
		Short: "Stream container logs",
		Long:  "Stream container logs from a lease. Supports filtering by service and following output.",
		Args:  cobra.NoArgs,
		Example: `  # Stream logs for all services
  akt provider lease-logs --dseq 12345 --provider akash1...

  # Follow logs for a specific service
  akt provider lease-logs --dseq 12345 --provider akash1... --service web --follow

  # Show last 100 lines
  akt provider lease-logs --dseq 12345 --provider akash1... --tail 100`,
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

			lid, err := leaseIDFromFlags(cmd, providerAddr)
			if err != nil {
				return err
			}

			follow, _ := cmd.Flags().GetBool("follow")
			service, _ := cmd.Flags().GetString("service")
			tail, _ := cmd.Flags().GetInt64("tail")

			logs, err := cl.LeaseLogs(ctx, lid, service, follow, tail)
			if err != nil {
				return fmt.Errorf("stream lease logs: %w", err)
			}

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case msg, ok := <-logs.Stream:
					if !ok {
						return nil
					}
					fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n", msg.Name, msg.Message)
				case reason, ok := <-logs.OnClose:
					if !ok {
						return nil
					}
					if reason != "" {
						return fmt.Errorf("log stream closed: %s", reason)
					}
					return nil
				}
			}
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
		Use:   "lease-events",
		Short: "Stream Kubernetes events",
		Long:  "Stream Kubernetes events for a lease from the provider.",
		Args:  cobra.NoArgs,
		Example: `  # Stream events
  akt provider lease-events --dseq 12345 --provider akash1...

  # Follow events
  akt provider lease-events --dseq 12345 --provider akash1... --follow`,
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

			lid, err := leaseIDFromFlags(cmd, providerAddr)
			if err != nil {
				return err
			}

			follow, _ := cmd.Flags().GetBool("follow")

			events, err := cl.LeaseEvents(ctx, lid, "", follow)
			if err != nil {
				return fmt.Errorf("stream lease events: %w", err)
			}

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case evt, ok := <-events.Stream:
					if !ok {
						return nil
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s [%s/%s] %s: %s\n",
						evt.Type, evt.Object.Kind, evt.Object.Name, evt.Reason, evt.Note)
				case reason, ok := <-events.OnClose:
					if !ok {
						return nil
					}
					if reason != "" {
						return fmt.Errorf("event stream closed: %s", reason)
					}
					return nil
				}
			}
		},
	}

	addLeaseFlags(cmd)
	cmd.Flags().BoolP("follow", "f", false, "Follow event output")

	return cmd
}

func leaseShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease-shell -- [command]",
		Short: "Open interactive shell",
		Long:  "Open an interactive shell session to a container in a lease.",
		Args:  cobra.MinimumNArgs(1),
		Example: `  # Open a bash shell
  akt provider lease-shell --dseq 12345 --provider akash1... --service web -- /bin/bash

  # Run a single command
  akt provider lease-shell --dseq 12345 --provider akash1... --service web -- ls -la`,
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

			lid, err := leaseIDFromFlags(cmd, providerAddr)
			if err != nil {
				return err
			}

			service, _ := cmd.Flags().GetString("service")
			if service == "" {
				return fmt.Errorf("--service is required for lease-shell")
			}

			tty, _ := cmd.Flags().GetBool("tty")

			return cl.LeaseShell(ctx, lid, service, 0, args,
				os.Stdin, os.Stdout, os.Stderr, tty, nil)
		},
	}

	addLeaseFlags(cmd)
	cmd.Flags().String("service", "", "Service name (required)")
	cmd.Flags().BoolP("tty", "t", true, "Allocate a TTY")

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

			if err := cl.SubmitManifest(ctx, dseq, mani); err != nil {
				return fmt.Errorf("submit manifest: %w", err)
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
		Use:   "get-manifest",
		Short: "Retrieve current manifest",
		Long:  "Retrieve the current manifest for a lease from the provider.",
		Args:  cobra.NoArgs,
		Example: `  # Get manifest for a lease
  akt provider get-manifest --dseq 12345 --provider akash1...`,
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

			lid, err := leaseIDFromFlags(cmd, providerAddr)
			if err != nil {
				return err
			}

			mani, err := cl.GetManifest(ctx, lid)
			if err != nil {
				return fmt.Errorf("get manifest: %w", err)
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

			if err := cl.MigrateHostnames(ctx, hostnames, dseq, gseq); err != nil {
				return fmt.Errorf("migrate hostnames: %w", err)
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

			if err := cl.MigrateEndpoints(ctx, endpoints, dseq, gseq); err != nil {
				return fmt.Errorf("migrate endpoints: %w", err)
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

// addLeaseFlags adds the common lease identification flags to a command.
func addLeaseFlags(cmd *cobra.Command) {
	cmd.Flags().Uint64("dseq", 0, "Deployment sequence number")
	cmd.Flags().Uint32("gseq", 1, "Group sequence number")
	cmd.Flags().Uint32("oseq", 1, "Order sequence number")
}

// leaseIDFromFlags builds a LeaseID from the command flags.
func leaseIDFromFlags(cmd *cobra.Command, provider sdk.AccAddress) (mtypes.LeaseID, error) {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	owner := cctx.GetFromAddress()

	dseq, _ := cmd.Flags().GetUint64("dseq")
	if dseq == 0 {
		return mtypes.LeaseID{}, fmt.Errorf("--dseq is required")
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

// resolveProvider resolves the provider address and URL. If a positional arg is
// given it is used as the provider address; otherwise --provider is required.
// The provider URL is queried from chain via the provider's on-chain HostURI.
// For now, we use the address directly as the URL placeholder — the actual
// on-chain query will be added when the query client is wired in.
func resolveProvider(cmd *cobra.Command, args []string) (sdk.AccAddress, string, error) {
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

	// Query the provider's on-chain record for HostURI.
	// For now, use the --provider-url flag if available, or require it.
	providerURL, _ := cmd.Flags().GetString("provider-url")
	if providerURL == "" {
		// TODO: query on-chain provider record for HostURI when query client is wired in.
		return nil, "", fmt.Errorf("--provider-url is required (on-chain provider URL lookup not yet implemented)")
	}

	return addr, providerURL, nil
}

// gatewayClientFromCmd creates a provider gateway client from the command context.
func gatewayClientFromCmd(cmd *cobra.Command, providerAddr sdk.AccAddress, providerURL string) (rest.Client, error) {
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	authType, _ := cmd.Flags().GetString("auth-type")

	return aktprovider.NewGatewayClient(
		cmd.Context(),
		cctx,
		cctx.GetFromAddress(),
		providerURL,
		authType,
		cctx.Keyring,
	)
}

// printJSON marshals v to indented JSON and writes it to the command's output.
func printJSON(cmd *cobra.Command, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal output: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
