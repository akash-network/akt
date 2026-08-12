package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/actionlog"
	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	cflags "pkg.akt.dev/akt/internal/cli/chain/flags"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func rootTestManager(t *testing.T) *aktctx.Manager {
	t.Helper()

	root := t.TempDir()
	m, err := aktctx.NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	cfg := m.Config()
	cfg.Keyrings[0].Backend = sdkkeyring.BackendTest
	if err := aktctx.SaveConfig(root, &cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	m, err = aktctx.NewManager(root)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatalf("CreateNetworkFromTemplate: %v", err)
	}

	return m
}

func executeRoot(t *testing.T, args ...string) error {
	t.Helper()

	cmd := NewRootCmd(BuildInfo{Version: "test"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)

	return Execute(cmd)
}

func TestRootKeyringUsesSoleSelectedContext(t *testing.T) {
	m := rootTestManager(t)
	if err := m.CreateKeyring(aktctx.Keyring{Name: "isolated", Backend: sdkkeyring.BackendTest}); err != nil {
		t.Fatalf("CreateKeyring: %v", err)
	}
	if err := m.CreateContext(aktctx.Context{
		Name:    "solo",
		Network: aktctx.Network{Name: "mainnet"},
		Keyring: aktctx.Keyring{Name: "isolated"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	if err := executeRoot(t, "--home", m.Root(), "context", "keys", "add", "alice", "--no-backup"); err != nil {
		t.Fatalf("keys add: %v", err)
	}

	enc := aktcodec.MakeEncodingConfig()
	keyrings := aktkeyring.NewManager(m.Root(), m.Config().Keyrings, enc.Codec)
	isolated, err := keyrings.Get("isolated")
	if err != nil {
		t.Fatalf("open isolated keyring: %v", err)
	}
	if _, err := isolated.Key("alice"); err != nil {
		t.Fatalf("alice was not written to the sole context's keyring: %v", err)
	}
	defaultKeyring, err := keyrings.Get("default")
	if err != nil {
		t.Fatalf("open default keyring: %v", err)
	}
	if _, err := defaultKeyring.Key("alice"); err == nil {
		t.Fatal("alice was also written to the unrelated default keyring")
	}
}

func TestRootActionLogUsesSelectedContext(t *testing.T) {
	m := rootTestManager(t)
	for _, name := range []string{"prod", "staging"} {
		if err := m.CreateContext(aktctx.Context{
			Name:       name,
			Network:    aktctx.Network{Name: "mainnet"},
			AuthMethod: aktctx.AuthMethodConsoleAPI,
		}); err != nil {
			t.Fatalf("CreateContext(%s): %v", name, err)
		}
		if err := aktctx.SetConsoleAPIKey(m.Root(), name, name+"-key"); err != nil {
			t.Fatalf("SetConsoleAPIKey(%s): %v", name, err)
		}
	}
	if err := m.UseContext("prod"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/deployments/42" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "staging-key" {
			t.Errorf("x-api-key = %q, want staging-key", got)
		}
		_, _ = w.Write([]byte(`{"data":{"success":true}}`))
	}))
	defer srv.Close()

	if err := executeRoot(t,
		"--home", m.Root(),
		"--context", "staging",
		"console", "deployment", "close", "42",
		"--console-api-url", srv.URL,
	); err != nil {
		t.Fatalf("console deployment close: %v", err)
	}

	stagingLog, err := actionlog.Open(aktctx.ActionLogPath(m.Root(), "staging"))
	if err != nil {
		t.Fatalf("open staging log: %v", err)
	}
	defer func() { _ = stagingLog.Close() }()
	entries, err := stagingLog.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read staging log: %v", err)
	}
	if len(entries) != 1 || entries[0].Type != actionlog.TypeConsole || entries[0].Action != "close-deployment" {
		t.Fatalf("staging entries = %+v, want one Console close-deployment entry", entries)
	}
}

