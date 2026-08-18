package console

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	"pkg.akt.dev/go/sdl"
)

// CreateDeployment creates a deployment via the managed wallet. Deposit is in
// USD. The returned manifest should be cached (see SaveManifest) so that
// CreateLease can send it after bid selection.
//
// Wire: POST /v1/deployments, body {"data":{"sdl":..., "deposit":...}}.
func (c *Client) CreateDeployment(ctx context.Context, sdl string, depositUSD float64) (*CreateDeploymentResult, error) {
	if err := validateDepositUSD(depositUSD); err != nil {
		c.record("create-deployment", "", err)
		return nil, err
	}

	versionHash, manifest, err := deploymentArtifacts(sdl)
	if err != nil {
		wrapped := fmt.Errorf("prepare deployment SDL: %w", err)
		c.recordOutcome("create-deployment", "", "failed", wrapped, nil)
		return nil, wrapped
	}

	before, err := c.listAllDeployments(ctx)
	if err != nil {
		wrapped := fmt.Errorf("snapshot deployments before create: %w", err)
		c.recordOutcome("create-deployment", "", "failed", wrapped, map[string]string{"versionHash": versionHash})
		return nil, wrapped
	}
	known := make(map[string]struct{}, len(before))
	for _, item := range before {
		known[item.Deployment.ID.DSeq.String()] = struct{}{}
	}

	body := envelope(map[string]any{
		"sdl":     sdl,
		"deposit": depositUSD,
	})

	var out CreateDeploymentResult
	err = c.doData(ctx, http.MethodPost, "/v1/deployments", body, &out)
	if err == nil {
		err = validateCreateDeploymentResult(&out)
	}
	if err == nil {
		if out.Manifest == "" {
			out.Manifest = manifest
		}
		c.recordOutcome("create-deployment", out.DSeq.String(), "success", nil, map[string]string{"versionHash": versionHash})
		return &out, nil
	}

	if definitiveCreateFailure(err) {
		c.recordOutcome("create-deployment", "", "failed", err, map[string]string{"versionHash": versionHash})
		return nil, err
	}

	if reconciled, ok := c.reconcileCreatedDeployment(ctx, known, versionHash, manifest); ok {
		c.recordOutcome("create-deployment", reconciled.DSeq.String(), "success", nil, map[string]string{"versionHash": versionHash})
		return reconciled, nil
	}

	unknown := fmt.Errorf("deployment creation outcome unknown after one submission (%w); the request was not replayed: inspect `akt console deployment list` for SDL version %s", err, versionHash)
	c.recordOutcome("create-deployment", "", "pending", unknown, map[string]string{"versionHash": versionHash})
	return nil, unknown
}

func validateCreateDeploymentResult(result *CreateDeploymentResult) error {
	if !validDeploymentDSeq(result.DSeq.String()) {
		return fmt.Errorf("console: POST /v1/deployments returned an invalid dseq %q", result.DSeq.String())
	}
	if result.SignTx == nil {
		return errors.New("console: POST /v1/deployments omitted the managed-wallet transaction receipt")
	}
	if !result.SignTx.codePresent {
		return errors.New("console: POST /v1/deployments transaction receipt omitted its code")
	}
	if result.SignTx.Code != 0 {
		return fmt.Errorf(
			"console: POST /v1/deployments transaction %s failed with code %d: %s",
			result.SignTx.TransactionHash,
			result.SignTx.Code,
			result.SignTx.RawLog,
		)
	}
	if strings.TrimSpace(result.SignTx.TransactionHash) == "" {
		return errors.New("console: POST /v1/deployments transaction receipt omitted its hash")
	}

	return nil
}

func deploymentArtifacts(rawSDL string) (string, string, error) {
	doc, err := sdl.Read([]byte(rawSDL))
	if err != nil {
		return "", "", err
	}

	version, err := doc.Version()
	if err != nil {
		return "", "", fmt.Errorf("derive version: %w", err)
	}
	mani, err := doc.Manifest()
	if err != nil {
		return "", "", fmt.Errorf("render manifest: %w", err)
	}
	manifestJSON, err := json.Marshal(mani)
	if err != nil {
		return "", "", fmt.Errorf("encode manifest: %w", err)
	}

	return base64.StdEncoding.EncodeToString(version), string(manifestJSON), nil
}

