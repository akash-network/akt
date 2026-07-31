package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"

	"pkg.akt.dev/akt/internal/actionlog"
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
		w.WriteHeader(http.StatusOK)
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
