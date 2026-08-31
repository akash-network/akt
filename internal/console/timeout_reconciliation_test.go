package console

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type timeoutTestRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn timeoutTestRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func timeoutTestResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func TestDoJSONAppliesMethodAwareRequestDeadlines(t *testing.T) {
	tests := []struct {
		method string
		want   time.Duration
	}{
		{method: http.MethodGet, want: 30 * time.Second},
		{method: http.MethodHead, want: 30 * time.Second},
		{method: http.MethodPost, want: 2 * time.Minute},
		{method: http.MethodPut, want: 2 * time.Minute},
		{method: http.MethodPatch, want: 2 * time.Minute},
		{method: http.MethodDelete, want: 2 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			var observed time.Time
			client := New("https://console.example.test", "key")
			client.httpClient.Transport = timeoutTestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				var ok bool
				observed, ok = request.Context().Deadline()
				if !ok {
					t.Fatal("request context has no deadline")
				}
				return timeoutTestResponse(request, http.StatusNoContent, ""), nil
			})

			started := time.Now()
			if err := client.doJSON(context.Background(), tt.method, "/test", nil, nil); err != nil {
				t.Fatalf("doJSON: %v", err)
			}

			got := observed.Sub(started)
			if got < tt.want-time.Second || got > tt.want+time.Second {
				t.Fatalf("request deadline = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDoJSONPreservesEarlierCallerDeadline(t *testing.T) {
	client := New("https://console.example.test", "key")
	var observed time.Time
	client.httpClient.Transport = timeoutTestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		observed, _ = request.Context().Deadline()
		return timeoutTestResponse(request, http.StatusNoContent, ""), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	want, _ := ctx.Deadline()
	if err := client.doJSON(ctx, http.MethodPost, "/test", nil, nil); err != nil {
		t.Fatalf("doJSON: %v", err)
	}
	if delta := observed.Sub(want); delta < -time.Millisecond || delta > time.Millisecond {
		t.Fatalf("request deadline differs from caller by %s", delta)
	}
}

func TestCreateLeaseReconcilesTransportFailureWithoutReplayingPost(t *testing.T) {
	var posts atomic.Int32
	var gets atomic.Int32
	client := New("https://console.example.test", "key")
	client.httpClient.Transport = timeoutTestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.Method {
		case http.MethodPost:
			posts.Add(1)
			return nil, errors.New("simulated response timeout")
		case http.MethodGet:
			gets.Add(1)
			return timeoutTestResponse(request, http.StatusOK, `{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"},"leases":[{"id":{"dseq":"42","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active"}]}}`), nil
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
			return nil, nil
		}
	})

	result, err := client.CreateLease(context.Background(), "manifest", []LeaseRequest{{
		DSeq: "42", GSeq: 1, OSeq: 1, Provider: "akash1provider",
	}})
	if err != nil {
		t.Fatalf("CreateLease: %v", err)
	}
	if len(result.Leases) != 1 || result.Leases[0].State != "active" {
		t.Fatalf("reconciled leases = %+v, want exact active lease", result.Leases)
	}
	if posts.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("requests POST=%d GET=%d, want one of each", posts.Load(), gets.Load())
	}
}

func TestCloseDeploymentReconcilesTransportFailureWithoutReplayingDelete(t *testing.T) {
	var gets atomic.Int32
	var deletes atomic.Int32
	var closed atomic.Bool
	client := New("https://console.example.test", "key")
	client.httpClient.Transport = timeoutTestRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.Method {
		case http.MethodGet:
			gets.Add(1)
			state := "active"
			if closed.Load() {
				state = "closed"
			}
			return timeoutTestResponse(request, http.StatusOK, `{"data":{"deployment":{"id":{"dseq":"42"},"state":"`+state+`"}}}`), nil
		case http.MethodDelete:
			deletes.Add(1)
			closed.Store(true)
			return nil, errors.New("simulated response timeout")
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
			return nil, nil
		}
	})

	if err := client.CloseDeployment(context.Background(), "42"); err != nil {
		t.Fatalf("CloseDeployment: %v", err)
	}
	if deletes.Load() != 1 || gets.Load() != 2 {
		t.Fatalf("requests DELETE=%d GET=%d, want one submission and two observations", deletes.Load(), gets.Load())
	}
}

func TestLeaseReconciliationWaitsBeyondLegacyAttemptLimit(t *testing.T) {
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		leases := `[]`
		if gets.Add(1) > maxRetries+2 {
			leases = `[{"id":{"dseq":"42","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active"}]`
		}
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"},"state":"active"},"leases":` + leases + `}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, reconciled, err := New(srv.URL, "key").reconcileCreatedLeasesUntil(
		ctx,
		[]LeaseRequest{{DSeq: "42", GSeq: 1, OSeq: 1, Provider: "akash1provider"}},
		time.Millisecond,
	)
	if err != nil || !reconciled || len(result.Leases) != 1 {
		t.Fatalf("reconcile result = %t, %+v, %v; want eventual active lease", reconciled, result, err)
	}
	if gets.Load() <= maxRetries+2 {
		t.Fatalf("reconciliation GETs = %d, want more than legacy bound", gets.Load())
	}
}

func TestCloseReconciliationWaitsBeyondLegacyAttemptLimit(t *testing.T) {
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		state := "active"
		if gets.Add(1) > maxRetries+2 {
			state = "closed"
		}
		_, _ = w.Write([]byte(`{"data":{"deployment":{"id":{"dseq":"42"},"state":"` + state + `"}}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	reconciled, err := New(srv.URL, "key").reconcileClosedDeploymentUntil(ctx, "42", time.Millisecond)
	if err != nil || !reconciled {
		t.Fatalf("reconcile result = %t, %v; want eventual closed deployment", reconciled, err)
	}
	if gets.Load() <= maxRetries+2 {
		t.Fatalf("reconciliation GETs = %d, want more than legacy bound", gets.Load())
	}
}

func TestCloseReconciliationAcceptsAbsentPostState(t *testing.T) {
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		gets.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	reconciled, err := New(srv.URL, "key").reconcileClosedDeploymentUntil(
		context.Background(),
		"42",
		time.Millisecond,
	)
	if err != nil || !reconciled {
		t.Fatalf("reconcile result = %t, %v; want absent deployment accepted", reconciled, err)
	}
	if gets.Load() != 1 {
		t.Fatalf("reconciliation GETs = %d, want 1", gets.Load())
	}
}

func TestReconciliationStopsBeforeReadingWhenCallerIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var requests atomic.Int32
	client := New("https://console.example.test", "key")
	client.httpClient.Transport = timeoutTestRoundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("unexpected transport call")
	})

	_, leaseReconciled, leaseErr := client.reconcileCreatedLeasesUntil(
		ctx,
		[]LeaseRequest{{DSeq: "42", GSeq: 1, OSeq: 1, Provider: "akash1provider"}},
		time.Millisecond,
	)
	if leaseReconciled || !errors.Is(leaseErr, context.Canceled) {
		t.Fatalf("lease reconciliation = %t, %v; want cancellation", leaseReconciled, leaseErr)
	}
	closeReconciled, closeErr := client.reconcileClosedDeploymentUntil(ctx, "42", time.Millisecond)
	if closeReconciled || !errors.Is(closeErr, context.Canceled) {
		t.Fatalf("close reconciliation = %t, %v; want cancellation", closeReconciled, closeErr)
	}
	if requests.Load() != 0 {
		t.Fatalf("canceled reconciliation made %d transport requests", requests.Load())
	}
}
