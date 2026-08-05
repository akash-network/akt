package store

import (
	"context"
	"sort"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/capability"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktctx "pkg.akt.dev/akt/internal/context"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

// syncAddrs returns two distinct, valid account addresses. They are derived
// from a throwaway keyring rather than hard-coded so the bech32 prefix always
// matches whatever the SDK is configured with.
func syncAddrs(t *testing.T) (string, string) {
	t.Helper()

	_, addrs := testKeyringWith(t, "addr-a", "addr-b")

	return addrs["addr-a"], addrs["addr-b"]
}

// testKeyringWith builds an in-memory keyring holding the named keys and
// returns it with each name's resolved address.
func testKeyringWith(t *testing.T, names ...string) (sdkkeyring.Keyring, map[string]string) {
	t.Helper()

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	addrs := make(map[string]string, len(names))

	for i, name := range names {
		rec, _, err := kr.NewMnemonic(
			name,
			sdkkeyring.English,
			"m/44'/118'/0'/0/"+string(rune('0'+i)),
			"",
			aktkeyring.DefaultAlgo(),
		)
		require.NoError(t, err)

		addr, err := rec.GetAddress()
		require.NoError(t, err)
		addrs[name] = addr.String()
	}

	return kr, addrs
}

// TestResolveTrackedAccountsDefaultsToTheDefaultAccount covers the documented
// default of SPEC §6.7: an unset tracked-accounts syncs the context's default
// account, which the client context has already resolved to an address.
func TestResolveTrackedAccountsDefaultsToTheDefaultAccount(t *testing.T) {
	addrA, _ := syncAddrs(t)

	addr, err := sdk.AccAddressFromBech32(addrA)
	require.NoError(t, err)

	cctx := sdkclient.Context{}.WithFromAddress(addr)
	rc := &aktctx.Context{Name: "prod", DefaultAccount: "alice"}

	got, err := resolveTrackedAccounts(rc, "", cctx)
	require.NoError(t, err)
	require.Equal(t, []string{addrA}, got)
}

// TestResolveTrackedAccountsUsesTheConfiguredList covers the multi-account
// case, including deduplication and a stable order.
func TestResolveTrackedAccountsUsesTheConfiguredList(t *testing.T) {
	addrA, addrB := syncAddrs(t)

	rc := &aktctx.Context{
		Name:            "prod",
		TrackedAccounts: []string{addrB, addrA, addrB},
	}

	got, err := resolveTrackedAccounts(rc, "", sdkclient.Context{})
	require.NoError(t, err)
	require.Len(t, got, 2, "a repeated account must not be reconciled twice")
	require.Contains(t, got, addrA)
	require.Contains(t, got, addrB)
	require.True(t, sort.StringsAreSorted(got), "owners must be in a stable order: %v", got)
}

// TestResolveTrackedAccountsResolvesKeyNames covers the "name or address"
// contract: a keyring key name is looked up, not stored verbatim as an owner.
func TestResolveTrackedAccountsResolvesKeyNames(t *testing.T) {
	kr, addrs := testKeyringWith(t, "alice")
	cctx := sdkclient.Context{}.WithKeyring(kr)

	rc := &aktctx.Context{Name: "prod", TrackedAccounts: []string{"alice"}}

	got, err := resolveTrackedAccounts(rc, "", cctx)
	require.NoError(t, err)
	require.Equal(t, []string{addrs["alice"]}, got)
}

// TestResolveTrackedAccountsExpandsStar covers tracked-accounts: ["*"].
func TestResolveTrackedAccountsExpandsStar(t *testing.T) {
	kr, addrs := testKeyringWith(t, "alice", "bob")
	cctx := sdkclient.Context{}.WithKeyring(kr)

	rc := &aktctx.Context{Name: "prod", TrackedAccounts: []string{trackAllAccounts}}

	got, err := resolveTrackedAccounts(rc, "", cctx)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Contains(t, got, addrs["alice"])
	require.Contains(t, got, addrs["bob"])
}

// TestResolveTrackedAccountsOverrideWins covers `akt store sync <account>`.
func TestResolveTrackedAccountsOverrideWins(t *testing.T) {
	addrA, addrB := syncAddrs(t)

	rc := &aktctx.Context{Name: "prod", TrackedAccounts: []string{addrA}}

	got, err := resolveTrackedAccounts(rc, addrB, sdkclient.Context{})
	require.NoError(t, err)
	require.Equal(t, []string{addrB}, got)
}

// TestResolveTrackedAccountsRejectsUnresolvableEntries pins the fail-loud
// rule: an account that cannot be resolved must not be quietly dropped from a
// sync that then reports success.
func TestResolveTrackedAccountsRejectsUnresolvableEntries(t *testing.T) {
	addrA, _ := syncAddrs(t)

	cases := []struct {
		name     string
		rc       *aktctx.Context
		override string
		cctx     sdkclient.Context
		wants    string
	}{
		{
			name:  "no account at all",
			rc:    &aktctx.Context{Name: "prod"},
			wants: "no account to sync",
		},
		{
			name:  "nil context and no client identity",
			rc:    nil,
			wants: "no account to sync",
		},
		{
			name:  "name with no keyring",
			rc:    &aktctx.Context{Name: "prod", TrackedAccounts: []string{"alice"}},
			wants: "no keyring is configured",
		},
		{
			name:  "star with no keyring",
			rc:    &aktctx.Context{Name: "prod", TrackedAccounts: []string{trackAllAccounts}},
			wants: "no keyring to enumerate",
		},
		{
			name:  "star mixed with another entry",
			rc:    &aktctx.Context{Name: "prod", TrackedAccounts: []string{addrA, trackAllAccounts}},
			wants: "cannot be combined",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveTrackedAccounts(tc.rc, tc.override, tc.cctx)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wants)
		})
	}
}