func TestRootRefusesToRunWhenSelectedActionLogCannotOpen(t *testing.T) {
	m := rootTestManager(t)
	if err := m.CreateContext(aktctx.Context{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := m.UseContext("prod"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	// A directory at the action-log path makes the append-only file
	// impossible to open on every supported platform.
	logPath := aktctx.ActionLogPath(m.Root(), "prod")
	if err := os.MkdirAll(logPath, 0o700); err != nil {
		t.Fatalf("block action log path: %v", err)
	}

	root := NewRootCmd(BuildInfo{Version: "test"})
	contextCmd, _, err := root.Find([]string{"context"})
	if err != nil {
		t.Fatalf("find context command: %v", err)
	}
	ran := false
	contextCmd.AddCommand(&cobra.Command{
		Use:  "probe-log-open",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			ran = true
			return nil
		},
	})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--home", m.Root(), "context", "probe-log-open"})

	err = Execute(root)
	if err == nil || !strings.Contains(err.Error(), "open action log") {
		t.Fatalf("action-log startup error = %v, want open failure", err)
	}
	if ran {
		t.Fatal("command ran without its required action-log boundary")
	}
}

func TestRootRejectsEveryRawTxBeforeConsoleContextTxHooks(t *testing.T) {
	m := rootTestManager(t)
	cfg := m.Config()
	cfg.Defaults.CommandGating = "off"
	if err := aktctx.SaveConfig(m.Root(), &cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	var err error
	m, err = aktctx.NewManager(m.Root())
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	if err := m.CreateContext(aktctx.Context{
		Name:       "managed",
		Network:    aktctx.Network{Name: "mainnet"},
		AuthMethod: aktctx.AuthMethodConsoleAPI,
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := m.UseContext("managed"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	commands := map[string][]string{
		"bank send":          {"tx", "bank", "send", "managed", "akash1qypqxpq9qcrsszg2pvxq6rs0zqg3yyc5jepelx", "1uakt"},
		"certificate create": {"tx", "cert", "generate"},
		"deployment create":  {"tx", "deployment", "create", "missing.yaml"},
	}

	for name, args := range commands {
		t.Run(name, func(t *testing.T) {
			var runErr error
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Fatalf("raw tx panicked instead of failing at the auth boundary: %v", recovered)
					}
				}()
				runErr = executeRoot(t, append([]string{"--home", m.Root()}, args...)...)
			}()

			if runErr == nil {
				t.Fatal("raw tx succeeded under console-api auth")
			}
			for _, want := range []string{"raw chain transactions", "keyring", "akt deploy", "akt console"} {
				if !strings.Contains(runErr.Error(), want) {
					t.Errorf("error %q missing %q", runErr, want)
				}
			}
		})
	}
}

func TestRootResolvesFromFlagThenEnvironmentThenContext(t *testing.T) {
	m := rootTestManager(t)
	enc := aktcodec.MakeEncodingConfig()
	keyrings := aktkeyring.NewManager(m.Root(), m.Config().Keyrings, enc.Codec)
	kr, err := keyrings.Get("default")
	if err != nil {
		t.Fatalf("open keyring: %v", err)
	}

	addresses := make(map[string]string)
	for _, name := range []string{"context-signer", "env-signer", "flag-signer"} {
		record, _, err := kr.NewMnemonic(
			name,
			sdkkeyring.English,
			"m/44'/118'/0'/0/0",
			"",
			aktkeyring.DefaultAlgo(),
		)
		if err != nil {
			t.Fatalf("NewMnemonic(%s): %v", name, err)
		}
		address, err := record.GetAddress()
		if err != nil {
			t.Fatalf("GetAddress(%s): %v", name, err)
		}
		addresses[name] = address.String()
	}

	if err := m.CreateContext(aktctx.Context{
		Name:           "prod",
		Network:        aktctx.Network{Name: "mainnet"},
		DefaultAccount: "context-signer",
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := m.UseContext("prod"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}
	t.Setenv("AKT_FROM", "env-signer")

	run := func(args ...string) sdkclient.Context {
		t.Helper()
		root := NewRootCmd(BuildInfo{Version: "test"})
		var got sdkclient.Context
		inspect := &cobra.Command{
			Use:  "inspect-from",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				got = chaincli.GetClientContextFromCmd(cmd)
				return nil
			},
		}
		inspect.Flags().String(cflags.FlagFrom, "", "test signer")
		root.AddCommand(inspect)
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(append([]string{"--home", m.Root(), "inspect-from"}, args...))
		if err := Execute(root); err != nil {
			t.Fatalf("execute inspect-from %v: %v", args, err)
		}
		return got
	}

	fromEnv := run()
	if fromEnv.FromName != "env-signer" || fromEnv.FromAddress.String() != addresses["env-signer"] {
		t.Errorf("AKT_FROM resolved name=%q address=%q", fromEnv.FromName, fromEnv.FromAddress)
	}

	fromFlag := run("--from", "flag-signer")
	if fromFlag.FromName != "flag-signer" || fromFlag.FromAddress.String() != addresses["flag-signer"] {
		t.Errorf("--from resolved name=%q address=%q", fromFlag.FromName, fromFlag.FromAddress)
	}
}

func TestRootRejectsOnlineChainMismatchBeforeLeafHooks(t *testing.T) {
	m := rootTestManager(t)
	if err := m.CreateContext(aktctx.Context{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := m.UseContext("prod"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	run := func(args ...string) (bool, error) {
		root := NewRootCmd(BuildInfo{Version: "test"})
		ran := false
		inspect := &cobra.Command{
			Use:  "inspect-tx-boundary",
			Args: cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				ran = true
				return nil
			},
		}
		cflags.AddTxFlagsToCmd(inspect)
		root.AddCommand(inspect)
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(append([]string{"--home", m.Root(), "inspect-tx-boundary"}, args...))
		return ran, Execute(root)
	}

	ran, err := run("--chain-id", "wrong-chain")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("online mismatch error = %v", err)
	}
	if ran {
		t.Fatal("online mismatch reached the leaf RunE")
	}

	ran, err = run("--chain-id", "other-chain", "--offline")
	if err != nil {
		t.Fatalf("offline mismatch: %v", err)
	}
	if !ran {
		t.Fatal("explicit offline mismatch did not reach the leaf RunE")
	}
}

func TestWorkflowDryRunRejectsOnlineChainMismatchBeforePlan(t *testing.T) {
	m := rootTestManager(t)
	if err := m.CreateContext(aktctx.Context{
		Name:    "prod",
		Network: aktctx.Network{Name: "mainnet"},
	}); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if err := m.UseContext("prod"); err != nil {
		t.Fatalf("UseContext: %v", err)
	}

	root := NewRootCmd(BuildInfo{Version: "test"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{
		"--home", m.Root(),
		"close", "1", "--dry-run",
		"--chain-id", "wrong-chain",
	})
	err := Execute(root)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("workflow chain mismatch error = %v", err)
	}
	if strings.Contains(out.String(), "Workflow: close") {
		t.Fatalf("chain mismatch printed a workflow plan:\n%s", out.String())
	}
}
