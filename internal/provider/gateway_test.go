package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/tools/remotecommand"

	mtypes "pkg.akt.dev/go/node/market/v1"
	rest "pkg.akt.dev/go/provider/client"
)

func TestStreamCloseError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reason  string
		follow  bool
		wantErr bool
	}{
		{"clean one-shot", "", false, false},
		{"clean follow", "", true, false},
		{"one-shot EOF", "unexpected EOF", false, false},
		{"follow EOF", "unexpected EOF", true, true},
		{"one-shot reset", "connection reset by peer", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := StreamCloseError("log", tc.reason, tc.follow)
			if (err != nil) != tc.wantErr {
				t.Fatalf("StreamCloseError(%q, %v) = %v, wantErr %v", tc.reason, tc.follow, err, tc.wantErr)
			}
		})
	}
}

func TestConsumeStreamDrainsRecordsAndCloseReason(t *testing.T) {
	for _, tc := range []struct {
		name    string
		follow  bool
		wantErr bool
	}{
		{"one-shot EOF", false, false},
		{"follow EOF", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := make(chan string, 2)
			stream <- "one"
			stream <- "two"
			close(stream)
			onClose := make(chan string, 1)
			onClose <- "unexpected EOF"
			close(onClose)

			var records []string
			err := ConsumeStream(context.Background(), "log", stream, onClose, tc.follow,
				func(record string) error {
					records = append(records, record)
					return nil
				})
			if strings.Join(records, ",") != "one,two" {
				t.Fatalf("records = %#v, want both records", records)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("ConsumeStream error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestConsumeStreamReturnsBoundaryErrors(t *testing.T) {
	t.Run("caller cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := ConsumeStream(ctx, "log", make(chan string), make(chan string), false,
			func(string) error { return nil })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ConsumeStream cancellation = %v, want context canceled", err)
		}
	})

	t.Run("record emitter", func(t *testing.T) {
		emitErr := errors.New("output failed")
		stream := make(chan string, 1)
		stream <- "record"

		err := ConsumeStream(context.Background(), "log", stream, nil, false,
			func(string) error { return emitErr })
		if !errors.Is(err, emitErr) {
			t.Fatalf("ConsumeStream emitter error = %v, want %v", err, emitErr)
		}
	})
}

func TestValidateLogTail(t *testing.T) {
	for _, tc := range []struct {
		name    string
		follow  bool
		tail    int64
		wantErr bool
	}{
		{"all lines", false, -1, false},
		{"bounded", false, 5, false},
		{"zero", false, 0, false},
		{"below sentinel", false, -2, true},
		{"tail while following", true, 5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateLogTail(tc.follow, tc.tail)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateLogTail(%v, %d) error = %v, wantErr %v", tc.follow, tc.tail, err, tc.wantErr)
			}
		})
	}
}

func TestRetainTail(t *testing.T) {
	var got []string
	for _, value := range []string{"one", "two", "three"} {
		got = RetainTail(got, value, 2)
	}
	if strings.Join(got, ",") != "two,three" {
		t.Fatalf("RetainTail = %#v, want [two three]", got)
	}
	if got := RetainTail([]string{"one"}, "two", 0); len(got) != 0 {
		t.Fatalf("RetainTail limit zero = %#v, want no records", got)
	}
}

func TestMatchesService(t *testing.T) {
	if !MatchesService("web-5cfc6c7b4b-4cl7z", "web") {
		t.Fatal("service must match its Kubernetes pod name")
	}
	if MatchesService("web-", "web") {
		t.Fatal("service must not match an empty runtime pod suffix")
	}
	if MatchesService("webhook-abc-123", "web") {
		t.Fatal("service prefix without a name boundary must not match")
	}
}

func TestHoldEOFUntilContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	raw := &signaledEOFReader{called: make(chan struct{})}
	reader := HoldEOF(ctx, raw)

	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := reader.Read(make([]byte, 1))
		done <- result{n: n, err: err}
	}()
	<-raw.called

	select {
	case got := <-done:
		t.Fatalf("EOF returned before the remote context completed: %#v", got)
	default:
	}

	cancel()
	select {
	case got := <-done:
		if got.n != 0 || got.err != nil {
			t.Fatalf("read after cancellation = (%d, %v), want (0, nil)", got.n, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("held EOF did not release after context cancellation")
	}
}

func TestHoldEOFReaderBoundaryResults(t *testing.T) {
	if HoldEOF(context.Background(), nil) != nil {
		t.Fatal("HoldEOF must preserve nil stdin")
	}

	t.Run("non EOF error", func(t *testing.T) {
		readErr := errors.New("read failed")
		reader := HoldEOF(context.Background(), staticReadResult{err: readErr})
		n, err := reader.Read(make([]byte, 1))
		if n != 0 || !errors.Is(err, readErr) {
			t.Fatalf("Read = (%d, %v), want (0, %v)", n, err, readErr)
		}
	})

	t.Run("bytes with EOF", func(t *testing.T) {
		reader := HoldEOF(context.Background(), staticReadResult{n: 1, err: io.EOF})
		n, err := reader.Read(make([]byte, 1))
		if n != 1 || err != nil {
			t.Fatalf("Read = (%d, %v), want (1, nil)", n, err)
		}
	})
}

func TestSelectShellStdin(t *testing.T) {
	if SelectShellStdin(context.Background(), nil, true, true, false, false) != nil {
		t.Fatal("SelectShellStdin must preserve nil stdin")
	}

	input := strings.NewReader("input")
	tests := []struct {
		name          string
		interactive   bool
		terminal      bool
		overrideSet   bool
		overrideValue bool
		wantAttached  bool
	}{
		{name: "interactive terminal", interactive: true, terminal: true, wantAttached: true},
		{name: "explicit terminal command", terminal: true, wantAttached: false},
		{name: "explicit piped command", terminal: false, wantAttached: true},
		{name: "forced terminal stdin", terminal: true, overrideSet: true, overrideValue: true, wantAttached: true},
		{name: "detached interactive stdin", interactive: true, terminal: true, overrideSet: true, overrideValue: false, wantAttached: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SelectShellStdin(
				context.Background(),
				input,
				tc.interactive,
				tc.terminal,
				tc.overrideSet,
				tc.overrideValue,
			)
			if attached := got != nil; attached != tc.wantAttached {
				t.Fatalf("stdin attached = %v, want %v", attached, tc.wantAttached)
			}
		})
	}
}

