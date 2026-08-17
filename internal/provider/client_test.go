package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	ctypes "pkg.akt.dev/go/node/cert/v1"
	rest "pkg.akt.dev/go/provider/client"
	ajwt "pkg.akt.dev/go/util/jwt"
	atls "pkg.akt.dev/go/util/tls"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

type failingSignKeyring struct {
	sdkkeyring.Keyring
	err error
}

func (kr failingSignKeyring) SignByAddress(
	_ sdk.Address,
	_ []byte,
	_ signing.SignMode,
) ([]byte, cryptotypes.PubKey, error) {
	return nil, nil, kr.err
}

type certificateQueryServer struct {
	ctypes.UnimplementedQueryServer
	calls int
	owner string
}

func (server *certificateQueryServer) Certificates(
	_ context.Context,
	request *ctypes.QueryCertificatesRequest,
) (*ctypes.QueryCertificatesResponse, error) {
	server.calls++
	server.owner = request.Filter.Owner
	if request.Filter.Serial == "" {
		return nil, errors.New("certificate query has no serial")
	}
	return &ctypes.QueryCertificatesResponse{
		Certificates: []ctypes.CertificateResponse{{
			Serial: request.Filter.Serial,
			Certificate: ctypes.Certificate{
				State: ctypes.CertificateValid,
			},
		}},
	}, nil
}

func newCertificateQueryConn(t *testing.T, query ctypes.QueryServer) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	ctypes.RegisterQueryServer(server, query)
	go func() {
		_ = server.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		"passthrough:///certificate-query-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("create certificate query client: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	})
	return conn
}

func newProviderTestIdentity(t *testing.T) (sdkkeyring.Keyring, *sdkkeyring.Record, sdk.AccAddress) {
	t.Helper()
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	record, _, err := kr.NewMnemonic(
		"owner",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatalf("create signing key: %v", err)
	}
	owner, err := record.GetAddress()
	if err != nil {
		t.Fatalf("get owner address: %v", err)
	}
	return kr, record, owner
}

func TestGatewayClientRequiresKeyring(t *testing.T) {
	addr := sdk.AccAddress([]byte("authenticated-owner"))
	cctx := sdkclient.Context{}.WithFromAddress(addr)

	_, err := NewGatewayClient(
		context.Background(),
		cctx,
		addr,
		"https://provider.example.com:8443",
		"jwt",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "requires a keyring") {
		t.Fatalf("error = %v, want a keyring remedy", err)
	}
}

func TestPublicGatewayClientDoesNotSendWalletAuthentication(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want none", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client, err := NewPublicGatewayClient(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatalf("create public gateway client: %v", err)
	}
	if _, err := client.Status(context.Background()); err != nil {
		t.Fatalf("query public provider status: %v", err)
	}
	if requests != 1 {
		t.Fatalf("provider requests = %d, want 1", requests)
	}
}

