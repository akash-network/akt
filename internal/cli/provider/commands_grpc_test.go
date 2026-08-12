package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	leasev1 "pkg.akt.dev/go/provider/lease/v1"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
	aktprovider "pkg.akt.dev/akt/internal/provider"
)

// echoAttestationServer answers AttestationQuote with a MOCK-format report that
// echoes the request nonce, the arrangement a live confidential-compute sidecar
// produces in mock mode, so the command's freshness check has something real to
// verify.
type echoAttestationServer struct {
	leasev1.UnimplementedLeaseRPCServer
	err    error
	report string
}

func (s echoAttestationServer) AttestationQuote(
	_ context.Context,
	req *leasev1.AttestationQuoteRequest,
) (*leasev1.AttestationQuoteResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.report != "" {
		return &leasev1.AttestationQuoteResponse{Report: s.report}, nil
	}

	nonce, err := base64.StdEncoding.DecodeString(req.GetNonce())
	if err != nil {
		return nil, err
	}

	report := make([]byte, 144)
	copy(report[0:4], "MOCK")
	copy(report[80:], nonce)

	return &leasev1.AttestationQuoteResponse{
		Report:      base64.StdEncoding.EncodeToString(report),
		TeePlatform: "mock",
	}, nil
}

func newAttestationCommandContext(t *testing.T) context.Context {
	t.Helper()

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	rec, _, err := kr.NewMnemonic("owner", sdkkeyring.English, "m/44'/118'/0'/0/0", "", aktkeyring.DefaultAlgo())
	require.NoError(t, err)
	owner, err := rec.GetAddress()
	require.NoError(t, err)

	cctx := sdkclient.Context{}.WithFromAddress(owner).WithKeyring(kr)
	return context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx)
}

func executeAttestationCommand(ctx context.Context) error {
	root := Commands()
	root.PersistentFlags().StringP("output", "o", "pretty", "output format")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"lease-attestation", "12345",
		"--provider", testProviderAddr,
		"--provider-url", "https://gw.example.com:8443",
		"-o", "json",
	})
	return root.ExecuteContext(ctx)
}

