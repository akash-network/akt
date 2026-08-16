package console

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/tools/remotecommand"
	mtypes "pkg.akt.dev/go/node/market/v1"
	rest "pkg.akt.dev/go/provider/client"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
	aktprovider "pkg.akt.dev/akt/internal/provider"
)

// testProviderAddr is a checksummed akash bech32 address (the sdkutil import
// in gateway.go registers the prefix) used as the lease's provider.
const testProviderAddr = "akash1skjwj5whet0lpe65qaq4rpq03hjxlwd97juzxv"

// jwtCapture records the /v1/create-jwt-token requests a test observed.
type jwtCapture struct {
	Count int
	TTL   int
	Scope []string
}

// newGatewayConsoleServer mocks the Console API endpoints the gateway helper
// hits for dseq 777: deployment detail (with the given leases JSON array),
// provider detail (pointing hostUri at gatewayURL), and JWT minting.
func newGatewayConsoleServer(t *testing.T, gatewayURL, leasesJSON string, jwt *jwtCapture) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/deployments/777":
			writeJSON(t, w, `{"data":{"deployment":{"id":{"owner":"akash1owner","dseq":"777"},"state":"active"},"leases":`+leasesJSON+`}}`)

		case "/v1/providers/" + testProviderAddr:
			writeJSON(t, w, `{"owner":"`+testProviderAddr+`","hostUri":"`+gatewayURL+`","isOnline":true}`)

		case "/v1/create-jwt-token":
			var body struct {
				Data struct {
					TTL    int `json:"ttl"`
					Leases struct {
						Access string   `json:"access"`
						Scope  []string `json:"scope"`
					} `json:"leases"`
				} `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode create-jwt-token request: %v", err)
			}
			jwt.Count++
			jwt.TTL = body.Data.TTL
			jwt.Scope = body.Data.Leases.Scope
			writeJSON(t, w, `{"data":{"token":"tok-live"}}`)

		default:
			t.Errorf("unexpected console request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// activeLeaseJSON is a deployment with one closed and one active lease; the
// active one uses gseq 2 so tests can prove the active lease was selected.
const activeLeaseJSON = `[
  {"id":{"owner":"akash1owner","dseq":"777","gseq":1,"oseq":1,"provider":"` + testProviderAddr + `"},"state":"closed"},
  {"id":{"owner":"akash1owner","dseq":"777","gseq":2,"oseq":1,"provider":"` + testProviderAddr + `"},"state":"active"}
]`

type consoleLeaseShellCapture struct {
	statusCalls int
	shellCalls  int
	id          mtypes.LeaseID
	service     string
	podIndex    uint
	command     []string
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	tty         bool
	resize      <-chan remotecommand.TerminalSize
	shellErr    error
}

func (capture *consoleLeaseShellCapture) LeaseStatus(
	context.Context,
	mtypes.LeaseID,
) (rest.LeaseStatus, error) {
	capture.statusCalls++
	return rest.LeaseStatus{}, nil
}

func (capture *consoleLeaseShellCapture) LeaseShell(
	_ context.Context,
	id mtypes.LeaseID,
	service string,
	podIndex uint,
	command []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
	resize <-chan remotecommand.TerminalSize,
) error {
	capture.shellCalls++
	capture.id = id
	capture.service = service
	capture.podIndex = podIndex
	capture.command = append([]string(nil), command...)
	capture.stdin = stdin
	capture.stdout = stdout
	capture.stderr = stderr
	capture.tty = tty
	capture.resize = resize

	return capture.shellErr
}

func TestRunConsoleLeaseShellForwardsProviderBoundary(t *testing.T) {
	shellErr := errors.New("remote process failed")
	capture := &consoleLeaseShellCapture{shellErr: shellErr}
	id := mtypes.LeaseID{
		Owner:    "akash1owner",
		DSeq:     777,
		GSeq:     2,
		OSeq:     1,
		Provider: testProviderAddr,
	}
	command := []string{"sh", "-c", "printf ready"}
	stdin := strings.NewReader("input")
	var stdout, stderr bytes.Buffer

	err := runConsoleLeaseShell(
		context.Background(), capture, id, "web", command, stdin, &stdout, &stderr, true,
	)
	if !errors.Is(err, shellErr) || !strings.Contains(err.Error(), "open lease shell") {
		t.Fatalf("shell error = %v, want wrapped remote failure", err)
	}
	if capture.statusCalls != 1 || capture.shellCalls != 1 {
		t.Fatalf("status/shell calls = %d/%d, want one preflight and one remote call", capture.statusCalls, capture.shellCalls)
	}
	if capture.id != id || capture.service != "web" || capture.podIndex != 0 ||
		!reflect.DeepEqual(capture.command, command) || capture.stdin != stdin ||
		capture.stdout != &stdout || capture.stderr != &stderr || !capture.tty || capture.resize != nil {
		t.Fatalf("forwarded shell call = %+v, want unchanged console shell arguments", capture)
	}
}

func TestStatusEndToEndViaGateway(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var gwPath, gwAuth string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gwPath = r.URL.Path
		gwAuth = r.Header.Get("Authorization")
		writeJSON(t, w, `{"services":{"web":{"name":"web","available":1,"total":1}},"forwarded_ports":{},"ips":null}`)
	}))
	defer gateway.Close()

	var jwt jwtCapture
	consoleSrv := newGatewayConsoleServer(t, gateway.URL, activeLeaseJSON, &jwt)
	defer consoleSrv.Close()

	out, err := execConsole(t, m, consoleSrv.URL, "status", "777")
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	// The active lease (gseq 2) must be targeted at the mocked hostUri with
	// the Console-minted JWT as Bearer auth.
	if gwPath != "/lease/777/2/1/status" {
		t.Errorf("gateway path = %q, want /lease/777/2/1/status", gwPath)
	}
	if gwAuth != "Bearer tok-live" {
		t.Errorf("gateway Authorization = %q, want Bearer tok-live", gwAuth)
	}
	if jwt.Count != 1 || jwt.TTL != 300 {
		t.Errorf("jwt mints = %d ttl = %d, want 1 mint with ttl 300", jwt.Count, jwt.TTL)
	}
	if len(jwt.Scope) != 1 || jwt.Scope[0] != "status" {
		t.Errorf("jwt scope = %v, want [status]", jwt.Scope)
	}
	if !strings.Contains(out, `"web"`) {
		t.Errorf("status output should include the service snapshot, got %q", out)
	}
}

func TestStatusWatchPollsUntilError(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	// First poll succeeds, the second fails: the watch loop must print the
	// snapshot and surface the poll error.
	calls := 0
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(t, w, `{"services":{"web":{"name":"web","available":1,"total":1}},"forwarded_ports":{},"ips":null}`)
	}))
	defer gateway.Close()

	var jwt jwtCapture
	consoleSrv := newGatewayConsoleServer(t, gateway.URL, activeLeaseJSON, &jwt)
	defer consoleSrv.Close()

	out, err := execConsole(t, m, consoleSrv.URL, "status", "777", "--watch", "--interval", "10ms")
	if err == nil {
		t.Fatal("watch must surface the failing poll")
	}
	if !strings.Contains(err.Error(), "query lease status") {
		t.Errorf("error = %v, want a lease status poll failure", err)
	}
	if calls != 2 {
		t.Errorf("gateway calls = %d, want 2 (snapshot + failing poll)", calls)
	}
	if jwt.TTL != 3600 {
		t.Errorf("watch jwt ttl = %d, want 3600", jwt.TTL)
	}
	if !strings.Contains(out, `"web"`) {
		t.Errorf("watch should print the first snapshot, got %q", out)
	}
}

// TestLogsResolvesGatewayAndScopes covers the resolution path for `logs`: the
// Console lookups happen and a logs-scoped JWT is minted with a TTL matching
// the follow mode. The mocked hostUri does not resolve, so the lease-status
// preflight fails after resolution and proves the command refuses to open a
// websocket until the provider recognizes the lease.
func TestLogsResolvesGatewayAndScopes(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var jwt jwtCapture
	consoleSrv := newGatewayConsoleServer(t, "http://gateway.invalid", activeLeaseJSON, &jwt)
	defer consoleSrv.Close()

	// One-shot: short token.
	_, err := execConsole(t, m, consoleSrv.URL, "logs", "777", "web")
	if err == nil || !strings.Contains(err.Error(), "query lease status") {
		t.Fatalf("logs should fail at the lease-status preflight, got %v", err)
	}
	if jwt.TTL != 300 || strings.Join(jwt.Scope, ",") != "logs,status" {
		t.Errorf("one-shot logs jwt = ttl %d scope %v, want ttl 300 scope [logs status]", jwt.TTL, jwt.Scope)
	}

	// Follow: long-lived token.
	_, err = execConsole(t, m, consoleSrv.URL, "logs", "777", "web", "--follow")
	if err == nil || !strings.Contains(err.Error(), "query lease status") {
		t.Fatalf("logs --follow should fail at the lease-status preflight, got %v", err)
	}
	if jwt.TTL != 3600 {
		t.Errorf("follow logs jwt ttl = %d, want 3600", jwt.TTL)
	}
}

func TestGatewayNoActiveLeaseError(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	var jwt jwtCapture
	leases := `[
	  {"id":{"owner":"akash1owner","dseq":"777","gseq":1,"oseq":1,"provider":"` + testProviderAddr + `"},"state":"closed"},
	  {"id":{"owner":"akash1owner","dseq":"777","gseq":2,"oseq":1,"provider":"` + testProviderAddr + `"},"state":"insufficient_funds"}
	]`
	consoleSrv := newGatewayConsoleServer(t, "http://gateway.invalid", leases, &jwt)
	defer consoleSrv.Close()

	_, err := execConsole(t, m, consoleSrv.URL, "status", "777")
	if err == nil {
		t.Fatal("status without an active lease must fail")
	}
	for _, want := range []string{"no active lease", "closed", "insufficient_funds"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err.Error(), want)
		}
	}
	if jwt.Count != 0 {
		t.Errorf("no JWT should be minted without an active lease, got %d mints", jwt.Count)
	}

	// No leases at all gets a pointer to lease creation.
	consoleSrv2 := newGatewayConsoleServer(t, "http://gateway.invalid", `[]`, &jwt)
	defer consoleSrv2.Close()

	_, err = execConsole(t, m, consoleSrv2.URL, "status", "777")
	if err == nil || !strings.Contains(err.Error(), "has no leases yet") {
		t.Errorf("error = %v, want a no-leases message", err)
	}
}

const screenSDL = `---
version: "2.0"
services:
  web:
    image: nginx
    expose:
      - port: 80
        to:
          - global: true
profiles:
  compute:
    web:
      resources:
        cpu:
          units: "100m"
        memory:
          size: "128Mi"
        storage:
          size: "1Gi"
  placement:
    westcoast:
      pricing:
        web:
          denom: uakt
          amount: 50
deployment:
  web:
    westcoast:
      profile: web
      count: 1
`

func TestScreenRequestShape(t *testing.T) {
	m := newTestManager(t)

	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/bid-screening" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, _ = io.ReadAll(r.Body)
		writeJSON(t, w, `{"providers":[{"owner":"akash1prov","isAudited":true,"organization":"acme"}]}`)
	}))
	defer srv.Close()

	sdlPath := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(sdlPath, []byte(screenSDL), 0o600); err != nil {
		t.Fatalf("write SDL: %v", err)
	}

	// Public endpoint: no API key configured anywhere.
	out, err := execConsole(t, m, srv.URL, "screen", sdlPath)
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if !strings.Contains(out, "akash1prov") || !strings.Contains(out, "acme") {
		t.Errorf("screen output should list the matched provider, got %q", out)
	}

	var req struct {
		Resources []struct {
			Resource struct {
				CPU struct {
					Units struct {
						Val string `json:"val"`
					} `json:"units"`
				} `json:"cpu"`
				Memory struct {
					Quantity struct {
						Val string `json:"val"`
					} `json:"quantity"`
				} `json:"memory"`
				GPU struct {
					Units struct {
						Val string `json:"val"`
					} `json:"units"`
				} `json:"gpu"`
				Storage []struct {
					Name     string `json:"name"`
					Quantity struct {
						Val string `json:"val"`
					} `json:"quantity"`
				} `json:"storage"`
				Endpoints []any `json:"endpoints"`
			} `json:"resource"`
			Count int `json:"count"`
			Price struct {
				Denom  string `json:"denom"`
				Amount string `json:"amount"`
			} `json:"price"`
		} `json:"resources"`
		Timezone string `json:"timezone"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode screening request: %v", err)
	}

	if len(req.Resources) != 1 {
		t.Fatalf("resources len = %d, want 1: %s", len(req.Resources), body)
	}

	r := req.Resources[0]
	if r.Resource.CPU.Units.Val != "100" {
		t.Errorf("cpu units = %q, want 100 (100m)", r.Resource.CPU.Units.Val)
	}
	if r.Resource.Memory.Quantity.Val != "134217728" {
		t.Errorf("memory quantity = %q, want 134217728 (128Mi)", r.Resource.Memory.Quantity.Val)
	}
	if r.Resource.GPU.Units.Val != "0" {
		t.Errorf("gpu units = %q, want 0", r.Resource.GPU.Units.Val)
	}
	if len(r.Resource.Storage) != 1 || r.Resource.Storage[0].Quantity.Val != "1073741824" {
		t.Errorf("storage = %+v, want one 1Gi volume", r.Resource.Storage)
	}
	if r.Resource.Endpoints == nil {
		t.Error("endpoints must be an array, not null")
	}
	if r.Count != 1 {
		t.Errorf("count = %d, want 1", r.Count)
	}
	if r.Price.Denom != "uakt" || r.Price.Amount != "50" {
		t.Errorf("price = %+v, want uakt/50 (integer string)", r.Price)
	}

	// The Console API rejects UTC-family zones; the fallback (or the real
	// local zone) must always be a slash-form IANA name.
	tz := req.Timezone
	if tz == "" || tz == "Local" || !strings.Contains(tz, "/") ||
		strings.HasPrefix(tz, "UTC") || strings.HasPrefix(tz, "Etc/") {
		t.Errorf("timezone = %q, want a non-UTC IANA zone name", tz)
	}
}

