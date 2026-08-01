package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

type contextLightClient struct {
	cctx sdkclient.Context
}

func (cl contextLightClient) Query() v1beta3.QueryClient { return nil }
func (cl contextLightClient) Node() v1beta3.NodeClient   { return nil }
func (cl contextLightClient) ClientContext() sdkclient.Context {
	return cl.cctx
}
func (contextLightClient) PrintMessage(interface{}) error { return nil }
func (contextLightClient) PrintJSON(interface{}) error    { return nil }

func TestProviderStatusIsPublic(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want no provider status credentials", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	result, err := HandleProviderStatus(nil)(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"provider_url": srv.URL,
		}},
	})
	if err != nil {
		t.Fatalf("provider status handler: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("provider status result = %#v, want success", result)
	}
	if requests != 1 {
		t.Fatalf("provider status requests = %d, want 1", requests)
	}
}

func TestProtectedProviderGatewayHandlersAuthenticate(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cl := newSignedLightClient(t)
	tests := []struct {
		name    string
		handler mcpserver.ToolHandlerFunc
		args    map[string]any
	}{
		{
			name:    "lease status",
			handler: HandleLeaseStatus(cl),
			args: map[string]any{
				"provider_url": srv.URL,
				"dseq":         float64(1),
				"gseq":         float64(1),
				"oseq":         float64(1),
			},
		},
		{
			name:    "service status",
			handler: HandleServiceStatus(cl),
			args: map[string]any{
				"provider_url": srv.URL,
				"dseq":         float64(1),
				"gseq":         float64(1),
				"oseq":         float64(1),
				"service":      "web",
			},
		},
		{
			name:    "submit manifest",
			handler: HandleSubmitManifest(cl),
			args: map[string]any{
				"provider_url":  srv.URL,
				"dseq":          float64(1),
				"manifest_json": `[]`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.handler(context.Background(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{Arguments: tt.args},
			})
			if err != nil {
				t.Fatalf("handler returned an error: %v", err)
			}
			if result == nil {
				t.Fatal("handler returned no result")
			}
			if result.IsError {
				t.Fatalf("handler returned an MCP error: %s", resultText(result))
			}
		})
	}

	if requests != len(tests) {
		t.Fatalf("gateway requests = %d, want %d", requests, len(tests))
	}
}

func TestGatewayClientRequiresSigningIdentity(t *testing.T) {
	tests := []struct {
		name string
		cl   v1beta3.LightClient
		want string
	}{
		{
			name: "default account",
			cl:   contextLightClient{},
			want: "default account",
		},
		{
			name: "keyring",
			cl: func() v1beta3.LightClient {
				signed := newSignedLightClient(t).(contextLightClient)
				signed.cctx = signed.cctx.WithKeyring(nil)
				return signed
			}(),
			want: "keyring",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gatewayClient(context.Background(), tt.cl, "https://provider.example.com")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func newSignedLightClient(t *testing.T) v1beta3.LightClient {
	t.Helper()

	kr := aktkeyring.NewInMemory(aktcodec.MakeEncodingConfig().Codec)
	record, _, err := kr.NewMnemonic(
		"testkey",
		sdkkeyring.English,
		"m/44'/118'/0'/0/0",
		"",
		aktkeyring.DefaultAlgo(),
	)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	addr, err := record.GetAddress()
	if err != nil {
		t.Fatalf("get key address: %v", err)
	}

	return contextLightClient{cctx: sdkclient.Context{}.
		WithKeyring(kr).
		WithFromName(record.Name).
		WithFromAddress(addr)}
}

func resultText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if content, ok := result.Content[0].(mcp.TextContent); ok {
		return content.Text
	}
	return fmt.Sprint(result.Content[0])
}
