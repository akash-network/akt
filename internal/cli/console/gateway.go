package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"k8s.io/apimachinery/pkg/api/resource"

	mtypes "pkg.akt.dev/go/node/market/v1"
	attrv1 "pkg.akt.dev/go/node/types/attributes/v1"
	rest "pkg.akt.dev/go/provider/client"
	"pkg.akt.dev/go/sdl"

	// Registers the akash bech32 prefixes with the cosmos-sdk global config
	// (sealed in sdkutil's init), so provider addresses parse correctly even
	// when this package is used without the chain command tree.
	_ "pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/akt/internal/console"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
	aktprovider "pkg.akt.dev/akt/internal/provider"
)

// JWT lifetimes for Console-minted provider gateway tokens, in seconds.
// One-shot calls get a short token; streams (follow/watch/shell) get an hour.
const (
	gatewayJWTTTLOneShot = 300
	gatewayJWTTTLStream  = 3600
)

// --- gateway resolution -------------------------------------------------------

// gatewayForDeployment resolves a Console-managed deployment to an
// authenticated provider gateway client, without any wallet or local key:
//
//  1. GetDeployment(dseq) and pick the first active lease,
//  2. GetProvider(lease.provider) for the gateway hostUri,
//  3. CreateJWTToken(ttl, scope) to mint a Console-signed scoped JWT that
//     provider gateways accept as `Authorization: Bearer`,
//  4. build a pkg.akt.dev provider REST client against the hostUri.
//
// ttl is in seconds (gatewayJWTTTLOneShot / gatewayJWTTTLStream); scope lists
// the gateway permissions the token grants (e.g. "status", "logs").
func gatewayForDeployment(cmd *cobra.Command, cl *console.Client, dseq string, ttl int, scope []string) (rest.Client, mtypes.LeaseID, error) {
	ctx := cmd.Context()

	detail, err := cl.GetDeployment(ctx, dseq)
	if err != nil {
		return nil, mtypes.LeaseID{}, fmt.Errorf("get deployment %s: %w", dseq, err)
	}

	lease, err := activeLease(detail, dseq)
	if err != nil {
		return nil, mtypes.LeaseID{}, err
	}

	lid, err := leaseIDFromConsole(detail, lease)
	if err != nil {
		return nil, mtypes.LeaseID{}, err
	}

	prov, err := cl.GetProvider(ctx, lease.ID.Provider)
	if err != nil {
		return nil, mtypes.LeaseID{}, fmt.Errorf("get provider %s: %w", lease.ID.Provider, err)
	}
	if prov.HostURI == "" {
		return nil, mtypes.LeaseID{}, fmt.Errorf("provider %s has no gateway hostUri in the Console catalog", lease.ID.Provider)
	}

	token, err := cl.CreateJWTToken(ctx, ttl, scope)
	if err != nil {
		return nil, mtypes.LeaseID{}, fmt.Errorf("mint provider JWT (scope %s): %w", strings.Join(scope, ","), err)
	}

	addr, err := sdk.AccAddressFromBech32(lease.ID.Provider)
	if err != nil {
		return nil, mtypes.LeaseID{}, fmt.Errorf("invalid provider address %q: %w", lease.ID.Provider, err)
	}

	gw, err := aktprovider.NewTokenGatewayClient(ctx, addr, prov.HostURI, token)
	if err != nil {
		return nil, mtypes.LeaseID{}, fmt.Errorf("create provider gateway client for %s: %w", prov.HostURI, err)
	}

	return gw, lid, nil
}

// activeLease returns the deployment's first active lease, or an error that
// names the states of the leases that do exist.
func activeLease(detail *console.DeploymentDetail, dseq string) (*console.Lease, error) {
	if len(detail.Leases) == 0 {
		return nil, fmt.Errorf("deployment %s has no leases yet: create one with `akt console lease create %s <provider>`", dseq, dseq)
	}

	states := make([]string, 0, len(detail.Leases))
	for i := range detail.Leases {
		if detail.Leases[i].State == "active" {
			return &detail.Leases[i], nil
		}
		states = append(states, detail.Leases[i].State)
	}

	return nil, fmt.Errorf("deployment %s has no active lease (lease states: %s)", dseq, strings.Join(states, ", "))
}