type signaledEOFReader struct {
	called chan struct{}
}

func (reader *signaledEOFReader) Read([]byte) (int, error) {
	close(reader.called)
	return 0, io.EOF
}

type staticReadResult struct {
	n   int
	err error
}

func (reader staticReadResult) Read(data []byte) (int, error) {
	for index := 0; index < reader.n && index < len(data); index++ {
		data[index] = 'x'
	}
	return reader.n, reader.err
}

func TestGatewayErrorIncludesProviderResponse(t *testing.T) {
	err := GatewayError("migrate hostnames", rest.ClientResponseError{
		Status:  http.StatusBadRequest,
		Message: `{"message":"hostname is already in use"}`,
	})
	if err == nil {
		t.Fatal("GatewayError unexpectedly returned nil")
	}
	for _, want := range []string{"migrate hostnames", "400", "hostname is already in use"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("GatewayError = %q, want %q", err, want)
		}
	}
}

type leaseStatusStub struct {
	err   error
	calls int
}

func (stub *leaseStatusStub) LeaseStatus(context.Context, mtypes.LeaseID) (rest.LeaseStatus, error) {
	stub.calls++
	return rest.LeaseStatus{}, stub.err
}

func TestCheckLeaseSurfacesMissingLease(t *testing.T) {
	stub := &leaseStatusStub{err: rest.ClientResponseError{
		Status:  404,
		Message: `{"message":"lease not found"}`,
	}}
	err := CheckLease(context.Background(), stub, mtypes.LeaseID{DSeq: 1})
	if stub.calls != 1 {
		t.Fatalf("LeaseStatus called %d times, want 1", stub.calls)
	}
	if err == nil || !strings.Contains(err.Error(), "lease not found") {
		t.Fatalf("CheckLease error = %v, want provider response detail", err)
	}
}

type leaseShellStub struct {
	leaseStatusStub
	shellCalls int
}

func (stub *leaseShellStub) LeaseShell(
	context.Context,
	mtypes.LeaseID,
	string,
	uint,
	[]string,
	io.Reader,
	io.Writer,
	io.Writer,
	bool,
	<-chan remotecommand.TerminalSize,
) error {
	stub.shellCalls++
	return nil
}

func TestRunLeaseShellRefusesMissingLeaseBeforeRemoteExecution(t *testing.T) {
	stub := &leaseShellStub{leaseStatusStub: leaseStatusStub{err: rest.ClientResponseError{
		Status:  404,
		Message: `{"message":"lease not found"}`,
	}}}

	err := RunLeaseShell(
		context.Background(),
		stub,
		mtypes.LeaseID{DSeq: 1},
		"web",
		0,
		[]string{"/bin/sh"},
		nil,
		io.Discard,
		io.Discard,
		false,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "lease not found") {
		t.Fatalf("RunLeaseShell error = %v, want missing lease detail", err)
	}
	if stub.shellCalls != 0 {
		t.Fatalf("LeaseShell called %d times after failed preflight", stub.shellCalls)
	}
}

func TestGatewayErrorPreservesOrdinaryErrors(t *testing.T) {
	sentinel := errors.New("dial failed")
	err := GatewayError("query status", sentinel)
	if !errors.Is(err, sentinel) {
		t.Fatalf("GatewayError lost wrapped error: %v", err)
	}
	if !strings.Contains(err.Error(), "query status") {
		t.Fatalf("GatewayError = %q, want action context", err)
	}
}

func TestGatewayErrorForProviderIncludesExactChainFallback(t *testing.T) {
	sentinel := errors.New("connection refused")
	const provider = "akash1providerfulladdress"
	err := GatewayErrorForProvider("query provider status", provider, sentinel)
	if !errors.Is(err, sentinel) {
		t.Fatalf("fallback error lost cause: %v", err)
	}
	want := "`akt query provider " + provider + "`"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("fallback error = %q, want %q", err, want)
	}
}

func TestGatewayErrorForProviderHandlesNilAndBlankProvider(t *testing.T) {
	if err := GatewayErrorForProvider("query status", "akash1provider", nil); err != nil {
		t.Fatalf("nil gateway error = %v", err)
	}
	sentinel := errors.New("offline")
	err := GatewayErrorForProvider("query status", "   ", sentinel)
	if !errors.Is(err, sentinel) || strings.Contains(err.Error(), "akt query provider") {
		t.Fatalf("blank-provider error = %v", err)
	}
}
