package provider

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"google.golang.org/grpc"

	manifest "pkg.akt.dev/go/manifest/v2beta3"
	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	ajwt "pkg.akt.dev/go/util/jwt"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

const testManifestJSON = `[{"name":"default","services":[{"name":"web","image":"nginx:latest","resources":{"id":1,"cpu":{},"memory":{},"storage":[],"gpu":{},"endpoints":[]},"count":1,"expose":[{"port":80,"proto":"TCP","global":true}]}]}]`

type contextLightClient struct {
	cctx  sdkclient.Context
	query v1beta3.QueryClient
}

func (cl contextLightClient) Query() v1beta3.QueryClient { return cl.query }
func (cl contextLightClient) Node() v1beta3.NodeClient   { return nil }
func (cl contextLightClient) ClientContext() sdkclient.Context {
	return cl.cctx
}
func (contextLightClient) PrintMessage(interface{}) error { return nil }
func (contextLightClient) PrintJSON(interface{}) error    { return nil }

type manifestQueryClient struct {
	v1beta3.QueryClient
	provider ptypes.QueryClient
}

func (client manifestQueryClient) Provider() ptypes.QueryClient { return client.provider }

type registeredProviderStub struct {
	ptypes.QueryClient
	hostURI           string
	err               error
	owner             string
	calls             int
	nilResponse       bool
	providersResponse *ptypes.QueryProvidersResponse
	providersErr      error
	providersCalls    int
}

func (stub *registeredProviderStub) Provider(_ context.Context, request *ptypes.QueryProviderRequest, _ ...grpc.CallOption) (*ptypes.QueryProviderResponse, error) {
	stub.calls++
	stub.owner = request.Owner
	if stub.err != nil {
		return nil, stub.err
	}
	if stub.nilResponse {
		return nil, nil
	}
	return &ptypes.QueryProviderResponse{Provider: ptypes.Provider{
		Owner:   request.Owner,
		HostURI: stub.hostURI,
	}}, nil
}

func (stub *registeredProviderStub) Providers(_ context.Context, _ *ptypes.QueryProvidersRequest, _ ...grpc.CallOption) (*ptypes.QueryProvidersResponse, error) {
	stub.providersCalls++
	if stub.providersErr != nil {
		return nil, stub.providersErr
	}
	if stub.providersResponse != nil {
		return stub.providersResponse, nil
	}
	return &ptypes.QueryProvidersResponse{}, nil
}

func providerOwnerAddress() string {
	return sdk.AccAddress(bytes.Repeat([]byte{9}, 20)).String()
}

func withRegisteredProvider(client v1beta3.LightClient, query ptypes.QueryClient) v1beta3.LightClient {
	configured := client.(contextLightClient)
	configured.query = manifestQueryClient{provider: query}
	return configured
}

