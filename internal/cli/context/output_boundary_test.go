package context

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
)

type contextFaultWriter struct {
	err error
}

func (w contextFaultWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type contextSequencedWriter struct {
	err    error
	failAt int
	writes int
}

func (w *contextSequencedWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}

	return len(p), nil
}

func TestPromptNetworkEditModeHonorsCommandIO(t *testing.T) {
	t.Run("destination failure", func(t *testing.T) {
		writeErr := errors.New("network prompt unavailable")
		cmd := &cobra.Command{}
		cmd.SetErr(contextFaultWriter{err: writeErr})
		cmd.SetIn(strings.NewReader("2\n"))

		fork, err := promptNetworkEditMode(cmd, "mainnet", "prod")
		if fork {
			t.Fatal("a failed prompt must not select a fork")
		}
		if !errors.Is(err, writeErr) {
			t.Fatalf("prompt error = %v, want destination error", err)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetIn(strings.NewReader(""))

		fork, err := promptNetworkEditMode(cmd, "mainnet", "prod")
		if fork {
			t.Fatal("empty input must not select a fork")
		}
		if err == nil || !strings.Contains(err.Error(), "read network edit selection") {
			t.Fatalf("prompt error = %v, want input error", err)
		}
	})

	for _, test := range []struct {
		name      string
		input     string
		wantFork  bool
		wantError string
	}{
		{name: "edit parent", input: "1\n"},
		{name: "fork", input: "2\n", wantFork: true},
		{name: "reject invalid", input: "3\n", wantError: "invalid selection"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetIn(strings.NewReader(test.input))

			fork, err := promptNetworkEditMode(cmd, "mainnet", "prod")
			if fork != test.wantFork {
				t.Errorf("fork = %t, want %t", fork, test.wantFork)
			}
			if test.wantError == "" && err != nil {
				t.Fatalf("prompt: %v", err)
			}
			if test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError)) {
				t.Fatalf("prompt error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestDeletePromptFailuresPreserveContext(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		writer    func(error) io.Writer
		wantError string
	}{
		{
			name: "confirmation destination",
			writer: func(writeErr error) io.Writer {
				return contextFaultWriter{err: writeErr}
			},
		},
		{
			name:      "empty input",
			input:     "",
			writer:    func(error) io.Writer { return &bytes.Buffer{} },
			wantError: "read delete confirmation",
		},
		{
			name:  "cancellation destination",
			input: "n\n",
			writer: func(writeErr error) io.Writer {
				return &contextSequencedWriter{err: writeErr, failAt: 2}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newTestManager(t)
			mgrFn := func() *aktctx.Manager { return m }
			runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet")

			writeErr := errors.New("delete prompt unavailable")
			cmd := deleteCmd(mgrFn)
			cmd.SetIn(strings.NewReader(test.input))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(test.writer(writeErr))
			cmd.SetArgs([]string{"prod"})

			err := cmd.Execute()
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("delete error = %v, want %q", err, test.wantError)
				}
			} else if !errors.Is(err, writeErr) {
				t.Fatalf("delete error = %v, want destination error", err)
			}
			if m.GetContext("prod") == nil {
				t.Fatal("a failed confirmation boundary removed the context")
			}
		})
	}
}

func TestEmptyActionLogStructuredOutput(t *testing.T) {
	m := newTestManager(t)
	mgrFn := func() *aktctx.Manager { return m }
	runOK(t, createCmd(mgrFn), "prod", "--network", "mainnet", "--set-current")

	t.Run("empty sequence", func(t *testing.T) {
		cmd := logCmd(mgrFn)
		cmd.Flags().Var(output.NewFormatFlag("pretty"), "output", "test output format")
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"--type", "workflow", "--output", "json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("context log: %v", err)
		}
		var entries []map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
			t.Fatalf("decode empty action log %q: %v", stdout.String(), err)
		}
		if len(entries) != 0 {
			t.Fatalf("empty action log = %#v", entries)
		}
	})

	t.Run("destination failure", func(t *testing.T) {
		writeErr := errors.New("action-log output unavailable")
		cmd := logCmd(mgrFn)
		cmd.Flags().Var(output.NewFormatFlag("pretty"), "output", "test output format")
		cmd.SetOut(contextFaultWriter{err: writeErr})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"--type", "workflow", "--output", "json"})

		if err := cmd.Execute(); !errors.Is(err, writeErr) {
			t.Fatalf("context log error = %v, want destination error", err)
		}
	})
}
