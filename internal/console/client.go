// Package console provides an HTTP client for the Akash Console Managed
// Wallet API at https://console-api.akash.network.
package console

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"pkg.akt.dev/akt/internal/actionlog"
)

// Client interacts with the Akash Console Managed Wallet API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	actionLog  *actionlog.Logger
}

// DeploymentResponse represents a deployment returned by the API.
type DeploymentResponse struct {
	DSeq     string `json:"dseq"`
	Manifest string `json:"manifest,omitempty"`
}

// DeploymentListResponse wraps a paginated list of deployments.
type DeploymentListResponse struct {
	Data []DeploymentResponse `json:"data"`
}

// BidResponse represents a bid from a provider.
type BidResponse struct {
	Provider string  `json:"provider"`
	Price    float64 `json:"price"`
}

// LeaseRequest identifies a specific bid to accept.
type LeaseRequest struct {
	DSeq     string `json:"dseq"`
	GSeq     uint32 `json:"gseq"`
	OSeq     uint32 `json:"oseq"`
	Provider string `json:"provider"`
}

// New creates a Console API client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithActionLog attaches a per-context action logger; state-changing Console
// API calls are then recorded as type=console entries (SPEC §5.6). A nil
// logger disables recording. Returns the client for chaining.
func (c *Client) WithActionLog(l *actionlog.Logger) *Client {
	c.actionLog = l
	return c
}

// record writes a console action entry. Best-effort: logging failures never
// affect the API call result.
func (c *Client) record(action, dseq string, opErr error) {
	if c.actionLog == nil {
		return
	}

	entry := actionlog.Entry{
		Type:   actionlog.TypeConsole,
		Action: action,
		Status: "success",
	}

	if n, err := strconv.ParseUint(dseq, 10, 64); err == nil {
		entry.DSeq = n
	}

	if opErr != nil {
		entry.Status = "failed"
		entry.Error = opErr.Error()
	}

	_ = c.actionLog.Log(entry)
}

// CreateDeployment creates a deployment via managed wallet. Deposit is in USD.
func (c *Client) CreateDeployment(ctx context.Context, sdl string, depositUSD float64) (*DeploymentResponse, error) {
	body := map[string]any{
		"sdl":     sdl,
		"deposit": depositUSD,
	}

	var resp DeploymentResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/deployments", body, &resp); err != nil {
		c.record("create-deployment", "", err)
		return nil, err
	}

	c.record("create-deployment", resp.DSeq, nil)

	return &resp, nil
}

// ListDeployments lists deployments with pagination.
func (c *Client) ListDeployments(ctx context.Context, skip, limit int) (*DeploymentListResponse, error) {
	path := fmt.Sprintf("/v1/deployments?skip=%d&limit=%d", skip, limit)

	var resp DeploymentListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetDeployment gets a single deployment.
func (c *Client) GetDeployment(ctx context.Context, dseq string) (*DeploymentResponse, error) {
	var resp DeploymentResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/deployments/"+url.PathEscape(dseq), nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// UpdateDeployment updates a deployment's SDL.
func (c *Client) UpdateDeployment(ctx context.Context, dseq, sdl string) (*DeploymentResponse, error) {
	body := map[string]any{
		"sdl": sdl,
	}

	var resp DeploymentResponse
	err := c.doJSON(ctx, http.MethodPut, "/v1/deployments/"+url.PathEscape(dseq), body, &resp)
	c.record("update-deployment", dseq, err)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// CloseDeployment closes a deployment.
func (c *Client) CloseDeployment(ctx context.Context, dseq string) error {
	err := c.doJSON(ctx, http.MethodDelete, "/v1/deployments/"+url.PathEscape(dseq), nil, nil)
	c.record("close-deployment", dseq, err)

	return err
}

// FetchBids fetches bids for a deployment.
func (c *Client) FetchBids(ctx context.Context, dseq string) ([]BidResponse, error) {
	path := "/v1/bids?dseq=" + url.QueryEscape(dseq)

	var resp []BidResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// CreateLease creates a lease from bids.
func (c *Client) CreateLease(ctx context.Context, manifest string, leases []LeaseRequest) (*DeploymentResponse, error) {
	body := map[string]any{
		"manifest": manifest,
		"leases":   leases,
	}

	leaseDSeq := ""
	if len(leases) > 0 {
		leaseDSeq = leases[0].DSeq
	}

	var resp DeploymentResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/leases", body, &resp)
	c.record("create-lease", leaseDSeq, err)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// Deposit adds funds to a deployment. Amount is in USD.
func (c *Client) Deposit(ctx context.Context, dseq string, amountUSD float64) error {
	body := map[string]any{
		"dseq":   dseq,
		"amount": amountUSD,
	}

	err := c.doJSON(ctx, http.MethodPost, "/v1/deposit-deployment", body, nil)
	c.record("deposit", dseq, err)

	return err
}

const maxRetries = 3

// doJSON executes an HTTP request with JSON encoding, error mapping, and retry.
func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any, result any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("console: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	var lastErr error
	for attempt := range maxRetries {
		// Reset body reader for retries.
		if reqBody != nil {
			data, _ := json.Marshal(reqBody) // already validated above
			bodyReader = bytes.NewReader(data)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return fmt.Errorf("console: create request: %w", err)
		}

		req.Header.Set("x-api-key", c.apiKey)
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("console: %s %s: %w", method, path, err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("console: read response: %w", err)
		}

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			if result != nil && len(respBody) > 0 {
				if err := json.Unmarshal(respBody, result); err != nil {
					return fmt.Errorf("console: decode response: %w", err)
				}
			}
			return nil

		case resp.StatusCode == http.StatusUnauthorized:
			return fmt.Errorf("console: invalid or expired API key (HTTP 401)")

		case resp.StatusCode == http.StatusPaymentRequired:
			return fmt.Errorf("console: insufficient funds (HTTP 402)")

		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("console: deployment not found (HTTP 404)")

		case resp.StatusCode == http.StatusTooManyRequests:
			lastErr = fmt.Errorf("console: rate limited (HTTP 429)")

		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("console: server error (HTTP %s)", strconv.Itoa(resp.StatusCode))

		default:
			return fmt.Errorf("console: unexpected status %d: %s", resp.StatusCode, string(respBody))
		}

		// Backoff before retry for 429 and 5xx.
		if attempt < maxRetries-1 {
			backoff := time.Duration(1<<uint(attempt)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return lastErr
}
