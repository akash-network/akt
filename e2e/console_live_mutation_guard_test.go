package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConsoleMutationEndpointGuard(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		hermetic bool
		want     string
		wantErr  string
	}{
		{
			name:     "staging sandbox",
			endpoint: "https://console-api-sandbox-staging.akash.network",
			want:     "https://console-api-sandbox-staging.akash.network",
		},
		{
			name:     "production namespace sandbox",
			endpoint: "https://console-api-sandbox.akash.network",
			want:     "https://console-api-sandbox.akash.network",
		},
		{
			name:     "canonicalizes case default port and root path",
			endpoint: "HTTPS://CONSOLE-API-SANDBOX-STAGING.AKASH.NETWORK:443/",
			want:     "https://console-api-sandbox-staging.akash.network",
		},
		{
			name:     "localhost loopback",
			endpoint: "http://localhost:3080/",
			hermetic: true,
			want:     "http://localhost:3080",
		},
		{
			name:     "IPv4 loopback",
			endpoint: "http://127.0.0.1:3080/",
			hermetic: true,
			want:     "http://127.0.0.1:3080",
		},
		{
			name:     "IPv6 loopback",
			endpoint: "http://[::1]:3080/",
			hermetic: true,
			want:     "http://[::1]:3080",
		},
		{name: "missing", wantErr: "required"},
		{
			name:     "loopback without hermetic capability",
			endpoint: "http://127.0.0.1:3080",
			wantErr:  "explicit hermetic tests",
		},
		{
			name:     "surrounding whitespace",
			endpoint: " https://console-api-sandbox-staging.akash.network/endpoint-canary ",
			wantErr:  "whitespace",
		},
		{
			name:     "malformed URL",
			endpoint: "%endpoint-canary",
			wantErr:  "absolute http(s) URL",
		},
		{
			name:     "production API",
			endpoint: "https://console-api.akash.network",
			wantErr:  "production",
		},
		{
			name:     "first-party sandbox over HTTP",
			endpoint: "http://console-api-sandbox-staging.akash.network",
			wantErr:  "must use HTTPS",
		},
		{
			name:     "sandbox-looking third party",
			endpoint: "https://api.sandbox.example.test",
			wantErr:  "approved Akash sandbox origin",
		},
		{
			name:     "allowlisted host suffix lookalike",
			endpoint: "https://console-api-sandbox-staging.akash.network.evil.test",
			wantErr:  "approved Akash sandbox origin",
		},
		{
			name:     "allowlisted host prefix lookalike",
			endpoint: "https://evil-console-api-sandbox-staging.akash.network",
			wantErr:  "approved Akash sandbox origin",
		},
		{
			name:     "localhost suffix lookalike",
			endpoint: "http://localhost.evil.test",
			wantErr:  "approved Akash sandbox origin",
		},
		{
			name:     "non-loopback HTTP IP",
			endpoint: "http://192.0.2.1",
			wantErr:  "approved Akash sandbox origin",
		},
		{
			name:     "non-default remote port",
			endpoint: "https://console-api-sandbox-staging.akash.network:8443",
			wantErr:  "default HTTPS port",
		},
		{
			name:     "remote base path",
			endpoint: "https://console-api-sandbox-staging.akash.network/endpoint-canary",
			wantErr:  "origin without a base path",
		},
		{
			name:     "loopback base path",
			endpoint: "http://127.0.0.1:3080/endpoint-canary",
			hermetic: true,
			wantErr:  "origin without a base path",
		},
		{
			name:     "terminal DNS dot",
			endpoint: "https://console-api-sandbox-staging.akash.network.",
			wantErr:  "approved Akash sandbox origin",
		},
		{
			name:     "HTTPS loopback",
			endpoint: "https://127.0.0.1:3080",
			hermetic: true,
			wantErr:  "must use HTTP",
		},
		{
			name:     "credentials in URL",
			endpoint: "https://endpoint-canary@console-api-sandbox-staging.akash.network",
			wantErr:  "must not contain credentials",
		},
		{
			name:     "query in URL",
			endpoint: "https://console-api-sandbox-staging.akash.network?key=endpoint-canary",
			wantErr:  "must not contain credentials",
		},
		{
			name:     "fragment in URL",
			endpoint: "https://console-api-sandbox-staging.akash.network#endpoint-canary",
			wantErr:  "must not contain credentials",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateConsoleMutationEndpoint(tc.endpoint, tc.hermetic)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("endpoint case %q did not return error class %q", tc.name, tc.wantErr)
				}
				for _, forbidden := range []string{tc.endpoint, "endpoint-canary"} {
					if forbidden != "" && strings.Contains(err.Error(), forbidden) {
						t.Fatalf("endpoint case %q leaked a forbidden input", tc.name)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("endpoint case %q unexpectedly failed", tc.name)
			}
			if got != tc.want {
				t.Fatalf("endpoint case %q returned the wrong canonical origin", tc.name)
			}
		})
	}
}

func TestConsoleCredentialEndpointGuard(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		key      string
		hermetic bool
		wantErr  string
	}{
		{
			name:     "staging sandbox",
			endpoint: "https://console-api-sandbox-staging.akash.network",
			key:      "ac.sk.staging.fake-secret",
		},
		{
			name:     "production namespace sandbox",
			endpoint: "https://console-api-sandbox.akash.network",
			key:      "ac.sk.production.key-canary",
		},
		{
			name:     "loopback fixture key",
			endpoint: "http://127.0.0.1:3080",
			key:      "opaque-fixture-key",
			hermetic: true,
		},
		{
			name:     "loopback fixture without hermetic capability",
			endpoint: "http://127.0.0.1:3080",
			key:      "opaque-fixture-key",
			wantErr:  "explicit hermetic tests",
		},
		{
			name:     "missing key",
			endpoint: "https://console-api-sandbox-staging.akash.network",
			wantErr:  "required",
		},
		{
			name:     "loopback key whitespace",
			endpoint: "http://127.0.0.1:3080",
			key:      " key-canary ",
			hermetic: true,
			wantErr:  "whitespace",
		},
		{
			name:     "staging key sent to production namespace sandbox",
			endpoint: "https://console-api-sandbox.akash.network",
			key:      "ac.sk.staging.key-canary",
			wantErr:  `requires a "production" service-key environment`,
		},
		{
			name:     "production key sent to staging sandbox",
			endpoint: "https://console-api-sandbox-staging.akash.network",
			key:      "ac.sk.production.key-canary",
			wantErr:  `requires a "staging" service-key environment`,
		},
		{
			name:     "missing secret segment",
			endpoint: "https://console-api-sandbox-staging.akash.network",
			key:      "ac.sk.staging",
			wantErr:  "unrecognized Console service-key format",
		},
		{
			name:     "empty secret segment",
			endpoint: "https://console-api-sandbox-staging.akash.network",
			key:      "ac.sk.staging.",
			wantErr:  "unrecognized Console service-key format",
		},
		{
			name:     "wrong service-key prefix",
			endpoint: "https://console-api-sandbox-staging.akash.network",
			key:      "xx.sk.staging.key-canary",
			wantErr:  "service-key prefix",
		},
		{
			name:     "wrong service-key type",
			endpoint: "https://console-api-sandbox-staging.akash.network",
			key:      "ac.pk.staging.key-canary",
			wantErr:  "service-key type",
		},
		{
			name:     "surrounding whitespace",
			endpoint: "https://console-api-sandbox-staging.akash.network",
			key:      " ac.sk.staging.key-canary ",
			wantErr:  "leading or trailing whitespace",
		},
		{
			name:     "invalid endpoint",
			endpoint: "%endpoint-canary",
			key:      "ac.sk.staging.key-canary",
			wantErr:  "absolute http(s) URL",
		},
		{
			name:     "unapproved endpoint",
			endpoint: "https://endpoint-canary.sandbox.example.test",
			key:      "ac.sk.staging.key-canary",
			wantErr:  "approved Akash sandbox origin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConsoleCredentialEndpoint(tc.endpoint, tc.key, tc.hermetic)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("credential case %q unexpectedly failed", tc.name)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("credential case %q did not return error class %q", tc.name, tc.wantErr)
			}
			for _, forbidden := range []string{tc.endpoint, tc.key, "endpoint-canary", "key-canary"} {
				if forbidden != "" && strings.Contains(err.Error(), forbidden) {
					t.Fatalf("credential case %q leaked a forbidden input", tc.name)
				}
			}
		})
	}
}

