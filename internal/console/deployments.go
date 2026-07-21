package console

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// CreateDeployment creates a deployment via the managed wallet. Deposit is in
// USD. The returned manifest should be cached (see SaveManifest) so that
// CreateLease can send it after bid selection.
//
// Wire: POST /v1/deployments, body {"data":{"sdl":..., "deposit":...}}.
func (c *Client) CreateDeployment(ctx context.Context, sdl string, depositUSD float64) (*CreateDeploymentResult, error) {
	body := envelope(map[string]any{
		"sdl":     sdl,
		"deposit": depositUSD,
	})

	var out CreateDeploymentResult
	if err := c.doData(ctx, http.MethodPost, "/v1/deployments", body, &out); err != nil {
		c.record("create-deployment", "", err)
		return nil, err
	}

	c.record("create-deployment", out.DSeq.String(), nil)

	return &out, nil
}

// ListDeployments lists deployments with pagination.
//
// Wire: GET /v1/deployments?skip=&limit=, data-enveloped response.
func (c *Client) ListDeployments(ctx context.Context, skip, limit int) (*DeploymentList, error) {
	path := fmt.Sprintf("/v1/deployments?skip=%d&limit=%d", skip, limit)

	var out DeploymentList
	if err := c.doData(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// GetDeployment fetches a single deployment with its leases and escrow
// account.
//
// Wire: GET /v1/deployments/{dseq}, data-enveloped response.
func (c *Client) GetDeployment(ctx context.Context, dseq string) (*DeploymentDetail, error) {
	var out DeploymentDetail
	if err := c.doData(ctx, http.MethodGet, "/v1/deployments/"+url.PathEscape(dseq), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateDeployment updates a deployment's SDL.
//
// Wire: PUT /v1/deployments/{dseq}, body {"data":{"sdl":...}}, data-enveloped
// response.
func (c *Client) UpdateDeployment(ctx context.Context, dseq, sdl string) (*DeploymentDetail, error) {
	body := envelope(map[string]any{"sdl": sdl})

	var out DeploymentDetail
	err := c.doData(ctx, http.MethodPut, "/v1/deployments/"+url.PathEscape(dseq), body, &out)
	c.record("update-deployment", dseq, err)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

// CloseDeployment closes a deployment. If the deployment is already closed
// (the API answers 404 or 400) it returns ErrAlreadyClosed, which callers may
// treat as success for idempotent behavior.
//
// Wire: DELETE /v1/deployments/{dseq}.
func (c *Client) CloseDeployment(ctx context.Context, dseq string) error {
	err := c.doJSON(ctx, http.MethodDelete, "/v1/deployments/"+url.PathEscape(dseq), nil, nil)

	switch {
	case err == nil:
		c.record("close-deployment", dseq, nil)
		return nil

	case isAlreadyClosed(err):
		// Desired end state reached; log as success but surface the sentinel.
		c.record("close-deployment", dseq, nil)
		return fmt.Errorf("%w (dseq %s)", ErrAlreadyClosed, dseq)

	default:
		c.record("close-deployment", dseq, err)
		return err
	}
}

// isAlreadyClosed reports whether a close attempt failed because the
// deployment no longer exists or is already closed (404 or 400).
func isAlreadyClosed(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}

	var httpErr *HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusBadRequest
}

// Deposit adds funds to a deployment's escrow. Amount is in USD.
//
// Wire: POST /v1/deposit-deployment, body {"data":{"dseq":..., "deposit":...}}.
func (c *Client) Deposit(ctx context.Context, dseq string, amountUSD float64) error {
	body := envelope(map[string]any{
		"dseq":    dseq,
		"deposit": amountUSD,
	})

	err := c.doJSON(ctx, http.MethodPost, "/v1/deposit-deployment", body, nil)
	c.record("deposit", dseq, err)

	return err
}

// FetchBids fetches bids for a deployment's open orders.
//
// Wire: GET /v1/bids?dseq=, response {"data":[{"bid":{...}}]}.
func (c *Client) FetchBids(ctx context.Context, dseq string) ([]Bid, error) {
	path := "/v1/bids?dseq=" + url.QueryEscape(dseq)

	var wrapped []struct {
		Bid Bid `json:"bid"`
	}
	if err := c.doData(ctx, http.MethodGet, path, nil, &wrapped); err != nil {
		return nil, err
	}

	bids := make([]Bid, 0, len(wrapped))
	for _, w := range wrapped {
		bids = append(bids, w.Bid)
	}

	return bids, nil
}

// CreateLease accepts one or more bids and sends the deployment manifest to
// the winning providers.
//
// Wire: POST /v1/leases. NOTE: unlike other writes, the request body is NOT
// data-enveloped: {"manifest":..., "leases":[{dseq,gseq,oseq,provider}]}.
// The response is data-enveloped.
func (c *Client) CreateLease(ctx context.Context, manifest string, leases []LeaseRequest) (*DeploymentDetail, error) {
	body := map[string]any{
		"manifest": manifest,
		"leases":   leases,
	}

	leaseDSeq := ""
	if len(leases) > 0 {
		leaseDSeq = leases[0].DSeq
	}

	var out DeploymentDetail
	err := c.doData(ctx, http.MethodPost, "/v1/leases", body, &out)
	c.record("create-lease", leaseDSeq, err)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

// GetDeploymentSettings fetches auto-top-up settings for a deployment.
// Returns an error matching ErrNotFound when no settings exist yet.
//
// Wire: GET /v2/deployment-settings/{dseq}, data-enveloped response.
func (c *Client) GetDeploymentSettings(ctx context.Context, dseq string) (*DeploymentSettings, error) {
	var out DeploymentSettings
	if err := c.doData(ctx, http.MethodGet, "/v2/deployment-settings/"+url.PathEscape(dseq), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// SetDeploymentAutoTopUp enables or disables auto-top-up for a deployment.
// It PATCHes existing settings and transparently falls back to creating them
// (POST) when none exist yet.
//
// Wire: PATCH /v2/deployment-settings/{dseq}, body
// {"data":{"autoTopUpEnabled":...}}; on 404, POST /v2/deployment-settings,
// body {"data":{"dseq":..., "autoTopUpEnabled":...}}.
func (c *Client) SetDeploymentAutoTopUp(ctx context.Context, dseq string, enabled bool) (*DeploymentSettings, error) {
	var out DeploymentSettings

	err := c.doData(ctx, http.MethodPatch, "/v2/deployment-settings/"+url.PathEscape(dseq),
		envelope(map[string]any{"autoTopUpEnabled": enabled}), &out)
	if errors.Is(err, ErrNotFound) {
		err = c.doData(ctx, http.MethodPost, "/v2/deployment-settings",
			envelope(map[string]any{"dseq": dseq, "autoTopUpEnabled": enabled}), &out)
	}

	c.record("update-deployment-settings", dseq, err)
	if err != nil {
		return nil, err
	}

	return &out, nil
}
