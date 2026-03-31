package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"pkg.akt.dev/akt/internal/bootstrap"
	chaincli "pkg.akt.dev/akt/internal/cli/chain"

	clicontext "pkg.akt.dev/akt/internal/cli/context"
	aktclient "pkg.akt.dev/akt/internal/client"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
	topcache "pkg.akt.dev/akt/internal/top/cache"
	toprpc "pkg.akt.dev/akt/internal/top/rpc"
	topui "pkg.akt.dev/akt/internal/top/ui"
	akttui "pkg.akt.dev/akt/internal/tui"
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

	// The manager and keyring manager are lazily initialized in PersistentPreRunE.
	var mgr *aktctx.Manager
	var krMgr *aktkeyring.Manager

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

			cfgRoot, err := aktctx.ConfigHome(v.GetString("home"))
			if err != nil {
				return err
			}

			// First-run bootstrap: if no config file exists, offer to
			// fetch networks from github.com/akash-network/net.
			cfgPath := aktctx.ConfigPath(cfgRoot)
			if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
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
			//    require a configured context to proceed.
			if !resolved && requiresContext(cmd) {
				return noContextError(mgr)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkInteractive(v); err != nil {
				return err
			}
			return akttui.Run()
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global persistent flags. Values are read from Viper at point of use,
	// not captured into Go variables.
	root.PersistentFlags().String("home", "", "Home directory for config, contexts, and keyrings (default: $AKT_HOME or ~/.config/akt)")
	root.PersistentFlags().String("context", "", "Active context name (overrides current-context in config)")
	root.PersistentFlags().StringP("output", "o", "table", "Output format: table, json, yaml")
	root.PersistentFlags().BoolP("interactive", "i", false, "Force interactive (TUI) mode even if disabled in config")

	// Context management (includes network and keys subcommands).
	root.AddCommand(clicontext.Commands(mgrFn, getKeyring))

	// TX and Query command trees from local chain CLI copy.
	root.AddCommand(chaincli.TxCmd())
	root.AddCommand(chaincli.QueryCmd())

	root.AddCommand(topCmd(v))
	root.AddCommand(versionCmd(bi))

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

	// The root command itself (help), context management, version, and top
	// work without an active context.
	switch {
	case path == "akt":
		return false
	case strings.HasPrefix(path, "akt context"):
		return false
	case strings.HasPrefix(path, "akt version"):
		return false
	case strings.HasPrefix(path, "akt top"):
		return false
	}

	return true
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

func topCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "top [rpc-endpoint]",
		Short: "Real-time consensus and provider monitor",
		Long: `Interactive TUI for monitoring Akash Network consensus state,
validator voting, provider fleet status, and governance parameters.

Connects to the RPC endpoint via WebSocket for real-time vote streaming.
The RPC endpoint must support WebSocket connections.

If --rpc is not specified, the endpoint is resolved from the active akt
context. A positional argument overrides the --rpc flag.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			// Resolve store path for the bbolt cache.
			cfgRoot, err := aktctx.ConfigHome("")
			if err != nil {
				return err
			}

			dbDir := filepath.Join(cfgRoot, "cache")
			if err := os.MkdirAll(dbDir, 0o755); err != nil {
				return fmt.Errorf("create cache dir: %w", err)
			}
			dbPath := filepath.Join(dbDir, "top.db")

			if cleanCache {
				_ = os.Remove(dbPath)
				fmt.Println("Cache cleared")
			}

			db, err := topcache.OpenDB(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			providerCache, err := topcache.Open(db)
			if err != nil {
				return err
			}

			monikerCache, err := topcache.OpenMonikerCache(db)
			if err != nil {
				return err
			}

			client := toprpc.NewClient(rpcEndpoint, restEndpoint)
			rpcProviderClient := toprpc.NewRPCProviderClient(rpcEndpoint)

			model := topui.NewModel(topui.ModelConfig{
				Client:             client,
				RPCClient:          rpcProviderClient,
				Cache:              providerCache,
				MonikerCache:       monikerCache,
				InsecureSkipVerify: insecure,
			})

			p := tea.NewProgram(model, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}

	cmd.Flags().String("rpc", "", "RPC endpoint URL (resolved from context if not set)")
	cmd.Flags().String("rest", "", "REST endpoint URL (resolved from context if not set)")
	cmd.Flags().Bool("clean-cache", false, "Delete top cache and start fresh")
	cmd.Flags().Bool("insecure", true, "Skip TLS certificate verification for provider queries")

	return cmd
}

func versionCmd(bi BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("akt %s (commit: %s, built: %s)\n", bi.Version, bi.Commit, bi.Date)
		},
	}
}
