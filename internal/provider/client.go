// Package provider provides a thin wrapper around the chain-sdk's provider
// REST client, integrating it with the akt context system's auth configuration.
package provider

import (
	"context"
	"crypto/tls"
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"

	rest "pkg.akt.dev/go/provider/client"
	ajwt "pkg.akt.dev/go/util/jwt"
	atls "pkg.akt.dev/go/util/tls"
)

// NewGatewayClient creates a provider gateway REST client configured for the
// given auth method. authType is "jwt" (default) or "mtls".
func NewGatewayClient(
	ctx context.Context,
	cctx sdkclient.Context,
	addr sdk.AccAddress,
	providerURL string,
	authType string,
	kr sdkkeyring.Keyring,
) (rest.Client, error) {
	opts := []rest.ClientOption{
		rest.WithProviderURL(providerURL),
	}

	switch authType {
	case "mtls":
		cert, err := loadMTLSCert(ctx, cctx)
		if err != nil {
			return nil, fmt.Errorf("load mTLS certificate: %w", err)
		}
		opts = append(opts, rest.WithAuthCerts([]tls.Certificate{cert}))
	case "jwt", "":
		signer := ajwt.NewSigner(kr, addr)
		opts = append(opts, rest.WithAuthJWTSigner(signer))
	default:
		return nil, fmt.Errorf("unsupported auth type %q (expected \"jwt\" or \"mtls\")", authType)
	}

	return rest.NewClient(ctx, addr, opts...)
}

// loadMTLSCert loads and validates the mTLS certificate for the current account.
func loadMTLSCert(ctx context.Context, cctx sdkclient.Context) (tls.Certificate, error) {
	return atls.LoadAndQueryCertificateForAccount(ctx, cctx, nil)
}