func TestConsoleMutationBudgetGuard(t *testing.T) {
	valid := map[string]string{
		envConsoleLiveAPIURL: "https://console-api-sandbox-staging.akash.network",
		envConsoleLiveKey:    "ac.sk.staging.fixture-key",
	}
	config, err := loadConsoleMutationConfig(func(key string) string { return valid[key] })
	if err != nil {
		t.Fatalf("load default mutation config: %v", err)
	}
	if config.MaxSpendUSD != consoleDefaultMaxSpendUSD ||
		config.MaxRequestUSD != consoleDefaultMaxRequestUSD ||
		config.MaxDeployments != consoleDefaultMaxDeployments ||
		config.MaxRuntime != consoleDefaultMaxRuntime {
		t.Fatalf("default mutation config = %+v", config)
	}

	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
	}{
		{"request below lifecycle", envConsoleMutationMaxRequest, "0.99", "cannot cover"},
		{"request over hard ceiling", envConsoleMutationMaxRequest, "5.01", "hard test ceiling"},
		{"non-finite request", envConsoleMutationMaxRequest, "NaN", "finite"},
		{"zero spend", envConsoleMutationMaxSpend, "0", "positive"},
		{"spend below fixed escrow", envConsoleMutationMaxSpend, "0.99", "cannot cover"},
		{"spend over hard ceiling", envConsoleMutationMaxSpend, "1.01", "hard test ceiling"},
		{"non-finite spend", envConsoleMutationMaxSpend, "NaN", "finite"},
		{"zero deployments", envConsoleMutationMaxDeployments, "0", "must allow"},
		{"too many deployments", envConsoleMutationMaxDeployments, "2", "hard test ceiling"},
		{"runtime too short", envConsoleMutationMaxRuntime, "1m", "minimum"},
		{"runtime too long", envConsoleMutationMaxRuntime, "11m", "hard test ceiling"},
		{"invalid runtime", envConsoleMutationMaxRuntime, "soon", "Go duration"},
		{"credential whitespace", envConsoleLiveKey, " ac.sk.staging.key-canary ", "whitespace"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{
				envConsoleLiveAPIURL: "https://console-api-sandbox-staging.akash.network",
				envConsoleLiveKey:    "ac.sk.staging.fixture-key",
			}
			values[tc.key] = tc.value
			_, err := loadConsoleMutationConfig(func(key string) string { return values[key] })
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("mutation config case %q did not return error class %q", tc.name, tc.wantErr)
			}
			if strings.Contains(err.Error(), "key-canary") {
				t.Fatalf("mutation config case %q leaked a credential canary", tc.name)
			}
		})
	}

	custom := map[string]string{
		envConsoleLiveAPIURL:             "https://console-api-sandbox-staging.akash.network",
		envConsoleLiveKey:                "ac.sk.staging.fixture-key",
		envConsoleMutationMaxRequest:     "2.5",
		envConsoleMutationMaxSpend:       "1.0",
		envConsoleMutationMaxDeployments: "1",
		envConsoleMutationMaxRuntime:     "7m",
	}
	config, err = loadConsoleMutationConfig(func(key string) string { return custom[key] })
	if err != nil {
		t.Fatalf("load custom mutation config: %v", err)
	}
	if config.MaxRequestUSD != 2.5 || config.MaxSpendUSD != 1 || config.MaxDeployments != 1 || config.MaxRuntime != 7*time.Minute {
		t.Fatalf("custom mutation config = %+v", config)
	}
}

func TestConsoleMutationBudgetReservationsFailBeforeWrites(t *testing.T) {
	budget := newConsoleMutationBudget(consoleMutationConfig{
		MaxRequestUSD:  consoleLifecycleRequestUSD,
		MaxDeployments: 1,
	})
	if err := budget.reserveDeployment(); err != nil {
		t.Fatal(err)
	}
	if err := budget.reserveDeployment(); err == nil {
		t.Fatal("second deployment reservation exceeded the budget without an error")
	}
	if err := budget.reserveLease(); err != nil {
		t.Fatal(err)
	}
	if err := budget.reserveLease(); err == nil {
		t.Fatal("second lease reservation exceeded the budget without an error")
	}
	if err := budget.reserveRequest(consoleLifecycleRequestUSD); err != nil {
		t.Fatal(err)
	}
	if err := budget.reserveRequest(0.01); err == nil {
		t.Fatal("request above the lifecycle budget was accepted")
	}
	for _, amount := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		fresh := newConsoleMutationBudget(consoleMutationConfig{MaxRequestUSD: 2, MaxDeployments: 1})
		if err := fresh.reserveRequest(amount); err == nil {
			t.Fatalf("invalid request reservation %v was accepted", amount)
		}
	}
}

func TestConsoleCommandDiagnosticRedactsCapturedOutput(t *testing.T) {
	secret := "akt_secret_key"
	payload := "services:\n  private-workload: true"
	result := consoleCommandResult{
		Stdout:          payload,
		Stderr:          "server echoed " + secret,
		StdoutBytes:     int64(len(payload)),
		StderrBytes:     int64(len("server echoed " + secret)),
		StdoutTruncated: true,
		Exit:            1,
	}

	diagnostic := consoleCommandDiagnostic(result)
	for _, forbidden := range []string{secret, payload, "private-workload"} {
		if strings.Contains(diagnostic, forbidden) {
			t.Fatalf("diagnostic leaked %q: %s", forbidden, diagnostic)
		}
	}
	for _, required := range []string{
		"exit=1",
		fmt.Sprintf("stdout_bytes=%d", len(payload)),
		fmt.Sprintf("stderr_bytes=%d", len("server echoed "+secret)),
		"stdout_truncated=true",
	} {
		if !strings.Contains(diagnostic, required) {
			t.Fatalf("diagnostic %q does not contain %q", diagnostic, required)
		}
	}
}

func TestConsoleCommandDiagnosticClassifiesSafeHTTPStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stderr string
		want   string
	}{
		{name: "sentinel HTTP status", stderr: "get user: console: invalid or expired API key (HTTP 401)", want: "error_class=console_http_401"},
		{name: "generic HTTP status", stderr: "console: unexpected status 422: private body", want: "error_class=console_http_422"},
		{name: "outer status wins over body marker", stderr: "console: unexpected status 422: upstream body mentioned (HTTP 401)", want: "error_class=console_http_422"},
		{name: "nearby number is not an HTTP status", stderr: "private status 4010", want: "error_class=process_error"},
		{name: "unknown process error", stderr: "private transport failure", want: "error_class=process_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostic := consoleCommandDiagnostic(consoleCommandResult{
				Stderr:      tc.stderr,
				StderrBytes: int64(len(tc.stderr)),
				Exit:        1,
				Err:         errors.New("exit status 1"),
			})
			if !strings.Contains(diagnostic, tc.want) {
				t.Fatalf("diagnostic %q does not contain %q", diagnostic, tc.want)
			}
			if strings.Contains(diagnostic, tc.stderr) {
				t.Fatalf("diagnostic leaked stderr %q", tc.stderr)
			}
		})
	}
}

func TestDecodeConsoleJSONDocumentRequiresOneDocument(t *testing.T) {
	var target struct {
		DSeq string `json:"dseq"`
	}
	if err := decodeConsoleJSONDocument([]byte(`{"dseq":"7","additive":true}`), &target); err != nil || target.DSeq != "7" {
		t.Fatalf("decodeConsoleJSONDocument valid additive payload = %+v, %v", target, err)
	}
	payload := `{"dseq":"7"}{"dseq":"8"}`
	if err := decodeConsoleJSONDocument([]byte(payload), &target); err == nil {
		t.Fatalf("decodeConsoleJSONDocument accepted multiple values: %s", payload)
	}
}

