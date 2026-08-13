package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"

	manifest "pkg.akt.dev/go/manifest/v2beta3"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	mtypes "pkg.akt.dev/go/node/market/v1"
	resources "pkg.akt.dev/go/node/types/resources/v1beta4"
	rest "pkg.akt.dev/go/provider/client"
	ajwt "pkg.akt.dev/go/util/jwt"
)

type cancellableGatewayStreamClient struct {
	rest.Client
	contexts chan context.Context
}

type gatewayStreamResultClient struct {
	rest.Client
	err error
}

type gatewayStreamCloseClient struct {
	rest.Client
	reason string
}

func (client gatewayStreamCloseClient) LeaseEvents(
	context.Context,
	mtypes.LeaseID,
	string,
	bool,
) (*rest.LeaseKubeEvents, error) {
	closed := make(chan string, 1)
	closed <- client.reason
	close(closed)
	return &rest.LeaseKubeEvents{OnClose: closed}, nil
}

func (client gatewayStreamCloseClient) LeaseLogs(
	context.Context,
	mtypes.LeaseID,
	string,
	bool,
	int64,
) (*rest.ServiceLogs, error) {
	closed := make(chan string, 1)
	closed <- client.reason
	close(closed)
	return &rest.ServiceLogs{OnClose: closed}, nil
}

type gatewayRequestClientFunc func(*http.Request) (*http.Response, error)

func (do gatewayRequestClientFunc) Do(req *http.Request) (*http.Response, error) {
	return do(req)
}

func (gatewayRequestClientFunc) DialContext(
	context.Context,
	string,
	http.Header,
) (*websocket.Conn, *http.Response, error) {
	return nil, nil, errors.New("websocket dial is not available in one-shot tests")
}

type gatewayRequestSource struct {
	rest.Client
	requestClient rest.ReqClient
}

func (source gatewayRequestSource) NewReqClient(context.Context) rest.ReqClient {
	return source.requestClient
}

func (source gatewayRequestSource) gatewayStreamRequestClient(context.Context) rest.ReqClient {
	return source.requestClient
}

type websocketGatewayRequestClient struct {
	serverURL *url.URL
}

func (client websocketGatewayRequestClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("HTTP requests are not available in websocket stream tests")
}

func (client websocketGatewayRequestClient) DialContext(
	ctx context.Context,
	endpoint string,
	header http.Header,
) (*websocket.Conn, *http.Response, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, nil, err
	}
	parsed.Scheme = "ws"
	parsed.Host = client.serverURL.Host
	return websocket.DefaultDialer.DialContext(ctx, parsed.String(), header)
}

type staticGatewayRequestClient struct {
	conn     *websocket.Conn
	response *http.Response
	err      error
}

func (client staticGatewayRequestClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("HTTP requests are not available in static websocket tests")
}

func (client staticGatewayRequestClient) DialContext(
	context.Context,
	string,
	http.Header,
) (*websocket.Conn, *http.Response, error) {
	return client.conn, client.response, client.err
}

type chunkedHandshakeBody struct {
	payload []byte
	offset  int
	closed  bool
}

func (body *chunkedHandshakeBody) Read(data []byte) (int, error) {
	if body.offset == len(body.payload) {
		return 0, io.EOF
	}

	const chunkSize = 257
	remaining := len(body.payload) - body.offset
	count := min(len(data), min(chunkSize, remaining))
	copy(data, body.payload[body.offset:body.offset+count])
	body.offset += count
	return count, nil
}

func (body *chunkedHandshakeBody) Close() error {
	body.closed = true
	return nil
}

type chunkedHandshakeRequestClient struct {
	body        *chunkedHandshakeBody
	header      http.Header
	hadDeadline bool
}

func (client *chunkedHandshakeRequestClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("HTTP requests are not available in websocket setup tests")
}

func (client *chunkedHandshakeRequestClient) DialContext(
	ctx context.Context,
	_ string,
	header http.Header,
) (*websocket.Conn, *http.Response, error) {
	client.header = header.Clone()
	_, client.hadDeadline = ctx.Deadline()
	return nil, &http.Response{
		StatusCode:       http.StatusForbidden,
		Body:             client.body,
		TransferEncoding: []string{"chunked"},
	}, websocket.ErrBadHandshake
}

type unboundedHandshakeGatewayClient struct {
	rest.Client
	requestClient rest.ReqClient
	token         string
}

func (client unboundedHandshakeGatewayClient) gatewayStreamRequestClient(context.Context) rest.ReqClient {
	return client.requestClient
}

func (client unboundedHandshakeGatewayClient) LeaseEvents(
	ctx context.Context,
	_ mtypes.LeaseID,
	_ string,
	_ bool,
) (*rest.LeaseKubeEvents, error) {
	err := client.copyHandshakeResponse(ctx)
	return nil, err
}

func (client unboundedHandshakeGatewayClient) LeaseLogs(
	ctx context.Context,
	_ mtypes.LeaseID,
	_ string,
	_ bool,
	_ int64,
) (*rest.ServiceLogs, error) {
	err := client.copyHandshakeResponse(ctx)
	return nil, err
}

func (client unboundedHandshakeGatewayClient) copyHandshakeResponse(ctx context.Context) error {
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+client.token)
	_, response, err := client.requestClient.DialContext(ctx, "wss://provider.example.com/stream", header)
	if !errors.Is(err, websocket.ErrBadHandshake) {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	var detail bytes.Buffer
	_, _ = io.Copy(&detail, response.Body)
	return rest.ClientResponseError{Status: response.StatusCode, Message: detail.String()}
}

type failingGatewayBody struct {
	err error
}

func (body failingGatewayBody) Read([]byte) (int, error) {
	return 0, body.err
}

func (failingGatewayBody) Close() error {
	return nil
}

func (client gatewayStreamResultClient) LeaseEvents(
	context.Context,
	mtypes.LeaseID,
	string,
	bool,
) (*rest.LeaseKubeEvents, error) {
	if client.err != nil {
		return nil, client.err
	}
	return &rest.LeaseKubeEvents{}, nil
}

