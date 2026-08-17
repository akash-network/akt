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
	"strings"
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

// maxResponseBodyBytes bounds every Console response before it can be decoded,
// returned in an error, or copied into an action log. Successful list responses
// are currently far smaller; the ceiling leaves room for growth without
// allowing a peer to exhaust client memory.
const maxResponseBodyBytes int64 = 16 << 20

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
			CheckRedirect: func(*http.Request, []*http.Request) error {
				// Return the response to doJSON without following it. Returning a
				// normal error here makes net/http embed the untrusted Location URL
				// in a *url.Error, which is unsafe diagnostic material.
				return http.ErrUseLastResponse
			},
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
	c.recordOutcome(action, dseq, "success", opErr, nil)
}

// recordOutcome writes an action with an explicit lifecycle status and optional
// audit parameters. It is used when an HTTP failure leaves a non-idempotent
// mutation pending rather than proving failure.
func (c *Client) recordOutcome(action, dseq, status string, opErr error, params any) {
	if c.actionLog == nil {
		return
	}

	entry := actionlog.Entry{
		Type:   actionlog.TypeConsole,
		Action: action,
		Status: status,
	}

	if n, err := strconv.ParseUint(dseq, 10, 64); err == nil {
		entry.DSeq = n
	}

	if opErr != nil {
		if entry.Status == "success" {
			entry.Status = "failed"
		}
		entry.Error = redactResponseSecret(opErr.Error(), c.apiKey)
	}
	if params != nil {
		if raw, err := json.Marshal(params); err == nil {
			entry.Params = raw
		}
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
	if len(env.Data) == 0 || bytes.Equal(bytes.TrimSpace(env.Data), []byte("null")) {
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
// semantics, so replaying them on 429 or 5xx is safe. A response status does
// not prove a non-idempotent request was never processed: even a 429 can be
// generated by an intermediary after an upstream accepted the write. POST,
// PATCH, and other non-idempotent methods are therefore never replayed.
func retryableStatus(method string, status int) bool {
	if status != http.StatusTooManyRequests && status < 500 {
		return false
	}

	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// doJSON executes an HTTP request with JSON encoding, error mapping, and
// retry with backoff (up to maxRetries attempts) on 429 and 5xx for idempotent
// methods only (see retryableStatus). The response body is unmarshaled
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
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("console: %s %s: %w", method, path, ctxErr)
			}
			return fmt.Errorf("console: %s %s: %s", method, path, redactResponseSecret(err.Error(), c.apiKey))
		}

		respBody, err := readResponseBody(resp.Body, maxResponseBodyBytes)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("console: read response: %w", err)
		}

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			if result != nil && len(bytes.TrimSpace(respBody)) == 0 {
				return fmt.Errorf("console: %s %s returned an empty response body", method, path)
			}
			if result != nil {
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

		case resp.StatusCode >= 300 && resp.StatusCode < 400:
			return fmt.Errorf("console: HTTP redirects are not allowed (HTTP %d)", resp.StatusCode)

		default:
			return &HTTPError{
				StatusCode: resp.StatusCode,
				Body:       redactResponseSecret(string(respBody), c.apiKey),
			}
		}

		// Never replay a request that could have been processed server-side:
		// duplicating a deployment or a USD deposit is worse than surfacing an
		// ambiguous response, including a 429 from an intermediary.
		if !retryableStatus(method, resp.StatusCode) {
			return lastErr
		}

		// Backoff before retry for idempotent 429/5xx responses.
		if attempt < maxRetries-1 {
			if err := waitForRetry(ctx, attempt); err != nil {
				return err
			}
		}
	}

	return lastErr
}

func readResponseBody(reader io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("invalid response limit %d", limit)
	}

	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d-byte limit", limit)
	}
	return body, nil
}

func redactResponseSecret(body, secret string) string {
	if secret == "" {
		return body
	}
	return strings.ReplaceAll(body, secret, "[REDACTED]")
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