func validDeploymentDSeq(dseq string) bool {
	n, err := strconv.ParseUint(dseq, 10, 64)
	return err == nil && n > 0
}

func definitiveCreateFailure(err error) bool {
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrInsufficientFunds) || errors.Is(err, ErrNotFound) {
		return true
	}

	var httpErr *HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 && httpErr.StatusCode != http.StatusTooManyRequests
}

func (c *Client) reconcileCreatedDeployment(
	ctx context.Context,
	known map[string]struct{},
	versionHash string,
	manifest string,
) (*CreateDeploymentResult, bool) {
	// Two extra reads beyond the normal retry bound allow for chain/API
	// propagation while keeping an ambiguous CLI invocation bounded.
	for attempt := range maxRetries + 2 {
		items, err := c.listAllDeployments(ctx)
		if err == nil {
			matches := make([]string, 0, 1)
			for _, item := range items {
				dseq := item.Deployment.ID.DSeq.String()
				if _, existed := known[dseq]; existed || item.Deployment.Hash != versionHash || !validDeploymentDSeq(dseq) {
					continue
				}
				matches = append(matches, dseq)
			}

			if len(matches) == 1 {
				return &CreateDeploymentResult{DSeq: FlexString(matches[0]), Manifest: manifest}, true
			}
			if len(matches) > 1 {
				// Multiple matching writes are already an ambiguous outcome;
				// waiting cannot identify which one this invocation created.
				return nil, false
			}
		}

		if attempt < maxRetries+1 {
			if err := waitForRetry(ctx, attempt); err != nil {
				return nil, false
			}
		}
	}

	return nil, false
}

const (
	deploymentCollectionPageSize   = 1000
	maxDeploymentCollectionPages   = 100
	maxDeploymentCollectionRecords = 10_000
)

func (c *Client) listAllDeployments(ctx context.Context) ([]DeploymentListItem, error) {
	var all []DeploymentListItem
	for pageCount, skip := 0, 0; ; pageCount++ {
		if pageCount >= maxDeploymentCollectionPages {
			return nil, deploymentPaginationLimitError()
		}

		page, err := c.ListDeployments(ctx, skip, deploymentCollectionPageSize)
		if err != nil {
			return nil, err
		}
		if len(page.Deployments) > maxDeploymentCollectionRecords-len(all) {
			return nil, deploymentPaginationLimitError()
		}
		all = append(all, page.Deployments...)
		if !page.Pagination.HasMore {
			return all, nil
		}

		step := page.Pagination.Limit
		if step <= 0 {
			step = len(page.Deployments)
		}
		next := page.Pagination.Skip + step
		if step <= 0 || next <= skip {
			return nil, fmt.Errorf("console: deployment pagination did not advance from skip %d", skip)
		}
		skip = next
	}
}

func deploymentPaginationLimitError() error {
	return fmt.Errorf(
		"console: deployment pagination exceeded safety limit (%d pages or %d records)",
		maxDeploymentCollectionPages,
		maxDeploymentCollectionRecords,
	)
}

// ListDeployments lists deployments with validated pagination.
//
// Wire: GET /v1/deployments?skip=&limit=, data-enveloped response.
func (c *Client) ListDeployments(ctx context.Context, skip, limit int) (*DeploymentList, error) {
	if skip < 0 {
		return nil, fmt.Errorf("console: deployment pagination skip must be non-negative, got %d", skip)
	}
	if limit < 1 {
		return nil, fmt.Errorf("console: deployment pagination limit must be greater than zero, got %d", limit)
	}

	path := "/v1/deployments?skip=" + strconv.Itoa(skip) + "&limit=" + strconv.Itoa(limit)

	var out DeploymentList
	if err := c.doData(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}

	regroupLeases(out.Deployments)

	return &out, nil
}