func (client gatewayStreamResultClient) LeaseLogs(
	context.Context,
	mtypes.LeaseID,
	string,
	bool,
	int64,
) (*rest.ServiceLogs, error) {
	if client.err != nil {
		return nil, client.err
	}
	return &rest.ServiceLogs{}, nil
}

func (client gatewayStreamResultClient) LeaseShell(
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
	return client.err
}

func (client *cancellableGatewayStreamClient) LeaseLogs(
	ctx context.Context,
	_ mtypes.LeaseID,
	_ string,
	_ bool,
	_ int64,
) (*rest.ServiceLogs, error) {
	client.contexts <- ctx
	stream := make(chan rest.ServiceLogMessage)
	closed := make(chan string)
	go func() {
		<-ctx.Done()
		close(stream)
		close(closed)
	}()
	return &rest.ServiceLogs{Stream: stream, OnClose: closed}, nil
}

func TestGatewayOneShotRejectsOversizedResponse(t *testing.T) {
	const responseLimit = 16 << 20

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"padding":"`))
		_, _ = w.Write([]byte(strings.Repeat("x", responseLimit)))
		_, _ = w.Write([]byte(`"}`))
	}))
	defer srv.Close()

	client, err := NewPublicGatewayClient(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatalf("create gateway client: %v", err)
	}

	_, err = client.Status(context.Background())
	if err == nil || !strings.Contains(err.Error(), "response exceeds 16777216-byte limit") {
		t.Fatalf("oversized response error = %v, want the fixed body limit", err)
	}
}

func TestGatewayErrorSanitizesProviderControlledDetail(t *testing.T) {
	const (
		bearerSecret = "provider-bearer-secret"
		apiSecret    = "provider-api-secret"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("\x1b]0;forged-title\x07\x1b[31mdenied\x1b[0m\r\n" +
			"Authorization: Bearer " + bearerSecret + "\n" +
			`{"x-api-key":"` + apiSecret + `"}` +
			strings.Repeat("z", 32<<10)))
	}))
	defer srv.Close()

	client, err := NewPublicGatewayClient(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatalf("create gateway client: %v", err)
	}

	_, err = client.Status(context.Background())
	err = GatewayError("query provider status", err)
	if err == nil {
		t.Fatal("malicious provider response unexpectedly succeeded")
	}
	detail := err.Error()
	for _, forbidden := range []string{"\x1b", "\x07", bearerSecret, apiSecret} {
		if strings.Contains(detail, forbidden) {
			t.Fatalf("gateway error contains unsafe data %q: %q", forbidden, detail)
		}
	}
	if !strings.Contains(detail, "[REDACTED]") {
		t.Fatalf("gateway error = %q, want an explicit redaction marker", detail)
	}
	if len(detail) > 5<<10 {
		t.Fatalf("gateway error length = %d, want bounded provider detail", len(detail))
	}
}

func TestGatewayOneShotAppliesOverallDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	client, err := NewPublicGatewayClient(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatalf("create gateway client: %v", err)
	}
	bounded, ok := client.(*gatewayClient)
	if !ok {
		t.Fatalf("gateway client type = %T, want the bounded adapter", client)
	}
	bounded.oneShotTimeout = 40 * time.Millisecond

	started := time.Now()
	_, err = client.Status(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("hanging request error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("hanging request took %s, want the overall deadline", elapsed)
	}
}

func TestGatewayTokenErrorRedactsExactCredential(t *testing.T) {
	const token = "console-token-returned-without-a-label"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization = %q, want the supplied bearer token", got)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("provider reflected " + token))
	}))
	defer srv.Close()

	client, err := NewTokenGatewayClient(context.Background(), nil, srv.URL, token)
	if err != nil {
		t.Fatalf("create token gateway client: %v", err)
	}
	_, err = client.Status(context.Background())
	err = GatewayError("query provider status", err)
	if err == nil || strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("token response error = %v, want exact credential redaction", err)
	}
}

func TestGatewayStreamRetainsCallerCancellation(t *testing.T) {
	delegate := &cancellableGatewayStreamClient{contexts: make(chan context.Context, 1)}
	wrapped, err := wrapGatewayClient(delegate, "https://provider.example.com", nil)
	if err != nil {
		t.Fatalf("wrap gateway client: %v", err)
	}
	bounded := wrapped.(*gatewayClient)
	bounded.oneShotTimeout = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	logs, err := wrapped.LeaseLogs(ctx, mtypes.LeaseID{}, "", true, -1)
	if err != nil {
		t.Fatalf("open caller-cancellable stream: %v", err)
	}
	streamCtx := <-delegate.contexts
	if _, hasDeadline := streamCtx.Deadline(); hasDeadline {
		t.Fatal("stream inherited the one-shot deadline")
	}

	select {
	case <-logs.Stream:
		t.Fatal("stream closed at the one-shot deadline")
	case <-time.After(3 * bounded.oneShotTimeout):
	}

	cancel()
	select {
	case _, open := <-logs.Stream:
		if open {
			t.Fatal("stream emitted a record after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not close after caller cancellation")
	}
}

func TestGatewayStreamHandshakeErrorsHaveAnOwnedReadBoundary(t *testing.T) {
	const token = "chunked-handshake-secret"
	payload := []byte(
		"\x1b[31mAuthorization: Bearer " + token + "\x1b[0m\n" +
			strings.Repeat("x", int(gatewayErrorBodyLimit*4)),
	)

	checks := map[string]func(rest.Client) error{
		"events": func(client rest.Client) error {
			_, err := client.LeaseEvents(context.Background(), mtypes.LeaseID{DSeq: 42}, "", true)
			return err
		},
		"logs": func(client rest.Client) error {
			_, err := client.LeaseLogs(context.Background(), mtypes.LeaseID{DSeq: 42}, "web", true, -1)
			return err
		},
	}

	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			body := &chunkedHandshakeBody{payload: payload}
			requestClient := &chunkedHandshakeRequestClient{body: body}
			delegate := unboundedHandshakeGatewayClient{
				requestClient: requestClient,
				token:         token,
			}
			client, err := wrapGatewayClient(
				delegate,
				"https://provider.example.com",
				func() (string, error) { return token, nil },
				token,
			)
			if err != nil {
				t.Fatalf("wrap gateway client: %v", err)
			}

			err = check(client)
			var responseErr rest.ClientResponseError
			if !errors.As(err, &responseErr) || responseErr.Status != http.StatusForbidden {
				t.Fatalf("handshake error = %v, want provider status %d", err, http.StatusForbidden)
			}
			if body.offset > int(gatewayErrorBodyLimit+1) {
				t.Fatalf("handshake body read = %d bytes, want at most %d", body.offset, gatewayErrorBodyLimit+1)
			}
			if !body.closed {
				t.Fatal("handshake response body was not closed")
			}
			if requestClient.hadDeadline {
				t.Fatal("stream setup inherited the one-shot deadline")
			}
			if got := requestClient.header.Get("Authorization"); got != "Bearer "+token {
				t.Fatalf("Authorization = %q, want the supplied stream credential", got)
			}
			if strings.Contains(responseErr.Message, token) || strings.Contains(responseErr.Message, "\x1b") {
				t.Fatalf("handshake detail is unsafe: %q", responseErr.Message)
			}
			if len(responseErr.Message) > gatewayErrorDetailLimit {
				t.Fatalf("handshake detail length = %d, want at most %d", len(responseErr.Message), gatewayErrorDetailLimit)
			}
		})
	}
}

