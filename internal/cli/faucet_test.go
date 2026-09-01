package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	flagdefs "pkg.akt.dev/akt/internal/flags"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func TestClassifyFaucetState(t *testing.T) {
	cases := []struct {
		name      string
		rc        *aktctx.Context
		isMainnet bool
		want      faucetState
	}{
		{
			name:      "console managed wins even without a network",
			rc:        &aktctx.Context{Name: "c", AuthMethod: aktctx.AuthMethodConsoleAPI},
			isMainnet: false,
			want:      faucetStateConsoleManaged,
		},
		{
			name:      "no network attached",
			rc:        &aktctx.Context{Name: "c", AuthMethod: aktctx.AuthMethodKeyring},
			isMainnet: false,
			want:      faucetStateNoNetwork,
		},
		{
			name:      "mainnet with no faucet",
			rc:        &aktctx.Context{Name: "c", AuthMethod: aktctx.AuthMethodKeyring, Network: aktctx.Network{Name: "mainnet", ChainID: "akashnet-2"}},
			isMainnet: true,
			want:      faucetStateMainnetNoFaucet,
		},
		{
			name:      "test network with no faucet configured",
			rc:        &aktctx.Context{Name: "c", AuthMethod: aktctx.AuthMethodKeyring, Network: aktctx.Network{Name: "testnet", ChainID: "testnet-oracle"}},
			isMainnet: false,
			want:      faucetStateNoFaucetConfigured,
		},
		{
			name:      "ready",
			rc:        &aktctx.Context{Name: "c", AuthMethod: aktctx.AuthMethodKeyring, Network: aktctx.Network{Name: "sandbox", ChainID: "sandbox-2", Faucet: "http://faucet.sandbox-2.aksh.pw/"}},
			isMainnet: false,
			want:      faucetStateReady,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyFaucetState(tc.rc, tc.isMainnet); got != tc.want {
				t.Errorf("classifyFaucetState = %v, want %v", got, tc.want)
			}
		})
	}
}

// A network-less context is normally unreachable through CreateContext, which
// validates the named network exists, so faucetStateNoNetwork's error branch
// is exercised directly against the pure functions instead.
func TestFaucetStateErrorNoNetwork(t *testing.T) {
	rc := &aktctx.Context{Name: "noNet", Network: aktctx.Network{Name: ""}}
	if got := classifyFaucetState(rc, false); got != faucetStateNoNetwork {
		t.Fatalf("classifyFaucetState = %v, want faucetStateNoNetwork", got)
	}

	err := faucetStateError(rc, faucetStateNoNetwork)
	if err == nil || !strings.Contains(err.Error(), "no network attached") {
		t.Fatalf("faucetStateError = %v, want mention of no network attached", err)
	}
}

func TestIsMainnetNetwork(t *testing.T) {
	cases := []struct {
		name           string
		net            aktctx.Network
		mainnetChainID string
		want           bool
	}{
		{"by name", aktctx.Network{Name: "mainnet-fork", ChainID: "custom-1"}, "", true},
		{"by well-known chain-id", aktctx.Network{Name: "prod", ChainID: "akashnet-2"}, "", true},
		{"by resolved mainnet chain-id", aktctx.Network{Name: "prod", ChainID: "custom-mainnet-1"}, "custom-mainnet-1", true},
		{"sandbox is not mainnet", aktctx.Network{Name: "sandbox", ChainID: "sandbox-2"}, "akashnet-2", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMainnetNetwork(tc.net, tc.mainnetChainID); got != tc.want {
				t.Errorf("isMainnetNetwork(%+v, %q) = %v, want %v", tc.net, tc.mainnetChainID, got, tc.want)
			}
		})
	}
}

// roundTripFunc adapts a plain function to http.RoundTripper, letting a test
// force client.Do to fail without a real transport.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestSubmitFaucetRequestInvalidURL(t *testing.T) {
	_, err := submitFaucetRequest(context.Background(), &http.Client{}, "http://%zz", "akash1abc")
	if err == nil || !strings.Contains(err.Error(), "invalid faucet URL") {
		t.Fatalf("err = %v, want mention of invalid faucet URL", err)
	}
}