func TestDecodeConsoleJSONStreamValidatesEveryRecord(t *testing.T) {
	var dseqs []string
	count, err := decodeConsoleJSONStream([]byte("{\"dseq\":\"7\"}\n{\"dseq\":\"8\"}\n"), func(raw json.RawMessage) error {
		var record struct {
			DSeq string `json:"dseq"`
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			return err
		}
		if record.DSeq == "" {
			return errors.New("missing dseq")
		}
		dseqs = append(dseqs, record.DSeq)
		return nil
	})
	if err != nil || count != 2 || !slices.Equal(dseqs, []string{"7", "8"}) {
		t.Fatalf("decodeConsoleJSONStream valid stream = count %d, dseqs %v, error %v", count, dseqs, err)
	}

	for _, payload := range []string{"null\n", "{\"dseq\":\"7\"}\n{"} {
		if _, err := decodeConsoleJSONStream([]byte(payload), func(json.RawMessage) error { return nil }); err == nil {
			t.Fatalf("decodeConsoleJSONStream accepted invalid stream %q", payload)
		}
	}

	_, err = decodeConsoleJSONStream([]byte("{\"dseq\":\"7\"}\n{\"dseq\":\"8\"}\n"), func(raw json.RawMessage) error {
		var record struct {
			DSeq string `json:"dseq"`
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			return err
		}
		if record.DSeq == "8" {
			return errors.New("rejected dseq")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "record 2") {
		t.Fatalf("decodeConsoleJSONStream visitor error = %v, want record index", err)
	}
}

func TestValidateConsoleLogRecord(t *testing.T) {
	for _, tc := range []struct {
		name        string
		payload     string
		wantContent bool
		wantErr     string
		forbidden   string
	}{
		{
			name:        "exact service",
			payload:     `{"name":"web","message":"ready"}`,
			wantContent: true,
		},
		{
			name:        "provider runtime pod",
			payload:     `{"name":"web-5bfc685996-wv9vs","message":"ready"}`,
			wantContent: true,
		},
		{
			name:        "stateful runtime pod",
			payload:     `{"name":"web-0","message":"ready"}`,
			wantContent: true,
		},
		{
			name:    "unbounded prefix",
			payload: `{"name":"webhook-5bfc685996-wv9vs","message":"ready"}`,
			wantErr: "log source does not match requested service",
		},
		{
			name:    "empty pod suffix",
			payload: `{"name":"web-","message":"ready"}`,
			wantErr: "log source does not match requested service",
		},
		{
			name:    "another service",
			payload: `{"name":"worker-5bfc685996-wv9vs","message":"ready"}`,
			wantErr: "log source does not match requested service",
		},
		{
			name:      "provider controlled source is not repeated",
			payload:   `{"name":"Bearer provider-jwt\nspoofed","message":"ready"}`,
			wantErr:   "log source does not match requested service",
			forbidden: "provider-jwt",
		},
		{
			name:    "empty message",
			payload: `{"name":"web-5bfc685996-wv9vs","message":" "}`,
		},
		{
			name:    "missing message",
			payload: `{"name":"web-5bfc685996-wv9vs"}`,
			wantErr: "log message is missing or null",
		},
		{
			name:    "null message",
			payload: `{"name":"web-5bfc685996-wv9vs","message":null}`,
			wantErr: "log message is missing or null",
		},
		{
			name:    "non-string message",
			payload: `{"name":"web-5bfc685996-wv9vs","message":7}`,
			wantErr: "cannot unmarshal number",
		},
		{
			name:    "malformed record",
			payload: `{"name":`,
			wantErr: "unexpected end of JSON input",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hasContent, err := validateConsoleLogRecord(json.RawMessage(tc.payload), "web")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("valid log record failed: %v", err)
				}
				if hasContent != tc.wantContent {
					t.Fatalf("log record substantive = %t, want %t", hasContent, tc.wantContent)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("log record error = %v, want %q", err, tc.wantErr)
			}
			if tc.forbidden != "" && strings.Contains(err.Error(), tc.forbidden) {
				t.Fatalf("log record error repeated provider-controlled data: %q", err)
			}
		})
	}
}