func TestProviderToolSchemasUseProviderOwnerAddresses(t *testing.T) {
	tests := []struct {
		name       string
		tool       func() mcp.Tool
		toolName   string
		required   []string
		properties []string
	}{
		{name: "list providers", tool: ToolListProviders, toolName: "akash_list_providers"},
		{name: "get provider", tool: ToolGetProvider, toolName: "akash_get_provider", required: []string{"owner"}, properties: []string{"owner"}},
		{name: "provider status", tool: ToolProviderStatus, toolName: "akash_provider_status", required: []string{"provider"}, properties: []string{"provider"}},
		{name: "lease status", tool: ToolLeaseStatus, toolName: "akash_lease_status", required: []string{"provider", "dseq", "gseq", "oseq"}, properties: []string{"provider", "dseq", "gseq", "oseq"}},
		{name: "service status", tool: ToolServiceStatus, toolName: "akash_service_status", required: []string{"provider", "dseq", "gseq", "oseq", "service"}, properties: []string{"provider", "dseq", "gseq", "oseq", "service"}},
		{name: "submit manifest", tool: ToolSubmitManifest, toolName: "akash_submit_manifest", required: []string{"provider", "dseq", "manifest_json"}, properties: []string{"provider", "dseq", "manifest_json"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := test.tool()
			if tool.Name != test.toolName {
				t.Fatalf("tool name = %q, want %q", tool.Name, test.toolName)
			}
			if _, exists := tool.InputSchema.Properties["provider_url"]; exists {
				t.Fatal("tool exposes caller-selected provider_url")
			}
			if len(tool.InputSchema.Properties) != len(test.properties) {
				t.Fatalf("schema properties = %v, want %v", tool.InputSchema.Properties, test.properties)
			}
			for _, property := range test.properties {
				if _, exists := tool.InputSchema.Properties[property]; !exists {
					t.Errorf("schema is missing property %q", property)
				}
			}
			if len(tool.InputSchema.Required) != len(test.required) {
				t.Fatalf("required properties = %v, want %v", tool.InputSchema.Required, test.required)
			}
			for _, required := range test.required {
				if !containsString(tool.InputSchema.Required, required) {
					t.Errorf("property %q is not required", required)
				}
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestOnChainProviderHandlersReportQueryOutcomes(t *testing.T) {
	providerAddress := providerOwnerAddress()
	providerRecord := ptypes.Provider{Owner: providerAddress, HostURI: "https://registered.example.com"}

	t.Run("list success", func(t *testing.T) {
		query := &registeredProviderStub{providersResponse: &ptypes.QueryProvidersResponse{
			Providers: []ptypes.Provider{providerRecord},
		}}
		result, err := HandleListProviders(withRegisteredProvider(contextLightClient{}, query))(
			context.Background(), mcp.CallToolRequest{},
		)
		if err != nil || result == nil || result.IsError {
			t.Fatalf("list providers result=%#v err=%v", result, err)
		}
		if query.providersCalls != 1 || !strings.Contains(resultText(result), providerAddress) {
			t.Fatalf("list query calls=%d result=%q", query.providersCalls, resultText(result))
		}
	})

	t.Run("list failure", func(t *testing.T) {
		query := &registeredProviderStub{providersErr: fmt.Errorf("provider list unavailable")}
		result, err := HandleListProviders(withRegisteredProvider(contextLightClient{}, query))(
			context.Background(), mcp.CallToolRequest{},
		)
		if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), "provider list unavailable") {
			t.Fatalf("list providers failure result=%#v err=%v", result, err)
		}
		if query.providersCalls != 1 {
			t.Fatalf("list query calls = %d, want 1", query.providersCalls)
		}
	})

	t.Run("get success", func(t *testing.T) {
		query := &registeredProviderStub{hostURI: providerRecord.HostURI}
		result, err := HandleGetProvider(withRegisteredProvider(contextLightClient{}, query))(
			context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"owner": providerAddress}}},
		)
		if err != nil || result == nil || result.IsError {
			t.Fatalf("get provider result=%#v err=%v", result, err)
		}
		if query.calls != 1 || query.owner != providerAddress || !strings.Contains(resultText(result), providerRecord.HostURI) {
			t.Fatalf("get query calls=%d owner=%q result=%q", query.calls, query.owner, resultText(result))
		}
	})

	t.Run("get input failure", func(t *testing.T) {
		query := &registeredProviderStub{}
		result, err := HandleGetProvider(withRegisteredProvider(contextLightClient{}, query))(
			context.Background(), mcp.CallToolRequest{},
		)
		if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), "owner") {
			t.Fatalf("get provider input failure result=%#v err=%v", result, err)
		}
		if query.calls != 0 {
			t.Fatalf("invalid input made %d provider queries, want none", query.calls)
		}
	})

	t.Run("get query failure", func(t *testing.T) {
		query := &registeredProviderStub{err: fmt.Errorf("provider lookup unavailable")}
		result, err := HandleGetProvider(withRegisteredProvider(contextLightClient{}, query))(
			context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"owner": providerAddress}}},
		)
		if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), "provider lookup unavailable") {
			t.Fatalf("get provider query failure result=%#v err=%v", result, err)
		}
		if query.calls != 1 || query.owner != providerAddress {
			t.Fatalf("get query calls=%d owner=%q", query.calls, query.owner)
		}
	})
}

