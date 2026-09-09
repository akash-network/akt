package pretty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	clioutput "pkg.akt.dev/akt/internal/output"
)

// WriteHighlightedJSON writes syntax-highlighted, indented JSON to w.
// Keys are cyan, strings green, numbers yellow, booleans magenta, null gray.
func WriteHighlightedJSON(w io.Writer, data []byte) error {
	checked := clioutput.NewCheckedWriter(w)

	// Pretty-print with indentation first.
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		// If we can't indent (not valid JSON), write as-is.
		_, writeErr := checked.Write(data)
		return checked.Complete(writeErr)
	}

	// Tokenize and colorize.
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.UseNumber()

	return checked.Complete(highlightJSON(checked, &buf))
}

// highlightJSON does a simple line-by-line colorization of indented JSON.
// This is simpler and more reliable than token-based colorization.
func highlightJSON(w io.Writer, buf *bytes.Buffer) error {
	lines := strings.Split(buf.String(), "\n")
	for _, line := range lines {
		colored := colorizeJSONLine(line)
		if _, err := fmt.Fprintln(w, colored); err != nil {
			return err
		}
	}
	return nil
}

// colorizeJSONLine applies syntax highlighting to a single JSON line.
func colorizeJSONLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return line
	}

	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]

	// Handle lines that are just structural characters.
	switch trimmed {
	case "{", "}", "[", "]", "{}", "[]",
		"},", "],":
		return line
	}

	// Try to split on key: value.
	if idx := strings.Index(trimmed, ":"); idx > 0 && trimmed[0] == '"' {
		// Find the end of the key.
		keyEnd := strings.Index(trimmed[1:], "\"")
		if keyEnd >= 0 {
			key := trimmed[:keyEnd+2] // includes quotes
			rest := trimmed[keyEnd+2:]

			coloredKey := StyleCyan.Render(key)
			coloredVal := colorizeJSONValue(strings.TrimPrefix(rest, ": "))

			return indent + coloredKey + ": " + coloredVal
		}
	}

	// Otherwise, colorize as a value.
	return indent + colorizeJSONValue(trimmed)
}

// colorizeJSONValue applies color to a JSON value string.
func colorizeJSONValue(val string) string {
	raw := strings.TrimSpace(val)
	trailing := ""
	if strings.HasSuffix(raw, ",") {
		trailing = ","
		raw = strings.TrimSuffix(raw, ",")
	}

	switch {
	case raw == "null":
		return StyleGray.Render(raw) + trailing
	case raw == "true" || raw == "false":
		return StyleMagenta.Render(raw) + trailing
	case len(raw) > 0 && raw[0] == '"':
		return StyleGreen.Render(raw) + trailing
	case len(raw) > 0 && (raw[0] >= '0' && raw[0] <= '9' || raw[0] == '-'):
		return StyleYellow.Render(raw) + trailing
	default:
		return val
	}
}
