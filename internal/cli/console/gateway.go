package console

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"golang.org/x/term"

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

	gw, err := rest.NewClient(ctx, addr,
		rest.WithProviderURL(prov.HostURI),
		rest.WithAuthToken(token),
	)
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

			follow, _ := cmd.Flags().GetBool("follow")
			tail, _ := cmd.Flags().GetInt64("tail")
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

	cmd.Flags().BoolP("follow", "f", false, "Follow log output")
	cmd.Flags().Int64("tail", -1, "Number of lines to show from the end of the logs")
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

			follow, _ := cmd.Flags().GetBool("follow")

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

			events, err := gw.LeaseEvents(ctx, lid, "", follow)
			if err != nil {
				return aktprovider.GatewayError("stream lease events", err)
			}

			return aktprovider.ConsumeStream(ctx, "event", events.Stream, events.OnClose, follow,
				func(event rest.LeaseEvent) error {
					return printLeaseEvent(cmd, event)
				})
		},
	}

	cmd.Flags().BoolP("follow", "f", false, "Follow event output")

	return cmd
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

			watch, _ := cmd.Flags().GetBool("watch")
			interval, _ := cmd.Flags().GetDuration("interval")
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

	cmd.Flags().Bool("watch", false, "Keep polling and printing status snapshots until interrupted")
	cmd.Flags().Duration("interval", 5*time.Second, "Polling interval used with --watch")

	return cmd
}

func shellCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "shell <dseq> <service> [-- command...]",
		Short: "Open a shell or run a command in a lease container",
		Long: "Open an interactive shell (default command /bin/sh) or run an explicit command in a " +
			"container of a Console-managed deployment, connecting to the provider gateway with a " +
			"short-lived Console-minted JWT — no wallet involved. exec is the same operation as " +
			"shell with an explicit command: `akt console shell <dseq> <svc> -- <cmd>`. A TTY is " +
			"allocated automatically when stdin is a terminal.",
		Args: cobra.MinimumNArgs(2),
		Example: `  # Interactive shell (defaults to /bin/sh)
  akt console shell 12345 web

  # Run a single command (a.k.a. exec)
  akt console shell 12345 web -- ls -la`,
		RunE: func(cmd *cobra.Command, args []string) error {
			shellCtx, cancelShell := context.WithCancel(cmd.Context())
			defer cancelShell()

			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			dseq, service := args[0], args[1]

			command := args[2:]
			if len(command) == 0 {
				command = []string{"/bin/sh"}
			}

			gw, lid, err := gatewayForDeployment(cmd, cl, dseq, gatewayJWTTTLStream, []string{"shell", "status"})
			if err != nil {
				return err
			}
			tty := term.IsTerminal(int(os.Stdin.Fd()))

			err = aktprovider.RunLeaseShell(shellCtx, gw, lid, service, 0, command,
				aktprovider.HoldEOF(shellCtx, os.Stdin),
				cmd.OutOrStdout(), cmd.ErrOrStderr(), tty, nil)
			return err
		},
	}
}

// --- bid screening ------------------------------------------------------------

func screenCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "screen <sdl-file>",
		Short: "List providers able to run an SDL (bid screening)",
		Long: "Ask the Console API which providers can satisfy an SDL's resource requirements " +
			"before creating a deployment. Resources are derived from the SDL client-side; the " +
			"endpoint is public, so no API key is needed.",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console screen deploy.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, false)
			if err != nil {
				return err
			}

			resources, err := screeningResourcesFromSDL(args[0])
			if err != nil {
				return err
			}

			providers, err := cl.ScreenBids(cmd.Context(), &console.BidScreeningRequest{
				Resources: resources,
				Timezone:  screeningTimezone(),
			})
			if err != nil {
				return fmt.Errorf("screen providers: %w", err)
			}

			if len(providers) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No providers matched the SDL's resource requirements.")
				return nil
			}

			return printJSON(cmd, providers)
		},
	}
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
