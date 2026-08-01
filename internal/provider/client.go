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

// NewPublicGatewayClient creates a provider gateway REST client without
// attaching wallet authentication. It is only suitable for public endpoints
// such as provider status.
func NewPublicGatewayClient(
	ctx context.Context,
	addr sdk.AccAddress,
	providerURL string,
) (rest.Client, error) {
	return rest.NewClient(ctx, addr, rest.WithProviderURL(providerURL))
}

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
	if err := ValidateGatewayAuthentication(addr, authType, kr); err != nil {
		return nil, err
	}

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
	}

	return rest.NewClient(ctx, addr, opts...)
}

// ValidateGatewayAuthentication checks every local prerequisite before a
// protected provider command performs URL discovery or gateway network work.
func ValidateGatewayAuthentication(
	addr sdk.AccAddress,
	authType string,
	kr sdkkeyring.Keyring,
) error {
	switch authType {
	case "jwt", "", "mtls":
	default:
		return fmt.Errorf("unsupported auth type %q (expected \"jwt\" or \"mtls\")", authType)
	}
	if addr.Empty() {
		return fmt.Errorf("provider gateway authentication requires a configured default account")
	}
	if kr == nil {
		return fmt.Errorf("provider gateway authentication requires a keyring")
	}
	if _, err := kr.KeyByAddress(addr); err != nil {
		return fmt.Errorf("provider gateway authentication selected account %s is not present in the configured keyring; import that key or change the context default account", addr)
	}
	return nil
}

// loadMTLSCert loads and validates the mTLS certificate for the current account.
func loadMTLSCert(ctx context.Context, cctx sdkclient.Context) (tls.Certificate, error) {
	return atls.LoadAndQueryCertificateForAccount(ctx, cctx, nil)
}
