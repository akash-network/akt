package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	protocol "github.com/mark3labs/mcp-go/mcp"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	v1beta3 "pkg.akt.dev/go/node/client/v1beta3"

	aktcodec "pkg.akt.dev/akt/internal/codec"
	aktconsole "pkg.akt.dev/akt/internal/console"
	aktkeyring "pkg.akt.dev/akt/internal/keyring"
)

type boundaryClient struct{}

func (boundaryClient) Query() v1beta3.QueryClient       { return nil }
func (boundaryClient) Node() v1beta3.NodeClient         { return nil }
func (boundaryClient) Tx() v1beta3.TxClient             { return nil }
func (boundaryClient) ClientContext() sdkclient.Context { return sdkclient.Context{} }
func (boundaryClient) PrintMessage(interface{}) error   { return nil }
func (boundaryClient) PrintJSON(interface{}) error      { return nil }

func chainQueryToolNames() []string {
	return []string{
		"akash_account_balance",
		"akash_block_height",
		"akash_get_bid",
		"akash_get_deployment",
		"akash_get_group",
		"akash_get_lease",
		"akash_get_order",
		"akash_get_provider",
		"akash_lease_status",
		"akash_list_audited_providers",
		"akash_list_bids",
		"akash_list_certificates",
		"akash_list_deployments",
		"akash_list_leases",
		"akash_list_orders",
		"akash_list_providers",
		"akash_node_status",
		"akash_provider_status",
		"akash_service_status",
	}
}

func chainWriteToolNames() []string {
	return []string{
		"akash_close_deployment",
		"akash_close_lease",
		"akash_create_lease",
		"akash_submit_manifest",
	}
}

func consoleQueryToolNames() []string {
	return []string{
		"console_get_deployment",
		"console_get_provider",
		"console_gpu_prices",
		"console_list_bids",
		"console_list_deployments",
		"console_list_providers",
		"console_usage_history",
		"console_wallet_balance",
	}
}

func consoleWriteToolNames() []string {
	return []string{"console_close_deployment", "console_deposit"}
}

func TestNewRegistersExactToolsForEachAvailableRail(t *testing.T) {
	consoleClient := aktconsole.New("http://console.invalid", "test-key")
	chainContext := sdkclient.Context{}.WithNodeURI("tcp://127.0.0.1:1")
	encoding := aktcodec.MakeEncodingConfig()
	keyringChainContext := chainContext.
		WithCodec(encoding.Codec).
		WithInterfaceRegistry(encoding.InterfaceRegistry).
		WithTxConfig(encoding.TxConfig).
		WithLegacyAmino(encoding.Amino).
		WithKeyring(aktkeyring.NewInMemory(encoding.Codec)).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithBroadcastMode("sync").
		WithSignModeStr(flags.SignModeDirect)

	tests := []struct {
		name          string
		clientContext sdkclient.Context
		consoleClient *aktconsole.Client
		enableWrites  bool
		want          []string
	}{
		{
			name:          "chain read only",
			clientContext: chainContext,
			want:          chainQueryToolNames(),
		},
		{
			name:          "console read only",
			consoleClient: consoleClient,
			want:          consoleQueryToolNames(),
		},
		{
			name:          "console writes enabled",
			consoleClient: consoleClient,
			enableWrites:  true,
			want:          append(consoleQueryToolNames(), consoleWriteToolNames()...),
		},
		{
			name:          "combined read only",
			clientContext: chainContext,
			consoleClient: consoleClient,
			want:          append(chainQueryToolNames(), consoleQueryToolNames()...),
		},
		{
			name:          "chain writes require a local keyring",
			clientContext: chainContext,
			consoleClient: consoleClient,
			enableWrites:  true,
			want: append(
				append(chainQueryToolNames(), consoleQueryToolNames()...),
				consoleWriteToolNames()...,
			),
		},
		{
			name:          "keyring context enables every write rail",
			clientContext: keyringChainContext,
			consoleClient: consoleClient,
			enableWrites:  true,
			want: append(
				append(
					append(chainQueryToolNames(), chainWriteToolNames()...),
					consoleQueryToolNames()...,
				),
				consoleWriteToolNames()...,
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, err := New(context.Background(), tc.clientContext, "jwt", tc.enableWrites, tc.consoleClient)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			registered := srv.mcp.ListTools()
			got := make([]string, 0, len(registered))
			for name := range registered {
				got = append(got, name)
			}
			sort.Strings(got)
			sort.Strings(tc.want)
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("registered tools = %v, want %v", got, tc.want)
			}
			if len(srv.schemas) != len(registered) {
				t.Fatalf("argument schemas = %d, registered tools = %d", len(srv.schemas), len(registered))
			}

			for _, name := range got {
				entry := registered[name]
				if entry == nil {
					t.Fatalf("registered tool %s is nil", name)
				}
				if entry.Tool.Description == "" {
					t.Errorf("%s has no description", name)
				}
				if entry.Tool.InputSchema.Type != "object" {
					t.Errorf("%s input schema type = %q, want object", name, entry.Tool.InputSchema.Type)
				}
				additional, ok := entry.Tool.InputSchema.AdditionalProperties.(bool)
				if !ok || additional {
					t.Errorf("%s input schema permits undeclared arguments: %#v", name, entry.Tool.InputSchema.AdditionalProperties)
				}
				if _, ok := srv.schemas[name]; !ok {
					t.Errorf("%s has no argument-validation schema", name)
				}
			}
		})
	}
}

