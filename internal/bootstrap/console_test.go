package bootstrap

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseYesNo(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
	}{
		{"y\n", false, true},
		{"Y\n", false, true},
		{"yes\n", false, true},
		{"n\n", true, false},
		{"NO\n", true, false},
		{"\n", false, false},
		{"\n", true, true},
		{"maybe\n", false, false},
		{"maybe\n", true, true},
		{"  y  \n", false, true},
	}

	for _, c := range cases {
		if got := parseYesNo(c.in, c.def); got != c.want {
			t.Errorf("parseYesNo(%q, %v) = %v, want %v", c.in, c.def, got, c.want)
		}
	}
}

func TestConsoleOnboardingSkipsWithoutTTY(t *testing.T) {
	// Test processes have no TTY on stdin, so onboarding must be a no-op.
	key, route := consoleOnboarding("prod")
	if key != "" || route {
		t.Errorf("expected non-interactive skip, got key=%q route=%v", key, route)
	}
}

func TestRunSkipsHeadless(t *testing.T) {
	// Test processes have no TTY: Run must decline to bootstrap — no
	// network fetch, no config written, nil error so the CLI continues
	// to its normal no-config handling.
	root := t.TempDir()

	if err := Run(root); err != nil {
		t.Fatalf("headless Run must not error, got: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "config.yaml")); !os.IsNotExist(err) {
		t.Fatal("headless Run must not write a config file")
	}
}

func TestPromptYesNoUsesInputAndDefault(t *testing.T) {
	tests := []struct {
		name  string
		input string
		def   bool
		want  bool
	}{
		{name: "explicit yes", input: "yes\n", want: true},
		{name: "explicit no", input: "no\n", def: true, want: false},
		{name: "empty default yes", input: "\n", def: true, want: true},
		{name: "read error uses default", def: true, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := withBootstrapIO(t, tc.input, func() {
				if got := promptYesNo("Continue?", tc.def); got != tc.want {
					t.Errorf("promptYesNo = %v, want %v", got, tc.want)
				}
			})
			if !strings.Contains(output, "Continue?") {
				t.Errorf("prompt did not name the question: %q", output)
			}
		})
	}
}

func TestPromptSecretFailsClosedWithoutTerminal(t *testing.T) {
	output := withBootstrapIO(t, "not-a-terminal\n", func() {
		if got := promptSecret("Secret: "); got != "" {
			t.Errorf("promptSecret on a pipe = %q, want empty", got)
		}
	})
	if !strings.Contains(output, "Secret: ") {
		t.Errorf("secret prompt output = %q", output)
	}
}

func TestValidateConsoleKeyReportsSuccessAndFailure(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    string
		wantKey string
	}{
		{
			name:    "authenticated",
			status:  http.StatusOK,
			body:    `{"data":{"id":"u1","username":"alice"}}`,
			want:    "Authenticated as alice",
			wantKey: "test-key",
		},
		{
			name:    "rejected but stored",
			status:  http.StatusUnauthorized,
			body:    `{"message":"invalid"}`,
			want:    "Could not validate the key",
			wantKey: "bad-key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transport := installDefaultTransport(t, map[string]stubResponse{
				"/v1/user/me": {status: tc.status, body: tc.body},
			})
			output := withBootstrapIO(t, "", func() { validateConsoleKey(tc.wantKey) })
			if !strings.Contains(output, tc.want) {
				t.Errorf("validation output = %q, want %q", output, tc.want)
			}
			if len(transport.seen) != 1 {
				t.Fatalf("Console validation requests = %v, want one", transport.seen)
			}
		})
	}
}

func withBootstrapIO(t *testing.T, stdin string, fn func()) string {
	t.Helper()

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if stdin != "" {
		if _, err := io.WriteString(inW, stdin); err != nil {
			t.Fatalf("write stdin fixture: %v", err)
		}
	}
	if err := inW.Close(); err != nil {
		t.Fatalf("close stdin fixture: %v", err)
	}

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	previousIn, previousErr := os.Stdin, os.Stderr
	os.Stdin, os.Stderr = inR, outW
	t.Cleanup(func() {
		os.Stdin, os.Stderr = previousIn, previousErr
		_ = inR.Close()
		_ = outR.Close()
		_ = outW.Close()
	})

	fn()
	os.Stdin, os.Stderr = previousIn, previousErr
	if err := outW.Close(); err != nil {
		t.Fatalf("close stderr fixture: %v", err)
	}
	output, err := io.ReadAll(outR)
	if err != nil {
		t.Fatalf("read stderr fixture: %v", err)
	}

	return string(output)
}
