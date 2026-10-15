package provider

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
	manifest "pkg.akt.dev/go/manifest/v2beta3"
	ctypes "pkg.akt.dev/go/node/cert/v1"
	rest "pkg.akt.dev/go/provider/client"
	ajwt "pkg.akt.dev/go/util/jwt"
	atls "pkg.akt.dev/go/util/tls"
)

// gatewayCertificateQuery applies the cert module's owner, serial, and state
// filters. Its records are immutable once the gRPC server starts.
type gatewayCertificateQuery struct {
	ctypes.UnimplementedQueryServer
	records  map[string]ctypes.CertificateResponse
	provider string
	err      error
}

func (q gatewayCertificateQuery) Certificates(_ context.Context, req *ctypes.QueryCertificatesRequest) (*ctypes.QueryCertificatesResponse, error) {
	if req.Filter.Owner == q.provider && q.err != nil {
		return nil, q.err
	}
	response := &ctypes.QueryCertificatesResponse{}
	if record, ok := q.records[req.Filter.Owner]; ok && record.Serial == req.Filter.Serial &&
		(req.Filter.State == "" || req.Filter.State == record.Certificate.State.String()) {
		response.Certificates = []ctypes.CertificateResponse{record}
	}
	return response, nil
}

func gatewayCommandCertificate(t *testing.T, cctx sdkclient.Context, domains []string) (*x509.Certificate, tls.Certificate, ctypes.CertificateResponse) {
	t.Helper()
	manager, err := atls.NewKeyPairManager(cctx, cctx.FromAddress)
	require.NoError(t, err)
	require.NoError(t, manager.Generate(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), domains))
	cert, pair, err := manager.ReadX509KeyPair()
	require.NoError(t, err)
	pubkey, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	require.NoError(t, err)
	return cert, pair, ctypes.CertificateResponse{
		Serial: cert.SerialNumber.String(),
		Certificate: ctypes.Certificate{
			State:  ctypes.CertificateValid,
			Cert:   pem.EncodeToMemory(&pem.Block{Type: ctypes.PemBlkTypeCertificate, Bytes: cert.Raw}),
			Pubkey: pem.EncodeToMemory(&pem.Block{Type: ctypes.PemBlkTypeECPublicKey, Bytes: pubkey}),
		},
	}
}

