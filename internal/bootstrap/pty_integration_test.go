//go:build !windows

package bootstrap

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"

	aktctx "pkg.akt.dev/akt/internal/context"
)

// TestRunThroughPTY exercises the actual first-run terminal state machine. The
// pure renderer tests cannot prove that raw-mode reads, canonical yes/no reads,
// secret input, and restoration compose in the order a human experiences.
func TestRunThroughPTY(t *testing.T) {
	cfgRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	installDefaultTransport(t, map[string]stubResponse{
		"/repos/akash-network/net/contents": {
			status: http.StatusOK,
			body: `[
				{"name":"mainnet","type":"dir"},
				{"name":"sandbox","type":"dir"},
				{"name":"README.md","type":"file"}
			]`,
		},
		"/mainnet/meta.json": {
			status: http.StatusOK,
			body:   networkMetaJSON("akashnet-2", "https://rpc.mainnet.invalid"),
		},
		"/sandbox/meta.json": {
			status: http.StatusOK,
			body:   networkMetaJSON("sandbox-1", "https://rpc.sandbox.invalid"),
		},
		"/v1/user/me": {
			status: http.StatusOK,
			body:   `{"data":{"id":"user-1","username":"pty-user"}}`,
		},
	})

	master, terminal, err := pty.Open()
	if err != nil {
		t.Fatalf("open pseudo-terminal: %v", err)
	}
	defer master.Close()

	originalStdin, originalStdout, originalStderr := os.Stdin, os.Stdout, os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("open stdout capture: %v", err)
	}
	os.Stdin, os.Stdout, os.Stderr = terminal, stdoutW, terminal
	t.Cleanup(func() {
		os.Stdin, os.Stdout, os.Stderr = originalStdin, originalStdout, originalStderr
		_ = terminal.Close()
		_ = stdoutW.Close()
		_ = stdoutR.Close()
	})

	done := make(chan error, 1)
	go func() { done <- Run(cfgRoot) }()

	transcript := newPTYTranscript(master)
	transcript.expectAndWrite(t, "Select networks", "\r")
	transcript.expectAndWrite(t, "Select keyring backend", "\r")
	transcript.expectAndWrite(t, "Select the active context", "\r")
	transcript.expectAndWrite(t, "Configure a Console API key", "y\n")
	transcript.expectAndWrite(t, "Console API key:", "")
	// The prompt is emitted immediately before ReadPassword disables terminal
	// echo. Let that terminal transition complete before simulating a paste;
	// otherwise the test can inject bytes into canonical mode in the tiny gap.
	time.Sleep(10 * time.Millisecond)
	transcript.write(t, "pty-secret-key\n")
	transcript.expectAndWrite(t, "Authenticated as pty-user", "")
	transcript.expectAndWrite(t, "Route deployments through Console", "y\n")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v\nterminal transcript:\n%s", err, transcript.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("first-run wizard did not finish\nterminal transcript:\n%s", transcript.String())
	}

	if err := terminal.Close(); err != nil {
		t.Fatalf("close terminal: %v", err)
	}
	if err := stdoutW.Close(); err != nil {
		t.Fatalf("close stdout capture: %v", err)
	}
	stdout, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if len(stdout) != 0 {
		t.Fatalf("wizard wrote data to stdout: %q", stdout)
	}

	cfg, err := aktctx.LoadConfig(cfgRoot)
	if err != nil {
		t.Fatalf("load generated config: %v", err)
	}
	if cfg.CurrentContext != "sandbox" {
		t.Fatalf("current context = %q, want safety-default sandbox", cfg.CurrentContext)
	}
	if len(cfg.Networks) != 2 || len(cfg.Contexts) != 2 || len(cfg.Keyrings) != 1 {
		t.Fatalf("generated config cardinality: networks=%d contexts=%d keyrings=%d", len(cfg.Networks), len(cfg.Contexts), len(cfg.Keyrings))
	}
	for _, context := range cfg.Contexts {
		if context.Name == "sandbox" && context.AuthMethod != aktctx.AuthMethodConsoleAPI {
			t.Fatalf("sandbox auth method = %q, want console-api", context.AuthMethod)
		}
	}
	key, err := aktctx.StoredConsoleAPIKey(cfgRoot, "sandbox")
	if err != nil {
		t.Fatalf("read stored Console key: %v", err)
	}
	if key != "pty-secret-key" {
		t.Fatalf("stored Console key = %q", key)
	}
	info, err := os.Stat(aktctx.ConsoleAPIKeyPath(cfgRoot, "sandbox"))
	if err != nil {
		t.Fatalf("stat stored Console key: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("Console key mode = %o, want 600", got)
	}
	if strings.Contains(transcript.String(), "pty-secret-key") {
		t.Fatal("secret input was echoed to the terminal transcript")
	}
}