// leaseIDFromConsole converts a Console lease record into a chain LeaseID for
// the provider gateway client. The owner falls back to the deployment owner
// when the lease record omits it.
func leaseIDFromConsole(detail *console.DeploymentDetail, lease *console.Lease) (mtypes.LeaseID, error) {
	dseq, err := strconv.ParseUint(lease.ID.DSeq.String(), 10, 64)
	if err != nil {
		return mtypes.LeaseID{}, fmt.Errorf("invalid lease dseq %q: %w", lease.ID.DSeq.String(), err)
	}

	owner := lease.ID.Owner
	if owner == "" {
		owner = detail.Deployment.ID.Owner
	}

	return mtypes.LeaseID{
		Owner:    owner,
		DSeq:     dseq,
		GSeq:     lease.ID.GSeq,
		OSeq:     lease.ID.OSeq,
		Provider: lease.ID.Provider,
	}, nil
}

// --- commands -----------------------------------------------------------------

func logsCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs <dseq> [service]",
		Short: "Stream container logs from the lease's provider",
		Long: "Stream container logs for a Console-managed deployment directly from the provider " +
			"gateway. The provider is resolved from the deployment's active lease and access is " +
			"authorized by a short-lived Console-minted JWT. JSON output is one compact object " +
			"per line; YAML output is one document per record.",
		Args: cobra.RangeArgs(1, 2),
		Example: `  # All services
  akt console logs 12345

  # One service, following output
  akt console logs 12345 web --follow

  # Last 100 lines
  akt console logs 12345 --tail 100`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			follow, _ := cmd.Flags().GetBool(flagdefs.FlagFollow)
			tail, _ := cmd.Flags().GetInt64(flagdefs.FlagTail)
			if err := aktprovider.ValidateLogTail(follow, tail); err != nil {
				return err
			}
			// FEEDBACK(2026-07): --service disabled for the positional-only
			// UX trial; the positional [service] argument is the only
			// source. Restore by uncommenting if users ask for the flag form
			// back.
			// service, _ := cmd.Flags().GetString("service")
			service := ""
			if len(args) > 1 {
				service = args[1]
			}

			ttl := gatewayJWTTTLOneShot
			if follow {
				ttl = gatewayJWTTTLStream
			}

			gw, lid, err := gatewayForDeployment(cmd, cl, args[0], ttl, []string{"logs", "status"})
			if err != nil {
				return err
			}
			if err := aktprovider.CheckLease(ctx, gw, lid); err != nil {
				return err
			}

			logs, err := gw.LeaseLogs(ctx, lid, service, follow, tail)
			if err != nil {
				return aktprovider.GatewayError("stream lease logs", err)
			}

			// Neither filter survives the round trip, so both are applied
			// here as well and the flags do what they claim:
			//
			//   - tail: the provider client takes the parameter and drops it
			//     (`_ int64` in its LeaseLogs signature), so nothing is sent.
			//   - service: the client does send ?services=, and the provider
			//     returns every service anyway.
			//
			// Buffering only happens for a bounded one-shot read; --follow
			// streams as before, since "last N lines of an endless stream"
			// has no meaning and buffering it would hang.
			buffered := tail >= 0
			var tailBuf []providerLogMsg

			emit := func(msg providerLogMsg) error {
				if !aktprovider.MatchesService(msg.Name, service) {
					return nil
				}

				if buffered {
					tailBuf = aktprovider.RetainTail(tailBuf, msg, tail)
					return nil
				}

				return output.PrintStreamRecord(cmd, msg, fmt.Sprintf("[%s] %s", msg.Name, msg.Message))
			}

			flush := func() error {
				for _, msg := range tailBuf {
					if err := output.PrintStreamRecord(cmd, msg, fmt.Sprintf("[%s] %s", msg.Name, msg.Message)); err != nil {
						return err
					}
				}

				return nil
			}

			streamErr := aktprovider.ConsumeStream(ctx, "log", logs.Stream, logs.OnClose, follow,
				func(msg rest.ServiceLogMessage) error {
					return emit(providerLogMsg{Name: msg.Name, Message: msg.Message})
				})
			if err := flush(); err != nil {
				return err
			}

			return streamErr
		},
	}

	cmd.Flags().BoolP(flagdefs.FlagFollow, "f", false, "Follow log output")
	cmd.Flags().Int64(flagdefs.FlagTail, -1, "Number of lines to show from the end of the logs")
	// FEEDBACK(2026-07): --service disabled for the positional-only UX trial
	// (use the positional form instead). Restore by uncommenting if users
	// ask for the flag form back.
	// cmd.Flags().String("service", "", "Filter logs by service name; alternative to the positional argument")

	return cmd
}

func eventsCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events <dseq>",
		Short: "Stream Kubernetes events from the lease's provider",
		Long: "Stream Kubernetes events for a Console-managed deployment directly from the provider " +
			"gateway, authorized by a short-lived Console-minted JWT. JSON output is one compact " +
			"object per line; YAML output is one document per record.",
		Args: cobra.ExactArgs(1),
		Example: `  # Recent events
  akt console events 12345

  # Follow events
  akt console events 12345 --follow`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			follow, _ := cmd.Flags().GetBool(flagdefs.FlagFollow)

			ttl := gatewayJWTTTLOneShot
			if follow {
				ttl = gatewayJWTTTLStream
			}

			gw, lid, err := gatewayForDeployment(cmd, cl, args[0], ttl, []string{"events", "status"})
			if err != nil {
				return err
			}
			if err := aktprovider.CheckLease(ctx, gw, lid); err != nil {
				return err
			}

			return streamLeaseEvents(ctx, cmd, gw, lid, follow)
		},
	}

	cmd.Flags().BoolP(flagdefs.FlagFollow, "f", false, "Follow event output")

	return cmd
}

type leaseEventsClient interface {
	LeaseEvents(context.Context, mtypes.LeaseID, string, bool) (*rest.LeaseKubeEvents, error)
}

func streamLeaseEvents(
	ctx context.Context,
	cmd *cobra.Command,
	client leaseEventsClient,
	lid mtypes.LeaseID,
	follow bool,
) error {
	events, err := client.LeaseEvents(ctx, lid, "", follow)
	if err != nil {
		return aktprovider.GatewayError("stream lease events", err)
	}

	return consumeLeaseEvents(ctx, cmd, events.Stream, events.OnClose, follow)
}

func consumeLeaseEvents(
	ctx context.Context,
	cmd *cobra.Command,
	stream <-chan rest.LeaseEvent,
	onClose <-chan string,
	follow bool,
) error {
	count := 0
	if err := aktprovider.ConsumeStream(ctx, "event", stream, onClose, follow,
		func(event rest.LeaseEvent) error {
			count++
			return printLeaseEvent(cmd, event)
		}); err != nil {
		return err
	}

	return printEmptyEvents(cmd, count, follow)
}

func statusCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <dseq>",
		Short: "Live lease status from the provider gateway",
		Long: "Query the live status of a Console-managed deployment's lease straight from the " +
			"provider gateway (services, forwarded ports, IPs), authorized by a short-lived " +
			"Console-minted JWT. `akt console deployment get` remains the Console-API view of the " +
			"same deployment. With --watch the status is re-polled until interrupted (or the " +
			"one-hour token expires).",
		Args: cobra.ExactArgs(1),
		Example: `  # One snapshot
  akt console status 12345

  # Poll every 5s until Ctrl-C
  akt console status 12345 --watch

  # Poll every 30s
  akt console status 12345 --watch --interval 30s`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			watch, _ := cmd.Flags().GetBool(flagdefs.FlagWatch)
			interval, _ := cmd.Flags().GetDuration(flagdefs.FlagInterval)
			if interval <= 0 {
				interval = 5 * time.Second
			}

			ttl := gatewayJWTTTLOneShot
			if watch {
				ttl = gatewayJWTTTLStream
			}

			gw, lid, err := gatewayForDeployment(cmd, cl, args[0], ttl, []string{"status"})
			if err != nil {
				return err
			}

			status, err := gw.LeaseStatus(ctx, lid)
			if err != nil {
				return aktprovider.GatewayError("query lease status", err)
			}
			if err := printJSON(cmd, status); err != nil {
				return err
			}

			if !watch {
				return nil
			}

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(interval):
				}

				status, err := gw.LeaseStatus(ctx, lid)
				if err != nil {
					return aktprovider.GatewayError("query lease status", err)
				}
				if err := printJSON(cmd, status); err != nil {
					return err
				}
			}
		},
	}

	cmd.Flags().Bool(flagdefs.FlagWatch, false, "Keep polling and printing status snapshots until interrupted")
	cmd.Flags().Duration(flagdefs.FlagInterval, 5*time.Second, "Polling interval used with --watch")

	return cmd
}

