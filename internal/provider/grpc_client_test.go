package provider

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
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

	mtypes "pkg.akt.dev/go/node/market/v1"
	leasev1 "pkg.akt.dev/go/provider/lease/v1"
	ajwt "pkg.akt.dev/go/util/jwt"

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

	tlsCfg, token, err := gatewayClientTLS(context.Background(), sdkclient.Context{}, addr, "provider.example.com", "jwt", kr, nil)
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
