package console

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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
	return c.CreateAPIKeyForProject(ctx, name, expiresAt, "")
}

// CreateAPIKeyForProject creates an API key limited to one project of the
// active organization; an empty projectID creates one that reaches every
// project its owner can. The key acts in the organization it is created in.
//
// Wire: POST /v1/api-keys, body
// {"data":{"name":..., "expiresAt"?:..., "projectId"?:...}}.
func (c *Client) CreateAPIKeyForProject(ctx context.Context, name, expiresAt, projectID string) (*CreatedAPIKey, error) {
	if strings.TrimSpace(name) == "" {
		err := errors.New("console: API key name must not be blank")
		c.record("create-api-key", "", err)
		return nil, err
	}

	payload := map[string]any{"name": name}
	if expiresAt != "" {
		payload["expiresAt"] = expiresAt
	}
	if projectID != "" {
		id, err := projectPathID(projectID)
		if err != nil {
			c.record("create-api-key", "", err)
			return nil, err
		}
		payload["projectId"] = id
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

// DeleteAPIKey deletes an API key by ID. Console's DELETE endpoint is
// idempotent, so the client first proves the key is present in the account's
// key list. A missing key is returned as ErrNotFound without sending DELETE,
// ensuring callers never report a deletion the API did not perform.
//
// Wire: DELETE /v1/api-keys/{id} → 204.
func (c *Client) DeleteAPIKey(ctx context.Context, id string) error {
	parsedID, parseErr := uuid.Parse(id)
	if parseErr != nil {
		err := fmt.Errorf("console: API key ID must be a valid UUID: %w", parseErr)
		c.record("delete-api-key", "", err)
		return err
	}

	keys, err := c.ListAPIKeys(ctx)
	if err != nil {
		err = fmt.Errorf("console: preflight API key deletion: %w", err)
		c.record("delete-api-key", "", err)
		return err
	}

	found := false
	for _, key := range keys {
		keyID, keyErr := uuid.Parse(key.ID)
		if keyErr == nil && keyID == parsedID {
			found = true
			break
		}
	}
	if !found {
		err = fmt.Errorf("%w: API key %s", ErrNotFound, parsedID.String())
		c.record("delete-api-key", "", err)
		return err
	}

	err = c.doJSON(ctx, http.MethodDelete, "/v1/api-keys/"+url.PathEscape(parsedID.String()), nil, nil)
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

	return c.mintJWTToken(ctx, ttl, map[string]any{
		"access": "scoped",
		"scope":  scope,
	})
}

// everyService names all of a deployment's services in a granular grant. The
// token schema requires a non-empty service list, while providers authorize
// a deployment grant by dseq and scope alone; Console's web app sends the
// same value.
const everyService = "*"

// CreateDeploymentJWTToken mints a short-lived JWT for one deployment's
// leases on one provider, for direct provider access. Inside a team, an API
// key limited to some projects (a project-bound key, a selected project, or a
// member's or viewer's key) may only mint tokens that name their deployments
// this way. Without a selected project the client first asks for the broader
// scoped token, which every key without teams uses, and falls back to the
// deployment grant only when Console refuses it with project_forbidden.
//
// Wire: POST /v1/create-jwt-token, body {"data":{"ttl":..., "leases":
// {"access":"granular","permissions":[{"provider":...,"access":"granular",
// "deployments":[{"dseq":N,"scope":[...],"services":["*"]}]}]}}}.
func (c *Client) CreateDeploymentJWTToken(ctx context.Context, ttl int, provider, dseq string, scope []string) (string, error) {
	if err := validateDSeq(dseq); err != nil {
		return "", err
	}
	dseqNumber, _ := strconv.ParseUint(dseq, 10, 64)
	if strings.TrimSpace(provider) == "" {
		return "", errors.New("console: provider address must not be blank")
	}

	if c.Project() == "" {
		token, err := c.CreateJWTToken(ctx, ttl, scope)
		var refusal *HTTPError
		if err == nil || !errors.As(err, &refusal) || refusal.Code != CodeProjectForbidden {
			return token, err
		}
	}

	if ttl < 1 || ttl > MaxJWTTTLSeconds {
		return "", fmt.Errorf("console: JWT TTL must be between 1 and %d seconds, got %d", MaxJWTTTLSeconds, ttl)
	}

	return c.mintJWTToken(ctx, ttl, map[string]any{
		"access": "granular",
		"permissions": []any{map[string]any{
			"provider": provider,
			"access":   "granular",
			"deployments": []any{map[string]any{
				"dseq":     dseqNumber,
				"scope":    scope,
				"services": []string{everyService},
			}},
		}},
	})
}

func (c *Client) mintJWTToken(ctx context.Context, ttl int, leases map[string]any) (string, error) {
	body := envelope(map[string]any{
		"ttl":    ttl,
		"leases": leases,
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
