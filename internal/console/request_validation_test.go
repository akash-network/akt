package console_test

import (
	"context"
	"math"
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
			_, err := client.CreateDeployment(context.Background(), validUpdateSDL, 0.49)
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

func TestDeploymentStateListAndMutationValidationEdges(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer srv.Close()
	client := console.New(srv.URL, "test-key")

	tests := []struct {
		name string
		call func() error
	}{
		{name: "state skip", call: func() error {
			_, err := client.ListDeploymentsByState(context.Background(), "active", -1, 1)
			return err
		}},
		{name: "state limit", call: func() error {
			_, err := client.ListDeploymentsByState(context.Background(), "active", 0, 0)
			return err
		}},
		{name: "state value", call: func() error {
			_, err := client.ListDeploymentsByState(context.Background(), "pending", 0, 1)
			return err
		}},
		{name: "update dseq", call: func() error {
			_, err := client.UpdateDeployment(context.Background(), "bad", validUpdateSDL)
			return err
		}},
		{name: "close dseq", call: func() error {
			return client.CloseDeployment(context.Background(), "bad")
		}},
		{name: "deposit dseq", call: func() error {
			return client.Deposit(context.Background(), "bad", 1)
		}},
		{name: "deposit nan", call: func() error {
			return client.Deposit(context.Background(), "1", math.NaN())
		}},
		{name: "deposit sub-micro precision", call: func() error {
			return client.Deposit(context.Background(), "1", 0.5000001)
		}},
		{name: "get settings dseq", call: func() error {
			_, err := client.GetDeploymentSettings(context.Background(), "bad")
			return err
		}},
		{name: "set settings dseq", call: func() error {
			_, err := client.SetDeploymentAutoTopUp(context.Background(), "bad", true)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, test.call())
		})
	}
	assert.Zero(t, requests.Load(), "validation failures must not reach Console")
}

func TestDeploymentStateListBoundsAndRequestFailure(t *testing.T) {
	t.Run("page bounds", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":{"deployments":[{"deployment":{"id":{"dseq":"1"},"state":"active"}}],"pagination":{"hasMore":false}}}`))
		}))
		defer srv.Close()
		result, err := console.New(srv.URL, "test-key").ListDeploymentsByState(context.Background(), " ACTIVE ", 10, 5)
		require.NoError(t, err)
		assert.Empty(t, result.Deployments)
		assert.Equal(t, 1, result.Pagination.Total)
	})

	t.Run("collection request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "offline", http.StatusBadGateway)
		}))
		defer srv.Close()
		_, err := console.New(srv.URL, "test-key").ListDeploymentsByState(context.Background(), "active", 0, 1)
		require.Error(t, err)
	})

	t.Run("mutable preflight request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "offline", http.StatusBadGateway)
		}))
		defer srv.Close()
		_, err := console.New(srv.URL, "test-key").SetDeploymentAutoTopUp(context.Background(), "42", true)
		require.ErrorContains(t, err, "preflight deployment settings")
	})
}
