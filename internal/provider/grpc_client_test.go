package provider

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	ctypes "pkg.akt.dev/go/node/cert/v1"
	mtypes "pkg.akt.dev/go/node/market/v1"
	leasev1 "pkg.akt.dev/go/provider/lease/v1"
	ajwt "pkg.akt.dev/go/util/jwt"
	atls "pkg.akt.dev/go/util/tls"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

// stubLeaseRPCServer is the in-memory gateway the gRPC client tests dial. It
// records the last request so a test can assert what reached the wire, and
// answers AttestationQuote with a canned response.
type stubLeaseRPCServer struct {
	leasev1.UnimplementedLeaseRPCServer
	lastReq *leasev1.AttestationQuoteRequest
	resp    *leasev1.AttestationQuoteResponse
}

func (s *stubLeaseRPCServer) AttestationQuote(
	_ context.Context,
	req *leasev1.AttestationQuoteRequest,
) (*leasev1.AttestationQuoteResponse, error) {
	s.lastReq = req
	return s.resp, nil
}

// gatewayAuthInterceptor accepts a call carrying either a Bearer token or a TLS
// client certificate, and rejects one carrying neither, standing in for the
// provider's own auth gate.
func gatewayAuthInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	if p, ok := peer.FromContext(ctx); ok {
		if ti, ok := p.AuthInfo.(credentials.TLSInfo); ok && len(ti.State.PeerCertificates) > 0 {
			return handler(ctx, req)
		}
	}

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("authorization"); len(vals) > 0 && strings.HasPrefix(vals[0], "Bearer ") {
			return handler(ctx, req)
		}
	}

	return nil, status.Error(codes.Unauthenticated, "missing gateway auth")
}

func startBufconnGateway(
	t *testing.T,
	srv leasev1.LeaseRPCServer,
	serverCreds credentials.TransportCredentials,
) *bufconn.Listener {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	opts := []grpc.ServerOption{grpc.ChainUnaryInterceptor(gatewayAuthInterceptor)}
	if serverCreds != nil {
		opts = append(opts, grpc.Creds(serverCreds))
	}

	gs := grpc.NewServer(opts...)
	leasev1.RegisterLeaseRPCServer(gs, srv)

	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	return lis
}

func bufconnDialOption(lis *bufconn.Listener) grpc.DialOption {
	return grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	})
}

func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-gateway"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

type providerCertificateFixture struct {
	owner   sdk.AccAddress
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	der     []byte
	certPEM []byte
	pubPEM  []byte
}

func newProviderCertificateFixture(t *testing.T, serverName string) providerCertificateFixture {
	t.Helper()

	owner := sdk.AccAddress(bytes.Repeat([]byte{0x42}, 20))
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject: pkix.Name{
			CommonName: owner.String(),
			ExtraNames: []pkix.AttributeTypeAndValue{{
				Type:  atls.AuthVersionOID,
				Value: "v0.0.1",
			}},
		},
		Issuer:                      pkix.Name{CommonName: owner.String()},
		NotBefore:                   time.Now().Add(-time.Hour),
		NotAfter:                    time.Now().Add(time.Hour),
		KeyUsage:                    x509.KeyUsageDataEncipherment | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:                 []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid:       true,
		PermittedDNSDomains:         []string{serverName},
		PermittedDNSDomainsCritical: true,
		DNSNames:                    []string{serverName},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)

	return providerCertificateFixture{
		owner: owner,
		cert:  cert,
		key:   key,
		der:   der,
		certPEM: pem.EncodeToMemory(&pem.Block{
			Type:  ctypes.PemBlkTypeCertificate,
			Bytes: der,
		}),
		pubPEM: pem.EncodeToMemory(&pem.Block{
			Type:  ctypes.PemBlkTypeECPublicKey,
			Bytes: pubDER,
		}),
	}
}

type recordingCertificateQuerier struct {
	cert   *x509.Certificate
	pubkey crypto.PublicKey
	err    error
	calls  int
	owner  sdk.Address
	serial *big.Int
}

func (querier *recordingCertificateQuerier) GetAccountCertificate(
	_ context.Context,
	owner sdk.Address,
	serial *big.Int,
) (*x509.Certificate, crypto.PublicKey, error) {
	querier.calls++
	querier.owner = owner
	querier.serial = new(big.Int).Set(serial)
	return querier.cert, querier.pubkey, querier.err
}

