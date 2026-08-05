package cli

import (
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

type keyLookupCounter struct {
	sdkkeyring.Keyring
	lookups int
}

func (k *keyLookupCounter) Key(uid string) (*sdkkeyring.Record, error) {
	k.lookups++
	return k.Keyring.Key(uid)
}

func TestDefaultOwnerResolutionTouchesKeyringOnlyForShorthand(t *testing.T) {
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	record, _, err := kr.NewMnemonic(
		"alice",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	require.NoError(t, err)
	address, err := record.GetAddress()
	require.NoError(t, err)

	counted := &keyLookupCounter{Keyring: kr}
	cctx := sdkclient.Context{}.
		WithFrom("alice").
		WithKeyring(counted)

	owner, err := defaultOwnerForQueryArg(cctx, []string{address.String()})
	require.NoError(t, err)
	require.Empty(t, owner)
	require.Zero(t, counted.lookups, "an explicit owner must not access the keyring")

	owner, err = defaultOwnerForQueryArg(cctx, []string{"not-an-id"})
	require.NoError(t, err)
	require.Empty(t, owner)
	require.Zero(t, counted.lookups, "invalid input must not access the keyring")

	owner, err = defaultOwnerForQueryArg(cctx, []string{"12345"})
	require.NoError(t, err)
	require.Equal(t, address.String(), owner)
	require.Equal(t, 1, counted.lookups, "numeric shorthand must resolve the named default account")
}