func TestProviderGatewayResolutionRejectsInvalidRegistryBoundaries(t *testing.T) {
	providerAddress := providerOwnerAddress()
	tests := []struct {
		name string
		cl   v1beta3.LightClient
		want string
	}{
		{name: "missing provider", cl: contextLightClient{query: manifestQueryClient{provider: &registeredProviderStub{}}}, want: "provider"},
		{name: "invalid provider", cl: contextLightClient{query: manifestQueryClient{provider: &registeredProviderStub{}}}, want: "invalid provider address"},
		{name: "missing light client", cl: nil, want: "provider query client is unavailable"},
		{name: "missing query client", cl: contextLightClient{}, want: "provider query client is unavailable"},
		{name: "missing provider query", cl: contextLightClient{query: manifestQueryClient{}}, want: "provider query client is unavailable"},
		{name: "query failure", cl: contextLightClient{query: manifestQueryClient{provider: &registeredProviderStub{err: fmt.Errorf("registry unavailable")}}}, want: "registry unavailable"},
		{name: "nil response", cl: contextLightClient{query: manifestQueryClient{provider: &registeredProviderStub{nilResponse: true}}}, want: "no registered host URI"},
		{name: "empty host URI", cl: contextLightClient{query: manifestQueryClient{provider: &registeredProviderStub{}}}, want: "no registered host URI"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments := map[string]any{"provider": providerAddress}
			switch test.name {
			case "missing provider":
				arguments = map[string]any{}
			case "invalid provider":
				arguments["provider"] = "not-an-address"
			}
			result, err := HandleProviderStatus(test.cl)(context.Background(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{Arguments: arguments},
			})
			if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), test.want) {
				t.Fatalf("provider resolution result=%#v err=%v, want %q", result, err, test.want)
			}
		})
	}
}

func TestProviderStatusReportsClientAndGatewayFailures(t *testing.T) {
	providerAddress := providerOwnerAddress()

	t.Run("invalid registered URL", func(t *testing.T) {
		query := &registeredProviderStub{hostURI: "%"}
		result, err := HandleProviderStatus(withRegisteredProvider(contextLightClient{}, query))(
			context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"provider": providerAddress}}},
		)
		if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), "failed to create provider client") {
			t.Fatalf("invalid registered URL result=%#v err=%v", result, err)
		}
		if query.calls != 1 || query.owner != providerAddress {
			t.Fatalf("provider lookup calls=%d owner=%q", query.calls, query.owner)
		}
	})

	t.Run("gateway failure", func(t *testing.T) {
		requests := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.URL.Path != "/status" {
				t.Errorf("provider path = %q, want /status", r.URL.Path)
			}
			http.Error(w, "status unavailable", http.StatusBadGateway)
		}))
		defer srv.Close()

		query := &registeredProviderStub{hostURI: srv.URL}
		result, err := HandleProviderStatus(withRegisteredProvider(contextLightClient{}, query))(
			context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"provider": providerAddress}}},
		)
		if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), "failed to get provider status") {
			t.Fatalf("gateway failure result=%#v err=%v", result, err)
		}
		if requests != 1 || query.calls != 1 {
			t.Fatalf("gateway requests=%d provider lookups=%d, want one each", requests, query.calls)
		}
	})
}

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

	providerAddress := providerOwnerAddress()
	providerQuery := &registeredProviderStub{hostURI: srv.URL}
	client := withRegisteredProvider(contextLightClient{}, providerQuery)
	result, err := HandleProviderStatus(client)(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"provider": providerAddress,
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
	if providerQuery.owner != providerAddress {
		t.Fatalf("provider lookup owner = %q, want %q", providerQuery.owner, providerAddress)
	}
}