type scriptedCertificateQueryServer struct {
	ctypes.UnimplementedQueryServer
	request  *ctypes.QueryCertificatesRequest
	response *ctypes.QueryCertificatesResponse
	err      error
}

func (server *scriptedCertificateQueryServer) Certificates(
	_ context.Context,
	request *ctypes.QueryCertificatesRequest,
) (*ctypes.QueryCertificatesResponse, error) {
	server.request = request
	if server.err != nil {
		return nil, server.err
	}
	return server.response, nil
}

func certificateQueryResponse(certPEM, pubPEM []byte) *ctypes.QueryCertificatesResponse {
	return &ctypes.QueryCertificatesResponse{
		Certificates: []ctypes.CertificateResponse{{
			Serial: "42",
			Certificate: ctypes.Certificate{
				State:  ctypes.CertificateValid,
				Cert:   certPEM,
				Pubkey: pubPEM,
			},
		}},
	}
}

func TestGRPCClientJWTDialAuthenticates(t *testing.T) {
	stub := &stubLeaseRPCServer{resp: &leasev1.AttestationQuoteResponse{Report: "cA=="}}
	lis := startBufconnGateway(t, stub, nil)

	cl, err := DialGRPCGatewayClient("passthrough:///bufnet", "signed-token",
		bufconnDialOption(lis),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cl.Close()

	_, err = leasev1.NewLeaseRPCClient(cl.conn).AttestationQuote(
		cl.authContext(context.Background()), &leasev1.AttestationQuoteRequest{})
	require.NoError(t, err)
}

func TestGRPCClientMTLSDialAuthenticates(t *testing.T) {
	cert := selfSignedCert(t)
	serverCreds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAnyClientCert,
		MinVersion:   tls.VersionTLS13,
	})
	clientCreds := credentials.NewTLS(&tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	})

	stub := &stubLeaseRPCServer{resp: &leasev1.AttestationQuoteResponse{Report: "cA=="}}
	lis := startBufconnGateway(t, stub, serverCreds)

	cl, err := DialGRPCGatewayClient("passthrough:///bufnet", "",
		bufconnDialOption(lis),
		grpc.WithTransportCredentials(clientCreds))
	require.NoError(t, err)
	defer cl.Close()

	_, err = leasev1.NewLeaseRPCClient(cl.conn).AttestationQuote(
		cl.authContext(context.Background()), &leasev1.AttestationQuoteRequest{})
	require.NoError(t, err)
}

