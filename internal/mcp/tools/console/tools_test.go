package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	aktconsole "pkg.akt.dev/akt/internal/console"
)

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
		"available_usd":      17.937977,
		"in_deployments_usd": 39.437176,
		"total_usd":          57.375153,
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