func useAttestationTestGateway(t *testing.T, srv leasev1.LeaseRPCServer) {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	leasev1.RegisterLeaseRPCServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	orig := grpcGatewayClient
	t.Cleanup(func() { grpcGatewayClient = orig })
	grpcGatewayClient = func(_ *cobra.Command, _, _ string) (*aktprovider.GRPCClient, error) {
		return aktprovider.DialGRPCGatewayClient("passthrough:///bufnet", "",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
}

// TestLeaseAttestationVerifiesEchoedNonce drives the real command against an
// in-memory gRPC gateway, the gRPC counterpart of the REST httptest path the
// other provider operations exercise.
func TestLeaseAttestationVerifiesEchoedNonce(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	leasev1.RegisterLeaseRPCServer(gs, &echoAttestationServer{})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	orig := grpcGatewayClient
	t.Cleanup(func() { grpcGatewayClient = orig })
	var gotProvider string
	grpcGatewayClient = func(_ *cobra.Command, _ string, provider string) (*aktprovider.GRPCClient, error) {
		gotProvider = provider
		return aktprovider.DialGRPCGatewayClient("passthrough:///bufnet", "",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	rec, _, err := kr.NewMnemonic("owner", sdkkeyring.English, "m/44'/118'/0'/0/0", "", aktkeyring.DefaultAlgo())
	require.NoError(t, err)
	owner, err := rec.GetAddress()
	require.NoError(t, err)

	cctx := sdkclient.Context{}.WithFromAddress(owner).WithKeyring(kr)

	ctx := context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx)

	var out bytes.Buffer
	root := Commands()
	root.PersistentFlags().StringP("output", "o", "pretty", "output format")
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"lease-attestation", "12345",
		"--provider", testProviderAddr,
		"--provider-url", "https://gw.example.com:8443",
		"-o", "json",
	})
	require.NoError(t, root.ExecuteContext(ctx))
	require.Equal(t, testProviderAddr, gotProvider)

	var got struct {
		Nonce         string `json:"nonce"`
		ReportSize    int    `json:"report_size_bytes"`
		NonceVerified bool   `json:"nonce_verified"`
		MockReport    bool   `json:"mock_report"`
		Quote         struct {
			TeePlatform string `json:"tee_platform"`
		} `json:"quote"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))

	require.True(t, got.NonceVerified)
	require.True(t, got.MockReport)
	require.Equal(t, 144, got.ReportSize)
	require.NotEmpty(t, got.Nonce)
	require.Equal(t, "mock", got.Quote.TeePlatform)

	// The default output is a human summary, not the raw base64 quote.
	var prettyOut bytes.Buffer
	prettyRoot := Commands()
	prettyRoot.PersistentFlags().StringP("output", "o", "pretty", "output format")
	prettyRoot.SetOut(&prettyOut)
	prettyRoot.SetErr(&bytes.Buffer{})
	prettyRoot.SetArgs([]string{
		"lease-attestation", "12345",
		"--provider", testProviderAddr,
		"--provider-url", "https://gw.example.com:8443",
	})
	require.NoError(t, prettyRoot.ExecuteContext(ctx))

	summary := prettyOut.String()
	require.Contains(t, summary, "mock")      // tee platform value
	require.Contains(t, summary, "144 bytes") // report size value
	require.Contains(t, summary, "-o json")   // points to the full report
	require.NotContains(t, summary, `"quote"` /* the JSON representation */)
}

func TestLeaseAttestationReportsBoundaryFailures(t *testing.T) {
	ctx := newAttestationCommandContext(t)

	t.Run("client construction", func(t *testing.T) {
		want := errors.New("construct attestation client")
		orig := grpcGatewayClient
		t.Cleanup(func() { grpcGatewayClient = orig })
		grpcGatewayClient = func(*cobra.Command, string, string) (*aktprovider.GRPCClient, error) {
			return nil, want
		}

		err := executeAttestationCommand(ctx)
		require.ErrorIs(t, err, want)
	})

	t.Run("gateway RPC", func(t *testing.T) {
		useAttestationTestGateway(t, &echoAttestationServer{
			err: status.Error(codes.PermissionDenied, "quote denied"),
		})

		err := executeAttestationCommand(ctx)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Contains(t, err.Error(), "request lease attestation")
	})

	t.Run("malformed report", func(t *testing.T) {
		useAttestationTestGateway(t, &echoAttestationServer{report: "%%%not-base64%%%"})

		err := executeAttestationCommand(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "decode attestation report")
	})
}

func TestGRPCGatewayClientFromCmdRejectsMissingIdentity(t *testing.T) {
	cmd := newProviderCmd(t)
	cctx := sdkclient.Context{}
	cmd.SetContext(context.WithValue(context.Background(), sdkclient.ClientContextKey, &cctx))

	client, err := grpcGatewayClientFromCmd(cmd, "https://gw.example.com:8443", testProviderAddr)
	require.Nil(t, client)
	require.ErrorContains(t, err, "requires a configured default account")
}

func TestGRPCGatewayClientFromCmdRejectsInvalidResolvedProvider(t *testing.T) {
	cmd := newProviderCmd(t)
	cmd.SetContext(newAttestationCommandContext(t))

	client, err := grpcGatewayClientFromCmd(cmd, "https://gw.example.com:8443", "not-a-provider")
	require.Nil(t, client)
	require.ErrorContains(t, err, "invalid resolved provider address")
}

func TestGRPCGatewayClientFromCmdBuildsAuthenticatedClient(t *testing.T) {
	cmd := newProviderCmd(t)
	cmd.SetContext(newAttestationCommandContext(t))

	client, err := grpcGatewayClientFromCmd(cmd, "https://gw.example.com:8443", testProviderAddr)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NoError(t, client.Close())
}