func TestGRPCClientUnauthenticatedDialRejected(t *testing.T) {
	stub := &stubLeaseRPCServer{resp: &leasev1.AttestationQuoteResponse{Report: "cA=="}}
	lis := startBufconnGateway(t, stub, nil)

	cl, err := DialGRPCGatewayClient("passthrough:///bufnet", "",
		bufconnDialOption(lis),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cl.Close()

	_, err = leasev1.NewLeaseRPCClient(cl.conn).AttestationQuote(
		cl.authContext(context.Background()), &leasev1.AttestationQuoteRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGRPCClientLeaseAttestationRoundTrip(t *testing.T) {
	stub := &stubLeaseRPCServer{resp: &leasev1.AttestationQuoteResponse{
		Report:      "TU9DSw==",
		TeePlatform: "snp",
	}}
	lis := startBufconnGateway(t, stub, nil)

	cl, err := DialGRPCGatewayClient("passthrough:///bufnet", "signed-token",
		bufconnDialOption(lis),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cl.Close()

	var nonce [64]byte
	for i := range nonce {
		nonce[i] = byte(i)
	}
	lid := mtypes.LeaseID{Owner: "akash1owner", DSeq: 12345, GSeq: 1, OSeq: 1, Provider: "akash1provider"}

	resp, err := cl.LeaseAttestation(context.Background(), lid, nonce, true)
	require.NoError(t, err)
	require.Equal(t, "snp", resp.GetTeePlatform())
	require.Equal(t, stub.resp.GetReport(), resp.GetReport())

	require.Equal(t, lid, stub.lastReq.GetLeaseId())
	require.True(t, stub.lastReq.GetBindTls())
	require.Equal(t, base64.StdEncoding.EncodeToString(nonce[:]), stub.lastReq.GetNonce())
}

func TestGRPCTargetFromProviderURL(t *testing.T) {
	host, target, err := grpcTargetFromProviderURL("https://provider.example.com:8443")
	require.NoError(t, err)
	require.Equal(t, "provider.example.com", host)
	require.Equal(t, "provider.example.com:8444", target)

	_, _, err = grpcTargetFromProviderURL("://not a url")
	require.Error(t, err)

	_, _, err = grpcTargetFromProviderURL("relative/path")
	require.ErrorContains(t, err, "has no host")
}

// TestGatewayClientTLSJWTSignsVerifiableToken exercises the production JWT path
// that the dial-level tests bypass: it asserts the ServerName and TLS config the
// gRPC client presents and that the hand-rolled attestation token both verifies
// against the account's own key and carries the claims the gateway expects.
func TestGatewayClientTLSJWTSignsVerifiableToken(t *testing.T) {
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	rec, _, err := kr.NewMnemonic("owner", sdkkeyring.English, "m/44'/118'/0'/0/0", "", aktkeyring.DefaultAlgo())
	require.NoError(t, err)
	addr, err := rec.GetAddress()
	require.NoError(t, err)
	pub, err := rec.GetPubKey()
	require.NoError(t, err)

	expectedProvider := sdk.AccAddress(bytes.Repeat([]byte{0x24}, 20))
	tlsCfg, token, err := gatewayClientTLS(
		context.Background(), sdkclient.Context{}, addr, expectedProvider,
		"provider.example.com", "jwt", kr, nil,
	)
	require.NoError(t, err)
	require.Equal(t, "provider.example.com", tlsCfg.ServerName)
	require.Equal(t, uint16(tls.VersionTLS13), tlsCfg.MinVersion)
	require.Empty(t, tlsCfg.Certificates)
	require.NotEmpty(t, token)

	var claims ajwt.Claims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
		return ajwt.NewVerifier(pub, addr), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	require.Equal(t, addr.String(), claims.Issuer)
	require.Equal(t, ajwt.AccessTypeFull, claims.Leases.Access)
	require.Equal(t, "v1", claims.Version)
	require.WithinDuration(t, claims.IssuedAt.Time.Add(15*time.Minute), claims.ExpiresAt.Time, time.Second)
}

func TestNewGRPCGatewayClientValidatesAndBuildsJWTTransport(t *testing.T) {
	kr, _, owner := newProviderTestIdentity(t)
	expectedProvider := sdk.AccAddress(bytes.Repeat([]byte{0x24}, 20))

	t.Run("provider identity", func(t *testing.T) {
		client, err := NewGRPCGatewayClient(
			context.Background(),
			sdkclient.Context{},
			owner,
			nil,
			"https://provider.example.com:8443",
			"jwt",
			kr,
			nil,
		)
		require.Nil(t, client)
		require.ErrorContains(t, err, "provider identity is required")
	})

	t.Run("authentication", func(t *testing.T) {
		client, err := NewGRPCGatewayClient(
			context.Background(),
			sdkclient.Context{},
			owner,
			expectedProvider,
			"https://provider.example.com:8443",
			"unsupported",
			kr,
			nil,
		)
		require.Nil(t, client)
		require.ErrorContains(t, err, "unsupported auth type")
	})

	t.Run("provider URL", func(t *testing.T) {
		client, err := NewGRPCGatewayClient(
			context.Background(),
			sdkclient.Context{},
			owner,
			expectedProvider,
			"://not a url",
			"jwt",
			kr,
			nil,
		)
		require.Nil(t, client)
		require.ErrorContains(t, err, "parse provider URL")
	})

	t.Run("JWT signing", func(t *testing.T) {
		signErr := errors.New("signing failed")
		client, err := NewGRPCGatewayClient(
			context.Background(),
			sdkclient.Context{},
			owner,
			expectedProvider,
			"https://provider.example.com:8443",
			"jwt",
			failingSignKeyring{Keyring: kr, err: signErr},
			nil,
		)
		require.Nil(t, client)
		require.ErrorIs(t, err, signErr)
		require.ErrorContains(t, err, "sign gateway JWT")
	})

	t.Run("success", func(t *testing.T) {
		client, err := NewGRPCGatewayClient(
			context.Background(),
			sdkclient.Context{},
			owner,
			expectedProvider,
			"https://provider.example.com:8443",
			"jwt",
			kr,
			nil,
		)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NotEmpty(t, client.token)
		require.Equal(t, "provider.example.com:8444", client.conn.Target())
		require.NoError(t, client.Close())
	})
}

func TestGatewayClientTLSMTLSLoadsCertificate(t *testing.T) {
	kr, _, owner := newProviderTestIdentity(t)
	queryServer := &certificateQueryServer{}
	queryConn := newCertificateQueryConn(t, queryServer)
	homeDir := t.TempDir()
	cctx := sdkclient.Context{}.
		WithKeyring(kr).
		WithFromAddress(owner).
		WithHomeDir(homeDir).
		WithGRPCClient(queryConn)

	manager, err := atls.NewKeyPairManager(cctx, owner)
	require.NoError(t, err)
	require.NoError(t, manager.Generate(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), nil))

	tlsConfig, token, err := gatewayClientTLS(
		context.Background(),
		cctx,
		owner,
		owner,
		"provider.example.com",
		"mtls",
		kr,
		nil,
	)
	require.NoError(t, err)
	require.Empty(t, token)
	require.Equal(t, "mtls.provider.example.com", tlsConfig.ServerName)
	require.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MinVersion)
	require.Len(t, tlsConfig.Certificates, 1)
	require.NotNil(t, tlsConfig.VerifyPeerCertificate)
	require.Equal(t, 1, queryServer.calls)
	require.Equal(t, owner.String(), queryServer.owner)

	t.Run("missing certificate", func(t *testing.T) {
		missingCertContext := cctx.WithHomeDir(t.TempDir())
		tlsConfig, token, err := gatewayClientTLS(
			context.Background(),
			missingCertContext,
			owner,
			owner,
			"provider.example.com",
			"mtls",
			kr,
			nil,
		)
		require.Nil(t, tlsConfig)
		require.Empty(t, token)
		require.ErrorContains(t, err, "load mTLS certificate")
	})
}

func TestVerifyProviderCertificate(t *testing.T) {
	fixture := newProviderCertificateFixture(t, "provider.example.com")

	t.Run("empty certificate chain", func(t *testing.T) {
		err := verifyProviderCertificate(
			context.Background(), nil, "provider.example.com", fixture.owner,
		)(nil, nil)
		var invalid atls.CertificateInvalidError
		require.ErrorAs(t, err, &invalid)
		require.Equal(t, atls.EmptyPeerCertificate, invalid.Reason)
	})

	t.Run("malformed DER", func(t *testing.T) {
		err := verifyProviderCertificate(context.Background(), nil, "provider.example.com", fixture.owner)(
			[][]byte{[]byte("not a certificate")},
			nil,
		)
		require.Error(t, err)
	})

	t.Run("missing on-chain querier", func(t *testing.T) {
		err := verifyProviderCertificate(context.Background(), nil, "provider.example.com", fixture.owner)(
			[][]byte{fixture.der},
			nil,
		)
		require.ErrorContains(t, err, "verify provider certificate")
	})

	t.Run("on-chain certificate", func(t *testing.T) {
		querier := &recordingCertificateQuerier{
			cert:   fixture.cert,
			pubkey: &fixture.key.PublicKey,
		}
		err := verifyProviderCertificate(context.Background(), querier, "provider.example.com", fixture.owner)(
			[][]byte{fixture.der},
			nil,
		)
		require.NoError(t, err)
		require.Equal(t, 1, querier.calls)
		require.Equal(t, fixture.owner.String(), querier.owner.String())
		require.Zero(t, fixture.cert.SerialNumber.Cmp(querier.serial))
	})

	t.Run("different provider identity", func(t *testing.T) {
		querier := &recordingCertificateQuerier{
			cert:   fixture.cert,
			pubkey: &fixture.key.PublicKey,
		}
		expectedProvider := sdk.AccAddress(bytes.Repeat([]byte{0x24}, 20))
		err := verifyProviderCertificate(
			context.Background(), querier, "provider.example.com", expectedProvider,
		)([][]byte{fixture.der}, nil)
		require.ErrorContains(t, err, "does not match expected provider")
		require.ErrorContains(t, err, expectedProvider.String())
		require.Zero(t, querier.calls)
	})

	t.Run("on-chain query failure", func(t *testing.T) {
		queryErr := errors.New("query failed")
		querier := &recordingCertificateQuerier{err: queryErr}
		err := verifyProviderCertificate(context.Background(), querier, "provider.example.com", fixture.owner)(
			[][]byte{fixture.der},
			nil,
		)
		require.ErrorIs(t, err, queryErr)
		require.Equal(t, 1, querier.calls)
	})

	t.Run("wrong registered public key", func(t *testing.T) {
		otherFixture := newProviderCertificateFixture(t, "provider.example.com")
		querier := &recordingCertificateQuerier{
			cert:   fixture.cert,
			pubkey: &otherFixture.key.PublicKey,
		}
		err := verifyProviderCertificate(context.Background(), querier, "provider.example.com", fixture.owner)(
			[][]byte{fixture.der},
			nil,
		)
		require.ErrorContains(t, err, "invalid certificate signature")
	})

	t.Run("multiple peer certificates", func(t *testing.T) {
		querier := &recordingCertificateQuerier{
			cert:   fixture.cert,
			pubkey: &fixture.key.PublicKey,
		}
		err := verifyProviderCertificate(context.Background(), querier, "provider.example.com", fixture.owner)(
			[][]byte{fixture.der, fixture.der},
			nil,
		)
		var invalid atls.CertificateInvalidError
		require.ErrorAs(t, err, &invalid)
		require.Equal(t, atls.TooManyPeerCertificates, invalid.Reason)
		require.Zero(t, querier.calls)
	})
}

func TestOnChainCertQuerierGetAccountCertificate(t *testing.T) {
	fixture := newProviderCertificateFixture(t, "provider.example.com")
	queryErr := errors.New("certificate query failed")

	tests := []struct {
		name     string
		response *ctypes.QueryCertificatesResponse
		queryErr error
		wantErr  string
	}{
		{
			name:     "query failure",
			queryErr: queryErr,
			wantErr:  queryErr.Error(),
		},
		{
			name:     "missing certificate",
			response: &ctypes.QueryCertificatesResponse{},
			wantErr:  "on-chain certificates are not available",
		},
		{
			name:     "malformed certificate PEM",
			response: certificateQueryResponse([]byte("not PEM"), fixture.pubPEM),
			wantErr:  ctypes.ErrInvalidCertificateValue.Error(),
		},
		{
			name: "malformed certificate DER",
			response: certificateQueryResponse(
				pem.EncodeToMemory(&pem.Block{Type: ctypes.PemBlkTypeCertificate, Bytes: []byte("not DER")}),
				fixture.pubPEM,
			),
			wantErr: "malformed certificate",
		},
		{
			name:     "malformed public key PEM",
			response: certificateQueryResponse(fixture.certPEM, []byte("not PEM")),
			wantErr:  ctypes.ErrInvalidPubkeyValue.Error(),
		},
		{
			name: "malformed public key DER",
			response: certificateQueryResponse(
				fixture.certPEM,
				pem.EncodeToMemory(&pem.Block{Type: ctypes.PemBlkTypeECPublicKey, Bytes: []byte("not DER")}),
			),
			wantErr: "asn1: structure error",
		},
		{
			name:     "success",
			response: certificateQueryResponse(fixture.certPEM, fixture.pubPEM),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &scriptedCertificateQueryServer{
				response: test.response,
				err:      test.queryErr,
			}
			conn := newCertificateQueryConn(t, server)
			querier := NewOnChainCertQuerier(sdkclient.Context{}.WithGRPCClient(conn))

			cert, publicKey, err := querier.GetAccountCertificate(
				context.Background(),
				fixture.owner,
				big.NewInt(42),
			)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Nil(t, cert)
				require.Nil(t, publicKey)
			} else {
				require.NoError(t, err)
				require.Equal(t, fixture.der, cert.Raw)
				require.Equal(t, &fixture.key.PublicKey, publicKey)
			}

			require.NotNil(t, server.request)
			require.Equal(t, fixture.owner.String(), server.request.Filter.Owner)
			require.Equal(t, "42", server.request.Filter.Serial)
			require.Equal(t, ctypes.CertificateValid.String(), server.request.Filter.State)
		})
	}
}

func TestDialGRPCGatewayClientRejectsMissingTransportCredentials(t *testing.T) {
	client, err := DialGRPCGatewayClient("passthrough:///missing-credentials", "")
	require.Nil(t, client)
	require.ErrorContains(t, err, "no transport security set")
}
