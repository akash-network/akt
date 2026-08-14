package e2e

import (
	"bufio"
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"pkg.akt.dev/go/sdl"
)

const (
	envConsoleMutationOptIn          = "AKT_E2E_CONSOLE_MUTATION"
	envConsoleMutationMaxRequest     = "AKT_E2E_CONSOLE_MAX_REQUEST_USD"
	envConsoleMutationMaxSpend       = "AKT_E2E_CONSOLE_MAX_SPEND_USD"
	envConsoleMutationMaxDeployments = "AKT_E2E_CONSOLE_MAX_DEPLOYMENTS"
	envConsoleMutationMaxRuntime     = "AKT_E2E_CONSOLE_MAX_RUNTIME"
	consoleMutationOptInValue        = "I_UNDERSTAND_THIS_SPENDS_SANDBOX_FUNDS"
	consoleRuntimeAPIKeyEnv          = "AKT_CONSOLE_API_KEY"

	consoleLifecycleRequestUSD       = 1.0
	consoleCreateDepositUSD          = 0.5
	consoleAdditionalDepositUSD      = 0.5
	consoleDefaultMaxRequestUSD      = 1.0
	consoleHardMaxRequestUSD         = 5.0
	consoleDefaultMaxSpendUSD        = 1.0
	consoleHardMaxSpendUSD           = 1.0
	consoleDefaultMaxDeployments     = 1
	consoleHardMaxDeployments        = 1
	consoleLifecycleMaxLeases        = 1
	consoleDefaultMaxRuntime         = 5 * time.Minute
	consoleMinimumMaxRuntime         = 2 * time.Minute
	consoleHardMaxRuntime            = 10 * time.Minute
	consoleCleanupReserve            = 90 * time.Second
	consoleCleanupDiscoveryBudget    = 30 * time.Second
	consoleCleanupMutationReserve    = 40 * time.Second
	consoleCleanupObservationReserve = 20 * time.Second
	consoleCleanupBalanceReserve     = 5 * time.Second
	consoleObserverRequestTimeout    = 10 * time.Second
	consoleCommandCaptureLimit       = 1 << 20
	consoleObserverResponseLimit     = 4 << 20
	consoleActionLogReadLimit        = 1 << 20
	consoleSafetyBillingInterval     = time.Second
)

type consoleMutationConfig struct {
	APIURL         string
	MaxRequestUSD  float64
	MaxSpendUSD    float64
	MaxDeployments int
	MaxRuntime     time.Duration
}

func loadConsoleMutationConfig(getenv func(string) string) (consoleMutationConfig, error) {
	apiURL, err := validateConsoleMutationEndpoint(getenv(envConsoleLiveAPIURL), false)
	if err != nil {
		return consoleMutationConfig{}, err
	}
	if err := validateConsoleCredentialEndpoint(apiURL, getenv(envConsoleLiveKey), false); err != nil {
		return consoleMutationConfig{}, err
	}

	maxRequest := consoleDefaultMaxRequestUSD
	if value := strings.TrimSpace(getenv(envConsoleMutationMaxRequest)); value != "" {
		maxRequest, err = strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(maxRequest) || math.IsInf(maxRequest, 0) {
			return consoleMutationConfig{}, fmt.Errorf("%s must be a finite USD amount, got %q", envConsoleMutationMaxRequest, value)
		}
	}
	if maxRequest < consoleLifecycleRequestUSD {
		return consoleMutationConfig{}, fmt.Errorf("%s=%.2f cannot cover the bounded lifecycle request total of $%.2f", envConsoleMutationMaxRequest, maxRequest, consoleLifecycleRequestUSD)
	}
	if maxRequest > consoleHardMaxRequestUSD {
		return consoleMutationConfig{}, fmt.Errorf("%s=%.2f exceeds the hard test ceiling of $%.2f", envConsoleMutationMaxRequest, maxRequest, consoleHardMaxRequestUSD)
	}

	maxSpend := consoleDefaultMaxSpendUSD
	if value := strings.TrimSpace(getenv(envConsoleMutationMaxSpend)); value != "" {
		maxSpend, err = strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(maxSpend) || math.IsInf(maxSpend, 0) {
			return consoleMutationConfig{}, fmt.Errorf("%s must be a finite USD amount, got %q", envConsoleMutationMaxSpend, value)
		}
	}
	if maxSpend <= 0 {
		return consoleMutationConfig{}, fmt.Errorf("%s must be a positive USD amount, got %.6f", envConsoleMutationMaxSpend, maxSpend)
	}
	if maxSpend < consoleLifecycleRequestUSD {
		return consoleMutationConfig{}, fmt.Errorf("%s=%.2f cannot cover the fixed lifecycle escrow of $%.2f", envConsoleMutationMaxSpend, maxSpend, consoleLifecycleRequestUSD)
	}
	if maxSpend > consoleHardMaxSpendUSD {
		return consoleMutationConfig{}, fmt.Errorf("%s=%.2f exceeds the hard test ceiling of $%.2f", envConsoleMutationMaxSpend, maxSpend, consoleHardMaxSpendUSD)
	}

	maxDeployments := consoleDefaultMaxDeployments
	if value := strings.TrimSpace(getenv(envConsoleMutationMaxDeployments)); value != "" {
		maxDeployments, err = strconv.Atoi(value)
		if err != nil {
			return consoleMutationConfig{}, fmt.Errorf("%s must be an integer, got %q", envConsoleMutationMaxDeployments, value)
		}
	}
	if maxDeployments < 1 {
		return consoleMutationConfig{}, fmt.Errorf("%s must allow the one deployment exercised by this lifecycle", envConsoleMutationMaxDeployments)
	}
	if maxDeployments > consoleHardMaxDeployments {
		return consoleMutationConfig{}, fmt.Errorf("%s=%d exceeds the hard test ceiling of %d deployment", envConsoleMutationMaxDeployments, maxDeployments, consoleHardMaxDeployments)
	}

	maxRuntime := consoleDefaultMaxRuntime
	if value := strings.TrimSpace(getenv(envConsoleMutationMaxRuntime)); value != "" {
		maxRuntime, err = time.ParseDuration(value)
		if err != nil {
			return consoleMutationConfig{}, fmt.Errorf("%s must be a Go duration such as 5m, got %q", envConsoleMutationMaxRuntime, value)
		}
	}
	if maxRuntime < consoleMinimumMaxRuntime {
		return consoleMutationConfig{}, fmt.Errorf("%s=%s leaves too little time for a bid and verified cleanup; minimum is %s", envConsoleMutationMaxRuntime, maxRuntime, consoleMinimumMaxRuntime)
	}
	if maxRuntime > consoleHardMaxRuntime {
		return consoleMutationConfig{}, fmt.Errorf("%s=%s exceeds the hard test ceiling of %s", envConsoleMutationMaxRuntime, maxRuntime, consoleHardMaxRuntime)
	}

	return consoleMutationConfig{
		APIURL:         apiURL,
		MaxRequestUSD:  maxRequest,
		MaxSpendUSD:    maxSpend,
		MaxDeployments: maxDeployments,
		MaxRuntime:     maxRuntime,
	}, nil
}

// validateConsoleMutationEndpoint deliberately accepts only Console's two
// first-party sandbox origins or a loopback HTTP origin used by hermetic tests.
// A sandbox-looking hostname is not sufficient because this URL receives an
// API credential. Rejection errors never include the secret-backed raw value.
func validateConsoleMutationEndpoint(raw string, allowLoopback bool) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%s is required for mutation runs; production is never the default", envConsoleLiveAPIURL)
	}
	if strings.TrimSpace(raw) != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", envConsoleLiveAPIURL)
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("%s must be an absolute http(s) URL", envConsoleLiveAPIURL)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", fmt.Errorf("%s must use http or https", envConsoleLiveAPIURL)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%s must not contain credentials, a query, or a fragment", envConsoleLiveAPIURL)
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("%s must include a hostname", envConsoleLiveAPIURL)
	}
	if host == "console-api.akash.network" || host == "console.akash.network" {
		return "", errors.New("refusing the production Console mutation endpoint")
	}

	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
	}
	if loopback {
		if !allowLoopback {
			return "", errors.New("loopback Console endpoints are restricted to explicit hermetic tests")
		}
		if scheme != "http" {
			return "", errors.New("loopback Console test endpoints must use HTTP")
		}
		if u.Path != "" && u.Path != "/" {
			return "", errors.New("loopback Console test endpoints must be an origin without a base path")
		}

		canonicalHost := host
		if strings.Contains(host, ":") {
			canonicalHost = "[" + host + "]"
		}
		if port := u.Port(); port != "" {
			canonicalHost = net.JoinHostPort(host, port)
		}
		return "http://" + canonicalHost, nil
	}

	if host != "console-api-sandbox-staging.akash.network" &&
		host != "console-api-sandbox.akash.network" {
		return "", errors.New("Console mutation endpoint is not an approved Akash sandbox origin")
	}
	if scheme != "https" {
		return "", errors.New("approved remote Console sandbox endpoints must use HTTPS")
	}
	if port := u.Port(); port != "" && port != "443" {
		return "", errors.New("approved remote Console sandbox endpoints must use the default HTTPS port")
	}
	if u.Path != "" && u.Path != "/" {
		return "", errors.New("approved remote Console sandbox endpoints must be an origin without a base path")
	}

	return "https://" + host, nil
}