func TestValidateConsoleLogStreamRequiresAggregateContent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		payload   string
		wantCount int
		wantErr   string
	}{
		{
			name:      "content and blank line",
			payload:   "{\"name\":\"web-a\",\"message\":\"ready\"}\n{\"name\":\"web-a\",\"message\":\"\"}\n",
			wantCount: 2,
		},
		{
			name:      "only blank lines",
			payload:   "{\"name\":\"web-a\",\"message\":\"\"}\n{\"name\":\"web-b\",\"message\":\" \\n\"}\n",
			wantCount: 2,
			wantErr:   "no substantive messages",
		},
		{
			name:    "empty stream",
			wantErr: "no records",
		},
		{
			name:      "late wrong service",
			payload:   "{\"name\":\"web-a\",\"message\":\"ready\"}\n{\"name\":\"worker-a\",\"message\":\"ready\"}\n",
			wantCount: 1,
			wantErr:   "record 2: log source does not match requested service",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count, err := validateConsoleLogStream([]byte(tc.payload), "web")
			if count != tc.wantCount {
				t.Fatalf("log stream count = %d, want %d", count, tc.wantCount)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("valid log stream failed: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("log stream error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestConsoleAktArgsKeepOutputFlagBeforeRemoteCommandSeparator(t *testing.T) {
	got := consoleAktArgs("/tmp/akt-home", "console", "shell", "7", "web", "--", "/bin/sh", "-c", "printf ok")
	want := []string{"--home", "/tmp/akt-home", "--output", "json", "console", "shell", "7", "web", "--", "/bin/sh", "-c", "printf ok"}
	if !slices.Equal(got, want) {
		t.Fatalf("consoleAktArgs() = %q, want %q", got, want)
	}
}

func TestConsoleSubprocessEnvironmentStripsHarnessSecrets(t *testing.T) {
	input := []string{
		"PATH=/usr/bin",
		envConsoleLiveKey + "=parent-secret",
		envConsoleLiveAPIURL + "=https://endpoint-canary.example.test",
		envConsoleMutationOptIn + "=" + consoleMutationOptInValue,
		envConsoleMutationMaxSpend + "=1.00",
		consoleRuntimeAPIKeyEnv + "=active-runtime-secret",
		"AKT_HOME=/tmp/akt-home",
	}
	got := consoleSubprocessEnvironment(input)
	want := []string{
		"PATH=/usr/bin",
		consoleRuntimeAPIKeyEnv + "=active-runtime-secret",
		"AKT_HOME=/tmp/akt-home",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("filtered subprocess environment = %q, want only runtime inputs", got)
	}
	for _, entry := range got {
		if strings.HasPrefix(entry, "AKT_E2E_CONSOLE_") {
			t.Fatalf("filtered subprocess environment retained a harness control")
		}
	}
}

func TestProbeConsoleWorkloadIngressRequiresBoundedSuccessfulBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("workload-ready"))
		case "/empty":
			w.WriteHeader(http.StatusOK)
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", consoleCommandCaptureLimit+1)))
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	if err := probeConsoleWorkloadIngress(t.Context(), server.URL+"/ok"); err != nil {
		t.Fatalf("probeConsoleWorkloadIngress success = %v", err)
	}
	for _, endpoint := range []string{
		server.URL + "/empty",
		server.URL + "/large",
		server.URL + "/redirect",
		"ftp://example.test/workload",
		"https://user:secret@example.test/workload",
		" ",
	} {
		if err := probeConsoleWorkloadIngress(t.Context(), endpoint); err == nil {
			t.Fatalf("probeConsoleWorkloadIngress(%q) succeeded", endpoint)
		}
	}
}

func TestConsoleAPIKeyObservationsRejectSecretsAndCompareStableIdentity(t *testing.T) {
	baseline := []consoleAPIKeyObservation{{
		ID:         "key-1",
		Name:       "baseline",
		ExpiresAt:  "2027-01-01T00:00:00Z",
		CreatedAt:  "2026-01-01T00:00:00Z",
		LastUsedAt: "2026-01-02T00:00:00Z",
		KeyFormat:  "v1",
	}}
	if err := validateConsoleAPIKeys(baseline); err != nil {
		t.Fatalf("validateConsoleAPIKeys valid baseline = %v", err)
	}
	volatileChange := append([]consoleAPIKeyObservation(nil), baseline...)
	volatileChange[0].LastUsedAt = "2026-08-12T00:00:00Z"
	if !sameConsoleAPIKeyRecords(baseline, volatileChange) {
		t.Fatal("sameConsoleAPIKeyRecords treated last-use telemetry as identity")
	}
	stableChange := append([]consoleAPIKeyObservation(nil), baseline...)
	stableChange[0].Name = "renamed"
	if sameConsoleAPIKeyRecords(baseline, stableChange) {
		t.Fatal("sameConsoleAPIKeyRecords ignored a stable name change")
	}

	child := consoleAPIKeyObservation{ID: "key-2", Name: "child"}
	withChild := append(append([]consoleAPIKeyObservation(nil), baseline...), child)
	if found, ok := findConsoleAPIKey(withChild, child.ID); !ok || found.Name != child.Name {
		t.Fatalf("findConsoleAPIKey() = %+v, %t", found, ok)
	}
	if got := consoleAPIKeysWithoutID(withChild, child.ID); !sameConsoleAPIKeyRecords(got, baseline) {
		t.Fatalf("consoleAPIKeysWithoutID() = %+v, want baseline", got)
	}

	for _, keys := range [][]consoleAPIKeyObservation{
		{{ID: "", Name: "blank-id"}},
		{{ID: "key-1", Name: ""}},
		{{ID: "key-1", Name: "one"}, {ID: "key-1", Name: "duplicate"}},
		{{ID: "key-1", Name: "leaked", Secret: json.RawMessage(`"one-time-secret"`)}},
	} {
		if err := validateConsoleAPIKeys(keys); err == nil {
			t.Fatalf("validateConsoleAPIKeys accepted invalid collection %+v", keys)
		}
	}
}

func TestConsoleLiveJSONContractValidatesNestedKindsAndCardinality(t *testing.T) {
	nonEmptyString := consoleLiveJSONContract{shape: consoleLiveString, nonEmpty: true}
	contract := consoleLiveJSONContract{
		shape:         consoleLiveObject,
		minProperties: 4,
		fields: []consoleLiveFieldContract{
			{name: "name", contract: nonEmptyString},
			{name: "count", contract: consoleLiveJSONContract{shape: consoleLiveNumber}},
			{name: "enabled", contract: consoleLiveJSONContract{shape: consoleLiveBool}},
			{name: "items", contract: consoleLiveJSONContract{
				shape:    consoleLiveArray,
				minItems: 1,
				item: &consoleLiveJSONContract{shape: consoleLiveObject, fields: []consoleLiveFieldContract{
					{name: "id", contract: nonEmptyString},
				}},
			}},
		},
	}

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "valid", body: `{"name":"catalog","count":1,"enabled":false,"items":[{"id":"item-1"}]}`},
		{name: "missing field", body: `{"name":"catalog","count":1,"enabled":false,"items":[{}]}`, wantErr: `omitted required field "id"`},
		{name: "wrong scalar kind", body: `{"name":"catalog","count":"1","enabled":false,"items":[{"id":"item-1"}]}`, wantErr: "want a JSON number"},
		{name: "null field", body: `{"name":null,"count":1,"enabled":false,"items":[{"id":"item-1"}]}`, wantErr: "empty or null"},
		{name: "empty identity", body: `{"name":" ","count":1,"enabled":false,"items":[{"id":"item-1"}]}`, wantErr: "empty string"},
		{name: "empty collection", body: `{"name":"catalog","count":0,"enabled":true,"items":[]}`, wantErr: "want at least 1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConsoleLiveJSONContract("response", json.RawMessage(tc.body), contract)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("contract validation error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateConsoleLiveProviderRegions(t *testing.T) {
	readySchema := map[string]string{
		"us-west": "United States West",
		"us-east": "United States East",
	}
	tests := []struct {
		name              string
		body              string
		expected          map[string]string
		requireMembership bool
		wantErr           string
	}{
		{
			name:              "mutation sandbox is ready with canonical membership",
			body:              fmt.Sprintf(`[{"key":"us-west","description":"United States West","providers":[]},{"key":"us-east","description":"United States East","providers":[%q]}]`, testMnemonicAddr),
			expected:          readySchema,
			requireMembership: true,
		},
		{
			name:     "duplicate region key",
			body:     `[{"key":"us-west","description":"United States West","providers":[]},{"key":"us-west","description":"United States West","providers":[]}]`,
			expected: readySchema,
			wantErr:  `repeats key "us-west"`,
		},
		{
			name:     "read-only sandbox may have no provider memberships",
			body:     `[{"key":"us-west","description":"United States West","providers":[]}]`,
			expected: map[string]string{"us-west": "United States West"},
		},
		{
			name:     "description must match independent schema",
			body:     `[{"key":"us-west","description":"wrong","providers":[]}]`,
			expected: map[string]string{"us-west": "United States West"},
			wantErr:  "differs from the attributes schema",
		},
		{
			name:     "unknown region is rejected",
			body:     `[{"key":"moon","description":"Moon","providers":[]}]`,
			expected: map[string]string{"us-west": "United States West"},
			wantErr:  "absent from the attributes schema",
		},
		{
			name:     "schema region omission is rejected",
			body:     `[{"key":"us-west","description":"United States West","providers":[]}]`,
			expected: readySchema,
			wantErr:  "attributes schema has 2",
		},
		{
			name:     "invalid provider address",
			body:     `[{"key":"us-west","description":"United States West","providers":["akash1provider"]}]`,
			expected: map[string]string{"us-west": "United States West"},
			wantErr:  "invalid account address",
		},
		{
			name:     "duplicate provider within region",
			body:     fmt.Sprintf(`[{"key":"us-west","description":"United States West","providers":[%q,%q]}]`, testMnemonicAddr, testMnemonicAddr),
			expected: map[string]string{"us-west": "United States West"},
			wantErr:  "repeats provider",
		},
		{
			name:              "mutation sandbox has no provider memberships",
			body:              `[{"key":"us-west","description":"United States West","providers":[]}]`,
			expected:          map[string]string{"us-west": "United States West"},
			requireMembership: true,
			wantErr:           "not deployment-ready",
		},
		{
			name:     "invalid JSON",
			body:     `[`,
			expected: readySchema,
			wantErr:  "decode provider-region catalog",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConsoleLiveProviderRegions(json.RawMessage(tc.body), tc.expected, tc.requireMembership)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("provider-region validation error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadConsoleLiveProviderRegionSchema(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "valid public schema",
			body: `{"location-region":{"key":"location-region","type":"option","values":[{"key":"us-west","description":"United States West"}]}}`,
		},
		{
			name:    "wrong attribute definition",
			body:    `{"location-region":{"key":"region","type":"string","values":[{"key":"us-west","description":"United States West"}]}}`,
			wantErr: "invalid location-region definition",
		},
		{
			name:    "empty values",
			body:    `{"location-region":{"key":"location-region","type":"option","values":[]}}`,
			wantErr: "no location-region values",
		},
		{
			name:    "duplicate key",
			body:    `{"location-region":{"key":"location-region","type":"option","values":[{"key":"us-west","description":"West"},{"key":"us-west","description":"West again"}]}}`,
			wantErr: "repeats location-region key",
		},
		{
			name:    "malformed document",
			body:    `{`,
			wantErr: "decode Console observer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/provider-attributes-schema" {
					http.NotFound(w, r)
					return
				}
				if r.Header.Get("x-api-key") != "" {
					t.Error("public provider schema request carried an API key")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)

			regions, err := loadConsoleLiveProviderRegionSchema(t.Context(), server.URL)
			if tc.wantErr == "" {
				if err != nil || regions["us-west"] != "United States West" || len(regions) != 1 {
					t.Fatalf("valid provider schema was not preserved: regions=%v error=%v", regions, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("provider schema error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateConsoleLiveTemplateCatalog(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "populated category", body: `[{"title":"AI","templates":[{"id":"tpl-1"}]}]`},
		{name: "empty category is schema-valid when aggregate is populated", body: `[{"title":"Empty","templates":[]},{"title":"Compute","templates":[{"id":"tpl-1"}]}]`},
		{name: "empty aggregate", body: `[]`, wantErr: "no templates"},
		{name: "all categories empty", body: `[{"title":"AI","templates":[]}]`, wantErr: "no templates"},
		{name: "duplicate category", body: `[{"title":"AI","templates":[{"id":"tpl-1"}]},{"title":"AI","templates":[{"id":"tpl-2"}]}]`, wantErr: "repeats category title"},
		{name: "duplicate template within category", body: `[{"title":"AI","templates":[{"id":"tpl-1"},{"id":"tpl-1"}]}]`, wantErr: "repeats template ID"},
		{name: "same template may be classified in multiple categories", body: `[{"title":"AI","templates":[{"id":"tpl-1"}]},{"title":"Compute","templates":[{"id":"tpl-1"}]}]`},
		{name: "empty category title", body: `[{"title":" ","templates":[{"id":"tpl-1"}]}]`, wantErr: "empty category title"},
		{name: "empty template ID", body: `[{"title":"AI","templates":[{"id":" "}]}]`, wantErr: "empty template ID"},
		{name: "invalid JSON", body: `[`, wantErr: "decode template catalog"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConsoleLiveTemplateCatalog(json.RawMessage(tc.body))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("template-catalog validation error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestConsoleProviderStatusContractRejectsZeroValuedSuccess(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "valid",
			body: `{
				"services":{"web":{"name":"web","available":1,"total":1,"uris":[],"observed_generation":1,"replicas":1,"updated_replicas":1,"ready_replicas":1,"available_replicas":1}},
				"forwarded_ports":{},
				"ips":null
			}`,
		},
		{name: "sdk zero value", body: `{"services":null,"forwarded_ports":null,"ips":null}`, wantErr: "services"},
		{name: "no service", body: `{"services":{},"forwarded_ports":{},"ips":{}}`, wantErr: "at least 1"},
		{
			name:    "missing service identity",
			body:    `{"services":{"web":{"name":"","available":1,"total":1,"uris":[],"observed_generation":1,"replicas":1,"updated_replicas":1,"ready_replicas":1,"available_replicas":1}},"forwarded_ports":{},"ips":{}}`,
			wantErr: "empty string",
		},
		{
			name:    "wrong replica kind",
			body:    `{"services":{"web":{"name":"web","available":"1","total":1,"uris":[],"observed_generation":1,"replicas":1,"updated_replicas":1,"ready_replicas":1,"available_replicas":1}},"forwarded_ports":{},"ips":{}}`,
			wantErr: "JSON number",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConsoleLiveJSONContract("status response", json.RawMessage(tc.body), consoleProviderStatusContract())
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("status contract error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestConsoleCappedBufferScansBeyondCaptureAndAcrossWrites(t *testing.T) {
	buffer := consoleCappedBuffer{limit: 4, needle: []byte("secret")}
	for _, chunk := range []string{"captured-prefix-se", "cr", "et-after-limit"} {
		if _, err := buffer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if !buffer.containsNeedle {
		t.Fatal("streaming credential scan missed a secret split across writes after the capture limit")
	}
	if !buffer.truncated || buffer.buffer.Len() != 4 {
		t.Fatalf("bounded capture = len %d truncated %t, want len 4 and truncated", buffer.buffer.Len(), buffer.truncated)
	}
}

func TestConsoleCleanupPhaseDeadlinesReserveFinalObservation(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	overall := now.Add(consoleCleanupReserve)
	discovery, mutations := consoleCleanupPhaseDeadlines(now, overall)

	if discovery.After(mutations) {
		t.Fatalf("discovery deadline %s is after mutation deadline %s", discovery, mutations)
	}
	if got := overall.Sub(mutations); got != consoleCleanupObservationReserve {
		t.Fatalf("final observation reserve = %s, want %s", got, consoleCleanupObservationReserve)
	}
	if got := discovery.Sub(now); got != consoleCleanupDiscoveryBudget {
		t.Fatalf("discovery budget = %s, want %s", got, consoleCleanupDiscoveryBudget)
	}
	if got := mutations.Sub(discovery); got != consoleCleanupMutationReserve {
		t.Fatalf("post-discovery mutation reserve = %s, want %s", got, consoleCleanupMutationReserve)
	}
	if consoleCleanupBalanceReserve >= consoleCleanupObservationReserve {
		t.Fatalf("terminal accounting must retain a positive reserve before the %s account-reconciliation reserve", consoleCleanupBalanceReserve)
	}
}

func TestConsoleCleanupPhaseDeadlinesClampWithoutStealingFinalObservation(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	overall := now.Add(consoleCleanupObservationReserve + consoleCleanupMutationReserve + 5*time.Second)
	discovery, mutations := consoleCleanupPhaseDeadlines(now, overall)

	if discovery != now.Add(5*time.Second) {
		t.Fatalf("short-reserve discovery deadline = %s, want %s", discovery, now.Add(5*time.Second))
	}
	if got := mutations.Sub(discovery); got != consoleCleanupMutationReserve {
		t.Fatalf("short-reserve mutation allowance = %s, want %s", got, consoleCleanupMutationReserve)
	}
	if got := overall.Sub(mutations); got != consoleCleanupObservationReserve {
		t.Fatalf("short-reserve final observation = %s, want %s", got, consoleCleanupObservationReserve)
	}
}

func TestConsoleCleanupRuntimeLimitDecision(t *testing.T) {
	tests := []struct {
		name      string
		present   bool
		hours     int
		wantPatch bool
		wantErr   bool
	}{
		{name: "absent limit", wantPatch: true},
		{name: "already bounded", present: true, hours: consoleLifecycleRuntimeLimitHours},
		{name: "cannot lower", present: true, hours: consoleLifecycleRuntimeLimitHours + 1, wantErr: true},
		{name: "invalid limit", present: true, hours: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := consoleSettingsObservation{}
			if tt.present {
				settings.RuntimeLimitHours = &tt.hours
			}

			gotPatch, err := consoleCleanupNeedsRuntimeLimitPatch(settings)
			if gotPatch != tt.wantPatch || (err != nil) != tt.wantErr {
				t.Fatalf("cleanup decision = patch %t, error %v; want patch %t, error %t", gotPatch, err, tt.wantPatch, tt.wantErr)
			}
		})
	}
}

func TestConsoleAPIObserverReadsStateWithoutLeakingBodies(t *testing.T) {
	const apiKey = "akt_observer_secret"
	var authenticatedRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != apiKey {
			t.Errorf("x-api-key = %q, want observer credential", r.Header.Get("x-api-key"))
		} else {
			authenticatedRequests.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/balances":
			_, _ = fmt.Fprint(w, `{"data":{"balance":2000000,"deployments":1000000,"total":3000000}}`)
		case "/v1/deployments":
			if r.URL.Query().Get("skip") != "0" || r.URL.Query().Get("limit") != "100" {
				t.Errorf("deployment list query = %q, want skip=0&limit=100", r.URL.RawQuery)
			}
			_, _ = fmt.Fprint(w, `{"data":{"deployments":[{"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":"active","hash":"hash-a"},"leases":[],"escrow_account":{"state":{"funds":[]}}}],"pagination":{"total":1,"skip":0,"limit":100,"hasMore":false}}}`)
		case "/v1/deployments/7":
			_, _ = fmt.Fprint(w, `{"data":{"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":"active","hash":"hash-a"},"leases":[],"escrow_account":{"state":{"funds":{"denom":"uact","amount":"500000"}}}}}`)
		case "/v1/bids":
			if r.URL.Query().Get("dseq") != "7" {
				t.Errorf("bid list query = %q, want dseq=7", r.URL.RawQuery)
			}
			_, _ = fmt.Fprint(w, `{"data":[{"bid":{"id":{"owner":"akash1owner","dseq":"7","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"open","price":{"denom":"uact","amount":"1"}}}]}`)
		case "/v2/deployment-settings/7":
			_, _ = fmt.Fprint(w, `{"data":{"dseq":"7","autoTopUpEnabled":true,"runtimeLimitHours":1,"runtimeEndsAt":null}}`)
		case "/v1/deployments/500":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"error":"server echoed akt_observer_secret and a private manifest"}`)
		default:
			t.Errorf("unexpected observer request %s", r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	observer := newConsoleAPIObserver(server.URL, apiKey)
	ctx := context.Background()

	balances, err := observer.getBalances(ctx)
	if err != nil || balances.TotalUSD() != 3 || balances.AvailableUSD() != 2 {
		t.Fatalf("getBalances() = %+v, %v", balances, err)
	}
	deployments, err := observer.listAllDeployments(ctx)
	if err != nil || len(deployments) != 1 || deployments[0].Deployment.ID.DSeq.String() != "7" {
		t.Fatalf("listAllDeployments() = %+v, %v", deployments, err)
	}
	detail, err := observer.getDeployment(ctx, "7")
	if err != nil || detail.Deployment.Hash != "hash-a" || len(detail.EscrowAccount.State.Funds) != 1 {
		t.Fatalf("getDeployment() = %+v, %v", detail, err)
	}
	bids, err := observer.listBids(ctx, "7")
	if err != nil || len(bids) != 1 || bids[0].ID.Provider != "akash1provider" {
		t.Fatalf("listBids() = %+v, %v", bids, err)
	}
	settings, err := observer.getDeploymentSettings(ctx, "7")
	if err != nil || settings.DSeq.String() != "7" || settings.RuntimeLimitHours == nil || *settings.RuntimeLimitHours != 1 {
		t.Fatalf("getDeploymentSettings() = %+v, %v", settings, err)
	}
	if _, err := observer.getDeployment(ctx, "500"); err == nil {
		t.Fatal("observer accepted an HTTP 500")
	} else if strings.Contains(err.Error(), apiKey) || strings.Contains(err.Error(), "private manifest") {
		t.Fatalf("observer error leaked response body: %v", err)
	}
	if got := authenticatedRequests.Load(); got != 6 {
		t.Fatalf("authenticated observer requests = %d, want 6", got)
	}
}

func TestConsoleAPIObserverRejectsMalformedSuccessBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
		call func(*consoleAPIObserver) error
	}{
		{
			name: "null data",
			body: `{"data":null}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.getBalances(context.Background())
				return err
			},
		},
		{
			name: "missing balance field",
			body: `{"data":{"balance":1,"deployments":1}}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.getBalances(context.Background())
				return err
			},
		},
		{
			name: "missing pagination total",
			body: `{"data":{"deployments":[],"pagination":{"skip":0,"limit":100,"hasMore":false}}}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.listAllDeployments(context.Background())
				return err
			},
		},
		{
			name: "missing pagination skip",
			body: `{"data":{"deployments":[],"pagination":{"total":0,"limit":100,"hasMore":false}}}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.listAllDeployments(context.Background())
				return err
			},
		},
		{
			name: "missing pagination has more",
			body: `{"data":{"deployments":[],"pagination":{"total":0,"skip":0,"limit":100}}}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.listAllDeployments(context.Background())
				return err
			},
		},
		{
			name: "deployment list item missing hash",
			body: `{"data":{"deployments":[{"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":"active"},"leases":[]}],"pagination":{"total":1,"skip":0,"limit":100,"hasMore":false}}}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.listAllDeployments(context.Background())
				return err
			},
		},
		{
			// runtimeLimitHours is legitimately null, so dseq is the only
			// field a settings response must carry.
			name: "settings missing dseq",
			body: `{"data":{"runtimeLimitHours":1}}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.getDeploymentSettings(context.Background(), "7")
				return err
			},
		},
		{
			name: "deployment detail missing leases",
			body: `{"data":{"deployment":{"id":{"dseq":"7"},"state":"closed"}}}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.getDeployment(context.Background(), "7")
				return err
			},
		},
		{
			name: "deployment detail missing escrow funds",
			body: `{"data":{"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":"active","hash":"hash-a"},"leases":[],"escrow_account":{"state":{}}}}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.getDeployment(context.Background(), "7")
				return err
			},
		},
		{
			name: "deployment detail repeats lease identity",
			body: `{"data":{"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":"active","hash":"hash-a"},"leases":[{"id":{"owner":"akash1owner","dseq":"7","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active","price":{"denom":"uact","amount":"1"}},{"id":{"owner":"akash1owner","dseq":"7","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active","price":{"denom":"uact","amount":"1"}}],"escrow_account":{"state":{"funds":[]}}}}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.getDeployment(context.Background(), "7")
				return err
			},
		},
		{
			name: "bid missing price",
			body: `{"data":[{"bid":{"id":{"owner":"akash1owner","dseq":"7","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"open"}}]}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.listBids(context.Background(), "7")
				return err
			},
		},
		{
			name: "duplicate bid identity",
			body: `{"data":[{"bid":{"id":{"owner":"akash1owner","dseq":"7","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"open","price":{"denom":"uact","amount":"1"}}},{"bid":{"id":{"owner":"akash1owner","dseq":"7","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"open","price":{"denom":"uact","amount":"1"}}}]}`,
			call: func(observer *consoleAPIObserver) error {
				_, err := observer.listBids(context.Background(), "7")
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(server.Close)

			if err := tc.call(newConsoleAPIObserver(server.URL, "test-key")); err == nil {
				t.Fatal("observer accepted a malformed success response")
			}
		})
	}
}

func TestConsoleAPIObserverPaginatesDeploymentsToExhaustion(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/v1/deployments" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("deployment list request = %s, want /v1/deployments with limit=100", r.URL.String())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch skip := r.URL.Query().Get("skip"); skip {
		case "0":
			requests.Add(1)
			_, _ = fmt.Fprint(w, `{"data":{"deployments":[{"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":"active","hash":"hash-a"},"leases":[]}],"pagination":{"total":2,"skip":0,"limit":100,"hasMore":true}}}`)
		case "1":
			requests.Add(1)
			_, _ = fmt.Fprint(w, `{"data":{"deployments":[{"deployment":{"id":{"owner":"akash1owner","dseq":"8"},"state":"closed","hash":"hash-b"},"leases":[]}],"pagination":{"total":2,"skip":1,"limit":100,"hasMore":false}}}`)
		default:
			t.Errorf("unexpected deployment offset %q", skip)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	deployments, err := newConsoleAPIObserver(server.URL, "test-key").listAllDeployments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 || len(deployments) != 2 ||
		deployments[0].Deployment.ID.DSeq.String() != "7" ||
		deployments[1].Deployment.ID.DSeq.String() != "8" {
		t.Fatal("deployment pagination did not return both exact identities")
	}
}

func TestConsoleAPIObserverRejectsDuplicateDeploymentAcrossPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		skip := r.URL.Query().Get("skip")
		hasMore := skip == "0"
		_, _ = fmt.Fprintf(w, `{"data":{"deployments":[{"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":"active","hash":"hash-a"},"leases":[]}],"pagination":{"total":2,"skip":%s,"limit":100,"hasMore":%t}}}`, skip, hasMore)
	}))
	t.Cleanup(server.Close)

	if _, err := newConsoleAPIObserver(server.URL, "test-key").listAllDeployments(context.Background()); err == nil || !strings.Contains(err.Error(), "duplicate dseq 7") {
		t.Fatalf("duplicate paginated deployment error = %v", err)
	}
}

func TestConsoleAPIObserverRejectsChangingDeploymentTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		skip := r.URL.Query().Get("skip")
		total := 3
		hasMore := true
		dseq := "7"
		if skip == "1" {
			total = 2
			hasMore = false
			dseq = "8"
		}
		_, _ = fmt.Fprintf(w, `{"data":{"deployments":[{"deployment":{"id":{"owner":"akash1owner","dseq":%q},"state":"active","hash":"hash-a"},"leases":[]}],"pagination":{"total":%d,"skip":%s,"limit":100,"hasMore":%t}}}`, dseq, total, skip, hasMore)
	}))
	t.Cleanup(server.Close)

	if _, err := newConsoleAPIObserver(server.URL, "test-key").listAllDeployments(context.Background()); err == nil || !strings.Contains(err.Error(), "total changed") {
		t.Fatalf("changing paginated deployment total error = %v", err)
	}
}

func TestConsoleFundsDeltaForDenom(t *testing.T) {
	prior, ok := new(big.Rat).SetString("0.100000000000000001")
	if !ok {
		t.Fatal("construct exact prior balance")
	}
	next, ok := new(big.Rat).SetString("500000.100000000000000002")
	if !ok {
		t.Fatal("construct exact next balance")
	}
	wantDelta, ok := new(big.Rat).SetString("500000.000000000000000001")
	if !ok {
		t.Fatal("construct exact delta")
	}
	wantPrior := new(big.Rat).Set(prior)
	wantNext := new(big.Rat).Set(next)
	before := map[string]*big.Rat{"uact": prior}
	after := map[string]*big.Rat{"uact": next}
	delta, ok := consoleFundsDeltaForDenom(before, after, "uact")
	if !ok || delta.Cmp(wantDelta) != 0 {
		t.Fatalf("consoleFundsDeltaForDenom() = %v, %t, want %v, true", delta, ok, wantDelta)
	}
	if before["uact"].Cmp(wantPrior) != 0 || after["uact"].Cmp(wantNext) != 0 {
		t.Fatal("consoleFundsDeltaForDenom mutated an input amount")
	}
	if _, ok := consoleFundsDeltaForDenom(before, after, "uusdc"); ok {
		t.Fatal("consoleFundsDeltaForDenom accepted an absent denomination")
	}
}

func TestConsoleEscrowFundsAcceptsFixedScaleAmounts(t *testing.T) {
	var detail consoleDeploymentObservation
	if err := json.Unmarshal([]byte(`{
		"escrow_account": {"state": {"funds": [
			{"denom":"uact","amount":"-0.100000000000000001"},
			{"denom":"uact","amount":"500000.100000000000000002"}
		]}}
	}`), &detail); err != nil {
		t.Fatalf("decode fixed-scale escrow fixture: %v", err)
	}

	funds, err := consoleEscrowFunds(detail)
	if err != nil {
		t.Fatalf("fixed-scale escrow amount failed: %v", err)
	}
	want, ok := new(big.Rat).SetString("500000.000000000000000001")
	if !ok {
		t.Fatal("construct exact observed-funds expectation")
	}
	if funds["uact"] == nil || funds["uact"].Cmp(want) != 0 {
		t.Fatalf("uact funds = %v, want %v", funds["uact"], want)
	}
}

func TestConsoleEscrowFundsRejectsInvalidAmounts(t *testing.T) {
	for _, amount := range []string{
		"not-a-decimal",
		"0.0000000000000000001",
		"1e6",
		"1/2",
	} {
		t.Run(amount, func(t *testing.T) {
			var detail consoleDeploymentObservation
			detail.escrowFundsPresent = true
			detail.EscrowAccount.State.Funds = consoleCoinObservations{{
				Denom:  "uact",
				Amount: consoleFlexibleID(amount),
			}}
			if _, err := consoleEscrowFunds(detail); err == nil || !strings.Contains(err.Error(), "not a valid fixed-point decimal") {
				t.Fatalf("consoleEscrowFunds(%q) error = %v", amount, err)
			}
		})
	}
}

func TestConsoleEscrowTransferredAcceptsExactNonNegativeAmounts(t *testing.T) {
	var detail consoleDeploymentObservation
	if err := json.Unmarshal([]byte(`{
		"escrow_account": {"state": {"transferred": [
			{"denom":"uact","amount":"0.100000000000000001"},
			{"denom":"uact","amount":"24.899999999999999999"}
		]}}
	}`), &detail); err != nil {
		t.Fatalf("decode fixed-scale transferred fixture: %v", err)
	}

	transferred, err := consoleEscrowTransferred(detail)
	if err != nil {
		t.Fatalf("fixed-scale transferred amount failed: %v", err)
	}
	if got := transferred["uact"]; got == nil || got.Cmp(big.NewRat(25, 1)) != 0 {
		t.Fatalf("uact transferred = %v, want 25", got)
	}
}

func TestConsoleEscrowTransferredRejectsUnprovedAmounts(t *testing.T) {
	tests := []struct {
		name   string
		amount string
		denom  string
		want   string
	}{
		{name: "negative", amount: "-0.000000000000000001", denom: "uact", want: "must be non-negative"},
		{name: "malformed", amount: "not-a-decimal", denom: "uact", want: "not a valid fixed-point decimal"},
		{name: "blank denomination", amount: "1", want: "omitted its denomination"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var detail consoleDeploymentObservation
			detail.escrowTransferredPresent = true
			detail.EscrowAccount.State.Transferred = consoleCoinObservations{{
				Denom:  tc.denom,
				Amount: consoleFlexibleID(tc.amount),
			}}
			if _, err := consoleEscrowTransferred(detail); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("consoleEscrowTransferred() error = %v, want %q", err, tc.want)
			}
		})
	}

	if _, err := consoleEscrowTransferred(consoleDeploymentObservation{}); err == nil || !strings.Contains(err.Error(), "omitted escrow transferred") {
		t.Fatalf("missing transferred error = %v", err)
	}
}