func TestProtectedProviderGatewayHandlersAuthenticate(t *testing.T) {
	requests := 0
	tokens := make([]string, 0, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		authorization := r.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		tokens = append(tokens, strings.TrimPrefix(authorization, "Bearer "))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cl := newSignedLightClient(t)
	cctx := cl.ClientContext()
	record, err := cctx.Keyring.KeyByAddress(cctx.FromAddress)
	if err != nil {
		t.Fatalf("find signing key: %v", err)
	}
	pubkey, err := record.GetPubKey()
	if err != nil {
		t.Fatalf("get signing public key: %v", err)
	}
	providerAddress := providerOwnerAddress()
	manifestClient := withRegisteredProvider(cl, &registeredProviderStub{hostURI: srv.URL})
	tests := []struct {
		name    string
		handler mcpserver.ToolHandlerFunc
		args    map[string]any
		scope   ajwt.PermissionScope
		service string
	}{
		{
			name:    "lease status",
			handler: HandleLeaseStatus(manifestClient, "jwt"),
			args: map[string]any{
				"provider": providerAddress,
				"dseq":     float64(1),
				"gseq":     float64(1),
				"oseq":     float64(1),
			},
			scope:   ajwt.PermissionScopeStatus,
			service: "*",
		},
		{
			name:    "service status",
			handler: HandleServiceStatus(manifestClient, "jwt"),
			args: map[string]any{
				"provider": providerAddress,
				"dseq":     float64(1),
				"gseq":     float64(1),
				"oseq":     float64(1),
				"service":  "web",
			},
			scope:   ajwt.PermissionScopeStatus,
			service: "web",
		},
		{
			name:    "submit manifest",
			handler: HandleSubmitManifest(manifestClient, "jwt"),
			args: map[string]any{
				"provider":      providerAddress,
				"dseq":          float64(1),
				"manifest_json": testManifestJSON,
			},
			scope:   ajwt.PermissionScopeSendManifest,
			service: "web",
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
				return
			}
			if result.IsError {
				t.Fatalf("handler returned an MCP error: %s", resultText(result))
			}
			if len(tokens) != requests {
				t.Fatalf("captured tokens = %d, requests = %d", len(tokens), requests)
			}
			claims := &ajwt.Claims{}
			parsed, err := jwt.ParseWithClaims(tokens[len(tokens)-1], claims, func(token *jwt.Token) (any, error) {
				if token.Method != ajwt.SigningMethodES256K {
					return nil, fmt.Errorf("unexpected signing method %s", token.Method.Alg())
				}
				return ajwt.NewVerifier(pubkey, cctx.FromAddress), nil
			})
			if err != nil {
				t.Fatalf("parse provider token: %v", err)
			}
			if !parsed.Valid {
				t.Fatal("provider token signature is invalid")
			}
			if err := claims.Validate(); err != nil {
				t.Fatalf("validate provider token: %v", err)
			}
			if claims.Leases.Access != ajwt.AccessTypeGranular || len(claims.Leases.Permissions) != 1 {
				t.Fatalf("lease authorization = %+v, want one granular permission", claims.Leases)
			}
			permission := claims.Leases.Permissions[0]
			if permission.Provider.String() != providerAddress || permission.Access != ajwt.AccessTypeGranular || len(permission.Deployments) != 1 {
				t.Fatalf("provider permission = %+v, want %s granular", permission, providerAddress)
			}
			deployment := permission.Deployments[0]
			if deployment.DSeq != 1 || len(deployment.Scope) != 1 || deployment.Scope[0] != tt.scope || len(deployment.Services) != 1 || deployment.Services[0] != tt.service {
				t.Fatalf("deployment permission = %+v, want dseq=1 scope=%s service=%s", deployment, tt.scope, tt.service)
			}
		})
	}

	if requests != len(tests) {
		t.Fatalf("gateway requests = %d, want %d", requests, len(tests))
	}
}

