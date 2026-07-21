package console

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

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
	payload := map[string]any{"name": name}
	if expiresAt != "" {
		payload["expiresAt"] = expiresAt
	}

	var out CreatedAPIKey
	err := c.doData(ctx, http.MethodPost, "/v1/api-keys", envelope(payload), &out)
	c.record("create-api-key", "", err)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

// DeleteAPIKey deletes an API key by ID. A missing key (404) is treated as a
// no-op and returns nil.
//
// Wire: DELETE /v1/api-keys/{id} → 204.
func (c *Client) DeleteAPIKey(ctx context.Context, id string) error {
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

	return out.Token, nil
}