func TestConsoleEscrowFundingAndSpendAreExactAndDoNotMutateInputs(t *testing.T) {
	var detail consoleDeploymentObservation
	if err := json.Unmarshal([]byte(`{
		"escrow_account": {"state": {
			"funds": [{"denom":"uact","amount":"975.000000000000000001"}],
			"transferred": [{"denom":"uact","amount":"24.999999999999999999"}]
		}}
	}`), &detail); err != nil {
		t.Fatalf("decode escrow accounting fixture: %v", err)
	}

	funded, transferred, err := consoleEscrowAccountingForDenom(detail, "uact")
	if err != nil {
		t.Fatalf("consoleEscrowAccountingForDenom() error = %v", err)
	}
	wantTransferred, ok := new(big.Rat).SetString("24.999999999999999999")
	if !ok {
		t.Fatal("construct exact transferred expectation")
	}
	if funded.Cmp(big.NewRat(1000, 1)) != 0 || transferred.Cmp(wantTransferred) != 0 {
		t.Fatalf("funded = %v, transferred = %v; want 1000, %v", funded, transferred, wantTransferred)
	}

	baseline := new(big.Rat).Set(transferred)
	final := new(big.Rat).Add(new(big.Rat).Set(transferred), big.NewRat(5, 1))
	wantBaseline := new(big.Rat).Set(baseline)
	wantFinal := new(big.Rat).Set(final)
	spend, err := consoleTransferredSpend(baseline, final)
	if err != nil || spend.Cmp(big.NewRat(5, 1)) != 0 {
		t.Fatalf("consoleTransferredSpend() = %v, %v; want 5, nil", spend, err)
	}
	if baseline.Cmp(wantBaseline) != 0 || final.Cmp(wantFinal) != 0 {
		t.Fatal("consoleTransferredSpend mutated an input amount")
	}
	if _, err := consoleTransferredSpend(final, baseline); err == nil || !strings.Contains(err.Error(), "regressed") {
		t.Fatalf("regressing transferred error = %v", err)
	}

	var wrongDenom consoleDeploymentObservation
	if err := json.Unmarshal([]byte(`{
		"escrow_account": {"state": {
			"funds": [{"denom":"uact","amount":"1000"}],
			"transferred": [{"denom":"uakt","amount":"1"}]
		}}
	}`), &wrongDenom); err != nil {
		t.Fatalf("decode wrong-denomination fixture: %v", err)
	}
	if _, _, err := consoleEscrowAccountingForDenom(wrongDenom, "uact"); err == nil || !strings.Contains(err.Error(), "unexpected denomination") {
		t.Fatalf("wrong-denomination accounting error = %v", err)
	}
}

