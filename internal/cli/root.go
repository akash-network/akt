package cli

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	sdkversion "github.com/cosmos/cosmos-sdk/version"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/bootstrap"
	"pkg.akt.dev/akt/internal/capability"
	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"

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

	"pkg.akt.dev/akt/internal/output"
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

	// Copied SDK commands interpolate this value while their command trees are
	// built. Release ldflags set it, but library users and unit tests do not.
	if sdkversion.AppName == "" || sdkversion.AppName == "<appd>" {
		sdkversion.AppName = "akt"
	}

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

		ctxName := activeContextName(mgr, v.GetString("context"))

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
		Short: "Akash Network CLI",
		Long: `akt is the unified command-line interface for the Akash Network. Configure
networks, contexts, and keys; query chain state; sign and broadcast
transactions; deploy and operate workloads through local keys or Akash
Console; interact with provider gateways; and monitor network health.

Getting started:
  akt sdl init web > deploy.yaml     # generate a starter SDL
  akt deploy deploy.yaml             # deploy it and pick a bid
  akt context show                   # see the active configuration

Running akt for the first time in a terminal walks you through creating a
context: the network to talk to, the keyring that signs, and where akt keeps
its record of your deployments.

Two ways to pay and sign, chosen per context:
  keyring       you hold the key, akt signs and broadcasts, costs are in AKT
  console-api   Akash Console holds a managed wallet and signs for you, in USD

Deployments are identified by a dseq (deployment sequence number), printed when
the deployment is created.`,
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
			if fromFlag := cmd.Flags().Lookup(cflags.FlagFrom); fromFlag != nil {
				_ = v.BindPFlag(cflags.FlagFrom, fromFlag)
			}

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
			// with nothing configured — and neither do commands that work
			// entirely without configuration.
			cfgPath := aktctx.ConfigPath(cfgRoot)
			if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) &&
				!isHelpInvocation(cmd, os.Args[1:]) && requiresConfig(cmd) {
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
			resolved, err := aktclient.MustResolveAndInit(
				cmd,
				mgr,
				krMgr,
				encCfg,
				v.GetString("context"),
				v.GetString(cflags.FlagFrom),
			)
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
			if resolved {
				rc, resolveErr := mgr.Resolve(activeContextName(mgr, v.GetString("context")))
				if resolveErr != nil {
					return resolveErr
				}
				applyTransactionDefaults(cmd, rc)
				if err := applyProviderDefaults(cmd, rc); err != nil {
					return err
				}
			}
			if resolved && cmd.Flags().Lookup(cflags.FlagOffline) != nil {
				if err := chaincli.ValidateTxInvocation(cmd); err != nil {
					return err
				}
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

			// 5. Open the action log for the selected context (if any).
			if selected := activeContextName(mgr, v.GetString("context")); selected != "" {
				logPath := aktctx.ActionLogPath(cfgRoot, selected)
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
				runtime, err := resolveMonitorRuntime(
					v,
					cctx.NodeURI,
					false,
					"",
					func() string { return resolvedCfgRoot },
					mgrFn,
				)
				if err != nil {
					return err
				}

				return akttui.Run(akttui.Config{
					Viper:        v,
					RPCEndpoint:  runtime.rpcEndpoint,
					RESTEndpoint: runtime.restEndpoint,
					CacheDir:     runtime.cacheDir,
					Insecure:     false,
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
	root.PersistentFlags().VarP(output.NewFormatFlag("pretty"), "output", "o", "Output format: pretty, json, yaml")
	// Local, not persistent: launching the TUI is only meaningful at the root.
	// As a persistent flag it was advertised on all ~400 subcommands and
	// silently discarded by every one of them -- `akt -i` refused with an
	// explanation while `akt version -i` accepted it and did nothing.
	root.Flags().BoolP("interactive", "i", false, "Launch the TUI. Currently disabled while UX feedback is collected; set AKT_EXPERIMENTAL_TUI=1 to opt in")
	root.PersistentFlags().CountP("verbose", "v", "Increase output verbosity (-v verbose, -vv debug)")
	root.PersistentFlags().BoolP("quiet", "q", false, "Suppress informational output; keep results and errors")

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

	homeFn := func() string { return resolvedCfgRoot }
	root.AddCommand(monitorCmd(v, homeFn, mgrFn))
	root.AddCommand(mcpCmd(mgrFn))
	root.AddCommand(cliprovider.Commands())
	root.AddCommand(clisdl.Commands())
	// ctxNameFn resolves the active context name for the command groups that
	// need it. The global --context override wins over current-context so
	// that `akt --context staging ...` stores credentials under and bills
	// the context the user named.
	ctxNameFn := func() string {
		if mgr != nil {
			return activeContextName(mgr, v.GetString("context"))
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
	enforceGroupInputValidation(root)
	enforceOutputValidation(root)
	enforceTransactionModeValidation(root)
	root.InitDefaultHelpCmd()
	prepareCommandHelp(root)

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

// applyTransactionDefaults resolves transaction economics without marking
// inherited defaults as explicit flags. Fixed fees and gas prices are two ways
// to determine one fee, so a higher-precedence value on either side suppresses
// a lower-precedence value on the other.
func applyTransactionDefaults(cmd *cobra.Command, rc *aktctx.Context) {
	flags := cmd.Flags()
	if flags.Lookup(cflags.FlagGas) == nil {
		return
	}

	setDefault := func(name, envName, contextValue string) {
		flag := flags.Lookup(name)
		if flag == nil || flag.Changed {
			return
		}
		value, exists := os.LookupEnv(envName)
		if !exists {
			value = contextValue
		}
		if value != "" {
			_ = flag.Value.Set(value)
		}
	}

	setDefault(cflags.FlagGas, "AKT_GAS", rc.Gas)
	setDefault(cflags.FlagGasAdjustment, "AKT_GAS_ADJUSTMENT", rc.GasAdjustment)
	if dryRun, _ := flags.GetBool(cflags.FlagDryRun); dryRun {
		// The SDK distinguishes gas auto (simulate then execute) from
		// --dry-run (simulate only). Leaving both enabled makes its simulation
		// builder demand a signing key even when the user supplied an address.
		// The gas value is immaterial to dry-run and is replaced by the estimate.
		_ = flags.Lookup(cflags.FlagGas).Value.Set("0")
	}

	feesFlag := flags.Lookup(cflags.FlagFees)
	pricesFlag := flags.Lookup(cflags.FlagGasPrices)
	if feesFlag == nil || pricesFlag == nil || feesFlag.Changed || pricesFlag.Changed {
		return
	}

	envFees, hasEnvFees := os.LookupEnv("AKT_FEES")
	envPrices, hasEnvPrices := os.LookupEnv("AKT_GAS_PRICES")
	switch {
	case hasEnvFees && envFees != "":
		_ = feesFlag.Value.Set(envFees)
		_ = pricesFlag.Value.Set("")
	case hasEnvPrices:
		_ = feesFlag.Value.Set("")
		_ = pricesFlag.Value.Set(envPrices)
	case hasEnvFees:
		_ = feesFlag.Value.Set("")
		_ = pricesFlag.Value.Set(rc.GasPrices)
	case rc.Fees != "":
		_ = feesFlag.Value.Set(rc.Fees)
		_ = pricesFlag.Value.Set("")
	case rc.GasPrices != "":
		_ = pricesFlag.Value.Set(rc.GasPrices)
	}
}

func applyProviderDefaults(cmd *cobra.Command, rc *aktctx.Context) error {
	if cmd.Name() == "status" && cmd.Parent() != nil && cmd.Parent().Name() == "provider" {
		return nil
	}

	flag := cmd.Flags().Lookup("auth-type")
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup("auth-type")
	}
	if flag == nil || flag.Changed {
		return nil
	}

	authType, err := aktctx.ResolveProviderAuthType(rc.AuthType)
	if err != nil {
		return err
	}
	if err := flag.Value.Set(authType); err != nil {
		return fmt.Errorf("apply context provider auth type: %w", err)
	}
	return nil
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

// requiresConfig returns true if the command needs akt configuration to
// exist, and so may trigger the first-run bootstrap wizard when none is
// found. It is deliberately narrower than requiresContext: a command is
// exempt here only when it produces the same output with or without any
// configuration at all.
//
// Without this, `akt version` on an unconfigured machine either launched
// the wizard (in a terminal) or printed the wizard's "no terminal
// available" notice to stderr (outside one) before printing the version.
// Both are wrong for the command people run to check that a binary works
// — the first blocks a scripted smoke test on interactive input, the
// second makes a clean run look like a failure.
func requiresConfig(cmd *cobra.Command) bool {
	path := cmd.CommandPath()

	switch {
	// Build metadata is compiled in; configuration cannot change it.
	case strings.HasPrefix(path, "akt version"):
		return false
	// Completion scripts are generated from the command tree alone.
	case strings.HasPrefix(path, "akt completion"):
		return false
	// A monitor invocation with its own RPC is a complete standalone setup.
	case strings.HasPrefix(path, "akt monitor"):
		return false
	// SDL authoring is entirely local (see requiresContext), so demanding
	// a network fetch before linting a local file would be backwards.
	case strings.HasPrefix(path, "akt sdl"):
		return false
	}

	return true
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
	// The MCP server resolves a Console credential the same way, and serves
	// the Console tools off it alone. Requiring a context here would refuse a
	// server to anyone holding only an API key, which is exactly the managed
	// setup that has no wallet to configure. It reports for itself when
	// neither rail is available.
	case strings.HasPrefix(path, "akt mcp"):
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
type monitorRuntime struct {
	rpcEndpoint  string
	cacheDir     string
	restEndpoint string
}

func resolveMonitorRuntime(
	v *viper.Viper,
	rpcEndpoint string,
	rpcExplicit bool,
	restEndpoint string,
	homeFn func() string,
	mgrFn func() *aktctx.Manager,
) (monitorRuntime, error) {
	cfgRoot := homeFn()
	if cfgRoot == "" {
		var err error
		cfgRoot, err = aktctx.ConfigHome(v.GetString("home"))
		if err != nil {
			return monitorRuntime{}, err
		}
	}

	var resolved *aktctx.Context
	if mgr := mgrFn(); mgr != nil {
		ctxName := activeContextName(mgr, v.GetString("context"))
		if ctxName != "" {
			var err error
			resolved, err = mgr.Resolve(ctxName)
			if err != nil {
				return monitorRuntime{}, err
			}
		}
	}

	selectedFromContext := !rpcExplicit
	if resolved != nil && rpcExplicit {
		selectedFromContext = endpointInNetwork(rpcEndpoint, resolved.Network.Endpoints.RPC)
	}
	if resolved != nil && !rpcExplicit &&
		resolved.Network.ChainID == "akashnet-2" &&
		sameMonitorEndpoint(rpcEndpoint, "https://rpc.akashnet.net:443") {
		if template, ok := aktctx.NetworkTemplates()["mainnet"]; ok && len(template.Endpoints.RPC) > 0 {
			rpcEndpoint = template.Endpoints.RPC[0]
		}
	}

	if restEndpoint == "" && selectedFromContext && resolved != nil &&
		len(resolved.Network.Endpoints.API) > 0 {
		restEndpoint = resolved.Network.Endpoints.API[0]
	}
	if restEndpoint == "" {
		var err error
		restEndpoint, err = deriveMonitorRESTEndpoint(rpcEndpoint)
		if err != nil {
			return monitorRuntime{}, err
		}
	}

	return monitorRuntime{
		rpcEndpoint:  rpcEndpoint,
		cacheDir:     filepath.Join(cfgRoot, "cache"),
		restEndpoint: restEndpoint,
	}, nil
}

func endpointInNetwork(endpoint string, candidates []string) bool {
	for _, candidate := range candidates {
		if sameMonitorEndpoint(endpoint, candidate) {
			return true
		}
	}
	return false
}

func sameMonitorEndpoint(left, right string) bool {
	return strings.TrimSuffix(arpcclient.NormalizeEndpoint(left), "/") ==
		strings.TrimSuffix(arpcclient.NormalizeEndpoint(right), "/")
}

func deriveMonitorRESTEndpoint(rpcEndpoint string) (string, error) {
	parsed, err := url.Parse(rpcEndpoint)
	if err != nil {
		return "", fmt.Errorf("derive monitor REST endpoint from RPC %q: %w", rpcEndpoint, err)
	}
	if parsed.Scheme == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("derive monitor REST endpoint: RPC endpoint %q must include a scheme and hostname", rpcEndpoint)
	}

	switch parsed.Scheme {
	case "ws", "tcp":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""

	trimmedPath := strings.TrimSuffix(parsed.Path, "/")
	if strings.HasSuffix(trimmedPath, "/rpc") {
		parsed.Path = strings.TrimSuffix(trimmedPath, "/rpc") + "/rest"
		parsed.RawPath = ""
		return parsed.String(), nil
	}

	parsed.Host = net.JoinHostPort(parsed.Hostname(), "1317")
	parsed.Path = ""
	parsed.RawPath = ""
	return parsed.String(), nil
}

func monitorRunE(
	v *viper.Viper,
	dashboard string,
	homeFn func() string,
	mgrFn func() *aktctx.Manager,
) func(*cobra.Command, []string) error {
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
		rpcExplicit := len(args) > 0 || cmd.Flags().Changed("rpc")

		// Resolve endpoints from active akt context when not
		// explicitly provided via flags.
		cctx := sdkclient.GetClientContextFromCmd(cmd)
		if rpcEndpoint == "" && cctx.NodeURI != "" {
			rpcEndpoint = cctx.NodeURI
		}

		if rpcEndpoint == "" {
			return fmt.Errorf("no RPC endpoint; provide one via --rpc flag, positional argument, or configure an akt context")
		}

		runtime, err := resolveMonitorRuntime(
			v,
			rpcEndpoint,
			rpcExplicit,
			restEndpoint,
			homeFn,
			mgrFn,
		)
		if err != nil {
			return err
		}
		rpcEndpoint = runtime.rpcEndpoint
		restEndpoint = runtime.restEndpoint

		// Ensure endpoints carry explicit ports (inferred from scheme
		// when omitted) so downstream CometBFT clients can connect.
		rpcEndpoint = arpcclient.NormalizeEndpoint(rpcEndpoint)
		restEndpoint = arpcclient.NormalizeEndpoint(restEndpoint)

		if cleanCache {
			if err := clearMonitorCache(runtime.cacheDir); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Cache cleared")
		}

		return akttui.RunMonitor(akttui.Config{
			Viper:            v,
			RPCEndpoint:      rpcEndpoint,
			RESTEndpoint:     restEndpoint,
			CacheDir:         runtime.cacheDir,
			Insecure:         insecure,
			Standalone:       true,
			InitialDashboard: dashboard,
		})
	}
}

func clearMonitorCache(cacheDir string) error {
	for _, name := range []string{"monitor.db", "top.db"} {
		path := filepath.Join(cacheDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clear monitor cache %s: %w", path, err)
		}
	}
	return nil
}

// addMonitorFlags adds the shared flags used by monitor and all its
// subcommands.
func addMonitorFlags(cmd *cobra.Command) {
	cmd.Flags().String("rpc", "", "RPC endpoint URL (resolved from context if not set)")
	cmd.Flags().String("rest", "", "REST endpoint URL (resolved from context if not set)")
	cmd.Flags().Bool("clean-cache", false, "Delete monitor cache and start fresh")
	cmd.Flags().Bool("insecure", false, "Skip TLS certificate verification for provider queries")
}

func monitorCmd(
	v *viper.Viper,
	homeFn func() string,
	mgrFn func() *aktctx.Manager,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor [rpc-endpoint]",
		Short: "Real-time monitoring hub",
		// Capability gating: monitoring needs a chain RPC endpoint.
		Annotations: map[string]string{capability.AnnotationKey: string(capability.ChainQuery)},
		Long: `Interactive TUI for monitoring Akash Network state in real time.

The monitor hub provides three dashboards navigable via Tab/Shift-Tab:

  Network     Consensus state, validators, governance proposals and parameters
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
  akt monitor https://rpc.akt.dev:443/rpc

  # Launch directly into the Provider dashboard
  akt monitor provider`,
		RunE: monitorRunE(v, "", homeFn, mgrFn),
	}

	addMonitorFlags(cmd)

	cmd.AddCommand(monitorNetworkCmd(v, homeFn, mgrFn))
	cmd.AddCommand(monitorProviderCmd(v, homeFn, mgrFn))
	cmd.AddCommand(monitorOracleCmd(v, homeFn, mgrFn))
	cmd.AddCommand(monitorBMECmd(v, homeFn, mgrFn))

	return cmd
}

func monitorNetworkCmd(
	v *viper.Viper,
	homeFn func() string,
	mgrFn func() *aktctx.Manager,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network [rpc-endpoint]",
		Short: "Network monitoring (consensus, validators, governance)",
		Long: `Launch the monitor directly into the Network dashboard.

Displays real-time consensus state (height, round, step), validator voting
progress, governance proposals and network parameters. Sub-tabs:

  1  Overview    Consensus state, vote progress bars, vote grid
  2  Validators  Scrollable validator list with signing history
  3  Governance  Recent and active proposals with vote tallies
  4  Parameters  Module-by-module parameter browser`,
		Args:    cobra.MaximumNArgs(1),
		Example: `  akt monitor network https://rpc.akt.dev:443/rpc`,
		RunE:    monitorRunE(v, "network", homeFn, mgrFn),
	}

	addMonitorFlags(cmd)

	return cmd
}

func monitorProviderCmd(
	v *viper.Viper,
	homeFn func() string,
	mgrFn func() *aktctx.Manager,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider [rpc-endpoint]",
		Short: "Provider fleet monitoring",
		Long: `Launch the monitor directly into the Provider dashboard.

Displays real-time provider fleet health: version distribution with dot
visualization, provider health scanning with priority-based scheduling,
and per-provider detail with node-level CPU/memory/GPU resources.`,
		Args:    cobra.MaximumNArgs(1),
		Example: `  akt monitor provider`,
		RunE:    monitorRunE(v, "provider", homeFn, mgrFn),
	}

	addMonitorFlags(cmd)

	return cmd
}

func monitorOracleCmd(
	v *viper.Viper,
	homeFn func() string,
	mgrFn func() *aktctx.Manager,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oracle [rpc-endpoint]",
		Short: "Oracle and BME monitoring",
		Long: `Launch the monitor directly into the Oracle/BME dashboard.

Displays oracle price data (aggregated prices, TWAP, health) and
BME state (mint status, vault balances, ledger entries). This is
the same dashboard as "akt monitor bme".`,
		Args:    cobra.MaximumNArgs(1),
		Example: `  akt monitor oracle`,
		RunE:    monitorRunE(v, "oracle-bme", homeFn, mgrFn),
	}

	addMonitorFlags(cmd)

	return cmd
}

func monitorBMECmd(
	v *viper.Viper,
	homeFn func() string,
	mgrFn func() *aktctx.Manager,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bme [rpc-endpoint]",
		Short: "BME and Oracle monitoring",
		Long: `Launch the monitor directly into the Oracle/BME dashboard.

Displays BME state (mint status, vault balances, ledger entries)
and oracle price data (aggregated prices, TWAP, health). This is
the same dashboard as "akt monitor oracle".`,
		Args:    cobra.MaximumNArgs(1),
		Example: `  akt monitor bme`,
		RunE:    monitorRunE(v, "oracle-bme", homeFn, mgrFn),
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
			long, _ := cmd.Flags().GetBool("long")

			if f := output.FormatFromCmd(cmd); f != output.FormatTable {
				payload := struct {
					Version   string `json:"version"            yaml:"version"`
					Commit    string `json:"commit"             yaml:"commit"`
					Built     string `json:"built"              yaml:"built"`
					Go        string `json:"go,omitempty"       yaml:"go,omitempty"`
					Platform  string `json:"platform,omitempty" yaml:"platform,omitempty"`
					BuildTags string `json:"buildTags,omitempty" yaml:"buildTags,omitempty"`
				}{Version: bi.Version, Commit: bi.Commit, Built: bi.Date}

				if long {
					payload.Go = runtime.Version()
					payload.Platform = runtime.GOOS + "/" + runtime.GOARCH

					if dbi, ok := debug.ReadBuildInfo(); ok {
						for _, setting := range dbi.Settings {
							if setting.Key == "-tags" && setting.Value != "" {
								payload.BuildTags = setting.Value
							}
						}
					}
				}

				return output.Print(f, payload)
			}

			if long {
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
