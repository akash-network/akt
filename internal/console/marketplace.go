package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Public marketplace/catalog endpoints. None of these require an API key;
// the key is still sent when the client has one configured.

// ScreenBids asks the Console API which providers can satisfy the given
// requirements/resources before a deployment is created. The API contract
// requires both Resources and Timezone (an IANA zone name); requests missing
// either are rejected locally instead of triggering an HTTP 400.
//
// Wire: POST /v1/bid-screening (request and response are NOT data-enveloped;
// the response is {"providers":[...]}).
func (c *Client) ScreenBids(ctx context.Context, req *BidScreeningRequest) ([]ScreenedProvider, error) {
	if req == nil {
		return nil, fmt.Errorf("console: bid screening: request is nil")
	}
	if len(req.Resources) == 0 {
		return nil, fmt.Errorf("console: bid screening: resources are required")
	}
	if req.Timezone == "" {
		return nil, fmt.Errorf("console: bid screening: timezone is required (IANA zone name, e.g. America/Chicago)")
	}

	var out struct {
		Providers []ScreenedProvider `json:"providers"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/bid-screening", req, &out); err != nil {
		return nil, err
	}

	return out.Providers, nil
}

// ListProviders lists providers. scope is optional (e.g. "trial");
// addresses optionally restricts the result to specific provider addresses.
//
// Wire: GET /v1/providers?scope=&addresses=. NOTE: the response is a
// TOP-LEVEL array, not data-enveloped.
func (c *Client) ListProviders(ctx context.Context, scope string, addresses []string) ([]Provider, error) {
	q := url.Values{}
	if scope != "" {
		q.Set("scope", scope)
	}
	if len(addresses) > 0 {
		q.Set("addresses", strings.Join(addresses, ","))
	}

	path := "/v1/providers"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}

	var out []Provider
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// GetProvider fetches detailed information (including stats and hostUri) for
// one provider. Common fields are typed; the full document is preserved in
// ProviderDetail.Raw.
//
// Wire: GET /v1/providers/{address}, top-level object.
func (c *Client) GetProvider(ctx context.Context, address string) (*ProviderDetail, error) {
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, "/v1/providers/"+url.PathEscape(address), nil, &raw); err != nil {
		return nil, err
	}

	var out ProviderDetail
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("console: decode provider detail: %w", err)
	}
	out.Raw = raw

	return &out, nil
}

// ListProviderRegions lists the regions providers advertise.
//
// Wire: GET /v1/provider-regions, TOP-LEVEL array.
func (c *Client) ListProviderRegions(ctx context.Context) ([]ProviderRegion, error) {
	var out []ProviderRegion
	if err := c.doJSON(ctx, http.MethodGet, "/v1/provider-regions", nil, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// ListAuditors lists known auditors.
//
// Wire: GET /v1/auditors, TOP-LEVEL array.
func (c *Client) ListAuditors(ctx context.Context) ([]Auditor, error) {
	var out []Auditor
	if err := c.doJSON(ctx, http.MethodGet, "/v1/auditors", nil, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// GetGPUPrices fetches the network-wide GPU availability and price catalog.
//
// Wire: GET /v1/gpu-prices, top-level object (not data-enveloped).
func (c *Client) GetGPUPrices(ctx context.Context) (*GPUPrices, error) {
	var out GPUPrices
	if err := c.doJSON(ctx, http.MethodGet, "/v1/gpu-prices", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// ListTemplates fetches the template catalog. The category/template tree is
// loosely specified upstream, so the enveloped payload is returned raw.
//
// Wire: GET /v1/templates-list, data-enveloped response.
func (c *Client) ListTemplates(ctx context.Context) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.doData(ctx, http.MethodGet, "/v1/templates-list", nil, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// GetTemplate fetches one deployment template by ID.
//
// Wire: GET /v1/templates/{id}, data-enveloped response.
func (c *Client) GetTemplate(ctx context.Context, id string) (*Template, error) {
	var out Template
	if err := c.doData(ctx, http.MethodGet, "/v1/templates/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