func TestProtectedProviderHandlersRejectInvalidArgumentsBeforeGatewayWork(t *testing.T) {
	providerAddress := providerOwnerAddress()
	tests := []struct {
		name    string
		handler func(v1beta3.LightClient) mcpserver.ToolHandlerFunc
		args    map[string]any
		want    string
	}{
		{
			name:    "lease missing provider",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleLeaseStatus(cl, "jwt") },
			args:    map[string]any{"dseq": float64(1), "gseq": float64(1), "oseq": float64(1)},
			want:    "provider",
		},
		{
			name:    "lease invalid dseq",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleLeaseStatus(cl, "jwt") },
			args:    map[string]any{"provider": providerAddress, "dseq": "bad", "gseq": float64(1), "oseq": float64(1)},
			want:    "dseq",
		},
		{
			name:    "lease invalid gseq",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleLeaseStatus(cl, "jwt") },
			args:    map[string]any{"provider": providerAddress, "dseq": float64(1), "gseq": "bad", "oseq": float64(1)},
			want:    "gseq",
		},
		{
			name:    "lease invalid oseq",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleLeaseStatus(cl, "jwt") },
			args:    map[string]any{"provider": providerAddress, "dseq": float64(1), "gseq": float64(1), "oseq": "bad"},
			want:    "oseq",
		},
		{
			name:    "service missing provider",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleServiceStatus(cl, "jwt") },
			args:    map[string]any{"dseq": float64(1), "gseq": float64(1), "oseq": float64(1), "service": "web"},
			want:    "provider",
		},
		{
			name:    "service invalid dseq",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleServiceStatus(cl, "jwt") },
			args:    map[string]any{"provider": providerAddress, "dseq": "bad", "gseq": float64(1), "oseq": float64(1), "service": "web"},
			want:    "dseq",
		},
		{
			name:    "service invalid gseq",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleServiceStatus(cl, "jwt") },
			args:    map[string]any{"provider": providerAddress, "dseq": float64(1), "gseq": "bad", "oseq": float64(1), "service": "web"},
			want:    "gseq",
		},
		{
			name:    "service invalid oseq",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleServiceStatus(cl, "jwt") },
			args:    map[string]any{"provider": providerAddress, "dseq": float64(1), "gseq": float64(1), "oseq": "bad", "service": "web"},
			want:    "oseq",
		},
		{
			name:    "service missing service",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleServiceStatus(cl, "jwt") },
			args:    map[string]any{"provider": providerAddress, "dseq": float64(1), "gseq": float64(1), "oseq": float64(1)},
			want:    "service",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &registeredProviderStub{hostURI: "http://gateway.invalid"}
			client := withRegisteredProvider(newSignedLightClient(t), query)
			result, err := test.handler(client)(context.Background(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{Arguments: test.args},
			})
			if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), test.want) {
				t.Fatalf("invalid argument result=%#v err=%v, want %q", result, err, test.want)
			}
			if strings.Contains(test.name, "missing provider") {
				if query.calls != 0 {
					t.Fatalf("missing provider made %d registry calls, want none", query.calls)
				}
			} else if query.calls != 1 || query.owner != providerAddress {
				t.Fatalf("registry calls=%d owner=%q, want one lookup for %q", query.calls, query.owner, providerAddress)
			}
		})
	}
}

