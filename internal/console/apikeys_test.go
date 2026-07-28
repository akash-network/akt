package console_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

func TestListAPIKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/api-keys", r.URL.Path)

		_, _ = w.Write([]byte(`{"data":[
			{"id":"key-1","name":"ci","createdAt":"2026-01-01T00:00:00Z","lastUsedAt":"2026-07-20T10:00:00Z","keyFormat":"ak-****"},
			{"id":"key-2","name":"laptop","expiresAt":"2027-01-01T00:00:00Z"}
		]}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	keys, err := c.ListAPIKeys(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 2)
	assert.Equal(t, "ci", keys[0].Name)
	assert.Equal(t, "ak-****", keys[0].KeyFormat)
	assert.Equal(t, "2027-01-01T00:00:00Z", keys[1].ExpiresAt)
}

func TestCreateAPIKeyReturnsSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/api-keys", r.URL.Path)

		data := decodeDataBody(t, r)
		assert.Equal(t, "ci-key", data["name"])
		assert.NotContains(t, data, "expiresAt", "empty expiresAt must be omitted")

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"key-9","name":"ci-key","apiKey":"ak-live-supersecret"}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	created, err := c.CreateAPIKey(context.Background(), "ci-key", "")
	require.NoError(t, err)
	assert.Equal(t, "key-9", created.ID)
	assert.Equal(t, "ak-live-supersecret", created.APIKey, "secret is only returned at creation")
}

func TestCreateAPIKeyWithExpiry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := decodeDataBody(t, r)
		assert.Equal(t, "2027-01-01T00:00:00Z", data["expiresAt"])

		_, _ = w.Write([]byte(`{"data":{"id":"key-10","name":"temp","apiKey":"ak-live-x"}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	_, err := c.CreateAPIKey(context.Background(), "temp", "2027-01-01T00:00:00Z")
	require.NoError(t, err)
}

func TestDeleteAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v1/api-keys/key-1", r.URL.Path)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	require.NoError(t, c.DeleteAPIKey(context.Background(), "key-1"))
}

func TestDeleteAPIKeyNotFoundIsNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	assert.NoError(t, c.DeleteAPIKey(context.Background(), "missing"), "404 on delete is a no-op")
}

func TestCreateJWTToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/create-jwt-token", r.URL.Path)

		data := decodeDataBody(t, r)
		assert.InDelta(t, 900.0, data["ttl"], 0.001)

		leases, ok := data["leases"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "scoped", leases["access"])
		assert.Equal(t, []any{"send-manifest", "logs"}, leases["scope"])

		_, _ = w.Write([]byte(`{"data":{"token":"eyJhbGciOi..."}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	token, err := c.CreateJWTToken(context.Background(), 900, []string{"send-manifest", "logs"})
	require.NoError(t, err)
	assert.Equal(t, "eyJhbGciOi...", token)
}
