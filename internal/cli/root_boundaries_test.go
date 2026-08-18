package cli

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"pkg.akt.dev/akt/internal/actionlog"
	aktclient "pkg.akt.dev/akt/internal/client"
	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
)

func TestRootPostRunReportsActionLogCloseFailure(t *testing.T) {
	logger, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("close action log: %v", err)
	}

	root := NewRootCmd(BuildInfo{})
	root.SetContext(cliutil.WithActionLog(context.Background(), logger))
	err = root.PersistentPostRunE(root, nil)
	if err == nil || !strings.Contains(err.Error(), "file already closed") {
		t.Fatalf("root post-run error = %v, want action-log close failure", err)
	}
}

func TestRootConfigurationBoundaries(t *testing.T) {
	root := &cobra.Command{Use: "akt"}

	for _, name := range []string{"version", "completion", "monitor", "mcp", "sdl", "context create", "context network create"} {
		parts := strings.Split(name, " ")
		cmd := &cobra.Command{Use: parts[len(parts)-1]}
		if len(parts) == 1 {
			root.AddCommand(cmd)
		} else {
			parent := &cobra.Command{Use: parts[0]}
			root.AddCommand(parent)
			parent.AddCommand(cmd)
		}
		if requiresConfig(cmd) {
			t.Errorf("%s unexpectedly requires bootstrap configuration", name)
		}
	}

	tx := &cobra.Command{Use: "tx"}
	root.AddCommand(tx)
	if !requiresConfig(tx) {
		t.Fatal("transaction commands must retain the configured-context startup path")
	}
	if requiresContext(root) {
		t.Fatal("the bare root command must work without a context")
	}
	if !requiresContext(tx) {
		t.Fatal("transaction commands must require a resolved context")
	}
}

func TestNoContextErrorDistinguishesEmptyAndUnselectedConfiguration(t *testing.T) {
	empty, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager(empty): %v", err)
	}
	if got := noContextError(empty).Error(); !strings.Contains(got, "no contexts configured") {
		t.Fatalf("empty-context error = %q", got)
	}

	mgr, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}
	for _, name := range []string{"prod", "staging"} {
		if err := mgr.CreateContext(aktctx.Context{
			Name:    name,
			Network: aktctx.Network{Name: "mainnet"},
		}); err != nil {
			t.Fatalf("CreateContext(%s): %v", name, err)
		}
	}

	got := noContextError(mgr).Error()
	for _, want := range []string{"no active context", "prod", "staging", "akt context use"} {
		if !strings.Contains(got, want) {
			t.Errorf("unselected-context error %q missing %q", got, want)
		}
	}
}

func TestRootSelectionAndGatingDefaults(t *testing.T) {
	if got := activeContextName(nil, "override"); got != "override" {
		t.Fatalf("nil-manager context = %q, want override", got)
	}

	v := viper.New()
	if got := gatingMode(v, nil); got != "" {
		t.Fatalf("empty gating mode = %q", got)
	}
	v.Set("defaults.command-gating", "hide")
	if got := gatingMode(v, nil); got != "hide" {
		t.Fatalf("Viper gating mode = %q, want hide", got)
	}

	mgr, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cfg := mgr.Config()
	cfg.Defaults.CommandGating = "dim"
	if err := aktctx.SaveConfig(mgr.Root(), &cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	mgr, err = aktctx.NewManager(mgr.Root())
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	if got := gatingMode(viper.New(), mgr); got != "dim" {
		t.Fatalf("config gating mode = %q, want dim", got)
	}
}

func TestInteractivePolicyHonorsFlagThenConfiguration(t *testing.T) {
	v := viper.New()
	v.Set("interactive", true)
	if err := checkInteractive(v); err != nil {
		t.Fatalf("explicit interactive flag: %v", err)
	}

	v = viper.New()
	v.Set("defaults.interactive", true)
	if err := checkInteractive(v); err != nil {
		t.Fatalf("interactive config: %v", err)
	}

	if err := checkInteractive(viper.New()); err == nil || !strings.Contains(err.Error(), "--interactive") {
		t.Fatalf("disabled interactive error = %v", err)
	}
}

func TestRootCanonicalFlagsDrivePreRunAndCompletion(t *testing.T) {
	t.Run("quiet conflicts with verbose", func(t *testing.T) {
		root := NewRootCmd(BuildInfo{})
		root.SetArgs([]string{"--home", t.TempDir(), "--quiet", "--verbose", "version"})
		err := Execute(root)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("root error = %v", err)
		}
	})

	t.Run("interactive root", func(t *testing.T) {
		root := NewRootCmd(BuildInfo{})
		root.SetArgs([]string{"--home", t.TempDir(), "--interactive"})
		err := Execute(root)
		if err == nil || !strings.Contains(err.Error(), "TUI is currently disabled") {
			t.Fatalf("root error = %v", err)
		}
	})

	root := NewRootCmd(BuildInfo{})
	complete, ok := root.GetFlagCompletionFunc(flagdefs.FlagContext)
	if !ok {
		t.Fatal("context completion is not registered")
	}
	names, directive := complete(root, nil, "")
	if len(names) != 0 || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("context completions = %v, directive = %v", names, directive)
	}
}