func TestConsoleBidSelectionUsesCorroboratedLowestBudgetSafePrice(t *testing.T) {
	makeBid := func(provider, denom, amount string) consoleBidObservation {
		return consoleBidObservation{
			ID: consoleLeaseID{
				Owner: "akash1owner", DSeq: "7", GSeq: 1, OSeq: 1, Provider: provider,
			},
			State: "open",
			Price: &consolePriceObservation{Denom: denom, Amount: consoleFlexibleID(amount)},
		}
	}

	expensive := makeBid("akash1aaa", "uact", "200000")
	cheap := makeBid("akash1zzz", "uact", "50000")
	uncorroborated := makeBid("akash1missing", "uact", "1")
	wrongDenom := makeBid("akash1denom", "uakt", "1")

	selected, projected, ok := selectConsoleBudgetSafeBid(
		[]consoleBidObservation{expensive, cheap, uncorroborated, wrongDenom},
		[]consoleBidObservation{cheap, expensive, wrongDenom},
		"7",
		1,
		10*time.Second,
	)
	if !ok || selected.ID.Provider != cheap.ID.Provider || projected.Cmp(big.NewRat(1, 2)) != 0 {
		t.Fatalf("selected bid = %+v, projected = %v, ok = %t; want cheapest corroborated $0.50 bid", selected, projected, ok)
	}

	if _, _, ok := selectConsoleBudgetSafeBid(
		[]consoleBidObservation{expensive},
		[]consoleBidObservation{expensive},
		"7",
		1,
		10*time.Second,
	); ok {
		t.Fatal("selected a bid whose full-runtime projection exceeds the spend ceiling")
	}
}

