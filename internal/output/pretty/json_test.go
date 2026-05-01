package pretty

import (
	"strings"
	"testing"

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
