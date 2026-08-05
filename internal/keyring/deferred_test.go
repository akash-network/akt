package keyring

import (
	"testing"

	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/stretchr/testify/require"

	aktcodec "pkg.akt.dev/akt/internal/codec"
)

func TestDeferredKeyringDoesNotLoadForBackendInspection(t *testing.T) {
	loads := 0
	deferred := NewDeferred(sdkkeyring.BackendOS, func() (sdkkeyring.Keyring, error) {
		loads++
		return NewInMemory(aktcodec.MakeEncodingConfig().Codec), nil
	})

	require.Equal(t, sdkkeyring.BackendOS, deferred.Backend())
	supported, ledger := deferred.SupportedAlgorithms()
	require.NotEmpty(t, supported)
	require.NotEmpty(t, ledger)
	require.Zero(t, loads)

	_, err := deferred.List()
	require.NoError(t, err)
	require.Equal(t, 1, loads)

	_, err = deferred.List()
	require.NoError(t, err)
	require.Equal(t, 1, loads, "the loaded keyring must be cached")
}