// TestResolveTrackedAccountsRejectsUnknownKeyNames covers the keyring-miss
// path with a real keyring present.
func TestResolveTrackedAccountsRejectsUnknownKeyNames(t *testing.T) {
	kr, _ := testKeyringWith(t, "alice")
	cctx := sdkclient.Context{}.WithKeyring(kr)

	rc := &aktctx.Context{Name: "prod", TrackedAccounts: []string{"carol"}}

	_, err := resolveTrackedAccounts(rc, "", cctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "carol")
	require.Contains(t, err.Error(), "neither an address nor a key")
}

// TestSyncCommandSurface pins the command contract: `store sync` takes the
// account positionally (SPEC §3.8, no required flags) and declares the
// chain-query capability so a console-only context is told what it needs
// instead of failing inside the transport (SPEC §2.10).
func TestSyncCommandSurface(t *testing.T) {
	addrA, addrB := syncAddrs(t)

	cmd := syncCmd(
		func() string { return t.TempDir() },
		func() string { return "prod" },
		func() *aktctx.Manager { return nil },
	)

	require.Equal(t, "sync [account]", cmd.Use)
	require.Equal(t, string(capability.ChainQuery), cmd.Annotations[capability.AnnotationKey])

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		require.NotContains(t, f.Annotations, cobra.BashCompOneRequiredFlag,
			"store sync must not require a flag")
	})

	// Two positionals is a usage error, not a silently ignored argument.
	require.Error(t, cmd.Args(cmd, []string{addrA, addrB}))
	require.NoError(t, cmd.Args(cmd, []string{addrA}))
	require.NoError(t, cmd.Args(cmd, nil))
}

// TestSyncCommandRequiresAChainClient covers the guard that turns a missing
// transport into a clear message rather than a nil-client panic.
func TestSyncCommandRequiresAChainClient(t *testing.T) {
	cmd := syncCmd(
		func() string { return t.TempDir() },
		func() string { return "prod" },
		func() *aktctx.Manager { return nil },
	)

	cmd.SetContext(context.Background())

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "chain connection"),
		"error should say a chain connection is needed, got %v", err)
}
