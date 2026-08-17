package output

import (
	flagdefs "pkg.akt.dev/akt/internal/flags"

	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// A table with no rows must say so rather than print a header on its own: a
// lone header reads as a rendering failure, not as an empty result
// (SPEC §10.3).
func TestPrintTableNoRowsPrintsEmptyMessage(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, PrintTable(&buf, []Column{{Header: "NAME"}, {Header: "CHAIN-ID"}}, nil))

	got := strings.TrimSpace(buf.String())
	if got != EmptyTableMessage {
		t.Errorf("PrintTable with no rows = %q, want %q", got, EmptyTableMessage)
	}
}

func TestPrintTableRendersRows(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, PrintTable(&buf, []Column{{Header: "NAME"}, {Header: "CHAIN-ID"}}, [][]string{
		{"mainnet", "akashnet-2"},
	}))

	got := buf.String()
	for _, want := range []string{"NAME", "CHAIN-ID", "mainnet", "akashnet-2"} {
		if !strings.Contains(got, want) {
			t.Errorf("PrintTable output %q is missing %q", got, want)
		}
	}
}

func TestPrintTablePropagatesFailureRaisedWhileWritingRow(t *testing.T) {
	wantErr := errors.New("row destination failed")
	err := PrintTable(
		outputBoundaryWriter{err: wantErr},
		[]Column{{Header: "NAME"}, {Header: "CHAIN-ID"}},
		[][]string{{"mainnet", "akashnet-2\f"}},
	)
	require.ErrorIs(t, err, wantErr)
}

func TestFormattersPropagateDestinationFailures(t *testing.T) {
	wantErr := errors.New("destination failed")
	columns := []Column{{Header: "NAME"}}
	rows := [][]string{{"mainnet"}}
	data := map[string]any{"name": "mainnet"}

	operations := []struct {
		name string
		run  func(io.Writer) error
	}{
		{name: "generic JSON", run: func(w io.Writer) error { return Fprint(w, FormatJSON, data) }},
		{name: "generic YAML", run: func(w io.Writer) error { return Fprint(w, FormatYAML, data) }},
		{name: "JSON semantics JSON", run: func(w io.Writer) error { return FprintJSONSemantics(w, FormatJSON, data) }},
		{name: "JSON semantics YAML", run: func(w io.Writer) error { return FprintJSONSemantics(w, FormatYAML, data) }},
		{name: "table rows", run: func(w io.Writer) error { return PrintTable(w, columns, rows) }},
		{name: "empty table", run: func(w io.Writer) error { return PrintTable(w, columns, nil) }},
	}
	failures := []struct {
		name string
		w    io.Writer
		want error
	}{
		{name: "hard error", w: outputBoundaryWriter{err: wantErr}, want: wantErr},
		{name: "short write", w: outputBoundaryWriter{short: true}, want: io.ErrShortWrite},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			for _, failure := range failures {
				t.Run(failure.name, func(t *testing.T) {
					require.ErrorIs(t, operation.run(failure.w), failure.want)
				})
			}
		})
	}
}

func TestPrintDataUsesCommandWriterAndPropagatesFailures(t *testing.T) {
	wantErr := errors.New("command stdout failed")
	columns := []Column{{Header: "NAME"}}
	rows := [][]string{{"mainnet"}}
	data := map[string]any{"name": "mainnet"}

	for _, format := range []string{"json", "yaml", "table"} {
		t.Run(format, func(t *testing.T) {
			for _, failure := range []struct {
				name string
				w    io.Writer
				want error
			}{
				{name: "hard error", w: outputBoundaryWriter{err: wantErr}, want: wantErr},
				{name: "short write", w: outputBoundaryWriter{short: true}, want: io.ErrShortWrite},
			} {
				t.Run(failure.name, func(t *testing.T) {
					cmd := &cobra.Command{}
					cmd.Flags().String(flagdefs.FlagOutput, format, "")
					cmd.SetOut(failure.w)

					require.ErrorIs(t, PrintData(cmd, columns, rows, data), failure.want)
				})
			}
		})
	}
}