func TestProjectedConsoleBidSpendRoundsRuntimeUp(t *testing.T) {
	price := &consolePriceObservation{Denom: "uact", Amount: "500000"}
	got, err := projectedConsoleBidSpendUSD(price, 1500*time.Millisecond)
	if err != nil || got.Cmp(big.NewRat(1, 1)) != 0 {
		t.Fatalf("projectedConsoleBidSpendUSD() = %v, %v; want $1.00", got, err)
	}
	for _, invalid := range []*consolePriceObservation{
		nil,
		{Denom: "uakt", Amount: "1"},
		{Denom: "uact", Amount: "0"},
	} {
		if _, err := projectedConsoleBidSpendUSD(invalid, time.Second); err == nil {
			t.Fatalf("projectedConsoleBidSpendUSD(%+v) accepted an unsafe price", invalid)
		}
	}
}

func TestConsoleDeploymentComparisonUsesCompleteStableRecords(t *testing.T) {
	decode := func(body string) consoleDeploymentObservation {
		t.Helper()
		var observation consoleDeploymentObservation
		if err := json.Unmarshal([]byte(body), &observation); err != nil {
			t.Fatal(err)
		}
		return observation
	}

	first := decode(`{"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":"active","hash":"hash-a"},"leases":[{"id":{"owner":"akash1owner","dseq":"7","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active","price":{"denom":"uact","amount":"1"}}]}`)
	second := decode(`{"deployment":{"id":{"owner":"akash1owner","dseq":"8"},"state":"closed","hash":"hash-b"},"leases":[]}`)
	if !sameConsoleDeploymentRecords([]consoleDeploymentObservation{first, second}, []consoleDeploymentObservation{second, first}) {
		t.Fatal("complete deployment comparison depended on list order")
	}

	changed := first
	changed.Deployment.Hash = "hash-c"
	if sameConsoleDeploymentRecords([]consoleDeploymentObservation{first}, []consoleDeploymentObservation{changed}) {
		t.Fatal("complete deployment comparison ignored an SDL hash mismatch")
	}
	changed = decode(`{"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":"active","hash":"hash-a"},"leases":[{"id":{"owner":"akash1owner","dseq":"7","gseq":1,"oseq":1,"provider":"akash1provider"},"state":"active","price":{"denom":"uact","amount":"1"}}]}`)
	changed.Leases[0].Price.Amount = consoleFlexibleID("2")
	if sameConsoleDeploymentRecords([]consoleDeploymentObservation{first}, []consoleDeploymentObservation{changed}) {
		t.Fatal("complete deployment comparison ignored a lease price mismatch")
	}
}

func TestConsoleObservedAccountNetChangeAcceptsCredits(t *testing.T) {
	tests := []struct {
		name    string
		before  int64
		after   int64
		want    int64
		wantErr bool
	}{
		{name: "unchanged", before: 2_000_000, after: 2_000_000},
		{name: "net debit", before: 2_000_000, after: 1_875_000, want: -125_000},
		{name: "point-in-time account adjustment appears as credit", before: 2_000_000, after: 2_000_028, want: 28},
		{name: "negative boundary", before: -1, after: 0, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := consoleObservedAccountNetChangeMicros(tc.before, tc.after)
			if (err != nil) != tc.wantErr {
				t.Fatalf("consoleObservedAccountNetChangeMicros(%d, %d) error = %v, wantErr %t", tc.before, tc.after, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("consoleObservedAccountNetChangeMicros(%d, %d) = %d, want %d", tc.before, tc.after, got, tc.want)
			}
		})
	}
}

