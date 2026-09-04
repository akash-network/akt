package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	aktconsole "pkg.akt.dev/akt/internal/console"
)

func TestHandlersRejectMissingAndInvalidMutationArgumentsWithoutCallingConsole(t *testing.T) {
	tests := []struct {
		name    string
		handler func(*aktconsole.Client) mcpserver.ToolHandlerFunc
		args    map[string]any
		want    string
	}{
		{name: "get deployment", handler: HandleGetDeployment, want: "missing required parameter: dseq"},
		{name: "list bids", handler: HandleListBids, want: "missing required parameter: dseq"},
		{name: "get provider", handler: HandleGetProvider, want: "missing required parameter: address"},
		{name: "close deployment", handler: HandleCloseDeployment, want: "missing required parameter: dseq"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tc.args
			result, err := tc.handler(nil)(context.Background(), req)
			if err != nil {
				t.Fatalf("transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("result = %#v, want MCP error", result)
			}
			if text := consoleResultText(t, result); !strings.Contains(text, tc.want) {
				t.Fatalf("error result = %q, want %q", text, tc.want)
			}
		})
	}
}

func TestReadHandlersReturnConsoleDependencyFailuresAsToolErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "console unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := aktconsole.New(srv.URL, "test-key")
	tests := []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		want    string
	}{
		{name: "list deployments", handler: HandleListDeployments(client), want: "failed to list deployments"},
		{name: "wallet balance", handler: HandleWalletBalance(client), want: "failed to get wallet balance"},
		{name: "usage history", handler: HandleUsageHistory(client), want: "get user"},
		{name: "list providers", handler: HandleListProviders(client), want: "failed to list providers"},
		{name: "GPU prices", handler: HandleGPUPrices(client), want: "failed to get GPU prices"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.handler(context.Background(), mcp.CallToolRequest{})
			if err != nil {
				t.Fatalf("transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("result = %#v, want MCP error", result)
			}
			if text := consoleResultText(t, result); !strings.Contains(text, tc.want) || !strings.Contains(text, "503") {
				t.Fatalf("error result = %q", text)
			}
		})
	}

}

