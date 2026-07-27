package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Format represents an output format.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// FormatFromCmd reads the --output flag value from a cobra command.
func FormatFromCmd(cmd *cobra.Command) Format {
	val, _ := cmd.Flags().GetString("output")
	switch Format(val) {
	case FormatJSON:
		return FormatJSON
	case FormatYAML:
		return FormatYAML
	default:
		return FormatTable
	}
}

// Column defines a table column.
type Column struct {
	Header string
	Width  int // 0 = auto
}

// PrintData is the unified output function for list commands.
// For table format, it prints columns and rows as an aligned table.
// For JSON/YAML, it serializes the structured data.
func PrintData(cmd *cobra.Command, columns []Column, rows [][]string, data any) error {
	format := FormatFromCmd(cmd)

	switch format {
	case FormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case FormatYAML:
		return writeYAML(os.Stdout, data, 2)
	default:
		PrintTable(os.Stdout, columns, rows)
		return nil
	}
}

// Print formats and writes data to stdout in the given format.
func Print(format Format, data any) error {
	return Fprint(os.Stdout, format, data)
}

// Fprint formats and writes data to w in the given format.
func Fprint(w io.Writer, format Format, data any) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case FormatYAML:
		return writeYAML(w, data, 2)
	case FormatTable:
		return fmt.Errorf("table format requires a specific printer; use PrintTable instead")
	default:
		return fmt.Errorf("unknown output format %q", format)
	}
}

// PrintTable writes rows as an aligned table.
func PrintTable(w io.Writer, columns []Column, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)

	// Header.
	headers := make([]string, len(columns))
	for i, c := range columns {
		headers[i] = c.Header
	}

	fmt.Fprintln(tw, strings.Join(headers, "\t"))

	// Rows.
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	_ = tw.Flush()
}

func writeYAML(w io.Writer, data any, indent int) error {
	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	if indent > 0 {
		enc.SetIndent(indent)
	}

	if err := enc.Encode(data); err != nil {
		return err
	}

	if err := enc.Close(); err != nil {
		return err
	}

	payload := buf.Bytes()
	if len(payload) == 0 {
		return nil
	}

	if !bytes.HasPrefix(payload, []byte("---")) {
		if _, err := io.WriteString(w, "---\n"); err != nil {
			return err
		}
	}

	_, err := w.Write(payload)
	return err
}