func TestSubmitFaucetRequestNilContextFailsToBuildRequest(t *testing.T) {
	//nolint:staticcheck // exercises the nil-context guard in submitFaucetRequest
	_, err := submitFaucetRequest(nil, &http.Client{}, "http://example.test", "akash1abc")
	if err == nil || !strings.Contains(err.Error(), "request to faucet") {
		t.Fatalf("err = %v, want mention of request to faucet", err)
	}
}

func TestSubmitFaucetRequestTransportError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}

	_, err := submitFaucetRequest(context.Background(), client, "http://example.test", "akash1abc")
	if err == nil || !strings.Contains(err.Error(), "request to faucet") {
		t.Fatalf("err = %v, want mention of request to faucet", err)
	}
}

// newFaucetCmd mirrors the network package's test helpers: a command built
// directly, outside root.go's persistent-flag wiring, so --output must be
// registered by hand the way the root command normally does it.
func newFaucetCmd(m *aktctx.Manager) *cobra.Command {
	cmd := faucetCmd(func() *aktctx.Manager { return m }, func() string { return "akashnet-2" })
	cmd.Flags().String(flagdefs.FlagOutput, "pretty", "Output format")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(nil)

	return cmd
}

// TestFaucetCommandNilManager exercises faucetCmd directly rather than through
// newFaucetCmd, since the mgr closure returning nil is exactly the case
// newFaucetCmd's non-nil *aktctx.Manager parameter cannot construct.
func TestFaucetCommandNilManager(t *testing.T) {
	cmd := faucetCmd(func() *aktctx.Manager { return nil }, func() string { return "" })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no akt configuration found") {
		t.Fatalf("err = %v, want mention of no akt configuration found", err)
	}
}

func TestFaucetCommandResolveErrorWithNoCurrentContext(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	cmd := faucetCmd(func() *aktctx.Manager { return m }, func() string { return "" })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err = cmd.Execute()
	want := "no context specified and no current-context set in config"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want mention of %q", err, want)
	}
}