func TestProtectedProviderHandlersReportAuthenticationAndGatewayFailures(t *testing.T) {
	providerAddress := providerOwnerAddress()
	validLeaseArgs := map[string]any{
		"provider": providerAddress,
		"dseq":     float64(9),
		"gseq":     float64(2),
		"oseq":     float64(3),
	}
	validServiceArgs := map[string]any{
		"provider": providerAddress,
		"dseq":     float64(9),
		"gseq":     float64(2),
		"oseq":     float64(3),
		"service":  "web",
	}
	tests := []struct {
		name    string
		handler func(v1beta3.LightClient) mcpserver.ToolHandlerFunc
		args    map[string]any
		path    string
		want    string
	}{
		{
			name:    "lease",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleLeaseStatus(cl, "jwt") },
			args:    validLeaseArgs,
			path:    "/lease/9/2/3/status",
			want:    "failed to get lease status",
		},
		{
			name:    "service",
			handler: func(cl v1beta3.LightClient) mcpserver.ToolHandlerFunc { return HandleServiceStatus(cl, "jwt") },
			args:    validServiceArgs,
			path:    "/lease/9/2/3/service/web/status",
			want:    "failed to get service status",
		},
	}

	for _, test := range tests {
		t.Run(test.name+" authentication", func(t *testing.T) {
			query := &registeredProviderStub{hostURI: "http://gateway.invalid"}
			client := withRegisteredProvider(contextLightClient{}, query)
			result, err := test.handler(client)(context.Background(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{Arguments: test.args},
			})
			if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), "default account") {
				t.Fatalf("authentication failure result=%#v err=%v", result, err)
			}
			if query.calls != 1 || query.owner != providerAddress {
				t.Fatalf("registry calls=%d owner=%q", query.calls, query.owner)
			}
		})

		t.Run(test.name+" gateway", func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != test.path {
					t.Errorf("provider path = %q, want %q", r.URL.Path, test.path)
				}
				if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
					t.Error("protected gateway request has no bearer token")
				}
				http.Error(w, "gateway rejected request", http.StatusBadGateway)
			}))
			defer srv.Close()

			query := &registeredProviderStub{hostURI: srv.URL}
			client := withRegisteredProvider(newSignedLightClient(t), query)
			result, err := test.handler(client)(context.Background(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{Arguments: test.args},
			})
			if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), test.want) {
				t.Fatalf("gateway failure result=%#v err=%v, want %q", result, err, test.want)
			}
			if requests != 1 || query.calls != 1 {
				t.Fatalf("gateway requests=%d registry calls=%d, want one each", requests, query.calls)
			}
		})
	}
}

func TestSubmitManifestRejectsInvalidInputBeforeProviderLookup(t *testing.T) {
	providerAddress := providerOwnerAddress()
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "missing provider", args: map[string]any{"dseq": float64(1), "manifest_json": testManifestJSON}, want: "provider"},
		{name: "invalid provider", args: map[string]any{"provider": "not-an-address", "dseq": float64(1), "manifest_json": testManifestJSON}, want: "invalid provider address"},
		{name: "invalid dseq", args: map[string]any{"provider": providerAddress, "dseq": "bad", "manifest_json": testManifestJSON}, want: "dseq"},
		{name: "missing manifest", args: map[string]any{"provider": providerAddress, "dseq": float64(1)}, want: "manifest_json"},
		{name: "invalid manifest JSON", args: map[string]any{"provider": providerAddress, "dseq": float64(1), "manifest_json": "{"}, want: "invalid manifest JSON"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &registeredProviderStub{hostURI: "http://gateway.invalid"}
			client := withRegisteredProvider(newSignedLightClient(t), query)
			result, err := HandleSubmitManifest(client, "jwt")(context.Background(), mcp.CallToolRequest{
				Params: mcp.CallToolParams{Arguments: test.args},
			})
			if err != nil || result == nil || !result.IsError || !strings.Contains(resultText(result), test.want) {
				t.Fatalf("invalid manifest input result=%#v err=%v, want %q", result, err, test.want)
			}
			if query.calls != 0 {
				t.Fatalf("invalid input made %d provider registry calls, want none", query.calls)
			}
		})
	}
}

