package client

import (
	"errors"
	"strings"
	"testing"

	keyring "github.com/99designs/keyring"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func TestBuildClientContextResolvesConfiguredIdentityAndTransport(t *testing.T) {
	enc := aktcodec.MakeEncodingConfig()
	keyring := aktkeyring.NewInMemory(enc.Codec)
	record, _, err := keyring.NewMnemonic("alice", sdkkeyring.English, "m/44'/118'/0'/0/0", "", aktkeyring.DefaultAlgo())
	if err != nil {
		t.Fatal(err)
	}
	wantAddress, err := record.GetAddress()
	if err != nil {
		t.Fatal(err)
	}
	rc := &aktctx.Context{
		Root:           t.TempDir(),
		DefaultAccount: "alice",
		Keyring:        aktctx.Keyring{Name: "default", Backend: sdkkeyring.BackendMemory},
		Network: aktctx.Network{
			ChainID:   "local-1",
			Endpoints: aktctx.Endpoints{RPC: []string{"https://rpc.example.test"}},
		},
	}

	cctx := BuildClientContext(rc, keyring, enc, "")
	if cctx.ChainID != "local-1" || cctx.NodeURI != "https://rpc.example.test:443" {
		t.Errorf("chain transport = %q/%q", cctx.ChainID, cctx.NodeURI)
	}
	if cctx.FromName != "alice" || !cctx.GetFromAddress().Equals(wantAddress) {
		t.Errorf("resolved identity = name %q address %s, want alice/%s", cctx.FromName, cctx.GetFromAddress(), wantAddress)
	}
	if cctx.BroadcastMode != "sync" || cctx.OutputFormat != "json" {
		t.Errorf("defaults = broadcast %q output %q", cctx.BroadcastMode, cctx.OutputFormat)
	}
	if cctx.Codec == nil || cctx.InterfaceRegistry == nil || cctx.TxConfig == nil || cctx.LegacyAmino == nil || cctx.AccountRetriever == nil {
		t.Fatal("client context omitted required codec or account dependencies")
	}
}

func TestBuildClientContextIdentityBoundaries(t *testing.T) {
	enc := aktcodec.MakeEncodingConfig()
	address := sdk.AccAddress{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	rc := &aktctx.Context{Root: t.TempDir(), DefaultAccount: "missing-name"}

	withoutKeyring := BuildClientContext(rc, nil, enc, "")
	if withoutKeyring.From != "missing-name" || withoutKeyring.FromName != "" || !withoutKeyring.GetFromAddress().Empty() {
		t.Errorf("unresolved named account = From %q name %q address %s", withoutKeyring.From, withoutKeyring.FromName, withoutKeyring.GetFromAddress())
	}

	addressOverride := BuildClientContext(rc, nil, enc, address.String())
	if !addressOverride.GetFromAddress().Equals(address) {
		t.Errorf("address override = %s, want %s", addressOverride.GetFromAddress(), address)
	}
	if addressOverride.NodeURI != "" {
		t.Errorf("context without endpoints selected node %q", addressOverride.NodeURI)
	}

	missingKey := BuildClientContext(rc, aktkeyring.NewInMemory(enc.Codec), enc, "other-name")
	if missingKey.From != "other-name" || missingKey.FromName != "" || !missingKey.GetFromAddress().Empty() {
		t.Errorf("missing named override was invented as resolved: %+v", missingKey)
	}
}

func TestBuildClientContextToleratesMalformedKeyringRecords(t *testing.T) {
	enc := aktcodec.MakeEncodingConfig()
	rc := &aktctx.Context{
		Root:    t.TempDir(),
		Network: aktctx.Network{ChainID: "chain-1"},
	}

	tests := []struct {
		name   string
		record *sdkkeyring.Record
	}{
		{name: "nil record"},
		{name: "record without public key", record: &sdkkeyring.Record{Name: "alice"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cctx sdkclient.Context
			require.NotPanics(t, func() {
				cctx = BuildClientContext(rc, accountLookupStub{record: tc.record}, enc, "alice")
			})
			require.Equal(t, "alice", cctx.From)
			require.Empty(t, cctx.FromName)
			require.True(t, cctx.GetFromAddress().Empty())
			require.Equal(t, "chain-1", cctx.ChainID)
			require.Equal(t, "sync", cctx.BroadcastMode)
		})
	}
}

func TestResolveAccountAddressBoundaries(t *testing.T) {
	address := sdk.AccAddress{20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	preResolved := sdkclient.Context{}.WithFromAddress(address)
	got, err := ResolveAccountAddress(preResolved)
	if err != nil || !got.Equals(address) {
		t.Fatalf("pre-resolved address = %s, %v", got, err)
	}

	got, err = ResolveAccountAddress(sdkclient.Context{})
	if err != nil || got != nil {
		t.Fatalf("empty identity = %s, %v, want nil/nil", got, err)
	}

	got, err = ResolveAccountAddress(sdkclient.Context{}.WithFrom("  " + address.String() + "  "))
	if err != nil || !got.Equals(address) {
		t.Fatalf("bech32 identity = %s, %v", got, err)
	}

	if _, err := ResolveAccountAddress(sdkclient.Context{}.WithFrom("alice")); err == nil || !strings.Contains(err.Error(), "keyring is unavailable") {
		t.Fatalf("nil keyring error = %v", err)
	}

	lookupErr := errors.New("locked")
	if _, err := ResolveAccountAddress(sdkclient.Context{}.WithFrom("alice").WithKeyring(accountLookupStub{err: lookupErr})); !errors.Is(err, lookupErr) {
		t.Fatalf("lookup error = %v, want locked", err)
	}
	if _, err := ResolveAccountAddress(sdkclient.Context{}.WithFrom("alice").WithKeyring(accountLookupStub{})); err == nil || !strings.Contains(err.Error(), "no record") {
		t.Fatalf("nil record error = %v", err)
	}

	badRecord := &sdkkeyring.Record{Name: "alice"}
	if _, err := ResolveAccountAddress(sdkclient.Context{}.WithFrom("alice").WithKeyring(accountLookupStub{record: badRecord})); err == nil || !strings.Contains(err.Error(), "address") {
		t.Fatalf("record address error = %v", err)
	}

	enc := aktcodec.MakeEncodingConfig()
	keyring := aktkeyring.NewInMemory(enc.Codec)
	record, _, err := keyring.NewMnemonic("alice", sdkkeyring.English, "m/44'/118'/0'/0/0", "", aktkeyring.DefaultAlgo())
	if err != nil {
		t.Fatal(err)
	}
	want, err := record.GetAddress()
	if err != nil {
		t.Fatal(err)
	}
	got, err = ResolveAccountAddress(sdkclient.Context{}.WithFrom("alice").WithKeyring(keyring))
	if err != nil || !got.Equals(want) {
		t.Fatalf("named identity = %s, %v, want %s", got, err, want)
	}
}

func TestInitClientContextPublishesSDKAndRPCContexts(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	rc := &aktctx.Context{
		Root:    t.TempDir(),
		Network: aktctx.Network{ChainID: "chain-1", Endpoints: aktctx.Endpoints{RPC: []string{"http://127.0.0.1"}}},
	}
	if err := InitClientContext(cmd, rc, nil, aktcodec.MakeEncodingConfig(), ""); err != nil {
		t.Fatal(err)
	}
	cctx := sdkclient.GetClientContextFromCmd(cmd)
	if cctx.ChainID != "chain-1" || cctx.NodeURI != "http://127.0.0.1:80" {
		t.Errorf("SDK context = chain %q node %q", cctx.ChainID, cctx.NodeURI)
	}
}

func TestInitClientContextRejectsMalformedRequiredIdentity(t *testing.T) {
	enc := aktcodec.MakeEncodingConfig()
	rc := &aktctx.Context{Root: t.TempDir()}

	tests := []struct {
		name        string
		record      *sdkkeyring.Record
		wantMessage string
	}{
		{name: "nil record", wantMessage: "no record"},
		{
			name:        "record without public key",
			record:      &sdkkeyring.Record{Name: "alice"},
			wantMessage: "no public key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			var err error
			require.NotPanics(t, func() {
				err = InitClientContext(cmd, rc, accountLookupStub{record: tc.record}, enc, "alice")
			})
			require.Error(t, err)
			require.ErrorContains(t, err, `resolve account "alice"`)
			require.ErrorContains(t, err, tc.wantMessage)
		})
	}
}

func TestMustResolveAndInitSelectionBoundaries(t *testing.T) {
	enc := aktcodec.MakeEncodingConfig()

	t.Run("no configured context", func(t *testing.T) {
		root := t.TempDir()
		manager, err := aktctx.NewManager(root)
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := MustResolveAndInit(&cobra.Command{Use: "test"}, manager, aktkeyring.NewManager(root, nil, enc.Codec), enc, "", "", LocalIdentityNone)
		if err != nil || resolved {
			t.Fatalf("no-context result = %v, %v", resolved, err)
		}
	})

	t.Run("multiple contexts require selection", func(t *testing.T) {
		manager, keyrings, root := clientContextFixture(t, 2)
		resolved, err := MustResolveAndInit(&cobra.Command{Use: "test"}, manager, keyrings, enc, "", "", LocalIdentityNone)
		if err != nil || resolved {
			t.Fatalf("multiple-context result = %v, %v (root %s)", resolved, err, root)
		}
	})

	t.Run("one context auto-selects", func(t *testing.T) {
		manager, keyrings, _ := clientContextFixture(t, 1)
		cmd := &cobra.Command{Use: "test"}
		resolved, err := MustResolveAndInit(cmd, manager, keyrings, enc, "", "", LocalIdentityNone)
		if err != nil || !resolved {
			t.Fatalf("single-context result = %v, %v", resolved, err)
		}
		if got := sdkclient.GetClientContextFromCmd(cmd).ChainID; got != "chain-1" {
			t.Errorf("auto-selected chain = %q, want chain-1", got)
		}
	})

	t.Run("explicit unknown context fails", func(t *testing.T) {
		manager, keyrings, _ := clientContextFixture(t, 1)
		resolved, err := MustResolveAndInit(&cobra.Command{Use: "test"}, manager, keyrings, enc, "missing", "", LocalIdentityNone)
		if err == nil || resolved || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("unknown-context result = %v, %v", resolved, err)
		}
	})

	t.Run("required identity reports missing keyring", func(t *testing.T) {
		manager, _, root := clientContextFixture(t, 1)
		emptyKeyrings := aktkeyring.NewManager(root, nil, enc.Codec)
		resolved, err := MustResolveAndInit(&cobra.Command{Use: "test"}, manager, emptyKeyrings, enc, "context-1", "", LocalIdentityRequired)
		if err == nil || resolved || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("missing-keyring result = %v, %v", resolved, err)
		}
	})

	t.Run("required identity rejects corrupt keyring record", func(t *testing.T) {
		manager, keyrings, _ := clientContextFixture(t, 1)
		kr, err := keyrings.Get("keyring-1")
		require.NoError(t, err)
		krWithDB, ok := kr.(sdkkeyring.KeyringWithDB)
		require.True(t, ok)

		record, err := enc.Codec.Marshal(&sdkkeyring.Record{Name: "alice"})
		require.NoError(t, err)
		require.NoError(t, krWithDB.DB().Set(keyring.Item{Key: "alice.info", Data: record}))

		var initErr error
		require.NotPanics(t, func() {
			_, initErr = MustResolveAndInit(
				&cobra.Command{Use: "test"}, manager, keyrings, enc,
				"context-1", "alice", LocalIdentityRequired,
			)
		})
		require.ErrorContains(t, initErr, `resolve account "alice"`)
		require.ErrorContains(t, initErr, "no public key")
	})
}

