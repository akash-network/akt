package console

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"pkg.akt.dev/go/sdl"
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

// ListDeployments lists deployments with pagination. Out-of-range values are
// omitted from the query instead of being sent — the API requires skip >= 0
// and limit >= 1, so a negative skip falls back to 0 and a non-positive limit
// falls back to the server default page size.
//
// Wire: GET /v1/deployments?skip=&limit=, data-enveloped response.
func (c *Client) ListDeployments(ctx context.Context, skip, limit int) (*DeploymentList, error) {
	var params []string
	if skip >= 0 {
		params = append(params, "skip="+strconv.Itoa(skip))
	}
	if limit >= 1 {
		params = append(params, "limit="+strconv.Itoa(limit))
	}

	path := "/v1/deployments"
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	var out DeploymentList
	if err := c.doData(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}

	regroupLeases(out.Deployments)

	return &out, nil
}

// regroupLeases re-files each lease under the deployment its own ID names.
//
// GET /v1/deployments hands back leases attached to the wrong deployments --
// typically 10 to 13 of 13 entries mispaired, as a permutation that differs on
// every call, so `deployment list` reported the wrong provider and the wrong
// price for nearly every row while GET /v1/deployments/{dseq} returned the
// right ones. The defect is upstream, but every lease carries its own dseq, so
// the correct pairing is already in the response and is recoverable without
// another request.
//
// A lease whose dseq matches no deployment in the page is dropped: it belongs
// to a deployment the caller did not ask about, and guessing a home for it
// would reintroduce the bug this fixes.
func regroupLeases(items []DeploymentListItem) {
	if len(items) == 0 {
		return
	}

	byDSeq := make(map[string]int, len(items))
	for i := range items {
		byDSeq[items[i].Deployment.ID.DSeq.String()] = i
	}

	regrouped := make([][]Lease, len(items))

	for i := range items {
		for _, lease := range items[i].Leases {
			owner, ok := byDSeq[lease.ID.DSeq.String()]
			if !ok {
				continue
			}

			regrouped[owner] = append(regrouped[owner], lease)
		}
	}

	for i := range items {
		items[i].Leases = regrouped[i]
	}
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

	var lastErr error
	for attempt := range maxRetries {
		var out DeploymentDetail
		lastErr = c.doData(ctx, http.MethodPut, "/v1/deployments/"+url.PathEscape(dseq), body, &out)
		if lastErr == nil {
			c.record("update-deployment", dseq, nil)
			return &out, nil
		}

		// A gateway or API handler can return an error after the idempotent
		// update reached chain state. The deployment hash is authoritative
		// proof, so do not replay a write whose desired version is present.
		if detail, ok := c.reconcileDeploymentUpdate(ctx, dseq, sdl); ok {
			c.record("update-deployment", dseq, nil)
			return detail, nil
		}

		if !isTransientManifestVersionError(lastErr) || attempt == maxRetries-1 {
			break
		}
		if err := waitForRetry(ctx, attempt); err != nil {
			lastErr = err
			break
		}
	}

	c.record("update-deployment", dseq, lastErr)
	return nil, lastErr
}

func (c *Client) reconcileDeploymentUpdate(ctx context.Context, dseq, rawSDL string) (*DeploymentDetail, bool) {
	doc, err := sdl.Read([]byte(rawSDL))
	if err != nil {
		return nil, false
	}

	version, err := doc.Version()
	if err != nil {
		return nil, false
	}

	detail, err := c.GetDeployment(ctx, dseq)
	if err != nil {
		return nil, false
	}

	expected := base64.StdEncoding.EncodeToString(version)
	return detail, detail.Deployment.Hash == expected
}

func isTransientManifestVersionError(err error) bool {
	var httpErr *HTTPError
	return errors.As(err, &httpErr) &&
		httpErr.StatusCode == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(httpErr.Body), "manifest version validation failed")
}

// CloseDeployment closes a deployment. If the deployment is already closed
// (the API answers 404, or 400 with an already-closed message) it returns
// ErrAlreadyClosed, which callers may treat as success for idempotent
// behavior. Any other 400 is a genuine failure and is returned as-is.
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
// deployment no longer exists or is already closed. A 404 always qualifies:
// the resource is gone, which is the desired end state. A 400 qualifies only
// when its body actually says already-closed/not-found — the pinned contract
// (testdata/openapi.json) documents only 200 for DELETE
// /v1/deployments/{dseq}, so a generic 400 (validation, not-owner, active
// leases, ...) is a real failure that must surface to the caller.
func isAlreadyClosed(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadRequest {
		return false
	}

	// "closed" also covers "already closed".
	body := strings.ToLower(httpErr.Body)

	return strings.Contains(body, "closed") || strings.Contains(body, "not found")
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
	if err != nil {
		// Never replay this POST: the live API can return an error after the
		// lease transaction succeeded. Read back the exact lease identities
		// and accept only authoritative active state.
		if detail, ok := c.reconcileCreatedLeases(ctx, leases); ok {
			c.record("create-lease", leaseDSeq, nil)
			return detail, nil
		}

		c.record("create-lease", leaseDSeq, err)
		return nil, err
	}

	c.record("create-lease", leaseDSeq, nil)
	return &out, nil
}

func (c *Client) reconcileCreatedLeases(ctx context.Context, requested []LeaseRequest) (*DeploymentDetail, bool) {
	if len(requested) == 0 || requested[0].DSeq == "" {
		return nil, false
	}

	dseq := requested[0].DSeq
	for _, req := range requested[1:] {
		if req.DSeq != dseq {
			return nil, false
		}
	}

	for attempt := range maxRetries {
		detail, err := c.GetDeployment(ctx, dseq)
		if err == nil && requestedLeasesActive(detail.Leases, requested) {
			return detail, true
		}

		if attempt < maxRetries-1 {
			if err := waitForRetry(ctx, attempt); err != nil {
				return nil, false
			}
		}
	}

	return nil, false
}

func requestedLeasesActive(actual []Lease, requested []LeaseRequest) bool {
	for _, req := range requested {
		matched := false
		for _, lease := range actual {
			if lease.ID.DSeq.String() == req.DSeq &&
				lease.ID.GSeq == req.GSeq &&
				lease.ID.OSeq == req.OSeq &&
				lease.ID.Provider == req.Provider &&
				strings.EqualFold(lease.State, "active") {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return len(requested) > 0
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
