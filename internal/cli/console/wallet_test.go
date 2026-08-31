package console

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// TestWalletListShowsDollarScaleCredits pins the /v1/wallets scaling:
// creditAmount is dollar-scale (reported with a denom, e.g. "usdc"), unlike
// /v1/balances which is µACT. A 25.5 credit must render as $25.50, not the
// $0.00 produced by dividing by 1e6.
func TestWalletListShowsDollarScaleCredits(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user/me":
			writeJSON(t, w, `{"data":{"id":"u1","username":"joe"}}`)
		case "/v1/wallets":
			// Mirrors the fixture in internal/console/wallet_test.go.
			writeJSON(t, w, `{"data":[{"address":"akash1abc","creditAmount":25.5,"isTrialing":true,"denom":"usdc"}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "wallet", "list")
	if err != nil {
		t.Fatalf("wallet list: %v", err)
	}

	if !strings.Contains(out, `"balance": "$25.50"`) {
		t.Errorf("wallet list must render creditAmount 25.5 as $25.50, got %q", out)
	}
	if strings.Contains(out, "$0.00") {
		t.Errorf("creditAmount must not be scaled as µ-denominated, got %q", out)
	}
	if !strings.Contains(out, `"denom": "usdc"`) {
		t.Errorf("wallet list should report the API's denom, got %q", out)
	}
}

func TestWalletAddressPrintsFullManagedAddress(t *testing.T) {
	const address = "akash1gnz8venxvenxvenxvenxvenxvenxvenx4m3e0y"
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user/me":
			writeJSON(t, w, `{"data":{"id":"u1"}}`)
		case "/v1/wallets":
			writeJSON(t, w, `{"data":[{"address":"`+address+`"}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL.RequestURI())
		}
	}))
	defer srv.Close()

	for _, format := range []string{"pretty", "json", "yaml"} {
		out, err := execConsole(t, m, srv.URL, "wallet", "address", "--output", format)
		if err != nil {
			t.Fatalf("wallet address (%s): %v", format, err)
		}
		if !strings.Contains(out, address) {
			t.Errorf("wallet address (%s) omitted full address: %q", format, out)
		}
	}
}

func TestWalletAddressReportsCredentialAndManagedWalletErrors(t *testing.T) {
	t.Run("missing Console credential", func(t *testing.T) {
		_, err := execConsole(t, newTestManager(t), "", "wallet", "address")
		if err == nil || !strings.Contains(err.Error(), "Console API key") {
			t.Fatalf("missing credential error = %v", err)
		}
	})

	t.Run("managed wallet lookup", func(t *testing.T) {
		m := newTestManager(t)
		if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
			t.Fatalf("SetConsoleAPIKey: %v", err)
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		_, err := execConsole(t, m, srv.URL, "wallet", "address")
		if err == nil || !strings.Contains(err.Error(), "managed wallet user") {
			t.Fatalf("managed wallet lookup error = %v", err)
		}
	})
}

// TestWalletSettingsReportsOneShape pins `wallet settings` to the shaped
// {autoReloadEnabled, configured} object on both success paths. The read and
// write paths used to hand the raw API record to the printer, so the same
// command answered with a different shape depending on which branch ran —
// and never reported "configured", which only the 404 branch produced.
func TestWalletSettingsReportsOneShape(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/wallet-settings" {
			t.Errorf("unexpected request %s", r.URL.Path)
			return
		}
		writeJSON(t, w, `{"data":{"autoReloadEnabled":true}}`)
	}))
	defer srv.Close()

	for name, args := range map[string][]string{
		"Read":  {"wallet", "settings"},
		"Write": {"wallet", "settings", "true"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := execConsole(t, m, srv.URL, args...)
			if err != nil {
				t.Fatalf("wallet settings: %v", err)
			}

			if !strings.Contains(out, `"autoReloadEnabled": true`) {
				t.Errorf("wallet settings should report autoReloadEnabled, got %q", out)
			}
			if !strings.Contains(out, `"configured": true`) {
				t.Errorf("wallet settings should report configured, got %q", out)
			}
		})
	}
}

