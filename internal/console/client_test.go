package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

// decodeDataBody unmarshals a {"data": ...} request body and returns the
// inner payload, failing the test on malformed envelopes.
func decodeDataBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
	require.NotNil(t, body.Data, "request body must be data-enveloped")

	return body.Data
}

func TestAPIKeyHeader(t *testing.T) {
	const expectedKey = "my-secret-key"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, expectedKey, r.Header.Get("x-api-key"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"1"}}}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, expectedKey)

	_, err := c.GetDeployment(context.Background(), "1")
	require.NoError(t, err)
}

func TestNoAPIKeyHeaderWhenUnset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present := r.Header["X-Api-Key"]
		assert.False(t, present, "x-api-key must not be sent when no key is configured")

		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "")
	_, err := c.ListProviders(context.Background(), "", nil)
	require.NoError(t, err)
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		sentinel error
	}{
		{"unauthorized", http.StatusUnauthorized, console.ErrUnauthorized},
		{"insufficient funds", http.StatusPaymentRequired, console.ErrInsufficientFunds},
		{"not found", http.StatusNotFound, console.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := console.New(srv.URL, "key")
			_, err := c.GetDeployment(context.Background(), "1")
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.sentinel)
		})
	}
}

func TestUnexpectedStatusReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"conflict"}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")
	_, err := c.GetDeployment(context.Background(), "1")
	require.Error(t, err)

	var httpErr *console.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
	assert.Contains(t, httpErr.Body, "conflict")
}

func TestRetryOn429ThenSuccess(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"owner":"akash1x","dseq":"42"}}}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")
	resp, err := c.GetDeployment(context.Background(), "42")
	require.NoError(t, err)
	assert.Equal(t, "42", resp.Deployment.ID.DSeq.String())
	assert.Equal(t, int32(2), calls.Load(), "expected exactly 2 calls (1 retry)")
}

func TestRetryOn5xxExhausted(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")
	_, err := c.GetDeployment(context.Background(), "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server error")
	assert.Equal(t, int32(3), calls.Load(), "expected max 3 attempts")
}

func TestDataEnvelopeUnwrapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"balance":1500000,"deployments":500000,"total":2000000}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")
	b, err := c.GetBalances(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1500000), b.Balance)
	assert.InDelta(t, 1.5, b.BalanceUSD(), 1e-9)
	assert.InDelta(t, 2.0, b.TotalUSD(), 1e-9)
}

func TestNoRetryAfterUnexpectedStatus(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")
	_, err := c.GetDeployment(context.Background(), "1")
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "4xx (non-429) must not retry")

	var httpErr *console.HTTPError
	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusTeapot, httpErr.StatusCode)
}

func TestFlexStringAcceptsNumbers(t *testing.T) {
	var v struct {
		DSeq console.FlexString `json:"dseq"`
	}

	require.NoError(t, json.Unmarshal([]byte(`{"dseq":123456}`), &v))
	assert.Equal(t, "123456", v.DSeq.String())

	require.NoError(t, json.Unmarshal([]byte(`{"dseq":"7890"}`), &v))
	assert.Equal(t, "7890", v.DSeq.String())
}