func TestGatewayBoundaryStreamsRecordsAndPreservesRequestSemantics(t *testing.T) {
	const token = "stream-token"
	requests := make(chan *url.URL, 2)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization = %q, want supplied stream token", got)
		}
		requests <- req.URL
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
			t.Errorf("write ping: %v", err)
			return
		}
		var record any
		if strings.HasSuffix(req.URL.Path, "/kubeevents") {
			record = rest.LeaseEvent{Type: "Normal", Reason: "Scheduled"}
		} else {
			record = rest.ServiceLogMessage{Name: "web", Message: "ready"}
		}
		if err := conn.WriteJSON(record); err != nil {
			t.Errorf("write stream record: %v", err)
			return
		}
		if err := conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "complete"),
			time.Now().Add(time.Second),
		); err != nil {
			t.Errorf("write close message: %v", err)
		}
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	client, err := wrapGatewayClient(
		gatewayRequestSource{requestClient: websocketGatewayRequestClient{serverURL: serverURL}},
		"https://provider.example.com",
		func() (string, error) { return token, nil },
		token,
	)
	if err != nil {
		t.Fatalf("wrap gateway client: %v", err)
	}
	id := mtypes.LeaseID{DSeq: 42, GSeq: 1, OSeq: 1}

	events, err := client.LeaseEvents(context.Background(), id, "ignored", true)
	if err != nil {
		t.Fatalf("open event stream: %v", err)
	}
	if event := receiveStreamValue(t, events.Stream); event.Type != "Normal" || event.Reason != "Scheduled" {
		t.Fatalf("event = %#v, want scheduled normal event", event)
	}
	if reason := receiveStreamValue(t, events.OnClose); reason != "" {
		t.Fatalf("event close reason = %q, want successful completion", reason)
	}
	eventRequest := receiveStreamValue(t, requests)
	if eventRequest.Path != "/lease/42/1/1/kubeevents" || eventRequest.Query().Get("follow") != "true" {
		t.Fatalf("event request = %q, want lease path with follow=true", eventRequest.RequestURI())
	}
	if eventRequest.Query().Has("services") {
		t.Fatalf("event request unexpectedly included services: %q", eventRequest.RequestURI())
	}

	logs, err := client.LeaseLogs(context.Background(), id, "web", false, 25)
	if err != nil {
		t.Fatalf("open log stream: %v", err)
	}
	if logLine := receiveStreamValue(t, logs.Stream); logLine.Name != "web" || logLine.Message != "ready" {
		t.Fatalf("log = %#v, want web ready message", logLine)
	}
	if reason := receiveStreamValue(t, logs.OnClose); reason != "" {
		t.Fatalf("log close reason = %q, want successful completion", reason)
	}
	logRequest := receiveStreamValue(t, requests)
	query := logRequest.Query()
	if logRequest.Path != "/lease/42/1/1/logs" || query.Get("follow") != "false" || query.Get("services") != "web" {
		t.Fatalf("log request = %q, want lease path with follow=false and services=web", logRequest.RequestURI())
	}
}

func TestGatewayBoundaryRejectsOversizedStreamMessages(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events bool
	}{
		{name: "events", events: true},
		{name: "logs", events: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upgrader := websocket.Upgrader{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := upgrader.Upgrade(w, req, nil)
				if err != nil {
					t.Errorf("upgrade websocket: %v", err)
					return
				}
				defer func() { _ = conn.Close() }()

				oversized := strings.Repeat("x", int(gatewayStreamMessageLimit)+1)
				if tc.events {
					_ = conn.WriteJSON(rest.LeaseEvent{Type: "Normal", Note: oversized})
					return
				}
				_ = conn.WriteJSON(rest.ServiceLogMessage{Name: "web", Message: oversized})
			}))
			defer server.Close()

			client := gatewayBoundaryClientForWebsocketServer(t, server.URL)
			var (
				records any
				onClose <-chan string
			)
			if tc.events {
				stream, err := client.LeaseEvents(context.Background(), mtypes.LeaseID{}, "", false)
				if err != nil {
					t.Fatalf("open event stream: %v", err)
				}
				records = stream.Stream
				onClose = stream.OnClose
			} else {
				stream, err := client.LeaseLogs(context.Background(), mtypes.LeaseID{}, "", false, -1)
				if err != nil {
					t.Fatalf("open log stream: %v", err)
				}
				records = stream.Stream
				onClose = stream.OnClose
			}

			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			switch stream := records.(type) {
			case <-chan rest.LeaseEvent:
				select {
				case record, open := <-stream:
					if open {
						t.Fatalf("oversized event reached the consumer: note length %d", len(record.Note))
					}
				case <-timer.C:
					t.Fatal("timed out waiting for oversized event rejection")
				}
			case <-chan rest.ServiceLogMessage:
				select {
				case record, open := <-stream:
					if open {
						t.Fatalf("oversized log reached the consumer: message length %d", len(record.Message))
					}
				case <-timer.C:
					t.Fatal("timed out waiting for oversized log rejection")
				}
			default:
				t.Fatalf("unexpected stream type %T", records)
			}

			reason := receiveStreamValue(t, onClose)
			if !strings.Contains(reason, "read limit") &&
				!strings.Contains(reason, "larger than the configured limit") {
				t.Fatalf("oversized %s close reason = %q, want size-limit detail", tc.name, reason)
			}
		})
	}
}

