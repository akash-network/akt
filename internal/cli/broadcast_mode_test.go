package cli

import (
	"bytes"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	aktctx "pkg.akt.dev/akt/internal/context"
	flagdefs "pkg.akt.dev/akt/internal/flags"
)

func TestBroadcastModeDefaultLeavesReadOnlyCommandsAlone(t *testing.T) {
	v := viper.New()
	v.Set("defaults.broadcast-mode", "invalid")
	if err := applyBroadcastModeDefault(&cobra.Command{Use: "query"}, v); err != nil {
		t.Fatalf("read-only command validated a transaction setting: %v", err)
	}
}

func TestRootBroadcastModePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, config, env, flag, want string
		wantError                     bool
	}{
		{name: "default", want: "sync"},
		{name: "configured block", config: "block", want: "block"},
		{name: "configured async", config: "async", want: "async"},
		{name: "environment overrides config", config: "async", env: "block", want: "block"},
		{name: "flag overrides environment", config: "block", env: "async", flag: "sync", want: "sync"},
		{name: "flag overrides config", config: "async", flag: "block", want: "block"},
		{name: "invalid config", config: "bogus", wantError: true},
		{name: "invalid environment", config: "block", env: "bogus", wantError: true},
		{name: "environment overrides invalid config", config: "bogus", env: "block", want: "block"},
		{name: "flag overrides invalid defaults", config: "bogus", env: "bogus", flag: "block", want: "block"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AKT_BROADCAST_MODE", tc.env)
			m := rootTestManager(t)
			if err := m.CreateContext(aktctx.Context{Name: "probe", Network: aktctx.Network{Name: "mainnet"}, Keyring: aktctx.Keyring{Name: "default"}}); err != nil {
				t.Fatal(err)
			}
			cfg := m.Config()
			cfg.CurrentContext = "probe"
			cfg.Defaults.BroadcastMode = tc.config
			cfg.Networks[0].Endpoints.RPC = []string{"http://127.0.0.1:1"}
			if err := aktctx.SaveConfig(m.Root(), &cfg); err != nil {
				t.Fatal(err)
			}
			root := NewRootCmd(BuildInfo{Version: "test"})
			leaf, _, err := root.Find([]string{"tx", "bank", "send"})
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			leaf.RunE = func(cmd *cobra.Command, _ []string) error {
				cctx, err := chaincli.GetClientTxContext(cmd)
				got = cctx.BroadcastMode
				if cmd.Flags().Changed(flagdefs.FlagBroadcastMode) != (tc.flag != "") {
					t.Error("applying a broadcast default changed explicit-flag semantics")
				}
				return err
			}
			address := sdk.AccAddress([]byte("12345678901234567890")).String()
			args := []string{"--home", m.Root(), "--context", "probe", "tx", "bank", "send", address, address, "1uakt", "--generate-only", "--offline"}
			if tc.flag != "" {
				args = append(args, "--broadcast-mode", tc.flag)
			}
			root.SetArgs(args)
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			err = Execute(root)
			if tc.wantError {
				if err == nil || ExitCode(err) != ExitConfig || !strings.Contains(err.Error(), "broadcast") {
					t.Fatalf("error = %v, want broadcast configuration error", err)
				}
				if got != "" {
					t.Fatal("invalid broadcast mode reached transaction execution")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("effective broadcast mode = %q, want %q", got, tc.want)
			}
		})
	}
}