// TestWalletSettingsUnconfiguredReportsSameShape pins the 404 path to the same
// object, plus the note naming the command that configures it.
func TestWalletSettingsUnconfiguredReportsSameShape(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "wallet", "settings")
	if err != nil {
		t.Fatalf("wallet settings: %v", err)
	}

	if !strings.Contains(out, `"autoReloadEnabled": false`) || !strings.Contains(out, `"configured": false`) {
		t.Errorf("unconfigured settings should report the defaults, got %q", out)
	}
	if !strings.Contains(out, "akt console wallet settings true") {
		t.Errorf("unconfigured settings should name the remedy, got %q", out)
	}
}

// usageTestServer serves the three endpoints the usage command touches, with
// the history body swappable between calls.
func usageTestServer(t *testing.T, historyBody *string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user/me":
			writeJSON(t, w, `{"data":{"id":"u1","username":"joe"}}`)
		case "/v1/wallets":
			writeJSON(t, w, `{"data":[{"address":"akash1wallet","creditAmount":25.5,"denom":"usdc"}]}`)
		case "/v1/usage/history":
			writeJSON(t, w, *historyBody)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
}

// TestUsageTotalSpentSumsRequestedRange pins the headline totalSpent to the
// sum of the per-day values within the requested range — not the cumulative
// lifetime figure carried by whichever point happens to come last — and
// proves the result is independent of the API's point ordering. The lifetime
// figure is reported separately as the range maximum of totalUsdcSpent.
func TestUsageTotalSpentSumsRequestedRange(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	const (
		ascending = `[
			{"date":"2026-07-01","activeDeployments":2,"dailyUsdcSpent":1.25,"totalUsdcSpent":10.0},
			{"date":"2026-07-02","activeDeployments":3,"dailyUsdcSpent":2.5,"totalUsdcSpent":12.5}
		]`
		descending = `[
			{"date":"2026-07-02","activeDeployments":3,"dailyUsdcSpent":2.5,"totalUsdcSpent":12.5},
			{"date":"2026-07-01","activeDeployments":2,"dailyUsdcSpent":1.25,"totalUsdcSpent":10.0}
		]`
	)

	body := ascending
	srv := usageTestServer(t, &body)
	defer srv.Close()

	for name, points := range map[string]string{"ascending": ascending, "descending": descending} {
		body = points

		out, err := execConsole(t, m, srv.URL, "usage", "2026-07-01", "2026-07-02")
		if err != nil {
			t.Fatalf("usage (%s points): %v", name, err)
		}

		// 1.25 + 2.5 within the range, regardless of point order.
		if !strings.Contains(out, `"totalSpent": "$3.75"`) {
			t.Errorf("%s points: totalSpent must be the range sum $3.75, got %q", name, out)
		}
		// Lifetime cumulative spend as of the range end, order-independent.
		if !strings.Contains(out, `"lifetimeSpent": "$12.50"`) {
			t.Errorf("%s points: lifetimeSpent must be $12.50, got %q", name, out)
		}
	}
}

// TestUsageEmptyHistoryOmitsLifetime pins the empty-range shape: a $0.00
// range total and no lifetime figure (there is no point to read it from).
func TestUsageEmptyHistoryOmitsLifetime(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	body := `[]`
	srv := usageTestServer(t, &body)
	defer srv.Close()

	out, err := execConsole(t, m, srv.URL, "usage")
	if err != nil {
		t.Fatalf("usage: %v", err)
	}

	if !strings.Contains(out, `"totalSpent": "$0.00"`) {
		t.Errorf("empty history should total $0.00, got %q", out)
	}
	if strings.Contains(out, "lifetimeSpent") {
		t.Errorf("empty history must omit lifetimeSpent, got %q", out)
	}
}