func TestLocalIdentityDryRunsDoNotOpenSigningKeyrings(t *testing.T) {
	root := &cobra.Command{Use: "akt"}

	provider := &cobra.Command{Use: "provider"}
	providerDryRun := &cobra.Command{Use: "send-manifest"}
	providerDryRun.Flags().Bool(flagdefs.FlagDryRun, true, "")
	provider.AddCommand(providerDryRun)
	root.AddCommand(provider)
	if got := localIdentityMode(providerDryRun); got != aktclient.LocalIdentityOnDemand {
		t.Fatalf("provider dry-run identity mode = %v, want on demand", got)
	}

	workflow := &cobra.Command{Use: "deploy"}
	workflow.Flags().Bool(flagdefs.FlagDryRun, true, "")
	root.AddCommand(workflow)
	if got := localIdentityMode(workflow); got != aktclient.LocalIdentityNone {
		t.Fatalf("workflow dry-run identity mode = %v, want none", got)
	}
}

func TestMCPWriteOptInKeepsIdentityOnDemand(t *testing.T) {
	root := &cobra.Command{Use: "akt"}
	mcpCmd := &cobra.Command{Use: "mcp"}
	mcpCmd.Flags().Bool("enable-writes", true, "")
	root.AddCommand(mcpCmd)

	if got := localIdentityMode(mcpCmd); got != aktclient.LocalIdentityOnDemand {
		t.Fatalf("MCP write identity mode = %v, want on demand", got)
	}
}

func TestVersionOutputContracts(t *testing.T) {
	bi := BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-08-12T00:00:00Z"}

	t.Run("short pretty", func(t *testing.T) {
		cmd := versionCmd(bi)
		cmd.Flags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "test output")
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("version: %v", err)
		}
		want := "akt 1.2.3 (commit: abc123, built: 2026-08-12T00:00:00Z)\n"
		if out.String() != want {
			t.Fatalf("version output = %q, want %q", out.String(), want)
		}
	})

	t.Run("long pretty", func(t *testing.T) {
		cmd := versionCmd(bi)
		cmd.Flags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "test output")
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--long"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("version --long: %v", err)
		}
		for _, want := range []string{
			"version:    1.2.3",
			"commit:     abc123",
			"go:         " + runtime.Version(),
			"platform:   " + runtime.GOOS + "/" + runtime.GOARCH,
		} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("long version output %q missing %q", out.String(), want)
			}
		}
	})

	t.Run("long JSON", func(t *testing.T) {
		cmd := versionCmd(bi)
		cmd.Flags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "test output")
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--long", "--output", "json"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("version --long --output json: %v", err)
		}
		var got struct {
			Version  string `json:"version"`
			Commit   string `json:"commit"`
			Go       string `json:"go"`
			Platform string `json:"platform"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("decode version JSON: %v", err)
		}
		if got.Version != bi.Version || got.Commit != bi.Commit || got.Go != runtime.Version() || got.Platform != runtime.GOOS+"/"+runtime.GOARCH {
			t.Fatalf("long version JSON = %+v", got)
		}
	})
}
