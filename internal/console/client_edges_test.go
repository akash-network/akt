package console_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	requested := make(chan *http.Request, 1)
	http.DefaultTransport = consoleRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested <- request.Clone(request.Context())
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       http.NoBody,
		}, nil
	})

	_, _ = console.New("", "k").GetUser(context.Background())
	request := <-requested
	if request.URL.Scheme != "https" || request.URL.Host != "console-api.akash.network" || request.URL.Path != "/v1/user/me" {
		t.Errorf("an empty baseURL resolved to %s, want %s/v1/user/me", request.URL, console.DefaultBaseURL)
	}
}

func TestDeploymentURLOnlyLinksProductionConsole(t *testing.T) {
	if got := console.New("", "key").DeploymentURL("4242"); got != "https://console.akash.network/deployments/4242" {
		t.Fatalf("production deployment URL = %q", got)
	}
	if got := console.New("https://console-api.example.test", "key").DeploymentURL("4242"); got != "" {
		t.Fatalf("custom API deployment URL = %q, want empty", got)
	}
	if got := console.New("", "key").DeploymentURL("0"); got != "" {
		t.Fatalf("invalid deployment URL = %q, want empty", got)
	}
}

type consoleRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn consoleRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		cancel()
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "k").GetDeployment(ctx, "1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request error = %v, want context.Canceled", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("cancellation should stop after the observed request, got %d calls", got)
	}
}

func TestCanceledRequestReturnsContextCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := console.New("http://127.0.0.1:1", "k").GetDeployment(ctx, "1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request error = %v, want context.Canceled", err)
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

// TestTransportFailureIsNotRetried covers the request-error branch: a dropped
// connection returns immediately rather than burning the retry budget, and the
// message names the method and path so the failing call is identifiable.
func TestTransportFailureIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack connection: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	_, err := console.New(srv.URL, "k").GetDeployment(context.Background(), "1")
	if err == nil {
		t.Fatal("a transport failure must surface")
	}
	if !strings.Contains(err.Error(), "/v1/deployments/1") {
		t.Errorf("error should name the failing path, got %q", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("a transport failure must not be retried, got %d requests", got)
	}
}
