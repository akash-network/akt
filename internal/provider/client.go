// Package provider provides a thin wrapper around the chain-sdk's provider
// REST client, integrating it with the akt context system's auth configuration.
package provider

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/golang-jwt/jwt/v5"

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
	client, err := rest.NewClient(ctx, addr, rest.WithProviderURL(providerURL))
	if err != nil {
		return nil, err
	}
	return wrapGatewayClient(gatewayStreamBoundaryClient{Client: client}, providerURL, nil)
}

// NewTokenGatewayClient creates a provider client authenticated by an existing
// bearer token, such as a Console-minted scoped gateway JWT.
func NewTokenGatewayClient(
	ctx context.Context,
	addr sdk.AccAddress,
	providerURL string,
	token string,
) (rest.Client, error) {
	client, err := rest.NewClient(
		ctx,
		addr,
		rest.WithProviderURL(providerURL),
		rest.WithAuthToken(token),
	)
	if err != nil {
		return nil, err
	}
	return wrapGatewayClient(gatewayStreamBoundaryClient{Client: client}, providerURL, func() (string, error) {
		return token, nil
	}, token)
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
	var (
		authorization gatewayAuthorization
		secrets       []string
	)

	switch authType {
	case "mtls":
		cert, err := loadMTLSCert(ctx, cctx)
		if err != nil {
			return nil, fmt.Errorf("load mTLS certificate: %w", err)
		}
		opts = append(opts, rest.WithAuthCerts([]tls.Certificate{cert}))
	case "jwt", "":
		token, err := newFullAccessJWT(kr, addr)
		if err != nil {
			return nil, err
		}
		opts = append(opts, rest.WithAuthToken(token))
		authorization = func() (string, error) {
			return token, nil
		}
		secrets = append(secrets, token)
	}

	client, err := rest.NewClient(ctx, addr, opts...)
	if err != nil {
		return nil, err
	}
	return wrapGatewayClient(
		gatewayStreamBoundaryClient{Client: client},
		providerURL,
		authorization,
		secrets...,
	)
}

// NewScopedGatewayClient creates a provider client whose JWT is restricted to
// one provider, deployment identity, and set of gateway operations. mTLS
// already proves possession of the local private key at each TLS connection;
// the granular claim is therefore only needed for bearer-token authentication.
func NewScopedGatewayClient(
	ctx context.Context,
	cctx sdkclient.Context,
	addr sdk.AccAddress,
	providerURL string,
	providerAddress sdk.AccAddress,
	deployment ajwt.PermissionDeployment,
	authType string,
	kr sdkkeyring.Keyring,
) (rest.Client, error) {
	if err := ValidateGatewayAuthentication(addr, authType, kr); err != nil {
		return nil, err
	}
	if providerAddress.Empty() {
		return nil, fmt.Errorf("provider gateway authentication requires a provider address")
	}

	opts := []rest.ClientOption{rest.WithProviderURL(providerURL)}
	var (
		authorization gatewayAuthorization
		secrets       []string
	)
	switch authType {
	case "mtls":
		cert, err := loadMTLSCert(ctx, cctx)
		if err != nil {
			return nil, fmt.Errorf("load mTLS certificate: %w", err)
		}
		opts = append(opts, rest.WithAuthCerts([]tls.Certificate{cert}))
	case "jwt", "":
		token, err := newScopedJWT(kr, addr, providerAddress, deployment)
		if err != nil {
			return nil, err
		}
		opts = append(opts, rest.WithAuthToken(token))
		authorization = func() (string, error) {
			return token, nil
		}
		secrets = append(secrets, token)
	}

	client, err := rest.NewClient(ctx, addr, opts...)
	if err != nil {
		return nil, err
	}
	return wrapGatewayClient(gatewayStreamBoundaryClient{Client: client}, providerURL, authorization, secrets...)
}

func newFullAccessJWT(kr sdkkeyring.Keyring, addr sdk.AccAddress) (string, error) {
	now := time.Now()
	claims := ajwt.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    addr.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		Version: "v1",
		Leases:  ajwt.Leases{Access: ajwt.AccessTypeFull},
	}

	token := jwt.NewWithClaims(ajwt.SigningMethodES256K, &claims)
	signed, err := token.SignedString(ajwt.NewSigner(kr, addr))
	if err != nil {
		return "", fmt.Errorf("sign provider JWT: %w", err)
	}
	return signed, nil
}

func newScopedJWT(
	kr sdkkeyring.Keyring,
	addr sdk.AccAddress,
	providerAddress sdk.AccAddress,
	deployment ajwt.PermissionDeployment,
) (string, error) {
	now := time.Now()
	claims := ajwt.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    addr.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		Version: "v1",
		Leases: ajwt.Leases{
			Access: ajwt.AccessTypeGranular,
			Permissions: ajwt.Permissions{{
				Provider:    providerAddress,
				Access:      ajwt.AccessTypeGranular,
				Deployments: []ajwt.PermissionDeployment{deployment},
			}},
		},
	}
	if err := claims.Leases.Validate(); err != nil {
		return "", fmt.Errorf("validate scoped provider JWT: %w", err)
	}

	token := jwt.NewWithClaims(ajwt.SigningMethodES256K, &claims)
	signed, err := token.SignedString(ajwt.NewSigner(kr, addr))
	if err != nil {
		return "", fmt.Errorf("sign scoped provider JWT: %w", err)
	}
	return signed, nil
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
