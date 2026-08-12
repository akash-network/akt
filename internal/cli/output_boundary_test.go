package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	aktctx "pkg.akt.dev/akt/internal/context"
	"pkg.akt.dev/akt/internal/output"
)

type commandOutputErrorWriter struct {
	err error
}

func (w commandOutputErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type commandOutputShortWriter struct{}

func (commandOutputShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestVersionStructuredOutputUsesCommandWriter(t *testing.T) {
	cmd := versionCmd(BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-08-11"})
	cmd.Flags().VarP(output.NewFormatFlag("pretty"), "output", "o", "test output")

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version JSON: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode version JSON %q: %v", stdout.String(), err)
	}
	if got["version"] != "1.2.3" || got["commit"] != "abc123" {
		t.Fatalf("version JSON = %#v", got)
	}

	sentinel := errors.New("version destination failed")
	cmd = versionCmd(BuildInfo{Version: "1.2.3"})
	cmd.Flags().VarP(output.NewFormatFlag("pretty"), "output", "o", "test output")
	cmd.SetOut(commandOutputErrorWriter{err: sentinel})
	cmd.SetArgs([]string{"--output", "yaml"})
	if err := cmd.Execute(); !errors.Is(err, sentinel) {
		t.Fatalf("version error = %v, want %v", err, sentinel)
	}
}

func TestVersionPrettyOutputRejectsShortWrites(t *testing.T) {
	for _, args := range [][]string{nil, {"--long"}} {
		name := "short"
		if len(args) != 0 {
			name = "long"
		}
		t.Run(name, func(t *testing.T) {
			cmd := versionCmd(BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-08-11"})
			cmd.Flags().VarP(output.NewFormatFlag("pretty"), "output", "o", "test output")
			cmd.SetOut(commandOutputShortWriter{})
			cmd.SetArgs(args)

			if err := cmd.Execute(); !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("version error = %v, want %v", err, io.ErrShortWrite)
			}
		})
	}
}

func TestCompletionScriptsUseCommandWriter(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			cmd := completionCmd()
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{shell})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("completion %s: %v", shell, err)
			}
			if !strings.Contains(strings.ToLower(stdout.String()), "completion") {
				t.Fatalf("completion %s did not use command writer: %q", shell, stdout.String())
			}
		})
	}
}

func TestMonitorCleanCachePropagatesDiagnosticFailure(t *testing.T) {
	home := t.TempDir()
	cacheDir := filepath.Join(home, "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("create cache directory: %v", err)
	}
	for _, name := range []string{"monitor.db", "top.db"} {
		if err := os.WriteFile(filepath.Join(cacheDir, name), []byte("stale"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	v := viper.New()
	v.Set("defaults.interactive", true)
	cmd := monitorCmd(
		v,
		func() string { return home },
		func() *aktctx.Manager { return nil },
		func() string { return "" },
	)
	writeErr := errors.New("cache diagnostic destination failed")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(commandOutputErrorWriter{err: writeErr})
	cmd.SetArgs([]string{"--clean-cache", "http://127.0.0.1:26657"})

	if err := cmd.Execute(); !errors.Is(err, writeErr) {
		t.Fatalf("monitor error = %v, want destination error", err)
	}
	for _, name := range []string{"monitor.db", "top.db"} {
		if _, err := os.Stat(filepath.Join(cacheDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s remains after clean-cache: %v", name, err)
		}
	}
}

func TestMonitorCleanCacheRejectsShortDiagnosticWrite(t *testing.T) {
	home := t.TempDir()
	cacheDir := filepath.Join(home, "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("create cache directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "monitor.db"), []byte("stale"), 0o600); err != nil {
		t.Fatalf("write monitor cache: %v", err)
	}

	v := viper.New()
	v.Set("defaults.interactive", true)
	cmd := monitorCmd(
		v,
		func() string { return home },
		func() *aktctx.Manager { return nil },
		func() string { return "" },
	)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetErr(commandOutputShortWriter{})
	cmd.SetArgs([]string{"--clean-cache", "http://127.0.0.1:26657"})

	if err := cmd.Execute(); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("monitor error = %v, want %v", err, io.ErrShortWrite)
	}
}
