package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chaincli "pkg.akt.dev/akt/internal/cli/chain"
	aktctx "pkg.akt.dev/akt/internal/context"
)

func TestConsoleDefaultOwnerResolverUsesManagedWallet(t *testing.T) {
	const address = "akash1gnz8venxvenxvenxvenxvenxvenxvenx4m3e0y"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/user/me":
			_, _ = w.Write([]byte(`{"data":{"id":"user-1"}}`))
		case "/v1/wallets":
			if got := r.URL.Query().Get("userId"); got != "user-1" {
				t.Errorf("wallet userId = %q, want user-1", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"address":"` + address + `","denom":"uact"}]}`))
		default:
			t.Errorf("unexpected request %s", r.URL.RequestURI())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ctx := withConsoleDefaultOwnerResolver(context.Background(), &aktctx.Context{
		ConsoleAPIURL: srv.URL,
		ConsoleAPIKey: "secret",
	})
	resolver, ok := ctx.Value(chaincli.ContextTypeDefaultOwnerResolver).(chaincli.DefaultOwnerResolver)
	if !ok {
		t.Fatal("Console owner resolver was not attached")
	}
	got, err := resolver(ctx)
	if err != nil {
		t.Fatalf("resolve managed wallet: %v", err)
	}
	if got != address {
		t.Fatalf("managed wallet = %q, want %q", got, address)
	}
}

func TestConsoleDefaultOwnerResolverReportsBoundaryFailures(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "user request",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "offline", http.StatusBadGateway)
			},
			want: "managed wallet user",
		},
		{
			name: "missing user id",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":{"id":""}}`))
			},
			want: "omitted user ID",
		},
		{
			name: "wallet request",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/user/me" {
					_, _ = w.Write([]byte(`{"data":{"id":"user-1"}}`))
					return
				}
				http.Error(w, "offline", http.StatusBadGateway)
			},
			want: "managed wallets",
		},
		{
			name: "no wallet address",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/user/me" {
					_, _ = w.Write([]byte(`{"data":{"id":"user-1"}}`))
					return
				}
				_, _ = w.Write([]byte(`{"data":[{"address":" "}]}`))
			},
			want: "no managed wallet address",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(test.handler)
			defer srv.Close()
			ctx := withConsoleDefaultOwnerResolver(context.Background(), &aktctx.Context{
				ConsoleAPIURL: srv.URL,
				ConsoleAPIKey: "secret",
			})
			resolver := ctx.Value(chaincli.ContextTypeDefaultOwnerResolver).(chaincli.DefaultOwnerResolver)
			_, err := resolver(ctx)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("resolver error = %v, want %q", err, test.want)
			}
		})
	}
}