func TestSubmitManifestRejectsSemanticallyInvalidJSONWithoutSideEffects(t *testing.T) {
	providerAddress := providerOwnerAddress()
	tests := []struct {
		name         string
		manifestJSON string
		want         string
	}{
		{name: "empty manifest", manifestJSON: `[]`, want: "manifest is empty"},
		{name: "group without services", manifestJSON: `[{"name":"default","services":[]}]`, want: "contains no services"},
		{name: "service without image", manifestJSON: `[{"name":"default","services":[{"name":"web"}]}]`, want: "empty image name"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gatewayRequests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				gatewayRequests++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			logger, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
			if err != nil {
				t.Fatalf("open action log: %v", err)
			}
			t.Cleanup(func() { _ = logger.Close() })

			query := &registeredProviderStub{hostURI: srv.URL}
			client := withRegisteredProvider(newSignedLightClient(t), query)
			ctx := cliutil.WithActionLog(context.Background(), logger)
			result, err := HandleSubmitManifest(client, "jwt")(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{Arguments: map[string]any{
					"provider":      providerAddress,
					"dseq":          float64(1),
					"manifest_json": test.manifestJSON,
				}},
			})
			if err != nil {
				t.Errorf("semantic manifest validation returned handler error: %v", err)
			}
			if result == nil || !result.IsError || !strings.Contains(resultText(result), test.want) {
				t.Errorf("semantic manifest result=%#v, want tool error containing %q", result, test.want)
			}
			if query.calls != 0 {
				t.Errorf("semantic manifest made %d provider registry calls, want none", query.calls)
			}
			if gatewayRequests != 0 {
				t.Errorf("semantic manifest made %d provider gateway requests, want none", gatewayRequests)
			}

			entries, err := logger.Read(actionlog.Filter{})
			if err != nil {
				t.Fatalf("read action log: %v", err)
			}
			if len(entries) != 0 {
				t.Errorf("semantic manifest wrote %d action log entries, want none: %+v", len(entries), entries)
			}
		})
	}
}

func TestManifestServiceNamesDeduplicatesInManifestOrder(t *testing.T) {
	mani := manifest.Manifest{
		{Name: "first", Services: manifest.Services{{Name: "web"}, {Name: "api"}}},
		{Name: "second", Services: manifest.Services{{Name: "web"}}},
	}

	got := manifestServiceNames(mani)
	if strings.Join(got, ",") != "web,api" {
		t.Fatalf("manifest service names = %v, want [web api]", got)
	}
	if got := manifestServiceNames(nil); len(got) != 0 {
		t.Fatalf("empty manifest service names = %v, want none", got)
	}
}

