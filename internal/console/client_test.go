package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestUnexpectedStatusRedactsEchoedAPIKey(t *testing.T) {
	const apiKey = "credential-that-must-not-escape"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"request used ` + apiKey + ` twice: ` + apiKey + `"}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, apiKey)
	_, err := c.GetDeployment(context.Background(), "1")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), apiKey)

	var httpErr *console.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.NotContains(t, httpErr.Body, apiKey)
	assert.Equal(t, 2, strings.Count(httpErr.Body, "[REDACTED]"))
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

func TestPostNotRetriedOn5xx(t *testing.T) {
	// A 5xx does not prove the server failed to process the request (a
	// gateway 502 can hide a completed write). Replaying a POST could
	// duplicate a deployment or charge the managed wallet twice, so POST
	// must do exactly one attempt and surface the error.
	var posts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"123"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"1000000"}],"transferred":[]}}}}`))
			if posts.Load() > 0 {
				cancel()
			}
			return
		}
		assert.Equal(t, http.MethodPost, r.Method)
		posts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")
	err := c.Deposit(ctx, "123", 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server error")
	assert.Equal(t, int32(1), posts.Load(), "POST + 5xx must not be retried")
}

func TestPostNotRetriedOn429(t *testing.T) {
	// A rate-limit response does not prove that a non-idempotent request was
	// never processed. Replaying it could duplicate a deployment or charge.
	var posts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"123"}},"leases":[],"escrow_account":{"state":{"funds":[{"denom":"uact","amount":"1000000"}],"transferred":[]}}}}`))
			if posts.Load() > 0 {
				cancel()
			}
			return
		}
		assert.Equal(t, http.MethodPost, r.Method)
		posts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")
	err := c.Deposit(ctx, "123", 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
	assert.Equal(t, int32(1), posts.Load(), "POST + 429 must not be replayed")
}

func TestDeleteRetriedOn5xx(t *testing.T) {
	// DELETE is idempotent by HTTP semantics: replaying it on 5xx is safe.
	var deletes atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"1"},"state":"active"}}}`))
		case http.MethodDelete:
			if deletes.Add(1) < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"success":true}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")
	require.NoError(t, c.CloseDeployment(context.Background(), "1"))
	assert.Equal(t, int32(3), deletes.Load(), "DELETE + 5xx must retry")
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

func TestMissingDataEnvelopeIsAnError(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "missing", body: `{"ok":true}`},
		{name: "null", body: `{"data":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A success-shaped response that omits or nulls the envelope must
			// not read as a successful call with a zero-valued result.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, err := console.New(srv.URL, "k").GetUser(context.Background())
			if err == nil {
				t.Fatal("a response without a non-null data envelope must be an error")
			}
			if !strings.Contains(err.Error(), "no data envelope") {
				t.Errorf("error %q should name the unusable envelope", err)
			}
		})
	}
}

func TestEmptyBodyIsAnErrorWhenResultExpected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "k").GetUsageHistory(context.Background(), "akash1x", "", "")
	if err == nil {
		t.Fatal("an empty successful response with an expected result must be an error")
	}
	if !strings.Contains(err.Error(), "empty response body") {
		t.Errorf("error %q should identify the empty success body", err)
	}
}

func TestEmptyDeleteBodyIsFineWhenNoResultExpected(t *testing.T) {
	// Endpoints that return no payload (e.g. DELETE) pass a nil result and
	// must keep succeeding on an empty body.
	const id = "11111111-1111-4111-8111-111111111111"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":[{"id":"` + id + `","name":"ci"}]}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := console.New(srv.URL, "k").DeleteAPIKey(context.Background(), id); err != nil {
		t.Fatalf("empty body with no expected result must succeed: %v", err)
	}
}
