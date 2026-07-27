package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/bootstrap"
	"pkg.akt.dev/akt/internal/capability"
	chaincli "pkg.akt.dev/akt/internal/cli/chain"

	cliconsole "pkg.akt.dev/akt/internal/cli/console"
	clicontext "pkg.akt.dev/akt/internal/cli/context"
	cliprovider "pkg.akt.dev/akt/internal/cli/provider"
	clisdl "pkg.akt.dev/akt/internal/cli/sdl"
	clistore "pkg.akt.dev/akt/internal/cli/store"
	cliworkflow "pkg.akt.dev/akt/internal/cli/workflow"
	aktclient "pkg.akt.dev/akt/internal/client"
	"pkg.akt.dev/akt/internal/cliutil"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/glyphs"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
	akttui "pkg.akt.dev/akt/internal/tui"

	"pkg.akt.dev/akt/internal/output/pretty"

	arpcclient "pkg.akt.dev/go/node/client"
)

// BuildInfo holds build-time metadata injected via ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRootCmd creates the root cobra command for akt.
func NewRootCmd(bi BuildInfo) *cobra.Command {
	cobra.EnableTraverseRunHooks = true

	v := viper.New()
	v.SetEnvPrefix("AKT")
	v.AutomaticEnv()
	v.SetDefault("defaults.interactive", true)

	encCfg := aktcodec.MakeEncodingConfig()

	// Register tx pretty formatters so that --output pretty renders
	// human-readable transaction results (SPEC §10.11).
	pretty.RegisterAllTxFormatters()

	// The manager, keyring manager, and config root are lazily initialized in PersistentPreRunE.
	var mgr *aktctx.Manager
	var krMgr *aktkeyring.Manager
	var resolvedCfgRoot string

	mgrFn := func() *aktctx.Manager { return mgr }

	// getKeyring returns the Cosmos SDK keyring for the current context.
	getKeyring := func() (sdkkeyring.Keyring, error) {
		if krMgr == nil {
			return nil, fmt.Errorf("keyring manager not initialized")
		}

		ctxName := v.GetString("context")
		if ctxName == "" && mgr != nil {
			ctxName = mgr.CurrentContext()
		}

		if ctxName == "" {
			return krMgr.Get("default")
		}

		ctx := mgr.GetContext(ctxName)
		if ctx == nil {
			return nil, fmt.Errorf("context %q not found", ctxName)
		}

		krName := ctx.Keyring.Name
		if krName == "" {
			krName = "default"
		}

		return krMgr.Get(krName)
	}

	root := &cobra.Command{
		Use:   "akt",
		Short: "Akash Network CLI and TUI",
		Long:  "akt is the unified command-line interface and terminal user interface for the Akash Network.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 1. Seed the SDK client.Context with encoding config so that
			//    downstream PersistentPreRunE hooks (tx/query) always find
			//    a non-nil ClientContextKey and CmdContext. No server config
			//    or CometBFT dirs are created — akt is a CLI, not a node.
			ctx := cmd.Context()

			initCctx := sdkclient.Context{}.
				WithCodec(encCfg.Codec).
				WithInterfaceRegistry(encCfg.InterfaceRegistry).
				WithTxConfig(encCfg.TxConfig).
				WithLegacyAmino(encCfg.Amino).
				WithInput(os.Stdin).
				WithAccountRetriever(authtypes.AccountRetriever{}).
				WithHomeDir(aktctx.DefaultHome)

			ctx = context.WithValue(ctx, chaincli.ContextTypeAddressCodec, encCfg.SigningOptions.AddressCodec)
			ctx = context.WithValue(ctx, chaincli.ContextTypeValidatorCodec, encCfg.SigningOptions.ValidatorAddressCodec)

			cmd.SetContext(ctx)

			initCctx = initCctx.WithCmdContext(cmd.Context())

			if err := chaincli.SetCmdClientContextHandler(initCctx, cmd); err != nil {
				return err
			}

			// 2. Bind persistent flags to Viper so the resolution chain
			//    (flag > env > config > default) works automatically.
			_ = v.BindPFlag("home", cmd.Flags().Lookup("home"))
			_ = v.BindPFlag("context", cmd.Flags().Lookup("context"))
			_ = v.BindPFlag("output", cmd.Flags().Lookup("output"))
			_ = v.BindPFlag("interactive", cmd.Flags().Lookup("interactive"))
			_ = v.BindPFlag("verbose", cmd.Flags().Lookup("verbose"))
			_ = v.BindPFlag("quiet", cmd.Flags().Lookup("quiet"))

			// Verbosity validation: -q and -v are mutually exclusive.
			if v.GetBool("quiet") && v.GetInt("verbose") > 0 {
				return fmt.Errorf("--quiet and --verbose are mutually exclusive")
			}

			cfgRoot, err := aktctx.ConfigHome(v.GetString("home"))
			if err != nil {
				return err
			}
			resolvedCfgRoot = cfgRoot

			// First-run bootstrap: if no config file exists, offer to
			// fetch networks from github.com/akash-network/net. Help
			// invocations never bootstrap — help must work on a machine
			// with nothing configured.
			cfgPath := aktctx.ConfigPath(cfgRoot)
			if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) && !isHelpInvocation(cmd, os.Args[1:]) {
				// Initialize glyphs before bootstrap (config not yet
				// available — uses flag/env/auto-detect only).
				initGlyphs(v)

				if err := bootstrap.Run(cfgRoot); err != nil {
					return err
				}
			}

			// Load the config file into Viper so settings like
			// defaults.interactive are available.
			v.SetConfigName("config")
			v.SetConfigType("yaml")
			v.AddConfigPath(cfgRoot)
			_ = v.ReadInConfig() // ok if file doesn't exist

			// Initialize glyphs (no-op if already done during bootstrap).
			// Now the full resolution chain is available: flag > env > config > auto.
			initGlyphs(v)

			mgr, err = aktctx.NewManager(cfgRoot)
			if err != nil {
				return err
			}

			// Initialize the keyring manager with all keyring configs.
			cfg := mgr.Config()
			krMgr = aktkeyring.NewManager(cfgRoot, cfg.Keyrings, encCfg.Codec)

			// 3. If an akt context is active, enrich the SDK client.Context
			//    with context-specific values (chain-id, RPC, keyring, etc.).
			resolved, err := aktclient.MustResolveAndInit(cmd, mgr, krMgr, encCfg, v.GetString("context"))
			if err != nil {
				return err
			}

			// 4. If no context was resolved, only allow commands that do not
			//    require a configured context to proceed. Help requests are
			//    always allowed: SDK group commands disable flag parsing, so
			//    cobra cannot short-circuit their --help before these hooks
			//    run, and help must never require a context.
			if !resolved && requiresContext(cmd) && !isHelpInvocation(cmd, os.Args[1:]) {
				return noContextError(mgr)
			}

			// 4b. Capability gating: derive the feature set from the active
			//     context (chain RPC present? Console key present?) and gate
			//     the command surface accordingly — commands whose transport
			//     is not configured are dimmed or hidden in help (mode from
			//     defaults.command-gating: dim | hide | off) and fail fast
			//     with an explanation instead of erroring mid-transport.
			if resolved {
				if rc, rcErr := mgr.Resolve(activeContextName(mgr, v.GetString("context"))); rcErr == nil {
					mode := capability.ParseMode(gatingMode(v, mgr))

					// The feature set describes the configuration; explicit
					// per-invocation overrides (--node, --console-api-key, a
					// positional monitor endpoint) grant their capability so
					// gating never rejects a command that carries its own
					// connection details.
					set := invocationCapabilities(capability.Resolve(rc), cmd, os.Args[1:], args)

					applyCapabilityGating(cmd.Root(), set, mode)

					if mode != capability.ModeOff && !isHelpInvocation(cmd, os.Args[1:]) {
						if err := requirementError(cmd, set); err != nil {
							return err
						}
					}
				}
			}

			// 5. Open the action log for the current context (if any).
			if current := mgr.CurrentContext(); current != "" {
				logPath := aktctx.ActionLogPath(cfgRoot, current)
				logger, logErr := actionlog.Open(logPath)
				if logErr == nil {
					ctx := cliutil.WithActionLog(cmd.Context(), logger)
					cmd.SetContext(ctx)
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// The TUI shell is DISABLED pending UX feedback (2026-07): bare
			// akt prints help instead of launching the dashboard. The code
			// path is kept compiled behind AKT_EXPERIMENTAL_TUI=1 so it can
			// be exercised while feedback is collected; akt monitor (the
			// real-time monitoring hub) is unaffected. Re-enable by removing
			// this gate once the TUI resource views are production-ready.
			if os.Getenv("AKT_EXPERIMENTAL_TUI") == "1" {
				if err := checkInteractive(v); err != nil {
					return err
				}

				cctx := sdkclient.GetClientContextFromCmd(cmd)
				cfgRoot, _ := aktctx.ConfigHome(v.GetString("home"))

				return akttui.Run(akttui.Config{
					Viper:        v,
					RPCEndpoint:  cctx.NodeURI,
					RESTEndpoint: "", // resolved by rpc.NewClient default when empty
					CacheDir:     filepath.Join(cfgRoot, "cache"),
					Insecure:     true,
				})
			}

			if interactive, _ := cmd.Flags().GetBool("interactive"); interactive {
				return fmt.Errorf("the TUI is currently disabled while UX feedback is collected; use the CLI commands or akt monitor")
			}

			return cmd.Help()
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			// Close the action logger opened in PersistentPreRunE (SPEC §5.6).
			if l := cliutil.ActionLogFromContext(cmd.Context()); l != nil {
				return l.Close()
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global persistent flags. Values are read from Viper at point of use,
	// not captured into Go variables.
	root.PersistentFlags().String("home", "", "Home directory for config, contexts, and keyrings (default: $AKT_HOME or ~/.config/akt)")
	root.PersistentFlags().String("context", "", "Active context name (overrides current-context in config)")
	root.PersistentFlags().StringP("output", "o", "pretty", "Output format: pretty, json, yaml")
	root.PersistentFlags().BoolP("interactive", "i", false, "Force interactive (TUI) mode even if disabled in config")
	root.PersistentFlags().CountP("verbose", "v", "Increase output verbosity (-v verbose, -vv debug)")
	root.PersistentFlags().BoolP("quiet", "q", false, "Suppress all output except errors")

	// Register shell completion for the global --context flag.
	_ = root.RegisterFlagCompletionFunc("context", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		m := mgrFn()
		if m == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		ctxs := m.ListContexts()
		names := make([]string, 0, len(ctxs))
		for _, c := range ctxs {
			names = append(names, c.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	// Context management (includes network and keys subcommands).
	root.AddCommand(clicontext.Commands(mgrFn, getKeyring))

	// TX and Query command trees from local chain CLI copy.
	root.AddCommand(chaincli.TxCmd())
	root.AddCommand(chaincli.QueryCmd())

	root.AddCommand(monitorCmd(v))
	root.AddCommand(mcpCmd())
	root.AddCommand(cliprovider.Commands())
	root.AddCommand(clisdl.Commands())
	homeFn := func() string { return resolvedCfgRoot }
	// ctxNameFn resolves the active context name for the command groups that
	// need it. The global --context override wins over current-context so
	// that `akt --context staging ...` stores credentials under and bills
	// the context the user named.
	ctxNameFn := func() string {
		if name := v.GetString("context"); name != "" {
			return name
		}
		if mgr != nil {
			return mgr.CurrentContext()
		}
		return ""
	}

	root.AddCommand(cliconsole.Commands(mgrFn))
	root.AddCommand(clistore.Commands(homeFn, ctxNameFn))

	// Workflow commands are discovered dynamically from workflow definitions.
	// Only workflows that exist (built-in or user-defined YAML) produce commands.
	for _, wfCmd := range cliworkflow.CommandsWithManager(homeFn, ctxNameFn, mgrFn) {
		root.AddCommand(wfCmd)
	}
	root.AddCommand(versionCmd(bi))
	root.AddCommand(completionCmd())

	// Capability gating must also shape help output when cobra
	// short-circuits --help before the persistent hooks run (parsed help
	// flags never reach PersistentPreRunE). Resolution here is best-effort:
	// any failure falls back to ungated help rather than blocking it.
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		// Help can run before PersistentPreRunE, so mirror the flags it
		// would have bound. Both --home and --context matter: help must
		// describe the context the user actually named.
		for _, name := range []string{"home", "context"} {
			if f := root.PersistentFlags().Lookup(name); f != nil && f.Changed {
				v.Set(name, f.Value.String())
			}
		}

		if cfgRoot, err := aktctx.ConfigHome(v.GetString("home")); err == nil {
			if m, mErr := aktctx.NewManager(cfgRoot); mErr == nil {
				if ctxName := activeContextName(m, v.GetString("context")); ctxName != "" {
					if rc, rcErr := m.Resolve(ctxName); rcErr == nil {
						// Same mode resolution as enforcement (flag > env >
						// config) so help and execution never disagree.
						mode := capability.ParseMode(gatingMode(v, m))
						set := invocationCapabilities(capability.Resolve(rc), c, os.Args[1:], nil)
						applyCapabilityGating(root, set, mode)
					}
				}
			}
		}

		defaultHelp(c, args)
	})

	return root
}

// Execute seeds the SDK client and server context keys on the command context
// (so that downstream PersistentPreRunE hooks find a non-nil ClientContextKey),
// then executes the root command.
func Execute(root *cobra.Command) error {
	ctx := context.Background()

	ctx = context.WithValue(ctx, sdkclient.ClientContextKey, &sdkclient.Context{})
	ctx = context.WithValue(ctx, sdkserver.ServerContextKey, sdkserver.NewDefaultContext())

	return root.ExecuteContext(ctx)
}

// requiresContext returns true if the command needs a fully resolved akt
// context (network, keyring, RPC) to operate. Commands that manage
// contexts themselves or display basic info are exempt.
func requiresContext(cmd *cobra.Command) bool {
	path := cmd.CommandPath() // e.g. "akt context create", "akt q bme status"

	// The root command itself (help), context management, version, and
	// monitor work without an active context.
	switch {
	case path == "akt":
		return false
	case strings.HasPrefix(path, "akt context"):
		return false
	case strings.HasPrefix(path, "akt version"):
		return false
	case strings.HasPrefix(path, "akt monitor"):
		return false
	case strings.HasPrefix(path, "akt completion"):
		return false
	// SDL authoring is entirely local: parsing, scaffolding, and linting
	// never touch a network or a credential.
	case strings.HasPrefix(path, "akt sdl"):
		return false
	// The console group resolves its own credential (flag > env > stored)
	// and works with no context at all when a key comes from the
	// environment; capability gating reports a missing key instead.
	case strings.HasPrefix(path, "akt console"):
		return false
	}

	return true
}

// helpRequested reports whether the invocation asks for help. Detection is
// argv-based because several clean-copied SDK group commands set
// DisableFlagParsing, which prevents cobra from seeing --help before the
// persistent hooks execute.
func helpRequested(args []string) bool {
	for _, a := range args {
		// Everything after the terminator is data for the command
		// (e.g. `provider lease-shell -- sh -h`), never a help request.
		if a == "--" {
			return false
		}

		// Only the flag forms count. A bare "help" is matched separately
		// via the resolved command: as a positional value (`tx deployment
		// close help`) it must NOT disable context or capability checks.
		if a == "--help" || a == "-h" || strings.HasPrefix(a, "--help=") {
			return true
		}
	}

	return false
}

// activeContextName resolves which context a run targets: the explicit
// override, else current-context, else the sole configured context (the
// same auto-selection the SDK client bootstrap performs). Returns "" when
// no single context can be determined.
func activeContextName(mgr *aktctx.Manager, override string) string {
	if mgr == nil {
		return override
	}

	return mgr.ActiveContext(override)
}

// gatingMode resolves the command-gating mode with the standard precedence
// (flag/env via viper, then config file).
func gatingMode(v *viper.Viper, mgr *aktctx.Manager) string {
	if mode := v.GetString("defaults.command-gating"); mode != "" {
		return mode
	}

	if mgr != nil {
		return mgr.Config().Defaults.CommandGating
	}

	return ""
}

// isHelpInvocation reports whether this run only prints help — either via a
// help flag or via cobra's built-in `help` command — in which case context
// and capability requirements are not enforced.
func isHelpInvocation(cmd *cobra.Command, args []string) bool {
	if cmd != nil && cmd.Name() == "help" {
		return true
	}

	return helpRequested(args)
}

// checkInteractive returns an error if interactive mode is disabled in config
// and the --interactive (-i) flag was not passed.
func checkInteractive(v *viper.Viper) error {
	// The -i flag overrides config.
	if v.GetBool("interactive") {
		return nil
	}
	// Config allows interactive mode.
	if v.GetBool("defaults.interactive") {
		return nil
	}
	return fmt.Errorf("interactive mode is disabled in config (defaults.interactive: false); use --interactive (-i) to override")
}

func noContextError(mgr *aktctx.Manager) error {
	contexts := mgr.ListContexts()
	if len(contexts) == 0 {
		return fmt.Errorf("no contexts configured; create one with \"akt context create\" and activate with \"akt context use\"")
	}

	names := make([]string, len(contexts))
	for i, c := range contexts {
		names[i] = c.Name
	}

	return fmt.Errorf("no active context; available contexts: %s\nactivate one with \"akt context use <name>\"", strings.Join(names, ", "))
}

// monitorRunE is the shared RunE for the monitor command and all its
// subcommands.  The dashboard parameter selects which hub tab is shown
// first (empty string = default = network).
func monitorRunE(v *viper.Viper, dashboard string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := checkInteractive(v); err != nil {
			return err
		}

		rpcEndpoint, _ := cmd.Flags().GetString("rpc")
		restEndpoint, _ := cmd.Flags().GetString("rest")
		cleanCache, _ := cmd.Flags().GetBool("clean-cache")
		insecure, _ := cmd.Flags().GetBool("insecure")

		// Positional argument overrides --rpc flag.
		if len(args) > 0 {
			rpcEndpoint = args[0]
		}

		// Resolve endpoints from active akt context when not
		// explicitly provided via flags.
		cctx := sdkclient.GetClientContextFromCmd(cmd)
		if rpcEndpoint == "" && cctx.NodeURI != "" {
			rpcEndpoint = cctx.NodeURI
		}

		if rpcEndpoint == "" {
			return fmt.Errorf("no RPC endpoint; provide one via --rpc flag, positional argument, or configure an akt context")
		}

		// Ensure endpoints carry explicit ports (inferred from scheme
		// when omitted) so downstream CometBFT clients can connect.
		rpcEndpoint = arpcclient.NormalizeEndpoint(rpcEndpoint)
		restEndpoint = arpcclient.NormalizeEndpoint(restEndpoint)

		// Resolve store path for the bbolt cache.
		cfgRoot, err := aktctx.ConfigHome("")
		if err != nil {
			return err
		}

		cacheDir := filepath.Join(cfgRoot, "cache")

		if cleanCache {
			dbPath := filepath.Join(cacheDir, "monitor.db")
			_ = os.Remove(dbPath)
			fmt.Println("Cache cleared")
		}

		return akttui.RunMonitor(akttui.Config{
			Viper:            v,
			RPCEndpoint:      rpcEndpoint,
			RESTEndpoint:     restEndpoint,
			CacheDir:         cacheDir,
			Insecure:         insecure,
			Standalone:       true,
			InitialDashboard: dashboard,
		})
	}
}

// addMonitorFlags adds the shared flags used by monitor and all its
// subcommands.
func addMonitorFlags(cmd *cobra.Command) {
	cmd.Flags().String("rpc", "", "RPC endpoint URL (resolved from context if not set)")
	cmd.Flags().String("rest", "", "REST endpoint URL (resolved from context if not set)")
	cmd.Flags().Bool("clean-cache", false, "Delete monitor cache and start fresh")
	cmd.Flags().Bool("insecure", true, "Skip TLS certificate verification for provider queries")
}

func monitorCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor [rpc-endpoint]",
		Short: "Real-time monitoring hub",
		// Capability gating: monitoring needs a chain RPC endpoint.
		Annotations: map[string]string{capability.AnnotationKey: string(capability.ChainQuery)},
		Long: `Interactive TUI for monitoring Akash Network state in real time.

The monitor hub provides three dashboards navigable via Tab/Shift-Tab:

  Network     Consensus state, validator voting, governance parameters
  Provider    Provider fleet health, version distribution, resources
  Oracle/BME  Oracle prices, price health, vault state, mint status

By default the Network dashboard is shown. Use a subcommand to launch
directly into a specific dashboard:

  akt monitor network   (consensus, validators, governance)
  akt monitor provider  (provider fleet monitoring)
  akt monitor oracle    (oracle prices + BME state)
  akt monitor bme       (alias for oracle)

Connects to the RPC endpoint via WebSocket for real-time vote streaming.
The RPC endpoint must support WebSocket connections.

If --rpc is not specified, the endpoint is resolved from the active akt
context. A positional argument overrides the --rpc flag.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  # Launch the monitor hub (defaults to Network dashboard)
  akt monitor

  # Connect to a specific RPC endpoint
  akt monitor https://rpc.akashnet.net:443

  # Launch directly into the Provider dashboard
  akt monitor provider`,
		RunE: monitorRunE(v, ""),
	}

	addMonitorFlags(cmd)

	cmd.AddCommand(monitorNetworkCmd(v))
	cmd.AddCommand(monitorProviderCmd(v))
	cmd.AddCommand(monitorOracleCmd(v))
	cmd.AddCommand(monitorBMECmd(v))

	return cmd
}

func monitorNetworkCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network [rpc-endpoint]",
		Short: "Network monitoring (consensus, validators, governance)",
		Long: `Launch the monitor directly into the Network dashboard.

Displays real-time consensus state (height, round, step), validator
voting progress, and governance parameters. Sub-tabs:

  1  Overview    Consensus state, vote progress bars, vote grid
  2  Validators  Scrollable validator list with signing history
  3  Governance  Module-by-module parameter browser`,
		Args:    cobra.MaximumNArgs(1),
		Example: `  akt monitor network https://rpc.akashnet.net:443`,
		RunE:    monitorRunE(v, "network"),
	}

	addMonitorFlags(cmd)

	return cmd
}

func monitorProviderCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider [rpc-endpoint]",
		Short: "Provider fleet monitoring",
		Long: `Launch the monitor directly into the Provider dashboard.

Displays real-time provider fleet health: version distribution with dot
visualization, provider health scanning with priority-based scheduling,
and per-provider detail with node-level CPU/memory/GPU resources.`,
		Args:    cobra.MaximumNArgs(1),
		Example: `  akt monitor provider`,
		RunE:    monitorRunE(v, "provider"),
	}

	addMonitorFlags(cmd)

	return cmd
}

func monitorOracleCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oracle [rpc-endpoint]",
		Short: "Oracle and BME monitoring",
		Long: `Launch the monitor directly into the Oracle/BME dashboard.

Displays oracle price data (aggregated prices, TWAP, health) and
BME state (mint status, vault balances, ledger entries). This is
the same dashboard as "akt monitor bme".`,
		Args:    cobra.MaximumNArgs(1),
		Example: `  akt monitor oracle`,
		RunE:    monitorRunE(v, "oracle-bme"),
	}

	addMonitorFlags(cmd)

	return cmd
}

func monitorBMECmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bme [rpc-endpoint]",
		Short: "BME and Oracle monitoring",
		Long: `Launch the monitor directly into the Oracle/BME dashboard.

Displays BME state (mint status, vault balances, ledger entries)
and oracle price data (aggregated prices, TWAP, health). This is
the same dashboard as "akt monitor oracle".`,
		Args:    cobra.MaximumNArgs(1),
		Example: `  akt monitor bme`,
		RunE:    monitorRunE(v, "oracle-bme"),
	}

	addMonitorFlags(cmd)

	return cmd
}

