package marshal

import (
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

func TestAddressOrDefaultDefersNamedAccountLookup(t *testing.T) {
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

	loads := 0
	deferred := aktkeyring.NewDeferred(kr.Backend(), func() (sdkkeyring.Keyring, error) {
		loads++
		return kr, nil
	})
	cctx := sdkclient.Context{}.WithFrom("alice").WithKeyring(deferred)

	explicit := "akash1explicit"
	got, err := AddressOrDefault(cctx, explicit)
	require.NoError(t, err)
	require.Equal(t, explicit, got)
	require.Zero(t, loads)

	got, err = AddressOrDefault(cctx, "")
	require.NoError(t, err)
	require.Equal(t, address.String(), got)
	require.Equal(t, 1, loads)
}