func clientContextFixture(t *testing.T, count int) (*aktctx.Manager, *aktkeyring.Manager, string) {
	t.Helper()
	root := t.TempDir()
	manager, err := aktctx.NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	keyringConfigs := make([]aktctx.Keyring, 0, count)
	for i := 1; i <= count; i++ {
		name := "context-" + string(rune('0'+i))
		networkName := "network-" + string(rune('0'+i))
		keyringName := "keyring-" + string(rune('0'+i))
		if err := manager.CreateNetwork(aktctx.Network{Name: networkName, ChainID: "chain-" + string(rune('0'+i))}); err != nil {
			t.Fatal(err)
		}
		config := aktctx.Keyring{Name: keyringName, Backend: sdkkeyring.BackendTest}
		if err := manager.CreateKeyring(config); err != nil {
			t.Fatal(err)
		}
		if err := manager.CreateContext(aktctx.Context{Name: name, Network: aktctx.Network{Name: networkName}, Keyring: aktctx.Keyring{Name: keyringName}}); err != nil {
			t.Fatal(err)
		}
		keyringConfigs = append(keyringConfigs, config)
	}
	return manager, aktkeyring.NewManager(root, keyringConfigs, aktcodec.MakeEncodingConfig().Codec), root
}

type accountLookupStub struct {
	sdkkeyring.Keyring
	record *sdkkeyring.Record
	err    error
}

func (stub accountLookupStub) Key(string) (*sdkkeyring.Record, error) {
	return stub.record, stub.err
}
