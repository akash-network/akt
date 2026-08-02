// Package console provides an HTTP client for the Akash Console API at
// https://console-api.akash.network (managed wallets, deployments, and the
// public marketplace/catalog endpoints).
//
// Authenticated endpoints require an API key sent via the x-api-key header.
// Public endpoints work without a key; the key is still sent when configured.
//
// Wire conventions: most write bodies are wrapped in a {"data": ...} envelope
// and most responses arrive as {"data": ...}. Exceptions (top-level arrays or
// objects) are noted on the individual methods.
package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"pkg.akt.dev/akt/internal/actionlog"
)

// DefaultBaseURL is the production Console API endpoint.
const DefaultBaseURL = "https://console-api.akash.network"

// MinDepositUSD is the minimum deployment deposit the Console API accepts.
// It lives here — the leaf package every Console caller already imports —
// so the CLI, the workflow transport, and the client itself cannot drift
// apart on the limit they enforce.
const MinDepositUSD = 0.5

// Sentinel errors returned by the client. Match with errors.Is.
var (
	// ErrUnauthorized indicates the API key is missing, invalid, or expired
	// (HTTP 401).
	ErrUnauthorized = errors.New("console: invalid or expired API key")

	// ErrInsufficientFunds indicates the managed wallet cannot cover the
	// operation (HTTP 402).
	ErrInsufficientFunds = errors.New("console: insufficient funds")

	// ErrNotFound indicates the requested resource does not exist (HTTP 404).
	ErrNotFound = errors.New("console: resource not found")

	// ErrAlreadyClosed is returned by CloseDeployment when the deployment is
	// already closed (the API answers 404, or 400 with an already-closed
	// message). Callers that want idempotent close semantics should treat it
	// as success:
	//
	//	if err := c.CloseDeployment(ctx, dseq); err != nil && !errors.Is(err, console.ErrAlreadyClosed) { ... }
	ErrAlreadyClosed = errors.New("console: deployment already closed")
)

// HTTPError is returned for non-2xx statuses that have no dedicated mapping.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("console: unexpected status %d: %s", e.StatusCode, e.Body)
}

// Client interacts with the Akash Console API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	actionLog  *actionlog.Logger
}

// New creates a Console API client. An empty baseURL selects DefaultBaseURL.
// The apiKey may be empty when only public endpoints are used.
func New(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

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

// envelope wraps a request payload in the {"data": ...} envelope used by most
// Console API write endpoints.
func envelope(v any) any {
	return map[string]any{"data": v}
}

// doData executes a request whose response body is a {"data": X} envelope and
// unmarshals X into result. A nil result discards the payload.
func (c *Client) doData(ctx context.Context, method, path string, reqBody, result any) error {
	var env struct {
		Data json.RawMessage `json:"data"`
	}

	if err := c.doJSON(ctx, method, path, reqBody, &env); err != nil {
		return err
	}

	if result == nil {
		return nil
	}

	// A caller that wants a payload must get one: without this check a
	// response that omits the envelope (an API change, a proxy's error
	// page, a wrong-shaped success) leaves result zero-valued and reads
	// as success — e.g. a deployment with an empty dseq.
	if len(env.Data) == 0 {
		return fmt.Errorf("console: %s %s returned no data envelope", method, path)
	}

	if err := json.Unmarshal(env.Data, result); err != nil {
		return fmt.Errorf("console: decode data envelope for %s %s: %w", method, path, err)
	}

	return nil
}

const maxRetries = 3

// retryableStatus reports whether a failed attempt with the given method and
// status may safely be re-sent.
//
// Money-safety rationale: GET/HEAD/PUT/DELETE are idempotent by HTTP
// semantics, so replaying them on 429 or 5xx is always safe. POST (and any
// other non-idempotent method, e.g. PATCH with fallback semantics) is only
// retried on 429 — a rate-limit rejection guarantees the server did NOT
// process the request. A 5xx gives no such guarantee: a gateway 502 can hide
// a write that already succeeded server-side, and replaying
// POST /v1/deployments, /v1/leases, or /v1/deposit-deployment would create
// duplicate deployments or charge the managed wallet twice.
func retryableStatus(method string, status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}

	// Remaining retryable statuses are 5xx: idempotent methods only.
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// doJSON executes an HTTP request with JSON encoding, error mapping, and
// retry with backoff (up to maxRetries attempts) on 429 and — for idempotent
// methods only, see retryableStatus — 5xx. The response body is unmarshaled
// into result as-is (no envelope handling); a nil result discards the body.
func (c *Client) doJSON(ctx context.Context, method, path string, reqBody, result any) error {
	var payload []byte
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("console: marshal request: %w", err)
		}
		payload = data
	}

	var lastErr error
	for attempt := range maxRetries {
		var bodyReader io.Reader
		if payload != nil {
			bodyReader = bytes.NewReader(payload)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return fmt.Errorf("console: create request: %w", err)
		}

		if c.apiKey != "" {
			req.Header.Set("x-api-key", c.apiKey)
		}
		if payload != nil {
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
			return fmt.Errorf("%w (HTTP 401)", ErrUnauthorized)

		case resp.StatusCode == http.StatusPaymentRequired:
			return fmt.Errorf("%w (HTTP 402)", ErrInsufficientFunds)

		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("%w (HTTP 404): %s %s", ErrNotFound, method, path)

		case resp.StatusCode == http.StatusTooManyRequests:
			lastErr = fmt.Errorf("console: rate limited (HTTP 429)")

		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("console: server error (HTTP %d)", resp.StatusCode)

		default:
			return &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody)}
		}

		// Never replay a request that could have been processed server-side
		// (non-idempotent method + 5xx): duplicating a deployment or a USD
		// deposit is worse than surfacing the error.
		if !retryableStatus(method, resp.StatusCode) {
			return lastErr
		}

		// Backoff before retry for 429 and (idempotent-only) 5xx.
		if attempt < maxRetries-1 {
			if err := waitForRetry(ctx, attempt); err != nil {
				return err
			}
		}
	}

	return lastErr
}

// waitForRetry applies the client's bounded exponential backoff while still
// allowing the caller's timeout or cancellation to stop the operation.
func waitForRetry(ctx context.Context, attempt int) error {
	backoff := time.Duration(1<<uint(attempt)) * 100 * time.Millisecond
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(backoff):
		return nil
	}
}
