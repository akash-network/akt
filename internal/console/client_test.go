package console_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

func TestCreateDeployment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/deployments", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "test-sdl", body["sdl"])
		assert.InDelta(t, 5.0, body["deposit"], 0.001)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(console.DeploymentResponse{
			DSeq: "12345",
		})
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.CreateDeployment(context.Background(), "test-sdl", 5.0)
	require.NoError(t, err)
	assert.Equal(t, "12345", resp.DSeq)
}

func TestListDeployments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/deployments", r.URL.Path)
		assert.Equal(t, "0", r.URL.Query().Get("skip"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(console.DeploymentListResponse{
			Data: []console.DeploymentResponse{
				{DSeq: "100"},
				{DSeq: "200"},
			},
		})
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.ListDeployments(context.Background(), 0, 10)
	require.NoError(t, err)
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "100", resp.Data[0].DSeq)
	assert.Equal(t, "200", resp.Data[1].DSeq)
}

func TestCloseDeployment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v1/deployments/99999", r.URL.Path)

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	err := c.CloseDeployment(context.Background(), "99999")
	require.NoError(t, err)
}

func TestAPIKeyHeader(t *testing.T) {
	const expectedKey = "my-secret-key"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, expectedKey, r.Header.Get("x-api-key"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(console.DeploymentResponse{DSeq: "1"})
	}))
	defer srv.Close()

	c := console.New(srv.URL, expectedKey)

	// Test across different methods.
	_, err := c.GetDeployment(context.Background(), "1")
	require.NoError(t, err)

	_, err = c.CreateDeployment(context.Background(), "sdl", 1.0)
	require.NoError(t, err)
}

func TestErrorHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "bad-key")
	_, err := c.GetDeployment(context.Background(), "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
}

func TestRetryOn5xx(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(console.DeploymentResponse{DSeq: "42"})
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	resp, err := c.GetDeployment(context.Background(), "42")
	require.NoError(t, err)
	assert.Equal(t, "42", resp.DSeq)
	assert.Equal(t, int32(2), calls.Load(), "expected exactly 2 calls (1 retry)")
}
