package console

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// GetUser fetches the authenticated Console user. The returned User.ID is the
// internal UUID needed by ListWallets.
//
// Wire: GET /v1/user/me, data-enveloped response.
func (c *Client) GetUser(ctx context.Context) (*User, error) {
	var out User
	if err := c.doData(ctx, http.MethodGet, "/v1/user/me", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// GetBalances fetches managed-wallet balances in µACT integer units. Use the
// *USD helpers on Balances for display values.
//
// Wire: GET /v1/balances, data-enveloped response.
func (c *Client) GetBalances(ctx context.Context) (*Balances, error) {
	var out Balances
	if err := c.doData(ctx, http.MethodGet, "/v1/balances", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// ListWallets lists managed wallets for a user. userID is the internal UUID
// from GetUser().ID, not the userId field.
//
// Wire: GET /v1/wallets?userId=, response {"data":[...]}.
func (c *Client) ListWallets(ctx context.Context, userID string) ([]Wallet, error) {
	path := "/v1/wallets?userId=" + url.QueryEscape(userID)

	var out []Wallet
	if err := c.doData(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// GetWalletSettings fetches account-level wallet settings.
//
// Wire: GET /v1/wallet-settings, data-enveloped response.
func (c *Client) GetWalletSettings(ctx context.Context) (*WalletSettings, error) {
	var out WalletSettings
	if err := c.doData(ctx, http.MethodGet, "/v1/wallet-settings", nil, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// UpdateWalletSettings enables or disables wallet auto-reload and returns the
// stored settings.
//
// Wire: PUT /v1/wallet-settings, body {"data":{"autoReloadEnabled":...}},
// data-enveloped response.
func (c *Client) UpdateWalletSettings(ctx context.Context, autoReloadEnabled bool) (*WalletSettings, error) {
	body := envelope(map[string]any{"autoReloadEnabled": autoReloadEnabled})

	var out WalletSettings
	err := c.doData(ctx, http.MethodPut, "/v1/wallet-settings", body, &out)
	c.record("update-wallet-settings", "", err)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

// GetWeeklyCost fetches the trailing weekly spend in USD.
//
// Wire: GET /v1/weekly-cost, response {"data":{"weeklyCost":...}}.
func (c *Client) GetWeeklyCost(ctx context.Context) (float64, error) {
	var out struct {
		WeeklyCost float64 `json:"weeklyCost"`
	}
	if err := c.doData(ctx, http.MethodGet, "/v1/weekly-cost", nil, &out); err != nil {
		return 0, err
	}

	return out.WeeklyCost, nil
}

// GetUsageHistory fetches daily spend history for an address between
// startDate and endDate (YYYY-MM-DD). Empty dates are omitted — the API
// then defaults endDate to today and startDate to 30 days before endDate;
// sending an empty string fails the API's format=date validation.
//
// Wire: GET /v1/usage/history?address=&startDate=&endDate=. NOTE: the
// response is a TOP-LEVEL array, not data-enveloped.
func (c *Client) GetUsageHistory(ctx context.Context, address, startDate, endDate string) ([]UsagePoint, error) {
	for _, d := range []string{startDate, endDate} {
		if d == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return nil, fmt.Errorf("console: invalid date %q: expected YYYY-MM-DD", d)
		}
	}

	q := url.Values{}
	q.Set("address", address)
	if startDate != "" {
		q.Set("startDate", startDate)
	}
	if endDate != "" {
		q.Set("endDate", endDate)
	}

	var out []UsagePoint
	if err := c.doJSON(ctx, http.MethodGet, "/v1/usage/history?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}

	return out, nil
}