func TestStreamCloseErr(t *testing.T) {
	tests := []struct {
		name    string
		reason  string
		follow  bool
		wantErr bool
	}{
		// A clean close carries no reason and always succeeds.
		{"clean close", "", false, false},
		{"clean close following", "", true, false},
		// The regression: a one-shot fetch ends when the provider closes the
		// connection, which surfaces as an EOF after every line was printed.
		{"one-shot eof", "unexpected EOF", false, false},
		{"one-shot plain eof", "EOF", false, false},
		{"one-shot eof mixed case", "Unexpected Eof", false, false},
		// Following asks for an open-ended stream, so an EOF cut it short.
		{"following eof", "unexpected EOF", true, true},
		// Genuine failures are reported either way.
		{"one-shot abnormal", "close 1006 (abnormal closure)", false, true},
		{"following abnormal", "connection reset by peer", true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := aktprovider.StreamCloseError("log", tc.reason, tc.follow)
			if tc.wantErr && err == nil {
				t.Fatalf("reason %q follow=%v: expected an error, got nil", tc.reason, tc.follow)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("reason %q follow=%v: expected nil, got %v", tc.reason, tc.follow, err)
			}
		})
	}
}

func TestPrintStreamRecordFormats(t *testing.T) {
	records := []providerLogMsg{
		{Name: "web-abc-123", Message: "first line"},
		{Name: "worker-def-456", Message: "second line"},
		{Name: "web-blank-789", Message: ""},
	}

	t.Run("pretty", func(t *testing.T) {
		cmd, buf := streamOutputCommand(t, "pretty")
		for _, record := range records {
			if err := output.PrintStreamRecord(cmd, record, fmt.Sprintf("[%s] %s", record.Name, record.Message)); err != nil {
				t.Fatalf("print stream record: %v", err)
			}
		}
		if got, want := buf.String(), "[web-abc-123] first line\n[worker-def-456] second line\n[web-blank-789] \n"; got != want {
			t.Errorf("pretty stream = %q, want %q", got, want)
		}
	})

	t.Run("jsonl", func(t *testing.T) {
		cmd, buf := streamOutputCommand(t, "json")
		for _, record := range records {
			if err := output.PrintStreamRecord(cmd, record, "unused"); err != nil {
				t.Fatalf("print stream record: %v", err)
			}
		}

		lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
		if len(lines) != len(records) {
			t.Fatalf("JSON stream has %d lines, want %d: %q", len(lines), len(records), buf.String())
		}
		for i, line := range lines {
			var got providerLogMsg
			if err := json.Unmarshal([]byte(line), &got); err != nil {
				t.Fatalf("decode JSONL record %d %q: %v", i, line, err)
			}
			if got != records[i] {
				t.Errorf("JSONL record %d = %#v, want %#v", i, got, records[i])
			}
		}
	})

	t.Run("yaml documents", func(t *testing.T) {
		cmd, buf := streamOutputCommand(t, "yaml")
		for _, record := range records {
			if err := output.PrintStreamRecord(cmd, record, "unused"); err != nil {
				t.Fatalf("print stream record: %v", err)
			}
		}

		if got := strings.Count(buf.String(), "---\n"); got != len(records) {
			t.Fatalf("YAML stream has %d document markers, want %d: %q", got, len(records), buf.String())
		}
		decoder := yaml.NewDecoder(strings.NewReader(buf.String()))
		for i, want := range records {
			var got providerLogMsg
			if err := decoder.Decode(&got); err != nil {
				t.Fatalf("decode YAML record %d: %v", i, err)
			}
			if got != want {
				t.Errorf("YAML record %d = %#v, want %#v", i, got, want)
			}
		}
	})
}

