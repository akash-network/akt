package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"

	manifest "pkg.akt.dev/go/manifest/v2beta3"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	mtypes "pkg.akt.dev/go/node/market/v1"
	rest "pkg.akt.dev/go/provider/client"
	providerv1 "pkg.akt.dev/go/provider/v1"
)

const (
	gatewayOneShotTimeout       = 30 * time.Second
	gatewayResponseBodyLimit    = int64(16 << 20)
	gatewayStreamMessageLimit   = int64(16 << 20)
	gatewayErrorBodyLimit       = int64(64 << 10)
	gatewayErrorDetailLimit     = 4 << 10
	gatewayContentTypeJSON      = "application/json; charset=UTF-8"
	gatewayTruncatedDetailLabel = "... [truncated]"
)

type gatewayAuthorization func() (string, error)

type gatewayStreamRequestSource interface {
	gatewayStreamRequestClient(context.Context) rest.ReqClient
}

type gatewayStreamBoundaryClient struct {
	rest.Client
}

func (client gatewayStreamBoundaryClient) gatewayStreamRequestClient(ctx context.Context) rest.ReqClient {
	return client.Client.NewReqClient(ctx)
}

// gatewayClient keeps finite HTTP exchanges and websocket setup failures inside
// akt's network boundary. Established streams retain the caller's lifetime
// rather than inheriting the one-shot deadline.
type gatewayClient struct {
	rest.Client
	host             *url.URL
	authorization    gatewayAuthorization
	redactionSecrets []string
	oneShotTimeout   time.Duration
}

func wrapGatewayClient(
	client rest.Client,
	providerURL string,
	authorization gatewayAuthorization,
	redactionSecrets ...string,
) (rest.Client, error) {
	host, err := url.Parse(providerURL)
	if err != nil {
		return nil, err
	}

	return &gatewayClient{
		Client:           client,
		host:             host,
		authorization:    authorization,
		redactionSecrets: redactionSecrets,
		oneShotTimeout:   gatewayOneShotTimeout,
	}, nil
}