// validateConsoleCredentialEndpoint catches the easy-to-miss distinction
// between Console's staging sandbox and its production-namespace sandbox.
// The environment segment is public metadata embedded in every Console
// service key; the credential itself is never included in an error.
func validateConsoleCredentialEndpoint(rawURL, key string, allowLoopback bool) error {
	if key == "" {
		return fmt.Errorf("%s is required", envConsoleLiveKey)
	}
	if strings.TrimSpace(key) != key {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", envConsoleLiveKey)
	}

	canonicalURL, err := validateConsoleMutationEndpoint(rawURL, allowLoopback)
	if err != nil {
		return err
	}
	u, err := url.Parse(canonicalURL)
	if err != nil {
		return errors.New("parse Console endpoint for credential validation")
	}

	host := strings.ToLower(u.Hostname())
	loopback := host == "localhost"
	if ip := net.ParseIP(host); ip != nil {
		loopback = ip.IsLoopback()
	}
	if loopback {
		return nil
	}

	expectedEnvironment := ""
	switch host {
	case "console-api-sandbox-staging.akash.network":
		expectedEnvironment = "staging"
	case "console-api-sandbox.akash.network":
		expectedEnvironment = "production"
	default:
		return errors.New("Console credential endpoint is not an approved Akash sandbox origin")
	}

	parts := strings.Split(key, ".")
	if len(parts) != 4 || parts[3] == "" {
		return fmt.Errorf("%s has an unrecognized Console service-key format", envConsoleLiveKey)
	}
	if parts[0] != "ac" {
		return fmt.Errorf("%s must use the Console service-key prefix", envConsoleLiveKey)
	}
	if parts[1] != "sk" {
		return fmt.Errorf("%s must use the Console service-key type", envConsoleLiveKey)
	}
	if parts[2] != expectedEnvironment {
		return fmt.Errorf("selected Console sandbox requires a %q service-key environment", expectedEnvironment)
	}

	return nil
}

type consoleCommandResult struct {
	Stdout          string
	Stderr          string
	StdoutBytes     int64
	StderrBytes     int64
	StdoutTruncated bool
	StderrTruncated bool
	CredentialLeak  bool
	Exit            int
	Err             error
}

type consoleCappedBuffer struct {
	buffer         bytes.Buffer
	limit          int
	total          int64
	truncated      bool
	needle         []byte
	tail           []byte
	containsNeedle bool
}

func (buffer *consoleCappedBuffer) Write(data []byte) (int, error) {
	buffer.total += int64(len(data))
	if len(buffer.needle) > 0 && !buffer.containsNeedle {
		window := make([]byte, 0, len(buffer.tail)+len(data))
		window = append(window, buffer.tail...)
		window = append(window, data...)
		buffer.containsNeedle = bytes.Contains(window, buffer.needle)
		keep := len(buffer.needle) - 1
		if keep > len(window) {
			keep = len(window)
		}
		buffer.tail = append(buffer.tail[:0], window[len(window)-keep:]...)
	}
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		write := len(data)
		if write > remaining {
			write = remaining
		}
		_, _ = buffer.buffer.Write(data[:write])
	}
	if buffer.total > int64(buffer.limit) {
		buffer.truncated = true
	}
	return len(data), nil
}

func (buffer *consoleCappedBuffer) String() string { return buffer.buffer.String() }

func consoleAktArgs(home string, args ...string) []string {
	fullArgs := []string{"--home", home, "--output", "json"}
	return append(fullArgs, args...)
}

func runConsoleAkt(ctx context.Context, t *testing.T, home string, args ...string) consoleCommandResult {
	t.Helper()

	cmd := exec.CommandContext(ctx, aktBinary(t), consoleAktArgs(home, args...)...)
	cmd.Env = consoleSubprocessEnvironment(os.Environ())
	secret := os.Getenv(consoleRuntimeAPIKeyEnv)
	stdout := consoleCappedBuffer{limit: consoleCommandCaptureLimit, needle: []byte(secret)}
	stderr := consoleCappedBuffer{limit: consoleCommandCaptureLimit, needle: []byte(secret)}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}

	result := consoleCommandResult{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		StdoutBytes:     stdout.total,
		StderrBytes:     stderr.total,
		StdoutTruncated: stdout.truncated,
		StderrTruncated: stderr.truncated,
		Exit:            exitCode,
		Err:             err,
	}
	if secret != "" && (stdout.containsNeedle || stderr.containsNeedle) {
		result.CredentialLeak = true
		// Erase the captured payload immediately so no later failure path can
		// accidentally print or persist the credential.
		result.Stdout = ""
		result.Stderr = ""
	}
	return result
}

func consoleSubprocessEnvironment(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		if strings.HasPrefix(entry, "AKT_E2E_CONSOLE_") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func consoleCommandDiagnostic(result consoleCommandResult) string {
	errorClass := "none"
	switch {
	case errors.Is(result.Err, context.DeadlineExceeded):
		errorClass = "deadline_exceeded"
	case errors.Is(result.Err, context.Canceled):
		errorClass = "canceled"
	case result.Err != nil:
		errorClass = classifyConsoleProcessError(result.Stderr)
	}

	return fmt.Sprintf(
		"exit=%d error_class=%s stdout_bytes=%d stderr_bytes=%d stdout_truncated=%t stderr_truncated=%t credential_leak=%t",
		result.Exit,
		errorClass,
		result.StdoutBytes,
		result.StderrBytes,
		result.StdoutTruncated,
		result.StderrTruncated,
		result.CredentialLeak,
	)
}

func classifyConsoleProcessError(stderr string) string {
	if strings.Contains(stderr, "deposit outcome unknown after one submission") {
		return "console_deposit_outcome_unknown"
	}

	bestIndex := -1
	bestStatus := 0
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
		http.StatusUnprocessableEntity,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		for _, marker := range []string{
			fmt.Sprintf("(HTTP %d)", status),
			fmt.Sprintf("unexpected status %d:", status),
		} {
			index := strings.Index(stderr, marker)
			if index >= 0 && (bestIndex < 0 || index < bestIndex) {
				bestIndex = index
				bestStatus = status
			}
		}
	}
	if bestIndex >= 0 {
		return fmt.Sprintf("console_http_%d", bestStatus)
	}

	return "process_error"
}

type consoleMutationBudget struct {
	maxRequestUSD  float64
	requestedUSD   float64
	maxDeployments int
	deployments    int
	maxLeases      int
	leases         int
}

func newConsoleMutationBudget(config consoleMutationConfig) *consoleMutationBudget {
	return &consoleMutationBudget{
		maxRequestUSD:  config.MaxRequestUSD,
		maxDeployments: config.MaxDeployments,
		maxLeases:      consoleLifecycleMaxLeases,
	}
}

func (budget *consoleMutationBudget) reserveRequest(amount float64) error {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return fmt.Errorf("refusing non-positive or non-finite budget reservation %.6f", amount)
	}
	if budget.requestedUSD+amount > budget.maxRequestUSD+1e-9 {
		return fmt.Errorf("Console lifecycle would request $%.2f after already reserving $%.2f, above the $%.2f request budget", amount, budget.requestedUSD, budget.maxRequestUSD)
	}
	budget.requestedUSD += amount
	return nil
}

func (budget *consoleMutationBudget) reserveDeployment() error {
	if budget.deployments+1 > budget.maxDeployments {
		return fmt.Errorf("Console lifecycle would create %d deployments, above the %d-deployment budget", budget.deployments+1, budget.maxDeployments)
	}
	budget.deployments++
	return nil
}

func (budget *consoleMutationBudget) reserveLease() error {
	if budget.leases+1 > budget.maxLeases {
		return fmt.Errorf("Console lifecycle would create %d leases, above the %d-lease budget", budget.leases+1, budget.maxLeases)
	}
	budget.leases++
	return nil
}

func requireConsoleSuccess(t *testing.T, result consoleCommandResult, command string) {
	t.Helper()
	if result.Exit != 0 || result.Err != nil || result.CredentialLeak {
		t.Fatalf("%s failed (%s)", command, consoleCommandDiagnostic(result))
	}
	if result.StdoutTruncated || result.StderrTruncated {
		t.Fatalf("%s exceeded the bounded output capture (%s)", command, consoleCommandDiagnostic(result))
	}
}

func decodeConsoleJSONDocument(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func requireConsoleJSON(t *testing.T, result consoleCommandResult, command string, target any) {
	t.Helper()
	requireConsoleSuccess(t, result, command)
	if err := decodeConsoleJSONDocument([]byte(result.Stdout), target); err != nil {
		t.Fatalf("%s did not return the expected JSON (%d bytes): %v", command, result.StdoutBytes, err)
	}
}

func decodeConsoleJSONStream(data []byte, visit func(json.RawMessage) error) (int, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	count := 0
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return count, nil
			}
			return count, err
		}
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return count, errors.New("stream record is empty or null")
		}
		if err := visit(raw); err != nil {
			return count, fmt.Errorf("record %d: %w", count+1, err)
		}
		count++
	}
}

func validateConsoleLogStream(data []byte, service string) (int, error) {
	substantive := 0
	count, err := decodeConsoleJSONStream(data, func(raw json.RawMessage) error {
		hasContent, err := validateConsoleLogRecord(raw, service)
		if err != nil {
			return err
		}
		if hasContent {
			substantive++
		}
		return nil
	})
	if err != nil {
		return count, err
	}
	if count == 0 {
		return 0, errors.New("log stream has no records")
	}
	if substantive == 0 {
		return count, errors.New("log stream has no substantive messages")
	}

	return count, nil
}