func TestRetainTailLogRecords(t *testing.T) {
	var records []providerLogMsg
	for _, record := range []providerLogMsg{
		{Name: "web-a", Message: "first"},
		{Name: "web-b", Message: "second"},
		{Name: "web-c", Message: "third"},
	} {
		records = aktprovider.RetainTail(records, record, 2)
	}

	want := []providerLogMsg{
		{Name: "web-b", Message: "second"},
		{Name: "web-c", Message: "third"},
	}
	if !reflect.DeepEqual(records, want) {
		t.Errorf("retained tail = %#v, want %#v", records, want)
	}
}

func TestPrintLeaseEventStructured(t *testing.T) {
	event := rest.LeaseEvent{
		Type:   "Normal",
		Reason: "Started",
		Note:   "container started",
		Object: rest.LeaseEventObject{Kind: "Pod", Namespace: "lease", Name: "web-abc-123"},
	}

	cmd, buf := streamOutputCommand(t, "pretty")
	if err := printLeaseEvent(cmd, event); err != nil {
		t.Fatalf("print pretty lease event: %v", err)
	}
	if got, want := buf.String(), "Normal [Pod/web-abc-123] Started: container started\n"; got != want {
		t.Errorf("pretty event = %q, want %q", got, want)
	}

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			cmd, buf := streamOutputCommand(t, format)
			if err := printLeaseEvent(cmd, event); err != nil {
				t.Fatalf("print lease event: %v", err)
			}

			got := decodeStructuredMap(t, format, buf.String())
			if got["type"] != "Normal" || got["reason"] != "Started" || got["note"] != "container started" {
				t.Errorf("event output = %#v, want the provider event fields", got)
			}
			object, ok := got["object"].(map[string]any)
			if !ok || object["kind"] != "Pod" || object["name"] != "web-abc-123" {
				t.Errorf("event object = %#v, want Pod/web-abc-123", got["object"])
			}
		})
	}
}