func TestFaucetCommandConsoleManagedContextErrors(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "console", AuthMethod: aktctx.AuthMethodConsoleAPI}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("console"); err != nil {
		t.Fatal(err)
	}

	err = newFaucetCmd(m).Execute()
	want := `the active context "console" uses a Console-managed wallet, which has no chain faucet; check the balance with 'akt console wallet balance' or add funds at https://console.akash.network`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestFaucetCommandMainnetHasNoFaucet(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateNetworkFromTemplate("mainnet", "mainnet"); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("prod"); err != nil {
		t.Fatal(err)
	}

	err = newFaucetCmd(m).Execute()
	want := `mainnet (akashnet-2) is a live network and has no faucet; acquire real AKT by transfer, or switch to a test network with 'akt context use sandbox'`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestFaucetCommandTestNetworkWithoutFaucetConfigured(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateNetwork(aktctx.Network{
		Name:      "custom-test",
		ChainID:   "custom-1",
		Endpoints: aktctx.Endpoints{RPC: []string{"http://localhost:26657"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "dev", Network: aktctx.Network{Name: "custom-test"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("dev"); err != nil {
		t.Fatal(err)
	}

	err = newFaucetCmd(m).Execute()
	want := `network "custom-test" (custom-1) has no faucet configured; set one with 'akt context network edit custom-test --faucet <url>'`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestFaucetCommandSuccessWithResolvableAddress(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateNetworkFromTemplate("sandbox", "sandbox"); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "sbx", Network: aktctx.Network{Name: "sandbox"}, DefaultAccount: "alice"}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("sbx"); err != nil {
		t.Fatal(err)
	}

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	record, _, err := kr.NewMnemonic("alice", sdkkeyring.English, "m/44'/118'/0'/0/0", "", aktkeyring.DefaultAlgo())
	if err != nil {
		t.Fatal(err)
	}
	address, err := record.GetAddress()
	if err != nil {
		t.Fatal(err)
	}

	cctx := sdkclient.Context{}.WithFrom("alice").WithKeyring(kr)

	cmd := newFaucetCmd(m)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetContext(context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, address.String()) {
		t.Errorf("output missing resolved address %q: %q", address.String(), out)
	}
	if strings.Contains(out, faucetAddressPlaceholder) {
		t.Errorf("output should not fall back to the placeholder when an address resolves: %q", out)
	}
	if !strings.Contains(out, "http://faucet.sandbox-2.aksh.pw/") {
		t.Errorf("output missing faucet URL: %q", out)
	}
	if !strings.Contains(out, "sandbox (sandbox-2)") {
		t.Errorf("output missing network/chain-id line: %q", out)
	}
}

func TestFaucetCommandSuccessWithoutResolvableAddressDoesNotFail(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateNetworkFromTemplate("sandbox", "sandbox"); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "sbx", Network: aktctx.Network{Name: "sandbox"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("sbx"); err != nil {
		t.Fatal(err)
	}

	cmd := newFaucetCmd(m)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, faucetAddressPlaceholder) {
		t.Errorf("output should fall back to the placeholder, got %q", out)
	}
	if !strings.Contains(out, "akt faucet --send") {
		t.Errorf("output should hint at --send, got %q", out)
	}
}

func newSandboxKeyringContext(t *testing.T, faucetURL string) (*aktctx.Manager, sdkclient.Context, string) {
	t.Helper()

	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateNetwork(aktctx.Network{
		Name:      "sandbox",
		ChainID:   "sandbox-2",
		Faucet:    faucetURL,
		Endpoints: aktctx.Endpoints{RPC: []string{"http://localhost:26657"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "sbx", Network: aktctx.Network{Name: "sandbox"}, DefaultAccount: "alice"}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("sbx"); err != nil {
		t.Fatal(err)
	}

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	record, _, err := kr.NewMnemonic("alice", sdkkeyring.English, "m/44'/118'/0'/0/0", "", aktkeyring.DefaultAlgo())
	if err != nil {
		t.Fatal(err)
	}
	address, err := record.GetAddress()
	if err != nil {
		t.Fatal(err)
	}

	cctx := sdkclient.Context{}.WithFrom("alice").WithKeyring(kr)
	return m, cctx, address.String()
}

func TestFaucetCommandSendSubmitsRequest(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)

		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/faucet" {
			t.Errorf("path = %s, want /faucet", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("content-type = %q, want form-urlencoded", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.PostFormValue("address"); got == "" {
			t.Error("address form field missing")
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"transactionHash":"ABC123"}`))
	}))
	defer srv.Close()

	m, cctx, address := newSandboxKeyringContext(t, srv.URL)

	cmd := newFaucetCmd(m)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetContext(context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx))
	if err := cmd.Flags().Set("send", "true"); err != nil {
		t.Fatal(err)
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("faucet handler called %d times, want 1", calls)
	}

	out := stdout.String()
	if !strings.Contains(out, address) {
		t.Errorf("output missing resolved address %q: %q", address, out)
	}
	if !strings.Contains(out, "ABC123") {
		t.Errorf("output missing transaction hash: %q", out)
	}
}

func TestFaucetCommandSendNonSuccessStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	m, cctx, _ := newSandboxKeyringContext(t, srv.URL)

	cmd := newFaucetCmd(m)
	cmd.SetContext(context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx))
	if err := cmd.Flags().Set("send", "true"); err != nil {
		t.Fatal(err)
	}

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("error = %v, want mention of status 500", err)
	}
}

func TestFaucetCommandSendWithoutResolvableAddressErrorsWithoutHittingFaucet(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateNetwork(aktctx.Network{
		Name:      "sandbox",
		ChainID:   "sandbox-2",
		Faucet:    srv.URL,
		Endpoints: aktctx.Endpoints{RPC: []string{"http://localhost:26657"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "sbx", Network: aktctx.Network{Name: "sandbox"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("sbx"); err != nil {
		t.Fatal(err)
	}

	cmd := newFaucetCmd(m)
	if err := cmd.Flags().Set("send", "true"); err != nil {
		t.Fatal(err)
	}

	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no default account resolved") {
		t.Fatalf("error = %v, want mention of no default account resolved", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("faucet handler called %d times, want 0", calls)
	}
}

func TestFaucetCommandSendRefusesMainnet(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateNetwork(aktctx.Network{
		Name:      "mainnet",
		ChainID:   "akashnet-2",
		Faucet:    srv.URL,
		Endpoints: aktctx.Endpoints{RPC: []string{"http://localhost:26657"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "prod", Network: aktctx.Network{Name: "mainnet"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("prod"); err != nil {
		t.Fatal(err)
	}

	cmd := newFaucetCmd(m)
	if err := cmd.Flags().Set("send", "true"); err != nil {
		t.Fatal(err)
	}

	err = cmd.Execute()
	want := `refusing to auto-request funds on mainnet "mainnet"; --send is for test networks only`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("faucet handler called %d times, want 0", calls)
	}
}

func TestFaucetCommandSendRecordsActionLogOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"transactionHash":"ABC123"}`))
	}))
	defer srv.Close()

	m, cctx, address := newSandboxKeyringContext(t, srv.URL)

	logger, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logger.Close() }()

	cmd := newFaucetCmd(m)
	ctx := context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx)
	cmd.SetContext(cliutil.WithActionLog(ctx, logger))
	if err := cmd.Flags().Set("send", "true"); err != nil {
		t.Fatal(err)
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	entries, err := logger.Read(actionlog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1: %+v", len(entries), entries)
	}

	e := entries[0]
	if e.Type != actionlog.TypeFaucet {
		t.Errorf("type = %q, want %q", e.Type, actionlog.TypeFaucet)
	}
	if e.Action != "request" {
		t.Errorf("action = %q, want %q", e.Action, "request")
	}
	if e.Account != address {
		t.Errorf("account = %q, want %q", e.Account, address)
	}
	if e.TxHash != "ABC123" {
		t.Errorf("tx_hash = %q, want %q", e.TxHash, "ABC123")
	}
	if e.Status != "success" {
		t.Errorf("status = %q, want %q", e.Status, "success")
	}
}

func TestFaucetCommandSendRecordsActionLogOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	m, cctx, _ := newSandboxKeyringContext(t, srv.URL)

	logger, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logger.Close() }()

	cmd := newFaucetCmd(m)
	ctx := context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx)
	cmd.SetContext(cliutil.WithActionLog(ctx, logger))
	if err := cmd.Flags().Set("send", "true"); err != nil {
		t.Fatal(err)
	}

	if err := cmd.Execute(); err == nil {
		t.Fatal("execute: want an error, got nil")
	}

	entries, err := logger.Read(actionlog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1: %+v", len(entries), entries)
	}

	e := entries[0]
	if e.Status != "failed" {
		t.Errorf("status = %q, want %q", e.Status, "failed")
	}
	if !strings.Contains(e.Error, "status 500") {
		t.Errorf("error = %q, want mention of status 500", e.Error)
	}
}

func TestFaucetCommandDisplayDoesNotRecordActionLog(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateNetworkFromTemplate("sandbox", "sandbox"); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "sbx", Network: aktctx.Network{Name: "sandbox"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("sbx"); err != nil {
		t.Fatal(err)
	}

	logger, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logger.Close() }()

	cmd := newFaucetCmd(m)
	cmd.SetContext(cliutil.WithActionLog(context.Background(), logger))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	entries, err := logger.Read(actionlog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %d, want 0: %+v", len(entries), entries)
	}
}

func TestFaucetCommandDisplayJSONOmitsEmptyStatus(t *testing.T) {
	m, err := aktctx.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.CreateNetworkFromTemplate("sandbox", "sandbox"); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateContext(aktctx.Context{Name: "sbx", Network: aktctx.Network{Name: "sandbox"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.UseContext("sbx"); err != nil {
		t.Fatal(err)
	}

	cmd := newFaucetCmd(m)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	if err := cmd.Flags().Set(flagdefs.FlagOutput, "json"); err != nil {
		t.Fatal(err)
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := stdout.String()
	if strings.Contains(out, `"status"`) {
		t.Errorf("display output should omit the empty status field: %q", out)
	}
	if strings.Contains(out, `"transaction_hash"`) {
		t.Errorf("display output should omit the empty transaction_hash field: %q", out)
	}
	if !strings.Contains(out, `"faucet_url"`) {
		t.Errorf("display output missing faucet_url field: %q", out)
	}
}

func TestFaucetCommandSendJSONIncludesStatusAndHash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"transactionHash":"ABC123"}`))
	}))
	defer srv.Close()

	m, cctx, _ := newSandboxKeyringContext(t, srv.URL)

	cmd := newFaucetCmd(m)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetContext(context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx))
	if err := cmd.Flags().Set(flagdefs.FlagOutput, "json"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("send", "true"); err != nil {
		t.Fatal(err)
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"status": "requested"`) {
		t.Errorf("send output missing status field: %q", out)
	}
	if !strings.Contains(out, `"transaction_hash": "ABC123"`) {
		t.Errorf("send output missing transaction_hash field: %q", out)
	}
}