func TestGatewayBoundaryStreamSetupFailures(t *testing.T) {
	base, err := url.Parse("https://provider.example.com")
	if err != nil {
		t.Fatalf("parse provider URL: %v", err)
	}

	t.Run("request source", func(t *testing.T) {
		requestClient := staticGatewayRequestClient{}
		source := gatewayStreamBoundaryClient{
			Client: gatewayRequestSource{requestClient: requestClient},
		}
		if got := source.gatewayStreamRequestClient(context.Background()); got != requestClient {
			t.Fatalf("request client = %#v, want embedded client's request transport", got)
		}
	})

	t.Run("invalid scheme", func(t *testing.T) {
		client, wrapErr := wrapGatewayClient(
			gatewayRequestSource{requestClient: staticGatewayRequestClient{}},
			"http://provider.example.com",
			nil,
		)
		if wrapErr != nil {
			t.Fatalf("wrap gateway client: %v", wrapErr)
		}
		_, streamErr := client.LeaseEvents(context.Background(), mtypes.LeaseID{}, "", false)
		if streamErr == nil || !strings.Contains(streamErr.Error(), "invalid uri scheme http") {
			t.Fatalf("stream error = %v, want invalid scheme rejection", streamErr)
		}
	})

	t.Run("authorization", func(t *testing.T) {
		authErr := errors.New("stream signer unavailable")
		client, wrapErr := wrapGatewayClient(
			gatewayRequestSource{requestClient: staticGatewayRequestClient{}},
			base.String(),
			func() (string, error) { return "", authErr },
		)
		if wrapErr != nil {
			t.Fatalf("wrap gateway client: %v", wrapErr)
		}
		_, streamErr := client.LeaseEvents(context.Background(), mtypes.LeaseID{}, "", false)
		if !errors.Is(streamErr, authErr) {
			t.Fatalf("stream error = %v, want authorization failure", streamErr)
		}
	})

	client := &gatewayClient{host: base, redactionSecrets: []string{"stored-secret"}}
	t.Run("transport", func(t *testing.T) {
		transportErr := errors.New("dial failed with Bearer stream-secret")
		streamErr := client.gatewayStreamSetupError(nil, transportErr, "stream-secret")
		if !errors.Is(streamErr, transportErr) || strings.Contains(streamErr.Error(), "stream-secret") {
			t.Fatalf("stream error = %v, want redacted transport cause", streamErr)
		}
	})

	t.Run("missing response body", func(t *testing.T) {
		streamErr := client.gatewayStreamSetupError(
			&http.Response{StatusCode: http.StatusUnauthorized},
			websocket.ErrBadHandshake,
			"",
		)
		var responseErr rest.ClientResponseError
		if !errors.As(streamErr, &responseErr) || responseErr.Status != http.StatusUnauthorized || responseErr.Message != "" {
			t.Fatalf("stream error = %#v, want empty provider 401 response", streamErr)
		}
	})

	t.Run("response read", func(t *testing.T) {
		readErr := errors.New("handshake body failed")
		streamErr := client.gatewayStreamSetupError(
			&http.Response{StatusCode: http.StatusBadGateway, Body: failingGatewayBody{err: readErr}},
			websocket.ErrBadHandshake,
			"",
		)
		if !errors.Is(streamErr, readErr) {
			t.Fatalf("stream error = %v, want response reader failure", streamErr)
		}
	})

	t.Run("closed connection", func(t *testing.T) {
		conn := newClosedWebsocketConn(t)
		requestClient := staticGatewayRequestClient{conn: conn}
		wrapped, wrapErr := wrapGatewayClient(
			gatewayRequestSource{requestClient: requestClient},
			base.String(),
			nil,
		)
		if wrapErr != nil {
			t.Fatalf("wrap gateway client: %v", wrapErr)
		}
		if _, streamErr := wrapped.LeaseEvents(context.Background(), mtypes.LeaseID{}, "", false); streamErr == nil {
			t.Fatal("stream setup unexpectedly accepted a closed websocket")
		}
		if refreshErr := refreshGatewayStreamDeadline(conn); refreshErr == nil {
			t.Fatal("ping refresh unexpectedly accepted a closed websocket")
		}
	})

	readErr := errors.New("stream read failed")
	if reason := gatewayStreamReadReason(readErr); reason != readErr.Error() {
		t.Fatalf("stream read reason = %q, want underlying error", reason)
	}
}

