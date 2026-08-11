package provider

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	ctypes "pkg.akt.dev/go/node/cert/v1"
	mtypes "pkg.akt.dev/go/node/market/v1"
	leasev1 "pkg.akt.dev/go/provider/lease/v1"
	ajwt "pkg.akt.dev/go/util/jwt"
	atls "pkg.akt.dev/go/util/tls"
)

// grpcGatewayPort is the fixed port the provider serves its gRPC gateway on,
// alongside the REST gateway on 8443. The REST host URI akt already resolves is
// the input; host:8444 is derived from it.
const grpcGatewayPort = "8444"

// GRPCClient talks to the provider gateway over gRPC. It is the transport twin
// of the REST Client in client.go: same auth modes, same TLS trust model, a
// different wire protocol.
type GRPCClient struct {
	conn  *grpc.ClientConn
	token string
}

// NewGRPCGatewayClient dials the provider gateway's gRPC endpoint with the
// given auth method. authType is "jwt" (default) or "mtls". It mirrors
// NewGatewayClient's signature so the command layer resolves auth once and
// hands the same inputs to either transport.
func NewGRPCGatewayClient(
	ctx context.Context,
	cctx sdkclient.Context,
	addr sdk.AccAddress,
	providerURL string,
	authType string,
	kr sdkkeyring.Keyring,
	certQuerier atls.CertificateQuerier,
) (*GRPCClient, error) {
	if err := ValidateGatewayAuthentication(addr, authType, kr); err != nil {
		return nil, err
	}

	host, target, err := grpcTargetFromProviderURL(providerURL)
	if err != nil {
		return nil, err
	}

	tlsCfg, token, err := gatewayClientTLS(ctx, cctx, addr, host, authType, kr, certQuerier)
	if err != nil {
		return nil, err
	}

	return DialGRPCGatewayClient(target, token, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
}

// gatewayClientTLS builds the TLS config and, for JWT auth, the bearer token the
// gRPC gateway client dials with. Isolating the auth-mode decision here lets a
// test assert ServerName and token selection without a live dial.
//
// The provider's gRPC gateway serves a self-signed certificate whose common name
// is the provider's on-chain address, so standard PKI against the system roots
// rejects it. verifyProviderCertificate authenticates it against the provider's
// registered certificate in the cert module instead, mirroring the SDK REST
// client rather than provider-services' blanket InsecureSkipVerify.
func gatewayClientTLS(
	ctx context.Context,
	cctx sdkclient.Context,
	addr sdk.AccAddress,
	host string,
	authType string,
	kr sdkkeyring.Keyring,
	certQuerier atls.CertificateQuerier,
) (*tls.Config, string, error) {
	serverName := host
	var clientCert *tls.Certificate
	var token string

	switch authType {
	case "mtls":
		cert, err := loadMTLSCert(ctx, cctx)
		if err != nil {
			return nil, "", fmt.Errorf("load mTLS certificate: %w", err)
		}
		clientCert = &cert
		serverName = "mtls." + host
	default: // "jwt" or unset; ValidateGatewayAuthentication has rejected the rest.
		signed, err := attestationJWT(ajwt.NewSigner(kr, addr))
		if err != nil {
			return nil, "", fmt.Errorf("sign gateway JWT: %w", err)
		}
		token = signed
	}

	tlsCfg := &tls.Config{
		MinVersion:            tls.VersionTLS13,
		ServerName:            serverName,
		InsecureSkipVerify:    true, // nolint: gosec // verifyProviderCertificate does the checking Go's default would.
		VerifyPeerCertificate: verifyProviderCertificate(ctx, certQuerier, serverName),
	}
	if clientCert != nil {
		tlsCfg.Certificates = []tls.Certificate{*clientCert}
	}

	return tlsCfg, token, nil
}

// verifyProviderCertificate authenticates the provider's gateway certificate. It
// first tries standard PKI, so a provider fronted by a publicly trusted CA still
// works, then verifies the provider's self-signed certificate against the one it
// registered on chain, the trust anchor for the common self-signed case.
func verifyProviderCertificate(
	ctx context.Context,
	certQuerier atls.CertificateQuerier,
	serverName string,
) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		certs := make([]*x509.Certificate, 0, len(rawCerts))
		for _, raw := range rawCerts {
			cert, err := x509.ParseCertificate(raw)
			if err != nil {
				return err
			}
			certs = append(certs, cert)
		}
		if len(certs) == 0 {
			return atls.CertificateInvalidError{Reason: atls.EmptyPeerCertificate}
		}

		intermediates := x509.NewCertPool()
		for _, ic := range certs[1:] {
			intermediates.AddCert(ic)
		}
		_, pkiErr := certs[0].Verify(x509.VerifyOptions{
			Intermediates: intermediates,
			DNSName:       serverName,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		if pkiErr == nil {
			return nil
		}

		if certQuerier == nil {
			return fmt.Errorf("verify provider certificate: %w", pkiErr)
		}

		_, _, err := atls.ValidatePeerCertificates(
			ctx, certQuerier, certs, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		)
		return err
	}
}

// onChainCertQuerier resolves a provider's registered certificate from the cert
// module so a self-signed peer certificate can be verified against it. It is the
// akt-owned CertificateQuerier the SDK's verification helper needs. cctx must be
// a query-capable client context (from GetClientQueryContext); the JWT auth
// context is not, so the caller passes the provider query context.
type onChainCertQuerier struct {
	cctx sdkclient.Context
}

// NewOnChainCertQuerier builds the CertificateQuerier the gRPC gateway client
// uses to authenticate a provider's self-signed certificate. cctx is a
// query-capable client context.
func NewOnChainCertQuerier(cctx sdkclient.Context) atls.CertificateQuerier {
	return onChainCertQuerier{cctx: cctx}
}

func (q onChainCertQuerier) GetAccountCertificate(
	ctx context.Context,
	owner sdk.Address,
	serial *big.Int,
) (*x509.Certificate, crypto.PublicKey, error) {
	resp, err := ctypes.NewQueryClient(q.cctx).Certificates(ctx, &ctypes.QueryCertificatesRequest{
		Filter: ctypes.CertificateFilter{
			Owner:  owner.String(),
			Serial: serial.String(),
			State:  ctypes.CertificateValid.String(),
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if len(resp.Certificates) == 0 {
		return nil, nil, atls.CertificateInvalidError{Reason: atls.OnChainCertsNotAvailable}
	}

	registered := resp.Certificates[0].Certificate

	blk, rest := pem.Decode(registered.Cert)
	if blk == nil || len(rest) > 0 || blk.Type != ctypes.PemBlkTypeCertificate {
		return nil, nil, ctypes.ErrInvalidCertificateValue
	}
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return nil, nil, err
	}

	blk, rest = pem.Decode(registered.Pubkey)
	if blk == nil || len(rest) > 0 || blk.Type != ctypes.PemBlkTypeECPublicKey {
		return nil, nil, ctypes.ErrInvalidPubkeyValue
	}
	pubkey, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return nil, nil, err
	}

	return cert, pubkey, nil
}

// DialGRPCGatewayClient is the transport-agnostic core of NewGRPCGatewayClient:
// it owns connection construction and auth bookkeeping, and takes the dial
// options as an argument so an in-memory (bufconn) transport can stand in for a
// real TLS dial under test.
func DialGRPCGatewayClient(target, token string, opts ...grpc.DialOption) (*GRPCClient, error) {
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial provider gRPC gateway %s: %w", target, err)
	}

	return &GRPCClient{conn: conn, token: token}, nil
}

// LeaseAttestation requests an attestation quote for a confidential-compute
// lease. The nonce is a fixed 64 bytes the sidecar echoes into the report's
// report_data; the gateway carries it base64-encoded on the wire.
func (c *GRPCClient) LeaseAttestation(
	ctx context.Context,
	id mtypes.LeaseID,
	nonce [64]byte,
	bindTLS bool,
) (*leasev1.AttestationQuoteResponse, error) {
	req := &leasev1.AttestationQuoteRequest{
		LeaseId: id,
		Nonce:   base64.StdEncoding.EncodeToString(nonce[:]),
		BindTls: bindTLS,
	}

	return leasev1.NewLeaseRPCClient(c.conn).AttestationQuote(c.authContext(ctx), req)
}

// authContext attaches JWT auth to an outgoing call. mTLS carries its identity
// in the TLS handshake, so it needs nothing here.
func (c *GRPCClient) authContext(ctx context.Context) context.Context {
	if c.token == "" {
		return ctx
	}

	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

// grpcTargetFromProviderURL turns the provider's REST host URI into the gRPC
// dial target and the hostname used for certificate verification.
func grpcTargetFromProviderURL(providerURL string) (host, target string, err error) {
	u, err := url.Parse(providerURL)
	if err != nil {
		return "", "", fmt.Errorf("parse provider URL %q: %w", providerURL, err)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return "", "", fmt.Errorf("provider URL %q has no host", providerURL)
	}

	return hostname, net.JoinHostPort(hostname, grpcGatewayPort), nil
}

// attestationJWT signs the same short-lived full-access gateway token the
// provider REST client and provider-services' own command produce.
func attestationJWT(signer ajwt.SignerI) (string, error) {
	now := time.Now()

	claims := ajwt.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    signer.GetAddress().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		Version: "v1",
		Leases:  ajwt.Leases{Access: ajwt.AccessTypeFull},
	}

	return jwt.NewWithClaims(ajwt.SigningMethodES256K, &claims).SignedString(signer)
}