func TestConsoleGrossSpendLimitUsesExactTransferredAmount(t *testing.T) {
	fractionalOverLimit := new(big.Rat).Add(
		big.NewRat(1_000_000, 1),
		big.NewRat(1, 1_000_000_000_000_000_000),
	)
	tests := []struct {
		name        string
		grossMicros *big.Rat
		wantErr     string
	}{
		{name: "bounded spend", grossMicros: big.NewRat(25, 1)},
		{name: "exact limit", grossMicros: big.NewRat(1_000_000, 1)},
		{name: "one attomicro over limit", grossMicros: fractionalOverLimit, wantErr: "gross provider spend"},
		{name: "whole micro over limit", grossMicros: big.NewRat(1_000_001, 1), wantErr: "gross provider spend"},
		{name: "negative gross is invalid", grossMicros: big.NewRat(-1, 1), wantErr: "cannot be negative"},
		{name: "missing gross is invalid", wantErr: "must be observed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConsoleGrossSpendLimit(tc.grossMicros, 1)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("validateConsoleGrossSpendLimit() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("validateConsoleGrossSpendLimit() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestConsoleSpendSummaryKeepsAccountReconciliationSecondary(t *testing.T) {
	credit := int64(28)
	got := consoleSpendSummary(big.NewRat(25, 1), &credit, 1)
	if !strings.Contains(got, "gross_spend_usd=0.000025") ||
		!strings.Contains(got, "account_net_change_usd=+0.000028") {
		t.Fatalf("credit summary = %q", got)
	}

	unproved := consoleSpendSummary(big.NewRat(25, 1), nil, 1)
	if !strings.Contains(unproved, "account_net_change_usd=unproved") || strings.Contains(unproved, "+0.000000") {
		t.Fatalf("unproved summary = %q", unproved)
	}
}

func TestConsoleTerminalDeploymentValidationUsesTheAccountingResponse(t *testing.T) {
	detail := consoleDeploymentObservation{}
	detail.Deployment.State = "closed"
	detail.Leases = []consoleLeaseObservation{{
		ID:    consoleLeaseID{DSeq: "7", GSeq: 1, OSeq: 1, Provider: "akash1provider"},
		State: "closed",
	}}
	if problem := consoleTerminalDeploymentProblem(detail); problem != "" {
		t.Fatalf("closed accounting response rejected: %s", problem)
	}

	detail.Deployment.State = "active"
	if problem := consoleTerminalDeploymentProblem(detail); !strings.Contains(problem, "deployment state") {
		t.Fatalf("regressed accounting response problem = %q", problem)
	}
	detail.Deployment.State = "closed"
	detail.Leases[0].State = "active"
	if problem := consoleTerminalDeploymentProblem(detail); !strings.Contains(problem, "still") {
		t.Fatalf("active-lease accounting response problem = %q", problem)
	}
	detail.Leases[0].State = ""
	if problem := consoleTerminalDeploymentProblem(detail); !strings.Contains(problem, "still") {
		t.Fatalf("blank-lease accounting response problem = %q", problem)
	}
}

func TestConsoleTerminalTransferredSpendFailsClosed(t *testing.T) {
	decode := func(state, funds, transferred string) consoleDeploymentObservation {
		t.Helper()
		var detail consoleDeploymentObservation
		body := fmt.Sprintf(`{
			"deployment":{"id":{"owner":"akash1owner","dseq":"7"},"state":%q,"hash":"hash"},
			"leases":[],
			"escrow_account":{"state":{"funds":%s,"transferred":%s}}
		}`, state, funds, transferred)
		if err := json.Unmarshal([]byte(body), &detail); err != nil {
			t.Fatalf("decode terminal accounting fixture: %v", err)
		}
		return detail
	}

	baseline := new(big.Rat)
	valid := decode("closed", `[{"denom":"uact","amount":"0.000000000000000000"}]`, `[{"denom":"uact","amount":"25.000000000000000000"}]`)
	spend, err := consoleTerminalTransferredSpend(valid, baseline, true)
	if err != nil || spend.Cmp(big.NewRat(25, 1)) != 0 {
		t.Fatalf("terminal spend = %v, %v; want 25, nil", spend, err)
	}

	tests := []struct {
		name            string
		detail          consoleDeploymentObservation
		requirePositive bool
		want            string
	}{
		{name: "nonterminal response", detail: decode("active", `[{"denom":"uact","amount":"0"}]`, `[{"denom":"uact","amount":"25"}]`), want: "not terminal"},
		{name: "unsettled funds", detail: decode("closed", `[{"denom":"uact","amount":"1"}]`, `[{"denom":"uact","amount":"25"}]`), want: "retained nonzero funds"},
		{name: "overdrawn funds", detail: decode("closed", `[{"denom":"uact","amount":"-0.000000000000000001"}]`, `[{"denom":"uact","amount":"25"}]`), want: "retained nonzero funds"},
		{name: "missing transferred field", detail: decode("closed", `[]`, `null`), want: "omitted escrow transferred"},
		{name: "paid lifecycle without transfer", detail: decode("closed", `[]`, `[]`), requirePositive: true, want: "no provider spend"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := consoleTerminalTransferredSpend(tc.detail, baseline, tc.requirePositive); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("consoleTerminalTransferredSpend() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestConsoleActionLogObserverReadsFileWithoutLeakingPayloads(t *testing.T) {
	const contextName = "sandbox"
	const secret = "akt_action_secret"
	home := t.TempDir()
	dir := filepath.Join(home, "contexts", contextName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`{"type":"context","action":"create","status":"success"}`,
		`{"type":"console","action":"create-deployment","status":"success","dseq":7,"params":{"manifest":"` + secret + `"}}`,
		`{"type":"provider","action":"lease-shell","status":"success","provider":"akash1provider","dseq":7}`,
		`{"type":"console","action":"close-deployment","status":"failed","dseq":7,"error":"server echoed ` + secret + `"}`,
		`{"type":"console","action":"create-api-key","status":"success"}`,
		`{"type":"console","action":"delete-api-key","status":"success"}`,
		`{"type":"console","action":"delete-api-key","status":"success"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "actions.log"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := readConsoleActions(home, contextName)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 || entries[3].Action != "close-deployment" || entries[4].Action != "create-deployment" {
		t.Fatalf("direct Console action observation returned the wrong filtered order")
	}
	for _, entry := range entries {
		summary := consoleActionSummary(entry)
		if strings.Contains(summary, secret) || strings.Contains(summary, "manifest") || strings.Contains(summary, "server echoed") {
			t.Fatalf("safe action summary leaked an action payload: %s", summary)
		}
	}

	providerEntries, err := readProviderActions(home, contextName)
	if err != nil {
		t.Fatal(err)
	}
	if len(providerEntries) != 1 || providerEntries[0].Action != "lease-shell" || providerEntries[0].Provider != "akash1provider" || providerEntries[0].DSeq != 7 {
		t.Fatalf("provider action observation = %+v, want the exact shell action", providerEntries)
	}
	assertConsoleActionTail(t, home, contextName, 2, "create-api-key", "delete-api-key", "delete-api-key")
}

func TestParseConsoleDollarAmountRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []string{"$NaN", "$Inf", "$-Inf", "$-0.01", "1.00"} {
		if _, err := parseConsoleDollarAmount(value); err == nil {
			t.Fatalf("parseConsoleDollarAmount(%q) succeeded", value)
		}
	}
	if got, err := parseConsoleDollarAmount("$1.25"); err != nil || got != 1.25 {
		t.Fatalf("parseConsoleDollarAmount($1.25) = %f, %v", got, err)
	}
}

func TestConsoleAPIObserverRejectsRedirectBeforeSecondRequest(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetRequests.Add(1)
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(source.Close)

	_, err := newConsoleAPIObserver(source.URL, "akt_secret").getBalances(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "redirect") {
		t.Fatalf("getBalances() error = %v, want redirect rejection", err)
	}
	if got := targetRequests.Load(); got != 0 {
		t.Fatalf("redirect target received %d requests, want none", got)
	}
}

func TestConsoleAPIObserverRedactsTransportErrors(t *testing.T) {
	const secret = "akt_transport_secret"
	observer := newConsoleAPIObserver("http://127.0.0.1:1/"+secret, "test-key")
	_, err := observer.getBalances(context.Background())
	if err == nil {
		t.Fatal("observer unexpectedly connected to a closed loopback port")
	}
	if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "transport_error") {
		t.Fatalf("observer transport diagnostic was not safely classified: %v", err)
	}
}
