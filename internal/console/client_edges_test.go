package console_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pkg.akt.dev/akt/internal/console"
)

// TestNewDefaultsToProductionBaseURL pins where a client with no explicit base
// URL sends its API key. An empty baseURL must resolve to the production host,
// not to a relative path (which would make every request fail) — and must not
// silently become some other host.
func TestNewDefaultsToProductionBaseURL(t *testing.T) {
	if console.DefaultBaseURL != "https://console-api.akash.network" {
		t.Fatalf("DefaultBaseURL = %q; the credential destination changed", console.DefaultBaseURL)
	}

	// A client built with "" must not attempt a relative request: point at a
	// context that is already cancelled so no traffic leaves the machine, and
	// assert the error names the default host.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := console.New("", "k").GetUser(ctx)
	if err == nil {
		t.Fatal("expected the cancelled context to fail the request")
	}
	if !strings.Contains(err.Error(), "console-api.akash.network") {
		t.Errorf("an empty baseURL must resolve to the default host, got %q", err)
	}
}

// TestBalancesUSDHelpersScaleEveryField pins the µACT -> USD conversion for
// all three balance fields. DeploymentsUSD reports escrowed funds and is the
// one with no other caller inside this package, so a regression there would
// misreport locked money by six orders of magnitude.
func TestBalancesUSDHelpersScaleEveryField(t *testing.T) {
	b := console.Balances{Balance: 12340000, Deployments: 7500000, Total: 19840000}

	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"BalanceUSD", b.BalanceUSD(), 12.34},
		{"DeploymentsUSD", b.DeploymentsUSD(), 7.5},
		{"TotalUSD", b.TotalUSD(), 19.84},
	}

	for _, c := range cases {
		if diff := c.got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	// Zero must stay zero rather than becoming a NaN or a scaled artifact.
	var zero console.Balances
	if zero.BalanceUSD() != 0 || zero.DeploymentsUSD() != 0 || zero.TotalUSD() != 0 {
		t.Errorf("zero balances must render as 0, got %+v", zero)
	}
}

// TestScreenBidsValidatesLocally covers the three guards that keep a malformed
// screening request from reaching the API as an opaque HTTP 400. Bid screening
// decides which providers are offered a deployment, so a silently-dropped
// requirement is a real correctness problem.
func TestScreenBidsValidatesLocally(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		_, _ = w.Write([]byte(`{"providers":[]}`))
	}))
	defer srv.Close()

	c := console.New(srv.URL, "k")

	cases := []struct {
		name string
		req  *console.BidScreeningRequest
		want string
	}{
		{"nil request", nil, "request is nil"},
		{
			"no resources",
			&console.BidScreeningRequest{Timezone: "America/Chicago"},
			"resources are required",
		},
		{
			"no timezone",
			&console.BidScreeningRequest{Resources: []byte(`{"cpu":1}`)},
			"timezone is required",
		},
	}

	for _, tc := range cases {
		_, err := c.ScreenBids(context.Background(), tc.req)
		if err == nil {
			t.Errorf("%s: expected a local rejection", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %q should mention %q", tc.name, err, tc.want)
		}
	}

	if n := atomic.LoadInt32(&requests); n != 0 {
		t.Errorf("locally-invalid screening requests must not be sent, got %d", n)
	}
}

// TestRetryAbortsOnContextCancellation covers the ctx.Done() arm of the retry
// backoff. Without it a cancelled command (Ctrl-C, a shell timeout) would keep
// re-sending for the full backoff schedule.
func TestRetryAbortsOnContextCancellation(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel while the client is sitting in its first backoff (100ms).
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := console.New(srv.URL, "k").GetDeployment(ctx, "1")
	if err == nil {
		t.Fatal("a cancelled context must surface an error")
	}
	if got := calls.Load(); got > 1 {
		t.Errorf("cancellation should stop further attempts, got %d calls", got)
	}
}

// TestDataEnvelopeDecodeErrorIsReported covers the unmarshal branch of doData:
// a data envelope whose payload has the wrong JSON type must be an error, not
// a silently zero-valued result.
func TestDataEnvelopeDecodeErrorIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Balances expects an object; hand it a string.
		_, _ = w.Write([]byte(`{"data":"not-an-object"}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "k").GetBalances(context.Background())
	if err == nil {
		t.Fatal("a wrong-typed data envelope must be an error")
	}
	if !strings.Contains(err.Error(), "decode data envelope") {
		t.Errorf("error should name the decode failure, got %q", err)
	}
}

// TestUndecodableSuccessBodyIsReported covers the non-enveloped decode branch
// of doJSON (used by the top-level-array endpoints): a 200 with a body that
// does not match the expected shape must fail rather than return an empty
// slice that reads as "no usage".
func TestUndecodableSuccessBodyIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"unexpected":"object"}`))
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "k").GetUsageHistory(context.Background(), "akash1x", "", "")
	if err == nil {
		t.Fatal("a body that is not the documented array must be an error")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("error should name the decode failure, got %q", err)
	}
}

// TestTransportFailureIsNotRetried covers the request-error branch: a dial
// failure returns immediately rather than burning the retry budget, and the
// message names the method and path so the failing call is identifiable.
func TestTransportFailureIsNotRetried(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	start := time.Now()
	_, err := console.New(url, "k").GetDeployment(context.Background(), "1")
	if err == nil {
		t.Fatal("a dial failure must surface")
	}
	if !strings.Contains(err.Error(), "/v1/deployments/1") {
		t.Errorf("error should name the failing path, got %q", err)
	}
	// Three attempts with backoff would take at least 300ms.
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Errorf("a dial failure must not be retried, took %s", elapsed)
	}
}