func TestGatewayBoundaryStreamCancellationAndDecodeFailures(t *testing.T) {
	t.Run("caller cancellation while socket read is blocked", func(t *testing.T) {
		connected := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		upgrader := websocket.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			conn, err := upgrader.Upgrade(w, req, nil)
			if err != nil {
				t.Errorf("upgrade websocket: %v", err)
				return
			}
			defer func() { _ = conn.Close() }()
			close(connected)
			<-release
		}))
		defer server.Close()

		client := gatewayBoundaryClientForWebsocketServer(t, server.URL)
		ctx, cancel := context.WithCancel(context.Background())
		logs, err := client.LeaseLogs(ctx, mtypes.LeaseID{}, "", true, -1)
		if err != nil {
			t.Fatalf("open log stream: %v", err)
		}
		receiveSignal(t, connected)
		cancel()

		select {
		case reason, open := <-logs.OnClose:
			if open {
				t.Fatalf("cancelled socket read reported close reason %q", reason)
			}
		case <-time.After(time.Second):
			t.Fatal("socket read remained blocked after caller cancellation")
		}
	})

	t.Run("caller cancellation", func(t *testing.T) {
		connected := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		upgrader := websocket.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			conn, err := upgrader.Upgrade(w, req, nil)
			if err != nil {
				t.Errorf("upgrade websocket: %v", err)
				return
			}
			defer func() { _ = conn.Close() }()
			if err := conn.WriteJSON(rest.ServiceLogMessage{Name: "web", Message: "blocked"}); err != nil {
				t.Errorf("write blocked log record: %v", err)
				return
			}
			close(connected)
			<-release
		}))
		defer server.Close()

		wrapped := gatewayBoundaryClientForWebsocketServer(t, server.URL)
		client := wrapped.(*gatewayClient)
		source := client.Client.(gatewayStreamRequestSource)
		ctx, cancel := context.WithCancel(context.Background())
		stream, onClose, err := openGatewayStream[rest.ServiceLogMessage](
			ctx,
			client,
			source,
			rest.ServiceLogsPath(mtypes.LeaseID{}),
			url.Values{"follow": []string{"true"}},
		)
		if err != nil {
			t.Fatalf("open log stream: %v", err)
		}
		receiveSignal(t, connected)
		cancel()
		select {
		case reason, open := <-onClose:
			if open {
				t.Fatalf("cancelled log stream reported close reason %q", reason)
			}
		case <-time.After(time.Second):
			t.Fatal("backpressured log stream remained open after caller cancellation")
		}
		if _, open := <-stream; open {
			t.Fatal("log record escaped after cancellation won the blocked delivery")
		}
	})

	for _, tc := range []struct {
		name        string
		events      bool
		wantReason  bool
		wantSnippet string
	}{
		{name: "event", events: true, wantReason: true, wantSnippet: "invalid character"},
		{name: "log", events: false, wantReason: true, wantSnippet: "invalid character"},
	} {
		t.Run(tc.name+" decode", func(t *testing.T) {
			upgrader := websocket.Upgrader{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := upgrader.Upgrade(w, req, nil)
				if err != nil {
					t.Errorf("upgrade websocket: %v", err)
					return
				}
				defer func() { _ = conn.Close() }()
				if err := conn.WriteMessage(websocket.TextMessage, []byte("not-json")); err != nil {
					t.Errorf("write malformed record: %v", err)
				}
			}))
			defer server.Close()

			client := gatewayBoundaryClientForWebsocketServer(t, server.URL)
			var onClose <-chan string
			if tc.events {
				stream, streamErr := client.LeaseEvents(context.Background(), mtypes.LeaseID{}, "", false)
				if streamErr != nil {
					t.Fatalf("open event stream: %v", streamErr)
				}
				onClose = stream.OnClose
			} else {
				stream, streamErr := client.LeaseLogs(context.Background(), mtypes.LeaseID{}, "", false, -1)
				if streamErr != nil {
					t.Fatalf("open log stream: %v", streamErr)
				}
				onClose = stream.OnClose
			}

			select {
			case reason, ok := <-onClose:
				if ok != tc.wantReason {
					t.Fatalf("close reason presence = %t (%q), want %t", ok, reason, tc.wantReason)
				}
				if tc.wantSnippet != "" && !strings.Contains(reason, tc.wantSnippet) {
					t.Fatalf("close reason = %q, want %q", reason, tc.wantSnippet)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for malformed stream to close")
			}
		})
	}
}

func TestGatewayMalformedLogAfterValidRecordFailsConsumption(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		if err := conn.WriteJSON(rest.ServiceLogMessage{Name: "web-a", Message: "ready"}); err != nil {
			t.Errorf("write valid log record: %v", err)
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte("not-json")); err != nil {
			t.Errorf("write malformed log record: %v", err)
		}
	}))
	defer server.Close()

	client := gatewayBoundaryClientForWebsocketServer(t, server.URL)
	logs, err := client.LeaseLogs(context.Background(), mtypes.LeaseID{}, "web", false, -1)
	if err != nil {
		t.Fatalf("open log stream: %v", err)
	}

	records := 0
	err = ConsumeStream(context.Background(), "log", logs.Stream, logs.OnClose, false,
		func(record rest.ServiceLogMessage) error {
			records++
			if record.Name != "web-a" || record.Message != "ready" {
				t.Fatalf("valid record = %#v, want web-a ready", record)
			}
			return nil
		})
	if records != 1 {
		t.Fatalf("consumed records = %d, want 1 before malformed frame", records)
	}
	if err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("consume malformed log stream error = %v, want decode failure", err)
	}
}

func TestGatewayBoundaryClassifiesWebsocketClosure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		code        int
		reason      string
		wantErr     bool
		wantSnippet string
	}{
		{
			name:   "normal closure with reason",
			code:   websocket.CloseNormalClosure,
			reason: "complete",
		},
		{
			name:        "internal error without reason",
			code:        websocket.CloseInternalServerErr,
			wantErr:     true,
			wantSnippet: "1011",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upgrader := websocket.Upgrader{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := upgrader.Upgrade(w, req, nil)
				if err != nil {
					t.Errorf("upgrade websocket: %v", err)
					return
				}
				defer func() { _ = conn.Close() }()

				if err := conn.WriteJSON(rest.ServiceLogMessage{Name: "web-a", Message: "ready"}); err != nil {
					t.Errorf("write log record: %v", err)
					return
				}
				if err := conn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(tc.code, tc.reason),
					time.Now().Add(time.Second),
				); err != nil {
					t.Errorf("write close message: %v", err)
				}
			}))
			defer server.Close()

			client := gatewayBoundaryClientForWebsocketServer(t, server.URL)
			logs, err := client.LeaseLogs(context.Background(), mtypes.LeaseID{}, "web", false, -1)
			if err != nil {
				t.Fatalf("open log stream: %v", err)
			}

			records := 0
			err = ConsumeStream(context.Background(), "log", logs.Stream, logs.OnClose, false,
				func(rest.ServiceLogMessage) error {
					records++
					return nil
				})
			if records != 1 {
				t.Fatalf("consumed records = %d, want 1", records)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("ConsumeStream error = %v, wantErr %t", err, tc.wantErr)
			}
			if tc.wantSnippet != "" && !strings.Contains(err.Error(), tc.wantSnippet) {
				t.Fatalf("ConsumeStream error = %v, want %q", err, tc.wantSnippet)
			}
		})
	}
}

