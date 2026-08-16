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

type streamRecord struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func TestPrintStreamRecordRejectsDestinationFailures(t *testing.T) {
	record := streamRecord{Name: "web-a", Message: "ready"}
	wantErr := errors.New("stream destination failed")

	for _, format := range []string{"pretty", "json", "yaml"} {
		for _, tc := range []struct {
			name string
			out  outputBoundaryWriter
			want error
		}{
			{name: "hard error", out: outputBoundaryWriter{err: wantErr}, want: wantErr},
			{name: "short write", out: outputBoundaryWriter{short: true}, want: io.ErrShortWrite},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				cmd := &cobra.Command{}
				cmd.Flags().String(flagdefs.FlagOutput, format, "")
				cmd.SetOut(tc.out)

				err := PrintStreamRecord(cmd, record, "[web-a] ready")
				if !errors.Is(err, tc.want) {
					t.Fatalf("PrintStreamRecord() error = %v, want %v", err, tc.want)
				}
			})
		}
	}
}

func TestPrintStreamRecord(t *testing.T) {
	record := streamRecord{Name: "web-a", Message: "ready"}

	for _, tc := range []struct {
		format string
		check  func(*testing.T, string)
	}{
		{"pretty", func(t *testing.T, got string) {
			if got != "[web-a] ready\n" {
				t.Fatalf("pretty output = %q", got)
			}
		}},
		{"json", func(t *testing.T, got string) {
			var decoded streamRecord
			if err := json.Unmarshal([]byte(got), &decoded); err != nil {
				t.Fatalf("decode JSONL: %v", err)
			}
			if decoded != record || strings.Count(got, "\n") != 1 {
				t.Fatalf("JSONL output = %q", got)
			}
		}},
		{"yaml", func(t *testing.T, got string) {
			var decoded streamRecord
			if err := yaml.Unmarshal([]byte(got), &decoded); err != nil {
				t.Fatalf("decode YAML: %v", err)
			}
			if decoded != record || !strings.HasPrefix(got, "---\n") {
				t.Fatalf("YAML output = %q", got)
			}
		}},
	} {
		t.Run(tc.format, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String(flagdefs.FlagOutput, tc.format, "")
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			if err := PrintStreamRecord(cmd, record, "[web-a] ready"); err != nil {
				t.Fatalf("PrintStreamRecord: %v", err)
			}
			tc.check(t, buf.String())
		})
	}
}
