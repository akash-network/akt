package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

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