func TestProviderCommandsOverTLS(t *testing.T) {
	for _, authType := range []string{"mtls", "jwt"} {
		for _, registration := range []string{"valid", "missing", "revoked", "query unavailable"} {
			t.Run(authType+"/"+registration, func(t *testing.T) {
				ctx := providerOutputTestContext(t)
				cctx := ctx.Value(sdkclient.ClientContextKey).(*sdkclient.Context)
				*cctx = cctx.WithHomeDir(t.TempDir())
				owner := cctx.FromAddress
				clientCert, _, clientRecord := gatewayCommandCertificate(t, *cctx, nil)
				providerCtx := providerOutputTestContext(t).Value(sdkclient.ClientContextKey).(*sdkclient.Context)
				*providerCtx = providerCtx.WithHomeDir(t.TempDir())
				provider := providerCtx.FromAddress.String()
				_, serverPair, providerRecord := gatewayCommandCertificate(t, *providerCtx, []string{"provider.example.com"})
				query := gatewayCertificateQuery{
					records:  map[string]ctypes.CertificateResponse{owner.String(): clientRecord},
					provider: provider,
				}
				switch registration {
				case "valid":
					query.records[provider] = providerRecord
				case "revoked":
					providerRecord.Certificate.State = ctypes.CertificateRevoked
					query.records[provider] = providerRecord
				case "query unavailable":
					query.err = status.Error(codes.Unavailable, "certificate query unavailable")
				}
				listener := bufconn.Listen(1024 * 1024)
				queryServer := grpc.NewServer()
				ctypes.RegisterQueryServer(queryServer, query)
				go func() { _ = queryServer.Serve(listener) }()
				t.Cleanup(queryServer.Stop)
				conn, err := grpc.NewClient("passthrough:///certificates",
					grpc.WithTransportCredentials(insecure.NewCredentials()),
					grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
						return listener.DialContext(ctx)
					}))
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, conn.Close()) })
				*cctx = cctx.WithGRPCClient(conn)

				record, err := cctx.Keyring.KeyByAddress(owner)
				require.NoError(t, err)
				pubkey, err := record.GetPubKey()
				require.NoError(t, err)
				var mu sync.Mutex
				var uploaded manifest.Manifest
				requests := map[string]int{}
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if authType == "mtls" {
						if r.TLS.ServerName != "mtls.127.0.0.1" || len(r.TLS.VerifiedChains) == 0 ||
							r.TLS.PeerCertificates[0].Subject.CommonName != owner.String() || r.Header.Get("Authorization") != "" {
							t.Error("request did not carry the verified mTLS account and expected SNI")
							http.Error(w, "invalid mTLS identity", http.StatusUnauthorized)
							return
						}
					} else {
						claims := &ajwt.Claims{}
						parsed, err := jwt.ParseWithClaims(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), claims,
							func(_ *jwt.Token) (any, error) { return ajwt.NewVerifier(pubkey, owner), nil },
							jwt.WithValidMethods([]string{ajwt.SigningMethodES256K.Alg()}))
						if err != nil || !parsed.Valid || claims.Issuer != owner.String() || len(r.TLS.PeerCertificates) != 0 {
							t.Error("request did not carry a valid wallet JWT")
							http.Error(w, "invalid JWT", http.StatusUnauthorized)
							return
						}
					}
					mu.Lock()
					defer mu.Unlock()
					request := r.Method + " " + r.URL.Path
					requests[request]++
					w.Header().Set("Content-Type", "application/json")
					switch request {
					case "PUT /deployment/42/manifest":
						if err := json.NewDecoder(r.Body).Decode(&uploaded); err != nil {
							t.Errorf("decode uploaded manifest: %v", err)
							http.Error(w, "invalid manifest", http.StatusBadRequest)
						}
					case "GET /lease/42/1/1/manifest":
						if err := json.NewEncoder(w).Encode(uploaded); err != nil {
							t.Errorf("return manifest: %v", err)
						}
					case "GET /lease/42/1/1/status":
						_, _ = io.WriteString(w, `{"services":{"web":{"name":"web","available":1,"total":1,"uris":["web.example.com"]}}}`)
					default:
						t.Errorf("unexpected request: %s", request)
						http.NotFound(w, r)
					}
				}))
				server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverPair}}
				if authType == "mtls" {
					server.TLS.ClientAuth = tls.RequireAndVerifyClientCert
					server.TLS.ClientCAs = x509.NewCertPool()
					server.TLS.ClientCAs.AddCert(clientCert)
				}
				server.Config.ErrorLog = log.New(io.Discard, "", 0)
				server.StartTLS()
				t.Cleanup(server.Close)

				actions, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.jsonl"))
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, actions.Close()) })
				ctx = cliutil.WithActionLog(ctx, actions)
				for _, args := range [][]string{
					{"send-manifest", "../../workflow/adapters/testdata/deployment.yaml", "--dseq", "42"},
					{"get-manifest", "42"},
					{"lease-status", "42"},
				} {
					t.Run(args[0], func(t *testing.T) {
						root := Commands()
						root.SilenceUsage = true
						var stdout bytes.Buffer
						root.SetOut(&stdout)
						root.SetErr(io.Discard)
						root.SetArgs(append(args, "--provider", provider, "--provider-url", server.URL, "--auth-type", authType))
						err := root.ExecuteContext(ctx)
						if registration != "valid" {
							require.Error(t, err)
							if registration == "query unavailable" {
								require.ErrorContains(t, err, "certificate query unavailable")
							} else {
								require.ErrorContains(t, err, atls.CertificateInvalidError{Reason: atls.OnChainCertsNotAvailable}.Error())
							}
							require.Empty(t, stdout.String())
							return
						}
						require.NoError(t, err)
						switch args[0] {
						case "send-manifest":
							require.Equal(t, fmt.Sprintf("Manifest submitted successfully to %s.\n", provider), stdout.String())
							mu.Lock()
							defer mu.Unlock()
							require.Len(t, uploaded, 1)
							require.Equal(t, "westcoast", uploaded[0].Name)
							require.Len(t, uploaded[0].Services, 1)
							require.Equal(t, "web", uploaded[0].Services[0].Name)
							require.Equal(t, "nginx", uploaded[0].Services[0].Image)
							require.EqualValues(t, 1, uploaded[0].Services[0].Count)
						case "get-manifest":
							var got manifest.Manifest
							require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
							mu.Lock()
							defer mu.Unlock()
							require.Equal(t, uploaded, got)
						case "lease-status":
							var got rest.LeaseStatus
							require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
							require.Equal(t, map[string]*rest.ServiceStatus{
								"web": {Name: "web", Available: 1, Total: 1, URIs: []string{"web.example.com"}},
							}, got.Services)
						}
					})
				}
				mu.Lock()
				defer mu.Unlock()
				if registration == "valid" {
					require.Equal(t, map[string]int{
						"PUT /deployment/42/manifest": 1, "GET /lease/42/1/1/manifest": 1, "GET /lease/42/1/1/status": 1,
					}, requests)
				} else {
					require.Empty(t, requests, "an unverified provider must receive no HTTP request")
				}
				entries, err := actions.Read(actionlog.Filter{})
				require.NoError(t, err)
				require.Len(t, entries, 1, "only send-manifest changes state")
				require.Equal(t, "send-manifest", entries[0].Action)
				require.Equal(t, provider, entries[0].Provider)
				require.EqualValues(t, 42, entries[0].DSeq)
				if registration == "valid" {
					require.Equal(t, "success", entries[0].Status)
				} else {
					require.Equal(t, "failed", entries[0].Status)
				}
			})
		}
	}
}