func TestSubmitManifestRecordsMCPProviderActionLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deployment/41/manifest":
			if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case "/deployment/42/manifest":
			if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			http.Error(w, "provider rejected manifest", http.StatusInternalServerError)
		case "/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	logger, err := actionlog.Open(filepath.Join(t.TempDir(), "actions.log"))
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	ctx := cliutil.WithActionLog(context.Background(), logger)
	providerAddress := providerOwnerAddress()
	providerQuery := &registeredProviderStub{hostURI: srv.URL}
	client := withRegisteredProvider(newSignedLightClient(t), providerQuery)

	read, err := HandleProviderStatus(client)(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{"provider": providerAddress}},
	})
	if err != nil || read == nil || read.IsError {
		t.Fatalf("provider read result=%#v err=%v", read, err)
	}

	handler := HandleSubmitManifest(client, "jwt")
	success, err := handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"provider": providerAddress, "dseq": float64(41), "manifest_json": testManifestJSON,
	}}})
	if err != nil || success == nil || success.IsError {
		t.Fatalf("successful submit result=%#v err=%v", success, err)
	}

	failure, err := handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"provider": providerAddress, "dseq": float64(42), "manifest_json": testManifestJSON,
	}}})
	if err != nil || failure == nil || !failure.IsError {
		t.Fatalf("failed submit result=%#v err=%v", failure, err)
	}

	setupFailure, err := HandleSubmitManifest(contextLightClient{
		query: manifestQueryClient{provider: &registeredProviderStub{hostURI: srv.URL}},
	}, "jwt")(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"provider":      providerAddress,
			"dseq":          float64(43),
			"manifest_json": testManifestJSON,
		}},
	})
	if err != nil || setupFailure == nil || !setupFailure.IsError {
		t.Fatalf("gateway setup failure result=%#v err=%v", setupFailure, err)
	}

	lookupFailure, err := HandleSubmitManifest(withRegisteredProvider(
		newSignedLightClient(t),
		&registeredProviderStub{err: fmt.Errorf("provider registry unavailable")},
	), "jwt")(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"provider": providerAddress, "dseq": float64(44), "manifest_json": testManifestJSON,
	}}})
	if err != nil || lookupFailure == nil || !lookupFailure.IsError || !strings.Contains(resultText(lookupFailure), "provider registry unavailable") {
		t.Fatalf("provider lookup failure result=%#v err=%v", lookupFailure, err)
	}

	invalidProvider, err := handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"provider": "not-an-address", "dseq": float64(45), "manifest_json": testManifestJSON,
	}}})
	if err != nil || invalidProvider == nil || !invalidProvider.IsError || !strings.Contains(resultText(invalidProvider), "invalid provider address") {
		t.Fatalf("invalid provider result=%#v err=%v", invalidProvider, err)
	}

	entries, err := logger.Read(actionlog.Filter{})
	if err != nil {
		t.Fatalf("read action log: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("one provider read, four valid mutation attempts, and one invalid input logged %d entries, want exactly 4: %+v", len(entries), entries)
	}
	if got := entries[0]; got.Type != actionlog.TypeProvider || got.Action != "send-manifest" || got.Provider != providerAddress || got.DSeq != 44 || got.Status != "failed" || !strings.Contains(got.Error, "provider registry unavailable") {
		t.Errorf("provider lookup failure entry = %+v", got)
	}
	if got := entries[1]; got.Type != actionlog.TypeProvider || got.Action != "send-manifest" || got.Provider != providerAddress || got.DSeq != 43 || got.Status != "failed" || !strings.Contains(got.Error, "default account") {
		t.Errorf("gateway setup failure entry = %+v", got)
	}
	if got := entries[2]; got.Type != actionlog.TypeProvider || got.Action != "send-manifest" || got.Provider != providerAddress || got.DSeq != 42 || got.Status != "failed" || got.Error == "" {
		t.Errorf("failed provider entry = %+v", got)
	}
	if got := entries[3]; got.Type != actionlog.TypeProvider || got.Action != "send-manifest" || got.Provider != providerAddress || got.DSeq != 41 || got.Status != "success" {
		t.Errorf("successful provider entry = %+v", got)
	}
	if providerQuery.owner != providerAddress {
		t.Errorf("provider lookup owner = %q, want %q", providerQuery.owner, providerAddress)
	}
}

func TestGatewayClientRequiresSigningIdentity(t *testing.T) {
	tests := []struct {
		name string
		cl   v1beta3.LightClient
		want string
	}{
		{
			name: "account resolution",
			cl: contextLightClient{cctx: sdkclient.Context{}.
				WithFrom("missing-account")},
			want: "keyring is unavailable",
		},
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
			_, err := gatewayClient(
				context.Background(),
				tt.cl,
				"https://provider.example.com",
				sdk.AccAddress([]byte("registered-provider")),
				ajwt.PermissionDeployment{
					Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
					DSeq:     1,
					Services: []string{"web"},
				},
				"jwt",
			)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestGatewayClientRejectsUnknownContextAuthType(t *testing.T) {
	_, err := gatewayClient(
		context.Background(),
		newSignedLightClient(t),
		"https://provider.example.com",
		sdk.AccAddress([]byte("registered-provider")),
		ajwt.PermissionDeployment{
			Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
			DSeq:     1,
			Services: []string{"web"},
		},
		"password",
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported auth type") {
		t.Fatalf("error = %v, want provider auth enum rejection", err)
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