// TestMultiSelectThroughPTY drives every supported movement form and both
// select-all and individual toggles. The resulting slice, not the rendered
// cursor, is the semantic assertion.
func TestMultiSelectThroughPTY(t *testing.T) {
	master, _, restore := attachBootstrapPTY(t)
	defer master.Close()
	defer restore()

	available := networks("mainnet", "sandbox", "testnet")
	done := make(chan []aktctx.Network, 1)
	go func() { done <- multiSelect(available) }()

	transcript := newPTYTranscript(master)
	transcript.expectCountAndWrite(t, "Select networks", 1, " ") // all off
	transcript.expectCountAndWrite(t, "Select networks", 2, "j") // first network
	transcript.expectCountAndWrite(t, "Select networks", 3, " ") // first on
	transcript.expectCountAndWrite(t, "Select networks", 4, "j") // second network
	transcript.expectCountAndWrite(t, "Select networks", 5, " ") // second on
	transcript.expectCountAndWrite(t, "Select networks", 6, "k")
	transcript.expectCountAndWrite(t, "Select networks", 7, "\x1b[B")
	transcript.expectCountAndWrite(t, "Select networks", 8, "\x1b[A")
	transcript.expectCountAndWrite(t, "Select networks", 9, "\r")

	select {
	case selected := <-done:
		if len(selected) != 2 || selected[0].Name != "mainnet" || selected[1].Name != "sandbox" {
			t.Fatalf("selected networks = %+v, want mainnet and sandbox", selected)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("multi-select did not finish\nterminal transcript:\n%s", transcript.String())
	}
}

// TestSingleSelectThroughPTY proves unavailable rows are skipped in both
// directions for letter and arrow navigation before confirming a real value.
func TestSingleSelectThroughPTY(t *testing.T) {
	master, _, restore := attachBootstrapPTY(t)
	defer master.Close()
	defer restore()

	options := []selectOption{
		{value: "file", label: "file"},
		{value: "blocked", label: "blocked", unavailable: true},
		{value: "test", label: "test"},
	}
	done := make(chan string, 1)
	go func() { done <- singleSelect("Storage", options, 0) }()

	transcript := newPTYTranscript(master)
	transcript.expectCountAndWrite(t, "Storage", 1, "j")
	transcript.expectCountAndWrite(t, "Storage", 2, "k")
	transcript.expectCountAndWrite(t, "Storage", 3, "\x1b[B")
	transcript.expectCountAndWrite(t, "Storage", 4, "\x1b[A")
	transcript.expectCountAndWrite(t, "Storage", 5, "\r")

	select {
	case selected := <-done:
		if selected != "file" {
			t.Fatalf("selected option = %q, want file", selected)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("single-select did not finish\nterminal transcript:\n%s", transcript.String())
	}

	if got := singleSelect("none", nil, 0); got != "" {
		t.Fatalf("empty options selected %q", got)
	}
	if got := singleSelect("none", []selectOption{{value: "blocked", unavailable: true}}, -1); got != "" {
		t.Fatalf("all-unavailable options selected %q", got)
	}
}

func TestConsoleOnboardingThroughPTY(t *testing.T) {
	tests := []struct {
		name       string
		inputs     []struct{ wait, write string }
		wantOutput string
	}{
		{
			name: "declined",
			inputs: []struct{ wait, write string }{
				{"Configure a Console API key", "n\n"},
			},
			wantOutput: "Skipped",
		},
		{
			name: "empty secret",
			inputs: []struct{ wait, write string }{
				{"Configure a Console API key", "y\n"},
				{"Console API key:", "\n"},
			},
			wantOutput: "No key entered",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			master, _, restore := attachBootstrapPTY(t)
			defer master.Close()
			defer restore()

			type result struct {
				key   string
				route bool
			}
			done := make(chan result, 1)
			go func() {
				key, route := consoleOnboarding("sandbox")
				done <- result{key: key, route: route}
			}()

			transcript := newPTYTranscript(master)
			for _, input := range tc.inputs {
				transcript.expectAndWrite(t, input.wait, "")
				if input.wait == "Console API key:" {
					time.Sleep(10 * time.Millisecond)
				}
				transcript.write(t, input.write)
			}

			select {
			case got := <-done:
				if got.key != "" || got.route {
					t.Fatalf("onboarding result = %+v, want skipped", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("Console onboarding did not finish\nterminal transcript:\n%s", transcript.String())
			}
			transcript.expectAndWrite(t, tc.wantOutput, "")
		})
	}
}

func networkMetaJSON(chainID, rpc string) string {
	return fmt.Sprintf(`{
		"chain_id": %q,
		"fees": {"fee_tokens": [{"denom":"uakt","high_gas_price":0.025}]},
		"apis": {
			"rpc": [{"address":%q}],
			"rest": [{"address":"https://api.invalid"}],
			"grpc": [{"address":"grpc.invalid:443"}]
		}
	}`, chainID, rpc)
}

func attachBootstrapPTY(t *testing.T) (*os.File, *os.File, func()) {
	t.Helper()

	master, terminal, err := pty.Open()
	if err != nil {
		t.Fatalf("open pseudo-terminal: %v", err)
	}
	originalStdin, originalStderr := os.Stdin, os.Stderr
	os.Stdin, os.Stderr = terminal, terminal

	return master, terminal, func() {
		os.Stdin, os.Stderr = originalStdin, originalStderr
		_ = terminal.Close()
	}
}

type ptyTranscript struct {
	master *os.File
	mu     sync.Mutex
	data   bytes.Buffer
	update chan struct{}
	errs   chan error
}

func newPTYTranscript(master *os.File) *ptyTranscript {
	p := &ptyTranscript{
		master: master,
		update: make(chan struct{}, 1),
		errs:   make(chan error, 1),
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				p.mu.Lock()
				_, _ = p.data.Write(buf[:n])
				p.mu.Unlock()
				select {
				case p.update <- struct{}{}:
				default:
				}
			}
			if err != nil {
				p.errs <- err
				return
			}
		}
	}()

	return p
}

func (p *ptyTranscript) expectAndWrite(t *testing.T, expected, input string) {
	p.expectCountAndWrite(t, expected, 1, input)
}

func (p *ptyTranscript) expectCountAndWrite(t *testing.T, expected string, count int, input string) {
	t.Helper()

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for strings.Count(p.String(), expected) < count {
		select {
		case <-p.update:
		case err := <-p.errs:
			t.Fatalf("wait for terminal text %q: %v\ntranscript:\n%s", expected, err, p.String())
		case <-timer.C:
			t.Fatalf("timeout waiting for terminal text %q\ntranscript:\n%s", expected, p.String())
		}
	}
	if input != "" {
		p.write(t, input)
	}
}

func (p *ptyTranscript) write(t *testing.T, input string) {
	t.Helper()

	if _, err := io.WriteString(p.master, input); err != nil {
		t.Fatalf("write terminal input: %v", err)
	}
}

func (p *ptyTranscript) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.data.String()
}