func TestResolveGatewayAuthPreservesNamedAccount(t *testing.T) {
	for _, authType := range []string{"mtls", "jwt"} {
		t.Run(authType, func(t *testing.T) {
			ctx := providerOutputTestContext(t)
			cctx := ctx.Value(sdkclient.ClientContextKey).(*sdkclient.Context)
			owner := cctx.FromAddress
			*cctx = cctx.WithFrom("owner").WithFromAddress(nil)
			cmd := newProviderCmd(t, "--auth-type", authType)
			cmd.SetContext(ctx)
			resolved, account, auth, err := resolveGatewayAuth(cmd)
			require.NoError(t, err)
			require.Equal(t, authType, auth)
			require.Equal(t, owner, account)
			require.Equal(t, owner, resolved.FromAddress)
			_, err = resolved.Keyring.KeyByAddress(owner)
			require.NoError(t, err)
		})
	}
}

func TestProviderCommandsRejectInvalidQueryContextBeforeGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("command reached the provider with an invalid certificate query context")
	}))
	defer server.Close()
	for _, authType := range []string{"public", "jwt", "mtls"} {
		t.Run(authType, func(t *testing.T) {
			root := Commands()
			root.PersistentFlags().String("chain-id", "", "")
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			ctx := walletlessCommandContext()
			args := []string{"status", testProviderAddr}
			if authType != "public" {
				ctx = providerOutputTestContext(t)
				args = []string{"lease-status", "42", "--provider", testProviderAddr, "--auth-type", authType}
			}
			cctx := ctx.Value(sdkclient.ClientContextKey).(*sdkclient.Context)
			*cctx = cctx.WithChainID("test-chain")
			root.SetArgs(append(args, "--provider-url", server.URL, "--chain-id", "wrong-chain"))
			require.ErrorContains(t, root.ExecuteContext(ctx), "initialize provider query client: --chain-id")
		})
	}
}
