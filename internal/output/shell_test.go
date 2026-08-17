package output

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func TestRunShellOutputPrettyStreamsDirectly(t *testing.T) {
	cmd, stdout, stderr := shellOutputCommand(t, "pretty")
	wantErr := errors.New("remote exit")

	err := RunShellOutput(cmd, false, true, func(out, errOut io.Writer, tty bool) error {
		if !tty {
			t.Fatal("pretty shell did not preserve TTY allocation")
		}
		_, _ = io.WriteString(out, "remote stdout\n")
		_, _ = io.WriteString(errOut, "remote stderr\n")
		return wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("RunShellOutput error = %v, want %v", err, wantErr)
	}
	if got := stdout.String(); got != "remote stdout\n" {
		t.Fatalf("stdout = %q", got)
	}
	if got := stderr.String(); got != "remote stderr\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRunShellOutputStructuredCapturesStreams(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			cmd, stdout, stderr := shellOutputCommand(t, format)
			wantErr := errors.New("remote exit")

			err := RunShellOutput(cmd, false, true, func(out, errOut io.Writer, tty bool) error {
				if tty {
					t.Fatal("structured shell allocated a TTY")
				}
				_, _ = io.WriteString(out, "remote stdout\n")
				_, _ = io.WriteString(errOut, "remote stderr\n")
				return wantErr
			})

			if !errors.Is(err, wantErr) {
				t.Fatalf("RunShellOutput error = %v, want %v", err, wantErr)
			}
			if stderr.Len() != 0 {
				t.Fatalf("diagnostic stderr = %q, want empty", stderr.String())
			}

			var got struct {
				Stdout string `json:"stdout" yaml:"stdout"`
				Stderr string `json:"stderr" yaml:"stderr"`
			}
			if format == "json" {
				if decodeErr := json.Unmarshal(stdout.Bytes(), &got); decodeErr != nil {
					t.Fatalf("decode JSON %q: %v", stdout.String(), decodeErr)
				}
			} else if decodeErr := yaml.Unmarshal(stdout.Bytes(), &got); decodeErr != nil {
				t.Fatalf("decode YAML %q: %v", stdout.String(), decodeErr)
			}
			if got.Stdout != "remote stdout\n" || got.Stderr != "remote stderr\n" {
				t.Fatalf("structured shell output = %#v", got)
			}
		})
	}
}

func TestRunShellOutputRefusesStructuredInteractiveShell(t *testing.T) {
	cmd, _, _ := shellOutputCommand(t, "json")
	run := false

	err := RunShellOutput(cmd, true, true, func(io.Writer, io.Writer, bool) error {
		run = true
		return nil
	})

	if err == nil || !strings.Contains(err.Error(), "explicit remote command") {
		t.Fatalf("RunShellOutput error = %v", err)
	}
	if run {
		t.Fatal("structured interactive refusal ran the remote shell")
	}
}

func TestRunShellOutputPreservesRemoteAndRenderErrors(t *testing.T) {
	cmd, _, _ := shellOutputCommand(t, "json")
	runErr := errors.New("remote exit")
	renderErr := errors.New("render failed")
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	if err := reader.CloseWithError(renderErr); err != nil {
		t.Fatalf("close pipe reader: %v", err)
	}
	cmd.SetOut(writer)

	err := RunShellOutput(cmd, false, false, func(io.Writer, io.Writer, bool) error {
		return runErr
	})

	if !errors.Is(err, runErr) {
		t.Fatalf("RunShellOutput error = %v, want remote cause %v", err, runErr)
	}
	if !errors.Is(err, renderErr) {
		t.Fatalf("RunShellOutput error = %v, want renderer cause %v", err, renderErr)
	}
}

func shellOutputCommand(t *testing.T, format string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	cmd := &cobra.Command{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().String(flagdefs.FlagOutput, format, "")
	return cmd, stdout, stderr
}