func (client *gatewayClient) Status(ctx context.Context) (*rest.ProviderStatus, error) {
	var result rest.ProviderStatus
	if err := client.doOneShot(ctx, http.MethodGet, rest.StatusPath(), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (client *gatewayClient) Validate(
	ctx context.Context,
	group dtypes.GroupSpec,
) (*providerv1.BidScreeningResponse, error) {
	if err := group.ValidateBasic(); err != nil {
		return nil, err
	}

	var result providerv1.BidScreeningResponse
	if err := client.doOneShot(ctx, http.MethodGet, rest.ValidatePath(), group, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (client *gatewayClient) SubmitManifest(
	ctx context.Context,
	dseq uint64,
	value manifest.Manifest,
) error {
	return client.doOneShot(ctx, http.MethodPut, rest.SubmitManifestPath(dseq), value, nil)
}

func (client *gatewayClient) GetManifest(
	ctx context.Context,
	id mtypes.LeaseID,
) (manifest.Manifest, error) {
	var result manifest.Manifest
	if err := client.doOneShot(ctx, http.MethodGet, rest.GetManifestPath(id), nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (client *gatewayClient) LeaseStatus(
	ctx context.Context,
	id mtypes.LeaseID,
) (rest.LeaseStatus, error) {
	var result rest.LeaseStatus
	if err := client.doOneShot(ctx, http.MethodGet, rest.LeaseStatusPath(id), nil, &result); err != nil {
		return rest.LeaseStatus{}, err
	}
	return result, nil
}

func (client *gatewayClient) ServiceStatus(
	ctx context.Context,
	id mtypes.LeaseID,
	service string,
) (*rest.ServiceStatus, error) {
	var result rest.ServiceStatus
	if err := client.doOneShot(ctx, http.MethodGet, rest.ServiceStatusPath(id, service), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (client *gatewayClient) MigrateHostnames(
	ctx context.Context,
	hostnames []string,
	dseq uint64,
	gseq uint32,
) error {
	body := rest.MigrateRequestBody{
		HostnamesToMigrate: hostnames,
		DestinationDSeq:    dseq,
		DestinationGSeq:    gseq,
	}
	return client.doOneShot(ctx, http.MethodPost, "hostname/migrate", body, nil)
}

func (client *gatewayClient) MigrateEndpoints(
	ctx context.Context,
	endpoints []string,
	dseq uint64,
	gseq uint32,
) error {
	body := rest.EndpointMigrateRequestBody{
		EndpointsToMigrate: endpoints,
		DestinationDSeq:    dseq,
		DestinationGSeq:    gseq,
	}
	return client.doOneShot(ctx, http.MethodPost, "endpoint/migrate", body, nil)
}

func (client *gatewayClient) LeaseEvents(
	ctx context.Context,
	id mtypes.LeaseID,
	services string,
	follow bool,
) (*rest.LeaseKubeEvents, error) {
	if source, ok := client.Client.(gatewayStreamRequestSource); ok {
		query := url.Values{}
		query.Set("follow", strconv.FormatBool(follow))
		stream, onClose, err := openGatewayStream[rest.LeaseEvent](
			ctx,
			client,
			source,
			rest.LeaseEventsPath(id),
			query,
		)
		if err != nil {
			return nil, err
		}
		return &rest.LeaseKubeEvents{
			Stream:  stream,
			OnClose: client.sanitizeStreamClose(ctx, onClose),
		}, nil
	}

	result, err := client.Client.LeaseEvents(ctx, id, services, follow)
	if err != nil {
		return nil, client.sanitizeError(err)
	}
	result.OnClose = client.sanitizeStreamClose(ctx, result.OnClose)
	return result, nil
}

func (client *gatewayClient) LeaseLogs(
	ctx context.Context,
	id mtypes.LeaseID,
	services string,
	follow bool,
	tailLines int64,
) (*rest.ServiceLogs, error) {
	if source, ok := client.Client.(gatewayStreamRequestSource); ok {
		query := url.Values{}
		query.Set("follow", strconv.FormatBool(follow))
		if services != "" {
			query.Set("services", services)
		}
		stream, onClose, err := openGatewayStream[rest.ServiceLogMessage](
			ctx,
			client,
			source,
			rest.ServiceLogsPath(id),
			query,
		)
		if err != nil {
			return nil, err
		}
		return &rest.ServiceLogs{
			Stream:  stream,
			OnClose: client.sanitizeStreamClose(ctx, onClose),
		}, nil
	}

	result, err := client.Client.LeaseLogs(ctx, id, services, follow, tailLines)
	if err != nil {
		return nil, client.sanitizeError(err)
	}
	result.OnClose = client.sanitizeStreamClose(ctx, result.OnClose)
	return result, nil
}

func (client *gatewayClient) LeaseShell(
	ctx context.Context,
	id mtypes.LeaseID,
	service string,
	podIndex uint,
	command []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
	terminalResize <-chan remotecommand.TerminalSize,
) error {
	err := client.Client.LeaseShell(
		ctx,
		id,
		service,
		podIndex,
		command,
		stdin,
		stdout,
		stderr,
		tty,
		terminalResize,
	)
	return client.sanitizeError(err)
}

func openGatewayStream[T any](
	ctx context.Context,
	client *gatewayClient,
	source gatewayStreamRequestSource,
	path string,
	query url.Values,
) (<-chan T, <-chan string, error) {
	endpoint := *client.host
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/" + path
	switch endpoint.Scheme {
	case "https", "wss":
		endpoint.Scheme = "wss"
	default:
		return nil, nil, fmt.Errorf(
			"invalid uri scheme %s. supported schemes http|https|ws|wss",
			endpoint.Scheme,
		)
	}
	endpoint.RawQuery = query.Encode()

	header := make(http.Header)
	token := ""
	var err error
	if client.authorization != nil {
		token, err = client.authorization()
		if err != nil {
			return nil, nil, client.sanitizeError(err)
		}
		if token != "" {
			header.Set("Authorization", "Bearer "+token)
		}
	}

	conn, response, err := source.gatewayStreamRequestClient(ctx).DialContext(
		ctx,
		endpoint.String(),
		header,
	)
	if err != nil {
		return nil, nil, client.gatewayStreamSetupError(response, err, token)
	}
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	conn.SetReadLimit(gatewayStreamMessageLimit)
	if err := conn.SetReadDeadline(time.Now().Add(rest.PingWait)); err != nil {
		_ = conn.Close()
		return nil, nil, client.sanitizeError(err, token)
	}
	conn.SetPingHandler(func(string) error {
		return refreshGatewayStreamDeadline(conn)
	})

	stream := make(chan T)
	onClose := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	go func() {
		defer close(done)
		defer close(stream)
		defer close(onClose)
		defer func() {
			_ = conn.Close()
		}()

		readGatewayStream(ctx, conn, stream, onClose)
	}()

	return stream, onClose, nil
}

func readGatewayStream[T any](
	ctx context.Context,
	conn *websocket.Conn,
	stream chan<- T,
	onClose chan<- string,
) {
	for {
		messageType, message, readErr := conn.ReadMessage()
		if readErr != nil {
			if ctx.Err() != nil {
				return
			}
			onClose <- gatewayStreamReadReason(readErr)
			return
		}

		switch messageType {
		case websocket.TextMessage:
			var record T
			if err := json.Unmarshal(message, &record); err != nil {
				onClose <- err.Error()
				return
			}
			select {
			case stream <- record:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (client *gatewayClient) gatewayStreamSetupError(
	response *http.Response,
	err error,
	token string,
) error {
	if response != nil && response.Body != nil {
		defer func() {
			_ = response.Body.Close()
		}()
	}
	if !errors.Is(err, websocket.ErrBadHandshake) || response == nil {
		return client.sanitizeError(err, token)
	}
	if response.Body == nil {
		return rest.ClientResponseError{Status: response.StatusCode}
	}

	detail, truncated, readErr := readGatewayBody(response.Body, gatewayErrorBodyLimit)
	if readErr != nil {
		return client.sanitizeError(
			fmt.Errorf("read provider websocket handshake response: %w", readErr),
			token,
		)
	}
	message := sanitizeGatewayDetail(
		string(detail),
		append(client.redactionSecrets, token)...,
	)
	message = boundGatewayDetail(message, gatewayErrorDetailLimit, truncated)
	return rest.ClientResponseError{Status: response.StatusCode, Message: message}
}

func refreshGatewayStreamDeadline(conn *websocket.Conn) error {
	if err := conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second)); err != nil {
		return err
	}
	return conn.SetReadDeadline(time.Now().Add(rest.PingWait))
}

func gatewayStreamReadReason(err error) string {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		if closeErr.Code == websocket.CloseNormalClosure {
			return ""
		}
		if closeErr.Text == "" {
			return fmt.Sprintf("websocket close code %d", closeErr.Code)
		}
		return closeErr.Text
	}
	return err.Error()
}

func (client *gatewayClient) doOneShot(
	ctx context.Context,
	method string,
	path string,
	payload any,
	result any,
) error {
	callCtx, cancel := context.WithTimeout(ctx, client.oneShotTimeout)
	defer cancel()

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	uri, err := rest.MakeURI(client.host, path)
	if err != nil {
		return client.sanitizeError(err)
	}
	req, err := http.NewRequestWithContext(callCtx, method, uri, body)
	if err != nil {
		return client.sanitizeError(err)
	}
	req.Header.Set("Content-Type", gatewayContentTypeJSON)

	token := ""
	if client.authorization != nil {
		token, err = client.authorization()
		if err != nil {
			return client.sanitizeError(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := client.Client.NewReqClient(callCtx).Do(req)
	if err != nil {
		return client.sanitizeError(err, token)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		detail, truncated, readErr := readGatewayBody(resp.Body, gatewayErrorBodyLimit)
		if readErr != nil {
			return client.sanitizeError(readErr, token)
		}
		message := sanitizeGatewayDetail(string(detail), append(client.redactionSecrets, token)...)
		message = boundGatewayDetail(message, gatewayErrorDetailLimit, truncated)
		return rest.ClientResponseError{Status: resp.StatusCode, Message: message}
	}

	responseBody, truncated, err := readGatewayBody(resp.Body, gatewayResponseBodyLimit)
	if err != nil {
		return client.sanitizeError(err, token)
	}
	if truncated {
		return fmt.Errorf("provider gateway response exceeds %d-byte limit", gatewayResponseBodyLimit)
	}
	if result == nil {
		return nil
	}

	if err := json.NewDecoder(bytes.NewReader(responseBody)).Decode(result); err != nil {
		return client.sanitizeError(err, token)
	}
	return nil
}

func readGatewayBody(reader io.Reader, limit int64) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > limit {
		return body[:limit], true, nil
	}
	return body, false, nil
}

func (client *gatewayClient) sanitizeError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}

	allSecrets := append(client.redactionSecrets, secrets...)
	var responseErr rest.ClientResponseError
	if errors.As(err, &responseErr) {
		responseErr.Message = boundGatewayDetail(
			sanitizeGatewayDetail(responseErr.Message, allSecrets...),
			gatewayErrorDetailLimit,
			false,
		)
		return responseErr
	}

	message := boundGatewayDetail(
		sanitizeGatewayDetail(err.Error(), allSecrets...),
		gatewayErrorDetailLimit,
		false,
	)
	return gatewayBoundaryError{message: message, cause: err}
}

func (client *gatewayClient) sanitizeStreamClose(ctx context.Context, input <-chan string) <-chan string {
	if input == nil {
		return nil
	}

	output := make(chan string, 1)
	go func() {
		defer close(output)
		for {
			select {
			case <-ctx.Done():
				return
			case reason, ok := <-input:
				if !ok {
					return
				}
				reason = boundGatewayDetail(
					sanitizeGatewayDetail(reason, client.redactionSecrets...),
					gatewayErrorDetailLimit,
					false,
				)
				output <- reason
			}
		}
	}()
	return output
}

type gatewayBoundaryError struct {
	message string
	cause   error
}

func (err gatewayBoundaryError) Error() string {
	return err.message
}

func (err gatewayBoundaryError) Unwrap() error {
	return err.cause
}

func sanitizeGatewayDetail(detail string, secrets ...string) string {
	detail = ansi.Strip(detail)
	detail = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, detail)

	for _, secret := range secrets {
		if secret != "" {
			detail = strings.ReplaceAll(detail, secret, "[REDACTED]")
		}
	}

	credentialPatterns := []struct {
		expression  string
		replacement string
	}{
		{
			expression:  `(?i)authorization["']?\s*[:=]\s*["']?(?:bearer\s+)?[^\s,;"'<>}\]]+`,
			replacement: "Authorization: [REDACTED]",
		},
		{
			expression:  `(?i)bearer\s+[^\s,;"'<>}\]]+`,
			replacement: "Bearer [REDACTED]",
		},
		{
			expression:  `(?i)(?:x-)?api[-_ ]?key["']?\s*[:=]\s*["']?[^\s,;"'<>}\]]+`,
			replacement: "api-key: [REDACTED]",
		},
	}
	for _, pattern := range credentialPatterns {
		detail = regexp.MustCompile(pattern.expression).ReplaceAllString(detail, pattern.replacement)
	}

	return strings.Join(strings.Fields(detail), " ")
}

func boundGatewayDetail(detail string, limit int, truncated bool) string {
	if limit <= 0 {
		return ""
	}
	if len(detail) <= limit && !truncated {
		return detail
	}
	if limit < len(gatewayTruncatedDetailLabel) {
		return gatewayTruncatedDetailLabel[:limit]
	}

	contentLimit := limit - len(gatewayTruncatedDetailLabel)
	if len(detail) > contentLimit {
		detail = detail[:contentLimit]
		for !utf8.ValidString(detail) {
			detail = detail[:len(detail)-1]
		}
	}
	return strings.TrimSpace(detail) + gatewayTruncatedDetailLabel
}