func newClosedWebsocketConn(t *testing.T) *websocket.Conn {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		_ = conn.Close()
	}))

	endpoint := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, response, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	if err != nil {
		server.Close()
		t.Fatalf("dial websocket test server: %v", err)
	}
	if err := conn.Close(); err != nil {
		server.Close()
		t.Fatalf("close websocket test client: %v", err)
	}
	server.Close()
	return conn
}

func gatewayBoundaryClientForWebsocketServer(t *testing.T, rawURL string) rest.Client {
	t.Helper()
	serverURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse websocket test server URL: %v", err)
	}
	client, err := wrapGatewayClient(
		gatewayRequestSource{requestClient: websocketGatewayRequestClient{serverURL: serverURL}},
		"https://provider.example.com",
		nil,
	)
	if err != nil {
		t.Fatalf("wrap gateway client: %v", err)
	}
	return client
}

func receiveSignal(t *testing.T, input <-chan struct{}) {
	t.Helper()
	select {
	case <-input:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream setup")
	}
}

func receiveStreamValue[T any](t *testing.T, input <-chan T) T {
	t.Helper()
	select {
	case value, ok := <-input:
		if !ok {
			t.Fatal("stream closed before emitting the expected value")
		}
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream value")
		var zero T
		return zero
	}
}

func TestGatewayOneShotMethodsShareBoundedAuthenticatedTransport(t *testing.T) {
	const token = "one-shot-method-token"
	wants := map[string]string{
		"GET /status":                          `{}`,
		"GET /validate":                        `{}`,
		"PUT /deployment/42/manifest":          "",
		"GET /lease/42/1/1/manifest":           `[]`,
		"GET /lease/42/1/1/status":             `{}`,
		"GET /lease/42/1/1/service/web/status": `{}`,
		"POST /hostname/migrate":               "",
		"POST /endpoint/migrate":               "",
	}
	seen := make(map[string]int, len(wants))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		response, ok := wants[key]
		if !ok {
			t.Errorf("unexpected gateway request %s", key)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		seen[key]++
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("%s Authorization = %q, want supplied token", key, got)
		}
		if r.Header.Get("Content-Type") != gatewayContentTypeJSON {
			t.Errorf("%s Content-Type = %q", key, r.Header.Get("Content-Type"))
		}
		if response != "" {
			_, _ = w.Write([]byte(response))
		}
	}))
	defer srv.Close()

	client, err := NewTokenGatewayClient(context.Background(), nil, srv.URL, token)
	if err != nil {
		t.Fatalf("create token gateway client: %v", err)
	}
	id := mtypes.LeaseID{DSeq: 42, GSeq: 1, OSeq: 1}

	if _, err := client.Status(context.Background()); err != nil {
		t.Fatalf("provider status: %v", err)
	}
	group := validGatewayGroupSpec()
	if err := group.ValidateBasic(); err != nil {
		t.Fatalf("test group spec is invalid: %v", err)
	}
	if _, err := client.Validate(context.Background(), group); err != nil {
		t.Fatalf("validate group: %v", err)
	}
	if err := client.SubmitManifest(context.Background(), 42, manifest.Manifest{}); err != nil {
		t.Fatalf("submit manifest: %v", err)
	}
	if _, err := client.GetManifest(context.Background(), id); err != nil {
		t.Fatalf("get manifest: %v", err)
	}
	if _, err := client.LeaseStatus(context.Background(), id); err != nil {
		t.Fatalf("lease status: %v", err)
	}
	if _, err := client.ServiceStatus(context.Background(), id, "web"); err != nil {
		t.Fatalf("service status: %v", err)
	}
	if err := client.MigrateHostnames(context.Background(), []string{"web.example"}, 42, 1); err != nil {
		t.Fatalf("migrate hostnames: %v", err)
	}
	if err := client.MigrateEndpoints(context.Background(), []string{"203.0.113.10"}, 42, 1); err != nil {
		t.Fatalf("migrate endpoints: %v", err)
	}

	for request := range wants {
		if seen[request] != 1 {
			t.Errorf("gateway request %s count = %d, want 1", request, seen[request])
		}
	}
}

func TestGatewayStreamErrorsAreSanitizedWithoutAddingDeadline(t *testing.T) {
	providerErr := rest.ClientResponseError{
		Status:  http.StatusForbidden,
		Message: "\x1b[31mAuthorization: Bearer stream-secret\x1b[0m",
	}
	wrapped, err := wrapGatewayClient(gatewayStreamResultClient{err: providerErr}, "https://provider.example.com", nil)
	if err != nil {
		t.Fatalf("wrap gateway client: %v", err)
	}

	_, eventErr := wrapped.LeaseEvents(context.Background(), mtypes.LeaseID{}, "", true)
	_, logErr := wrapped.LeaseLogs(context.Background(), mtypes.LeaseID{}, "", true, -1)
	shellErr := wrapped.LeaseShell(
		context.Background(),
		mtypes.LeaseID{},
		"web",
		0,
		[]string{"true"},
		nil,
		io.Discard,
		io.Discard,
		false,
		nil,
	)
	for name, err := range map[string]error{"events": eventErr, "logs": logErr, "shell": shellErr} {
		if err == nil || strings.Contains(err.Error(), "stream-secret") || strings.Contains(err.Error(), "\x1b") {
			t.Errorf("%s stream error = %v, want sanitized provider detail", name, err)
		}
	}

	success, err := wrapGatewayClient(gatewayStreamResultClient{}, "https://provider.example.com", nil)
	if err != nil {
		t.Fatalf("wrap successful stream client: %v", err)
	}
	if _, err := success.LeaseEvents(context.Background(), mtypes.LeaseID{}, "", false); err != nil {
		t.Fatalf("successful event stream: %v", err)
	}
	if _, err := success.LeaseLogs(context.Background(), mtypes.LeaseID{}, "", false, -1); err != nil {
		t.Fatalf("successful log stream: %v", err)
	}
	if err := success.LeaseShell(context.Background(), mtypes.LeaseID{}, "web", 0, nil, nil, io.Discard, io.Discard, false, nil); err != nil {
		t.Fatalf("successful shell stream: %v", err)
	}
}