func TestUsageHistoryRejectsMalformedDatesBeforeConsoleRequest(t *testing.T) {
	requests := make(chan struct{}, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/user/me":
			_, _ = w.Write([]byte(`{"data":{"id":"user-1"}}`))
		case "/v1/wallets":
			_, _ = w.Write([]byte(`{"data":[{"address":"akash1wallet"}]}`))
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	client := aktconsole.New(srv.URL, "test-key")
	for _, field := range []string{"start_date", "end_date"} {
		t.Run(field, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{field: "2026-99-42"}
			result, err := HandleUsageHistory(client)(context.Background(), req)
			if err != nil {
				t.Fatalf("transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("result = %#v, want MCP error", result)
			}
			if text := consoleResultText(t, result); !strings.Contains(text, field) || !strings.Contains(text, "YYYY-MM-DD") {
				t.Fatalf("date error = %q", text)
			}
		})
	}
	select {
	case <-requests:
		t.Fatal("malformed date reached the Console API")
	default:
	}
}

func TestUsageHistoryResolvesManagedWalletAndForwardsDates(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/user/me":
			_, _ = w.Write([]byte(`{"data":{"id":"user-1"}}`))
		case "/v1/wallets":
			if got := r.URL.Query().Get("userId"); got != "user-1" {
				t.Errorf("wallet userId = %q, want user-1", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"address":""},{"address":"akash1managed"}]}`))
		case "/v1/usage/history":
			query := r.URL.Query()
			want := map[string]string{
				"address":   "akash1managed",
				"startDate": "2026-07-01",
				"endDate":   "2026-07-31",
			}
			if len(query) != len(want) {
				t.Errorf("usage query = %v, want exactly %v", query, want)
			}
			for key, value := range want {
				if got := query.Get(key); got != value {
					t.Errorf("usage query %s = %q, want %q", key, got, value)
				}
			}
			_, _ = w.Write([]byte(`[{"date":"2026-07-01","activeDeployments":2,"dailyUsdcSpent":1.25,"totalUsdcSpent":9.5}]`))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"start_date": "2026-07-01",
		"end_date":   "2026-07-31",
	}
	result, err := HandleUsageHistory(aktconsole.New(srv.URL, "test-key"))(context.Background(), req)
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("result = %#v, want usage history", result)
	}
	if got := strings.Join(paths, ","); got != "/v1/user/me,/v1/wallets,/v1/usage/history" {
		t.Fatalf("Console requests = %s, want user, wallet, then usage", got)
	}

	var got []aktconsole.UsagePoint
	if err := json.Unmarshal([]byte(consoleResultText(t, result)), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(got) != 1 || got[0].Date != "2026-07-01" || got[0].ActiveDeployments != 2 || got[0].DailyUsdcSpent != 1.25 || got[0].TotalUsdcSpent != 9.5 {
		t.Fatalf("usage result = %#v, want the Console history point", got)
	}
}

func TestUsageHistoryReportsDownstreamFailures(t *testing.T) {
	tests := []struct {
		name          string
		walletStatus  int
		walletPayload string
		usageStatus   int
		wantError     string
		wantPaths     string
	}{
		{
			name:         "wallet lookup",
			walletStatus: http.StatusServiceUnavailable,
			wantError:    "list wallets",
			wantPaths:    "/v1/user/me,/v1/wallets,/v1/wallets,/v1/wallets",
		},
		{
			name:          "no wallet address",
			walletPayload: `{"data":[{"address":""}]}`,
			wantError:     "no managed wallet with an on-chain address",
			wantPaths:     "/v1/user/me,/v1/wallets",
		},
		{
			name:          "usage request",
			walletPayload: `{"data":[{"address":"akash1managed"}]}`,
			usageStatus:   http.StatusServiceUnavailable,
			wantError:     "failed to get usage history",
			wantPaths:     "/v1/user/me,/v1/wallets,/v1/usage/history,/v1/usage/history,/v1/usage/history",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")

				switch r.URL.Path {
				case "/v1/user/me":
					_, _ = w.Write([]byte(`{"data":{"id":"user-1"}}`))
				case "/v1/wallets":
					if tc.walletStatus != 0 {
						http.Error(w, "wallets unavailable", tc.walletStatus)
						return
					}
					_, _ = w.Write([]byte(tc.walletPayload))
				case "/v1/usage/history":
					if got := r.URL.Query().Get("address"); got != "akash1managed" {
						t.Errorf("usage address = %q, want akash1managed", got)
					}
					http.Error(w, "usage unavailable", tc.usageStatus)
				default:
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			t.Cleanup(srv.Close)

			result, err := HandleUsageHistory(aktconsole.New(srv.URL, "test-key"))(
				context.Background(),
				mcp.CallToolRequest{},
			)
			if err != nil {
				t.Fatalf("transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("result = %#v, want MCP error", result)
			}
			if text := consoleResultText(t, result); !strings.Contains(text, tc.wantError) {
				t.Fatalf("error result = %q, want %q", text, tc.wantError)
			}
			if got := strings.Join(paths, ","); got != tc.wantPaths {
				t.Fatalf("Console requests = %s, want %s", got, tc.wantPaths)
			}
		})
	}
}

func TestWalletBalanceReturnsExplicitUSDFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"balance":17937977,"deployments":39437176,"total":57375153}}`))
	}))
	defer srv.Close()

	result, err := HandleWalletBalance(aktconsole.New(srv.URL, "test-key"))(
		context.Background(),
		mcp.CallToolRequest{},
	)
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("handler returned an MCP error: %#v", result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content items = %d, want 1", len(result.Content))
	}

	content, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want mcp.TextContent", result.Content[0])
	}

	var got map[string]float64
	if err := json.Unmarshal([]byte(content.Text), &got); err != nil {
		t.Fatalf("decode result: %v\n%s", err, content.Text)
	}

	want := map[string]float64{
		"available_usd": 17.937977,
		"escrow_usd":    39.437176,
		"total_usd":     57.375153,
	}
	if len(got) != len(want) {
		t.Fatalf("result fields = %#v, want exactly %#v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %v, want %v", key, got[key], value)
		}
	}
}

func consoleResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("content = %#v, want one item", result.Content)
	}
	content, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T", result.Content[0])
	}
	return content.Text
}