// ListDeploymentsByState traverses the bounded deployment collection, applies
// the state filter, and only then applies the requested page window.
func (c *Client) ListDeploymentsByState(ctx context.Context, state string, skip, limit int) (*DeploymentList, error) {
	if skip < 0 {
		return nil, fmt.Errorf("console: deployment pagination skip must be non-negative, got %d", skip)
	}
	if limit < 1 {
		return nil, fmt.Errorf("console: deployment pagination limit must be greater than zero, got %d", limit)
	}
	state = strings.ToLower(strings.TrimSpace(state))
	if state != "active" && state != "closed" {
		return nil, fmt.Errorf("console: deployment state must be active or closed, got %q", state)
	}

	all, err := c.listAllDeployments(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]DeploymentListItem, 0, len(all))
	for _, item := range all {
		if strings.EqualFold(item.Deployment.State, state) {
			filtered = append(filtered, item)
		}
	}

	total := len(filtered)
	start := skip
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	return &DeploymentList{
		Deployments: filtered[start:end],
		Pagination: Pagination{
			Total:   total,
			Skip:    skip,
			Limit:   limit,
			HasMore: end < total,
		},
	}, nil
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
	if err := validateDSeq(dseq); err != nil {
		return nil, err
	}

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
	if err := validateDSeq(dseq); err != nil {
		c.record("update-deployment", dseq, err)
		return nil, err
	}

	expectedHash, _, err := deploymentArtifacts(sdl)
	if err != nil {
		wrapped := fmt.Errorf("prepare deployment SDL: %w", err)
		c.record("update-deployment", dseq, wrapped)
		return nil, wrapped
	}
	if _, err := c.requireMutableDeployment(ctx, dseq); err != nil {
		wrapped := fmt.Errorf("preflight deployment update: %w", err)
		c.record("update-deployment", dseq, wrapped)
		return nil, wrapped
	}

	body := envelope(map[string]any{"sdl": sdl})

	var lastErr error
	for attempt := range maxRetries {
		var out DeploymentDetail
		lastErr = c.doData(ctx, http.MethodPut, "/v1/deployments/"+url.PathEscape(dseq), body, &out)
		if lastErr == nil {
			lastErr = validateDeploymentUpdateResult(&out, dseq, expectedHash)
			if lastErr == nil {
				c.record("update-deployment", dseq, nil)
				return &out, nil
			}
		}

		// A gateway or API handler can return an error after the idempotent
		// update reached chain state. The deployment hash is authoritative
		// proof, so do not replay a write whose desired version is present.
		if detail, ok := c.reconcileDeploymentUpdate(ctx, dseq, expectedHash); ok {
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

func validateDeploymentUpdateResult(result *DeploymentDetail, dseq, expectedHash string) error {
	if result.Deployment.ID.DSeq.String() != dseq {
		return fmt.Errorf("console: deployment update response returned dseq %q, want %q", result.Deployment.ID.DSeq.String(), dseq)
	}
	if result.Deployment.Hash != expectedHash {
		return fmt.Errorf("console: deployment update response returned SDL hash %q, want %q", result.Deployment.Hash, expectedHash)
	}

	return nil
}

func (c *Client) reconcileDeploymentUpdate(ctx context.Context, dseq, expectedHash string) (*DeploymentDetail, bool) {
	detail, err := c.GetDeployment(ctx, dseq)
	if err != nil {
		return nil, false
	}

	return detail, detail.Deployment.ID.DSeq.String() == dseq && detail.Deployment.Hash == expectedHash
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
	if err := validateDSeq(dseq); err != nil {
		c.record("close-deployment", dseq, err)
		return err
	}

	var out struct {
		Success *bool `json:"success"`
	}
	err := c.doData(ctx, http.MethodDelete, "/v1/deployments/"+url.PathEscape(dseq), nil, &out)
	if err == nil && (out.Success == nil || !*out.Success) {
		err = errors.New("console: close deployment response did not acknowledge success: true")
	}

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

	body := strings.ToLower(httpErr.Body)

	return strings.Contains(body, "already closed") ||
		strings.Contains(body, "deployment closed") ||
		strings.Contains(body, "not found")
}

// Deposit adds funds to a deployment's escrow. Amount is in USD.
//
// Wire: POST /v1/deposit-deployment, body {"data":{"dseq":..., "deposit":...}}.
func (c *Client) Deposit(ctx context.Context, dseq string, amountUSD float64) error {
	if err := validateDSeq(dseq); err != nil {
		c.record("deposit", dseq, err)
		return err
	}
	if err := validateDepositUSD(amountUSD); err != nil {
		c.record("deposit", dseq, err)
		return err
	}

	expectedMicros, err := consoleDepositMicros(amountUSD)
	if err != nil {
		wrapped := fmt.Errorf("prepare deployment deposit: %w", err)
		c.record("deposit", dseq, wrapped)
		return wrapped
	}

	before, err := c.GetDeployment(ctx, dseq)
	if err != nil {
		wrapped := fmt.Errorf("snapshot deployment escrow before deposit: %w", err)
		c.record("deposit", dseq, wrapped)
		return wrapped
	}
	if err := requireMutableState(before, dseq); err != nil {
		c.record("deposit", dseq, err)
		return err
	}
	if before.Deployment.ID.DSeq.String() != dseq {
		wrapped := fmt.Errorf("console: deployment pre-state returned dseq %q, want %q", before.Deployment.ID.DSeq.String(), dseq)
		c.record("deposit", dseq, wrapped)
		return wrapped
	}
	beforeTotals, err := deploymentEscrowTotals(before)
	if err != nil {
		wrapped := fmt.Errorf("snapshot deployment escrow before deposit: %w", err)
		c.record("deposit", dseq, wrapped)
		return wrapped
	}

	body := envelope(map[string]any{
		"dseq":    dseq,
		"deposit": amountUSD,
	})

	var out DeploymentDetail
	postErr := c.doData(ctx, http.MethodPost, "/v1/deposit-deployment", body, &out)
	if postErr == nil && out.Deployment.ID.DSeq.String() != dseq {
		postErr = fmt.Errorf("console: deposit response returned dseq %q, want %q", out.Deployment.ID.DSeq.String(), dseq)
	}
	if postErr == nil {
		afterTotals, escrowErr := deploymentEscrowTotals(&out)
		switch {
		case escrowErr != nil:
			postErr = fmt.Errorf("console: validate deposit response escrow: %w", escrowErr)
		case depositTotalDeltaMatches(beforeTotals, afterTotals, expectedMicros):
			c.record("deposit", dseq, nil)
			return nil
		default:
			postErr = fmt.Errorf("console: deposit response escrow did not increase by exactly %s uact", expectedMicros)
		}
	}
	if definitiveCreateFailure(postErr) {
		c.record("deposit", dseq, postErr)
		return postErr
	}

	if reconciled, reconcileErr := c.reconcileDeposit(ctx, dseq, beforeTotals, expectedMicros); reconciled {
		c.record("deposit", dseq, nil)
		return nil
	} else if reconcileErr != nil {
		postErr = errors.Join(postErr, reconcileErr)
	}
	unknown := fmt.Errorf("deposit outcome unknown after one submission (%w); the request was not replayed: inspect deployment %s escrow state", postErr, dseq)
	c.recordOutcome("deposit", dseq, "pending", unknown, map[string]any{"amountUSD": amountUSD})
	return unknown
}

func consoleDepositMicros(amountUSD float64) (*big.Int, error) {
	amount, ok := new(big.Rat).SetString(strconv.FormatFloat(amountUSD, 'f', -1, 64))
	if !ok || amount.Sign() <= 0 {
		return nil, fmt.Errorf("deposit must be a positive finite USD amount, got %v", amountUSD)
	}

	amount.Mul(amount, big.NewRat(1_000_000, 1))
	if !amount.IsInt() {
		return nil, fmt.Errorf("deposit %v USD has precision below one micro-ACT", amountUSD)
	}

	return new(big.Int).Set(amount.Num()), nil
}

func validateDepositUSD(amountUSD float64) error {
	if amountUSD < MinDepositUSD {
		return fmt.Errorf("console: deployment deposit must be at least $%.2f, got %v", MinDepositUSD, amountUSD)
	}
	if _, err := consoleDepositMicros(amountUSD); err != nil {
		return fmt.Errorf("console: invalid deployment deposit: %w", err)
	}

	return nil
}

func deploymentEscrowTotals(detail *DeploymentDetail) (map[string]*big.Rat, error) {
	if detail == nil || len(detail.EscrowAccount) == 0 || string(detail.EscrowAccount) == "null" {
		return nil, errors.New("deployment omitted its escrow account")
	}

	var escrow struct {
		State struct {
			Funds       json.RawMessage `json:"funds"`
			Transferred json.RawMessage `json:"transferred"`
		} `json:"state"`
	}
	if err := json.Unmarshal(detail.EscrowAccount, &escrow); err != nil {
		return nil, fmt.Errorf("decode deployment escrow: %w", err)
	}

	type coin struct {
		Denom  string     `json:"denom"`
		Amount FlexString `json:"amount"`
	}

	totals := make(map[string]*big.Rat)
	addCoins := func(field string, raw json.RawMessage, allowNegative bool) error {
		encoded := strings.TrimSpace(string(raw))
		if encoded == "" || encoded == "null" {
			return fmt.Errorf("deployment escrow omitted its %s", field)
		}

		var coins []coin
		if strings.HasPrefix(encoded, "[") {
			if err := json.Unmarshal(raw, &coins); err != nil {
				return fmt.Errorf("decode deployment escrow %s: %w", field, err)
			}
		} else {
			var single coin
			if err := json.Unmarshal(raw, &single); err != nil {
				return fmt.Errorf("decode deployment escrow %s: %w", field, err)
			}
			coins = []coin{single}
		}

		for _, coin := range coins {
			denom := strings.ToLower(strings.TrimSpace(coin.Denom))
			if denom == "" {
				return fmt.Errorf("deployment escrow %s entry omitted its denomination", field)
			}
			amount, err := parseDeploymentEscrowAmount(coin.Amount.String())
			if err != nil {
				return fmt.Errorf("deployment escrow %s amount %q for %s is not a valid fixed-point decimal: %w", field, coin.Amount, denom, err)
			}
			if !allowNegative && amount.Sign() < 0 {
				return fmt.Errorf("deployment escrow %s amount %q for %s must be non-negative", field, coin.Amount, denom)
			}
			if current := totals[denom]; current != nil {
				amount.Add(amount, current)
			}
			totals[denom] = amount
		}

		return nil
	}

	if err := addCoins("funds", escrow.State.Funds, true); err != nil {
		return nil, err
	}
	if err := addCoins("transferred", escrow.State.Transferred, false); err != nil {
		return nil, err
	}

	return totals, nil
}

func parseDeploymentEscrowAmount(raw string) (*big.Rat, error) {
	amount, err := sdkmath.LegacyNewDecFromStr(raw)
	if err != nil {
		return nil, err
	}

	return new(big.Rat).SetFrac(
		amount.BigInt(),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(sdkmath.LegacyPrecision), nil),
	), nil
}

const (
	depositReconciliationWindow       = 30 * time.Second
	depositReconciliationPollInterval = 2 * time.Second
)

func (c *Client) reconcileDeposit(
	ctx context.Context,
	dseq string,
	beforeTotals map[string]*big.Rat,
	expectedMicros *big.Int,
) (bool, error) {
	reconciliationCtx, cancel := context.WithTimeout(ctx, depositReconciliationWindow)
	defer cancel()

	return c.reconcileDepositUntil(
		reconciliationCtx,
		dseq,
		beforeTotals,
		expectedMicros,
		depositReconciliationPollInterval,
	)
}

func (c *Client) reconcileDepositUntil(
	ctx context.Context,
	dseq string,
	beforeTotals map[string]*big.Rat,
	expectedMicros *big.Int,
	pollInterval time.Duration,
) (bool, error) {
	var lastErr error
	for {
		detail, err := c.GetDeployment(ctx, dseq)
		if err != nil {
			lastErr = err
		} else if detail.Deployment.ID.DSeq.String() != dseq {
			lastErr = fmt.Errorf("console: deployment post-state returned dseq %q, want %q", detail.Deployment.ID.DSeq.String(), dseq)
		} else if afterTotals, escrowErr := deploymentEscrowTotals(detail); escrowErr != nil {
			lastErr = escrowErr
		} else if depositTotalDeltaMatches(beforeTotals, afterTotals, expectedMicros) {
			return true, nil
		} else {
			lastErr = fmt.Errorf("deployment escrow did not increase by exactly %s uact", expectedMicros)
		}

		if err := waitForDepositObservation(ctx, pollInterval); err != nil {
			return false, errors.Join(lastErr, err)
		}
	}
}

func waitForDepositObservation(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func depositTotalDeltaMatches(before, after map[string]*big.Rat, expected *big.Int) bool {
	afterAmount := after["uact"]
	if afterAmount == nil {
		return false
	}
	beforeAmount := before["uact"]
	if beforeAmount == nil {
		beforeAmount = new(big.Rat)
	}

	delta := new(big.Rat).Sub(new(big.Rat).Set(afterAmount), beforeAmount)
	return delta.Cmp(new(big.Rat).SetInt(expected)) == 0
}

// FetchBids fetches bids for a deployment's open orders.
//
// Wire: GET /v1/bids?dseq=, response {"data":[{"bid":{...}}]}.
func (c *Client) FetchBids(ctx context.Context, dseq string) ([]Bid, error) {
	if err := validateDSeq(dseq); err != nil {
		return nil, err
	}

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
	for _, lease := range leases {
		if err := validateDSeq(lease.DSeq); err != nil {
			return nil, err
		}
	}

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
	if err == nil && !requestedLeasesActive(out.Leases, leases) {
		err = errors.New("console: create lease response omitted a requested active lease")
	}
	if err != nil {
		// Never replay this POST: the live API can return an error after the
		// lease transaction succeeded. Read back the exact lease identities
		// and accept only authoritative active state.
		if detail, ok := c.reconcileCreatedLeases(ctx, leases); ok {
			c.record("create-lease", leaseDSeq, nil)
			return detail, nil
		}

		if definitiveCreateFailure(err) {
			c.record("create-lease", leaseDSeq, err)
			return nil, err
		}

		unknown := fmt.Errorf("lease creation outcome unknown after one submission (%w); the request was not replayed: inspect deployment %s for the requested lease", err, leaseDSeq)
		c.recordOutcome("create-lease", leaseDSeq, "pending", unknown, nil)
		return nil, unknown
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
	if err := validateDSeq(dseq); err != nil {
		return nil, err
	}

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
	if err := validateDSeq(dseq); err != nil {
		c.record("update-deployment-settings", dseq, err)
		return nil, err
	}
	if _, err := c.requireMutableDeployment(ctx, dseq); err != nil {
		wrapped := fmt.Errorf("preflight deployment settings: %w", err)
		c.record("update-deployment-settings", dseq, wrapped)
		return nil, wrapped
	}

	var out struct {
		DSeq                 *FlexString `json:"dseq"`
		AutoTopUpEnabled     *bool       `json:"autoTopUpEnabled"`
		EstimatedTopUpAmount float64     `json:"estimatedTopUpAmount"`
		TopUpFrequencyMs     int64       `json:"topUpFrequencyMs"`
	}

	err := c.doData(ctx, http.MethodPatch, "/v2/deployment-settings/"+url.PathEscape(dseq),
		envelope(map[string]any{"autoTopUpEnabled": enabled}), &out)
	if errors.Is(err, ErrNotFound) {
		err = c.doData(ctx, http.MethodPost, "/v2/deployment-settings",
			envelope(map[string]any{"dseq": dseq, "autoTopUpEnabled": enabled}), &out)
	}
	if err == nil && (out.DSeq == nil || out.DSeq.String() != dseq || out.AutoTopUpEnabled == nil || *out.AutoTopUpEnabled != enabled) {
		err = errors.New("console: deployment settings response did not echo the requested dseq and auto-top-up value")
	}

	c.record("update-deployment-settings", dseq, err)
	if err != nil {
		return nil, err
	}

	return &DeploymentSettings{
		DSeq:                 *out.DSeq,
		AutoTopUpEnabled:     *out.AutoTopUpEnabled,
		EstimatedTopUpAmount: out.EstimatedTopUpAmount,
		TopUpFrequencyMs:     out.TopUpFrequencyMs,
	}, nil
}

func (c *Client) requireMutableDeployment(ctx context.Context, dseq string) (*DeploymentDetail, error) {
	detail, err := c.GetDeployment(ctx, dseq)
	if err != nil {
		return nil, err
	}
	if err := requireMutableState(detail, dseq); err != nil {
		return nil, err
	}

	return detail, nil
}

func requireMutableState(detail *DeploymentDetail, dseq string) error {
	state := "unknown"
	if detail != nil && strings.TrimSpace(detail.Deployment.State) != "" {
		state = strings.ToLower(strings.TrimSpace(detail.Deployment.State))
	}
	if state == "closed" {
		return fmt.Errorf("console: deployment %s is closed and cannot be modified", dseq)
	}

	return nil
}