func TestRegisteredToolsRejectInvalidArgumentsBeforeDependencies(t *testing.T) {
	consoleAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "backend should not be reached by an invalid call", http.StatusInternalServerError)
	}))
	defer consoleAPI.Close()

	consoleClient := aktconsole.New(consoleAPI.URL, "test-key")
	srv, err := New(
		context.Background(),
		sdkclient.Context{}.WithNodeURI("tcp://127.0.0.1:1"),
		"jwt",
		false,
		consoleClient,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Build the full registry without constructing a signing client. Every
	// call below is invalid before a dependency or transaction can be used.
	srv.registerWriteTools(context.Background(), boundaryClient{}, "jwt")
	srv.registerConsoleWriteTools(consoleClient)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "akash_account_balance", args: map[string]any{"address": true}, want: "address must be a string"},
		{name: "akash_list_deployments", args: map[string]any{"state": "pending"}, want: "state must be one of active, closed"},
		{name: "akash_get_deployment", args: map[string]any{"owner": "akash1owner"}, want: "missing required parameter: dseq"},
		{name: "akash_get_group", args: map[string]any{"owner": "akash1owner", "dseq": float64(1)}, want: "missing required parameter: gseq"},
		{name: "akash_list_orders", args: map[string]any{"state": "pending"}, want: "state must be one of open, active, closed"},
		{name: "akash_get_order", args: map[string]any{}, want: "missing required parameter: owner"},
		{name: "akash_list_bids", args: map[string]any{"owner": "akash1owner", "dseq": float64(0)}, want: "dseq must be greater than or equal to 1"},
		{name: "akash_get_bid", args: map[string]any{}, want: "missing required parameter: owner"},
		{name: "akash_list_leases", args: map[string]any{"owner": "akash1owner", "dseq": 1.5}, want: "dseq must be a whole number"},
		{name: "akash_get_lease", args: map[string]any{}, want: "missing required parameter: owner"},
		{name: "akash_get_provider", args: map[string]any{}, want: "missing required parameter: owner"},
		{name: "akash_provider_status", args: map[string]any{}, want: "missing required parameter: provider"},
		{name: "akash_lease_status", args: map[string]any{}, want: "missing required parameter: provider"},
		{name: "akash_service_status", args: map[string]any{}, want: "missing required parameter: provider"},
		{name: "akash_provider_status", args: map[string]any{"provider": "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", "provider_url": "https://spoofed.example.test"}, want: "unknown parameter: provider_url"},
		{name: "akash_lease_status", args: map[string]any{"provider": "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", "provider_url": "https://spoofed.example.test", "dseq": float64(1), "gseq": float64(1), "oseq": float64(1)}, want: "unknown parameter: provider_url"},
		{name: "akash_service_status", args: map[string]any{"provider": "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", "provider_url": "https://spoofed.example.test", "dseq": float64(1), "gseq": float64(1), "oseq": float64(1), "service": "web"}, want: "unknown parameter: provider_url"},
		{name: "akash_list_audited_providers", args: map[string]any{"owner": float64(1)}, want: "owner must be a string"},
		{name: "akash_list_certificates", args: map[string]any{"state": "expired"}, want: "state must be one of valid, revoked"},
		{name: "akash_close_deployment", args: map[string]any{}, want: "missing required parameter: dseq"},
		{name: "akash_create_lease", args: map[string]any{}, want: "missing required parameter: owner"},
		{name: "akash_close_lease", args: map[string]any{}, want: "missing required parameter: owner"},
		{name: "akash_submit_manifest", args: map[string]any{"provider": "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", "dseq": float64(1), "manifest_json": "{"}, want: "invalid manifest JSON"},
		{name: "akash_submit_manifest", args: map[string]any{"provider": "akash19sk2chd930sa5lxeg3wdjmdgk0e9tnn7qhw3rk", "provider_url": "https://spoofed.example.test", "dseq": float64(1), "manifest_json": "[]"}, want: "unknown parameter: provider_url"},
		{name: "console_list_deployments", args: map[string]any{"skip": float64(-1)}, want: "skip must be greater than or equal to 0"},
		{name: "console_get_deployment", args: map[string]any{}, want: "missing required parameter: dseq"},
		{name: "console_list_bids", args: map[string]any{}, want: "missing required parameter: dseq"},
		{name: "console_usage_history", args: map[string]any{"start_date": float64(1)}, want: "start_date must be a string"},
		{name: "console_list_providers", args: map[string]any{"scope": "active"}, want: "scope must be one of all, trial"},
		{name: "console_get_provider", args: map[string]any{}, want: "missing required parameter: address"},
		{name: "console_close_deployment", args: map[string]any{}, want: "missing required parameter: dseq"},
		{name: "console_deposit", args: map[string]any{"dseq": "1", "amount_usd": 0.01}, want: "amount_usd must be greater than or equal to 0.5"},
		{name: "akash_account_balance", args: map[string]any{"unknown": "akash1typo"}, want: "unknown parameter: unknown"},
		{name: "akash_get_deployment", args: map[string]any{"owner": "akash1owner", "dseq": float64(9007199254740992)}, want: "dseq must be less than or equal to 9007199254740991"},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := callRegisteredTool(t, srv, i+1, tc.name, tc.args)
			if result.rpcError != nil {
				t.Fatalf("JSON-RPC error = %d %s", result.rpcError.Code, result.rpcError.Message)
			}
			if !result.IsError {
				t.Fatalf("invalid call returned success: %s", result.text())
			}
			if !strings.Contains(result.text(), tc.want) {
				t.Fatalf("result = %q, want error containing %q", result.text(), tc.want)
			}
		})
	}
}