func TestGatewayShellRedactsTheExactGeneratedToken(t *testing.T) {
	kr, _, owner := newProviderTestIdentity(t)
	wrapped, err := NewGatewayClient(
		context.Background(),
		sdkClientContextForBoundary(kr, owner),
		owner,
		"https://provider.example.com",
		"jwt",
		kr,
	)
	if err != nil {
		t.Fatalf("create keyring gateway client: %v", err)
	}
	client := wrapped.(*gatewayClient)
	if len(client.redactionSecrets) != 1 || client.redactionSecrets[0] == "" {
		t.Fatalf("shell redaction secrets = %d, want the exact generated JWT", len(client.redactionSecrets))
	}
	token := client.redactionSecrets[0]
	client.Client = gatewayStreamResultClient{err: rest.ClientResponseError{
		Status:  http.StatusForbidden,
		Message: "provider echoed " + token,
	}}

	err = client.LeaseShell(
		context.Background(),
		mtypes.LeaseID{},
		"web",
		0,
		nil,
		nil,
		io.Discard,
		io.Discard,
		false,
		nil,
	)
	var responseErr rest.ClientResponseError
	if !errors.As(err, &responseErr) || strings.Contains(responseErr.Message, token) ||
		!strings.Contains(responseErr.Message, "[REDACTED]") {
		t.Fatalf("shell error = %#v, want the generated request token redacted exactly", err)
	}
}

func TestGatewayStreamCloseReasonsAreBoundedAndSanitized(t *testing.T) {
	const secret = "split-stream-secret"
	reason := "Be\x1b[31marer " + secret + "\x1b[0m " + strings.Repeat("x", 8<<10)
	wrapped, err := wrapGatewayClient(
		gatewayStreamCloseClient{reason: reason},
		"https://provider.example.com",
		nil,
		secret,
	)
	if err != nil {
		t.Fatalf("wrap stream close client: %v", err)
	}

	events, err := wrapped.LeaseEvents(context.Background(), mtypes.LeaseID{}, "", true)
	if err != nil {
		t.Fatalf("open event stream: %v", err)
	}
	logs, err := wrapped.LeaseLogs(context.Background(), mtypes.LeaseID{}, "", true, -1)
	if err != nil {
		t.Fatalf("open log stream: %v", err)
	}

	for name, closed := range map[string]<-chan string{"events": events.OnClose, "logs": logs.OnClose} {
		detail, ok := <-closed
		if !ok {
			t.Fatalf("%s close channel ended without its reason", name)
		}
		if strings.Contains(detail, "\x1b") || strings.Contains(detail, secret) || len(detail) > gatewayErrorDetailLimit {
			t.Errorf("%s close reason is unsafe: %q", name, detail)
		}
		if !strings.Contains(detail, "[REDACTED]") {
			t.Errorf("%s close reason = %q, want a redaction marker", name, detail)
		}
		if _, open := <-closed; open {
			t.Errorf("%s close channel remained open", name)
		}
	}
}

func TestGatewayTypedReadsRejectMalformedResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	client, err := NewPublicGatewayClient(context.Background(), nil, srv.URL)
	if err != nil {
		t.Fatalf("create gateway client: %v", err)
	}
	id := mtypes.LeaseID{DSeq: 42, GSeq: 1, OSeq: 1}
	checks := map[string]func() error{
		"validate": func() error {
			_, err := client.Validate(context.Background(), validGatewayGroupSpec())
			return err
		},
		"manifest": func() error {
			_, err := client.GetManifest(context.Background(), id)
			return err
		},
		"lease status": func() error {
			_, err := client.LeaseStatus(context.Background(), id)
			return err
		},
		"service status": func() error {
			_, err := client.ServiceStatus(context.Background(), id, "web")
			return err
		},
	}
	for name, check := range checks {
		if err := check(); err == nil || !strings.Contains(err.Error(), "invalid character") {
			t.Errorf("%s error = %v, want malformed JSON rejection", name, err)
		}
	}

	if _, err := client.Validate(context.Background(), dtypes.GroupSpec{}); err == nil || !strings.Contains(err.Error(), "empty group spec") {
		t.Fatalf("invalid group error = %v, want local validation before HTTP", err)
	}
}

