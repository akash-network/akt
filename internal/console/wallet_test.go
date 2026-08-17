package console_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/akt/internal/console"
)

func TestGetUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/user/me", r.URL.Path)

		_, _ = w.Write([]byte(`{"data":{"id":"uuid-internal","userId":"auth0|abc","username":"alice","email":"a@example.com","emailVerified":true}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	u, err := c.GetUser(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "uuid-internal", u.ID)
	assert.Equal(t, "auth0|abc", u.UserID)
	assert.True(t, u.EmailVerified)
}

func TestListWallets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/wallets", r.URL.Path)
		assert.Equal(t, "uuid-internal", r.URL.Query().Get("userId"))

		_, _ = w.Write([]byte(`{"data":[{"address":"akash1abc","creditAmount":25.5,"isTrialing":true,"denom":"usdc"}]}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	ws, err := c.ListWallets(context.Background(), "uuid-internal")
	require.NoError(t, err)
	require.Len(t, ws, 1)
	assert.Equal(t, "akash1abc", ws[0].Address)
	assert.InDelta(t, 25.5, ws[0].CreditAmount, 0.001)
	assert.True(t, ws[0].IsTrialing)
}

func TestWalletSettingsRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/wallet-settings", r.URL.Path)

		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"autoReloadEnabled":false}}`))

		case http.MethodPut:
			data := decodeDataBody(t, r)
			assert.Equal(t, true, data["autoReloadEnabled"])
			_, _ = w.Write([]byte(`{"data":{"autoReloadEnabled":true}}`))

		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")

	s, err := c.GetWalletSettings(context.Background())
	require.NoError(t, err)
	assert.False(t, s.AutoReloadEnabled)

	s, err = c.UpdateWalletSettings(context.Background(), true)
	require.NoError(t, err)
	assert.True(t, s.AutoReloadEnabled)
}

func TestGetWeeklyCost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/weekly-cost", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"weeklyCost":13.37}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	cost, err := c.GetWeeklyCost(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 13.37, cost, 0.001)
}

func TestGetUsageHistoryTopLevelArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/usage/history", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, "akash1abc", q.Get("address"))
		assert.Equal(t, "2026-07-01", q.Get("startDate"))
		assert.Equal(t, "2026-07-21", q.Get("endDate"))

		// Top-level array, NOT data-enveloped.
		_, _ = w.Write([]byte(`[
			{"date":"2026-07-01","activeDeployments":2,"dailyUsdcSpent":1.25,"totalUsdcSpent":10.0},
			{"date":"2026-07-02","activeDeployments":3,"dailyUsdcSpent":2.5,"totalUsdcSpent":12.5}
		]`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "test-key")
	points, err := c.GetUsageHistory(context.Background(), "akash1abc", "2026-07-01", "2026-07-21")
	require.NoError(t, err)
	require.Len(t, points, 2)
	assert.Equal(t, "2026-07-01", points[0].Date)
	assert.Equal(t, 3, points[1].ActiveDeployments)
	assert.InDelta(t, 12.5, points[1].TotalUsdcSpent, 0.001)
}

func TestGetUsageHistoryOmitsEmptyDates(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "k")

	if _, err := c.GetUsageHistory(context.Background(), "akash1abc", "", ""); err != nil {
		t.Fatalf("GetUsageHistory: %v", err)
	}

	if gotQuery.Get("address") != "akash1abc" {
		t.Errorf("address missing from query: %v", gotQuery)
	}
	// Empty dates must be omitted entirely — the API rejects empty strings
	// with a format=date validation error and defaults omitted values.
	if _, present := gotQuery["startDate"]; present {
		t.Errorf("empty startDate must be omitted, got %v", gotQuery)
	}
	if _, present := gotQuery["endDate"]; present {
		t.Errorf("empty endDate must be omitted, got %v", gotQuery)
	}
}

func TestGetUsageHistoryValidatesDateFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request should be sent for an invalid date")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "k")

	if _, err := c.GetUsageHistory(context.Background(), "akash1abc", "01/02/2026", ""); err == nil {
		t.Fatal("expected invalid date format to be rejected client-side")
	}
}

// TestUpdateWalletSettingsCreatesWhenAbsent covers the account state that is
// most common: auto-reload has never been configured, so there is no settings
// record and PUT answers 404. Without the POST fallback the update fails on
// exactly the accounts that need it.
func TestUpdateWalletSettingsCreatesWhenAbsent(t *testing.T) {
	var methods []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)

		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"NotFound","message":"no wallet settings"}`))

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"autoReloadEnabled":true}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")

	got, err := c.UpdateWalletSettings(context.Background(), true)
	require.NoError(t, err)
	assert.True(t, got.AutoReloadEnabled)
	assert.Equal(t, []string{http.MethodPut, http.MethodPost}, methods,
		"a 404 from PUT must fall back to POST to create the record")
}

// TestUpdateWalletSettingsUsesPutWhenPresent guards the other direction: an
// account that already has settings must not be sent a redundant create.
func TestUpdateWalletSettingsUsesPutWhenPresent(t *testing.T) {
	var methods []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"autoReloadEnabled":false}}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "key")

	_, err := c.UpdateWalletSettings(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, []string{http.MethodPut}, methods, "an existing record needs only the PUT")
}

func TestUpdateWalletSettingsRejectsMissingOrMismatchedAcknowledgement(t *testing.T) {
	for _, body := range []string{`{"data":{}}`, `{"data":{"autoReloadEnabled":false}}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			_, err := console.New(srv.URL, "key").UpdateWalletSettings(context.Background(), true)
			require.ErrorContains(t, err, "did not echo")
		})
	}
}
