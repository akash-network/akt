package output

import (
	"bytes"
	"strings"
	"testing"
)

// A table with no rows must say so rather than print a header on its own: a
// lone header reads as a rendering failure, not as an empty result
// (SPEC §10.3).
func TestPrintTableNoRowsPrintsEmptyMessage(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, []Column{{Header: "NAME"}, {Header: "CHAIN-ID"}}, nil)

	got := strings.TrimSpace(buf.String())
	if got != EmptyTableMessage {
		t.Errorf("PrintTable with no rows = %q, want %q", got, EmptyTableMessage)
	}
}

func TestPrintTableRendersRows(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, []Column{{Header: "NAME"}, {Header: "CHAIN-ID"}}, [][]string{
		{"mainnet", "akashnet-2"},
	})

	got := buf.String()
	for _, want := range []string{"NAME", "CHAIN-ID", "mainnet", "akashnet-2"} {
		if !strings.Contains(got, want) {
			t.Errorf("PrintTable output %q is missing %q", got, want)
		}
	}
}
