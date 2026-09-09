package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConsoleSandboxFairUsePolicyPreparation(t *testing.T) {
	for _, alreadyAccepted := range []bool{false, true} {
		t.Run(fmt.Sprintf("already_accepted_%t", alreadyAccepted), func(t *testing.T) {
			const apiKey = "akt_policy_test_secret"
			var accepted atomic.Bool
			accepted.Store(alreadyAccepted)
			var reads, writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-api-key") != apiKey {
					t.Error("sandbox setup did not use the configured credential")
				}
				switch r.Method + " " + r.URL.Path {
				case "GET /v1/user/me":
					reads.Add(1)
					if accepted.Load() {
						_, _ = fmt.Fprint(w, `{"data":{"fairUsePolicyAcceptedAt":"2026-09-09T17:00:00.123Z"}}`)
					} else {
						_, _ = fmt.Fprint(w, `{"data":{"fairUsePolicyAcceptedAt":null}}`)
					}
				case "POST /v1/user/acceptFairUsePolicy":
					if reads.Load() != 1 || accepted.Load() || r.ContentLength != 0 {
						t.Error("acceptance must follow an unaccepted profile and have no request body")
					}
					writes.Add(1)
					accepted.Store(true)
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected sandbox setup request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			observer := newConsoleAPIObserver(server.URL, apiKey)
			for range 2 {
				if err := acceptConsoleSandboxFairUsePolicy(t.Context(), observer); err != nil {
					t.Fatal(err)
				}
			}
			wantWrites, wantReads := int32(1), int32(3)
			if alreadyAccepted {
				wantWrites, wantReads = 0, 2
			}
			if writes.Load() != wantWrites || reads.Load() != wantReads {
				t.Fatalf("setup made %d writes and %d reads, want %d and %d", writes.Load(), reads.Load(), wantWrites, wantReads)
			}
		})
	}
}

func TestConsoleSandboxFairUsePolicyRejectsUnconfirmedAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		confirmation string
		wantError    string
	}{
		{name: "forbidden", status: http.StatusForbidden, wantError: "HTTP 403"},
		{name: "server error", status: http.StatusInternalServerError, wantError: "HTTP 500"},
		{name: "unexpected success status", status: http.StatusOK, wantError: "HTTP 200"},
		{name: "not recorded", status: http.StatusNoContent, confirmation: `{"data":{"fairUsePolicyAcceptedAt":null}}`, wantError: "did not record"},
		{name: "missing field", status: http.StatusNoContent, confirmation: `{"data":{}}`, wantError: "did not record"},
		{name: "invalid timestamp", status: http.StatusNoContent, confirmation: `{"data":{"fairUsePolicyAcceptedAt":"private-policy-payload"}}`, wantError: "timestamp"},
		{name: "empty timestamp", status: http.StatusNoContent, confirmation: `{"data":{"fairUsePolicyAcceptedAt":""}}`, wantError: "timestamp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const apiKey = "akt_policy_test_secret"
			var writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					writes.Add(1)
					w.WriteHeader(tc.status)
					if tc.status != http.StatusNoContent {
						_, _ = fmt.Fprint(w, apiKey+" private-policy-payload")
					}
				} else if writes.Load() == 0 {
					_, _ = fmt.Fprint(w, `{"data":{"fairUsePolicyAcceptedAt":null}}`)
				} else {
					_, _ = fmt.Fprint(w, tc.confirmation)
				}
			}))
			t.Cleanup(server.Close)
			err := acceptConsoleSandboxFairUsePolicy(t.Context(), newConsoleAPIObserver(server.URL, apiKey))
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("setup error = %v, want %q", err, tc.wantError)
			}
			if strings.Contains(err.Error(), apiKey) || strings.Contains(err.Error(), "private-policy-payload") {
				t.Fatal("setup error disclosed a credential or response body")
			}
			if writes.Load() != 1 {
				t.Fatalf("setup made %d acceptance requests, want one without retries", writes.Load())
			}
		})
	}
}

func TestConsoleSandboxFairUsePolicyRejectsRedirects(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetRequests.Add(1)
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = fmt.Fprint(w, `{"data":{"fairUsePolicyAcceptedAt":null}}`)
		} else {
			http.Redirect(w, r, target.URL+"/private-policy-payload", http.StatusTemporaryRedirect)
		}
	}))
	t.Cleanup(source.Close)
	observer := newConsoleAPIObserver(source.URL, "akt_policy_test_secret")
	err := acceptConsoleSandboxFairUsePolicy(t.Context(), observer)
	if err == nil || strings.Contains(err.Error(), "private-policy-payload") {
		t.Fatalf("redirect rejection was not safely reported: %v", err)
	}
	if targetRequests.Load() != 0 {
		t.Fatal("acceptance followed a redirect")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := acceptConsoleSandboxFairUsePolicy(canceled, observer); err == nil {
		t.Fatal("setup ignored cancellation")
	}
}