func TestRegisteredToolNormalizesBlankOptionalStringBeforeHandler(t *testing.T) {
	var observedScope string
	consoleAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedScope = r.URL.Query().Get("scope")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer consoleAPI.Close()

	srv, err := New(context.Background(), sdkclient.Context{}, "jwt", false, aktconsole.New(consoleAPI.URL, "test-key"))
	if err != nil {
		t.Fatal(err)
	}
	result := callRegisteredTool(t, srv, 1, "console_list_providers", map[string]any{"scope": " \t"})
	if result.rpcError != nil || result.IsError {
		t.Fatalf("blank optional scope failed: rpc=%v text=%q", result.rpcError, result.text())
	}
	if observedScope != "" {
		t.Fatalf("handler forwarded blank optional scope %q", observedScope)
	}
}

type registeredToolResult struct {
	IsError bool `json:"isError"`
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	rpcError *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
}

func (result registeredToolResult) text() string {
	var text strings.Builder
	for _, content := range result.Content {
		text.WriteString(content.Text)
	}
	return text.String()
}

func callRegisteredTool(t *testing.T, srv *Server, id int, name string, args map[string]any) registeredToolResult {
	t.Helper()

	request, err := json.Marshal(map[string]any{
		"jsonrpc": protocol.JSONRPC_VERSION,
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": args,
		},
	})
	if err != nil {
		t.Fatalf("marshal %s request: %v", name, err)
	}

	response := srv.mcp.HandleMessage(context.Background(), request)
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal %s response: %v", name, err)
	}

	var envelope struct {
		Result registeredToolResult `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("decode %s response: %v\n%s", name, err, encoded)
	}
	envelope.Result.rpcError = envelope.Error
	if envelope.Result.rpcError != nil && envelope.Result.rpcError.Message == "" {
		t.Fatalf("%s returned an empty JSON-RPC error: %s", name, string(encoded))
	}

	return envelope.Result
}