type consoleLeaseShellRunner func(
	context.Context,
	aktprovider.LeaseShellClient,
	mtypes.LeaseID,
	string,
	[]string,
	io.Reader,
	io.Writer,
	io.Writer,
	bool,
) error

func runConsoleLeaseShell(
	ctx context.Context,
	client aktprovider.LeaseShellClient,
	id mtypes.LeaseID,
	service string,
	command []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
) error {
	return aktprovider.RunLeaseShell(ctx, client, id, service, 0, command,
		stdin, stdout, stderr, tty, nil)
}

func shellCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return shellCmdWithRunner(mgrFn, runConsoleLeaseShell)
}

func shellCmdWithRunner(mgrFn func() *aktctx.Manager, run consoleLeaseShellRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell <dseq> [service] [-- command...]",
		Short: "Open a shell or run a command in a lease container",
		Long: "Open an interactive shell (default command /bin/sh) or run an explicit command in a " +
			"container of a Console-managed deployment, connecting to the provider gateway with a " +
			"short-lived Console-minted JWT — no wallet involved. exec is the same operation as " +
			"shell with an explicit command: `akt console shell <dseq> <svc> -- <cmd>`. A TTY is " +
			"allocated automatically when stdin is a terminal.",
		Args: cobra.MinimumNArgs(1),
		Example: `  # Interactive shell (defaults to /bin/sh)
  akt console shell 12345 web

  # Run a single command (a.k.a. exec)
  akt console shell 12345 web -- ls -la

  # Capture an explicit command as structured stdout and stderr
  akt console shell 12345 web -o json -- pwd`,
		RunE: func(cmd *cobra.Command, args []string) error {
			shellCtx, cancelShell := context.WithCancel(cmd.Context())
			defer cancelShell()
			dash := cmd.ArgsLenAtDash()
			service := ""
			var command []string
			switch {
			case dash >= 0:
				if dash > 1 {
					service = args[1]
				}
				command = args[dash:]
			case len(args) > 1:
				service = args[1]
				command = args[2:]
			}

			interactive := len(command) == 0
			if err := output.ValidateShellOutput(cmd, interactive); err != nil {
				return err
			}
			if interactive && !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("interactive shell requires a terminal; non-interactive stdin must provide an explicit command after `--`")
			}

			cl, rc, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			dseq := args[0]
			if service == "" {
				service, err = defaultShellService(rc, dseq)
				if err != nil {
					return err
				}
			}
			if len(command) == 0 {
				command = []string{"/bin/sh"}
			}

			gw, lid, err := gatewayForDeployment(cmd, cl, dseq, gatewayJWTTTLStream, []string{"shell", "status"})
			if err != nil {
				return err
			}
			tty := term.IsTerminal(int(os.Stdin.Fd()))
			stdinOverride, _ := cmd.Flags().GetBool(flagdefs.FlagStdin)
			stdin := aktprovider.SelectShellStdin(
				shellCtx,
				os.Stdin,
				interactive,
				tty,
				cmd.Flags().Changed(flagdefs.FlagStdin),
				stdinOverride,
			)

			err = output.RunShellOutput(cmd, interactive, tty, func(stdout, stderr io.Writer, shellTTY bool) error {
				return run(shellCtx, gw, lid, service, command, stdin, stdout, stderr, shellTTY)
			})
			aktprovider.RecordAction(cmd.Context(), "lease-shell", lid.Provider, lid.DSeq, err)

			return err
		},
	}

	cmd.Flags().Bool(flagdefs.FlagStdin, false, "Force stdin attachment for an explicit terminal command")

	return cmd
}