// initGlyphs resolves the glyph mode from env/config and initialises the
// glyphs package. Safe to call multiple times — [glyphs.Init] uses sync.Once.
func initGlyphs(v *viper.Viper) {
	modeStr := v.GetString("defaults.glyph-mode")

	mode, err := glyphs.ParseMode(modeStr)
	if err != nil {
		mode = glyphs.ModeAuto
	}

	glyphs.Init(mode)
}

func completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for akt.

These scripts enable tab-completion for commands, flags, and arguments
in your shell. Follow the instructions for your shell below.`,
		Example: `  # Bash
  akt completion bash > /etc/bash_completion.d/akt

  # Zsh
  akt completion zsh > "${fpath[1]}/_akt"

  # Fish
  akt completion fish > ~/.config/fish/completions/akt.fish

  # PowerShell
  akt completion powershell > akt.ps1`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenBashCompletionV2(os.Stdout, true)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenZshCompletion(os.Stdout)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenFishCompletion(os.Stdout, true)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "powershell",
		Short: "Generate PowerShell completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenPowerShellCompletion(os.Stdout)
		},
	})

	return cmd
}

func versionCmd(bi BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Example: `  # Short form
  akt version

  # Full build info (Go version, platform, build tags)
  akt version --long`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()

			if long, _ := cmd.Flags().GetBool("long"); long {
				info := []struct{ k, v string }{
					{"version", bi.Version},
					{"commit", bi.Commit},
					{"built", bi.Date},
					{"go", runtime.Version()},
					{"platform", runtime.GOOS + "/" + runtime.GOARCH},
				}

				if bi, ok := debug.ReadBuildInfo(); ok {
					for _, setting := range bi.Settings {
						if setting.Key == "-tags" && setting.Value != "" {
							info = append(info, struct{ k, v string }{"build tags", setting.Value})
						}
					}
				}

				for _, i := range info {
					if _, err := fmt.Fprintf(out, "%-11s %s\n", i.k+":", i.v); err != nil {
						return err
					}
				}

				return nil
			}

			_, err := fmt.Fprintf(out, "akt %s (commit: %s, built: %s)\n", bi.Version, bi.Commit, bi.Date)
			return err
		},
	}

	// Build-time detail for bug reports (TASKS T035).
	cmd.Flags().Bool("long", false, "Print full build information")

	return cmd
}