func validateConsoleLogRecord(raw json.RawMessage, service string) (bool, error) {
	var record struct {
		Name    string  `json:"name"`
		Message *string `json:"message"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return false, err
	}

	podPrefix := service + "-"
	if record.Name != service && (!strings.HasPrefix(record.Name, podPrefix) || len(record.Name) <= len(podPrefix)) {
		return false, errors.New("log source does not match requested service")
	}
	if record.Message == nil {
		return false, errors.New("log message is missing or null")
	}

	return strings.TrimSpace(*record.Message) != "", nil
}

func probeConsoleWorkloadIngress(ctx context.Context, rawURI string) error {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return errors.New("workload URI is empty")
	}
	if !strings.Contains(rawURI, "://") {
		if err := probeConsoleWorkloadIngress(ctx, "https://"+rawURI); err == nil {
			return nil
		}
		if err := probeConsoleWorkloadIngress(ctx, "http://"+rawURI); err == nil {
			return nil
		}
		return errors.New("workload hostname was unreachable over HTTPS and HTTP")
	}

	parsed, err := url.Parse(rawURI)
	if err != nil {
		return fmt.Errorf("parse workload URI: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return errors.New("workload URI must be an HTTP(S) endpoint without credentials")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("construct workload request: %w", err)
	}
	client := &http.Client{
		Timeout: consoleObserverRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request workload ingress: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, consoleCommandCaptureLimit))
		return fmt.Errorf("workload ingress returned HTTP status class %dxx", response.StatusCode/100)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, consoleCommandCaptureLimit+1))
	if err != nil {
		return fmt.Errorf("read workload response: %w", err)
	}
	if len(body) == 0 {
		return errors.New("workload ingress returned an empty response body")
	}
	if len(body) > consoleCommandCaptureLimit {
		return fmt.Errorf("workload ingress response exceeded the %d-byte limit", consoleCommandCaptureLimit)
	}
	return nil
}

type consoleFlexibleID string

func (id *consoleFlexibleID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return errors.New("identifier or amount cannot be null")
	}
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*id = consoleFlexibleID(value)
		return nil
	}

	var value json.Number
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*id = consoleFlexibleID(value.String())
	return nil
}

func (id consoleFlexibleID) String() string { return string(id) }

type consoleLeaseID struct {
	Owner    string            `json:"owner"`
	DSeq     consoleFlexibleID `json:"dseq"`
	GSeq     uint32            `json:"gseq"`
	OSeq     uint32            `json:"oseq"`
	Provider string            `json:"provider"`
}

type consolePriceObservation struct {
	Denom  string            `json:"denom"`
	Amount consoleFlexibleID `json:"amount"`
}

type consoleLeaseObservation struct {
	ID    consoleLeaseID           `json:"id"`
	State string                   `json:"state"`
	Price *consolePriceObservation `json:"price"`
}

type consoleCoinObservation struct {
	Denom  string            `json:"denom"`
	Amount consoleFlexibleID `json:"amount"`
}

type consoleCoinObservations []consoleCoinObservation

func (coins *consoleCoinObservations) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*coins = nil
		return nil
	}
	if len(data) > 0 && data[0] == '[' {
		var list []consoleCoinObservation
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*coins = list
		return nil
	}

	var coin consoleCoinObservation
	if err := json.Unmarshal(data, &coin); err != nil {
		return err
	}
	*coins = []consoleCoinObservation{coin}
	return nil
}

type consoleDeploymentObservation struct {
	Deployment struct {
		ID struct {
			Owner string            `json:"owner"`
			DSeq  consoleFlexibleID `json:"dseq"`
		} `json:"id"`
		State string `json:"state"`
		Hash  string `json:"hash"`
	} `json:"deployment"`
	Leases        []consoleLeaseObservation `json:"leases"`
	EscrowAccount struct {
		State struct {
			Funds       consoleCoinObservations `json:"funds"`
			Transferred consoleCoinObservations `json:"transferred"`
		} `json:"state"`
	} `json:"escrow_account"`
	deploymentPresent        bool
	identityPresent          bool
	ownerPresent             bool
	dseqPresent              bool
	statePresent             bool
	hashPresent              bool
	leasesPresent            bool
	escrowPresent            bool
	escrowFundsPresent       bool
	escrowTransferredPresent bool
}

func (observation *consoleDeploymentObservation) UnmarshalJSON(data []byte) error {
	type wireObservation consoleDeploymentObservation
	var wire wireObservation
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	deployment, hasDeployment := object["deployment"]
	wire.deploymentPresent = hasDeployment && !bytes.Equal(bytes.TrimSpace(deployment), []byte("null"))
	if wire.deploymentPresent {
		var deploymentObject map[string]json.RawMessage
		if err := json.Unmarshal(deployment, &deploymentObject); err == nil {
			identity, hasIdentity := deploymentObject["id"]
			wire.identityPresent = hasIdentity && !bytes.Equal(bytes.TrimSpace(identity), []byte("null"))
			if wire.identityPresent {
				var identityObject map[string]json.RawMessage
				if err := json.Unmarshal(identity, &identityObject); err == nil {
					owner, hasOwner := identityObject["owner"]
					wire.ownerPresent = hasOwner && !bytes.Equal(bytes.TrimSpace(owner), []byte("null"))
					dseq, hasDSeq := identityObject["dseq"]
					wire.dseqPresent = hasDSeq && !bytes.Equal(bytes.TrimSpace(dseq), []byte("null"))
				}
			}
			state, hasState := deploymentObject["state"]
			wire.statePresent = hasState && !bytes.Equal(bytes.TrimSpace(state), []byte("null"))
			hash, hasHash := deploymentObject["hash"]
			wire.hashPresent = hasHash && !bytes.Equal(bytes.TrimSpace(hash), []byte("null"))
		}
	}
	leases, hasLeases := object["leases"]
	wire.leasesPresent = hasLeases && !bytes.Equal(bytes.TrimSpace(leases), []byte("null"))

	var escrow struct {
		State json.RawMessage `json:"state"`
	}
	if raw, ok := object["escrow_account"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && json.Unmarshal(raw, &escrow) == nil {
		wire.escrowPresent = true
		var state map[string]json.RawMessage
		if json.Unmarshal(escrow.State, &state) == nil {
			funds, ok := state["funds"]
			wire.escrowFundsPresent = ok && !bytes.Equal(bytes.TrimSpace(funds), []byte("null"))
			transferred, ok := state["transferred"]
			wire.escrowTransferredPresent = ok && !bytes.Equal(bytes.TrimSpace(transferred), []byte("null"))
		}
	}

	*observation = consoleDeploymentObservation(wire)
	return nil
}

type consoleDeploymentPagination struct {
	Total         int
	Skip          int
	Limit         int
	HasMore       bool
	fieldsPresent bool
}

func (pagination *consoleDeploymentPagination) UnmarshalJSON(data []byte) error {
	var wire struct {
		Total   *int  `json:"total"`
		Skip    *int  `json:"skip"`
		Limit   *int  `json:"limit"`
		HasMore *bool `json:"hasMore"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Total == nil || wire.Skip == nil || wire.Limit == nil || wire.HasMore == nil {
		*pagination = consoleDeploymentPagination{}
		return nil
	}
	*pagination = consoleDeploymentPagination{
		Total:         *wire.Total,
		Skip:          *wire.Skip,
		Limit:         *wire.Limit,
		HasMore:       *wire.HasMore,
		fieldsPresent: true,
	}
	return nil
}

type consoleDeploymentList struct {
	Deployments        []consoleDeploymentObservation
	Pagination         consoleDeploymentPagination
	deploymentsPresent bool
	paginationPresent  bool
}

func (list *consoleDeploymentList) UnmarshalJSON(data []byte) error {
	var wire struct {
		Deployments *[]consoleDeploymentObservation `json:"deployments"`
		Pagination  *consoleDeploymentPagination    `json:"pagination"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*list = consoleDeploymentList{
		deploymentsPresent: wire.Deployments != nil,
		paginationPresent:  wire.Pagination != nil,
	}
	if wire.Deployments != nil {
		list.Deployments = *wire.Deployments
	}
	if wire.Pagination != nil {
		list.Pagination = *wire.Pagination
	}
	return nil
}

type consoleBidObservation struct {
	ID    consoleLeaseID           `json:"id"`
	State string                   `json:"state"`
	Price *consolePriceObservation `json:"price"`
}

func validConsolePrice(price *consolePriceObservation) bool {
	if price == nil || price.Denom == "" || price.Amount.String() == "" {
		return false
	}
	amount, ok := new(big.Rat).SetString(price.Amount.String())
	return ok && amount.Sign() > 0
}

func projectedConsoleBidSpendUSD(price *consolePriceObservation, paidRuntime time.Duration) (*big.Rat, error) {
	if !validConsolePrice(price) || !strings.EqualFold(price.Denom, "uact") {
		return nil, errors.New("bid price must be a positive uact amount")
	}
	amount, ok := new(big.Rat).SetString(price.Amount.String())
	if !ok {
		return nil, errors.New("bid price amount is not numeric")
	}
	if paidRuntime < 0 {
		return nil, errors.New("paid runtime cannot be negative")
	}
	intervals := int64(0)
	if paidRuntime > 0 {
		intervals = int64((paidRuntime + consoleSafetyBillingInterval - 1) / consoleSafetyBillingInterval)
	}
	return new(big.Rat).Quo(
		new(big.Rat).Mul(amount, new(big.Rat).SetInt64(intervals)),
		new(big.Rat).SetInt64(1_000_000),
	), nil
}

func selectConsoleBudgetSafeBid(
	cliBids []consoleBidObservation,
	directBids []consoleBidObservation,
	dseq string,
	maxSpendUSD float64,
	paidRuntime time.Duration,
) (consoleBidObservation, *big.Rat, bool) {
	spendLimit := new(big.Rat).SetFloat64(maxSpendUSD)
	if spendLimit == nil || spendLimit.Sign() <= 0 {
		return consoleBidObservation{}, nil, false
	}
	type candidate struct {
		bid       consoleBidObservation
		price     *big.Rat
		projected *big.Rat
	}
	eligible := make([]candidate, 0, len(cliBids))
	for _, bid := range cliBids {
		if bid.ID.Owner == "" || bid.ID.DSeq.String() != dseq || bid.ID.Provider == "" || bid.ID.GSeq == 0 || bid.ID.OSeq == 0 ||
			(!strings.EqualFold(bid.State, "open") && !strings.EqualFold(bid.State, "active")) {
			continue
		}
		var corroborated *consoleBidObservation
		for index := range directBids {
			if sameConsoleBid(bid, directBids[index]) {
				corroborated = &directBids[index]
				break
			}
		}
		if corroborated == nil {
			continue
		}
		projected, err := projectedConsoleBidSpendUSD(corroborated.Price, paidRuntime)
		if err != nil || projected.Cmp(spendLimit) > 0 {
			continue
		}
		price, _ := new(big.Rat).SetString(corroborated.Price.Amount.String())
		eligible = append(eligible, candidate{bid: *corroborated, price: price, projected: projected})
	}
	if len(eligible) == 0 {
		return consoleBidObservation{}, nil, false
	}
	sort.Slice(eligible, func(i, j int) bool {
		if comparison := eligible[i].price.Cmp(eligible[j].price); comparison != 0 {
			return comparison < 0
		}
		if eligible[i].bid.ID.Provider != eligible[j].bid.ID.Provider {
			return eligible[i].bid.ID.Provider < eligible[j].bid.ID.Provider
		}
		if eligible[i].bid.ID.GSeq != eligible[j].bid.ID.GSeq {
			return eligible[i].bid.ID.GSeq < eligible[j].bid.ID.GSeq
		}
		return eligible[i].bid.ID.OSeq < eligible[j].bid.ID.OSeq
	})
	return eligible[0].bid, eligible[0].projected, true
}

type consoleActionObservation struct {
	Type     string          `json:"type"`
	Action   string          `json:"action"`
	Status   string          `json:"status"`
	DSeq     uint64          `json:"dseq"`
	Provider string          `json:"provider"`
	Error    string          `json:"error"`
	Params   json.RawMessage `json:"params"`
}

type consoleBalancesObservation struct {
	Balance     int64 `json:"balance"`
	Deployments int64 `json:"deployments"`
	Total       int64 `json:"total"`
}

type consoleAPIKeyObservation struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	ExpiresAt  string          `json:"expiresAt"`
	CreatedAt  string          `json:"createdAt"`
	LastUsedAt string          `json:"lastUsedAt"`
	KeyFormat  string          `json:"keyFormat"`
	Secret     json.RawMessage `json:"apiKey"`
}

func (balances consoleBalancesObservation) AvailableUSD() float64 {
	return float64(balances.Balance) / 1e6
}

func (balances consoleBalancesObservation) TotalUSD() float64 {
	return float64(balances.Total) / 1e6
}

type consoleSettingsObservation struct {
	DSeq             consoleFlexibleID `json:"dseq"`
	AutoTopUpEnabled bool              `json:"autoTopUpEnabled"`
}

type consoleObserverHTTPError struct {
	Method       string
	Path         string
	StatusCode   int
	ResponseSize int
}

type consoleObserverRedirectError struct{}

func (*consoleObserverRedirectError) Error() string {
	return "Console observer redirects are not allowed"
}

func (err *consoleObserverHTTPError) Error() string {
	return fmt.Sprintf(
		"Console observer %s %s returned HTTP %d (%d response bytes)",
		err.Method,
		err.Path,
		err.StatusCode,
		err.ResponseSize,
	)
}

type consoleAPIObserver struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func newConsoleAPIObserver(baseURL, apiKey string) *consoleAPIObserver {
	return &consoleAPIObserver{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: consoleObserverRequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return &consoleObserverRedirectError{}
			},
		},
	}
}

func (observer *consoleAPIObserver) getDocument(ctx context.Context, path string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, observer.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build Console observer GET %s failed", path)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	if observer.apiKey != "" {
		request.Header.Set("x-api-key", observer.apiKey)
	}

	response, err := observer.client.Do(request)
	if err != nil {
		errorClass := "transport_error"
		var redirectErr *consoleObserverRedirectError
		switch {
		case errors.As(err, &redirectErr):
			errorClass = "redirect_rejected"
		case errors.Is(err, context.DeadlineExceeded):
			errorClass = "deadline_exceeded"
		case errors.Is(err, context.Canceled):
			errorClass = "canceled"
		}
		return nil, fmt.Errorf("execute Console observer GET %s failed (%s)", path, errorClass)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, consoleObserverResponseLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read Console observer GET %s response failed", path)
	}
	if len(body) > consoleObserverResponseLimit {
		return nil, fmt.Errorf("Console observer GET %s response exceeded %d bytes", path, consoleObserverResponseLimit)
	}
	if observer.apiKey != "" && bytes.Contains(body, []byte(observer.apiKey)) {
		return nil, fmt.Errorf("Console observer GET %s response contained the API credential", path)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, &consoleObserverHTTPError{
			Method:       http.MethodGet,
			Path:         path,
			StatusCode:   response.StatusCode,
			ResponseSize: len(body),
		}
	}
	return body, nil
}

func (observer *consoleAPIObserver) getJSON(ctx context.Context, path string, target any) error {
	body, err := observer.getDocument(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode Console observer GET %s document (%d bytes): %w", path, len(body), err)
	}
	return nil
}

func (observer *consoleAPIObserver) getData(ctx context.Context, path string, target any) error {
	body, err := observer.getDocument(ctx, path)
	if err != nil {
		return err
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode Console observer GET %s envelope (%d bytes): %w", path, len(body), err)
	}
	data := bytes.TrimSpace(envelope.Data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return fmt.Errorf("Console observer GET %s returned no data field (%d bytes)", path, len(body))
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode Console observer GET %s data (%d bytes): %w", path, len(body), err)
	}
	return nil
}

func (observer *consoleAPIObserver) getBalances(ctx context.Context) (consoleBalancesObservation, error) {
	var wire struct {
		Balance     *int64 `json:"balance"`
		Deployments *int64 `json:"deployments"`
		Total       *int64 `json:"total"`
	}
	if err := observer.getData(ctx, "/v1/balances", &wire); err != nil {
		return consoleBalancesObservation{}, err
	}
	if wire.Balance == nil || wire.Deployments == nil || wire.Total == nil {
		return consoleBalancesObservation{}, errors.New("Console observer balances omitted a required amount")
	}
	if *wire.Balance < 0 || *wire.Deployments < 0 || *wire.Total < 0 {
		return consoleBalancesObservation{}, errors.New("Console observer balances contained a negative amount")
	}
	return consoleBalancesObservation{
		Balance:     *wire.Balance,
		Deployments: *wire.Deployments,
		Total:       *wire.Total,
	}, nil
}

func validateConsoleAPIKeys(keys []consoleAPIKeyObservation) error {
	seen := make(map[string]struct{}, len(keys))
	for index, key := range keys {
		if strings.TrimSpace(key.ID) == "" || strings.TrimSpace(key.Name) == "" {
			return fmt.Errorf("Console API key %d omitted its ID or name", index)
		}
		if len(bytes.TrimSpace(key.Secret)) != 0 {
			return fmt.Errorf("Console API key %s exposed its one-time secret field", key.ID)
		}
		if _, duplicate := seen[key.ID]; duplicate {
			return fmt.Errorf("Console API key list returned duplicate ID %s", key.ID)
		}
		seen[key.ID] = struct{}{}
	}
	return nil
}

func (observer *consoleAPIObserver) listAPIKeys(ctx context.Context) ([]consoleAPIKeyObservation, error) {
	var keys []consoleAPIKeyObservation
	if err := observer.getData(ctx, "/v1/api-keys", &keys); err != nil {
		return nil, err
	}
	if err := validateConsoleAPIKeys(keys); err != nil {
		return nil, err
	}
	return keys, nil
}

func (observer *consoleAPIObserver) username(ctx context.Context) (string, error) {
	var identity struct {
		Username string `json:"username"`
	}
	if err := observer.getData(ctx, "/v1/user/me", &identity); err != nil {
		return "", err
	}
	if strings.TrimSpace(identity.Username) == "" {
		return "", errors.New("Console identity omitted its username")
	}
	return identity.Username, nil
}

func listConsoleAPIKeys(ctx context.Context, t *testing.T, home string) []consoleAPIKeyObservation {
	t.Helper()

	var keys []consoleAPIKeyObservation
	requireConsoleJSON(t,
		runConsoleAkt(ctx, t, home, "console", "apikey", "list"),
		"akt console apikey list",
		&keys,
	)
	if err := validateConsoleAPIKeys(keys); err != nil {
		t.Fatal(err)
	}
	return keys
}

func sameConsoleAPIKeyRecords(left, right []consoleAPIKeyObservation) bool {
	fingerprint := func(keys []consoleAPIKeyObservation) []string {
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, strings.Join([]string{key.ID, key.Name, key.ExpiresAt, key.CreatedAt, key.KeyFormat}, "\x00"))
		}
		sort.Strings(values)
		return values
	}
	return slices.Equal(fingerprint(left), fingerprint(right))
}

func findConsoleAPIKey(keys []consoleAPIKeyObservation, id string) (consoleAPIKeyObservation, bool) {
	for _, key := range keys {
		if key.ID == id {
			return key, true
		}
	}
	return consoleAPIKeyObservation{}, false
}

func consoleAPIKeysWithoutID(keys []consoleAPIKeyObservation, id string) []consoleAPIKeyObservation {
	filtered := make([]consoleAPIKeyObservation, 0, len(keys))
	for _, key := range keys {
		if key.ID != id {
			filtered = append(filtered, key)
		}
	}
	return filtered
}

func validateConsoleDeploymentPage(page consoleDeploymentList, skip int, requireEscrow bool) error {
	if !page.deploymentsPresent || !page.paginationPresent || !page.Pagination.fieldsPresent {
		return fmt.Errorf("Console deployment page at offset %d omitted required collection or pagination fields", skip)
	}
	if page.Pagination.Total < 0 || page.Pagination.Skip != skip || page.Pagination.Limit <= 0 {
		return fmt.Errorf("Console deployment page at offset %d returned inconsistent pagination", skip)
	}
	if len(page.Deployments) > page.Pagination.Limit || skip+len(page.Deployments) > page.Pagination.Total {
		return fmt.Errorf("Console deployment page at offset %d exceeded its reported limit or total", skip)
	}
	for _, item := range page.Deployments {
		if err := validateConsoleDeploymentObservation(item, "", true, requireEscrow); err != nil {
			return fmt.Errorf("Console deployment page at offset %d contained an invalid entry: %w", skip, err)
		}
	}
	return nil
}

func validateConsoleDeploymentObservation(
	observation consoleDeploymentObservation,
	wantDSeq string,
	requireHash bool,
	requireEscrow bool,
) error {
	dseq := observation.Deployment.ID.DSeq.String()
	if !observation.deploymentPresent || !observation.identityPresent || !observation.dseqPresent ||
		!observation.ownerPresent || !observation.statePresent || !observation.leasesPresent {
		return errors.New("deployment omitted its record, identity, owner, dseq, state, or leases field")
	}
	if !validConsoleDSeq(dseq) || observation.Deployment.ID.Owner == "" || observation.Deployment.State == "" {
		return errors.New("deployment contained an invalid owner, dseq, or state")
	}
	if wantDSeq != "" && dseq != wantDSeq {
		return fmt.Errorf("deployment returned dseq %s, want %s", dseq, wantDSeq)
	}
	if requireHash && (!observation.hashPresent || observation.Deployment.Hash == "") {
		return errors.New("deployment omitted its SDL hash")
	}
	if requireEscrow && (!observation.escrowPresent || !observation.escrowFundsPresent) {
		return errors.New("deployment omitted its escrow account or funds")
	}
	seenLeases := make(map[string]struct{}, len(observation.Leases))
	for _, lease := range observation.Leases {
		if lease.ID.Owner == "" || lease.ID.DSeq.String() != dseq || lease.ID.GSeq == 0 || lease.ID.OSeq == 0 ||
			lease.ID.Owner != observation.Deployment.ID.Owner || lease.ID.Provider == "" || lease.State == "" || !validConsolePrice(lease.Price) {
			return fmt.Errorf("deployment %s contained a lease without a complete identity, state, or price", dseq)
		}
		identity := fmt.Sprintf("%s/%s/%d/%d/%s", lease.ID.Owner, lease.ID.DSeq, lease.ID.GSeq, lease.ID.OSeq, lease.ID.Provider)
		if _, duplicate := seenLeases[identity]; duplicate {
			return fmt.Errorf("deployment %s contained duplicate lease identity %s", dseq, identity)
		}
		seenLeases[identity] = struct{}{}
	}
	return nil
}

func validConsoleDSeq(dseq string) bool {
	value, err := strconv.ParseUint(dseq, 10, 64)
	return err == nil && value > 0
}

func (observer *consoleAPIObserver) listAllDeployments(ctx context.Context) ([]consoleDeploymentObservation, error) {
	const pageSize = 100

	var deployments []consoleDeploymentObservation
	seenDSeqs := make(map[string]struct{})
	reportedTotal := -1
	for skip := 0; ; {
		path := "/v1/deployments?skip=" + strconv.Itoa(skip) + "&limit=" + strconv.Itoa(pageSize)
		var page consoleDeploymentList
		if err := observer.getData(ctx, path, &page); err != nil {
			return nil, fmt.Errorf("list Console deployments at offset %d: %w", skip, err)
		}
		if err := validateConsoleDeploymentPage(page, skip, false); err != nil {
			return nil, err
		}
		if reportedTotal < 0 {
			reportedTotal = page.Pagination.Total
		} else if page.Pagination.Total != reportedTotal {
			return nil, fmt.Errorf("Console deployment pagination total changed from %d to %d", reportedTotal, page.Pagination.Total)
		}
		for _, item := range page.Deployments {
			dseq := item.Deployment.ID.DSeq.String()
			if _, duplicate := seenDSeqs[dseq]; duplicate {
				return nil, fmt.Errorf("Console deployment pagination returned duplicate dseq %s", dseq)
			}
			seenDSeqs[dseq] = struct{}{}
		}
		deployments = append(deployments, page.Deployments...)
		seen := skip + len(page.Deployments)
		if !page.Pagination.HasMore {
			if seen != page.Pagination.Total {
				return nil, fmt.Errorf("Console deployment pagination ended after %d records but reported total %d", seen, page.Pagination.Total)
			}
			return deployments, nil
		}
		if len(page.Deployments) == 0 || seen >= page.Pagination.Total {
			return nil, fmt.Errorf("Console deployment pagination claimed more records after offset %d without a valid next page", skip)
		}
		next := seen
		if next <= skip {
			return nil, fmt.Errorf("Console deployment pagination did not advance from offset %d", skip)
		}
		skip = next
	}
}

func (observer *consoleAPIObserver) getDeployment(ctx context.Context, dseq string) (consoleDeploymentObservation, error) {
	var detail consoleDeploymentObservation
	if err := observer.getData(ctx, "/v1/deployments/"+url.PathEscape(dseq), &detail); err != nil {
		return consoleDeploymentObservation{}, err
	}
	if err := validateConsoleDeploymentObservation(detail, dseq, true, true); err != nil {
		return consoleDeploymentObservation{}, fmt.Errorf("Console observer deployment %s was invalid: %w", dseq, err)
	}
	return detail, nil
}

func (observer *consoleAPIObserver) listBids(ctx context.Context, dseq string) ([]consoleBidObservation, error) {
	var wrapped []struct {
		Bid *consoleBidObservation `json:"bid"`
	}
	if err := observer.getData(ctx, "/v1/bids?dseq="+url.QueryEscape(dseq), &wrapped); err != nil {
		return nil, err
	}

	bids := make([]consoleBidObservation, 0, len(wrapped))
	seen := make(map[string]struct{}, len(wrapped))
	for _, item := range wrapped {
		if item.Bid == nil {
			return nil, errors.New("Console observer bid list contained an entry without a bid")
		}
		bid := *item.Bid
		if bid.ID.Owner == "" || bid.ID.DSeq.String() != dseq || bid.ID.GSeq == 0 || bid.ID.OSeq == 0 ||
			bid.ID.Provider == "" || bid.State == "" || !validConsolePrice(bid.Price) {
			return nil, fmt.Errorf("Console observer bid list for %s contained a bid without a complete identity, state, or price", dseq)
		}
		identity := fmt.Sprintf("%s/%s/%d/%d/%s", bid.ID.Owner, bid.ID.DSeq, bid.ID.GSeq, bid.ID.OSeq, bid.ID.Provider)
		if _, duplicate := seen[identity]; duplicate {
			return nil, fmt.Errorf("Console observer bid list returned duplicate identity %s", identity)
		}
		seen[identity] = struct{}{}
		bids = append(bids, bid)
	}
	return bids, nil
}

func (observer *consoleAPIObserver) getDeploymentSettings(ctx context.Context, dseq string) (consoleSettingsObservation, error) {
	var wire struct {
		DSeq             *consoleFlexibleID `json:"dseq"`
		AutoTopUpEnabled *bool              `json:"autoTopUpEnabled"`
	}
	if err := observer.getData(ctx, "/v2/deployment-settings/"+url.PathEscape(dseq), &wire); err != nil {
		return consoleSettingsObservation{}, err
	}
	if wire.DSeq == nil || wire.AutoTopUpEnabled == nil {
		return consoleSettingsObservation{}, fmt.Errorf("Console observer deployment settings for %s omitted a required field", dseq)
	}
	if wire.DSeq.String() != dseq {
		return consoleSettingsObservation{}, fmt.Errorf("Console observer deployment settings for %s returned dseq %s", dseq, wire.DSeq.String())
	}
	return consoleSettingsObservation{DSeq: *wire.DSeq, AutoTopUpEnabled: *wire.AutoTopUpEnabled}, nil
}

func consoleCleanupPhaseDeadlines(now, overall time.Time) (time.Time, time.Time) {
	mutationDeadline := overall.Add(-consoleCleanupObservationReserve)
	if mutationDeadline.Before(now) {
		mutationDeadline = now
	}
	discoveryDeadline := now.Add(consoleCleanupDiscoveryBudget)
	latestDiscoveryDeadline := mutationDeadline.Add(-consoleCleanupMutationReserve)
	if latestDiscoveryDeadline.Before(now) {
		latestDiscoveryDeadline = now
	}
	if discoveryDeadline.After(latestDiscoveryDeadline) {
		discoveryDeadline = latestDiscoveryDeadline
	}
	return discoveryDeadline, mutationDeadline
}

func listAllConsoleDeployments(ctx context.Context, t *testing.T, home string) ([]consoleDeploymentObservation, error) {
	t.Helper()

	const pageSize = 100
	var deployments []consoleDeploymentObservation
	seenDSeqs := make(map[string]struct{})
	reportedTotal := -1
	for skip := 0; ; {
		result := runConsoleAkt(ctx, t, home, "console", "deployment", "list", "--skip", strconv.Itoa(skip), "--limit", strconv.Itoa(pageSize))
		if result.Exit != 0 || result.Err != nil || result.CredentialLeak {
			return nil, fmt.Errorf("list Console deployments at offset %d (%s)", skip, consoleCommandDiagnostic(result))
		}
		if result.StdoutTruncated || result.StderrTruncated {
			return nil, fmt.Errorf("list Console deployments at offset %d exceeded bounded capture (%s)", skip, consoleCommandDiagnostic(result))
		}

		var page consoleDeploymentList
		if err := decodeConsoleJSONDocument([]byte(result.Stdout), &page); err != nil {
			return nil, fmt.Errorf("decode Console deployment page at offset %d: %w", skip, err)
		}
		if err := validateConsoleDeploymentPage(page, skip, false); err != nil {
			return nil, fmt.Errorf("akt deployment list contract failed: %w", err)
		}
		if reportedTotal < 0 {
			reportedTotal = page.Pagination.Total
		} else if page.Pagination.Total != reportedTotal {
			return nil, fmt.Errorf("akt deployment pagination total changed from %d to %d", reportedTotal, page.Pagination.Total)
		}
		for _, item := range page.Deployments {
			dseq := item.Deployment.ID.DSeq.String()
			if _, duplicate := seenDSeqs[dseq]; duplicate {
				return nil, fmt.Errorf("akt deployment pagination returned duplicate dseq %s", dseq)
			}
			seenDSeqs[dseq] = struct{}{}
		}
		deployments = append(deployments, page.Deployments...)
		if !page.Pagination.HasMore {
			if page.Pagination.Skip+len(page.Deployments) != page.Pagination.Total {
				return nil, fmt.Errorf("akt deployment list ended at an inconsistent total")
			}
			return deployments, nil
		}

		step := len(page.Deployments)
		next := page.Pagination.Skip + step
		if step == 0 || next <= skip {
			return nil, fmt.Errorf("Console deployment pagination did not advance from offset %d", skip)
		}
		skip = next
	}
}

func getConsoleDeployment(ctx context.Context, t *testing.T, home, dseq string) (consoleDeploymentObservation, consoleCommandResult, error) {
	t.Helper()

	result := runConsoleAkt(ctx, t, home, "console", "deployment", "get", dseq)
	if result.Exit != 0 || result.Err != nil || result.CredentialLeak {
		return consoleDeploymentObservation{}, result, fmt.Errorf("get deployment %s (%s)", dseq, consoleCommandDiagnostic(result))
	}
	if result.StdoutTruncated || result.StderrTruncated {
		return consoleDeploymentObservation{}, result, fmt.Errorf("get deployment %s exceeded bounded capture (%s)", dseq, consoleCommandDiagnostic(result))
	}

	var detail consoleDeploymentObservation
	if err := decodeConsoleJSONDocument([]byte(result.Stdout), &detail); err != nil {
		return consoleDeploymentObservation{}, result, fmt.Errorf("decode deployment %s: %w", dseq, err)
	}
	if err := validateConsoleDeploymentObservation(detail, dseq, true, true); err != nil {
		return consoleDeploymentObservation{}, result, fmt.Errorf("akt deployment get %s returned an invalid document: %w", dseq, err)
	}
	return detail, result, nil
}

func readActionsByType(home, contextName, actionType string) ([]consoleActionObservation, error) {
	path := filepath.Join(home, "contexts", contextName, "actions.log")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat Console action log: %w", err)
	}
	if info.Size() > consoleActionLogReadLimit {
		return nil, fmt.Errorf("Console action log exceeded the %d-byte test limit", consoleActionLogReadLimit)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open Console action log: %w", err)
	}
	defer file.Close()

	limited := io.LimitReader(file, consoleActionLogReadLimit+1)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), consoleActionLogReadLimit)
	entries := make([]consoleActionObservation, 0)
	for line := 1; scanner.Scan(); line++ {
		var entry consoleActionObservation
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("decode Console action log line %d (%d bytes): %w", line, len(scanner.Bytes()), err)
		}
		if entry.Type == actionType {
			entries = append(entries, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read bounded Console action log: %w", err)
	}
	// Keep the prior command's most-recent-first contract so the semantic
	// assertions below do not depend on storage order.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}

func readConsoleActions(home, contextName string) ([]consoleActionObservation, error) {
	return readActionsByType(home, contextName, "console")
}

func readProviderActions(home, contextName string) ([]consoleActionObservation, error) {
	return readActionsByType(home, contextName, "provider")
}

func consoleActionSummary(entry consoleActionObservation) string {
	return fmt.Sprintf(
		"type=%q action=%q status=%q dseq=%d error_bytes=%d params_bytes=%d",
		entry.Type,
		entry.Action,
		entry.Status,
		entry.DSeq,
		len(entry.Error),
		len(entry.Params),
	)
}

func assertConsoleActions(t *testing.T, home, contextName, dseq string, expected ...string) {
	t.Helper()

	entries, err := readConsoleActions(home, contextName)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(expected) {
		t.Fatalf("Console action log has %d entries, want %d (%v)", len(entries), len(expected), expected)
	}

	wantDSeq, err := strconv.ParseUint(dseq, 10, 64)
	if dseq != "" && err != nil {
		t.Fatalf("invalid expected dseq %q: %v", dseq, err)
	}
	for i, action := range expected {
		entry := entries[len(entries)-1-i]
		if entry.Type != "console" || entry.Action != action || entry.Status != "success" {
			t.Errorf("Console action %d = %s, want action=%q type=console status=success", i, consoleActionSummary(entry), action)
		}
		if dseq != "" && entry.DSeq != wantDSeq {
			t.Errorf("Console action %q dseq = %d, want %d", action, entry.DSeq, wantDSeq)
		}
		if entry.Error != "" {
			t.Errorf("successful Console action %q retained %d error bytes", action, len(entry.Error))
		}
	}
}

func assertConsoleActionTail(t *testing.T, home, contextName string, priorCount int, expected ...string) {
	t.Helper()

	entries, err := readConsoleActions(home, contextName)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != priorCount+len(expected) {
		t.Fatalf("Console action log has %d entries, want %d after action tail %v", len(entries), priorCount+len(expected), expected)
	}
	for index, action := range expected {
		entry := entries[len(entries)-1-(priorCount+index)]
		if entry.Type != "console" || entry.Action != action || entry.Status != "success" || entry.DSeq != 0 || entry.Error != "" {
			t.Errorf("Console action tail %d = %s, want action=%q type=console status=success dseq=0", index, consoleActionSummary(entry), action)
		}
	}
}

func consoleSDLVersionHash(rawSDL string) (string, error) {
	doc, err := sdl.Read([]byte(rawSDL))
	if err != nil {
		return "", err
	}
	version, err := doc.Version()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(version), nil
}

func newConsoleRunID(t *testing.T) string {
	t.Helper()

	random := make([]byte, 6)
	if _, err := cryptorand.Read(random); err != nil {
		t.Fatalf("generate Console E2E run identity: %v", err)
	}
	return "akt-e2e-" + time.Now().UTC().Format("20060102t150405") + "-" + hex.EncodeToString(random)
}

func consoleLifecycleSDL(runID, phase string) string {
	return fmt.Sprintf(`---
version: "2.0"
services:
  web:
    image: baktun/hello-akash-world:1.0.0
    env:
      - AKT_E2E_RUN_ID=%s
      - AKT_E2E_PHASE=%s
    expose:
      - port: 3000
        as: 80
        to:
          - global: true
profiles:
  compute:
    web:
      resources:
        cpu:
          units: 0.5
        memory:
          size: 512Mi
        storage:
          - size: 512Mi
  placement:
    dcloud:
      pricing:
        web:
          denom: uact
          amount: 1000
deployment:
  web:
    dcloud:
      profile: web
      count: 1
`, runID, phase)
}

func writeConsoleSDL(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write Console E2E SDL: %v", err)
	}
	return path
}

func consoleEscrowFunds(detail consoleDeploymentObservation) (map[string]*big.Rat, error) {
	if !detail.escrowFundsPresent {
		return nil, errors.New("deployment detail omitted escrow funds")
	}
	funds := make(map[string]*big.Rat, len(detail.EscrowAccount.State.Funds))
	for _, coin := range detail.EscrowAccount.State.Funds {
		if coin.Denom == "" {
			return nil, errors.New("escrow fund omitted its denomination")
		}
		amount, err := parseConsoleObservedEscrowAmount(coin.Amount.String())
		if err != nil {
			return nil, fmt.Errorf("escrow amount %q for %s is not a valid fixed-point decimal: %w", coin.Amount, coin.Denom, err)
		}
		if current := funds[coin.Denom]; current != nil {
			amount.Add(amount, current)
		}
		funds[coin.Denom] = amount
	}
	return funds, nil
}

func consoleEscrowTransferred(detail consoleDeploymentObservation) (map[string]*big.Rat, error) {
	if !detail.escrowTransferredPresent {
		return nil, errors.New("deployment detail omitted escrow transferred")
	}
	transferred := make(map[string]*big.Rat, len(detail.EscrowAccount.State.Transferred))
	for _, coin := range detail.EscrowAccount.State.Transferred {
		if coin.Denom == "" {
			return nil, errors.New("escrow transferred coin omitted its denomination")
		}
		amount, err := parseConsoleObservedEscrowAmount(coin.Amount.String())
		if err != nil {
			return nil, fmt.Errorf("escrow transferred amount %q for %s is not a valid fixed-point decimal: %w", coin.Amount, coin.Denom, err)
		}
		if amount.Sign() < 0 {
			return nil, fmt.Errorf("escrow transferred amount for %s must be non-negative", coin.Denom)
		}
		if current := transferred[coin.Denom]; current != nil {
			amount.Add(amount, current)
		}
		transferred[coin.Denom] = amount
	}
	return transferred, nil
}

func consoleEscrowAmountForDenom(coins map[string]*big.Rat, denom, field string) (*big.Rat, error) {
	denoms := make([]string, 0, len(coins))
	for coinDenom := range coins {
		denoms = append(denoms, coinDenom)
	}
	sort.Strings(denoms)
	for _, coinDenom := range denoms {
		if coinDenom != denom {
			return nil, fmt.Errorf("escrow %s used unexpected denomination %s instead of %s", field, coinDenom, denom)
		}
		if coins[coinDenom] == nil {
			return nil, fmt.Errorf("escrow %s for %s had no amount", field, coinDenom)
		}
	}
	if amount := coins[denom]; amount != nil {
		return new(big.Rat).Set(amount), nil
	}
	return new(big.Rat), nil
}

func consoleEscrowAccountingForDenom(detail consoleDeploymentObservation, denom string) (*big.Rat, *big.Rat, error) {
	funds, err := consoleEscrowFunds(detail)
	if err != nil {
		return nil, nil, err
	}
	transferred, err := consoleEscrowTransferred(detail)
	if err != nil {
		return nil, nil, err
	}
	fundsAmount, err := consoleEscrowAmountForDenom(funds, denom, "funds")
	if err != nil {
		return nil, nil, err
	}
	transferredAmount, err := consoleEscrowAmountForDenom(transferred, denom, "transferred")
	if err != nil {
		return nil, nil, err
	}
	return new(big.Rat).Add(fundsAmount, transferredAmount), transferredAmount, nil
}

func consoleTransferredSpend(before, after *big.Rat) (*big.Rat, error) {
	if before == nil || after == nil {
		return nil, errors.New("Console transferred spend requires pre-lease and terminal observations")
	}
	if before.Sign() < 0 || after.Sign() < 0 {
		return nil, errors.New("Console cumulative transferred amount cannot be negative")
	}
	spend := new(big.Rat).Sub(new(big.Rat).Set(after), before)
	if spend.Sign() < 0 {
		return nil, errors.New("Console cumulative transferred amount regressed after the lease")
	}
	return spend, nil
}

func consoleTerminalTransferredSpend(detail consoleDeploymentObservation, baseline *big.Rat, requirePositive bool) (*big.Rat, error) {
	if problem := consoleTerminalDeploymentProblem(detail); problem != "" {
		return nil, fmt.Errorf("terminal escrow was not terminal: %s", problem)
	}
	funded, transferred, err := consoleEscrowAccountingForDenom(detail, "uact")
	if err != nil {
		return nil, err
	}
	if funded.Cmp(transferred) != 0 {
		return nil, errors.New("terminal escrow retained nonzero funds")
	}
	spend, err := consoleTransferredSpend(baseline, transferred)
	if err != nil {
		return nil, err
	}
	if requirePositive && spend.Sign() == 0 {
		return nil, errors.New("leased deployment reported no provider spend")
	}
	return spend, nil
}

func parseConsoleObservedEscrowAmount(raw string) (*big.Rat, error) {
	amount, err := sdkmath.LegacyNewDecFromStr(raw)
	if err != nil {
		return nil, err
	}

	return new(big.Rat).SetFrac(
		amount.BigInt(),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(sdkmath.LegacyPrecision), nil),
	), nil
}

func consoleFundsDeltaForDenom(before, after map[string]*big.Rat, denom string) (*big.Rat, bool) {
	if denom == "" || after[denom] == nil {
		return nil, false
	}
	prior := before[denom]
	if prior == nil {
		prior = new(big.Rat)
	}
	return new(big.Rat).Sub(new(big.Rat).Set(after[denom]), prior), true
}

func consoleActiveLeaseCount(detail consoleDeploymentObservation) int {
	count := 0
	for _, lease := range detail.Leases {
		if strings.EqualFold(lease.State, "active") {
			count++
		}
	}
	return count
}

func waitForConsoleCondition(ctx context.Context, interval time.Duration, probe func() (bool, string, error)) error {
	var last string
	for {
		ok, observation, err := probe()
		if err == nil && ok {
			return nil
		}
		if err != nil {
			last = err.Error()
		} else if observation != "" {
			last = observation
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if last == "" {
				last = "condition was never observed"
			}
			return fmt.Errorf("%s: %w", last, ctx.Err())
		case <-timer.C:
		}
	}
}

func consoleTerminalState(ctx context.Context, observer *consoleAPIObserver, dseq string) (bool, string, error) {
	getObservation := ""
	detail, err := observer.getDeployment(ctx, dseq)
	if err != nil {
		var httpErr *consoleObserverHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			getObservation = "deployment get reports it absent"
		} else {
			return false, "", err
		}
	} else {
		if problem := consoleTerminalDeploymentProblem(detail); problem != "" {
			return false, problem, nil
		}
		getObservation = "deployment get reports closed with no active lease"
	}

	deployments, err := observer.listAllDeployments(ctx)
	if err != nil {
		return false, "", err
	}
	for _, item := range deployments {
		if item.Deployment.ID.DSeq.String() == dseq && !strings.EqualFold(item.Deployment.State, "closed") {
			return false, fmt.Sprintf("deployment list still reports state %q", item.Deployment.State), nil
		}
	}
	return true, getObservation + "; deployment list reports closed or absent", nil
}

func consoleTerminalDeploymentProblem(detail consoleDeploymentObservation) string {
	if !strings.EqualFold(detail.Deployment.State, "closed") {
		return fmt.Sprintf("deployment state is %q", detail.Deployment.State)
	}
	for _, lease := range detail.Leases {
		if strings.EqualFold(lease.State, "active") || lease.State == "" {
			return fmt.Sprintf("lease %s/%d/%d at %s is still %q", lease.ID.DSeq, lease.ID.GSeq, lease.ID.OSeq, lease.ID.Provider, lease.State)
		}
	}
	return ""
}

func waitForConsoleTerminalState(ctx context.Context, observer *consoleAPIObserver, dseq string) error {
	return waitForConsoleCondition(ctx, 2*time.Second, func() (bool, string, error) {
		return consoleTerminalState(ctx, observer, dseq)
	})
}

func consoleObservedAccountNetChangeMicros(beforeTotal, afterTotal int64) (int64, error) {
	if beforeTotal < 0 || afterTotal < 0 {
		return 0, errors.New("Console total balance cannot be negative")
	}
	return afterTotal - beforeTotal, nil
}

func validateConsoleGrossSpendLimit(grossSpendMicros *big.Rat, maxSpendUSD float64) error {
	if grossSpendMicros == nil {
		return errors.New("Console gross provider spend must be observed")
	}
	if grossSpendMicros.Sign() < 0 {
		return errors.New("Console gross provider spend cannot be negative")
	}
	maxSpend := new(big.Rat).SetFloat64(maxSpendUSD)
	if maxSpend == nil || maxSpend.Sign() <= 0 {
		return errors.New("Console maximum spend must be positive")
	}
	maxSpendMicros := new(big.Rat).Mul(maxSpend, big.NewRat(1_000_000, 1))
	if grossSpendMicros.Cmp(maxSpendMicros) > 0 {
		return fmt.Errorf("Console gross provider spend exceeded the $%.6f limit", maxSpendUSD)
	}
	return nil
}

func consoleSpendSummary(grossSpendMicros *big.Rat, accountNetChangeMicros *int64, maxSpendUSD float64) string {
	grossSpendUSD := new(big.Rat).Quo(new(big.Rat).Set(grossSpendMicros), big.NewRat(1_000_000, 1))
	accountChange := "unproved"
	if accountNetChangeMicros != nil {
		accountChange = fmt.Sprintf("%+.6f", float64(*accountNetChangeMicros)/1e6)
	}
	return fmt.Sprintf(
		"Console mutation gross_spend_usd=%s account_net_change_usd=%s max_spend_usd=%.6f",
		grossSpendUSD.FloatString(6),
		accountChange,
		maxSpendUSD,
	)
}

func consoleBoundedDeadline(parent context.Context, desired time.Time) (time.Time, bool) {
	if parent.Err() != nil {
		return time.Time{}, false
	}
	if parentDeadline, ok := parent.Deadline(); ok && parentDeadline.Before(desired) {
		desired = parentDeadline
	}
	if !time.Now().Before(desired) {
		return time.Time{}, false
	}
	return desired, true
}

type consoleResourceTracker struct {
	t               *testing.T
	home            string
	contextName     string
	deadline        time.Time
	observer        *consoleAPIObserver
	baseline        map[string]struct{}
	baselineTotal   int64
	maxSpendUSD     float64
	maxDeployments  int
	hashes          map[string]struct{}
	known           map[string]struct{}
	confirmed       map[string]struct{}
	transferredBase map[string]*big.Rat
	createAttempted bool
}

func newConsoleResourceTracker(
	t *testing.T,
	home string,
	contextName string,
	deadline time.Time,
	observer *consoleAPIObserver,
	baseline []consoleDeploymentObservation,
	baselineTotal int64,
	config consoleMutationConfig,
	hashes ...string,
) *consoleResourceTracker {
	baselineIDs := make(map[string]struct{}, len(baseline))
	for _, item := range baseline {
		baselineIDs[item.Deployment.ID.DSeq.String()] = struct{}{}
	}
	ownedHashes := make(map[string]struct{}, len(hashes))
	for _, hash := range hashes {
		ownedHashes[hash] = struct{}{}
	}
	return &consoleResourceTracker{
		t:               t,
		home:            home,
		contextName:     contextName,
		deadline:        deadline,
		observer:        observer,
		baseline:        baselineIDs,
		baselineTotal:   baselineTotal,
		maxSpendUSD:     config.MaxSpendUSD,
		maxDeployments:  config.MaxDeployments,
		hashes:          ownedHashes,
		known:           make(map[string]struct{}),
		confirmed:       make(map[string]struct{}),
		transferredBase: make(map[string]*big.Rat),
	}
}

func (tracker *consoleResourceTracker) track(dseq string) {
	if dseq != "" {
		tracker.known[dseq] = struct{}{}
	}
}

func (tracker *consoleResourceTracker) confirm(dseq string) {
	if _, known := tracker.known[dseq]; known {
		tracker.confirmed[dseq] = struct{}{}
	}
}

func (tracker *consoleResourceTracker) recordTransferredBaseline(dseq string, amount *big.Rat) {
	if dseq != "" && amount != nil {
		tracker.transferredBase[dseq] = new(big.Rat).Set(amount)
	}
}

func (tracker *consoleResourceTracker) markCreateAttempted() {
	tracker.createAttempted = true
}

func (tracker *consoleResourceTracker) cleanup() {
	tracker.t.Helper()

	now := time.Now()
	if !now.Before(tracker.deadline) {
		tracker.t.Errorf("Console cleanup started after its overall deadline")
		return
	}
	discoveryDeadline, mutationDeadline := consoleCleanupPhaseDeadlines(now, tracker.deadline)
	discoveryCtx, cancelDiscovery := context.WithDeadline(context.Background(), discoveryDeadline)
	defer cancelDiscovery()

	candidates := make(map[string]struct{}, len(tracker.confirmed))
	for dseq := range tracker.confirmed {
		candidates[dseq] = struct{}{}
	}

	discover := func() error {
		deployments, err := tracker.observer.listAllDeployments(discoveryCtx)
		if err != nil {
			return err
		}
		for _, item := range deployments {
			dseq := item.Deployment.ID.DSeq.String()
			if _, existed := tracker.baseline[dseq]; existed {
				continue
			}
			if _, owned := tracker.hashes[item.Deployment.Hash]; owned {
				candidates[dseq] = struct{}{}
			}
		}
		for dseq := range tracker.known {
			if _, confirmed := tracker.confirmed[dseq]; confirmed {
				continue
			}
			if _, existed := tracker.baseline[dseq]; existed {
				continue
			}
			detail, err := tracker.observer.getDeployment(discoveryCtx, dseq)
			if err != nil {
				var httpErr *consoleObserverHTTPError
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					continue
				}
				return err
			}
			if _, owned := tracker.hashes[detail.Deployment.Hash]; !owned {
				return fmt.Errorf("Console create returned dseq %s with an unexpected SDL hash; cleanup will not mutate it", dseq)
			}
			candidates[dseq] = struct{}{}
		}
		return nil
	}

	var discoverySucceeded bool
	var lastDiscoveryErr error
	for {
		if err := discover(); err != nil {
			lastDiscoveryErr = err
		} else {
			discoverySucceeded = true
			lastDiscoveryErr = nil
			if len(candidates) > tracker.maxDeployments || !tracker.createAttempted {
				break
			}
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-discoveryCtx.Done():
			timer.Stop()
		case <-timer.C:
		}
		if discoveryCtx.Err() != nil {
			break
		}
	}
	if !discoverySucceeded {
		tracker.t.Errorf("Console cleanup never completed deployment discovery: %v", lastDiscoveryErr)
	}
	if tracker.createAttempted && len(candidates) == 0 {
		tracker.t.Errorf("Console cleanup could not identify the outcome of the attempted deployment create by its unique SDL hash")
	}

	ids := make([]string, 0, len(candidates))
	for dseq := range candidates {
		ids = append(ids, dseq)
	}
	sort.Strings(ids)
	if len(ids) > tracker.maxDeployments {
		tracker.t.Errorf("Console mutation produced %d owned deployments, above the %d-deployment limit; cleanup will close all of them", len(ids), tracker.maxDeployments)
	}

	mutationCtx, cancelMutations := context.WithDeadline(context.Background(), mutationDeadline)
	defer cancelMutations()
	for _, dseq := range ids {
		if mutationCtx.Err() != nil {
			tracker.t.Errorf("Console cleanup exhausted its mutation phase before processing dseq %s", dseq)
			continue
		}
		terminal, _, terminalErr := consoleTerminalState(mutationCtx, tracker.observer, dseq)
		if terminalErr != nil {
			tracker.t.Errorf("Console cleanup could not establish pre-cleanup state for %s: %v", dseq, terminalErr)
		}
		if !terminal {
			settingsDeadline, ok := consoleBoundedDeadline(mutationCtx, time.Now().Add(8*time.Second))
			if !ok {
				tracker.t.Errorf("Console cleanup had no time left to disable auto-top-up for %s", dseq)
			} else {
				settingsCtx, cancelSettings := context.WithDeadline(mutationCtx, settingsDeadline)
				settingsResult := runConsoleAkt(settingsCtx, tracker.t, tracker.home, "console", "deployment", "settings", dseq, "false")
				cancelSettings()
				if settingsResult.Exit != 0 || settingsResult.Err != nil || settingsResult.CredentialLeak || settingsResult.StdoutTruncated || settingsResult.StderrTruncated {
					tracker.t.Errorf("Console cleanup could not disable auto-top-up for %s (%s)", dseq, consoleCommandDiagnostic(settingsResult))
				} else if verifyDeadline, ok := consoleBoundedDeadline(mutationCtx, time.Now().Add(5*time.Second)); ok {
					verifyCtx, cancelVerify := context.WithDeadline(mutationCtx, verifyDeadline)
					settings, verifyErr := tracker.observer.getDeploymentSettings(verifyCtx, dseq)
					cancelVerify()
					if verifyErr != nil || settings.DSeq.String() != dseq || settings.AutoTopUpEnabled {
						tracker.t.Errorf("Console cleanup could not independently verify disabled auto-top-up for %s", dseq)
					}
				} else {
					tracker.t.Errorf("Console cleanup had no time left to verify disabled auto-top-up for %s", dseq)
				}
			}
		}

		closeDeadline, ok := consoleBoundedDeadline(mutationCtx, time.Now().Add(10*time.Second))
		if !ok {
			tracker.t.Errorf("Console cleanup had no time left to close %s", dseq)
			continue
		}
		closeCtx, cancelClose := context.WithDeadline(mutationCtx, closeDeadline)
		result := runConsoleAkt(closeCtx, tracker.t, tracker.home, "console", "deployment", "close", dseq)
		cancelClose()
		if result.Exit != 0 || result.Err != nil || result.CredentialLeak || result.StdoutTruncated || result.StderrTruncated {
			tracker.t.Errorf("Console cleanup close %s failed (%s)", dseq, consoleCommandDiagnostic(result))
		}
	}

	finalStateDeadline := tracker.deadline.Add(-consoleCleanupBalanceReserve)
	if finalStateDeadline.Before(time.Now()) {
		finalStateDeadline = time.Now()
	}
	grossSpendMicros := new(big.Rat)
	grossSpendProved := len(ids) > 0
	if time.Now().Before(finalStateDeadline) {
		finalStateCtx, cancelFinalState := context.WithDeadline(context.Background(), finalStateDeadline)
		for _, dseq := range ids {
			if err := waitForConsoleTerminalState(finalStateCtx, tracker.observer, dseq); err != nil {
				tracker.t.Errorf("Console cleanup did not verify terminal state for %s: %v", dseq, err)
				grossSpendProved = false
				continue
			}
			detail, err := tracker.observer.getDeployment(finalStateCtx, dseq)
			if err != nil {
				tracker.t.Errorf("Console cleanup could not observe terminal escrow for %s: %v", dseq, err)
				grossSpendProved = false
				continue
			}
			baseline, hasBaseline := tracker.transferredBase[dseq]
			if !hasBaseline {
				baseline = new(big.Rat)
			}
			spend, err := consoleTerminalTransferredSpend(detail, baseline, hasBaseline)
			if err != nil {
				tracker.t.Errorf("Console cleanup could not prove terminal provider spend for %s: %v", dseq, err)
				grossSpendProved = false
				continue
			}
			grossSpendMicros.Add(grossSpendMicros, spend)
		}
		cancelFinalState()
	} else if len(ids) > 0 {
		tracker.t.Errorf("Console cleanup exhausted the terminal-observation phase")
		grossSpendProved = false
	}

	if len(ids) > 0 {
		entries, err := readConsoleActions(tracker.home, tracker.contextName)
		if err != nil {
			tracker.t.Errorf("Console cleanup could not verify action logs: %v", err)
		} else {
			for _, dseq := range ids {
				want, _ := strconv.ParseUint(dseq, 10, 64)
				found := false
				for _, entry := range entries {
					if entry.Action == "close-deployment" && entry.Status == "success" && entry.DSeq == want {
						found = true
						break
					}
				}
				if !found {
					tracker.t.Errorf("Console cleanup found no successful close-deployment action for dseq %s", dseq)
				}
			}
		}
	}

	var accountNetChangeMicros *int64
	if !time.Now().Before(tracker.deadline) {
		tracker.t.Errorf("Console cleanup exhausted the final account-reconciliation phase")
	} else {
		balanceCtx, cancelBalance := context.WithDeadline(context.Background(), tracker.deadline)
		finalBalances, err := tracker.observer.getBalances(balanceCtx)
		cancelBalance()
		if err != nil {
			tracker.t.Errorf("Console cleanup could not observe final account reconciliation: %v", err)
		} else {
			netChange, changeErr := consoleObservedAccountNetChangeMicros(tracker.baselineTotal, finalBalances.Total)
			if changeErr != nil {
				tracker.t.Errorf("Console cleanup could not reconcile the signed account-total change: %v", changeErr)
			} else {
				accountNetChangeMicros = &netChange
			}
		}
	}
	if grossSpendProved {
		if err := validateConsoleGrossSpendLimit(grossSpendMicros, tracker.maxSpendUSD); err != nil {
			tracker.t.Errorf("Console mutation exceeded its spend contract: %v", err)
		}
		tracker.t.Log(consoleSpendSummary(grossSpendMicros, accountNetChangeMicros, tracker.maxSpendUSD))
	}
}
