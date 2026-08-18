package console_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

func TestConsoleRequestBoundariesRejectInvalidValuesBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer srv.Close()

	client := console.New(srv.URL, "test-key")
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{name: "negative pagination", call: func() error {
			_, err := client.ListDeployments(context.Background(), -1, 20)
			return err
		}, want: "skip must be non-negative"},
		{name: "zero page size", call: func() error {
			_, err := client.ListDeployments(context.Background(), 0, 0)
			return err
		}, want: "limit must be greater than zero"},
		{name: "deployment dseq", call: func() error {
			_, err := client.GetDeployment(context.Background(), "not-a-number")
			return err
		}, want: "invalid dseq"},
		{name: "bid dseq", call: func() error {
			_, err := client.FetchBids(context.Background(), "not-a-number")
			return err
		}, want: "invalid dseq"},
		{name: "settings dseq", call: func() error {
			_, err := client.GetDeploymentSettings(context.Background(), "0")
			return err
		}, want: "invalid dseq"},
		{name: "lease dseq", call: func() error {
			_, err := client.CreateLease(context.Background(), "[]", []console.LeaseRequest{{
				DSeq: "bad", GSeq: 1, OSeq: 1, Provider: "akash1provider",
			}})
			return err
		}, want: "invalid dseq"},
		{name: "create deposit", call: func() error {
			_, err := client.CreateDeployment(context.Background(), validUpdateSDL, 0.499999)
			return err
		}, want: "at least $0.50"},
		{name: "escrow deposit", call: func() error {
			return client.Deposit(context.Background(), "123", 0.01)
		}, want: "at least $0.50"},
		{name: "api key uuid", call: func() error {
			return client.DeleteAPIKey(context.Background(), "")
		}, want: "valid UUID"},
		{name: "jwt zero ttl", call: func() error {
			_, err := client.CreateJWTToken(context.Background(), 0, []string{"logs"})
			return err
		}, want: "between 1 and 3600"},
		{name: "jwt excessive ttl", call: func() error {
			_, err := client.CreateJWTToken(context.Background(), 3601, []string{"logs"})
			return err
		}, want: "between 1 and 3600"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, test.call(), test.want)
		})
	}
	assert.Zero(t, requests.Load(), "invalid requests must not reach Console")
}

func TestConsoleDeploymentMutationsRejectClosedState(t *testing.T) {
	tests := []struct {
		name string
		call func(*console.Client) error
	}{
		{name: "update", call: func(client *console.Client) error {
			_, err := client.UpdateDeployment(context.Background(), "42", validUpdateSDL)
			return err
		}},
		{name: "deposit", call: func(client *console.Client) error {
			return client.Deposit(context.Background(), "42", 0.50)
		}},
		{name: "settings", call: func(client *console.Client) error {
			_, err := client.SetDeploymentAutoTopUp(context.Background(), "42", false)
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/deployments/42", r.URL.Path)
				_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"},"state":"closed"}}}`))
			}))
			defer srv.Close()

			err := test.call(console.New(srv.URL, "test-key"))
			require.ErrorContains(t, err, "is closed")
			assert.Equal(t, int32(1), requests.Load(), "closed mutation must stop after state preflight")
		})
	}
}
