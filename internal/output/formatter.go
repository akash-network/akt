package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
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

// FprintJSONSemantics formats JSON-backed data without letting YAML reflection
// change its field names or scalar types. It is opt-in for callers whose data
// model is defined by JSON tags and json.Marshaler implementations, such as the
// Console API. Generic Fprint keeps its existing YAML-tag behavior.
func FprintJSONSemantics(w io.Writer, format Format, data any) error {
	if format != FormatYAML {
		return Fprint(w, format, data)
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}

	root, err := jsonValueYAMLNode(value)
	if err != nil {
		return err
	}

	document := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{root},
	}

	return writeYAML(w, document, 2)
}

func jsonValueYAMLNode(value any) (*yaml.Node, error) {
	switch value := value.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(value)}, nil
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}, nil
	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(value.String(), ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value.String()}, nil
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range value {
			child, err := jsonValueYAMLNode(item)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, key := range keys {
			child, err := jsonValueYAMLNode(value[key])
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
				child,
			)
		}
		return node, nil
	default:
		return nil, fmt.Errorf("unsupported decoded JSON value %T", value)
	}
}

// EmptyTableMessage is what PrintTable writes when it has no rows.
const EmptyTableMessage = "(no results)"

// PrintTable writes rows as an aligned table.
//
// With no rows it writes EmptyTableMessage instead of a header on its own: a
// lone header reads as a rendering failure rather than as an empty result
// (SPEC §10.3). Callers that can name what is missing should say so themselves
// and skip this call. This is the legacy tabwriter engine, kept for the
// remaining PrintData callers; pretty output uses
// pretty.WriteTableOrEmpty (SPEC §10.12).
func PrintTable(w io.Writer, columns []Column, rows [][]string) {
	if len(rows) == 0 {
		fmt.Fprintln(w, EmptyTableMessage)
		return
	}

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