func streamOutputCommand(t *testing.T, format string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cmd := &cobra.Command{}
	cmd.Flags().String(flagdefs.FlagOutput, format, "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	return cmd, &buf
}

func TestShellRejectsStructuredInteractiveModeBeforeContextResolution(t *testing.T) {
	resolved := false
	cmd := shellCmd(func() *aktctx.Manager {
		resolved = true
		return nil
	})
	cmd.Flags().String(flagdefs.FlagOutput, "json", "")
	cmd.SetArgs([]string{"12345", "web"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "explicit remote command") {
		t.Fatalf("shell error = %v", err)
	}
	if resolved {
		t.Fatal("structured interactive shell resolved context before refusal")
	}
}

func TestShellStdinOverrideDefaultsToAutomaticSelection(t *testing.T) {
	cmd := shellCmd(func() *aktctx.Manager { return nil })
	flag := cmd.Flags().Lookup(flagdefs.FlagStdin)
	if flag == nil {
		t.Fatal("console shell has no --stdin override")
		return
	}
	if flag.DefValue != "false" {
		t.Fatalf("--stdin default = %q, want false force-override default", flag.DefValue)
	}
}

func TestShellRecordsProviderActionOutcomeExactlyOnce(t *testing.T) {
	tests := []struct {
		name      string
		runErr    error
		wantState string
	}{
		{name: "success", wantState: "success"},
		{name: "failure", runErr: errors.New("remote command failed"), wantState: "failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newTestManager(t)
			if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
				t.Fatalf("SetConsoleAPIKey: %v", err)
			}

			var jwt jwtCapture
			consoleSrv := newGatewayConsoleServer(t, "https://gateway.example.test", activeLeaseJSON, &jwt)
			defer consoleSrv.Close()

			logPath := filepath.Join(t.TempDir(), "actions.log")
			logger, err := actionlog.Open(logPath)
			if err != nil {
				t.Fatalf("open action log: %v", err)
			}
			t.Cleanup(func() { _ = logger.Close() })

			runnerCalls := 0
			runner := func(
				_ context.Context,
				_ aktprovider.LeaseShellClient,
				id mtypes.LeaseID,
				service string,
				command []string,
				_ io.Reader,
				_ io.Writer,
				_ io.Writer,
				_ bool,
			) error {
				runnerCalls++
				if id.Provider != testProviderAddr || id.DSeq != 777 {
					t.Errorf("shell lease = %+v, want provider %s dseq 777", id, testProviderAddr)
				}
				if service != "web" || strings.Join(command, " ") != "echo ok" {
					t.Errorf("shell service/command = %q/%v, want web/[echo ok]", service, command)
				}
				return test.runErr
			}

			cmd := shellCmdWithRunner(func() *aktctx.Manager { return m }, runner)
			cmd.Flags().VarP(output.NewFormatFlag("pretty"), flagdefs.FlagOutput, "o", "Output format: pretty, json, yaml")
			cmd.Flags().String(flagdefs.FlagContext, "", "Active context name")
			cmd.Flags().String(flagdefs.FlagConsoleAPIURL, consoleSrv.URL, "Console API base URL")
			cmd.Flags().String(flagdefs.FlagConsoleAPIKey, "", "Console API key")
			cmd.SetContext(cliutil.WithActionLog(context.Background(), logger))
			cmd.SetArgs([]string{"777", "web", "--", "echo", "ok"})
			var outputBuffer bytes.Buffer
			cmd.SetOut(&outputBuffer)
			cmd.SetErr(&outputBuffer)

			err = cmd.Execute()
			if !errors.Is(err, test.runErr) {
				t.Fatalf("shell error = %v, want %v", err, test.runErr)
			}
			if runnerCalls != 1 {
				t.Fatalf("shell runner calls = %d, want 1", runnerCalls)
			}

			entries := readRawActionLogJSONL(t, logPath)
			if len(entries) != 1 {
				t.Fatalf("raw action log entries = %d, want exactly 1: %s", len(entries), mustReadFile(t, logPath))
			}
			entry := entries[0]
			if entry["type"] != "provider" || entry["action"] != "lease-shell" || entry["provider"] != testProviderAddr || entry["dseq"] != float64(777) || entry["status"] != test.wantState {
				t.Errorf("raw shell action entry = %#v", entry)
			}
			if test.runErr == nil {
				if _, exists := entry["error"]; exists {
					t.Errorf("successful shell entry has error: %#v", entry)
				}
			} else if entry["error"] != test.runErr.Error() {
				t.Errorf("failed shell error = %#v, want %q", entry["error"], test.runErr)
			}
		})
	}
}

