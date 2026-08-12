package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	aktctx "pkg.akt.dev/akt/internal/context"
)

func requireNetworkChainID(t *testing.T, mgr *aktctx.Manager, name string) string {
	t.Helper()

	network := mgr.GetNetwork(name)
	if network == nil {
		t.Fatalf("network %q missing from test config", name)
		return ""
	}
	if network.ChainID == "" {
		t.Fatalf("network %q has no chain ID", name)
	}

	return network.ChainID
}

func TestResolveMonitorRuntimeHonorsHomeAndContextAPI(t *testing.T) {
	home := t.TempDir()
	mgr, err := aktctx.NewManager(home)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}
	if err := mgr.CreateContext(aktctx.Context{
		Name:    "monitoring",
		Network: aktctx.Network{Name: "mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := mgr.UseContext("monitoring"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}
	mainnetChainID := requireNetworkChainID(t, mgr, "mainnet")

	v := viper.New()
	v.Set("context", "monitoring")
	runtime, err := resolveMonitorRuntime(
		v,
		"https://rpc.akt.dev:443/rpc",
		false,
		"",
		func() string { return home },
		func() *aktctx.Manager { return mgr },
		mainnetChainID,
	)
	if err != nil {
		t.Fatalf("resolveMonitorRuntime: %v", err)
	}
	if want := filepath.Join(home, "cache"); runtime.cacheDir != want {
		t.Errorf("cache dir = %q, want %q", runtime.cacheDir, want)
	}
	if want := mgr.GetNetwork("mainnet").Endpoints.API[0]; runtime.restEndpoint != want {
		t.Errorf("REST endpoint = %q, want context API %q", runtime.restEndpoint, want)
	}

	runtime, err = resolveMonitorRuntime(
		v,
		"https://rpc.akt.dev:443/rpc",
		false,
		"https://override.example.com",
		func() string { return home },
		func() *aktctx.Manager { return mgr },
		mainnetChainID,
	)
	if err != nil {
		t.Fatalf("resolveMonitorRuntime override: %v", err)
	}
	if runtime.restEndpoint != "https://override.example.com" {
		t.Errorf("REST override = %q, want explicit endpoint", runtime.restEndpoint)
	}
}

func TestResolveMonitorRuntimeDoesNotMixAdHocRPCWithContextAPI(t *testing.T) {
	home := t.TempDir()
	mgr, err := aktctx.NewManager(home)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}
	if err := mgr.CreateContext(aktctx.Context{
		Name:    "monitoring",
		Network: aktctx.Network{Name: "mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := mgr.UseContext("monitoring"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}
	mainnetChainID := requireNetworkChainID(t, mgr, "mainnet")

	runtime, err := resolveMonitorRuntime(
		viper.New(),
		"https://custom.example.com:26657",
		true,
		"",
		func() string { return home },
		func() *aktctx.Manager { return mgr },
		mainnetChainID,
	)
	if err != nil {
		t.Fatalf("resolveMonitorRuntime: %v", err)
	}
	if runtime.rpcEndpoint != "https://custom.example.com:26657" {
		t.Errorf("RPC endpoint = %q, want explicit custom RPC", runtime.rpcEndpoint)
	}
	if runtime.restEndpoint != "https://custom.example.com:1317" {
		t.Errorf("REST endpoint = %q, want same-origin derivation", runtime.restEndpoint)
	}
}

func TestResolveMonitorRuntimeDerivesGatewayRESTPath(t *testing.T) {
	runtime, err := resolveMonitorRuntime(
		viper.New(),
		"https://rpc.example.com:443/rpc",
		true,
		"",
		func() string { return t.TempDir() },
		func() *aktctx.Manager { return nil },
		"",
	)
	if err != nil {
		t.Fatalf("resolveMonitorRuntime: %v", err)
	}
	if runtime.restEndpoint != "https://rpc.example.com:443/rest" {
		t.Errorf("REST endpoint = %q, want /rest gateway path", runtime.restEndpoint)
	}
}

func TestResolveMonitorRuntimeConvertsTCPToHTTPForREST(t *testing.T) {
	runtime, err := resolveMonitorRuntime(
		viper.New(),
		"tcp://rpc.example.com:26657",
		true,
		"",
		func() string { return t.TempDir() },
		func() *aktctx.Manager { return nil },
		"",
	)
	if err != nil {
		t.Fatalf("resolveMonitorRuntime: %v", err)
	}
	if runtime.restEndpoint != "http://rpc.example.com:1317" {
		t.Errorf("REST endpoint = %q, want HTTP endpoint", runtime.restEndpoint)
	}
}

func TestResolveMonitorRuntimeUpgradesLegacyBuiltInRPC(t *testing.T) {
	home := t.TempDir()
	mgr, err := aktctx.NewManager(home)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	mainnet := aktctx.Network{
		Name:    "mainnet",
		ChainID: "registry-mainnet-current",
		Endpoints: aktctx.Endpoints{
			RPC: []string{"https://rpc.akt.dev:443/rpc"},
		},
	}
	if err := mgr.CreateNetwork(mainnet); err != nil {
		t.Fatalf("CreateNetwork(mainnet): %v", err)
	}
	if err := mgr.CreateNetwork(aktctx.Network{
		Name:    "legacy-mainnet",
		ChainID: mainnet.ChainID,
		Endpoints: aktctx.Endpoints{
			RPC: []string{"https://rpc.akashnet.net:443"},
			API: []string{"https://api.akashnet.net:443"},
		},
	}); err != nil {
		t.Fatalf("CreateNetwork: %v", err)
	}
	if err := mgr.CreateContext(aktctx.Context{
		Name:    "monitoring",
		Network: aktctx.Network{Name: "legacy-mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := mgr.UseContext("monitoring"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	runtime, err := resolveMonitorRuntime(
		viper.New(),
		"https://rpc.akashnet.net:443",
		false,
		"",
		func() string { return home },
		func() *aktctx.Manager { return mgr },
		mainnet.ChainID,
	)
	if err != nil {
		t.Fatalf("resolveMonitorRuntime: %v", err)
	}
	if runtime.rpcEndpoint != "https://rpc.akt.dev:443/rpc" {
		t.Errorf("RPC endpoint = %q, want current built-in monitor endpoint", runtime.rpcEndpoint)
	}
	if runtime.restEndpoint != "https://api.akashnet.net:443" {
		t.Errorf("REST endpoint = %q, want matching context API", runtime.restEndpoint)
	}
}

func TestMonitorInsecureFlagDefaultsToFalse(t *testing.T) {
	cmd := monitorCmd(
		viper.New(),
		func() string { return t.TempDir() },
		func() *aktctx.Manager { return nil },
		func() string { return "" },
	)

	commands := []*cobra.Command{cmd}
	commands = append(commands, cmd.Commands()...)
	for _, candidate := range commands {
		flag := candidate.Flags().Lookup("insecure")
		if flag == nil {
			t.Fatalf("%s has no --insecure flag", candidate.CommandPath())
			return
		}
		if flag.DefValue != "false" {
			t.Errorf("%s --insecure default = %q, want false", candidate.CommandPath(), flag.DefValue)
		}
	}
}

func TestMonitorCommandPropagatesRuntimeStartupFailure(t *testing.T) {
	parent := t.TempDir()
	occupied := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(occupied, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("write occupied home: %v", err)
	}

	v := viper.New()
	v.Set("defaults.interactive", true)
	cmd := monitorCmd(
		v,
		func() string { return occupied },
		func() *aktctx.Manager { return nil },
		func() string { return "" },
	)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"http://127.0.0.1:26657"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "create monitor cache directory") {
		t.Fatalf("monitor startup error = %v, want cache initialization failure", err)
	}
}

func TestMonitorWithExplicitRPCDoesNotRequireConfig(t *testing.T) {
	root := &cobra.Command{Use: "akt"}
	monitor := &cobra.Command{Use: "monitor"}
	root.AddCommand(monitor)

	if requiresConfig(monitor) {
		t.Fatal("monitor with its own RPC must not trigger config bootstrap")
	}
}

func TestMCPDoesNotRunInteractiveConfigBootstrap(t *testing.T) {
	root := &cobra.Command{Use: "akt"}
	mcp := &cobra.Command{Use: "mcp"}
	root.AddCommand(mcp)

	if requiresConfig(mcp) {
		t.Fatal("MCP stdio must not trigger interactive config bootstrap")
	}
}

func TestClearMonitorCacheRemovesCurrentAndLegacyFiles(t *testing.T) {
	cacheDir := t.TempDir()
	for _, name := range []string{"monitor.db", "top.db"} {
		if err := os.WriteFile(filepath.Join(cacheDir, name), []byte("cache"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := clearMonitorCache(cacheDir); err != nil {
		t.Fatalf("clearMonitorCache: %v", err)
	}
	for _, name := range []string{"monitor.db", "top.db"} {
		if _, err := os.Stat(filepath.Join(cacheDir, name)); !os.IsNotExist(err) {
			t.Errorf("%s still exists: %v", name, err)
		}
	}
}

func TestClearMonitorCacheReportsDeletionFailure(t *testing.T) {
	cacheDir := t.TempDir()
	dbPath := filepath.Join(cacheDir, "monitor.db")
	if err := os.Mkdir(dbPath, 0o700); err != nil {
		t.Fatalf("mkdir monitor.db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dbPath, "child"), []byte("busy"), 0o600); err != nil {
		t.Fatalf("write child: %v", err)
	}

	err := clearMonitorCache(cacheDir)
	if err == nil || !strings.Contains(err.Error(), "monitor.db") {
		t.Fatalf("error = %v, want failed cache path", err)
	}
}