func TestGatewayAuthenticationRejectsInvalidBoundary(t *testing.T) {
	kr, _, owner := newProviderTestIdentity(t)
	tests := []struct {
		name     string
		address  sdk.AccAddress
		authType string
		want     string
	}{
		{name: "unknown auth type", address: owner, authType: "password", want: "unsupported auth type"},
		{name: "empty account", authType: "jwt", want: "default account"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateGatewayAuthentication(test.address, test.authType, kr); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
			_, err := NewScopedGatewayClient(
				context.Background(),
				sdkclient.Context{}.WithKeyring(kr).WithFromAddress(test.address),
				test.address,
				"https://provider.example.com:8443",
				sdk.AccAddress(bytes.Repeat([]byte{7}, 20)),
				ajwt.PermissionDeployment{
					Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
					DSeq:     42,
					Services: []string{"web"},
				},
				test.authType,
				kr,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("scoped client error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestGatewayClientRequiresSelectedKey(t *testing.T) {
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	_, _, err := kr.NewMnemonic(
		"different-key",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatalf("create unrelated key: %v", err)
	}

	addr := sdk.AccAddress([]byte("missing-signing-key"))
	for _, authType := range []string{"jwt", "mtls"} {
		t.Run(authType, func(t *testing.T) {
			_, err := NewGatewayClient(
				context.Background(),
				sdkclient.Context{}.WithFromAddress(addr),
				addr,
				"https://provider.example.com:8443",
				authType,
				kr,
			)
			if err == nil || !strings.Contains(err.Error(), "selected account") ||
				!strings.Contains(err.Error(), "configured keyring") {
				t.Fatalf("error = %v, want a provider signing-key remedy", err)
			}
		})
	}
}

func TestGatewayClientSendsVerifiableFullAccessJWT(t *testing.T) {
	kr, record, owner := newProviderTestIdentity(t)
	pubkey, err := record.GetPubKey()
	if err != nil {
		t.Fatalf("get owner public key: %v", err)
	}

	var authorization string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("provider path = %q, want /status", r.URL.Path)
		}
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client, err := NewGatewayClient(
		context.Background(),
		sdkclient.Context{}.WithKeyring(kr).WithFromAddress(owner),
		owner,
		srv.URL,
		"",
		kr,
	)
	if err != nil {
		t.Fatalf("create default JWT gateway client: %v", err)
	}
	if _, err := client.Status(context.Background()); err != nil {
		t.Fatalf("query provider status: %v", err)
	}
	if !strings.HasPrefix(authorization, "Bearer ") {
		t.Fatalf("Authorization = %q, want bearer token", authorization)
	}

	claims := &ajwt.Claims{}
	parsed, err := jwt.ParseWithClaims(strings.TrimPrefix(authorization, "Bearer "), claims, func(token *jwt.Token) (any, error) {
		if token.Method != ajwt.SigningMethodES256K {
			return nil, fmt.Errorf("unexpected signing method %s", token.Method.Alg())
		}
		return ajwt.NewVerifier(pubkey, owner), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("verify provider JWT: valid=%t err=%v", parsed != nil && parsed.Valid, err)
	}
	if claims.Issuer != owner.String() || claims.Leases.Access != ajwt.AccessTypeFull {
		t.Fatalf("provider JWT claims = issuer %q leases %+v", claims.Issuer, claims.Leases)
	}
}

func TestMTLSGatewayClientsRequireAndLoadOnChainCertificate(t *testing.T) {
	kr, _, owner := newProviderTestIdentity(t)
	providerAddress := sdk.AccAddress(bytes.Repeat([]byte{7}, 20))
	query := &certificateQueryServer{}
	queryConn := newCertificateQueryConn(t, query)
	home := t.TempDir()
	cctx := sdkclient.Context{}.
		WithKeyring(kr).
		WithFromAddress(owner).
		WithHomeDir(home).
		WithGRPCClient(queryConn)

	manager, err := atls.NewKeyPairManager(cctx, owner)
	if err != nil {
		t.Fatalf("create certificate manager: %v", err)
	}
	if err := manager.Generate(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), nil); err != nil {
		t.Fatalf("generate client certificate: %v", err)
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("mTLS request Authorization = %q, want none", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	clients := []struct {
		name string
		new  func() (rest.Client, error)
	}{
		{
			name: "gateway",
			new: func() (rest.Client, error) {
				return NewGatewayClient(context.Background(), cctx, owner, srv.URL, "mtls", kr)
			},
		},
		{
			name: "scoped gateway",
			new: func() (rest.Client, error) {
				return NewScopedGatewayClient(
					context.Background(),
					cctx,
					owner,
					srv.URL,
					providerAddress,
					ajwt.PermissionDeployment{
						Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
						DSeq:     42,
						Services: []string{"web"},
					},
					"mtls",
					kr,
				)
			},
		},
	}

	for _, test := range clients {
		t.Run(test.name, func(t *testing.T) {
			client, err := test.new()
			if err != nil {
				t.Fatalf("create mTLS client: %v", err)
			}
			if _, err := client.Status(context.Background()); err != nil {
				t.Fatalf("query provider status: %v", err)
			}
		})
	}
	if query.calls != len(clients) || query.owner != owner.String() {
		t.Fatalf("certificate queries=%d owner=%q, want %d for %q", query.calls, query.owner, len(clients), owner)
	}
	if requests != len(clients) {
		t.Fatalf("provider requests = %d, want %d", requests, len(clients))
	}

	emptyHomeContext := sdkclient.Context{}.
		WithKeyring(kr).
		WithFromAddress(owner).
		WithHomeDir(t.TempDir()).
		WithGRPCClient(queryConn)
	_, err = NewScopedGatewayClient(
		context.Background(),
		emptyHomeContext,
		owner,
		srv.URL,
		providerAddress,
		ajwt.PermissionDeployment{
			Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
			DSeq:     42,
			Services: []string{"web"},
		},
		"mtls",
		kr,
	)
	if err == nil || !strings.Contains(err.Error(), "load mTLS certificate") {
		t.Fatalf("missing mTLS certificate error = %v", err)
	}
	_, err = NewGatewayClient(
		context.Background(),
		emptyHomeContext,
		owner,
		srv.URL,
		"mtls",
		kr,
	)
	if err == nil || !strings.Contains(err.Error(), "load mTLS certificate") {
		t.Fatalf("missing unscoped mTLS certificate error = %v", err)
	}
}

func TestScopedJWTRestrictsProviderDeploymentAndScope(t *testing.T) {
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	record, _, err := kr.NewMnemonic(
		"owner",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatalf("create signing key: %v", err)
	}
	owner, err := record.GetAddress()
	if err != nil {
		t.Fatalf("get owner address: %v", err)
	}
	pubkey, err := record.GetPubKey()
	if err != nil {
		t.Fatalf("get owner public key: %v", err)
	}
	providerAddress := sdk.AccAddress(bytes.Repeat([]byte{7}, 20))
	deployment := ajwt.PermissionDeployment{
		Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
		DSeq:     42,
		GSeq:     3,
		OSeq:     2,
		Services: []string{"web"},
	}

	token, err := newScopedJWT(kr, owner, providerAddress, deployment)
	if err != nil {
		t.Fatalf("create scoped JWT: %v", err)
	}

	claims := &ajwt.Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (any, error) {
		if token.Method != ajwt.SigningMethodES256K {
			return nil, fmt.Errorf("unexpected signing method %s", token.Method.Alg())
		}
		return ajwt.NewVerifier(pubkey, owner), nil
	})
	if err != nil {
		t.Fatalf("parse scoped JWT: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("scoped JWT signature is invalid")
	}
	if err := claims.Validate(); err != nil {
		t.Fatalf("validate scoped JWT claims: %v", err)
	}
	if claims.Issuer != owner.String() || claims.Version != "v1" {
		t.Fatalf("identity claims = issuer %q version %q", claims.Issuer, claims.Version)
	}
	if claims.IssuedAt == nil || claims.NotBefore == nil || claims.ExpiresAt == nil {
		t.Fatalf("time claims are incomplete: %+v", claims.RegisteredClaims)
	}
	lifetime := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if lifetime != 15*time.Minute {
		t.Fatalf("token lifetime = %s, want 15m", lifetime)
	}
	if claims.Leases.Access != ajwt.AccessTypeGranular || len(claims.Leases.Permissions) != 1 {
		t.Fatalf("lease permissions = %+v, want one granular permission", claims.Leases)
	}
	permission := claims.Leases.Permissions[0]
	if !permission.Provider.Equals(providerAddress) || permission.Access != ajwt.AccessTypeGranular || len(permission.Deployments) != 1 {
		t.Fatalf("provider permission = %+v, want one granular deployment for %s", permission, providerAddress)
	}
	got := permission.Deployments[0]
	if got.DSeq != deployment.DSeq || got.GSeq != deployment.GSeq || got.OSeq != deployment.OSeq ||
		len(got.Scope) != 1 || got.Scope[0] != ajwt.PermissionScopeStatus ||
		len(got.Services) != 1 || got.Services[0] != "web" {
		t.Fatalf("deployment permission = %+v, want %+v", got, deployment)
	}

	var authorization string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	client, err := NewScopedGatewayClient(
		context.Background(),
		sdkclient.Context{}.WithKeyring(kr).WithFromAddress(owner),
		owner,
		srv.URL,
		providerAddress,
		deployment,
		"jwt",
		kr,
	)
	if err != nil {
		t.Fatalf("create scoped gateway client: %v", err)
	}
	if _, err := client.Status(context.Background()); err != nil {
		t.Fatalf("query provider through scoped client: %v", err)
	}
	if !strings.HasPrefix(authorization, "Bearer ") {
		t.Fatalf("Authorization = %q, want scoped bearer token", authorization)
	}
	wireClaims := &ajwt.Claims{}
	wireToken, err := jwt.ParseWithClaims(strings.TrimPrefix(authorization, "Bearer "), wireClaims, func(token *jwt.Token) (any, error) {
		return ajwt.NewVerifier(pubkey, owner), nil
	})
	if err != nil || !wireToken.Valid {
		t.Fatalf("verify scoped gateway JWT: valid=%t err=%v", wireToken != nil && wireToken.Valid, err)
	}
	if len(wireClaims.Leases.Permissions) != 1 ||
		!wireClaims.Leases.Permissions[0].Provider.Equals(providerAddress) ||
		len(wireClaims.Leases.Permissions[0].Deployments) != 1 ||
		wireClaims.Leases.Permissions[0].Deployments[0].DSeq != deployment.DSeq {
		t.Fatalf("wire JWT permissions = %+v", wireClaims.Leases)
	}
}

func TestScopedJWTRejectsInvalidBoundary(t *testing.T) {
	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	record, _, err := kr.NewMnemonic(
		"owner",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatalf("create signing key: %v", err)
	}
	owner, err := record.GetAddress()
	if err != nil {
		t.Fatalf("get owner address: %v", err)
	}

	_, err = newScopedJWT(
		kr,
		owner,
		sdk.AccAddress(bytes.Repeat([]byte{7}, 20)),
		ajwt.PermissionDeployment{
			Scope: ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
			DSeq:  42,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "empty services") {
		t.Fatalf("error = %v, want invalid granular deployment rejection", err)
	}

	_, err = NewScopedGatewayClient(
		context.Background(),
		sdkclient.Context{}.WithFromAddress(owner),
		owner,
		"https://provider.example.com:8443",
		nil,
		ajwt.PermissionDeployment{
			Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
			DSeq:     42,
			Services: []string{"web"},
		},
		"jwt",
		kr,
	)
	if err == nil || !strings.Contains(err.Error(), "provider address") {
		t.Fatalf("error = %v, want empty provider rejection", err)
	}

	_, err = NewScopedGatewayClient(
		context.Background(),
		sdkclient.Context{}.WithFromAddress(owner),
		owner,
		"https://provider.example.com:8443",
		sdk.AccAddress(bytes.Repeat([]byte{7}, 20)),
		ajwt.PermissionDeployment{
			Scope: ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
			DSeq:  42,
		},
		"jwt",
		kr,
	)
	if err == nil || !strings.Contains(err.Error(), "validate scoped provider JWT") {
		t.Fatalf("error = %v, want invalid deployment rejection from scoped client", err)
	}
}

func TestScopedJWTReportsSigningFailure(t *testing.T) {
	kr, _, owner := newProviderTestIdentity(t)
	signErr := errors.New("signing device unavailable")
	_, err := newScopedJWT(
		failingSignKeyring{Keyring: kr, err: signErr},
		owner,
		sdk.AccAddress(bytes.Repeat([]byte{7}, 20)),
		ajwt.PermissionDeployment{
			Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
			DSeq:     42,
			Services: []string{"web"},
		},
	)
	if !errors.Is(err, signErr) || !strings.Contains(err.Error(), "sign scoped provider JWT") {
		t.Fatalf("signing error = %v, want wrapped device failure", err)
	}
}

func TestGatewayClientPropagatesFullAccessJWTSigningFailure(t *testing.T) {
	kr, _, owner := newProviderTestIdentity(t)
	signErr := errors.New("signing device unavailable")
	client, err := NewGatewayClient(
		context.Background(),
		sdkclient.Context{}.WithFromAddress(owner),
		owner,
		"https://provider.example.com:8443",
		"jwt",
		failingSignKeyring{Keyring: kr, err: signErr},
	)
	if client != nil || !errors.Is(err, signErr) || !strings.Contains(err.Error(), "sign provider JWT") {
		t.Fatalf("gateway client/signing error = %T, %v, want nil client and wrapped device failure", client, err)
	}
}
