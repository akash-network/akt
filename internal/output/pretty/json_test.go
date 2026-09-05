package pretty

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestWriteHighlightedJSON(t *testing.T) {
	tests := map[string]struct {
		input []byte
	}{
		"SimpleObject": {
			input: []byte(`{"key":"value","num":42}`),
		},
		"Nested": {
			input: []byte(`{"name":"test","count":10,"tags":["alpha","beta"],"nested":{"flag":true,"nothing":null}}`),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf strings.Builder
			err := WriteHighlightedJSON(&buf, tc.input)
			if err != nil {
				t.Fatalf("WriteHighlightedJSON returned error: %v", err)
			}
			golden.RequireEqual(t, buf.String())
		})
	}
}

func TestWriteHighlightedJSONPreservesValidJSON(t *testing.T) {
	input := []byte(`{"name":"test","count":10,"tags":["alpha","beta"],"nested":{"flag":true,"nothing":null}}`)

	var buf strings.Builder
	if err := WriteHighlightedJSON(&buf, input); err != nil {
		t.Fatalf("WriteHighlightedJSON returned error: %v", err)
	}
	rendered := []byte(ansi.Strip(buf.String()))
	if !json.Valid(rendered) {
		t.Fatalf("highlighted JSON is invalid after stripping ANSI styling:\n%s", rendered)
	}
}

func TestWriteHighlightedJSONPropagatesWriterFailures(t *testing.T) {
	wantErr := errors.New("output unavailable")

	for _, input := range []struct {
		name string
		data []byte
	}{
		{name: "valid JSON", data: []byte(`{"status":"ready"}`)},
		{name: "invalid JSON fallback", data: []byte(`{"status":`)},
	} {
		t.Run(input.name, func(t *testing.T) {
			requirements := []struct {
				name string
				w    io.Writer
				want error
			}{
				{name: "hard error", w: errorWriter{err: wantErr}, want: wantErr},
				{name: "short write", w: shortWriter{}, want: io.ErrShortWrite},
			}

			for _, requirement := range requirements {
				t.Run(requirement.name, func(t *testing.T) {
					if err := WriteHighlightedJSON(requirement.w, input.data); !errors.Is(err, requirement.want) {
						t.Fatalf("WriteHighlightedJSON() error = %v, want %v", err, requirement.want)
					}
				})
			}
		})
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}