func TestGatewayOneShotBoundaryPropagatesLocalAndTransportFailures(t *testing.T) {
	base, err := url.Parse("https://provider.example.com")
	if err != nil {
		t.Fatalf("parse test provider URL: %v", err)
	}

	t.Run("marshal payload", func(t *testing.T) {
		client := &gatewayClient{host: base, oneShotTimeout: time.Second}
		err := client.doOneShot(context.Background(), http.MethodPost, "status", make(chan int), nil)
		if err == nil || !strings.Contains(err.Error(), "unsupported type") {
			t.Fatalf("marshal error = %v", err)
		}
	})

	t.Run("invalid URI", func(t *testing.T) {
		client := &gatewayClient{host: &url.URL{Scheme: "https", Host: "["}, oneShotTimeout: time.Second}
		err := client.doOneShot(context.Background(), http.MethodGet, "status", nil, nil)
		if err == nil {
			t.Fatal("invalid URI unexpectedly succeeded")
		}
	})

	t.Run("invalid method", func(t *testing.T) {
		client := &gatewayClient{host: base, oneShotTimeout: time.Second}
		err := client.doOneShot(context.Background(), "bad method", "status", nil, nil)
		if err == nil || !strings.Contains(err.Error(), "invalid method") {
			t.Fatalf("invalid method error = %v", err)
		}
	})

	t.Run("authorization", func(t *testing.T) {
		authErr := errors.New("signing device unavailable")
		client := &gatewayClient{
			host:           base,
			oneShotTimeout: time.Second,
			authorization: func() (string, error) {
				return "", authErr
			},
		}
		err := client.doOneShot(context.Background(), http.MethodGet, "status", nil, nil)
		if !errors.Is(err, authErr) || err.Error() != authErr.Error() {
			t.Fatalf("authorization error = %v, want wrapped signing failure", err)
		}
	})

	t.Run("transport", func(t *testing.T) {
		transportErr := errors.New("dial failed with Bearer transport-secret")
		client := gatewayClientForRequest(t, func(*http.Request) (*http.Response, error) {
			return nil, transportErr
		})
		err := client.doOneShot(context.Background(), http.MethodGet, "status", nil, nil)
		if !errors.Is(err, transportErr) || strings.Contains(err.Error(), "transport-secret") {
			t.Fatalf("transport error = %v, want redacted wrapped cause", err)
		}
	})

	for _, status := range []int{http.StatusOK, http.StatusBadGateway} {
		t.Run(fmt.Sprintf("response body %d", status), func(t *testing.T) {
			readErr := errors.New("response body read failed")
			client := gatewayClientForRequest(t, func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: status,
					Body:       failingGatewayBody{err: readErr},
				}, nil
			})
			err := client.doOneShot(context.Background(), http.MethodGet, "status", nil, nil)
			if !errors.Is(err, readErr) {
				t.Fatalf("response read error = %v, want wrapped reader failure", err)
			}
		})
	}
}

func TestGatewayConstructorsRejectMalformedProviderURL(t *testing.T) {
	const malformedURL = "://missing-scheme"
	if _, err := NewPublicGatewayClient(context.Background(), nil, malformedURL); err == nil {
		t.Fatal("public gateway accepted malformed URL")
	}
	if _, err := NewTokenGatewayClient(context.Background(), nil, malformedURL, "token"); err == nil {
		t.Fatal("token gateway accepted malformed URL")
	}

	kr, _, owner := newProviderTestIdentity(t)
	if _, err := NewGatewayClient(
		context.Background(),
		sdkClientContextForBoundary(kr, owner),
		owner,
		malformedURL,
		"jwt",
		kr,
	); err == nil {
		t.Fatal("authenticated gateway accepted malformed URL")
	}
	if _, err := NewScopedGatewayClient(
		context.Background(),
		sdkClientContextForBoundary(kr, owner),
		owner,
		malformedURL,
		sdk.AccAddress(strings.Repeat("p", 20)),
		gatewayStatusPermission(),
		"jwt",
		kr,
	); err == nil {
		t.Fatal("scoped gateway accepted malformed URL")
	}
	if _, err := wrapGatewayClient(gatewayStreamResultClient{}, malformedURL, nil); err == nil {
		t.Fatal("gateway boundary accepted malformed URL")
	}
}

func TestGatewayDetailTruncationIsUTF8SafeAtEveryLimit(t *testing.T) {
	if got := boundGatewayDetail("provider detail", 1, true); got != "." {
		t.Fatalf("one-byte limit detail = %q", got)
	}
	if got := boundGatewayDetail("provider detail", 0, true); got != "" {
		t.Fatalf("zero-byte limit detail = %q", got)
	}

	detail := strings.Repeat("x", gatewayErrorDetailLimit-16) + "éé"
	got := boundGatewayDetail(detail, gatewayErrorDetailLimit-12, true)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, gatewayTruncatedDetailLabel) {
		t.Fatalf("bounded UTF-8 detail = %q", got)
	}
	got = boundGatewayDetail("é", len(gatewayTruncatedDetailLabel)+1, true)
	if !utf8.ValidString(got) || got != gatewayTruncatedDetailLabel {
		t.Fatalf("split-rune detail = %q, want only the truncation marker", got)
	}
}

func TestFullAccessGatewayJWTReportsSigningFailure(t *testing.T) {
	kr, _, owner := newProviderTestIdentity(t)
	signErr := errors.New("signing device unavailable")
	_, err := newFullAccessJWT(failingSignKeyring{Keyring: kr, err: signErr}, owner)
	if !errors.Is(err, signErr) || !strings.Contains(err.Error(), "sign provider JWT") {
		t.Fatalf("full-access signing error = %v, want wrapped device failure", err)
	}
}

func gatewayClientForRequest(t *testing.T, do gatewayRequestClientFunc) *gatewayClient {
	t.Helper()
	wrapped, err := wrapGatewayClient(
		gatewayRequestSource{requestClient: do},
		"https://provider.example.com",
		nil,
	)
	if err != nil {
		t.Fatalf("wrap request client: %v", err)
	}
	return wrapped.(*gatewayClient)
}

func sdkClientContextForBoundary(kr sdkkeyring.Keyring, owner sdk.AccAddress) sdkclient.Context {
	return sdkclient.Context{}.WithKeyring(kr).WithFromAddress(owner)
}

func gatewayStatusPermission() ajwt.PermissionDeployment {
	return ajwt.PermissionDeployment{
		Scope:    ajwt.PermissionScopes{ajwt.PermissionScopeStatus},
		DSeq:     42,
		Services: []string{"web"},
	}
}

func validGatewayGroupSpec() dtypes.GroupSpec {
	minimum := dtypes.GetValidationConfig().Unit.Min
	return dtypes.GroupSpec{
		Name: "group",
		Resources: dtypes.ResourceUnits{{
			Resources: resources.Resources{
				ID:     1,
				CPU:    &resources.CPU{Units: resources.NewResourceValue(uint64(minimum.CPU))},
				GPU:    &resources.GPU{Units: resources.NewResourceValue(uint64(minimum.GPU))},
				Memory: &resources.Memory{Quantity: resources.NewResourceValue(minimum.Memory)},
				Storage: resources.Volumes{{
					Name:     "default",
					Quantity: resources.NewResourceValue(minimum.Storage),
				}},
				Endpoints: resources.Endpoints{},
			},
			Count: 1,
			Price: sdk.NewInt64DecCoin("uakt", 1),
		}},
	}
}