func TestReadOnlyGatewayCommandsDoNotWriteActionLog(t *testing.T) {
	m := newTestManager(t)
	if err := aktctx.SetConsoleAPIKey(m.Root(), "prod", "sekrit"); err != nil {
		t.Fatalf("SetConsoleAPIKey: %v", err)
	}

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lease/777/2/1/status" {
			t.Errorf("unexpected gateway path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, `{"services":{"web":{"name":"web","available":1,"total":1}},"forwarded_ports":{},"ips":null}`)
	}))
	defer gateway.Close()

	var jwt jwtCapture
	consoleSrv := newGatewayConsoleServer(t, gateway.URL, activeLeaseJSON, &jwt)
	defer consoleSrv.Close()

	logPath := filepath.Join(t.TempDir(), "actions.log")
	logger, err := actionlog.Open(logPath)
	if err != nil {
		t.Fatalf("open action log: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	ctx := cliutil.WithActionLog(context.Background(), logger)

	if _, err := execConsoleContext(ctx, t, m, consoleSrv.URL, "status", "777"); err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, command := range []string{"logs", "events"} {
		if _, err := execConsoleContext(ctx, t, m, consoleSrv.URL, command, "777"); err == nil || !strings.Contains(err.Error(), "invalid uri scheme") {
			t.Errorf("%s error = %v, want post-preflight stream scheme error", command, err)
		}
	}

	if raw := mustReadFile(t, logPath); len(bytes.TrimSpace(raw)) != 0 {
		t.Fatalf("read-only gateway commands wrote action log JSONL: %s", raw)
	}
}

func readRawActionLogJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	raw := bytes.TrimSpace(mustReadFile(t, path))
	if len(raw) == 0 {
		return nil
	}

	lines := bytes.Split(raw, []byte{'\n'})
	entries := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode raw action log line %d %q: %v", i, line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func TestMatchesService(t *testing.T) {
	tests := []struct {
		name    string
		pod     string
		service string
		want    bool
	}{
		// No filter requested: everything passes.
		{"empty filter", "web-5cfc6c7b4b-4cl7z", "", true},
		// The provider reports pod names, so the service name has to match
		// through the replicaset/pod suffix Kubernetes appends.
		{"pod of the service", "web-5cfc6c7b4b-4cl7z", "web", true},
		{"pod named exactly", "web", "web", true},
		{"empty runtime suffix", "web-", "web", false},
		// The bug this guards: asking for one service used to return them all.
		{"pod of another service", "cache-666dd889cf-zpbn5", "web", false},
		// A prefix that is not a name boundary must not match, or `web` would
		// silently include `webhook`.
		{"prefix but different service", "webhook-abc-123", "web", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := aktprovider.MatchesService(tc.pod, tc.service); got != tc.want {
				t.Fatalf("MatchesService(%q, %q) = %v, want %v", tc.pod, tc.service, got, tc.want)
			}
		})
	}
}
