package marshal

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"

	aktclient "pkg.akt.dev/akt/internal/client"
)

// AddressOrDefault returns explicit unchanged, or resolves the account carried
// by cctx. A named account opens an on-demand keyring only on this fallback.
func AddressOrDefault(cctx sdkclient.Context, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	addr, err := aktclient.ResolveAccountAddress(cctx)
	if err != nil {
		return "", err
	}
	if addr.Empty() {
		return "", nil
	}

	return addr.String(), nil
}
