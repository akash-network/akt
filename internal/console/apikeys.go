package console

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// MaxJWTTTLSeconds bounds Console-issued provider credentials.
const MaxJWTTTLSeconds = 3600

// ListAPIKeys lists the account's API keys. Secrets are never included.
//
// Wire: GET /v1/api-keys, response {"data":[...]}.
func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var out []APIKey
	if err := c.doData(ctx, http.MethodGet, "/v1/api-keys", nil, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// CreateAPIKey creates a new API key. expiresAt is optional (RFC 3339; empty
// means no expiry). The returned CreatedAPIKey.APIKey secret is shown exactly
// once — persist it immediately.
//
// Wire: POST /v1/api-keys, body {"data":{"name":..., "expiresAt"?:...}},
// data-enveloped response.
func (c *Client) CreateAPIKey(ctx context.Context, name, expiresAt string) (*CreatedAPIKey, error) {
	if strings.TrimSpace(name) == "" {
		err := errors.New("console: API key name must not be blank")
		c.record("create-api-key", "", err)
		return nil, err
	}

	payload := map[string]any{"name": name}
	if expiresAt != "" {
		payload["expiresAt"] = expiresAt
	}

	var out CreatedAPIKey
	err := c.doData(ctx, http.MethodPost, "/v1/api-keys", envelope(payload), &out)
	if err == nil && (strings.TrimSpace(out.ID) == "" || out.Name != name || strings.TrimSpace(out.Name) == "" || strings.TrimSpace(out.APIKey) == "") {
		err = errors.New("console: API key response omitted its nonblank ID, requested name, or one-time secret")
	}
	if err == nil {
		c.record("create-api-key", "", nil)
		return &out, nil
	}
	if definitiveCreateFailure(err) {
		c.record("create-api-key", "", err)
		return nil, err
	}

	unknown := fmt.Errorf("API key creation outcome unknown after one submission (%w); the request was not replayed because its one-time secret cannot be recovered", err)
	c.recordOutcome("create-api-key", "", "pending", unknown, map[string]string{"name": name})
	return nil, unknown
}

// DeleteAPIKey deletes an API key by ID. A missing key (404) is treated as a
// no-op and returns nil.
//
// Wire: DELETE /v1/api-keys/{id} → 204.
func (c *Client) DeleteAPIKey(ctx context.Context, id string) error {
	if _, parseErr := uuid.Parse(id); parseErr != nil {
		err := fmt.Errorf("console: API key ID must be a valid UUID: %w", parseErr)
		c.record("delete-api-key", "", err)
		return err
	}

	err := c.doJSON(ctx, http.MethodDelete, "/v1/api-keys/"+url.PathEscape(id), nil, nil)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}

	c.record("delete-api-key", "", err)

	return err
}

// CreateJWTToken mints a short-lived JWT scoped to the given lease
// permissions, for direct provider access. ttl is in seconds.
//
// Wire: POST /v1/create-jwt-token, body
// {"data":{"ttl":..., "leases":{"access":"scoped","scope":[...]}}},
// response {"data":{"token":...}}.
func (c *Client) CreateJWTToken(ctx context.Context, ttl int, scope []string) (string, error) {
	if ttl < 1 || ttl > MaxJWTTTLSeconds {
		return "", fmt.Errorf("console: JWT TTL must be between 1 and %d seconds, got %d", MaxJWTTTLSeconds, ttl)
	}

	body := envelope(map[string]any{
		"ttl": ttl,
		"leases": map[string]any{
			"access": "scoped",
			"scope":  scope,
		},
	})

	var out struct {
		Token string `json:"token"`
	}
	if err := c.doData(ctx, http.MethodPost, "/v1/create-jwt-token", body, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Token) == "" {
		return "", errors.New("console: JWT response returned a blank token")
	}

	return out.Token, nil
}