func defaultShellService(rc *aktctx.Context, dseq string) (string, error) {
	if rc == nil {
		return "", fmt.Errorf("service is required without an active context")
	}
	raw, err := console.LoadManifest(rc.Root, rc.Name, dseq)
	if err != nil {
		return "", fmt.Errorf("resolve shell service for deployment %s: %w; pass the service explicitly", dseq, err)
	}
	names, err := manifestServiceNames(raw)
	if err != nil {
		return "", fmt.Errorf("resolve shell service for deployment %s: %w", dseq, err)
	}
	switch len(names) {
	case 0:
		return "", fmt.Errorf("deployment %s manifest contains no services", dseq)
	case 1:
		return names[0], nil
	default:
		return "", fmt.Errorf("deployment %s has multiple services (%s); pass one explicitly", dseq, strings.Join(names, ", "))
	}
}

func manifestServiceNames(raw string) ([]string, error) {
	var groups []struct {
		Services []struct {
			Name string `json:"name"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return nil, fmt.Errorf("decode cached manifest: %w", err)
	}
	unique := make(map[string]struct{})
	for _, group := range groups {
		for _, service := range group.Services {
			if name := strings.TrimSpace(service.Name); name != "" {
				unique[name] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)

	return names, nil
}

// --- bid screening ------------------------------------------------------------

func screenCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "screen [sdl-file]",
		Short: "List providers able to run an SDL (bid screening)",
		Long: "Ask the Console API which providers can satisfy resource requirements before " +
			"creating a deployment. Start from an SDL, resource flags, or both; explicit flags " +
			"override SDL-derived values. The endpoint is public, so no API key is needed.",
		Args: cobra.MaximumNArgs(1),
		Example: `  akt console screen deploy.yaml
  akt console screen --cpu 2 --memory 4Gi --gpu 1 --gpu-model a100`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			req, err := screeningRequestFromCmd(cmd, args)
			if err != nil {
				return err
			}

			providers, err := cl.ScreenBids(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("screen providers: %w", err)
			}

			if len(providers) == 0 {
				return printConsoleText(cmd, "No providers matched the SDL's resource requirements.\n")
			}

			return printJSON(cmd, providers)
		},
	}

	cmd.Flags().String("cpu", "1", "CPU quantity (for example 500m or 2)")
	cmd.Flags().String("memory", "512Mi", "Memory quantity")
	cmd.Flags().String("storage", "1Gi", "Ephemeral storage quantity")
	cmd.Flags().Uint32("gpu", 0, "GPU count")
	cmd.Flags().String("gpu-model", "", "Required GPU model attribute")
	cmd.Flags().Int("count", 1, "Number of identical resource units")
	cmd.Flags().StringArray("attribute", nil, "Required provider attribute key=value (repeatable)")
	cmd.Flags().StringSlice("signed-by", nil, "Accept providers audited by any listed address")
	cmd.Flags().Int64("reclamation-window", 0, "Minimum provider reclamation window in seconds")

	return cmd
}

func screeningRequestFromCmd(cmd *cobra.Command, args []string) (*console.BidScreeningRequest, error) {
	resourceFlags := []string{"cpu", "memory", "storage", "gpu", "gpu-model", "count"}
	hasResourceFlag := false
	for _, name := range resourceFlags {
		hasResourceFlag = hasResourceFlag || cmd.Flags().Changed(name)
	}

	var raw json.RawMessage
	var err error
	if len(args) == 1 {
		raw, err = screeningResourcesFromSDL(args[0])
		if err != nil {
			return nil, err
		}
	} else if !hasResourceFlag {
		return nil, fmt.Errorf("provide an SDL file or at least one resource flag (--cpu, --memory, --storage, --gpu, --gpu-model, --count)")
	} else {
		raw = json.RawMessage(`[{
			"resource":{"id":1,"cpu":{"units":{"val":"1000"}},"memory":{"quantity":{"val":"536870912"}},"gpu":{"units":{"val":"0"}},"storage":[{"name":"default","quantity":{"val":"1073741824"}}],"endpoints":[]},
			"count":1,"price":{"denom":"uact","amount":"0"}
			}]`)
	}

	return screeningRequestFromResources(cmd, args, raw)
}

func screeningRequestFromResources(cmd *cobra.Command, args []string, raw json.RawMessage) (*console.BidScreeningRequest, error) {
	var units []map[string]any
	if err := json.Unmarshal(raw, &units); err != nil {
		return nil, fmt.Errorf("decode screening resources: %w", err)
	}
	for _, unit := range units {
		resourceValue, ok := unit["resource"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("screening resource omitted its resource object")
		}
		if cmd.Flags().Changed("cpu") || len(args) == 0 {
			value, _ := cmd.Flags().GetString("cpu")
			parsed, err := screeningQuantity(value, true)
			if err != nil {
				return nil, fmt.Errorf("--cpu: %w", err)
			}
			resourceValue["cpu"] = map[string]any{"units": map[string]any{"val": parsed}}
		}
		if cmd.Flags().Changed("memory") || len(args) == 0 {
			value, _ := cmd.Flags().GetString("memory")
			parsed, err := screeningQuantity(value, false)
			if err != nil {
				return nil, fmt.Errorf("--memory: %w", err)
			}
			resourceValue["memory"] = map[string]any{"quantity": map[string]any{"val": parsed}}
		}
		if cmd.Flags().Changed("storage") || len(args) == 0 {
			value, _ := cmd.Flags().GetString("storage")
			parsed, err := screeningQuantity(value, false)
			if err != nil {
				return nil, fmt.Errorf("--storage: %w", err)
			}
			resourceValue["storage"] = []any{map[string]any{"name": "default", "quantity": map[string]any{"val": parsed}}}
		}
		if cmd.Flags().Changed("gpu") || cmd.Flags().Changed("gpu-model") || len(args) == 0 {
			gpu, _ := cmd.Flags().GetUint32("gpu")
			model, _ := cmd.Flags().GetString("gpu-model")
			if model != "" && gpu == 0 {
				gpu = 1
			}
			gpuValue := map[string]any{"units": map[string]any{"val": strconv.FormatUint(uint64(gpu), 10)}}
			if model != "" {
				gpuValue["attributes"] = []any{map[string]any{"key": "vendor/nvidia/model", "value": model}}
			}
			resourceValue["gpu"] = gpuValue
		}
		if cmd.Flags().Changed("count") || len(args) == 0 {
			count, _ := cmd.Flags().GetInt("count")
			if count < 1 {
				return nil, fmt.Errorf("--count must be greater than zero")
			}
			unit["count"] = count
		}
	}
	// The tree above contains only JSON maps, slices, strings, and integers.
	// Those values cannot fail JSON encoding after the successful decode.
	raw, _ = json.Marshal(units)

	attributes, _ := cmd.Flags().GetStringArray("attribute")
	requirements := console.BidScreeningRequirements{}
	for _, item := range attributes {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("--attribute must use key=value, got %q", item)
		}
		requirements.Attributes = append(requirements.Attributes, console.Attribute{Key: strings.TrimSpace(key), Value: strings.TrimSpace(value)})
	}
	requirements.SignedBy.AnyOf, _ = cmd.Flags().GetStringSlice("signed-by")

	reclamation, _ := cmd.Flags().GetInt64("reclamation-window")
	if reclamation < 0 {
		return nil, fmt.Errorf("--reclamation-window must be non-negative")
	}
	var reclamationRaw json.RawMessage
	if cmd.Flags().Changed("reclamation-window") {
		reclamationRaw = json.RawMessage(strconv.FormatInt(reclamation, 10))
	}

	return &console.BidScreeningRequest{
		Requirements:      requirements,
		Resources:         raw,
		Timezone:          screeningTimezone(),
		ReclamationWindow: reclamationRaw,
	}, nil
}

func screeningQuantity(value string, cpu bool) (string, error) {
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		return "", err
	}
	if quantity.Sign() <= 0 {
		return "", fmt.Errorf("quantity must be greater than zero")
	}
	if cpu {
		return strconv.FormatInt(quantity.MilliValue(), 10), nil
	}

	return strconv.FormatInt(quantity.Value(), 10), nil
}

// screeningTimezone returns the local IANA zone name for bid screening. The
// Console API rejects UTC-family zones, so UTC/Local (and abbreviations that
// are not real zone names) fall back to America/Chicago, mirroring the
// Console reference CLI.
func screeningTimezone() string {
	name := time.Local.String()
	if name == "" || name == "Local" ||
		strings.HasPrefix(name, "UTC") || strings.HasPrefix(name, "Etc/") ||
		!strings.Contains(name, "/") {
		return "America/Chicago"
	}

	return name
}

// screeningResourcesFromSDL derives the /v1/bid-screening resources array from
// an SDL file's deployment groups.
//
// The wire shape is built by hand instead of marshaling the chain structs:
// those serialize memory/storage quantities under "size" and prices as decimal
// strings, both of which the bid-screening schema rejects (it wants "quantity"
// objects and integer-string amounts). Deliberate simplification: endpoint
// details are dropped (sent as an empty list) — the screening endpoint matches
// on cpu/memory/gpu/storage quantities and attributes, which are all carried
// over.
func screeningResourcesFromSDL(path string) (json.RawMessage, error) {
	doc, err := sdl.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read SDL file: %w", err)
	}

	groups, err := doc.DeploymentGroups()
	if err != nil {
		return nil, fmt.Errorf("derive deployment groups from SDL: %w", err)
	}

	type val struct {
		Val string `json:"val"`
	}
	type units struct {
		Units      val                 `json:"units"`
		Attributes []console.Attribute `json:"attributes,omitempty"`
	}
	type quantity struct {
		Quantity   val                 `json:"quantity"`
		Attributes []console.Attribute `json:"attributes,omitempty"`
	}
	type volume struct {
		Name       string              `json:"name"`
		Quantity   val                 `json:"quantity"`
		Attributes []console.Attribute `json:"attributes,omitempty"`
	}
	type resource struct {
		ID        uint32   `json:"id"`
		CPU       units    `json:"cpu"`
		Memory    quantity `json:"memory"`
		GPU       units    `json:"gpu"`
		Storage   []volume `json:"storage"`
		Endpoints []any    `json:"endpoints"`
	}
	type price struct {
		Denom  string `json:"denom"`
		Amount string `json:"amount"`
	}
	type unit struct {
		Resource resource `json:"resource"`
		Count    uint32   `json:"count"`
		Price    price    `json:"price"`
	}

	var out []unit
	for _, g := range groups {
		for _, ru := range g.Resources {
			r := resource{
				ID:        ru.ID,
				CPU:       units{Units: val{"0"}},
				Memory:    quantity{Quantity: val{"0"}},
				GPU:       units{Units: val{"0"}},
				Storage:   []volume{},
				Endpoints: []any{},
			}

			if ru.CPU != nil {
				r.CPU = units{val{ru.CPU.Units.Val.String()}, screeningAttrs(ru.CPU.Attributes)}
			}
			if ru.Memory != nil {
				r.Memory = quantity{val{ru.Memory.Quantity.Val.String()}, screeningAttrs(ru.Memory.Attributes)}
			}
			if ru.GPU != nil {
				r.GPU = units{val{ru.GPU.Units.Val.String()}, screeningAttrs(ru.GPU.Attributes)}
			}
			for _, v := range ru.Storage {
				r.Storage = append(r.Storage, volume{v.Name, val{v.Quantity.Val.String()}, screeningAttrs(v.Attributes)})
			}

			out = append(out, unit{
				Resource: r,
				Count:    ru.Count,
				Price: price{
					Denom: ru.Price.Denom,
					// The schema wants an integer string; SDL prices are
					// decimal on chain, so round up.
					Amount: ru.Price.Amount.Ceil().TruncateInt().String(),
				},
			})
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("SDL %s produced no resource groups", path)
	}

	return json.Marshal(out)
}

// screeningAttrs converts chain attributes to the Console wire type.
func screeningAttrs(attrs attrv1.Attributes) []console.Attribute {
	if len(attrs) == 0 {
		return nil
	}

	out := make([]console.Attribute, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, console.Attribute{Key: a.Key, Value: a.Value})
	}

	return out
}

// providerLogMsg is the shape the log stream yields, narrowed to the fields
// the CLI prints so the filtering helpers can be tested without a provider.
type providerLogMsg struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func printLeaseEvent(cmd *cobra.Command, event rest.LeaseEvent) error {
	pretty := fmt.Sprintf("%s [%s/%s] %s: %s",
		event.Type, event.Object.Kind, event.Object.Name, event.Reason, event.Note)

	return output.PrintStreamRecord(cmd, event, pretty)
}

func printEmptyEvents(cmd *cobra.Command, count int, follow bool) error {
	if count == 0 && !follow && output.FormatFromCmd(cmd) == output.FormatTable {
		return printConsoleText(cmd, "No recent events\n")
	}

	return nil
}
